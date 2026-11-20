// Module-owned i18n for traceAccount (admin.traceAccount.*). Keep zh/en keys identical.
export default {
  traceAccount: {
    menuEntry: 'Trace',
    title: 'Account trace: {name}',
    subtitle: '#{id} · {platform}',
    retry: 'Retry',
    loadFailed: 'Failed to load: {message}',
    none: '—',
    copy: 'Copy',
    copied: 'Copied',
    tabs: {
      overview: 'Overview',
      status: 'Scheduling status',
      ledger: 'Subscription ledger'
    },
    range: {
      label: 'Window',
      h1: 'Last 1h',
      h24: 'Last 24h',
      d7: 'Last 7d'
    },
    overview: {
      failures: 'Failures',
      failuresHint: 'Failed upstream attempts in window',
      distribution: 'Failure kinds',
      lastFailure: 'Last failure',
      noFailure: 'No failures in window',
      fingerprints: 'Credential fingerprints',
      fingerprintsHint: 'Credential versions used in window',
      attemptsTitle: 'Failed attempts',
      attemptsLimitHint: 'Showing up to the latest {limit} attempts',
      empty: 'No failed attempts in this window. The account looks healthy.',
      fpSwitchedAt: 'This fingerprint was switched in at {time} (see ledger)',
      requestIdMissing: 'request_id not returned by the API',
      columns: {
        time: 'Time',
        requestId: 'request_id',
        status: 'Status',
        kind: 'Kind',
        errorCode: 'Error code',
        fingerprint: 'Fingerprint',
        failover: 'Failover'
      },
      failoverFrom: 'From #{id}',
      firstHop: 'First hop'
    },
    kinds: {
      auth: 'Auth',
      rate_limit: 'Rate limit',
      overload: 'Overload',
      transient: 'Transient',
      client: 'Client'
    },
    status: {
      empty: 'No scheduling status changes in this window (never cooled down or removed).',
      enterCooling: 'Entered cooldown',
      enterInvalid: 'Marked invalid (removed)',
      enterState: 'Entered {state}',
      coolingUntil: 'Cooling until {time}',
      coolingDuration: 'about {minutes} min',
      fromState: 'Previous state: {state}',
      reason: 'Reason: {reason}',
      errorCode: 'Error code: {code}',
      triggeredBy: 'Triggered by',
      owner: 'Who recovers it: ',
      sources: {
        classify: {
          label: 'Triggered by a request',
          desc: 'An upstream response to a real request was classified as an account problem.',
          owner: 'Cooldown recovers automatically; "invalid" needs an admin to check the credential or upstream account.'
        },
        refresh: {
          label: 'Token refresh',
          desc: 'Background OAuth token refresh failed.',
          owner: 'An admin re-authorizes or updates the credential.'
        },
        manual: {
          label: 'Manual',
          desc: 'An admin changed the scheduling status by hand.',
          owner: 'The admin who made the change.'
        },
        expiry: {
          label: 'Subscription expired',
          desc: 'The expiry time passed and the account was auto-paused.',
          owner: 'Renew, then extend the expiry time in account settings.'
        },
        scheduled_test: {
          label: 'Scheduled test',
          desc: 'A scheduled connectivity test failed.',
          owner: 'Recovers after the next passing test; persistent failures need an admin.'
        },
        unknown: {
          label: 'Other source ({source})',
          owner: 'Needs manual judgement.'
        }
      }
    },
    ledger: {
      hideTokenRefresh: 'Hide token refreshes',
      hiddenCount: '{count} token refreshes hidden',
      total: '{count} events',
      emptyTitle: 'No ledger entries yet',
      emptyDesc: 'The ledger is written by a database trigger. Accounts created before the ledger was enabled and not changed since (status, expiry, plan/quota, credentials…) have an empty ledger; recording starts with the next change to those fields.',
      allHidden: 'Only token refresh events exist and they are hidden.',
      credentialChanged: 'Credential fingerprint changed',
      credentialFailures: 'This credential failed {count} times in the overview window',
      credentialNoFailures: 'No failures for this credential in the overview window',
      extended: 'extended by {days} days',
      shortened: 'shortened by {days} days',
      snapshot: 'Initial snapshot',
      statusAfter: 'Status after: {status}',
      source: 'Source: {source}',
      operator: 'Operator #{id}',
      emptyValue: '(empty)',
      yes: 'Yes',
      no: 'No',
      events: {
        created: 'Created',
        deleted: 'Deleted',
        restored: 'Restored',
        expired: 'Expired (paused)',
        renewed: 'Renewed',
        expiry_shortened: 'Expiry shortened',
        expiry_set: 'Expiry set',
        expiry_cleared: 'Expiry cleared',
        plan_changed: 'Plan changed',
        quota_changed: 'Quota changed',
        disabled: 'Disabled',
        errored: 'Errored',
        recovered: 'Recovered',
        enabled: 'Enabled',
        credential_rotated: 'Key rotated',
        token_refreshed: 'Token refreshed',
        unschedulable: 'Unschedulable',
        schedulable: 'Schedulable',
        updated: 'Other update'
      },
      fields: {
        status: 'Status',
        schedulable: 'Schedulable',
        expires_at: 'Expires at',
        auto_pause_on_expired: 'Auto-pause on expiry',
        deleted_at: 'Deleted at',
        proxy_id: 'Proxy',
        concurrency: 'Concurrency',
        priority: 'Priority',
        rate_multiplier: 'Rate multiplier',
        name: 'Name',
        credential_fp: 'Credential fingerprint',
        platform: 'Platform',
        type: 'Type',
        extraPrefix: 'Plan/quota · {key}'
      }
    }
  }
}
