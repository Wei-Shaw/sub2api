<template>
  <div class="space-y-5">
    <!-- Name -->
    <div v-if="showIdentity">
      <label class="input-label">{{ t('admin.accounts.accountName') }}</label>
      <input :value="modelValue.name" type="text" required class="input"
        :placeholder="t('admin.accounts.enterAccountName')"
        @input="update('name', ($event.target as HTMLInputElement).value)" />
    </div>
    <!-- Notes -->
    <div v-if="showIdentity">
      <label class="input-label">{{ t('admin.accounts.notes') }}</label>
      <textarea :value="modelValue.notes" rows="2" class="input"
        :placeholder="t('admin.accounts.notesPlaceholder')"
        @input="update('notes', ($event.target as HTMLTextAreaElement).value)"></textarea>
      <p class="input-hint">{{ t('admin.accounts.notesHint') }}</p>
    </div>

    <!-- Proxy -->
    <div v-if="showSettings" class="border-t border-gray-200 pt-5 dark:border-dark-600">
      <label class="input-label">{{ t('admin.accounts.proxy') }}</label>
      <ProxySelect :model-value="modelValue.proxy_id" :proxies="proxies"
        @update:model-value="update('proxy_id', $event)" />
    </div>

    <!-- Numeric grid: concurrency, load_factor, priority, rate_multiplier -->
    <div v-if="showSettings" class="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <div>
        <label class="input-label">{{ t('admin.accounts.concurrency') }}</label>
        <input :value="modelValue.concurrency" type="number" min="1" class="input"
          @input="update('concurrency', Math.max(1, Number(($event.target as HTMLInputElement).value) || 1))" />
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.loadFactor') }}</label>
        <input :value="modelValue.load_factor" type="number" min="1" class="input"
          :placeholder="String(modelValue.concurrency)"
          @input="updateNullable('load_factor', ($event.target as HTMLInputElement).value)" />
        <p class="input-hint">{{ t('admin.accounts.loadFactorHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.priority') }}</label>
        <input :value="modelValue.priority" type="number" min="1" class="input"
          @input="update('priority', Math.max(1, Number(($event.target as HTMLInputElement).value) || 1))" />
        <p class="input-hint">{{ t('admin.accounts.priorityHint') }}</p>
      </div>
      <div>
        <label class="input-label">{{ t('admin.accounts.billingRateMultiplier') }}</label>
        <input :value="modelValue.rate_multiplier" type="number" min="0" step="0.001" class="input"
          @input="update('rate_multiplier', Math.max(0, Number(($event.target as HTMLInputElement).value) || 0))" />
        <p class="input-hint">{{ t('admin.accounts.billingRateMultiplierHint') }}</p>
      </div>
    </div>

    <!-- Quota control (apikey/bedrock types) -->
    <div v-if="showSettings && showQuota" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <div class="mb-3 flex items-center justify-between">
        <div>
          <label class="input-label mb-0">{{ t('admin.accounts.quotaControl.enableQuotaLimit') }}</label>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.quotaControl.enableQuotaLimitHint') }}</p>
        </div>
        <button type="button" @click="update('quota_enabled', !modelValue.quota_enabled)"
          :class="['relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
            modelValue.quota_enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600']">
          <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            modelValue.quota_enabled ? 'translate-x-5' : 'translate-x-0']" />
        </button>
      </div>
      <div v-if="modelValue.quota_enabled" class="space-y-3">
        <div class="grid grid-cols-3 gap-3">
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.totalLimit') }} ($)</label>
            <input :value="modelValue.quota_limit" type="number" min="0" step="0.01" class="input"
              :placeholder="t('admin.accounts.quotaControl.unlimited')"
              @input="updateQuota('quota_limit', ($event.target as HTMLInputElement).value)" />
          </div>
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.dailyLimit') }} ($)</label>
            <input :value="modelValue.quota_daily_limit" type="number" min="0" step="0.01" class="input"
              :placeholder="t('admin.accounts.quotaControl.unlimited')"
              @input="updateQuota('quota_daily_limit', ($event.target as HTMLInputElement).value)" />
          </div>
          <div>
            <label class="input-label text-xs">{{ t('admin.accounts.quotaControl.weeklyLimit') }} ($)</label>
            <input :value="modelValue.quota_weekly_limit" type="number" min="0" step="0.01" class="input"
              :placeholder="t('admin.accounts.quotaControl.unlimited')"
              @input="updateQuota('quota_weekly_limit', ($event.target as HTMLInputElement).value)" />
          </div>
        </div>
      </div>
    </div>

    <!-- Expiration -->
    <div v-if="showSettings" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <label class="input-label">{{ t('admin.accounts.expiresAt') }}</label>
      <input :value="expiresAtDisplay" type="datetime-local" class="input"
        @input="onExpiresAtInput(($event.target as HTMLInputElement).value)" />
      <p class="input-hint">{{ t('admin.accounts.expiresAtHint') }}</p>
    </div>

    <!-- Auto pause on expired -->
    <div v-if="showSettings" class="flex items-center justify-between">
      <div>
        <span class="input-label mb-0">{{ t('admin.accounts.autoPauseOnExpired') }}</span>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.autoPauseOnExpiredDesc') }}</p>
      </div>
      <button type="button" @click="update('auto_pause_on_expired', !modelValue.auto_pause_on_expired)"
        :class="['relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          modelValue.auto_pause_on_expired ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600']">
        <span :class="['pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
          modelValue.auto_pause_on_expired ? 'translate-x-5' : 'translate-x-0']" />
      </button>
    </div>

    <!-- Groups -->
    <div v-if="showSettings && !isSimpleMode" class="border-t border-gray-200 pt-4 dark:border-dark-600">
      <GroupSelect :model-value="modelValue.group_ids" :groups="groups"
        @update:model-value="update('group_ids', $event)" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { CommonAccountFields, PlatformFormContext } from '../../account-form-types'
