<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
        <div>
          <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '任务记录' : '任务列表' }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ isVideoGatewayDemoMode ? '查看谁发起、当前状态、失败原因。' : '按状态和通道查看最近任务，快速定位结果、失败原因并复制参数重新创建。' }}
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <RouterLink class="btn btn-primary" to="/admin/video/create">
            <Icon name="plus" size="sm" />
            {{ isVideoGatewayDemoMode ? '试跑一条任务' : '创建任务' }}
          </RouterLink>
          <button class="btn btn-outline" type="button" :disabled="loading" @click="loadTasks">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            刷新
          </button>
        </div>
      </div>

      <!-- 演示模式：视频任务 / AI 调用记录 切换 -->
      <div v-if="isVideoGatewayDemoMode && authStore.isAdmin" class="inline-flex rounded-lg border border-gray-200 bg-gray-50 p-0.5 dark:border-dark-700 dark:bg-dark-800">
        <span class="rounded-md bg-white px-4 py-1.5 text-sm font-medium text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white">
          视频任务
        </span>
        <RouterLink
          to="/admin/console/ai-records"
          class="rounded-md px-4 py-1.5 text-sm font-medium text-gray-500 transition-colors hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200"
        >
          AI 调用记录
        </RouterLink>
      </div>

      <section class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-800">
        <div class="grid gap-3 lg:grid-cols-[1fr_220px_auto] lg:items-center">
          <div class="flex flex-wrap gap-2">
            <button
              v-for="item in quickStatusFilters"
              :key="item.label"
              class="btn btn-sm"
              :class="filters.status === item.status ? 'btn-primary' : 'btn-outline'"
              type="button"
              @click="applyQuickStatus(item.status)"
            >
              {{ item.label }}
            </button>
          </div>
          <select v-model="filters.provider" class="input" @change="resetAndLoad">
            <option value="">{{ isVideoGatewayDemoMode ? '全部任务来源' : '全部通道' }}</option>
            <option v-for="provider in visibleProviderOptions" :key="provider.value" :value="provider.value">{{ provider.label }}</option>
          </select>
          <div class="flex justify-start lg:justify-end">
            <button class="btn btn-outline" type="button" @click="clearFilters">
              <Icon name="x" size="sm" />
              清空筛选
            </button>
          </div>
        </div>
      </section>

      <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="overflow-x-auto">
          <table class="min-w-full divide-y divide-gray-200 text-sm dark:divide-dark-700">
            <thead class="bg-gray-50 text-left text-xs uppercase text-gray-500 dark:bg-dark-700/40 dark:text-gray-400">
              <tr>
                <th class="px-5 py-3 font-medium">{{ isVideoGatewayDemoMode ? '试跑任务' : '任务' }}</th>
                <th v-if="authStore.isAdmin" class="px-5 py-3 font-medium">发起人</th>
                <th class="px-5 py-3 font-medium">{{ isVideoGatewayDemoMode ? '任务来源' : '通道' }}</th>
                <th class="px-5 py-3 font-medium">{{ isVideoGatewayDemoMode ? '处理账号' : '系统调度账号' }}</th>
                <th class="px-5 py-3 font-medium">当前状态</th>
                <th class="px-5 py-3 font-medium">花费</th>
                <th class="px-5 py-3 font-medium">结果 / 失败原因</th>
                <th class="px-5 py-3 font-medium">创建时间</th>
                <th class="px-5 py-3 font-medium">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
              <tr v-for="task in tasks" :key="task.id" class="hover:bg-gray-50 dark:hover:bg-dark-700/40">
                <td class="px-5 py-3">
                  <RouterLink class="font-medium text-gray-900 hover:text-primary-600 dark:text-white dark:hover:text-primary-300" :to="`/admin/video/tasks/${task.id}`">
                    #{{ task.id }} | {{ taskTypeLabel(task.task_type) }}
                  </RouterLink>
                  <div class="prompt-summary mt-1 max-w-xl text-xs text-gray-500 dark:text-gray-400">{{ promptDisplayText(task.prompt) }}</div>
                </td>
                <td v-if="authStore.isAdmin" class="px-5 py-3 text-gray-700 dark:text-gray-200">
                  {{ createdByLabel(task) }}
                </td>
                <td class="px-5 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="providerBadgeClass(task.provider)">
                    {{ providerLabel(task.provider) }}
                  </span>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ modelDisplayName(task.provider, task.model) }}</div>
                </td>
                <td class="px-5 py-3 text-gray-700 dark:text-gray-200">
                  <div class="font-medium">{{ routeAccountLabel(task) }}</div>
                  <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ isVideoGatewayDemoMode ? '系统自动调度' : '实际路由账号' }}</div>
                </td>
                <td class="px-5 py-3">
                  <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="statusBadgeClass(task.status)">
                    {{ statusLabel(task.status) }}
                  </span>
                  <span v-if="task.delivery_status" class="ml-1 inline-flex rounded-md bg-slate-100 px-2 py-1 text-[10px] font-medium text-slate-700 dark:bg-slate-500/15 dark:text-slate-200">
                    {{ deliveryStatusLabel(task) }}
                  </span>
                </td>
                <td class="px-5 py-3 tabular-nums text-teal-600 dark:text-teal-300">
                  {{ formatTaskCost(task) }}
                </td>
                <td class="px-5 py-3">
                  <div v-if="hasDeliverableAsset(task)" class="space-y-2">
                    <video
                      v-if="hasUsableRemoteAsset(task)"
                      class="h-16 w-28 rounded border border-gray-200 object-cover dark:border-dark-600"
                      :src="task.result_url"
                      preload="metadata"
                      muted
                      playsinline
                    />
                    <div class="flex flex-wrap items-center gap-2">
                      <a
                        v-if="hasUsableRemoteAsset(task)"
                        class="inline-flex items-center gap-1 text-primary-600 hover:text-primary-700 dark:text-primary-300"
                        :href="task.result_url"
                        target="_blank"
                        rel="noreferrer"
                      >
                        <Icon name="externalLink" size="xs" />
                        打开结果
                      </a>
                      <span
                        v-if="task.local_asset_available"
                        class="rounded bg-emerald-50 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-200"
                      >
                        已归档
                      </span>
                    </div>
                    <div
                      v-if="hasUsableRemoteAsset(task) && resultExpiryLabel(task)"
                      class="text-xs"
                      :class="isResultExpiryNear(task) ? 'text-yellow-600 dark:text-yellow-300' : 'text-gray-500 dark:text-gray-400'"
                      :title="resultExpiryLabel(task)"
                    >
                      {{ resultExpiryLabel(task) }}
                    </div>
                  </div>
                  <div v-else-if="task.error_message" class="max-w-sm rounded-md border border-red-200 bg-red-50 p-2 text-xs leading-5 text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
                    <div class="font-medium">{{ isVideoGatewayDemoMode ? '任务失败原因' : '失败原因' }}</div>
                    <div>{{ errorMessageLabel(task.error_message) }}</div>
                  </div>
                  <span v-else class="text-gray-400">等待回收</span>
                </td>
                <td class="px-5 py-3 text-gray-500 dark:text-gray-400">{{ formatDate(task.created_at) }}</td>
                <td class="px-5 py-3">
                  <div class="flex flex-wrap gap-2">
                    <a v-if="hasUsableRemoteAsset(task)" class="btn btn-sm btn-outline" :href="task.result_url" target="_blank" rel="noreferrer">打开结果</a>
                    <button v-if="hasUsableRemoteAsset(task)" class="btn btn-sm btn-outline" type="button" @click="copyResultUrl(task)">
                      复制链接
                    </button>
                    <button
                      v-if="task.local_asset_available"
                      class="btn btn-sm btn-outline"
                      type="button"
                      @click="openLocalAsset(task)"
                    >
                      本地归档
                    </button>
                    <button class="btn btn-sm btn-outline" type="button" @click="copyToCreate(task)">
                      {{ isVideoGatewayDemoMode ? '复制参数重新提交' : '复制参数' }}
                    </button>
                    <RouterLink class="btn btn-sm btn-outline" :to="`/admin/video/tasks/${task.id}`">
                      {{ isVideoGatewayDemoMode ? '查看任务详情' : '查看详情' }}
                    </RouterLink>
                  </div>
                </td>
              </tr>
              <tr v-if="!loading && !tasks.length">
                <td :colspan="authStore.isAdmin ? 9 : 8" class="px-5 py-8 text-center text-sm text-gray-500 dark:text-gray-400">
                  <div class="space-y-3">
                    <div>{{ isVideoGatewayDemoMode ? '当前还没有任务记录。你可以先试跑一条任务，检查系统是否能正常接收、处理和记录。' : '没有任务。可以先创建一个演示任务验证流转。' }}</div>
                    <RouterLink class="btn btn-sm btn-outline" to="/admin/video/create">{{ isVideoGatewayDemoMode ? '试跑一条任务' : '创建一个演示任务' }}</RouterLink>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
        <div class="flex items-center justify-between border-t border-gray-200 px-5 py-4 text-sm dark:border-dark-700">
          <div class="text-gray-500 dark:text-gray-400">共 {{ pagination.total }} 条</div>
          <div class="flex gap-2">
            <button class="btn btn-sm btn-outline" type="button" :disabled="pagination.page <= 1" @click="changePage(pagination.page - 1)">上一页</button>
            <span class="flex items-center px-2 text-gray-700 dark:text-gray-200">{{ pagination.page }} / {{ pagination.pages }}</span>
            <button class="btn btn-sm btn-outline" type="button" :disabled="pagination.page >= pagination.pages" @click="changePage(pagination.page + 1)">下一页</button>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, watch } from 'vue'
