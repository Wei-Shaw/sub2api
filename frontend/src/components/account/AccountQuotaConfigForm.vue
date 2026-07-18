<template>
  <div class="space-y-3">
    <div>
      <label class="input-label">{{ t('admin.accounts.quotaSource.label') }}</label>
      <Select
        :model-value="selectedSource"
        :options="sourceOptions"
        :searchable="false"
        @update:model-value="onSourceChange"
      />
      <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
        {{ mode === 'upstream' ? t('admin.accounts.quotaSource.upstreamHint') : t('admin.accounts.quotaSource.manualHint') }}
      </p>
    </div>

    <template v-if="mode === 'upstream'">
      <div v-for="field in selectedProvider?.config_fields || []" :key="field.key">
        <label v-if="field.type !== 'boolean'" class="input-label">{{ fieldLabel(field) }}</label>
        <input
          v-if="field.type !== 'boolean'"
          class="input"
          :type="field.type"
          :required="field.required"
          :placeholder="fieldPlaceholder(field)"
          :value="config[field.key] ?? ''"
          @input="updateConfigField(field.key, field.type, ($event.target as HTMLInputElement).value)"
        />
        <label v-else class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input
            type="checkbox"
            :checked="config[field.key] === true"
            @change="updateConfigField(field.key, field.type, ($event.target as HTMLInputElement).checked)"
          />
          {{ field.label }}
        </label>
        <p
          v-if="selectedProvider?.id === 'newapi' && field.key === 'user_id'"
          class="mt-1 text-xs text-gray-500 dark:text-gray-400"
        >
          {{ t('admin.accounts.quotaSource.newAPIUserIDHint') }}
        </p>
        <p
          v-else-if="selectedProvider?.id === 'newapi' && field.key === 'quota_per_usd'"
          class="mt-1 text-xs text-gray-500 dark:text-gray-400"
        >
          {{ t('admin.accounts.quotaSource.newAPIQuotaPerUSDHint') }}
        </p>
      </div>
    </template>

    <slot v-if="mode === 'manual'" name="manual" />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AccountQuotaProvider } from '@/types'
import Select from '@/components/common/Select.vue'

const props = defineProps<{
  mode: 'manual' | 'upstream'
  provider: string
  config: Record<string, unknown>
}>()

const emit = defineEmits<{
  'update:mode': [value: 'manual' | 'upstream']
  'update:provider': [value: string]
  'update:config': [value: Record<string, unknown>]
}>()

const { t } = useI18n()
const providers = ref<AccountQuotaProvider[]>([
  { id: 'sub2api', name: 'Sub2API', supported_account_types: ['apikey'], required_credentials: ['base_url', 'api_key'], config_fields: [] },
  {
    id: 'newapi',
    name: 'NewAPI',
    supported_account_types: ['apikey'],
    required_credentials: ['base_url', 'api_key'],
    config_fields: [
      {
        key: 'user_id',
        label: 'NewAPI User ID',
        type: 'number',
        placeholder: 'Optional for legacy versions'
      },
      {
        key: 'quota_per_usd',
        label: 'Quota per USD',
        type: 'number',
        placeholder: '500000'
      }
    ]
  }
])
const availableProviders = computed(() => {
  if (!props.provider || providers.value.some(item => item.id === props.provider)) return providers.value
  return [
    ...providers.value,
    { id: props.provider, name: props.provider, supported_account_types: ['apikey'], required_credentials: ['base_url', 'api_key'], config_fields: [] }
  ]
})
const sourceOptions = computed(() => [
  { value: 'manual', label: t('admin.accounts.quotaSource.manual') },
  ...availableProviders.value.map(item => ({ value: item.id, label: item.name }))
])

const selectedSource = computed(() => props.mode === 'manual' ? 'manual' : props.provider)
const onSourceChange = (newValue: string | number | boolean | null) => {
  const value = String(newValue ?? 'manual')
  if (value === 'manual') {
    emit('update:mode', 'manual')
    return
  }
  emit('update:provider', value)
  emit('update:mode', 'upstream')
}
const selectedProvider = computed(() => availableProviders.value.find(item => item.id === props.provider))
const fieldLabel = (field: AccountQuotaProvider['config_fields'][number]) => {
  if (selectedProvider.value?.id === 'newapi' && field.key === 'user_id') {
    return t('admin.accounts.quotaSource.newAPIUserID')
  }
  if (selectedProvider.value?.id === 'newapi' && field.key === 'quota_per_usd') {
    return t('admin.accounts.quotaSource.newAPIQuotaPerUSD')
  }
  return field.label
}
const fieldPlaceholder = (field: AccountQuotaProvider['config_fields'][number]) => {
  if (selectedProvider.value?.id === 'newapi' && field.key === 'user_id') {
    return t('admin.accounts.quotaSource.newAPIUserIDPlaceholder')
  }
  if (selectedProvider.value?.id === 'newapi' && field.key === 'quota_per_usd') {
    return '500000'
  }
  return field.placeholder
}
const updateConfigField = (key: string, type: string, value: string | boolean) => {
  emit('update:config', {
    ...props.config,
    [key]: type === 'number' && typeof value === 'string' ? (value === '' ? null : Number(value)) : value
  })
}

onMounted(async () => {
  try {
    const remoteProviders = await adminAPI.accounts.getQuotaProviders()
    if (remoteProviders.length > 0) providers.value = remoteProviders
  } catch {
    // Keep the built-in descriptors so account editing still works if discovery is temporarily unavailable.
  }
})
</script>
