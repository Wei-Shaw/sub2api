// Module-owned i18n for traceRequest (admin.traceRequest.*). Keep zh/en keys identical.
export default {
  traceRequest: {
    tabDetail: 'Details',
    tabTrace: 'Trace',
    tabTraceDisabled: 'This error has no request_id, so it cannot be traced',
    title: 'Request chain',
    refresh: 'Refresh',
    loading: 'Loading trace…',
    noRequestId: 'This error has no request_id, so the request chain cannot be queried.',
    empty: 'This request left no trace records',
    emptyHint: 'Errors recorded before tracing was enabled, or requests that never reached an upstream, have no records.',
    loadFailed: 'Failed to load trace',
    retry: 'Retry',
    noAttempts: 'No per-hop records: this may be an error recorded before tracing was enabled, or the request never reached an upstream.',
    noAttemptsSuccess: 'The request succeeded first time; attempts are only recorded for requests that finally failed.',
    attemptNo: 'Hop {n}',
    account: 'Account #{id}',
    accountUnknown: 'Unknown account',
    copyAccountId: 'Click to copy account ID',
    copiedAccountId: 'Copied account ID {id}',
    switchFromTo: 'Failover: switched from account #{from} to account #{to}',
    credentialFpHint: 'Credential fingerprint: a change on the same account means the credential was rotated',
    expand: 'Expand',
    collapse: 'Collapse',
    otherStatusEvents: 'Other account status changes',
    summary: {
      attempts: '{n} hops',
      switches: '{n} account switches',
      succeeded: 'Succeeded',
      failed: 'Failed'
    },
    fields: {
      errorCode: 'Error code',
      elapsed: 'Elapsed (since previous hop)',
      model: 'Model',
      credentialFp: 'Credential FP',
      proxy: 'Proxy',
      accountState: 'Account state',
      upstreamUrl: 'Upstream URL'
    },
    kind: {
      none: 'OK',
      auth: 'Auth failure',
      rate_limit: 'Rate limited',
      overload: 'Overloaded',
      transient: 'Transient',
      client: 'Client error'
    },
    statusEvent: {
      coolingUntil: 'Account #{id} cooling until {until}',
      invalid: 'Account #{id} marked invalid (removed from scheduling)',
      changed: 'Account #{id} state: {from} → {to}'
    },
    source: {
      classify: 'Error classification',
      refresh: 'Token refresh',
      manual: 'Manual',
      expiry: 'Expiry',
      scheduled_test: 'Scheduled test'
    },
    final: {
      success: 'Final result: success',
      failure: 'Final result: failure',
      account: 'Account',
      model: 'Model',
      tokens: 'Input / output tokens',
      duration: 'Total duration',
      firstToken: 'First token',
      cost: 'Cost',
      upstreamStatus: 'Upstream {code}',
      phase: 'Phase',
      type: 'Type'
    }
  }
}
