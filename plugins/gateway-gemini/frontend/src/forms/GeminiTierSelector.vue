<template>
  <div class="mt-4">
    <label class="input-label">{{ t('admin.accounts.gemini.tier.label') }}</label>
    <div class="mt-2">
      <select v-if="accountCategory === 'oauth-based' && oauthType === 'google_one'"
        :value="tierGoogleOne" class="input"
        @change="onGoogleOneChange">
        <option value="google_one_free">{{ t('admin.accounts.gemini.tier.googleOne.free') }}</option>
        <option value="google_ai_pro">{{ t('admin.accounts.gemini.tier.googleOne.pro') }}</option>
        <option value="google_ai_ultra">{{ t('admin.accounts.gemini.tier.googleOne.ultra') }}</option>
      </select>
      <select v-else-if="accountCategory === 'oauth-based' && oauthType === 'code_assist'"
        :value="tierGcp" class="input"
        @change="onGcpChange">
        <option value="gcp_standard">{{ t('admin.accounts.gemini.tier.gcp.standard') }}</option>
        <option value="gcp_enterprise">{{ t('admin.accounts.gemini.tier.gcp.enterprise') }}</option>
      </select>
      <select v-else :value="tierAiStudio" class="input"
        @change="onAiStudioChange">
        <option value="aistudio_free">{{ t('admin.accounts.gemini.tier.aiStudio.free') }}</option>
        <option value="aistudio_paid">{{ t('admin.accounts.gemini.tier.aiStudio.paid') }}</option>
      </select>
    </div>
    <p class="input-hint">
      {{ accountCategory === 'apikey'
        ? t('admin.accounts.gemini.tier.aiStudioHint')
        : t('admin.accounts.gemini.tier.hint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'

defineProps<{
  accountCategory: string
  oauthType: string
  tierGoogleOne: string
  tierGcp: string
  tierAiStudio: string
}>()

const emit = defineEmits<{
  'update:tierGoogleOne': [value: string]
  'update:tierGcp': [value: string]
  'update:tierAiStudio': [value: string]
}>()

const { t } = useI18n()

function onGoogleOneChange(e: Event) {
  emit('update:tierGoogleOne', (e.target as HTMLSelectElement).value)
}
function onGcpChange(e: Event) {
  emit('update:tierGcp', (e.target as HTMLSelectElement).value)
}
function onAiStudioChange(e: Event) {
  emit('update:tierAiStudio', (e.target as HTMLSelectElement).value)
}
</script>
