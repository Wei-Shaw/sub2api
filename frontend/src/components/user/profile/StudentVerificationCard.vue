<template>
  <div v-if="loaded && status?.enabled" class="card" data-testid="student-verification-card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.studentVerification.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.studentVerification.description') }}
      </p>
    </div>
    <div class="px-6 py-6 space-y-4">
      <!-- Current record -->
      <template v-if="verification">
        <div
          class="flex items-start justify-between rounded-lg border px-4 py-3"
          :class="statusBorderClass"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-2">
              <span
                class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                :class="statusBadgeClass"
              >
                {{ statusLabel }}
              </span>
              <span class="truncate font-mono text-sm text-gray-700 dark:text-gray-300">
                {{ verification.email }}
              </span>
            </div>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              {{ t('profile.studentVerification.expiresAt') }}:
              {{ formatDate(verification.expires_at) }}
            </p>
          </div>
        </div>

        <!-- Entitlement summary -->
        <ul class="space-y-1 text-sm text-gray-600 dark:text-gray-300">
          <li v-if="status && status.group_count > 0">
            {{ t('profile.studentVerification.benefitGroups', { count: status.group_count }) }}
          </li>
          <li v-if="status && status.rebate_rate > 0">
            {{ t('profile.studentVerification.benefitRebate', { rate: status.rebate_rate }) }}
          </li>
        </ul>

        <p v-if="verification.status === 'revoked'" class="text-sm text-red-600 dark:text-red-400">
          {{ t('profile.studentVerification.revokedHint') }}
        </p>
      </template>

      <p v-else class="text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.studentVerification.notVerified') }}
      </p>

      <!-- Verify / renew form (hidden for revoked) -->
      <template v-if="!verification || verification.status !== 'revoked'">
        <div v-if="!formOpen" class="pt-1">
          <button type="button" class="btn btn-secondary btn-sm" @click="openForm">
            {{
              verification && verification.status === 'active'
                ? t('profile.studentVerification.renewOrChange')
                : t('profile.studentVerification.startVerify')
            }}
          </button>
        </div>

        <div v-else class="space-y-3 border-t border-gray-100 pt-4 dark:border-dark-700">
          <div>
            <label class="input-label">{{ t('profile.studentVerification.schoolEmail') }}</label>
            <div class="flex gap-2">
              <input
                v-model="email"
                type="email"
                class="input flex-1"
                :placeholder="t('profile.studentVerification.emailPlaceholder')"
                :disabled="codeSent"
              />
              <button
                type="button"
                class="btn btn-secondary whitespace-nowrap"
                :disabled="!email || sending || countdown > 0"
                @click="sendCode"
              >
                <template v-if="countdown > 0">
                  {{ t('profile.studentVerification.resendIn', { seconds: countdown }) }}
                </template>
                <template v-else>
                  {{ sending ? t('common.sending') : t('profile.studentVerification.sendCode') }}
                </template>
              </button>
            </div>
          </div>

          <div v-if="codeSent">
            <label class="input-label">{{ t('profile.studentVerification.code') }}</label>
            <div class="flex gap-2">
              <input
                v-model="code"
                type="text"
                maxlength="6"
                class="input w-32"
                :placeholder="t('profile.studentVerification.codePlaceholder')"
                @keyup.enter="verify"
              />
              <button
                type="button"
                class="btn btn-primary whitespace-nowrap"
                :disabled="code.length !== 6 || verifying"
                @click="verify"
              >
                {{ verifying ? t('common.loading') : t('profile.studentVerification.verify') }}
              </button>
              <button type="button" class="btn btn-secondary" @click="resetForm">
                {{ t('common.cancel') }}
              </button>
            </div>
            <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
              {{ t('profile.studentVerification.codeSentHint') }}
            </p>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  getStudentVerificationStatus,
  sendStudentVerificationCode,
  verifyStudentEmail,
  type StudentVerificationRecord,
  type StudentVerificationStatusResponse,
} from '@/api/studentVerification'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()

const loaded = ref(false)
const status = ref<StudentVerificationStatusResponse | null>(null)
const verification = computed<StudentVerificationRecord | null>(() => status.value?.verification ?? null)

const formOpen = ref(false)
const email = ref('')
const code = ref('')
const codeSent = ref(false)
const sending = ref(false)
const verifying = ref(false)
const countdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const statusLabel = computed(() => {
  switch (verification.value?.status) {
    case 'active':
      return t('profile.studentVerification.statusActive')
    case 'expired':
      return t('profile.studentVerification.statusExpired')
    case 'revoked':
      return t('profile.studentVerification.statusRevoked')
    default:
      return ''
  }
})

const statusBadgeClass = computed(() => {
  switch (verification.value?.status) {
    case 'active':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'expired':
      return 'bg-yellow-100 text-yellow-700 dark:bg-yellow-900/30 dark:text-yellow-300'
    default:
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
  }
})

const statusBorderClass = computed(() => {
  switch (verification.value?.status) {
    case 'active':
      return 'border-green-200 bg-green-50 dark:border-green-800 dark:bg-green-900/10'
    case 'expired':
      return 'border-yellow-200 bg-yellow-50 dark:border-yellow-800 dark:bg-yellow-900/10'
    default:
      return 'border-red-200 bg-red-50 dark:border-red-800 dark:bg-red-900/10'
  }
})

function formatDate(value: string): string {
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? value : d.toLocaleDateString()
}

async function loadStatus() {
  try {
    status.value = await getStudentVerificationStatus()
  } catch {
    status.value = null
  } finally {
    loaded.value = true
  }
}

function openForm() {
  formOpen.value = true
}

function resetForm() {
  formOpen.value = false
  codeSent.value = false
  code.value = ''
}

function startCountdown() {
  countdown.value = 60
  if (countdownTimer) clearInterval(countdownTimer)
  countdownTimer = setInterval(() => {
    countdown.value--
    if (countdown.value <= 0 && countdownTimer) {
      clearInterval(countdownTimer)
      countdownTimer = null
    }
  }, 1000)
}

async function sendCode() {
  const target = email.value.trim()
  if (!target) return
  sending.value = true
  try {
    await sendStudentVerificationCode(target)
    codeSent.value = true
    startCountdown()
    appStore.showSuccess(t('profile.studentVerification.codeSent'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    sending.value = false
  }
}

async function verify() {
  const target = email.value.trim()
  if (!target || code.value.length !== 6) return
  verifying.value = true
  try {
    status.value = await verifyStudentEmail(target, code.value)
    appStore.showSuccess(t('profile.studentVerification.verifySuccess'))
    resetForm()
    await loadStatus()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    verifying.value = false
  }
}

onMounted(loadStatus)
onUnmounted(() => {
  if (countdownTimer) clearInterval(countdownTimer)
})
</script>
