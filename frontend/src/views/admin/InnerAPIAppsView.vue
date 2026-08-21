<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ t('innerApiApps.admin.title') }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.description') }}</span>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadList" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              {{ t('innerApiApps.admin.createButton') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="rows" :loading="loading">
          <template #cell-app_name="{ row }">
            <span class="font-medium text-gray-900 dark:text-gray-100">{{ row.app_name }}</span>
          </template>

          <template #cell-app_id="{ row }">
            <code class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200">{{ row.app_id }}</code>
          </template>

          <template #cell-enabled="{ row }">
            <span :class="['badge', row.enabled ? 'badge-success' : 'badge-default']">
              {{ row.enabled ? t('innerApiApps.admin.status.enabled') : t('innerApiApps.admin.status.disabled') }}
            </span>
          </template>

          <template #cell-permissions="{ row }">
            <div class="flex flex-wrap gap-1">
              <span
                v-for="permission in row.permissions"
                :key="permission"
                class="rounded bg-gray-100 px-1.5 py-0.5 text-xs text-gray-700 dark:bg-dark-700 dark:text-gray-200"
              >{{ permission }}</span>
              <span v-if="!row.permissions?.length" class="text-xs text-gray-400">-</span>
            </div>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button
                v-if="row.enabled"
                class="btn btn-secondary btn-xs"
                :disabled="togglingId === row.app_id"
                @click="toggle(row, false)"
              >{{ t('innerApiApps.admin.actions.disable') }}</button>
              <button
                v-else
                class="btn btn-primary btn-xs"
                :disabled="togglingId === row.app_id"
                @click="toggle(row, true)"
              >{{ t('innerApiApps.admin.actions.enable') }}</button>
              <button class="btn btn-secondary btn-xs" @click="openPermissionsDialog(row)">{{ t('innerApiApps.admin.actions.permissions') }}</button>
              <button class="btn btn-secondary btn-xs" @click="openStats(row)">{{ t('innerApiApps.admin.actions.stats') }}</button>
              <button class="btn btn-secondary btn-xs" @click="askRefresh(row)">{{ t('innerApiApps.admin.actions.refreshToken') }}</button>
              <button class="btn btn-danger btn-xs" @click="askDelete(row)">{{ t('innerApiApps.admin.actions.delete') }}</button>
            </div>
          </template>

          <template #empty>
            <p class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.empty') }}</p>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Create Dialog -->
    <BaseDialog
      :show="showFormDialog"
      :title="t('innerApiApps.admin.form.createTitle')"
      @close="showFormDialog = false"
    >
      <div class="space-y-4">
        <div>
          <label class="form-label">{{ t('innerApiApps.admin.form.appName') }}</label>
          <input
            v-model="appName"
            type="text"
            class="input"
            :placeholder="t('innerApiApps.admin.form.appNamePlaceholder')"
          />
        </div>
        <div>
          <label class="form-label">{{ t('innerApiApps.admin.form.permissions') }}</label>
          <div class="grid gap-2 sm:grid-cols-2">
            <label
              v-for="permission in INNER_API_PERMISSIONS"
              :key="permission"
              class="flex items-center gap-2 rounded border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
            >
              <input v-model="selectedPermissions" :value="permission" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
              <span>{{ t(`innerApiApps.admin.permissions.${permission.replace(':', '_')}`) }}</span>
              <code class="ml-auto text-xs text-gray-400">{{ permission }}</code>
            </label>
          </div>
        </div>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showFormDialog = false">{{ t('innerApiApps.admin.form.cancel') }}</button>
        <button class="btn btn-primary" :disabled="submitting" @click="doCreate">
          {{ submitting ? t('common.saving') : t('innerApiApps.admin.form.save') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Permission dialog -->
    <BaseDialog
      :show="showPermissionsDialog"
      :title="t('innerApiApps.admin.permissionsDialog.title', { name: permissionsAppName })"
      @close="showPermissionsDialog = false"
    >
      <div class="grid gap-2 sm:grid-cols-2">
        <label
          v-for="permission in INNER_API_PERMISSIONS"
          :key="permission"
          class="flex items-center gap-2 rounded border border-gray-200 px-3 py-2 text-sm dark:border-dark-600"
        >
          <input v-model="selectedPermissions" :value="permission" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600 focus:ring-primary-500" />
          <span>{{ t(`innerApiApps.admin.permissions.${permission.replace(':', '_')}`) }}</span>
          <code class="ml-auto text-xs text-gray-400">{{ permission }}</code>
        </label>
      </div>
      <template #footer>
        <button class="btn btn-secondary" @click="showPermissionsDialog = false">{{ t('innerApiApps.admin.form.cancel') }}</button>
        <button class="btn btn-primary" :disabled="permissionsSubmitting" @click="savePermissions">
          {{ permissionsSubmitting ? t('common.saving') : t('innerApiApps.admin.form.savePermissions') }}
        </button>
      </template>
    </BaseDialog>

    <!-- One-time token reveal -->
    <BaseDialog
      :show="showTokenDialog"
      :title="t('innerApiApps.admin.tokenReveal.title')"
      :close-on-escape="false"
      @close="noopClose"
    >
      <div class="space-y-3">
        <p class="rounded-lg border border-red-300 bg-red-50 p-3 text-sm font-medium text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300">
          {{ t('innerApiApps.admin.tokenReveal.banner') }}
        </p>
        <div class="flex items-center gap-2">
          <code class="flex-1 break-all rounded bg-gray-100 px-3 py-2 text-sm text-gray-800 dark:bg-dark-700 dark:text-gray-100">{{ revealedToken }}</code>
          <button class="btn btn-secondary btn-sm" @click="copyToken">
            {{ tokenCopied ? t('innerApiApps.admin.tokenReveal.copied') : t('innerApiApps.admin.tokenReveal.copy') }}
          </button>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-primary" @click="closeTokenDialog">{{ t('innerApiApps.admin.tokenReveal.done') }}</button>
      </template>
    </BaseDialog>

    <!-- Stats dialog -->
    <BaseDialog
      :show="showStatsDialog"
      :title="t('innerApiApps.admin.stats.title', { name: statsAppName })"
      @close="showStatsDialog = false"
    >
      <div v-if="stats" class="space-y-2 text-sm">
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.stats.netDeducted') }}</span><span class="font-semibold text-gray-900 dark:text-gray-100">{{ stats.net_deducted }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.stats.totalDeducted') }}</span><span>{{ stats.total_deducted }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.stats.totalRefunded') }}</span><span>{{ stats.total_refunded }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.stats.deductCount') }}</span><span>{{ stats.deduct_count }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('innerApiApps.admin.stats.refundCount') }}</span><span>{{ stats.refund_count }}</span></div>
      </div>
      <p v-else class="py-4 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</p>
      <template #footer>
        <button class="btn btn-primary" @click="showStatsDialog = false">{{ t('common.close') }}</button>
      </template>
    </BaseDialog>

    <!-- Refresh token confirm -->
    <ConfirmDialog
      :show="showRefreshDialog"
      :title="t('innerApiApps.admin.refreshConfirm.title')"
      :message="t('innerApiApps.admin.refreshConfirm.body')"
      :confirm-text="t('innerApiApps.admin.refreshConfirm.confirm')"
      :cancel-text="t('innerApiApps.admin.refreshConfirm.cancel')"
      danger
      @confirm="confirmRefresh"
      @cancel="showRefreshDialog = false"
    />

    <!-- Delete confirm -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('innerApiApps.admin.deleteConfirm.title')"
      :message="t('innerApiApps.admin.deleteConfirm.body', { name: deletingName })"
      :confirm-text="t('innerApiApps.admin.deleteConfirm.confirm')"
      :cancel-text="t('innerApiApps.admin.deleteConfirm.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import {
  listInnerAPIApps,
  createInnerAPIApp,
  setInnerAPIAppEnabled,
  refreshInnerAPIAppToken,
  deleteInnerAPIApp,
  getInnerAPIAppStats,
  setInnerAPIAppPermissions,
  INNER_API_PERMISSIONS,
  type InnerAPIApp,
  type InnerAPIAppStats,
  type InnerAPIPermission
} from '@/api/admin/innerApiApps'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const rows = ref<InnerAPIApp[]>([])
const loading = ref(false)
const submitting = ref(false)
const togglingId = ref<string | null>(null)

