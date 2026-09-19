<template>
  <AppLayout>
    <div class="space-y-6">
      <div>
        <h1 class="text-2xl font-semibold">{{ text('组织部门额度统计', 'Organization quota statistics') }}</h1>
        <p class="mt-2 text-sm text-gray-500">{{ text('按当前钉钉组织归属统计实际消费（美元），部门包含子部门，同一汇总内按平台用户去重。跨部门、跨公司的排行金额不可直接相加。仅统计已绑定的平台成员和保留的使用记录。', 'Actual usage cost in USD, attributed to the current DingTalk directory. Departments include descendants; each aggregate deduplicates platform users. Overlapping department or company rows cannot be added together. Only linked members and retained usage logs are included.') }}</p>
      </div>
      <form class="card grid items-end gap-4 p-5 sm:grid-cols-2 lg:grid-cols-5" @submit.prevent="query">
        <label class="text-sm">{{ text('开始日期', 'Start date') }}<input v-model="startDate" data-testid="statistics-start" type="date" :disabled="loading" required class="input mt-1" /></label>
        <label class="text-sm">{{ text('结束日期（含当天）', 'End date (inclusive)') }}<input v-model="endDate" data-testid="statistics-end" type="date" :disabled="loading" required :min="startDate" class="input mt-1" /></label>
        <div><label for="statistics-company" class="text-sm">{{ text('公司', 'Company') }}</label><Select id="statistics-company" v-model="company" :options="companyOptions" :disabled="loading" searchable @change="department = ''" /></div>
        <div><label for="statistics-department" class="text-sm">{{ text('部门（含子部门）', 'Department (including descendants)') }}</label><Select id="statistics-department" v-model="department" :options="departmentOptions" :disabled="loading" searchable /></div>
        <button class="btn btn-primary" :disabled="loading">{{ loading ? text('查询中…', 'Loading…') : text('查询', 'Search') }}</button>
      </form>
      <p v-if="error" role="alert" class="rounded-lg bg-red-50 p-3 text-red-700 dark:bg-red-950">{{ error }}</p>
      <template v-if="result">
        <p class="text-sm text-gray-500">{{ text('查询区间（本地时间）：', 'Query period (local time): ') }}{{ queriedPeriod }}</p>
        <div class="grid gap-4 sm:grid-cols-3">
          <div class="card p-5"><p class="text-sm text-gray-500">{{ text('实际消费', 'Actual cost') }}</p><p class="mt-2 text-2xl font-semibold">{{ money(result.total.cost) }}</p></div>
          <div class="card p-5"><p class="text-sm text-gray-500">{{ text('请求次数', 'Requests') }}</p><p class="mt-2 text-2xl font-semibold">{{ result.total.requests.toLocaleString() }}</p></div>
          <div class="card p-5"><p class="text-sm text-gray-500">{{ text('已绑定成员', 'Linked members') }}</p><p class="mt-2 text-2xl font-semibold">{{ result.total.members.toLocaleString() }}</p></div>
        </div>
        <section class="card space-y-4 p-5">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex flex-wrap gap-2" role="tablist" :aria-label="text('使用排行', 'Usage rankings')">
              <button v-for="tab in tabs" :key="tab.value" type="button" role="tab" :aria-selected="ranking === tab.value" class="btn" :class="ranking === tab.value ? 'btn-primary' : 'btn-secondary'" @click="ranking = tab.value">{{ tab.label }}</button>
            </div>
            <input v-model="search" type="search" class="input max-w-xs" :aria-label="text('搜索排行', 'Search rankings')" :placeholder="text('搜索名称或 ID', 'Search name or ID')" />
          </div>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-sm">
              <thead><tr><th class="p-2">{{ ranking === 'users' ? text('排名', 'Rank') : text('同级消费排名', 'Rank within parent') }}</th><th class="p-2">{{ text('名称', 'Name') }}</th><th v-if="ranking !== 'users'" class="p-2">{{ text('成员数', 'Members') }}</th><th class="p-2">{{ text('请求次数', 'Requests') }}</th><th class="p-2">{{ text('实际消费 ↓', 'Actual cost ↓') }}</th></tr></thead>
              <tbody>
                <tr v-for="row in pagedRows" :key="row.key" :data-depth="row.depth" class="border-t dark:border-dark-600">
                  <td class="p-2">{{ row.rank }}</td>
                  <td class="p-2"><div class="flex items-center gap-1" :style="{ paddingLeft: `${row.depth * 20}px` }">
                    <button v-if="row.hasChildren" type="button" class="shrink-0 rounded p-1" :aria-expanded="!!search.trim() || expandedRows.has(row.key)" :aria-label="`${text('展开/收起', 'Expand/collapse')} ${row.name}`" :disabled="!!search.trim()" @click="toggleRow(row.key)">{{ search.trim() || expandedRows.has(row.key) ? '▾' : '▸' }}</button>
                    <span v-else class="w-6 shrink-0" />
                    <div>{{ row.name }}<p class="text-xs text-gray-500">{{ row.app_id ? `${appName(row.app_id)} / ` : '' }}#{{ row.id }}</p></div>
                  </div></td>
                  <td v-if="ranking !== 'users'" class="p-2">{{ row.members }}</td><td class="p-2">{{ row.requests.toLocaleString() }}</td><td class="p-2 font-medium">{{ money(row.cost) }}</td>
                </tr>
              </tbody>
            </table>
            <p v-if="!rows.length" class="py-8 text-center text-gray-500">{{ text('暂无匹配数据；请检查组织同步、成员绑定和筛选条件。', 'No matching data. Check directory sync, linked members and filters.') }}</p>
          </div>
          <div v-if="pageCount > 1" class="flex items-center justify-end gap-3 text-sm"><button class="btn btn-secondary" :disabled="page === 1" @click="page--">{{ text('上一页', 'Previous') }}</button><span>{{ page }} / {{ pageCount }}</span><button class="btn btn-secondary" :disabled="page >= pageCount" @click="page++">{{ text('下一页', 'Next') }}</button></div>
        </section>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select from '@/components/common/Select.vue'
