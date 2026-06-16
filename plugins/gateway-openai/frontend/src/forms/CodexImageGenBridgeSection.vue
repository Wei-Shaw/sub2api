<template>
  <div class="overflow-hidden rounded-lg border border-sky-100 bg-sky-50/60 shadow-sm dark:border-sky-900/50 dark:bg-sky-950/20">
    <div class="flex items-start gap-3 px-4 py-3">
      <div class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-white text-sky-600 shadow-sm ring-1 ring-sky-100 dark:bg-dark-800 dark:text-sky-300 dark:ring-sky-900/60">
        <Icon name="sparkles" size="sm" />
      </div>
      <div class="min-w-0 flex-1">
        <div class="flex flex-wrap items-center gap-2">
          <label class="input-label mb-0">{{ t('admin.accounts.openai.codexImageGenerationBridge') }}</label>
          <span
            class="rounded-full px-2 py-0.5 text-[11px] font-medium"
            :class="badgeClass"
          >
            {{ badgeLabel }}
          </span>
        </div>
        <p class="mt-1 text-xs leading-5 text-slate-600 dark:text-slate-300">
          {{ t('admin.accounts.openai.codexImageGenerationBridgeDesc') }}
        </p>
      </div>
    </div>
    <div class="border-t border-sky-100 bg-white/70 p-2 dark:border-sky-900/50 dark:bg-dark-800/70">
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <button
          v-for="option in options"
          :key="option.value"
          type="button"
          @click="$emit('update:mode', option.value)"
          :class="[
            'group flex min-h-[68px] items-start gap-2 rounded-md border px-3 py-2 text-left transition-all',
            mode === option.value
              ? 'border-sky-300 bg-sky-50 text-sky-900 shadow-sm ring-1 ring-sky-200 dark:border-sky-700 dark:bg-sky-900/25 dark:text-sky-100 dark:ring-sky-800'
              : 'border-transparent bg-transparent text-slate-600 hover:border-gray-200 hover:bg-gray-50 dark:text-slate-300 dark:hover:border-dark-500 dark:hover:bg-dark-700'
          ]"
        >
          <span
            :class="[
              'mt-0.5 flex h-5 w-5 shrink-0 items-center justify-center rounded-full border transition-colors',
              mode === option.value
                ? 'border-sky-500 bg-sky-500 text-white'
                : 'border-gray-300 text-transparent group-hover:border-gray-400 dark:border-dark-500'
            ]"
          >
            <Icon name="check" size="xs" :stroke-width="2" />
          </span>
          <span class="min-w-0">
            <span class="block text-sm font-medium">{{ option.label }}</span>
            <span class="mt-0.5 block text-xs leading-4 text-slate-500 dark:text-slate-400">{{ option.description }}</span>
          </span>
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'

type CodexImageGenBridgeMode = 'inherit' | 'enabled' | 'disabled'

defineProps<{
  mode: CodexImageGenBridgeMode
  options: Array<{ value: CodexImageGenBridgeMode; label: string; description: string }>
  badgeLabel: string
  badgeClass: string
}>()

defineEmits<{
  'update:mode': [value: CodexImageGenBridgeMode]
}>()

const { t } = useI18n()
</script>