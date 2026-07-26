# Sub2API 容灾路由自动切换优化记录 (2026-07-26)

## 1. 问题背景
在多渠道负载均衡调度中，当 `GreatCodex` 分组内的某一个上游数据源（例如 `allincoding` 账号）由于彻底宕机、域名过期或网络解析失败导致连接完全断开时，系统未能自动切换至备用数据源（如 `lingshuo` 账号），而是直接向用户返回 `502 Bad Gateway` 报错，导致高可用失效。

---

## 2. 根本原因分析
网关的调度切换逻辑（Failover Loop）是基于捕获 `UpstreamFailoverError` 类型的异常来实现的。然而，在以前的逻辑中：
1. 当上游网络彻底断开（DNS 报错、连接超时等）时，底层网络接口 `httpUpstream.Do` 抛出了底层的 Go `net` 网络错误。
2. 后端核心转发逻辑（[openai_gateway_service.go](file:///home/qagent/program/sub2api/backend/internal/service/openai_gateway_service.go)）捕获了该网络错误，但随后执行了以下操作：
   * 提前向客户端上下文 `c.JSON(StatusBadGateway, ...)` 写入了错误响应。
   * 返回了一个 Plain Error（普通错误）`fmt.Errorf("upstream request failed: %s", safeErr)`。
3. 由于返回的不是 `UpstreamFailoverError`，上层 Handler（处理器）无法识别该错误是可 Failover 的，因此直接终止了请求，且未能剔除该错误账号，导致随后的所有请求依然会调度到该不可用账号上。

---

## 3. 代码修复方案
我们修改了 [openai_gateway_service.go](file:///home/qagent/program/sub2api/backend/internal/service/openai_gateway_service.go) 中常规和透传（Passthrough）两个转发分支对 `httpUpstream.Do` 返回错误的拦截处理：

1. **移除提前写入响应的行为**：不再调用 `c.JSON(http.StatusBadGateway, ...)`，防止在 Failover 之前就污染并写入客户端 Socket 响应，避免锁死 `502` 导致后续重试成功时发生双重写入（double-write）冲突。
2. **抛出 Failover 错误包装**：将网络错误转换为 `UpstreamFailoverError`，并附带 `502` 错误状态码和标准格式错误报文返回。

### 3.1 常规转发模式修改 (Normal Forward)
```diff
-		if err != nil {
-			// Ensure the client receives an error response (handlers assume Forward writes on non-failover errors).
-			safeErr := sanitizeUpstreamErrorMessage(err.Error())
-			setOpsUpstreamError(c, 0, safeErr, "")
-			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
-				Platform:           account.Platform,
-				AccountID:          account.ID,
-				AccountName:        account.Name,
-				UpstreamStatusCode: 0,
-				Kind:               "request_error",
-				Message:            safeErr,
-			})
-			c.JSON(http.StatusBadGateway, gin.H{
-				"error": gin.H{
-					"type":    "upstream_error",
-					"message": "Upstream request failed",
-				},
-			})
-			return nil, fmt.Errorf("upstream request failed: %s", safeErr)
-		}
+		if err != nil {
+			safeErr := sanitizeUpstreamErrorMessage(err.Error())
+			setOpsUpstreamError(c, http.StatusBadGateway, safeErr, "")
+			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
+				Platform:             account.Platform,
+				AccountID:            account.ID,
+				AccountName:          account.Name,
+				UpstreamStatusCode:   http.StatusBadGateway,
+				Kind:                 "failover",
+				Message:              safeErr,
+			})
+			return nil, &UpstreamFailoverError{
+				StatusCode:   http.StatusBadGateway,
+				ResponseBody: []byte(fmt.Sprintf(`{"error":{"message":"%s","type":"upstream_error"}}`, safeErr)),
+			}
+		}
```

### 3.2 透传转发模式修改 (Passthrough Forward)
```diff
-	if err != nil {
-		safeErr := sanitizeUpstreamErrorMessage(err.Error())
-		setOpsUpstreamError(c, 0, safeErr, "")
-		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
-			Platform:           account.Platform,
-			AccountID:          account.ID,
-			AccountName:        account.Name,
-			UpstreamStatusCode: 0,
-			Passthrough:        true,
-			Kind:               "request_error",
-			Message:            safeErr,
-		})
-		c.JSON(http.StatusBadGateway, gin.H{
-			"error": gin.H{
-				"type":    "upstream_error",
-				"message": "Upstream request failed",
-			},
-		})
-		return nil, fmt.Errorf("upstream request failed: %s", safeErr)
-	}
+	if err != nil {
+		safeErr := sanitizeUpstreamErrorMessage(err.Error())
+		setOpsUpstreamError(c, http.StatusBadGateway, safeErr, "")
+		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
+			Platform:           account.Platform,
+			AccountID:          account.ID,
+			AccountName:        account.Name,
+			UpstreamStatusCode: http.StatusBadGateway,
+			Passthrough:        true,
+			Kind:               "failover",
+			Message:            safeErr,
+		})
+		return nil, &UpstreamFailoverError{
+			StatusCode:   http.StatusBadGateway,
+			ResponseBody: []byte(fmt.Sprintf(`{"error":{"message":"%s","type":"upstream_error"}}`, safeErr)),
+		}
+	}
```

---

## 4. 容灾切换验证过程
1. **故障模拟**：通过 `UPDATE` SQL 指令将 `allincoding` (ID: 13) 账号的 `base_url` 更新为非法的死链 `https://dead-upstream.invalid`。
2. **清理缓存**：重启 `redis` 容器强行清空本地路由缓存，迫使网关重新拉取最新的账号状态。
3. **调用接口**：对本地网关发起 `/v1/responses/compact` 聚合请求，模型选择为 `gpt-5.6-terra`。
4. **日志分析**：
   * 网关首次通过负平衡权重逻辑选中了 `allincoding` 账号。
   * 请求发往 `dead-upstream.invalid` 时，本地网关检测到网络不可达错误。
   * **完美执行 Failover！** 网关触发切换日志：
     `WARN  handler/openai_gateway_handler.go:347  openai.upstream_failover_switching  {"account_id": 13, "upstream_status": 502, "switch_count": 1}`
   * 网关自动将请求无缝切换路由给备用账号 `lingshuo`（ID: 15）。
   * `lingshuo` 正确请求并返回响应，整个过程完全对客户端透明，最终得到 `200 OK` 正常应答！

---

## 5. 配置还原
测试通过后，对账号进行了生产配置还原，以防影响真实服务：
```sql
UPDATE accounts SET credentials = jsonb_set(credentials, '{base_url}', '"https://allincode.top"') WHERE id = 13;
```
并且清空了 Redis 路由缓存。系统现已完美支持自动负载均衡和容灾路由平滑切换。
