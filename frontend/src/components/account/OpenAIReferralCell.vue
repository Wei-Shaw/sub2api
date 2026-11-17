<template>
  <div class="flex flex-wrap items-center gap-1.5">
    <button
      type="button"
      data-testid="referral-count"
      class="rounded px-1.5 py-0.5 text-[10px] font-medium text-violet-600 hover:bg-violet-50 disabled:opacity-50 dark:text-violet-400 dark:hover:bg-violet-900/30"
      :disabled="loading || sending"
      :title="countTitle"
      @click="refresh()"
    >
      {{ t('admin.accounts.openaiReferral.available') }} {{ countDisplay }}
    </button>
    <button
      type="button"
      data-testid="referral-open"
      class="rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 hover:bg-blue-50 disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
      :disabled="sending || isShadow"
      :title="isShadow ? t('admin.accounts.openaiReferral.shadowHint') : undefined"
      @click="openDialog"
    >
      {{ t('admin.accounts.openaiReferral.invite') }}
    </button>
    <span v-if="error && !show" class="max-w-48 truncate text-[10px] text-red-600" :title="error">{{ error }}</span>
    <span v-if="warning && !show" class="max-w-48 truncate text-[10px] text-amber-700 dark:text-amber-400" :title="warning">{{ warning }}</span>
    <BaseDialog
      v-if="show"
      :show="show"
      :title="t('admin.accounts.openaiReferral.invite')"
      :show-close-button="!sending"
      :close-on-escape="!sending"
      @close="closeDialog"
    >
      <form :id="formID" class="space-y-4" @submit.prevent="sendInvite">
        <p class="text-sm text-gray-600 dark:text-gray-300">
          {{ t('admin.accounts.openaiReferral.fromAccount') }} <strong>{{ account.name }}</strong>
        </p>
        <div class="flex items-center justify-between rounded-lg bg-violet-50 p-3 text-sm dark:bg-violet-900/20">
          <div>
            <p class="font-medium text-violet-800 dark:text-violet-300">{{ programLabel }}</p>
            <p class="mt-1 text-gray-600 dark:text-gray-300">{{ t('admin.accounts.openaiReferral.available') }} {{ countDisplay }}</p>
          </div>
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading || sending" @click="refresh()">
            {{ loading ? t('common.loading') : t('common.refresh') }}
          </button>
        </div>
        <div v-if="eligibility?.title || eligibility?.description || eligibility?.rules?.length" class="space-y-2 text-sm text-gray-600 dark:text-gray-300">
          <p v-if="eligibility.title" class="font-medium">{{ eligibility.title }}</p>
          <p v-if="eligibility.description">{{ eligibility.description }}</p>
          <ul v-if="eligibility.rules?.length" class="list-disc space-y-1 pl-5">
            <li v-for="(rule, index) in eligibility.rules" :key="index">{{ rule }}</li>
          </ul>
        </div>
        <p v-if="fresh && (!eligibility?.should_show || count === null || count <= 0)" class="text-sm text-amber-700 dark:text-amber-400">
          {{ t('admin.accounts.openaiReferral.unavailable') }}
        </p>
        <div>
          <label :for="emailID" class="input-label">{{ t('admin.accounts.openaiReferral.email') }}</label>
          <input
            :id="emailID"
            v-model="email"
            type="email"
            class="input"
            autocomplete="off"
            maxlength="254"
            required
            :disabled="sending"
            placeholder="friend@example.com"
            data-testid="referral-email"
          />
        </div>
        <label v-if="needsConsent" class="flex items-start gap-2 text-sm text-gray-600 dark:text-gray-300">
          <input v-model="confirmed" type="checkbox" class="mt-1" :disabled="sending" data-testid="referral-consent" />
          {{ t('admin.accounts.openaiReferral.consent') }}
        </label>
        <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        <p v-if="sentEmail" role="status" class="text-sm text-emerald-700 dark:text-emerald-400">
          {{ t('admin.accounts.openaiReferral.sent', { email: sentEmail }) }}
        </p>
        <p v-if="warning" role="status" class="text-sm text-amber-700 dark:text-amber-400">{{ warning }}</p>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="sending" @click="closeDialog">{{ t('common.close') }}</button>
        <button type="submit" :form="formID" class="btn btn-primary" :disabled="!canSend" data-testid="referral-send">
          {{ sending ? t('admin.accounts.openaiReferral.sending') : t('admin.accounts.openaiReferral.send') }}
        </button>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import type { OpenAIReferralEligibility } from '@/types/openaiReferrals'
