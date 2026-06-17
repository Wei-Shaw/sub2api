<template>
  <div v-if="visible" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-cyan-700 transition-colors hover:bg-cyan-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-cyan-300 dark:hover:bg-cyan-900/30"
        :disabled="loading"
        :title="t('admin.accounts.usageWindow.grokProbeTooltip')"
        @click="handleProbe"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading REDACTED"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ t('admin.accounts.usageWindow.grokProbe') REDACTEDREDACTED
      </button>

      <button
        type="button"
        class="inline-flex cursor-not-allowed items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-gray-400 opacity-70 dark:text-gray-500"
        disabled
        :title="t('admin.accounts.usageWindow.grokResetUnsupportedTooltip')"
      >
        {{ t('admin.accounts.usageWindow.grokResetUnsupported') REDACTEDREDACTED
      </button>
    </div>

    <div v-if="summary" class="text-[10px] text-gray-600 dark:text-gray-300">
      {{ summary REDACTEDREDACTED
    </div>
    <div v-if="error" class="truncate text-[10px] text-red-600 dark:text-red-400" :title="error">
      {{ truncatedError REDACTEDREDACTED
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { adminAPI REDACTED from '@/api/admin'
import type { GrokQuotaProbeResult, GrokQuotaWindow REDACTED from '@/api/admin/grok'
import type { Account REDACTED from '@/types'

const props = defineProps<{
  account: Account
REDACTED>()

const { t REDACTED = useI18n()

const visible = computed(() => props.account.platform === 'grok' && props.account.type === 'oauth')
const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<GrokQuotaProbeResult | null>(null)

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string REDACTED REDACTED
  REDACTED
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
REDACTED

const formatWindow = (label: string, window?: GrokQuotaWindow | null): string | null => {
  if (!window || window.limit == null || window.remaining == null) return null
  return `${labelREDACTED ${window.remainingREDACTED/${window.limitREDACTED`
REDACTED

const retryAfterLabel = computed(() => {
  const seconds = data.value?.snapshot?.retry_after_seconds
  if (seconds == null || seconds <= 0) return null
  if (seconds < 60) return `${secondsREDACTEDs`
  return `${Math.ceil(seconds / 60)REDACTEDm`
REDACTED)

const summary = computed(() => {
  const snapshot = data.value?.snapshot
  if (!data.value) return ''
  if (!snapshot) return t('admin.accounts.usageWindow.grokNoHeaders')
  const parts = [
    formatWindow(t('admin.accounts.usageWindow.grokRequests'), snapshot.requests),
    formatWindow(t('admin.accounts.usageWindow.grokTokens'), snapshot.tokens)
  ].filter(Boolean)
  if (retryAfterLabel.value) {
    parts.push(t('admin.accounts.usageWindow.grokRetryAfter', { time: retryAfterLabel.value REDACTED))
  REDACTED
  if (snapshot.entitlement_status) {
    parts.push(snapshot.entitlement_status)
  REDACTED
  return parts.length > 0 ? parts.join(' | ') : t('admin.accounts.usageWindow.grokNoHeaders')
REDACTED)

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)REDACTED...` : error.value
REDACTED)

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    data.value = await adminAPI.grok.queryQuota(props.account.id)
  REDACTED catch (e) {
    error.value = extractErrorMessage(e)
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

watch(
  () => props.account.id,
  () => {
    data.value = null
    error.value = null
    loading.value = false
  REDACTED
)
</script>
