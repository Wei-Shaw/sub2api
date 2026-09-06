<template>
  <section class="card" data-testid="gateway-runtime-settings-card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t('admin.settings.gatewayRuntime.title') }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('admin.settings.gatewayRuntime.description') }}
      </p>
    </div>

    <div class="space-y-5 p-6">
      <div v-if="loading" class="flex items-center gap-2 text-sm text-gray-500" aria-live="polite">
        <span class="h-4 w-4 animate-spin rounded-full border-b-2 border-primary-600" />
        {{ t('common.loading') }}
      </div>

      <div
        v-else-if="loadError"
        class="flex flex-wrap items-center justify-between gap-3 rounded border border-red-200 bg-red-50 px-4 py-3 dark:border-red-800 dark:bg-red-900/20"
        role="alert"
      >
        <span class="text-sm text-red-700 dark:text-red-300">{{ loadError }}</span>
        <button type="button" class="btn btn-secondary btn-sm" data-testid="gateway-runtime-retry" @click="loadSettings">
          {{ t('admin.settings.gatewayRuntime.retry') }}
        </button>
      </div>

      <template v-else>
        <div>
          <label for="gateway-runtime-isolation" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t('admin.settings.gatewayRuntime.connectionPoolIsolation') }}
          </label>
          <select
            id="gateway-runtime-isolation"
            v-model="form.connection_pool_isolation"
            class="input max-w-md"
            data-testid="gateway-runtime-isolation"
          >
            <option value="account_proxy">{{ t('admin.settings.gatewayRuntime.isolationAccountProxy') }}</option>
            <option value="account">{{ t('admin.settings.gatewayRuntime.isolationAccount') }}</option>
            <option value="proxy" :disabled="strictIsolationActive">
              {{ t('admin.settings.gatewayRuntime.isolationProxy') }}
            </option>
          </select>
          <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.gatewayRuntime.connectionPoolIsolationHint') }}
          </p>
        </div>

        <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
          <div class="flex items-center justify-between gap-4">
            <div>
              <label id="gateway-runtime-privacy-label" class="font-medium text-gray-900 dark:text-white">
                {{ t('admin.settings.gatewayRuntime.outboundPrivacy') }}
              </label>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.gatewayRuntime.outboundPrivacyHint') }}
              </p>
            </div>
            <Toggle
              v-model="form.outbound_privacy.enabled"
              aria-labelledby="gateway-runtime-privacy-label"
              data-testid="gateway-runtime-privacy-enabled"
            />
          </div>

          <div class="mt-4 flex items-center justify-between gap-4">
            <div>
              <label id="gateway-runtime-strict-label" class="font-medium text-gray-900 dark:text-white">
                {{ t('admin.settings.gatewayRuntime.strictAccountIsolation') }}
              </label>
              <p class="text-sm text-gray-500 dark:text-gray-400">
                {{ t('admin.settings.gatewayRuntime.strictAccountIsolationHint') }}
              </p>
            </div>
            <Toggle
              v-model="form.outbound_privacy.strict_account_isolation"
              :disabled="!form.outbound_privacy.enabled"
              aria-labelledby="gateway-runtime-strict-label"
              data-testid="gateway-runtime-strict-isolation"
            />
          </div>

          <div class="mt-4">
            <label for="gateway-runtime-preserve-headers" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.settings.gatewayRuntime.preserveHeaders') }}
            </label>
            <textarea
              id="gateway-runtime-preserve-headers"
              v-model="preserveHeadersInput"
              rows="3"
              class="input font-mono text-sm"
              :disabled="!form.outbound_privacy.enabled"
              :placeholder="t('admin.settings.gatewayRuntime.preserveHeadersPlaceholder')"
              data-testid="gateway-runtime-preserve-headers"
            />
            <p class="mt-1.5 text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.settings.gatewayRuntime.preserveHeadersHint') }}
            </p>
          </div>
        </div>

        <div class="border-t border-gray-100 pt-5 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
            {{ t('admin.settings.gatewayRuntime.openAIWSBudget') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.settings.gatewayRuntime.openAIWSBudgetHint') }}
          </p>
          <div class="mt-4 grid grid-cols-1 gap-4 sm:grid-cols-3">
            <div>
              <label for="gateway-runtime-ws-max" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.gatewayRuntime.maxConnections') }}
              </label>
              <input
                id="gateway-runtime-ws-max"
                v-model.number="form.openai_ws.max_conns_per_account"
                type="number"
                min="1"
                step="1"
                class="input"
                data-testid="gateway-runtime-ws-max"
              />
            </div>
            <div>
              <label for="gateway-runtime-ws-min-idle" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.gatewayRuntime.minIdle') }}
              </label>
              <input
                id="gateway-runtime-ws-min-idle"
                v-model.number="form.openai_ws.min_idle_per_account"
                type="number"
                min="0"
                step="1"
                class="input"
                data-testid="gateway-runtime-ws-min-idle"
              />
            </div>
            <div>
              <label for="gateway-runtime-ws-max-idle" class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">
                {{ t('admin.settings.gatewayRuntime.maxIdle') }}
              </label>
              <input
                id="gateway-runtime-ws-max-idle"
                v-model.number="form.openai_ws.max_idle_per_account"
                type="number"
                min="0"
                step="1"
                class="input"
                data-testid="gateway-runtime-ws-max-idle"
              />
            </div>
          </div>
          <p v-if="validationError" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">
            {{ validationError }}
          </p>
        </div>

        <div class="flex justify-end border-t border-gray-100 pt-5 dark:border-dark-700">
          <button
            type="button"
            class="btn btn-primary"
            :disabled="saving"
            data-testid="gateway-runtime-save"
            @click="saveSettings"
          >
            {{ saving ? t('common.saving') : t('admin.settings.gatewayRuntime.apply') }}
          </button>
        </div>
      </template>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import type { GatewayRuntimeSettings } from '@/api/admin/gatewayRuntime'
import Toggle from '@/components/common/Toggle.vue'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const loading = ref(true)
const saving = ref(false)
const loadError = ref('')
const preserveHeadersInput = ref('')

const form = reactive<GatewayRuntimeSettings>({
  connection_pool_isolation: 'account_proxy',
  outbound_privacy: {
    enabled: true,
    strict_account_isolation: true,
    preserve_headers: []
  },
  openai_ws: {
    max_conns_per_account: 128,
    min_idle_per_account: 4,
    max_idle_per_account: 12
  }
})

const strictIsolationActive = computed(
  () => form.outbound_privacy.enabled && form.outbound_privacy.strict_account_isolation
)

function parsePreserveHeaders(value: string): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const token of value.split(/[\n,]+/)) {
    const header = token.trim()
    const key = header.toLowerCase()
    if (!header || seen.has(key)) continue
    seen.add(key)
    result.push(header)
  }
  return result
}

