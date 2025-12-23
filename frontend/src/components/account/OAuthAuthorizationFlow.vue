<template>
  <div class="rounded-lg border border-blue-200 bg-blue-50 dark:border-blue-700 dark:bg-blue-900/30 p-6">
    <div class="flex items-start gap-4">
      <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-blue-500">
        <svg class="w-5 h-5 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
          <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m13.35-.622l1.757-1.757a4.5 4.5 0 00-6.364-6.364l-4.5 4.5a4.5 4.5 0 001.242 7.244" />
        </svg>
      </div>
      <div class="flex-1">
        <h4 class="mb-3 font-semibold text-blue-900 dark:text-blue-200">{{ oauthTitle }}</h4>

        <!-- Auth Method Selection -->
        <div v-if="showCookieOption" class="mb-4">
          <label class="mb-2 block text-sm font-medium text-blue-800 dark:text-blue-300">
            {{ methodLabel }}
          </label>
          <div class="flex gap-4">
            <label class="flex cursor-pointer items-center gap-2">
              <input
                v-model="inputMethod"
                type="radio"
                value="manual"
                class="text-blue-600 focus:ring-blue-500"
              />
              <span class="text-sm text-blue-900 dark:text-blue-200">{{ t('admin.accounts.oauth.manualAuth') }}</span>
            </label>
            <label class="flex cursor-pointer items-center gap-2">
              <input
                v-model="inputMethod"
                type="radio"
                value="cookie"
                class="text-blue-600 focus:ring-blue-500"
              />
              <span class="text-sm text-blue-900 dark:text-blue-200">{{ t('admin.accounts.oauth.cookieAutoAuth') }}</span>
            </label>
          </div>
        </div>

        <!-- Cookie Auto-Auth Form -->
        <div v-if="inputMethod === 'cookie'" class="space-y-4">
          <div class="rounded-lg border border-blue-300 dark:border-blue-600 bg-white/80 dark:bg-gray-800/80 p-4">
            <p class="mb-3 text-sm text-blue-700 dark:text-blue-300">
              {{ t('admin.accounts.oauth.cookieAutoAuthDesc') }}
            </p>

            <!-- sessionKey Input -->
            <div class="mb-4">
              <label class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-700 dark:text-gray-300">
                <svg class="w-4 h-4 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
                </svg>
                {{ t('admin.accounts.oauth.sessionKey') }}
                <span
                  v-if="parsedKeyCount > 1 && allowMultiple"
                  class="rounded-full bg-blue-500 px-2 py-0.5 text-xs text-white"
                >
                  {{ t('admin.accounts.oauth.keysCount', { count: parsedKeyCount }) }}
                </span>
                <button
                  v-if="showHelp"
                  type="button"
                  class="text-blue-500 hover:text-blue-600"
                  @click="showHelpDialog = !showHelpDialog"
                >
                  <svg class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M9.879 7.519c1.171-1.025 3.071-1.025 4.242 0 1.172 1.025 1.172 2.687 0 3.712-.203.179-.43.326-.67.442-.745.361-1.45.999-1.45 1.827v.75M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9 5.25h.008v.008H12v-.008z" />
                  </svg>
                </button>
              </label>
              <textarea
                v-model="sessionKeyInput"
                rows="3"
                class="input w-full font-mono text-sm resize-y"
                :placeholder="allowMultiple ? t('admin.accounts.oauth.sessionKeyPlaceholder') : t('admin.accounts.oauth.sessionKeyPlaceholderSingle')"
              ></textarea>
              <p
                v-if="parsedKeyCount > 1 && allowMultiple"
                class="mt-1 text-xs text-blue-600 dark:text-blue-400"
              >
                {{ t('admin.accounts.oauth.batchCreateAccounts', { count: parsedKeyCount }) }}
              </p>
            </div>

            <!-- Help Section -->
            <div
              v-if="showHelpDialog && showHelp"
              class="mb-4 rounded-lg border border-amber-200 bg-amber-50 dark:border-amber-700 dark:bg-amber-900/30 p-3"
            >
              <h5 class="mb-2 font-semibold text-amber-800 dark:text-amber-200">
                {{ t('admin.accounts.oauth.howToGetSessionKey') }}
              </h5>
              <ol class="list-inside list-decimal space-y-1 text-xs text-amber-700 dark:text-amber-300">
                <li v-html="t('admin.accounts.oauth.step1')"></li>
                <li v-html="t('admin.accounts.oauth.step2')"></li>
                <li v-html="t('admin.accounts.oauth.step3')"></li>
                <li v-html="t('admin.accounts.oauth.step4')"></li>
                <li v-html="t('admin.accounts.oauth.step5')"></li>
                <li v-html="t('admin.accounts.oauth.step6')"></li>
              </ol>
              <p class="mt-2 text-xs text-amber-600 dark:text-amber-400" v-html="t('admin.accounts.oauth.sessionKeyFormat')"></p>
            </div>

            <!-- Error Message -->
            <div
              v-if="error"
              class="mb-4 rounded-lg border border-red-200 bg-red-50 dark:border-red-700 dark:bg-red-900/30 p-3"
            >
              <p class="text-sm text-red-600 dark:text-red-400 whitespace-pre-line">
                {{ error }}
              </p>
            </div>

            <!-- Auth Button -->
            <button
              type="button"
              class="btn btn-primary w-full"
              :disabled="loading || !sessionKeyInput.trim()"
              @click="handleCookieAuth"
            >
              <svg v-if="loading" class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
              </svg>
              <svg v-else class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09z" />
              </svg>
              {{ loading ? t('admin.accounts.oauth.authorizing') : t('admin.accounts.oauth.startAutoAuth') }}
            </button>
          </div>
        </div>

        <!-- Manual Authorization Flow -->
        <div v-else class="space-y-4">
          <p class="mb-4 text-sm text-blue-800 dark:text-blue-300">
            {{ oauthFollowSteps }}
          </p>

          <!-- Step 1: Generate Auth URL -->
          <div class="rounded-lg border border-blue-300 dark:border-blue-600 bg-white/80 dark:bg-gray-800/80 p-4">
            <div class="flex items-start gap-3">
              <div class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-blue-600 text-xs font-bold text-white">
                1
              </div>
              <div class="flex-1">
                <p class="mb-2 font-medium text-blue-900 dark:text-blue-200">
                  {{ oauthStep1GenerateUrl }}
                </p>
                <button
                  v-if="!authUrl"
                  type="button"
                  :disabled="loading"
                  class="btn btn-primary text-sm"
                  @click="handleGenerateUrl"
                >
                  <svg v-if="loading" class="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
                    <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
                    <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
                  </svg>
                  <svg v-else class="w-4 h-4 mr-2" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m13.35-.622l1.757-1.757a4.5 4.5 0 00-6.364-6.364l-4.5 4.5a4.5 4.5 0 001.242 7.244" />
                  </svg>
                  {{ loading ? t('admin.accounts.oauth.generating') : oauthGenerateAuthUrl }}
                </button>
                <div v-else class="space-y-3">
                  <div class="flex items-center gap-2">
                    <input
                      :value="authUrl"
                      readonly
                      type="text"
                      class="input flex-1 bg-gray-50 dark:bg-gray-700 font-mono text-xs"
                    />
                    <button
                      type="button"
                      class="btn btn-secondary p-2"
                      title="Copy URL"
                      @click="handleCopyUrl"
                    >
                      <svg v-if="!copied" class="w-4 h-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M15.666 3.888A2.25 2.25 0 0013.5 2.25h-3c-1.03 0-1.9.693-2.166 1.638m7.332 0c.055.194.084.4.084.612v0a.75.75 0 01-.75.75H9a.75.75 0 01-.75-.75v0c0-.212.03-.418.084-.612m7.332 0c.646.049 1.288.11 1.927.184 1.1.128 1.907 1.077 1.907 2.185V19.5a2.25 2.25 0 01-2.25 2.25H6.75A2.25 2.25 0 014.5 19.5V6.257c0-1.108.806-2.057 1.907-2.185a48.208 48.208 0 011.927-.184" />
                      </svg>
                      <svg v-else class="w-4 h-4 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
                      </svg>
                    </button>
                  </div>
                  <button
                    type="button"
                    class="text-xs text-blue-600 hover:text-blue-700 dark:text-blue-400"
                    @click="handleRegenerate"
                  >
                    <svg class="w-3 h-3 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
                    </svg>
                    {{ t('admin.accounts.oauth.regenerate') }}
                  </button>
                </div>
              </div>
            </div>
          </div>

          <!-- Step 2: Open URL and authorize -->
          <div class="rounded-lg border border-blue-300 dark:border-blue-600 bg-white/80 dark:bg-gray-800/80 p-4">
            <div class="flex items-start gap-3">
              <div class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-blue-600 text-xs font-bold text-white">
                2
              </div>
              <div class="flex-1">
                <p class="mb-2 font-medium text-blue-900 dark:text-blue-200">
                  {{ oauthStep2OpenUrl }}
                </p>
                <p class="text-sm text-blue-700 dark:text-blue-300">
                  {{ oauthOpenUrlDesc }}
                </p>
                <!-- OpenAI Important Notice -->
                <div v-if="isOpenAI" class="mt-2 rounded border border-amber-300 dark:border-amber-700 bg-amber-50 dark:bg-amber-900/30 p-3">
                  <p class="text-xs text-amber-800 dark:text-amber-300" v-html="oauthImportantNotice">
                  </p>
                </div>
                <!-- Proxy Warning (for non-OpenAI) -->
                <div v-else-if="showProxyWarning" class="mt-2 rounded border border-yellow-300 dark:border-yellow-700 bg-yellow-50 dark:bg-yellow-900/30 p-3">
                  <p class="text-xs text-yellow-800 dark:text-yellow-300" v-html="t('admin.accounts.oauth.proxyWarning')">
                  </p>
                </div>
              </div>
            </div>
          </div>

          <!-- Step 3: Enter authorization code -->
          <div class="rounded-lg border border-blue-300 dark:border-blue-600 bg-white/80 dark:bg-gray-800/80 p-4">
            <div class="flex items-start gap-3">
              <div class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full bg-blue-600 text-xs font-bold text-white">
                3
              </div>
              <div class="flex-1">
                <p class="mb-2 font-medium text-blue-900 dark:text-blue-200">
                  {{ oauthStep3EnterCode }}
                </p>
                <p class="mb-3 text-sm text-blue-700 dark:text-blue-300" v-html="oauthAuthCodeDesc">
                </p>
                <div>
                  <label class="input-label">
                    <svg class="w-4 h-4 inline mr-1 text-blue-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M15.75 5.25a3 3 0 013 3m3 0a6 6 0 01-7.029 5.912c-.563-.097-1.159.026-1.563.43L10.5 17.25H8.25v2.25H6v2.25H2.25v-2.818c0-.597.237-1.17.659-1.591l6.499-6.499c.404-.404.527-1 .43-1.563A6 6 0 1121.75 8.25z" />
                    </svg>
                    {{ oauthAuthCode }}
                  </label>
                  <textarea
                    v-model="authCodeInput"
                    rows="3"
                    class="input w-full font-mono text-sm resize-none"
                    :placeholder="oauthAuthCodePlaceholder"
                  ></textarea>
                  <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                    <svg class="w-3 h-3 inline mr-1" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
                      <path stroke-linecap="round" stroke-linejoin="round" d="M11.25 11.25l.041-.02a.75.75 0 011.063.852l-.708 2.836a.75.75 0 001.063.853l.041-.021M21 12a9 9 0 11-18 0 9 9 0 0118 0zm-9-3.75h.008v.008H12V8.25z" />
                    </svg>
                    {{ oauthAuthCodeHint }}
                  </p>
                </div>

                <!-- Error Message -->
                <div
                  v-if="error"
                  class="mt-3 rounded-lg border border-red-200 bg-red-50 dark:border-red-700 dark:bg-red-900/30 p-3"
                >
                  <p class="text-sm text-red-600 dark:text-red-400 whitespace-pre-line">
                    {{ error }}
                  </p>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useClipboard } from '@/composables/useClipboard'
