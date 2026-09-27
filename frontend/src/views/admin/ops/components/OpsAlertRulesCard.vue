<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import DataTable from '@/components/common/DataTable.vue'
import type { Column } from '@/components/common/types'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import { adminAPI } from '@/api'
import { opsAPI } from '@/api/admin/ops'
import type { AlertRule, MetricType, Operator } from '../types'
import type { EmailNotificationConfig, OpsSeverity } from '@/api/admin/ops'
import { formatDateTime } from '../utils/opsFormatters'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const rules = ref<AlertRule[]>([])

async function load() {
  loading.value = true
  try {
    rules.value = await opsAPI.listAlertRules()
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to load rules', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.loadFailed'))
    rules.value = []
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  load()
  loadGroups()
})

const sortedRules = computed(() => {
  return [...rules.value].sort((a, b) => (b.id || 0) - (a.id || 0))
})

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.ops.alertRules.table.name') },
  { key: 'metric_type', label: t('admin.ops.alertRules.table.metric') },
  { key: 'severity', label: t('admin.ops.alertRules.table.severity'), class: 'font-bold' },
  { key: 'enabled', label: t('admin.ops.alertRules.table.enabled') },
  { key: 'actions', label: t('admin.ops.alertRules.table.actions'), class: 'text-right' }
])

const showEditor = ref(false)
const saving = ref(false)
const editingId = ref<number | null>(null)
const draft = ref<AlertRule | null>(null)

type MetricGroup = 'system' | 'group' | 'account'

interface MetricDefinition {
  type: MetricType
  group: MetricGroup
  label: string
  description: string
  recommendedOperator: Operator
  recommendedThreshold: number
  unit?: string
}

const groupMetricTypes = new Set<MetricType>([
  'group_available_accounts',
  'group_available_ratio',
  'group_rate_limit_ratio'
])

function parsePositiveInt(value: unknown): number | null {
  if (value == null) return null
  if (typeof value === 'boolean') return null
  const n = typeof value === 'number' ? value : Number.parseInt(String(value), 10)
  return Number.isFinite(n) && n > 0 ? n : null
}

const groupOptionsBase = ref<SelectOption[]>([])

async function loadGroups() {
  try {
    const list = await adminAPI.groups.getAll()
    groupOptionsBase.value = list.map((g) => ({ value: g.id, label: g.name }))
  } catch (err) {
    console.error('[OpsAlertRulesCard] Failed to load groups', err)
    groupOptionsBase.value = []
  }
}

const isGroupMetricSelected = computed(() => {
  const metricType = draft.value?.metric_type
  return metricType ? groupMetricTypes.has(metricType) : false
})

const draftGroupId = computed<number | null>({
  get() {
    return parsePositiveInt(draft.value?.filters?.group_id)
  },
  set(value) {
    if (!draft.value) return
    if (value == null) {
      if (!draft.value.filters) return
      delete draft.value.filters.group_id
      if (Object.keys(draft.value.filters).length === 0) {
        delete draft.value.filters
      }
      return
    }
    if (!draft.value.filters) draft.value.filters = {}
    draft.value.filters.group_id = value
  }
})

const groupOptions = computed<SelectOption[]>(() => {
  if (isGroupMetricSelected.value) return groupOptionsBase.value
  return [{ value: null, label: t('admin.ops.alertRules.form.allGroups') }, ...groupOptionsBase.value]
})

