1|# Redeem Usage Ledger + Once-Per-User Campaign Implementation Plan
2|
3|> **For Hermes:** Use test-driven-development and verification-before-completion. Do not implement production code before the matching failing test has been written and observed failing.
4|
5|**Goal:** Generalize Sub2API redeem code redemption so every successful redemption is recorded in a first-class usage ledger, while supporting both legacy single-use codes and campaign/shared codes where each user can redeem only once per scope.
6|
7|**Architecture:** `redeem_codes` becomes the code/reward/policy definition table. A new `redeem_code_usages` table becomes the source of truth for redemption history and per-user/campaign enforcement. Legacy fields such as `redeem_codes.used_by`, `used_at`, and `status=used` are kept only as backward-compatible projections/aggregates, not as the primary usage record.
8|
9|**Tech Stack:** Go 1.26.4, Gin, Ent ORM, PostgreSQL migrations, Vue 3, pnpm, Vitest.
10|
11|---
12|
13|## Current Context
14|
15|Current redeem behavior is single-use-by-row:
16|
17|- Schema: `backend/ent/schema/redeem_code.go`
18|  - `code` unique.
19|  - `type` controls reward semantics: `balance`, `concurrency`, `subscription`, `invitation`.
20|  - `status`, `used_by`, `used_at` encode one total redemption.
21|  - `group_id` and `validity_days` carry subscription reward metadata.
22|- Service: `backend/internal/service/redeem_service.go`
23|  - `Redeem()` gets code by code string, checks status/expiry.
24|  - It starts a transaction.
25|  - It calls `redeemRepo.Use(txCtx, redeemCode.ID, userID)` before granting reward.
26|  - `redeemRepo.Use()` currently updates `redeem_codes` with `WHERE status = 'unused'` and sets `status='used', used_by, used_at`.
27|- History: `backend/internal/repository/redeem_code_repo.go`
28|  - `ListByUser()` and `ListByUserPaginated()` query `redeem_codes.used_by = userID`.
29|- Admin generate: `backend/internal/service/admin_service.go:3357` creates one row per code.
30|- Admin UI: `frontend/src/views/admin/RedeemView.vue` assumes one row has at most one `used_by`/`used_at`.
31|
32|User-approved target:
33|
34|- Do **not** bolt on usage checks while still treating `redeem_codes.used_by` as primary.
35|- All successful redemption writes must go through the new usage ledger.
36|- Generalize for future extension.
37|- TDD first.
38|
39|---
40|
41|## Product Contract
42|
43|### Reward type remains separate from usage policy
44|
45|Keep `redeem_codes.type` as the reward selector:
46|
47|- `balance`
48|- `subscription`
49|- `concurrency`
50|- `invitation`
51|
52|Do **not** add pseudo reward types such as `balance_once_per_user` or `subscription_once_per_user`.
53|
54|Add usage policy fields instead:
55|
56|- `usage_policy`
57|  - `single_use`: legacy behavior, each code has one total successful redemption.
58|  - `once_per_user`: each user can redeem at most once per `usage_scope`.
59|- `usage_scope`
60|  - Campaign/scope key used for once-per-user enforcement.
61|  - For one shared public code, set `usage_scope` to that campaign key or the code itself.
62|  - For many unique codes in one campaign, all rows share the same `usage_scope`.
63|- `max_total_uses`
64|  - Optional global cap per code definition.
65|  - Default `1` for legacy/single-use rows.
66|  - `NULL` or `0` means unlimited total uses, subject to per-user policy.
67|- `max_uses_per_user`
68|  - Default `1` for `once_per_user`.
69|  - May be generalized later, but v1 will only support `1` for policy `once_per_user`.
70|- `used_count`
71|  - Aggregate counter updated in the same transaction as usage insert and reward grant.
72|
73|### Usage ledger is source of truth
74|
75|New table `redeem_code_usages` records every successful redemption:
76|
77|- `id`
78|- `redeem_code_id`
79|- `usage_scope`
80|- `user_id`
81|- `code_snapshot`
82|- `type_snapshot`
83|- `value_snapshot`
84|- `group_id_snapshot`
85|- `validity_days_snapshot`
86|- `used_at`
87|- `metadata` optional JSONB for future extension
88|
89|Critical unique index:
90|
91|```sql
92|CREATE UNIQUE INDEX ... ON redeem_code_usages(usage_scope, user_id)
93|WHERE usage_scope IS NOT NULL;
94|```
95|
96|This enforces the campaign-level once-per-user rule across concurrent requests and across multiple codes in the same campaign.
97|
98|For true single-use behavior, enforce total cap with row lock + conditional counter/status update in the same transaction. Do not rely on cache-only or pre-check-only logic.
99|
100|---
101|
102|## Data Model Plan
103|
104|### Migration
105|
106|Create a new local/fork migration so upstream migration numbers remain conflict-free during future rebases:
107|
108|- Create: `backend/migrations/local/local_0019_redeem_usage_ledger.sql`
109|
110|Migration contents:
111|
112|1. Add columns to `redeem_codes`:
113|   - `usage_policy TEXT NOT NULL DEFAULT 'single_use'`
114|   - `usage_scope TEXT NULL`
115|   - `max_total_uses INT NULL`
116|   - `max_uses_per_user INT NULL`
117|   - `used_count INT NOT NULL DEFAULT 0`
118|2. Backfill rows:
119|   - `usage_scope = code` for historical rows unless there is a better deterministic scope.
120|   - `max_total_uses = 1` for all existing rows.
121|   - `max_uses_per_user = NULL` for `single_use` rows.
122|   - `used_count = 1` where `status='used' AND used_by IS NOT NULL`, otherwise `0`.
123|3. Add check constraints:
124|   - `usage_policy IN ('single_use', 'once_per_user')`
125|   - `used_count >= 0`
126|   - `max_total_uses IS NULL OR max_total_uses >= 0`
127|   - `max_uses_per_user IS NULL OR max_uses_per_user > 0`
128|   - if `usage_policy='once_per_user'`, `usage_scope IS NOT NULL AND usage_scope <> ''` and `max_uses_per_user = 1` for v1.
129|4. Create `redeem_code_usages` table.
130|5. Backfill historical used rows:
131|   - Insert one usage for every `redeem_codes` row with `status='used' AND used_by IS NOT NULL`.
132|   - Snapshot code/type/value/group/validity fields.
133|   - `used_at = COALESCE(redeem_codes.used_at, redeem_codes.created_at)`.
134|6. Add indexes:
135|   - `idx_redeem_codes_usage_scope`
136|   - `idx_redeem_codes_usage_policy`
137|   - `idx_redeem_code_usages_redeem_code_id`
138|   - `idx_redeem_code_usages_user_id`
139|   - unique `(usage_scope, user_id)` partial index.
140|
141|Important migration constraint:
142|
143|- Do not edit existing applied migration files.
144|- Add new migration only.
145|
146|### Ent schemas
147|
148|Modify:
149|
150|- `backend/ent/schema/redeem_code.go`
151|
152|Add new schema:
153|
154|- `backend/ent/schema/redeem_code_usage.go`
155|
156|Then regenerate Ent:
157|
158|```bash
159|cd backend
160|go generate ./ent
161|```
162|
163|Generated files under `backend/ent/` must be committed.
164|
165|---
166|
167|## Service / Repository Design
168|
169|### New service constants
170|
171|Add constants near existing redeem constants, likely in `backend/internal/domain/constants.go` or service-level constants if this repo aliases domain constants into service:
172|
173|```go
174|const (
175|    RedeemUsagePolicySingleUse   = "single_use"
176|    RedeemUsagePolicyOncePerUser = "once_per_user"
177|)
178|```
179|
180|### Service model changes
181|
182|Modify `backend/internal/service/redeem_code.go`:
183|
184|- Extend `RedeemCode`:
185|  - `UsagePolicy string`
186|  - `UsageScope *string` or string with empty semantics
187|  - `MaxTotalUses *int`
188|  - `MaxUsesPerUser *int`
189|  - `UsedCount int`
190|- Add `NormalizeRedeemUsageScope(scope string) string`.
191|- Add `NormalizeRedeemUsagePolicy(policy string) string`.
192|- Add helper methods:
193|  - `EffectiveUsagePolicy() string`
194|  - `EffectiveUsageScope() string`
195|  - `HasTotalUsageRemaining() bool`
196|
197|Add a `RedeemCodeUsage` service model:
198|
199|```go
200|type RedeemCodeUsage struct {
201|    ID int64
202|    RedeemCodeID int64
203|    UsageScope string
204|    UserID int64
205|    CodeSnapshot string
206|    TypeSnapshot string
207|    ValueSnapshot float64
208|    GroupIDSnapshot *int64
209|    ValidityDaysSnapshot int
210|    UsedAt time.Time
211|    RedeemCode *RedeemCode
212|    User *User
213|}
214|```
215|
216|### Repository interface changes
217|
218|Modify `RedeemCodeRepository` in `backend/internal/service/redeem_service.go` or split a dedicated `RedeemUsageRepository` if that is cleaner.
219|
220|Preferred general approach:
221|
222|- Keep code definition operations in `RedeemCodeRepository`.
223|- Add usage ledger operations to the same repository for now to minimize DI blast radius, but name methods cleanly.
224|
225|Add methods:
226|
227|```go
228|GetByCodeForUpdate(ctx context.Context, code string) (*RedeemCode, error)
229|CreateUsage(ctx context.Context, usage *RedeemCodeUsage) error
230|IncrementUsageCount(ctx context.Context, codeID int64, exhausted bool) error
231|ListUsagesByUser(ctx context.Context, userID int64, limit int) ([]RedeemCodeUsage, error)
232|ListUsagesByUserPaginated(ctx context.Context, userID int64, params pagination.PaginationParams, codeType string) ([]RedeemCodeUsage, *pagination.PaginationResult, error)
233|```
234|
235|Implementation detail:
236|
237|- `GetByCodeForUpdate` must use the transaction client and apply `FOR UPDATE`.
238|- `CreateUsage` must rely on the DB unique index to reject duplicate `(usage_scope, user_id)`. Translate unique violation into a new service error.
239|- Do not implement the once-per-user rule as `SELECT COUNT` followed by insert only. That races.
240|
241|### New service errors
242|
243|Add:
244|
245|```go
246|ErrRedeemScopeAlreadyUsed = infraerrors.Conflict("REDEEM_SCOPE_ALREADY_USED", "you have already redeemed a code from this campaign")
247|ErrRedeemUsageLimitReached = infraerrors.Conflict("REDEEM_USAGE_LIMIT_REACHED", "redeem code usage limit reached")
248|ErrRedeemUsagePolicyInvalid = infraerrors.BadRequest("REDEEM_USAGE_POLICY_INVALID", "invalid redeem usage policy")
249|```
250|
251|Keep existing `ErrRedeemCodeUsed` behavior for legacy/single-use API compatibility where possible.
252|
253|### New redeem transaction flow
254|
255|Replace current `redeemRepo.Use(txCtx, redeemCode.ID, userID)` as the first primary usage mutation.
256|
257|New transaction in `RedeemService.Redeem`:
258|
259|1. Begin transaction.
260|2. Load code by normalized code with row lock.
261|3. Validate status, expiry, policy, subscription group requirements.
262|4. Validate total usage cap.
263|5. Insert `redeem_code_usages` row with snapshots.
264|6. Grant reward with existing branch logic.
265|7. Update `redeem_codes.used_count`.
266|8. Update legacy projection fields:
267|   - For `single_use` or when total cap is exhausted:
268|     - set `status='used'`, `used_by=userID`, `used_at=now` if preserving current API shape.
269|   - For `once_per_user` not exhausted:
270|     - leave `status='unused'`/active-compatible.
271|     - do not set single `used_by` unless product wants first-user projection. Prefer not setting it to avoid misleading admin UI.
272|9. Commit.
273|10. Invalidate caches.
274|11. Fetch the updated code and return.
275|
276|Important rollback rule:
277|
278|- Usage insert, reward grant, counter update, and projection update all happen in the same DB transaction.
279|- If grant fails, no usage row remains.
280|
281|---
282|
283|## API Contract Changes
284|
285|### Admin generate request
286|
287|Modify `backend/internal/handler/admin/redeem_handler.go`:
288|
289|Add request fields to `GenerateRedeemCodesRequest` and `CreateAndRedeemCodeRequest`:
290|
291|```go
292|UsagePolicy string `json:"usage_policy" binding:"omitempty,oneof=single_use once_per_user"`
293|UsageScope string `json:"usage_scope"`
294|MaxTotalUses *int `json:"max_total_uses" binding:"omitempty,min=0"`
295|MaxUsesPerUser *int `json:"max_uses_per_user" binding:"omitempty,min=1"`
296|```
297|
298|Validation:
299|
300|- Default policy: `single_use`.
301|- `single_use`:
302|  - default `max_total_uses=1`.
303|  - `max_uses_per_user` should be nil or ignored.
304|- `once_per_user`:
305|  - require non-empty `usage_scope`, or generate a deterministic campaign scope for a generate request.
306|  - set/require `max_uses_per_user=1` for v1.
307|  - `max_total_uses` optional.
308|- For generated batches with `once_per_user`, all codes in the request should share one `usage_scope` unless the request explicitly asks per-code scope. V1 should share the campaign scope.
309|
310|### DTO output
311|
312|Modify:
313|
314|- `backend/internal/handler/dto/types.go`
315|- `backend/internal/handler/dto/mappers.go`
316|
317|Expose:
318|
319|- `usage_policy`
320|- `usage_scope`
321|- `max_total_uses`
322|- `max_uses_per_user`
323|- `used_count`
324|
325|For user redeem response/history, include enough to render reward and time, not necessarily all admin policy fields.
326|
327|### User history API
328|
329|Update `GET /api/v1/redeem/history` to read from usage ledger rather than `redeem_codes.used_by`.
330|
331|Backward compatibility:
332|
333|- Historical used rows are backfilled into usage ledger.
334|- Response shape can stay compatible by mapping usage snapshots to existing DTO-like shape.
335|
336|---
337|
338|## Frontend Plan
339|
340|Modify:
341|
342|- `frontend/src/types/index.ts`
343|- `frontend/src/api/admin/redeem.ts`
344|- `frontend/src/views/admin/RedeemView.vue`
345|- locale files under `frontend/src/i18n/locales/`
346|
347|Admin UI requirements:
348|
349|1. Existing default generate flow remains single-use.
350|2. Add a usage policy section in generate modal:
351|   - `Mỗi mã chỉ dùng 1 lần` / `Single-use code`.
352|   - `Mỗi user chỉ dùng 1 lần trong campaign` / `Once per user per campaign`.
353|3. If once-per-user selected:
354|   - Show `usage_scope` input with helper text.
355|   - Show optional `max_total_uses` input.
356|   - `max_uses_per_user` fixed at `1` or hidden with explanatory text.
357|4. In code table, display:
358|   - policy badge
359|   - usage scope, truncated with title tooltip
360|   - `used_count / max_total_uses` where applicable
361|5. Do not show a misleading single `used_by` value for reusable campaign codes. Prefer `-` or a usage count link/future drawer.
362|
363|---
364|
365|## TDD Task List
366|
367|### Task 1: Write service-level RED test for legacy single-use ledger write
368|
369|**Objective:** Prove existing single-use redemption must insert a usage ledger row and update aggregate count.
370|
371|**Files:**
372|
373|- Modify test: `backend/internal/service/redeem_service_test.go` or create `backend/internal/service/redeem_usage_ledger_test.go`.
374|- May require test fakes in same package.
375|
376|**Test behavior:**
377|
378|- Given a `balance` code with default/single-use policy.
379|- When user redeems it.
380|- Then:
381|  - user balance increases once.
382|  - usage ledger has exactly one row.
383|  - usage snapshot matches code type/value/code.
384|  - code `used_count=1`.
385|
386|**Run:**
387|
388|```bash
389|cd backend
390|GOTOOLCHAIN=auto go test ./internal/service -run 'TestRedeemWritesUsageLedgerForSingleUse' -count=1
391|```
392|
393|**Expected RED:** fail because no ledger model/repository method exists yet.
394|
395|### Task 2: Add migration and Ent schema for usage ledger
396|
397|**Objective:** Add DB structures needed by Task 1.
398|
399|**Files:**
400|
401|- Create: `backend/migrations/local/local_0019_redeem_usage_ledger.sql`
402|- Modify: `backend/ent/schema/redeem_code.go`
403|- Create: `backend/ent/schema/redeem_code_usage.go`
404|- Generated: `backend/ent/**`
405|
406|**Run generation:**
407|
408|```bash
409|cd backend
410|GOTOOLCHAIN=auto go generate ./ent
411|```
412|
413|**Verification:**
414|
415|```bash
416|cd backend
417|GOTOOLCHAIN=auto go test ./ent ./migrations -count=1
418|```
419|
420|If there is no migration test package target, run the nearest existing migration/schema tests and full compile later.
421|
422|### Task 3: Add service models/errors and repository mapping
423|
424|**Objective:** Compile service model and repository usage operations without yet changing full redeem flow.
425|
426|**Files:**
427|
428|- Modify: `backend/internal/service/redeem_code.go`
429|- Modify: `backend/internal/service/redeem_service.go`
430|- Modify: `backend/internal/repository/redeem_code_repo.go`
431|
432|**Implementation notes:**
433|
434|- Add `RedeemCodeUsage` service model.
435|- Add policy constants and errors.
436|- Map new `redeem_codes` fields in entity conversion.
437|- Implement `CreateUsage`, usage conversion, and count update helpers.
438|
439|**Run:**
440|
441|```bash
442|cd backend
443|GOTOOLCHAIN=auto go test ./internal/repository ./internal/service -run 'TestRedeemWritesUsageLedgerForSingleUse' -count=1
444|```
445|
446|**Expected:** still fail until redeem flow is refactored.
447|
448|### Task 4: Refactor redeem flow to ledger-first transaction
449|
450|**Objective:** Make Task 1 pass by moving primary usage write into `redeem_code_usages`.
451|
452|**Files:**
453|
454|- Modify: `backend/internal/service/redeem_service.go`
455|- Modify: `backend/internal/repository/redeem_code_repo.go`
456|
457|**Key implementation constraints:**
458|
459|- Begin tx before primary validation that needs row locking.
460|- Load code `FOR UPDATE` in transaction.
461|- Insert usage before grant reward, but same transaction ensures rollback on grant failure.
462|- For `single_use`, update legacy projection and cap status as before.
463|
464|**Run:**
465|
466|```bash
467|cd backend
468|GOTOOLCHAIN=auto go test ./internal/service -run 'TestRedeemWritesUsageLedgerForSingleUse' -count=1
469|```
470|
471|**Expected GREEN:** Task 1 test passes.
472|
473|### Task 5: RED/GREEN single-use second-user rejection
474|
475|**Objective:** Preserve legacy behavior exactly for old codes.
476|
477|**Test:** `TestRedeemSingleUseRejectsSecondUserThroughUsageLimit`
478|
479|Assertions:
480|
481|- user A succeeds.
482|- user B gets conflict.
483|- no second usage row.
484|- user B balance/subscription not changed.
485|
486|**Run:**
487|
488|```bash
489|cd backend
490|GOTOOLCHAIN=auto go test ./internal/service -run 'TestRedeemSingleUseRejectsSecondUserThroughUsageLimit' -count=1
491|```
492|
493|### Task 6: RED/GREEN once-per-user same-code behavior
494|
495|**Objective:** Support one shared public code that many users can redeem, once per user.
496|
497|**Test:** `TestRedeemOncePerUserAllowsDifferentUsersButRejectsRepeatUser`
498|
499|Setup:
500|
501|- code type `balance`
502|- `usage_policy=once_per_user`
503|- `usage_scope=campaign-balance-1`
504|- `max_uses_per_user=1`
505|- no total cap
506|
507|Assertions:
508|
509|- user A first redeem succeeds.
510|- user A second redeem fails with `REDEEM_SCOPE_ALREADY_USED`.
511|- user B redeem succeeds.
512|- two usage rows total.
513|- used count is 2.
514|
515|### Task 7: RED/GREEN multiple-codes same campaign scope
516|
517|**Objective:** Block one user from redeeming multiple different codes in the same campaign.
518|
519|**Test:** `TestRedeemOncePerUserScopeBlocksDifferentCodeInSameCampaign`
520|
521|Setup:
522|
523|- code A and code B both `usage_scope=campaign-june`.
524|- user A redeems A.
525|- user A tries B.
526|- user B tries B.
527|
528|Assertions:
529|
530|- user A second attempt fails.
531|- user B succeeds.
532|- ledger rows: A/userA, B/userB only.
533|
534|### Task 8: RED/GREEN subscription once-per-user
535|
536|**Objective:** Ensure generalized policy works for subscription reward without separate type hacks.
537|
538|**Test:** `TestRedeemOncePerUserSubscriptionUsesLedgerSnapshots`
539|
540|Assertions:
541|
542|- user A gets subscription.
543|- usage snapshot records subscription type, group ID, validity days.
544|- user A cannot redeem another code in same scope.
545|- user B can redeem.
546|
547|### Task 9: RED/GREEN rollback safety
548|
549|**Objective:** Prove usage ledger is transactional and not chắp nối.
550|
551|**Test:** `TestRedeemRollsBackUsageWhenRewardGrantFails`
552|
553|Setup:
554|
555|- Force subscription assignment or user balance update to fail after usage insert.
556|
557|Assertions:
558|
559|- redeem returns error.
560|- no usage row persisted.
561|- used count unchanged.
562|- code can be retried if failure was transient.
563|
564|### Task 10: RED/GREEN concurrency/race protection
565|
566|**Objective:** DB unique index must prevent concurrent same-user campaign abuse.
567|
568|**Test:** integration-style test preferred: `TestRedeemOncePerUserConcurrentRequestsCreateOneUsage`
569|
570|Setup:
571|
572|- two codes same campaign scope or one reusable code.
573|- launch concurrent redeem attempts for same user.
574|
575|Assertions:
576|
577|- exactly one success.
578|- exactly one usage row.
579|- reward granted once.
580|
581|Run with integration target if it needs Postgres:
582|
583|```bash
584|cd backend
585|GOTOOLCHAIN=auto go test -tags=integration ./internal/repository -run 'TestRedeemOncePerUserConcurrentRequestsCreateOneUsage' -count=1
586|```
587|
588|### Task 11: Move user history to usage ledger
589|
590|**Objective:** History must represent multiple users using the same code.
591|
592|**Tests:**
593|
594|- `TestRedeemHistoryReadsUsageLedgerForSharedCode`
595|- handler test for `GET /api/v1/redeem/history` if existing handler tests are present.
596|
597|Assertions:
598|
599|- same shared code redeemed by user A and user B.
600|- user A history shows user A usage only.
601|- user B history shows user B usage only.
602|- history uses snapshot fields, not single `redeem_codes.used_by`.
603|
604|**Files:**
605|
606|- Modify: `backend/internal/repository/redeem_code_repo.go`
607|- Modify: `backend/internal/service/redeem_service.go`
608|- Modify DTO mapper if needed.
609|
610|### Task 12: Admin generate API supports policy/scope
611|
612|**Objective:** Admin can create single-use and once-per-user codes without hidden defaults that break campaigns.
613|
614|**Tests:**
615|
616|- `backend/internal/handler/admin/redeem_handler_test.go`
617|- `backend/internal/service/admin_service_*_test.go`
618|
619|Cases:
620|
621|1. Existing payload without policy still creates legacy single-use code.
622|2. `once_per_user` requires/generates non-empty `usage_scope`.
623|3. Batch generation with one `usage_scope` applies same scope to all generated rows.
624|4. Invalid policy rejected.
625|5. Subscription once-per-user still requires `group_id` and valid subscription group.
626|
627|### Task 13: Admin DTO/list exposes aggregate policy fields
628|
629|**Objective:** Admin can see policy/scope/count and does not get misleading `used_by` for reusable codes.
630|
631|**Files:**
632|
633|- Modify: `backend/internal/handler/dto/types.go`
634|- Modify: `backend/internal/handler/dto/mappers.go`
635|- Modify handler tests.
636|
637|Assertions:
638|
639|- admin list response includes `usage_policy`, `usage_scope`, `used_count`, `max_total_uses`, `max_uses_per_user`.
640|- once-per-user active code with usage rows returns `used_count > 0` while status remains active/unused until cap reached.
641|
642|### Task 14: Frontend types/API update
643|
644|**Objective:** Frontend API can send and receive new policy fields.
645|
646|**Files:**
647|
648|- Modify: `frontend/src/types/index.ts`
649|- Modify: `frontend/src/api/admin/redeem.ts`
650|
651|**Tests:**
652|
653|- Add/update Vitest around admin redeem API payload if existing patterns allow.
654|
655|**Run:**
656|
657|```bash
658|pnpm --dir frontend run typecheck
659|pnpm --dir frontend exec vitest run src/views/admin/__tests__/RedeemView.batchUpdate.spec.ts
660|```
661|
662|### Task 15: Admin Redeem UI update
663|
664|**Objective:** Operator can choose once-per-user campaign in the generate modal and see usage counts in the table.
665|
666|**Files:**
667|
668|- Modify: `frontend/src/views/admin/RedeemView.vue`
669|- Modify locale files:
670|  - `frontend/src/i18n/locales/en.ts`
671|  - `frontend/src/i18n/locales/zh.ts`
672|  - `frontend/src/i18n/locales/vi.ts`
673|  - `frontend/src/i18n/locales/ko.ts`
674|
675|UI requirements:
676|
677|- Default remains legacy single-use.
678|- Add usage-policy selector/toggle.
679|- If once-per-user:
680|  - show usage scope input.
681|  - show optional max total uses input.
682|  - show helper copy explaining same user cannot redeem another code in same campaign.
683|- Table displays policy badge, scope, and used count.
684|
685|**Tests:**
686|
687|- Update/add `frontend/src/views/admin/__tests__/RedeemView.*.spec.ts`.
688|- Test payload shape when once-per-user selected.
689|
690|### Task 16: Full local verification
691|
692|Run exact local gates:
693|
694|```bash
695|cd backend
696|GOTOOLCHAIN=auto go test ./internal/service ./internal/repository ./internal/handler/admin -count=1
697|GOTOOLCHAIN=auto go test ./...
698|
699|pnpm --dir frontend run lint:check
700|pnpm --dir frontend run typecheck
701|pnpm --dir frontend exec vitest run src/views/admin/__tests__/RedeemView.batchUpdate.spec.ts
702|pnpm --dir frontend run build
703|
704|make secret-scan
705|```
706|
707|If full integration tests require Docker/Testcontainers and fail due environment, capture exact blocker and run targeted integration tests that are feasible.
708|
709|### Task 17: Runtime smoke before production deploy
710|
711|Because this touches money/subscription entitlements, do a local HTTP smoke before reporting ready:
712|
713|1. Start local backend with temp Postgres/Redis or repo compose overlay.
714|2. Create temp users/admin session.
715|3. Exercise read/write paths against temp DB only:
716|   - create single-use balance code; redeem once; second redeem fails.
717|   - create once-per-user balance shared code; two users redeem; same user repeat fails.
718|   - create once-per-user subscription campaign; verify subscription assignment.
719|   - verify `/api/v1/redeem/history` from usage ledger.
720|4. Browser-smoke admin generate form if UI changed materially.
721|5. Cleanup all temp containers/processes/ports.
722|
723|### Task 18: Diff self-review and deploy boundary
724|
725|Before push/deploy:
726|
727|- Inspect `git diff --stat` and full diff for accidental churn.
728|- Confirm no secrets or production credentials in tests/plans.
729|- Confirm migration number does not collide after `git fetch origin main`.
730|- Confirm old dirty checkout `/root/projects/sub2api` is not mixed into this worktree.
731|- Push only after local verification passes.
732|- After push to `origin/main`, monitor current-SHA:
733|  - `CI`
734|  - `Security Scan`
735|  - `Docker Build & Deploy via SSH`
736|- Post-deploy smoke production read-only endpoints.
737|- Do **not** perform real production redeem mutations unless explicitly approved.
738|
739|---
740|
741|## Risks and Mitigations
742|
743|### Risk: Historical data mismatch
744|
745|If historical used codes are not backfilled into `redeem_code_usages`, user history will appear to lose old rows.
746|
747|Mitigation:
748|
749|- Migration backfills historical `status='used' AND used_by IS NOT NULL` rows.
750|- Add migration/backfill regression if the repo has migration tests.
751|
752|### Risk: Race condition in once-per-user campaign
753|
754|If service does `SELECT` before `INSERT`, two concurrent requests can both pass.
755|
756|Mitigation:
757|
758|- Enforce with DB unique index `(usage_scope, user_id)`.
759|- Treat unique violation as `ErrRedeemScopeAlreadyUsed`.
760|- Add concurrency integration test.
761|
762|### Risk: Reward granted but usage not recorded
763|
764|This is the user's explicit anti-pattern warning.
765|
766|Mitigation:
767|
768|- Usage insert, reward grant, counter update, and projection update all happen in the same transaction.
769|- Add rollback test where reward grant fails after usage insert attempt.
770|
771|### Risk: Admin UI misleading `used_by`
772|
773|Reusable codes cannot be represented by a single `used_by`.
774|
775|Mitigation:
776|
777|- Admin list should show `used_count` and policy/scope.
778|- Keep `used_by` only for legacy/single-use rows or first-use projection if required, but do not present it as complete usage for campaign codes.
779|
780|### Risk: Existing callers expect `type` only
781|
782|Existing admin and payment fulfillment integrations may not know new fields.
783|
784|Mitigation:
785|
786|- Defaults preserve `single_use` behavior.
787|- Existing payloads continue to work.
788|- `CreateAndRedeem` remains backward-compatible.
789|
790|---
791|
792|## Acceptance Criteria
793|
794|- Every successful redeem creates exactly one `redeem_code_usages` row.
795|- Legacy single-use codes still allow exactly one total successful redeem.
796|- Once-per-user scope allows multiple users but only one usage per user per scope.
797|- A user cannot redeem two different codes from the same once-per-user campaign scope.
798|- Balance and subscription reward paths both use the same usage-ledger pipeline.
799|- Failed reward grants roll back usage rows and counters.
800|- User history reads from usage ledger and supports shared-code histories.
801|- Admin can create and inspect once-per-user campaign codes.
802|- Backend full tests, frontend typecheck/build/tests, secret scan pass before push.
803|- Production deploy is monitored by current SHA and only read-only smoke is performed unless production mutation is separately approved.
804|