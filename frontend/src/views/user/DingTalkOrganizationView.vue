<template>
  <AppLayout>
    <div class="space-y-6">
      <div>
        <h1 class="text-2xl font-semibold">{{ text('钉钉组织与额度', 'DingTalk organizations and quota') }}</h1>
        <p class="mt-2 text-sm text-gray-500">{{ text('组织架构来自钉钉。负责人可为所管部门及子部门成员增加余额，累计分配不能超过管理员设定的总额度。', 'The directory comes from DingTalk. Managers can add balance to members of their departments and subdepartments within a cumulative budget.') }}</p>
      </div>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950">{{ error }}</p>
      <p v-if="notice" role="status" class="rounded-lg bg-green-50 p-3 text-green-700 dark:bg-green-950">{{ notice }}</p>
      <p v-if="loading">{{ text('加载中…', 'Loading…') }}</p>

      <div v-if="bindingApps.length" class="card flex flex-wrap items-end gap-3 p-5">
        <label class="min-w-48 flex-1 text-sm">{{ text('绑定我的钉钉应用', 'Link my DingTalk application') }}
          <select v-model="bindingApp" class="input mt-1"><option v-for="app in bindingApps" :key="app.id" :value="app.id">{{ app.name }}</option></select>
        </label>
        <button class="btn btn-secondary" :disabled="busy" @click="bindApplication">{{ text('绑定 / 添加钉钉身份', 'Link / add DingTalk identity') }}</button>
      </div>

      <details v-if="isAdmin" class="card p-5">
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
          <button v-if="isAdmin" class="btn btn-primary" :disabled="busy" @click="syncDirectory">{{ busy ? text('处理中…', 'Working…') : text('同步钉钉组织', 'Sync DingTalk directory') }}</button>
        </div>
        <p class="text-sm text-gray-500">{{ text('最近同步：', 'Last synced: ') }}{{ directory.synced_at ? new Date(directory.synced_at).toLocaleString() : text('尚未同步', 'Never') }}. {{ text('分配额度要求组织数据在 24 小时内同步。', 'Quota allocation requires a directory synced within 24 hours.') }}</p>
        <div class="grid gap-4 lg:grid-cols-[260px_1fr]">
          <div class="space-y-1 border-r pr-3 dark:border-dark-600">
            <p class="mb-2 font-semibold">{{ text('部门', 'Departments') }}</p>
            <button v-for="dept in departmentRows" :key="dept.id" class="block w-full rounded px-2 py-2 text-left text-sm" :class="selectedDepartment === dept.id ? 'bg-primary-100 text-primary-700' : 'hover:bg-gray-100 dark:hover:bg-dark-700'" :style="{ paddingLeft: `${8 + dept.depth * 16}px` }" @click="selectedDepartment = dept.id">{{ dept.name }}</button>
            <p v-if="!departmentRows.length" class="text-sm text-gray-500">{{ text('暂无可管理的部门', 'No managed departments') }}</p>
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead><tr><th class="p-2">{{ text('成员', 'Member') }}</th><th class="p-2">{{ text('平台用户', 'Platform user') }}</th><th class="p-2">{{ text('余额', 'Balance') }}</th><th class="p-2">{{ text('操作', 'Action') }}</th></tr></thead>
              <tbody><tr v-for="member in members" :key="`${member.department_id}-${member.staff_id}`" class="border-t dark:border-dark-600">
                <td class="p-2">{{ member.name }}</td><td class="p-2">{{ member.user_id || text('尚未绑定', 'Not linked') }}</td><td class="p-2">{{ member.user_id ? money(member.balance) : '—' }}</td>
                <td class="p-2"><button class="btn btn-secondary" :disabled="busy || !member.user_id || member.user_id === auth.user?.id || staleDirectory" @click="openGrant(member)">{{ text('增加额度', 'Add quota') }}</button></td>
              </tr></tbody>
            </table>
            <p v-if="!members.length" class="py-6 text-gray-500">{{ text('此部门暂无成员', 'No members in this department') }}</p>
          </div>
        </div>
      </section>
      <p v-else-if="!loading" class="card p-5">{{ text('暂无可管理的组织。请先配置应用、同步组织，再由管理员配置负责人。', 'No managed organizations. Configure an app, sync the directory and assign department managers.') }}</p>

      <section class="card space-y-4 p-5">
        <h2 class="font-semibold">{{ isAdmin ? text('部门负责人及最大可分配额度', 'Department managers and allocation budgets') : text('我的可分配额度', 'My allocation budget') }}</h2>
        <div v-for="manager in managers" :key="manager.user_id" class="flex flex-wrap items-center justify-between gap-3 rounded border p-3 dark:border-dark-600">
          <div><p>{{ manager.name || `#${manager.user_id}` }} · {{ manager.enabled ? text('已启用', 'Enabled') : text('已撤销', 'Revoked') }}</p><p class="text-sm text-gray-500">{{ text('总额度 / 已分配 / 剩余：', 'Budget / allocated / remaining: ') }}{{ money(manager.limit_cents / 100) }} / {{ money(manager.used_cents / 100) }} / {{ money((manager.limit_cents - manager.used_cents) / 100) }}</p><p class="text-sm text-gray-500">{{ manager.departments.map(d => `${d.app_id} / ${departmentName(d.app_id, d.department_id)}`).join('、') }}</p></div>
          <button v-if="isAdmin" class="btn btn-secondary" :disabled="busy" @click="editManager(manager)">{{ text('编辑', 'Edit') }}</button>
        </div>
        <p class="text-sm text-gray-500">{{ text('总额度跨应用、跨部门累计；0 表示不能分配。调整权限不会重置已分配金额。', 'The cumulative budget covers every app and department. Zero allows no allocation. Permission changes never reset spending.') }}</p>
        <button v-if="isAdmin" class="btn btn-secondary" :disabled="busy" @click="editManager()">{{ text('添加负责人', 'Add manager') }}</button>
        <form v-if="managerForm" class="space-y-3 rounded border p-4 dark:border-dark-600" @submit.prevent="saveManager">
          <div class="grid gap-3 md:grid-cols-2">
            <label>{{ text('负责人平台用户 ID', 'Manager platform user ID') }}<input v-model.number="managerForm.user_id" class="input" type="number" min="1" required :readonly="editingManager" /></label>
            <label>{{ text('最大累计可分配额度', 'Maximum cumulative allocation') }}<input data-testid="manager-limit" v-model.number="managerLimit" class="input" type="number" :min="managerForm.used_cents / 100" max="1000000000" step="0.01" required /></label>
          </div>
          <label class="block"><input v-model="managerForm.enabled" type="checkbox" /> {{ text('允许分配额度', 'Allow quota allocation') }}</label>
          <p class="text-sm">{{ text('选择当前应用下负责的部门（包含子部门）；其它应用的授权会保留。', 'Select managed departments in this app, including descendants. Assignments in other apps are retained.') }}</p>
          <label v-for="dept in departmentRows" :key="dept.id" class="block text-sm"><input type="checkbox" :checked="managerForm.departments.some(d => d.app_id === selectedApp && d.department_id === dept.id)" @change="toggleDepartment(dept.id, ($event.target as HTMLInputElement).checked)" /> {{ '\u3000'.repeat(dept.depth) }}{{ dept.name }}</label>
          <div class="flex gap-3"><button class="btn btn-primary" :disabled="busy">{{ text('保存负责人', 'Save manager') }}</button><button type="button" class="btn btn-secondary" @click="managerForm = null">{{ text('取消', 'Cancel') }}</button></div>
        </form>
      </section>

      <section ref="grantPanel" v-if="grantMember" class="card space-y-3 p-5" role="region" :aria-label="text('分配额度', 'Allocate quota')">
        <h2 class="font-semibold">{{ text('为成员增加额度：', 'Add quota for: ') }}{{ grantMember.name }} (#{{ grantMember.user_id }})</h2>
        <form class="flex flex-wrap items-end gap-3" @submit.prevent="submitGrant">
          <label>{{ text('增加余额金额', 'Balance to add') }}<input data-testid="grant-amount" v-model.number="grantAmount" type="number" min="0.01" max="1000000000" step="0.01" required class="input" :disabled="!!pendingGrant" /></label>
          <button class="btn btn-primary" :disabled="busy || staleDirectory">{{ pendingGrant ? text('重试同一笔分配', 'Retry this allocation') : text('确认增加额度', 'Confirm allocation') }}</button>
          <button type="button" class="btn btn-secondary" :disabled="busy || !!pendingGrant" @click="grantMember = null">{{ text('取消', 'Cancel') }}</button>
        </form>
        <p v-if="pendingGrant" class="text-sm text-gray-500">{{ text('重试将使用相同请求编号，避免重复入账。', 'Retries use the same request ID to prevent duplicate credits.') }}</p>
      </section>

      <section class="card p-5">
        <h2 class="mb-3 font-semibold">{{ text('最近 100 笔分配记录', 'Latest 100 allocations') }}</h2>
        <div class="overflow-x-auto"><table class="w-full text-left text-sm"><thead><tr><th class="p-2">{{ text('时间', 'Time') }}</th><th class="p-2">{{ text('操作人 → 成员', 'Actor → member') }}</th><th class="p-2">{{ text('应用 / 部门', 'App / department') }}</th><th class="p-2">{{ text('金额', 'Amount') }}</th></tr></thead><tbody><tr v-for="grant in grants" :key="grant.id" class="border-t dark:border-dark-600"><td class="p-2">{{ new Date(grant.created_at).toLocaleString() }}</td><td class="p-2">#{{ grant.actor_id }} → #{{ grant.target_id }}</td><td class="p-2">{{ grant.app_id }} / {{ departmentName(grant.app_id, grant.department_id) }}</td><td class="p-2">+{{ money(grant.amount_cents / 100) }}</td></tr></tbody></table></div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { startOAuthBinding } from '@/api/user'
import { useAuthStore } from '@/stores/auth'
import { dingTalkAPI, publicDingTalkApps, type DingTalkApp, type DingTalkDirectory, type DingTalkManager, type DingTalkMember, type DingTalkGrant, type DingTalkGrantInput } from '@/api/dingtalk'

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
const staleDirectory = computed(() => !directory.value.synced_at || Date.now() - Date.parse(directory.value.synced_at) >= 86400000)
const departmentRows = computed(() => {
  const ds = directory.value.departments
  const byID = new Map(ds.map(d => [d.id, d]))
  const result: { id: number; name: string; depth: number }[] = []
  const seen = new Set<number>()
  function visit(id: number, depth: number) {
    if (seen.has(id)) return
    seen.add(id)
    const d = byID.get(id)
    if (!d) return
    result.push({ id, name: d.name, depth })
    ds.filter(child => child.parent_id === id && child.id !== id).forEach(child => visit(child.id, depth + 1))
  }
  ds.filter(d => !byID.has(d.parent_id)).forEach(d => visit(d.id, 0))
  return result
})
const members = computed(() => directory.value.members.filter(m => m.department_id === selectedDepartment.value))
const managers = ref<DingTalkManager[]>([])
const grants = ref<DingTalkGrant[]>([])
const managerForm = ref<DingTalkManager | null>(null)
const managerLimit = ref(0)
const editingManager = ref(false)
const grantPanel = ref<HTMLElement | null>(null)
const grantMember = ref<DingTalkMember | null>(null)
const grantAmount = ref(1)
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
async function refreshDirectory() {
  directory.value = { departments: [], members: [], synced_at: null }
  if (!selectedApp.value) return
  directory.value = await api.directory(selectedApp.value)
  selectedDepartment.value = departmentRows.value.some(d => d.id === selectedDepartment.value) ? selectedDepartment.value : departmentRows.value[0]?.id || 0
}
async function loadDirectory() { await perform(refreshDirectory) }
async function refreshAccounting() { [managers.value, grants.value] = await Promise.all([api.managers(), api.grants()]) }
async function bindApplication() { await perform(() => startOAuthBinding('dingtalk', { redirectTo: '/organization/dingtalk', dingTalkAppID: bindingApp.value })) }
function addApp() { apps.value.push({ id: '', name: '', client_id: '', client_secret: '', redirect_url: callbackURL, corp_id: '', enabled: true }) }
async function saveApps() { await perform(async () => { apps.value = await api.saveApps(apps.value); savedIDs.value = new Set(apps.value.map(a => a.id)); if (!choices.value.some(a => a.id === selectedApp.value)) selectedApp.value = choices.value[0]?.id || ''; await refreshDirectory(); notice.value = text('应用配置已保存', 'Applications saved') }) }
async function syncDirectory() { await perform(async () => { await api.sync(selectedApp.value); await refreshDirectory(); notice.value = text('组织架构与成员已同步', 'Directory and members synced') }) }
function editManager(manager?: DingTalkManager) {
  editingManager.value = !!manager
  managerForm.value = manager ? JSON.parse(JSON.stringify(manager)) : { user_id: 0, limit_cents: 0, used_cents: 0, enabled: true, departments: [] }
  managerLimit.value = (manager?.limit_cents || 0) / 100
}
function toggleDepartment(id: number, checked: boolean) {
  if (!managerForm.value) return
  managerForm.value.departments = managerForm.value.departments.filter(d => d.app_id !== selectedApp.value || d.department_id !== id)
  if (checked) managerForm.value.departments.push({ app_id: selectedApp.value, department_id: id })
}
async function saveManager() { await perform(async () => { if (!managerForm.value) return; await api.saveManager({ ...managerForm.value, limit_cents: Math.round(managerLimit.value * 100) }); managerForm.value = null; await refreshAccounting(); notice.value = text('负责人权限和额度已保存', 'Manager permissions and budget saved') }) }
async function openGrant(member: DingTalkMember) { if (pendingGrant.value) return; grantMember.value = member; grantApp.value = selectedApp.value; grantAmount.value = 1; await nextTick(); grantPanel.value?.scrollIntoView?.({ behavior: 'smooth', block: 'center' }) }
async function submitGrant() {
  await perform(async () => {
    if (!grantMember.value) return
    pendingGrant.value ||= { app_id: grantApp.value, department_id: grantMember.value.department_id, target_id: grantMember.value.user_id, amount: grantAmount.value, request_id: crypto.randomUUID() }
    try { await api.grant(pendingGrant.value) } catch (e) {
      const status = (e as { status?: number; response?: { status?: number } }).status || (e as { response?: { status?: number } }).response?.status
      if (status && status >= 400 && status < 500) pendingGrant.value = null
      throw e
    }
    pendingGrant.value = null; grantMember.value = null
    notice.value = text('额度已到账', 'Quota credited')
    await Promise.all([refreshDirectory(), refreshAccounting()])
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
