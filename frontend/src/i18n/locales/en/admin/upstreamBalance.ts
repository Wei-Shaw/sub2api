export default {
  upstreamBalance: {
    title: 'Upstream Balance', description: 'Monitor account balances across Sub2API and New-API upstreams.', add: 'Add upstream', addTitle: 'Add upstream', editTitle: 'Edit upstream', refreshAll: 'Refresh all', enabledOnly: 'Enabled only', empty: 'No upstream monitors configured',
    remaining: 'Remaining', used: 'Used', requests: 'Requests', group: 'Group', account: 'Upstream account', probe: 'Refresh', never: 'Never probed',
    channelStatus: 'Channel status', healthy: 'healthy', todayChanges: 'Rate changes today', changedToday: 'Rates changed today', noChangesToday: 'No changes today', recentChanges: 'Recent rate changes', totalGroups: 'Current groups', groups: 'Groups and rates',
    loadError: 'Failed to load upstream balances', saveError: 'Failed to save upstream', probeError: 'Upstream probe failed', deleteError: 'Failed to delete upstream', deleteConfirm: 'Delete “{name}”?',
    form: { name: 'Site name', type: 'Upstream type', baseUrl: 'Base URL', credentialMode: 'Login method', passwordMode: 'Username and password', tokenMode: 'Token / Cookie', email: 'Login email', username: 'Login username', password: 'Password', passwordHelp: 'Credentials are encrypted. Accounts requiring 2FA or a login challenge cannot be used for automatic login.', accessToken: 'Access token', accessTokenPlaceholder: 'access_token returned by login', accessTokenHelp: 'Use the Sub2API login access token, not an sk- API key.', cookie: 'Browser Cookie', cookieHelp: 'Copy the complete Cookie request header after signing in to New-API.', userId: 'New-Api-User ID', userIdHelp: 'Shown on the New-API personal settings page.', interval: 'Probe interval (minutes)', order: 'Display order', lowThreshold: 'Low balance threshold (USD)', enabled: 'Enable monitoring' },
    status: { ok: 'Healthy', low: 'Low balance', failed: 'Probe failed', pending: 'Pending', disabled: 'Disabled' },
  },
}
