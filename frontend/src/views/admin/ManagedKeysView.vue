<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex flex-wrap items-center gap-3">
            <div class="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-300">
              托管 Key：{{ pagination.total }}
            </div>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <button
              type="button"
              class="btn btn-secondary px-2 md:px-3"
              :disabled="loading"
              title="刷新"
              @click="loadManagedKeys"
            >
              <Icon name="refresh" size="md" :class="loading ? 'animate-spin' : ''" />
            </button>
            <button type="button" class="btn btn-primary" @click="openCreateDialog">
              <Icon name="plus" size="md" class="mr-2" />
              新建托管 Key
            </button>
          </div>
        </div>
      </template>

      <template #table>
        <div class="table-wrapper">
          <table v-if="loading || managedKeys.length > 0">
            <thead>
              <tr>
                <th>客户</th>
                <th>API Key</th>
                <th>分组</th>
                <th>并发</th>
                <th>额度</th>
                <th>窗口用量</th>
                <th>策略</th>
                <th>到期</th>
                <th>最后使用</th>
                <th>状态</th>
                <th class="w-32">操作</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="loading">
                <td colspan="11" class="py-12 text-center text-gray-500 dark:text-dark-400">
                  加载中...
                </td>
              </tr>
              <tr v-for="item in managedKeys" v-else :key="item.user.id">
                <td>
                  <div class="min-w-52">
                    <div class="font-medium text-gray-900 dark:text-white">
                      {{ displayCustomerName(item) }}
                    </div>
                    <div class="mt-1 max-w-72 truncate text-xs text-gray-500 dark:text-dark-400">
                      {{ managedContact(item) || item.user.email }}
                    </div>
                  </div>
                </td>
                <td>
                  <div v-if="item.api_key" class="flex min-w-56 items-center gap-2">
                    <code class="rounded-md bg-gray-100 px-2 py-1 font-mono text-xs text-gray-800 dark:bg-dark-700 dark:text-dark-100">
                      {{ maskApiKey(item.api_key.key) }}
                    </code>
                    <button
                      type="button"
                      class="btn btn-ghost btn-sm px-2"
                      title="复制 API Key"
                      @click="copyText(item.api_key.key, `key-${item.api_key.id}`)"
                    >
                      <Icon :name="copiedField === `key-${item.api_key.id}` ? 'check' : 'copy'" size="sm" />
                    </button>
                  </div>
                  <span v-else class="text-sm text-gray-400">未创建</span>
                </td>
                <td>
                  <GroupBadge
                    v-if="groupFor(item)"
                    :name="groupFor(item)!.name"
                    :platform="groupFor(item)!.platform"
                    :subscription-type="groupFor(item)!.subscription_type"
                    :rate-multiplier="groupFor(item)!.rate_multiplier"
                  />
                  <span v-else class="text-sm text-gray-400">未绑定</span>
                </td>
                <td>
                  <span class="font-medium text-gray-900 dark:text-white">{{ item.user.concurrency }}</span>
                  <span v-if="item.user.rpm_limit" class="ml-2 text-xs text-gray-500 dark:text-dark-400">
                    {{ item.user.rpm_limit }} RPM
                  </span>
                </td>
                <td>
                  <div v-if="item.api_key" class="text-sm">
                    <span class="text-gray-900 dark:text-white">{{ formatCurrency(item.api_key.quota_used) }}</span>
                    <span class="text-gray-500 dark:text-dark-400">
                      / {{ item.api_key.quota > 0 ? formatCurrency(item.api_key.quota) : '不限' }}
                    </span>
                  </div>
                  <span v-else class="text-sm text-gray-400">-</span>
                </td>
                <td>
                  <div v-if="item.api_key" class="min-w-44 space-y-1 text-xs text-gray-600 dark:text-dark-300">
                    <div>{{ usageLine(item.api_key, '5h') }}</div>
                    <div>{{ usageLine(item.api_key, '1d') }}</div>
                    <div>{{ usageLine(item.api_key, '1mo') }}</div>
                  </div>
                  <span v-else class="text-sm text-gray-400">-</span>
                </td>
                <td>
                  <div v-if="item.api_key" class="min-w-32 space-y-1 text-xs text-gray-600 dark:text-dark-300">
                    <div>{{ ipLockLabel(item) }}</div>
                    <div>{{ limitActionLabel(item.api_key.limit_action) }}</div>
                  </div>
                  <span v-else class="text-sm text-gray-400">-</span>
                </td>
                <td>
                  <span class="text-sm text-gray-600 dark:text-dark-300">
                    {{ item.api_key?.expires_at ? formatDateTime(item.api_key.expires_at) : '长期' }}
                  </span>
                </td>
                <td>
                  <span class="text-sm text-gray-600 dark:text-dark-300">
                    {{ item.api_key?.last_used_at ? formatDateTime(item.api_key.last_used_at) : '从未' }}
                  </span>
                </td>
                <td>
                  <span :class="statusClass(item.api_key?.status)">
                    {{ statusLabel(item.api_key?.status) }}
                  </span>
                </td>
                <td>
                  <div v-if="item.api_key" class="flex items-center gap-1">
                    <button
                      type="button"
                      class="btn btn-ghost btn-sm px-2"
                      title="查看交付信息"
                      @click="showDeliveryForExisting(item)"
                    >
                      <Icon name="eye" size="sm" />
                    </button>
                    <button
                      type="button"
                      class="btn btn-ghost btn-sm px-2"
                      title="重置 IP 锁"
                      @click="resetIPLock(item)"
                    >
                      <Icon name="refresh" size="sm" />
                    </button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
          <div v-else class="flex min-h-96 items-center justify-center">
            <EmptyState
              title="还没有托管 Key"
              description="暂无记录"
              action-text="新建托管 Key"
              @action="openCreateDialog"
            />
          </div>
        </div>
      </template>

      <template #pagination>
        <Pagination
          v-model:page="pagination.page"
          v-model:page-size="pagination.page_size"
          :total="pagination.total"
          @update:page="loadManagedKeys"
          @update:page-size="handlePageSizeChange"
        />
      </template>
    </TablePageLayout>

    <BaseDialog
      :show="showCreateDialog"
      title="新建托管 Key"
      width="wide"
      @close="closeCreateDialog"
    >
      <form class="space-y-5" @submit.prevent="submitCreate">
        <div class="grid gap-4 md:grid-cols-2">
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">客户名称</span>
            <input v-model.trim="form.customer_name" class="input" required placeholder="例如 Oceanway 客户 A" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">联系方式</span>
            <input v-model.trim="form.contact" class="input" placeholder="微信、邮箱或备注名" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">Key 名称</span>
            <input v-model.trim="form.key_name" class="input" placeholder="留空则自动生成" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">绑定分组</span>
            <Select v-model="form.group_id" :options="groupOptions" searchable />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">并发数</span>
            <input v-model.number="form.concurrency" type="number" min="1" class="input" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">账户余额</span>
            <input v-model.number="form.balance" type="number" min="0" step="0.01" class="input" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">Key 额度</span>
            <input v-model.number="form.quota" type="number" min="0" step="0.01" class="input" placeholder="0 = 不限" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">每分钟限制</span>
            <input v-model.number="form.rpm_limit" type="number" min="0" class="input" placeholder="0 = 不限" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">有效天数</span>
            <input v-model.number="form.expires_in_days" type="number" min="0" class="input" placeholder="留空或 0 = 长期" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">自定义 Key</span>
            <input v-model.trim="form.custom_key" class="input font-mono" placeholder="留空则自动生成" />
          </label>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">IP 锁策略</span>
            <Select v-model="form.ip_lock_mode" :options="ipLockOptions" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">到量动作</span>
            <Select v-model="form.limit_action" :options="limitActionOptions" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">5 小时限额</span>
            <input v-model.number="form.rate_limit_5h" type="number" min="0" step="0.01" class="input" placeholder="0 = 不限" />
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">每日限额</span>
            <input v-model.number="form.rate_limit_1d" type="number" min="0" step="0.01" class="input" placeholder="0 = 不限" />
          </label>
          <label class="block md:col-span-2">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">每月限额</span>
            <input v-model.number="form.rate_limit_1mo" type="number" min="0" step="0.01" class="input" placeholder="0 = 不限" />
          </label>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">IP 白名单</span>
            <textarea v-model="form.ip_whitelist" rows="3" class="input resize-none" placeholder="每行一个 IP 或 CIDR"></textarea>
          </label>
          <label class="block">
            <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">IP 黑名单</span>
            <textarea v-model="form.ip_blacklist" rows="3" class="input resize-none" placeholder="每行一个 IP 或 CIDR"></textarea>
          </label>
        </div>

        <label class="block">
          <span class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-dark-200">内部备注</span>
          <textarea v-model="form.notes" rows="3" class="input resize-none" placeholder="仅管理员可见"></textarea>
        </label>

        <div v-if="formError" class="rounded-lg bg-red-50 px-3 py-2 text-sm text-red-600 dark:bg-red-900/20 dark:text-red-300">
          {{ formError }}
        </div>
      </form>

      <template #footer>
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="closeCreateDialog">
          取消
        </button>
        <button type="button" class="btn btn-primary" :disabled="submitting" @click="submitCreate">
          <Icon v-if="submitting" name="refresh" size="md" class="mr-2 animate-spin" />
          {{ submitting ? '创建中...' : '创建并生成 Key' }}
        </button>
      </template>
    </BaseDialog>

    <BaseDialog
      :show="deliveryDialogVisible"
      title="交付信息"
      width="wide"
      @close="closeDeliveryDialog"
    >
      <div v-if="deliveryData" class="space-y-4">
        <div class="grid gap-3 md:grid-cols-2">
          <DeliveryField
            label="API Key"
            :value="deliveryData.delivery.api_key"
            field-id="delivery-key"
            :copied-field="copiedField"
            @copy="copyText"
          />
          <DeliveryField
            label="Authorization"
            :value="deliveryData.delivery.authorization_header"
            field-id="delivery-auth"
            :copied-field="copiedField"
            @copy="copyText"
          />
          <DeliveryField
            label="OpenAI / Claude Base URL"
            :value="deliveryData.delivery.openai_base_url"
            field-id="delivery-v1"
            :copied-field="copiedField"
            @copy="copyText"
          />
          <DeliveryField
            label="Gemini Base URL"
            :value="deliveryData.delivery.gemini_base_url"
            field-id="delivery-gemini"
            :copied-field="copiedField"
            @copy="copyText"
          />
        </div>
        <div class="rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-700 dark:border-dark-700 dark:bg-dark-900/40 dark:text-dark-200">
          客户：{{ deliveryData.user.username || deliveryData.user.email }}，
          并发：{{ deliveryData.user.concurrency }}，
          Key 额度：{{ deliveryData.api_key.quota > 0 ? formatCurrency(deliveryData.api_key.quota) : '不限' }}。
        </div>
      </div>

      <template #footer>
        <button type="button" class="btn btn-primary" @click="closeDeliveryDialog">
          完成
        </button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, reactive, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import GroupBadge from '@/components/common/GroupBadge.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import type { CreateManagedKeyRequest, ManagedKey, ManagedKeyResponse } from '@/api/admin/apiKeys'
