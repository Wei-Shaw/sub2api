<template>
  <div v-if="tgAuthResult" class="min-h-screen bg-surface-sunken px-4 py-10">
    <div class="mx-auto max-w-2xl card p-6 text-center">
      <div class="mx-auto h-8 w-8 animate-spin rounded-full border-2 border-accent border-t-transparent"></div>
      <p class="mt-4 text-body text-fg-muted">{{ t('auth.telegram.callbackProcessing') }}</p>
    </div>
  </div>
  <LinuxDoCallbackView v-else provider="telegram" />
</template>

<script setup lang="ts">
import { onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { postTelegramAuthResult, TELEGRAM_POPUP_RESULT_EVENT } from '@/utils/telegramLogin'
import LinuxDoCallbackView from './LinuxDoCallbackView.vue'

const { t } = useI18n()

// Telegram returns `#tgAuthResult=<base64 json>`. Hand it to the backend in a POST body
// (not a query string, so it stays out of access logs); the backend verifies it and redirects
// back here with the usual token / pending-session fragment, which LinuxDoCallbackView handles.
const tgAuthResult = new URLSearchParams(window.location.hash.slice(1)).get('tgAuthResult')

onMounted(() => {
  if (!tgAuthResult) return
  history.replaceState(null, '', window.location.pathname + window.location.search)
  // Inside the login popup: hand the result to the page that opened it.
  if (window.opener && window.opener !== window) {
    window.opener.postMessage({ event: TELEGRAM_POPUP_RESULT_EVENT, result: tgAuthResult }, window.location.origin)
    window.close()
    return
  }
  postTelegramAuthResult(tgAuthResult)
})
</script>
