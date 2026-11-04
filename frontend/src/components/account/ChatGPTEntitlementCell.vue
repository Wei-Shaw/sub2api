<template>
  <div class="space-y-0.5 text-[10px] leading-tight" data-testid="chatgpt-entitlement">
    <span :class="expired ? 'text-red-600 dark:text-red-400' : 'text-gray-600 dark:text-gray-400'">
      {{ t(`admin.accounts.entitlement.${status}`) }}
    </span>
    <div v-for="field in fields" :key="field.key" class="text-gray-500 dark:text-gray-400" :title="field.raw || undefined">
      {{ t(`admin.accounts.entitlement.${field.key}`) }}：{{ field.text }}
      <span v-if="field.key === 'grace' && graceExpired" class="text-amber-600">（{{ t('admin.accounts.entitlement.ended') }}）</span>
    </div>
    <div v-if="!snapshot" class="text-gray-400">{{ t('admin.accounts.entitlement.refreshHint') }}</div>
    <div v-else-if="snapshot.checked_at" class="text-gray-400">
      {{ t('admin.accounts.entitlement.checkedAt') }}：{{ display(snapshot.checked_at) }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useNow } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { formatDateTime } from '@/utils/format'
import type { Account } from '@/types'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const now = useNow({ interval: 60_000 })
const snapshot = computed(() => {
  const value = props.account.credentials?.chatgpt_entitlement
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null
  const data = value as Record<string, unknown>
  const accountID = props.account.credentials?.chatgpt_account_id
  if (accountID && data.account_id !== accountID) return null
  return data
})
const timestamp = (value: unknown): number | null => {
  if (typeof value !== 'string' || !value.trim()) return null
  const time = Date.parse(value)
  return Number.isFinite(time) ? time : null
}
const expiry = computed(() => snapshot.value ? snapshot.value.expires_at : props.account.credentials?.subscription_expires_at)
const expired = computed(() => {
  const time = timestamp(expiry.value)
  return time !== null && time <= now.value.getTime()
})
const status = computed(() => timestamp(expiry.value) === null ? 'unknown' : expired.value ? 'expired' : 'valid')
const graceExpired = computed(() => {
  const time = timestamp(snapshot.value?.grace_period_end_timestamp)
  return time !== null && time <= now.value.getTime()
})
const display = (value: unknown) => timestamp(value) === null
  ? t('admin.accounts.entitlement.notProvided') : formatDateTime(value as string)
const fields = computed(() => [
  { key: 'grace', value: snapshot.value?.grace_period_end_timestamp },
  { key: 'expires', value: expiry.value },
  { key: 'renews', value: snapshot.value?.renews_at },
  { key: 'cancels', value: snapshot.value?.cancels_at },
].map(({ key, value }) => ({ key, raw: typeof value === 'string' ? value : '', text: display(value) })))
</script>
