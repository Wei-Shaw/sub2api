-- 清理过期成员后返回反向索引的 clientID 列表及其 score（最后活跃时间戳）
-- KEYS[1] = client_affinity_rev:{groupID}:{accountID}
-- ARGV[1] = 过期阈值时间戳 (now - ttl)
-- 返回: {clientID1, score1, clientID2, score2, ...}（按最近使用降序）
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
return redis.call('ZREVRANGEBYSCORE', KEYS[1], '+inf', '-inf', 'WITHSCORES')
