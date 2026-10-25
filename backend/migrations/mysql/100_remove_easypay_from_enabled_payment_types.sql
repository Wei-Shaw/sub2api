-- MySQL 方言版本（翻译自 ../100_remove_easypay_from_enabled_payment_types.sql）
--
-- Removes "easypay" from ENABLED_PAYMENT_TYPES setting.
-- "easypay" is a provider key, not a payment type. Valid payment types
-- are: alipay, wxpay, alipay_direct, wxpay_direct, stripe.
--
-- Idempotent: safe to run multiple times.
--
-- 翻译说明：
--   array_to_string(array_remove(string_to_array(value, ','), 'easypay'), ',')
--   → 等价的三段 REPLACE：先去掉 'easypay,' 与 ',easypay'（串中部/两端），再去掉裸
--   'easypay'（整串恰为该词的情形），TRIM 去除可能残留的边缘逗号。
--   合法支付类型（alipay/wxpay/alipay_direct/wxpay_direct/stripe）均不含 'easypay'
--   子串，因此与「按逗号拆分后移除整词再拼接」的语义一致；该写法与
--   ../101_add_payment_mode.sql 中 PG 原生对 supported_types 的处理方式相同。
-- TODO-REVIEW: 若历史数据中存在包含 'easypay' 作为子串的其他 token（如 'easypay2'），
--   PG 版 array_remove 只移除整词、会保留该 token，而 REPLACE 写法会误伤其子串。
--   当前代码库的合法支付类型不存在此情形，确认无此数据后可移除本标注。

UPDATE settings
   SET value = TRIM(BOTH ',' FROM REPLACE(REPLACE(REPLACE(
           value, 'easypay,', ''), ',easypay', ''), 'easypay', ''))
 WHERE `key` = 'ENABLED_PAYMENT_TYPES'
   AND value LIKE '%easypay%';
