<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.serviceQuota.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.description') }}</p>
          </div>
          <div class="flex items-center gap-3">
            <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">
              <Icon name="refresh" size="sm" class="mr-2" />
              {{ t('common.refresh') }}
            </button>
            <button class="btn btn-primary" type="button" @click="openCreate">
              <Icon name="plus" size="sm" class="mr-2" />
              {{ t('admin.serviceQuota.createRule') }}
            </button>
          </div>
        </div>
      </template>

      <template #filters>
        <div class="grid gap-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 md:grid-cols-4">
          <select v-model="filters.limiter" class="input">
            <option value="">{{ t('admin.serviceQuota.filters.allTypes') }}</option>
            <option v-for="item in limiterOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <select v-model="filters.counterMode" class="input">
            <option value="">{{ t('admin.serviceQuota.filters.allCounterModes') }}</option>
            <option v-for="item in counterModeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <select v-model="filters.fallback" class="input">
            <option value="">{{ t('admin.serviceQuota.filters.allFallback') }}</option>
            <option value="true">{{ t('admin.serviceQuota.fallback.yes') }}</option>
            <option value="false">{{ t('admin.serviceQuota.fallback.no') }}</option>
          </select>
          <select v-model="filters.enabled" class="input">
            <option value="">{{ t('admin.serviceQuota.filters.allStatus') }}</option>
            <option value="true">{{ t('common.enabled') }}</option>
            <option value="false">{{ t('common.disabled') }}</option>
          </select>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredRules" :loading="loading">
          <template #cell-enabled="{ value }">
            <span :class="['badge', value ? 'badge-success' : 'badge-gray']">
              {{ value ? t('common.enabled') : t('common.disabled') }}
            </span>
          </template>

          <template #cell-scope="{ row }">
            <div class="space-y-1">
              <div class="font-medium text-gray-900 dark:text-white">{{ scopePrimaryLabel(row) }}</div>
              <div class="max-w-md truncate text-xs text-gray-500 dark:text-gray-400">{{ scopeDetail(row) }}</div>
            </div>
          </template>

          <template #cell-limiter_type="{ row }">
            <span :class="['badge', limiterBadgeClass(row.limiter_type)]">{{ limiterLabel(row.limiter_type) }}</span>
          </template>

          <template #cell-counter_mode="{ row }">
            <div class="space-y-1">
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ counterModeLabel(row.counter_mode) }}</div>
              <div v-if="row.counter_mode === 'user' && row.target_user_ids?.length" class="text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.serviceQuota.userId', { id: row.target_user_ids.join(', ') }) }}
              </div>
            </div>
          </template>

          <template #cell-is_fallback="{ row }">
            <span :class="['badge', row.is_fallback ? 'badge-yellow' : 'badge-gray']">
              {{ row.is_fallback ? t('admin.serviceQuota.fallback.yes') : t('admin.serviceQuota.fallback.no') }}
            </span>
          </template>

          <template #cell-window_mode="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ windowLabel(row) }}</span>
          </template>

          <template #cell-limit_value="{ row }">
            <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ formatLimit(row) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button class="action-btn hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20" type="button" :title="t('common.edit')" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="action-btn hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" type="button" :title="t('common.delete')" @click="askDelete(row)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.serviceQuota.emptyTitle')"
              :description="t('admin.serviceQuota.emptyDescription')"
              :action-text="t('admin.serviceQuota.createRule')"
              @action="openCreate"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <BaseDialog :show="showDialog" :title="editingID ? t('admin.serviceQuota.editRule') : t('admin.serviceQuota.createRule')" width="wide" @close="closeDialog">
      <form id="service-quota-form" class="space-y-5" @submit.prevent="save">
        <div class="grid gap-4 md:grid-cols-2">
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.columns.status') }}</span>
            <select v-model="form.enabled" class="input">
              <option :value="true">{{ t('common.enabled') }}</option>
              <option :value="false">{{ t('common.disabled') }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.columns.type') }}</span>
            <select v-model="form.limiter_type" class="input">
              <option v-for="item in limiterOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.form.counterMode') }}</span>
            <select v-model="form.counter_mode" class="input">
              <option v-for="item in counterModeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ counterModeHint(form.counter_mode) }}</span>
          </label>
        </div>

        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">{{ t('admin.serviceQuota.form.scopeMatching') }}</div>
          <div class="grid gap-4 md:grid-cols-2">
            <label class="form-field">
              <span class="input-label">{{ t('admin.serviceQuota.form.platform') }}</span>
              <input v-model="form.platform" class="input" :placeholder="t('admin.serviceQuota.form.platformPlaceholder')" />
            </label>
            <label class="form-field">
              <span class="input-label">{{ t('admin.serviceQuota.form.groupId') }}</span>
              <input v-model.number="form.group_id" class="input" min="1" type="number" :placeholder="t('common.optional')" />
            </label>
            <label class="form-field">
              <span class="input-label">{{ t('admin.serviceQuota.form.accountId') }}</span>
              <input v-model.number="form.account_id" class="input" min="1" type="number" :placeholder="t('common.optional')" />
            </label>
            <label class="form-field">
              <span class="input-label">{{ t('admin.serviceQuota.form.modelPattern') }}</span>
              <input v-model="form.model_pattern" class="input" :placeholder="t('admin.serviceQuota.form.modelPatternPlaceholder')" />
            </label>
          </div>
        </div>

        <div class="grid gap-4 md:grid-cols-3">
          <label v-if="form.counter_mode === 'user'" class="form-field md:col-span-3">
            <span class="input-label">{{ t('admin.serviceQuota.form.targetUserIds') }}</span>
            <input v-model="targetUserIdsInput" class="input" type="text" :placeholder="t('admin.serviceQuota.form.targetUserIdsPlaceholder')" required />
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.form.targetUserIdsRequired') }}</span>
          </label>
          <label class="form-field flex items-center gap-2">
            <input v-model="form.is_fallback" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
            <span class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">{{ t('admin.serviceQuota.form.fallback') }}</span>
              <span class="ml-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.fallback.hint') }}</span>
            </span>
          </label>
          <label v-if="form.limiter_type !== 'concurrency'" class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.columns.window') }}</span>
            <select v-model="form.window_mode" class="input">
              <option value="fixed">{{ t('admin.serviceQuota.windows.fixed') }}</option>
              <option value="rolling">{{ t('admin.serviceQuota.windows.rolling') }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.columns.limit') }}</span>
            <input v-model.number="form.limit_value" class="input" min="0" step="0.000001" type="number" required />
          </label>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" type="button" @click="closeDialog">{{ t('common.cancel') }}</button>
          <button class="btn btn-primary" type="submit" form="service-quota-form" :disabled="saving">
            {{ saving ? t('common.saving') : t('common.save') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!deletingRule"
      :title="t('admin.serviceQuota.deleteRule')"
      :message="t('admin.serviceQuota.deleteConfirm', { type: deletingRule ? limiterLabel(deletingRule.limiter_type) : '' })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deletingRule = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { Column } from '@/components/common/types'
import { createServiceQuotaRule, deleteServiceQuotaRule, listServiceQuotaRules, updateServiceQuotaRule, type ServiceQuotaRule, type ServiceQuotaRuleInput } from '@/api/admin/serviceQuota'

const { t } = useI18n()
const appStore = useAppStore()

const limiterOptions = computed(() => [
  { value: 'rpm', label: t('admin.serviceQuota.limiters.rpm') },
  { value: 'tpm', label: t('admin.serviceQuota.limiters.tpm') },
  { value: 'tpd', label: t('admin.serviceQuota.limiters.tpd') },
  { value: 'daily_usd', label: t('admin.serviceQuota.limiters.dailyUsd') },
  { value: 'concurrency', label: t('admin.serviceQuota.limiters.concurrency') },
])
const counterModeOptions = computed(() => [
  { value: 'user', label: t('admin.serviceQuota.counterModes.user') },
  { value: 'per_user', label: t('admin.serviceQuota.counterModes.perUser') },
  { value: 'shared', label: t('admin.serviceQuota.counterModes.shared') },
])

const columns = computed<Column[]>(() => [
  { key: 'enabled', label: t('admin.serviceQuota.columns.status') },
  { key: 'scope', label: t('admin.serviceQuota.columns.scope') },
  { key: 'limiter_type', label: t('admin.serviceQuota.columns.type') },
  { key: 'counter_mode', label: t('admin.serviceQuota.columns.counterMode') },
  { key: 'is_fallback', label: t('admin.serviceQuota.columns.fallback') },
  { key: 'window_mode', label: t('admin.serviceQuota.columns.window') },
  { key: 'limit_value', label: t('admin.serviceQuota.columns.limit') },
  { key: 'actions', label: t('admin.serviceQuota.columns.actions') },
])

const rules = ref<ServiceQuotaRule[]>([])
const loading = ref(false)
const saving = ref(false)
const showDialog = ref(false)
const editingID = ref<number | null>(null)
const deletingRule = ref<ServiceQuotaRule | null>(null)
const filters = reactive({ limiter: '', counterMode: '', fallback: '', enabled: '' })
const form = reactive<ServiceQuotaRuleInput>(blankRule())
const targetUserIdsInput = ref('')

const filteredRules = computed(() => rules.value.filter((rule) => {
  if (filters.limiter && rule.limiter_type !== filters.limiter) return false
  if (filters.counterMode && rule.counter_mode !== filters.counterMode) return false
  if (filters.fallback && String(rule.is_fallback) !== filters.fallback) return false
  if (filters.enabled && String(rule.enabled) !== filters.enabled) return false
  return true
}))

watch(() => form.limiter_type, (value) => {
  if (value === 'concurrency') form.window_mode = 'fixed'
})

function blankRule(): ServiceQuotaRuleInput {
  return {
    enabled: true,
    limiter_type: 'rpm',
    counter_mode: 'per_user',
    is_fallback: false,
    target_user_ids: null,
    window_mode: 'fixed',
    limit_value: 60,
  }
}

function resetForm(rule?: ServiceQuotaRule) {
  Object.assign(form, blankRule(), rule || {})
  editingID.value = rule?.id ?? null
  targetUserIdsInput.value = (rule?.target_user_ids || []).join(',')
}

function optionLabel(options: Array<{ value: string; label: string }>, value: string): string {
  return options.find((item) => item.value === value)?.label || value
}

function limiterLabel(value: string): string { return optionLabel(limiterOptions.value, value) }

function scopePrimaryLabel(rule: ServiceQuotaRule): string {
  const fields = [rule.model_pattern, rule.account_id, rule.group_id, rule.platform]
  const hit = fields.find((v) => v !== null && v !== undefined && v !== '')
  if (hit === rule.model_pattern) return t('admin.serviceQuota.scopes.model')
  if (hit === rule.account_id) return t('admin.serviceQuota.scopes.account')
  if (hit === rule.group_id) return t('admin.serviceQuota.scopes.group')
  if (hit === rule.platform) return t('admin.serviceQuota.scopes.platform')
  return t('admin.serviceQuota.scopes.global')
}
function counterModeLabel(value: string): string { return optionLabel(counterModeOptions.value, value) }

function counterModeHint(value: string): string {
  const map: Record<string, string> = {
    user: t('admin.serviceQuota.counterModeHints.user'),
    per_user: t('admin.serviceQuota.counterModeHints.perUser'),
    shared: t('admin.serviceQuota.counterModeHints.shared'),
  }
  return map[value] || ''
}

function scopeDetail(rule: ServiceQuotaRule): string {
  const parts = [
    rule.platform && t('admin.serviceQuota.scopeDetails.platform', { value: rule.platform }),
    rule.group_id && t('admin.serviceQuota.scopeDetails.group', { value: rule.group_id }),
    rule.account_id && t('admin.serviceQuota.scopeDetails.account', { value: rule.account_id }),
    rule.model_pattern && t('admin.serviceQuota.scopeDetails.model', { value: rule.model_pattern }),
  ].filter(Boolean)
  return parts.length > 0 ? parts.join(' / ') : t('admin.serviceQuota.scopeDetails.allRequests')
}

function windowLabel(rule: ServiceQuotaRule): string {
  if (rule.limiter_type === 'concurrency') return t('admin.serviceQuota.windows.none')
  return rule.window_mode === 'rolling' ? t('admin.serviceQuota.windows.rolling') : t('admin.serviceQuota.windows.fixed')
}

function formatLimit(rule: ServiceQuotaRule): string {
  const limit = formatLimitValue(rule, rule.limit_value)
  if (rule.current_usage === undefined || rule.current_usage === null) return limit
  return `${formatLimitValue(rule, rule.current_usage)} / ${limit}`
}

function formatLimitValue(rule: ServiceQuotaRule, value: number): string {
  if (rule.limiter_type === 'daily_usd') return `$${Number(value).toFixed(6).replace(/\.?0+$/, '')}`
  return String(value)
}

function limiterBadgeClass(value: string): string {
  const classes: Record<string, string> = {
    rpm: 'badge-blue',
    tpm: 'badge-purple',
    tpd: 'badge-indigo',
    daily_usd: 'badge-green',
    concurrency: 'badge-yellow',
  }
  return classes[value] || 'badge-gray'
}

function parseUserIds(raw: string): number[] {
  return raw
    .split(/[,\s]+/)
    .map((part) => part.trim())
    .filter((part) => part.length > 0)
    .map((part) => Number(part))
    .filter((n) => Number.isFinite(n) && n > 0)
}

function normalizePayload(): ServiceQuotaRuleInput {
  const payload = { ...form }
  payload.platform = cleanText(payload.platform)
  payload.model_pattern = cleanText(payload.model_pattern)
  payload.group_id = cleanNumber(payload.group_id)
  payload.account_id = cleanNumber(payload.account_id)
  if (payload.counter_mode === 'user') {
    payload.target_user_ids = parseUserIds(targetUserIdsInput.value)
  } else {
    payload.target_user_ids = null
  }
  if (payload.limiter_type === 'concurrency') payload.window_mode = 'fixed'
  return payload
}

function cleanText(value?: string | null): string | null { return value && value.trim() ? value.trim() : null }
function cleanNumber(value?: number | null): number | null { return value && value > 0 ? value : null }

async function load() {
  loading.value = true
  try {
    rules.value = await listServiceQuotaRules()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.serviceQuota.loadError')))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  resetForm()
  showDialog.value = true
}

function openEdit(rule: ServiceQuotaRule) {
  resetForm(rule)
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
}

async function save() {
  saving.value = true
  try {
    const payload = normalizePayload()
    if (editingID.value) await updateServiceQuotaRule(editingID.value, payload)
    else await createServiceQuotaRule(payload)
    appStore.showSuccess(t('admin.serviceQuota.saveSuccess'))
    showDialog.value = false
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.serviceQuota.saveError')))
  } finally {
    saving.value = false
  }
}

function askDelete(rule: ServiceQuotaRule) {
  deletingRule.value = rule
}

async function confirmDelete() {
  if (!deletingRule.value) return
  try {
    await deleteServiceQuotaRule(deletingRule.value.id)
    appStore.showSuccess(t('admin.serviceQuota.deleteSuccess'))
    deletingRule.value = null
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.serviceQuota.deleteError')))
  }
}

onMounted(load)
</script>

<style scoped>
.form-field {
  @apply space-y-1.5;
}

.action-btn {
  @apply rounded-lg p-1.5 text-gray-500 transition-colors dark:text-gray-400;
}
</style>
