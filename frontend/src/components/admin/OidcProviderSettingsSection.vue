<template>
  <div class="space-y-6">
    <div>
      <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">
        {{ t('oidc.admin.settings.title') }}
      </h3>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t('oidc.admin.settings.description') }}
      </p>
    </div>

    <div v-if="loading" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('common.loading') }}
    </div>

    <template v-else>
      <!-- 主开关 -->
      <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
        <label class="flex items-start gap-3">
          <input
            type="checkbox"
            class="mt-1 h-4 w-4"
            :checked="form.enabled"
            @change="onToggleEnabled"
          />
          <span>
            <span class="block text-sm font-medium text-gray-900 dark:text-gray-100">
              {{ t('oidc.admin.settings.enabled') }}
            </span>
            <span class="block text-xs text-gray-500 dark:text-gray-400">
              {{ t('oidc.admin.settings.enabledHint') }}
            </span>
          </span>
        </label>
      </div>

      <!-- Issuer URL -->
      <div>
        <label class="form-label">{{ t('oidc.admin.settings.issuerUrl') }}</label>
        <input
          v-model="form.issuer_url"
          type="text"
          class="input"
          :placeholder="t('oidc.admin.settings.issuerUrlPlaceholder')"
        />
        <p class="mt-1 text-xs" :class="issuerUrlError ? 'text-red-600 dark:text-red-400' : 'text-gray-500 dark:text-gray-400'">
          {{ issuerUrlError || t('oidc.admin.settings.issuerUrlHint') }}
        </p>
      </div>

      <!-- TTLs -->
      <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
        <div>
          <label class="form-label">{{ t('oidc.admin.settings.accessTokenTtl') }}</label>
          <input v-model.number="form.access_token_ttl_seconds" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="form-label">{{ t('oidc.admin.settings.idTokenTtl') }}</label>
          <input v-model.number="form.id_token_ttl_seconds" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="form-label">{{ t('oidc.admin.settings.refreshTokenTtl') }}</label>
          <input v-model.number="form.refresh_token_ttl_seconds" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="form-label">{{ t('oidc.admin.settings.codeTtl') }}</label>
          <input v-model.number="form.code_ttl_seconds" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="form-label">{{ t('oidc.admin.settings.ssoCookieMaxAge') }}</label>
          <input v-model.number="form.sso_cookie_max_age_seconds" type="number" min="1" class="input" />
        </div>
        <div>
          <label class="form-label">{{ t('oidc.admin.settings.ssoCookieDomain') }}</label>
          <input
            v-model="form.sso_cookie_domain"
            type="text"
            class="input"
            :placeholder="t('oidc.admin.settings.ssoCookieDomainPlaceholder')"
          />
        </div>
      </div>

      <div class="flex justify-end">
        <button type="button" class="btn btn-primary" :disabled="saving" @click="onSaveClicked">
          {{ saving ? t('common.saving') : t('oidc.admin.settings.save') }}
        </button>
      </div>

      <!-- 签名密钥 -->
      <div class="border-t border-gray-200 pt-6 dark:border-dark-700">
        <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t('oidc.admin.signingKeys.title') }}
        </h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('oidc.admin.signingKeys.description') }}
        </p>

        <div class="mt-3 flex justify-end">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="keysLoading" @click="askRotate">
            {{ t('oidc.admin.signingKeys.rotate') }}
          </button>
        </div>

        <p v-if="signingKeys.length === 0" class="mt-3 text-sm text-gray-500 dark:text-gray-400">
          {{ t('oidc.admin.signingKeys.empty') }}
        </p>
        <table v-else class="mt-3 w-full text-sm">
          <thead>
            <tr class="border-b border-gray-200 text-left text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
              <th class="py-2">{{ t('oidc.admin.signingKeys.kid') }}</th>
              <th class="py-2">{{ t('oidc.admin.signingKeys.status') }}</th>
              <th class="py-2">{{ t('oidc.admin.signingKeys.createdAt') }}</th>
              <th class="py-2">{{ t('oidc.admin.signingKeys.retiredAt') }}</th>
              <th class="py-2"></th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="k in signingKeys" :key="k.kid" class="border-b border-gray-100 dark:border-dark-800">
              <td class="py-2"><code class="text-xs">{{ k.kid }}</code></td>
              <td class="py-2">
                <span :class="['badge', k.is_active ? 'badge-success' : 'badge-default']">
                  {{ k.is_active ? t('oidc.admin.signingKeys.active') : t('oidc.admin.signingKeys.retired') }}
                </span>
              </td>
              <td class="py-2 text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(k.created_at) }}</td>
              <td class="py-2 text-xs text-gray-500 dark:text-gray-400">
                {{ k.retired_at ? formatDateTime(k.retired_at) : '—' }}
              </td>
              <td class="py-2 text-right">
                <button
                  v-if="k.removable"
                  type="button"
                  class="btn btn-danger btn-xs"
                  @click="askDeleteKey(k.kid)"
                >
                  {{ t('oidc.admin.signingKeys.delete') }}
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- 第三方接入帮助 (任务 11.1) -->
      <div class="border-t border-gray-200 pt-6 dark:border-dark-700">
        <h3 class="text-base font-semibold text-gray-900 dark:text-gray-100">
          {{ t('oidc.admin.help.title') }}
        </h3>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
          {{ t('oidc.admin.help.intro') }}
        </p>

        <dl class="mt-4 space-y-3 text-sm">
          <div>
            <dt class="font-medium text-gray-900 dark:text-gray-100">
              {{ t('oidc.admin.help.discoveryLabel') }}
            </dt>
            <dd class="mt-1">
              <code v-if="discoveryUrl" class="break-all text-xs">{{ discoveryUrl }}</code>
              <span v-else class="text-xs text-gray-400">{{ t('oidc.admin.help.discoveryNeedIssuer') }}</span>
            </dd>
          </div>
          <div>
            <dt class="font-medium text-gray-900 dark:text-gray-100">
              {{ t('oidc.admin.help.jwksLabel') }}
            </dt>
            <dd class="mt-1">
              <code v-if="jwksUrl" class="break-all text-xs">{{ jwksUrl }}</code>
              <span v-else class="text-xs text-gray-400">{{ t('oidc.admin.help.discoveryNeedIssuer') }}</span>
            </dd>
          </div>
          <div>
            <dt class="font-medium text-gray-900 dark:text-gray-100">
              {{ t('oidc.admin.help.scopesLabel') }}
            </dt>
            <dd class="mt-1 flex flex-wrap gap-1.5">
              <code v-for="s in allowedScopes" :key="s" class="rounded bg-gray-100 px-1.5 py-0.5 text-xs dark:bg-dark-800">{{ s }}</code>
            </dd>
          </div>
          <div>
            <dt class="font-medium text-gray-900 dark:text-gray-100">
              {{ t('oidc.admin.help.redirectLabel') }}
            </dt>
            <dd class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('oidc.admin.help.redirectBody') }}</dd>
          </div>
          <div>
            <dt class="font-medium text-gray-900 dark:text-gray-100">
              {{ t('oidc.admin.help.pkceLabel') }}
            </dt>
            <dd class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('oidc.admin.help.pkceBody') }}</dd>
          </div>
        </dl>
      </div>
    </template>

    <!-- 启用确认 -->
    <ConfirmDialog
      :show="showEnableConfirm"
      :title="t('oidc.admin.settings.enableConfirm.title')"
      :message="t('oidc.admin.settings.enableConfirm.body')"
      :confirm-text="t('oidc.admin.settings.enableConfirm.confirm')"
      :cancel-text="t('oidc.admin.settings.enableConfirm.cancel')"
      @confirm="confirmEnable"
      @cancel="cancelEnable"
    />

    <!-- 轮换确认 -->
    <ConfirmDialog
      :show="showRotateConfirm"
      :title="t('oidc.admin.signingKeys.rotateConfirm.title')"
      :message="t('oidc.admin.signingKeys.rotateConfirm.body')"
      :confirm-text="t('oidc.admin.signingKeys.rotateConfirm.confirm')"
      :cancel-text="t('oidc.admin.signingKeys.rotateConfirm.cancel')"
      @confirm="confirmRotate"
      @cancel="showRotateConfirm = false"
    />

    <!-- 删除密钥确认 -->
    <ConfirmDialog
      :show="showDeleteKeyConfirm"
      :title="t('oidc.admin.signingKeys.deleteConfirm.title')"
      :message="t('oidc.admin.signingKeys.deleteConfirm.body')"
      :confirm-text="t('oidc.admin.signingKeys.deleteConfirm.confirm')"
      :cancel-text="t('oidc.admin.signingKeys.deleteConfirm.cancel')"
      danger
      @confirm="confirmDeleteKey"
      @cancel="showDeleteKeyConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import {
  getProviderSettings,
  updateProviderSettings,
  listSigningKeys,
  rotateSigningKey,
  deleteSigningKey,
  OIDC_ALLOWED_SCOPES,
  type OidcProviderSettings,
  type OidcSigningKey
} from '@/api/admin/oidcClients'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)

