<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('oauth.connectedApps.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('oauth.connectedApps.description') }}
      </p>
    </div>
    <div class="px-6 py-6">
      <!-- Loading state -->
      <div v-if="loading" class="flex items-center justify-center py-8">
        <div class="animate-spin rounded-full h-8 w-8 border-b-2 border-primary-500"></div>
      </div>

      <!-- Load error -->
      <div v-else-if="error && grants.length === 0" class="text-sm text-red-600 dark:text-red-400">
        {{ error }}
      </div>

      <!-- Empty state -->
      <p v-else-if="grants.length === 0" class="py-2 text-sm text-gray-500 dark:text-gray-400">
        {{ t('oauth.connectedApps.empty') }}
      </p>

      <!-- Grants -->
      <ul v-else class="space-y-4">
        <li
          v-for="grant in grants"
          :key="grant.client_id"
          class="flex items-start justify-between gap-4 rounded-lg border border-gray-100 p-4 dark:border-dark-700"
        >
          <div class="min-w-0">
            <div class="flex items-center gap-3">
              <img
                v-if="grant.client_logo_url"
                :src="grant.client_logo_url"
                :alt="grant.client_name"
                class="h-9 w-9 rounded-lg object-cover"
              />
              <div class="min-w-0">
                <p class="truncate text-sm font-medium text-gray-900 dark:text-white">
                  {{ grant.client_name }}
                </p>
                <p class="truncate text-xs text-gray-400 dark:text-dark-500">
                  {{ grant.client_id }} ·
                  {{ t('oauth.connectedApps.approvedAt', { date: formatDate(grant.approved_at) }) }}
                </p>
              </div>
            </div>
            <div class="mt-2 flex flex-wrap gap-1.5">
              <span
                v-for="scope in grant.scopes"
                :key="scope"
                class="rounded-md bg-gray-100 px-2 py-0.5 font-mono text-xs text-gray-600 dark:bg-dark-800 dark:text-dark-300"
              >{{ scope }}</span>
            </div>
          </div>
          <button
            type="button"
            class="shrink-0 rounded-lg border border-red-200 px-3 py-1.5 text-xs font-medium text-red-600 transition hover:bg-red-50 disabled:cursor-not-allowed disabled:opacity-50 dark:border-red-900/50 dark:text-red-400 dark:hover:bg-red-900/20"
            :disabled="revoking === grant.client_id"
            @click="revoke(grant)"
          >
            {{ revoking === grant.client_id ? t('oauth.connectedApps.revoking') : t('oauth.connectedApps.revoke') }}
          </button>
        </li>
      </ul>

      <!-- Inline revoke error (list stays visible) -->
      <p v-if="error && grants.length > 0" class="mt-3 text-sm text-red-600 dark:text-red-400">
        {{ error }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  listOAuthGrants,
  revokeOAuthGrant,
  type OAuthGrantView
} from '@/api/oauthProvider'

const { t } = useI18n()
const grants = ref<OAuthGrantView[]>([])
const loading = ref(true)
const error = ref('')
const revoking = ref<string | null>(null)

function formatDate(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleDateString()
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    grants.value = await listOAuthGrants()
  } catch (err: any) {
    error.value = err?.message || t('oauth.connectedApps.loadFailed')
  } finally {
    loading.value = false
  }
}

async function revoke(grant: OAuthGrantView): Promise<void> {
  revoking.value = grant.client_id
  error.value = ''
  try {
    await revokeOAuthGrant(grant.client_id)
    grants.value = grants.value.filter((item) => item.client_id !== grant.client_id)
  } catch (err: any) {
    error.value = err?.message || t('oauth.connectedApps.revokeFailed')
  } finally {
    revoking.value = null
  }
}

onMounted(load)
</script>
