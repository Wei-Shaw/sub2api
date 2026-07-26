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

### Requirement: Account-menu identity information
The current-user payload, account menu, and profile SHALL display public account ID and whether the identity is a root/main account or an IAM/sub-account for every user. A company root SHALL additionally display company name and owner role. An IAM user SHALL additionally display company name, IAM login principal, immutable IAM user ID, member status, effective policy names, and non-sensitive balance-source information.

#### Scenario: Root account menu
- **WHEN** an approved company owner opens the account menu
- **THEN** it SHALL show the 16-digit account ID, main-account identity, company name, and organization-owner role

#### Scenario: IAM account menu
- **WHEN** an IAM user opens the account menu
- **THEN** it SHALL show the shared 16-digit root account ID, sub-account identity, 18-digit IAM user ID, login principal, company, member status, and policy names
- **AND** root balance amounts SHALL be omitted unless finance-read permission is effective

### Requirement: Frontend enforcement mirrors backend authorization
The frontend SHALL hide or disable unavailable routes and actions based on the current-user organization context and permissions, while every corresponding backend endpoint SHALL independently enforce the same authorization and organization scope.

#### Scenario: Manually navigate to owner route
- **WHEN** an IAM member enters an owner-only route URL directly
- **THEN** the frontend SHALL route to an access-denied view
- **AND** a direct API request SHALL also be denied by the backend
