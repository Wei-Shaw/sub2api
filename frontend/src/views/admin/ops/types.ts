// Ops 前端视图层的共享类型（与后端 DTO 解耦）。

export type ChartState = 'loading' | 'empty' | 'ready'

// Re-export ops alert/settings types so view components can import from a single place
// while keeping the API contract centralized in `@/api/admin/ops`.
export type {
  AlertRule,
  AlertEvent,
  AlertSeverity,
  ThresholdMode,
  MetricType,
  Operator,
  EmailNotificationConfig,
  OpsDistributedLockSettings,
  OpsAlertRuntimeSettings,
  OpsMetricThresholds,
  OpsAdvancedSettings,
  OpsDataRetentionSettings,
  OpsAggregationSettings,
  OpsRuntimeLogConfig,
  OpsSystemLog,
  OpsSystemLogSinkHealth
} from '@/api/admin/ops'
