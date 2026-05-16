# Update Log

## 2026-05-17 - Admin proxy management supports IPv6

### What changed

- Added IPv6-aware proxy host normalization in backend proxy create, update, URL generation, duplicate checks, and import/export key matching.
- Updated admin `IP管理` UI to add an `IPv4 / IPv6` selection block in create and edit dialogs, aligned with the existing modal style.
- Replaced IPv4-only proxy URL parsing and address formatting in the admin proxy page with shared utilities that correctly support bracketed IPv6 input such as `socks5://user:pass@[2001:db8::1]:1080`.
- Updated proxy display and copy behavior so IPv6 addresses are shown as `[ipv6]:port` and full proxy URLs remain valid.
- Normalized frontend proxy queue grouping keys so the same IPv6 proxy is treated consistently whether the source host is stored as `2001:db8::1` or `[2001:db8::1]`.
- Added regression tests for IPv6 proxy reuse across import paths and bracketed/unbracketed host formats.

### Why it changed

- The original admin proxy workflow only reliably handled IPv4-style `host:port` parsing and formatting.
- SOCKS5 IPv6 proxies could be entered inconsistently, duplicated across imports, or copied into invalid URL formats.
- The goal of this change is to make IPv6 proxies first-class in both backend data handling and the admin UI, not just add a visual toggle.

### Verification

- Backend targeted tests:
  - `go test ./internal/service -run 'TestNormalizeProxyHost|TestProxyURL|TestProxyURLNormalizesBracketedIPv6Host'`
  - `go test ./internal/handler/admin -run 'TestProxyHandlerCreateNormalizesIPv6Host|TestProxyImportDataReusesIPv6ProxyAcrossBracketFormats|TestProxyImportDataReusesIPv6ProxyWhenKeyOnlyUsesBracketedHost|TestImportDataReusesIPv6ProxyKeyAcrossBracketFormats'`
- Frontend checks:
  - `pnpm typecheck`
  - `pnpm build`
- Built module runtime spot-check:
  - Verified `parseProxyUrl`, `normalizeProxyHost`, `formatProxyAddress`, and `buildProxyUrl` on an IPv6 SOCKS5 example from the built `proxyAddress` asset.

### Environment notes

- Full `go test ./internal/service ./internal/handler/admin` is currently blocked by an existing machine-level IPv6 loopback issue in tests that start `httptest` on `tcp6 [::1]`.
- `pnpm test:run src/utils/__tests__/proxyAddress.spec.ts src/utils/__tests__/usageLoadQueue.spec.ts` is currently blocked on this machine by a local `localhost` resolution/listen failure in Vitest startup, while `typecheck` and production build both pass.

### Preview entry

- Frontend dev preview: run `pnpm dev` in `D:\挣钱\token\_release_worktrees\sub2api-desktop-client\frontend`
- Expected local preview URL: `http://localhost:5173`
