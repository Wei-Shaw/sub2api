<template>
  <BaseDialog :show="show" title="视频任务详情" width="wide" @close="emit('update:show', false)">
    <!-- Loading -->
    <div v-if="loading" class="flex justify-center py-10">
      <svg class="h-7 w-7 animate-spin text-primary-500" fill="none" viewBox="0 0 24 24">
        <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4" />
        <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
      </svg>
    </div>

    <!-- Error state -->
    <div v-else-if="loadError" class="py-8 text-center text-sm text-red-500">
      加载失败，请稍后重试
    </div>

    <!-- Detail content -->
    <div v-else-if="detail" class="space-y-4 text-sm">
      <div class="grid grid-cols-2 gap-x-6 gap-y-3">
        <!-- Task ID (async_video_tasks.id) -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">任务 ID</span>
          <div class="mt-0.5 flex items-center gap-2">
            <span class="font-mono text-gray-900 dark:text-dark-100">{{ detail.id }}</span>
            <button
              type="button"
              class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
              title="复制"
              @click="handleCopy(String(detail.id))"
            >
              <Icon name="copy" class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        <!-- Upstream Request ID (task_id in provider) -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">上游 task_id</span>
          <div class="mt-0.5 flex items-center gap-2">
            <span class="max-w-[260px] truncate font-mono text-gray-900 dark:text-dark-100" :title="detail.upstream_request_id || '-'">
              {{ detail.upstream_request_id || '-' }}
            </span>
            <button
              v-if="detail.upstream_request_id"
              type="button"
              class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
              title="复制"
              @click="handleCopy(detail.upstream_request_id)"
            >
              <Icon name="copy" class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        <!-- Internal Request ID -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">内部 request_id</span>
          <div class="mt-0.5 flex items-center gap-2">
            <span class="max-w-[260px] truncate font-mono text-xs text-gray-900 dark:text-dark-100" :title="detail.internal_request_id || '-'">
              {{ detail.internal_request_id || '-' }}
            </span>
            <button
              v-if="detail.internal_request_id"
              type="button"
              class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
              title="复制"
              @click="handleCopy(detail.internal_request_id)"
            >
              <Icon name="copy" class="h-3.5 w-3.5" />
            </button>
          </div>
        </div>
        <!-- Status -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">状态</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ detail.status || '-' }}</p>
        </div>
        <!-- Model -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">模型</span>
          <p class="mt-0.5 break-all text-gray-900 dark:text-dark-100">{{ detail.requested_model || '-' }}</p>
        </div>
        <!-- Resolution / Duration -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">分辨率 × 时长</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">
            {{ detail.resolution || '-' }}
            <span class="mx-1 text-gray-400">×</span>
            {{ detail.duration_seconds > 0 ? `${detail.duration_seconds}s` : '-' }}
          </p>
        </div>
        <!-- Aspect ratio -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">宽高比</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ detail.aspect_ratio || '-' }}</p>
        </div>
        <!-- Final Cost -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">实扣费用</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">${{ (detail.final_cost || 0).toFixed(6) }}</p>
        </div>
        <!-- Created / Finished -->
        <div>
          <span class="font-medium text-gray-500 dark:text-dark-400">创建时间</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ formatDateTime(detail.created_at) }}</p>
        </div>
        <div v-if="detail.finished_at">
          <span class="font-medium text-gray-500 dark:text-dark-400">完成时间</span>
          <p class="mt-0.5 text-gray-900 dark:text-dark-100">{{ formatDateTime(detail.finished_at) }}</p>
        </div>
      </div>

      <!-- Error reason -->
      <div v-if="detail.error_reason">
        <span class="font-medium text-gray-500 dark:text-dark-400">错误原因</span>
        <pre class="mt-1 max-h-[30vh] overflow-auto whitespace-pre-wrap break-all rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300">{{ detail.error_reason }}</pre>
      </div>

      <!-- Video URLs -->
      <div v-if="videoURLs.length">
        <span class="font-medium text-gray-500 dark:text-dark-400">视频链接</span>
        <ul class="mt-1 space-y-1">
          <li v-for="(url, idx) in videoURLs" :key="idx" class="flex items-center gap-2">
            <a :href="url" target="_blank" rel="noopener noreferrer" class="max-w-[500px] truncate text-primary-600 hover:underline dark:text-primary-400" :title="url">{{ url }}</a>
            <button
              type="button"
              class="rounded p-1 text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-gray-700 dark:hover:text-gray-300"
              title="复制"
              @click="handleCopy(url)"
            >
              <Icon name="copy" class="h-3.5 w-3.5" />
            </button>
          </li>
        </ul>
      </div>

      <!-- Request payload -->
      <div v-if="detail.request_payload && Object.keys(detail.request_payload).length">
        <span class="font-medium text-gray-500 dark:text-dark-400">请求参数</span>
        <pre class="mt-1 max-h-[35vh] overflow-auto whitespace-pre-wrap break-all rounded-lg border border-gray-200 bg-gray-50 p-3 text-xs text-gray-800 dark:border-dark-700 dark:bg-dark-900 dark:text-dark-200">{{ pretty(detail.request_payload) }}</pre>
      </div>
    </div>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import videoModelsAPI, { type VideoTaskItem } from '@/api/videoModels'
import { organizationAPI } from '@/api'
import { formatDateTime } from '@/utils/format'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{
  show: boolean
  taskId: number | null
	organizationUsageId?: number | null
  /**
   * admin 模式：true 时调用 /admin/video-tasks/by-id/:id 接口（不校验归属）；
   * 未传或 false 时调用 /user/video-models/tasks/by-id/:id（强制归属校验）。
   * 用于管理员使用记录页复用同一组件查看任意用户的任务详情。
   */
  admin?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:show', v: boolean): void
}>()

const { copyToClipboard } = useClipboard()

const loading = ref(false)
const loadError = ref(false)
const detail = ref<VideoTaskItem | null>(null)

// 优先展示 cos_urls（若已开启 COS 转存），退回 video_urls。
const videoURLs = computed<string[]>(() => {
  const d = detail.value
  if (!d) return []
  if (d.cos_urls && d.cos_urls.length) return d.cos_urls
  return d.video_urls || []
})

watch(
  () => [props.show, props.taskId, props.organizationUsageId] as const,
  ([show, id, usageId]) => {
    if (show && id != null && id > 0) {
			void fetchDetail(id, usageId)
    } else if (!show) {
      detail.value = null
      loadError.value = false
    }
  }
)

async function fetchDetail(id: number, organizationUsageId?: number | null) {
  loading.value = true
  loadError.value = false
  detail.value = null
  try {
    if (organizationUsageId != null && organizationUsageId > 0) {
			detail.value = await organizationAPI.getUsageVideoTask(organizationUsageId)
		} else {
			const resp = props.admin
				? await videoModelsAPI.getTaskByIdAdmin(id)
				: await videoModelsAPI.getTaskById(id)
			detail.value = resp.data
		}
  } catch (e) {
    console.error('[VideoTaskDetailModal] Failed to load task detail:', e)
    loadError.value = true
  } finally {
    loading.value = false
  }
}

function handleCopy(text: string) {
  if (!text) return
  void copyToClipboard(text, '已复制')
}

function pretty(v: unknown): string {
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}
</script>
