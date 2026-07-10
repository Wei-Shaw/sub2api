<template>
  <AppLayout>
    <div class="space-y-6">
      <div class="flex flex-col gap-3 border-b border-gray-200 pb-4 dark:border-dark-700 md:flex-row md:items-center md:justify-between">
        <div>
          <div class="flex items-center gap-3">
            <RouterLink class="text-sm font-medium text-gray-500 hover:text-gray-900 dark:text-gray-400 dark:hover:text-white" to="/admin/video/tasks">
              {{ isVideoGatewayDemoMode ? '任务记录' : '任务列表' }}
            </RouterLink>
            <span class="text-gray-300 dark:text-dark-500">/</span>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '任务详情' : `#${task?.id || route.params.id}` }}</h1>
          </div>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ isVideoGatewayDemoMode ? '查看任务全过程，包括调度、状态变化和失败原因。' : '查看单个任务的参数、处理时间线、结果或失败原因。' }}
          </p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button v-if="task && !isTerminalStatus(task.status)" class="btn btn-outline" type="button" :disabled="cancelling" @click="cancelTask">
            <Icon name="ban" size="sm" />
            {{ isVideoGatewayDemoMode ? '取消任务' : '取消任务' }}
          </button>
          <button v-if="task" class="btn btn-outline" type="button" @click="copyToCreate">
            {{ isVideoGatewayDemoMode ? '复制参数重新提交' : '复制参数' }}
          </button>
          <button class="btn btn-outline" type="button" :disabled="loading" @click="loadTask">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
            刷新
          </button>
        </div>
      </div>

      <div v-if="lifecycleError" class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/20 dark:bg-red-500/10 dark:text-red-300">
        {{ lifecycleError }}
      </div>

      <section v-if="task" class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '任务结果概览' : '任务概览' }}</h2>
          <div class="flex flex-wrap gap-2">
            <a v-if="remoteAssetAvailable" class="btn btn-sm btn-outline" :href="task.result_url" target="_blank" rel="noreferrer">
              {{ isVideoGatewayDemoMode ? '打开结果链接' : '打开结果' }}
            </a>
            <button v-if="remoteAssetAvailable" class="btn btn-sm btn-outline" type="button" @click="copyResultUrl">
              复制结果链接
            </button>
            <button
              v-if="task.local_asset_available"
              class="btn btn-sm btn-outline"
              type="button"
              @click="openLocalAsset"
            >
              打开本地归档
            </button>
            <button class="btn btn-sm btn-outline" type="button" @click="copyToCreate">
              {{ isVideoGatewayDemoMode ? '复制参数重新提交' : '复制参数' }}
            </button>
          </div>
        </div>
        <p
          v-if="remoteAssetAvailable && resultExpiryHint"
          class="mt-2 text-xs"
          :class="resultExpiryNear ? 'text-yellow-600 dark:text-yellow-300' : 'text-gray-500 dark:text-gray-400'"
        >
          {{ resultExpiryHint }}
        </p>
        <div v-if="remoteAssetAvailable" class="mt-4 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-700">
          <video
            class="max-h-80 w-full bg-black object-contain"
            :src="task.result_url"
            controls
            preload="metadata"
            playsinline
          />
        </div>
        <p v-if="task.local_asset_available" class="mt-2 text-xs text-emerald-700 dark:text-emerald-300">
          本机已归档{{ task.local_asset_saved_at ? `（${task.local_asset_saved_at}）` : '' }}，上游链接过期后仍可打开本地文件。
        </p>
        <div class="mt-4 grid gap-4 lg:grid-cols-[0.7fr_1.2fr_1fr]">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ isVideoGatewayDemoMode ? '任务状态' : '状态' }}</div>
            <div class="mt-2">
              <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="statusBadgeClass(task.status)">
                {{ statusLabel(task.status) }}
              </span>
            </div>
          </div>
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">{{ isVideoGatewayDemoMode ? '任务结果 / 失败原因' : '结果 / 失败原因' }}</div>
            <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ outcomeTitle }}</div>
            <p class="mt-1 text-xs leading-5" :class="task.error_message ? 'text-red-600 dark:text-red-300' : 'text-gray-500 dark:text-gray-400'">{{ outcomeDescription }}</p>
          </div>
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">下一步动作</div>
            <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ nextActionTitle }}</div>
            <p class="mt-1 text-xs leading-5 text-gray-500 dark:text-gray-400">{{ nextActionDescription }}</p>
          </div>
        </div>
      </section>

      <section v-if="task" class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700">
          <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '系统处理信息' : '通道信息' }}</h2>
          <p v-if="isVideoGatewayDemoMode" class="mt-1 text-sm text-gray-500 dark:text-gray-400">确认任务已经进入本机试跑流程，并留下可追踪记录。</p>
        </div>
        <dl class="divide-y divide-gray-100 text-sm dark:divide-dark-700">
          <div v-for="row in channelRows" :key="row.label" class="grid gap-2 px-5 py-3 sm:grid-cols-[160px_1fr]">
            <dt class="text-gray-500 dark:text-gray-400">{{ row.label }}</dt>
            <dd class="min-w-0 break-words text-gray-900 dark:text-gray-100">{{ row.value || '-' }}</dd>
          </div>
        </dl>
      </section>

      <details v-if="routingTrace" class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
        <summary class="cursor-pointer border-b border-gray-200 px-5 py-4 text-base font-semibold text-gray-900 dark:border-dark-700 dark:text-white">
          {{ isVideoGatewayDemoMode ? '调度记录' : '路由记录' }}
        </summary>
        <div class="px-5 pt-4">
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">只展示账号选择、跳过原因和调度策略，不展示任何底层凭证字段。</p>
        </div>
        <div class="grid gap-4 p-5 lg:grid-cols-3">
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">调度策略</div>
            <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ routingStrategyLabel(routingTrace.strategy) }}</div>
          </div>
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">选中账号</div>
            <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ routingTrace.selected_account_name || `账号 #${routingTrace.selected_account_id}` }}</div>
            <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ providerLabel(routingTrace.provider) }}</div>
          </div>
          <div class="rounded-lg border border-gray-200 p-4 dark:border-dark-700">
            <div class="text-xs font-medium uppercase text-gray-500 dark:text-gray-400">调度原因</div>
            <div class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">{{ routingTrace.reason || '-' }}</div>
          </div>
        </div>
        <div class="border-t border-gray-200 px-5 py-4 dark:border-dark-700">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-white">未被选中的账号</h3>
          <div v-if="routingTrace.skippedAccounts.length" class="mt-3 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div v-for="account in routingTrace.skippedAccounts" :key="`${account.provider}-${account.id}`" class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-700 dark:bg-dark-700/30">
              <div class="text-sm font-medium text-gray-900 dark:text-white">{{ account.display_name || `账号 #${account.id}` }}</div>
              <div class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ providerLabel(account.provider) }}</div>
              <div class="mt-2 text-xs font-medium text-amber-700 dark:text-amber-300">{{ account.reason || '-' }}</div>
            </div>
          </div>
          <div v-else class="mt-3 rounded-lg border border-dashed border-gray-200 p-3 text-sm text-gray-500 dark:border-dark-700 dark:text-gray-400">没有跳过账号。</div>
        </div>
      </details>

      <div v-if="task" class="grid gap-6 xl:grid-cols-[0.9fr_1.1fr]">
        <section class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <div class="border-b border-gray-200 px-5 py-4 dark:border-dark-700">
            <h2 class="text-base font-semibold text-gray-900 dark:text-white">{{ isVideoGatewayDemoMode ? '完整提示词与任务参数' : '任务参数' }}</h2>
          </div>
          <dl class="divide-y divide-gray-100 text-sm dark:divide-dark-700">
            <div v-for="row in parameterRows" :key="row.label" class="grid gap-2 px-5 py-3 sm:grid-cols-[160px_1fr]">
              <dt class="text-gray-500 dark:text-gray-400">{{ row.label }}</dt>
              <dd class="min-w-0 break-words text-gray-900 dark:text-gray-100">{{ row.value || '-' }}</dd>
            </div>
          </dl>
        </section>

        <details class="rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800">
          <summary class="cursor-pointer border-b border-gray-200 px-5 py-4 text-base font-semibold text-gray-900 dark:border-dark-700 dark:text-white">
            {{ isVideoGatewayDemoMode ? '运行记录' : '处理时间线' }}
          </summary>
          <div class="space-y-4 p-5">
            <div v-for="event in task.events || []" :key="event.id" class="relative pl-6">
              <span class="absolute left-0 top-1.5 h-2.5 w-2.5 rounded-full" :class="eventDotClass(event.event_type)"></span>
              <div class="flex flex-col gap-1 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <div class="font-medium text-gray-900 dark:text-white">{{ eventMessageLabel(event.message, event.event_type) }}</div>
                  <div class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">{{ eventTypeLabel(event.event_type) }}</div>
                </div>
                <div class="text-xs text-gray-500 dark:text-gray-400">{{ formatDate(event.created_at) }}</div>
              </div>
              <details v-if="!isVideoGatewayDemoMode && event.payload_json && Object.keys(event.payload_json).length" class="mt-2">
                <summary class="cursor-pointer text-xs font-medium text-gray-500 dark:text-gray-400">技术数据（payload）</summary>
                <pre class="mt-2 max-h-64 overflow-auto rounded-md bg-gray-50 p-3 text-xs text-gray-700 dark:bg-dark-900 dark:text-gray-200">{{ JSON.stringify(event.payload_json, null, 2) }}</pre>
              </details>
            </div>
            <div v-if="!(task.events || []).length" class="py-8 text-center text-sm text-gray-500 dark:text-gray-400">{{ isVideoGatewayDemoMode ? '暂无运行记录' : '暂无时间线事件' }}</div>
          </div>
        </details>
      </div>

      <details v-if="task" class="rounded-lg border border-gray-200 bg-white p-5 dark:border-dark-700 dark:bg-dark-800">
        <summary class="cursor-pointer text-base font-semibold text-gray-900 dark:text-white">内部技术详情</summary>
        <div v-if="isVideoGatewayDemoMode" class="mt-4 rounded-md bg-gray-50 p-4 text-sm text-gray-600 dark:bg-dark-900 dark:text-gray-300">
          技术详情默认折叠。客户演示模式仅展示任务状态、通道、参数、运行记录、结果和失败原因；底层字段保留给内部排障环境。
        </div>
        <pre v-else class="mt-4 max-h-96 overflow-auto rounded-md bg-gray-50 p-4 text-xs text-gray-700 dark:bg-dark-900 dark:text-gray-200">{{ JSON.stringify(task, null, 2) }}</pre>
      </details>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { videoTaskAPI, type VideoTask } from '@/api/admin/video'
