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
          <CaptchaChallenge
            ref="captchaRef"
            :turnstile-enabled="captchaProvider === 'turnstile'"
            :turnstile-site-key="captchaSiteKey"
            :tencent-enabled="captchaProvider === 'tencent_captcha'"
            :tencent-app-id="tencentCaptchaAppId"
            :tencent-region="tencentCaptchaRegion"
            :aliyun-enabled="captchaProvider === 'aliyun_captcha'"
            :aliyun-scene-id="aliyunCaptchaSceneId"
            :aliyun-prefix="aliyunCaptchaPrefix"
            :aliyun-region="aliyunCaptchaRegion"
            @verify="onCaptchaVerify"
            @expire="onCaptchaExpire"
            @error="onCaptchaError"
          />
        </div>
        <button
          class="btn btn-primary w-full"
          type="submit"
          :disabled="authActionDisabled || (captchaEnabled && captchaProvider !== 'tencent_captcha' && !captchaToken)"
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
import CaptchaChallenge from '@/components/CaptchaChallenge.vue'
import { getPublicSettings } from '@/api/auth'
import { useAppStore, useAuthStore } from '@/stores'
import { extractI18nErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const router = useRouter()
const auth = useAuthStore()
const appStore = useAppStore()
const loading = ref(false)
const publicSettingsLoaded = ref(false)
const captchaEnabled = ref(false)
const captchaProvider = ref<'turnstile' | 'tencent_captcha' | 'aliyun_captcha'>('turnstile')
const captchaSiteKey = ref('')
const captchaToken = ref('')
const tencentCaptchaAppId = ref('')
const tencentCaptchaRegion = ref('cn')
const aliyunCaptchaSceneId = ref('')
const aliyunCaptchaPrefix = ref('')
const aliyunCaptchaRegion = ref('cn')
const captchaRef = ref<InstanceType<typeof CaptchaChallenge> | null>(null)
const form = reactive({ principal: '', password: '' })
const authActionDisabled = computed(() => loading.value || !publicSettingsLoaded.value)

onMounted(async () => {
  try {
    const settings = await getPublicSettings()
    captchaEnabled.value = settings.captcha_enabled ?? settings.turnstile_enabled
    captchaProvider.value = settings.captcha_provider === 'tencent_captcha'
        ? 'tencent_captcha'
        : settings.captcha_provider === 'aliyun_captcha'
          ? 'aliyun_captcha'
        : 'turnstile'
    captchaSiteKey.value = settings.captcha_site_key || settings.turnstile_site_key || ''
    tencentCaptchaAppId.value = settings.tencent_captcha_app_id || ''
    tencentCaptchaRegion.value = settings.tencent_captcha_region || 'cn'
    aliyunCaptchaSceneId.value = settings.aliyun_captcha_scene_id || ''
    aliyunCaptchaPrefix.value = settings.aliyun_captcha_prefix || ''
    aliyunCaptchaRegion.value = settings.aliyun_captcha_region || 'cn'
  } catch (error) {
    console.error('Failed to load public settings:', error)
  } finally {
    publicSettingsLoaded.value = true
  }
})

function onCaptchaVerify(token: string, randstr = ''): void {
  captchaToken.value = token
  captchaRandstr.value = randstr
}

const captchaRandstr = ref('')

function onCaptchaExpire(): void {
  captchaToken.value = ''
  appStore.showError(t('auth.captchaExpired'))
}

function onCaptchaError(): void {
  captchaToken.value = ''
  appStore.showError(t('auth.captchaFailed'))
}

async function submit() {
  if (authActionDisabled.value) return
  if (captchaEnabled.value && captchaProvider.value === 'turnstile' && !captchaToken.value) {
    appStore.showError(t('auth.completeCaptchaVerification'))
    return
  }
  loading.value = true
  try {
    if (captchaEnabled.value && captchaProvider.value !== 'turnstile') {
      const proof = await captchaRef.value?.verifyAction()
      if (!proof) return
      captchaToken.value = proof.token
      captchaRandstr.value = proof.randstr
    }
    const captchaPayload: Record<string, string> | undefined = captchaEnabled.value
      ? captchaProvider.value === 'tencent_captcha'
        ? { token: captchaToken.value, randstr: captchaRandstr.value }
        : { token: captchaToken.value }
    : undefined
    const response = await auth.loginIAM({ ...form, captcha_payload: captchaPayload })
    await router.replace((response.user as typeof response.user & { must_change_password?: boolean }).must_change_password ? '/organization/change-password' : '/dashboard')
  } catch (error: unknown) {
    captchaRef.value?.reset()
    captchaToken.value = ''
    const message = extractI18nErrorMessage(error, t, 'auth.errors', t('auth.loginFailed'))
    appStore.showError(message)
  } finally {
    loading.value = false
  }
}
</script>