const metricDefinitions = computed(() => {
  return [
    // System-level metrics
    {
      type: 'success_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.successRate'),
      description: t('admin.ops.alertRules.metricDescriptions.successRate'),
      recommendedOperator: '<',
      recommendedThreshold: 99,
      unit: '%'
    },
    {
      type: 'error_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.errorRate'),
      description: t('admin.ops.alertRules.metricDescriptions.errorRate'),
      recommendedOperator: '>',
      recommendedThreshold: 1,
      unit: '%'
    },
    {
      type: 'upstream_error_rate',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.upstreamErrorRate'),
      description: t('admin.ops.alertRules.metricDescriptions.upstreamErrorRate'),
      recommendedOperator: '>',
      recommendedThreshold: 1,
      unit: '%'
    },
    {
      type: 'cpu_usage_percent',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.cpu'),
      description: t('admin.ops.alertRules.metricDescriptions.cpu'),
      recommendedOperator: '>',
      recommendedThreshold: 80,
      unit: '%'
    },
    {
      type: 'memory_usage_percent',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.memory'),
      description: t('admin.ops.alertRules.metricDescriptions.memory'),
      recommendedOperator: '>',
      recommendedThreshold: 80,
      unit: '%'
    },
    {
      type: 'concurrency_queue_depth',
      group: 'system',
      label: t('admin.ops.alertRules.metrics.queueDepth'),
      description: t('admin.ops.alertRules.metricDescriptions.queueDepth'),
      recommendedOperator: '>',
      recommendedThreshold: 10
    },

    // Group-level metrics (requires group_id filter)
    {
      type: 'group_available_accounts',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupAvailableAccounts'),
      description: t('admin.ops.alertRules.metricDescriptions.groupAvailableAccounts'),
      recommendedOperator: '<',
      recommendedThreshold: 1
    },
    {
      type: 'group_available_ratio',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupAvailableRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.groupAvailableRatio'),
      recommendedOperator: '<',
      recommendedThreshold: 50,
      unit: '%'
    },
    {
      type: 'group_rate_limit_ratio',
      group: 'group',
      label: t('admin.ops.alertRules.metrics.groupRateLimitRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.groupRateLimitRatio'),
      recommendedOperator: '>',
      recommendedThreshold: 10,
      unit: '%'
    },

    // Account-level metrics
    {
      type: 'account_rate_limited_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountRateLimitedCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountRateLimitedCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'account_error_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountErrorCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountErrorCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'account_error_ratio',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountErrorRatio'),
      description: t('admin.ops.alertRules.metricDescriptions.accountErrorRatio'),
      recommendedOperator: '>',
      recommendedThreshold: 5,
      unit: '%'
    },
    {
      type: 'account_temp_unscheduled_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.accountTempUnscheduledCount'),
      description: t('admin.ops.alertRules.metricDescriptions.accountTempUnscheduledCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    },
    {
      type: 'overload_account_count',
      group: 'account',
      label: t('admin.ops.alertRules.metrics.overloadAccountCount'),
      description: t('admin.ops.alertRules.metricDescriptions.overloadAccountCount'),
      recommendedOperator: '>',
      recommendedThreshold: 0
    }
  ] satisfies MetricDefinition[]
})

const selectedMetricDefinition = computed(() => {
  const metricType = draft.value?.metric_type
  if (!metricType) return null
  return metricDefinitions.value.find((m) => m.type === metricType) ?? null
})

const metricOptions = computed(() => {
  const buildGroup = (group: MetricGroup): SelectOption[] => {
    const items = metricDefinitions.value.filter((m) => m.group === group)
    if (items.length === 0) return []
    const headerValue = `__group__${group}`
    return [
      {
        value: headerValue,
        label: t(`admin.ops.alertRules.metricGroups.${group}`),
        disabled: true,
        kind: 'group'
      },
      ...items.map((m) => ({ value: m.type, label: m.label }))
    ]
  }

  return [...buildGroup('system'), ...buildGroup('group'), ...buildGroup('account')]
})

const operatorOptions = computed(() => {
  const ops: Operator[] = ['>', '>=', '<', '<=', '==', '!=']
  return ops.map((o) => ({ value: o, label: o }))
})

const severityOptions = computed(() => {
  const sev: OpsSeverity[] = ['P0', 'P1', 'P2', 'P3']
  return sev.map((s) => ({ value: s, label: s }))
})

const windowOptions = computed(() => {
  const windows = [1, 5, 60]
  return windows.map((m) => ({ value: m, label: `${m}m` }))
})