import type { AdminGroup } from '@/types'
import { useAppStore } from '@/stores'
import { extractApiErrorMessage } from '@/utils/apiError'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { maskApiKey } from '@/utils/maskApiKey'

const appStore = useAppStore()

const managedKeys = ref<ManagedKey[]>([])
const groups = ref<AdminGroup[]>([])
const loading = ref(false)
const submitting = ref(false)
const showCreateDialog = ref(false)
const deliveryDialogVisible = ref(false)
const deliveryData = ref<ManagedKeyResponse | null>(null)
const formError = ref('')
const copiedField = ref('')

const pagination = reactive({
  page: 1,
  page_size: 20,
  total: 0
})

function createEmptyForm() {
  return {
    customer_name: '',
    contact: '',
    key_name: '',
    group_id: null as number | null,
    balance: 1000,
    concurrency: 1,
    rpm_limit: 0,
    quota: 0,
    expires_in_days: null as number | null,
    custom_key: '',
    ip_lock_mode: 'auto_single_ip' as 'off' | 'auto_single_ip',
    limit_action: 'soft_throttle' as 'hard_block' | 'soft_throttle',
    rate_limit_5h: 0,
    rate_limit_1d: 0,
    rate_limit_1mo: 0,
    ip_whitelist: '',
    ip_blacklist: '',
    notes: ''
  }
}

