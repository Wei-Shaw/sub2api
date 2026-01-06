# Dependency Security

This document describes how dependency and toolchain security is managed in this repo.

## Go Toolchain Policy (Pinned to 1.25.5)

The Go toolchain is pinned to 1.25.5 to address known security issues.

Locations that MUST stay aligned:
- `backend/go.mod`: `go 1.25.5` and `toolchain go1.25.5`
- `Dockerfile`: `GOLANG_IMAGE=golang:1.25.5-alpine`
- Workflows: use `go-version-file: backend/go.mod` and verify `go1.25.5`

Update process:
1. Change `backend/go.mod` (go + toolchain) to the new patch version.
2. Update `Dockerfile` GOLANG_IMAGE to the same patch version.
3. Update workflows if needed and keep the `go version` check in place.
4. Run `govulncheck` and the CI security scan workflow.

## Security Scans

Automated scans run via `.github/workflows/security-scan.yml`:
- `govulncheck` for Go dependencies
- `gosec` for static security issues
- `pnpm audit` for frontend production dependencies

Policy:
- High/Critical findings fail the build unless explicitly exempted.
- Exemptions must include mitigation and an expiry date.

## Audit Exceptions

Exception list location: `.github/audit-exceptions.yml`

Required fields:
- `package`
- `advisory` (GHSA ID or advisory URL from pnpm audit)
- `severity`
- `mitigation`
- `expires_on` (recommended <= 90 days)

Process:
1. Add an exception with mitigation details and an expiry date.
2. Ensure the exception is reviewed before expiry.
3. Remove the exception when the dependency is upgraded or replaced.

## Frontend xlsx Mitigation (Plan A)

Current mitigation:
- Use dynamic import so `xlsx` only loads during export.
- Keep export access restricted and data scope limited.

## Rollback Guidance

If a change causes issues:
- Go: revert `backend/go.mod` and `Dockerfile` to the previous version.
- Frontend: revert the dynamic import change if needed.
- CI: remove exception entries and re-run scans to confirm status.
