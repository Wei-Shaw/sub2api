<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">{{ t('admin.serviceQuota.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.description') }}</p>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-wrap items-center gap-3">
            <select v-model="filters.counterMode" class="input w-auto min-w-[160px]">
              <option value="">{{ t('admin.serviceQuota.filters.allCounterModes') }}</option>
              <option v-for="item in counterModeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
            <select v-model="filters.fallback" class="input w-auto min-w-[160px]">
              <option value="">{{ t('admin.serviceQuota.filters.allFallback') }}</option>
              <option value="true">{{ t('admin.serviceQuota.fallback.yes') }}</option>
              <option value="false">{{ t('admin.serviceQuota.fallback.no') }}</option>
            </select>
            <select v-model="filters.enabled" class="input w-auto min-w-[160px]">
              <option value="">{{ t('admin.serviceQuota.filters.allStatus') }}</option>
              <option value="true">{{ t('common.enabled') }}</option>
              <option value="false">{{ t('common.disabled') }}</option>
            </select>
          </div>
          <div class="flex shrink-0 items-center gap-3">
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

      <template #table>
        <DataTable :columns="columns" :data="filteredRules" :loading="loading">
          <template #cell-enabled="{ value }">
            <span :class="['badge', value ? 'badge-success' : 'badge-gray']">
              {{ value ? t('common.enabled') : t('common.disabled') }}
            </span>
          </template>

          <template #cell-name="{ row }">
            <div class="space-y-0.5">
              <div class="font-medium text-gray-900 dark:text-white">
                {{ row.name || t('admin.serviceQuota.unnamedRule', { id: row.id }) }}
              </div>
              <div class="text-xs text-gray-400">#{{ row.id }}</div>
            </div>
          </template>

          <template #cell-limiters="{ row }">
            <div class="flex flex-wrap gap-1.5">
              <span
                v-for="lim in row.limiters"
                :key="lim.id"
                :class="['badge', limiterBadgeClass(lim.limiter_type)]"
                :title="`${limiterLabel(lim.limiter_type)} = ${formatLimitValue(lim)}`"
              >
                {{ limiterLabel(lim.limiter_type) }}
                <span class="ml-1 font-mono text-[10px] opacity-80">{{ formatLimitValue(lim) }}</span>
              </span>
            </div>
          </template>

          <template #cell-paths="{ row }">
            <div class="space-y-1">
              <div v-if="row.paths.length === 0" class="text-xs text-gray-400">{{ t('admin.serviceQuota.scopeDetails.allRequests') }}</div>
              <div v-else>
                <div class="text-sm text-gray-700 dark:text-gray-200">{{ pathSummary(row.paths[0]) }}</div>
                <div v-if="row.paths.length > 1" class="text-xs text-gray-400">
                  {{ t('admin.serviceQuota.morePaths', { count: row.paths.length - 1 }) }}
                </div>
              </div>
            </div>
          </template>

          <template #cell-counter_mode="{ row }">
            <div class="space-y-0.5">
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
      <form id="service-quota-form" class="space-y-6" @submit.prevent="save">
        <section class="grid gap-4 md:grid-cols-2">
          <label class="form-field md:col-span-2">
            <span class="input-label">{{ t('admin.serviceQuota.form.name') }}</span>
            <input v-model="form.name" :placeholder="t('admin.serviceQuota.form.namePlaceholder')" class="input" maxlength="128" />
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.columns.status') }}</span>
            <select v-model="form.enabled" class="input">
              <option :value="true">{{ t('common.enabled') }}</option>
              <option :value="false">{{ t('common.disabled') }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">{{ t('admin.serviceQuota.form.counterMode') }}</span>
            <select v-model="form.counter_mode" class="input">
              <option v-for="item in counterModeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ counterModeHint(form.counter_mode) }}</span>
          </label>
          <label class="form-field md:col-span-2 flex items-center gap-2">
            <input v-model="form.is_fallback" type="checkbox" class="h-4 w-4 rounded border-gray-300" />
            <span class="text-sm">
              <span class="font-medium text-gray-900 dark:text-white">{{ t('admin.serviceQuota.form.fallback') }}</span>
              <span class="ml-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.fallback.hint') }}</span>
            </span>
          </label>
        </section>

        <section v-if="form.counter_mode === 'user'" class="space-y-2">
          <span class="input-label">{{ t('admin.serviceQuota.form.targetUserIds') }}</span>
          <UserMultiSelect
            v-model="selectedTargetUsers"
            :placeholder="t('admin.serviceQuota.form.targetUserIdsPlaceholder')"
          />
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.form.targetUserIdsRequired') }}</span>
        </section>

        <section class="space-y-2">
          <div class="flex items-baseline justify-between">
            <span class="input-label">{{ t('admin.serviceQuota.form.limitersTitle') }}</span>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.form.limitersHint') }}</span>
          </div>
          <LimiterEditor v-model="form.limiters" />
        </section>

        <section class="space-y-2">
          <div class="flex items-baseline justify-between">
            <span class="input-label">{{ t('admin.serviceQuota.form.pathsTitle') }}</span>
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.serviceQuota.form.pathsHint') }}</span>
          </div>
          <PathEditor v-model="form.paths" />
        </section>
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
      :message="t('admin.serviceQuota.deleteConfirm', { name: deletingRule?.name || `#${deletingRule?.id}` })"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deletingRule = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import UserMultiSelect from '@/components/common/UserMultiSelect.vue'
import LimiterEditor from '@/components/admin/LimiterEditor.vue'
import PathEditor from '@/components/admin/PathEditor.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { Column } from '@/components/common/types'
import type { SimpleUser } from '@/api/admin/usage'
import {
  createServiceQuotaRule,
  deleteServiceQuotaRule,
  listServiceQuotaRules,
  updateServiceQuotaRule,
  type ServiceQuotaLimiterDef,
  type ServiceQuotaPathDef,
  type ServiceQuotaRule,
  type ServiceQuotaRuleInput,
} from '@/api/admin/serviceQuota'

const { t } = useI18n()
const appStore = useAppStore()

const counterModeOptions = computed(() => [
  { value: 'user', label: t('admin.serviceQuota.counterModes.user') },
  { value: 'per_user', label: t('admin.serviceQuota.counterModes.perUser') },
  { value: 'shared', label: t('admin.serviceQuota.counterModes.shared') },
])

const columns = computed<Column[]>(() => [
  { key: 'enabled', label: t('admin.serviceQuota.columns.status') },
  { key: 'name', label: t('admin.serviceQuota.columns.name') },
  { key: 'limiters', label: t('admin.serviceQuota.columns.limiters') },
  { key: 'paths', label: t('admin.serviceQuota.columns.paths') },
  { key: 'counter_mode', label: t('admin.serviceQuota.columns.counterMode') },
  { key: 'is_fallback', label: t('admin.serviceQuota.columns.fallback') },
  { key: 'actions', label: t('admin.serviceQuota.columns.actions') },
])

const rules = ref<ServiceQuotaRule[]>([])
const loading = ref(false)
const saving = ref(false)
const showDialog = ref(false)
const editingID = ref<number | null>(null)
const deletingRule = ref<ServiceQuotaRule | null>(null)
const filters = reactive({ counterMode: '', fallback: '', enabled: '' })
const form = reactive<ServiceQuotaRuleInput>(blankRule())
const selectedTargetUsers = ref<SimpleUser[]>([])

const filteredRules = computed(() => rules.value.filter((rule) => {
  if (filters.counterMode && rule.counter_mode !== filters.counterMode) return false
  if (filters.fallback && String(rule.is_fallback) !== filters.fallback) return false
  if (filters.enabled && String(rule.enabled) !== filters.enabled) return false
  return true
}))

function blankRule(): ServiceQuotaRuleInput {
  return {
    enabled: true,
    name: null,
    counter_mode: 'per_user',
    is_fallback: false,
    target_user_ids: null,
    limiters: [{ limiter_type: 'rpm', window_mode: 'fixed', limit_value: 60 }],
    paths: [{ platform: null, channel_id: null, group_id: null, account_id: null, model_pattern: null }],
  }
}

function resetForm(rule?: ServiceQuotaRule) {
  const initial = blankRule()
  if (rule) {
    initial.enabled = rule.enabled
    initial.name = rule.name ?? null
    initial.counter_mode = rule.counter_mode
    initial.is_fallback = rule.is_fallback
    initial.limiters = rule.limiters.map((l) => ({
      limiter_type: l.limiter_type,
      window_mode: l.window_mode,
      limit_value: l.limit_value,
    }))
    initial.paths = rule.paths.map((p) => ({
      platform: p.platform ?? null,
      channel_id: p.channel_id ?? null,
      group_id: p.group_id ?? null,
      account_id: p.account_id ?? null,
      model_pattern: p.model_pattern ?? null,
    }))
    initial.target_user_ids = rule.target_user_ids ?? null
  }
  Object.assign(form, initial)
  editingID.value = rule?.id ?? null
  selectedTargetUsers.value = (rule?.target_users || []).map((u) => ({ id: u.id, email: u.email }))
}

function limiterLabel(value: string): string {
  const map: Record<string, string> = {
    rpm: t('admin.serviceQuota.limiters.rpm'),
    tpm: t('admin.serviceQuota.limiters.tpm'),
    tpd: t('admin.serviceQuota.limiters.tpd'),
    daily_usd: t('admin.serviceQuota.limiters.dailyUsd'),
    concurrency: t('admin.serviceQuota.limiters.concurrency'),
  }
  return map[value] || value
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

function formatLimitValue(lim: ServiceQuotaLimiterDef): string {
  if (lim.limiter_type === 'daily_usd') return `$${Number(lim.limit_value).toFixed(6).replace(/\.?0+$/, '')}`
  return String(lim.limit_value)
}

function counterModeLabel(value: string): string {
  return counterModeOptions.value.find((item) => item.value === value)?.label || value
}

function counterModeHint(value: string): string {
  const map: Record<string, string> = {
    user: t('admin.serviceQuota.counterModeHints.user'),
    per_user: t('admin.serviceQuota.counterModeHints.perUser'),
    shared: t('admin.serviceQuota.counterModeHints.shared'),
  }
  return map[value] || ''
}

function pathSummary(path: ServiceQuotaPathDef): string {
  const parts: string[] = []
  if (path.platform) parts.push(t('admin.serviceQuota.scopeDetails.platform', { value: path.platform }))
  if (path.channel_id) parts.push(t('admin.serviceQuota.scopeDetails.channel', { value: path.channel_id }))
  if (path.group_id) parts.push(t('admin.serviceQuota.scopeDetails.group', { value: path.group_id }))
  if (path.account_id) parts.push(t('admin.serviceQuota.scopeDetails.account', { value: path.account_id }))
  if (path.model_pattern) parts.push(t('admin.serviceQuota.scopeDetails.model', { value: path.model_pattern }))
  return parts.length > 0 ? parts.join(' / ') : t('admin.serviceQuota.scopeDetails.allRequests')
}

function normalizePayload(): ServiceQuotaRuleInput {
  const payload: ServiceQuotaRuleInput = {
    enabled: form.enabled,
    name: cleanText(form.name),
    counter_mode: form.counter_mode,
    is_fallback: form.is_fallback,
    limiters: form.limiters.map((l) => ({
      limiter_type: l.limiter_type,
      window_mode: l.limiter_type === 'concurrency' ? 'fixed' : l.window_mode,
      limit_value: Number(l.limit_value),
    })),
    paths: form.paths.map((p) => ({
      platform: cleanText(p.platform),
      channel_id: cleanNumber(p.channel_id),
      group_id: cleanNumber(p.group_id),
      account_id: cleanNumber(p.account_id),
      model_pattern: cleanText(p.model_pattern),
    })),
    target_user_ids: form.counter_mode === 'user' ? selectedTargetUsers.value.map((u) => u.id) : null,
  }
  return payload
}

function cleanText(value?: string | null): string | null {
  return value && value.trim() ? value.trim() : null
}

function cleanNumber(value?: number | null): number | null {
  return value && value > 0 ? value : null
}

async function load() {
  loading.value = true
  try {
    rules.value = await listServiceQuotaRules()
  } catch (error: unknown) {
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
    if (editingID.value) {
      await updateServiceQuotaRule(editingID.value, payload)
    } else {
      await createServiceQuotaRule(payload)
    }
    appStore.showSuccess(t('admin.serviceQuota.saveSuccess'))
    showDialog.value = false
    await load()
  } catch (error: unknown) {
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
  } catch (error: unknown) {
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