import ProxySelect from './ProxySelect.vue'
import GroupSelect from './GroupSelect.vue'

const props = defineProps<{
  modelValue: CommonAccountFields
  context: PlatformFormContext
  mode?: 'full' | 'identity' | 'settings'
}>()

const emit = defineEmits<{
  'update:modelValue': [value: CommonAccountFields]
}>()

const { t } = useI18n()

const showIdentity = computed(() => (props.mode ?? 'full') !== 'settings')
const showSettings = computed(() => (props.mode ?? 'full') !== 'identity')

const proxies = computed(() => props.context.hostData?.proxies ?? [])
const allGroups = computed(() => props.context.hostData?.groups ?? [])
const groups = computed(() => {
  const platform = props.context.hostData?.platform
  if (!platform) return allGroups.value
  const compat = props.context.hostData?.compatiblePlatforms ?? []
  const allowed = new Set([platform, ...compat])
  return allGroups.value.filter((g) => !g.platform || allowed.has(g.platform))
})
const isSimpleMode = computed(() => props.context.hostData?.isSimpleMode ?? false)
const showQuota = computed(() =>
  props.context.accountCategory === 'apikey' || props.context.accountCategory === 'bedrock'
)

const expiresAtDisplay = computed(() => {
  const ts = props.modelValue.expires_at
  if (!ts) return ''
  const d = new Date(ts * 1000)
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
})

function update<K extends keyof CommonAccountFields>(key: K, value: CommonAccountFields[K]) {
  emit('update:modelValue', { ...props.modelValue, [key]: value })
}

function updateNullable(key: 'load_factor', raw: string) {
  const trimmed = raw.trim()
  const val = trimmed === '' ? null : Math.max(1, Number(trimmed) || 1)
  update(key, val)
}

function updateQuota(key: 'quota_limit' | 'quota_daily_limit' | 'quota_weekly_limit', raw: string) {
  const trimmed = raw.trim()
  update(key, trimmed === '' ? null : Math.max(0, Number(trimmed) || 0))
}

function onExpiresAtInput(value: string) {
  if (!value) {
    update('expires_at', null)
    return
  }
  const ts = Math.floor(new Date(value).getTime() / 1000)
  if (!isNaN(ts)) update('expires_at', ts)
}
</script>
