<template>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
    <div class="sm:col-span-2">
      <label class="input-label">{{ t('admin.proxies.pools.form.name') }} *</label>
      <Input :model-value="modelValue.name" :placeholder="t('admin.proxies.pools.form.name')" @update:model-value="patch('name', String($event ?? ''))" />
    </div>
    <div class="sm:col-span-2">
      <label class="input-label">{{ t('admin.proxies.pools.form.sourceType') }}</label>
      <Select :model-value="modelValue.source_type" :options="sourceTypeOptions" @update:model-value="onSourceTypeChange" />
    </div>
    <div v-if="modelValue.source_type === 'subscription'" class="sm:col-span-2">
      <label class="input-label">{{ t('admin.proxies.pools.form.subscription') }}</label>
      <Select :model-value="modelValue.subscription_id" :options="subscriptionOptions" @update:model-value="onSubscriptionChange" />
    </div>
    <div v-else class="sm:col-span-2">
      <label class="input-label">{{ t('admin.proxies.pools.form.extractUrl') }} *</label>
      <Input :model-value="modelValue.extract_url" type="url" placeholder="https://..." @update:model-value="patch('extract_url', String($event ?? ''))" />
      <p class="mt-1 text-xs text-gray-400">{{ t('admin.proxies.pools.form.extractUrlHint') }}</p>
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.protocol') }}</label>
      <Select :model-value="modelValue.protocol" :options="protocolOptions" @update:model-value="patch('protocol', String($event ?? 'http'))" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.authMode') }}</label>
      <Select :model-value="modelValue.auth_mode" :options="authModeOptions" @update:model-value="patch('auth_mode', String($event ?? 'none'))" />
    </div>
    <div v-if="modelValue.auth_mode === 'fixed'">
      <label class="input-label">{{ t('admin.proxies.pools.form.username') }}</label>
      <Input :model-value="modelValue.username" @update:model-value="patch('username', String($event ?? ''))" />
    </div>
    <div v-if="modelValue.auth_mode === 'fixed'">
      <label class="input-label">{{ t('admin.proxies.pools.form.password') }}</label>
      <Input :model-value="modelValue.password" type="password" @update:model-value="patch('password', String($event ?? ''))" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.responseFormat') }}</label>
      <Select :model-value="modelValue.response_format" :options="formatOptions" @update:model-value="patch('response_format', String($event ?? 'txt'))" />
    </div>
    <div v-if="modelValue.response_format === 'txt'">
      <label class="input-label">{{ t('admin.proxies.pools.form.lineSeparator') }}</label>
      <Input :model-value="modelValue.line_separator" placeholder="\r\n" @update:model-value="patch('line_separator', String($event ?? ''))" />
    </div>
    <div v-if="modelValue.response_format === 'json'">
      <label class="input-label">{{ t('admin.proxies.pools.form.ipFieldPath') }}</label>
      <Input :model-value="modelValue.ip_field_path" placeholder="ip" @update:model-value="patch('ip_field_path', String($event ?? ''))" />
    </div>
    <div v-if="modelValue.response_format === 'json'">
      <label class="input-label">{{ t('admin.proxies.pools.form.portFieldPath') }}</label>
      <Input :model-value="modelValue.port_field_path" placeholder="port" @update:model-value="patch('port_field_path', String($event ?? ''))" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.refreshInterval') }}</label>
      <Input :model-value="modelValue.refresh_interval_sec" type="number" min="60" @update:model-value="patchNumber('refresh_interval_sec', $event)" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.ipDuration') }}</label>
      <Input :model-value="modelValue.ip_duration_sec" type="number" min="30" @update:model-value="patchNumber('ip_duration_sec', $event)" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.extractCount') }}</label>
      <Input :model-value="modelValue.extract_count" type="number" min="1" @update:model-value="patchNumber('extract_count', $event)" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.minAlive') }}</label>
      <Input :model-value="modelValue.min_alive" type="number" min="1" @update:model-value="patchNumber('min_alive', $event)" />
    </div>
    <div>
      <label class="input-label">{{ t('admin.proxies.pools.form.healthCheckInterval') }}</label>
      <Input :model-value="modelValue.health_check_interval_sec" type="number" min="0" @update:model-value="patchNumber('health_check_interval_sec', $event)" />
      <p class="mt-1 text-xs text-gray-400">{{ t('admin.proxies.pools.form.healthCheckHint') }}</p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Input from '@/components/common/Input.vue'
import Select from '@/components/common/Select.vue'

export type PoolFormModel = {
  name: string
  source_type: string
  subscription_id: number | null
  extract_url: string
  protocol: string
  auth_mode: string
  username: string
  password: string
  response_format: string
  line_separator: string
  ip_field_path: string
  port_field_path: string
  refresh_interval_sec: number
  ip_duration_sec: number
  extract_count: number
  min_alive: number
  health_check_interval_sec: number
  enabled?: boolean
}

const props = withDefaults(defineProps<{
  modelValue: PoolFormModel
  protocolOptions: Array<{ value: string; label: string }>
  authModeOptions: Array<{ value: string; label: string }>
  formatOptions: Array<{ value: string; label: string }>
  sourceTypeOptions?: Array<{ value: string; label: string }>
  subscriptionOptions?: Array<{ value: string | number | boolean | null; label: string }>
}>(), {
  subscriptionOptions: () => []
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: PoolFormModel): void
}>()

const { t } = useI18n()

const defaultSourceTypeOptions = [
  { value: 'extract_api', label: t('admin.proxies.pools.form.sourceExtractApi') },
  { value: 'subscription', label: t('admin.proxies.pools.form.sourceSubscription') }
]

const sourceTypeOptions = props.sourceTypeOptions ?? defaultSourceTypeOptions

function onSourceTypeChange(val: unknown) {
  const v = String(val ?? 'extract_api')
  patch('source_type', v)
  if (v === 'subscription') {
    patch('extract_url', '')
  } else {
    patch('subscription_id', null)
  }
}

function onSubscriptionChange(val: unknown) {
  const n = val === null ? null : Number(val)
  patch('subscription_id', n)
}

function patch<K extends keyof PoolFormModel>(key: K, value: PoolFormModel[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function patchNumber(key: 'refresh_interval_sec' | 'ip_duration_sec' | 'extract_count' | 'min_alive' | 'health_check_interval_sec', raw: unknown) {
  const n = typeof raw === 'number' ? raw : Number(raw)
  patch(key, Number.isFinite(n) ? n : 0)
}
</script>
