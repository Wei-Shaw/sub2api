<template>
  <AuthLayout>
    <div class="space-y-6">
      <!-- 加载中 -->
      <div v-if="loading" class="py-8 text-center">
        <p class="text-sm text-gray-500 dark:text-dark-400">
          {{ t('oidc.consent.loading') }}
        </p>
      </div>

      <!-- 错误 -->
      <div v-else-if="errorMessage" class="space-y-4">
        <div class="text-center">
          <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('oidc.consent.errorTitle') }}
          </h2>
          <p class="mt-2 text-sm text-red-600 dark:text-red-400">
            {{ errorMessage }}
          </p>
        </div>
        <button class="btn btn-secondary w-full" @click="goDashboard">
          {{ t('oidc.consent.backToDashboard') }}
        </button>
      </div>

      <!-- 同意表单 -->
      <div v-else class="space-y-6">
        <div class="text-center">
          <h2 class="text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('oidc.consent.title') }}
          </h2>
          <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">
            {{ t('oidc.consent.subtitle', { client: clientName }) }}
          </p>
        </div>

        <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800/60">
          <p class="mb-3 text-sm font-medium text-gray-900 dark:text-white">
            {{ t('oidc.consent.scopesTitle') }}
          </p>
          <ul class="space-y-3">
            <li
              v-for="item in scopes"
              :key="item.scope"
              class="flex items-start gap-3"
              :data-testid="`consent-scope-${item.scope}`"
            >
              <span
                class="mt-1 inline-block h-2 w-2 flex-shrink-0 rounded-full"
                :class="item.sensitive ? 'bg-red-500' : 'bg-primary-500'"
              />
              <span class="space-y-0.5">
                <span
                  class="block text-sm font-medium"
                  :class="item.sensitive ? 'text-red-600 dark:text-red-400' : 'text-gray-900 dark:text-white'"
                >
                  {{ scopeTitle(item.scope) }}
                </span>
                <span
                  class="block text-xs"
                  :class="item.sensitive ? 'text-red-500 dark:text-red-400' : 'text-gray-500 dark:text-dark-400'"
                >
                  {{ scopeDescription(item.scope) }}
                </span>
              </span>
            </li>
          </ul>

          <p
            v-if="hasSensitiveScope"
            data-testid="consent-sensitive-warning"
            class="mt-4 rounded-lg border border-red-300 bg-red-50 p-3 text-xs font-medium text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300"
          >
            {{ t('oidc.consent.sensitiveWarning') }}
          </p>
        </div>

        <div class="grid gap-3 sm:grid-cols-2">
          <button
            data-testid="consent-deny"
            class="btn btn-secondary w-full"
            :disabled="submitting"
            @click="decide('deny')"
          >
            {{ t('oidc.consent.deny') }}
          </button>
          <button
            data-testid="consent-allow"
            class="btn btn-primary w-full"
            :disabled="submitting"
            @click="decide('allow')"
          >
            {{ submitting ? t('common.processing') : t('oidc.consent.allow') }}
          </button>
        </div>
      </div>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import {
  getConsentInfo,
  submitConsentDecision,
  type OidcConsentScope,
  type OidcConsentError
} from '@/api/oidcConsent'

const route = useRoute()
const router = useRouter()
const { t, te } = useI18n()

const loading = ref(true)
const submitting = ref(false)
const errorMessage = ref('')
const clientName = ref('')
const scopes = ref<OidcConsentScope[]>([])

const consentToken = computed(() => String(route.query.consent || ''))
const hasSensitiveScope = computed(() => scopes.value.some((s) => s.sensitive))

/** scope -> i18n key 片段（把 `sub2api:balance` 这类带冒号的 scope 归一化）。 */
function scopeKey(scope: string): string {
  switch (scope) {
    case 'openid':
      return 'openid'
    case 'profile':
      return 'profile'
    case 'email':
      return 'email'
    case 'offline_access':
      return 'offlineAccess'
    case 'sub2api:balance':
      return 'balance'
    case 'sub2api:apikey':
      return 'apikey'
    default:
      return ''
  }
}

function scopeTitle(scope: string): string {
  const key = scopeKey(scope)
  const i18nKey = `oidc.consent.scopes.${key}.title`
  return key && te(i18nKey) ? t(i18nKey) : scope
}

function scopeDescription(scope: string): string {
  const key = scopeKey(scope)
  const i18nKey = `oidc.consent.scopes.${key}.description`
  return key && te(i18nKey) ? t(i18nKey) : ''
}

function goDashboard() {
  void router.replace('/dashboard')
}

async function load() {
  loading.value = true
  errorMessage.value = ''
  const token = consentToken.value
  if (!token) {
    errorMessage.value = t('oidc.consent.missingToken')
    loading.value = false
    return
  }
  try {
    const info = await getConsentInfo(token)
    clientName.value = info.client_name || info.client_id
    scopes.value = info.scopes || []
  } catch (e) {
    const err = e as OidcConsentError
    // 未登录/会话错配 → 引导登录并保留回跳。
    if (err.status === 401 || err.error === 'login_required') {
      void router.replace({ path: '/login', query: { redirect: route.fullPath } })
      return
    }
    errorMessage.value = err.error_description || err.error || t('oidc.consent.loadFailed')
  } finally {
    loading.value = false
  }
}

async function decide(action: 'allow' | 'deny') {
  if (submitting.value) return
  submitting.value = true
  try {
    const { redirect } = await submitConsentDecision(consentToken.value, action)
    // 后端返回的是 RP 的回跳地址（携带 code/state 或 error），需整页跳转，
    // 避免 XHR 上的跨域 302 问题。
    window.location.assign(redirect)
  } catch (e) {
    const err = e as OidcConsentError
    if (err.status === 401 || err.error === 'login_required') {
      void router.replace({ path: '/login', query: { redirect: route.fullPath } })
      return
    }
    errorMessage.value = err.error_description || err.error || t('oidc.consent.submitFailed')
    submitting.value = false
  }
}

onMounted(load)
</script>