function applySettings(settings: GatewayRuntimeSettings): void {
  form.connection_pool_isolation = settings.connection_pool_isolation
  form.outbound_privacy.enabled = settings.outbound_privacy.enabled
  form.outbound_privacy.strict_account_isolation = settings.outbound_privacy.strict_account_isolation
  form.outbound_privacy.preserve_headers = [...(settings.outbound_privacy.preserve_headers || [])]
  form.openai_ws.max_conns_per_account = settings.openai_ws.max_conns_per_account
  form.openai_ws.min_idle_per_account = settings.openai_ws.min_idle_per_account
  form.openai_ws.max_idle_per_account = settings.openai_ws.max_idle_per_account
  preserveHeadersInput.value = form.outbound_privacy.preserve_headers.join(', ')
}

const validationError = computed(() => {
  const maxConnections = Number(form.openai_ws.max_conns_per_account)
  const minIdle = Number(form.openai_ws.min_idle_per_account)
  const maxIdle = Number(form.openai_ws.max_idle_per_account)
  if (![maxConnections, minIdle, maxIdle].every(Number.isInteger)) {
    return t('admin.settings.gatewayRuntime.integerRequired')
  }
  if (maxConnections <= 0 || minIdle < 0 || maxIdle < 0) {
    return t('admin.settings.gatewayRuntime.nonNegativeBudget')
  }
  if (minIdle > maxIdle) {
    return t('admin.settings.gatewayRuntime.minIdleExceedsMaxIdle')
  }
  if (maxIdle > maxConnections) {
    return t('admin.settings.gatewayRuntime.maxIdleExceedsMaxConnections')
  }
  return ''
})

async function loadSettings(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    applySettings(await adminAPI.gatewayRuntime.getSettings())
  } catch (error) {
    loadError.value = extractApiErrorMessage(error, t('admin.settings.gatewayRuntime.loadFailed'))
  } finally {
    loading.value = false
  }
}

async function saveSettings(): Promise<void> {
  if (validationError.value) {
    appStore.showError(validationError.value)
    return
  }
  saving.value = true
  try {
    const payload: GatewayRuntimeSettings = {
      connection_pool_isolation: strictIsolationActive.value && form.connection_pool_isolation === 'proxy'
        ? 'account_proxy'
        : form.connection_pool_isolation,
      outbound_privacy: {
        enabled: form.outbound_privacy.enabled,
        strict_account_isolation: form.outbound_privacy.strict_account_isolation,
        preserve_headers: parsePreserveHeaders(preserveHeadersInput.value)
      },
      openai_ws: {
        max_conns_per_account: Number(form.openai_ws.max_conns_per_account),
        min_idle_per_account: Number(form.openai_ws.min_idle_per_account),
        max_idle_per_account: Number(form.openai_ws.max_idle_per_account)
      }
    }
    applySettings(await adminAPI.gatewayRuntime.updateSettings(payload))
    appStore.showSuccess(t('admin.settings.gatewayRuntime.saved'))
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('admin.settings.gatewayRuntime.saveFailed')))
  } finally {
    saving.value = false
  }
}

onMounted(loadSettings)
</script>
