/**
 * English i18n — `monitorCommon.*` namespace (shared admin + user monitor keys).
 *
 * T19 Fix B — split out from monolithic `en.ts`.
 */
export default {
  status: {
    operational: 'Operational',
    degraded: 'Degraded',
    failed: 'Failed',
    error: 'Error',
    unknown: '-',
  },
  providers: {
    openai: 'OpenAI',
    anthropic: 'Anthropic',
    gemini: 'Gemini',
  },
  extraModelsHeader: 'Extra Models',
  extraModelsEmpty: 'No extra models',
  latencyEmpty: '-',
  availabilityPrefix: 'Availability',
  dialogLatency: 'Dialog Latency',
  endpointPing: 'Endpoint PING',
  history60pts: 'HISTORY ({n} PTS)',
  nextUpdateIn: 'NEXT UPDATE IN {n}s',
  past: 'PAST',
  now: 'NOW',
  maintenancePaused: 'Maintenance · timeline paused',
  extraModelsCount: '+ {n} models',
  pollEvery: '{n}s polling',
  updatedAt: 'Updated {time}',
  relativeSecondsAgo: '{n}s ago',
  relativeMinutesAgo: '{n}m ago',
  relativeHoursAgo: '{n}h ago',
  relativeDaysAgo: '{n}d ago',
}
