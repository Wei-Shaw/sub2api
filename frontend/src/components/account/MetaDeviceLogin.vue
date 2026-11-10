<template>
  <div class="space-y-4">
    <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-600 dark:bg-dark-800">
      <div class="mb-3 text-sm text-gray-700 dark:text-gray-200">
        {{ t('admin.accounts.oauth.meta.description') }}
      </div>

      <button
        v-if="!deviceCodeId"
        type="button"
        class="btn btn-primary w-full disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="loading"
        @click="start"
      >
        {{ loading ? t('admin.accounts.oauth.meta.starting') : t('admin.accounts.oauth.meta.startButton') }}
      </button>

      <div v-else class="space-y-4">
        <div class="rounded border border-dashed border-gray-300 p-3 text-center dark:border-dark-500">
          <div class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.accounts.oauth.meta.userCodeLabel') }}</div>
          <div class="mt-1 font-mono text-2xl tracking-widest text-gray-900 dark:text-white">{{ userCode }}</div>
        </div>

        <a
          class="btn btn-secondary w-full"
          :href="verificationURL"
          target="_blank"
          rel="noopener noreferrer"
        >
          {{ t('admin.accounts.oauth.meta.openVerification') }}
        </a>

        <div class="text-xs text-gray-500 dark:text-gray-400">
          {{ statusText }}
        </div>

        <button
          type="button"
          class="btn btn-secondary w-full"
          :disabled="loading"
          @click="poll"
        >
          {{ t('admin.accounts.oauth.meta.checkNow') }}
        </button>
      </div>
    </div>

    <div v-if="error" class="rounded-lg bg-red-50 p-3 text-sm text-red-700 dark:bg-red-900/20 dark:text-red-300">
      {{ error }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'

const props = withDefaults(defineProps<{
  proxyId?: number | null
  name?: string
  concurrency?: number
  priority?: number
  groupIds?: number[]
  pollIntervalSeconds?: number
}>(), {
  proxyId: null,
  name: '',
  concurrency: undefined,
  priority: undefined,
  groupIds: undefined,
  pollIntervalSeconds: 5,
})

const emit = defineEmits<{
  (e: 'created', account: Record<string, unknown>): void
  (e: 'error', message: string): void
}>()

const { t } = useI18n()

const loading = ref(false)
const error = ref('')
const deviceCodeId = ref('')
const userCode = ref('')
const verificationURI = ref('')
const verificationURIComplete = ref('')
const expiresAt = ref(0)
const state = ref('pending')
const now = ref(Date.now())

let pollTimer: ReturnType<typeof setTimeout> | null = null
const POLL_TICK_MS = 1000

const verificationURL = computed(() => verificationURIComplete.value || verificationURI.value)

const statusText = computed(() => {
  if (state.value === 'slow_down') return t('admin.accounts.oauth.meta.stateSlowDown')
  if (state.value === 'denied') return t('admin.accounts.oauth.meta.stateDenied')
  if (state.value === 'expired') return t('admin.accounts.oauth.meta.stateExpired')
  const remaining = Math.max(0, Math.floor((expiresAt.value - now.value) / 1000))
  return t('admin.accounts.oauth.meta.statePending', { seconds: remaining })
})

function stopPolling() {
  if (pollTimer) {
    clearTimeout(pollTimer)
    pollTimer = null
  }
}

function nextIntervalSeconds() {
  return props.pollIntervalSeconds > 0 ? props.pollIntervalSeconds : 5
}

async function start() {
  loading.value = true
  error.value = ''
  state.value = 'pending'
  try {
    const started = await adminAPI.meta.startDeviceFlow(props.proxyId ? { proxy_id: props.proxyId } : {})
    deviceCodeId.value = started.device_code_id
    userCode.value = started.user_code
    verificationURI.value = started.verification_uri
    verificationURIComplete.value = started.verification_uri_complete
    expiresAt.value = Date.now() + started.expires_in * 1000
    now.value = Date.now()
    stopPolling()
    pollTimer = setTimeout(poll, (started.interval > 0 ? started.interval : nextIntervalSeconds()) * POLL_TICK_MS)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

async function poll() {
  if (!deviceCodeId.value) return
  loading.value = true
  now.value = Date.now()
  try {
    const result = await adminAPI.meta.pollDeviceFlow({
      device_code_id: deviceCodeId.value,
      name: props.name || undefined,
      concurrency: props.concurrency,
      priority: props.priority,
      group_ids: props.groupIds,
      proxy_id: props.proxyId ?? undefined,
    })
    state.value = result.state
    if (result.state === 'authorized') {
      stopPolling()
      emit('created', (result.account as Record<string, unknown>) ?? {})
      return
    }
    if (result.state === 'denied' || result.state === 'expired') {
      stopPolling()
      const fallback = result.state === 'denied'
        ? t('admin.accounts.oauth.meta.stateDenied')
        : t('admin.accounts.oauth.meta.stateExpired')
      error.value = result.message || fallback
      return
    }
    const seconds = result.interval > 0 ? result.interval : nextIntervalSeconds()
    pollTimer = setTimeout(poll, seconds * POLL_TICK_MS)
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

onBeforeUnmount(stopPolling)

defineExpose({ start, poll })
</script>
