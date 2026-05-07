<template>
  <div>
    <label class="input-label">
      {{ t('admin.users.groups') }}
      <span class="font-normal text-gray-400">{{ t('common.selectedCount', { count: modelValue.length }) }}</span>
    </label>
    <div
      class="grid max-h-32 grid-cols-2 gap-1 overflow-y-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <label
        v-for="group in filteredGroups"
        :key="group.id"
        class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
        :title="t('admin.groups.rateAndAccounts', { rate: group.rate_multiplier, count: group.account_count || 0 })"
      >
        <input
          type="checkbox"
          :value="group.id"
          :checked="modelValue.includes(group.id)"
          @change="handleChange(group.id, ($event.target as HTMLInputElement).checked)"
          class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
        />
        <GroupBadge
          :name="group.name"
          :platform="group.platform"
          :subscription-type="group.subscription_type"
          :rate-multiplier="group.rate_multiplier"
          class="min-w-0 flex-1"
        />
        <span class="shrink-0 text-xs text-gray-400">{{ group.account_count || 0 }}</span>
      </label>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('common.noGroupsAvailable') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupBadge from './GroupBadge.vue'
import type { AdminGroup, GroupPlatform } from '@/types'
import { usePlatforms } from '@/composables/usePlatforms'

const { t } = useI18n()
const { platforms } = usePlatforms()

interface Props {
  modelValue: number[]
  groups: AdminGroup[]
  platform?: GroupPlatform // Optional platform filter
  mixedScheduling?: boolean // For antigravity accounts: allow anthropic/gemini groups
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

// Platforms compatible with antigravity mixed scheduling.
// TODO: replace with plugin declaration compatible_platforms when available.
const FALLBACK_MIXED_PLATFORMS = ['antigravity', 'anthropic', 'gemini']

const mixedSchedulingPlatforms = computed<string[]>(() => {
  // Read all known platform IDs dynamically; fall back to hardcoded list
  if (platforms.value.length > 0) {
    return platforms.value.map((p) => p.platform)
  }
  return FALLBACK_MIXED_PLATFORMS
})

// Filter groups by platform if specified
const filteredGroups = computed(() => {
  if (!props.platform) {
    return props.groups
  }
  // antigravity 账户启用混合调度后，可选择 compatible 平台的分组
  if (props.platform === 'antigravity' && props.mixedScheduling) {
    const allowed = new Set(mixedSchedulingPlatforms.value)
    return props.groups.filter((g) => allowed.has(g.platform))
  }
  // 默认：只能选择同 platform 的分组
  return props.groups.filter((g) => g.platform === props.platform)
})

const handleChange = (groupId: number, checked: boolean) => {
  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>
