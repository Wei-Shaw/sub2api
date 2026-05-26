<template>
  <div class="space-y-4">
    <!-- Auth Mode Radio -->
    <div>
      <label class="input-label">{{ t('admin.accounts.bedrockAuthMode') }}</label>
      <div class="mt-2 flex gap-4">
        <label v-for="mode in AUTH_MODES" :key="mode.value" class="flex cursor-pointer items-center">
          <input
            :checked="authMode === mode.value"
            type="radio"
            :value="mode.value"
            class="mr-2 text-primary-600 focus:ring-primary-500"
            @change="$emit('update:authMode', mode.value)"
          />
          <span class="text-sm text-gray-700 dark:text-gray-300">{{ t(mode.label) }}</span>
        </label>
      </div>
    </div>

    <!-- SigV4 fields -->
    <template v-if="authMode === 'sigv4'">
      <div>
        <label class="input-label">{{ t('admin.accounts.bedrockAccessKeyId') }}</label>
        <input
          :value="accessKeyId" type="text" required class="input font-mono" placeholder="AKIA..."
          @input="$emit('update:accessKeyId', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.bedrockSecretAccessKey') }}</label>
        <input
          :value="secretAccessKey" type="password" required class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          @input="$emit('update:secretAccessKey', ($event.target as HTMLInputElement).value)"
        />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.bedrockSessionToken') }}</label>
        <input
          :value="sessionToken" type="password" class="input font-mono"
          autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
          @input="$emit('update:sessionToken', ($event.target as HTMLInputElement).value)"
        />
        <p class="input-hint">{{ t('admin.accounts.bedrockSessionTokenHint') }}</p>
      </div>
    </template>

    <!-- API Key field -->
    <div v-if="authMode === 'apikey'">
      <label class="input-label">{{ t('admin.accounts.bedrockApiKeyInput') }}</label>
      <input
        :value="apiKeyValue" type="password" required class="input font-mono"
        autocomplete="new-password" data-1p-ignore data-lpignore="true" data-bwignore="true"
        @input="$emit('update:apiKeyValue', ($event.target as HTMLInputElement).value)"
      />
    </div>

    <!-- Region -->
    <div>
      <label class="input-label">{{ t('admin.accounts.bedrockRegion') }}</label>
      <select :value="region" class="input" @change="$emit('update:region', ($event.target as HTMLSelectElement).value)">
        <optgroup v-for="group in BEDROCK_REGION_OPTIONS" :key="group.label" :label="group.label">
          <option v-for="opt in group.options" :key="opt.value" :value="opt.value">{{ opt.label }}</option>
        </optgroup>
      </select>
      <p class="input-hint">{{ t('admin.accounts.bedrockRegionHint') }}</p>
    </div>

    <!-- Force Global -->
    <div>
      <label class="flex items-center gap-2 cursor-pointer">
        <input
          :checked="forceGlobal" type="checkbox"
          class="rounded border-gray-300 text-primary-600 focus:ring-primary-500 dark:border-dark-500"
          @change="$emit('update:forceGlobal', ($event.target as HTMLInputElement).checked)"
        />
        <span class="text-sm text-gray-700 dark:text-gray-300">{{ t('admin.accounts.bedrockForceGlobal') }}</span>
      </label>
      <p class="input-hint mt-1">{{ t('admin.accounts.bedrockForceGlobalHint') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { BEDROCK_REGION_OPTIONS } from '../constants'

const { t } = useI18n()

const AUTH_MODES = [
  { value: 'sigv4' as const, label: 'admin.accounts.bedrockAuthModeSigv4' },
  { value: 'apikey' as const, label: 'admin.accounts.bedrockAuthModeApikey' },
]

defineProps<{
  authMode: 'sigv4' | 'apikey'
  accessKeyId: string
  secretAccessKey: string
  sessionToken: string
  apiKeyValue: string
  region: string
  forceGlobal: boolean
}>()

defineEmits<{
  'update:authMode': [value: 'sigv4' | 'apikey']
  'update:accessKeyId': [value: string]
  'update:secretAccessKey': [value: string]
  'update:sessionToken': [value: string]
  'update:apiKeyValue': [value: string]
  'update:region': [value: string]
  'update:forceGlobal': [value: boolean]
}>()
</script>
