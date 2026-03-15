-- 清除单个账号在指定分组的所有亲和记录（正向+反向）
-- KEYS[1] = client_affinity_rev:{groupID}:{accountID}  (反向索引)
-- ARGV[1] = groupID   (用于构建正向 key)
-- ARGV[2] = accountID (正向索引中要移除的成员)
-- 返回: 清理的客户端数量
local rev_key = KEYS[1]
local group_id = ARGV[1]
local account_id = ARGV[2]

-- 获取反向索引中所有客户端 ID
local clients = redis.call('ZRANGE', rev_key, 0, -1)
if #clients == 0 then
    return 0
end

-- 从每个客户端的正向索引中移除该账号
for _, client_id in ipairs(clients) do
    local fwd_key = 'client_affinity:' .. group_id .. ':' .. client_id
    redis.call('ZREM', fwd_key, account_id)
    -- 如果正向索引为空，删除 key
    if redis.call('ZCARD', fwd_key) == 0 then
        redis.call('DEL', fwd_key)
    end
end

-- 删除反向索引
redis.call('DEL', rev_key)

return #clients
