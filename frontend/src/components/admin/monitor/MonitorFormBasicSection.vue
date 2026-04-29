<template>
  <div class="space-y-5">
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.form.name') }} <span class="text-red-500">*</span></label>
      <input v-model="name" type="text" required class="input" :placeholder="t('admin.channelMonitor.form.namePlaceholder')" />
    </div>

    <div>
      <label class="input-label">{{ t('admin.channelMonitor.form.provider') }} <span class="text-red-500">*</span></label>
      <div class="grid grid-cols-3 gap-3">
        <button
          v-for="opt in providerOptions"
          :key="opt.value"
          type="button"
          :aria-pressed="provider === opt.value"
          class="flex items-center justify-center gap-2 rounded-lg border-2 px-3 py-2.5 text-sm font-medium transition-colors"
          :class="providerPickerClass(opt.value, provider === opt.value)"
          @click="provider = opt.value"
        >
          <ProviderIcon :provider="opt.value" :size="18" />
          <span>{{ opt.label }}</span>
        </button>
      </div>
    </div>

    <div>
      <label class="input-label">{{ t('admin.channelMonitor.form.endpoint') }} <span class="text-red-500">*</span></label>
      <div class="flex gap-2">
        <input v-model="endpoint" type="text" required class="input flex-1" :placeholder="t('admin.channelMonitor.form.endpointPlaceholder')" />
        <button type="button" @click="useCurrentDomain" class="btn btn-secondary whitespace-nowrap">
          {{ t('admin.channelMonitor.form.useCurrentDomain') }}
        </button>
      </div>
    </div>

    <div>
      <label class="input-label">
        {{ t('admin.channelMonitor.form.apiKey') }}<span v-if="!editing" class="text-red-500"> *</span>
      </label>
      <div class="flex gap-2">
        <input
          v-model="apiKey"
          type="password"
          autocomplete="new-password"
          data-1p-ignore
          data-lpignore="true"
          data-bwignore="true"
          :required="!editing"
          class="input flex-1 font-mono"
          :placeholder="editing ? t('admin.channelMonitor.form.apiKeyEditPlaceholder') : t('admin.channelMonitor.form.apiKeyPlaceholder')"
        />
        <button type="button" @click="emit('open-key-picker')" class="btn btn-secondary whitespace-nowrap">
          {{ t('admin.channelMonitor.form.useMyKey') }}
        </button>
      </div>
      <p v-if="editing && editing.api_key_masked" class="mt-1 text-xs text-gray-400">{{ editing.api_key_masked }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ChannelMonitor, Provider } from '@/api/admin/channelMonitor'
import ProviderIcon from '@/components/user/monitor/ProviderIcon.vue'
import { useChannelMonitorFormat } from '@/composables/useChannelMonitorFormat'
import {
  PROVIDER_OPENAI,
  PROVIDER_ANTHROPIC,
  PROVIDER_GEMINI,
} from '@/constants/channelMonitor'

defineProps<{
  editing: ChannelMonitor | null
}>()

const emit = defineEmits<{
  (e: 'open-key-picker'): void
}>()

const name = defineModel<string>('name', { required: true })
const provider = defineModel<Provider>('provider', { required: true })
const endpoint = defineModel<string>('endpoint', { required: true })
const apiKey = defineModel<string>('apiKey', { required: true })

const { t } = useI18n()
const { providerPickerClass } = useChannelMonitorFormat()

interface ProviderOption {
  value: Provider
  label: string
}

const providerOptions = computed<ProviderOption[]>(() => [
  { value: PROVIDER_ANTHROPIC, label: t('monitorCommon.providers.anthropic') },
  { value: PROVIDER_OPENAI, label: t('monitorCommon.providers.openai') },
  { value: PROVIDER_GEMINI, label: t('monitorCommon.providers.gemini') },
])

function useCurrentDomain() {
  endpoint.value = window.location.origin
}
</script>