import { preferredVideoTaskAsset, useVideoTaskLifecycle } from '@/composables/useVideoTaskLifecycle'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import { isVideoGatewayDemoMode } from '@/utils/productMode'
import {
  errorMessageLabel,
  eventMessageLabel,
  eventTypeLabel,
  extractRoutingTrace,
  formatDate,
  isTerminalStatus,
  modelDisplayName,
  createdByLabel,
  providerLabel,
  routeAccountLabel,
  routingStrategyLabel,
  saveTaskDraft,
  statusBadgeClass,
  statusLabel,
  taskTypeLabel,
} from './videoUtils'

const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const cancelling = ref(false)

const taskId = computed(() => Number(route.params.id))
const lifecycle = useVideoTaskLifecycle<VideoTask>({
  fetch: (signal) => videoTaskAPI.get(taskId.value, signal),
  shouldContinue: (value) => Boolean(value && (!isTerminalStatus(value.status) || value.delivery_status === 'archiving')),
})
const task = lifecycle.task
const loading = lifecycle.loading
const lifecycleError = lifecycle.error
const remoteAssetAvailable = computed(() => preferredVideoTaskAsset(task.value).kind === 'remote')
const routingTrace = computed(() => task.value ? extractRoutingTrace(task.value) : null)
const resultExpiryNear = computed(() => {
  const raw = task.value?.result_url_expires_at
  if (!raw) return false
  const expires = new Date(raw).getTime()
  if (Number.isNaN(expires)) return false
  const remaining = expires - Date.now()
  return remaining > 0 && remaining <= 2 * 60 * 60 * 1000
})
const resultExpiryHint = computed(() => {
  if (!task.value?.result_url) return ''
  const raw = task.value.result_url_expires_at
  if (!raw) return '结果链接过期时间未知，请尽快下载或复制。'
  const expires = new Date(raw)
  if (Number.isNaN(expires.getTime())) return '结果链接过期时间未知，请尽快下载或复制。'
  const label = formatDate(raw)
  const source = task.value.result_url_expiry_source
  if (source === 'estimated') {
    return `结果链接预计约在 ${label} 过期（上游未提供签名过期参数，按完成时间 +24h 估算）。请尽快下载或复制。`
  }
  if (expires.getTime() <= Date.now()) {
    return `结果链接可能已于 ${label} 过期。`
  }
  return `结果链接将于 ${label} 过期，请尽快下载或复制。`
})
const outcomeTitle = computed(() => {
  if (!task.value) return '-'
  if (task.value.error_message) return isVideoGatewayDemoMode ? '任务失败，原因已记录' : '任务失败'
  if (task.value.result_url) return isVideoGatewayDemoMode ? '任务成功，结果已回收' : '生成成功'
  if (!isTerminalStatus(task.value.status)) return isVideoGatewayDemoMode ? '生成中，等待结果回收' : '处理中'
  return isVideoGatewayDemoMode ? '暂无结果' : '暂无结果'
})
const outcomeDescription = computed(() => {
  if (!task.value) return '-'
  if (task.value.error_message) return errorMessageLabel(task.value.error_message)
  if (task.value.result_url) return isVideoGatewayDemoMode ? '结果链接已经进入任务详情，管理员可以审计完整链路。' : '结果链接已回收。'
  if (!isTerminalStatus(task.value.status)) return isVideoGatewayDemoMode ? '系统仍在等待上游处理或状态回收。' : '任务仍在处理。'
  return isVideoGatewayDemoMode ? '没有返回结果链接，请查看时间线确认原因。' : '没有返回结果链接。'
})
const nextActionTitle = computed(() => {
  if (!task.value) return '-'
  if (task.value.next_action === 'poll') return '等待状态回传'
  if (task.value.next_action === 'archive') return '生成完成，归档中'
  if (task.value.next_action === 'download') return task.value.local_asset_available ? '本地资产可下载' : '结果可下载'
  if (task.value.next_action === 'reconcile_dispatch') return '已提交，需确认上游任务'
  if (task.value.next_action === 'review_settlement') return '账务待处理'
  if (task.value.next_action === 'review_delivery') return '交付失败，请查看原因'
  if (task.value.error_message) return '复制参数重新提交'
  if (task.value.result_url) return '打开或复制结果'
  if (!isTerminalStatus(task.value.status)) return '等待或刷新状态'
  return '查看时间线'
})
const nextActionDescription = computed(() => {
  if (!task.value) return '-'
  if (task.value.settlement_status === 'error') return '账务结算等待后台重试，不会改变已有生成结果。'
  if (task.value.dispatch_state === 'unknown') return '上游提交结果无法确认，系统不会自动再次创建任务。'
  if (task.value.delivery_status === 'archiving') return '生成结果已完成，正在写入本地资产。'
  if (task.value.delivery_status === 'deliverable') return '优先使用本地资产；尚未归档时使用上游链接。'
  if (task.value.delivery_status === 'delivery_failed') return lifecycleError.value || errorMessageLabel(task.value.error_message || '归档失败，暂无可交付资产。')
  if (task.value.error_message) return '先看失败原因，再调整提示词或切换生成通道重新提交。'
  if (task.value.result_url) return '结果已经回收，可打开链接或复制给业务方。'
  if (!isTerminalStatus(task.value.status)) return '系统会继续轮询并回收结果或失败原因。'
  return '从运行记录确认最后一次状态变化。'
})
const channelRows = computed(() => {
  if (!task.value) return []
  if (isVideoGatewayDemoMode) {
    return [
      { label: '任务编号', value: `#${task.value.id}` },
      { label: '发起人', value: createdByLabel(task.value) },
      { label: '任务来源', value: providerLabel(task.value.provider) },
      { label: '处理账号', value: routeAccountLabel(task.value) },
      { label: '处理方式', value: routingStrategyLabel(task.value.routing_strategy) },
      { label: '处理说明', value: task.value.routing_reason ? '已选择当前可用的本机试跑账号' : '-' },
      { label: '模型', value: modelDisplayName(task.value.provider, task.value.model) },
      { label: '当前状态', value: statusLabel(task.value.status) },
      { label: '创建时间', value: formatDate(task.value.created_at) },
      { label: '完成时间', value: task.value.completed_at ? formatDate(task.value.completed_at) : '-' },
    ]
  }
  return [
    { label: '任务编号', value: `#${task.value.id}` },
    { label: '发起人', value: createdByLabel(task.value) },
    { label: '通道', value: providerLabel(task.value.provider) },
    { label: '实际路由账号', value: routeAccountLabel(task.value) },
    { label: '模型', value: modelDisplayName(task.value.provider, task.value.model) },
    { label: '状态', value: statusLabel(task.value.status) },
  ]
})

