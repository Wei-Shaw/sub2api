<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">系统检查</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            只看老板当天需要确认的四件事：本机服务、试跑任务、备份、内网访问。
          </p>
        </div>
        <button class="btn btn-outline" type="button" :disabled="loading" @click="loadChecks">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
          刷新
        </button>
      </div>

      <section class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <article
          v-for="item in checkItems"
          :key="item.title"
          class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800"
        >
          <div class="flex items-start justify-between gap-3">
            <div>
              <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ item.title }}</h2>
              <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-gray-400">{{ item.description }}</p>
            </div>
            <span class="shrink-0 rounded-md px-2 py-1 text-xs font-semibold" :class="item.className">
              {{ item.status }}
            </span>
          </div>
        </article>
      </section>

      <details class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">上线边界</summary>
        <div class="mt-4 grid gap-3 text-sm text-gray-600 dark:text-gray-300 md:grid-cols-3">
          <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-500/20 dark:bg-amber-500/10">正式上线未开启</div>
          <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-500/20 dark:bg-amber-500/10">商业化未开启</div>
          <div class="rounded-lg border border-amber-200 bg-amber-50 p-3 dark:border-amber-500/20 dark:bg-amber-500/10">真实生成通道未接入</div>
        </div>
      </details>

      <details class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <summary class="cursor-pointer text-sm font-semibold text-gray-900 dark:text-white">给技术同学执行</summary>
        <div class="mt-4 space-y-3 text-sm text-gray-600 dark:text-gray-300">
          <p>如第二台电脑打不开，先在 Windows 管理员 PowerShell 执行启用脚本；演示结束后可执行回滚脚本。</p>
          <pre class="overflow-x-auto rounded bg-slate-950 p-4 text-xs leading-6 text-slate-100"><code>.\deploy\day0\windows_enable_lan_access.ps1
.\deploy\day0\windows_disable_lan_access.ps1</code></pre>
        </div>
      </details>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'

type CheckState = 'normal' | 'warning' | 'unknown'

const appStore = useAppStore()
const loading = ref(false)
const serviceReady = ref<CheckState>('unknown')
const setupReady = ref<CheckState>('unknown')
const taskCount = ref<number | null>(null)

const statusClass = {
  normal: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300',
  warning: 'bg-amber-50 text-amber-700 dark:bg-amber-500/10 dark:text-amber-300',
  unknown: 'bg-slate-100 text-slate-700 dark:bg-dark-700 dark:text-slate-200',
}

const checkItems = computed(() => [
  {
    title: '本机服务',
    description: serviceReady.value === 'normal' ? '网页和健康检查已响应。' : '等待刷新确认。',
    status: serviceReady.value === 'normal' ? '正常' : '待确认',
    className: statusClass[serviceReady.value],
  },
  {
    title: '试跑任务',
    description: taskCount.value && taskCount.value > 0 ? `已记录 ${taskCount.value} 条任务。` : '可从试跑任务页提交一条任务验证。',
    status: taskCount.value && taskCount.value > 0 ? '正常' : '可试跑',
    className: taskCount.value && taskCount.value > 0 ? statusClass.normal : statusClass.unknown,
  },
  {
    title: '备份',
    description: setupReady.value === 'normal' ? '本机数据已可按操作卡备份。' : '等待服务初始化完成。',
    status: setupReady.value === 'normal' ? '可用' : '待确认',
    className: statusClass[setupReady.value],
  },
  {
    title: '内网访问',
    description: '本机浏览器可先确认；第二台电脑需现场打开地址验证。',
    status: '现场确认',
    className: statusClass.warning,
  },
])

const backendRoot = (import.meta.env.VITE_API_BASE_URL || '/api/v1')
  .replace(/\/api\/v1\/?$/, '')
  .replace(/\/$/, '')

function backendFetch(path: string): Promise<Response> {
  return fetch(`${backendRoot}${path}`, { cache: 'no-store' })
}

async function loadChecks() {
  loading.value = true
  try {
    const [healthResponse, setupResponse, dashboard] = await Promise.all([
      backendFetch('/health').catch(() => null),
      backendFetch('/setup/status').catch(() => null),
      adminAPI.video.dashboard().catch(() => null),
    ])
    serviceReady.value = healthResponse?.ok ? 'normal' : 'warning'
    setupReady.value = setupResponse?.ok ? 'normal' : 'warning'
    taskCount.value = dashboard?.today_tasks ?? 0
  } catch (err) {
    serviceReady.value = 'warning'
    appStore.showError(extractApiErrorMessage(err, '系统检查失败'))
  } finally {
    loading.value = false
  }
}

onMounted(loadChecks)
</script>
