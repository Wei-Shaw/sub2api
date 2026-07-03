/** WebSearch emulation mode values (must match backend WebSearchMode* constants in account.go) */
export const WEB_SEARCH_MODE_DEFAULT = 'default' as const
export const WEB_SEARCH_MODE_ENABLED = 'enabled' as const
export const WEB_SEARCH_MODE_DISABLED = 'disabled' as const
export type WebSearchMode = typeof WEB_SEARCH_MODE_DEFAULT | typeof WEB_SEARCH_MODE_ENABLED | typeof WEB_SEARCH_MODE_DISABLED

/** Quota notification threshold type values (must match thresholdType* constants in balance_notify_service.go) */
export const QUOTA_THRESHOLD_TYPE_FIXED = 'fixed' as const
export const QUOTA_THRESHOLD_TYPE_PERCENTAGE = 'percentage' as const
export type QuotaThresholdType = typeof QUOTA_THRESHOLD_TYPE_FIXED | typeof QUOTA_THRESHOLD_TYPE_PERCENTAGE

/** Quota reset mode values */
export const QUOTA_RESET_MODE_ROLLING = 'rolling' as const
export const QUOTA_RESET_MODE_FIXED = 'fixed' as const
export type QuotaResetMode = typeof QUOTA_RESET_MODE_ROLLING | typeof QUOTA_RESET_MODE_FIXED

/** Vertex AI location options for Service Account accounts */
export const VERTEX_LOCATION_OPTIONS = [
  {
    label: 'Common',
    options: [
      { value: 'us-central1', label: 'us-central1 (Iowa)' },
      { value: 'global', label: 'global' },
      { value: 'us', label: 'us' },
      { value: 'eu', label: 'eu' }
    ]
  },
  {
    label: 'United States',
    options: [
      { value: 'us-east1', label: 'us-east1 (South Carolina)' },
      { value: 'us-east4', label: 'us-east4 (Northern Virginia)' },
      { value: 'us-east5', label: 'us-east5 (Columbus)' },
      { value: 'us-south1', label: 'us-south1 (Dallas)' },
      { value: 'us-west1', label: 'us-west1 (Oregon)' },
      { value: 'us-west4', label: 'us-west4 (Las Vegas)' }
    ]
  },
  {
    label: 'Europe',
    options: [
      { value: 'europe-west1', label: 'europe-west1 (Belgium)' },
      { value: 'europe-west2', label: 'europe-west2 (London)' },
      { value: 'europe-west3', label: 'europe-west3 (Frankfurt)' },
      { value: 'europe-west4', label: 'europe-west4 (Netherlands)' },
      { value: 'europe-west6', label: 'europe-west6 (Zurich)' },
      { value: 'europe-west8', label: 'europe-west8 (Milan)' },
      { value: 'europe-west9', label: 'europe-west9 (Paris)' }
    ]
  },
  {
    label: 'Asia Pacific',
    options: [
      { value: 'asia-east1', label: 'asia-east1 (Taiwan)' },
      { value: 'asia-east2', label: 'asia-east2 (Hong Kong)' },
      { value: 'asia-northeast1', label: 'asia-northeast1 (Tokyo)' },
      { value: 'asia-northeast3', label: 'asia-northeast3 (Seoul)' },
      { value: 'asia-south1', label: 'asia-south1 (Mumbai)' },
      { value: 'asia-southeast1', label: 'asia-southeast1 (Singapore)' },
      { value: 'australia-southeast1', label: 'australia-southeast1 (Sydney)' }
    ]
  }
] as const

/** 下拉选项类型（兼容 Select.vue：kind:'group' 渲染为不可选分组标题） */
export interface RegionSelectOption {
  value: string
  label: string
  disabled?: boolean
  kind?: 'group'
  [key: string]: unknown
}

/** Vertex 地区：展平 VERTEX_LOCATION_OPTIONS 供 Select.vue 直接使用（分组标题不可选） */
export const VERTEX_LOCATION_SELECT_OPTIONS: RegionSelectOption[] = VERTEX_LOCATION_OPTIONS.flatMap(
  (g) => [
    { value: `__group_${g.label}`, label: g.label, disabled: true, kind: 'group' as const },
    ...g.options.map((o) => ({ value: o.value, label: o.label }))
  ]
)

/** AWS Bedrock 地区选项（原 CreateAccountModal 模板硬编码的 optgroup 抽取至此） */
export const BEDROCK_REGION_OPTIONS = [
  {
    label: 'US',
    options: [
      { value: 'us-east-1', label: 'us-east-1 (N. Virginia)' },
      { value: 'us-east-2', label: 'us-east-2 (Ohio)' },
      { value: 'us-west-1', label: 'us-west-1 (N. California)' },
      { value: 'us-west-2', label: 'us-west-2 (Oregon)' },
      { value: 'us-gov-east-1', label: 'us-gov-east-1 (GovCloud US-East)' },
      { value: 'us-gov-west-1', label: 'us-gov-west-1 (GovCloud US-West)' }
    ]
  },
  {
    label: 'Europe',
    options: [
      { value: 'eu-west-1', label: 'eu-west-1 (Ireland)' },
      { value: 'eu-west-2', label: 'eu-west-2 (London)' },
      { value: 'eu-west-3', label: 'eu-west-3 (Paris)' },
      { value: 'eu-central-1', label: 'eu-central-1 (Frankfurt)' },
      { value: 'eu-central-2', label: 'eu-central-2 (Zurich)' },
      { value: 'eu-south-1', label: 'eu-south-1 (Milan)' },
      { value: 'eu-south-2', label: 'eu-south-2 (Spain)' },
      { value: 'eu-north-1', label: 'eu-north-1 (Stockholm)' }
    ]
  },
  {
    label: 'Asia Pacific',
    options: [
      { value: 'ap-northeast-1', label: 'ap-northeast-1 (Tokyo)' },
      { value: 'ap-northeast-2', label: 'ap-northeast-2 (Seoul)' },
      { value: 'ap-northeast-3', label: 'ap-northeast-3 (Osaka)' },
      { value: 'ap-south-1', label: 'ap-south-1 (Mumbai)' },
      { value: 'ap-south-2', label: 'ap-south-2 (Hyderabad)' },
      { value: 'ap-southeast-1', label: 'ap-southeast-1 (Singapore)' },
      { value: 'ap-southeast-2', label: 'ap-southeast-2 (Sydney)' }
    ]
  },
  {
    label: 'Canada',
    options: [{ value: 'ca-central-1', label: 'ca-central-1 (Canada)' }]
  },
  {
    label: 'South America',
    options: [{ value: 'sa-east-1', label: 'sa-east-1 (São Paulo)' }]
  }
] as const

/** Bedrock 地区：展平供 Select.vue 使用 */
export const BEDROCK_REGION_SELECT_OPTIONS: RegionSelectOption[] = BEDROCK_REGION_OPTIONS.flatMap(
  (g) => [
    { value: `__group_${g.label}`, label: g.label, disabled: true, kind: 'group' as const },
    ...g.options.map((o) => ({ value: o.value, label: o.label }))
  ]
)
