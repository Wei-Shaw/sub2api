<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div>
            <h1 class="text-xl font-semibold text-gray-900 dark:text-white">公开价格导出配置</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              这里维护 `/api/provider/pricing` 的对外展示价格。金额单位固定为 CNY / 每百万 tokens，不影响真实计费。
            </p>
          </div>
          <div class="flex items-center gap-3">
            <span v-if="saveSuccessMessage" class="text-sm font-medium text-emerald-600 dark:text-emerald-400">
              {{ saveSuccessMessage }}
            </span>
            <button type="button" class="btn btn-secondary" :disabled="loading || saving" @click="load">
              刷新
            </button>
            <button type="button" class="btn btn-secondary" :disabled="saving" @click="addRow">
              新增
            </button>
            <button type="button" class="btn btn-primary" :disabled="loading || saving" @click="save">
              {{ saving ? '保存中...' : '保存配置' }}
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div v-if="formError" class="mb-4 rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200">
          {{ formError }}
        </div>
        <div v-if="saveSuccessMessage" class="mb-4 rounded-lg border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-200">
          {{ saveSuccessMessage }}
        </div>

        <div class="space-y-4">
          <div
            v-for="(item, index) in rows"
            :id="`provider-price-row-${item.local_id}`"
            :key="item.local_id"
            class="rounded-xl border bg-white p-4 dark:bg-dark-900"
            :class="item.local_id === invalidRowLocalId ? 'border-red-300 shadow-sm shadow-red-100 dark:border-red-500/50' : 'border-gray-200 dark:border-dark-700'"
          >
            <div class="mb-4 flex items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <input v-model="item.enabled" type="checkbox" class="h-4 w-4 rounded border-gray-300 text-primary-600" />
                <span class="text-sm font-medium text-gray-900 dark:text-white">价格覆盖项 {{ index + 1 }}</span>
              </div>
              <button type="button" class="btn btn-secondary btn-sm" @click="removeRow(index)">
                删除
              </button>
            </div>

            <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">分组名称</label>
                <input v-model.trim="item.group_name" type="text" class="input" placeholder="例如 default" />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">模型名</label>
                <input v-model.trim="item.model_name" type="text" class="input" placeholder="输入完整模型名" />
              </div>
            </div>

            <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-5">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">输入价格</label>
                <input v-model="item.input_price_text" type="number" step="0.000001" min="0" class="input" />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">输出</label>
                <input v-model="item.output_price_text" type="number" step="0.000001" min="0" class="input" />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">缓存写入</label>
                <input v-model="item.cache_create_price_text" type="number" step="0.000001" min="0" class="input" />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">缓存读取</label>
                <input v-model="item.cache_input_price_text" type="number" step="0.000001" min="0" class="input" />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">缓存写入 1H</label>
                <input v-model="item.cache_create_price_1h_text" type="number" step="0.000001" min="0" class="input" />
              </div>
            </div>

            <div class="mt-4 grid grid-cols-1 gap-4 md:grid-cols-[minmax(0,1fr)_120px]">
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">备注</label>
                <input v-model.trim="item.note" type="text" class="input" placeholder="例如 活动价 / 推荐价" />
              </div>
              <div>
                <label class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-300">排序</label>
                <input v-model.number="item.sort_order" type="number" step="1" class="input" />
              </div>
            </div>

            <p
              v-if="item.local_id === invalidRowLocalId && formError"
              class="mt-4 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
            >
              {{ formError }}
            </p>
          </div>

          <div v-if="rows.length === 0" class="rounded-xl border border-dashed border-gray-300 bg-white px-6 py-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:bg-dark-900 dark:text-dark-300">
            暂无 override，当前 `/api/provider/pricing` 会走自动推导逻辑。
          </div>
        </div>
      </template>
    </TablePageLayout>
  </AppLayout>
</template>

