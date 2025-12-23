<template>
  <div>
    <label class="input-label">
      Groups
      <span class="text-gray-400 font-normal">({{ modelValue.length }} selected)</span>
    </label>
    <div
      class="grid grid-cols-2 gap-1 max-h-32 overflow-y-auto p-2 border border-gray-200 dark:border-dark-600 rounded-lg bg-gray-50 dark:bg-dark-800"
    >
      <label
        v-for="group in filteredGroups"
        :key="group.id"
        class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-white dark:hover:bg-dark-700 cursor-pointer transition-colors"
        :title="`${group.rate_multiplier}x rate · ${group.account_count || 0} accounts`"
      >
        <input
          type="checkbox"
          :value="group.id"
          :checked="modelValue.includes(group.id)"
          @change="handleChange(group.id, ($event.target as HTMLInputElement).checked)"
          class="w-3.5 h-3.5 text-primary-500 border-gray-300 dark:border-dark-500 rounded focus:ring-primary-500 shrink-0"
        />
        <GroupBadge
          :name="group.name"
          :subscription-type="group.subscription_type"
          :rate-multiplier="group.rate_multiplier"
          class="flex-1 min-w-0"
        />
        <span class="text-xs text-gray-400 shrink-0">{{ group.account_count || 0 }}</span>
      </label>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 text-center text-sm text-gray-500 dark:text-gray-400 py-2"
      >
        No groups available
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import GroupBadge from './GroupBadge.vue'
import type { Group, GroupPlatform } from '@/types'

interface Props {
  modelValue: number[]
  groups: Group[]
  platform?: GroupPlatform // Optional platform filter
}

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

// Filter groups by platform if specified
const filteredGroups = computed(() => {
  if (!props.platform) {
    return props.groups
  }
  return props.groups.filter(g => g.platform === props.platform)
})

const handleChange = (groupId: number, checked: boolean) => {
  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter(id => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>
