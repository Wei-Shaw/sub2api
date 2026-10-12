export default {
  videoTasks: {
    title: 'Video Tasks',
    description: 'Read-only task progress, failures and billing status',
    coverage: 'Recorded video tasks are checked automatically. Unknown submissions require upstream review and are never automatically resubmitted.',
    billingNote: 'Charges are synchronized with usage records. This page monitors progress only and never charges or retroactively bills a task. Expected cost is not a recorded charge.',
    elapsed: '{hours}h {minutes}m {seconds}s',
    search: 'Search',
    searchPlaceholder: 'Task / model / user / account (literal text)',
    status: 'Status',
    allStatuses: 'All statuses',
    recent: 'Recent',
    days: 'Last {count} days',
    empty: 'No video tasks match these filters.',
    loadFailed: 'Could not load video tasks. Refresh to try again.',
    detail: 'Task details',
    columns: {
      task: 'Task / model', user: 'User / API key', account: 'Account / group',
      status: 'Status / explanation', billing: 'Billing', created: 'Created', actions: 'Details'
    },
    fields: {
      taskId: 'Task ID', kind: 'Record type', model: 'Model', user: 'User', userId: 'User ID',
      apiKey: 'API key name', apiKeyId: 'API key ID', account: 'Account', accountId: 'Account ID',
      group: 'Group', groupId: 'Group ID', expected: 'Expected cost', charged: 'Charged cost',
      created: 'Created', updated: 'Updated', completed: 'Completed', nextCheck: 'Next check',
      attempts: 'Processing attempts', duration: 'Duration (seconds)', resolution: 'Resolution', usageLogId: 'Usage record ID', elapsed: 'Elapsed until completion / last update'
    },
    kinds: { task: 'Task', submission: 'Unresolved submission' },
    statuses: {
      in_progress: 'In progress', delayed: 'Delayed', query_retry: 'Status check retry',
      billing_pending: 'Billing pending', completed: 'Completed', failed: 'Failed',
      submission_unknown: 'Submission result unknown', review_required: 'Needs review', unknown: 'Unknown status'
    },
    explanations: {
      in_progress: 'The task is still processing.',
      delayed: 'Pending for over two hours. This does not confirm a failure.',
      query_retry: 'A status check could not be completed. The system will retry the check; this does not create another video.',
      billing_pending: 'Completion or billing is being reconciled. Consult the billing status for the recorded charge.',
      completed: 'Task processing is complete.',
      failed: 'The upstream task ended unsuccessfully. If it was not charged, no refund is needed.',
      submission_unknown: 'Submission may still be in progress or its result is unknown. Do not submit again with a new idempotency key; a duplicate video could be created. Site billing needs review; upstream costs may already have been incurred.',
      review_required: 'Automatic checks are paused pending upstream review. This does not mean generation failed and will not recreate the video or trigger a charge.',
      unknown: 'No recognized task status is available.'
    },
    billing: { not_charged: 'Not charged', pending: 'Pending settlement', charged: 'Charged', review: 'Needs review', unknown: 'Unknown billing status' },
    reasons: {
      poll_failed: 'The latest status check failed. The system will check again.',
      poll_not_found: 'The upstream could not find the task. Checks will retry within a bounded period without recreating the video.',
      poll_exhausted: 'The task is over a day old and repeated checks could not confirm its outcome. Automatic checks have stopped.',
      account_route_changed: 'The original upstream account or route changed. Checks are paused to avoid associating the wrong result.',
      poll_timeout: 'The status check timed out. This does not confirm a task failure; the system will check again.',
      upstream_failed: 'The upstream service reported that the task failed.',
      upstream_expired: 'The upstream service reported an expired result. Generation and billing outcomes still need review; the video will not be generated again.',
      upstream_cancelled: 'The upstream service reported that the task was cancelled.',
      completion_save_failed: 'The completion record could not be saved. The system will reconcile it again.',
      billing_failed: 'Billing settlement is not yet confirmed. The system will reconcile it again without generating another video.',
      unknown: 'An additional error was recorded; no safe explanation is available.'
    }
  }
}
