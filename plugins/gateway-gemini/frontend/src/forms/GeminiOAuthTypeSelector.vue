<template>
  <div class="mt-4">
    <label class="input-label">{{ t('admin.accounts.oauth.gemini.oauthTypeLabel') }}</label>
    <div class="mt-2 grid grid-cols-2 gap-3">
      <!-- Google One -->
      <button type="button" @click="emit('select', 'google_one')"
        :class="['flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
          oauthType === 'google_one'
            ? 'border-purple-500 bg-purple-50 dark:bg-purple-900/20'
            : 'border-gray-200 hover:border-purple-300 dark:border-dark-600 dark:hover:border-purple-700']">
        <div :class="['flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
          oauthType === 'google_one' ? 'bg-purple-500 text-white' : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400']">
          <Icon name="user" size="sm" />
        </div>
        <div class="min-w-0">
          <span class="block text-sm font-medium text-gray-900 dark:text-white">Google One</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.gemini.oauthType.googleOneDesc') || 'Personal accounts' }}</span>
        </div>
      </button>

      <!-- GCP Code Assist -->
      <button type="button" @click="emit('select', 'code_assist')"
        :class="['flex items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
          oauthType === 'code_assist'
            ? 'border-blue-500 bg-blue-50 dark:bg-blue-900/20'
            : 'border-gray-200 hover:border-blue-300 dark:border-dark-600 dark:hover:border-blue-700']">
        <div :class="['flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
          oauthType === 'code_assist' ? 'bg-blue-500 text-white' : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400']">
          <Icon name="cloud" size="sm" />
        </div>
        <div class="min-w-0">
          <span class="block text-sm font-medium text-gray-900 dark:text-white">GCP Code Assist</span>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.gemini.oauthType.codeAssistDesc') || 'Enterprise, requires GCP project' }}</span>
          <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">
            <a :href="helpLinks.gcpProject" class="text-semantic-info ml-1 hover:underline"
              target="_blank" rel="noreferrer">
              {{ t('admin.accounts.gemini.oauthType.gcpProjectLink') }}
            </a>
          </div>
        </div>
      </button>
    </div>

    <!-- Advanced toggle -->
    <div class="mt-3">
      <button type="button" @click="emit('toggle-advanced')"
        class="flex items-center gap-2 text-sm text-gray-600 hover:text-gray-900 dark:text-gray-400 dark:hover:text-gray-200">
        <svg :class="['h-4 w-4 transition-transform', showAdvanced ? 'rotate-90' : '']"
          fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <span>{{ t('admin.accounts.gemini.oauthType.advancedToggle') || (showAdvanced ? 'Hide' : 'Show') + ' advanced options' }}</span>
      </button>
    </div>

    <!-- AI Studio (advanced) -->
    <div v-if="showAdvanced" class="mt-3 group relative">
      <button type="button" :disabled="!aiStudioEnabled" @click="emit('select', 'ai_studio')"
        :class="['flex w-full items-center gap-3 rounded-lg border-2 p-3 text-left transition-all',
          !aiStudioEnabled ? 'cursor-not-allowed opacity-60' : '',
          oauthType === 'ai_studio'
            ? 'border-amber-500 bg-amber-50 dark:bg-amber-900/20'
            : 'border-gray-200 hover:border-amber-300 dark:border-dark-600 dark:hover:border-amber-700']">
        <div :class="['flex h-8 w-8 shrink-0 items-center justify-center rounded-lg',
          oauthType === 'ai_studio' ? 'bg-amber-500 text-white' : 'bg-gray-100 text-gray-500 dark:bg-dark-600 dark:text-gray-400']">
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
            <path stroke-linecap="round" stroke-linejoin="round"
              d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
          </svg>
        </div>
        <div class="min-w-0">
          <span class="block text-sm font-medium text-gray-900 dark:text-white">
            {{ t('admin.accounts.gemini.oauthType.customTitle') }}
          </span>
          <span class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.gemini.oauthType.customDesc') }}
          </span>
        </div>
        <span v-if="!aiStudioEnabled"
          class="badge-warning ml-auto shrink-0 rounded px-2 py-0.5 text-xs">
          {{ t('admin.accounts.oauth.gemini.aiStudioNotConfiguredShort') }}
        </span>
      </button>
      <div v-if="!aiStudioEnabled"
        class="surface-warning text-semantic-warning pointer-events-none absolute right-0 top-full z-50 mt-2 w-80 rounded-md px-3 py-2 text-xs opacity-0 shadow-lg transition-opacity group-hover:opacity-100">
        {{ t('admin.accounts.oauth.gemini.aiStudioNotConfiguredTip') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Icon } from '@sub2api/plugin-sdk'

defineProps<{
  oauthType: 'code_assist' | 'google_one' | 'ai_studio'
  aiStudioEnabled: boolean
  showAdvanced: boolean
  helpLinks: { gcpProject: string }
}>()

const emit = defineEmits<{
  select: [type: 'code_assist' | 'google_one' | 'ai_studio']
  'toggle-advanced': []
}>()

const { t } = useI18n()
</script>
