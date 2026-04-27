<template>
  <!-- V5 W7.3 — User-facing read-only Channel Status view.
       简化版本: 09fd83ab host 的 ChannelStatusView 用 Hero + CardGrid + Timeline
       chart + DetailDialog 重渲染, 总计 ~20 个组件 ~1500+ 行. plugin 端只保留
       表格主干 (search + 平台筛选 + 列出可用率/延迟). 不带 chart, 不带自动刷新,
       不带详情弹窗 -- 这些全部依赖 host 内部 composable (useAutoRefresh,
       cachedPublicSettings) 与重型 timeline 组件, 后续 V5+ 可再补.
       此页面已经覆盖了核心信息密度: 渠道 -> 平台 -> 主模型 -> 7d 可用率 -> 延迟. -->
  <div class="plugin-channels-layout">
    <div class="layout-section-fixed">
      <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
        <div class="flex flex-1 flex-wrap items-center gap-3">
          <div class="relative w-full sm:w-80">
            <Icon
              name="search"
              size="md"
              class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
            />
            <input
              v-model="searchQuery"
              type="text"
              :placeholder="t('channelStatus.searchPlaceholder')"
              class="input pl-10"
            />
          </div>
          <Select
            v-model="providerFilter"
            :options="providerFilterOptions"
            :placeholder="t('channelStatus.allProviders')"
            class="w-44"
          />
        </div>
        <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
          <button
            @click="reload"
            :disabled="loading"
            class="btn btn-secondary"
            :title="t('common.refresh', 'Refresh')"
          >
            <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
          </button>
        </div>
      </div>
    </div>

    <div class="layout-section-scrollable">
      <div class="card table-scroll-container">
        <DataTable :columns="columns" :data="filteredItems" :loading="loading">
          <template #cell-name="{ row }">
            <div class="flex flex-col">
              <span class="font-medium text-gray-900 dark:text-white">{{ row.name }}</span>
              <span v-if="row.group_name" class="text-xs text-gray-500 dark:text-gray-400">
                {{ row.group_name }}
              </span>
            </div>
          </template>

          <template #cell-provider="{ row }">
            <span
              class="inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium"
              :class="providerBadgeClass(row.provider)"
            >
              {{ providerLabel(row.provider) }}
            </span>
          </template>

          <template #cell-primary_model="{ row }">
            <div class="flex flex-col">
              <span class="text-sm text-gray-900 dark:text-gray-100">{{ row.primary_model }}</span>
              <span
                v-if="row.primary_status"
                class="mt-0.5 inline-flex w-fit items-center rounded px-1.5 py-0.5 text-[10px] font-medium"
                :class="statusBadgeClass(row.primary_status)"
              >
                {{ statusLabel(row.primary_status) }}
              </span>
            </div>
          </template>

          <template #cell-availability_7d="{ row }">
            <span class="text-sm text-gray-900 dark:text-gray-100">
              {{ formatPercent(row.availability_7d) }}
            </span>
          </template>

          <template #cell-latency="{ row }">
            <span class="text-sm text-gray-900 dark:text-gray-100">
              {{ formatLatency(row.primary_latency_ms) }}
            </span>
          </template>

          <template #empty>
            <EmptyState
              :title="t('channelStatus.empty.title')"
              :description="t('channelStatus.empty.description')"
            />
          </template>
        </DataTable>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
/**
 * V5 W7.3 — Read-only Channel Status table for end users.
 */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  DataTable,
  EmptyState,
  Icon,
  Select,
  type Column,
} from '@sub2api/plugin-sdk'
import {
  channelMonitorUserAPI,
  type UserMonitorView,
} from '../../api/user/channelMonitor'
import type { Provider } from '../../api/admin/channelMonitor'
import { useChannelMonitorFormat } from '../../composables/useChannelMonitorFormat'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
} from '../../utils/channelMonitorConstants'
import { getSdk } from '../../api/sdk'

const { t } = useI18n()
const sdk = getSdk()
const {
  providerLabel,
  providerBadgeClass,
  statusLabel,
  statusBadgeClass,
  formatLatency,
  formatPercent,
} = useChannelMonitorFormat()

const items = ref<UserMonitorView[]>([])
const loading = ref(false)
const searchQuery = ref('')
const providerFilter = ref<Provider | ''>('')

let abortController: AbortController | null = null

const columns = computed<Column[]>(() => [
  { key: 'name', label: t('channelStatus.columns.name'), sortable: false },
  { key: 'provider', label: t('channelStatus.columns.provider'), sortable: false },
  { key: 'primary_model', label: t('channelStatus.columns.primaryModel'), sortable: false },
  { key: 'availability_7d', label: t('channelStatus.columns.availability7d'), sortable: false },
  { key: 'latency', label: t('channelStatus.columns.latency'), sortable: false },
])

const providerFilterOptions = computed(() => [
  { value: '', label: t('channelStatus.allProviders') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
])

const filteredItems = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  const provider = providerFilter.value
  return items.value.filter((it) => {
    if (provider && it.provider !== provider) return false
    if (!q) return true
    return (
      it.name.toLowerCase().includes(q) ||
      it.primary_model.toLowerCase().includes(q) ||
      (it.group_name || '').toLowerCase().includes(q)
    )
  })
})

function extractMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message
    if (typeof m === 'string' && m) return m
  }
  return fallback
}

async function reload() {
  if (abortController) abortController.abort()
  const ctrl = new AbortController()
  abortController = ctrl
  loading.value = true
  try {
    const res = await channelMonitorUserAPI.list({ signal: ctrl.signal })
    if (ctrl.signal.aborted || abortController !== ctrl) return
    items.value = res.items || []
  } catch (err: unknown) {
    const e = err as { name?: string; code?: string }
    if (e?.name === 'AbortError' || e?.code === 'ERR_CANCELED') return
    sdk.notify.error(extractMessage(err, t('channelStatus.loadError')))
  } finally {
    if (abortController === ctrl) {
      loading.value = false
      abortController = null
    }
  }
}

onMounted(reload)
onBeforeUnmount(() => {
  abortController?.abort()
})
</script>
