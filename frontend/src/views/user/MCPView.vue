<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <section class="card p-5 sm:p-6" aria-labelledby="mcp-connection-title" :aria-busy="connectionLoading">
        <div class="flex items-start gap-3">
          <div class="rounded-xl bg-primary-50 p-3 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400">
            <Icon name="mcp" size="lg" />
          </div>
          <div>
            <h2 id="mcp-connection-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('mcp.connectionTitle') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('mcp.connectionDescription') }}</p>
          </div>
        </div>
        <div class="mt-5">
          <label for="mcp-endpoint" class="input-label">{{ t('mcp.endpoint') }}</label>
          <div class="flex flex-col gap-2 sm:flex-row">
            <input id="mcp-endpoint" :value="endpoint" :placeholder="connectionLoading ? t('mcp.connectionLoading') : ''" class="input min-w-0 flex-1 font-mono text-sm" readonly />
            <button data-testid="copy-endpoint" class="btn btn-secondary shrink-0" type="button" :disabled="!endpoint || connectionLoading" @click="copyToClipboard(endpoint)">{{ t('mcp.copyEndpoint') }}</button>
          </div>
          <div v-if="connectionError" class="mt-3 flex flex-wrap items-center gap-3">
            <p role="alert" class="text-sm text-red-600 dark:text-red-400">{{ connectionError }}</p>
            <button data-testid="retry-connection" class="btn btn-secondary btn-sm" type="button" :disabled="connectionLoading" @click="loadConnection">{{ t('common.refresh') }}</button>
          </div>
          <ol class="mt-5 list-decimal space-y-2 pl-5 text-sm text-gray-600 dark:text-gray-300">
            <li>{{ t('mcp.connectStep1') }}</li>
            <li>{{ t('mcp.connectStep2') }}</li>
            <li>{{ t('mcp.connectStep3') }}</li>
          </ol>
        </div>
        <p class="mt-5 border-t border-gray-100 pt-4 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">{{ t('mcp.capabilities') }}</p>
      </section>

      <section class="card" aria-labelledby="mcp-grants-title">
        <div class="flex flex-wrap items-start justify-between gap-4 border-b border-gray-100 p-5 dark:border-dark-700 sm:p-6">
          <div class="max-w-xl">
            <h2 id="mcp-grants-title" class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('mcp.grantsTitle') }}</h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('mcp.grantsDescription') }}</p>
          </div>
          <button data-testid="refresh-grants" type="button" class="btn btn-secondary" :disabled="loading || revoking" @click="loadGrants">{{ t('common.refresh') }}</button>
        </div>
        <div class="p-5 sm:p-6" :aria-busy="loading">
          <p v-if="loadError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ loadError }}</p>
          <p v-else-if="loading" role="status" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('mcp.loading') }}</p>
          <p v-else-if="grants.length === 0" class="rounded-xl border border-dashed border-gray-200 px-4 py-10 text-center text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">{{ t('mcp.empty') }}</p>
          <ul v-else class="divide-y divide-gray-100 dark:divide-dark-700">
            <li v-for="grant in grants" :key="grant.id" class="flex items-start justify-between gap-4 py-5 first:pt-0 last:pb-0" :data-grant-id="grant.id">
              <div class="min-w-0 space-y-2">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="break-all font-medium text-gray-900 dark:text-white">{{ grant.client_name }}</span>
                  <span class="rounded-md bg-gray-100 px-2 py-0.5 text-xs text-gray-600 dark:bg-dark-700 dark:text-gray-300">{{ grant.scope === 'admin' ? t('mcp.adminScope') : t('mcp.userScope') }}</span>
                  <span class="rounded-md bg-primary-50 px-2 py-0.5 text-xs text-primary-700 dark:bg-primary-900/20 dark:text-primary-300">{{ grant.read_only ? t('mcp.readOnly') : t('mcp.readWrite') }}</span>
                  <span class="text-xs" :class="isActive(grant) ? 'text-emerald-600 dark:text-emerald-400' : 'text-gray-500 dark:text-gray-400'">{{ grantStatus(grant) }}</span>
                </div>
                <p class="truncate font-mono text-xs text-gray-500 dark:text-gray-400" :title="grant.client_id">{{ grant.client_id }}</p>
                <div class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-gray-400">
                  <span>{{ t('mcp.createdAt', { date: formatDate(grant.created_at) }) }}</span>
                  <span>{{ t('mcp.expiresAt', { date: formatDate(grant.expires_at) }) }}</span>
                  <span>{{ grant.last_used_at ? t('mcp.lastUsedAt', { date: formatDate(grant.last_used_at) }) : t('mcp.neverUsed') }}</span>
                </div>
              </div>
              <button v-if="!grant.revoked_at" type="button" class="btn btn-ghost btn-sm shrink-0 text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-950/20" :aria-label="`${t('mcp.revoke')} ${grant.client_name}`" @click="revokeTarget = grant">{{ t('mcp.revoke') }}</button>
            </li>
          </ul>
        </div>
      </section>
    </div>

    <BaseDialog :show="!!revokeTarget" :title="t('mcp.revokeTitle')" width="narrow" :close-on-escape="!revoking" :show-close-button="!revoking" @close="closeRevoke">
      <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('mcp.revokeConfirm', { name: revokeTarget?.client_name || '' }) }}</p>
      <p v-if="revokeError" role="alert" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ revokeError }}</p>
      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="revoking" @click="closeRevoke">{{ t('common.cancel') }}</button>
          <button data-testid="confirm-revoke" type="button" class="btn btn-danger" :disabled="revoking || loading" @click="revokeGrant">{{ revoking ? t('common.processing') : t('mcp.revoke') }}</button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { mcpAPI, type MCPGrant } from '@/api/mcp'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { Icon } from '@/components/icons'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores/app'