const form = reactive(createEmptyForm())

const groupOptions = computed(() => [
  { value: null, label: '不绑定分组' },
  ...groups.value.map((group) => ({
    value: group.id,
    label: `${group.name} · ${group.platform}${group.subscription_type === 'subscription' ? ' · 订阅' : ''}`
  }))
])

const ipLockOptions = [
  { value: 'auto_single_ip', label: '自动锁定首个 IP' },
  { value: 'off', label: '关闭自动锁定' }
]

const limitActionOptions = [
  { value: 'soft_throttle', label: '到量后软降速' },
  { value: 'hard_block', label: '到量后拒绝' }
]

const groupById = computed(() => {
  const map = new Map<number, AdminGroup>()
  for (const group of groups.value) {
    map.set(group.id, group)
  }
  return map
})

async function loadGroups() {
  groups.value = await adminAPI.groups.getAll()
}

async function loadManagedKeys() {
  loading.value = true
  try {
    const result = await adminAPI.apiKeys.listManagedKeys(pagination.page, pagination.page_size)
    managedKeys.value = result.items
    pagination.total = result.total
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '加载托管 Key 失败'))
  } finally {
    loading.value = false
  }
}

function handlePageSizeChange() {
  pagination.page = 1
  loadManagedKeys()
}

function openCreateDialog() {
  resetForm()
  formError.value = ''
  showCreateDialog.value = true
}

