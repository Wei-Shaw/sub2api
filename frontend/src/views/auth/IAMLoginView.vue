<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="grid grid-cols-2 rounded-md bg-gray-100 p-1 dark:bg-dark-800">
        <router-link to="/login" class="rounded px-3 py-2 text-center text-sm text-gray-600 dark:text-dark-300">
          {{ t('organization.login.personal') }}
        </router-link>
        <span class="rounded bg-white px-3 py-2 text-center text-sm font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white">
          {{ t('organization.login.iam') }}
        </span>
      </div>

      <div>
        <h2 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('organization.login.title') }}</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('organization.login.subtitle') }}</p>
      </div>

      <form class="space-y-4" @submit.prevent="submit">
        <div>
          <label class="input-label" for="iam-principal">{{ t('organization.login.principal') }}</label>
          <input
            id="iam-principal"
            v-model.trim="form.principal"
            class="input font-mono"
            required
            autocomplete="username"
            maxlength="91"
            pattern="[A-Za-z0-9._-]{1,64}@c[1-9][0-9]{14}\.opentk\.ai"
            :disabled="authActionDisabled"
          />
        </div>
        <div>
          <label class="input-label" for="iam-password">{{ t('auth.passwordLabel') }}</label>
          <input id="iam-password" v-model="form.password" class="input" required type="password" autocomplete="current-password" :disabled="authActionDisabled" />
        </div>
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
        <button
          class="btn btn-primary w-full"
          type="submit"
          :disabled="authActionDisabled || (turnstileEnabled && !turnstileToken)"
        >
          {{ loading ? t('auth.signingIn') : t('auth.signIn') }}
        </button>
      </form>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import AuthLayout from '@/components/layout/AuthLayout.vue'
import TurnstileWidget from '@/components/CaptchaChallenge.vue'
import { getPublicSettings } from '@/api/auth'
import { useAppStore, useAuthStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const appStore = useAppStore()
const loading = ref(false)
const publicSettingsLoaded = ref(false)

// Public settings
const turnstileEnabled = ref<boolean>(false)
const turnstileSiteKey = ref<string>('')
const tencentCaptchaEnabled = ref<boolean>(false)
const tencentCaptchaAppId = ref<string>('')
const tencentCaptchaRegion = ref<string>('cn')
const aliyunCaptchaEnabled = ref<boolean>(false)
const aliyunCaptchaSceneId = ref<string>('')
const aliyunCaptchaPrefix = ref<string>('')
const aliyunCaptchaRegion = ref<string>('cn')

// Captcha proof state
const turnstileRef = ref<InstanceType<typeof TurnstileWidget> | null>(null)
const turnstileToken = ref<string>('')
const tencentCaptchaRandstr = ref<string>('')

const aliyunCaptchaReady = computed(
  () =>
    aliyunCaptchaEnabled.value &&
    Boolean(aliyunCaptchaSceneId.value) &&
    Boolean(aliyunCaptchaPrefix.value)
)
// 动作触发式验证码（腾讯/阿里云）：提交时弹窗验证
const actionCaptchaEnabled = computed(
  () => (tencentCaptchaEnabled.value && Boolean(tencentCaptchaAppId.value)) || aliyunCaptchaReady.value
)
const captchaEnabled = computed(
  () => (turnstileEnabled.value && Boolean(turnstileSiteKey.value)) || actionCaptchaEnabled.value
)

const form = reactive({ principal: '', password: '' })
const authActionDisabled = computed(() => loading.value || !publicSettingsLoaded.value)

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    turnstileEnabled.value = settings.turnstile_enabled
    turnstileSiteKey.value = settings.turnstile_site_key || ''
    tencentCaptchaEnabled.value = settings.tencent_captcha_enabled === true
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
    tencentCaptchaRegion.value = settings.tencent_captcha_region || 'cn'
    aliyunCaptchaEnabled.value = settings.aliyun_captcha_enabled === true
    aliyunCaptchaSceneId.value = settings.aliyun_captcha_scene_id || ''
    aliyunCaptchaPrefix.value = settings.aliyun_captcha_prefix || ''
    aliyunCaptchaRegion.value = settings.aliyun_captcha_region || 'cn'
  } catch (error) {
    console.error('Failed to load public settings:', error)
  } finally {
    publicSettingsLoaded.value = true
  }
})

// ==================== Captcha Handlers ====================

function onTurnstileVerify(token: string, randstr = ''): void {
  turnstileToken.value = token
  tencentCaptchaRandstr.value = randstr
}

function onTurnstileExpire(): void {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  appStore.showError(t('auth.turnstileExpired'))
}

function onTurnstileError(): void {
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
  appStore.showError(t('auth.turnstileFailed'))
}

function resetCaptchaProof(): void {
  turnstileRef.value?.reset()
  turnstileToken.value = ''
  tencentCaptchaRandstr.value = ''
}

async function acquireActionProof(): Promise<boolean> {
  if (!actionCaptchaEnabled.value) return true

  const proof = await turnstileRef.value?.verifyAction()
  if (!proof) return false

  turnstileToken.value = proof.token
  tencentCaptchaRandstr.value = proof.randstr
  return true
}

async function submit(): Promise<void> {
  if (authActionDisabled.value) return

  if (turnstileEnabled.value && !turnstileToken.value) {
    appStore.showError(t('auth.completeVerification'))
    return
  }

  if (!(await acquireActionProof())) {
    return
  }

  loading.value = true
  try {
    const response = await auth.loginIAM({
      ...form,
      turnstile_token:
        turnstileEnabled.value || aliyunCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_ticket: tencentCaptchaEnabled.value ? turnstileToken.value : undefined,
      tencent_captcha_randstr: tencentCaptchaEnabled.value ? tencentCaptchaRandstr.value : undefined
    })
    await router.replace(
      response.user.must_change_password ? '/organization/change-password' : '/dashboard'
    )
  } catch (error: unknown) {
    appStore.showError(extractI18nErrorMessage(error, t, 'auth.errors', t('auth.loginFailed')))
  } finally {
    if (captchaEnabled.value) {
      resetCaptchaProof()
    }
    loading.value = false
  }
}
</script>
