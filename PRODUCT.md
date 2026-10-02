# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Two primary audiences, weighted equally:

- **Paying end users** of a Vietnamese relay service. They are mostly developers using Claude Code, Codex, Gemini CLI and similar tools. They come to top up their balance, create and manage API keys, check usage, quota and subscriptions, see which models and channels are available, and look at orders and affiliate earnings. They often do this on a phone.
- **The operator and staff** who run the service day to day in the admin console. That work covers upstream accounts, groups and composite groups, channels and channel monitoring, proxies, users, subscriptions, redeem and promo codes, payments and orders, risk control, ops, audit logs, announcements, backups, plugins and settings. They sometimes triage from a phone too.

## Product Purpose

Sub2API is an AI API gateway. It pools upstream subscription and API-key accounts (Anthropic, OpenAI, Gemini, Grok, DeepSeek and others) and resells them as per-user API keys. It tracks billing down to the token and handles scheduling, concurrency and rate limits. Success means end users can top up and start calling models with little friction and can trust what they are billed. It also means operators can keep upstream accounts healthy and the business profitable without fighting the console.

## Positioning

This is a fork of Wei-Shaw/sub2api (origin: YunaBotDeveloper/sub2api) built for a commercial relay service in the Vietnamese market. What sets it apart from the upstream and from generic relays is local payments: built-in GPM Pay bank transfer (VietQR) and NOWPayments crypto top-up (`docs/PAYMENT.md`), aimed at Vietnamese customers. It competes with other relays on price and stability.

## Operating Context

- End users set up the key and base URL once in their CLI or IDE, then come back now and then to top up, rotate keys or check spend and quota.
- Operators come back constantly and scan dense tables, filters, charts and account health.
- Payment flow: QR or bank-transfer pages (`PaymentView`, `PaymentQRCodeView`, `PaymentResultView`) and popup bridges.
- The public site has a home page (admins can replace it with custom HTML or an iframe, or switch to a compact mode), a model plaza, a public key-usage lookup, legal documents and custom pages.
- A setup wizard runs on first deploy.

## Capabilities and Constraints

- Stack: Go (Gin, Ent) backend; Vue 3 + Vite + TailwindCSS + Pinia + vue-i18n frontend (`frontend/`). Charts use chart.js; onboarding uses driver.js; model/provider icons are inline SVG paths extracted from `@lobehub/icons` (not a dependency).
- Light and dark themes both exist and must both be supported.
- **Operator-branded:** the admin sets the site name, logo, custom home content, doc URL and custom pages at runtime. The UI must not hard-code a brand, and it has to work with whatever name and logo the operator supplies.
- **Localization:** en and zh ship today. A **Vietnamese (vi) locale must be added**, and layouts must handle longer strings and diacritics.
- **Mobile matters** for both end-user and admin flows.
- Keeping the fork easy to merge with upstream was offered as a constraint and **not selected**. The redesign may diverge from upstream markup.
- Undecided: whether Vietnamese becomes the default locale.

## Brand Commitments

None on the product side. Branding belongs to the operator and is configured at runtime. The repo ships a default `frontend/public/logo.svg` and `assets/logo.svg`.

## Evidence on Hand

- Real product surfaces: every view under `frontend/src/views/` (admin, user, auth, public, setup).
- Docs: `docs/PAYMENT.md`, `docs/COMPOSITE_GROUPS.md`, `docs/ASYNC_IMAGE_TASKS.md`, `docs/PLUGIN_DEVELOPMENT.md`, `docs/legal/`.
- No testimonials, customer counts, uptime figures, benchmarks or pricing claims exist. Do not invent any.

## Product Principles

1. **Trust in the numbers.** Balance, usage, cost and quota must be exact, clearly labeled and never ambiguous. Money is involved.
2. **Two speeds, one product.** End users make occasional, low-friction visits. Operators work in dense panels all day. Serve each without watering down the other.
3. **Operator's brand, not ours.** Every surface must look finished under an arbitrary site name, logo and language.
4. **Local first.** Vietnamese payment rails and language are core paths, not add-ons.
5. **Works from a phone.** Top-up, key management, usage checks and urgent admin triage must be complete on mobile.

## Accessibility & Inclusion

No formal standard has been confirmed. Baseline: keyboard-operable admin tables and forms, sufficient contrast in both themes, and diacritic-safe typography for Vietnamese.
