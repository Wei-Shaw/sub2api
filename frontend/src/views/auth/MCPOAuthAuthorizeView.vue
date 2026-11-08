<template>
  <main class="min-h-screen bg-gray-50 px-4 py-8 dark:bg-dark-950 sm:py-12">
    <section class="card mx-auto max-w-2xl p-5 sm:p-8" aria-labelledby="mcp-consent-title" :aria-busy="loading || submitting">
      <div class="mb-6 flex items-start gap-3">
        <div class="rounded-xl bg-primary-50 p-3 text-primary-600 dark:bg-primary-900/20 dark:text-primary-400"><Icon name="mcp" size="lg" /></div>
        <div>
          <h1 id="mcp-consent-title" class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('mcp.consentTitle') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('mcp.consentDescription') }}</p>
        </div>
      </div>
      <p v-if="loading" role="status" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('mcp.consentLoading') }}</p>
      <div v-else-if="previewError" class="space-y-4">
        <p role="alert" class="text-sm text-red-600 dark:text-red-400">{{ previewError }}</p>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('mcp.consentRetryHint') }}</p>
        <RouterLink to="/settings/mcp" class="btn btn-secondary">{{ t('mcp.backToConnections') }}</RouterLink>
      </div>
      <form v-else-if="consent" class="space-y-6" @submit.prevent="decide(true)">
        <div class="rounded-xl border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900">
          <dl class="space-y-4 text-sm">
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('mcp.applicationName') }}</dt>
              <dd data-testid="client-name" class="mt-1 break-all text-lg font-semibold text-gray-900 dark:text-white">{{ consent.client_name }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('mcp.clientId') }}</dt>
              <dd class="mt-1 truncate font-mono text-xs text-gray-700 dark:text-gray-200" :title="consent.client_id">{{ consent.client_id }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('mcp.returnAddress') }}</dt>
              <dd data-testid="redirect-origin" class="mt-1 break-all font-semibold text-gray-900 dark:text-white">{{ redirectOrigin }}</dd>
              <dd data-testid="redirect-uri" class="mt-1 break-all font-mono text-xs text-gray-600 dark:text-gray-300">{{ consent.redirect_uri }}</dd>
            </div>
            <div>
              <dt class="text-gray-500 dark:text-gray-400">{{ t('mcp.requestedAccess') }}</dt>
              <dd class="mt-1 text-gray-700 dark:text-gray-200">{{ t('mcp.requestedAccessDescription') }}</dd>
              <dd class="mt-1 break-all font-mono text-xs text-gray-600 dark:text-gray-300">{{ consent.resource }}</dd>
            </div>
          </dl>
          <p class="mt-4 border-t border-gray-200 pt-4 text-sm text-amber-800 dark:border-dark-700 dark:text-amber-300">{{ t('mcp.untrustedApplicationHint') }}</p>
        </div>
        <p v-if="authStore.user?.email" class="break-all text-sm text-gray-600 dark:text-gray-300">{{ t('mcp.signedInAs', { email: authStore.user.email }) }}</p>
        <div>
          <label for="mcp-consent-scope" class="input-label">{{ t('mcp.scope') }}</label>
          <select id="mcp-consent-scope" v-model="scope" class="input" :disabled="submitting" aria-describedby="mcp-scope-hint">
            <option value="user">{{ t('mcp.userScope') }}</option>
            <option v-if="authStore.isAdmin" value="admin">{{ t('mcp.adminScope') }}</option>
          </select>
          <p id="mcp-scope-hint" class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ scope === 'admin' ? t('mcp.adminScopeHint') : t('mcp.userScopeHint') }}</p>
        </div>
        <div>
          <label for="mcp-consent-permissions" class="input-label">{{ t('mcp.permissions') }}</label>
          <select id="mcp-consent-permissions" v-model="readOnly" class="input" :disabled="submitting" aria-describedby="mcp-permissions-hint">
            <option :value="true">{{ t('mcp.readOnly') }}</option>
            <option :value="false">{{ t('mcp.readWrite') }}</option>
          </select>
          <p id="mcp-permissions-hint" class="mt-2 text-xs" :class="readOnly ? 'text-gray-500 dark:text-gray-400' : 'text-amber-700 dark:text-amber-400'">{{ readOnly ? t('mcp.readOnlyHint') : t('mcp.readWriteHint') }}</p>
          <p v-if="scope === 'admin' && !readOnly" data-testid="admin-write-hint" class="mt-1 text-xs text-amber-700 dark:text-amber-400">{{ t('mcp.adminWriteHint') }}</p>
        </div>
        <p v-if="submitError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ submitError }}</p>
        <div class="flex justify-end gap-3 border-t border-gray-100 pt-5 dark:border-dark-700">
          <button data-testid="deny" type="button" class="btn btn-secondary" :disabled="submitting" @click="decide(false)">{{ t('mcp.deny') }}</button>
          <button data-testid="approve" type="submit" class="btn btn-primary" :disabled="submitting">{{ submitting ? t('common.processing') : t('mcp.approve') }}</button>
        </div>
      </form>
    </section>
    <TotpStepUpDialog :controller="stepUp" />
  </main>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { mcpAPI, type MCPAccessScope, type MCPAuthorization, type MCPConsent } from '@/api/mcp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import { Icon } from '@/components/icons'
