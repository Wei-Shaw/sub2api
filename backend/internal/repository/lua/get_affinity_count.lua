-- 清理过期成员后返回反向索引的成员数量
-- KEYS[1] = client_affinity_rev:{groupID}:{accountID}
-- ARGV[1] = 过期阈值时间戳 (now - ttl)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
return redis.call('ZCARD', KEYS[1])
