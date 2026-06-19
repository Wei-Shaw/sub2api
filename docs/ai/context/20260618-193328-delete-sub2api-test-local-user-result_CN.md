# 删除 `sub2api-test-local@example.com` 测试用户结果

## 背景

用户根据截图要求删除套餐列表中的测试用户：

```text
sub2api-test-local@example.com
```

执行计划见：

```text
docs/ai/context/20260618-193328-delete-sub2api-test-local-user-plan_CN.md
```

## 删除前状态

- 用户 ID：`2`
- 邮箱：`sub2api-test-local@example.com`
- 角色：`user`
- API Key：1 个，掩码 `sk-398143e...e649c4`
- 订阅：1 条，分组 `codex-pool-19-usd`

## 备份

删除前已备份 PostgreSQL：

```text
/Users/wujianxiang/CodeSpace/sub2api/.tmp-sub2api-before-delete-test-local-20260618-193328.dump
```

备份大小约 `834K`。

## 执行结果

已在单个数据库事务中完成：

1. 将目标用户 API Key 写入 `deleted_api_key_audits`。
2. 软删除目标用户 API Key，并用 tombstone 覆盖原 `api_keys.key`。
3. 删除目标用户关联 auth identity 数据。
4. 软删除目标用户。
5. 清理 Redis L2 认证缓存并发布 L1 失效通知。

## 验证结果

数据库验证：

- `user_deleted=true,user_id=2,role=user`
- `active_keys=0`
- `deleted_keys=1`
- `audit_rows=1`
- Redis 认证缓存：`EXISTS apikey:auth:<hash> = 0`

原 Key 反向验证：

- `http://127.0.0.1:18080/v1/models`：HTTP 401，`INVALID_API_KEY`
- `https://aaccx.pw/v1/models`：HTTP 401，`INVALID_API_KEY`
- `https://api.aaccx.pw/v1/models`：HTTP 401，`INVALID_API_KEY`

## 注意

- 完整 API Key 没有写入文档。
- 本次只删除 `sub2api-test-local@example.com`，未修改其它用户。
