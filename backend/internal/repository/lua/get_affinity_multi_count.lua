-- 从反向索引解析多维度计数（用户/客户端/每用户客户端）
-- KEYS[1] = affinity_rev:{groupID}:{accountID}
-- ARGV[1] = 过期阈值时间戳 (now - ttl)
-- ARGV[2] = 目标 userID（传 "" 则不计算 perUserClients）
-- 返回: {totalMembers, uniqueUsers, uniqueClients, perUserClients}
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])

local members = redis.call('ZRANGE', KEYS[1], 0, -1)
local total = #members
if total == 0 then
    return {0, 0, 0, 0}
end

local target_user = ARGV[2]
local users = {}
local clients = {}
local user_count = 0
local client_count = 0
local per_user_count = 0

for _, member in ipairs(members) do
    local sep = string.find(member, '/', 1, true)
    if sep then
        local uid = string.sub(member, 1, sep - 1)
        local cid = string.sub(member, sep + 1)
        if not users[uid] then
            users[uid] = true
            user_count = user_count + 1
        end
        if not clients[cid] then
            clients[cid] = true
            client_count = client_count + 1
        end
        if target_user ~= '' and uid == target_user then
            per_user_count = per_user_count + 1
        end
    end
end

return {total, user_count, client_count, per_user_count}
