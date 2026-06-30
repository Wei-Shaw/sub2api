<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ t('billingApps.admin.title') }}
          </h2>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.description') }}</span>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadList" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              {{ t('billingApps.admin.createButton') }}
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
              {{ row.enabled ? t('billingApps.admin.status.enabled') : t('billingApps.admin.status.disabled') }}
            </span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button
                v-if="row.enabled"
                class="btn btn-secondary btn-xs"
                :disabled="togglingId === row.app_id"
                @click="toggle(row, false)"
              >{{ t('billingApps.admin.actions.disable') }}</button>
              <button
                v-else
                class="btn btn-primary btn-xs"
                :disabled="togglingId === row.app_id"
                @click="toggle(row, true)"
              >{{ t('billingApps.admin.actions.enable') }}</button>
              <button class="btn btn-secondary btn-xs" @click="openStats(row)">{{ t('billingApps.admin.actions.stats') }}</button>
              <button class="btn btn-secondary btn-xs" @click="askRefresh(row)">{{ t('billingApps.admin.actions.refreshToken') }}</button>
              <button class="btn btn-danger btn-xs" @click="askDelete(row)">{{ t('billingApps.admin.actions.delete') }}</button>
            </div>
          </template>

          <template #empty>
            <p class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.empty') }}</p>
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Create Dialog -->
    <BaseDialog
      :show="showFormDialog"
      :title="t('billingApps.admin.form.createTitle')"
      @close="showFormDialog = false"
    >
      <div class="space-y-4">
        <div>
          <label class="form-label">{{ t('billingApps.admin.form.appName') }}</label>
          <input
            v-model="appName"
            type="text"
            class="input"
            :placeholder="t('billingApps.admin.form.appNamePlaceholder')"
          />
        </div>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showFormDialog = false">{{ t('billingApps.admin.form.cancel') }}</button>
        <button class="btn btn-primary" :disabled="submitting" @click="doCreate">
          {{ submitting ? t('common.saving') : t('billingApps.admin.form.save') }}
        </button>
      </template>
    </BaseDialog>

    <!-- One-time token reveal -->
    <BaseDialog
      :show="showTokenDialog"
      :title="t('billingApps.admin.tokenReveal.title')"
      :close-on-escape="false"
      @close="noopClose"
    >
      <div class="space-y-3">
        <p class="rounded-lg border border-red-300 bg-red-50 p-3 text-sm font-medium text-red-700 dark:border-red-700 dark:bg-red-900/30 dark:text-red-300">
          {{ t('billingApps.admin.tokenReveal.banner') }}
        </p>
        <div class="flex items-center gap-2">
          <code class="flex-1 break-all rounded bg-gray-100 px-3 py-2 text-sm text-gray-800 dark:bg-dark-700 dark:text-gray-100">{{ revealedToken }}</code>
          <button class="btn btn-secondary btn-sm" @click="copyToken">
            {{ tokenCopied ? t('billingApps.admin.tokenReveal.copied') : t('billingApps.admin.tokenReveal.copy') }}
          </button>
        </div>
      </div>
      <template #footer>
        <button class="btn btn-primary" @click="closeTokenDialog">{{ t('billingApps.admin.tokenReveal.done') }}</button>
      </template>
    </BaseDialog>

    <!-- Stats dialog -->
    <BaseDialog
      :show="showStatsDialog"
      :title="t('billingApps.admin.stats.title', { name: statsAppName })"
      @close="showStatsDialog = false"
    >
      <div v-if="stats" class="space-y-2 text-sm">
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.stats.netDeducted') }}</span><span class="font-semibold text-gray-900 dark:text-gray-100">{{ stats.net_deducted }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.stats.totalDeducted') }}</span><span>{{ stats.total_deducted }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.stats.totalRefunded') }}</span><span>{{ stats.total_refunded }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.stats.deductCount') }}</span><span>{{ stats.deduct_count }}</span></div>
        <div class="flex justify-between"><span class="text-gray-500 dark:text-gray-400">{{ t('billingApps.admin.stats.refundCount') }}</span><span>{{ stats.refund_count }}</span></div>
      </div>
      <p v-else class="py-4 text-center text-sm text-gray-500 dark:text-gray-400">{{ t('common.loading') }}</p>
      <template #footer>
        <button class="btn btn-primary" @click="showStatsDialog = false">{{ t('common.close') }}</button>
      </template>
    </BaseDialog>

    <!-- Refresh token confirm -->
    <ConfirmDialog
      :show="showRefreshDialog"
      :title="t('billingApps.admin.refreshConfirm.title')"
      :message="t('billingApps.admin.refreshConfirm.body')"
      :confirm-text="t('billingApps.admin.refreshConfirm.confirm')"
      :cancel-text="t('billingApps.admin.refreshConfirm.cancel')"
      danger
      @confirm="confirmRefresh"
      @cancel="showRefreshDialog = false"
    />

    <!-- Delete confirm -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('billingApps.admin.deleteConfirm.title')"
      :message="t('billingApps.admin.deleteConfirm.body', { name: deletingName })"
      :confirm-text="t('billingApps.admin.deleteConfirm.confirm')"
      :cancel-text="t('billingApps.admin.deleteConfirm.cancel')"
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
  listBillingApps,
  createBillingApp,
  setBillingAppEnabled,
  refreshBillingAppToken,
  deleteBillingApp,
  getBillingAppStats,
  type BillingApp,
  type BillingAppStats
} from '@/api/admin/billingApps'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const rows = ref<BillingApp[]>([])
const loading = ref(false)
const submitting = ref(false)
const togglingId = ref<string | null>(null)

