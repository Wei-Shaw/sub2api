# 减少长会话请求的复制与持有

长会话中的图片、工具输出和历史消息会使单次请求达到数十 MiB。请求体预算控制同时处理的数据量；本次调整进一步减少单次处理的分配，并让大缓冲随所属请求释放。

## 调整范围

- 模型候选读取同步借用请求字节，返回的短模型名独立拥有字符串。请求视图中的模型、会话和计费短字段采用同样的拥有权。
- OAuth 输入元数据逐项清理，复用其他消息的原始 JSON，最后集中合成一次完整请求。重复键、转义字段、用户内容和大整数沿用原语义。
- 压缩标记整理保留现有普通请求快速检查；包含标记的请求按项读取，重复键与特殊输入沿用完整解码路径。
- OpenCode 协议转换保存经过校验的小型会话提示，沿用会话来源优先级。
- 内容审计队列保存任务指针，完整任务在实际入队时分配，继续沿用队列容量、处理和统计逻辑。

功能、API、数据库和 Redis 契约沿用现有定义。

## 固定输入对照

基线为 `530c5e3a4f9c73ca2b0881191d0ef8e0d82c5484`，目标文件与 `v0.2.21` 相同。环境为 Apple M1 Pro、Go 1.27.3、`GOMAXPROCS=2`，每个场景执行一次。输入由测试生成，图片字段使用固定字符填充。

下表的分配量为对应处理步骤单次调用的累计分配，进程常驻内存另行测量。

| 场景 | 基线分配 | 当前分配 | 基线耗时 | 当前耗时 |
| --- | ---: | ---: | ---: | ---: |
| 16MiB 图片＋50 条元数据清理 | 856,201,368B | 16,797,968B | 248.43ms | 20.01ms |
| 112MiB 图片＋50 条元数据清理 | 5,990,028,952B | 117,461,264B | 1,717.43ms | 137.10ms |
| 112MiB 图片，标记已在末尾 | 385,892,640B | 616B | 260.57ms | 227.01ms |
| 112MiB 图片，标记移至末尾 | 620,792,400B | 117,449,352B | 364.25ms | 429.59ms |

标记移动路径增加嵌套重复键检查，以沿用完整解码的取值语义。该场景的分配量下降约 81%，本次单次测量耗时由 364.25ms 增至 429.59ms；标记已在末尾时直接复用原始请求。

内容审计空服务当前初始化分配为 803,360B。原队列的 100,000 个完整任务槽约占 28.8MB，指针槽按实际任务数量分配完整任务。

上游基线运行新增回归时，短模型名继续持有约 16MiB 请求；16MiB 图片与 50 条元数据清理累计分配约 856MB；2MiB 带压缩标记的请求完整解码分配约 10.5～14.7MB。修复后的用例通过，另有回归覆盖原始字节、重复键、转义字段、会话来源与大整数语义。

复测命令：

```sh
cd backend
GOMAXPROCS=2 go test -p 1 ./internal/pkg/requestmodel ./internal/service ./internal/handler
GOMAXPROCS=2 go test -p 1 -tags=unit ./internal/service -run 'Test.*Compaction|TestHasCompaction|Test.*Moderation|Test.*OpenCode|Test.*OAuthInput'
GOMAXPROCS=2 go test -p 1 ./internal/service -run '^$' -bench 'BenchmarkOpenAIOAuthInputMetadata|BenchmarkCompactionTriggerLargeRequest|BenchmarkContentModerationIdleService' -benchtime=1x -benchmem
```
