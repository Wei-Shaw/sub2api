UPDATE batch_image_jobs
SET user_deleted_at = COALESCE(user_deleted_at, updated_at, created_at, NOW()),
    updated_at = NOW()
WHERE user_deleted_at IS NULL
  AND provider_job_name IS NULL
  AND status = 'failed'
  AND last_error_code IN (
      'INSUFFICIENT_BALANCE',
      'PROVIDER_SUBMIT_FAILED',
      'BATCH_IMAGE_PROVIDER_SUBMIT_FAILED',
      'BATCH_IMAGE_VERTEX_GCS_BUCKET_MISSING',
      'VERTEX_MANAGED_GCS_BUCKET_MISSING',
      'BATCH_IMAGE_PROVIDER_MISSING_API_KEY',
      'BATCH_IMAGE_PROVIDER_MISSING_SERVICE_ACCOUNT',
      'BATCH_IMAGE_PROVIDER_UNSUPPORTED_ACCOUNT'
  );
