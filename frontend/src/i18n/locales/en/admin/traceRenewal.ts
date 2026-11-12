// Module-owned i18n for traceRenewal (admin.traceRenewal.*). Keep zh/en keys identical.
export default {
  traceRenewal: {
    title: 'Renewal',
    refresh: 'Refresh',
    windowLabel: 'Window',
    windowDays: '{n}d',
    loading: 'Loading…',
    loadFailed: 'Failed to load expiring accounts',
    retry: 'Retry',
    empty: 'No accounts need renewal within {days} days',
    alertScopeHint:
      'The "accounts expiring soon" alert metric counts only accounts expiring within 72 hours plus expired accounts still in scheduling — {n} right now. Paused expired accounts are excluded.',
    groups: {
      expiredActive: 'Expired, still scheduling',
      expiredPaused: 'Expired, paused',
      within24h: 'Within 24 hours',
      within3d: 'Within 3 days',
      within7d: 'Within 7 days',
      later: 'Beyond 7 days'
    },
    groupHints: {
      expiredActive: 'Still receives traffic but its credentials/subscription have lapsed — renew or pause now',
      expiredPaused: 'No longer receives traffic; re-enable scheduling after renewal'
    },
    columns: {
      name: 'Account',
      platform: 'Platform',
      remaining: 'Time left',
      expiresAt: 'Expires at',
      autoPause: 'Auto-pause on expiry',
      scheduling: 'Scheduling'
    },
    remaining: {
      leftMinutes: '{n} min left',
      leftHours: '{n} h left',
      leftDays: '{n} d left',
      expiredMinutes: 'Expired {n} min ago',
      expiredHours: 'Expired {n} h ago',
      expiredDays: 'Expired {n} d ago'
    },
    autoPauseOn: 'On',
    autoPauseOff: 'Off',
    scheduling: {
      active: 'Scheduling',
      paused: 'Paused',
      disabled: 'Disabled',
      error: 'Error'
    },
    openAccount: 'Open in account management'
  }
}