const columns = computed<Column[]>(() => [
  { key: 'app_name', label: t('innerApiApps.admin.table.name') },
  { key: 'app_id', label: t('innerApiApps.admin.table.appId') },
  { key: 'enabled', label: t('innerApiApps.admin.table.enabled') },
  { key: 'permissions', label: t('innerApiApps.admin.table.permissions') },
  { key: 'actions', label: t('innerApiApps.admin.table.actions') }
])

async function loadList() {
  loading.value = true
  try {
    rows.value = (await listInnerAPIApps()) ?? []
  } catch {
    appStore.showError(t('innerApiApps.admin.loadFailed'))
  } finally {
    loading.value = false
  }
}

// ---- Create ----
const showFormDialog = ref(false)
const appName = ref('')
const selectedPermissions = ref<InnerAPIPermission[]>([...INNER_API_PERMISSIONS])

function openCreateDialog() {
  appName.value = ''
  selectedPermissions.value = [...INNER_API_PERMISSIONS]
  showFormDialog.value = true
}

async function doCreate() {
  if (!appName.value.trim()) {
    appStore.showError(t('innerApiApps.admin.form.nameRequired'))
    return
  }
  submitting.value = true
  try {
    const created = await createInnerAPIApp(appName.value.trim(), selectedPermissions.value)
    showFormDialog.value = false
    revealToken(created.token)
    await loadList()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('innerApiApps.admin.form.saveFailed'))
  } finally {
    submitting.value = false
  }
}

