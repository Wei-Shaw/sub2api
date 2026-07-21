<template>
  <div
    class="rounded-lg border border-blue-200 bg-blue-50 p-4 dark:border-blue-700 dark:bg-blue-900/30"
  >
    <div class="flex items-start gap-4">
      <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-blue-500">
        <Icon name="link" size="md" class="text-white" />
      </div>
      <div class="flex-1">
        <h4 class="mb-3 font-semibold text-blue-900 dark:text-blue-200">
          {{ t('admin.accounts.oauth.kimi.title') }}
        </h4>
        <p class="mb-4 text-sm text-blue-800 dark:text-blue-300">
          {{ t('admin.accounts.oauth.kimi.followSteps') }}
        </p>

        <!-- Step 1: Start device authorization -->
        <div
          class="rounded-lg border border-blue-300 bg-white/80 p-4 dark:border-blue-600 dark:bg-gray-800/80"
        >
          <div class="flex items-start gap-3">
            <div
              class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-blue-600 text-xs font-bold text-white"
            >
              1
            </div>
            <div class="flex-1">
              <p class="mb-2 font-medium text-blue-900 dark:text-blue-200">
                {{ t('admin.accounts.oauth.kimi.step1Start') }}
              </p>
              <button
                v-if="!hasSession || isTerminal"
                type="button"
                :disabled="loading"
                class="btn btn-primary text-sm"
                @click="handleStart"
              >
                <svg
                  v-if="loading"
                  class="-ml-1 mr-2 h-4 w-4 animate-spin"
                  fill="none"
                  viewBox="0 0 24 24"
                >
                  <circle
                    class="opacity-25"
                    cx="12"
                    cy="12"
                    r="10"
                    stroke="currentColor"
                    stroke-width="4"
                  ></circle>
                  <path
                    class="opacity-75"
                    fill="currentColor"
                    d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                  ></path>
                </svg>
                <Icon v-else name="link" size="sm" class="mr-2" />
                {{
                  loading
                    ? t('admin.accounts.oauth.kimi.starting')
                    : isTerminal
                      ? t('admin.accounts.oauth.kimi.restart')
                      : t('admin.accounts.oauth.kimi.startAuth')
                }}
              </button>
              <div v-else class="space-y-3">
                <!-- User code display -->
                <div>
                  <label class="input-label">{{ t('admin.accounts.oauth.kimi.userCode') }}</label>
                  <div class="flex items-center gap-2">
                    <code
                      class="flex-1 rounded-lg border border-blue-200 bg-gray-50 px-4 py-2 text-center font-mono text-xl font-bold tracking-[0.3em] text-gray-900 dark:border-blue-700 dark:bg-gray-700 dark:text-white"
                    >
                      {{ userCode }}
                    </code>
                    <button
                      type="button"
                      class="btn btn-secondary p-2"
                      :title="t('admin.accounts.oauth.kimi.copyCode')"
                      @click="handleCopyCode"
                    >
                      <svg
                        v-if="!copied"
                        class="h-4 w-4"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="1.5"
                      >
                        <path
                          stroke-linecap="round"
                          stroke-linejoin="round"
                          d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184"
                        />
                      </svg>
                      <Icon v-else name="check" size="sm" class="text-green-500" :stroke-width="2" />
                    </button>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Step 2: Open verification page and confirm -->
        <div
          v-if="hasSession && !isTerminal"
          class="mt-4 rounded-lg border border-blue-300 bg-white/80 p-4 dark:border-blue-600 dark:bg-gray-800/80"
        >
          <div class="flex items-start gap-3">
            <div
              class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-blue-600 text-xs font-bold text-white"
            >
              2
            </div>
            <div class="flex-1">
              <p class="mb-2 font-medium text-blue-900 dark:text-blue-200">
                {{ t('admin.accounts.oauth.kimi.step2Open') }}
              </p>
              <p class="mb-3 text-sm text-blue-700 dark:text-blue-300">
                {{ t('admin.accounts.oauth.kimi.openUrlDesc') }}
              </p>
              <a
                :href="verificationUriComplete"
                target="_blank"
                rel="noreferrer"
                class="btn btn-primary text-sm"
              >
                <Icon name="link" size="sm" class="mr-2" />
                {{ t('admin.accounts.oauth.kimi.openVerificationPage') }}
              </a>

              <!-- Polling status -->
              <div class="mt-3 flex items-center gap-2 text-sm">
                <template v-if="status === 'awaiting_user'">
                  <svg class="h-4 w-4 animate-spin text-blue-500" fill="none" viewBox="0 0 24 24">
                    <circle
                      class="opacity-25"
                      cx="12"
                      cy="12"
                      r="10"
                      stroke="currentColor"
                      stroke-width="4"
                    ></circle>
                    <path
                      class="opacity-75"
                      fill="currentColor"
                      d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"
                    ></path>
                  </svg>
                  <span class="text-blue-700 dark:text-blue-300">
                    {{ t('admin.accounts.oauth.kimi.waiting') }}
                  </span>
                </template>
                <template v-else-if="status === 'success'">
                  <Icon name="check" size="sm" class="text-green-500" :stroke-width="2" />
                  <span class="text-green-600 dark:text-green-400">
                    {{ t('admin.accounts.oauth.kimi.authorized') }}
                  </span>
                </template>
              </div>
            </div>
          </div>
        </div>

        <!-- Terminal error (expired / denied / failed) -->
        <div
          v-if="isTerminal && error"
          class="mt-4 rounded-lg border border-red-200 bg-red-50 p-3 dark:border-red-700 dark:bg-red-900/30"
        >
          <p class="whitespace-pre-line text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import { useKimiOAuth } from '@/composables/useKimiOAuth'
import type { KimiTokenInfo } from '@/api/admin/kimi'
import Icon from '@/components/icons/Icon.vue'

interface Props {
  proxyId?: number | null
}

const props = withDefaults(defineProps<Props>(), {
  proxyId: null
})

const emit = defineEmits<{
  authorized: [tokenInfo: KimiTokenInfo]
}>()

const { t } = useI18n()
const { copied, copyToClipboard } = useClipboard()

const kimiOAuth = useKimiOAuth({
  onAuthorized: (tokenInfo) => {
    emit('authorized', tokenInfo)
  }
})

const { sessionId, userCode, verificationUriComplete, status, loading, error } = kimiOAuth

const hasSession = computed(() => !!sessionId.value)
const isTerminal = computed(
  () => status.value === 'expired' || status.value === 'denied' || status.value === 'error'
)

const handleStart = async () => {
  await kimiOAuth.startDeviceAuth(props.proxyId)
}

const handleCopyCode = () => {
  if (userCode.value) {
    copyToClipboard(userCode.value, t('admin.accounts.oauth.kimi.codeCopied'))
  }
}

defineExpose({
  start: handleStart,
  reset: kimiOAuth.resetState,
  stopPolling: kimiOAuth.stopPolling
})
</script>
