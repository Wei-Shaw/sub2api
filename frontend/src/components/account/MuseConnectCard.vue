<template>
  <section class="space-y-3 rounded-lg border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-700" data-testid="muse-connect">
    <div>
      <p class="font-medium text-gray-900 dark:text-gray-100">{{ t('admin.accounts.muse.connectTitle') }}</p>
      <p class="input-hint">{{ t('admin.accounts.muse.connectDescription') }}</p>
    </div>
    <button type="button" class="btn btn-primary" :disabled="!canConnect" data-testid="muse-connect-start" @click="start">
      {{ t(received ? 'admin.accounts.muse.connectAgain' : 'admin.accounts.muse.connectButton') }}
    </button>
    <div v-if="pending" class="space-y-2 text-sm" role="status">
      <p>{{ t('admin.accounts.muse.connectWaiting') }}</p>
      <p class="text-gray-500 dark:text-gray-400">{{ t('admin.accounts.muse.connectDestination', { origin, owner: ownerLabel }) }}</p>
      <button type="button" class="text-primary-600 dark:text-primary-400" @click="cancel">{{ t('common.cancel') }}</button>
      <span hidden data-sub2api-muse-connect="1" :data-nonce="pending.nonce" :data-origin="origin"
        :data-owner-id="ownerId" :data-owner-label="ownerLabel" :data-account-id="accountId || 0" :data-expires-at="pending.expiresAt" />
    </div>
    <p v-if="received" role="status" class="text-sm text-green-700 dark:text-green-400">{{ t('admin.accounts.muse.connectReceived') }}</p>
    <p v-if="error" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t(error) }}</p>
    <details class="text-sm" data-testid="muse-export-guide">
      <summary class="cursor-pointer font-medium text-primary-600 dark:text-primary-400">{{ t('admin.accounts.muse.connectHelperTitle') }}</summary>
      <div class="mt-3 space-y-3">
        <p class="input-hint">{{ t('admin.accounts.muse.connectHelperDescription') }}</p>
        <a href="/muse-session-exporter.zip?v=3" download class="btn btn-secondary btn-sm inline-flex">{{ t('admin.accounts.muse.connectHelperDownload') }}</a>
        <ol class="list-decimal space-y-2 pl-5 text-gray-600 dark:text-gray-300">
          <li>{{ t('admin.accounts.muse.connectHelperInstall') }} <code class="text-xs">chrome://extensions</code> / <code class="text-xs">edge://extensions</code></li>
          <li>{{ t('admin.accounts.muse.connectHelperSignIn') }} <a href="https://muse.ai/" target="_blank" rel="noopener noreferrer">muse.ai</a></li>
          <li>{{ t('admin.accounts.muse.connectHelperApprove') }}</li>
        </ol>
      </div>
    </details>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { parseMuseConnectSession } from './museSession'

const props = defineProps<{ ownerId: number; ownerLabel: string; accountId?: number; disabled?: boolean }>()
const emit = defineEmits<{ connected: [session: string]; invalidated: [] }>()
const { t } = useI18n()
const origin = window.location.origin
const pending = ref<{ nonce: string; expiresAt: number }>()
const received = ref(false)
const error = ref('')
let expiryTimer: ReturnType<typeof setTimeout> | undefined
const canConnect = computed(() => Number.isSafeInteger(props.ownerId) && props.ownerId > 0 && !props.disabled)

function cancel() {
  clearTimeout(expiryTimer)
  expiryTimer = undefined
  pending.value = undefined
}
function start() {
  if (!canConnect.value) return
  cancel()
  error.value = ''
  const secure = window.location.protocol === 'https:' || ['localhost', '127.0.0.1', '[::1]'].includes(window.location.hostname)
  if (!secure || !window.crypto?.getRandomValues) { error.value = 'admin.accounts.muse.connectSecureOrigin'; return }
  const bytes = window.crypto.getRandomValues(new Uint8Array(32))
  pending.value = { nonce: Array.from(bytes, n => n.toString(16).padStart(2, '0')).join(''), expiresAt: Date.now() + 120_000 }
  expiryTimer = setTimeout(() => { cancel(); error.value = 'admin.accounts.muse.connectExpired' }, 120_000)
}
function receive(event: MessageEvent) {
  const request = pending.value
  if (!request || event.source !== window || event.origin !== origin) return
  const data = event.data
  if (!data || data.type !== 'sub2api:muse:session:v1' || data.nonce !== request.nonce ||
      data.ownerId !== props.ownerId || data.accountId !== (props.accountId || 0)) return
  if (Date.now() >= request.expiresAt) { cancel(); error.value = 'admin.accounts.muse.connectExpired'; return }
  // Consume before parsing: a rejected or accepted transfer cannot be replayed.
  cancel()
  try {
    const raw = JSON.stringify(data.session)
    if (!raw || raw.length > 64 * 1024) throw new Error('invalid session')
    parseMuseConnectSession(raw)
    received.value = true
    error.value = ''
    emit('connected', raw)
  } catch { error.value = 'admin.accounts.muse.connectFailed' }
}
watch(() => [props.ownerId, props.accountId, props.disabled], () => {
  cancel()
  error.value = ''
  if (received.value) emit('invalidated')
  received.value = false
}, { flush: 'sync' })
onMounted(() => window.addEventListener('message', receive))
onUnmounted(() => { cancel(); window.removeEventListener('message', receive) })
</script>
