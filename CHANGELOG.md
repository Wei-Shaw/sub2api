# Changelog

All notable changes to this project are documented in this file.

The format is based on Keep a Changelog, and this project aims to follow Semantic Versioning.

## [Unreleased]

### Breaking Changes

- Admin ops error logs: `GET /api/v1/admin/ops/error-logs` now enforces `limit <= 500` (previously `<= 5000`). Requests with `limit > 500` return `400 Bad Request` (`Invalid limit (must be 1-500)`).

### Migration

- Prefer the paginated endpoint `GET /api/v1/admin/ops/errors` using `page` / `page_size`.
- If you must keep using `.../error-logs`, reduce `limit` to `<= 500` and fetch multiple pages by splitting queries (e.g., by time window) instead of requesting a single large result set.

