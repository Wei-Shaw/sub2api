<template>
  <main class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-950">
    <section class="mx-auto max-w-lg rounded-xl bg-white p-8 shadow-sm dark:bg-dark-900">
      <div v-if="loading" class="py-12 text-center text-sm text-gray-500 dark:text-dark-400">{{ t('oauth.authorize.loading') }}</div>
      <div v-else-if="error" class="space-y-4">
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('oauth.authorize.unavailable') }}</h1>
        <p class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
      </div>
      <div v-else-if="transaction" class="space-y-6">
        <div class="flex items-center gap-4">
          <img
            v-if="transaction.client_logo_url"
            :src="transaction.client_logo_url"
            :alt="transaction.client_name"
            class="h-12 w-12 rounded-lg object-cover"
          />
          <div>
            <p class="text-xs uppercase tracking-wide text-gray-500 dark:text-dark-400">{{ t('oauth.authorize.signInWith') }}</p>
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ transaction.client_name }}</h1>
          </div>
        </div>

        <p class="text-sm text-gray-600 dark:text-dark-300">
          {{ t('oauth.authorize.requestAccess') }}
        </p>

        <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
          <h2 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('oauth.authorize.requestedPermissions') }}</h2>
          <ul class="mt-3 space-y-2 text-sm text-gray-600 dark:text-dark-300">
            <li v-for="scope in transaction.scope" :key="scope" class="flex items-center gap-2">
              <span class="h-1.5 w-1.5 rounded-full bg-primary-500" />
              {{ scopeLabel(scope) }}
            </li>
          </ul>
        </div>

        <p class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('oauth.authorize.revokeHint') }}
        </p>

        <div class="flex gap-3">
          <button type="button" class="btn btn-secondary flex-1" :disabled="submitting" @click="submit('deny')">
            {{ t('common.cancel') }}
          </button>
          <button type="button" class="btn btn-primary flex-1" :disabled="submitting" @click="submit('approve')">
            {{ submitting ? t('oauth.authorize.authorizing') : t('oauth.authorize.authorize') }}
          </button>
        </div>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import {
  approveOAuthAuthorization,
  getOAuthAuthorizationTransaction,
  type OAuthAuthorizationTransaction
} from '@/api/oauthProvider'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { t } = useI18n()
const transaction = ref<OAuthAuthorizationTransaction | null>(null)
const loading = ref(true)
const submitting = ref(false)
const error = ref('')

const scopeLabel = (scope: string): string => {
  const labels: Record<string, string> = {
    openid: 'oauthScopes.openid',
    profile: 'oauthScopes.profile',
    'groups:read': 'oauthScopes.groupsRead',
    'keys:read': 'oauthScopes.keysRead',
    'keys:create': 'oauthScopes.keysCreate',
    'keys:revoke': 'oauthScopes.keysRevoke'
  }
  return labels[scope] ? t(labels[scope]) : scope
}

onMounted(async () => {
  await auth.checkAuth()
  const id = typeof route.query.transaction_id === 'string' ? route.query.transaction_id : ''
  if (!id) {
    error.value = t('oauth.authorize.missingTransaction')
    loading.value = false
    return
  }
  if (!auth.isAuthenticated) {
    await router.replace({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  try {
    transaction.value = await getOAuthAuthorizationTransaction(id)
  } catch (err: any) {
    if (err?.status === 401 || err?.status === 403) {
      await router.replace({ path: '/login', query: { redirect: route.fullPath } })
      return
    }
    error.value = err?.message || t('oauth.authorize.invalidTransaction')
  } finally {
    loading.value = false
  }
})

async function submit(decision: 'approve' | 'deny') {
  if (!transaction.value) return
  submitting.value = true
  try {
    const result = await approveOAuthAuthorization(transaction.value.id, decision)
    window.location.assign(result.redirect_uri)
  } catch (err: any) {
    error.value = err?.message || t('oauth.authorize.failed')
    submitting.value = false
  }
}
</script>
