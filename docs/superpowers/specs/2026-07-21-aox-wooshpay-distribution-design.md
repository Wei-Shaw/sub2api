# Aox WooshPay Distribution Design

Date: 2026-07-21
Status: Proposed for implementation
Tracking issue: https://github.com/iHuHoo/sub2api/issues/4

## 1. Objective

Maintain a small Aox distribution of Sub2API that adds WooshPay-hosted checkout for one-time CNY recharge payments through Alipay and UnionPay, while keeping upgrades from the official Sub2API repository predictable and reviewable.

The first planned release is based on official Sub2API `v0.1.163` and is displayed as:

`v0.1.163-aox.0.0.1 (WooshPay)`

The design deliberately separates the generic WooshPay feature from Aox-specific release and update behavior so the payment provider can be proposed upstream without carrying Aox operational policy into the upstream pull request.

## 2. Confirmed Scope

### Included in the first release

- WooshPay Checkout Session integration.
- Processing currency: CNY only.
- Payment methods: `alipay` and `unionpay`.
- One-time recharge payments.
- Hosted checkout redirect returned by WooshPay.
- Signed webhook processing for `payment_intent.succeeded`.
- Payment order reconciliation and idempotent balance crediting.
- Admin configuration and user-facing payment entry.
- Test-mode verification followed by a controlled live smoke test.
- Aox-managed build/update status and GitHub Actions candidate images.

### Excluded from the first release

- AlipayHK and HKD payments.
- Recurring subscriptions.
- Refund UI or refund API integration.
- WooshPay Direct PaymentIntent integration.
- Automatic production deployment.
- Replacing the existing updater by merely hardcoding a different GitHub repository.

## 3. Repository and Branch Model

Repository roles:

- `Wei-Shaw/sub2api`: official upstream.
- `iHuHoo/sub2api`: Aox fork.

Branches:

- `main`: clean mirror of `Wei-Shaw/sub2api:main`. No Aox-only commits.
- `feature/wooshpay-provider`: generic, upstreamable WooshPay implementation.
- `aox/main`: production source containing the reviewed Aox patch stack.
- `upgrade/vX.Y.Z`: generated candidate branch for a specific official release.

The WooshPay provider is developed and reviewed independently on `feature/wooshpay-provider`. The upstream pull request is created from that branch. Aox-only updater policy, version display, and release automation remain outside the generic upstream pull request.

`aox/main` is updated only through reviewed pull requests. Automation must not force-push or deploy it directly.

## 4. Version Contract

The conceptual format is:

`{official-version}-aox.{aox-revision} (WooshPay)`

The concrete first-release values are:

- Release title/display: `v0.1.163-aox.0.0.1 (WooshPay)`
- Git tag: `v0.1.163-aox.0.0.1`
- Docker tag: `0.1.163-aox.0.0.1`

Rules:

1. The official version segment follows the official Sub2API release.
2. An official upgrade changes the official segment and retains the applicable Aox patch revision unless a new Aox revision is required by the release process.
3. An Aox-only fix increments the Aox revision, for example `aox.0.0.1` to `aox.0.0.2`.
4. `(WooshPay)` is display metadata. Parentheses are excluded from Git and Docker tags for tool compatibility.
5. Release artifacts must record both the source commit and the immutable container digest.

## 5. WooshPay Provider Architecture

Sub2API has a compile-time Go payment provider interface and a fixed provider factory; it does not currently expose a runtime plugin loader. The implementation will therefore be a small built-in provider adapter, isolated in provider-specific files and registered through the existing factory.

The provider responsibilities are:

1. Validate provider configuration.
2. Create a WooshPay Checkout Session.
3. Return the hosted checkout URL to the existing payment flow.
4. Verify webhook signatures against the raw request body.
5. Parse successful payment events.
6. Match the event to an existing pending payment order.
7. Reconcile currency, amount, merchant order identity, and provider identifiers.
8. Invoke the existing idempotent payment completion path.

No WooshPay secret is sent to the browser. All WooshPay API calls and signature verification happen in the backend.

## 6. Checkout Session Flow

The backend creates a pending Sub2API payment order before contacting WooshPay. It then sends a server-side request to:

`POST /v1/checkout/sessions`

The request uses Basic authorization and contains:

- `mode: "payment"`
- `success_url`
- `cancel_url`
- `payment_method_types: ["alipay", "unionpay"]`
- one line item
- `price_data.currency: "CNY"`
- the exact minor-unit amount expected by the Sub2API order
- a non-secret product description
- order metadata or reference fields sufficient to reconcile the callback

WooshPay returns a Checkout Session containing a hosted `url` and a `payment_intent` identifier. The provider stores the relevant identifiers on the payment order and returns the hosted URL to the frontend.

The browser redirect and `success_url` are user experience signals only. They must never credit a balance. Payment success is established only by a verified server-to-server event, with optional provider-side reconciliation when required.

## 7. Webhook Security and Idempotency

The public webhook endpoint accepts POST requests and must retain the exact raw request bytes before JSON decoding.

Signature verification:

1. Read the `Wooshpay-Signature` header.
2. Parse the `t` timestamp and one or more `v1` signatures.
3. Reject missing, malformed, or stale timestamps using a bounded tolerance.
4. Construct `signed_payload` as `timestamp + "." + raw_body`.
5. Compute HMAC-SHA256 with the configured webhook endpoint secret.
6. Compare signatures using a constant-time comparison.
7. Only then parse and process the JSON event.

