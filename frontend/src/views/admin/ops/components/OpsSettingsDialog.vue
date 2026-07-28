<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { opsAPI } from '@/api/admin/ops'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import type {
  OpsAlertRuntimeSettings,
  OpsEmailBehaviorSettings,
  AlertSeverity,
  OpsAdvancedSettings,
  OpsMetricThresholds
} from '../types'

const { t } = useI18n()
const appStore = useAppStore()

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  close: []
  saved: []
}>()

const loading = ref(false)
const saving = ref(false)
const scheduleInfo = ref({
  timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'Local',
  next_runs: {} as Record<string, string>
})

// 运行时设置
const runtimeSettings = ref<OpsAlertRuntimeSettings | null>(null)
// 运维内部邮件行为；总开关和收件人在全局邮件设置中管理
const emailBehavior = ref<OpsEmailBehaviorSettings | null>(null)
// 高级设置
const advancedSettings = ref<OpsAdvancedSettings | null>(null)
// 指标阈值配置
const metricThresholds = ref<OpsMetricThresholds>({
  sla_percent_min: 99.5,
  ttft_p99_ms_max: 500,
  request_error_rate_percent_max: 5,
  upstream_error_rate_percent_max: 5
})

// 加载所有配置
async function loadAllSettings() {
  loading.value = true
  try {
    const settings = await opsAPI.getMonitoringSettings()
    runtimeSettings.value = settings.runtime
    emailBehavior.value = settings.email_behavior
    advancedSettings.value = settings.advanced
    scheduleInfo.value = settings.schedule_info || scheduleInfo.value
    // 兼容旧 payload：后端未返回该字段时补默认值，保证表单可绑定
    if (advancedSettings.value && !advancedSettings.value.openai_account_quota_auto_pause) {
      advancedSettings.value.openai_account_quota_auto_pause = { default_threshold_5h: 0, default_threshold_7d: 0 }
    }
    // 如果后端返回了阈值，使用后端的值；否则保持默认值
    const thresholds = settings.metric_thresholds
    if (thresholds && Object.keys(thresholds).length > 0) {
        metricThresholds.value = {
          sla_percent_min: thresholds.sla_percent_min ?? 99.5,
          ttft_p99_ms_max: thresholds.ttft_p99_ms_max ?? 500,
          request_error_rate_percent_max: thresholds.request_error_rate_percent_max ?? 5,
          upstream_error_rate_percent_max: thresholds.upstream_error_rate_percent_max ?? 5
        }
    }
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to load settings', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.settings.loadFailed'))
  } finally {
    loading.value = false
  }
}

// 监听弹窗打开
watch(() => props.show, (show) => {
  if (show) {
    loadAllSettings()
  }
})

// 严重级别选项
const severityOptions: Array<{ value: AlertSeverity | ''; label: string }> = [
  { value: '', label: t('admin.ops.email.minSeverityAll') },
  { value: 'critical', label: t('common.critical') },
  { value: 'warning', label: t('common.warning') },
  { value: 'info', label: t('common.info') }
]

const weekdayOptions = computed(() => [
  { value: 1, label: t('admin.ops.settings.weekdays.monday') },
  { value: 2, label: t('admin.ops.settings.weekdays.tuesday') },
  { value: 3, label: t('admin.ops.settings.weekdays.wednesday') },
  { value: 4, label: t('admin.ops.settings.weekdays.thursday') },
  { value: 5, label: t('admin.ops.settings.weekdays.friday') },
  { value: 6, label: t('admin.ops.settings.weekdays.saturday') },
  { value: 0, label: t('admin.ops.settings.weekdays.sunday') }
])

