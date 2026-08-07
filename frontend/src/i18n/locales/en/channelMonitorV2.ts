/** Channel Monitor V2 (user + admin passive monitor UI) */
export default {
  channelMonitorV2: {
    title: 'Channel Monitor',
    updating: 'Updating data',
    updatedTo: 'Updated to {timeREDACTED',
    partialCoverage: 'Partial historical coverage',
    timeRange: 'Time range',
    clearFilters: 'Reset',
    refreshingFilters: 'Filters changed; refreshing matrix, trend, and details…',
    switchingData: 'Switching filtered data…',
    summaryAria: 'Selected range summary',
    loadFailed: 'Failed to load channel monitor',
    detailLoadFailed: 'Failed to load channel monitor details',
    otherModels: 'Other models',
    ignored: 'Ignored',
    currentUser: 'Current user',
    ranges: { '90m': '90m', '24h': '24h', '7d': '7d', '30d': '30d' REDACTED,
    filters: {
      platform: 'Platform', allPlatforms: 'All', group: 'Group', allGroups: 'All', model: 'Model', allModels: 'All',
      empty: 'No options', selectedCount: '{countREDACTED', labelValue: '{labelREDACTED: {valueREDACTED'
    REDACTED,
    groupBy: {
      label: 'Group by', platform: 'Platform', platformGroup: 'Platform / Group', platformModel: 'Platform / Model', platformGroupModel: 'Platform / Group / Model'
    REDACTED,
    trendView: { label: 'Trend view', pulse: 'Pulse matrix', line: 'Line chart' REDACTED,
    healthMode: { label: 'Health display', overall: 'Overall', success: 'Error rate', ttft: 'First token', cache: 'Cache rate' REDACTED,
    tabs: { aria: 'Detail dimension', models: 'Models', errors: 'Error reasons', users: 'User ranking' REDACTED,
    metrics: {
      rpm: 'RPM', tpm: 'TPM', rpmDetail: 'Request rate', tpmDetail: 'Token rate', errorRate: 'Error rate', ttft: 'First token', ttftP50: 'First token P50', durationP50: 'Duration P50', cacheRate: 'Cache rate', cacheDetail: 'Read cache share', successRate: 'Success rate',
      successRateValue: 'Success rate {valueREDACTED', errorRateValue: 'Error rate {valueREDACTED', rpmValue: 'RPM {valueREDACTED', tpmValue: 'TPM {valueREDACTED', ttftValue: 'First token {valueREDACTED', durationValue: 'Duration {valueREDACTED', cacheRateValue: 'Cache rate {valueREDACTED'
    REDACTED,
    table: { platformModel: 'Platform / Model', rank: 'Rank', user: 'User' REDACTED,
    empty: { title: 'No data to display', description: 'Try changing the time range or filters' REDACTED,
    bucket: { minutes: '{countREDACTED-minute buckets', hours: '{countREDACTED-hour buckets', days: '{countREDACTED-day buckets' REDACTED,
    matrix: {
      title: 'Availability trend', description: 'Each row is a channel dimension and each block is an aggregate interval; hover for details', wheelZoom: 'Wheel over blocks to zoom', wheelZoomX: 'Wheel over blocks to zoom horizontally', dimension: 'Channel dimension', emptyTitle: 'No matrix data for the selected window', legendAria: 'Health score legend', bad: 'Bad', good: 'Good', healthyLegend: 'Healthy (≥80)', warningLegend: 'Watch (50–79)', criticalLegend: 'Critical (<50)', unknownLegend: 'No traffic / insufficient samples', noTraffic: 'No traffic in this interval', noTrafficAt: '{timeREDACTED · no traffic', scoreLine: 'Health score {scoreREDACTED', resetZoom: 'Reset zoom'
    REDACTED,
    chart: {
      title: 'Availability trend', description: 'Smoothed trend: error rate · first token P50 · cache rate', emptyTitle: 'No trend data for the selected window', errorLegend: 'Error rate (left axis %)', cacheLegend: 'Cache rate (left axis %)', ttftLegend: 'First token P50 (right axis)', errorDataset: 'Error rate trend %', cacheDataset: 'Cache rate trend %', ttftDataset: 'First token trend P50 (ms)', percentAxis: 'Rate %', resetZoom: 'Reset zoom'
    REDACTED,
    errorDetail: { http: 'HTTP {codeREDACTED', upstream: 'Upstream {codeREDACTED', noMessage: 'No error message', empty: 'Category rates only (sample messages are admin-only)' REDACTED,
    errorCategories: {
      content_policy: 'Content policy', authentication: 'Authentication', context_limit: 'Context limit', invalid_request: 'Invalid request', model_unsupported: 'Unsupported model', group_access: 'Group access', quota_or_balance: 'Quota or balance', account_pool_unavailable: 'Account pool unavailable', rate_or_capacity: 'Rate or capacity', timeout: 'Timeout', transport_or_stream: 'Transport or stream', upstream_forbidden: 'Upstream forbidden', not_found: 'Not found', client_cancelled: 'Client cancelled', upstream_5xx: 'Upstream 5xx', internal: 'Internal', other: 'Other'
    REDACTED,
    rank: {
      gold: 'Rank 1 gold',
      silver: 'Rank 2 silver',
      bronze: 'Rank 3 bronze',
      place: 'Rank {nREDACTED',
      unranked: 'Unranked',
    REDACTED,
    settings: {
      title: 'V2 data monitor config',
      description:
        'Configure passive usage aggregation dimensions (platform / model / group) and refresh cadence. Health colors and details on the user /monitor page show rates, RPM, and TPM — not absolute request volume.',
      save: 'Save',
      loading: 'Loading…',
      loadFailed: 'Failed to load V2 config',
      saveSuccess: 'V2 monitor config saved',
      saveFailed: 'Failed to save V2 config',
      modeBanner:
        'System mode is currently {modeREDACTED. V2 minute aggregation will not run; this config can be prepared now and takes effect after switching to {modeV2REDACTED. Change mode under System Settings → Feature switches.',
      modeClosed: 'Channel monitor disabled',
      modeV1: 'V1 active probes',
      modeV2: 'V2 passive monitoring',
      enableTitle: 'Enable V2 aggregation',
      enableHint:
        'Applies when system mode is V2. Turning this off only stops this config’s aggregation; the system mode switch remains under Feature switches.',
      refreshTitle: 'Aggregation interval',
      refreshHint: 'Affects matrix time granularity and refresh cadence',
      refreshAria: 'Aggregation interval',
      platformsTitle: 'Platforms and models',
      platformsHint:
        'Leave empty = show all real model names; when filled, only listed models get their own rows and the rest roll into “Other”',
      modelsPlaceholder: 'Empty = all real models; or list popular models (rest → Other)',
      badgeAllModels: 'All models',
      badgeOther: '+ Other',
      groupsTitle: 'Monitored groups',
      groupsSelected: '{countREDACTED groups selected',
      groupsAll: 'All groups',
      groupsEmpty: 'No groups available',
      errorsTitle: 'Error categories and ignores',
      errorsHint:
        'Checked “ignore” categories are excluded from error rate and health score, but still appear greyed in the error breakdown. Unmatched errors roll into “Other”.',
      ignoredSummary: 'Ignored {ignoredREDACTED categories · counted in error rate {countedREDACTED categories',
      healthTitle: 'Health thresholds',
      healthHint:
        'Controls user-facing color bands and overall score. Defaults are tolerant so small error rates or low cache do not immediately show as unhealthy.',
      fields: {
        minimumSample: 'Minimum samples',
        warningError: 'Error rate watch %',
        criticalError: 'Error rate critical %',
        targetTtft: 'TTFT target ms',
        warningTtft: 'TTFT watch ms',
        criticalTtft: 'TTFT critical ms',
        warningCache: 'Cache rate watch %',
        criticalCache: 'Cache rate critical %',
      REDACTED,
      namedModelsEmpty: 'Platform model lists are empty: every real model name will be shown (not folded into “Other”).',
      namedModelsCount: 'Showing {countREDACTED named model dimensions; unlisted models fold into per-platform “Other”.',
      userContractTitle: 'User-facing display contract',
      userContract: {
        health: 'Health color weights: error rate 60% + first-token P50 20% + cache rate 20% (thresholds configurable above)',
        trend: 'Trend can switch between pulse matrix and line chart (error · cache · first token)',
        latency: 'Latency shows AVG · P50 · P90; absolute request / error counts are not shown',
        models: 'Empty model lists show real names and never dump everything into “Other”',
      REDACTED,
    REDACTED,
    admin: {
      descriptionV1:
        'System mode is V1 active probes: manage probe monitors and run checks now; V2 aggregation does not run.',
      descriptionV2:
        'System mode is V2 passive monitoring: configure aggregation dimensions; V1 active probes do not run.',
      tabAria: 'Monitor management',
      tabV2: 'V2 data monitor config',
      tabV1Active: 'V1 active probes',
      tabV1History: 'V1 history (probes not active in current mode)',
    REDACTED,
  REDACTED,
REDACTED
