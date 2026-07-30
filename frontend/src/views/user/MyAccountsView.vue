<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <div class="min-w-0 flex-1">
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ t('myAccounts.subtitle') }}
            </p>
          </div>
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              @click="loadAccounts"
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh')"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateModal" class="btn btn-primary">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('myAccounts.create') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable
          :columns="columns"
          :data="accounts"
          :loading="loading"
          :server-side-sort="false"
        >
          <template #cell-name="{ value }">
            <span class="font-medium text-gray-900 dark:text-white">{{ value }}</span>
          </template>

          <template #cell-platform="{ value }">
            <span
              class="inline-flex items-center gap-1.5 rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-700 dark:bg-dark-600 dark:text-gray-200"
            >
              <PlatformIcon :platform="value" size="xs" />
              {{ platformLabel(value) }}
            </span>
          </template>

          <template #cell-type="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-300">
              {{ typeLabel(value) }}
            </span>
          </template>

          <template #cell-visibility="{ value, row }">
            <div class="flex flex-col items-start gap-0.5">
              <span
                :class="[
                  'inline-flex rounded-full px-2 py-0.5 text-xs font-medium',
                  value === 'public'
                    ? 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300'
                    : 'bg-gray-100 text-gray-600 dark:bg-gray-700 dark:text-gray-300'
                ]"
              >
                {{
                  value === 'public'
                    ? t('myAccounts.visibility.public')
                    : t('myAccounts.visibility.private')
                }}
              </span>
              <span
                v-if="row.visibility_reason"
                class="max-w-[10rem] text-[10px] text-amber-600 dark:text-amber-400"
                :title="visibilityReasonLabel(row.visibility_reason)"
              >
                {{ visibilityReasonLabel(row.visibility_reason) }}
              </span>
            </div>
          </template>

          <template #cell-upstream_plan="{ value }">
            <span
              v-if="value"
              class="inline-flex rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] font-medium text-slate-600 dark:bg-slate-800 dark:text-slate-300"
            >
              {{ value }}
            </span>
            <span v-else class="text-xs text-gray-400 dark:text-gray-500">—</span>
          </template>

          <template #cell-status="{ value }">
            <span
              :class="[
                'badge',
                value === 'active' ? 'badge-success' : value === 'error' ? 'badge-danger' : 'badge-gray'
              ]"
            >
              {{ statusLabel(value) }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex flex-wrap items-center gap-2">
              <button
                type="button"
                class="btn btn-secondary btn-sm"
                :disabled="actionLoadingId === row.id"
                @click="toggleVisibility(row)"
              >
                {{
                  row.visibility === 'public'
                    ? t('myAccounts.actions.makePrivate')
                    : t('myAccounts.actions.makePublic')
                }}
              </button>
              <button
                type="button"
                class="btn btn-danger btn-sm"
                :disabled="actionLoadingId === row.id"
                @click="askDelete(row)"
              >
                {{ t('common.delete') }}
              </button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          v-if="pagination.total > 0"
          :page="pagination.page"
          :page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="onPageChange"
          @update:page-size="onPageSizeChange"
        />
      </template>
    </TablePageLayout>

    <!-- Create modal -->
    <BaseDialog
      :show="showCreateModal"
      :title="t('myAccounts.createTitle')"
      width="normal"
      @close="closeCreateModal"
    >
      <form class="space-y-4" @submit.prevent="handleCreate">
        <div>
          <label class="input-label">{{ t('myAccounts.form.name') }}</label>
          <input
            v-model="createForm.name"
            type="text"
            required
            class="input"
            :placeholder="t('myAccounts.form.namePlaceholder')"
          />
        </div>

        <div>
          <label class="input-label">{{ t('myAccounts.form.platform') }}</label>
          <Select v-model="createForm.platform" :options="platformOptions" />
        </div>

        <div>
          <label class="input-label">{{ t('myAccounts.form.type') }}</label>
          <Select v-model="createForm.type" :options="typeOptions" />
          <p class="input-hint">{{ t('myAccounts.form.typeHint') }}</p>
        </div>

        <div v-if="createForm.type === 'apikey'">
          <label class="input-label">{{ t('myAccounts.form.apiKey') }}</label>
          <input
            v-model="createForm.credentialValue"
            type="password"
            autocomplete="off"
            class="input font-mono text-sm"
            :placeholder="t('myAccounts.form.apiKeyPlaceholder')"
          />
        </div>
        <div v-else>
          <label class="input-label">{{ t('myAccounts.form.accessToken') }}</label>
          <textarea
            v-model="createForm.credentialValue"
            rows="3"
            class="input font-mono text-sm"
            :placeholder="t('myAccounts.form.accessTokenPlaceholder')"
          />
          <p class="input-hint">{{ t('myAccounts.form.oauthPasteHint') }}</p>
        </div>

        <div>
          <label class="input-label">{{ t('myAccounts.form.visibility') }}</label>
          <Select v-model="createForm.visibility" :options="visibilityOptions" />
          <p class="input-hint">{{ t('myAccounts.form.visibilityHint') }}</p>
          <p
            v-if="createForm.visibility === 'public' && !typeSupportsPublic"
            class="mt-1 text-xs text-amber-600 dark:text-amber-400"
          >
            {{ t('myAccounts.form.publicUnsupportedHint') }}
          </p>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeCreateModal">
            {{ t('common.cancel') }}
          </button>
          <button
            type="button"
            class="btn btn-primary"
            :disabled="submitting"
            @click="handleCreate"
          >
            {{ submitting ? t('common.saving') : t('common.create') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('myAccounts.deleteTitle')"
      :message="
        t('myAccounts.deleteConfirm', {
          name: deletingAccount?.name || ''
        })
      "
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import userAccountsAPI from '@/api/userAccounts'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { Account, AccountPlatform, AccountType } from '@/types'

const { t } = useI18n()
const appStore = useAppStore()

/** AllowedQuotaPlatforms: anthropic / openai / gemini / antigravity / grok */
const QUOTA_PLATFORMS: AccountPlatform[] = [
  'anthropic',
  'openai',
  'gemini',
  'antigravity',
  'grok'
]

const accounts = ref<Account[]>([])
const loading = ref(false)
const submitting = ref(false)
const actionLoadingId = ref<number | null>(null)
const showCreateModal = ref(false)
const showDeleteDialog = ref(false)
const deletingAccount = ref<Account | null>(null)

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0,
  pages: 0
})

const createForm = reactive({
  name: '',
  platform: 'openai' as AccountPlatform,
  type: 'apikey' as AccountType,
  credentialValue: '',
  visibility: 'private' as 'private' | 'public'
})

const columns = computed(() => [
  { key: 'name', label: t('myAccounts.columns.name'), sortable: false },
  { key: 'platform', label: t('myAccounts.columns.platform'), sortable: false },
  { key: 'type', label: t('myAccounts.columns.type'), sortable: false },
  { key: 'visibility', label: t('myAccounts.columns.visibility'), sortable: false },
  { key: 'upstream_plan', label: t('myAccounts.columns.upstreamPlan'), sortable: false },
  { key: 'status', label: t('myAccounts.columns.status'), sortable: false },
  { key: 'actions', label: t('myAccounts.columns.actions'), sortable: false }
])

const platformOptions = computed(() =>
  QUOTA_PLATFORMS.map((p) => ({
    value: p,
    label: platformLabel(p)
  }))
)

/** Platform → allowed account types (mirrors backend isUserAllowedAccountType) */
function allowedTypesForPlatform(platform: AccountPlatform): AccountType[] {
  switch (platform) {
    case 'openai':
    case 'gemini':
    case 'anthropic':
      return ['oauth', 'apikey']
    case 'grok':
    case 'antigravity':
      return ['oauth']
    default:
      return ['oauth']
  }
}

const typeOptions = computed(() =>
  allowedTypesForPlatform(createForm.platform).map((ty) => ({
    value: ty,
    label: typeLabel(ty)
  }))
)

const visibilityOptions = computed(() => [
  { value: 'private', label: t('myAccounts.visibility.private') },
  { value: 'public', label: t('myAccounts.visibility.public') }
])

/** openai apikey cannot go public (backend force-private) */
const typeSupportsPublic = computed(() => {
  if (createForm.platform === 'openai' && createForm.type === 'apikey') return false
  return true
})

watch(
  () => createForm.platform,
  (platform) => {
    const allowed = allowedTypesForPlatform(platform)
    if (!allowed.includes(createForm.type)) {
      createForm.type = allowed[0]
    }
  }
)

function platformLabel(platform: string): string {
  const key = `admin.groups.platforms.${platform}`
  const translated = t(key)
  return translated === key ? platform : translated
}

function typeLabel(type: string): string {
  if (type === 'apikey') return t('myAccounts.types.apikey')
  if (type === 'oauth') return t('myAccounts.types.oauth')
  if (type === 'setup-token') return t('myAccounts.types.setupToken')
  return type
}

function statusLabel(status: string): string {
  if (status === 'active') return t('myAccounts.status.active')
  if (status === 'inactive' || status === 'disabled') return t('myAccounts.status.disabled')
  if (status === 'error') return t('myAccounts.status.error')
  return status
}

function visibilityReasonLabel(reason: string): string {
  if (reason === 'plan_probe_failed') return t('myAccounts.reasons.planProbeFailed')
  if (reason === 'plan_probe_unsupported') return t('myAccounts.reasons.planProbeUnsupported')
  if (reason === 'plan_empty') return t('myAccounts.reasons.planEmpty')
  return reason
}

async function loadAccounts() {
  loading.value = true
  try {
    const res = await userAccountsAPI.list(pagination.page, pagination.page_size)
    accounts.value = res.items ?? []
    pagination.total = res.total ?? 0
    pagination.pages = res.pages ?? 0
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('myAccounts.failedToLoad')))
  } finally {
    loading.value = false
  }
}