import type { AddMethod, AuthInputMethod } from '@/composables/useAccountOAuth'

interface Props {
  addMethod: AddMethod
  authUrl?: string
  sessionId?: string
  loading?: boolean
  error?: string
  showHelp?: boolean
  showProxyWarning?: boolean
  allowMultiple?: boolean
  methodLabel?: string
  showCookieOption?: boolean // Whether to show cookie auto-auth option
  platform?: 'anthropic' | 'openai' // Platform type for different UI/text
}

const props = withDefaults(defineProps<Props>(), {
  authUrl: '',
  sessionId: '',
  loading: false,
  error: '',
  showHelp: true,
  showProxyWarning: true,
  allowMultiple: false,
  methodLabel: 'Authorization Method',
  showCookieOption: true,
  platform: 'anthropic'
})

const emit = defineEmits<{
  'generate-url': []
  'exchange-code': [code: string]
  'cookie-auth': [sessionKey: string]
  'update:inputMethod': [method: AuthInputMethod]
}>()

const { t } = useI18n()

// Platform-specific translation helpers
const isOpenAI = computed(() => props.platform === 'openai')

// Get translation key based on platform
const getOAuthKey = (key: string) => {
  if (isOpenAI.value) {
    // Try OpenAI-specific key first
    const openaiKey = `admin.accounts.oauth.openai.${key}`
    return openaiKey
  }
  return `admin.accounts.oauth.${key}`
}

