<template>
  <main class="min-h-screen bg-gray-50 px-4 py-10 dark:bg-dark-950">
    <section class="mx-auto max-w-lg rounded-xl bg-white p-8 text-center shadow-sm dark:bg-dark-900">
      <div class="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-red-100 dark:bg-red-900/30">
        <svg class="h-6 w-6 text-red-600 dark:text-red-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
          <path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01M10.29 3.86L1.82 18a2 2 0 001.71 3h16.94a2 2 0 001.71-3L13.71 3.86a2 2 0 00-3.42 0z" />
        </svg>
      </div>
      <h1 class="text-xl font-semibold text-gray-900 dark:text-white">Authorization failed</h1>
      <p v-if="code" class="mt-1 text-xs font-mono text-gray-400 dark:text-dark-500">{{ code }}</p>
      <p class="mt-3 text-sm text-gray-600 dark:text-dark-300">{{ message }}</p>
      <p class="mt-2 text-xs text-gray-400 dark:text-dark-500">
        Return to the application that started this sign-in and try again. No access was granted.
      </p>
      <button type="button" class="btn btn-secondary mt-6" @click="close">Close this page</button>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'

const route = useRoute()

const code = computed(() => (typeof route.query.error === 'string' ? route.query.error : ''))
const message = computed(() => {
  if (typeof route.query.message === 'string' && route.query.message.length > 0) {
    return route.query.message
  }
  const descriptions: Record<string, string> = {
    invalid_client: 'The application is not registered with this Sub2API instance.',
    invalid_request: 'The authorization request was malformed or has expired.',
    invalid_scope: 'The application requested permissions it is not allowed to have.',
    access_denied: 'The authorization was cancelled or denied.',
    transaction_expired: 'The authorization request expired before it was completed.'
  }
  return descriptions[code.value] ?? 'The authorization could not be completed.'
})

function close(): void {
  window.close()
}
</script>
