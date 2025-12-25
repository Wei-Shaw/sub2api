<template>
  <div>
    <label class="input-label">
      Groups
      <span class="font-normal text-gray-400">({{ modelValue.length REDACTEDREDACTED selected)</span>
    </label>
    <div
      class="grid max-h-32 grid-cols-2 gap-1 overflow-y-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <label
        v-for="group in filteredGroups"
        :key="group.id"
        class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
        :title="`${group.rate_multiplierREDACTEDx rate · ${group.account_count || 0REDACTED accounts`"
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
          :subscription-type="group.subscription_type"
          :rate-multiplier="group.rate_multiplier"
          class="min-w-0 flex-1"
        />
        <span class="shrink-0 text-xs text-gray-400">{{ group.account_count || 0 REDACTEDREDACTED</span>
      </label>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        No groups available
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed REDACTED from 'vue'
import GroupBadge from './GroupBadge.vue'
import type { Group, GroupPlatform REDACTED from '@/types'

interface Props {
  modelValue: number[]
  groups: Group[]
  platform?: GroupPlatform // Optional platform filter
REDACTED

const props = defineProps<Props>()
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
REDACTED>()

// Filter groups by platform if specified
const filteredGroups = computed(() => {
  if (!props.platform) {
    return props.groups
  REDACTED
  return props.groups.filter((g) => g.platform === props.platform)
REDACTED)

const handleChange = (groupId: number, checked: boolean) => {
  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
REDACTED
</script>
