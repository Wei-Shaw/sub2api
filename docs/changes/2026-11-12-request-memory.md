# 减少长会话请求的复制与持有

- 模型候选同步读取请求字节，返回独立持有的模型名。继续收集 JSON 和 multipart 两种解析路径的候选。
- 压缩标记沿用上游提前返回检查。需要重排时保留其他字段的顺序和转义，只重建一次请求。存在重复字段或特殊输入时沿用完整解码的取值规则。
- OpenCode 协议转换保存经过校验的会话标识，沿用会话来源优先级。

上游已实现 OAuth 元数据一次合成、请求视图短字段独立持有，以及审计队列按需分配。本次以最新 `main` 的实现为基础。

## 压缩标记重排的内存分配

以 `81ce8356d` 为基线。在 Apple M1 Pro 和 Go 1.27.4 上用相同的合成请求各运行 3 次，每次处理 1 个请求。表中为单次调用的累计分配中位数。

| 图片字段大小 | 上游基线 | 本次调整 |
| --- | ---: | ---: |
| 16 MiB | 117,475,888 B | 16,787,224 B |
| 112 MiB | 620,792,368 B | 117,450,520 B |

## 验证

使用合成图片和工具输出构造长会话，分别验证标记移动、已在末尾的快速返回，以及嵌套重复字段。模型候选与 OpenCode 回归分别覆盖大请求释放和会话标识优先级。

```sh
cd backend
GOMAXPROCS=2 go test -p 2 -tags=unit ./internal/pkg/requestmodel ./internal/service ./internal/handler -run 'Test.*(Compaction|OAuthInput|OpenCode|OpenAIRequestView)|^TestFromBody|^TestConflictingField' -count=1
GOMAXPROCS=2 go test -p 1 ./internal/service -run '^$' -bench '^BenchmarkCompactionTriggerLargeRequest' -benchtime=1x -benchmem
```
