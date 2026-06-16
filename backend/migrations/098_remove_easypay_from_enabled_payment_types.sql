-- 098_remove_easypay_from_enabled_payment_types.sql
--
-- Removes "easypay" from ENABLED_PAYMENT_TYPES setting.
-- "easypay" is a provider key, not a payment type. Valid payment types
-- are: alipay, wxpay, alipay_direct, wxpay_direct, stripe.
--
-- Idempotent: safe to run multiple times.

UPDATE settings
   SET value = array_to_string(
       array_remove(
           string_to_array(value, ','),
           'easypay'
       ), ','
   )
 WHERE key = 'ENABLED_PAYMENT_TYPES'
   AND value LIKE '%easypay%';