const parameterRows = computed(() => {
  if (!task.value) return []
  return [
    { label: isVideoGatewayDemoMode ? '任务类型' : '任务类型', value: taskTypeLabel(task.value.task_type) },
    { label: '完整提示词', value: task.value.prompt },
    { label: '负向提示词', value: task.value.negative_prompt },
    { label: '参考图', value: task.value.reference_image_url },
    { label: '参考视频', value: task.value.reference_video_url },
    { label: '画幅比例', value: task.value.aspect_ratio },
    { label: '时长', value: `${task.value.duration}s` },
    { label: '分辨率', value: task.value.resolution },
  ]
})

function eventDotClass(eventType: string): string {
  if (eventType === 'succeeded') return 'bg-emerald-500'
  if (eventType === 'failed') return 'bg-red-500'
  if (eventType === 'routed') return 'bg-sky-500'
  if (eventType === 'running') return 'bg-amber-500'
  return 'bg-gray-400'
}

async function loadTask() {
  try {
    await lifecycle.refresh()
    if (lifecycle.error.value) {
      appStore.showError(lifecycle.error.value)
    }
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '加载任务详情失败' : '加载任务详情失败'))
  }
}

async function cancelTask() {
  if (!task.value) return
  cancelling.value = true
  try {
    await videoTaskAPI.cancel(task.value.id)
    appStore.showSuccess(isVideoGatewayDemoMode ? '任务已取消' : '视频任务已取消')
    await loadTask()
  } catch (err) {
    appStore.showError(extractApiErrorMessage(err, isVideoGatewayDemoMode ? '取消任务失败' : '取消视频任务失败'))
  } finally {
    cancelling.value = false
  }
}

