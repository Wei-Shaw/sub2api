-- 清理过期成员后返回反向索引的成员列表（按最近使用降序）
-- 成员格式: {userID}/{clientID}
-- KEYS[1] = affinity_rev:{groupID}:{accountID}
-- ARGV[1] = 过期阈值时间戳 (now - ttl)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
return redis.call('ZREVRANGE', KEYS[1], 0, -1)
