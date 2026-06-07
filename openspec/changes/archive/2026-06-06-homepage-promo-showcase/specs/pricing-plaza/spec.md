## MODIFIED Requirements

### Requirement: Homepage Promotes Plaza

The public `HomeView` SHALL provide visible affordances directing visitors to the pricing plaza and to active recharge campaigns. Concretely:

1. The pre-existing top-bar plaza link SHALL remain visible on every viewport width.
2. The hero section SHALL retain its primary "Get Started" call-to-action.
3. A `HomeShowcaseSection` block SHALL be rendered below the Features Grid (replacing the previously-rendered Supported Providers block) whose content depends on payment state and active recharge campaign:
   - When `appStore.cachedPublicSettings.payment_enabled === false`, the entire section SHALL NOT render.
   - When `payment_enabled === true`, the section SHALL render a subscription-plan preview consisting of **at most 3** plan cards (sourced via `PlanPlazaCards` with `:max-items="3"`) followed by a "View all plans →" link (i18n key `home.plans.view_all`) navigating to `/plaza/plans`. The preview SHALL render even if no recharge campaign is active.
   - When `payment_enabled === true` AND `GET /api/v1/plaza/recharge-promo` returns a non-null `promo` object, a `HomePromoBanner` SHALL render **above** the plan preview. The banner SHALL display the campaign `name`, the tier list (rendered via `home.promo.tier_label`), an optional "活动至 {date}" line when `valid_until` is present, and a primary call-to-action labelled `home.promo.cta_recharge` ("Recharge now"). Activating the CTA SHALL navigate to `/purchase` for authenticated visitors and to `/login?redirect=/purchase` for anonymous visitors (using `useAuthRedirect.gotoOrLogin`).
4. The homepage SHALL NOT render any recharge red-dot, regardless of dismissal state. The red dot is reserved for `PaymentView` and the user sidebar.
5. Failure to load the recharge campaign (network error, non-2xx response) SHALL NOT block plan rendering. The banner is silently skipped; no error toast is surfaced.
6. The banner SHALL render the `name` field via standard text interpolation (no `v-html`); HTML/script content embedded in `name` SHALL be rendered as literal text.
7. The `HomeView` SHALL no longer render a Supported Providers block, a `PricingTeaser` block, or a "View model pricing" hero secondary CTA. Their i18n keys (`home.providers.*`, `home.cta_view_pricing`) SHALL be removed if no other view references them.

#### Scenario: Anonymous visitor with active campaign and 5 plans

- **GIVEN** `payment_enabled === true`, the campaign endpoint returns a `promo` with `name = "618 充值返现"` and three tiers, and the plan plaza has 5 for-sale plans
- **WHEN** an anonymous visitor loads `/`
- **THEN** the showcase section renders the promo banner with title "618 充值返现", the three tier rows, and a "Recharge now" button; below the banner, exactly 3 plan cards are rendered followed by a "View all plans →" link to `/plaza/plans`

#### Scenario: Anonymous visitor with no active campaign

- **GIVEN** `payment_enabled === true` and the campaign endpoint returns `{ "promo": null }`
- **WHEN** an anonymous visitor loads `/`
- **THEN** no banner renders; the showcase section still renders up to 3 plan cards and the "View all plans →" link

#### Scenario: Authenticated visitor clicks Recharge now

- **GIVEN** the visitor is authenticated and the promo banner is rendered
- **WHEN** the visitor clicks the "Recharge now" CTA
- **THEN** the router navigates to `/purchase`

#### Scenario: Anonymous visitor clicks Recharge now

- **GIVEN** the visitor is anonymous and the promo banner is rendered
- **WHEN** the visitor clicks the "Recharge now" CTA
- **THEN** the router navigates to `/login` with `redirect` query resolving to `/purchase`

#### Scenario: Payment disabled hides entire section

- **GIVEN** `payment_enabled === false` and a campaign is active server-side
- **WHEN** an anonymous visitor loads `/`
- **THEN** neither the promo banner nor any plan card renders; the showcase section is absent from the DOM

#### Scenario: Recharge promo endpoint fails

- **GIVEN** `GET /api/v1/plaza/recharge-promo` returns HTTP 500
- **WHEN** the homepage finishes loading
- **THEN** no banner renders, no error toast is shown, and the plan preview still renders normally

#### Scenario: View all plans link

- **GIVEN** the showcase section is rendered with 3 plan cards
- **WHEN** the visitor clicks the "View all plans →" link
- **THEN** the router navigates to `/plaza/plans`

#### Scenario: Plans fewer than 3

- **GIVEN** only 2 for-sale plans exist
- **WHEN** the homepage renders
- **THEN** exactly 2 plan cards render and the "View all plans →" link is still visible

#### Scenario: Top-bar plaza link visible on mobile

- **WHEN** an anonymous visitor loads `/` at viewport width < 640px
- **THEN** the top-bar plaza link is visible (not hidden by responsive utility classes)

#### Scenario: Banner does not interpret embedded HTML

- **GIVEN** an active campaign with `name = "<script>alert(1)</script>"`
- **WHEN** the banner renders
- **THEN** the literal string `<script>alert(1)</script>` is shown as text and no script executes

#### Scenario: No recharge red dot on homepage

- **GIVEN** the visitor has never dismissed the active campaign and the campaign banner is rendered
- **WHEN** the homepage renders
- **THEN** no red-dot indicator appears on the banner, the CTA, or any other homepage element
