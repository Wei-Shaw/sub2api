## ADDED Requirements

### Requirement: Organization-scoped usage snapshots
Every new usage record created for a company root or IAM user SHALL snapshot `organization_id` and `payer_user_id` in addition to the consuming user. Organization usage SHALL begin at the organization's approval time and SHALL not retroactively include the root user's earlier personal usage.

#### Scenario: IAM usage is recorded
- **WHEN** an IAM member completes a billable request
- **THEN** the usage record SHALL identify the member as consumer, their organization, and the effective payer

#### Scenario: Root usage before approval
- **WHEN** company usage is queried after approval
- **THEN** usage created by the root before the organization effective time SHALL be excluded

### Requirement: Safe organization usage APIs
Organization usage APIs SHALL use an independent `/organization/*` namespace and SHALL derive `organization_id` from the authenticated owner. The client SHALL NOT select an arbitrary organization or unscoped member ID. The APIs SHALL support pagination, time range, member, API key, model, endpoint, and status filters and SHALL return organization-wide totals and trends.

#### Scenario: Owner lists company usage
- **WHEN** an active owner requests organization usage
- **THEN** results SHALL include only records whose snapshotted organization matches the owner's organization and whose time is not earlier than organization activation

#### Scenario: Client supplies foreign member ID
- **WHEN** an owner filters by a member ID outside their organization
- **THEN** the API SHALL reject the filter or return no rows without exposing foreign member information

### Requirement: Organization usage data boundary
The organization usage response SHALL expose member display/login identity, API key name, requested model, token counts, actual user charge, request time, endpoint, status, and duration. It SHALL NOT expose upstream account identities, internal upstream cost or profit, raw upstream errors, secrets, or system-admin-only metadata.

#### Scenario: Organization usage row
- **WHEN** an owner reads a member usage row
- **THEN** the row SHALL contain the documented company-visible fields
- **AND** SHALL omit upstream account, internal cost, raw error, and credential fields

### Requirement: Organization user spending drill-down
The organization dashboard user spending ranking SHALL allow an owner to select a ranked user and view that user's model-level usage for the same organization-scoped time range. Each model row SHALL show the model, request count, total token count, and actual user charge. The ranked user label SHALL prefer the user's non-empty username and SHALL fall back to the canonical IAM login principal for an IAM identity without a username.

#### Scenario: Owner opens a ranked IAM user's model usage
- **WHEN** an owner selects an IAM user in the organization user spending ranking
- **THEN** the console SHALL expand that user with model-level request, token, and actual-charge totals for the active dashboard time range
- **AND** the backend SHALL constrain the aggregation to the selected member in the owner's organization

#### Scenario: Ranked IAM user has no username
- **WHEN** an IAM user in the spending ranking has an empty username
- **THEN** the ranking SHALL display the canonical `<login_name>@<company_id>.opentk.ai` principal

### Requirement: Organization console navigation
An approved active organization owner SHALL receive organization navigation for members, authorization, balance allocation, finance, and company usage. IAM members SHALL receive only personal navigation plus policy-authorized organization views. Personal users SHALL receive an upgrade-company action. Suspended organizations SHALL not expose mutating organization actions to members.

#### Scenario: Personal account menu
- **WHEN** an eligible personal root opens the account menu
- **THEN** the menu SHALL include an upgrade-company action and current application status when present

#### Scenario: Company owner navigation
- **WHEN** an active organization owner opens the console
- **THEN** organization member, authorization, allocation, finance, and usage entries SHALL be available

#### Scenario: IAM finance reader navigation
- **WHEN** an IAM member has `CompanyFinanceReadOnly`
- **THEN** the finance read-only entry SHALL be available
- **AND** owner-only member and authorization entries SHALL remain unavailable

### Requirement: Runtime company feature settings and documentation link
The system-settings feature panel SHALL expose the company application, IAM, public-ID-finalized, and billing-integration switches plus a positive USD upgrade fee, using their deployment configuration values as defaults when no persisted override exists. Enabling company applications or IAM SHALL require both readiness switches. Fee changes SHALL apply only to subsequently submitted applications. The panel SHALL also accept an optional absolute HTTP(S) company documentation URL.

#### Scenario: Persisted switch overrides deployment default
- **WHEN** an administrator saves a company feature switch in system settings
- **THEN** subsequent company feature checks and public settings SHALL use the saved value without requiring a deployment configuration edit

#### Scenario: Enable a product switch before readiness
- **WHEN** an administrator enables company applications or IAM while either readiness switch is disabled
- **THEN** the settings update SHALL be rejected without persisting an invalid combination

#### Scenario: Change the company upgrade fee
- **WHEN** an administrator saves a positive upgrade fee
- **THEN** eligibility and newly submitted applications SHALL use that fee
- **AND** existing applications SHALL retain their original fee snapshots

