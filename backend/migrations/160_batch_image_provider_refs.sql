ALTER TABLE batch_image_jobs
    ADD COLUMN IF NOT EXISTS provider_input_ref VARCHAR(1024),
    ADD COLUMN IF NOT EXISTS provider_output_ref VARCHAR(1024);
