<template>
  <form class="space-y-4" @submit.prevent="handleSubmit">
    <!--
      Every field here used to be a bare `<input>` carrying nothing but a
      placeholder: no label, no `for`/`id` pair, no described-by. A placeholder
      is not a label — it disappears the moment the user types, and a screen
      reader announces an unlabelled edit box. `FormField` owns all three, plus
      the reserved message row.
    -->
    <FormField :id="fieldId('email')" :label="t('auth.emailLabel')">
      <template #default="{ describedBy }">
        <input
          :id="fieldId('email')"
          v-model="email"
          :data-testid="`${testIdPrefix}-create-account-email`"
          type="email"
          autocomplete="email"
          :aria-describedby="describedBy"
          class="input"
          :placeholder="t('auth.emailPlaceholder')"
          :disabled="isSubmitting || isSendingCode"
        />
      </template>
    </FormField>

    <!--
      The 6-character minimum was enforced only by `disabled` and an early
      return in `handleSubmit`. A user who typed four characters got a dead
      button and no explanation, and pressing Enter did nothing at all. The rule
      is now stated up front in the row `FormField` reserves anyway.
    -->
    <FormField :id="fieldId('password')" :label="t('auth.passwordLabel')" :hint="t('auth.passwordHint')">
      <template #default="{ describedBy }">
        <input
          :id="fieldId('password')"
          v-model="password"
          :data-testid="`${testIdPrefix}-create-account-password`"
          type="password"
          autocomplete="new-password"
          :aria-describedby="describedBy"
          class="input"
          :placeholder="t('auth.passwordPlaceholder')"
          :disabled="isSubmitting"
        />
      </template>
    </FormField>

    <div v-if="captchaEnabled">
      <TurnstileWidget
        ref="turnstileRef"
        :turnstile-enabled="turnstileEnabled"
        :turnstile-site-key="turnstileSiteKey"
        :tencent-enabled="tencentCaptchaEnabled"
        :tencent-app-id="tencentCaptchaAppId"
        :tencent-region="tencentCaptchaRegion"
        :aliyun-enabled="aliyunCaptchaEnabled"
        :aliyun-scene-id="aliyunCaptchaSceneId"
        :aliyun-prefix="aliyunCaptchaPrefix"
        :aliyun-region="aliyunCaptchaRegion"
        @verify="onTurnstileVerify"
        @expire="onTurnstileExpire"
        @error="onTurnstileError"
      />
    </div>

    <!--
      Code + send-code on one row. The "sent" confirmation used to be a separate
      `<p>` that appeared under the row and pushed the submit button down by a
      line; it now replaces the hint inside the reserved message row, so nothing
      moves. The send button is `md` (32px) against a 36px field — the two
      scales are deliberate, so the row is centred rather than stretched.
    -->
    <!--
      `hint` is passed even though `#message` overrides what is rendered:
      `FormField` only advertises `aria-describedby` when it has hint or error
      text, so without it the message row would be invisible to a screen reader.
    -->
    <FormField
      v-if="emailVerifyEnabled"
      :id="fieldId('verify-code')"
      :label="t('auth.verificationCode')"
      :hint="t('auth.verificationCodeHint')"
    >
      <template #default="{ describedBy }">
        <div class="flex items-center gap-2">
          <input
            :id="fieldId('verify-code')"
            v-model="verifyCode"
            :data-testid="`${testIdPrefix}-create-account-verify-code`"
            type="text"
            inputmode="numeric"
            autocomplete="one-time-code"
            maxlength="6"
            :aria-describedby="describedBy"
            class="input min-w-0 flex-1 font-mono tabular-nums tracking-[0.18em]"
            placeholder="123456"
            :disabled="isSubmitting"
          />
          <!--
            The label no longer swaps to "Sending…" mid-press: `Button` keeps
            the label's box, overlays the spinner and sets `aria-busy`, so the
            control does not change width under the cursor. The countdown is a
            state change rather than a press, and it is tabular so the digits
            do not jitter as it ticks down.
          -->
          <Button
            :data-testid="`${testIdPrefix}-create-account-send-code`"
            type="button"
            variant="outline"
            size="md"
            class="shrink-0 tabular-nums"
            :loading="isSendingCode"
            :disabled="sendCodeDisabled"
            @click="handleSendCode"
          >
            {{ sendCodeLabel }}
          </Button>
        </div>
      </template>
      <template #message>
        <span v-if="sendCodeSuccess" class="text-success">{{ t('auth.codeSentSuccess') }}</span>
        <span v-else>{{ t('auth.verificationCodeHint') }}</span>
      </template>
    </FormField>

    <FormField
      v-if="invitationCodeEnabled"
      :id="fieldId('invitation-code')"
      :label="t('auth.invitationCodeLabel')"
    >
      <template #default="{ describedBy }">
        <input
          :id="fieldId('invitation-code')"
          v-model="invitationCode"
          :data-testid="`${testIdPrefix}-create-account-invitation-code`"
          type="text"
          :aria-describedby="describedBy"
          class="input"
          :placeholder="t('auth.invitationCodePlaceholder')"
          :disabled="isSubmitting"
        />
      </template>
    </FormField>

    <div class="space-y-2">
      <Button
        :data-testid="`${testIdPrefix}-create-account-submit`"
        type="button"
        tone="accent"
        variant="solid"
        size="md"
        block
        :loading="isSubmitting"
        :disabled="submitDisabled"
        @click="handleSubmit"
      >
        {{ t('auth.createAccount') }}
      </Button>
      <Button
        :data-testid="`${testIdPrefix}-create-account-switch-to-bind`"
        type="button"
        variant="outline"
        size="md"
        block
        :disabled="isSubmitting"
        @click="emitSwitchToBind"
      >
        {{ t('auth.alreadyHaveAccount') }}
      </Button>
    </div>
  </form>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
