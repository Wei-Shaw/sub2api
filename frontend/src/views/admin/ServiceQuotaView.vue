<template>
  <div class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-gray-100">服务配额</h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">管理 RPM、TPM、TPD、每日 USD 与并发规则。</p>
      </div>
      <button class="rounded bg-blue-600 px-4 py-2 text-sm text-white" @click="openCreate">新增规则</button>
    </div>

    <div class="overflow-x-auto rounded-lg border border-gray-200 dark:border-gray-700">
      <table class="min-w-full text-sm">
        <thead class="bg-gray-50 text-left dark:bg-gray-800">
          <tr>
            <th class="px-4 py-3">状态</th>
            <th class="px-4 py-3">作用域</th>
            <th class="px-4 py-3">类型</th>
            <th class="px-4 py-3">目标模式</th>
            <th class="px-4 py-3">窗口</th>
            <th class="px-4 py-3">额度</th>
            <th class="px-4 py-3">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="rule in rules" :key="rule.id" class="border-t border-gray-100 dark:border-gray-700">
            <td class="px-4 py-3">{{ rule.enabled ? '启用' : '停用' }}</td>
            <td class="px-4 py-3">{{ scopeText(rule) }}</td>
            <td class="px-4 py-3">{{ rule.limiter_type }}</td>
            <td class="px-4 py-3">{{ rule.target_mode }}</td>
            <td class="px-4 py-3">{{ rule.limiter_type === 'concurrency' ? '-' : rule.window_mode }}</td>
            <td class="px-4 py-3">{{ rule.limit_value }}</td>
            <td class="space-x-2 px-4 py-3">
              <button class="text-blue-600" @click="openEdit(rule)">编辑</button>
              <button class="text-red-600" @click="remove(rule.id)">删除</button>
            </td>
          </tr>
          <tr v-if="!loading && rules.length === 0"><td class="px-4 py-6 text-center text-gray-500" colspan="7">暂无规则</td></tr>
        </tbody>
      </table>
    </div>

    <form v-if="editing" class="grid gap-4 rounded-lg border p-4 dark:border-gray-700" @submit.prevent="save">
      <div class="grid grid-cols-2 gap-4 md:grid-cols-4">
        <label><span>启用</span><input v-model="form.enabled" type="checkbox" class="ml-2" /></label>
        <label><span>作用域</span><select v-model="form.scope_level" class="input"><option v-for="v in scopes" :key="v">{{ v }}</option></select></label>
        <label><span>类型</span><select v-model="form.limiter_type" class="input"><option v-for="v in limiters" :key="v">{{ v }}</option></select></label>
        <label><span>目标</span><select v-model="form.target_mode" class="input"><option v-for="v in targets" :key="v">{{ v }}</option></select></label>
        <label><span>平台</span><input v-model="form.platform" class="input" placeholder="anthropic/openai..." /></label>
        <label><span>分组 ID</span><input v-model.number="form.group_id" class="input" type="number" /></label>
        <label><span>账号 ID</span><input v-model.number="form.account_id" class="input" type="number" /></label>
        <label><span>模型匹配</span><input v-model="form.model_pattern" class="input" placeholder="claude-opus-*" /></label>
        <label><span>用户 ID</span><input v-model.number="form.target_user_id" class="input" type="number" /></label>
        <label><span>窗口</span><select v-model="form.window_mode" class="input"><option>fixed</option><option>rolling</option></select></label>
        <label><span>额度</span><input v-model.number="form.limit_value" class="input" min="0" step="0.000001" type="number" /></label>
      </div>
      <div class="space-x-2"><button class="rounded bg-blue-600 px-4 py-2 text-white">保存</button><button type="button" @click="editing = false">取消</button></div>
    </form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { createServiceQuotaRule, deleteServiceQuotaRule, listServiceQuotaRules, updateServiceQuotaRule, type ServiceQuotaRule, type ServiceQuotaRuleInput } from '@/api/admin/serviceQuota'

const scopes = ['global', 'platform', 'group', 'account', 'model']
const limiters = ['rpm', 'tpm', 'tpd', 'daily_usd', 'concurrency']
const targets = ['user', 'per_user', 'shared', 'default']
const rules = ref<ServiceQuotaRule[]>([])
const loading = ref(false)
const editing = ref(false)
const editingID = ref<number | null>(null)
const form = reactive<ServiceQuotaRuleInput>(blankRule())

function blankRule(): ServiceQuotaRuleInput { return { enabled: true, scope_level: 'global', limiter_type: 'rpm', target_mode: 'per_user', window_mode: 'fixed', limit_value: 60 } }
function clean<T>(v: T): T | null { return v === '' || v === 0 ? null : v }
function scopeText(r: ServiceQuotaRule): string { return [r.scope_level, r.platform, r.group_id && `group:${r.group_id}`, r.account_id && `acct:${r.account_id}`, r.model_pattern].filter(Boolean).join(' / ') }
async function load() { loading.value = true; try { rules.value = await listServiceQuotaRules() } finally { loading.value = false } }
function openCreate() { Object.assign(form, blankRule()); editingID.value = null; editing.value = true }
function openEdit(rule: ServiceQuotaRule) { Object.assign(form, rule); editingID.value = rule.id; editing.value = true }
async function save() { const payload = { ...form, platform: clean(form.platform), group_id: clean(form.group_id), account_id: clean(form.account_id), model_pattern: clean(form.model_pattern), target_user_id: clean(form.target_user_id) }; editingID.value ? await updateServiceQuotaRule(editingID.value, payload) : await createServiceQuotaRule(payload); editing.value = false; await load() }
async function remove(id: number) { await deleteServiceQuotaRule(id); await load() }
onMounted(load)
</script>

<style scoped>
.input { @apply mt-1 block w-full rounded border border-gray-300 bg-white px-2 py-1 dark:border-gray-600 dark:bg-gray-900; }
label span { @apply block text-xs text-gray-500; }
</style>