import { RouterLink, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { videoTaskAPI, type VideoProvider, type VideoTask, type VideoTaskListParams, type VideoTaskStatus } from '@/api/admin/video'
import { preferredVideoTaskAsset, useVideoTaskLifecycle } from '@/composables/useVideoTaskLifecycle'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isVideoGatewayDemoMode } from '@/utils/productMode'
import { formatByCurrency } from '@/composables/useDisplayCurrency'
import { useAdminDisplayCurrencyRate } from '@/composables/useAdminDisplayCurrencyRate'
import {
  errorMessageLabel,
  formatDate,
  modelDisplayName,
  createdByLabel,
  providerBadgeClass,
  providerLabel,
  providerOptions,
  promptDisplayText,
  routeAccountLabel,
  saveTaskDraft,
  statusBadgeClass,
  statusLabel,
  taskTypeLabel,
  isTerminalStatus,
} from './videoUtils'

const appStore = useAppStore()
const authStore = useAuthStore()
const router = useRouter()
const { usdCnyRate, loadUsdCnyRate } = useAdminDisplayCurrencyRate()
const filters = reactive({ status: '', provider: '' })
const pagination = reactive({ page: 1, page_size: 20, total: 0, pages: 1 })
const lifecycle = useVideoTaskLifecycle<{ items: VideoTask[]; total: number; page: number; page_size: number; pages: number }>({
  autoStart: false,
  baseDelayMs: 4_000,
  terminalDelayMs: 15_000,
  fetch: (signal) => {
    const params: VideoTaskListParams = { page: pagination.page, page_size: pagination.page_size }
    if (filters.status) params.status = filters.status as VideoTaskStatus
    if (filters.provider) params.provider = filters.provider as VideoProvider
    return videoTaskAPI.list(params, signal)
  },
  shouldContinue: (value) => Boolean(value?.items.some((task) => !isTerminalStatus(task.status) || task.delivery_status === 'archiving')),
})
const loading = lifecycle.loading
const tasks = computed(() => lifecycle.data.value?.items ?? [])
watch(() => lifecycle.data.value, (res) => {
  if (!res) return
  pagination.total = res.total
  pagination.page = res.page
  pagination.page_size = res.page_size
  pagination.pages = res.pages
})
const visibleProviderOptions = computed(() => (
  isVideoGatewayDemoMode
    ? providerOptions.filter((provider) => provider.value === 'mock')
    : providerOptions
))
// Seedance 真实计费为人民币（V-2/V-3），其余按美元展示
function formatTaskCost(task: VideoTask): string {
  if (task.cost_estimate <= 0) return '—'
  return formatByCurrency(task.cost_estimate, task.currency, usdCnyRate.value)
}

