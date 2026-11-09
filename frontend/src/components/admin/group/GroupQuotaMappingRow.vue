<template>
  <div class="space-y-2 rounded-lg border border-gray-200 bg-white p-2.5 dark:border-dark-600 dark:bg-dark-800" data-testid="quota-mapping-row">
    <div class="flex items-center gap-2">
      <Select :model-value="source" :options="modelOptions" searchable creatable :disabled="disabled" :aria-label="t('quotaPolicy.source')" :placeholder="t('quotaPolicy.source')" :search-placeholder="t('quotaPolicy.modelSearch')" :creatable-prefix="t('quotaPolicy.useCustomModel')" class="min-w-0 flex-1" data-testid="quota-source" @update:model-value="emit('update:source', String($event ?? ''))" />
      <span class="text-gray-400">→</span>
      <Select :model-value="target" :options="modelOptions" searchable creatable :disabled="disabled" :aria-label="t('quotaPolicy.target')" :placeholder="t('quotaPolicy.target')" :search-placeholder="t('quotaPolicy.modelSearch')" :creatable-prefix="t('quotaPolicy.useCustomModel')" class="min-w-0 flex-1" data-testid="quota-target" @update:model-value="emit('update:target', String($event ?? ''))" />
      <button type="button" class="p-1 text-gray-400 hover:text-red-500" :disabled="disabled" :aria-label="t('quotaPolicy.remove')" @click="emit('remove')"><Icon name="x" size="sm" /></button>
    </div>
    <div class="flex items-start justify-between gap-2 text-xs">
      <p role="status" :class="statusClass" data-testid="quota-check-status">{{ statusText }}</p>
      <button type="button" class="shrink-0 text-primary-600 disabled:opacity-50 dark:text-primary-400" :disabled="disabled || checking || !canCheck" data-testid="quota-check-retry" @click="check(true)">{{ t('quotaPolicy.retest') }}</button>
    </div>
    <details v-if="result" class="text-xs text-gray-500 dark:text-gray-400" data-testid="quota-check-details">
      <summary class="cursor-pointer">{{ t('quotaPolicy.checkDetails') }} · {{ formatTime(result.checked_at) }}</summary>
      <ul class="mt-2 space-y-1.5">
        <li v-for="account in result.accounts" :key="account.account_id" :class="account.status === 'passed' ? 'text-green-700 dark:text-green-400' : 'text-amber-700 dark:text-amber-400'">
          {{ account.account_name }} #{{ account.account_id }} → {{ account.upstream_model || target }} · {{ t(`quotaPolicy.checkAccount_${account.status}`) }}<span v-if="account.latency_ms !== undefined"> · {{ account.latency_ms }} ms</span><span v-if="account.error_code"> · {{ account.error_code }}</span><span v-if="account.message"> · {{ account.message }}</span><span v-if="account.cached"> · {{ t('quotaPolicy.cachedCheck') }}</span>
        </li>
      </ul>
      <p v-for="warning in result.warnings" :key="warning" class="mt-1 text-amber-700 dark:text-amber-400">{{ warning }}</p>
    </details>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import * as policyAPI from '@/api/admin/groupQuotaPolicy'
import type { QuotaMappingCheck, QuotaModelCatalog } from '@/api/admin/groupQuotaPolicy'

const props = defineProps<{ groupId: number; source: string; target: string; accountIds: number[]; catalog: QuotaModelCatalog | null; duplicateSource?: boolean; disabled?: boolean }>()
const emit = defineEmits<{ 'update:source': [value: string]; 'update:target': [value: string]; remove: [] }>()
const { t } = useI18n()
const checking = ref(false)
const waiting = ref(false)
const result = ref<QuotaMappingCheck | null>(null)
const failure = ref('')
const expired = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined
let expiryTimer: ReturnType<typeof setTimeout> | undefined
let controller: AbortController | undefined
let generation = 0

