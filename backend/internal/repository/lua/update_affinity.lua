-- 原子双写正向+反向索引
-- KEYS[1] = client_affinity:{groupID}:{clientID}       (正向: client → accounts)
-- KEYS[2] = client_affinity_rev:{groupID}:{accountID}  (反向: account → clients)
-- ARGV[1] = 当前时间戳 (score)
-- ARGV[2] = TTL 秒数
-- ARGV[3] = accountID (正向索引的成员)
-- ARGV[4] = 过期阈值时间戳 (now - ttl)
-- ARGV[5] = clientID  (反向索引的成员)
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[4])
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[3])
redis.call('EXPIRE', KEYS[1], ARGV[2])
redis.call('ZREMRANGEBYSCORE', KEYS[2], '-inf', ARGV[4])
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[5])
redis.call('EXPIRE', KEYS[2], ARGV[2])
return 1
