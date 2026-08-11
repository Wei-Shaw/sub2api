<template>
  <BaseDialog
    :show="show"
    :title="t('admin.accounts.tempUnschedulable.statusTitle')"
    width="normal"
    @close="handleClose"
  >
    <div class="space-y-4">
      <div v-if="loading" class="flex items-center justify-center py-8">
        <svg class="h-6 w-6 animate-spin text-gray-400" fill="none" viewBox="0 0 24 24">
          <circle
            class="opacity-25"
            cx="12"
            cy="12"
            r="10"
            stroke="currentColor"
            stroke-width="4"
          ></circle>
          <path
            class="opacity-75"
            fill="currentColor"
            d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
          ></path>
        </svg>
      </div>

      <div v-else-if="!isActive" class="rounded-lg border border-gray-200 p-4 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
        {{ t('admin.accounts.tempUnschedulable.notActive') REDACTEDREDACTED
      </div>

      <div v-else class="space-y-4">
        <div class="rounded-lg border border-emerald-200 bg-emerald-50 p-3 text-sm text-emerald-800 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-300">
          {{ t('admin.accounts.recoverStateHint') REDACTEDREDACTED
        </div>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-600">
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.tempUnschedulable.accountName') REDACTEDREDACTED
          </p>
          <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
            {{ account?.name || '-' REDACTEDREDACTED
          </p>
        </div>

        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.triggeredAt') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ triggeredAtText REDACTEDREDACTED
            </p>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.until') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ untilText REDACTEDREDACTED
            </p>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.remaining') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ remainingText REDACTEDREDACTED
            </p>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.errorCode') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ state?.status_code || '-' REDACTEDREDACTED
            </p>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.matchedKeyword') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ state?.matched_keyword || '-' REDACTEDREDACTED
            </p>
          </div>
          <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
            <p class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.accounts.tempUnschedulable.ruleOrder') REDACTEDREDACTED
            </p>
            <p class="mt-1 text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ ruleIndexDisplay REDACTEDREDACTED
            </p>
          </div>
        </div>

        <div class="rounded-lg border border-gray-200 p-3 dark:border-dark-600">
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.accounts.tempUnschedulable.errorMessage') REDACTEDREDACTED
          </p>
          <div class="mt-2 rounded bg-gray-50 p-2 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-300">
            {{ state?.error_message || '-' REDACTEDREDACTED
          </div>
        </div>

        <div
          v-if="hasThresholdEvidence"
          class="rounded-lg border border-blue-200 bg-blue-50 p-3 text-sm text-blue-800 dark:border-blue-500/30 dark:bg-blue-500/10 dark:text-blue-300"
          data-testid="temp-unsched-trigger-evidence"
        >
          {{ triggerEvidenceText REDACTEDREDACTED
        </div>
      </div>
    </div>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.close') REDACTEDREDACTED
        </button>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="!isActive || resetting"
          @click="handleReset"
        >
          <svg
            v-if="resetting"
            class="-ml-1 mr-2 h-4 w-4 animate-spin"
            fill="none"
            viewBox="0 0 24 24"
          >
            <circle
              class="opacity-25"
              cx="12"
              cy="12"
              r="10"
              stroke="currentColor"
              stroke-width="4"
            ></circle>
            <path
              class="opacity-75"
              fill="currentColor"
              d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
            ></path>
          </svg>
          {{ t('admin.accounts.recoverState') REDACTEDREDACTED
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore REDACTED from '@/stores/app'
import { adminAPI REDACTED from '@/api/admin'
import type { Account, TempUnschedulableStatus REDACTED from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { formatDateTime REDACTED from '@/utils/format'

const props = defineProps<{
  show: boolean
  account: Account | null
REDACTED>()

const emit = defineEmits<{
  close: []
  reset: [account: Account]
REDACTED>()

const { t REDACTED = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const resetting = ref(false)
const status = ref<TempUnschedulableStatus | null>(null)

const state = computed(() => status.value?.state || null)

const isActive = computed(() => {
  if (!status.value?.active || !state.value) return false
  return state.value.until_unix * 1000 > Date.now()
REDACTED)

const ruleIndexDisplay = computed(() => {
  if (!state.value || !state.value.matched_keyword || state.value.rule_index < 0) return '-'
  return state.value.rule_index + 1
REDACTED)

const hasThresholdEvidence = computed(() => (state.value?.trigger_count || 0) > 1)

const triggerEvidenceText = computed(() => {
  const count = state.value?.trigger_count || 0
  const threshold = state.value?.trigger_threshold || 0
  const minutes = state.value?.trigger_window_minutes || 0
  if (threshold > 0 && minutes > 0) {
    return t('admin.accounts.tempUnschedulable.multipleErrorTrigger', { count, threshold, minutes REDACTED)
  REDACTED
  if (threshold > 0) {
    return t('admin.accounts.tempUnschedulable.multipleErrorTriggerNoWindow', { count, threshold REDACTED)
  REDACTED
  if (minutes > 0) {
    return t('admin.accounts.tempUnschedulable.multipleErrorCountInWindow', { count, minutes REDACTED)
  REDACTED
  return t('admin.accounts.tempUnschedulable.multipleErrorCount', { count REDACTED)
REDACTED)

const triggeredAtText = computed(() => {
  if (!state.value?.triggered_at_unix) return '-'
  return formatDateTime(new Date(state.value.triggered_at_unix * 1000))
REDACTED)

const untilText = computed(() => {
  if (!state.value?.until_unix) return '-'
  return formatDateTime(new Date(state.value.until_unix * 1000))
REDACTED)

const remainingText = computed(() => {
  if (!state.value) return '-'
  const remainingMs = state.value.until_unix * 1000 - Date.now()
  if (remainingMs <= 0) {
    return t('admin.accounts.tempUnschedulable.expired')
  REDACTED
  const minutes = Math.ceil(remainingMs / 60000)
  if (minutes < 60) {
    return t('admin.accounts.tempUnschedulable.remainingMinutes', { minutes REDACTED)
  REDACTED
  const hours = Math.floor(minutes / 60)
  const rest = minutes % 60
  if (rest === 0) {
    return t('admin.accounts.tempUnschedulable.remainingHours', { hours REDACTED)
  REDACTED
  return t('admin.accounts.tempUnschedulable.remainingHoursMinutes', { hours, minutes: rest REDACTED)
REDACTED)

const loadStatus = async () => {
  if (!props.account) return
  loading.value = true
  try {
    status.value = await adminAPI.accounts.getTempUnschedulableStatus(props.account.id)
  REDACTED catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.tempUnschedulable.failedToLoad'))
    status.value = null
  REDACTED finally {
    loading.value = false
  REDACTED
REDACTED

const handleClose = () => {
  emit('close')
REDACTED

const handleReset = async () => {
  if (!props.account) return
  resetting.value = true
  try {
    const updated = await adminAPI.accounts.recoverState(props.account.id)
    appStore.showSuccess(t('admin.accounts.recoverStateSuccess'))
    emit('reset', updated)
    handleClose()
  REDACTED catch (error: any) {
    appStore.showError(error?.message || t('admin.accounts.recoverStateFailed'))
  REDACTED finally {
    resetting.value = false
  REDACTED
REDACTED

watch(
  () => [props.show, props.account?.id],
  ([visible]) => {
    if (visible && props.account) {
      loadStatus()
      return
    REDACTED
    status.value = null
  REDACTED
)
</script>