// ---- Permissions ----
const showPermissionsDialog = ref(false)
const permissionsAppId = ref<string | null>(null)
const permissionsAppName = ref('')
const permissionsSubmitting = ref(false)

function openPermissionsDialog(row: InnerAPIApp) {
  permissionsAppId.value = row.app_id
  permissionsAppName.value = row.app_name
  selectedPermissions.value = [...(row.permissions ?? [])]
  showPermissionsDialog.value = true
}

async function savePermissions() {
  if (!permissionsAppId.value) return
  permissionsSubmitting.value = true
  try {
    const result = await setInnerAPIAppPermissions(permissionsAppId.value, selectedPermissions.value)
    const row = rows.value.find((item) => item.app_id === permissionsAppId.value)
    if (row) row.permissions = [...result.permissions]
    showPermissionsDialog.value = false
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('innerApiApps.admin.permissionsDialog.failed'))
  } finally {
    permissionsSubmitting.value = false
  }
}

// ---- One-time token reveal ----
const showTokenDialog = ref(false)
const revealedToken = ref('')
const tokenCopied = ref(false)

function revealToken(token: string) {
  revealedToken.value = token
  tokenCopied.value = false
  showTokenDialog.value = true
}

async function copyToken() {
  try {
    await navigator.clipboard.writeText(revealedToken.value)
    tokenCopied.value = true
  } catch {
    appStore.showError(t('common.copyFailed'))
  }
}

function closeTokenDialog() {
  showTokenDialog.value = false
  revealedToken.value = ''
}

// 一次性 token 弹窗不允许通过 X / ESC 关闭，只能点“我已保存”。
function noopClose() {
  /* intentionally no-op */
}

// ---- Enable / disable ----
async function toggle(row: InnerAPIApp, enabled: boolean) {
  togglingId.value = row.app_id
  try {
    await setInnerAPIAppEnabled(row.app_id, enabled)
    row.enabled = enabled
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('innerApiApps.admin.toggleFailed'))
  } finally {
    togglingId.value = null
  }
}

// ---- Stats ----
const showStatsDialog = ref(false)
const stats = ref<InnerAPIAppStats | null>(null)
const statsAppName = ref('')

async function openStats(row: InnerAPIApp) {
  stats.value = null
  statsAppName.value = row.app_name
  showStatsDialog.value = true
  try {
    stats.value = await getInnerAPIAppStats(row.app_id)
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('innerApiApps.admin.stats.failed'))
    showStatsDialog.value = false
  }
}

// ---- Refresh token ----
const showRefreshDialog = ref(false)
const refreshingId = ref<string | null>(null)

function askRefresh(row: InnerAPIApp) {
  refreshingId.value = row.app_id
  showRefreshDialog.value = true
}

async function confirmRefresh() {
  if (!refreshingId.value) return
  try {
    const res = await refreshInnerAPIAppToken(refreshingId.value)
    showRefreshDialog.value = false
    refreshingId.value = null
    revealToken(res.token)
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('innerApiApps.admin.refreshConfirm.failed'))
  }
}

// ---- Delete ----
const showDeleteDialog = ref(false)
const deletingId = ref<string | null>(null)
const deletingName = ref('')

function askDelete(row: InnerAPIApp) {
  deletingId.value = row.app_id
  deletingName.value = row.app_name
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingId.value) return
  try {
    await deleteInnerAPIApp(deletingId.value)
    showDeleteDialog.value = false
    deletingId.value = null
    await loadList()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('innerApiApps.admin.deleteConfirm.failed'))
  }
}

onMounted(loadList)
</script>