const form = reactive<OidcProviderSettings>({
  enabled: false,
  issuer_url: '',
  access_token_ttl_seconds: 3600,
  id_token_ttl_seconds: 3600,
  refresh_token_ttl_seconds: 2592000,
  code_ttl_seconds: 600,
  sso_cookie_max_age_seconds: 2592000,
  sso_cookie_domain: ''
})

const allowedScopes = OIDC_ALLOWED_SCOPES

// 由 issuer_url 推导第三方接入所需的 discovery / jwks 端点（仅在 issuer 合法时展示）。
const discoveryUrl = computed(() => {
  const v = form.issuer_url.trim()
  if (!v || issuerUrlError.value) return ''
  return `${v}/.well-known/openid-configuration`
})
const jwksUrl = computed(() => {
  const v = form.issuer_url.trim()
  if (!v || issuerUrlError.value) return ''
  return `${v}/.well-known/jwks.json`
})

// 前端轻量校验（最终以后端 ValidateOidcIssuerURL 为准）。
const issuerUrlError = computed(() => {
  const v = form.issuer_url.trim()
  if (!v) return ''
  if (!v.startsWith('https://')) return t('oidc.admin.settings.issuerUrlHint')
  if (v.endsWith('/')) return t('oidc.admin.settings.issuerUrlHint')
  if (v.includes('?') || v.includes('#')) return t('oidc.admin.settings.issuerUrlHint')
  return ''
})

