export default {
  audit: {
    title: 'Audit Logs',
    description: 'Records management-plane operations by admins and users. Header credentials keep only their first/last characters and request bodies are redacted. Entries cannot be deleted individually; clearing all requires two-factor verification.',
    clearAll: 'Clear All',
    empty: 'No audit logs yet',
    loadFailed: 'Failed to load audit logs',
    filters: {
      all: 'All',
      q: 'Keyword',
      qPlaceholder: 'Path / action / actor email',
      actorEmail: 'Actor Email',
      action: 'Action',
      clientIp: 'Client IP',
      method: 'Method',
      authMethod: 'Auth Method',
      result: 'Result',
      resultSuccess: 'Success',
      resultFailure: 'Failure',
      startTime: 'Start Time',
      endTime: 'End Time'
    },
    columns: {
      time: 'Time',
      actor: 'Actor',
      action: 'Action',
      method: 'Method',
      result: 'Result',
      clientIp: 'Client IP',
      detail: 'Detail'
    },
    detail: {
      title: 'Audit Log Detail',
      actorRole: 'Role',
      methodPath: 'Method / Path',
      latency: 'Latency',
      requestId: 'Request ID',
      credential: 'Credential (masked)',
      userAgent: 'User-Agent',
      requestBody: 'Request Body (redacted)',
      extra: 'Extra'
    },
    clearConfirm: {
      title: 'Clear All Audit Logs',
      message: 'This permanently deletes all audit logs and cannot be undone. The clear action itself is recorded. Continue?',
      totpTitle: 'Enter Two-Factor Code',
      totpHint: 'Clearing audit logs requires a fresh TOTP verification.',
      success: 'Cleared {count} audit log(s)',
      failed: 'Failed to clear audit logs'
    }
  }
}
