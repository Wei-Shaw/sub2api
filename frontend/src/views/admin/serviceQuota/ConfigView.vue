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
          <template #cell-enabled="{ row }">
            <EnabledToggleCell :row="row" @updated="load" />
          </template>

          <template #cell-name="{ row }">
            <div class="space-y-0.5">
              <div class="font-medium text-gray-900 dark:text-white">
                {{ row.name || t('admin.serviceQuota.unnamedRule', { id: row.id }) }}
              </div>
              <div class="text-xs text-gray-400">#{{ row.id }}</div>
            </div>
          </template>

          <!-- 一个限流器一行：chip 用统一颜色（与监控页 / 用户限额页同一套），值用千分位 -->
          <template #cell-limiters="{ row }">
            <div class="flex flex-col gap-1">
              <span
                v-for="lim in row.limiters"
                :key="lim.id"
                :class="['inline-flex w-fit items-center gap-1.5 rounded-md px-2 py-0.5 text-[11px] font-semibold', limiterChipClass(lim.limiter_type)]"
                :title="`${limiterLabel(lim.limiter_type)} = ${formatLimitValue(lim)}`"
              >
                <span>{{ limiterLabel(lim.limiter_type) }}</span>
                <span class="font-mono opacity-80">{{ formatLimitValue(lim) }}</span>
              </span>
            </div>
          </template>

          <template #cell-paths="{ row }">
            <div v-if="row.paths.length === 0" class="text-xs text-gray-400">
              {{ t('admin.serviceQuota.scopeDetails.allRequests') }}
            </div>
            <div v-else class="max-h-32 space-y-1 overflow-auto">
              <PathChevron
                v-for="(path, i) in row.paths"
                :key="i"
                :summary="pathDefToSummary(path)"
                show-internal
              />
            </div>
          </template>

          <template #cell-counter_mode="{ row }">
            <div class="text-sm font-medium text-gray-900 dark:text-white">{{ counterModeLabel(row.counter_mode) }}</div>
          </template>

          <template #cell-target_users="{ row }">
            <div v-if="row.counter_mode !== 'user'" class="text-xs text-gray-400">—</div>
            <div v-else-if="!targetUsersFor(row).length" class="text-xs text-gray-400">—</div>
            <div v-else class="flex flex-wrap items-center gap-1">
              <span
                v-for="u in targetUsersFor(row).slice(0, TARGET_USERS_VISIBLE_LIMIT)"
                :key="u.id"
                class="inline-flex items-center rounded-md bg-gray-100 px-2 py-0.5 text-[11px] font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-200"
                :title="`#${u.id}`"
              >
                {{ u.label }}
              </span>
              <span
                v-if="targetUsersFor(row).length > TARGET_USERS_VISIBLE_LIMIT"
                class="text-[11px] text-gray-500 dark:text-gray-400"
              >
                {{ t('admin.serviceQuota.targetUsersOverflow', { count: targetUsersFor(row).length - TARGET_USERS_VISIBLE_LIMIT }) }}
              </span>
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

    <RuleEditDialog
      :show="showDialog"
      :editing-rule="editingRule"
      @update:show="showDialog = $event"
      @saved="load"
    />

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
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import EnabledToggleCell from './components/EnabledToggleCell.vue'
import PathChevron from '@/components/serviceQuota/PathChevron.vue'
import RuleEditDialog from './components/RuleEditDialog.vue'
import { useEntityName } from '@/components/serviceQuota/entityNames'
import type { PathSummary } from '@/components/serviceQuota/pathRender'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { limiterChipClass } from '@/utils/limiterColors'
import { formatThousands } from '@/utils/format'
import type { Column } from '@/components/common/types'
import {
  deleteServiceQuotaRule,
  listServiceQuotaRules,
  type ServiceQuotaLimiterDef,
  type ServiceQuotaPathDef,
  type ServiceQuotaRule,
} from '@/api/admin/serviceQuota'

const { t } = useI18n()
const appStore = useAppStore()