import { isStepUpBlocked, isStepUpCancelled, stepUpBlockReason, useStepUp } from '@/composables/useStepUp'
import { useAuthStore } from '@/stores/auth'

const { t } = useI18n()
const route = useRoute()
const authStore = useAuthStore()
const stepUp = useStepUp()
const loading = ref(true)
const submitting = ref(false)
const previewError = ref('')
const submitError = ref('')
const consent = ref<MCPConsent | null>(null)
const scope = ref<MCPAccessScope>('user')
const readOnly = ref(true)
let authorization: MCPAuthorization | null = null
let generation = 0
const redirectOrigin = computed(() => consent.value ? new URL(consent.value.redirect_uri).origin : '')

function errorMessage(error: unknown, fallback: string): string {
  const message = (error as { message?: unknown } | null)?.message
  return typeof message === 'string' && message ? message : fallback
}

function parseAuthorization(): MCPAuthorization {
  const required = ['response_type', 'client_id', 'redirect_uri', 'code_challenge', 'code_challenge_method', 'resource'] as const
  const allowed = [...required, 'scope', 'state']
  const values: Record<string, string> = {}
  for (const [key, value] of Object.entries(route.query)) {
    if (!allowed.includes(key) || typeof value !== 'string' || value.length > 4096) {
      throw new Error(t('mcp.invalidRequest'))
    }
    values[key] = value
  }
  if (required.some(key => !values[key]) || values.response_type !== 'code' ||
    values.code_challenge_method !== 'S256' || !/^[A-Za-z0-9_-]{43}$/.test(values.code_challenge) ||
    (values.scope !== undefined && values.scope !== '' && values.scope !== 'mcp')) {
    throw new Error(t('mcp.invalidRequest'))
  }
  return values as unknown as MCPAuthorization
}

function validatePreview(result: MCPConsent, request: MCPAuthorization): void {
  if (result.client_id !== request.client_id || result.redirect_uri !== request.redirect_uri ||
    result.resource !== request.resource || result.scope !== 'mcp' || typeof result.client_name !== 'string') {
    throw new Error(t('mcp.invalidRequest'))
  }
  const redirect = new URL(result.redirect_uri)
  if (!['https:', 'http:'].includes(redirect.protocol) || redirect.username || redirect.password || redirect.hash) {
    throw new Error(t('mcp.invalidRequest'))
  }
}

async function loadConsent(): Promise<void> {
  const current = ++generation
  stepUp.onCancel()
  loading.value = true
  submitting.value = false
  previewError.value = ''
  submitError.value = ''
  consent.value = null
  authorization = null
  scope.value = 'user'
  readOnly.value = true
  try {
    const request = parseAuthorization()
    const result = await mcpAPI.previewAuthorization({ ...request })
    if (current !== generation) return
    validatePreview(result, request)
    authorization = Object.freeze({ ...request })
    consent.value = result
  } catch (error) {
    if (current === generation) previewError.value = errorMessage(error, t('mcp.consentLoadFailed'))
  } finally {
    if (current === generation) loading.value = false
  }
}

async function decide(approved: boolean): Promise<void> {
  if (loading.value || submitting.value || !consent.value || !authorization) return
  const current = generation
  const expectedRedirect = new URL(consent.value.redirect_uri)
  const input = {
    authorization: { ...authorization },
    approved,
    scope: approved && authStore.isAdmin ? scope.value : 'user' as const,
    read_only: approved ? readOnly.value : true
  }
  submitting.value = true
  submitError.value = ''
  try {
    const action = async () => {
      // A changed route or unmounted page must not retry a previous consent.
      if (current !== generation) throw new Error(t('mcp.invalidRequest'))
      try {
        return await mcpAPI.authorize(input)
      } catch (error) {
        // Do not open a step-up prompt for a request that is no longer shown.
        if (current !== generation) throw new Error(t('mcp.invalidRequest'))
        throw error
      }
    }
    const result = approved && input.scope === 'admin' ? await stepUp.run(action) : await action()
    if (current !== generation) return
    const destination = new URL(result.redirect_url)
    if (destination.origin !== expectedRedirect.origin || destination.pathname !== expectedRedirect.pathname ||
      destination.username || destination.password || destination.hash) {
      throw new Error(t('mcp.invalidRedirect'))
    }
    window.location.assign(result.redirect_url)
  } catch (error) {
    if (current !== generation) return
    if (!isStepUpCancelled(error)) {
      submitError.value = isStepUpBlocked(error)
        ? stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN' ? t('stepUp.adminApiKeyForbidden') : t('stepUp.notEnabled')
        : errorMessage(error, t('mcp.consentSubmitFailed'))
    }
    submitting.value = false
  }
}

watch(() => route.fullPath, loadConsent, { immediate: true })
onBeforeUnmount(() => {
  generation++
  authorization = null
  stepUp.onCancel()
})
</script>