function deliveryStatusLabel(task: VideoTask): string {
  switch (task.delivery_status) {
    case 'processing': return '处理中'
    case 'archiving': return '归档中'
    case 'deliverable': return task.local_asset_available ? '本地资产可下载' : '结果可下载'
    case 'delivery_failed': return '交付失败'
    default: return task.delivery_status || ''
  }
}

const quickStatusFilters: Array<{ label: string; status: '' | VideoTaskStatus }> = [
  { label: '全部', status: '' },
  { label: '已完成', status: 'succeeded' },
  { label: isVideoGatewayDemoMode ? '处理中' : '生成中', status: 'running' },
  { label: '失败', status: 'failed' },
]

async function loadTasks() {
  try {
    if (lifecycle.data.value === null) await lifecycle.start()
    else await lifecycle.refresh()
    if (lifecycle.error.value) {
      appStore.showError(lifecycle.error.value)
    }
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '加载任务记录失败' : '加载任务列表失败'))
  }
}

function resetAndLoad() {
  pagination.page = 1
  loadTasks()
}

function clearFilters() {
  filters.status = ''
  filters.provider = ''
  resetAndLoad()
}

function applyQuickStatus(status: '' | VideoTaskStatus) {
  filters.status = status
  resetAndLoad()
}

function changePage(page: number) {
  pagination.page = page
  loadTasks()
}

