-- 清理过期成员后返回亲和账号列表（按最近使用降序）
-- KEYS[1] = client_affinity:{groupID}:{clientID}
-- ARGV[1] = 过期阈值时间戳 (now - ttl)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
return redis.call('ZREVRANGE', KEYS[1], 0, -1)
