<template>
  <div class="rounded border border-line bg-surface">
    <div class="border-b border-line px-4 py-3">
      <h2 class="text-sm font-semibold text-ink">
        {{ t('profile.balanceNotify.title') }}
      </h2>
      <p class="mt-0.5 text-xs text-ink-tertiary">
        {{ t('profile.balanceNotify.description') }}
      </p>
    </div>
    <div class="space-y-5 px-4 py-4">
      <!--
        Enable. Was a peer-checkbox pill built from fourteen utilities and two
        hardcoded `dark:` grays; `.switch` is the tokenized 2px-radius track.
      -->
      <div class="flex items-center justify-between gap-4">
        <span id="balance-notify-enabled-label" class="text-xs font-medium text-ink-secondary">
          {{ t('profile.balanceNotify.enabled') }}
        </span>
        <button
          type="button"
          role="switch"
          class="switch"
          :class="{ 'switch-active': notifyEnabled }"
          :aria-checked="notifyEnabled"
          aria-labelledby="balance-notify-enabled-label"
          @click="toggleNotifyEnabled"
        >
          <span class="switch-thumb" aria-hidden="true"></span>
        </button>
      </div>

      <template v-if="notifyEnabled">
        <!-- Custom threshold with save button -->
        <FormField
          id="balance-notify-threshold"
          :label="t('profile.balanceNotify.threshold')"
          :hint="t('profile.balanceNotify.thresholdHint')"
        >
          <template #default="{ describedBy }">
            <div class="flex items-start gap-2">
              <div class="relative min-w-0 flex-1">
                <span
                  class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3 font-mono text-xs text-ink-tertiary"
                  aria-hidden="true"
                >
                  $
                </span>
                <input
                  id="balance-notify-threshold"
                  v-model.number="customThreshold"
                  type="number"
                  min="0"
                  step="0.01"
                  class="input pl-7 font-mono tabular-nums"
                  :aria-describedby="describedBy"
                  :placeholder="thresholdPlaceholder"
                />
              </div>
              <Button
                tone="accent"
                variant="solid"
                size="md"
                class="h-9 shrink-0"
                :loading="savingThreshold"
                @click="handleThresholdUpdate"
              >
                {{ t('common.save') }}
              </Button>
            </div>
          </template>
        </FormField>

        <!-- Email list with toggles -->
        <div>
          <p class="text-xs font-medium text-ink-secondary">
            {{ t('profile.balanceNotify.extraEmails') }}
          </p>
          <p class="mt-1 text-2xs text-ink-tertiary">
            {{ t('profile.balanceNotify.extraEmailsHint') }}
          </p>

          <!-- Saved email entries -->
          <div v-if="emailEntries.length > 0" class="mt-3 divide-y divide-line-subtle border-y border-line-subtle">
            <div
              v-for="(entry, idx) in emailEntries"
              :key="idx"
              class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 py-2"
            >
              <div class="flex min-w-0 flex-1 items-center gap-2">
                <button
                  type="button"
                  role="switch"
                  class="switch h-4 w-8 shrink-0"
                  :class="{ 'switch-active': !entry.disabled }"
                  :aria-checked="!entry.disabled"
                  :aria-label="entry.email"
                  @click="handleEmailToggle(entry)"
                >
                  <span class="switch-thumb h-2.5 w-2.5" aria-hidden="true"></span>
                </button>
                <span class="truncate text-sm text-ink">{{ entry.email }}</span>
              </div>
              <div class="flex shrink-0 items-center gap-2">
                <template v-if="!entry.verified">
                  <!-- Inline verify flow for saved unverified emails -->
                  <template v-if="verifyingEmail === entry.email">
                    <input
                      v-model="verifyCode"
                      type="text"
                      inputmode="numeric"
                      maxlength="6"
                      class="input h-7 w-20 px-2 font-mono text-xs tabular-nums"
                      :aria-label="t('profile.balanceNotify.codePlaceholder')"
                      :placeholder="t('profile.balanceNotify.codePlaceholder')"
                    />
                    <Button
                      size="xs"
                      :disabled="!verifyCode || verifyCode.length !== 6"
                      :loading="verifyingSaved"
                      @click="verifySavedEmail(entry.email)"
                    >
                      {{ t('profile.balanceNotify.verify') }}
                    </Button>
                    <span
                      v-if="verifyCountdown > 0"
                      class="font-mono text-2xs tabular-nums text-ink-tertiary"
                    >
                      {{ verifyCountdown }}s
                    </span>
                    <Button
                      v-else
                      size="xs"
                      variant="quiet"
                      :loading="sendingSavedCode"
                      @click="sendCodeForSaved(entry.email)"
                    >
                      {{ t('profile.balanceNotify.resend') }}
                    </Button>
                    <Button size="xs" variant="quiet" @click="verifyingEmail = ''">
                      {{ t('common.cancel') }}
                    </Button>
                  </template>
                  <template v-else>
                    <StatusDot tone="warn" :label="t('profile.balanceNotify.unverified')" />
                    <Button
                      size="xs"
                      :loading="sendingSavedCode"
                      @click="sendCodeForSaved(entry.email)"
                    >
                      {{ t('profile.balanceNotify.verify') }}
                    </Button>
                  </template>
                </template>
                <StatusDot
                  v-else
                  tone="success"
                  muted
                  :label="t('profile.balanceNotify.verified')"
                />
                <Button size="xs" variant="quiet" tone="danger" @click="handleRemoveEmail(entry.email)">
                  {{ t('profile.balanceNotify.removeEmail') }}
                </Button>
              </div>
            </div>
          </div>

          <!-- Pending (unverified) emails in verification flow -->
          <div v-if="pendingEmails.length > 0" class="mt-3 space-y-2">
            <div
              v-for="(pe, idx) in pendingEmails"
              :key="pe.email"
              class="flex flex-wrap items-center gap-x-2 gap-y-2 border border-line bg-surface-sunken px-3 py-2"
            >
              <span class="min-w-0 flex-1 truncate text-sm text-ink">{{ pe.email }}</span>
              <div v-if="!pe.codeSent" class="flex shrink-0 items-center gap-2">
                <Button size="xs" :loading="pe.sending" @click="sendCodeFor(idx)">
                  {{ t('profile.balanceNotify.sendCode') }}
                </Button>
                <Button size="xs" variant="quiet" tone="danger" @click="pendingEmails.splice(idx, 1)">
                  {{ t('profile.balanceNotify.removeEmail') }}
                </Button>
              </div>
              <div v-else class="flex shrink-0 items-center gap-2">
                <input
                  v-model="pe.code"
                  type="text"
                  inputmode="numeric"
                  maxlength="6"
                  class="input h-7 w-20 px-2 font-mono text-xs tabular-nums"
                  :aria-label="t('profile.balanceNotify.codePlaceholder')"
                  :placeholder="t('profile.balanceNotify.codePlaceholder')"
                />
                <Button
                  size="xs"
                  :disabled="!pe.code || pe.code.length !== 6"
                  :loading="pe.verifying"
                  @click="verifyPending(idx)"
                >
                  {{ t('profile.balanceNotify.verify') }}
                </Button>
                <span
                  v-if="pe.countdown > 0"
                  class="font-mono text-2xs tabular-nums text-ink-tertiary"
                >
                  {{ pe.countdown }}s
                </span>
                <Button v-else size="xs" variant="quiet" :loading="pe.sending" @click="sendCodeFor(idx)">
                  {{ t('profile.balanceNotify.resend') }}
                </Button>
              </div>
            </div>
          </div>

          <!-- Add new email input (hidden when at limit) -->
          <div v-if="canAddMore" class="mt-3 flex items-start gap-2">
            <input
              v-model="newEmail"
              type="email"
              class="input min-w-0 flex-1"
              :aria-label="t('profile.balanceNotify.emailPlaceholder')"
              :placeholder="t('profile.balanceNotify.emailPlaceholder')"
              @keyup.enter="addPendingEmail"
            />
            <Button size="md" class="h-9 shrink-0" :disabled="!newEmail" @click="addPendingEmail">
              {{ t('common.add') }}
            </Button>
          </div>
          <p v-else class="mt-3 text-xs text-ink-tertiary">
            {{ t('profile.balanceNotify.maxEmailsReached') }}
          </p>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import StatusDot from '@/components/common/StatusDot.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { userAPI } from '@/api'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { NotifyEmailEntry } from '@/types'

