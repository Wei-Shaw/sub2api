<template>
  <AuthLayout wide>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ isBinding ? '绑定企业微信' : '企业微信登录' }}
        </h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
          {{ statusText }}
        </p>
      </div>

      <div class="space-y-4">
        <div class="mx-auto flex min-h-[416px] w-full justify-center overflow-hidden rounded-xl bg-white dark:bg-white">
          <canvas
            v-show="requiresPrivateInfo && !isStarting"
            ref="mobileQrCanvas"
            class="m-auto"
          ></canvas>
          <div
            v-show="authorizeUrl && !isStarting && !requiresPrivateInfo"
            ref="loginPanelEl"
            class="wecom-login-panel"
          ></div>
          <div
            v-if="isStarting"
            class="flex h-[416px] w-full items-center justify-center"
          >
            <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-500 border-t-transparent"></div>
          </div>
        </div>

        <button
          type="button"
          class="btn btn-primary w-full"
          :disabled="isStarting"
          @click="restart"
        >
          {{ isStarting ? '正在刷新...' : '刷新登录面板' }}
        </button>

        <p v-if="errorMessage" class="text-center text-sm text-red-600 dark:text-red-400">
          {{ errorMessage }}
        </p>
      </div>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import QRCode from 'qrcode'
import { AuthLayout } from '@/components/layout'
import {
  getWeComMobileOAuthStatus,
  prepareOAuthBindAccessTokenCookie,
  startWeComMobileOAuth
} from '@/api/auth'
import { useAppStore } from '@/stores'
import {
  buildWeComLoginPanelParams,
  loadWeComSDK,
  type WeComLoginInstance
} from './wecomLoginPanel'
import { resolveOAuthPromoCode } from '@/utils/oauthPromoCode'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

const authorizeUrl = ref('')
const sessionId = ref('')
const errorMessage = ref('')
const isStarting = ref(false)
const waiting = ref(false)
const isWeComClientLoggedIn = ref<boolean | null>(null)
const requiresPrivateInfo = ref(false)
const loginPanelEl = ref<HTMLElement | null>(null)
const mobileQrCanvas = ref<HTMLCanvasElement | null>(null)
let pollTimer: ReturnType<typeof setTimeout> | null = null
let loginPanel: WeComLoginInstance | null = null

const isBinding = computed(() => route.query.intent === 'bind_current_user')
const statusText = computed(() => {
  if (errorMessage.value) return '授权没有完成，请刷新二维码后重试。'
  if (requiresPrivateInfo.value) return '请使用手机企业微信扫码授权，用于获取邮箱、姓名和头像。'
  if (isWeComClientLoggedIn.value === true) return '已检测到企业微信客户端，请在官方登录组件中确认。'
  if (isWeComClientLoggedIn.value === false) return '未检测到企业微信客户端，请在官方登录组件中扫码登录。'
  if (waiting.value) return '请在企业微信登录组件中完成授权。'
  return '正在创建企业微信登录组件。'
})

function sanitizeRedirectPath(path: unknown): string {
  const value = typeof path === 'string' ? path.trim() : ''
  if (!value || !value.startsWith('/') || value.startsWith('//') || value.includes('://')) {
    return isBinding.value ? '/profile' : '/dashboard'
  }
  if (value.includes('\n') || value.includes('\r')) {
    return isBinding.value ? '/profile' : '/dashboard'
  }
  return value
}

