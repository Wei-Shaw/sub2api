import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { extractApiErrorMessage, type SelectOption } from '@sub2api/plugin-sdk'
import riskControlAPI, {
  type ContentModerationAPIKeyLoad,
  type ContentModerationAPIKeyStatus,
  type ContentModerationConfig,
  type ContentModerationModelFilter,
  type ContentModerationModelFilterType,
  type ContentModerationTestAuditResult,
  type KeywordBlockingMode,
  type ModerationGroup,
  type ModerationMode,
  type UpdateContentModerationConfig,
} from '../api/riskControl'
import { getSdk } from '../api/sdk'
import type {
  APIKeysWriteMode,
  ConfigForm,
  ContentModerationLog,
  ContentModerationRuntimeStatus,
  FiltersState,
  KeywordNoticeView,
  ModerationScoreRow,
  OverviewItem,
  PaginationState,
  RiskThresholdRow,
  SettingsTab,
  WorkerSlotState,
} from './types'
import {
  formatDateTime,
  formatNumber,
  percent,
  percentWidth,
  latencyText,
  clampPercent,
  formatThresholdPercent,
  parseApiKeys,
  parseBlockedKeywords,
  normalizeKeywordBlockingMode,
  normalizeModelFilter,
  normalizeModelFilterType,
  normalizeModelNames,
  normalizeDateTimeLocal,
  fileToDataURL,
  modeLabel as modeLabelHelper,
  resultLabel as resultLabelHelper,
  resultBadgeClass,
  apiKeyStatusDotClass,
  apiKeyStatusBadgeClass,
  apiKeyRowKey,
  workerSlotClass,
  workerDotClass,
} from './riskControlHelpers'

export const maxModerationTestImages = 1
export const maxModerationTestImageSize = 8 * 1024 * 1024
export const maxVisibleApiKeyRows = 3
export const blockedKeywordMax = 10000

export const riskThresholdDefaults: Record<string, number> = {
  harassment: 98,
  'harassment/threatening': 90,
  hate: 65,
  'hate/threatening': 65,
  illicit: 95,
  'illicit/violent': 95,
  'self-harm': 65,
  'self-harm/intent': 85,
  'self-harm/instructions': 65,
  sexual: 65,
  'sexual/minors': 65,
  violence: 95,
  'violence/graphic': 95,
}

export const riskThresholdCategories = Object.keys(riskThresholdDefaults)

