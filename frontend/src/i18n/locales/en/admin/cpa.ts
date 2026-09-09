export default {
  cpa: {
    title: 'CPA Panel',
    description: 'Read-only view of the local CLIProxyAPI instance (credentials, quota, usage, logs)',
    notConfigured: 'CPA panel is not configured yet. Fill in the base URL and management key below.',
    disabled: 'CPA panel is disabled. Enable it below to load data.',

    connection: {
      title: 'Connection',
      baseUrl: 'CPA base URL',
      baseUrlHint: 'Reachable from the Sub2API container, e.g. http://host.docker.internal:8317',
      managementKey: 'Management key',
      managementKeyPlaceholder: 'Enter to set, leave empty to keep the current key',
      managementKeyKeep: 'A key is stored ({hint}). Leave empty to keep it.',
      managementKeyClear: 'Clear the stored key',
      enabled: 'Enable CPA panel',
      save: 'Save',
      saved: 'CPA settings saved',
      test: 'Test connection',
      testing: 'Testing…',
      reachable: 'Reachable',
      unreachable: 'Unreachable',
      healthOk: 'Health OK',
      keyAccepted: 'Management key accepted',
      keyRejected: 'Management key rejected',
      version: 'Version',
      commit: 'Commit',
      buildDate: 'Build date',
      supportPlugin: 'Plugin support',
      neverTested: 'Not tested yet'
    },

    tabs: {
      authFiles: 'Auth files & quota',
      usage: 'Usage',
      logs: 'Logs'
    },

    authFiles: {
      empty: 'No auth files reported by CPA.',
      name: 'Auth file',
      provider: 'Provider',
      status: 'Status',
      account: 'Account',
      success: 'Success',
      failed: 'Failed',
      lastRefresh: 'Last refresh',
      nextRetry: 'Next retry',
      priority: 'Priority',
      note: 'Note',
      plan: 'Plan',
      refreshQuota: 'Fetch quota',
      refreshAllQuota: 'Fetch all quotas',
      quotaLoading: 'Fetching quota…',
      quotaError: 'Quota request failed: {message}',
      quotaEmpty: 'No quota data returned',
      fiveHour: '5h window',
      weekly: 'Weekly window',
      monthly: 'Monthly window',
      codeReview: 'Code review',
      additional: 'Additional limit: {name}',
      resetCredits: 'Reset credits',
      passiveSignals: 'Passive quota signals',
      noSignals: 'No passive signals observed yet',
      observedAt: 'Observed at {time}',
      recentRequests: 'Recent requests',
      unavailable: 'Unavailable',
      disabled: 'Disabled'
    },

    usage: {
      empty: 'No upstream API-key usage reported by CPA.',
      hint: 'CPA only reports usage for API-key upstreams. OAuth credentials are covered by the auth-files tab.',
      provider: 'Provider',
      key: 'Upstream key',
      success: 'Success',
      failed: 'Failed',
      recent: 'Recent buckets'
    },

    logs: {
      requestLogs: 'Request logs',
      requestLogsDisabled: 'CPA has logging-to-file disabled, so request logs are unavailable. Enable it in CPA if you need them.',
      errorLogs: 'Error response logs',
      empty: 'No error logs.',
      fileName: 'File',
      size: 'Size',
      modified: 'Modified',
      view: 'View',
      contentTitle: 'Log content: {name}',
      contentEmpty: '(empty)',
      close: 'Close'
    },

    common: {
      refresh: 'Refresh',
      loading: 'Loading…',
      error: 'Failed to load: {message}',
      enabled: 'Enabled',
      disabled: 'Disabled',
      yes: 'Yes',
      no: 'No',
      unknown: 'Unknown'
    }
  }
}