function clearPollTimer(): void {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

function unmountLoginPanel(): void {
  if (!loginPanel) return
  loginPanel.unmount()
  loginPanel = null
}

async function completeLoginPanelAuth(code: string, state: string): Promise<void> {
  const params = new URLSearchParams({ code, state })
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const resp = await fetch(`${normalized}/auth/oauth/wecom/mobile/callback?${params.toString()}`, {
    credentials: 'include'
  })
  if (!resp.ok) {
    throw new Error('企业微信授权回调失败')
  }
}

async function mountLoginPanel(rawURL: string): Promise<void> {
  await nextTick()
  if (!loginPanelEl.value) {
    throw new Error('企业微信登录组件挂载失败')
  }
  unmountLoginPanel()
  const ww = await loadWeComSDK()
  const params = buildWeComLoginPanelParams(rawURL, 'callback', 'middle')
  loginPanel = ww.createWWLoginPanel({
    el: loginPanelEl.value,
    params,
    onCheckWeComLogin: ({ isWeComLogin }) => {
      isWeComClientLoggedIn.value = isWeComLogin
    },
    onOpenInWecom: () => {
      isWeComClientLoggedIn.value = true
    },
    onLoginSuccess: ({ code }) => {
      void completeLoginPanelAuth(code, params.state).catch((error: unknown) => {
        const err = error as { message?: string }
        errorMessage.value = err.message || '企业微信授权回调失败'
        waiting.value = false
      })
    },
    onLoginFail: (error) => {
      errorMessage.value = error.errMsg || '企业微信登录组件授权失败'
      waiting.value = false
    }
  })
}

async function showMobileOAuthQRCode(rawURL: string): Promise<void> {
  await nextTick()
  if (!mobileQrCanvas.value) {
    throw new Error('企业微信手机授权二维码创建失败')
  }
  unmountLoginPanel()
  requiresPrivateInfo.value = true
  await QRCode.toCanvas(mobileQrCanvas.value, rawURL, {
    width: 260,
    margin: 2,
    color: {
      dark: '#111827',
      light: '#ffffff'
    }
  })
}

async function pollStatus(delayMs: number): Promise<void> {
  clearPollTimer()
  pollTimer = setTimeout(async () => {
    try {
      const status = await getWeComMobileOAuthStatus(sessionId.value)
      if (status.status === 'completed') {
        await router.replace({
          path: '/auth/wecom/callback',
          query: { redirect: sanitizeRedirectPath(status.redirect || route.query.redirect) }
        })
        return
      }
      if (status.privateinfo_required && !status.authorize_url) {
        throw new Error('企业微信手机授权链接缺失')
      }
      if (status.privateinfo_required && status.authorize_url && !requiresPrivateInfo.value) {
        await showMobileOAuthQRCode(status.authorize_url)
      }
      if (status.status === 'failed') {
        errorMessage.value = status.message || status.error || '企业微信授权失败'
        waiting.value = false
        return
      }
      await pollStatus(status.poll_interval_ms || delayMs)
    } catch (error: unknown) {
      const err = error as { message?: string }
      errorMessage.value = err.message || '企业微信授权状态查询失败'
      waiting.value = false
    }
  }, delayMs)
}

async function start(): Promise<void> {
  clearPollTimer()
  unmountLoginPanel()
  errorMessage.value = ''
  authorizeUrl.value = ''
  sessionId.value = ''
  isWeComClientLoggedIn.value = null
  requiresPrivateInfo.value = false
  isStarting.value = true
  waiting.value = false
  try {
    if (isBinding.value) {
      await prepareOAuthBindAccessTokenCookie()
    }
    const result = await startWeComMobileOAuth({
      redirect: sanitizeRedirectPath(route.query.redirect),
      intent: isBinding.value ? 'bind_current_user' : 'login',
      promo_code: isBinding.value ? undefined : resolveOAuthPromoCode(route.query.promo, route.query.promo_code)
    })
    sessionId.value = result.session_id
    authorizeUrl.value = result.authorize_url
    await mountLoginPanel(result.authorize_url)
    waiting.value = true
    await pollStatus(result.poll_interval_ms || 2000)
  } catch (error: unknown) {
    const err = error as { message?: string }
    errorMessage.value = err.message || '企业微信登录组件创建失败'
  } finally {
    isStarting.value = false
  }
}

function restart(): void {
  void start()
}

onMounted(() => {
  appStore.fetchPublicSettings()
  void start()
})

onBeforeUnmount(() => {
  clearPollTimer()
  unmountLoginPanel()
})
</script>

<style scoped>
.wecom-login-panel :deep(iframe) {
  display: block;
  border: 0;
}
</style>