import { dingTalkAPI, type DingTalkStatistics } from '@/api/dingtalk'
import { buildDingTalkUsageTree, flattenDingTalkUsageTree } from '@/utils/dingtalkUsageTree'
import { useAuthStore } from '@/stores/auth'

const { locale } = useI18n()
const text = (zh: string, en: string) => locale.value.startsWith('zh') ? zh : en
const money = (n: number) => `$${n.toFixed(4)}`
const api = dingTalkAPI(useAuthStore().isAdmin)
function dateInput(d: Date) { return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}` }
const today = new Date()
const firstDay = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 29)
const startDate = ref(dateInput(firstDay))
const endDate = ref(dateInput(today))
const company = ref('')
const department = ref('')
const result = ref<DingTalkStatistics | null>(null)
const organizations = ref<DingTalkStatistics['organizations']>([])
const loading = ref(false)
const error = ref('')
const queriedPeriod = ref('')
const ranking = ref<'companies' | 'departments' | 'users'>('companies')
const tabs = computed(() => [
  { value: 'companies' as const, label: text('公司使用排行', 'Company ranking') },
  { value: 'departments' as const, label: text('部门使用排行', 'Department ranking') },
  { value: 'users' as const, label: text('个人使用排行', 'Member ranking') }
])
const companyOptions = computed(() => [{ value: '', label: text('全部公司', 'All companies') }, ...[...new Map(organizations.value.map(a => [a.company_id, { value: a.company_id, label: a.name }]) || []).values()]])
const departmentOptions = computed(() => [{ value: '', label: text('全部部门', 'All departments') }, ...organizations.value.filter(a => !company.value || company.value === a.company_id).flatMap(a => a.departments.map(d => ({ value: `${a.id}:${d.id}`, label: `${a.name} / ${d.name} (#${d.id})` })))])
const search = ref('')
const page = ref(1)
const pageSize = 50
const expandedRows = ref(new Set<string>())
const treeRoots = computed(() => result.value ? buildDingTalkUsageTree(result.value, ranking.value) : [])
const rows = computed(() => flattenDingTalkUsageTree(treeRoots.value, expandedRows.value, search.value))
const visibleRootKeys = computed(() => [...new Set(rows.value.map(row => row.rootKey))])
const pageCount = computed(() => Math.ceil(visibleRootKeys.value.length / pageSize))
// Paginate complete root branches so children never appear without their parent.
const pagedRows = computed(() => {
  const roots = new Set(visibleRootKeys.value.slice((page.value - 1) * pageSize, page.value * pageSize))
  return rows.value.filter(row => roots.has(row.rootKey))
})
function toggleRow(key: string) { if (expandedRows.value.has(key)) expandedRows.value.delete(key); else expandedRows.value.add(key) }
watch([ranking, result], () => { expandedRows.value = new Set(treeRoots.value.map(node => node.key)) })
watch([ranking, search, result], () => { page.value = 1 })
function appName(id: string) { return organizations.value.find(a => a.id === id)?.name || id }
async function query() {
  if (loading.value) return
  error.value = ''
  const start = new Date(`${startDate.value}T00:00:00`)
  const end = new Date(`${endDate.value}T00:00:00`)
  end.setDate(end.getDate() + 1)
  if (!Number.isFinite(start.getTime()) || !Number.isFinite(end.getTime()) || start >= end) { error.value = text('请选择有效的起止日期', 'Choose a valid date range'); return }
  loading.value = true
  const period = `${startDate.value} — ${endDate.value}`
  const [app, dept] = department.value.split(':')
  try {
    result.value = await api.statistics({ start: start.toISOString(), end: end.toISOString(), company_id: company.value || undefined, app_id: app || undefined, department_id: dept ? Number(dept) : undefined })
    organizations.value = result.value.organizations
    queriedPeriod.value = period
  } catch (e) { result.value = null; error.value = (e as Error).message || text('统计加载失败', 'Unable to load statistics') }
  finally { loading.value = false }
}
onMounted(query)
</script>
