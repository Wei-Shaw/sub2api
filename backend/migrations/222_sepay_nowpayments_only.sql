-- Replace the retired payment gateways (EasyPay, Alipay, WeChat Pay, Stripe,
-- Airwallex) with SePay and NOWPayments, and remove the refund subsystem.
--
-- Retired provider instances are disabled rather than deleted: historical
-- orders reference them by id, and keeping the rows lets an operator still see
-- which merchant an old order was payable to. They can never be re-enabled —
-- the admin UI no longer offers those provider keys and the provider factory
-- rejects them.
UPDATE payment_provider_instances
SET enabled = FALSE,
    updated_at = NOW()
WHERE provider_key NOT IN ('sepay', 'nowpayments')
  AND enabled = TRUE;

-- Refunds are gone: neither SePay (a plain bank transfer) nor NOWPayments
-- (crypto) exposes an automated refund API, so there is nothing left for these
-- columns to record. Dropping them discards historical refund records.
ALTER TABLE payment_orders
    DROP COLUMN IF EXISTS refund_amount,
    DROP COLUMN IF EXISTS refund_reason,
    DROP COLUMN IF EXISTS refund_at,
    DROP COLUMN IF EXISTS force_refund,
    DROP COLUMN IF EXISTS refund_requested_at,
    DROP COLUMN IF EXISTS refund_request_reason,
    DROP COLUMN IF EXISTS refund_requested_by;

ALTER TABLE payment_provider_instances
    DROP COLUMN IF EXISTS refund_enabled,
    DROP COLUMN IF EXISTS allow_user_refund;

-- Orders parked in a refund state have no live status left to sit in. They are
-- money that reached us and was later returned out of band, which is closest to
-- CANCELLED — not COMPLETED, because whatever they bought was undone.
UPDATE payment_orders
SET status = 'CANCELLED',
    updated_at = NOW()
WHERE status IN (
    'REFUND_REQUESTED', 'REFUNDING', 'REFUND_PENDING',
    'PARTIALLY_REFUNDED', 'REFUNDED', 'REFUND_FAILED'
);

-- Settings that only existed for the retired gateways.
DELETE FROM settings
WHERE key IN (
    'ALIPAY_FORCE_QRCODE',
    'ALIPAY_MOBILE_PRECREATE_DEEP_LINK',
    'payment_visible_method_alipay_source',
    'payment_visible_method_wxpay_source',
    'payment_visible_method_alipay_enabled',
    'payment_visible_method_wxpay_enabled'
);

-- The subscription conversion rate changes meaning, not just name: it used to
-- turn a USD plan price into CNY, and now turns it into VND. Carrying the old
-- number over would undercharge by roughly four orders of magnitude, so the
-- rate is dropped and the admin must set it again for SePay.
DELETE FROM settings WHERE key = 'SUBSCRIPTION_USD_TO_CNY_RATE';

-- The checkout page must not keep offering methods that no longer resolve to a
-- provider. Leaving this empty makes it fall back to whatever is enabled.
UPDATE settings
SET value = ''
WHERE key = 'ENABLED_PAYMENT_TYPES';
