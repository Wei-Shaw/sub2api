<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-700">
    <h4 class="mb-3 text-sm font-semibold text-gray-900 dark:text-white">
      {{ t('payment.adminSettings.providerConfig') }}
    </h4>
    <div class="space-y-3">
      <div v-for="field in resolvedFields" :key="field.key">
        <label class="input-label">
          {{ field.label }}
          <span v-if="field.optional" class="text-xs text-gray-400">({{ t('common.optional') }})</span>
          <span v-else class="input-required"> *</span>
        </label>
        <textarea
          v-if="field.sensitive && field.key.toLowerCase().includes('key') && field.key !== 'pkey'"
          v-model="config[field.key]"
          rows="3"
          class="input font-mono text-xs"
          autocomplete="new-password"
          data-1p-ignore
          data-lpignore="true"
          data-bwignore="true"
          spellcheck="false"
          :placeholder="editing ? t('admin.accounts.leaveEmptyToKeep') : ''"
        />
        <div v-else-if="field.sensitive" class="relative">
          <input
            :type="visibleFields[field.key] ? 'text' : 'password'"
            v-model="config[field.key]"
            class="input pr-10"
            autocomplete="new-password"
            data-1p-ignore
            data-lpignore="true"
            data-bwignore="true"
            spellcheck="false"
            :placeholder="editing ? t('admin.accounts.leaveEmptyToKeep') : (field.defaultValue || '')"
          />
          <button
            type="button"
            @click="visibleFields[field.key] = !visibleFields[field.key]"
            class="absolute inset-y-0 right-0 flex items-center pr-3 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
          >
            <svg v-if="visibleFields[field.key]" class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21" /></svg>
            <svg v-else class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" /></svg>
          </button>
        </div>
        <Select
          v-else-if="field.options?.length"
          v-model="config[field.key]"
          :options="field.options"
          :searchable="field.options.length > 5"
        />
        <input
          v-else
          type="text"
          v-model="config[field.key]"
          class="input"
          :placeholder="field.defaultValue || ''"
        />
        <p v-if="field.hintKey" class="mt-1 text-xs leading-relaxed text-gray-500 dark:text-gray-400">
          {{ t(field.hintKey) }}
        </p>
      </div>
    </div>

    <!-- Callback URLs -->
    <div v-if="callbackPaths" class="mt-4 space-y-3">
      <div v-if="callbackPaths.notifyUrl">
        <label class="input-label">{{ t('payment.adminSettings.field_notifyUrl') }} <span class="input-required">*</span></label>
        <div class="flex">
          <input :value="notifyBaseUrl" @input="emit('update:notifyBaseUrl', ($event.target as HTMLInputElement).value)" type="text" class="input min-w-0 flex-1 !rounded-r-none !border-r-0" :placeholder="defaultBaseUrl" />
          <span class="inline-flex items-center whitespace-nowrap rounded-r-lg border border-gray-300 bg-gray-50 px-3 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-400">{{ callbackPaths.notifyUrl }}</span>
        </div>
      </div>
      <div v-if="callbackPaths.returnUrl">
        <label class="input-label">{{ t('payment.adminSettings.field_returnUrl') }} <span class="input-required">*</span></label>
        <div class="flex">
          <input :value="returnBaseUrl" @input="emit('update:returnBaseUrl', ($event.target as HTMLInputElement).value)" type="text" class="input min-w-0 flex-1 !rounded-r-none !border-r-0" :placeholder="defaultBaseUrl" />
          <span class="inline-flex items-center whitespace-nowrap rounded-r-lg border border-gray-300 bg-gray-50 px-3 text-xs text-gray-500 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-400">{{ callbackPaths.returnUrl }}</span>
        </div>
      </div>
    </div>

    <!-- Webhook hint -->
    <div v-if="providerWebhookUrl" class="surface-info mt-3 rounded-lg p-3">
      <p class="text-semantic-info text-xs">
        {{ t(providerWebhookHint) }}
      </p>
      <code class="surface-info-strong mt-1 block break-all rounded px-2 py-1 text-xs">
        {{ providerWebhookUrl }}
      </code>
      <p v-if="providerKey === 'stripe'" class="text-semantic-info mt-2 text-xs leading-relaxed">
        {{ t('admin.settings.payment.stripeWebhookApiVersionHint', { version: STRIPE_SDK_API_VERSION }) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Select } from '@sub2api/plugin-sdk'
import { STRIPE_SDK_API_VERSION } from './providerConfig'

const { t } = useI18n()

defineProps<{
  editing: boolean
  providerKey: string
  config: Record<string, string>
  visibleFields: Record<string, boolean>
  resolvedFields: Array<{
    key: string
    label: string
    sensitive?: boolean
    optional?: boolean
    defaultValue?: string
    hintKey?: string
    clearable?: boolean
    options?: Array<{ value: string; label: string }>
  }>
  callbackPaths: { notifyUrl?: string; returnUrl?: string } | null
  notifyBaseUrl: string
  returnBaseUrl: string
  defaultBaseUrl: string
  providerWebhookUrl: string
  providerWebhookHint: string
}>()

const emit = defineEmits<{
  'update:notifyBaseUrl': [val: string]
  'update:returnBaseUrl': [val: string]
}>()
</script>