const maxTotalEmails = 3

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
  extraEmails: NotifyEmailEntry[]
  systemDefaultThreshold: number
  userEmail: string
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const notifyEnabled = ref(props.enabled)
const customThreshold = ref<number | null>(props.threshold)
const emailEntries = ref<NotifyEmailEntry[]>([...props.extraEmails])
const pendingEmails = ref<PendingEmail[]>([])
const newEmail = ref('')
const savingThreshold = ref(false)

// State for verifying saved unverified emails
const verifyingEmail = ref('')
const verifyCode = ref('')
const verifyingSaved = ref(false)
const sendingSavedCode = ref(false)
const verifyCountdown = ref(0)
let verifyTimer: ReturnType<typeof setInterval> | null = null

const canAddMore = computed(() => {
  return emailEntries.value.length + pendingEmails.value.length < maxTotalEmails
})

const thresholdPlaceholder = computed(() =>
  props.systemDefaultThreshold > 0
    ? `${t('profile.balanceNotify.systemDefault')} $${props.systemDefaultThreshold}`
    : t('profile.balanceNotify.thresholdPlaceholder')
)

watch(() => props.enabled, (val) => { notifyEnabled.value = val })
watch(() => props.threshold, (val) => { customThreshold.value = val })
watch(() => props.extraEmails, (val) => { emailEntries.value = [...val] })