// Computed translations for current platform
const oauthTitle = computed(() => t(getOAuthKey('title')))
const oauthFollowSteps = computed(() => t(getOAuthKey('followSteps')))
const oauthStep1GenerateUrl = computed(() => t(getOAuthKey('step1GenerateUrl')))
const oauthGenerateAuthUrl = computed(() => t(getOAuthKey('generateAuthUrl')))
const oauthStep2OpenUrl = computed(() => t(getOAuthKey('step2OpenUrl')))
const oauthOpenUrlDesc = computed(() => t(getOAuthKey('openUrlDesc')))
const oauthStep3EnterCode = computed(() => t(getOAuthKey('step3EnterCode')))
const oauthAuthCodeDesc = computed(() => t(getOAuthKey('authCodeDesc')))
const oauthAuthCode = computed(() => t(getOAuthKey('authCode')))
const oauthAuthCodePlaceholder = computed(() => t(getOAuthKey('authCodePlaceholder')))
const oauthAuthCodeHint = computed(() => t(getOAuthKey('authCodeHint')))
const oauthImportantNotice = computed(() => isOpenAI.value ? t('admin.accounts.oauth.openai.importantNotice') : '')

// Local state
const inputMethod = ref<AuthInputMethod>(props.showCookieOption ? 'manual' : 'manual')
const authCodeInput = ref('')
const sessionKeyInput = ref('')
const showHelpDialog = ref(false)