function newRuleDraft(): AlertRule {
  return {
    name: '',
    description: '',
    enabled: true,
    metric_type: 'error_rate',
    operator: '>',
    threshold: 1,
    window_minutes: 1,
    sustained_minutes: 2,
    severity: 'P1',
    cooldown_minutes: 10,
    notify_email: true
  }
}

// The evaluator silently skips email when alert emails are off, have no recipients, or the rule is
// below alert.min_severity; warn in the editor.
const emailConfig = ref<EmailNotificationConfig | null>(null)
const EMAIL_SEVERITY_RANK: Record<string, number> = { critical: 3, warning: 2, info: 1 }
const emailAlertsInactive = computed(() => {
  const alert = emailConfig.value?.alert
  if (!alert) return false
  if (!alert.enabled || !alert.recipients?.some((r) => r.trim())) return true
  // Mirrors backend shouldSendOpsAlertEmailByMinSeverity: P0 = critical, P1 = warning, others = info.
  const minRank = EMAIL_SEVERITY_RANK[(alert.min_severity || '').trim().toLowerCase()] ?? 0
  const severity = String(draft.value?.severity ?? '').trim().toUpperCase()
  const ruleRank = severity === 'P0' ? 3 : severity === 'P1' ? 2 : 1
  return ruleRank < minRank
})

async function loadEmailConfig() {
  try {
    emailConfig.value = await opsAPI.getEmailNotificationConfig()
  } catch {
    emailConfig.value = null // best-effort hint only
  }
}

function openCreate() {
  editingId.value = null
  draft.value = newRuleDraft()
  showEditor.value = true
  void loadEmailConfig()
}

function openEdit(rule: AlertRule) {
  editingId.value = rule.id ?? null
  draft.value = JSON.parse(JSON.stringify(rule))
  showEditor.value = true
  void loadEmailConfig()
}

const editorValidation = computed(() => {
  const errors: string[] = []
  const r = draft.value
  if (!r) return { valid: true, errors }
  if (!r.name || !r.name.trim()) errors.push(t('admin.ops.alertRules.validation.nameRequired'))
  if (!r.metric_type) errors.push(t('admin.ops.alertRules.validation.metricRequired'))
  if (groupMetricTypes.has(r.metric_type) && !parsePositiveInt(r.filters?.group_id)) {
    errors.push(t('admin.ops.alertRules.validation.groupIdRequired'))
  }
  if (!r.operator) errors.push(t('admin.ops.alertRules.validation.operatorRequired'))
  if (!(typeof r.threshold === 'number' && Number.isFinite(r.threshold)))
    errors.push(t('admin.ops.alertRules.validation.thresholdRequired'))
  if (!(typeof r.window_minutes === 'number' && Number.isFinite(r.window_minutes) && [1, 5, 60].includes(r.window_minutes))) {
    errors.push(t('admin.ops.alertRules.validation.windowRange'))
  }
  if (!(typeof r.sustained_minutes === 'number' && Number.isFinite(r.sustained_minutes) && r.sustained_minutes >= 1 && r.sustained_minutes <= 1440)) {
    errors.push(t('admin.ops.alertRules.validation.sustainedRange'))
  }
  if (!(typeof r.cooldown_minutes === 'number' && Number.isFinite(r.cooldown_minutes) && r.cooldown_minutes >= 0 && r.cooldown_minutes <= 1440)) {
    errors.push(t('admin.ops.alertRules.validation.cooldownRange'))
  }
  return { valid: errors.length === 0, errors }
})

async function save() {
  if (!draft.value) return
  if (!editorValidation.value.valid) {
    appStore.showError(editorValidation.value.errors[0] || t('admin.ops.alertRules.validation.invalid'))
    return
  }
  saving.value = true
  try {
    if (editingId.value) {
      await opsAPI.updateAlertRule(editingId.value, draft.value)
    } else {
      await opsAPI.createAlertRule(draft.value)
    }
    showEditor.value = false
    draft.value = null
    editingId.value = null
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.saveSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to save rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.saveFailed'))
  } finally {
    saving.value = false
  }
}

