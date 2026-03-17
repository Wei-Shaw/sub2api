-- 清除单个账号在指定分组的所有亲和记录（正向+反向）
-- KEYS[1] = affinity_rev:{groupID}:{accountID}  (反向索引)
-- ARGV[1] = groupID   (用于构建正向 key)
-- ARGV[2] = accountID (正向索引中要移除的成员)
-- 返回: 清理的成员数量
local rev_key = KEYS[1]
local group_id = ARGV[1]
local account_id = ARGV[2]

-- 获取反向索引中所有成员 ({userID}/{clientID})
local members = redis.call('ZRANGE', rev_key, 0, -1)
if #members == 0 then
    return 0
end

-- 从每个成员的正向索引中移除该账号
for _, member in ipairs(members) do
    local fwd_key = 'affinity:' .. group_id .. ':' .. member
    redis.call('ZREM', fwd_key, account_id)
    -- 如果正向索引为空，删除 key
    if redis.call('ZCARD', fwd_key) == 0 then
        redis.call('DEL', fwd_key)
    end
end

-- 删除反向索引
redis.call('DEL', rev_key)

return #members
