<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.balanceNotify.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.balanceNotify.description') }}
      </p>
    </div>
    <div class="px-6 py-6 space-y-6">
      <!-- Enable toggle -->
      <div class="flex items-center justify-between">
        <label class="input-label mb-0">{{ t('profile.balanceNotify.enabled') }}</label>
        <label class="relative inline-flex items-center cursor-pointer">
          <input type="checkbox" v-model="notifyEnabled" @change="handleToggle" class="sr-only peer" />
          <div class="w-11 h-6 bg-gray-200 peer-focus:outline-none peer-focus:ring-4 peer-focus:ring-primary-300 dark:peer-focus:ring-primary-800 rounded-full peer dark:bg-gray-700 peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-5 after:w-5 after:transition-all dark:after:border-gray-600 peer-checked:bg-primary-600"></div>
        </label>
      </div>

      <template v-if="notifyEnabled">
        <!-- Custom threshold -->
        <div>
          <label class="input-label">
            {{ t('profile.balanceNotify.threshold') }}
            <span class="text-xs text-gray-400 ml-2">{{ t('profile.balanceNotify.thresholdHint') }}</span>
          </label>
          <div class="flex items-center gap-2">
            <span class="text-gray-500">$</span>
            <input
              v-model.number="customThreshold"
              type="number"
              min="0"
              step="0.01"
              class="input flex-1"
              :placeholder="systemDefaultThreshold > 0 ? `${t('profile.balanceNotify.systemDefault')} $${systemDefaultThreshold}` : t('profile.balanceNotify.thresholdPlaceholder')"
              @blur="handleThresholdUpdate"
            />
          </div>
        </div>

        <!-- Primary email (always shown, with toggle) -->
        <div>
          <label class="input-label">{{ t('profile.balanceNotify.extraEmails') }}</label>
          <div class="space-y-2 mb-3">
            <div class="flex items-center justify-between px-3 py-2 bg-gray-50 dark:bg-dark-700 rounded-lg">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ userEmail }}</span>
              <span class="text-xs text-gray-400">{{ t('profile.balanceNotify.primaryEmail') }}</span>
            </div>
          </div>

          <!-- Verified extra emails -->
          <div v-if="extraEmails.length > 0" class="space-y-2 mb-3">
            <div v-for="email in extraEmails" :key="email"
              class="flex items-center justify-between px-3 py-2 bg-gray-50 dark:bg-dark-700 rounded-lg">
              <span class="text-sm text-gray-700 dark:text-gray-300">{{ email }}</span>
              <button @click="handleRemoveEmail(email)" class="text-red-500 hover:text-red-700 text-sm">
                {{ t('profile.balanceNotify.removeEmail') }}
              </button>
            </div>
          </div>

          <!-- Pending (unverified) emails -->
          <div v-if="pendingEmails.length > 0" class="space-y-2 mb-3">
            <div v-for="(pe, idx) in pendingEmails" :key="pe.email"
              class="flex items-center gap-2 px-3 py-2 bg-yellow-50 dark:bg-yellow-900/10 rounded-lg border border-yellow-200 dark:border-yellow-800">
              <span class="flex-1 text-sm text-gray-700 dark:text-gray-300">{{ pe.email }}</span>
              <div v-if="!pe.codeSent" class="flex items-center gap-1">
                <button @click="sendCodeFor(idx)" :disabled="pe.sending" class="text-xs text-primary-600 hover:text-primary-700">
                  {{ t('profile.balanceNotify.sendCode') }}
                </button>
                <button @click="pendingEmails.splice(idx, 1)" class="text-xs text-red-500 hover:text-red-700 ml-1">
                  {{ t('profile.balanceNotify.removeEmail') }}
                </button>
              </div>
              <div v-else class="flex items-center gap-1">
                <input
                  v-model="pe.code"
                  type="text"
                  maxlength="6"
                  class="w-20 rounded border border-gray-300 px-2 py-1 text-xs dark:border-dark-500 dark:bg-dark-700"
                  :placeholder="t('profile.balanceNotify.codePlaceholder')"
                />
                <button @click="verifyPending(idx)" :disabled="!pe.code || pe.code.length !== 6 || pe.verifying" class="text-xs text-primary-600 hover:text-primary-700">
                  {{ t('profile.balanceNotify.verify') }}
                </button>
                <span v-if="pe.countdown > 0" class="text-xs text-gray-400">{{ pe.countdown }}s</span>
                <button v-else @click="sendCodeFor(idx)" :disabled="pe.sending" class="text-xs text-gray-500 hover:text-gray-700">
                  {{ t('profile.balanceNotify.resend') }}
                </button>
              </div>
            </div>
          </div>

          <!-- Add new email input -->
          <div class="flex gap-2">
            <input
              v-model="newEmail"
              type="email"
              class="input flex-1"
              :placeholder="t('profile.balanceNotify.emailPlaceholder')"
              @keyup.enter="addPendingEmail"
            />
            <button
              @click="addPendingEmail"
              :disabled="!newEmail"
              class="btn btn-secondary whitespace-nowrap"
            >
              {{ t('common.add') }}
            </button>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'