#### Scenario: Company documentation is configured
- **WHEN** a user can see the organization-console navigation item and the effective documentation URL is non-empty
- **THEN** a help icon SHALL appear beside that navigation item
- **AND** activating it SHALL open the HTTP(S) documentation URL in a new window with opener isolation

#### Scenario: Company documentation is absent or invalid
- **WHEN** the documentation URL is empty
- **THEN** the organization-console navigation item SHALL render without a help icon
- **AND** a non-HTTP(S) URL SHALL be rejected by the settings API

### Requirement: Account-menu identity information
The current-user payload, account menu, and profile SHALL display the user's immutable public account ID and whether the identity is a root/main account or an IAM/sub-account for every user. A company root SHALL additionally display company name and owner role. An IAM user SHALL additionally display company name, IAM login principal, its immutable account ID, member status, effective policy names, and non-sensitive balance-source information.

#### Scenario: Root account menu
- **WHEN** an approved company owner opens the account menu
- **THEN** it SHALL show the 16-digit account ID, main-account identity, company name, and organization-owner role

#### Scenario: IAM account menu
- **WHEN** an IAM user opens the account menu
- **THEN** it SHALL show the IAM user's own 16-digit account ID, sub-account identity, login principal, company, member status, and policy names
- **AND** root balance amounts SHALL be omitted unless finance-read permission is effective

### Requirement: Frontend enforcement mirrors backend authorization
The frontend SHALL hide or disable unavailable routes and actions based on the current-user organization context and permissions, while every corresponding backend endpoint SHALL independently enforce the same authorization and organization scope.

#### Scenario: Manually navigate to owner route
- **WHEN** an IAM member enters an owner-only route URL directly
- **THEN** the frontend SHALL route to an access-denied view
- **AND** a direct API request SHALL also be denied by the backend

### Requirement: Owner-managed member spend limits
An active organization owner SHALL configure an optional all-member default spend limit and member-specific overrides for one or more active IAM members. Each rule SHALL require at least one positive daily or monthly USD amount. A member-specific rule SHALL take precedence over the all-member default, and each selected member SHALL be evaluated independently.

#### Scenario: Configure selected members
- **WHEN** an owner selects multiple IAM members and submits daily and monthly limits
- **THEN** each selected member SHALL receive an independent rule with the submitted amounts

#### Scenario: Member override wins
- **WHEN** an all-member rule and a member-specific rule both apply to an IAM member
- **THEN** the member-specific rule SHALL be the effective rule

### Requirement: Company-sponsored usage accounting and enforcement
Daily and monthly limit usage SHALL be the net actual user charge in UTC calendar windows for usage whose snapshotted balance source is `company`, legacy `shared`, or `subscription`. Usage paid from `allocated` or `self` balance SHALL NOT count. Before a new company-sponsored hold or deduction, the backend SHALL reject a charge that would exceed the effective daily or monthly limit and SHALL NOT fall back to another balance source.

#### Scenario: Allocated usage is excluded
- **WHEN** a member consumes only allocated member balance
- **THEN** their daily and monthly company-sponsored usage SHALL remain unchanged

#### Scenario: Shared and subscription usage count
- **WHEN** a member consumes shared company balance or an enterprise subscription
- **THEN** the actual charge SHALL count toward both applicable windows

#### Scenario: Limit would be exceeded
- **WHEN** current counted usage plus a new charge exceeds an effective limit
- **THEN** the charge SHALL be rejected before funds or subscription quota change

### Requirement: Spend-limit email alerts
A spend-limit rule MAY enable email alerts with a threshold from 1 through 100 percent. The affected member SHALL be a recipient when a deliverable member email exists, and the owner MAY configure additional valid email recipients. Alert delivery SHALL use the durable notification outbox and SHALL be deduplicated per organization, member, rule revision, period, and UTC window.

#### Scenario: Threshold is crossed
- **WHEN** counted usage reaches or exceeds an enabled rule's threshold for the first time in a window
- **THEN** one alert SHALL be enqueued for each deduplicated recipient

#### Scenario: Repeated requests above threshold
- **WHEN** later requests remain above the same threshold in the same window
- **THEN** no duplicate logical alert SHALL be enqueued

### Requirement: Member limit usage presentation
The organization console SHALL show each current IAM member's daily and monthly company-sponsored usage. When a limit exists, the value SHALL be rendered as current usage divided by the effective limit; otherwise it SHALL show current usage with an unlimited indication. Only organization owners SHALL mutate rules, while an IAM member MAY view their own usage and effective limits.

#### Scenario: Member has daily and monthly limits
- **WHEN** the console renders that member
- **THEN** daily and monthly fields SHALL show `current / limit`

#### Scenario: No monthly limit
- **WHEN** a member has no effective monthly limit
- **THEN** the monthly field SHALL show current usage and indicate that no limit is configured
