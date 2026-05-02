<template>
  <div class="flex flex-wrap gap-2">
    <label
      v-for="p in platforms"
      :key="p"
      class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm transition-colors"
      :class="modelValue === p
        ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700'
        : 'border-gray-200 hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700'"
    >
      <input
        type="checkbox"
        :checked="modelValue === p"
        class="h-3.5 w-3.5 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
        @change="toggle(p)"
      />
      <PlatformIcon :platform="p as GroupPlatform" size="xs" :class="platformTextClass(p)" />
      <span :class="platformTextClass(p)">{{ t('admin.groups.platforms.' + p, p) }}</span>
    </label>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import { platformTextClass } from '@/utils/platformColors'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()

const props = defineProps<{
  modelValue: string | null | undefined
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | null): void
}>()

const platforms = ['anthropic', 'openai', 'gemini', 'antigravity']

function toggle(p: string) {
  if (props.modelValue === p) {
    emit('update:modelValue', null)
  } else {
    emit('update:modelValue', p)
  }
}
</script>
