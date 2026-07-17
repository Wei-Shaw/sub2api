<template>
  <div class="border-t pt-4">
    <div>
      <label class="input-label">{{ t('admin.groups.autoRouting.title') }}</label>
      <p class="input-hint">{{ t('admin.groups.autoRouting.hint') }}</p>
    </div>

    <div
      class="mt-3 rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800"
    >
      <div class="mb-2 flex items-center justify-between gap-3">
        <span class="text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t('admin.groups.autoRouting.candidates') }}
        </span>
        <span class="text-xs text-gray-400">
          {{ t('common.selectedCount', { count: candidateGroupIds.length }) }}
        </span>
      </div>
      <div class="grid max-h-40 grid-cols-1 gap-1 overflow-y-auto sm:grid-cols-2">
        <label
          v-for="group in candidateGroups"
          :key="group.id"
          class="flex cursor-pointer items-center gap-2 rounded px-2 py-2 hover:bg-white dark:hover:bg-dark-700"
        >
          <input
            type="checkbox"
            :value="group.id"
            :checked="candidateGroupIds.includes(group.id)"
            class="h-4 w-4 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
            @change="updateCandidate(group.id, ($event.target as HTMLInputElement).checked)"
          />
          <span class="min-w-0 flex-1 truncate text-sm text-gray-700 dark:text-gray-300">
            {{ group.name }}
          </span>
          <span class="shrink-0 text-xs text-gray-400">{{ group.rate_multiplier }}x</span>
        </label>
        <p
          v-if="candidateGroups.length === 0"
          class="py-3 text-center text-sm text-gray-500 dark:text-gray-400 sm:col-span-2"
        >
          {{ t('admin.groups.autoRouting.noCandidates') }}
        </p>
      </div>
      <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.groups.autoRouting.balanceOnlyHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import type { AdminGroup } from '@/types'
import { filterAutoRoutingCandidates } from '@/views/admin/groupsAutoRouting'

const props = defineProps<{
  candidateGroupIds: number[]
  groups: AdminGroup[]
  currentGroupId?: number
}>()

const emit = defineEmits<{
  'update:candidateGroupIds': [value: number[]]
}>()

const { t } = useI18n()

const candidateGroups = computed(() =>
  filterAutoRoutingCandidates(props.groups, props.currentGroupId)
)

const updateCandidate = (groupId: number, checked: boolean) => {
  const nextIds = checked
    ? [...new Set([...props.candidateGroupIds, groupId])]
    : props.candidateGroupIds.filter((id) => id !== groupId)
  emit('update:candidateGroupIds', nextIds)
}
</script>
