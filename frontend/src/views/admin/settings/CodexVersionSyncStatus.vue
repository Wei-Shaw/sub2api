<template>
  <div class="text-xs text-gray-500 dark:text-gray-400" data-testid="codex-version-sync-status">
    <details>
      <summary class="cursor-pointer leading-6" aria-live="polite">
        <template v-if="status">
          {{ t('admin.settings.gatewayForwarding.codexSync.effective', { version: status.effective_version }) }}
          · {{ t(sourceKeys[status.version_source]) }} ·
          <span :class="warning ? 'text-amber-600 dark:text-amber-400' : ''">{{ statusLabel }}</span>
        </template>
        <template v-else>
          {{ t(readFailed ? 'admin.settings.gatewayForwarding.codexSync.unavailable' : 'admin.settings.gatewayForwarding.codexSync.loading') }}
        </template>
      </summary>
      <div class="mt-2 space-y-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-800">
        <p>{{ t('admin.settings.gatewayForwarding.codexSync.savedHint') }}</p>
        <dl v-if="status" class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5">
          <dt>{{ t('admin.settings.gatewayForwarding.codexSync.synced') }}</dt>
          <dd>{{ status.synced_version || '—' }}</dd>
          <dt>{{ t('admin.settings.gatewayForwarding.codexSync.checked') }}</dt>
          <dd>{{ formatTime(status.last_checked_at) }}</dd>
          <dt>{{ t('admin.settings.gatewayForwarding.codexSync.succeeded') }}</dt>
          <dd>{{ formatTime(status.last_succeeded_at) }}</dd>
          <dt>{{ t('admin.settings.gatewayForwarding.codexSync.updated') }}</dt>
          <dd>{{ formatTime(status.last_version_updated_at) }}</dd>
          <dt>{{ t('admin.settings.gatewayForwarding.codexSync.next') }}</dt>
          <dd>{{ formatTime(status.next_check_at) }}</dd>
          <template v-if="status.rate_limit_reset_at">
            <dt>{{ t('admin.settings.gatewayForwarding.codexSync.reset') }}</dt>
            <dd>{{ formatTime(status.rate_limit_reset_at) }}</dd>
          </template>
          <template v-if="status.status === 'failed'">
            <dt>{{ t('admin.settings.gatewayForwarding.codexSync.error') }}</dt>
            <dd class="text-amber-600 dark:text-amber-400">
              {{ errorLabel }}<span v-if="status.http_status"> (HTTP {{ status.http_status }})</span>
            </dd>
          </template>
        </dl>
        <p v-if="status?.retry_exhausted">{{ t('admin.settings.gatewayForwarding.codexSync.exhaustedHint') }}</p>
        <p v-if="status?.persistence_failed" class="text-amber-600 dark:text-amber-400">{{ t('admin.settings.gatewayForwarding.codexSync.persistenceHint') }}</p>
        <button type="button" class="text-primary-600 hover:underline disabled:opacity-50" :disabled="loading" @click="loadStatus">
          {{ t('admin.settings.gatewayForwarding.codexSync.refresh') }}
        </button>
      </div>
    </details>
    <p v-if="status?.version_source === 'manual'" class="mt-1">
      {{ t('admin.settings.gatewayForwarding.codexSync.manualHint') }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type { CodexVersionSyncStatus } from '@/api/admin/settings'

const props = withDefaults(defineProps<{ active?: boolean; refreshToken?: number }>(), {
  active: true,
  refreshToken: 0,
})
const { t, locale } = useI18n()
const status = ref<CodexVersionSyncStatus | null>(null)
const loading = ref(false)
const readFailed = ref(false)
let sequence = 0
let timer: ReturnType<typeof setInterval> | undefined

const sourceKeys = {
  manual: 'admin.settings.gatewayForwarding.codexSync.sourceManual',
  synced: 'admin.settings.gatewayForwarding.codexSync.sourceSynced',
  builtin: 'admin.settings.gatewayForwarding.codexSync.sourceBuiltin',
}
const errorKeys: Record<string, string> = {
  github_primary_rate_limit: 'admin.settings.gatewayForwarding.codexSync.errors.primaryLimit',
  github_secondary_rate_limit: 'admin.settings.gatewayForwarding.codexSync.errors.secondaryLimit',
  github_rate_limit: 'admin.settings.gatewayForwarding.codexSync.errors.secondaryLimit',
  github_unauthorized: 'admin.settings.gatewayForwarding.codexSync.errors.unauthorized',
  github_forbidden: 'admin.settings.gatewayForwarding.codexSync.errors.forbidden',
  github_unavailable: 'admin.settings.gatewayForwarding.codexSync.errors.unavailable',
  network_timeout: 'admin.settings.gatewayForwarding.codexSync.errors.timeout',
  network_error: 'admin.settings.gatewayForwarding.codexSync.errors.network',
  no_stable_release: 'admin.settings.gatewayForwarding.codexSync.errors.noRelease',
  github_invalid_response: 'admin.settings.gatewayForwarding.codexSync.errors.invalidResponse',
  version_persist_failed: 'admin.settings.gatewayForwarding.codexSync.errors.persistence',
  settings_read_failed: 'admin.settings.gatewayForwarding.codexSync.errors.settings',
}
const warning = computed(() => status.value?.persistence_failed || (status.value?.auto_sync_enabled && status.value.status === 'failed'))
const statusLabel = computed(() => {
  if (status.value?.persistence_failed) return t('admin.settings.gatewayForwarding.codexSync.persistenceFailed')
  if (!status.value?.auto_sync_enabled) return t('admin.settings.gatewayForwarding.codexSync.disabled')
  if (status.value.status === 'success') return t('admin.settings.gatewayForwarding.codexSync.success')
  if (status.value.status === 'failed') return t('admin.settings.gatewayForwarding.codexSync.failed')
  return t('admin.settings.gatewayForwarding.codexSync.neverChecked')
})
const errorLabel = computed(() => t(errorKeys[status.value?.error_code || ''] || 'admin.settings.gatewayForwarding.codexSync.errors.other'))

function formatTime(value?: string | null): string {
  if (!value) return '—'
  const at = new Date(value)
  return Number.isNaN(at.getTime()) ? '—' : at.toLocaleString(locale.value)
}

async function loadStatus() {
  if (!props.active || document.visibilityState === 'hidden') return
  const current = ++sequence
  loading.value = true
  try {
    const snapshot = await adminAPI.settings.getOpenAICodexVersionSyncStatus()
    if (current !== sequence || !props.active) return
    status.value = snapshot
    readFailed.value = false
  } catch {
    if (current !== sequence || !props.active) return
    // 不用上一次的绿色状态掩盖当前读取失败，也不影响设置表单的加载与保存。
    status.value = null
    readFailed.value = true
  } finally {
    if (current === sequence) loading.value = false
  }
}

function restartPolling() {
  sequence++
  loading.value = false
  if (timer) clearInterval(timer)
  timer = undefined
  if (!props.active || document.visibilityState === 'hidden') return
  void loadStatus()
  // 只刷新本地只读快照，不会触发 GitHub 请求；隐藏标签或切换设置页后停止。
  timer = setInterval(() => { if (!loading.value) void loadStatus() }, 60_000)
}

watch(() => [props.active, props.refreshToken], restartPolling, { immediate: true })
onMounted(() => { document.addEventListener('visibilitychange', restartPolling) })
onUnmounted(() => {
  sequence++
  if (timer) clearInterval(timer)
  document.removeEventListener('visibilitychange', restartPolling)
})
</script>