// By path, not through `components/common/index.ts`: the barrel re-exports
// LocaleSwitcher, which pulls `createI18n` into the graph and breaks the specs
// that mock `vue-i18n` with a partial factory.
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
import { getPublicSettings, sendPendingOAuthVerifyCode } from '@/api/auth'
import { useAppStore } from '@/stores'

export type PendingOAuthCreateAccountPayload = {
  email: string
  password: string
  verifyCode: string
  turnstileToken?: string
  tencentCaptchaTicket?: string
  tencentCaptchaRandstr?: string
  invitationCode?: string
}

const props = defineProps<{
  initialEmail: string
  testIdPrefix: string
  isSubmitting: boolean
  errorMessage?: string
}>()

const emit = defineEmits<{
  submit: [payload: PendingOAuthCreateAccountPayload]
  switchToBind: [email: string]
}>()

const { t } = useI18n()
const appStore = useAppStore()

const email = ref('')
const password = ref('')
const verifyCode = ref('')
const invitationCode = ref('')
const isSendingCode = ref(false)
const sendCodeError = ref('')
const sendCodeSuccess = ref(false)
const countdown = ref(0)
const invitationCodeEnabled = ref(false)
const emailVerifyEnabled = ref(true)
const turnstileEnabled = ref(false)
const turnstileSiteKey = ref('')
const tencentCaptchaEnabled = ref(false)
const tencentCaptchaAppId = ref('')
const tencentCaptchaRegion = ref('cn')
const aliyunCaptchaEnabled = ref(false)
const aliyunCaptchaSceneId = ref('')
const aliyunCaptchaPrefix = ref('')
const aliyunCaptchaRegion = ref('cn')
const turnstileToken = ref('')
const tencentCaptchaRandstr = ref('')
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const aliyunCaptchaReady = computed(
  () =>
    aliyunCaptchaEnabled.value &&
    Boolean(aliyunCaptchaSceneId.value) &&
    Boolean(aliyunCaptchaPrefix.value)
)
// 动作触发式验证码（腾讯/阿里云）：发送验证码、提交时弹窗验证
const actionCaptchaEnabled = computed(
  () =>
    (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value)) ||
    aliyunCaptchaReady.value
)
const captchaEnabled = computed(
  () =>
    (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) || actionCaptchaEnabled.value
)

/** `FormField` owns `for`/`id`; the prefix keeps the ids unique per provider. */
function fieldId(name: string): string {
  return `${props.testIdPrefix}-create-account-${name}`
}

const sendCodeLabel = computed(() =>
  countdown.value > 0 ? t('auth.resendCountdown', { countdown: countdown.value }) : t('auth.sendCode')
)

const sendCodeDisabled = computed(
  () =>
    props.isSubmitting ||
    isSendingCode.value ||
    countdown.value > 0 ||
    !email.value.trim() ||
    (turnstileEnabled.value && !turnstileToken.value)
)

const submitDisabled = computed(
  () =>
    props.isSubmitting ||
    !email.value.trim() ||
    password.value.length < 6 ||
    (invitationCodeEnabled.value && !invitationCode.value.trim()) ||
    (turnstileEnabled.value && !turnstileToken.value)
)

let countdownTimer: ReturnType<typeof setInterval> | null = null

watch(
  () => props.initialEmail,
  value => {
    email.value = value || ''
  },
  { immediate: true }
)

watch(sendCodeError, value => {
  if (value) {
    appStore.showError(value)
  }
})

watch(
  () => props.errorMessage,
  value => {
    if (value) {
      appStore.showError(value)
      if (captchaEnabled.value) {
        resetTurnstile()
      }
    }
  }
)

function clearCountdown() {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
}

function startCountdown(seconds: number) {
  clearCountdown()
  countdown.value = Math.max(0, seconds)

  if (countdown.value <= 0) {
    return
  }

  countdownTimer = setInterval(() => {
    if (countdown.value <= 1) {
      countdown.value = 0
      clearCountdown()
      return
    }

    countdown.value -= 1
  }, 1000)
}

function getRequestErrorMessage(error: unknown, fallback: string): string {
  const err = error as { message?: string; response?: { data?: { detail?: string; message?: string } } }
  return err.response?.data?.detail || err.response?.data?.message || err.message || fallback
}

