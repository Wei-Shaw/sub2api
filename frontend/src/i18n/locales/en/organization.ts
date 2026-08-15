export default {
  organization: {
    console: 'Company management', accountId: 'Account ID', accountType: { label: 'Account type', personal: 'Personal account', company: 'Company account' }, accountIdentity: { label: 'Account identity', root: 'Main account', iam: 'Sub-account' }, iamUserId: 'IAM member ID', principal: 'Principal', companyName: 'Company name', companyId: 'Company ID', companySize: 'Company size', role: 'Role', policies: 'Policies', reviewReason: 'Review reason',
    roleValue: { owner: 'Organization owner', member: 'IAM member' },
    status: { pending: 'Pending', approved: 'Approved', rejected: 'Rejected', withdrawn: 'Withdrawn', active: 'Active', disabled: 'Disabled', archived: 'Archived', suspended: 'Suspended' },
    tabs: { members: 'Members', allocation: 'Allocation', finance: 'Finance', limits: 'Member limits', dashboard: 'Dashboard', subscriptions: 'Subscriptions', usage: 'Usage', audit: 'Audit log', settings: 'Feature settings' },
    audit: {
      title: 'Audit log',
      description: 'Records of company recharge, authorization, allocation and spend-limit configuration events.',
      categoryLabel: 'Category',
      allCategories: 'All categories',
      categories: { recharge: 'Recharge', authorize: 'Authorize', allocate: 'Allocate', spend_limit: 'Spend limit', other: 'Other' },
      time: 'Time',
      actor: 'Actor',
      subject: 'Target',
      action: 'Action',
      result: 'Result',
      detail: 'Details',
      resultValues: { success: 'Success', denied: 'Denied', failed: 'Failed' },
      actions: {
        organization_balance_company_deposit: 'Deposit to company balance',
        organization_balance_company_withdraw: 'Withdraw from company balance',
        iam_policy_change: 'Policy change',
        organization_balance_allocate: 'Allocate balance',
        organization_balance_reclaim: 'Reclaim balance',
        spend_limit_upsert: 'Update spend limit',
        spend_limit_delete: 'Delete spend limit',
        organization_subscription_admin_assign: 'Assign subscription',
        organization_subscription_admin_revoke: 'Revoke subscription'
      },
      empty: 'No audit records yet.',
      total: '{total} records',
      previous: 'Previous',
      next: 'Next'
    },
    authorization: { title: 'Authorize {name}', subtitle: 'Select the policies to grant this user', empty: 'No policies available' },
    policyMeta: {
      CompanyFinanceReadOnly: { name: 'Company finance read only', description: 'View the root account available, frozen, and total balance, plus the organization dashboard and usage records.' },
      CompanySharedBalanceUse: { name: 'Company shared balance use', description: 'Use the company balance for consumption without viewing its amount.' },
      CompanyFinanceManage: { name: 'Company finance manage', description: 'View root balance, organization dashboard and usage records, and allocate or reclaim member balance, configure member spend limits, purchase or cancel company subscription plans.' },
      IAMUserManage: { name: 'IAM user manage', description: 'Create IAM members and reset their login password (does not include disabling or archiving members).' }
    },
    login: { personal: 'Personal account', iam: 'IAM login', title: 'IAM member login', subtitle: 'Sign in with your full login account and password', loginName: 'Login name', principal: 'Login account', genericError: 'The login account or password is incorrect' },
    upgrade: { title: 'Upgrade to company account', backToProfile: 'Back to profile', feeLabel: 'Upgrade fee', feeNotice: 'The fee is frozen from your available balance when you submit. It is charged only after the upgrade is approved. If the upgrade is rejected or withdrawn, the frozen amount is returned to your balance.', chargedFee: 'Fee snapshot', free: 'Free', submit: 'Submit for review', withdraw: 'Cancel application', insufficientBalance: 'Your balance cannot cover the upgrade fee', companySizePlaceholder: 'Select company size', companySizeInvalid: 'Please select a valid company size', ineligible: { not_personal_root: 'This identity cannot request a company upgrade.', already_company_account: 'This account is already a company account.', application_pending: 'An upgrade application is already pending.', unknown: 'This account is not eligible for a company upgrade.' } },
	nameChange: { action: 'Request rename', title: 'Request company rename', submit: 'Submit for review', pending: 'The rename request is pending. The current company name remains active until approval.' },
    password: { title: 'Change initial password', new: 'New password', confirm: 'Confirm password', mismatch: 'The passwords do not match' },
	recovery: { title: 'IAM recovery email', code: 'Verification code', send: 'Send code', verify: 'Verify email', change: 'Change email', sent: 'Verification code sent.', verified: 'Recovery email verified.' },
	members: { slots: 'IAM members {used}/{limit}', create: 'Create IAM member', username: 'Username (optional)', usernamePlaceholder: 'Display username', password: 'Password', generatePassword: 'Generate password', showPassword: 'Show password', hidePassword: 'Hide password', mustChangePassword: 'Require this member to change the password at first sign-in', recoveryEmail: 'Recovery email (optional)', allocateFunds: 'Allocate / reclaim funds', resetPassword: 'Reset password', disable: 'Disable', enable: 'Enable', archive: 'Archive', archiveConfirm: 'Archive IAM member {name}? This cannot be undone.', authorize: 'Authorize', oneTimeCredential: 'One-time credentials', oneTimeWarning: 'This password cannot be displayed again after closing.', copied: 'Credentials copied' },
    spendLimits: { configure: 'Configure member spending limits', description: 'Only shared company balance and enterprise subscription usage count. Member allocated balance is excluded. Member-specific rules override the all-member default.', target: 'Limit scope', allMembers: 'All members limit', selectedMembers: 'Selected members', noMembers: 'No configurable members', dailyLimit: 'Daily limit', monthlyLimit: 'Monthly limit', enableAlert: 'Enable email alerts', alertThreshold: 'Alert threshold', additionalRecipients: 'Recipients', recipientsPlaceholder: 'Enter an email or select a company member', recipientsHint: 'Autocomplete uses company members\' recovery emails. You can also add an email manually.', removeRecipient: 'Remove recipient', rules: 'Configured rules', alert: 'Alert', currentUsage: 'Current member usage', dailyUsage: 'Today / limit', monthlyUsage: 'This month / limit', unlimited: 'Unlimited', noUsage: 'No member usage', deleteConfirm: 'Delete this member spending limit rule?' },
    allocation: { amount: 'Amount', allocate: 'Allocate', reclaim: 'Reclaim', rootAvailable: 'Company balance available to allocate: {amount}', targetAvailable: 'Target account available balance' },
    finance: { available: 'Available', frozen: 'Frozen', total: 'Total', companyBalance: 'Company balance', company_available: 'Company available', company_frozen: 'Company frozen', company_total: 'Company total', noPermission: 'No permission to view', transferAmount: 'Deposit to company', deposit: 'Deposit to company', withdraw: 'Withdraw to personal', depositAvailable: 'Available to deposit', withdrawAvailable: 'Available to withdraw', companyBalanceHint: 'Move funds from your personal balance into the company balance, or back. Company API keys consume the company balance.' },
    balanceSource: { label: 'Balance source', self: 'Root balance', allocated: 'Allocated member balance', shared: 'Legacy shared balance', company: 'Company balance', subscription: 'Enterprise subscription' },
    subscriptions: { description: 'Provision subscription plans (groups) for the company; enterprise API keys can bind to these plans and consume their quota.', createTitle: 'Provision subscription', group: 'Plan group', selectGroup: 'Select a group', createHint: 'Provisioning a subscription requires a paid order, just like a personal subscription. The current owner pays and the subscription is provisioned onto the company subject.', empty: 'No subscriptions yet', rate: 'Rate', status: 'Status', usage: 'Usage', expiresAt: 'Expires', daily: 'Daily', monthly: 'Monthly', cancel: 'Cancel', statuses: { active: 'Active', expired: 'Expired', cancelled: 'Cancelled' } },
    dashboard: { requests: 'Requests', tokens: 'Tokens', cost: 'Actual cost', members: 'Company members' },
    usage: { member: 'Login name', username: 'Username', allMembers: 'All members', apiKey: 'API key', apiKeyId: 'API key ID', searchApiKeyPlaceholder: 'Search API key by name...', model: 'Model', endpoint: 'Endpoint', tokens: 'Tokens', charge: 'Charge', duration: 'Duration (ms)', time: 'Request time', start: 'Start time', end: 'End time', charged: 'Charged', refunded: 'Refunded', total: '{total} records', previous: 'Previous', next: 'Next', columnSettings: 'Columns', statRequests: 'Total requests', statInputTokens: 'Input tokens', statOutputTokens: 'Output tokens', statCost: 'Total cost', trendTitle: 'Daily usage trend', trendEmpty: 'No trend data' },
    admin: { title: 'Company account reviews', applicant: 'Applicant', similar: 'Similar names', approve: 'Approve', reject: 'Reject', upgrades: 'Account upgrades', nameChanges: 'Name changes', organizations: 'Organizations', currentName: 'Current name', requestedName: 'Requested name', audit: 'Audit history', members: 'IAM members', suspend: 'Suspend', reactivate: 'Reactivate' },
    settings: {
      title: 'Feature settings',
      description: 'Manage company-wide feature toggles. Changes apply to all members of this company.',
      autoSwitchSubscription: {
        label: 'Auto-switch subscription plan',
        description: 'When enabled, an API key bound to an enterprise subscription automatically falls over to the next same-platform subscription with quota available when the current plan is exhausted (or has expired). Candidates are tried in order of subscription start time (oldest first).',
        on: 'Enabled',
        off: 'Disabled'
      },
      save: 'Save',
      saving: 'Saving...',
      saved: 'Settings saved',
      noPermission: 'You do not have permission to change feature settings.',
      fallback: {
        badge: 'Auto-switch',
        badgeOff: 'Auto-switch off',
        chainTitle: 'Fallback order',
        chainCurrent: 'Current',
        chainNext: 'Candidate #{index}',
        chainEmpty: 'No fallback candidates (no other same-platform enterprise subscription is available).',
        loading: 'Loading...',
        loadFailed: 'Failed to load fallback order.',
        tooltipHelp: 'View auto-switch help and fallback order',
        tooltipIntro: 'When the current plan is exhausted (or has expired), the following same-platform plans will be tried in order.'
      }
    }
  }
}
