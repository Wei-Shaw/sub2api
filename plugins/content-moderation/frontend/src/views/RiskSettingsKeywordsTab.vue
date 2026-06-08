<template>
  <div class="space-y-5">
    <div
      class="flex items-start gap-3 rounded-lg border p-4"
      :class="ctx.keywordNotice.value.toneClass"
    >
      <Icon
        :name="ctx.keywordNotice.value.icon"
        size="md"
        :class="ctx.keywordNotice.value.iconClass"
      />
      <div class="text-sm leading-6">
        <p class="font-medium" :class="ctx.keywordNotice.value.titleClass">{{ ctx.keywordNotice.value.title }}</p>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ ctx.keywordNotice.value.description }}</p>
      </div>
    </div>

    <div class="space-y-2">
      <label class="input-label">{{ t('admin.riskControl.keywordBlockingMode') }}</label>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <button
          v-for="option in ctx.keywordBlockingModeOptions.value"
          :key="option.value"
          type="button"
          class="rounded-lg border p-3 text-left transition-colors"
          :class="ctx.configForm.keyword_blocking_mode === option.value
            ? 'border-primary-300 bg-primary-50 text-primary-900 shadow-sm dark:border-primary-700 dark:bg-primary-900/20 dark:text-primary-100'
            : 'border-gray-100 hover:bg-gray-50 dark:border-dark-700 dark:hover:bg-dark-700/60'"
          @click="ctx.configForm.keyword_blocking_mode = option.value"
        >
          <div class="flex items-center justify-between gap-2">
            <span class="text-sm font-semibold">{{ option.label }}</span>
            <span
              class="flex h-4 w-4 flex-shrink-0 items-center justify-center rounded-full border"
              :class="ctx.configForm.keyword_blocking_mode === option.value
                ? 'border-primary-500 bg-primary-500 text-white'
                : 'border-gray-300 text-transparent dark:border-dark-500'"
            >
              <Icon name="check" size="xs" :stroke-width="2" />
            </span>
          </div>
          <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ option.description }}</p>
        </button>
      </div>
    </div>

    <div>
      <div class="mb-2 flex items-center justify-between">
        <label class="input-label mb-0">{{ t('admin.riskControl.blockedKeywords') }}</label>
        <span class="inline-flex rounded-md bg-gray-100 px-2 py-1 text-xs text-gray-500 dark:bg-dark-700 dark:text-gray-300">
          {{ t('admin.riskControl.blockedKeywordCount', { count: ctx.blockedKeywordCount.value }) }}
        </span>
      </div>
      <textarea
        v-model="ctx.configForm.blocked_keywords_text"
        class="input min-h-52 resize-y font-mono text-sm"
        :placeholder="t('admin.riskControl.blockedKeywordsPlaceholder')"
        :disabled="ctx.configForm.keyword_blocking_mode === 'api_only'"
      ></textarea>
      <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.riskControl.blockedKeywordsLimit', { max: blockedKeywordMax }) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { inject } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'
import { RiskControlKey } from './riskControlContext'
import { blockedKeywordMax } from './useRiskControl'

const { t } = useI18n()
const ctx = inject(RiskControlKey)!
</script>