const counterModeOptions = computed(() => [
  { value: 'user', label: t('admin.serviceQuota.counterModes.user') },
  { value: 'per_user', label: t('admin.serviceQuota.counterModes.perUser') },
  { value: 'shared', label: t('admin.serviceQuota.counterModes.shared') },
])

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('admin.serviceQuota.columns.name') },
  { key: 'limiters', label: t('admin.serviceQuota.columns.limiters') },
  { key: 'paths', label: t('admin.serviceQuota.columns.paths') },
  { key: 'counter_mode', label: t('admin.serviceQuota.columns.counterMode') },
  { key: 'target_users', label: t('admin.serviceQuota.columns.targetUsers') },
  { key: 'is_fallback', label: t('admin.serviceQuota.columns.fallback') },
  { key: 'enabled', label: t('admin.serviceQuota.columns.status') },
  { key: 'actions', label: t('admin.serviceQuota.columns.actions') },
])

// 限制单元格 chip 数量，避免一屏挤十几个用户名；超出折叠为 "等 N 人"
const TARGET_USERS_VISIBLE_LIMIT = 3

const rules = ref<ServiceQuotaRule[]>([])
const loading = ref(false)
const showDialog = ref(false)
const editingRule = ref<ServiceQuotaRule | null>(null)
const deletingRule = ref<ServiceQuotaRule | null>(null)
const filters = reactive({ counterMode: '', fallback: '', enabled: '' })

const filteredRules = computed(() => rules.value.filter((rule) => {
  if (filters.counterMode && rule.counter_mode !== filters.counterMode) return false
  if (filters.fallback && String(rule.is_fallback) !== filters.fallback) return false
  if (filters.enabled && String(rule.enabled) !== filters.enabled) return false
  return true
}))

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

function formatLimitValue(lim: ServiceQuotaLimiterDef): string {
  if (lim.limiter_type === 'daily_usd') return `$${Number(lim.limit_value).toFixed(6).replace(/\.?0+$/, '')}`
  // 整数限额（rpm/tpm/tpd/concurrency）用美式千分位逗号
  return formatThousands(Math.round(lim.limit_value))
}

function counterModeLabel(value: string): string {
  return counterModeOptions.value.find((item) => item.value === value)?.label || value
}

// 把规则的 target_users / target_user_ids 整理为 chip 数据：
//   - 后端在 target_users 里返回 {id,email}，优先用 email 当 label
//   - 老数据可能只有 target_user_ids，无 target_users，回退到 useEntityName 异步解析
//   - 异步解析的占位为 "#id"，名称回填后会自动重渲（Vue 自动追踪 ref.value）
interface TargetUserDisplay {
  id: number
  label: string
}

function targetUsersFor(rule: ServiceQuotaRule): TargetUserDisplay[] {
  if (rule.counter_mode !== 'user') return []
  // target_users 已带 email：直接使用，避免触发 N 次 useEntityName 请求
  if (rule.target_users && rule.target_users.length > 0) {
    return rule.target_users.map((u) => ({ id: u.id, label: u.email || `#${u.id}` }))
  }
  const ids = rule.target_user_ids || []
  return ids.map((id) => ({ id, label: useEntityName('user', id).value || `#${id}` }))
}

// 复用 RuntimeTable 的 PathChevron 渲染：把 ServiceQuotaPathDef 适配成 PathSummary。
// 复用 PathChevron 让监控页与配置页对"全部 nil 视为通配 / 平台 chip / chevron 链"
// 三个语义保持一致——避免一处修改另一处漂移（复用原则）。
function pathDefToSummary(path: ServiceQuotaPathDef): PathSummary {
  return {
    platform: path.platform ?? null,
    channel_id: path.channel_id ?? null,
    group_id: path.group_id ?? null,
    account_id: path.account_id ?? null,
    model_pattern: path.model_pattern ?? null,
  }
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
  editingRule.value = null
  showDialog.value = true
}

function openEdit(rule: ServiceQuotaRule) {
  editingRule.value = rule
  showDialog.value = true
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
.action-btn {
  @apply rounded-lg p-1.5 text-gray-500 transition-colors dark:text-gray-400;
}
</style>
