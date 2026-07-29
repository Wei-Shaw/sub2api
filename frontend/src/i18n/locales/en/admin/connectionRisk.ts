export default {
  connectionRisk: {
    title: 'Connection Risk',
    description:
      'Detect multi-IP/UA API key sharing, credential leaks, and session-binding anomalies. Observe-only by default.',
    tabs: {
      events: 'Events',
      config: 'Policy',
      runtime: 'Runtime',
    },
    columns: {
      severity: 'Severity',
      score: 'Score',
      subject: 'Subject',
      rules: 'Rules',
      status: 'Status',
      lastSeen: 'Last seen',
    },
    filters: {
      allStatus: 'All statuses',
      allSeverity: 'All severities',
      userId: 'User ID',
      keyId: 'Key ID',
    },
    empty: 'No risk events',
    actions: {
      ack: 'Acknowledge',
      resolve: 'Resolve',
      suppress: 'Suppress',
      whitelist: 'Apply IP whitelist',
      runRetention: 'Run retention',
    },
    config: {
      enabled: 'Enable feature (settings layer)',
      emitEnabled: 'Enable hot-path signal emit',
      workerEnabled: 'Enable scoring worker',
      includeReadOnly: 'Include read-only endpoints',
      r7Admin: 'Count admin session-binding mismatches (R7)',
      softThrottle: 'Enable soft throttle (Phase B)',
      autoDisable: 'Enable auto-disable (Phase C, dangerous)',
      sampleRate: 'Evidence sample rate',
      workerInterval: 'Worker interval (seconds)',
      throttleRpm: 'Throttle absolute RPM',
      retentionDays: 'Event retention days',
      yamlHint:
        'Process master switch connection_risk.enabled must still be true in YAML, otherwise emit and worker are no-ops.',
    },
    messages: {
      saved: 'Settings saved',
      actionOk: 'Action completed',
      whitelisted: 'IP whitelist merged and key exempted',
      retention: 'Deleted {count} expired events',
    },
    errors: {
      loadEvents: 'Failed to load events',
      loadConfig: 'Failed to load config',
      loadRuntime: 'Failed to load runtime',
      saveConfig: 'Failed to save config',
      action: 'Action failed',
      noSampleIPs: 'No sample_ips in evidence',
    },
  },
}
