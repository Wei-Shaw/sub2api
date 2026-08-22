export default {
  userIsolationLookup: {
    title: 'Risk User Lookup',
    description: 'Resolve an upstream user isolation identifier',
    account: 'Upstream account',
    selectAccount: 'Select an account with user isolation enabled',
    searchAccount: 'Search account name',
    noEligibleAccounts: 'No matching user isolation accounts',
    isolationID: 'Upstream user identifier',
    isolationIDPlaceholder: 'u1_...',
    lookup: 'Locate user',
    locating: 'Locating...',
    result: 'Lookup result',
    exactMatch: 'Exact match',
    userID: 'User ID',
    email: 'Email',
    username: 'Username',
    status: 'Status',
    lastActiveAt: 'Last active',
    lastUsedAt: 'Last used',
    viewUser: 'User management',
    viewUsage: 'Usage records',
    unknown: '-',
    errors: {
      INVALID_USER_ISOLATION_ID: 'The upstream user identifier is invalid',
      USER_ISOLATION_NOT_ENABLED: 'User isolation is not enabled for this account',
      USER_ISOLATION_USER_NOT_FOUND: 'No matching user was found for this account',
      ACCOUNT_NOT_FOUND: 'The upstream account does not exist',
      USER_ISOLATION_SECRET_UNAVAILABLE: 'The user isolation secret is unavailable',
      default: 'Risk user lookup failed'
    }
  }
}