async function load() {
  loading.value = true
  try {
    const s = await getProviderSettings()
    Object.assign(form, s)
    await loadKeys()
  } catch {
    appStore.showError(t('oidc.admin.settings.loadFailed'))
  } finally {
    loading.value = false
  }
}

// ---- 启用确认门控 ----
const showEnableConfirm = ref(false)

function onToggleEnabled(e: Event) {
  const checked = (e.target as HTMLInputElement).checked
  if (checked && !form.enabled) {
    // 开启前确认
    showEnableConfirm.value = true
    // 先回退视觉状态，待确认后再置 true
    ;(e.target as HTMLInputElement).checked = false
    return
  }
  form.enabled = checked
}

function confirmEnable() {
  form.enabled = true
  showEnableConfirm.value = false
}

function cancelEnable() {
  form.enabled = false
  showEnableConfirm.value = false
}

async function onSaveClicked() {
  if (form.issuer_url.trim() && issuerUrlError.value) {
    appStore.showError(issuerUrlError.value)
    return
  }
  saving.value = true
  try {
    const updated = await updateProviderSettings({
      enabled: form.enabled,
      issuer_url: form.issuer_url.trim(),
      access_token_ttl_seconds: form.access_token_ttl_seconds,
      id_token_ttl_seconds: form.id_token_ttl_seconds,
      refresh_token_ttl_seconds: form.refresh_token_ttl_seconds,
      code_ttl_seconds: form.code_ttl_seconds,
      sso_cookie_max_age_seconds: form.sso_cookie_max_age_seconds,
      sso_cookie_domain: form.sso_cookie_domain.trim()
    })
    Object.assign(form, updated)
    appStore.showSuccess(t('oidc.admin.settings.saveSuccess'))
    await loadKeys()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('oidc.admin.settings.saveFailed'))
  } finally {
    saving.value = false
  }
}

// ---- 签名密钥 ----
const signingKeys = ref<OidcSigningKey[]>([])
const keysLoading = ref(false)

async function loadKeys() {
  keysLoading.value = true
  try {
    signingKeys.value = (await listSigningKeys()) ?? []
  } catch {
    // 未启用时可能没有密钥，静默处理（不弹错避免干扰）。
    signingKeys.value = []
  } finally {
    keysLoading.value = false
  }
}

const showRotateConfirm = ref(false)
function askRotate() {
  showRotateConfirm.value = true
}
async function confirmRotate() {
  showRotateConfirm.value = false
  try {
    await rotateSigningKey()
    appStore.showSuccess(t('oidc.admin.signingKeys.rotateSuccess'))
    await loadKeys()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('oidc.admin.signingKeys.rotateFailed'))
  }
}

const showDeleteKeyConfirm = ref(false)
const deletingKid = ref('')
function askDeleteKey(kid: string) {
  deletingKid.value = kid
  showDeleteKeyConfirm.value = true
}
async function confirmDeleteKey() {
  showDeleteKeyConfirm.value = false
  if (!deletingKid.value) return
  try {
    await deleteSigningKey(deletingKid.value)
    await loadKeys()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('oidc.admin.signingKeys.deleteFailed'))
  } finally {
    deletingKid.value = ''
  }
}

onMounted(load)
</script>
