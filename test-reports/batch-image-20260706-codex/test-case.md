# Batch Image QA Test Case

Date: 2026-07-06
Branch: `feature/batch-image-foundation`

## Scope

Validate the Sub2API batch image feature before broader external review:

- Gateway API authentication and public response shape
- Available batch image model listing
- Balance hold failure path before upstream submission
- Completed job detail, item listing, and download path
- Billing hold, release, capture, settlement, and recovery unit coverage
- Frontend batch image page type/build/test health
- Agent-copy instruction text for slower polling and resume records

## Test Data

- Local endpoint: `http://127.0.0.1:8080`
- Local completed batch used for read/download smoke: `imgbatch_8944d988d7b92fcba158a9317fe3e699`
- No API key or secret is stored in this report.

## Cases

| ID | Case | Expected |
|---|---|---|
| BI-API-001 | `GET /v1/images/batches` without key | `401`, `API_KEY_REQUIRED` |
| BI-API-002 | `GET /v1/images/batches/models` with key | `200`, returns supported image batch models |
| BI-API-003 | Submit with intentionally insufficient balance | `402`, `BATCH_IMAGE_INSUFFICIENT_BALANCE`, no provider submission |
| BI-API-004 | Fetch completed batch detail | `200`, terminal status and cost fields present |
| BI-API-005 | Fetch completed batch items | `200`, success/failure item summary present |
| BI-API-006 | Download completed successful images | `200 application/zip`, non-empty archive |
| BI-BILL-001 | Reserve balance hold | Available balance decreases, frozen balance increases |
| BI-BILL-002 | Capture hold with actual cost below hold | Remainder released, frozen balance returns to zero |
| BI-BILL-003 | Reject actual cost above hold | Settlement fails before over-capture |
| BI-BILL-004 | Release stale/unsubmitted hold | Stale job fails and frozen funds are released |
| BI-FE-001 | Frontend typecheck/build | Pass |
| BI-FE-002 | Full frontend test suite | Pass |
| BI-FE-003 | Batch image guide copy text | Includes slower polling and local resume-record requirements |

