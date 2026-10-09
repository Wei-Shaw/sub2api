<template>
  <section class="card p-4" :aria-label="t('usage.uaDistribution')" :aria-busy="loading">
    <h3 class="mb-4 text-sm font-semibold text-gray-900 dark:text-white">{{ t('usage.uaDistribution') }}</h3>
    <div v-if="loading" class="flex h-48 items-center justify-center" role="status">
      <LoadingSpinner />
    </div>
    <div v-else-if="error" class="flex h-48 flex-col items-center justify-center gap-3" role="alert">
      <p class="text-sm text-red-600 dark:text-red-400">{{ t('usage.uaLoadError') }}</p>
      <button type="button" class="btn btn-secondary" @click="emit('retry')">{{ t('usage.uaRetry') }}</button>
    </div>
    <div v-else-if="total === 0" class="flex h-48 items-center justify-center text-sm text-gray-500 dark:text-gray-400">
      {{ t('usage.uaNoData') }}
    </div>
    <div v-else class="flex flex-col items-center gap-6 sm:flex-row sm:items-start">
      <!-- 列表提供完整、可操作的文本数据，画布作为辅助视图避免屏幕阅读器重复朗读。 -->
      <div class="h-48 w-48 shrink-0" aria-hidden="true">
        <Doughnut :data="chartData" :options="chartOptions" />
      </div>
      <div class="max-h-96 w-full min-w-0 overflow-y-auto text-xs">
        <div class="grid grid-cols-[minmax(0,1fr)_5rem_4rem] gap-2 px-2 pb-2 text-gray-500 dark:text-gray-400">
          <span>{{ t('usage.uaClient') }}</span>
          <span class="text-right">{{ t('usage.uaRequests') }}</span>
          <span class="text-right">{{ t('usage.uaShare') }}</span>
        </div>
        <details v-for="(client, index) in clients" :key="client.name" class="border-t border-gray-100 dark:border-dark-700" data-testid="ua-client">
          <summary class="cursor-pointer rounded px-2 py-2 text-gray-900 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-white">
            <span class="inline-grid w-[calc(100%-1.25rem)] grid-cols-[minmax(0,1fr)_5rem_4rem] items-center gap-2 align-middle">
              <span class="flex min-w-0 items-center gap-2">
                <span class="h-2 w-2 shrink-0 rounded-full" :style="{ backgroundColor: colorAt(index) }" />
                <span class="break-all font-medium">{{ clientLabel(client.name) }}</span>
              </span>
              <span class="text-right tabular-nums">{{ client.requests.toLocaleString() }}</span>
              <span class="text-right tabular-nums">{{ share(client.requests) }}</span>
            </span>
          </summary>
          <div class="ml-3 border-l border-gray-200 pl-2 dark:border-dark-600">
            <details v-for="version in client.versions" :key="version.name" data-testid="ua-version">
              <summary class="cursor-pointer rounded px-2 py-2 text-gray-700 focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:text-gray-300">
                <span class="inline-flex w-[calc(100%-1.25rem)] items-start justify-between gap-3 align-middle">
                  <span class="min-w-0 break-all">{{ t('usage.uaVersion') }}: {{ version.name || t('usage.uaUnknownVersion') }}</span>
                  <span class="shrink-0 tabular-nums">{{ version.requests.toLocaleString() }}</span>
                </span>
              </summary>
              <ul class="space-y-2 bg-gray-50 p-3 dark:bg-dark-800">
                <li v-for="ua in version.agents" :key="ua.user_agent" class="flex items-start justify-between gap-3" data-testid="ua-raw">
                  <code class="min-w-0 whitespace-pre-wrap break-all text-gray-600 dark:text-gray-400">{{ ua.user_agent.trim() ? ua.user_agent : t('usage.uaMissing') }}</code>
                  <span class="shrink-0 text-gray-700 tabular-nums dark:text-gray-300">{{ ua.requests.toLocaleString() }}</span>
                </li>
              </ul>
            </details>
          </div>
        </details>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Chart as ChartJS, ArcElement, Tooltip, Legend } from 'chart.js'
import { Doughnut } from 'vue-chartjs'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import type { UserAgentStat } from '@/api/admin/usage'

ChartJS.register(ArcElement, Tooltip, Legend)

const props = withDefaults(defineProps<{
  stats: UserAgentStat[]
  loading?: boolean
  error?: boolean
}>(), { loading: false, error: false })
const emit = defineEmits<{ retry: [] }>()
const { t } = useI18n()

const colors = ['#3b82f6', '#10b981', '#f59e0b', '#ef4444', '#8b5cf6', '#ec4899', '#14b8a6', '#f97316']
const colorAt = (index: number) => colors[index] || '#9ca3af'
const clientLabelKeys = new Map([['__browser__', 'uaBrowser'], ['__missing__', 'uaMissing'], ['__unknown__', 'uaUnknown']])
const clientLabel = (name: string) => {
  const key = clientLabelKeys.get(name)
  return key ? t(`usage.${key}`) : name
}

type VersionGroup = { name: string; requests: number; agents: UserAgentStat[] }
const byRequests = (a: { requests: number; name: string }, b: { requests: number; name: string }) =>
  b.requests - a.requests || a.name.localeCompare(b.name)

// 服务端返回整个筛选范围的 UA 聚合；这里只合并展示层级，不从分页明细估算占比。
const clients = computed(() => {
  const groups = new Map<string, { name: string; requests: number; versions: Map<string, VersionGroup> }>()
  for (const row of props.stats) {
    let client = groups.get(row.client)
    if (!client) {
      client = { name: row.client, requests: 0, versions: new Map() }
      groups.set(row.client, client)
    }
    client.requests += row.requests
    let version = client.versions.get(row.version)
    if (!version) {
      version = { name: row.version, requests: 0, agents: [] }
      client.versions.set(row.version, version)
    }
    version.requests += row.requests
    version.agents.push(row)
  }
  return [...groups.values()].map(client => ({
    ...client,
    versions: [...client.versions.values()].sort(byRequests).map(version => ({
      ...version,
      agents: version.agents.sort((a, b) => b.requests - a.requests || a.user_agent.localeCompare(b.user_agent)),
    })),
  })).sort(byRequests)
})

const total = computed(() => clients.value.reduce((sum, client) => sum + client.requests, 0))
const share = (requests: number) => `${(total.value ? requests / total.value * 100 : 0).toFixed(1)}%`
const chartData = computed(() => {
  const visible = clients.value.slice(0, 8).map(client => ({ name: clientLabel(client.name), requests: client.requests }))
  // 仅压缩画布扇区，列表保留全部分类，“其他”也计入总请求数。
  if (clients.value.length > 8) {
    visible.push({ name: t('usage.uaOther'), requests: clients.value.slice(8).reduce((sum, client) => sum + client.requests, 0) })
  }
  return {
    labels: visible.map(client => client.name),
    datasets: [{ data: visible.map(client => client.requests), backgroundColor: visible.map((_, index) => colorAt(index)), borderWidth: 0 }],
  }
})
const chartOptions = {
  responsive: true,
  maintainAspectRatio: false,
  plugins: {
    legend: { display: false },
    tooltip: { callbacks: { label: (context: { label: string; raw: unknown }) => `${context.label}: ${Number(context.raw).toLocaleString()} (${share(Number(context.raw))})` } },
  },
}
</script>
