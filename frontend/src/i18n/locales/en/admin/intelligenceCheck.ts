export default {
  intelligenceCheck: {
    title: 'Intelligence check',
    description: 'Runs the "pelican riding a bicycle" drawing task on an account to gauge the model\'s real capability.',
    subtitle: 'Every account with the intelligence check enabled runs the "pelican riding a bicycle" drawing task once and produces an animated single-file HTML artwork you can preview directly. The check only produces the artwork; whether it passes is decided by an admin review. There is one card per enabled account, showing that account\'s latest result.',
    refresh: 'Refresh',
    runAll: 'Re-run all',
    running: 'Running…',
    rerun: 'Re-run',
    empty: 'No accounts yet. Add one under Account Management first.',
    noEnabledAccounts: 'No account has the intelligence check enabled yet. Go to Account Management → edit an account → Intelligence check and switch it on.',
    previewNotice: 'The artwork is AI-generated. Its scripts run inside an isolated sandbox, so previewing is safe.',
    accountLabel: 'Account {index}',
    badge: {
      pass: 'Passed',
      fail: 'Not passed',
      unknown: 'Pending review',
      runFailed: 'Run failed',
      running: 'Running',
      untested: 'Untested'
    },
    card: {
      noModel: 'No model recorded',
      accountMeta: '#{id} · {model}',
      accountMetaWithEffort: '#{id} · {model} · {effort}',
      publicMetaWithEffort: '{model} · effort {effort}',
      detail: '{latency} · {time}',
      noArtifact: 'No artwork yet',
      pendingThumbnail: 'Loading artwork…',
      generating: 'Generating artwork…',
      untestedHint: 'No check recorded yet',
      approve: 'Approve',
      reject: 'Reject',
      reviewHint: 'Pending review: inspect the artwork and decide',
      reviewed: 'Reviewed {time}',
      reviewedNoTime: 'Reviewed'
    },
    reviewSubmitted: 'Review submitted: {verdict}',
    reviewSubmittedSynced: 'Review submitted: {verdict} (account status synced)',
    error: {
      load: 'Failed to load intelligence check data',
      trigger: 'Failed to queue the check',
      review: 'Failed to submit the review'
    }
  }
}