function copyToCreate(task: VideoTask) {
  saveTaskDraft(task)
  appStore.showInfo(isVideoGatewayDemoMode ? '已复制任务参数，可在试跑任务页调整后重新提交。' : '已复制任务参数，可在创建页调整后重新提交。')
  router.push('/admin/video/create')
}

function hasUsableRemoteAsset(task: VideoTask): boolean {
  return preferredVideoTaskAsset(task).kind === 'remote'
}

function hasDeliverableAsset(task: VideoTask): boolean {
  return preferredVideoTaskAsset(task).kind !== 'unavailable'
}

function resultExpiryLabel(task: VideoTask): string {
  if (!task.result_url || !task.result_url_expires_at) return ''
  const expires = new Date(task.result_url_expires_at)
  if (Number.isNaN(expires.getTime())) return ''
  const label = formatDate(task.result_url_expires_at)
  if (expires.getTime() <= Date.now()) return `可能已过期（${label}）`
  if (task.result_url_expiry_source === 'estimated') return `预计 ${label} 过期`
  return `${label} 过期`
}

function isResultExpiryNear(task: VideoTask): boolean {
  if (!task.result_url_expires_at) return false
  const expires = new Date(task.result_url_expires_at).getTime()
  if (Number.isNaN(expires)) return false
  const remaining = expires - Date.now()
  return remaining <= 2 * 60 * 60 * 1000
}

async function copyResultUrl(task: VideoTask) {
  if (!task.result_url) return
  try {
    await navigator.clipboard.writeText(task.result_url)
    appStore.showSuccess('结果链接已复制。')
  } catch {
    appStore.showError('复制失败，请手动选择链接。')
  }
}

async function openLocalAsset(task: VideoTask) {
  const asset = preferredVideoTaskAsset(task)
  if (asset.kind === 'unavailable') {
    appStore.showError(task.error_message || lifecycle.error.value || '当前任务没有可下载的本地资产或远端结果')
    return
  }
  if (asset.kind === 'remote') {
    window.open(asset.url, '_blank', 'noopener,noreferrer')
    return
  }
  try {
    const blob = await videoTaskAPI.getLocalAssetBlob(task.id)
    const url = URL.createObjectURL(blob)
    window.open(url, '_blank', 'noopener,noreferrer')
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
  } catch (err) {
    if (task.result_url) {
      window.open(task.result_url, '_blank', 'noopener,noreferrer')
      return
    }
    appStore.showError(extractApiErrorMessage(err, '打开本地归档失败'))
  }
}

onMounted(() => {
  void loadUsdCnyRate()
  void loadTasks()
})
</script>

<style scoped>
.prompt-summary {
  display: -webkit-box;
  overflow: hidden;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}
</style>
