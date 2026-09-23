<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div><h1 class="text-2xl font-semibold">{{ text('项目负责人', 'Project managers') }}</h1><p class="mt-2 text-sm text-gray-500">{{ text('管理负责人的组织权限及累计可分配额度。调整权限或增加额度不会重置已分配金额。', 'Manage organization permissions and cumulative allocation budgets. Permission changes and budget increases preserve previous allocations.') }}</p></div>
        <div v-if="isAdmin" class="flex gap-2"><button class="btn btn-secondary" :disabled="loading" @click="loadManagers()">{{ text('刷新', 'Refresh') }}</button><button class="btn btn-primary" :disabled="loading || appsLoading" @click="openDialog('create')">{{ text('添加负责人', 'Add manager') }}</button></div>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-green-50 p-3 text-green-700 dark:bg-green-950">{{ notice }}</p>
      <section v-if="isAdmin" class="card overflow-hidden">
        <div class="overflow-x-auto">
          <table data-testid="manager-table" class="w-full text-left text-sm">
            <thead><tr><th class="p-3">{{ text('负责人', 'Manager') }}</th><th class="p-3">{{ text('状态', 'Status') }}</th><th class="p-3">{{ text('总额度 / 已分配 / 剩余', 'Budget / allocated / remaining') }}</th><th class="p-3">{{ text('授权部门', 'Departments') }}</th><th class="p-3">{{ text('操作', 'Actions') }}</th></tr></thead>
            <tbody><tr v-for="manager in managers" :key="manager.user_id" class="border-t dark:border-dark-600">
              <td class="p-3">{{ manager.name || `#${manager.user_id}` }}<p class="text-xs text-gray-500">#{{ manager.user_id }}</p></td>
              <td class="p-3">{{ manager.enabled ? text('已启用', 'Enabled') : text('已撤销', 'Revoked') }}</td>
              <td class="p-3">{{ money(manager.limit_cents) }} / {{ money(manager.used_cents) }} / {{ money(manager.limit_cents - manager.used_cents) }}</td>
              <td class="p-3">{{ manager.departments.length }}</td>
              <td class="p-3"><div class="flex flex-wrap gap-2">
                <button class="btn btn-secondary" :disabled="loading" @click="openDialog('edit', manager)">{{ text('编辑', 'Edit') }}</button>
                <button class="btn btn-secondary" :disabled="loading" @click="openDialog('permissions', manager)">{{ text('组织权限分配', 'Organization permissions') }}</button>
                <button class="btn btn-secondary" :disabled="loading" @click="openDialog('increase', manager)">{{ text('增加可分配额度', 'Increase allocation budget') }}</button>
                <button class="btn btn-secondary" :disabled="loading" @click="openDialog('history', manager)">{{ text('查看分配历史', 'Allocation history') }}</button>
              </div></td>
            </tr></tbody>
          </table>
        </div>
        <p v-if="loading" class="p-5">{{ text('加载中…', 'Loading…') }}</p>
        <p v-else-if="!managers.length" class="p-5 text-gray-500">{{ text('暂无项目负责人', 'No project managers') }}</p>
        <div class="flex flex-wrap items-center justify-end gap-3 border-t p-3 text-sm dark:border-dark-600" data-testid="manager-pagination">
          <span>{{ text('每页 20 条，共', '20 per page, total') }} {{ total }}</span>
          <button class="btn btn-secondary" :disabled="loading || page <= 1" @click="loadManagers(page - 1)">{{ text('上一页', 'Previous') }}</button>
          <span>{{ page }} / {{ Math.max(1, Math.ceil(total / 20)) }}</span>
          <button class="btn btn-secondary" :disabled="loading || page * 20 >= total" @click="loadManagers(page + 1)">{{ text('下一页', 'Next') }}</button>
        </div>
      </section>
    </div>

    <BaseDialog v-if="isAdmin && dialog && managerForm" :show="true" :title="dialogTitle" width="wide" :show-close-button="!busy && !pendingIncrease" :close-on-escape="!busy && !pendingIncrease" @close="closeDialog">
      <p v-if="dialogError" role="alert" class="mb-4 rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950">{{ dialogError }}</p>
      <p v-if="dialog !== 'create'" class="mb-4 font-medium">{{ managerForm.name || `#${managerForm.user_id}` }} (#{{ managerForm.user_id }})</p>
      <template v-if="dialog === 'history'">
        <p v-if="historyLoading">{{ text('加载中…', 'Loading…') }}</p>
        <table v-else data-testid="manager-history" class="w-full text-left text-sm">
          <thead><tr><th class="p-2">{{ text('时间', 'Time') }}</th><th class="p-2">{{ text('接收成员', 'Recipient') }}</th><th class="p-2">{{ text('应用 / 部门', 'App / department') }}</th><th class="p-2">{{ text('分配金额', 'Amount') }}</th></tr></thead>
          <tbody><tr v-for="grant in history" :key="grant.id" class="border-t dark:border-dark-600"><td class="p-2">{{ new Date(grant.created_at).toLocaleString() }}</td><td class="p-2">#{{ grant.target_id }}</td><td class="p-2">{{ appName(grant.app_id) }} / {{ departmentName(grant.app_id, grant.department_id) }}</td><td class="p-2">{{ money(grant.amount_cents) }}</td></tr></tbody>
        </table>
        <p v-if="!historyLoading && !history.length && !dialogError" class="py-5 text-gray-500">{{ text('暂无分配记录', 'No allocations') }}</p>
        <div class="mt-4 flex flex-wrap items-center justify-end gap-3 text-sm" data-testid="history-pagination"><span>{{ text('每页 20 条，共', '20 per page, total') }} {{ historyTotal }}</span><button class="btn btn-secondary" :disabled="historyLoading || historyPage <= 1" @click="loadHistory(historyPage - 1)">{{ text('上一页', 'Previous') }}</button><span>{{ historyPage }} / {{ Math.max(1, Math.ceil(historyTotal / 20)) }}</span><button class="btn btn-secondary" :disabled="historyLoading || historyPage * 20 >= historyTotal" @click="loadHistory(historyPage + 1)">{{ text('下一页', 'Next') }}</button></div>
      </template>
      <form v-else class="space-y-4" @submit.prevent="submit">
        <fieldset class="space-y-4" :disabled="busy">
          <label v-if="dialog === 'create' || dialog === 'permissions'" class="block text-sm">{{ text('选择组织应用', 'Organization application') }}
            <select v-model="selectedApp" class="input mt-1" :disabled="directoryLoading" @change="loadDirectory"><option v-for="app in apps" :key="app.id" :value="app.id">{{ app.name }}{{ managerForm.departments.some(d => d.app_id === app.id) ? ` (${text('已授权', 'Assigned')})` : '' }}</option></select>
          </label>
          <p v-if="directoryLoading">{{ text('加载组织中…', 'Loading directory…') }}</p>
          <template v-if="dialog === 'create' || dialog === 'edit'">
            <div v-if="dialog === 'create'"><label for="manager-user">{{ text('项目负责人', 'Project manager') }}</label><Select id="manager-user" :model-value="managerForm.user_id || null" :options="managerOptions" :searchable="true" :disabled="busy || directoryLoading" :placeholder="text('搜索姓名、ID 或部门', 'Search name, ID or department')" @update:model-value="selectManager" /></div>
            <label class="block">{{ text('最大累计可分配额度', 'Maximum cumulative allocation') }}<input v-model.number="managerLimit" data-testid="manager-limit" class="input mt-1" type="number" :min="managerForm.used_cents / 100" max="1000000000" step="0.01" required /></label>
            <label class="block"><input v-model="managerForm.enabled" type="checkbox" /> {{ text('允许分配额度', 'Allow quota allocation') }}</label>
          </template>
          <template v-if="dialog === 'create' || dialog === 'permissions'">
            <p class="text-sm text-gray-500">{{ text('选择负责部门（包含子部门）；其他应用的授权会保留。添加负责人时默认勾选其所在部门。', 'Select departments including descendants; permissions in other apps are retained. A new manager’s own departments are selected by default.') }}</p>
            <DepartmentPermissions v-if="directory && !directoryLoading" :key="selectedApp" :departments="directory.departments" :selected="managerForm.departments.filter(d => d.app_id === selectedApp).map(d => d.department_id)" @toggle="toggleDepartment" />
          </template>
          <template v-if="dialog === 'increase'">
            <p class="text-sm">{{ text('当前总额度 / 已分配 / 剩余：', 'Current budget / allocated / remaining: ') }}{{ money(managerForm.limit_cents) }} / {{ money(managerForm.used_cents) }} / {{ money(managerForm.limit_cents - managerForm.used_cents) }}</p>
            <label class="block">{{ text('增加额度', 'Amount to add') }}<input v-model.number="increaseAmount" data-testid="budget-increase" class="input mt-1" type="number" min="0.01" :max="(100000000000 - managerForm.limit_cents) / 100" step="0.01" required :disabled="!!pendingIncrease" /></label>
            <p class="text-sm text-gray-500">{{ text('增加负责人的可分配上限，不直接充值其个人余额。', 'Increases the allocation budget without crediting the manager’s personal balance.') }}</p>
            <p v-if="pendingIncrease" class="text-sm text-amber-600">{{ text('结果尚未确认，请重试同一笔增加额度，避免重复入账。', 'Result not confirmed. Retry the same increase to avoid duplicate credits.') }}</p>
          </template>
        </fieldset>
        <div class="flex justify-end gap-3">
          <button type="button" class="btn btn-secondary" :disabled="busy || !!pendingIncrease" @click="closeDialog">{{ text('取消', 'Cancel') }}</button>
          <button class="btn btn-primary" :disabled="busy || directoryLoading || !managerForm.user_id || ((dialog === 'create' || dialog === 'permissions') && !directory)">{{ pendingIncrease ? text('重试同一笔', 'Retry same increase') : text('保存', 'Save') }}</button>
        </div>
      </form>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import DepartmentPermissions from '@/components/dingtalk/DepartmentPermissions.vue'
import { dingTalkAPI, publicDingTalkApps, type DingTalkDirectory, type DingTalkManager, type DingTalkGrant } from '@/api/dingtalk'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const isAdmin = computed(() => auth.isAdmin)
const api = dingTalkAPI(true)
const { locale } = useI18n()
const text = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const money = (cents: number) => `$${(cents / 100).toFixed(2)}`
const message = (e: unknown) => (e as Error).message || text('操作失败', 'Operation failed')
const managers = ref<DingTalkManager[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const error = ref('')
const notice = ref('')
const appsLoading = ref(true)
const apps = ref<{ id: string; name: string }[]>([])
const selectedApp = ref('')
const directory = ref<DingTalkDirectory | null>(null)
const directoryCache = new Map<string, DingTalkDirectory>()
const directoryLoading = ref(false)
type Dialog = 'create' | 'edit' | 'permissions' | 'increase' | 'history'
const dialog = ref<Dialog | null>(null)
const managerForm = ref<DingTalkManager | null>(null)
const managerLimit = ref(500)
const increaseAmount = ref(500)
const pendingIncrease = ref<{ manager: number; amount: number; request_id: string } | null>(null)
const busy = ref(false)
const dialogError = ref('')
const history = ref<DingTalkGrant[]>([])
const historyPage = ref(1)
const historyTotal = ref(0)
const historyLoading = ref(false)
let dialogVersion = 0
let disposed = false
onUnmounted(() => { disposed = true; dialogVersion++ })
const dialogTitle = computed(() => ({
  create: text('添加负责人', 'Add manager'), edit: text('编辑负责人', 'Edit manager'),
  permissions: text('组织权限分配', 'Organization permissions'), increase: text('增加可分配额度', 'Increase allocation budget'), history: text('查看分配历史', 'Allocation history')
})[dialog.value || 'create'])
const managerOptions = computed(() => {
  const grouped = new Map<number, { value: number; label: string; description: string }>()
  for (const m of directory.value?.members || []) {
    if (!m.user_id || m.user_id === auth.user?.id) continue
    const description = `${m.staff_id} ${departmentName(selectedApp.value, m.department_id)}`
    const existing = grouped.get(m.user_id)
    if (existing) existing.description += ` ${description}`
    else grouped.set(m.user_id, { value: m.user_id, label: `${m.name} (#${m.user_id})`, description })
  }
  if (managerForm.value?.user_id && !grouped.has(managerForm.value.user_id)) grouped.set(managerForm.value.user_id, { value: managerForm.value.user_id, label: managerForm.value.name || `#${managerForm.value.user_id}`, description: '' })
  return [...grouped.values()]
})
function appName(id: string) { return apps.value.find(a => a.id === id)?.name || id }
function departmentName(app: string, id: number) { return directoryCache.get(app)?.departments.find(d => d.id === id)?.name || `#${id}` }
async function loadManagers(target = page.value) {
  loading.value = true; error.value = ''
  try {
    const data = await api.managerPage(target)
    if (disposed) return
    managers.value = data.items; total.value = data.total; page.value = target
  } catch (e) { error.value = message(e) }
  finally { loading.value = false }
}
async function loadDirectory() {
  const app = selectedApp.value
  const version = dialogVersion
  directory.value = null
  if (!app) return
  directoryLoading.value = true; dialogError.value = ''
  try {
    const loaded = await api.directory(app)
    if (disposed || version !== dialogVersion || app !== selectedApp.value) return
    directoryCache.set(app, loaded); directory.value = loaded
  } catch (e) { if (version === dialogVersion) dialogError.value = message(e) }
  finally { if (version === dialogVersion) directoryLoading.value = false }
}
async function openDialog(mode: Dialog, manager?: DingTalkManager) {
  if (!isAdmin.value || busy.value || pendingIncrease.value) return
  dialogVersion++; dialogError.value = ''; directoryLoading.value = false
  const draft: DingTalkManager = manager ? { ...manager, departments: manager.departments.map(d => ({ ...d })) } : { user_id: 0, limit_cents: 50000, used_cents: 0, enabled: true, departments: [] }
  managerForm.value = draft
  managerLimit.value = draft.limit_cents / 100
  increaseAmount.value = 500; dialog.value = mode
  if (mode === 'permissions') selectedApp.value = manager?.departments.find(d => apps.value.some(a => a.id === d.app_id))?.app_id || apps.value[0]?.id || ''
  if (mode === 'create' || mode === 'permissions') await loadDirectory()
  if (mode === 'history') { history.value = []; historyTotal.value = 0; historyPage.value = 1; await loadHistory(1) }
}
function closeDialog() { if (busy.value || pendingIncrease.value) return; dialog.value = null; managerForm.value = null; dialogVersion++ }
function selectManager(value: string | number | boolean | null) {
  if (typeof value !== 'number' || !managerForm.value) return
  managerForm.value.user_id = value
  managerForm.value.name = directory.value?.members.find(m => m.user_id === value)?.name
  managerForm.value.departments = [...new Set(directory.value?.members.filter(m => m.user_id === value).map(m => m.department_id))].map(id => ({ app_id: selectedApp.value, department_id: id }))
}
function toggleDepartment(id: number, checked: boolean) {
  if (!managerForm.value) return
  managerForm.value.departments = managerForm.value.departments.filter(d => d.app_id !== selectedApp.value || d.department_id !== id)
  if (checked) managerForm.value.departments.push({ app_id: selectedApp.value, department_id: id })
}
async function loadHistory(target: number) {
  if (!managerForm.value) return
  const version = dialogVersion
  historyLoading.value = true; dialogError.value = ''
  try {
    const data = await api.managerGrants(managerForm.value.user_id, target)
    if (disposed || version !== dialogVersion) return
    history.value = data.items; historyTotal.value = data.total; historyPage.value = target
  } catch (e) { if (version === dialogVersion) { history.value = []; dialogError.value = message(e) } }
  finally { if (version === dialogVersion) historyLoading.value = false }
}
async function submit() {
  if (!isAdmin.value || busy.value || !managerForm.value) return
  busy.value = true; dialogError.value = ''; notice.value = ''
  const manager = managerForm.value
  try {
    if (dialog.value === 'create') await api.createManager({ ...manager, limit_cents: Math.round(managerLimit.value * 100) })
    else if (dialog.value === 'edit') {
      const limit = Math.round(managerLimit.value * 100)
      await api.patchManager(manager.user_id, { enabled: manager.enabled, ...(limit !== manager.limit_cents ? { limit_cents: limit, expected_limit_cents: manager.limit_cents } : {}) })
    } else if (dialog.value === 'permissions') await api.patchManager(manager.user_id, { departments: manager.departments })
    else if (dialog.value === 'increase') {
      pendingIncrease.value ||= { manager: manager.user_id, amount: increaseAmount.value, request_id: crypto.randomUUID() }
      const { manager: id, ...input } = pendingIncrease.value
      try { await api.increaseBudget(id, input) } catch (e) {
        const status = (e as { status?: number; response?: { status?: number } }).status || (e as { response?: { status?: number } }).response?.status
        if (status && status >= 400 && status < 500 && status !== 408) pendingIncrease.value = null
        throw e
      }
      pendingIncrease.value = null
    }
    dialog.value = null; managerForm.value = null; dialogVersion++
    notice.value = text('负责人配置已更新', 'Manager updated')
    await loadManagers()
  } catch (e) { dialogError.value = message(e) }
  finally { busy.value = false }
}
onMounted(async () => {
  if (!isAdmin.value) { error.value = text('仅系统管理员可管理项目负责人', 'Only administrators can manage project managers'); return }
  await Promise.all([loadManagers(), (async () => {
    try {
      const [configured, publicApps] = await Promise.all([api.apps(), publicDingTalkApps()])
      apps.value = [...(publicApps.some(a => a.id === 'default') ? [{ id: 'default', name: text('默认钉钉应用', 'Default DingTalk app') }] : []), ...configured.filter(a => a.enabled)]
      selectedApp.value = apps.value[0]?.id || ''
    } catch (e) { error.value = message(e) }
    finally { appsLoading.value = false }
  })()])
})
</script>