import { refreshOpenAIReferrals, sendOpenAIReferralInvite } from '@/api/admin/accounts'
import BaseDialog from '@/components/common/BaseDialog.vue'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const eligibility = ref<OpenAIReferralEligibility | null>(props.account.extra?.codex_referral_snapshot ?? null)
const show = ref(false)
const loading = ref(false)
const sending = ref(false)
const fresh = ref(false)
const email = ref('')
const confirmed = ref(false)
const error = ref('')
const warning = ref('')
const sentEmail = ref('')
let generation = 0
const isShadow = computed(() => props.account.parent_account_id != null)
const formID = computed(() => `codex-referral-form-${props.account.id}`)
const emailID = computed(() => `codex-referral-email-${props.account.id}`)
const count = computed(() => {
  const value = eligibility.value?.available_invites
  return typeof value === 'number' && Number.isSafeInteger(value) && value >= 0 ? value : null
})
const countDisplay = computed(() => count.value ?? '—')
const countTitle = computed(() => {
  const time = eligibility.value?.fetched_at
  return time ? t('admin.accounts.openaiReferral.checkedAt', { time: new Date(time * 1000).toLocaleString() }) : t('admin.accounts.openaiReferral.queryHint')
})
const programLabel = computed(() => t(eligibility.value?.program_id === 'codex_referral_workspace'
  ? 'admin.accounts.openaiReferral.workspace' : 'admin.accounts.openaiReferral.personal'))
const needsConsent = computed(() => eligibility.value?.requires_explicit_confirmation !== false)
const canSend = computed(() => fresh.value && !isShadow.value && !loading.value && !sending.value
  && eligibility.value?.should_show === true && count.value !== null && count.value > 0
  && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.value.trim()) && (!needsConsent.value || confirmed.value))

function errorMessage(value: unknown, duringSend = false): string {
  const err = value as { status?: number; reason?: string; code?: string; message?: string }
  const key = err.reason || err.code || ''
  // The server may have sent the email before the response was lost. Only
  // sends have an uncertain outcome; a failed eligibility query is read-only.
  if (duringSend && (err.status === 0 || ['ECONNABORTED', 'ETIMEDOUT', 'ERR_NETWORK', 'ERR_CANCELED'].includes(key))) {
    return t('admin.accounts.openaiReferral.sendUnknown')
  }
  const keys: Record<string, string> = {
    OPENAI_REFERRAL_UNAVAILABLE: 'unavailable', OPENAI_REFERRAL_FORBIDDEN: 'unavailable',
    OPENAI_REFERRAL_INVALID_EMAIL: 'invalidEmail', OPENAI_REFERRAL_REJECTED: 'rejected',
    OPENAI_REFERRAL_ALREADY_EXISTS: 'alreadyInvited', OPENAI_REFERRAL_RATE_LIMITED: 'rateLimited',
    OPENAI_REFERRAL_SEND_UNKNOWN: 'sendUnknown',
    OPENAI_REFERRAL_PROGRAM_CHANGED: 'programChanged', OPENAI_REFERRAL_CONFIRMATION_REQUIRED: 'consentRequired',
  }
  return keys[key] ? t(`admin.accounts.openaiReferral.${keys[key]}`) : err.message || t('common.error')
}

async function refresh() {
  if (loading.value || sending.value) return
  const current = generation
  loading.value = true
  fresh.value = false
  error.value = ''
  warning.value = ''
  try {
    const result = await refreshOpenAIReferrals(props.account.id)
    if (current !== generation) return
    eligibility.value = result.eligibility
    fresh.value = result.eligibility !== null
    if (!result.cache_persisted) warning.value = t('admin.accounts.openaiReferral.cacheFailed')
  } catch (err) {
    if (current === generation) error.value = errorMessage(err)
  } finally {
    if (current === generation) loading.value = false
  }
}

function openDialog() {
  if (isShadow.value || sending.value) return
  show.value = true
  email.value = ''
  confirmed.value = false
  sentEmail.value = ''
  void refresh()
}

function closeDialog() {
  if (!sending.value) show.value = false
}

async function sendInvite() {
  if (!canSend.value || !eligibility.value) return
  const current = generation
  sending.value = true
  error.value = ''
  warning.value = ''
  sentEmail.value = ''
  try {
    const result = await sendOpenAIReferralInvite(props.account.id, {
      email: email.value.trim(), program_id: eligibility.value.program_id, confirmed: confirmed.value,
    })
    if (current !== generation) return
    if (!result.sent) throw { code: 'OPENAI_REFERRAL_SEND_UNKNOWN' }
    eligibility.value = result.eligibility
    fresh.value = !result.refresh_failed && result.eligibility !== null
    sentEmail.value = result.email
    email.value = ''
    confirmed.value = false
    if (result.refresh_failed) warning.value = t('admin.accounts.openaiReferral.refreshFailed')
    else if (!result.cache_persisted) warning.value = t('admin.accounts.openaiReferral.cacheFailed')
  } catch (err) {
    if (current !== generation) return
    fresh.value = false
    error.value = errorMessage(err, true)
  } finally {
    if (current === generation) sending.value = false
  }
}

watch(email, () => { confirmed.value = false })
watch(() => props.account.id, () => {
  generation++
  eligibility.value = props.account.extra?.codex_referral_snapshot ?? null
  show.value = false
  fresh.value = false
  loading.value = false
  sending.value = false
  email.value = ''
  confirmed.value = false
  error.value = ''
  warning.value = ''
  sentEmail.value = ''
})
</script>