async function copyText(value: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(value)
    appStore.showSuccess(successMessage)
  } catch {
    appStore.showError('复制失败，请手动选择内容复制。')
  }
}

function copyResultUrl() {
  if (!task.value?.result_url) return
  copyText(task.value.result_url, '结果链接已复制。')
}

async function openLocalAsset() {
  const asset = preferredVideoTaskAsset(task.value)
  if (asset.kind === 'unavailable') {
    appStore.showError(task.value?.error_message || lifecycleError.value || '当前任务没有可下载的本地资产或远端结果')
    return
  }
  if (asset.kind === 'remote') {
    window.open(asset.url, '_blank', 'noopener,noreferrer')
    return
  }
  const taskForAsset = task.value
  if (!taskForAsset) return
  try {
    const blob = await videoTaskAPI.getLocalAssetBlob(taskForAsset.id)
    const url = URL.createObjectURL(blob)
    window.open(url, '_blank', 'noopener,noreferrer')
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000)
  } catch (err) {
    if (task.value?.result_url) {
      window.open(task.value.result_url, '_blank', 'noopener,noreferrer')
      return
    }
    appStore.showError(extractApiErrorMessage(err, '打开本地归档失败'))
  }
}

function copyToCreate() {
  if (!task.value) return
  saveTaskDraft(task.value)
  appStore.showInfo(isVideoGatewayDemoMode ? '已复制任务参数，可在试跑任务页调整后重新提交。' : '已复制任务参数，可在创建页调整后重新提交。')
  router.push('/admin/video/create')
}

</script>