export function useRiskControl() {
  const { t } = useI18n()

  const appStore = {
    showSuccess: (message: string) => getSdk().notify.success(message),
    showError: (message: string) => getSdk().notify.error(message),
    showInfo: (message: string) => getSdk().notify.info(message),
  }

  // --- Refs ---

  const loading = ref(true)
  const saving = ref(false)
  const logsLoading = ref(false)
  const statusLoading = ref(false)
  const apiKeyTesting = ref(false)
  const hashActionLoading = ref(false)
  const unbanningUserID = ref<number | null>(null)
  const settingsOpen = ref(false)
  const activeSettingsTab = ref<SettingsTab>('basic')
  const groupSearch = ref('')
  const flaggedHashInput = ref('')
  const groups = ref<ModerationGroup[]>([])
  const logs = ref<ContentModerationLog[]>([])
  const status = ref<ContentModerationRuntimeStatus | null>(null)
  const testedApiKeyStatuses = ref<ContentModerationAPIKeyStatus[]>([])
  const pendingDeleteApiKeyHashes = ref<string[]>([])
  const apiKeyRowsExpanded = ref(false)
  const moderationTestPrompt = ref('')
  const moderationTestImages = ref<string[]>([])
  const moderationTestResult = ref<ContentModerationTestAuditResult | null>(null)
  const inputDetailRow = ref<ContentModerationLog | null>(null)
  let statusTimer: number | null = null

  const configForm = reactive<ConfigForm>({
    enabled: false,
    mode: 'pre_block',
    base_url: 'https://api.openai.com',
    model: 'omni-moderation-latest',
    api_keys_text: '',
    api_key_configured: false,
    api_key_masked: '',
    api_key_count: 0,
    api_key_masks: [],
    api_key_statuses: [],
    api_keys_mode: 'append',
    clear_api_key: false,
    timeout_ms: 3000,
    retry_count: 2,
    sample_rate: 100,
    all_groups: true,
    group_ids: [],
    record_non_hits: false,
    worker_count: 4,
    queue_size: 32768,
    block_status: 403,
    block_message: '内容审计命中风险规则，请调整输入后重试',
    email_on_hit: true,
    auto_ban_enabled: true,
    ban_threshold: 10,
    violation_window_hours: 720,
    hit_retention_days: 180,
    non_hit_retention_days: 3,
    pre_hash_check_enabled: false,
    thresholds: { ...riskThresholdDefaults },
    blocked_keywords_text: '',
    keyword_blocking_mode: 'keyword_and_api',
    model_filter_type: 'all',
    model_filter_models: [],
  })

  const pagination = reactive<PaginationState>({
    page: 1, page_size: 20, total: 0, pages: 1,
  })

  const filters = reactive<FiltersState>({
    result: '', group_id: 0, endpoint: '', search: '', from: '', to: '',
  })

  // --- Select options ---

  const settingsTabs = computed<Array<{ id: SettingsTab; label: string }>>(() => [
    { id: 'basic', label: t('admin.riskControl.tabs.basic') },
    { id: 'scope', label: t('admin.riskControl.tabs.scope') },
    { id: 'runtime', label: t('admin.riskControl.tabs.runtime') },
    { id: 'response', label: t('admin.riskControl.tabs.response') },
    { id: 'riskThresholds', label: t('admin.riskControl.tabs.riskThresholds') },
    { id: 'keywords', label: t('admin.riskControl.tabs.keywords') },
    { id: 'retention', label: t('admin.riskControl.tabs.retention') },
  ])

  const modeOptions = computed<SelectOption[]>(() => [
    { value: 'pre_block', label: t('admin.riskControl.modePreBlock') },
    { value: 'observe', label: t('admin.riskControl.modeObserve') },
    { value: 'off', label: t('admin.riskControl.modeOff') },
  ])

  const resultOptions = computed<SelectOption[]>(() => [
    { value: '', label: t('admin.riskControl.result.all') },
    { value: 'hit', label: t('admin.riskControl.result.hit') },
    { value: 'blocked', label: t('admin.riskControl.result.blocked') },
    { value: 'pass', label: t('admin.riskControl.result.pass') },
    { value: 'error', label: t('admin.riskControl.result.error') },
  ])

  const endpointOptions = computed<SelectOption[]>(() => [
    { value: '', label: t('admin.riskControl.filters.allEndpoints') },
    { value: '/v1/messages', label: '/v1/messages' },
    { value: '/v1/responses', label: '/v1/responses' },
    { value: '/v1/chat/completions', label: '/v1/chat/completions' },
    { value: '/v1beta/models', label: '/v1beta/models' },
    { value: '/v1/images/generations', label: '/v1/images/generations' },
    { value: '/v1/images/edits', label: '/v1/images/edits' },
  ])

  const groupFilterOptions = computed<SelectOption[]>(() => [
    { value: 0, label: t('admin.riskControl.filters.allGroups') },
    ...groups.value.map((group) => ({
      value: group.id,
      label: `${group.name} (${group.platform})`,
    })),
  ])

  const keywordBlockingModeOptions = computed<Array<{ value: KeywordBlockingMode; label: string; description: string }>>(() => [
    { value: 'keyword_and_api', label: t('admin.riskControl.keywordModeKeywordAndApi'), description: t('admin.riskControl.keywordModeKeywordAndApiDesc') },
    { value: 'keyword_only', label: t('admin.riskControl.keywordModeKeywordOnly'), description: t('admin.riskControl.keywordModeKeywordOnlyDesc') },
    { value: 'api_only', label: t('admin.riskControl.keywordModeApiOnly'), description: t('admin.riskControl.keywordModeApiOnlyDesc') },
  ])

  const modelFilterOptions = computed<Array<{ value: ContentModerationModelFilterType; label: string; description: string }>>(() => [
    { value: 'all', label: t('admin.riskControl.modelFilterAll'), description: t('admin.riskControl.modelFilterAllDesc') },
    { value: 'include', label: t('admin.riskControl.modelFilterInclude'), description: t('admin.riskControl.modelFilterIncludeDesc') },
    { value: 'exclude', label: t('admin.riskControl.modelFilterExclude'), description: t('admin.riskControl.modelFilterExcludeDesc') },
  ])

  // --- Bound helpers that need modeOptions ---

  function modeLabel(mode: ModerationMode): string {
    return modeLabelHelper(mode, modeOptions.value)
  }

  function modeDescription(mode: ModerationMode): string {
    const descriptions: Record<ModerationMode, string> = {
      pre_block: t('admin.riskControl.modePreBlockDesc'),
      observe: t('admin.riskControl.modeObserveDesc'),
      off: t('admin.riskControl.modeOffDesc'),
    }
    return descriptions[mode] ?? ''
  }

  function resultLabel(row: ContentModerationLog): string {
    return resultLabelHelper(row, t)
  }

  function apiKeyStatusLabel(statusValue: ContentModerationAPIKeyStatus['status']): string {
    const labels: Record<ContentModerationAPIKeyStatus['status'], string> = {
      ok: t('admin.riskControl.apiKeyStatusOk'),
      error: t('admin.riskControl.apiKeyStatusError'),
      frozen: t('admin.riskControl.apiKeyStatusFrozen'),
      unknown: t('admin.riskControl.apiKeyStatusUnknown'),
    }
    return labels[statusValue] ?? labels.unknown
  }

  function apiKeyStatusMeta(row: ContentModerationAPIKeyStatus): string {
    const parts: string[] = []
    parts.push(t('admin.riskControl.apiKeyFailureCount', { count: row.failure_count || 0 }))
    if (row.last_latency_ms > 0) parts.push(t('admin.riskControl.apiKeyLatency', { ms: row.last_latency_ms }))
    if (row.last_http_status > 0) parts.push(t('admin.riskControl.apiKeyHTTPStatus', { status: row.last_http_status }))
    if (row.frozen_until) {
      parts.push(t('admin.riskControl.apiKeyFrozenUntil', { time: formatDateTime(row.frozen_until) }))
    } else if (row.last_checked_at) {
      parts.push(t('admin.riskControl.apiKeyLastChecked', { time: formatDateTime(row.last_checked_at) }))
    } else {
      parts.push(t('admin.riskControl.apiKeyNotTested'))
    }
    return parts.join(' / ')
  }

  function violationCountText(row: ContentModerationLog): string {
    if (!row.flagged) return '-'
    return t('admin.riskControl.violationCount', { count: row.violation_count || 1 })
  }

  function canUnbanRow(row: ContentModerationLog): boolean {
    return Boolean(row.auto_banned && row.user_id && row.user_status === 'disabled')
  }

  function inputSummaryText(row: ContentModerationLog): string {
    return row.input_excerpt || row.error || '-'
  }

  function isStoredApiKeyPendingDelete(row: ContentModerationAPIKeyStatus): boolean {
    return row.configured && row.key_hash !== '' && pendingDeleteApiKeyHashes.value.includes(row.key_hash)
  }

  // --- Computed ---

  const selectedGroupCount = computed(() => String(configForm.group_ids.length))
  const modelFilterModelCount = computed(() => configForm.model_filter_models.length)

  const modelFilterSummary = computed(() => {
    if (configForm.model_filter_type === 'include') return t('admin.riskControl.modelFilterIncludeSummary', { count: modelFilterModelCount.value })
    if (configForm.model_filter_type === 'exclude') return t('admin.riskControl.modelFilterExcludeSummary', { count: modelFilterModelCount.value })
    return t('admin.riskControl.modelFilterAllSummary')
  })

  const modelFilterPreviewModels = computed(() => configForm.model_filter_models.slice(0, 6))
  const hiddenModelFilterModelCount = computed(() => Math.max(0, configForm.model_filter_models.length - modelFilterPreviewModels.value.length))

  const filteredGroups = computed(() => {
    const keyword = groupSearch.value.trim().toLowerCase()
    if (!keyword) return groups.value
    return groups.value.filter((g) => g.name.toLowerCase().includes(keyword) || String(g.platform).toLowerCase().includes(keyword))
  })

  const inputApiKeyCount = computed(() => parseApiKeys(configForm.api_keys_text).length)
  const blockedKeywordList = computed(() => parseBlockedKeywords(configForm.blocked_keywords_text))
  const blockedKeywordCount = computed(() => blockedKeywordList.value.length)
  const pendingDeletedApiKeyCount = computed(() => pendingDeleteApiKeyHashes.value.length)
  const effectiveStoredApiKeyCount = computed(() => Math.max(0, configForm.api_key_count - pendingDeletedApiKeyCount.value))

  const apiKeysPlaceholder = computed(() =>
    configForm.api_keys_mode === 'replace' ? t('admin.riskControl.apiKeysPlaceholderReplace') : t('admin.riskControl.apiKeysPlaceholder'),
  )
  const apiKeysModeHint = computed(() =>
    configForm.api_keys_mode === 'replace' ? t('admin.riskControl.apiKeysModeReplaceHint') : t('admin.riskControl.apiKeysModeAppendHint'),
  )
  const hasModerationAuditInput = computed(() => moderationTestPrompt.value.trim() !== '' || moderationTestImages.value.length > 0)
  const isFlaggedHashInputValid = computed(() => /^[a-fA-F0-9]{64}$/.test(flaggedHashInput.value.trim()))

  const storedApiKeyTestButtonText = computed(() => {
    if (apiKeyTesting.value) return t('admin.riskControl.testingApiKeys')
    if (hasModerationAuditInput.value) return t('admin.riskControl.testContentWithStoredApiKey')
    return t('admin.riskControl.testStoredApiKeys')
  })

  const savedApiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => {
    const rows = status.value?.api_key_statuses?.length ? status.value.api_key_statuses : configForm.api_key_statuses
    return Array.isArray(rows) ? rows : []
  })

  const apiKeyRows = computed<ContentModerationAPIKeyStatus[]>(() => [...savedApiKeyRows.value, ...testedApiKeyStatuses.value])
  const visibleApiKeyRows = computed(() => apiKeyRowsExpanded.value ? apiKeyRows.value : apiKeyRows.value.slice(0, maxVisibleApiKeyRows))
  const hiddenApiKeyRowCount = computed(() => Math.max(0, apiKeyRows.value.length - visibleApiKeyRows.value.length))
  const canToggleApiKeyRows = computed(() => apiKeyRows.value.length > maxVisibleApiKeyRows)

  const activeSavedApiKeyRows = computed(() => savedApiKeyRows.value.filter((row) => !isStoredApiKeyPendingDelete(row)))

  const apiKeyHealthBadges = computed(() => {
    const counts: Record<ContentModerationAPIKeyStatus['status'], number> = { ok: 0, error: 0, frozen: 0, unknown: 0 }
    for (const row of activeSavedApiKeyRows.value) counts[row.status] = (counts[row.status] ?? 0) + 1
    if (activeSavedApiKeyRows.value.length === 0 && effectiveStoredApiKeyCount.value > 0) counts.unknown = effectiveStoredApiKeyCount.value
    return (['ok', 'frozen', 'error', 'unknown'] as Array<ContentModerationAPIKeyStatus['status']>)
      .map((s) => ({ status: s, count: counts[s] }))
      .filter((item) => item.count > 0)
  })

  const apiKeyHealthSummary = computed(() => {
    if (!configForm.api_key_configured) return ''
    if (apiKeyHealthBadges.value.length === 0) return t('admin.riskControl.apiKeyStatusUnknown')
    return apiKeyHealthBadges.value.map((b) => `${apiKeyStatusLabel(b.status)} ${b.count}`).join(' · ')
  })

  const keywordNoticeTones = {
    info: {
      icon: 'infoCircle' as const,
      toneClass: 'border-primary-100 bg-primary-50/60 dark:border-primary-900/40 dark:bg-primary-900/10',
      iconClass: 'mt-0.5 flex-shrink-0 text-primary-500 dark:text-primary-300',
      titleClass: 'text-primary-700 dark:text-primary-200',
    },
    warning: {
      icon: 'exclamationTriangle' as const,
      toneClass: 'border-amber-200 bg-amber-50 dark:border-amber-900/40 dark:bg-amber-900/20',
      iconClass: 'mt-0.5 flex-shrink-0 text-amber-500 dark:text-amber-300',
      titleClass: 'text-amber-700 dark:text-amber-200',
    },
  }

  const keywordNotice = computed<KeywordNoticeView>(() => {
    const strategy = configForm.keyword_blocking_mode
    if (strategy === 'api_only') return { ...keywordNoticeTones.info, title: t('admin.riskControl.keywordModeApiOnlyNotice'), description: t('admin.riskControl.keywordModeApiOnlyDesc') }
    if (configForm.mode !== 'pre_block') return { ...keywordNoticeTones.warning, title: t('admin.riskControl.blockedKeywordsModeWarning', { mode: modeLabel(configForm.mode) }), description: t('admin.riskControl.blockedKeywordsDescription') }
    if (strategy === 'keyword_only') return { ...keywordNoticeTones.info, title: t('admin.riskControl.keywordModeKeywordOnlyNotice'), description: t('admin.riskControl.keywordModeKeywordOnlyDesc') }
    return { ...keywordNoticeTones.info, title: t('admin.riskControl.blockedKeywordsPreBlockHint'), description: t('admin.riskControl.blockedKeywordsDescription') }
  })

  const runtimeMode = computed<ModerationMode>(() => status.value?.mode ?? configForm.mode)
  const showPreBlockRuntimeCard = computed(() => runtimeMode.value === 'pre_block')
  const showWorkerRuntimeCard = computed(() => runtimeMode.value === 'observe')

  const runtimeBadgeText = computed(() => {
    if (!status.value?.risk_control_enabled) return t('admin.riskControl.riskSwitchOff')
    if (!configForm.enabled || configForm.mode === 'off') return t('admin.riskControl.overview.disabled')
    return t('admin.riskControl.overview.enabled')
  })

  const runtimeBadgeClass = computed(() => {
    if (!status.value?.risk_control_enabled || !configForm.enabled || configForm.mode === 'off') return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
    return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-300'
  })

  const overviewItems = computed<OverviewItem[]>(() => [
    {
      key: 'status',
      label: t('admin.riskControl.overview.status'),
      value: configForm.enabled ? t('admin.riskControl.overview.enabled') : t('admin.riskControl.overview.disabled'),
      meta: modeLabel(configForm.mode),
      icon: 'shield',
      iconClass: configForm.enabled ? 'bg-emerald-50 text-emerald-600 dark:bg-emerald-900/20 dark:text-emerald-300' : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400',
      badge: runtimeBadgeText.value,
      badgeClass: runtimeBadgeClass.value,
    },
    {
      key: 'api-key',
      label: t('admin.riskControl.overview.apiKey'),
      value: configForm.api_key_configured ? t('admin.riskControl.apiKeyCount', { count: configForm.api_key_count }) : t('admin.riskControl.notConfigured'),
      meta: configForm.api_key_configured ? apiKeyHealthSummary.value || configForm.model || '-' : configForm.model || '-',
      icon: 'key',
      iconClass: 'bg-sky-50 text-sky-600 dark:bg-sky-900/20 dark:text-sky-300',
    },
    {
      key: 'scope',
      label: t('admin.riskControl.overview.groupScope'),
      value: configForm.all_groups ? t('admin.riskControl.allGroups') : selectedGroupCount.value,
      meta: modelFilterSummary.value,
      icon: 'users',
      iconClass: 'bg-violet-50 text-violet-600 dark:bg-violet-900/20 dark:text-violet-300',
    },
    {
      key: 'logs',
      label: t('admin.riskControl.overview.logs'),
      value: formatNumber(pagination.total),
      meta: t('admin.riskControl.overview.currentFilter'),
      icon: 'document',
      iconClass: 'bg-amber-50 text-amber-600 dark:bg-amber-900/20 dark:text-amber-300',
    },
  ])

  const queueUsagePercent = computed(() => `${Math.min(100, Math.max(0, status.value?.queue_usage_percent ?? 0)).toFixed(1)}%`)
  const queueUsageStyle = computed(() => ({ width: queueUsagePercent.value }))

  const preBlockMetricItems = computed(() => [
    { key: 'active', label: t('admin.riskControl.preBlockActive'), value: formatNumber(status.value?.pre_block_active ?? 0), meta: t('admin.riskControl.preBlockActiveHint'), class: 'bg-sky-50 dark:bg-sky-900/10', valueClass: 'text-sky-700 dark:text-sky-300' },
    { key: 'checked', label: t('admin.riskControl.preBlockChecked'), value: formatNumber(status.value?.pre_block_checked ?? 0), meta: t('admin.riskControl.preBlockCheckedHint'), class: 'bg-gray-50 dark:bg-dark-700/50', valueClass: 'text-gray-900 dark:text-white' },
    { key: 'allowed', label: t('admin.riskControl.preBlockAllowed'), value: formatNumber(status.value?.pre_block_allowed ?? 0), meta: t('admin.riskControl.preBlockAllowedHint'), class: 'bg-emerald-50 dark:bg-emerald-900/10', valueClass: 'text-emerald-700 dark:text-emerald-300' },
    { key: 'blocked', label: t('admin.riskControl.preBlockBlocked'), value: formatNumber(status.value?.pre_block_blocked ?? 0), meta: t('admin.riskControl.preBlockBlockedHint'), class: 'bg-rose-50 dark:bg-rose-900/10', valueClass: 'text-rose-700 dark:text-rose-300' },
    { key: 'errors', label: t('admin.riskControl.preBlockErrors'), value: formatNumber(status.value?.pre_block_errors ?? 0), meta: t('admin.riskControl.preBlockErrorsHint'), class: 'bg-amber-50 dark:bg-amber-900/10', valueClass: 'text-amber-700 dark:text-amber-300' },
    { key: 'latency', label: t('admin.riskControl.preBlockAvgLatency'), value: `${formatNumber(status.value?.pre_block_avg_latency_ms ?? 0)} ms`, meta: t('admin.riskControl.preBlockAvgLatencyHint'), class: 'bg-violet-50 dark:bg-violet-900/10', valueClass: 'text-violet-700 dark:text-violet-300' },
  ])

  const preBlockAPIKeyLoads = computed<ContentModerationAPIKeyLoad[]>(() =>
    [...(status.value?.pre_block_api_key_loads ?? [])].sort((a, b) => a.index - b.index),
  )
  const preBlockAPIKeyMaxTotal = computed(() => Math.max(1, ...preBlockAPIKeyLoads.value.map((item) => item.total || 0)))

  const preBlockAPIKeyLoadSummaryText = computed(() => t('admin.riskControl.preBlockAPIKeyLoadSummary', {
    active: formatNumber(status.value?.pre_block_api_key_active ?? 0),
    available: formatNumber(status.value?.pre_block_api_key_available_count ?? 0),
    total: formatNumber(status.value?.pre_block_api_key_total_calls ?? 0),
    workerActive: formatNumber(status.value?.active_workers ?? 0),
    workerTotal: formatNumber(status.value?.worker_count ?? configForm.worker_count),
  }))

  function preBlockAPIKeyLoadWidth(total: number): string {
    return `${Math.min(100, Math.max(0, (total / preBlockAPIKeyMaxTotal.value) * 100)).toFixed(1)}%`
  }

  const workerSlots = computed(() => {
    const total = Math.max(0, status.value?.worker_count ?? configForm.worker_count)
    const active = Math.max(0, status.value?.active_workers ?? 0)
    const enabled = Boolean(status.value?.risk_control_enabled && status.value?.enabled && status.value?.mode !== 'off')
    return Array.from({ length: total }, (_, i) => ({
      id: i + 1,
      state: (!enabled ? 'disabled' : i < active ? 'active' : 'idle') as WorkerSlotState,
      label: !enabled ? t('admin.riskControl.workerDisabled') : i < active ? t('admin.riskControl.workerActive') : t('admin.riskControl.workerIdle'),
    }))
  })

  const moderationScoreRows = computed<ModerationScoreRow[]>(() => {
    const result = moderationTestResult.value
    if (!result) return []
    return Object.entries(result.category_scores || {})
      .map(([category, score]) => ({ category, score, threshold: result.thresholds?.[category] ?? 1, hit: score >= (result.thresholds?.[category] ?? 1) }))
      .sort((a, b) => b.score - a.score)
  })

  const riskThresholdRows = computed<RiskThresholdRow[]>(() =>
    riskThresholdCategories.map((cat) => ({ category: cat, value: configForm.thresholds[cat] ?? riskThresholdDefaults[cat], defaultValue: riskThresholdDefaults[cat] })),
  )

  const inputDetailText = computed(() => {
    if (!inputDetailRow.value) return '-'
    return inputDetailRow.value.input_excerpt || inputDetailRow.value.error || '-'
  })

  // --- Internal helpers ---

  function prunePendingDeleteAPIKeyHashes() {
    const currentHashes = new Set(savedApiKeyRows.value.map((row) => row.key_hash).filter(Boolean))
    pendingDeleteApiKeyHashes.value = pendingDeleteApiKeyHashes.value.filter((hash) => currentHashes.has(hash))
  }

  function mergeConfiguredAPIKeyStatuses(items: ContentModerationAPIKeyStatus[]) {
    if (!hasModerationAuditInput.value || configForm.api_key_statuses.length === 0) {
      configForm.api_key_statuses = items
      return
    }
    const updates = new Map(items.map((item) => [item.key_hash, item]))
    configForm.api_key_statuses = configForm.api_key_statuses.map((item) => updates.get(item.key_hash) ?? item)
  }

  function riskThresholdsFromConfig(thresholds: Record<string, number> | null | undefined): Record<string, number> {
    const out: Record<string, number> = { ...riskThresholdDefaults }
    for (const cat of riskThresholdCategories) {
      const value = thresholds?.[cat]
      if (Number.isFinite(value)) out[cat] = clampPercent(Number(value) * 100)
    }
    return out
  }

  function buildRiskThresholdPayload(): Record<string, number> {
    const payload: Record<string, number> = {}
    for (const cat of riskThresholdCategories) payload[cat] = Number((clampPercent(configForm.thresholds[cat]) / 100).toFixed(4))
    return payload
  }

  function buildModelFilterPayload(): ContentModerationModelFilter {
    const type = normalizeModelFilterType(configForm.model_filter_type)
    if (type === 'all') return { type: 'all', models: [] }
    return { type, models: normalizeModelNames(configForm.model_filter_models) }
  }

  function applyConfig(config: ContentModerationConfig) {
    configForm.enabled = config.enabled
    configForm.mode = config.mode
    configForm.base_url = config.base_url || 'https://api.openai.com'
    configForm.model = config.model || 'omni-moderation-latest'
    configForm.api_keys_text = ''
    configForm.api_key_configured = config.api_key_configured
    configForm.api_key_masked = config.api_key_masked || ''
    configForm.api_key_count = config.api_key_count || 0
    configForm.api_key_masks = Array.isArray(config.api_key_masks) ? [...config.api_key_masks] : []
    configForm.api_key_statuses = Array.isArray(config.api_key_statuses) ? [...config.api_key_statuses] : []
    configForm.api_keys_mode = 'append'
    configForm.clear_api_key = false
    pendingDeleteApiKeyHashes.value = []
    testedApiKeyStatuses.value = []
    apiKeyRowsExpanded.value = false
    configForm.timeout_ms = config.timeout_ms || 3000
    configForm.retry_count = config.retry_count ?? 2
    configForm.sample_rate = config.sample_rate ?? 100
    configForm.all_groups = config.all_groups
    configForm.group_ids = Array.isArray(config.group_ids) ? [...config.group_ids] : []
    configForm.record_non_hits = config.record_non_hits
    configForm.worker_count = config.worker_count || 4
    configForm.queue_size = config.queue_size || 32768
    configForm.block_status = config.block_status || 403
    configForm.block_message = config.block_message || '内容审计命中风险规则，请调整输入后重试'
    configForm.email_on_hit = config.email_on_hit ?? true
    configForm.auto_ban_enabled = config.auto_ban_enabled ?? true
    configForm.ban_threshold = config.ban_threshold || 10
    configForm.violation_window_hours = config.violation_window_hours || 720
    configForm.hit_retention_days = config.hit_retention_days || 180
    configForm.non_hit_retention_days = Math.min(Math.max(config.non_hit_retention_days || 3, 1), 3)
    configForm.pre_hash_check_enabled = config.pre_hash_check_enabled ?? false
    configForm.thresholds = riskThresholdsFromConfig(config.thresholds)
    configForm.blocked_keywords_text = Array.isArray(config.blocked_keywords) ? config.blocked_keywords.join('\n') : ''
    configForm.keyword_blocking_mode = normalizeKeywordBlockingMode(config.keyword_blocking_mode)
    const mf = normalizeModelFilter(config.model_filter)
    configForm.model_filter_type = mf.type
    configForm.model_filter_models = mf.models
  }

  // --- Async actions ---

  async function loadAll() {
    loading.value = true
    try {
      const [config, groupItems, runtimeStatus] = await Promise.all([
        riskControlAPI.getConfig(), riskControlAPI.getGroups(), riskControlAPI.getStatus(),
      ])
      applyConfig(config)
      groups.value = groupItems
      status.value = runtimeStatus
      if (Array.isArray(runtimeStatus.api_key_statuses)) {
        configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
        prunePendingDeleteAPIKeyHashes()
      }
      await loadLogs()
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.loadFailed')))
    } finally {
      loading.value = false
    }
  }

  async function loadStatus(silent = true) {
    statusLoading.value = true
    try {
      const runtimeStatus = await riskControlAPI.getStatus()
      status.value = runtimeStatus
      if (Array.isArray(runtimeStatus.api_key_statuses)) {
        configForm.api_key_statuses = [...runtimeStatus.api_key_statuses]
        prunePendingDeleteAPIKeyHashes()
      }
    } catch (err: unknown) {
      if (!silent) appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.statusFailed')))
    } finally {
      statusLoading.value = false
    }
  }

  async function loadLogs() {
    logsLoading.value = true
    try {
      const result = await riskControlAPI.listLogs({
        page: pagination.page, page_size: pagination.page_size,
        result: filters.result || undefined, group_id: filters.group_id || undefined,
        endpoint: filters.endpoint || undefined, search: filters.search || undefined,
        from: normalizeDateTimeLocal(filters.from), to: normalizeDateTimeLocal(filters.to),
      })
      logs.value = result.items
      pagination.total = result.total
      pagination.page = result.page
      pagination.page_size = result.page_size
      pagination.pages = result.pages
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.logsFailed')))
    } finally {
      logsLoading.value = false
    }
  }

  async function saveConfig() {
    saving.value = true
    try {
      const modelFilterPayload = buildModelFilterPayload()
      if (modelFilterPayload.type !== 'all' && modelFilterPayload.models.length === 0) {
        appStore.showError(t('admin.riskControl.modelFilterModelsRequired'))
        return
      }
      const payload: UpdateContentModerationConfig = {
        enabled: configForm.enabled, mode: configForm.mode, base_url: configForm.base_url, model: configForm.model,
        timeout_ms: Number(configForm.timeout_ms) || 3000, retry_count: Number(configForm.retry_count) || 0,
        sample_rate: Number(configForm.sample_rate) || 0, all_groups: configForm.all_groups,
        group_ids: configForm.all_groups ? [] : [...configForm.group_ids], record_non_hits: configForm.record_non_hits,
        clear_api_key: configForm.clear_api_key, worker_count: Number(configForm.worker_count) || 4,
        queue_size: Number(configForm.queue_size) || 32768, block_status: Number(configForm.block_status) || 403,
        block_message: configForm.block_message || '内容审计命中风险规则，请调整输入后重试',
        email_on_hit: configForm.email_on_hit, auto_ban_enabled: configForm.auto_ban_enabled,
        ban_threshold: Number(configForm.ban_threshold) || 10, violation_window_hours: Number(configForm.violation_window_hours) || 720,
        hit_retention_days: Number(configForm.hit_retention_days) || 180,
        non_hit_retention_days: Math.min(Math.max(Number(configForm.non_hit_retention_days) || 3, 1), 3),
        pre_hash_check_enabled: configForm.pre_hash_check_enabled, thresholds: buildRiskThresholdPayload(),
        blocked_keywords: blockedKeywordList.value, keyword_blocking_mode: configForm.keyword_blocking_mode,
        model_filter: modelFilterPayload,
      }
      const keys = parseApiKeys(configForm.api_keys_text)
      if (!payload.clear_api_key && configForm.api_keys_mode === 'replace' && keys.length === 0) {
        appStore.showError(t('admin.riskControl.apiKeysReplaceNoInput'))
        return
      }
      if (keys.length > 0) { payload.api_keys = keys; payload.api_keys_mode = configForm.api_keys_mode; payload.clear_api_key = false }
      if (!payload.clear_api_key && configForm.api_keys_mode !== 'replace' && pendingDeleteApiKeyHashes.value.length > 0) {
        payload.delete_api_key_hashes = [...pendingDeleteApiKeyHashes.value]
      }
      const updated = await riskControlAPI.updateConfig(payload)
      applyConfig(updated)
      settingsOpen.value = false
      appStore.showSuccess(t('admin.riskControl.saved'))
      await Promise.all([loadStatus(true), loadLogs()])
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.saveFailed')))
    } finally {
      saving.value = false
    }
  }

  async function unbanUser(row: ContentModerationLog) {
    if (!row.user_id || unbanningUserID.value !== null) return
    unbanningUserID.value = row.user_id
    try {
      const result = await riskControlAPI.unbanUser(row.user_id)
      logs.value = logs.value.map((item) => item.user_id !== row.user_id ? item : { ...item, user_status: result.status })
      appStore.showSuccess(t('admin.riskControl.unbanSuccess'))
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.unbanFailed')))
    } finally {
      unbanningUserID.value = null
    }
  }

  async function deleteFlaggedHash() {
    if (!isFlaggedHashInputValid.value || hashActionLoading.value) return
    hashActionLoading.value = true
    try {
      const result = await riskControlAPI.deleteFlaggedHash(flaggedHashInput.value)
      flaggedHashInput.value = ''
      await loadStatus(true)
      appStore.showSuccess(result.deleted ? t('admin.riskControl.flaggedHashDeleted') : t('admin.riskControl.flaggedHashNotFound'))
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashDeleteFailed')))
    } finally {
      hashActionLoading.value = false
    }
  }

  async function clearFlaggedHashes() {
    if (hashActionLoading.value) return
    if (!window.confirm(t('admin.riskControl.clearFlaggedHashesConfirm'))) return
    hashActionLoading.value = true
    try {
      const result = await riskControlAPI.clearFlaggedHashes()
      await loadStatus(true)
      appStore.showSuccess(t('admin.riskControl.flaggedHashesCleared', { count: result.deleted }))
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.flaggedHashesClearFailed')))
    } finally {
      hashActionLoading.value = false
    }
  }

  async function testApiKeys(useInputKeys: boolean) {
    const keys = useInputKeys ? parseApiKeys(configForm.api_keys_text) : []
    if (useInputKeys && keys.length === 0) { appStore.showError(t('admin.riskControl.apiKeyTestNoInput')); return }
    apiKeyTesting.value = true
    try {
      const result = await riskControlAPI.testAPIKeys({
        api_keys: keys, base_url: configForm.base_url, model: configForm.model,
        timeout_ms: Number(configForm.timeout_ms) || 3000, prompt: moderationTestPrompt.value, images: moderationTestImages.value,
      })
      moderationTestResult.value = result.audit_result ?? null
      if (useInputKeys) {
        testedApiKeyStatuses.value = result.items.map((item) => ({ ...item, configured: false }))
      } else {
        mergeConfiguredAPIKeyStatuses(result.items)
        testedApiKeyStatuses.value = []
        await loadStatus(true)
      }
      appStore.showSuccess(t('admin.riskControl.apiKeyTestDone', { count: result.items.length }))
    } catch (err: unknown) {
      appStore.showError(extractApiErrorMessage(err, t('admin.riskControl.apiKeyTestFailed')))
    } finally {
      apiKeyTesting.value = false
    }
  }

  // --- UI actions ---

  function openSettings() { activeSettingsTab.value = 'basic'; settingsOpen.value = true }
  function reloadLogsFromFirstPage() { pagination.page = 1; void loadLogs() }
  function onPageChange(page: number) { pagination.page = page; void loadLogs() }
  function onPageSizeChange(pageSize: number) { pagination.page = 1; pagination.page_size = pageSize; void loadLogs() }

  function toggleClearApiKey() {
    configForm.clear_api_key = !configForm.clear_api_key
    if (configForm.clear_api_key) {
      configForm.api_keys_text = ''; configForm.api_keys_mode = 'append'
      testedApiKeyStatuses.value = []; pendingDeleteApiKeyHashes.value = []
    }
  }

  function setAPIKeysMode(mode: APIKeysWriteMode) {
    configForm.api_keys_mode = mode
    if (mode === 'replace') pendingDeleteApiKeyHashes.value = []
  }

  function setModelFilterType(type: ContentModerationModelFilterType) {
    configForm.model_filter_type = type
    if (type === 'all') configForm.model_filter_models = []
  }

  function toggleDeleteStoredApiKey(row: ContentModerationAPIKeyStatus) {
    if (!row.configured || !row.key_hash) return
    const idx = pendingDeleteApiKeyHashes.value.indexOf(row.key_hash)
    if (idx >= 0) { pendingDeleteApiKeyHashes.value.splice(idx, 1); return }
    pendingDeleteApiKeyHashes.value.push(row.key_hash)
  }

  function toggleGroup(groupID: number) {
    const idx = configForm.group_ids.indexOf(groupID)
    if (idx >= 0) configForm.group_ids.splice(idx, 1)
    else configForm.group_ids.push(groupID)
  }

  function isGroupSelected(groupID: number): boolean { return configForm.group_ids.includes(groupID) }
  function resetRiskThresholds() { configForm.thresholds = { ...riskThresholdDefaults } }
  function openInputDetail(row: ContentModerationLog) { inputDetailRow.value = row }
  function closeInputDetail() { inputDetailRow.value = null }

  function clearModerationTestInput() {
    moderationTestPrompt.value = ''; moderationTestImages.value = []; moderationTestResult.value = null
  }

  function removeModerationTestImage(index: number) { moderationTestImages.value.splice(index, 1) }

  async function handleModerationImageUpload(event: Event) {
    const input = event.target as HTMLInputElement
    await addModerationTestFiles(input.files); input.value = ''
  }

  async function handleModerationImageDrop(event: DragEvent) {
    await addModerationTestFiles(event.dataTransfer?.files ?? null)
  }

  async function handleModerationImagePaste(event: ClipboardEvent) {
    const files = Array.from(event.clipboardData?.files ?? []).filter((f) => f.type.startsWith('image/'))
    if (files.length === 0) return
    event.preventDefault()
    await addModerationTestFiles(files)
  }

  async function addModerationTestFiles(files: FileList | File[] | null) {
    if (!files) return
    for (const file of Array.from(files).filter((f) => f.type.startsWith('image/'))) {
      if (moderationTestImages.value.length >= maxModerationTestImages) {
        appStore.showError(t('admin.riskControl.auditTestImageLimit', { count: maxModerationTestImages })); return
      }
      if (file.size > maxModerationTestImageSize) { appStore.showError(t('admin.riskControl.auditTestImageTooLarge')); continue }
      try { moderationTestImages.value.push(await fileToDataURL(file)) } catch { appStore.showError(t('admin.riskControl.auditTestImageReadFailed')) }
    }
  }

  // --- Lifecycle ---

  onMounted(() => {
    void loadAll()
    statusTimer = window.setInterval(() => { void loadStatus(true) }, 15000)
  })

  onUnmounted(() => {
    if (statusTimer !== null) { window.clearInterval(statusTimer); statusTimer = null }
  })

  return {
    loading, saving, logsLoading, statusLoading, apiKeyTesting, hashActionLoading,
    unbanningUserID, settingsOpen, activeSettingsTab, groupSearch, flaggedHashInput,
    groups, logs, status, testedApiKeyStatuses, pendingDeleteApiKeyHashes,
    apiKeyRowsExpanded, moderationTestPrompt, moderationTestImages, moderationTestResult,
    inputDetailRow, configForm, pagination, filters,
    settingsTabs, modeOptions, resultOptions, endpointOptions, groupFilterOptions,
    keywordBlockingModeOptions, modelFilterOptions,
    selectedGroupCount, modelFilterModelCount, modelFilterSummary, modelFilterPreviewModels,
    hiddenModelFilterModelCount, filteredGroups, inputApiKeyCount, pendingDeletedApiKeyCount,
    effectiveStoredApiKeyCount, apiKeysPlaceholder, apiKeysModeHint, hasModerationAuditInput,
    isFlaggedHashInputValid, storedApiKeyTestButtonText, savedApiKeyRows, apiKeyRows,
    visibleApiKeyRows, hiddenApiKeyRowCount, canToggleApiKeyRows, apiKeyHealthBadges,
    apiKeyHealthSummary, blockedKeywordCount, keywordNotice, overviewItems,
    runtimeMode, showPreBlockRuntimeCard, showWorkerRuntimeCard, queueUsagePercent,
    queueUsageStyle, preBlockMetricItems, preBlockAPIKeyLoads, preBlockAPIKeyLoadSummaryText,
    workerSlots, moderationScoreRows, riskThresholdRows, inputDetailText,
    formatDateTime, formatNumber, percent, percentWidth, latencyText, modeLabel, modeDescription,
    preBlockAPIKeyLoadWidth, resultLabel, resultBadgeClass, workerSlotClass, workerDotClass,
    apiKeyRowKey, apiKeyStatusLabel, apiKeyStatusBadgeClass, apiKeyStatusDotClass,
    apiKeyStatusMeta, violationCountText, canUnbanRow, inputSummaryText,
    isStoredApiKeyPendingDelete, formatThresholdPercent,
    loadStatus, saveConfig, loadLogs, unbanUser, deleteFlaggedHash, clearFlaggedHashes,
    testApiKeys, openSettings, reloadLogsFromFirstPage, onPageChange, onPageSizeChange,
    toggleClearApiKey, setAPIKeysMode, setModelFilterType, toggleDeleteStoredApiKey,
    toggleGroup, isGroupSelected, resetRiskThresholds, openInputDetail, closeInputDetail,
    clearModerationTestInput, removeModerationTestImage, handleModerationImageUpload,
    handleModerationImageDrop, handleModerationImagePaste,
  }
}

export type RiskControlComposable = ReturnType<typeof useRiskControl>
