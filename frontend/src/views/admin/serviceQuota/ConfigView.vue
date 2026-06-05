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
            <!-- 操作样式与监控页 RuntimeTable 一致：图标在上 / 文字在下，灰色基调，
                 编辑 hover 切蓝色、删除 hover 切红色（与 AccountsView 同款） -->
            <div class="flex items-center gap-1">
              <button
                type="button"
                class="action-btn flex flex-col items-center gap-0.5 hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                :title="t('common.edit')"
                @click="openEdit(row)"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit') }}</span>
              </button>
              <button
                type="button"
                class="action-btn flex flex-col items-center gap-0.5 hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                :title="t('common.delete')"
                @click="askDelete(row)"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete') }}</span>
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
      @cancel="cancelDelete"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { limiterChipClass } from '@/utils/limiterColors'
import type { Column } from '@/components/common/types'
import {
  listServiceQuotaRules,
  type ServiceQuotaRule,
} from '@/api/admin/serviceQuota'
import { useServiceQuotaFilters } from './composables/useServiceQuotaFilters'
import { useRuleDeleteDialog } from './composables/useRuleDeleteDialog'
import { useServiceQuotaDisplay } from './composables/useServiceQuotaDisplay'

const { t } = useI18n()
const appStore = useAppStore()

const {
  counterModeOptions,
  limiterLabel,
  formatLimitValue,
  counterModeLabel,
  targetUsersFor,
  pathDefToSummary,
} = useServiceQuotaDisplay()

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

const { filters, filteredRules } = useServiceQuotaFilters(rules)
const { deletingRule, askDelete, cancelDelete, confirmDelete } = useRuleDeleteDialog(load)

async function load() {
  loading.value = true
  try {
    rules.value = await listServiceQuotaRules()
  } catch (error: unknown) {
    appStore.showError(
      extractI18nErrorMessage(error, t, 'common.errors', t('admin.serviceQuota.loadError')),
    )
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

onMounted(load)
</script>

<style scoped>
.action-btn {
  @apply rounded-lg p-1.5 text-gray-500 transition-colors dark:text-gray-400;
}
</style>