function cronTime(spec: string, fallback = '09:00'): string {
  const parts = spec.trim().split(/\s+/)
  if (parts.length !== 5 || !/^\d+$/.test(parts[0]) || !/^\d+$/.test(parts[1])) return fallback
  const minute = Number(parts[0])
  const hour = Number(parts[1])
  if (minute < 0 || minute > 59 || hour < 0 || hour > 23) return fallback
  return `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
}

function dailyCron(time: string): string {
  const match = /^(\d{2}):(\d{2})$/.exec(time)
  if (!match) return '0 9 * * *'
  return `${Number(match[2])} ${Number(match[1])} * * *`
}

function weeklyCron(time: string, weekday: number): string {
  return `${dailyCron(time).replace(/ \*$/, '')} ${weekday}`
}

function cronWeekday(spec: string): number {
  const parts = spec.trim().split(/\s+/)
  const day = Number(parts[4])
  return Number.isInteger(day) && day >= 0 && day <= 6 ? day : 1
}

function isProductDailySchedule(spec: string): boolean {
  const parts = spec.trim().split(/\s+/)
  return parts.length === 5 &&
    /^\d+$/.test(parts[0]) &&
    /^\d+$/.test(parts[1]) &&
    parts[2] === '*' &&
    parts[3] === '*' &&
    parts[4] === '*'
}

function isProductWeeklySchedule(spec: string): boolean {
  const parts = spec.trim().split(/\s+/)
  return parts.length === 5 &&
    /^\d+$/.test(parts[0]) &&
    /^\d+$/.test(parts[1]) &&
    parts[2] === '*' &&
    parts[3] === '*' &&
    /^[0-6]$/.test(parts[4])
}

type ReportScheduleField =
  | 'daily_summary_schedule'
  | 'error_digest_schedule'
  | 'account_health_schedule'

function reportTime(field: ReportScheduleField) {
  return computed<string>({
    get: () => emailBehavior.value ? cronTime(String(emailBehavior.value.report[field] || '')) : '09:00',
    set: (value) => {
      if (!emailBehavior.value) return
      emailBehavior.value.report[field] = dailyCron(value)
    }
  })
}

const dailySummaryTime = reportTime('daily_summary_schedule')
const errorDigestTime = reportTime('error_digest_schedule')
const accountHealthTime = reportTime('account_health_schedule')
const weeklySummaryTime = computed<string>({
  get: () => emailBehavior.value ? cronTime(emailBehavior.value.report.weekly_summary_schedule) : '09:00',
  set: (value) => {
    if (!emailBehavior.value) return
    emailBehavior.value.report.weekly_summary_schedule = weeklyCron(
      value,
      cronWeekday(emailBehavior.value.report.weekly_summary_schedule)
    )
  }
})
const weeklySummaryWeekday = computed<number>({
  get: () => emailBehavior.value ? cronWeekday(emailBehavior.value.report.weekly_summary_schedule) : 1,
  set: (value) => {
    if (!emailBehavior.value) return
    emailBehavior.value.report.weekly_summary_schedule = weeklyCron(
      cronTime(emailBehavior.value.report.weekly_summary_schedule),
      Number(value)
    )
  }
})
const dailySummaryHasLegacySchedule = computed(() =>
  !!emailBehavior.value && !isProductDailySchedule(emailBehavior.value.report.daily_summary_schedule)
)
const weeklySummaryHasLegacySchedule = computed(() =>
  !!emailBehavior.value && !isProductWeeklySchedule(emailBehavior.value.report.weekly_summary_schedule)
)
const errorDigestHasLegacySchedule = computed(() =>
  !!emailBehavior.value && !isProductDailySchedule(emailBehavior.value.report.error_digest_schedule)
)
const accountHealthHasLegacySchedule = computed(() =>
  !!emailBehavior.value && !isProductDailySchedule(emailBehavior.value.report.account_health_schedule)
)

function nextRunLabel(kind: string): string {
  const raw = scheduleInfo.value.next_runs?.[kind]
  if (!raw) return t('admin.ops.settings.nextRunAfterSave')
  try {
    return new Intl.DateTimeFormat(undefined, {
      timeZone: scheduleInfo.value.timezone,
      dateStyle: 'medium',
      timeStyle: 'short'
    }).format(new Date(raw))
  } catch {
    return raw
  }
}

// OpenAI 账号配额自动暂停：后端按 0~1 分数存储，UI 按百分比(0~100)展示
const quotaAutoPause5hPercent = computed<number | null>({
  get() {
    const v = advancedSettings.value?.openai_account_quota_auto_pause?.default_threshold_5h
    return v && v > 0 ? Math.round(v * 1000) / 10 : null
  },
  set(val) {
    if (!advancedSettings.value?.openai_account_quota_auto_pause) return
    advancedSettings.value.openai_account_quota_auto_pause.default_threshold_5h = val != null && val > 0 ? val / 100 : 0
  }
})
const quotaAutoPause7dPercent = computed<number | null>({
  get() {
    const v = advancedSettings.value?.openai_account_quota_auto_pause?.default_threshold_7d
    return v && v > 0 ? Math.round(v * 1000) / 10 : null
  },
  set(val) {
    if (!advancedSettings.value?.openai_account_quota_auto_pause) return
    advancedSettings.value.openai_account_quota_auto_pause.default_threshold_7d = val != null && val > 0 ? val / 100 : 0
  }
})

// 验证
const validation = computed(() => {
  const errors: string[] = []

  // 验证运行时设置
  if (runtimeSettings.value) {
    const evalSeconds = runtimeSettings.value.evaluation_interval_seconds
    if (!Number.isFinite(evalSeconds) || evalSeconds < 1 || evalSeconds > 86400) {
      errors.push(t('admin.ops.runtime.validation.evalIntervalRange'))
    }
  }

  // 运维页只校验告警/报告的细粒度行为；总开关和收件人由邮件设置负责。
  if (emailBehavior.value) {
    const alert = emailBehavior.value.alert
    if (
      !Number.isFinite(alert.rate_limit_per_hour) ||
      alert.rate_limit_per_hour < 0 ||
      alert.rate_limit_per_hour > 10000
    ) {
      errors.push(t('admin.ops.email.validation.rateLimitRange'))
    }
    if (
      !Number.isFinite(alert.batching_window_seconds) ||
      alert.batching_window_seconds < 0 ||
      alert.batching_window_seconds > 86400
    ) {
      errors.push(t('admin.ops.email.validation.batchWindowRange'))
    }

    const report = emailBehavior.value.report
    if (!Number.isFinite(report.error_digest_min_count) || report.error_digest_min_count < 0) {
      errors.push(t('admin.ops.email.validation.digestMinCountRange'))
    }
    if (
      !Number.isFinite(report.account_health_error_rate_threshold) ||
      report.account_health_error_rate_threshold < 0 ||
      report.account_health_error_rate_threshold > 100
    ) {
      errors.push(t('admin.ops.email.validation.accountHealthThresholdRange'))
    }
  }

  // 验证高级设置
  if (advancedSettings.value) {
    const { error_log_retention_days, minute_metrics_retention_days, hourly_metrics_retention_days } = advancedSettings.value.data_retention
    if (error_log_retention_days < 0 || error_log_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (minute_metrics_retention_days < 0 || minute_metrics_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }
    if (hourly_metrics_retention_days < 0 || hourly_metrics_retention_days > 365) {
      errors.push(t('admin.ops.settings.validation.retentionDaysRange'))
    }

    const { default_threshold_5h, default_threshold_7d } = advancedSettings.value.openai_account_quota_auto_pause
    if (default_threshold_5h < 0 || default_threshold_5h > 1 || default_threshold_7d < 0 || default_threshold_7d > 1) {
      errors.push(t('admin.ops.settings.validation.openaiQuotaAutoPauseRange'))
    }
  }

  // 验证指标阈值
  if (metricThresholds.value.sla_percent_min != null && (metricThresholds.value.sla_percent_min <= 0 || metricThresholds.value.sla_percent_min > 100)) {
    errors.push(t('admin.ops.settings.validation.slaMinPercentRange'))
  }
  if (metricThresholds.value.ttft_p99_ms_max != null && metricThresholds.value.ttft_p99_ms_max <= 0) {
    errors.push(t('admin.ops.settings.validation.ttftP99MaxRange'))
  }
  if (metricThresholds.value.request_error_rate_percent_max != null && (metricThresholds.value.request_error_rate_percent_max <= 0 || metricThresholds.value.request_error_rate_percent_max > 100)) {
    errors.push(t('admin.ops.settings.validation.requestErrorRateMaxRange'))
  }
  if (metricThresholds.value.upstream_error_rate_percent_max != null && (metricThresholds.value.upstream_error_rate_percent_max <= 0 || metricThresholds.value.upstream_error_rate_percent_max > 100)) {
    errors.push(t('admin.ops.settings.validation.upstreamErrorRateMaxRange'))
  }

  return { valid: errors.length === 0, errors }
})

// 保存所有配置
async function saveAllSettings() {
  if (!validation.value.valid) {
    appStore.showError(validation.value.errors[0])
    return
  }

  saving.value = true
  try {
    if (!runtimeSettings.value || !emailBehavior.value || !advancedSettings.value) return
    const updated = await opsAPI.updateMonitoringSettings({
      runtime: runtimeSettings.value,
      email_behavior: emailBehavior.value,
      advanced: advancedSettings.value,
      metric_thresholds: metricThresholds.value
    })
    runtimeSettings.value = updated.runtime
    emailBehavior.value = updated.email_behavior
    advancedSettings.value = updated.advanced
    metricThresholds.value = updated.metric_thresholds
    scheduleInfo.value = updated.schedule_info || scheduleInfo.value
    appStore.showSuccess(t('admin.ops.settings.saveSuccess'))
    emit('saved')
    emit('close')
  } catch (err: any) {
    console.error('[OpsSettingsDialog] Failed to save settings', err)
    appStore.showError(err?.response?.data?.message || err?.response?.data?.detail || t('admin.ops.settings.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <BaseDialog :show="show" :title="t('admin.ops.settings.title')" width="extra-wide" @close="emit('close')">
    <div v-if="loading" class="py-10 text-center text-sm text-gray-500">
      {{ t('common.loading') }}
    </div>

    <div v-else-if="runtimeSettings && emailBehavior && advancedSettings" class="space-y-6">
      <!-- 验证错误 -->
      <div v-if="!validation.valid" class="rounded-lg border border-amber-200 bg-amber-50 p-3 text-xs text-amber-800 dark:border-amber-900/50 dark:bg-amber-900/20 dark:text-amber-200">
        <div class="font-bold">{{ t('admin.ops.settings.validation.title') }}</div>
        <ul class="mt-1 list-disc space-y-1 pl-4">
          <li v-for="msg in validation.errors" :key="msg">{{ msg }}</li>
        </ul>
      </div>

      <!-- 数据采集频率 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.dataCollection') }}</h4>
        <div>
          <label class="input-label">{{ t('admin.ops.settings.evaluationInterval') }}</label>
          <input
            v-model.number="runtimeSettings.evaluation_interval_seconds"
            type="number"
            min="1"
            max="86400"
            class="input"
          />
          <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.evaluationIntervalHint') }}</p>
        </div>
      </div>

      <!-- 告警细粒度行为；通道总开关和收件人在邮件设置中管理 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.alertConfig') }}</h4>
        <p class="mb-4 rounded-lg bg-blue-50 px-3 py-2 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
          {{ t('admin.ops.settings.emailPolicyOwnershipHint') }}
        </p>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div>
            <label class="input-label">{{ t('admin.ops.settings.minSeverity') }}</label>
            <Select v-model="emailBehavior.alert.min_severity" :options="severityOptions" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.minSeverityHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.ops.email.rateLimitPerHour') }}</label>
            <input v-model.number="emailBehavior.alert.rate_limit_per_hour" type="number" min="0" max="10000" class="input" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.rateLimitHint') }}</p>
          </div>
          <div>
            <label class="input-label">{{ t('admin.ops.email.batchWindowSeconds') }}</label>
            <input v-model.number="emailBehavior.alert.batching_window_seconds" type="number" min="0" max="86400" class="input" />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.batchWindowHint') }}</p>
          </div>
          <div class="flex items-start justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600">
            <div>
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.email.includeResolved') }}</label>
              <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.includeResolvedHint') }}</p>
            </div>
            <Toggle v-model="emailBehavior.alert.include_resolved_alerts" />
          </div>
        </div>
      </div>

      <!-- 运维报告细粒度计划 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.reportConfig') }}</h4>
        <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.reportBehaviorHint') }}</p>
        <div
          class="mb-4 rounded-lg bg-slate-100 px-3 py-2 text-xs text-slate-600 dark:bg-dark-600 dark:text-slate-300"
          data-testid="ops-report-timezone"
          :data-timezone="scheduleInfo.timezone"
        >
          {{ t('admin.ops.settings.serverTimezone', { timezone: scheduleInfo.timezone }) }}
        </div>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.email.dailySummary') }}</label>
              <Toggle v-model="emailBehavior.report.daily_summary_enabled" />
            </div>
            <div v-if="emailBehavior.report.daily_summary_enabled" class="mt-3">
              <label class="input-label">{{ t('admin.ops.settings.sendTime') }}</label>
              <input v-model="dailySummaryTime" type="time" class="input" />
              <p v-if="dailySummaryHasLegacySchedule" class="mt-2 text-xs text-amber-600 dark:text-amber-400">
                {{ t('admin.ops.settings.legacyScheduleHint') }}
              </p>
              <p class="mt-2 text-xs text-gray-500">{{ t('admin.ops.settings.nextRun', { time: nextRunLabel('daily_summary') }) }}</p>
            </div>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.email.weeklySummary') }}</label>
              <Toggle v-model="emailBehavior.report.weekly_summary_enabled" />
            </div>
            <div v-if="emailBehavior.report.weekly_summary_enabled" class="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.weekday') }}</label>
                <Select v-model="weeklySummaryWeekday" :options="weekdayOptions" />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.sendTime') }}</label>
                <input v-model="weeklySummaryTime" type="time" class="input" />
              </div>
              <p v-if="weeklySummaryHasLegacySchedule" class="text-xs text-amber-600 dark:text-amber-400 sm:col-span-2">
                {{ t('admin.ops.settings.legacyScheduleHint') }}
              </p>
              <p class="text-xs text-gray-500 sm:col-span-2">{{ t('admin.ops.settings.nextRun', { time: nextRunLabel('weekly_summary') }) }}</p>
            </div>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.email.errorDigest') }}</label>
              <Toggle v-model="emailBehavior.report.error_digest_enabled" />
            </div>
            <div v-if="emailBehavior.report.error_digest_enabled" class="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.sendTime') }}</label>
                <input v-model="errorDigestTime" type="time" class="input" />
                <p v-if="errorDigestHasLegacySchedule" class="mt-2 text-xs text-amber-600 dark:text-amber-400">
                  {{ t('admin.ops.settings.legacyScheduleHint') }}
                </p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.email.errorDigestMinCount') }}</label>
                <input v-model.number="emailBehavior.report.error_digest_min_count" type="number" min="0" class="input" />
              </div>
              <p class="text-xs text-gray-500 sm:col-span-2">{{ t('admin.ops.settings.nextRun', { time: nextRunLabel('error_digest') }) }}</p>
            </div>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.email.accountHealth') }}</label>
              <Toggle v-model="emailBehavior.report.account_health_enabled" />
            </div>
            <div v-if="emailBehavior.report.account_health_enabled" class="mt-3 grid gap-3 sm:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.sendTime') }}</label>
                <input v-model="accountHealthTime" type="time" class="input" />
                <p v-if="accountHealthHasLegacySchedule" class="mt-2 text-xs text-amber-600 dark:text-amber-400">
                  {{ t('admin.ops.settings.legacyScheduleHint') }}
                </p>
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.email.accountHealthThreshold') }}</label>
                <input
                  v-model.number="emailBehavior.report.account_health_error_rate_threshold"
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  class="input"
                />
              </div>
              <p class="text-xs text-gray-500 sm:col-span-2">{{ t('admin.ops.settings.nextRun', { time: nextRunLabel('account_health') }) }}</p>
            </div>
          </div>
        </div>
      </div>

      <!-- 指标阈值配置 -->
      <div class="rounded-2xl bg-gray-50 p-4 dark:bg-dark-700/50">
        <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.settings.metricThresholds') }}</h4>
        <p class="mb-4 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.ops.settings.metricThresholdsHint') }}</p>

        <div class="space-y-4">
          <div>
            <label class="input-label">{{ t('admin.ops.settings.slaMinPercent') }}</label>
            <input
              v-model.number="metricThresholds.sla_percent_min"
              type="number"
              min="0.1"
              max="100"
              step="0.1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.slaMinPercentHint') }}</p>
          </div>


          <div>
            <label class="input-label">{{ t('admin.ops.settings.ttftP99MaxMs') }}</label>
            <input
              v-model.number="metricThresholds.ttft_p99_ms_max"
              type="number"
              min="1"
              step="50"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.ttftP99MaxMsHint') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.settings.requestErrorRateMaxPercent') }}</label>
            <input
              v-model.number="metricThresholds.request_error_rate_percent_max"
              type="number"
              min="0.1"
              max="100"
              step="0.1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.requestErrorRateMaxPercentHint') }}</p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.settings.upstreamErrorRateMaxPercent') }}</label>
            <input
              v-model.number="metricThresholds.upstream_error_rate_percent_max"
              type="number"
              min="0.1"
              max="100"
              step="0.1"
              class="input"
            />
            <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.upstreamErrorRateMaxPercentHint') }}</p>
          </div>
        </div>
      </div>

      <!-- 高级设置 -->
      <details class="rounded-2xl bg-gray-50 dark:bg-dark-700/50">
        <summary class="cursor-pointer p-4 text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.ops.settings.advancedSettings') }}
        </summary>
        <div class="space-y-4 px-4 pb-4">
          <!-- 数据保留策略 -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.dataRetention') }}</h5>

            <div class="flex items-center justify-between">
              <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.enableCleanup') }}</label>
              <Toggle v-model="advancedSettings.data_retention.cleanup_enabled" />
            </div>

            <div v-if="advancedSettings.data_retention.cleanup_enabled">
              <label class="input-label">{{ t('admin.ops.settings.cleanupSchedule') }}</label>
              <input
                v-model="advancedSettings.data_retention.cleanup_schedule"
                type="text"
                class="input"
                placeholder="0 2 * * *"
              />
              <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.cleanupScheduleHint') }}</p>
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-3">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.errorLogRetentionDays') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.error_log_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.minuteMetricsRetentionDays') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.minute_metrics_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.hourlyMetricsRetentionDays') }}</label>
                <input
                  v-model.number="advancedSettings.data_retention.hourly_metrics_retention_days"
                  type="number"
                  min="0"
                  max="365"
                  class="input"
                />
              </div>
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.ops.settings.retentionDaysHint') }}</p>
          </div>

          <!-- 预聚合任务 -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.aggregation') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.enableAggregation') }}</label>
                <p class="mt-1 text-xs text-gray-500">{{ t('admin.ops.settings.aggregationHint') }}</p>
              </div>
              <Toggle v-model="advancedSettings.aggregation.aggregation_enabled" />
            </div>
          </div>

          <!-- OpenAI 账号配额自动暂停（全局默认阈值） -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.openaiQuotaAutoPause') }}</h5>
            <p class="text-xs text-gray-500">{{ t('admin.ops.settings.openaiQuotaAutoPauseHint') }}</p>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="input-label">{{ t('admin.ops.settings.openaiQuotaAutoPauseDefault5h') }}</label>
                <input
                  v-model.number="quotaAutoPause5hPercent"
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  class="input"
                  data-testid="ops-quota-auto-pause-5h"
                />
              </div>
              <div>
                <label class="input-label">{{ t('admin.ops.settings.openaiQuotaAutoPauseDefault7d') }}</label>
                <input
                  v-model.number="quotaAutoPause7dPercent"
                  type="number"
                  min="0"
                  max="100"
                  step="0.1"
                  class="input"
                  data-testid="ops-quota-auto-pause-7d"
                />
              </div>
            </div>
            <p class="text-xs text-gray-500">{{ t('admin.ops.settings.openaiQuotaAutoPauseThresholdHint') }}</p>
          </div>

          <!-- Error Filtering -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.errorFiltering') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreCountTokensErrors') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreCountTokensErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_count_tokens_errors" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreContextCanceled') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreContextCanceledHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_context_canceled" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreNoAvailableAccounts') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreNoAvailableAccountsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_no_available_accounts" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.ignoreInsufficientBalanceErrors') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.ignoreInsufficientBalanceErrorsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.ignore_insufficient_balance_errors" />
            </div>
          </div>

          <!-- Auto Refresh -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.autoRefresh') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.enableAutoRefresh') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.enableAutoRefreshHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.auto_refresh_enabled" />
            </div>

            <div v-if="advancedSettings.auto_refresh_enabled">
              <label class="input-label">{{ t('admin.ops.settings.refreshInterval') }}</label>
              <Select
                v-model="advancedSettings.auto_refresh_interval_seconds"
                :options="[
                  { value: 15, label: t('admin.ops.settings.refreshInterval15s') },
                  { value: 30, label: t('admin.ops.settings.refreshInterval30s') },
                  { value: 60, label: t('admin.ops.settings.refreshInterval60s') }
                ]"
              />
            </div>
          </div>

          <!-- Dashboard Cards -->
          <div class="space-y-3">
            <h5 class="text-xs font-semibold text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.dashboardCards') }}</h5>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.displayAlertEvents') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.displayAlertEventsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_alert_events" />
            </div>

            <div class="flex items-center justify-between">
              <div>
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.ops.settings.displayOpenAITokenStats') }}</label>
                <p class="mt-1 text-xs text-gray-500">
                  {{ t('admin.ops.settings.displayOpenAITokenStatsHint') }}
                </p>
              </div>
              <Toggle v-model="advancedSettings.display_openai_token_stats" />
            </div>
          </div>
        </div>
      </details>
    </div>

    <template #footer>
      <div class="flex justify-end gap-2">
        <button class="btn btn-secondary" @click="emit('close')">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="saving || !validation.valid" @click="saveAllSettings">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>
