<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-medium text-gray-900 dark:text-white">
        {{ t('profile.systemToken.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('profile.systemToken.description') }}
      </p>
    </div>
    <div class="px-6 py-6 space-y-4">
      <!-- Loading -->
      <div v-if="loading" class="text-sm text-gray-500">{{ t('common.loading') }}...</div>

      <!-- Generated token display (shown once after generation) -->
      <div v-else-if="generatedToken" class="space-y-3">
        <div class="rounded-lg border border-green-200 bg-green-50 p-4 dark:border-green-800 dark:bg-green-900/20">
          <p class="mb-2 text-sm font-medium text-green-800 dark:text-green-300">
            {{ t('profile.systemToken.tokenGenerated') }}
          </p>
          <div class="flex items-center gap-2">
            <code class="flex-1 break-all rounded bg-white px-3 py-2 font-mono text-sm dark:bg-dark-800">
              {{ generatedToken }}
            </code>
            <button @click="copyToken" class="btn btn-secondary px-3 py-2 text-sm shrink-0">
              {{ t('common.copy') }}
            </button>
          </div>
          <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
            {{ t('profile.systemToken.usage') }}
          </p>
        </div>
      </div>

      <!-- Status -->
      <template v-else>
        <p v-if="hasToken" class="text-sm text-gray-600 dark:text-gray-400">
          {{ t('profile.systemToken.hasToken') }}
        </p>
        <p v-else class="text-sm text-gray-500 dark:text-gray-400">
          {{ t('profile.systemToken.noToken') }}
        </p>
      </template>

      <!-- Actions -->
      <div class="flex gap-3">
        <button
          v-if="!hasToken && !generatedToken"
          @click="handleGenerate"
          :disabled="operating"
          class="btn btn-primary"
        >
          {{ t('profile.systemToken.generate') }}
        </button>
        <button
          v-if="hasToken || generatedToken"
          @click="handleRegenerate"
          :disabled="operating"
          class="btn btn-secondary"
        >
          {{ t('profile.systemToken.regenerate') }}
        </button>
        <button
          v-if="hasToken || generatedToken"
          @click="handleRevoke"
          :disabled="operating"
          class="btn btn-danger"
        >
          {{ t('profile.systemToken.revoke') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { systemTokenAPI } from '@/api'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(true)
const operating = ref(false)
const hasToken = ref(false)
const generatedToken = ref('')

onMounted(async () => {
  try {
    const res = await systemTokenAPI.getSystemTokenStatus()
    hasToken.value = res.has_token
  } catch {
    // ignore
  } finally {
    loading.value = false
  }
})

async function handleGenerate() {
  operating.value = true
  try {
    const res = await systemTokenAPI.generateSystemToken()
    generatedToken.value = res.token
    hasToken.value = true
  } catch {
    appStore.showError(t('common.operationFailed'))
  } finally {
    operating.value = false
  }
}

async function handleRegenerate() {
  if (!confirm(t('profile.systemToken.confirmRegenerate'))) return
  await handleGenerate()
}

async function handleRevoke() {
  if (!confirm(t('profile.systemToken.confirmRevoke'))) return
  operating.value = true
  try {
    await systemTokenAPI.revokeSystemToken()
    hasToken.value = false
    generatedToken.value = ''
    appStore.showSuccess(t('profile.systemToken.tokenRevoked'))
  } catch {
    appStore.showError(t('common.operationFailed'))
  } finally {
    operating.value = false
  }
}

function copyToken() {
  navigator.clipboard.writeText(generatedToken.value)
  appStore.showSuccess(t('profile.systemToken.copySuccess'))
}
</script>
