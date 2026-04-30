<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex w-full items-start justify-between gap-3">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.serviceQuotaMonitor.title') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.serviceQuotaMonitor.description') }}
            </p>
          </div>
          <div class="flex shrink-0 items-center gap-2">
            <span v-if="snapshot" class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.serviceQuotaMonitor.asOf', { seconds: secondsSinceUpdate }) }}
            </span>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="loading"
              :title="t('admin.serviceQuotaMonitor.refresh')"
              @click="manualRefresh"
            >
              <Icon name="refresh" :class="{ 'animate-spin': loading }" size="sm" class="mr-1" />
              {{ t('admin.serviceQuotaMonitor.refresh') }}
            </button>
            <AutoRefreshButton
              :enabled="autoEnabled"
              :interval-seconds="autoInterval"
              :countdown="countdown"
              :intervals="REFRESH_INTERVALS"
              @update:enabled="setAutoEnabled"
              @update:interval="setAutoInterval"
            />
          </div>
        </div>
      </template>

      <template #filters>
        <div class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800">
          <FilterBar :model-value="filter" @update:model-value="onFilterChange" />
          <div class="mt-3 rounded-lg bg-blue-50 px-3 py-2 text-xs text-blue-700 dark:bg-blue-900/20 dark:text-blue-300">
            {{ scopeHint }}
          </div>
          <div v-if="errorMessage" class="mt-3 rounded-lg bg-red-50 px-3 py-2 text-xs text-red-600 dark:bg-red-900/20 dark:text-red-400">
            {{ errorMessage }}
          </div>
          <div v-else-if="snapshot && !snapshot.enabled" class="mt-3 rounded-lg bg-yellow-50 px-3 py-2 text-xs text-yellow-700 dark:bg-yellow-900/20 dark:text-yellow-400">
            {{ t('admin.serviceQuotaMonitor.disabled') }}
          </div>
          <div v-else-if="snapshot?.truncated" class="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-400">
            {{ t('admin.serviceQuotaMonitor.truncated', { count: rows.length }) }}
          </div>
        </div>
      </template>

      <template #table>
        <QuotaMonitorTable
          :rows="rows"
          :loading="loading"
          :show-internal="true"
          @reset="onResetRow"
          @refresh="onRefreshRow"
        />
      </template>
    </TablePageLayout>

    <ConfirmDialog
      :show="!!pendingReset"
      :title="t('admin.serviceQuotaMonitor.resetConfirmTitle')"
      :message="resetConfirmMessage"
      :confirm-text="t('admin.serviceQuotaMonitor.reset')"
      :cancel-text="t('common.cancel')"
      :danger="true"
      @confirm="confirmReset"
      @cancel="pendingReset = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import AutoRefreshButton from '@/components/common/AutoRefreshButton.vue'
import FilterBar from './components/FilterBar.vue'
import QuotaMonitorTable from '@/components/serviceQuota/QuotaMonitorTable.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import {
  getServiceQuotaMonitorSnapshot,
  resetServiceQuotaCounter,
  type LimiterRuntime,
  type ServiceQuotaMonitorFilter,
} from '@/api/admin/serviceQuota'
import { useQuotaMonitorPolling } from '@/components/serviceQuota/composables/useQuotaMonitorPolling'
import { useAppStore } from '@/stores/app'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const REFRESH_INTERVALS = [1, 5, 10, 30, 60] as const
const DEFAULT_INTERVAL = 5

const errorMessage = ref('')
const filter = ref<ServiceQuotaMonitorFilter>({})

function onFilterChange(next: ServiceQuotaMonitorFilter): void {
  filter.value = next
}

const {
  snapshot,
  loading,
  autoEnabled,
  autoInterval,
  countdown,
  secondsSinceUpdate,
  loadSnapshot,
  setAutoEnabled,
  setAutoInterval,
  start,
  stop,
} = useQuotaMonitorPolling({
  defaultIntervalSeconds: DEFAULT_INTERVAL,
  // 单一端点：手动刷新整体替换 items；轮询走 mergeSnapshotItems 原地更新动态字段
  fetchSnapshot: () => getServiceQuotaMonitorSnapshot({ ...filter.value }),
  onError: (err) => {
    errorMessage.value = extractI18nErrorMessage(
      err,
      t,
      'common.errors',
      t('admin.serviceQuotaMonitor.loadError'),
    )
    appStore.showError(errorMessage.value)
  },
  // 轮询失败静默：让上一次成功的快照继续展示，不打断用户视觉
})

const rows = computed<LimiterRuntime[]>(() => snapshot.value?.items ?? [])

// scopeHint 根据 filter.user_id 是否存在切换文案：
//   - 未选用户：解释能看到"指定用户" + "全局共享" 规则；选择用户后可看 per_user
//   - 已选用户：解释当前展示该用户被指定的规则、shared、以及该用户的 per_user 计数
// 与监控展开语义（scopeUsersForRule）一一对应，避免用户对"为什么 per_user 没显示"困惑。
const scopeHint = computed<string>(() => {
  return filter.value.user_id
    ? t('admin.serviceQuotaMonitor.scopeHintWithUser')
    : t('admin.serviceQuotaMonitor.scopeHintNoUser')
})

// 重置流程：RuntimeTable emit('reset', row) → 弹 confirm → POST → 立刻刷新一次
const pendingReset = ref<LimiterRuntime | null>(null)

const resetConfirmMessage = computed<string>(() => {
  const r = pendingReset.value
  if (!r) return ''
  return t('admin.serviceQuotaMonitor.resetConfirmMessage', {
    rule: r.rule_name || `#${r.rule_id}`,
    limiter: (r.limiter_type || '').toUpperCase(),
  })
})

function onResetRow(row: LimiterRuntime): void {
  pendingReset.value = row
}

// 单行 refresh 等同于"整页刷新"：当前后端 Snapshot 是 batch 接口，
// 单条与全表成本一样，保持简单。
function onRefreshRow(_row: LimiterRuntime): void {
  manualRefresh()
}

async function manualRefresh(): Promise<void> {
  errorMessage.value = ''
  await loadSnapshot()
}

async function confirmReset(): Promise<void> {
  const row = pendingReset.value
  pendingReset.value = null
  if (!row) return
  try {
    await resetServiceQuotaCounter({
      rule_id: row.rule_id,
      path_id: row.path_id,
      limiter_type: row.limiter_type,
      scope_user_id: row.scope_user_id ?? null,
    })
    appStore.showSuccess(t('admin.serviceQuotaMonitor.resetSuccess'))
    await manualRefresh()
  } catch (err: unknown) {
    appStore.showError(
      extractI18nErrorMessage(err, t, 'common.errors', t('admin.serviceQuotaMonitor.resetError')),
    )
  }
}

// filter 变化 = "配置过滤范围变了" → 走全量端点重新拉规则元信息
watch(filter, () => {
  manualRefresh()
}, { deep: true })

onMounted(() => {
  manualRefresh()
  start()
})

onBeforeUnmount(() => {
  stop()
})
</script>
