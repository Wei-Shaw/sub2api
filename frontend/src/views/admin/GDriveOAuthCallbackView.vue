<template>
  <div class="flex min-h-screen items-center justify-center bg-gray-50 px-4 dark:bg-dark-900">
    <div class="w-full max-w-md text-center">
      <div v-if="error" class="card p-8">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
          <svg class="h-6 w-6 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
          </svg>
        </div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.soraS3.gdrive.oauthFailed') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ error }}</p>
        <p class="mt-4 text-xs text-gray-400 dark:text-gray-500">{{ t('admin.settings.soraS3.gdrive.closeWindow') }}</p>
      </div>
      <div v-else-if="sent" class="card p-8">
        <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-green-100 dark:bg-green-900/30">
          <svg class="h-6 w-6 text-green-600 dark:text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M5 13l4 4L19 7" />
          </svg>
        </div>
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('admin.settings.soraS3.gdrive.oauthSuccess') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.soraS3.gdrive.closeWindow') }}</p>
      </div>
      <div v-else class="card p-8">
        <div class="mx-auto mb-4 h-8 w-8 animate-spin rounded-full border-4 border-gray-200 border-t-blue-500"></div>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('admin.settings.soraS3.gdrive.processing') }}</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

const route = useRoute()
const { t } = useI18n()

const error = ref('')
const sent = ref(false)

onMounted(() => {
  const code = route.query.code as string
  const state = route.query.state as string
  const errorParam = route.query.error as string
  const errorDesc = route.query.error_description as string

  if (errorParam) {
    error.value = errorDesc || errorParam
    return
  }

  if (!code) {
    error.value = 'No authorization code received'
    return
  }

  // Send code back to opener window via postMessage
  if (window.opener) {
    window.opener.postMessage(
      { type: 'gdrive_oauth_callback', code, state },
      window.location.origin
    )
    sent.value = true
    // Auto-close after a short delay
    setTimeout(() => window.close(), 1500)
  } else {
    error.value = 'Cannot communicate with the parent window. Please close this window and try again.'
  }
})
</script>