function resetTurnstile() {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  turnstileRef.value?.reset()
}

function onTurnstileVerify(token: string, randstr = '') {
  turnstileToken.value = token
  tencentCaptchaRandstr.value = randstr
  sendCodeError.value = ''
}

function onTurnstileExpire() {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  sendCodeError.value = t('auth.turnstileExpired')
}

function onTurnstileError() {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  sendCodeError.value = t('auth.turnstileFailed')
}

async function acquireActionProof(): Promise<boolean> {
  if (!actionCaptchaEnabled.value) return true

  const proof = await turnstileRef.value?.verifyAction()
  if (!proof) return false

  turnstileToken.value = proof.token
  tencentCaptchaRandstr.value = proof.randstr
  return true
}

async function handleSendCode() {
  // Cleared BEFORE the guards below, not after. The toast is driven by a watcher
  // on this ref, so re-raising an identical message (two clicks with no proof)
  // was a no-op and the second click failed in silence.
  sendCodeError.value = ''

  const trimmedEmail = email.value.trim()
  if (!trimmedEmail) {
    return
  }

  if (turnstileEnabled.value && !turnstileToken.value) {
    sendCodeError.value = t('auth.completeVerification')
    return
  }

  if (!(await acquireActionProof())) {
    return
  }

  isSendingCode.value = true
  sendCodeSuccess.value = false

  try {
    const response = await sendPendingOAuthVerifyCode({
      email: trimmedEmail,
      turnstile_token:
        turnstileEnabled.value || aliyunCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value ? tencentCaptchaRandstr.value : undefined
    })
    sendCodeSuccess.value = true
    startCountdown(response.countdown)
  } catch (error: unknown) {
    sendCodeError.value = getRequestErrorMessage(error, t('auth.sendCodeFailed'))
  } finally {
    if (captchaEnabled.value) {
      resetTurnstile()
    }
    isSendingCode.value = false
  }
}

async function handleSubmit() {
  sendCodeError.value = ''

  const trimmedEmail = email.value.trim()
  if (!trimmedEmail || password.value.length < 6) {
    return
  }

  // Turnstile 票据一次性：发送验证码已消耗上一枚，reset 后要等新票据回调。
  // 缺票时不能提交——create-account 端点会校验验证码，空 token 直接被判失败。
  // 表单的隐式提交（输入框回车）绕得过按钮的 disabled，所以这里必须再挡一次。
  if (turnstileEnabled.value && !turnstileToken.value) {
    sendCodeError.value = t('auth.completeVerification')
    return
  }

  if (!(await acquireActionProof())) {
    return
  }

  emit('submit', {
    email: trimmedEmail,
    password: password.value,
    verifyCode: emailVerifyEnabled.value ? verifyCode.value.trim() : '',
    ...((turnstileEnabled.value || aliyunCaptchaEnabled.value) && turnstileToken.value
      ? { turnstileToken: turnstileToken.value }
      : {}),
    ...(tencentCaptchaEnabled.value && turnstileToken.value
      ? {
          tencentCaptchaTicket: turnstileToken.value,
          tencentCaptchaRandstr: tencentCaptchaRandstr.value
        }
      : {}),
    invitationCode: invitationCode.value.trim() || undefined
  })

  if (actionCaptchaEnabled.value) {
    resetTurnstile()
  }
}

function emitSwitchToBind() {
  emit('switchToBind', email.value.trim())
}

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    invitationCodeEnabled.value = settings.invitation_code_enabled === true
    emailVerifyEnabled.value = settings.email_verify_enabled !== false
    turnstileEnabled.value = settings.turnstile_enabled === true
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    tencentCaptchaEnabled.value = settings.tencent_captcha_enabled === true
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
    tencentCaptchaRegion.value = settings.tencent_captcha_region || 'cn'
    aliyunCaptchaEnabled.value = settings.aliyun_captcha_enabled === true
    aliyunCaptchaSceneId.value = settings.aliyun_captcha_scene_id || ''
    aliyunCaptchaPrefix.value = settings.aliyun_captcha_prefix || ''
    aliyunCaptchaRegion.value = settings.aliyun_captcha_region || 'cn'
  } catch {
    invitationCodeEnabled.value = false
    emailVerifyEnabled.value = true
    turnstileEnabled.value = false
    turnstileSiteKey.value = ''
    tencentCaptchaEnabled.value = false
    tencentCaptchaAppId.value = ''
    tencentCaptchaRegion.value = 'cn'
    aliyunCaptchaEnabled.value = false
    aliyunCaptchaSceneId.value = ''
    aliyunCaptchaPrefix.value = ''
    aliyunCaptchaRegion.value = 'cn'
  }
})

onUnmounted(() => {
  clearCountdown()
})
</script>

<!--
  The `<style scoped>` block held `.fade-*` classes for a `<transition
  name="fade">` this template never had — dead CSS carrying a banned
  `transition: all`.
-->