const modelOptions = computed(() => {
  const selected = new Set(props.accountIds)
  return [...(props.catalog?.models ?? [])].sort((a, b) => Number(b.account_ids.some(id => selected.has(id))) - Number(a.account_ids.some(id => selected.has(id))) || a.id.localeCompare(b.id)).map(model => ({
    value: model.id,
    label: `${model.id} · ${model.account_ids.map(id => props.catalog?.accounts.find(account => account.id === id)?.name || `#${id}`).join(', ')}`,
  }))
})
function invalidModelName(model: string) {
  return /[\s*]/.test(model) || Array.from(model).some(character => character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127) || new TextEncoder().encode(model).length > 200
}
const invalid = computed(() => !props.source || !props.target ? '' : props.source === props.target || props.duplicateSource || invalidModelName(props.source) || invalidModelName(props.target) ? t('quotaPolicy.validationMappings') : '')
const canCheck = computed(() => !!props.source && !!props.target && !!props.accountIds.length && !invalid.value)
const statusText = computed(() => {
  if (invalid.value) return invalid.value
  if (checking.value) return t('quotaPolicy.checkingMapping')
  if (waiting.value) return t('quotaPolicy.waitingCheck')
  if (failure.value) return failure.value
  if (expired.value) return t('quotaPolicy.checkExpired')
  if (!result.value) return t('quotaPolicy.checkUntested')
  return t(`quotaPolicy.check_${result.value.status}`, { passed: result.value.accounts.filter(account => account.status === 'passed').length, total: result.value.accounts.length })
})
const statusClass = computed(() => !invalid.value && !failure.value && !expired.value && !checking.value && !waiting.value && result.value?.status === 'passed' ? 'text-green-700 dark:text-green-400' : invalid.value || failure.value || expired.value || result.value?.status === 'failed' || result.value?.status === 'partial' ? 'text-amber-700 dark:text-amber-400' : 'text-gray-500 dark:text-gray-400')

function invalidate() {
  generation++
  clearTimeout(timer)
  clearTimeout(expiryTimer)
  controller?.abort()
  checking.value = false
  waiting.value = false
  result.value = null
  failure.value = ''
  expired.value = false
}
// Opening an existing policy never starts billable probes. Completed edits do.
watch(() => JSON.stringify([props.groupId, props.source, props.target, [...props.accountIds].sort((a, b) => a - b), props.catalog?.checked_at, props.duplicateSource]), (_next, previous) => {
  invalidate()
  // A catalog refresh invalidates results but should not by itself bill a test.
  const before = JSON.parse(previous) as unknown[]
  const after = JSON.parse(_next) as unknown[]
  if (before.slice(0, 4).join('|') === after.slice(0, 4).join('|') || !canCheck.value) return
  waiting.value = true
  timer = setTimeout(() => { void check(false) }, 650)
})

async function check(force: boolean) {
  if (!canCheck.value || checking.value) return
  clearTimeout(timer)
  clearTimeout(expiryTimer)
  const current = ++generation
  controller?.abort()
  controller = new AbortController()
  waiting.value = false
  checking.value = true
  result.value = null
  failure.value = ''
  expired.value = false
  try {
    const response = await policyAPI.validateMapping(props.groupId, { source: props.source, target: props.target, account_ids: [...props.accountIds], force }, controller.signal)
    if (current !== generation) return
    result.value = response
    if (response.expires_at) {
      const delay = response.expires_at * 1000 - Date.now()
      if (delay <= 0) expired.value = true
      else expiryTimer = setTimeout(() => { expired.value = true }, delay)
    }
  } catch (cause) {
    if (current !== generation) return
    const error = cause as { name?: string; code?: string; message?: string; status?: number }
    if (error.name !== 'CanceledError' && error.code !== 'ERR_CANCELED') failure.value = error.status === 400 && error.message ? error.message : t('quotaPolicy.checkRequestFailed')
  } finally {
    if (current === generation) checking.value = false
  }
}
function formatTime(seconds: number) { return new Date(seconds * 1000).toLocaleString() }
onBeforeUnmount(invalidate)
</script>
