-- 充值赠送活动改造：从"历史快照"语义切换为"列表 CRUD"语义。
--
-- 1) 新增列：
--      - name        VARCHAR(120) NOT NULL DEFAULT '默认活动' —— 列表 UI 可读名
--      - updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()       —— 自动更新时间戳，参与 version
-- 2) 新增 partial unique index，确保全表至多一行 enabled=TRUE：
--      CREATE UNIQUE INDEX rechargepromoactivity_enabled
--      ON recharge_promo_activities (enabled) WHERE enabled = TRUE
--
-- forward-only；本文件全部走默认事务（无并发索引创建语句，因此不需要 *_notx.sql）。

ALTER TABLE recharge_promo_activities
    ADD COLUMN IF NOT EXISTS name VARCHAR(120) NOT NULL DEFAULT '默认活动';

ALTER TABLE recharge_promo_activities
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- 关闭旧的"每次保存追加一行"留下的多余 enabled=TRUE 行：保留最新一条 enabled=TRUE，
-- 把更早的同状态行降级为 enabled=FALSE。dev 环境多数表为空，无副作用。
UPDATE recharge_promo_activities
   SET enabled = FALSE
 WHERE enabled = TRUE
   AND id NOT IN (
       SELECT id FROM recharge_promo_activities
        WHERE enabled = TRUE
        ORDER BY created_at DESC
        LIMIT 1
   );

CREATE UNIQUE INDEX IF NOT EXISTS rechargepromoactivity_enabled
    ON recharge_promo_activities (enabled) WHERE enabled = TRUE;