// Clipboard
const { copied, copyToClipboard } = useClipboard()

// Computed
const parsedKeyCount = computed(() => {
  return sessionKeyInput.value.split('\n').map(k => k.trim()).filter(k => k).length
})

// Watchers
watch(inputMethod, (newVal) => {
  emit('update:inputMethod', newVal)
})

// Auto-extract code from OpenAI callback URL
// e.g., http://localhost:1455/auth/callback?code=ac_xxx...&scope=...&state=...
watch(authCodeInput, (newVal) => {
  if (!isOpenAI.value) return

  const trimmed = newVal.trim()
  // Check if it looks like a URL with code parameter
  if (trimmed.includes('?') && trimmed.includes('code=')) {
    try {
      // Try to parse as URL
      const url = new URL(trimmed)
      const code = url.searchParams.get('code')
      if (code && code !== trimmed) {
        // Replace the input with just the code
        authCodeInput.value = code
      }
    } catch {
      // If URL parsing fails, try regex extraction
      const match = trimmed.match(/[?&]code=([^&]+)/)
      if (match && match[1] && match[1] !== trimmed) {
        authCodeInput.value = match[1]
      }
    }
  }
})

// Methods
const handleGenerateUrl = () => {
  emit('generate-url')
}

const handleCopyUrl = () => {
  if (props.authUrl) {
    copyToClipboard(props.authUrl, 'URL copied to clipboard')
  }
}

const handleRegenerate = () => {
  authCodeInput.value = ''
  emit('generate-url')
}

const handleCookieAuth = () => {
  if (sessionKeyInput.value.trim()) {
    emit('cookie-auth', sessionKeyInput.value)
  }
}

// Expose methods and state
defineExpose({
  authCode: authCodeInput,
  sessionKey: sessionKeyInput,
  inputMethod,
  reset: () => {
    authCodeInput.value = ''
    sessionKeyInput.value = ''
    inputMethod.value = 'manual'
    showHelpDialog.value = false
  }
})
</script>
