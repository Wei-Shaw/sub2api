<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
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
                :placeholder="t('admin.availableChannels.searchPlaceholder')"
                class="input pl-10"
              />
            </div>
          </div>

          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadChannels"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <AvailableChannelsTable
          :columns="columns"
          :rows="filteredChannels"
          :loading="loading"
          pricing-key-prefix="admin.availableChannels.pricing"
          :no-pricing-label="t('admin.availableChannels.noPricing')"
          :no-models-label="t('admin.availableChannels.noModels')"
          :empty-label="t('admin.availableChannels.empty')"
        >
          <template #empty-groups>{{ t('admin.availableChannels.noGroups') }}</template>

          <template #cell-status="{ row }">
            <span
              :class="statusStyleOf(row.status).cls"
              class="inline-flex items-center rounded px-2 py-0.5 text-xs font-medium"
            >
              {{ statusStyleOf(row.status).label }}
            </span>
          </template>

          <template #cell-billing_model_source="{ row }">
            <span class="text-xs text-gray-700 dark:text-gray-300">
              {{ billingSourceLabelOf(row.billing_model_source) }}
            </span>
          </template>
        </AvailableChannelsTable>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import AvailableChannelsTable from '@/components/channels/AvailableChannelsTable.vue'
import channelsAPI, { type AvailableChannel } from '@/api/admin/channels'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  CHANNEL_STATUS_ACTIVE,
  CHANNEL_STATUS_DISABLED,
  BILLING_MODEL_SOURCE_REQUESTED,
  BILLING_MODEL_SOURCE_UPSTREAM,
  BILLING_MODEL_SOURCE_CHANNEL_MAPPED,
  type ChannelStatus,
  type BillingModelSource
} from '@/constants/channel'

const { t } = useI18n()
const appStore = useAppStore()

const channels = ref<AvailableChannel[]>([])
const loading = ref(false)
const searchQuery = ref('')

const columns = computed(() => [
  { key: 'name', label: t('admin.availableChannels.columns.name') },
  { key: 'status', label: t('admin.availableChannels.columns.status') },
  { key: 'billing_model_source', label: t('admin.availableChannels.columns.billingSource') },
  { key: 'groups', label: t('admin.availableChannels.columns.groups') },
  { key: 'supported_models', label: t('admin.availableChannels.columns.supportedModels') }
])

/**
 * 显示样式：i18n label + Tailwind class，按 ChannelStatus 完整穷举。
 * Record 键类型强制未来新增 ChannelStatus 成员时 TS 编译失败，避免遗漏分支。
 */
const statusStyles = computed<Record<ChannelStatus, { label: string; cls: string }>>(() => ({
  [CHANNEL_STATUS_ACTIVE]: {
    label: t('admin.availableChannels.statusActive'),
    cls: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400'
  },
  [CHANNEL_STATUS_DISABLED]: {
    label: t('admin.availableChannels.statusDisabled'),
    cls: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
  }
}))

/**
 * BillingModelSource 显式映射：避免将后端 snake_case 字面量直接拼成 i18n key，
 * 同时在 BillingModelSource 扩展时 TS 编译失败以暴露遗漏。
 */
const billingSourceLabels = computed<Record<BillingModelSource, string>>(() => ({
  [BILLING_MODEL_SOURCE_REQUESTED]: t('admin.availableChannels.billingSource.requested'),
  [BILLING_MODEL_SOURCE_UPSTREAM]: t('admin.availableChannels.billingSource.upstream'),
  [BILLING_MODEL_SOURCE_CHANNEL_MAPPED]: t('admin.availableChannels.billingSource.channel_mapped')
}))

// 运行时兜底：即便 service 层归一化漏点或后端新增未同步的 enum 值传入，
// 也不会触发 undefined.cls 崩溃；统一降级为 "-"。
const DEFAULT_STATUS_STYLE = { label: '-', cls: '' }
const DEFAULT_BILLING_LABEL = '-'

function statusStyleOf(status: ChannelStatus | undefined): { label: string; cls: string } {
  return status ? statusStyles.value[status] : DEFAULT_STATUS_STYLE
}

function billingSourceLabelOf(src: BillingModelSource | undefined): string {
  return src ? billingSourceLabels.value[src] : DEFAULT_BILLING_LABEL
}

const filteredChannels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  if (!q) return channels.value
  return channels.value.filter((ch) => {
    if (ch.name.toLowerCase().includes(q)) return true
    if ((ch.description || '').toLowerCase().includes(q)) return true
    if (ch.groups.some((g) => g.name.toLowerCase().includes(q))) return true
    if (ch.supported_models.some((m) => m.name.toLowerCase().includes(q))) return true
    return false
  })
})

async function loadChannels() {
  loading.value = true
  try {
    channels.value = await channelsAPI.listAvailable()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    loading.value = false
  }
}

onMounted(loadChannels)
</script>
