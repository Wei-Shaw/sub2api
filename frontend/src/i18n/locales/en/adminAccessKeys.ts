export default {
  "title": "Admin access keys",
  "authorizationHint": "Grant operations within resource scopes. Unselected operations are denied.",
  "sharedAccountWarning": "Shared account changes affect every associated group. Global permissions apply independently of group scopes.",
  "secretWarning": "The full key is shown only once. Copy and save it before closing.",
  "copySecret": "Copy key",
  "dismissSecret": "Saved, close",
  "create": "Create administrator key",
  "refresh": "Refresh",
  "loading": "Loading keys and permissions…",
  "noDescription": "No description",
  "never": "Never",
  "neverUsed": "Never used",
  "summaryLabel": "Authorization summary",
  "editPermissions": "Edit permissions",
  "disable": "Disable",
  "enable": "Enable",
  "rotate": "Rotate",
  "revoke": "Permanently revoke",
  "deleteRecord": "Delete record",
  "editTitle": "Edit key",
  "createTitle": "Create key",
  "name": "Name",
  "description": "Description",
  "expiration": "Expiration (local time; leave empty for no expiration)",
  "allowedIps": "Allowed IPs / CIDRs (one per line; empty allows any IP)",
  "enabled": "Enable key",
  "allGroups": "All groups, including future groups",
  "groups": "Groups (multiple selection)",
  "includeUngrouped": "Include ungrouped accounts",
  "accountTypes": "Account types (account operations only; none means no account access)",
  "platforms": "Platforms (multiple selection; empty allows all)",
  "accountOperations": "Allowed account operations",
  "groupOperations": "Allowed group operations (restricted by the groups and platforms above)",
  "modelAllowlistHint": "The group model allowlist restricts which models the group can use.",
  "removeRule": "Remove rule",
  "addRule": "Add resource rule",
  "globalPermissions": "Global permissions (all disabled by default)",
  "processing": "Processing…",
  "save": "Save key",
  "cancel": "Cancel",
  "other": "Other",
  "allGroupsSummary": "All groups, including future groups",
  "noGroups": "No groups",
  "ungroupedSummary": ", including ungrouped accounts",
  "noAccountTypes": "No account types selected (no account access)",
  "anyPlatform": "Any platform",
  "operationFailed": "Operation failed. Please retry.",
  "nameRequired": "Enter a key name",
  "expirationInvalid": "Enter a valid expiration",
  "saved": "Key saved",
  "statusUpdated": "Key status updated",
  "rotated": "Key rotated",
  "revoked": "Key permanently revoked",
  "removed": "Key removed from the list. Historical operation records are retained.",
  "copied": "Key copied",
  "copyFailed": "Copy failed. Please copy the key manually.",
  "statusRevoked": "Revoked",
  "statusDisabled": "Disabled",
  "statusExpired": "Expired",
  "statusEnabled": "Enabled",
  "summary": "Global permissions: {globals} · Resource rules: {rules}",
  "ruleLine": "Rule {index}: {summary}",
  "expires": "Expires: {value}",
  "lastUsed": "Last used: {value}",
  "ruleLegend": "Account and group resource rule {index}",
  "groupFallback": "Group #{id}",
  "ruleSummary": "{groups}; Account types: {types} (account operations only); Platforms: {platforms}; Operations: {count}",
  "rotateConfirm": "Rotating “{name}” immediately invalidates its current key. Continue?",
  "revokeConfirm": "Permanently revoking “{name}” cannot be undone. Continue?",
  "deleteConfirm": "Delete the key record “{name}”? This removes it from the list only. Historical operation records are retained.",
  "legacy": "Legacy full-access key (migration to scoped keys recommended)",
  "fullAccessLabel": "Full access (legacy administrator key compatibility)",
  "fullAccessWarning": "This key can access all administrative resources, read sensitive data, and change account and financial settings, including future administrative endpoints. Group, platform, account type, and operation rules no longer restrict this key. Endpoints requiring an administrator browser session or step-up verification keep those requirements.",
  "rulesSuspended": "Resource rules are suspended and resume when full access is disabled. Expiration, IP restrictions, rate limits, and auditing remain active.",
  "fullAccessConfirmTitle": "Confirm full access",
  "fullAccessPhrase": "ENABLE FULL ACCESS",
  "fullAccessPhraseHint": "Type “{phrase}” to confirm:",
  "unknownPermission": "Unknown permission ({id})",
  "permissions": {
    "accounts_didentity_dread": {
      "label": "Read account email, plan, and creation time (no tokens)",
      "category": "Accounts"
    },
    "accounts_dusage_dread": {
      "label": "Read and refresh authorized account quota and snapshots",
      "category": "Accounts"
    },
    "accounts_dstats_dread": {
      "label": "Read account usage and cost statistics",
      "category": "Accounts"
    },
    "openai_dquota_dread": {
      "label": "Read OpenAI upstream quota and remaining resets",
      "category": "OpenAI"
    },
    "openai_dquota_dreset": {
      "label": "Use an OpenAI reset to restore quota",
      "category": "OpenAI"
    },
    "openai_dreferrals_dread": {
      "label": "Read OpenAI referral eligibility and remaining invitations",
      "category": "OpenAI"
    },
    "openai_dreferrals_dinvite": {
      "label": "Send OpenAI invitation emails",
      "category": "OpenAI"
    },
    "accounts_doptions_dwrite": {
      "label": "Write OpenAI OAuth identity and connection options",
      "category": "Accounts"
    },
    "accounts_drate_dwrite": {
      "label": "Set account billing multiplier",
      "category": "Accounts"
    },
    "groups_dlist_dread": {
      "label": "Read selected group details (name, platform, and status)",
      "category": "Groups"
    },
    "groups_dcapacity_dread": {
      "label": "Read selected group capacity (entire group, regardless of account type)",
      "category": "Groups"
    },
    "openai_dtokens_drefresh": {
      "label": "Refresh caller-provided OpenAI tokens (returns tokens; selected proxy allowed)",
      "category": "OAuth"
    },
    "accounts_dread": {
      "label": "Read accounts (redacted)",
      "category": "Accounts"
    },
    "accounts_dcreate": {
      "label": "Create accounts",
      "category": "Accounts"
    },
    "accounts_dupdate": {
      "label": "Edit account names and notes",
      "category": "Accounts"
    },
    "accounts_dcredentials": {
      "label": "Write credentials (no read access)",
      "category": "Accounts"
    },
    "accounts_dmodels_dwrite": {
      "label": "Edit account model allowlist IDs and mappings",
      "category": "Accounts"
    },
    "accounts_dnetwork": {
      "label": "Change endpoints, proxies, and connection options",
      "category": "Accounts"
    },
    "accounts_dstatus": {
      "label": "Enable, disable, and toggle account scheduling",
      "category": "Accounts"
    },
    "accounts_dschedule": {
      "label": "Change concurrency, priority, and expiration",
      "category": "Accounts"
    },
    "accounts_drefresh": {
      "label": "Refresh OAuth credentials",
      "category": "Accounts"
    },
    "accounts_drecover": {
      "label": "Clear account errors and rate-limit state",
      "category": "Accounts"
    },
    "accounts_ddelete": {
      "label": "Delete accounts (affects shared groups)",
      "category": "Accounts"
    },
    "accounts_dgroups": {
      "label": "Change authorized group associations",
      "category": "Accounts"
    },
    "groups_dmodels_dread": {
      "label": "Read selected group model allowlists and candidate IDs",
      "category": "Group models"
    },
    "groups_dmodels_dwrite": {
      "label": "Edit selected group model allowlists (affects the entire group)",
      "category": "Group models"
    },
    "proxies_dread": {
      "label": "Read proxy IP candidates (paginated, without credentials)",
      "category": "Proxies"
    },
    "models_ddefaults_dread": {
      "label": "Read default model allowlist IDs and mappings",
      "category": "Models"
    },
    "dashboard_dread": {
      "label": "Read site-wide business reports",
      "category": "Reports"
    },
    "users_dread": {
      "label": "Read all user profiles (including contact details)",
      "category": "Users"
    },
    "users_dbalance_dread": {
      "label": "Read all user balances and transaction history",
      "category": "Finance"
    },
    "users_dbalance_dwrite": {
      "label": "Add or subtract any user balance",
      "category": "Finance"
    },
    "users_dquotas_dread": {
      "label": "Read all user platform quotas",
      "category": "Users"
    },
    "users_dquotas_dwrite": {
      "label": "Change and reset all user platform quotas",
      "category": "Users"
    },
    "groups_dread": {
      "label": "Read all group configurations",
      "category": "Groups"
    },
    "usage_dread": {
      "label": "Read site-wide usage details and user usage",
      "category": "Reports"
    },
    "payment_dread": {
      "label": "Read all payment orders and reports",
      "category": "Finance"
    },
    "payment_dcancel": {
      "label": "Cancel payment orders",
      "category": "Finance"
    },
    "payment_dfulfill": {
      "label": "Retry payment order fulfillment",
      "category": "Finance"
    },
    "payment_drefund": {
      "label": "Issue refunds and confirm refund results",
      "category": "Finance"
    },
    "payment_dplans_dread": {
      "label": "Read all payment plans",
      "category": "Finance"
    },
    "payment_dplans_dwrite": {
      "label": "Create, edit, and delete payment plans",
      "category": "Finance"
    },
    "subscriptions_dread": {
      "label": "Read all subscription records",
      "category": "Subscriptions"
    },
    "subscriptions_dassign": {
      "label": "Assign subscriptions to any user",
      "category": "Subscriptions"
    },
    "subscriptions_dextend": {
      "label": "Extend subscription expiration",
      "category": "Subscriptions"
    },
    "subscriptions_drevoke": {
      "label": "Revoke subscriptions",
      "category": "Subscriptions"
    },
    "subscriptions_drestore": {
      "label": "Restore subscriptions",
      "category": "Subscriptions"
    },
    "subscriptions_dreset": {
      "label": "Reset subscription quota",
      "category": "Subscriptions"
    },
    "announcements_dread": {
      "label": "Read all announcements and read receipts",
      "category": "Content"
    },
    "announcements_dwrite": {
      "label": "Create, publish, and edit announcements",
      "category": "Content"
    },
    "announcements_ddelete": {
      "label": "Delete announcements",
      "category": "Content"
    },
    "promo_dread": {
      "label": "Read promotion codes and usage records",
      "category": "Finance"
    },
    "promo_dwrite": {
      "label": "Create and edit promotion code benefits",
      "category": "Finance"
    },
    "promo_ddelete": {
      "label": "Delete promotion codes",
      "category": "Finance"
    },
    "ops_dread": {
      "label": "Read site-wide operational metrics",
      "category": "Monitoring"
    },
    "ops_derrors_dread": {
      "label": "Read site-wide request error details (sensitive)",
      "category": "Monitoring"
    },
    "ops_derrors_dresolve": {
      "label": "Mark request error resolution status",
      "category": "Monitoring"
    },
    "ops_dalerts_dread": {
      "label": "Read site-wide alert rules and events",
      "category": "Monitoring"
    },
    "ops_dalerts_dwrite": {
      "label": "Change alert rules, status, and silences",
      "category": "Monitoring"
    },
    "system_dread": {
      "label": "Read current and available system versions",
      "category": "System"
    }
  },
  "listSeparator": ", "
}