const columns = computed<Column[]>(() => [
  { key: 'app_name', label: t('billingApps.admin.table.name') },
  { key: 'app_id', label: t('billingApps.admin.table.appId') },
  { key: 'enabled', label: t('billingApps.admin.table.enabled') },
  { key: 'actions', label: t('billingApps.admin.table.actions') }
])

async function loadList() {
  loading.value = true
  try {
    rows.value = (await listBillingApps()) ?? []
  } catch {
    appStore.showError(t('billingApps.admin.loadFailed'))
  } finally {
    loading.value = false
  }
}

// ---- Create ----
const showFormDialog = ref(false)
const appName = ref('')

function openCreateDialog() {
  appName.value = ''
  showFormDialog.value = true
}

async function doCreate() {
  if (!appName.value.trim()) {
    appStore.showError(t('billingApps.admin.form.nameRequired'))
    return
  }
  submitting.value = true
  try {
    const created = await createBillingApp(appName.value.trim())
    showFormDialog.value = false
    revealToken(created.token)
    await loadList()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('billingApps.admin.form.saveFailed'))
  } finally {
    submitting.value = false
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
async function toggle(row: BillingApp, enabled: boolean) {
  togglingId.value = row.app_id
  try {
    await setBillingAppEnabled(row.app_id, enabled)
    row.enabled = enabled
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('billingApps.admin.toggleFailed'))
  } finally {
    togglingId.value = null
  }
}

// ---- Stats ----
const showStatsDialog = ref(false)
const stats = ref<BillingAppStats | null>(null)
const statsAppName = ref('')

async function openStats(row: BillingApp) {
  stats.value = null
  statsAppName.value = row.app_name
  showStatsDialog.value = true
  try {
    stats.value = await getBillingAppStats(row.app_id)
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('billingApps.admin.stats.failed'))
    showStatsDialog.value = false
  }
}

// ---- Refresh token ----
const showRefreshDialog = ref(false)
const refreshingId = ref<string | null>(null)

function askRefresh(row: BillingApp) {
  refreshingId.value = row.app_id
  showRefreshDialog.value = true
}

async function confirmRefresh() {
  if (!refreshingId.value) return
  try {
    const res = await refreshBillingAppToken(refreshingId.value)
    showRefreshDialog.value = false
    refreshingId.value = null
    revealToken(res.token)
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('billingApps.admin.refreshConfirm.failed'))
  }
}

// ---- Delete ----
const showDeleteDialog = ref(false)
const deletingId = ref<string | null>(null)
const deletingName = ref('')

function askDelete(row: BillingApp) {
  deletingId.value = row.app_id
  deletingName.value = row.app_name
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingId.value) return
  try {
    await deleteBillingApp(deletingId.value)
    showDeleteDialog.value = false
    deletingId.value = null
    await loadList()
  } catch (e: unknown) {
    appStore.showError((e as { message?: string })?.message ?? t('billingApps.admin.deleteConfirm.failed'))
  }
}

onMounted(loadList)
</script>
