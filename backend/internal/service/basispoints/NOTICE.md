# Source attribution

This package is adapted from hloolx/codex2api, whose README declares MIT License.
Original author of the Basispoints changes: hloolx. Source: https://github.com/hloolx/codex2api

Requested commits and required intervening fixes, in order:

- 9d02d3f5e5d69632ebb9590082a833c0a0916356 — initial Basispoints routing.
- c125e560eefb5fd15c995943eb1e111795b0635f — tool-loop identity and replay.
- 20ff3e860d9a149e2df731e37ba1d9b56ae053fc — envelope formatting and model access errors.
- d39f7e3697aab342e303bf4be0142e39b6a58515 — tool catalog and complete terminal items.
- 4dea83ec53b7668419edd2a9a9dd40fb55fdaacd — HTTPS image references.

Only the protocol package is imported; Sub2API supplies its own account setting,
OAuth credential lifecycle, proxy transport, usage recording and frontend.
The repositories have different layouts; this is a source port, not a Git merge
of the other application's deployment, database or account pool implementation.

Additional review reference: JaxsonWang/cpa-plugin-oai-basispoints at
05b2d97efa1bd117da6bd4d362d6e88f8e483680. Its tool/args envelope examples
were compared with this package. We retain scoped caches, incremental text
streaming and multiple-terminal-tool handling rather than its global call-ID
cache and single-transport extraction. No CPA plugin ABI is imported.

## 2026-09-26 protocol comparison

Behavioral references reviewed for this change:

- JaxsonWang/cpa-plugin-oai-basispoints v0.1.14, commit
  1b9359ae1eda41b0eee556af8ed6edd059d7f8e7: attachment endpoint and multipart
  contract, tool validation, and bounded regeneration.
- zhu961212/sub2api-oai-basispoints 0.5.20, commit
  5b4afc1c01267190277e62eb4c3253ae2ed28015: session catalog inheritance,
  first-tool correction eligibility, usage aggregation, and idle cache expiry.

The second repository does not specify a license for its own code. These
behaviors were implemented independently using this project's existing bridge,
HTTP transport, replay cache, and JSON Schema dependency; no source files or
plugin ABI from that repository were copied. The original attribution above
continues to apply to the existing adapter.

Native attachment interoperability reference: zhu961212/sub2api-oai-basispoints
at 6c611b2562a7a316184b0ec3b27473ab6e767e6e, for the multipart attachments
endpoint and openai_file_id response field. The uploader and metadata cache
are implemented in this package using Sub2API's existing image validation and
account transport; no plugin runtime or deployment configuration is imported.

## 2026-09-27 native tool screenshot correction

The attachment implementation entered the owner fork in PR #102; PR #99
subsequently integrated protocol completion. The image relay is tracked in PR #66.

Behavioral correction reference: zhu961212/sub2api-oai-basispoints commit
586dc42dccbed2d685ad35d8e1a0c96a2aea173c, internal/attachments/README.md
and the associated tool image and detail tests. Its protocol investigation
distinguishes user-message file_id attachments from inline image_url tool
screenshots, with nullish detail defaulting to auto. This correction is
independently implemented using the existing native image validation and
request-scoped bridge; no source files were copied. The cited official
frontend asset could not be fetched from this development environment, and
no authenticated upstream visual acceptance test is claimed.

## 2026-09-26 OAuth encrypted-history recovery

Reviewed JaxsonWang/cpa-plugin-oai-basispoints v0.1.18, commit
11df6f8855847ec1957b0d2f4271a9cd9b13cfe1 (MIT), including its v0.1.17
capability rollback: a native model's multi-agent v2 metadata does not establish
that the BPS relay can handle encrypted agent messages. Sub2API independently
limits these declarations by OAuth account, mapped model and group route,
retaining native/API-key capabilities and its existing plaintext tool bridge.
No source or plugin ABI was copied. The bounded HTTP 400 reasoning recovery
uses Sub2API's existing OAuth recovery approach and is not claimed as a feature
of that reference release. Its v0.1.18 incremental-streaming changes do not
justify replaying an already delivered stream.

## 2026-09-29 message attachment references

Behavioral reference: JaxsonWang/cpa-plugin-oai-basispoints v0.2.4, commit
08e349cf20d12d721fd704c40654253f3db8a3b3, for the BPS message attachment contract: input_image file references
contain only type and file_id. This is implemented independently at Sub2API's
existing translated-history boundary after input validation; HTTPS image URLs
and tool screenshot payloads retain their existing behavior. No source files or
CPA plugin ABI were copied.

## 2026-09-29 in-band failure classification

Behavioral reference: JaxsonWang/cpa-plugin-oai-basispoints v0.2.7, commit
8960a41dacec8ceca2ae2550ce8c9515997cb7a6, for safe error-code classification
and explicit error-status precedence. Sub2API independently applies this
contract to its existing BPS SSE bridge and buffered JSON responses, preserves
the actual upstream HTTP status in diagnostics, and uses its own BPS-only
cooldown without replaying accepted generations. No CPA ABI or WS transport
implementation is imported.
