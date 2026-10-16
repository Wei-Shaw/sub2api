-- 注册审核：记录用户注册时填写的申请理由，供管理员后台审核参考。
ALTER TABLE users ADD COLUMN IF NOT EXISTS registration_reason TEXT NOT NULL DEFAULT '';