<script setup lang="ts">
import { nextTick, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import { adminAPI } from '@/api'
import { useAppStore } from '@/stores'
import type { ProviderPriceOverride } from '@/api/admin/providerPricing'

type EditableRow = ProviderPriceOverride & {
  local_id: string
  input_price_text: string
  output_price_text: string
  cache_input_price_text: string
  cache_create_price_text: string
  cache_create_price_1h_text: string
}

const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const rows = ref<EditableRow[]>([])
const formError = ref('')
const saveSuccessMessage = ref('')
const invalidRowLocalId = ref('')

function rowToEditable(item: ProviderPriceOverride, index: number): EditableRow {
  return {
    ...item,
    local_id: `${item.id || 'row'}-${index}-${Date.now()}`,
    input_price_text: item.input_price == null ? '' : String(item.input_price),
    output_price_text: item.output_price == null ? '' : String(item.output_price),
    cache_input_price_text: item.cache_input_price == null ? '' : String(item.cache_input_price),
    cache_create_price_text: item.cache_create_price == null ? '' : String(item.cache_create_price),
    cache_create_price_1h_text: item.cache_create_price_1h == null ? '' : String(item.cache_create_price_1h),
  }
}

function parsePrice(value: string | number | null | undefined): number | null {
  if (value == null) return null
  const trimmed = String(value).trim()
  if (!trimmed) return null
  const num = Number(trimmed)
  return Number.isFinite(num) ? num : null
}

async function load() {
  loading.value = true
  formError.value = ''
  invalidRowLocalId.value = ''
  try {
    const items = await adminAPI.providerPricing.listOverrides()
    rows.value = items.map((item, index) => rowToEditable(item, index))
  } catch (error: unknown) {
    appStore.showError(String((error as { message?: string })?.message || '加载失败'))
  } finally {
    loading.value = false
  }
}

function addRow() {
  formError.value = ''
  saveSuccessMessage.value = ''
  invalidRowLocalId.value = ''
  rows.value.push(rowToEditable({
    id: '',
    group_name: '',
    model_name: '',
    input_price: null,
    output_price: null,
    cache_input_price: null,
    cache_create_price: null,
    cache_create_price_1h: null,
    enabled: true,
    note: '',
    sort_order: rows.value.length,
  }, rows.value.length))
}

function removeRow(index: number) {
  saveSuccessMessage.value = ''
  if (rows.value[index]?.local_id === invalidRowLocalId.value) {
    invalidRowLocalId.value = ''
    formError.value = ''
  }
  rows.value.splice(index, 1)
}

function isBlankRow(item: ProviderPriceOverride): boolean {
  return !item.group_name &&
    !item.model_name &&
    !item.note &&
    ![
      item.input_price,
      item.output_price,
      item.cache_input_price,
      item.cache_create_price,
      item.cache_create_price_1h,
    ].some((value) => typeof value === 'number' && value > 0)
}

function getInvalidReason(item: ProviderPriceOverride): string | null {
  if (!item.group_name) {
    return '分组名称必填。'
  }
  if (!item.model_name) {
    return '模型名必填。'
  }
  if (![
    item.input_price,
    item.output_price,
    item.cache_input_price,
    item.cache_create_price,
    item.cache_create_price_1h,
  ].some((value) => typeof value === 'number' && value > 0)) {
    return '至少要填写一个大于 0 的价格。'
  }
  return null
}

async function focusInvalidRow(localId: string) {
  await nextTick()
  document.getElementById(`provider-price-row-${localId}`)?.scrollIntoView({
    behavior: 'smooth',
    block: 'center',
  })
}

async function save() {
  formError.value = ''
  saveSuccessMessage.value = ''
  invalidRowLocalId.value = ''
  const payload = rows.value.map((item, index) => ({
    local_id: item.local_id,
    id: item.id.trim(),
    group_name: item.group_name.trim(),
    model_name: item.model_name.trim(),
    input_price: parsePrice(item.input_price_text),
    output_price: parsePrice(item.output_price_text),
    cache_input_price: parsePrice(item.cache_input_price_text),
    cache_create_price: parsePrice(item.cache_create_price_text),
    cache_create_price_1h: parsePrice(item.cache_create_price_1h_text),
    enabled: item.enabled,
    note: item.note?.trim() || '',
    sort_order: Number.isFinite(item.sort_order) ? item.sort_order : index,
  }))

  const nonBlankRows = payload.filter((item) => !isBlankRow(item))
  const ignoredBlankCount = payload.length - nonBlankRows.length
  const invalidRow = nonBlankRows.find((item) => getInvalidReason(item) !== null)
  if (invalidRow) {
    invalidRowLocalId.value = invalidRow.local_id
    formError.value = `第 ${rows.value.findIndex((item) => item.local_id === invalidRow.local_id) + 1} 行填写不完整：${getInvalidReason(invalidRow)}`
    appStore.showError(formError.value)
    await focusInvalidRow(invalidRow.local_id)
    return
  }

  saving.value = true
  try {
    const updated = await adminAPI.providerPricing.updateOverrides(nonBlankRows.map(({ local_id, ...item }) => item))
    rows.value = updated.map((item, index) => rowToEditable(item, index))
    const savedAt = new Date().toLocaleTimeString('zh-CN', { hour12: false })
    saveSuccessMessage.value = ignoredBlankCount > 0
      ? `公开价格导出配置已保存 ${savedAt}，并忽略了 ${ignoredBlankCount} 条空白项。`
      : `公开价格导出配置已保存 ${savedAt}`
    appStore.showSuccess('公开价格导出配置已保存')
  } catch (error: unknown) {
    formError.value = String((error as { message?: string })?.message || '保存失败')
    appStore.showError(formError.value)
  } finally {
    saving.value = false
  }
}

load()
</script>
