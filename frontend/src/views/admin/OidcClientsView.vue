<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ t('oidc.admin.title') }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('oidc.admin.description') }}</span>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadList" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              {{ t('oidc.admin.createButton') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="rows" :loading="loading">
          <template #cell-client_name="{ row }">
            <span class="font-medium text-gray-900 dark:text-gray-100">{{ row.client_name }}</span>
          </template>

          <template #cell-client_id="{ row }">
            <code class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ row.client_id }}</code>
          </template>

          <template #cell-allowed_scopes="{ row }">
            <div class="flex flex-wrap gap-1">
              <span
                v-for="s in row.allowed_scopes"
                :key="s"
                class="rounded px-1.5 py-0.5 text-xs"
                :class="isSensitiveScope(s)
                  ? 'bg-red-100 text-red-700 dark:bg-red-900/40 dark:text-red-300'
                  : 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'"
              >{{ s }}</span>
            </div>
          </template>

          <template #cell-redirect_uris="{ row }">
            <span class="text-xs text-gray-600 dark:text-gray-400">
              {{ t('oidc.admin.table.uriCount', { count: row.redirect_uris.length }) }}
            </span>
          </template>

          <template #cell-enabled="{ row }">
            <span :class="['badge', row.enabled ? 'badge-success' : 'badge-default']">
              {{ row.enabled ? t('oidc.admin.status.enabled') : t('oidc.admin.status.disabled') }}
            </span>
          </template>

          <template #cell-created_at="{ value }">
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary btn-xs" @click="openEditDialog(row)">{{ t('oidc.admin.actions.edit') }}</button>
              <button class="btn btn-secondary btn-xs" @click="askResetSecret(row)">{{ t('oidc.admin.actions.resetSecret') }}</button>
              <button class="btn btn-danger btn-xs" @click="askDelete(row)">{{ t('oidc.admin.actions.delete') }}</button>
            </div>
          </template>

          <template #empty>
            <p class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('oidc.admin.empty') }}</p>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Create / Edit Dialog -->
    <BaseDialog
      :show="showFormDialog"
      :title="editingId === null ? t('oidc.admin.form.createTitle') : t('oidc.admin.form.editTitle')"
      @close="showFormDialog = false"
    >
      <div class="space-y-4">
        <div>
          <label class="form-label">{{ t('oidc.admin.form.clientName') }}</label>
          <input
            v-model="form.client_name"
            type="text"
            class="input"
            :placeholder="t('oidc.admin.form.clientNamePlaceholder')"
          />
        </div>

        <div>
          <div class="mb-2 flex items-center justify-between">
            <span class="form-label !mb-0">{{ t('oidc.admin.form.redirectUris') }}</span>
            <button class="btn btn-secondary btn-xs" @click="addRedirectUri">{{ t('oidc.admin.form.addRedirectUri') }}</button>
          </div>
          <div class="space-y-2">
            <div v-for="(_, idx) in form.redirect_uris" :key="idx" class="flex items-center gap-2">
              <input
                v-model="form.redirect_uris[idx]"
                type="text"
                class="input flex-1"
                :placeholder="t('oidc.admin.form.redirectUriPlaceholder')"
              />
              <button class="btn btn-ghost btn-xs text-red-500" @click="removeRedirectUri(idx)">
                {{ t('common.remove') }}
              </button>
            </div>
          </div>
          <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('oidc.admin.form.redirectUriHint') }}</p>
        </div>

        <div>
          <label class="form-label">{{ t('oidc.admin.form.allowedScopes') }}</label>
          <div class="flex flex-wrap gap-3">
            <label
              v-for="scope in allowedScopes"
              :key="scope"
              class="flex items-center gap-1.5 text-sm"
            >
              <input
                type="checkbox"
                class="h-4 w-4"
                :value="scope"
                :checked="form.allowed_scopes.includes(scope)"
                @change="toggleScope(scope)"
              />
              <span :class="isSensitiveScope(scope) ? 'text-red-600 dark:text-red-400' : 'text-gray-700 dark:text-gray-200'">
                {{ scope }}
              </span>
            </label>
          </div>
          <p v-if="formHasSensitiveScope" class="mt-2 text-xs font-medium text-red-600 dark:text-red-400">
            {{ t('oidc.admin.sensitiveScopeWarning') }}
          </p>
        </div>

        <div class="flex items-center gap-3">
          <input id="oidc-consent-required" v-model="form.consent_required" type="checkbox" class="h-4 w-4" />
          <label for="oidc-consent-required" class="form-label !mb-0">{{ t('oidc.admin.form.consentRequired') }}</label>
        </div>

        <div class="flex items-center gap-3">
          <input id="oidc-enabled" v-model="form.enabled" type="checkbox" class="h-4 w-4" />
          <label for="oidc-enabled" class="form-label !mb-0">{{ t('oidc.admin.form.enabled') }}</label>
        </div>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showFormDialog = false">{{ t('oidc.admin.form.cancel') }}</button>
        <button class="btn btn-primary" :disabled="submitting" @click="onSubmitClicked">
          {{ submitting ? t('common.saving') : t('oidc.admin.form.save') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Sensitive scope confirm modal -->
    <BaseDialog
      :show="showSensitiveConfirm"
      :title="t('oidc.admin.confirmModal.title')"
      @close="showSensitiveConfirm = false"
    >
      <div class="space-y-3">
        <p class="rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300">
          {{ t('oidc.admin.confirmModal.body') }}
        </p>
        <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-200">
          <input v-model="sensitiveConfirmChecked" type="checkbox" class="h-4 w-4" />
          {{ t('oidc.admin.confirmModal.checkbox') }}
        </label>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showSensitiveConfirm = false">{{ t('oidc.admin.confirmModal.cancel') }}</button>
        <button
          class="btn btn-danger"
          :disabled="!sensitiveConfirmChecked || submitting"
          @click="confirmSensitiveAndSave"
        >
          {{ t('oidc.admin.confirmModal.confirm') }}
        </button>
      </template>
    </BaseDialog>

    <!-- One-time secret reveal -->
    <BaseDialog
      :show="showSecretDialog"
      :title="t('oidc.admin.secretReveal.title')"
      :close-on-escape="false"
      @close="noopClose"
    >
      <div class="space-y-3">
        <p class="rounded-lg border border-red-300 bg-red-50 p-3 text-sm font-medium text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300">
          {{ secretBanner }}
        </p>
        <div class="flex items-center gap-2">
          <code class="flex-1 break-all rounded bg-gray-100 px-3 py-2 text-sm text-gray-800 dark:bg-dark-700 dark:text-gray-100">{{ revealedSecret }}</code>
          <button class="btn btn-secondary btn-sm" @click="copySecret">
            {{ secretCopied ? t('oidc.admin.secretReveal.copied') : t('oidc.admin.secretReveal.copy') }}
          </button>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-primary" @click="closeSecretDialog">{{ t('oidc.admin.secretReveal.done') }}</button>
      </template>
    </BaseDialog>

    <!-- Delete confirm -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('oidc.admin.deleteConfirm.title')"
      :message="t('oidc.admin.deleteConfirm.body', { name: deletingName })"
      :confirm-text="t('oidc.admin.deleteConfirm.confirm')"
      :cancel-text="t('oidc.admin.deleteConfirm.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />

    <!-- Reset secret confirm -->
    <ConfirmDialog
      :show="showResetDialog"
      :title="t('oidc.admin.resetSecretConfirm.title')"
      :message="t('oidc.admin.resetSecretConfirm.body')"
      :confirm-text="t('oidc.admin.resetSecretConfirm.confirm')"
      :cancel-text="t('oidc.admin.resetSecretConfirm.cancel')"
      danger
      @confirm="confirmResetSecret"
      @cancel="showResetDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { formatDateTime } from '@/utils/format'
import {
  listClients,
  createClient,
  updateClient,
  deleteClient,
  resetClientSecret,
  isSensitiveScope,
  OIDC_ALLOWED_SCOPES,
  type OidcClient
} from '@/api/admin/oidcClients'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const rows = ref<OidcClient[]>([])
const loading = ref(false)
const submitting = ref(false)

const allowedScopes = OIDC_ALLOWED_SCOPES

const showFormDialog = ref(false)
const editingId = ref<number | null>(null)

interface FormState {
  client_name: string
  redirect_uris: string[]
  allowed_scopes: string[]
  consent_required: boolean
  enabled: boolean
}
const form = reactive<FormState>({
  client_name: '',
  redirect_uris: [''],
  allowed_scopes: ['openid'],
  consent_required: true,
  enabled: true
})

const columns = computed<Column[]>(() => [
  { key: 'client_name', label: t('oidc.admin.table.name') },
  { key: 'client_id', label: t('oidc.admin.table.clientId') },
  { key: 'allowed_scopes', label: t('oidc.admin.table.scopes') },
  { key: 'redirect_uris', label: t('oidc.admin.table.redirectUris') },
  { key: 'enabled', label: t('oidc.admin.table.enabled') },
  { key: 'created_at', label: t('oidc.admin.table.createdAt') },
  { key: 'actions', label: t('oidc.admin.table.actions') }
])

const formHasSensitiveScope = computed(() => form.allowed_scopes.some((s) => isSensitiveScope(s)))

async function loadList() {
  loading.value = true
  try {
    rows.value = (await listClients()) ?? []
  } catch {
    appStore.showError(t('oidc.admin.loadFailed'))
  } finally {
    loading.value = false
  }
}

function resetForm() {
  form.client_name = ''
  form.redirect_uris = ['']
  form.allowed_scopes = ['openid']
  form.consent_required = true
  form.enabled = true
}

function openCreateDialog() {
  resetForm()
  editingId.value = null
  showFormDialog.value = true
}

function openEditDialog(row: OidcClient) {
  editingId.value = row.id
  form.client_name = row.client_name
  form.redirect_uris = row.redirect_uris.length ? [...row.redirect_uris] : ['']
  form.allowed_scopes = [...row.allowed_scopes]
  form.consent_required = row.consent_required
  form.enabled = row.enabled
  showFormDialog.value = true
}

function addRedirectUri() {
  form.redirect_uris.push('')
}

function removeRedirectUri(idx: number) {
  form.redirect_uris.splice(idx, 1)
  if (form.redirect_uris.length === 0) form.redirect_uris.push('')
}

function toggleScope(scope: string) {
  const i = form.allowed_scopes.indexOf(scope)
  if (i >= 0) {
    form.allowed_scopes.splice(i, 1)
  } else {
    form.allowed_scopes.push(scope)
  }
}

// ---- Save flow (with sensitive-scope confirm gate) ----
const showSensitiveConfirm = ref(false)
const sensitiveConfirmChecked = ref(false)

function validateForm(): boolean {
  if (!form.client_name.trim()) {
    appStore.showError(t('oidc.admin.form.nameRequired'))
    return false
  }
  const uris = form.redirect_uris.map((u) => u.trim()).filter(Boolean)
  if (uris.length === 0) {
    appStore.showError(t('oidc.admin.form.redirectUriRequired'))
    return false
  }
  return true
}

function onSubmitClicked() {
  if (!validateForm()) return
  if (formHasSensitiveScope.value) {
    sensitiveConfirmChecked.value = false
    showSensitiveConfirm.value = true
    return
  }
  void doSave()
}

function confirmSensitiveAndSave() {
  showSensitiveConfirm.value = false
  void doSave()
}

async function doSave() {
  const uris = form.redirect_uris.map((u) => u.trim()).filter(Boolean)
  submitting.value = true
  try {
    if (editingId.value === null) {
      const created = await createClient({
        client_name: form.client_name.trim(),
        redirect_uris: uris,
        allowed_scopes: form.allowed_scopes,
        consent_required: form.consent_required,
        enabled: form.enabled
      })
      showFormDialog.value = false
      revealSecret(created.client_secret, t('oidc.admin.secretReveal.createBanner'))
    } else {
      await updateClient(editingId.value, {
        client_name: form.client_name.trim(),
        redirect_uris: uris,
        allowed_scopes: form.allowed_scopes,
        consent_required: form.consent_required,
        enabled: form.enabled
      })
      showFormDialog.value = false
    }
    await loadList()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('oidc.admin.form.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// ---- One-time secret reveal ----
const showSecretDialog = ref(false)
const revealedSecret = ref('')
const secretBanner = ref('')
const secretCopied = ref(false)

function revealSecret(secret: string, banner: string) {
  revealedSecret.value = secret
  secretBanner.value = banner
  secretCopied.value = false
  showSecretDialog.value = true
}

async function copySecret() {
  try {
    await navigator.clipboard.writeText(revealedSecret.value)
    secretCopied.value = true
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

function closeSecretDialog() {
  showSecretDialog.value = false
  revealedSecret.value = ''
}

// 一次性密钥弹窗不允许通过 X / ESC 关闭，只能点“我已保存”。
function noopClose() {
  /* intentionally no-op */
}

// ---- Delete ----
const showDeleteDialog = ref(false)
const deletingId = ref<number | null>(null)
const deletingName = ref('')

function askDelete(row: OidcClient) {
  deletingId.value = row.id
  deletingName.value = row.client_name
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (deletingId.value === null) return
  try {
    await deleteClient(deletingId.value)
    showDeleteDialog.value = false
    deletingId.value = null
    await loadList()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('oidc.admin.deleteConfirm.failed'))
  }
}

// ---- Reset secret ----
const showResetDialog = ref(false)
const resettingId = ref<number | null>(null)

function askResetSecret(row: OidcClient) {
  resettingId.value = row.id
  showResetDialog.value = true
}

async function confirmResetSecret() {
  if (resettingId.value === null) return
  try {
    const res = await resetClientSecret(resettingId.value)
    showResetDialog.value = false
    resettingId.value = null
    revealSecret(res.client_secret, t('oidc.admin.secretReveal.resetBanner'))
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('oidc.admin.resetSecretConfirm.failed'))
  }
}

onMounted(loadList)
</script>
