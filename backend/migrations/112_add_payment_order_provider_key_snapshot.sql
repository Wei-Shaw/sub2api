ALTER TABLE payment_orders ADD COLUMN provider_key VARCHAR(30);

UPDATE payment_orders
SET provider_key = (
    SELECT provider_key
    FROM payment_provider_instances
    WHERE CAST(id AS TEXT) = payment_orders.provider_instance_id
)
WHERE provider_key IS NULL
  AND provider_instance_id IS NOT NULL;
