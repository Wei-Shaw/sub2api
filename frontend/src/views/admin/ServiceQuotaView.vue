<template>
  <AppLayout>
    <TablePageLayout>
      <template #actions>
        <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">服务配额</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              管理叠加生效的 RPM、TPM、TPD、每日 USD 与并发规则。
            </p>
          </div>
          <div class="flex items-center gap-3">
            <button class="btn btn-secondary" type="button" :disabled="loading" @click="load">
              <Icon name="refresh" size="sm" class="mr-2" />
              刷新
            </button>
            <button class="btn btn-primary" type="button" @click="openCreate">
              <Icon name="plus" size="sm" class="mr-2" />
              新增规则
            </button>
          </div>
        </div>
      </template>

      <template #filters>
        <div class="grid gap-3 rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800 md:grid-cols-4">
          <select v-model="filters.limiter" class="input">
            <option value="">全部类型</option>
            <option v-for="item in limiterOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <select v-model="filters.scope" class="input">
            <option value="">全部作用域</option>
            <option v-for="item in scopeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <select v-model="filters.target" class="input">
            <option value="">全部目标模式</option>
            <option v-for="item in targetOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
          </select>
          <select v-model="filters.enabled" class="input">
            <option value="">全部状态</option>
            <option value="true">启用</option>
            <option value="false">停用</option>
          </select>
        </div>
      </template>

      <template #table>
        <DataTable :columns="columns" :data="filteredRules" :loading="loading">
          <template #cell-enabled="{ value }">
            <span :class="['badge', value ? 'badge-success' : 'badge-gray']">
              {{ value ? '启用' : '停用' }}
            </span>
          </template>

          <template #cell-scope="{ row }">
            <div class="space-y-1">
              <div class="font-medium text-gray-900 dark:text-white">{{ scopeLabel(row.scope_level) }}</div>
              <div class="max-w-md truncate text-xs text-gray-500 dark:text-gray-400">{{ scopeDetail(row) }}</div>
            </div>
          </template>

          <template #cell-limiter_type="{ row }">
            <span :class="['badge', limiterBadgeClass(row.limiter_type)]">{{ limiterLabel(row.limiter_type) }}</span>
          </template>

          <template #cell-target_mode="{ row }">
            <div class="space-y-1">
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ targetLabel(row.target_mode) }}</div>
              <div v-if="row.target_user_id" class="text-xs text-gray-500 dark:text-gray-400">用户 ID：{{ row.target_user_id }}</div>
            </div>
          </template>

          <template #cell-window_mode="{ row }">
            <span class="text-sm text-gray-700 dark:text-gray-300">{{ windowLabel(row) }}</span>
          </template>

          <template #cell-limit_value="{ row }">
            <span class="font-mono text-sm font-semibold text-gray-900 dark:text-white">{{ formatLimit(row) }}</span>
          </template>

          <template #cell-actions="{ row }">
            <div class="flex items-center gap-1">
              <button class="action-btn hover:bg-blue-50 hover:text-blue-600 dark:hover:bg-blue-900/20" type="button" title="编辑" @click="openEdit(row)">
                <Icon name="edit" size="sm" />
              </button>
              <button class="action-btn hover:bg-red-50 hover:text-red-600 dark:hover:bg-red-900/20" type="button" title="删除" @click="askDelete(row)">
                <Icon name="trash" size="sm" />
              </button>
            </div>
          </template>

          <template #empty>
            <EmptyState title="暂无服务配额规则" description="创建第一条规则后，系统会在现有限制外叠加检查。" action-text="新增规则" @action="openCreate" />
          </template>
        </DataTable>
      </template>
    </TablePageLayout>

    <BaseDialog :show="showDialog" :title="editingID ? '编辑服务配额规则' : '新增服务配额规则'" width="wide" @close="closeDialog">
      <form id="service-quota-form" class="space-y-5" @submit.prevent="save">
        <div class="grid gap-4 md:grid-cols-2">
          <label class="form-field">
            <span class="input-label">状态</span>
            <select v-model="form.enabled" class="input">
              <option :value="true">启用</option>
              <option :value="false">停用</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">限流类型</span>
            <select v-model="form.limiter_type" class="input">
              <option v-for="item in limiterOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">作用域层级</span>
            <select v-model="form.scope_level" class="input">
              <option v-for="item in scopeOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">目标模式</span>
            <select v-model="form.target_mode" class="input">
              <option v-for="item in targetOptions" :key="item.value" :value="item.value">{{ item.label }}</option>
            </select>
          </label>
        </div>

        <div class="rounded-xl border border-gray-200 p-4 dark:border-dark-700">
          <div class="mb-3 text-sm font-medium text-gray-900 dark:text-white">作用域匹配</div>
          <div class="grid gap-4 md:grid-cols-2">
            <label class="form-field">
              <span class="input-label">渠道/平台</span>
              <input v-model="form.platform" class="input" placeholder="如 anthropic / openai / gemini" />
            </label>
            <label class="form-field">
              <span class="input-label">分组 ID</span>
              <input v-model.number="form.group_id" class="input" min="1" type="number" placeholder="可选" />
            </label>
            <label class="form-field">
              <span class="input-label">账号 ID</span>
              <input v-model.number="form.account_id" class="input" min="1" type="number" placeholder="可选" />
            </label>
            <label class="form-field">
              <span class="input-label">模型匹配</span>
              <input v-model="form.model_pattern" class="input" placeholder="如 claude-opus-*" />
            </label>
          </div>
        </div>

        <div class="grid gap-4 md:grid-cols-3">
          <label v-if="form.target_mode === 'user'" class="form-field">
            <span class="input-label">绑定用户 ID</span>
            <input v-model.number="form.target_user_id" class="input" min="1" type="number" placeholder="必填" />
          </label>
          <label v-if="form.limiter_type !== 'concurrency'" class="form-field">
            <span class="input-label">窗口模式</span>
            <select v-model="form.window_mode" class="input">
              <option value="fixed">固定窗口</option>
              <option value="rolling">滚动窗口</option>
            </select>
          </label>
          <label class="form-field">
            <span class="input-label">额度</span>
            <input v-model.number="form.limit_value" class="input" min="0" step="0.000001" type="number" required />
          </label>
        </div>
      </form>

      <template #footer>
        <div class="flex justify-end gap-3">
          <button class="btn btn-secondary" type="button" @click="closeDialog">取消</button>
          <button class="btn btn-primary" type="submit" form="service-quota-form" :disabled="saving">
            {{ saving ? '保存中...' : '保存' }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <ConfirmDialog
      :show="!!deletingRule"
      title="删除服务配额规则"
      :message="`确定删除 ${deletingRule ? limiterLabel(deletingRule.limiter_type) : ''} 规则吗？删除后立即停止生效。`"
      confirm-text="删除"
      cancel-text="取消"
      :danger="true"
      @confirm="confirmDelete"
      @cancel="deletingRule = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import type { Column } from '@/components/common/types'
import { createServiceQuotaRule, deleteServiceQuotaRule, listServiceQuotaRules, updateServiceQuotaRule, type ServiceQuotaRule, type ServiceQuotaRuleInput } from '@/api/admin/serviceQuota'

const appStore = useAppStore()

const scopeOptions = [
  { value: 'global', label: '全局' },
  { value: 'platform', label: '渠道' },
  { value: 'group', label: '分组' },
  { value: 'account', label: '账号' },
  { value: 'model', label: '模型' },
]
const limiterOptions = [
  { value: 'rpm', label: 'RPM' },
  { value: 'tpm', label: 'TPM' },
  { value: 'tpd', label: 'TPD' },
  { value: 'daily_usd', label: '每日 USD' },
  { value: 'concurrency', label: '并发' },
]
const targetOptions = [
  { value: 'user', label: '绑定指定用户' },
  { value: 'per_user', label: '每用户独立' },
  { value: 'shared', label: '全局共享' },
  { value: 'default', label: '默认规则' },
]

const columns: Column[] = [
  { key: 'enabled', label: '状态' },
  { key: 'scope', label: '作用域' },
  { key: 'limiter_type', label: '类型' },
  { key: 'target_mode', label: '目标模式' },
  { key: 'window_mode', label: '窗口' },
  { key: 'limit_value', label: '额度' },
  { key: 'actions', label: '操作' },
]

const rules = ref<ServiceQuotaRule[]>([])
const loading = ref(false)
const saving = ref(false)
const showDialog = ref(false)
const editingID = ref<number | null>(null)
const deletingRule = ref<ServiceQuotaRule | null>(null)
const filters = reactive({ limiter: '', scope: '', target: '', enabled: '' })
const form = reactive<ServiceQuotaRuleInput>(blankRule())

const filteredRules = computed(() => rules.value.filter((rule) => {
  if (filters.limiter && rule.limiter_type !== filters.limiter) return false
  if (filters.scope && rule.scope_level !== filters.scope) return false
  if (filters.target && rule.target_mode !== filters.target) return false
  if (filters.enabled && String(rule.enabled) !== filters.enabled) return false
  return true
}))

watch(() => form.limiter_type, (value) => {
  if (value === 'concurrency') form.window_mode = 'fixed'
})

function blankRule(): ServiceQuotaRuleInput {
  return { enabled: true, scope_level: 'global', limiter_type: 'rpm', target_mode: 'per_user', window_mode: 'fixed', limit_value: 60 }
}

function resetForm(rule?: ServiceQuotaRule) {
  Object.assign(form, blankRule(), rule || {})
  editingID.value = rule?.id ?? null
}

function optionLabel(options: Array<{ value: string; label: string }>, value: string): string {
  return options.find((item) => item.value === value)?.label || value
}

function scopeLabel(value: string): string { return optionLabel(scopeOptions, value) }
function limiterLabel(value: string): string { return optionLabel(limiterOptions, value) }
function targetLabel(value: string): string { return optionLabel(targetOptions, value) }

function scopeDetail(rule: ServiceQuotaRule): string {
  const parts = [
    rule.platform && `渠道：${rule.platform}`,
    rule.group_id && `分组：${rule.group_id}`,
    rule.account_id && `账号：${rule.account_id}`,
    rule.model_pattern && `模型：${rule.model_pattern}`,
  ].filter(Boolean)
  return parts.length > 0 ? parts.join(' / ') : '所有请求'
}

function windowLabel(rule: ServiceQuotaRule): string {
  if (rule.limiter_type === 'concurrency') return '无窗口'
  return rule.window_mode === 'rolling' ? '滚动窗口' : '固定窗口'
}

function formatLimit(rule: ServiceQuotaRule): string {
  if (rule.limiter_type === 'daily_usd') return `$${Number(rule.limit_value).toFixed(6).replace(/\.0+$/, '')}`
  return String(rule.limit_value)
}

function limiterBadgeClass(value: string): string {
  const classes: Record<string, string> = {
    rpm: 'badge-blue',
    tpm: 'badge-purple',
    tpd: 'badge-indigo',
    daily_usd: 'badge-green',
    concurrency: 'badge-yellow',
  }
  return classes[value] || 'badge-gray'
}

function normalizePayload(): ServiceQuotaRuleInput {
  const payload = { ...form }
  payload.platform = cleanText(payload.platform)
  payload.model_pattern = cleanText(payload.model_pattern)
  payload.group_id = cleanNumber(payload.group_id)
  payload.account_id = cleanNumber(payload.account_id)
  payload.target_user_id = cleanNumber(payload.target_user_id)
  if (payload.target_mode !== 'user') payload.target_user_id = null
  if (payload.limiter_type === 'concurrency') payload.window_mode = 'fixed'
  return payload
}

function cleanText(value?: string | null): string | null { return value && value.trim() ? value.trim() : null }
function cleanNumber(value?: number | null): number | null { return value && value > 0 ? value : null }

async function load() {
  loading.value = true
  try {
    rules.value = await listServiceQuotaRules()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '加载服务配额规则失败'))
  } finally {
    loading.value = false
  }
}

function openCreate() {
  resetForm()
  showDialog.value = true
}

function openEdit(rule: ServiceQuotaRule) {
  resetForm(rule)
  showDialog.value = true
}

function closeDialog() {
  showDialog.value = false
}

async function save() {
  saving.value = true
  try {
    const payload = normalizePayload()
    if (editingID.value) await updateServiceQuotaRule(editingID.value, payload)
    else await createServiceQuotaRule(payload)
    appStore.showSuccess('服务配额规则已保存')
    showDialog.value = false
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '保存服务配额规则失败'))
  } finally {
    saving.value = false
  }
}

function askDelete(rule: ServiceQuotaRule) {
  deletingRule.value = rule
}

async function confirmDelete() {
  if (!deletingRule.value) return
  try {
    await deleteServiceQuotaRule(deletingRule.value.id)
    appStore.showSuccess('服务配额规则已删除')
    deletingRule.value = null
    await load()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, '删除服务配额规则失败'))
  }
}

onMounted(load)
</script>

<style scoped>
.form-field {
  @apply space-y-1.5;
}

.action-btn {
  @apply rounded-lg p-1.5 text-gray-500 transition-colors dark:text-gray-400;
}
</style>
