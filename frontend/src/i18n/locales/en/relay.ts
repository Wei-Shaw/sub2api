export default {
  relay: {
    keepStatus: 'Keep current status',
    pending: 'Pending', pendingNotice: 'Pending earnings {amount}: credited on refresh after the consumer funds their debt.',
    title: 'My relay', description: 'Connect your upstreams for private use, or share accounts to earn spending credit.',
    privateNotice: 'Private groups are owner-only and incur no site charge. Upstream costs and quotas still apply.',
    publicNotice: 'Public calls are charged at group prices and reward the account supplier, including self-use. Requests pass through the supplier’s upstream.',
    balanceNotice: 'Earnings go directly to your spending balance. Withdrawals are not supported.',
    totalEarned: 'Total earnings', todayEarned: 'Today’s earnings (UTC)', accounts: 'My upstream accounts', privateGroups: 'My private groups',
    earnings: 'Earnings history', addAccount: 'Connect upstream', editAccount: 'Edit upstream', addGroup: 'Create private group', editGroup: 'Edit private group',
    emptyAccounts: 'Connect an upstream account to start using your relay.', emptyEarnings: 'Earnings appear here when your shared accounts receive calls.',
    name: 'Name', platform: 'Platform', type: 'Credential type', apiKey: 'API key', oauth: 'OAuth credentials', baseUrl: 'Upstream URL',
    baseUrlHint: 'Leave blank for the platform default, or enter a public HTTP(S) relay URL.',
    secret: 'Upstream API key', secretHint: 'Leave blank when editing to retain the existing credential.', concurrency: 'Account concurrency',
    credentialJson: 'Additional credentials (optional JSON)', credentialHint: 'For OAuth, supply access_token and refresh_token. model_mapping is also supported.',
    privateSelection: 'Private groups', publicSelection: 'Share with public groups', publicSelectionHint: 'Selected groups can route public traffic to this account. Unselect to stop sharing. Private and public calls share upstream quotas.',
    supplierRate: 'Supplier share (0–1)', supplierRateHint: 'Applies to user-owned accounts in public balance groups. 0.9 means a 90% share.',
    shared: 'Shared', privateOnly: 'Private only', groups: 'Groups', status: 'Status', active: 'Active', disabled: 'Disabled',
    createKey: 'Manage API keys', createKeyHint: 'Create an API key bound to your private group for free site usage.',
    saved: 'Saved', saveFailed: 'Could not save', loadFailed: 'Could not load', deleted: 'Deleted', deleteFailed: 'Could not delete',
    deleteTitle: 'Delete resource', deleteMessage: 'New calls will stop. Historical earnings remain. Delete this resource?',
    invalidJson: 'Additional credentials must be a JSON object', groupRequired: 'Select at least one private or public group',
    accountId: 'Account', groupId: 'Group', charged: 'Charge', earned: 'Earnings', time: 'Time', selfUse: 'Self-use',
    previous: 'Previous', next: 'Next', page: 'Page {page}', defaultGroup: 'Default private group',
    descriptionLabel: 'Description', refresh: 'Refresh'
  }
}
