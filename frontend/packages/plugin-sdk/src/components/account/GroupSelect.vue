<template>
  <div>
    <label class="input-label">
      {{ t('admin.accounts.groups') }}
      <span class="font-normal text-gray-400">{{ t('admin.accounts.selectedGroups', { count: modelValue.length }) }}</span>
    </label>
    <div
      v-if="isSearchable"
      class="flex items-center gap-2 rounded-t-lg border border-b-0 border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
      <input
        v-model="searchText"
        type="text"
        :placeholder="t('common.searchPlaceholder')"
        class="flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-gray-100 dark:placeholder:text-dark-400"
      />
    </div>
    <div
      :class="[
        'grid max-h-32 grid-cols-2 gap-1 overflow-y-auto p-2',
        isSearchable
          ? 'rounded-b-lg border border-t-0 border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
          : 'rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
      ]"
    >
      <label
        v-for="group in filteredGroups"
        :key="group.id"
        class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
      >
        <input
          type="checkbox"
          :value="group.id"
          :checked="modelValue.includes(group.id)"
          class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
          @change="handleChange(group.id, ($event.target as HTMLInputElement).checked)"
        />
        <!-- Inline GroupBadge: platform icon + name + rate -->
        <span :class="['inline-flex min-w-0 flex-1 items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium', badgeClass(group.platform)]">
          <PlatformIcon v-if="group.platform" :platform="group.platform" size="sm" />
          <span class="truncate">{{ group.name }}</span>
          <span v-if="group.rate_multiplier != null && group.rate_multiplier !== 1"
            class="rounded bg-black/10 px-1.5 py-0.5 text-[10px] font-semibold dark:bg-white/10">
            {{ group.rate_multiplier }}x
          </span>
        </span>
        <span class="shrink-0 text-xs text-gray-400">{{ group.account_count || 0 }}</span>
      </label>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.noGroupsAvailable') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon, PlatformIcon } from '@sub2api/plugin-sdk'
import type { SdkGroup } from '../../account-form-types'

const { t } = useI18n()

const BADGE_COLORS: Record<string, string> = {
  anthropic: 'bg-amber-50 text-amber-700 dark:bg-amber-900/20 dark:text-amber-400',
  openai: 'bg-green-50 text-green-700 dark:bg-green-900/20 dark:text-green-400',
  gemini: 'bg-sky-50 text-sky-700 dark:bg-sky-900/20 dark:text-sky-400',
  antigravity: 'bg-purple-50 text-purple-700 dark:bg-purple-900/20 dark:text-purple-400',
}
const BADGE_DEFAULT = 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'

function badgeClass(platform?: string): string {
  if (!platform) return BADGE_DEFAULT
  return BADGE_COLORS[platform] ?? BADGE_DEFAULT
}

interface Props {
  modelValue: number[]
  groups: SdkGroup[]
  searchable?: boolean | 'auto'
}

const props = withDefaults(defineProps<Props>(), {
  searchable: 'auto'
})

const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const searchText = ref('')

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return props.groups.length > 5
  return props.searchable
})

const filteredGroups = computed(() => {
  if (!isSearchable.value || !searchText.value) return props.groups
  const q = searchText.value.toLowerCase()
  return props.groups.filter((g) => g.name.toLowerCase().includes(q))
})

const handleChange = (groupId: number, checked: boolean) => {
  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>
