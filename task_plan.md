# Task Plan: Backend Billing Logic Documentation

## Goal
Produce an accurate Chinese document that explains the complete backend billing logic, including request billing, subscriptions, user balance deduction, recharge/payment fulfillment, refunds, redeem/admin adjustments, and a sequence diagram.

## Current Phase
Complete

## Phases

### Phase 1: Requirements & Source Discovery
- [x] Identify billing-related schemas, repositories, services, handlers, and existing docs
- [x] Record source-backed findings
- **Status:** complete

### Phase 2: Trace Runtime Request Billing
- [x] Trace auth-time billing block checks
- [x] Trace cost calculation and multiplier logic
- [x] Trace transactional usage apply and cache behavior
- **Status:** complete

### Phase 3: Trace Recharge & Subscription Fulfillment
- [x] Trace payment order creation, webhook, fulfillment, expiry, refund
- [x] Trace redeem code and admin balance/subscription adjustments
- **Status:** complete

### Phase 4: Write Documentation
- [x] Create a Chinese document under docs/
- [x] Include an accurate Mermaid sequence diagram
- [x] Include source references and edge cases
- **Status:** complete

### Phase 5: Verification & Delivery
- [x] Re-check document against source files
- [x] Report modified files and verification result
- **Status:** complete

## Decisions Made
| Decision | Rationale |
|----------|-----------|
| Use Mermaid sequence diagram inside Markdown | Existing docs are Markdown; Mermaid is easy to review and maintain. |
| Focus on backend implementation, not frontend UX | User explicitly asked for backend billing logic. |

## Errors Encountered
| Error | Attempt | Resolution |
|-------|---------|------------|
| planning session catch-up script missing in `.cc-switch` skill path | 1 | Continued with local source discovery and replaced old planning files for this task. |
