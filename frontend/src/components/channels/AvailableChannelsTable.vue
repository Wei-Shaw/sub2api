<template>
  <DataTable :columns="columns" :data="rows" :loading="loading">
    <template #cell-name="{ row }">
      <div class="font-medium text-gray-900 dark:text-white">{{ row.name }}</div>
      <div
        v-if="row.description"
        class="mt-0.5 text-xs text-gray-500 dark:text-gray-400"
      >
        {{ row.description }}
      </div>
    </template>

    <template #cell-groups="{ row }">
      <div v-if="row.groups.length === 0" class="text-xs text-gray-400">
        <slot name="empty-groups">-</slot>
      </div>
      <div v-else class="flex flex-wrap gap-1">
        <span
          v-for="g in row.groups"
          :key="g.id"
          class="inline-flex items-center rounded bg-blue-50 px-2 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900/30 dark:text-blue-300"
        >
          {{ g.name }}
        </span>
      </div>
    </template>

    <template #cell-supported_models="{ row }">
      <div v-if="row.supported_models.length === 0" class="text-xs text-gray-400">
        {{ noModelsLabel }}
      </div>
      <div v-else class="flex max-w-[560px] flex-wrap gap-1">
        <SupportedModelChip
          v-for="m in row.supported_models"
          :key="`${m.platform}-${m.name}`"
          :model="m"
          :pricing-key-prefix="pricingKeyPrefix"
          :no-pricing-label="noPricingLabel"
        />
      </div>
    </template>

    <!-- 允许父组件为额外列提供自定义渲染（如 admin 的 status / billing_model_source）。 -->
    <template v-for="slot in extraCellSlots" :key="slot" #[slot]="scope">
      <slot :name="slot" v-bind="scope" />
    </template>

    <template #empty>
      <slot name="empty">
        <div class="flex flex-col items-center py-8">
          <Icon name="inbox" size="xl" class="mb-3 h-12 w-12 text-gray-400" />
          <p class="text-sm text-gray-500 dark:text-gray-400">{{ emptyLabel }}</p>
        </div>
      </slot>
    </template>
  </DataTable>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import DataTable from '@/components/common/DataTable.vue'
import Icon from '@/components/icons/Icon.vue'
import SupportedModelChip from './SupportedModelChip.vue'
import type { UserSupportedModel } from '@/api/channels'
import type { ChannelStatus, BillingModelSource } from '@/constants/channel'

interface GroupRef {
  id: number
  name: string
  platform?: string
}

interface Row {
  name: string
  description?: string
  groups: GroupRef[]
  // 复用 user 侧最小 DTO；admin 侧 SupportedModel 结构上是其超集，可直接传入。
  supported_models: UserSupportedModel[]
  // admin 独有字段：用精确类型代替 `unknown`，让消费端无需 `as` 断言，
  // 也能在后端新增 union 成员时让前端 Record 查表立刻出空而非崩溃。
  status?: ChannelStatus
  billing_model_source?: BillingModelSource
}

interface Column {
  key: string
  label: string
}

defineProps<{
  columns: Column[]
  rows: Row[]
  loading: boolean
  pricingKeyPrefix: string
  noPricingLabel: string
  noModelsLabel: string
  emptyLabel: string
}>()

const slots = useSlots()
/**
 * 透传父组件提供的 cell-* 插槽（除本组件内置的 name/groups/supported_models/empty-groups/empty
 * 之外），让 admin 场景可以自定义 status / billing_model_source 等列。
 */
const extraCellSlots = computed(() => {
  const reserved = new Set(['cell-name', 'cell-groups', 'cell-supported_models', 'empty-groups', 'empty'])
  return Object.keys(slots).filter((name) => name.startsWith('cell-') && !reserved.has(name))
})
</script>
