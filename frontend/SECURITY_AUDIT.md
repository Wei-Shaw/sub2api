# Frontend dependency audit

The frontend security workflow installs `frontend/pnpm-lock.yaml` with
`pnpm install --frozen-lockfile`, audits production dependencies with
`pnpm audit --prod --audit-level=high --json`, and checks the result with
`tools/check_pnpm_audit_exceptions.py` against `.github/audit-exceptions.yml`.
High/critical findings require an exact package/advisory exception with a
matching severity, documented mitigation and unexpired date. Prefer patched
dependencies; do not add exceptions for advisories with available fixes.

## SheetJS migration (2026-10-09)

The two `xlsx` exceptions expired on 2026-10-06. The only application use is
the dynamically imported admin usage export in
`frontend/src/views/admin/UsageView.vue`: it builds a worksheet from arrays
of selected API fields and writes an XLSX file. It does not read uploaded
spreadsheets or call SheetJS parsing APIs. The frontend route requires an
admin, and the backend usage endpoint is under admin authentication.

GHSA-4r6h-8v6p-xvw6 explicitly states that export-only workflows are unaffected
by its file-reading prototype pollution. GHSA-5pgg-2g8v-p4x9 describes ReDoS
without the same export-only exclusion. Lazy loading and admin authorization
reduce exposure but do not establish that all writer paths are unaffected,
especially when exported fields contain user-controlled strings. Therefore
neither exception is renewed.

The npm `xlsx` release remains unpatched. Use the official SheetJS CDN tarball
for version 0.20.2, the minimum version fixing both advisories, with its
integrity recorded in the pnpm lockfile. The package name and export API stay
unchanged. Preserve the existing boundaries: dynamic loading only on export,
admin permissions, filtered data scope, selected columns, pagination and
cancellation; do not introduce spreadsheet uploads or parsing.

Upstream advisory references:

- https://github.com/advisories/GHSA-4r6h-8v6p-xvw6
- https://github.com/advisories/GHSA-5pgg-2g8v-p4x9
- https://cdn.sheetjs.com/advisories/CVE-2024-22363

Targeted validation includes the admin usage export contract tests and a real
SheetJS writer smoke test, plus the HTTP client tests for the axios upgrade.