const showDeleteConfirm = ref(false)
const pendingDelete = ref<AlertRule | null>(null)

function requestDelete(rule: AlertRule) {
  pendingDelete.value = rule
  showDeleteConfirm.value = true
}

async function confirmDelete() {
  if (!pendingDelete.value?.id) return
  try {
    await opsAPI.deleteAlertRule(pendingDelete.value.id)
    showDeleteConfirm.value = false
    pendingDelete.value = null
    await load()
    appStore.showSuccess(t('admin.ops.alertRules.deleteSuccess'))
  } catch (err: any) {
    console.error('[OpsAlertRulesCard] Failed to delete rule', err)
    appStore.showError(err?.response?.data?.detail || t('admin.ops.alertRules.deleteFailed'))
  }
}

function cancelDelete() {
  showDeleteConfirm.value = false
  pendingDelete.value = null
}
</script>

<template>
  <div class="card p-5">
    <div class="mb-4 flex flex-wrap items-start justify-between gap-3 sm:gap-4">
      <div>
        <h3 class="text-body font-bold text-accent-strong">{{ t('admin.ops.alertRules.title') }}</h3>
        <p class="mt-1 text-xs text-fg-muted">{{ t('admin.ops.alertRules.description') }}</p>
      </div>

      <div class="flex items-center gap-2">
        <button class="btn btn-sm btn-primary" :disabled="loading" @click="openCreate">
          {{ t('admin.ops.alertRules.create') }}
        </button>
        <button
          class="btn btn-secondary btn-sm"
          :disabled="loading"
          @click="load"
        >
          <svg class="h-3.5 w-3.5" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15" />
          </svg>
          {{ t('common.refresh') }}
        </button>
      </div>
    </div>

    <div v-if="loading" class="py-10 text-center text-sm text-fg-muted">
      {{ t('admin.ops.alertRules.loading') }}
    </div>

    <div v-else-if="sortedRules.length === 0" class="border-y border-border py-8 text-center text-sm text-fg-muted">
      {{ t('admin.ops.alertRules.empty') }}
    </div>

    <div v-else class="max-h-[520px] overflow-hidden border border-border">
      <div class="max-h-[520px] overflow-y-auto">
        <DataTable :columns="columns" :data="sortedRules" row-key="id">
          <template #cell-name="{ row }">
            <div class="max-w-[360px] whitespace-normal">
              <div class="text-label font-bold text-fg">{{ row.name }}</div>
              <div v-if="row.description" class="mt-0.5 line-clamp-2 text-meta text-fg-muted">
                {{ row.description }}
              </div>
              <div v-if="row.updated_at" class="mt-1 text-meta text-fg-subtle">
                {{ formatDateTime(row.updated_at) }}
              </div>
            </div>
          </template>
          <template #cell-metric_type="{ row }">
            <span class="font-mono">{{ row.metric_type }}</span>
            <span class="mx-1 text-fg-subtle">{{ row.operator }}</span>
            <span class="font-mono">{{ row.threshold }}</span>
          </template>
          <template #cell-enabled="{ row }">
            {{ row.enabled ? t('common.enabled') : t('common.disabled') }}
          </template>
          <template #cell-actions="{ row }">
            <div class="flex items-center justify-end gap-2">
              <button class="btn btn-sm btn-secondary" @click="openEdit(row)">{{ t('common.edit') }}</button>
              <button class="btn btn-sm btn-danger" @click="requestDelete(row)">{{ t('common.delete') }}</button>
            </div>
          </template>
        </DataTable>
      </div>
    </div>

    <BaseDialog
      :show="showEditor"
      :title="editingId ? t('admin.ops.alertRules.editTitle') : t('admin.ops.alertRules.createTitle')"
      width="wide"
      @close="showEditor = false"
    >
      <div class="space-y-4">
        <div v-if="!editorValidation.valid" class="border border-danger/40 bg-danger-weak px-3 py-2 text-xs text-danger-strong">
          <div class="font-bold">{{ t('admin.ops.alertRules.validation.title') }}</div>
          <ul class="mt-1 list-disc pl-5">
            <li v-for="e in editorValidation.errors" :key="e">{{ e }}</li>
          </ul>
        </div>

        <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.name') }}</label>
            <input v-model="draft!.name" class="input" type="text" />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">{{ t('admin.ops.alertRules.form.description') }}</label>
            <input v-model="draft!.description" class="input" type="text" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.metric') }}</label>
            <Select v-model="draft!.metric_type" :options="metricOptions" />
            <div v-if="selectedMetricDefinition" class="mt-1 space-y-0.5 text-xs text-fg-muted">
              <p>{{ selectedMetricDefinition.description }}</p>
              <p>
                {{
                  t('admin.ops.alertRules.hints.recommended', {
                    operator: selectedMetricDefinition.recommendedOperator,
                    threshold: selectedMetricDefinition.recommendedThreshold,
                    unit: selectedMetricDefinition.unit || ''
                  })
                }}
              </p>
            </div>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.operator') }}</label>
            <Select v-model="draft!.operator" :options="operatorOptions" />
          </div>

          <div class="md:col-span-2">
            <label class="input-label">
              {{ t('admin.ops.alertRules.form.groupId') }}
              <span v-if="isGroupMetricSelected" class="ml-1 text-danger">*</span>
            </label>
            <Select
              v-model="draftGroupId"
              :options="groupOptions"
              searchable
              :placeholder="t('admin.ops.alertRules.form.groupPlaceholder')"
              :error="isGroupMetricSelected && !draftGroupId"
            />
            <p class="mt-1 text-xs text-fg-muted">
              {{ isGroupMetricSelected ? t('admin.ops.alertRules.hints.groupRequired') : t('admin.ops.alertRules.hints.groupOptional') }}
            </p>
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.threshold') }}</label>
            <input v-model.number="draft!.threshold" class="input" type="number" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.severity') }}</label>
            <Select v-model="draft!.severity" :options="severityOptions" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.window') }}</label>
            <Select v-model="draft!.window_minutes" :options="windowOptions" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.sustained') }}</label>
            <input v-model.number="draft!.sustained_minutes" class="input" type="number" min="1" max="1440" />
          </div>

          <div>
            <label class="input-label">{{ t('admin.ops.alertRules.form.cooldown') }}</label>
            <input v-model.number="draft!.cooldown_minutes" class="input" type="number" min="0" max="1440" />
          </div>

          <div class="flex items-center justify-between border-y border-border py-3 md:col-span-2">
            <span class="text-xs font-bold text-fg">{{ t('admin.ops.alertRules.form.enabled') }}</span>
            <input v-model="draft!.enabled" type="checkbox" class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent" />
          </div>

          <div class="flex items-center justify-between border-y border-border py-3 md:col-span-2">
            <span class="text-xs font-bold text-fg">{{ t('admin.ops.alertRules.form.notifyEmail') }}</span>
            <input v-model="draft!.notify_email" type="checkbox" class="h-4 w-4 rounded-sm border-border-strong text-accent focus:ring-accent" />
          </div>
          <p v-if="draft!.notify_email && emailAlertsInactive" class="text-xs text-warning md:col-span-2" data-test="notify-email-inactive">
            {{ t('admin.ops.alertRules.form.notifyEmailInactive') }}
          </p>
        </div>
      </div>

      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <button class="btn btn-secondary" :disabled="saving" @click="showEditor = false">
            {{ t('common.cancel') }}
          </button>
          <button class="btn btn-primary" :disabled="saving" @click="save">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteConfirm"
      :title="t('admin.ops.alertRules.deleteConfirmTitle')"
      :message="t('admin.ops.alertRules.deleteConfirmMessage')"
      :confirmText="t('common.delete')"
      :cancelText="t('common.cancel')"
      @confirm="confirmDelete"
      @cancel="cancelDelete"
    />
  </div>
</template>
