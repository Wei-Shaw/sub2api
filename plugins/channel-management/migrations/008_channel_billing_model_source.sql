-- Add billing_model_source to channels (controls whether billing uses requested or upstream model)
ALTER TABLE channels ADD COLUMN IF NOT EXISTS billing_model_source VARCHAR(20) DEFAULT 'requested';
