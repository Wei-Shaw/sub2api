<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center gap-3">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
            {{ t('admin.rechargePromos.title') }}
          </h2>
          <div class="flex flex-1 flex-wrap items-center justify-end gap-2">
            <button @click="loadList" :disabled="loading" class="btn btn-secondary" :title="t('common.refresh')">
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button @click="openCreateDialog" class="btn btn-primary">
              {{ t('admin.rechargePromos.createBtn') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="rows" :loading="loading">
          <template #cell-name="{ row }">
            <div class="flex items-center gap-2">
              <span class="font-medium text-gray-900 dark:text-gray-100">{{ row.name }}</span>
              <span v-if="row.note" class="text-xs text-gray-500 dark:text-gray-400">— {{ row.note }}</span>
            </div>
          </template>

          <template #cell-enabled="{ row }">
            <button
              type="button"
              class="inline-flex items-center"
              :title="t('admin.rechargePromos.toggleHint')"
              @click="onToggle(row)"
            >
              <span :class="['badge', row.enabled ? 'badge-success' : 'badge-default']">
                {{ row.enabled ? t('admin.rechargePromos.statusEnabled') : t('admin.rechargePromos.statusDisabled') }}
              </span>
            </button>
          </template>

          <template #cell-tiers="{ row }">
            <div class="flex flex-wrap gap-1 text-xs">
              <span
                v-for="t in row.tiers"
                :key="t.min_amount"
                class="rounded bg-gray-100 px-1.5 py-0.5 text-gray-700 dark:bg-dark-700 dark:text-gray-200"
              >≥{{ t.min_amount }} → +{{ formatRate(t.bonus_rate) }}</span>
            </div>
          </template>

          <template #cell-window="{ row }">
            <div class="text-xs text-gray-600 dark:text-gray-400">
              <div>{{ row.valid_from ? formatDateTime(row.valid_from) : t('admin.rechargePromos.noLowerBound') }}</div>
              <div>~ {{ row.valid_until ? formatDateTime(row.valid_until) : t('admin.rechargePromos.noUpperBound') }}</div>
            </div>
          </template>

          <template #cell-updated_at="{ value }">
            <span class="text-xs text-gray-500 dark:text-gray-400">{{ formatDateTime(value) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-2">
              <button class="btn btn-secondary btn-xs" @click="openEditDialog(row)">{{ t('common.edit') }}</button>
              <button class="btn btn-danger btn-xs" @click="askDelete(row)">{{ t('common.delete') }}</button>
            </div>
          </template>
        </DataTable>
      </template>

      <template #pagination>
        <Pagination
          :page="pagination.page"
          :page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="(p) => { pagination.page = p; loadList() }"
          @update:page-size="(s) => { pagination.page_size = s; pagination.page = 1; loadList() }"
        />
      </template>
    </TablePageLayout>

    <!-- Create / Edit Dialog -->
    <BaseDialog
      :show="showFormDialog"
      :title="editingId === null ? t('admin.rechargePromos.createTitle') : t('admin.rechargePromos.editTitle')"
      @close="showFormDialog = false"
    >
      <div class="space-y-4">
        <div>
          <label class="form-label">{{ t('admin.rechargePromos.fields.name') }}</label>
          <input v-model="form.name" type="text" class="input" :placeholder="t('admin.rechargePromos.fields.namePlaceholder')" />
        </div>
        <div class="flex items-center gap-3">
          <input id="promo-enabled" v-model="form.enabled" type="checkbox" class="h-4 w-4" />
          <label for="promo-enabled" class="form-label !mb-0">{{ t('admin.rechargePromos.fields.enabled') }}</label>
          <span class="text-xs text-gray-500 dark:text-gray-400">{{ t('admin.rechargePromos.fields.enabledHint') }}</span>
        </div>
        <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <div>
            <label class="form-label">{{ t('admin.rechargePromos.fields.validFrom') }}</label>
            <input v-model="form.valid_from" type="datetime-local" class="input" />
          </div>
          <div>
            <label class="form-label">{{ t('admin.rechargePromos.fields.validUntil') }}</label>
            <input v-model="form.valid_until" type="datetime-local" class="input" />
          </div>
        </div>

        <div>
          <div class="mb-2 flex items-center justify-between">
            <span class="form-label !mb-0">{{ t('admin.rechargePromos.fields.tiers') }}</span>
            <button class="btn btn-secondary btn-xs" @click="addTier">{{ t('admin.rechargePromos.fields.addTier') }}</button>
          </div>
          <div class="space-y-2">
            <div v-for="(tier, idx) in form.tiers" :key="idx" class="flex items-center gap-2">
              <span class="text-xs text-gray-500">≥</span>
              <input
                type="number"
                v-model.number="tier.min_amount"
                step="0.01"
                min="0"
                class="input w-32"
                :placeholder="t('admin.rechargePromos.fields.minAmount')"
              />
              <span class="text-xs text-gray-500">×</span>
              <input
                type="number"
                v-model.number="tier.bonus_rate"
                step="0.01"
                min="0"
                max="0.99"
                class="input w-32"
                :placeholder="t('admin.rechargePromos.fields.bonusRate')"
              />
              <span class="text-xs text-gray-500">({{ formatRate(tier.bonus_rate) }})</span>
              <button class="btn btn-ghost btn-xs ml-auto text-red-500" @click="removeTier(idx)">
                {{ t('common.remove') }}
              </button>
            </div>
            <p v-if="form.tiers.length === 0" class="text-xs text-gray-500">
              {{ t('admin.rechargePromos.fields.tiersEmptyHint') }}
            </p>
          </div>
        </div>

        <div>
          <label class="form-label">{{ t('admin.rechargePromos.fields.note') }}</label>
          <textarea v-model="form.note" rows="2" class="input" />
        </div>
      </div>

      <template #footer>
        <button class="btn btn-secondary" @click="showFormDialog = false">{{ t('common.cancel') }}</button>
        <button class="btn btn-primary" :disabled="submitting" @click="submitForm">
          {{ submitting ? t('common.saving') : t('common.save') }}
        </button>
      </template>
    </BaseDialog>

    <!-- Delete Confirm -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.rechargePromos.deleteTitle')"
      :message="t('admin.rechargePromos.deleteConfirm')"
      :confirm-text="t('common.delete')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import { formatDateTime } from '@/utils/format'
import type {
  RechargePromoActivity,
  CreateOrUpdateRechargePromoRequest,
  RechargePromoTier
} from '@/api/admin/rechargePromos'
import type { Column } from '@/components/common/types'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import Pagination from '@/components/common/Pagination.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { validateRechargePromoForm } from './rechargePromoFormValidation'

const { t } = useI18n()
const appStore = useAppStore()

const rows = ref<RechargePromoActivity[]>([])
const loading = ref(false)
const submitting = ref(false)

const pagination = reactive({ page: 1, page_size: 20, total: 0 })

const showFormDialog = ref(false)
const showDeleteDialog = ref(false)
const editingId = ref<number | null>(null)
const deletingId = ref<number | null>(null)

interface TierInput { min_amount: number; bonus_rate: number }
interface FormState {
  name: string
  enabled: boolean
  valid_from: string
  valid_until: string
  tiers: TierInput[]
  note: string
}
const form = reactive<FormState>({
  name: '',
  enabled: false,
  valid_from: '',
  valid_until: '',
  tiers: [],
  note: ''
})

const columns = computed<Column[]>(() => [
  { key: 'id', label: 'ID' },
  { key: 'name', label: t('admin.rechargePromos.columns.name') },
  { key: 'enabled', label: t('admin.rechargePromos.columns.status') },
  { key: 'tiers', label: t('admin.rechargePromos.columns.tiers') },
  { key: 'window', label: t('admin.rechargePromos.columns.window') },
  { key: 'updated_at', label: t('admin.rechargePromos.columns.updatedAt') },
  { key: 'actions', label: t('admin.rechargePromos.columns.actions') }
])

function formatRate(r: number): string {
  if (!Number.isFinite(r)) return ''
  return `${(r * 100).toFixed(0)}%`
}

function resetForm() {
  form.name = ''
  form.enabled = false
  form.valid_from = ''
  form.valid_until = ''
  form.tiers = []
  form.note = ''
}

async function loadList() {
  loading.value = true
  try {
    const resp = await adminAPI.rechargePromos.list(pagination.page, pagination.page_size)
    rows.value = resp.items ?? []
    pagination.total = resp.total ?? 0
  } catch (e) {
    appStore.showError(t('admin.rechargePromos.loadFailed'))
  } finally {
    loading.value = false
  }
}

function openCreateDialog() {
  resetForm()
  editingId.value = null
  // 初始默认一档，方便填写
  form.tiers = [{ min_amount: 100, bonus_rate: 0.05 }]
  showFormDialog.value = true
}

function openEditDialog(row: RechargePromoActivity) {
  editingId.value = row.id
  form.name = row.name
  form.enabled = row.enabled
  form.valid_from = row.valid_from ? toLocalInput(row.valid_from) : ''
  form.valid_until = row.valid_until ? toLocalInput(row.valid_until) : ''
  form.tiers = row.tiers.map((tier) => ({ min_amount: tier.min_amount, bonus_rate: tier.bonus_rate }))
  form.note = row.note ?? ''
  showFormDialog.value = true
}

function addTier() {
  const last = form.tiers[form.tiers.length - 1]
  const nextMin = last ? Number(last.min_amount) + 100 : 100
  form.tiers.push({ min_amount: nextMin, bonus_rate: 0.05 })
}

function removeTier(idx: number) {
  form.tiers.splice(idx, 1)
}

function toLocalInput(iso: string): string {
  // datetime-local input expects "YYYY-MM-DDTHH:mm" without timezone
  const d = new Date(iso)
  if (isNaN(d.getTime())) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function fromLocalInput(v: string): string | null {
  if (!v) return null
  const d = new Date(v)
  if (isNaN(d.getTime())) return null
  return d.toISOString()
}

async function submitForm() {
  const error = validateRechargePromoForm(form)
  if (error) {
    appStore.showError(t(`admin.rechargePromos.errors.${error}`))
    return
  }
  // 校验通过后再做 sort —— 用户输入已经升序，这里只是把 string number 归一化为数字。
  const sortedTiers: RechargePromoTier[] = [...form.tiers]
    .map((tier) => ({ min_amount: Number(tier.min_amount), bonus_rate: Number(tier.bonus_rate) }))
    .sort((a, b) => a.min_amount - b.min_amount)

  const payload: CreateOrUpdateRechargePromoRequest = {
    name: form.name.trim(),
    enabled: form.enabled,
    valid_from: fromLocalInput(form.valid_from),
    valid_until: fromLocalInput(form.valid_until),
    tiers: sortedTiers,
    note: form.note.trim() || null
  }
  submitting.value = true
  try {
    if (editingId.value === null) {
      await adminAPI.rechargePromos.create(payload)
      appStore.showSuccess(t('admin.rechargePromos.created'))
    } else {
      await adminAPI.rechargePromos.update(editingId.value, payload)
      appStore.showSuccess(t('admin.rechargePromos.updated'))
    }
    showFormDialog.value = false
    await loadList()
  } catch (e: unknown) {
    const msg = (e as { message?: string })?.message ?? t('admin.rechargePromos.saveFailed')
    appStore.showError(msg)
  } finally {
    submitting.value = false
  }
}

async function onToggle(row: RechargePromoActivity) {
  try {
    await adminAPI.rechargePromos.toggle(row.id, !row.enabled)
    appStore.showSuccess(t('admin.rechargePromos.toggled'))
    await loadList()
  } catch (e: unknown) {
    const msg = (e as { message?: string })?.message ?? t('admin.rechargePromos.toggleFailed')
    appStore.showError(msg)
  }
}

function askDelete(row: RechargePromoActivity) {
  deletingId.value = row.id
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (deletingId.value === null) return
  try {
    await adminAPI.rechargePromos.delete(deletingId.value)
    appStore.showSuccess(t('admin.rechargePromos.deleted'))
    showDeleteDialog.value = false
    deletingId.value = null
    await loadList()
  } catch (e: unknown) {
    const msg = (e as { message?: string })?.message ?? t('admin.rechargePromos.deleteFailed')
    appStore.showError(msg)
  }
}

onMounted(() => {
  loadList()
})
</script>