function onPageChange(page: number) {
  pagination.page = page
  loadAccounts()
}

function onPageSizeChange(pageSize: number) {
  pagination.page_size = pageSize
  pagination.page = 1
  loadAccounts()
}

function openCreateModal() {
  createForm.name = ''
  createForm.platform = 'openai'
  createForm.type = 'apikey'
  createForm.credentialValue = ''
  createForm.visibility = 'private'
  showCreateModal.value = true
}

function closeCreateModal() {
  showCreateModal.value = false
}

function buildCredentials(): Record<string, unknown> {
  const raw = createForm.credentialValue.trim()
  if (!raw) return {}
  if (createForm.type === 'apikey') {
    return { api_key: raw }
  }
  // oauth / setup-token: paste access_token only (no full OAuth wizard in v1)
  return { access_token: raw }
}

async function handleCreate() {
  if (!createForm.name.trim()) {
    appStore.showError(t('myAccounts.form.nameRequired'))
    return
  }
  if (!createForm.credentialValue.trim()) {
    appStore.showError(
      createForm.type === 'apikey'
        ? t('myAccounts.form.apiKeyRequired')
        : t('myAccounts.form.accessTokenRequired')
    )
    return
  }

  submitting.value = true
  try {
    const created = await userAccountsAPI.create({
      name: createForm.name.trim(),
      platform: createForm.platform,
      type: createForm.type,
      credentials: buildCredentials(),
      visibility: createForm.visibility
    })
    if (created.visibility_reason) {
      appStore.showSuccess(
        t('myAccounts.createSuccessForcedPrivate', {
          reason: visibilityReasonLabel(created.visibility_reason)
        })
      )
    } else {
      appStore.showSuccess(t('myAccounts.createSuccess'))
    }
    closeCreateModal()
    pagination.page = 1
    await loadAccounts()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('myAccounts.failedToCreate')))
  } finally {
    submitting.value = false
  }
}