interface PendingEmail {
  email: string
  codeSent: boolean
  code: string
  sending: boolean
  verifying: boolean
  countdown: number
  timer: ReturnType<typeof setInterval> | null
}

const props = defineProps<{
  enabled: boolean
  threshold: number | null
  extraEmails: string[]
  systemDefaultThreshold: number
  userEmail: string
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const notifyEnabled = ref(props.enabled)
const customThreshold = ref<number | null>(props.threshold)
const extraEmails = ref<string[]>([...props.extraEmails])
const pendingEmails = ref<PendingEmail[]>([])
const newEmail = ref('')

watch(() => props.enabled, (val) => { notifyEnabled.value = val })
watch(() => props.threshold, (val) => { customThreshold.value = val })
watch(() => props.extraEmails, (val) => { extraEmails.value = [...val] })

onUnmounted(() => {
  for (const pe of pendingEmails.value) {
    if (pe.timer) clearInterval(pe.timer)
  }
})

const handleToggle = async () => {
  try {
    const updated = await userAPI.updateProfile({ balance_notify_enabled: notifyEnabled.value })
    authStore.user = updated
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
    notifyEnabled.value = !notifyEnabled.value
  }
}

const handleThresholdUpdate = async () => {
  try {
    const threshold = customThreshold.value && customThreshold.value > 0 ? customThreshold.value : 0
    const updated = await userAPI.updateProfile({ balance_notify_threshold: threshold })
    authStore.user = updated
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

function addPendingEmail() {
  const email = newEmail.value.trim()
  if (!email) return
  if (email === props.userEmail || extraEmails.value.includes(email) || pendingEmails.value.some(p => p.email === email)) {
    appStore.showError(t('common.error'))
    return
  }
  pendingEmails.value.push({ email, codeSent: false, code: '', sending: false, verifying: false, countdown: 0, timer: null })
  newEmail.value = ''
}

async function sendCodeFor(idx: number) {
  const pe = pendingEmails.value[idx]
  if (!pe) return
  pe.sending = true
  try {
    await userAPI.sendNotifyEmailCode(pe.email)
    pe.codeSent = true
    pe.countdown = 60
    pe.timer = setInterval(() => {
      pe.countdown--
      if (pe.countdown <= 0 && pe.timer) {
        clearInterval(pe.timer)
        pe.timer = null
      }
    }, 1000)
    appStore.showSuccess(t('profile.balanceNotify.codeSent'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    pe.sending = false
  }
}

async function verifyPending(idx: number) {
  const pe = pendingEmails.value[idx]
  if (!pe || !pe.code || pe.code.length !== 6) return
  pe.verifying = true
  try {
    await userAPI.verifyNotifyEmail(pe.email, pe.code)
    extraEmails.value.push(pe.email)
    if (pe.timer) clearInterval(pe.timer)
    pendingEmails.value.splice(idx, 1)
    appStore.showSuccess(t('profile.balanceNotify.verifySuccess'))
    const updated = await userAPI.getProfile()
    authStore.user = updated
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    pe.verifying = false
  }
}

const handleRemoveEmail = async (email: string) => {
  try {
    await userAPI.removeNotifyEmail(email)
    extraEmails.value = extraEmails.value.filter(e => e !== email)
    appStore.showSuccess(t('profile.balanceNotify.removeSuccess'))
    const updated = await userAPI.getProfile()
    authStore.user = updated
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}
</script>
