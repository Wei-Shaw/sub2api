# Update Log

## 2026-05-17 - Admin proxy management supports IPv6

### What changed

- Added IPv6-aware proxy host normalization in backend proxy create, update, URL generation, duplicate checks, and import/export key matching.
- Added a persisted `ip_version` field for proxies, including ent schema, SQL migrations, repository mapping, service models, API requests, DTO responses, admin import/export, and frontend API types.
- Updated backend proxy testing so `IP版本 = IPv6` uses IPv6 probe endpoints instead of the old IPv4-only `ip-api.com` / `httpbin.org` chain.
- Added IPv6 probe endpoint parsing through `api6.ipify.org` and `api64.ipify.org`, and reject successful probe responses that do not return an IPv6 exit address when IPv6 is selected.
- Updated admin `IP管理` UI to add an `IPv4 / IPv6` exit IP selection block in create and edit dialogs, aligned with the existing modal style.
- Replaced IPv4-only proxy URL parsing and address formatting in the admin proxy page with shared utilities that correctly support bracketed IPv6 input such as `socks5://user:pass@[2001:db8::1]:1080`.
- Updated proxy display and copy behavior so IPv6 addresses are shown as `[ipv6]:port` and full proxy URLs remain valid.
- Normalized frontend proxy queue grouping keys so the same IPv6 proxy is treated consistently whether the source host is stored as `2001:db8::1` or `[2001:db8::1]`.
- Added regression tests for IPv6 proxy reuse, `ip_version` normalization, IPv6 probe endpoint selection, and bracketed/unbracketed host formats.

### Why it changed

- The original admin proxy workflow only reliably handled IPv4-style `host:port` parsing and formatting.
- The previous IPv6 UI toggle did not fully affect the backend probe path, so SOCKS5 proxies with IPv4 entry hosts but IPv6 exits could still be tested through IPv4-only endpoints.
- SOCKS5 IPv6 proxies could be entered inconsistently, duplicated across imports, copied into invalid URL formats, or probed with the wrong exit IP expectation.
- The goal of this change is to make IPv6 proxies first-class in backend data handling, persistence, connectivity checks, import/export, and the admin UI, not just add a visual toggle.

### Verification

- Backend targeted tests:
  - `go test ./internal/repository -run 'TestProxyProbeServiceSuite/TestProbeProxy_IPv6UsesIPv6ProbeEndpoint|TestProxyProbeServiceSuite/TestProbeProxy_Success_IPAPI|TestProxyProbeServiceSuite/TestProbeProxy_Success_HTTPBinFallback|TestProxyProbeServiceSuite/TestProbeProxy_AllFail'`
  - `go test ./internal/service -run 'TestNormalizeProxyHost|TestNormalizeProxyIPVersion|TestProxyURL|TestProxyURLNormalizesBracketedIPv6Host'`
  - `go test ./internal/handler/admin -run 'TestProxyHandlerCreateNormalizesIPv6Host|TestProxyExportDataRespectsFilters|TestProxyImportDataReusesIPv6ProxyAcrossBracketFormats|TestProxyImportDataReusesIPv6ProxyWhenKeyOnlyUsesBracketedHost|TestImportDataReusesIPv6ProxyKeyAcrossBracketFormats'`
- Frontend checks:
  - `pnpm test:run src/utils/__tests__/proxyAddress.spec.ts`
  - `NODE_OPTIONS=--max-old-space-size=4096 pnpm build`
- Docker image build:
  - Added the same frontend heap setting to the Docker frontend builder so Server A deployment builds do not fail with Vite/Node out-of-memory errors.
- Real SOCKS5 IPv6 exit verification on Server A:
  - `curl -x socks5h://[redacted] 'http://api6.ipify.org?format=json'` returned an IPv6 address.
  - `curl -x socks5h://[redacted] 'http://api64.ipify.org?format=json'` returned the same IPv6 address.
- Built module runtime spot-check:
  - Verified `parseProxyUrl`, `normalizeProxyHost`, `formatProxyAddress`, and `buildProxyUrl` on an IPv6 SOCKS5 example from the built `proxyAddress` asset.

### Environment notes

- Local Windows Go cannot download the required `go1.26.3` toolchain in this environment, so backend generation and targeted backend tests were run on Server A with the matching Go toolchain.
- Local Windows frontend `pnpm typecheck` is still blocked by existing dependency/type issues such as missing `axios` resolution and unrelated implicit `any` diagnostics; the Linux/Node frontend build and targeted proxy address test pass.

### Preview entry

- Frontend dev preview: run `pnpm dev` in `D:\挣钱\token\_release_worktrees\sub2api-desktop-client\frontend`
- Expected local preview URL: `http://localhost:5173`