import { formatDate } from '@/utils/format'

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()
const endpoint = ref('')
const connectionLoading = ref(false)
const connectionError = ref('')
const grants = ref<MCPGrant[]>([])
const loading = ref(false)
const loadError = ref('')
const revokeTarget = ref<MCPGrant | null>(null)
const revoking = ref(false)
const revokeError = ref('')
let unmounted = false

function errorMessage(error: unknown, fallback: string): string {
  const message = (error as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : fallback
}

async function loadConnection(): Promise<void> {
  if (connectionLoading.value) return
  connectionLoading.value = true
  connectionError.value = ''
  endpoint.value = ''
  try {
    const result = await mcpAPI.getConnectionConfig()
    if (!unmounted) endpoint.value = result.resource
  } catch (error) {
    if (!unmounted) connectionError.value = errorMessage(error, t('mcp.connectionLoadFailed'))
  } finally {
    connectionLoading.value = false
  }
}

function isActive(grant: MCPGrant): boolean {
  return !grant.revoked_at && !grant.invalidated && new Date(grant.expires_at).getTime() > Date.now()
}

function grantStatus(grant: MCPGrant): string {
  if (grant.revoked_at) return t('mcp.revoked')
  if (grant.invalidated) return t('mcp.invalidated')
  return isActive(grant) ? t('mcp.active') : t('mcp.expired')
}

async function loadGrants(): Promise<void> {
  if (loading.value || revoking.value) return
  loading.value = true
  loadError.value = ''
  try {
    const result = await mcpAPI.listGrants()
    if (!unmounted) grants.value = result
  } catch (error) {
    if (!unmounted) loadError.value = errorMessage(error, t('mcp.loadFailed'))
  } finally {
    loading.value = false
  }
}

function closeRevoke(): void {
  if (revoking.value) return
  revokeTarget.value = null
  revokeError.value = ''
}

async function revokeGrant(): Promise<void> {
  if (!revokeTarget.value || revoking.value || loading.value) return
  revoking.value = true
  revokeError.value = ''
  const id = revokeTarget.value.id
  try {
    await mcpAPI.revokeGrant(id)
    if (unmounted) return
    grants.value = grants.value.map(grant => grant.id === id ? { ...grant, revoked_at: new Date().toISOString() } : grant)
    revokeTarget.value = null
    appStore.showSuccess(t('mcp.revokeSuccess'))
  } catch (error) {
    if (!unmounted) revokeError.value = errorMessage(error, t('mcp.revokeFailed'))
  } finally {
    revoking.value = false
  }
}

onMounted(() => {
  void loadConnection()
  void loadGrants()
})
onBeforeUnmount(() => { unmounted = true })
</script>