function closeCreateDialog() {
  if (!submitting.value) {
    showCreateDialog.value = false
  }
}

function closeDeliveryDialog() {
  deliveryDialogVisible.value = false
  deliveryData.value = null
}

function resetForm() {
  Object.assign(form, createEmptyForm())
}

function splitLines(value: string): string[] {
  return value
    .split('\n')
    .map((item) => item.trim())
    .filter(Boolean)
}

function optionalPositiveInt(value: number | null): number | null {
  const normalized = Number(value)
  return Number.isFinite(normalized) && normalized > 0 ? Math.floor(normalized) : null
}

function numericValue(value: number, fallback = 0): number {
  const normalized = Number(value)
  return Number.isFinite(normalized) ? normalized : fallback
}

async function submitCreate() {
  formError.value = ''
  if (!form.customer_name.trim()) {
    formError.value = '请填写客户名称'
    return
  }

  submitting.value = true
  try {
    const payload: CreateManagedKeyRequest = {
      customer_name: form.customer_name.trim(),
      contact: form.contact.trim(),
      key_name: form.key_name.trim(),
      group_id: form.group_id,
      balance: numericValue(form.balance, 1000),
      concurrency: Math.max(1, Math.floor(numericValue(form.concurrency, 1))),
      rpm_limit: Math.max(0, Math.floor(numericValue(form.rpm_limit, 0))),
      quota: Math.max(0, numericValue(form.quota, 0)),
      expires_in_days: optionalPositiveInt(form.expires_in_days),
      custom_key: form.custom_key.trim() || null,
      ip_lock_mode: form.ip_lock_mode,
      limit_action: form.limit_action,
      rate_limit_5h: Math.max(0, numericValue(form.rate_limit_5h, 0)),
      rate_limit_1d: Math.max(0, numericValue(form.rate_limit_1d, 0)),
      rate_limit_1mo: Math.max(0, numericValue(form.rate_limit_1mo, 0)),
      ip_whitelist: splitLines(form.ip_whitelist),
      ip_blacklist: splitLines(form.ip_blacklist),
      notes: form.notes.trim()
    }

    const result = await adminAPI.apiKeys.createManagedKey(payload)
    deliveryData.value = result
    deliveryDialogVisible.value = true
    showCreateDialog.value = false
    appStore.showSuccess('托管 Key 已创建')
    await loadManagedKeys()
  } catch (err) {
    formError.value = extractApiErrorMessage(err, '创建托管 Key 失败')
  } finally {
    submitting.value = false
  }
}

function displayCustomerName(item: ManagedKey): string {
  return item.user.username || managedNoteValue(item, 'customer') || item.user.email
}

function managedContact(item: ManagedKey): string {
  return managedNoteValue(item, 'contact')
}

function managedNoteValue(item: ManagedKey, key: string): string {
  const prefix = `${key}:`
  const line = (item.user.notes || '')
    .split('\n')
    .find((part) => part.toLowerCase().startsWith(prefix))
  return line ? line.slice(prefix.length).trim() : ''
}