// When list is empty on mount, pre-fill the add input with user's email
onMounted(() => {
  if (emailEntries.value.length === 0 && props.userEmail) {
    newEmail.value = props.userEmail
  }
})

onUnmounted(() => {
  for (const pe of pendingEmails.value) {
    if (pe.timer) clearInterval(pe.timer)
  }
  if (verifyTimer) clearInterval(verifyTimer)
})

/**
 * The switch is a `role="switch"` button now rather than a hidden checkbox, so
 * the flip is explicit instead of riding `v-model` + `@change`. The optimistic
 * write and its rollback are unchanged.
 */
const toggleNotifyEnabled = async () => {
  notifyEnabled.value = !notifyEnabled.value
  await handleToggle()
}

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
  savingThreshold.value = true
  try {
    const threshold = customThreshold.value && customThreshold.value > 0 ? customThreshold.value : 0
    const updated = await userAPI.updateProfile({ balance_notify_threshold: threshold })
    authStore.user = updated
    appStore.showSuccess(t('common.saved'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    savingThreshold.value = false
  }
}

async function handleEmailToggle(entry: NotifyEmailEntry) {
  const newDisabled = !entry.disabled
  try {
    const updated = await userAPI.toggleNotifyEmail(entry.email, newDisabled)
    authStore.user = updated
    emailEntries.value = [...updated.balance_notify_extra_emails]
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

function addPendingEmail() {
  const email = newEmail.value.trim()
  if (!email) return
  // Check duplicates
  const isDuplicate = emailEntries.value.some(e => e.email.toLowerCase() === email.toLowerCase())
    || pendingEmails.value.some(p => p.email.toLowerCase() === email.toLowerCase())
  if (isDuplicate) {
    appStore.showError(t('profile.balanceNotify.emailDuplicate'))
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
    if (pe.timer) clearInterval(pe.timer)
    pendingEmails.value.splice(idx, 1)
    appStore.showSuccess(t('profile.balanceNotify.verifySuccess'))
    const updated = await userAPI.getProfile()
    authStore.user = updated
    emailEntries.value = [...updated.balance_notify_extra_emails]
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    pe.verifying = false
  }
}

const handleRemoveEmail = async (email: string) => {
  try {
    await userAPI.removeNotifyEmail(email)
    appStore.showSuccess(t('profile.balanceNotify.removeSuccess'))
    const updated = await userAPI.getProfile()
    authStore.user = updated
    emailEntries.value = [...updated.balance_notify_extra_emails]
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  }
}

// Verify saved unverified emails
async function sendCodeForSaved(email: string) {
  sendingSavedCode.value = true
  try {
    await userAPI.sendNotifyEmailCode(email)
    verifyingEmail.value = email
    verifyCode.value = ''
    verifyCountdown.value = 60
    if (verifyTimer) clearInterval(verifyTimer)
    verifyTimer = setInterval(() => {
      verifyCountdown.value--
      if (verifyCountdown.value <= 0 && verifyTimer) {
        clearInterval(verifyTimer)
        verifyTimer = null
      }
    }, 1000)
    appStore.showSuccess(t('profile.balanceNotify.codeSent'))
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    sendingSavedCode.value = false
  }
}

async function verifySavedEmail(email: string) {
  if (!verifyCode.value || verifyCode.value.length !== 6) return
  verifyingSaved.value = true
  try {
    await userAPI.verifyNotifyEmail(email, verifyCode.value)
    verifyingEmail.value = ''
    verifyCode.value = ''
    if (verifyTimer) { clearInterval(verifyTimer); verifyTimer = null }
    appStore.showSuccess(t('profile.balanceNotify.verifySuccess'))
    const updated = await userAPI.getProfile()
    authStore.user = updated
    emailEntries.value = [...updated.balance_notify_extra_emails]
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('common.error')))
  } finally {
    verifyingSaved.value = false
  }
}
</script>