For `payment_intent.succeeded`, the handler must verify:

- the event type and object status are consistent;
- the order exists and is in a state eligible for completion;
- currency is exactly CNY;
- provider amount equals the stored order amount in minor units;
- merchant/order reference matches the stored order;
- payment intent and other provider identifiers do not conflict with stored values;
- the same event or payment intent cannot credit more than once.

The existing database transaction/idempotent completion mechanism remains the final authority. Duplicate webhook delivery must return a successful acknowledgement after confirming the order was already completed, without applying balance twice.

Invalid signatures, unknown orders, conflicting identifiers, and amount/currency mismatches never credit an account and are logged without secrets or full sensitive payloads.

## 8. Provider Configuration and UI

The admin UI adds a real `WooshPay` provider type instead of disguising it as Alipay, Stripe, or another provider.

Configuration fields are limited to values the backend needs, such as:

- enabled/display name;
- live/test API base selection where supported;
- WooshPay API credential fields;
- webhook secret;
- success and cancel URL configuration or derivation;
- allowed payment methods fixed to Alipay and UnionPay for the first release.

Secret fields are write-only or masked after storage. Provider configuration responses must not expose plaintext secrets.

The user payment UI presents one WooshPay checkout entry labeled for CNY Alipay/UnionPay. WooshPay Checkout chooses the final method from the configured list. AlipayHK is not shown.

Refund capability remains disabled even though the provider documentation lists refund support for the selected methods.

## 9. Managed Update Behavior

The official updater downloads a release binary and atomically replaces the running executable. That behavior is unsafe for an Aox image because an official binary would remove the WooshPay/Aox patches.

For an Aox managed build:

- backend update and rollback endpoints reject the operation with a clear managed-build response;
- the frontend hides or disables the official update action;
- the system page displays the upstream base version, Aox revision, and that updates are managed by the Aox image pipeline;
- Docker deployments update by pulling a reviewed Aox image, not by replacing a binary inside a running container.

Backend enforcement is mandatory. UI-only hiding is insufficient.

The first implementation must not simply set `githubRepo = "iHuHoo/sub2api"` in the existing updater. A future upstream-friendly change may generalize release repositories and channels, but it is not needed to secure the first managed build.

## 10. Upstream Tracking and Release Pipeline

A scheduled and manually dispatchable GitHub Actions workflow checks official releases.

When a new official release is detected, the workflow:

1. Creates or updates `upgrade/vX.Y.Z` from the official tag.
2. Reapplies the maintained Aox patch stack.
3. Stops visibly on conflicts while preserving the candidate branch.
4. Runs backend unit/integration tests.
5. Runs frontend lint, typecheck, and critical tests.
6. Builds the container image.
7. Publishes a candidate image to GHCR with an immutable digest.
8. Opens or updates one pull request into `aox/main` containing test results, conflicts, changelog context, source commits, and image digest.

The workflow never deploys production. Promotion requires human review and approval.

## 11. Testing Strategy

### Provider unit tests

- Checkout request method, path, authentication, amount, currency, methods, and URLs.
- Successful response parsing and provider identifier persistence.
- Provider timeout, non-2xx, malformed JSON, and missing URL behavior.
- Configuration validation without logging secrets.

### Webhook tests

- Valid signature and successful event.
- Invalid or missing signature.
- Timestamp outside tolerance.
- Raw-body modification after signing.
- Duplicate event delivery.
- Duplicate payment intent under a different event ID.
- Unknown order.
- Amount mismatch.
- Currency mismatch.
- Conflicting provider transaction ID.
- Concurrent duplicate delivery credits exactly once.

### Frontend tests

- WooshPay admin fields render and serialize correctly.
- Secret fields remain masked.
- CNY Alipay/UnionPay payment entry is visible only when enabled.
- Managed Aox build does not expose a working official update action.
- Official non-Aox build retains existing update behavior.

### Release tests

- Upstream candidate workflow is idempotent.
- Conflict path opens/updates a visible failed candidate without modifying production.
- Image labels, Git tag, release title, and Docker tag follow the version contract.
- Built image reports its source commit and Aox version.

## 12. Rollout and Operations

1. Implement with test credentials and mocked provider tests.
2. Configure a WooshPay test webhook and execute test-mode Alipay/UnionPay flows.
3. Verify duplicate delivery and failure recovery.
4. Build an Aox candidate image from the reviewed commit.
5. Deploy to a staging instance with production-like persistence.
6. Perform one low-value controlled live payment per enabled method if WooshPay supports the required live validation.
7. Promote the immutable image digest to production manually.
8. Monitor payment failures, signature rejections, unmatched orders, duplicate events, and completion latency.

Rollback means redeploying the previous known-good Aox image digest. It does not invoke the in-app binary rollback endpoint.

## 13. Delivery Breakdown

- Issue #1: WooshPay provider, UI, callback, and tests on `feature/wooshpay-provider`.
- Issue #3: managed update guard and version/status behavior on `aox/main`.
- Issue #2: upstream release tracking, candidate PR, testing, and GHCR image workflow.
- Issue #4: overall release gates and coordination.

Implementation begins only after this design is reviewed. The detailed executable implementation plan will map each acceptance criterion to files, tests, and verification commands.