function groupFor(item: ManagedKey): AdminGroup | null {
  const apiKey = item.api_key
  if (!apiKey) return null
  if (apiKey.group) return apiKey.group as AdminGroup
  return apiKey.group_id ? groupById.value.get(apiKey.group_id) ?? null : null
}

function statusLabel(status?: string): string {
  const labels: Record<string, string> = {
    active: '活跃',
    inactive: '停用',
    quota_exhausted: '额度用尽',
    expired: '已过期'
  }
  return status ? labels[status] ?? status : '未知'
}

function statusClass(status?: string): string {
  const base = 'inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium'
  if (status === 'active') {
    return `${base} bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300`
  }
  if (status === 'quota_exhausted' || status === 'expired') {
    return `${base} bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300`
  }
  return `${base} bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-300`
}

function usageLine(apiKey: ManagedKey['api_key'], window: '5h' | '1d' | '1mo'): string {
  if (!apiKey) return '-'
  const fields = {
    '5h': { label: '5h', used: apiKey.usage_5h, limit: apiKey.rate_limit_5h },
    '1d': { label: '日', used: apiKey.usage_1d, limit: apiKey.rate_limit_1d },
    '1mo': { label: '月', used: apiKey.usage_1mo ?? 0, limit: apiKey.rate_limit_1mo ?? 0 }
  }[window]
  const limit = fields.limit > 0 ? formatCurrency(fields.limit) : '不限'
  return `${fields.label}: ${formatCurrency(fields.used || 0)} / ${limit}`
}

function ipLockLabel(item: ManagedKey): string {
  const mode = item.ip_lock?.mode || item.api_key?.ip_lock_mode
  if (mode !== 'auto_single_ip') return 'IP: 未锁定'
  return item.ip_lock?.locked_ip ? `IP: ${item.ip_lock.locked_ip}` : 'IP: 等待首访'
}

function limitActionLabel(action?: string): string {
  return action === 'soft_throttle' ? '超限: 软降速' : '超限: 拒绝'
}

async function showDeliveryForExisting(item: ManagedKey) {
  if (!item.api_key) return
  try {
    deliveryData.value = await adminAPI.apiKeys.getManagedKeyDelivery(item.user.id)
    deliveryDialogVisible.value = true
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '加载交付信息失败'))
  }
}

async function resetIPLock(item: ManagedKey) {
  if (!item.api_key) return
  try {
    await adminAPI.apiKeys.resetManagedKeyIPLock(item.user.id)
    appStore.showSuccess('IP 锁已重置')
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, '重置 IP 锁失败'))
  }
}

async function copyText(value: string, fieldId: string) {
  await navigator.clipboard.writeText(value)
  copiedField.value = fieldId
  window.setTimeout(() => {
    if (copiedField.value === fieldId) {
      copiedField.value = ''
    }
  }, 1400)
}

const DeliveryField = defineComponent({
  name: 'DeliveryField',
  props: {
    label: { type: String, required: true },
    value: { type: String, required: true },
    fieldId: { type: String, required: true },
    copiedField: { type: String, required: true }
  },
  emits: ['copy'],
  setup(props, { emit }) {
    return () =>
      h('div', { class: 'rounded-lg border border-gray-200 bg-white p-3 dark:border-dark-700 dark:bg-dark-800' }, [
        h('div', { class: 'mb-2 text-xs font-medium uppercase text-gray-500 dark:text-dark-400' }, props.label),
        h('div', { class: 'flex items-center gap-2' }, [
          h('code', { class: 'min-w-0 flex-1 truncate rounded-md bg-gray-100 px-2 py-1.5 font-mono text-xs text-gray-800 dark:bg-dark-700 dark:text-dark-100' }, props.value),
          h(
            'button',
            {
              type: 'button',
              class: 'btn btn-ghost btn-sm px-2',
              title: '复制',
              onClick: () => emit('copy', props.value, props.fieldId)
            },
            [h(Icon, { name: props.copiedField === props.fieldId ? 'check' : 'copy', size: 'sm' })]
          )
        ])
      ])
  }
})

onMounted(async () => {
  await Promise.all([loadGroups(), loadManagedKeys()])
})
</script>
