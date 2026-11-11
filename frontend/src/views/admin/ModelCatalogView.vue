<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div
          class="flex items-start gap-2 rounded-lg border border-blue-200 bg-blue-50 px-4 py-3 text-sm text-blue-800 dark:border-blue-800/50 dark:bg-blue-900/20 dark:text-blue-300"
        >
          <Icon name="infoCircle" size="md" class="mt-0.5 flex-shrink-0" />
          <div class="space-y-1">
            <p>{{ t('admin.modelCatalog.helpPricing') }}</p>
            <p>{{ t('admin.modelCatalog.helpEnabled') }}</p>
          </div>
        </div>
      </template>

      <template #filters>
        <div class="flex flex-col justify-between gap-4 lg:flex-row lg:items-start">
          <!-- Left: Search + Filters -->
          <div class="flex flex-1 flex-wrap items-center gap-3">
            <div class="relative w-full sm:w-64">
              <Icon
                name="search"
                size="md"
                class="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400 dark:text-gray-500"
              />
              <input
                v-model="searchQuery"
                type="text"
                :placeholder="t('admin.modelCatalog.searchPlaceholder')"
                class="input pl-10"
                @input="handleSearch"
              />
            </div>

            <Select
              v-model="filters.platform"
              :options="platformFilterOptions"
              :placeholder="t('admin.modelCatalog.allPlatforms')"
              class="w-40"
              @change="loadEntries"
            />

            <Select
              v-model="filters.vendor"
              :options="vendorFilterOptions"
              :placeholder="t('admin.modelCatalog.allVendors')"
              class="w-40"
              @change="loadEntries"
            />

            <Select
              v-model="filters.enabled"
              :options="enabledFilterOptions"
              :placeholder="t('admin.modelCatalog.allStatus')"
              class="w-36"
              @change="loadEntries"
            />
          </div>

          <!-- Right: Actions -->
          <div class="flex w-full flex-shrink-0 flex-wrap items-center justify-end gap-3 lg:w-auto">
            <button
              :disabled="loading"
              class="btn btn-secondary"
              :title="t('common.refresh', 'Refresh')"
              @click="loadEntries"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button class="btn btn-primary" @click="openCreateDialog">
              <Icon name="plus" size="md" class="mr-2" />
              {{ t('admin.modelCatalog.createModel') }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="entries" :loading="loading" row-key="id">
          <template #cell-model_id="{ row }">
            <div class="flex items-center gap-2">
              <ModelIcon :model="row.model_id" size="16px" />
              <span class="font-mono text-sm font-medium text-gray-900 dark:text-white">{{ row.model_id }}</span>
            </div>
          </template>

          <template #cell-display_name="{ row }">
            <div class="max-w-[16rem]">
              <div class="text-sm text-gray-900 dark:text-gray-100">{{ row.display_name || '-' }}</div>
              <div
                v-if="row.description"
                class="truncate text-xs text-gray-500 dark:text-gray-400"
                :title="row.description"
              >
                {{ row.description }}
              </div>
            </div>
          </template>

          <template #cell-platform="{ row }">
            <span
              class="inline-flex items-center gap-1 rounded px-2 py-0.5 text-xs font-medium"
              :class="platformBadgeLightClass(row.platform)"
            >
              <PlatformIcon :platform="row.platform" size="xs" />
              {{ platformLabel(row.platform) }}
            </span>
          </template>

          <template #cell-vendor="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ value || '-' }}</span>
          </template>

          <template #cell-input_price="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatPrice(value) }}</span>
          </template>
          <template #cell-output_price="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatPrice(value) }}</span>
          </template>
          <template #cell-cache_write_price="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatPrice(value) }}</span>
          </template>
          <template #cell-cache_read_price="{ value }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ formatPrice(value) }}</span>
          </template>

          <template #cell-context_window="{ row }">
            <span
              class="text-sm text-gray-600 dark:text-gray-400"
              :title="row.max_output_tokens ? `max output: ${row.max_output_tokens.toLocaleString()}` : undefined"
            >
              {{ formatTokens(row.context_window) }}
            </span>
          </template>

          <template #cell-released_at="{ value }">
            <span class="text-sm text-gray-600 dark:text-gray-400">{{ normalizeDate(value) || '-' }}</span>
          </template>

          <template #cell-enabled="{ row }">
            <Toggle :model-value="row.enabled" @update:model-value="toggleEnabled(row)" />
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-primary-600 dark:hover:bg-dark-700 dark:hover:text-primary-400"
                @click="openEditDialog(row)"
              >
                <Icon name="edit" size="sm" />
                <span class="text-xs">{{ t('common.edit', 'Edit') }}</span>
              </button>
              <button
                class="flex flex-col items-center gap-0.5 rounded-lg p-1.5 text-gray-500 transition-colors hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20 dark:hover:text-red-400"
                @click="handleDelete(row)"
              >
                <Icon name="trash" size="sm" />
                <span class="text-xs">{{ t('common.delete', 'Delete') }}</span>
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState
              :title="t('admin.modelCatalog.noModelsYet')"
              :description="t('admin.modelCatalog.createFirstModel')"
              :action-text="t('admin.modelCatalog.createModel')"
              @action="openCreateDialog"
            />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <!-- Create/Edit Dialog -->
    <BaseDialog
      :show="showDialog"
      :title="editingEntry ? t('admin.modelCatalog.editModel') : t('admin.modelCatalog.createModel')"
      width="wide"
      @close="closeDialog"
    >
      <form id="model-catalog-form" class="space-y-5" @submit.prevent="handleSubmit">
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <!-- Model ID -->
          <div>
            <label class="input-label">
              {{ t('admin.modelCatalog.form.modelId') }} <span class="text-red-500">*</span>
            </label>
            <input
              v-model="form.model_id"
              type="text"
              required
              class="input font-mono"
              :placeholder="t('admin.modelCatalog.form.modelIdPlaceholder')"
            />
            <p class="input-hint">{{ t('admin.modelCatalog.form.modelIdHint') }}</p>
          </div>

          <!-- Platform -->
          <div>
            <label class="input-label">
              {{ t('admin.modelCatalog.form.platform') }} <span class="text-red-500">*</span>
            </label>
            <Select v-model="form.platform" :options="platformOptions" />
          </div>

          <!-- Vendor -->
          <div>
            <label class="input-label">{{ t('admin.modelCatalog.form.vendor') }}</label>
            <input
              v-model="form.vendor"
              type="text"
              list="model-catalog-vendor-suggestions"
              class="input"
              :placeholder="t('admin.modelCatalog.form.vendorPlaceholder')"
            />
            <datalist id="model-catalog-vendor-suggestions">
              <option v-for="v in vendorSuggestions" :key="v" :value="v" />
            </datalist>
            <p class="input-hint">{{ t('admin.modelCatalog.form.vendorHint') }}</p>
          </div>

          <!-- Display name -->
          <div>
            <label class="input-label">{{ t('admin.modelCatalog.form.displayName') }}</label>
            <input
              v-model="form.display_name"
              type="text"
              class="input"
              :placeholder="t('admin.modelCatalog.form.displayNamePlaceholder')"
            />
          </div>
        </div>

        <!-- Description -->
        <div>
          <label class="input-label">{{ t('admin.modelCatalog.form.description') }}</label>
          <textarea
            v-model="form.description"
            rows="2"
            class="input"
            :placeholder="t('admin.modelCatalog.form.descriptionPlaceholder')"
          ></textarea>
        </div>

        <!-- Prices ($/MTok) -->
        <div>
          <label class="input-label">
            {{ t('admin.modelCatalog.form.prices') }}
            <span class="ml-1 font-normal text-gray-400">$/MTok</span>
          </label>
          <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
            <div v-for="field in priceFields" :key="field.key">
              <label class="text-xs text-gray-400">{{ field.label }}</label>
              <input
                v-model="form[field.key]"
                type="number"
                step="any"
                min="0"
                class="input mt-0.5 text-sm"
                :placeholder="t('admin.modelCatalog.form.pricePlaceholder')"
              />
            </div>
          </div>
          <p class="input-hint">{{ t('admin.modelCatalog.form.pricesHint') }}</p>
        </div>

        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <!-- Context window -->
          <div>
            <label class="input-label">{{ t('admin.modelCatalog.form.contextWindow') }}</label>
            <input v-model="form.context_window" type="number" min="0" step="1" class="input" placeholder="0" />
            <p class="input-hint">{{ t('admin.modelCatalog.form.tokensHint') }}</p>
          </div>

          <!-- Max output tokens -->
          <div>
            <label class="input-label">{{ t('admin.modelCatalog.form.maxOutputTokens') }}</label>
            <input v-model="form.max_output_tokens" type="number" min="0" step="1" class="input" placeholder="0" />
            <p class="input-hint">{{ t('admin.modelCatalog.form.tokensHint') }}</p>
          </div>

          <!-- Released at -->
          <div>
            <label class="input-label">{{ t('admin.modelCatalog.form.releasedAt') }}</label>
            <input v-model="form.released_at" type="date" class="input" />
          </div>

          <!-- Sort order -->
          <div>
            <label class="input-label">{{ t('admin.modelCatalog.form.sortOrder') }}</label>
            <input v-model="form.sort_order" type="number" step="1" class="input" placeholder="0" />
            <p class="input-hint">{{ t('admin.modelCatalog.form.sortOrderHint') }}</p>
          </div>
        </div>

        <!-- Enabled -->
        <div class="flex items-center justify-between border-t border-gray-200 pt-4 dark:border-dark-700">
          <div>
            <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
              {{ t('admin.modelCatalog.form.enabled') }}
            </label>
            <p class="mt-0.5 text-xs text-gray-400">{{ t('admin.modelCatalog.form.enabledHint') }}</p>
          </div>
          <Toggle v-model="form.enabled" />
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" @click="closeDialog">
            {{ t('common.cancel', 'Cancel') }}
          </button>
          <button type="submit" form="model-catalog-form" :disabled="submitting" class="btn btn-primary">
            {{
              submitting
                ? t('common.submitting', 'Submitting...')
                : editingEntry
                  ? t('common.update', 'Update')
                  : t('common.create', 'Create')
            }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!-- Delete Confirmation -->
    <ConfirmDialog
      :show="showDeleteDialog"
      :title="t('admin.modelCatalog.deleteModel')"
      :message="t('admin.modelCatalog.deleteConfirm', { model: deletingEntry?.model_id ?? '' })"
      :confirm-text="t('common.delete', 'Delete')"
      :cancel-text="t('common.cancel', 'Cancel')"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="showDeleteDialog = false"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { adminAPI } from '@/api/admin'
import {
  MODEL_CATALOG_PLATFORMS,
  MODEL_CATALOG_VENDOR_SUGGESTIONS,
  type ModelCatalogEntry,
  type ModelCatalogPlatform,
  type ModelCatalogRequest
} from '@/api/admin/modelCatalog'
import { mTokToPerToken, perTokenToMTok, toNullableNumber } from '@/components/admin/channel/types'
import { reloadCatalogModels } from '@/composables/useModelWhitelist'
import type { Column } from '@/components/common/types'
import { platformBadgeLightClass, platformLabel } from '@/utils/platformColors'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

type PriceKey = 'input_price' | 'output_price' | 'cache_write_price' | 'cache_read_price'

interface CatalogForm {
  model_id: string
  platform: ModelCatalogPlatform
  vendor: string
  display_name: string
  description: string
  // 显示单位 $/MTok；'' / null = 不覆盖
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  context_window: number | string
  max_output_tokens: number | string
  released_at: string
  enabled: boolean
  sort_order: number | string
}

// ── Table columns ──
const columns = computed<Column[]>(() => [
  { key: 'model_id', label: t('admin.modelCatalog.columns.modelId') },
  { key: 'display_name', label: t('admin.modelCatalog.columns.displayName') },
  { key: 'platform', label: t('admin.modelCatalog.columns.platform') },
  { key: 'vendor', label: t('admin.modelCatalog.columns.vendor') },
  { key: 'input_price', label: `${t('admin.modelCatalog.columns.inputPrice')} ($/MTok)` },
  { key: 'output_price', label: `${t('admin.modelCatalog.columns.outputPrice')} ($/MTok)` },
  { key: 'cache_write_price', label: `${t('admin.modelCatalog.columns.cacheWritePrice')} ($/MTok)` },
  { key: 'cache_read_price', label: `${t('admin.modelCatalog.columns.cacheReadPrice')} ($/MTok)` },
  { key: 'context_window', label: t('admin.modelCatalog.columns.contextWindow') },
  { key: 'released_at', label: t('admin.modelCatalog.columns.releasedAt') },
  { key: 'enabled', label: t('admin.modelCatalog.columns.enabled') },
  { key: 'actions', label: t('admin.modelCatalog.columns.actions') }
])

const priceFields = computed<{ key: PriceKey; label: string }[]>(() => [
  { key: 'input_price', label: t('admin.modelCatalog.form.inputPrice') },
  { key: 'output_price', label: t('admin.modelCatalog.form.outputPrice') },
  { key: 'cache_write_price', label: t('admin.modelCatalog.form.cacheWritePrice') },
  { key: 'cache_read_price', label: t('admin.modelCatalog.form.cacheReadPrice') }
])

const platformOptions = computed(() =>
  MODEL_CATALOG_PLATFORMS.map((p) => ({ value: p, label: platformLabel(p) }))
)

const platformFilterOptions = computed(() => [
  { value: '', label: t('admin.modelCatalog.allPlatforms') },
  ...platformOptions.value
])

const enabledFilterOptions = computed(() => [
  { value: '', label: t('admin.modelCatalog.allStatus') },
  { value: 'true', label: t('admin.modelCatalog.statusEnabled') },
  { value: 'false', label: t('admin.modelCatalog.statusDisabled') }
])

// ── State ──
const entries = ref<ModelCatalogEntry[]>([])
const loading = ref(false)
const searchQuery = ref('')
const filters = reactive({ platform: '', vendor: '', enabled: '' })
// 记录出现过的 vendor，供过滤器 / 表单建议使用
const seenVendors = ref<string[]>([])

const vendorSuggestions = computed(() => {
  const set = new Set<string>(MODEL_CATALOG_VENDOR_SUGGESTIONS)
  for (const v of seenVendors.value) set.add(v)
  return Array.from(set)
})

const vendorFilterOptions = computed(() => [
  { value: '', label: t('admin.modelCatalog.allVendors') },
  ...vendorSuggestions.value.map((v) => ({ value: v, label: v }))
])

const showDialog = ref(false)
const editingEntry = ref<ModelCatalogEntry | null>(null)
const submitting = ref(false)
const showDeleteDialog = ref(false)
const deletingEntry = ref<ModelCatalogEntry | null>(null)

function emptyForm(): CatalogForm {
  return {
    model_id: '',
    platform: 'anthropic',
    vendor: 'anthropic',
    display_name: '',
    description: '',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    context_window: 0,
    max_output_tokens: 0,
    released_at: '',
    enabled: true,
    sort_order: 0
  }
}

const form = reactive<CatalogForm>(emptyForm())

// ── Helpers ──
function formatPrice(value: number | null | undefined): string {
  const mtok = perTokenToMTok(value)
  return mtok === null ? '-' : `$${mtok}`
}

function formatTokens(value: number | null | undefined): string {
  if (!value || value <= 0) return '-'
  if (value >= 1_000_000 && value % 1_000_000 === 0) return `${value / 1_000_000}M`
  if (value >= 1000) return `${parseFloat((value / 1000).toFixed(1))}K`
  return String(value)
}

/** API 可能返回 YYYY-MM-DD 或完整时间戳，统一截取日期部分 */
function normalizeDate(value: string | null | undefined): string {
  if (!value) return ''
  return value.slice(0, 10)
}

function toNonNegativeInt(value: number | string | null | undefined): number | null {
  if (value === '' || value === null || value === undefined) return 0
  const num = Number(value)
  return Number.isInteger(num) && num >= 0 ? num : null
}

function entryToRequest(entry: ModelCatalogEntry): ModelCatalogRequest {
  return {
    model_id: entry.model_id,
    platform: entry.platform,
    vendor: entry.vendor ?? '',
    display_name: entry.display_name ?? '',
    description: entry.description ?? '',
    input_price: entry.input_price ?? null,
    output_price: entry.output_price ?? null,
    cache_write_price: entry.cache_write_price ?? null,
    cache_read_price: entry.cache_read_price ?? null,
    context_window: entry.context_window ?? 0,
    max_output_tokens: entry.max_output_tokens ?? 0,
    released_at: normalizeDate(entry.released_at) || null,
    enabled: entry.enabled,
    sort_order: entry.sort_order ?? 0
  }
}

/** 校验并构造请求；失败时 toast 并返回 null */
function buildRequestFromForm(): ModelCatalogRequest | null {
  const modelId = form.model_id.trim()
  if (!modelId) {
    appStore.showError(t('admin.modelCatalog.validation.modelIdRequired'))
    return null
  }
  if (/[\s*]/.test(modelId)) {
    appStore.showError(t('admin.modelCatalog.validation.modelIdInvalid'))
    return null
  }
  if (!MODEL_CATALOG_PLATFORMS.includes(form.platform)) {
    appStore.showError(t('admin.modelCatalog.validation.platformRequired'))
    return null
  }

  const prices = {} as Record<PriceKey, number | null>
  for (const { key } of priceFields.value) {
    const raw = form[key]
    const num = toNullableNumber(raw)
    if (raw !== null && raw !== '' && (num === null || num < 0)) {
      appStore.showError(t('admin.modelCatalog.validation.priceInvalid'))
      return null
    }
    prices[key] = mTokToPerToken(num)
  }

  const contextWindow = toNonNegativeInt(form.context_window)
  const maxOutput = toNonNegativeInt(form.max_output_tokens)
  if (contextWindow === null || maxOutput === null) {
    appStore.showError(t('admin.modelCatalog.validation.tokensInvalid'))
    return null
  }

  const sortOrderNum = Number(form.sort_order === '' ? 0 : form.sort_order)

  return {
    model_id: modelId,
    platform: form.platform,
    vendor: form.vendor.trim(),
    display_name: form.display_name.trim(),
    description: form.description.trim(),
    ...prices,
    context_window: contextWindow,
    max_output_tokens: maxOutput,
    released_at: form.released_at ? form.released_at : null,
    enabled: form.enabled,
    sort_order: Number.isFinite(sortOrderNum) ? Math.trunc(sortOrderNum) : 0
  }
}

// ── Load ──
let loadSeq = 0

async function loadEntries() {
  const seq = ++loadSeq
  loading.value = true
  try {
    const res = await adminAPI.modelCatalog.list({
      platform: (filters.platform || undefined) as ModelCatalogPlatform | undefined,
      vendor: filters.vendor || undefined,
      search: searchQuery.value.trim() || undefined,
      enabled: filters.enabled === '' ? undefined : filters.enabled === 'true'
    })
    if (seq !== loadSeq) return
    entries.value = res.items || []
    const vendors = new Set(seenVendors.value)
    for (const item of entries.value) {
      if (item.vendor) vendors.add(item.vendor)
    }
    seenVendors.value = Array.from(vendors)
  } catch (error: unknown) {
    if (seq !== loadSeq) return
    appStore.showError(extractApiErrorMessage(error, t('admin.modelCatalog.loadError')))
  } finally {
    if (seq === loadSeq) loading.value = false
  }
}

let searchTimeout: ReturnType<typeof setTimeout> | undefined
function handleSearch() {
  clearTimeout(searchTimeout)
  searchTimeout = setTimeout(() => {
    loadEntries()
  }, 300)
}

// ── Dialog ──
function openCreateDialog() {
  editingEntry.value = null
  Object.assign(form, emptyForm())
  showDialog.value = true
}

function openEditDialog(entry: ModelCatalogEntry) {
  editingEntry.value = entry
  Object.assign(form, {
    model_id: entry.model_id,
    platform: entry.platform,
    vendor: entry.vendor ?? '',
    display_name: entry.display_name ?? '',
    description: entry.description ?? '',
    input_price: perTokenToMTok(entry.input_price),
    output_price: perTokenToMTok(entry.output_price),
    cache_write_price: perTokenToMTok(entry.cache_write_price),
    cache_read_price: perTokenToMTok(entry.cache_read_price),
    context_window: entry.context_window ?? 0,
    max_output_tokens: entry.max_output_tokens ?? 0,
    released_at: normalizeDate(entry.released_at),
    enabled: entry.enabled,
    sort_order: entry.sort_order ?? 0
  } satisfies CatalogForm)
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
  editingEntry.value = null
}

async function handleSubmit() {
  if (submitting.value) return
  const req = buildRequestFromForm()
  if (!req) return

  submitting.value = true
  const isEdit = !!editingEntry.value
  try {
    if (editingEntry.value) {
      await adminAPI.modelCatalog.update(editingEntry.value.id, req)
      appStore.showSuccess(t('admin.modelCatalog.updateSuccess'))
    } else {
      await adminAPI.modelCatalog.create(req)
      appStore.showSuccess(t('admin.modelCatalog.createSuccess'))
    }
    closeDialog()
    void reloadCatalogModels()
    await loadEntries()
  } catch (error: unknown) {
    appStore.showError(
      extractApiErrorMessage(
        error,
        isEdit ? t('admin.modelCatalog.updateError') : t('admin.modelCatalog.createError')
      )
    )
  } finally {
    submitting.value = false
  }
}

// ── Toggle enabled (full replace PUT) ──
async function toggleEnabled(entry: ModelCatalogEntry) {
  const next = !entry.enabled
  try {
    const updated = await adminAPI.modelCatalog.update(entry.id, { ...entryToRequest(entry), enabled: next })
    void reloadCatalogModels()
    if (filters.enabled !== '' && (filters.enabled === 'true') !== next) {
      await loadEntries()
    } else {
      Object.assign(entry, updated ?? { enabled: next })
    }
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelCatalog.updateError')))
  }
}

// ── Delete ──
function handleDelete(entry: ModelCatalogEntry) {
  deletingEntry.value = entry
  showDeleteDialog.value = true
}

async function confirmDelete() {
  if (!deletingEntry.value) return
  try {
    await adminAPI.modelCatalog.remove(deletingEntry.value.id)
    appStore.showSuccess(t('admin.modelCatalog.deleteSuccess'))
    showDeleteDialog.value = false
    deletingEntry.value = null
    void reloadCatalogModels()
    await loadEntries()
  } catch (error: unknown) {
    appStore.showError(extractApiErrorMessage(error, t('admin.modelCatalog.deleteError')))
  }
}

// ── Lifecycle ──
onMounted(() => {
  loadEntries()
})

onUnmounted(() => {
  clearTimeout(searchTimeout)
})
</script>