async function toggleVisibility(row: Account) {
  const next = row.visibility === 'public' ? 'private' : 'public'
  actionLoadingId.value = row.id
  try {
    const updated = await userAccountsAPI.setVisibility(row.id, next)
    const idx = accounts.value.findIndex((a) => a.id === row.id)
    if (idx >= 0) {
      accounts.value[idx] = { ...accounts.value[idx], ...updated }
    }
    if (updated.visibility_reason) {
      appStore.showError(
        t('myAccounts.visibilityForcedPrivate', {
          reason: visibilityReasonLabel(updated.visibility_reason)
        })
      )
    } else {
      appStore.showSuccess(
        next === 'public'
          ? t('myAccounts.madePublic')
          : t('myAccounts.madePrivate')
      )
    }
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('myAccounts.failedToUpdateVisibility')))
  } finally {
    actionLoadingId.value = null
  }
}

function askDelete(row: Account) {
  deletingAccount.value = row
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingAccount.value) return
  const id = deletingAccount.value.id
  actionLoadingId.value = id
  try {
    await userAccountsAPI.remove(id)
    appStore.showSuccess(t('myAccounts.deleteSuccess'))
    showDeleteDialog.value = false
    deletingAccount.value = null
    await loadAccounts()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('myAccounts.failedToDelete')))
  } finally {
    actionLoadingId.value = null
  }
}

onMounted(loadAccounts)
</script>
