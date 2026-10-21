<template>
  <AppLayout>
    <div class="space-y-6">
      <div>
        <h1 class="text-2xl font-semibold">{{ allocationMode ? text('组织额度分配', 'Organization quota allocation') : text('钉钉组织', 'DingTalk organization') }}</h1>
        <p class="mt-2 text-sm text-gray-500">{{ text('组织架构来自钉钉。负责人可为所管部门及子部门成员增加余额，累计分配不能超过管理员设定的总额度。', 'The directory comes from DingTalk. Managers can add balance to members of their departments and subdepartments within a cumulative budget.') }}</p>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-green-50 p-3 text-green-700 dark:bg-green-950">{{ notice }}</p>
      <p v-if="loading">{{ text('加载中…', 'Loading…') }}</p>

      <div v-if="!allocationMode && bindingApps.length" class="card flex flex-wrap items-end gap-3 p-5">
        <label class="min-w-48 flex-1 text-sm">{{ text('绑定我的钉钉应用', 'Link my DingTalk application') }}
          <select v-model="bindingApp" class="input mt-1"><option v-for="app in bindingApps" :key="app.id" :value="app.id">{{ app.name }}</option></select>
        </label>
        <button class="btn btn-secondary" :disabled="busy" @click="bindApplication">{{ text('绑定 / 添加钉钉身份', 'Link / add DingTalk identity') }}</button>
      </div>

      <details v-if="isAdmin && !allocationMode" class="card p-5">
        <summary class="cursor-pointer font-semibold">{{ text('钉钉应用配置', 'DingTalk applications') }} ({{ apps.length }})</summary>
        <p class="my-3 text-sm text-gray-500">{{ text('每个应用需要企业内部应用的登录和通讯录权限。应用标识、Client ID 和企业 ID 保存后不可修改；停用会停止该应用的登录与额度分配。旧版默认应用仍在系统设置中维护。', 'Each internal app needs login and directory permissions. Saved IDs and corporation are immutable. Disable an app to stop login and allocation. The legacy default app remains in system settings.') }}</p>
        <form class="space-y-4" @submit.prevent="saveApps">
          <fieldset v-for="(app, index) in apps" :key="index" class="rounded-lg border p-4 dark:border-dark-600" :disabled="busy">
            <div class="grid gap-3 md:grid-cols-2">
              <label class="text-sm">{{ text('应用标识', 'Application ID') }}<input v-model="app.id" class="input mt-1" pattern="[a-zA-Z0-9_-]{1,64}" required :readonly="savedIDs.has(app.id)" /></label>
              <label class="text-sm">{{ text('应用名称', 'Name') }}<input v-model="app.name" class="input mt-1" required /></label>
              <label class="text-sm">Client ID<input v-model="app.client_id" class="input mt-1" required :readonly="savedIDs.has(app.id)" /></label>
              <label class="text-sm">Client Secret<input v-model="app.client_secret" class="input mt-1" type="password" autocomplete="new-password" :required="!app.client_secret_configured" :placeholder="app.client_secret_configured ? text('已配置，留空保留', 'Configured; leave blank to keep') : ''" /></label>
              <label class="text-sm">{{ text('企业 ID（可选）', 'Corporation ID (optional)') }}<input v-model="app.corp_id" class="input mt-1" :readonly="savedIDs.has(app.id)" /></label>
              <label class="text-sm">{{ text('回调地址', 'Redirect URL') }}<input v-model="app.redirect_url" class="input mt-1" type="url" required :placeholder="callbackURL" /></label>
            </div>
            <div class="mt-3 flex gap-4">
              <label><input v-model="app.enabled" type="checkbox" /> {{ text('启用', 'Enabled') }}</label>
              <button v-if="!savedIDs.has(app.id)" type="button" class="text-red-600" @click="apps.splice(index, 1)">{{ text('移除', 'Remove') }}</button>
            </div>
          </fieldset>
          <div class="flex gap-3">
            <button type="button" class="btn btn-secondary" :disabled="busy" @click="addApp">{{ text('添加应用', 'Add application') }}</button>
            <button class="btn btn-primary" :disabled="busy">{{ text('保存应用', 'Save applications') }}</button>
          </div>
        </form>
      </details>

      <section v-if="!loading && choices.length" class="card space-y-4 p-5">
        <div class="flex flex-wrap items-end gap-3">
          <label class="min-w-48 flex-1 text-sm">{{ text('选择组织应用', 'Organization application') }}
            <select v-model="selectedApp" class="input mt-1" :disabled="busy" @change="loadDirectory"><option v-for="app in choices" :key="app.id" :value="app.id">{{ app.name }}</option></select>
          </label>
          <button class="btn btn-secondary" :disabled="busy" @click="loadDirectory">{{ text('刷新', 'Refresh') }}</button>
          <button v-if="isAdmin && !allocationMode" class="btn btn-primary" :disabled="busy || syncJob?.status === 'running'" @click="syncDirectory">{{ syncJob?.status === 'running' ? text('处理中…', 'Working…') : text('同步钉钉组织', 'Sync DingTalk directory') }}</button>
        </div>
        <p v-if="syncJob?.status === 'running'" role="status" class="text-sm text-primary-600">{{ text('正在后台同步，可离开此页面，完成后会自动更新。', 'Sync is running in the background. You may leave this page; the directory refreshes when complete.') }}</p>
        <p v-if="syncJob?.status === 'failed'" role="alert" class="text-sm text-red-600">{{ text('同步失败，可点击同步重试：', 'Sync failed. Click sync to retry: ') }}{{ syncJob.error }}</p>
        <p class="text-sm text-gray-500">{{ text('最近同步：', 'Last synced: ') }}{{ directory.synced_at ? new Date(directory.synced_at).toLocaleString() : text('尚未同步', 'Never') }}</p>
        <div v-if="selectedDepartmentPath.length" class="flex flex-wrap items-center gap-2 rounded-lg bg-primary-50 p-3 text-sm dark:bg-dark-700" data-testid="selected-department-path">
          <span>{{ text('当前部门：', 'Current department: ') }}{{ selectedDepartmentPath.map(d => d.name).join(' / ') }}</span>
          <button class="btn btn-secondary" type="button" @click="locateDepartment">{{ text('定位选中部门', 'Locate selected department') }}</button>
        </div>
        <div class="grid gap-4 lg:grid-cols-[260px_1fr]">
          <div ref="departmentTree" class="max-h-[32rem] space-y-1 overflow-auto border-r pr-3 dark:border-dark-600">
            <p class="mb-2 font-semibold">{{ text('部门', 'Departments') }}</p>
            <div v-for="dept in visibleDepartmentRows" :key="dept.id" :data-department-id="dept.id" class="flex items-center" :style="{ paddingLeft: `${dept.depth * 16}px` }">
              <button v-if="dept.hasChildren" type="button" class="shrink-0 rounded p-1" :aria-expanded="expandedDepartments.has(dept.id)" :aria-label="`${text('展开/收起', 'Expand/collapse')} ${dept.name}`" @click="toggleCollapse(dept.id)">{{ expandedDepartments.has(dept.id) ? '▾' : '▸' }}</button>
              <span v-else class="w-6 shrink-0" />
              <button class="min-w-0 flex-1 rounded px-2 py-2 text-left text-sm" :aria-current="selectedDepartment === dept.id ? 'true' : undefined" :class="selectedDepartment === dept.id ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100 dark:hover:bg-dark-700'" @click="selectedDepartment = dept.id; memberSearch = ''">{{ dept.name }}</button>
            </div>
            <p v-if="!departmentRows.length" class="text-sm text-gray-500">{{ text('暂无可管理的部门', 'No managed departments') }}</p>
          </div>
          <div class="overflow-x-auto">
            <label class="mb-3 block text-sm">{{ allocationMode ? text('搜索所选部门及下级部门成员（姓名、钉钉 ID、平台用户 ID）', 'Search selected department and descendants (name, DingTalk ID or user ID)') : text('搜索整个组织成员（姓名、钉钉 ID、平台用户 ID）', 'Search organization members (name, DingTalk ID or user ID)') }}<input v-model="memberSearch" data-testid="member-search" type="search" class="input mt-1" /></label>
            <table class="w-full text-left text-sm">
              <thead><tr><th class="p-2">{{ text('成员', 'Member') }}</th><th class="p-2">{{ text('平台用户', 'Platform user') }}</th><th class="p-2">{{ text('余额', 'Balance') }}</th><th v-if="allocationMode" class="p-2">{{ text('操作', 'Action') }}</th></tr></thead>
              <tbody><tr v-for="member in pagedMembers" :key="`${member.department_id}-${member.staff_id}`" class="border-t dark:border-dark-600">
                <td class="p-2">{{ member.name }}<p class="text-xs text-gray-500">{{ departmentName(selectedApp, member.department_id) }}</p></td><td class="p-2">{{ member.user_id || text('尚未绑定', 'Not linked') }}</td><td class="p-2">{{ member.user_id ? money(member.balance) : '—' }}</td>
                <td v-if="allocationMode" class="p-2"><button class="btn btn-secondary" :disabled="busy || !member.user_id || (isAdmin && member.user_id === auth.user?.id)" @click="openGrant(member)">{{ text('增加额度', 'Add quota') }}</button></td>
              </tr></tbody>
            </table>
            <div v-if="members.length > memberPageSize" class="mt-3 flex items-center gap-3 text-sm"><button class="btn btn-secondary" :disabled="memberPage === 1" @click="memberPage--">{{ text('上一页', 'Previous') }}</button><span>{{ memberPage }} / {{ Math.ceil(members.length / memberPageSize) }} · {{ text('每页 20 条', '20 per page') }} · {{ members.length }} {{ text('名成员', 'members') }}</span><button class="btn btn-secondary" :disabled="memberPage * memberPageSize >= members.length" @click="memberPage++">{{ text('下一页', 'Next') }}</button></div>
            <p v-if="!members.length" class="py-6 text-gray-500">{{ text('此部门暂无成员', 'No members in this department') }}</p>
          </div>
        </div>
      </section>
      <p v-else-if="!loading" class="card p-5">{{ text('暂无可管理的组织。请先在钉钉组织菜单配置应用、同步组织，再由管理员配置负责人。', 'No managed organizations. Configure an app, sync the directory and assign department managers.') }}</p>

      <section v-if="allocationMode" class="card space-y-4 p-5">
        <h2 class="font-semibold">{{ isAdmin ? text('管理员充值权限', 'Administrator allocation access') : text('我的可分配额度', 'My allocation budget') }}</h2>
        <div v-for="manager in displayedManagers" :key="manager.user_id" class="rounded border p-3 dark:border-dark-600">
          <p>{{ manager.name || `#${manager.user_id}` }} · {{ manager.enabled ? text('已启用', 'Enabled') : text('已撤销', 'Revoked') }}</p>
          <p class="text-sm text-gray-500">{{ text('总额度 / 已分配 / 剩余：', 'Budget / allocated / remaining: ') }}{{ money(manager.limit_cents / 100) }} / {{ money(manager.used_cents / 100) }} / {{ money((manager.limit_cents - manager.used_cents) / 100) }}</p>
        </div>
        <p class="text-sm text-gray-500">{{ isAdmin ? text('系统管理员可为全部组织的成员充值。', 'System administrators can credit members in all organizations.') : text('可为授权部门内的成员（包括自己）分配额度；总额度跨应用、跨部门累计。', 'Allocate to members, including yourself, within authorized departments. The budget is cumulative across apps and departments.') }}</p>
      </section>

      <BaseDialog
        v-if="allocationMode && grantMember"
        :show="true"
        :title="text('增加额度', 'Add quota')"
        width="narrow"
        :show-close-button="!busy && !pendingGrant"
        :close-on-escape="!busy && !pendingGrant"
        @close="grantMember = null"
      >
        <form class="space-y-4" data-testid="grant-form" @submit.prevent="submitGrant">
          <p class="font-medium">{{ text('为成员增加额度：', 'Add quota for: ') }}{{ grantMember.name }} (#{{ grantMember.user_id }})</p>
          <label class="block">{{ text('增加余额金额', 'Balance to add') }}
            <input data-testid="grant-amount" v-model.number="grantAmount" type="number" min="0.01" max="1000000000" step="0.01" required class="input mt-1" :disabled="!!pendingGrant" />
          </label>
          <p v-if="pendingGrant" class="text-sm text-gray-500">{{ text('重试将使用相同请求编号，避免重复入账。', 'Retries use the same request ID to prevent duplicate credits.') }}</p>
          <div class="flex justify-end gap-3">
            <button type="button" class="btn btn-secondary" :disabled="busy || !!pendingGrant" @click="grantMember = null">{{ text('取消', 'Cancel') }}</button>
            <button class="btn btn-primary" :disabled="busy">{{ pendingGrant ? text('重试同一笔分配', 'Retry this allocation') : text('确认增加额度', 'Confirm allocation') }}</button>
          </div>
        </form>
      </BaseDialog>

      <section v-if="allocationMode" class="card p-5">
        <h2 class="mb-3 font-semibold">{{ text('最近 100 笔分配记录', 'Latest 100 allocations') }}</h2>
        <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">{{ text('时间', 'Time') }}</th><th class="p-2">{{ text('操作人 → 成员', 'Actor → member') }}</th><th class="p-2">{{ text('应用 / 部门', 'App / department') }}</th><th class="p-2">{{ text('金额', 'Amount') }}</th></tr></thead><tbody><tr v-for="grant in grants" :key="grant.id" class="border-t dark:border-dark-600"><td class="p-2">{{ new Date(grant.created_at).toLocaleString() }}</td><td class="p-2">#{{ grant.actor_id }} → #{{ grant.target_id }}</td><td class="p-2">{{ grant.app_id }} / {{ departmentName(grant.app_id, grant.department_id) }}</td><td class="p-2">+{{ money(grant.amount_cents / 100) }}</td></tr></tbody></table></div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { buildDingTalkDepartmentRows, dingTalkDepartmentPath } from '@/utils/dingtalkDepartments'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { startOAuthBinding } from '@/api/user'
import { useAuthStore } from '@/stores/auth'
import { dingTalkAPI, publicDingTalkApps, type DingTalkApp, type DingTalkDirectory, type DingTalkManager, type DingTalkMember, type DingTalkGrant, type DingTalkGrantInput, type DingTalkSyncJob } from '@/api/dingtalk'

const props = withDefaults(defineProps<{ mode?: 'organization' | 'allocation' }>(), { mode: 'organization' })
const allocationMode = computed(() => props.mode === 'allocation')
const auth = useAuthStore()
const isAdmin = computed(() => auth.isAdmin)
const api = dingTalkAPI(auth.isAdmin)
const { locale } = useI18n()
const text = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const money = (n: number) => `$${n.toFixed(2)}`
const apps = ref<DingTalkApp[]>([])
const savedIDs = ref(new Set<string>())
const hasDefault = ref(false)
const bindingApps = ref<{ id: string; name: string }[]>([])
const bindingApp = ref('')
const choices = computed(() => [...(hasDefault.value ? [{ id: 'default', name: text('默认钉钉应用', 'Default DingTalk app') }] : []), ...apps.value.filter(a => a.enabled && savedIDs.value.has(a.id))])
const selectedApp = ref('')
const selectedDepartment = ref(0)
const directory = ref<DingTalkDirectory>({ departments: [], members: [], synced_at: null })
const expandedDepartments = ref(new Set<number>())
const departmentRows = computed(() => buildDingTalkDepartmentRows(directory.value.departments))
const departmentTree = ref<HTMLElement | null>(null)
const selectedDepartmentPath = computed(() => dingTalkDepartmentPath(directory.value.departments, selectedDepartment.value))
async function locateDepartment() {
  selectedDepartmentPath.value.slice(0, -1).forEach(d => expandedDepartments.value.add(d.id))
  await nextTick()
  departmentTree.value?.querySelector<HTMLElement>(`[data-department-id="${selectedDepartment.value}"]`)?.scrollIntoView?.({ block: 'nearest', behavior: 'smooth' })
}
watch(() => selectedDepartmentPath.value.map(d => d.id).join('/'), () => { void locateDepartment() })
const visibleDepartmentRows = computed(() => {
  let hiddenBelow = Infinity
  return departmentRows.value.filter(d => {
    if (d.depth > hiddenBelow) return false
    hiddenBelow = expandedDepartments.value.has(d.id) ? Infinity : d.depth
    return true
  })
})
function toggleCollapse(id: number) {
  if (expandedDepartments.value.has(id)) expandedDepartments.value.delete(id)
  else expandedDepartments.value.add(id)
}
watch(selectedApp, () => { expandedDepartments.value.clear() })
const memberSearch = ref('')
const memberPage = ref(1)
const memberPageSize = 20
const selectedDepartmentScope = computed(() => {
  const children = new Map<number, number[]>()
  for (const d of directory.value.departments) { const ids = children.get(d.parent_id) || []; ids.push(d.id); children.set(d.parent_id, ids) }
  const scope = new Set<number>()
  const queue = [selectedDepartment.value]
  while (queue.length) { const id = queue.pop()!; if (scope.has(id)) continue; scope.add(id); queue.push(...(children.get(id) || [])) }
  return scope
})
const members = computed(() => {
  const query = memberSearch.value.trim().toLocaleLowerCase()
  const seen = new Set<string>()
  return directory.value.members.filter(m => {
    if (allocationMode.value ? !selectedDepartmentScope.value.has(m.department_id) : !query && m.department_id !== selectedDepartment.value) return false
    if (query && ![m.name, m.staff_id, m.user_id || ''].some(v => String(v).toLocaleLowerCase().includes(query))) return false
    // One recharge action per person, retaining a real authorized membership for the grant.
    const key = m.user_id ? `user:${m.user_id}` : `staff:${m.staff_id}`
    if (seen.has(key)) return false
    seen.add(key)
    return true
  })
})
const pagedMembers = computed(() => members.value.slice((memberPage.value - 1) * memberPageSize, memberPage.value * memberPageSize))
watch([memberSearch, selectedDepartment, directory], () => { memberPage.value = 1 })
const managers = ref<DingTalkManager[]>([])
const displayedManagers = computed(() => allocationMode.value && isAdmin.value ? [] : managers.value)
const grants = ref<DingTalkGrant[]>([])
const grantMember = ref<DingTalkMember | null>(null)
const grantAmount = ref(100)
const grantApp = ref('')
const pendingGrant = ref<DingTalkGrantInput | null>(null)
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const callbackURL = `${window.location.origin}/api/v1/auth/oauth/dingtalk/callback`
function departmentName(app: string, id: number) { return app === selectedApp.value ? directory.value.departments.find(d => d.id === id)?.name || `#${id}` : `#${id}` }
async function perform(work: () => Promise<void>) {
  if (busy.value) return
  busy.value = true; error.value = ''; notice.value = ''
  try { await work() } catch (e) { error.value = (e as { message?: string }).message || text('操作失败', 'Operation failed') } finally { busy.value = false }
}
const syncJob = ref<DingTalkSyncJob | null>(null)
let syncTimer: ReturnType<typeof setTimeout> | undefined
let disposed = false
function stopSyncPolling() { clearTimeout(syncTimer); syncTimer = undefined }
async function pollSync(app: string) {
  stopSyncPolling()
  if (disposed || selectedApp.value !== app) return
  try {
    const job = await api.syncStatus(app)
    if (disposed || selectedApp.value !== app) return
    const wasRunning = syncJob.value?.status === 'running'
    syncJob.value = job
    if (wasRunning && job.status === 'succeeded') {
      await refreshDirectory(false)
      notice.value = text('组织架构与成员已同步', 'Directory and members synced')
    }
    if (job.status !== 'running') return
  } catch (e) { if (!disposed && selectedApp.value === app) error.value = (e as Error).message || text('同步状态获取失败，正在重试', 'Unable to fetch sync status; retrying') }
  if (!disposed && selectedApp.value === app) syncTimer = setTimeout(() => void pollSync(app), 3000)
}
onUnmounted(() => { disposed = true; stopSyncPolling() })
async function refreshDirectory(checkSync = true) {
  directory.value = { departments: [], members: [], synced_at: null }
  if (!selectedApp.value) return
  const app = selectedApp.value
  const loaded = await api.directory(app)
  if (disposed || app !== selectedApp.value) return
  directory.value = loaded
  selectedDepartment.value = departmentRows.value.some(d => d.id === selectedDepartment.value) ? selectedDepartment.value : departmentRows.value[0]?.id || 0
  if (isAdmin.value && checkSync) { syncJob.value = null; await pollSync(app) }
}
async function loadDirectory() { await perform(async () => { memberSearch.value = ''; await refreshDirectory() }) }
async function refreshAccounting() { [managers.value, grants.value] = await Promise.all([api.managers(), api.grants()]) }
async function bindApplication() { await perform(() => startOAuthBinding('dingtalk', { redirectTo: '/organization/dingtalk', dingTalkAppID: bindingApp.value })) }
function addApp() { apps.value.push({ id: '', name: '', client_id: '', client_secret: '', redirect_url: callbackURL, corp_id: '', enabled: true }) }
async function saveApps() { await perform(async () => { apps.value = await api.saveApps(apps.value); savedIDs.value = new Set(apps.value.map(a => a.id)); if (!choices.value.some(a => a.id === selectedApp.value)) selectedApp.value = choices.value[0]?.id || ''; await refreshDirectory(); notice.value = text('应用配置已保存', 'Applications saved') }) }
async function syncDirectory() {
  await perform(async () => {
    const app = selectedApp.value
    syncJob.value = await api.sync(app)
    notice.value = text('已提交后台同步任务', 'Background sync submitted')
    await pollSync(app)
  })
}
function openGrant(member: DingTalkMember) { if (pendingGrant.value) return; grantMember.value = member; grantApp.value = selectedApp.value; grantAmount.value = 100 }
async function submitGrant() {
  await perform(async () => {
    if (!grantMember.value) return
    pendingGrant.value ||= { app_id: grantApp.value, department_id: grantMember.value.department_id, target_id: grantMember.value.user_id, amount: grantAmount.value, request_id: crypto.randomUUID() }
    try { await api.grant(pendingGrant.value) } catch (e) {
      const status = (e as { status?: number; response?: { status?: number } }).status || (e as { response?: { status?: number } }).response?.status
      if (status && status >= 400 && status < 500) pendingGrant.value = null
      throw e
    }
    const creditedSelf = grantMember.value.user_id === auth.user?.id
    pendingGrant.value = null; grantMember.value = null
    notice.value = text('额度已到账', 'Quota credited')
    await Promise.all([refreshDirectory(), refreshAccounting(), ...(creditedSelf ? [auth.refreshUser()] : [])])
  })
}
onMounted(async () => {
  await perform(async () => {
    apps.value = await api.apps(); savedIDs.value = new Set(apps.value.map(a => a.id))
    bindingApps.value = await publicDingTalkApps(); bindingApp.value = bindingApps.value[0]?.id || ''
    if (isAdmin.value) hasDefault.value = bindingApps.value.some(a => a.id === 'default')
    else hasDefault.value = apps.value.some(a => a.id === 'default')
    apps.value = apps.value.filter(a => a.id !== 'default')
    selectedApp.value = choices.value[0]?.id || ''
    await Promise.all([refreshDirectory(), refreshAccounting()])
  })
  loading.value = false
})
</script>
