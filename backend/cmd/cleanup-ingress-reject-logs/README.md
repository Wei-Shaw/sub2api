# Ingress rejection log cleanup

This maintenance command removes historical admission rejections from
`ops_error_logs` without matching unrelated authentication or upstream errors.
It is a dry run unless `--execute` is supplied, and always requires an explicit
RFC3339 cutoff.

```sh
go run ./cmd/cleanup-ingress-reject-logs --before 2026-07-17T00:00:00Z
go run ./cmd/cleanup-ingress-reject-logs --before 2026-07-17T00:00:00Z --execute
```

Run the execute form only after every application instance has been upgraded so
older instances cannot add new ingress rejection rows below the chosen cutoff.
The classifier intentionally retains invariant failures such as
`USER_NOT_FOUND`, database errors, quota/billing errors, and upstream failures.

After the rollout and cleanup are verified, run
`backend/scripts/finalize-ingress-reject-cleanup.sql` in a maintenance window to
remove the deprecated plaintext-key audit table and attribution columns.
