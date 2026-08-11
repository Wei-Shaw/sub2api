<!--
  VideoPlaygroundHistory：演练台"历史记录"Tab 的内容组件。

  职责：
    - 拉取 GET /user/video-models/tasks?slug=<slug> 的分页列表；
    - 列表以"整行卡片"形式展示；整行点击弹出"详情弹窗"（耗时/输入/输出/花费）；
    - 视频缩略图单独点击（stopPropagation）弹出"播放弹窗"直接内嵌播放；
    - 详情弹窗底部保留"重放"次要动作，通过 emit('replay') 回传给父组件。

  为什么不再显示行内"打开外链"和"重放"按钮：
    - 行内多按钮会让"整行可点击"这个交互目标变得歧义；
    - 视频播放改成弹窗内嵌，比新开外链更符合演练台内的沉浸感；
    - 重放频率相对低，放进详情弹窗即可，同时避免误点。
-->
<template>
  <div class="space-y-4">
    <!-- 操作栏：刷新按钮 + 说明 -->
    <div class="flex items-center justify-between gap-3">
      <div class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('videoModels.playground.historyHint') }}
      </div>
      <button type="button" class="btn btn-secondary btn-xs" :disabled="loading" @click="reload">
        {{ loading ? t('common.loading') : t('common.refresh') }}
      </button>
    </div>

    <!-- 加载中 -->
    <div v-if="loading && items.length === 0" class="rounded border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-gray-700">
      {{ t('common.loading') }}
    </div>

    <!-- 空态 -->
    <div v-else-if="!items.length" class="rounded border border-dashed border-gray-300 p-8 text-center text-sm text-gray-500 dark:border-gray-700 dark:bg-gray-950/40">
      {{ t('videoModels.playground.historyEmpty') }}
    </div>

    <!-- 列表：整行可点击 → 打开详情弹窗；视频缩略图单独可点击 → 打开视频弹窗 -->
    <ul v-else class="divide-y divide-gray-200 rounded-lg border border-gray-200 dark:divide-gray-800 dark:border-gray-800">
      <li
        v-for="item in items"
        :key="item.id"
        role="button"
        tabindex="0"
        class="flex cursor-pointer gap-3 p-3 transition hover:bg-gray-50 focus:bg-gray-50 focus:outline-none dark:hover:bg-gray-900 dark:focus:bg-gray-900"
        @click="openDetail(item)"
        @keydown.enter.prevent="openDetail(item)"
        @keydown.space.prevent="openDetail(item)"
      >
        <!--
          缩略图：视频存在时可点击独立弹出"播放弹窗"。
          使用 @click.stop 阻止事件冒泡到外层 <li>，避免同时打开详情弹窗。
          无视频时给灰底占位；整行点击行为不受影响。
        -->
        <div
          class="group relative h-16 w-24 shrink-0 overflow-hidden rounded bg-black"
          :class="firstUrl(item) ? 'cursor-zoom-in' : ''"
          @click.stop="firstUrl(item) ? openVideo(item) : null"
        >
          <video
            v-if="firstUrl(item)"
            :src="firstUrl(item)"
            muted
            preload="metadata"
            class="h-full w-full object-cover"
          />
          <div v-else class="flex h-full w-full items-center justify-center text-[10px] text-gray-500">
            {{ t('videoModels.playground.historyNoVideo') }}
          </div>
          <!-- 播放三角图标叠加（仅在有视频且 hover 时高亮） -->
          <div
            v-if="firstUrl(item)"
            class="pointer-events-none absolute inset-0 flex items-center justify-center bg-black/20 opacity-0 transition group-hover:opacity-100"
          >
            <svg class="h-6 w-6 text-white drop-shadow" viewBox="0 0 24 24" fill="currentColor">
              <path d="M8 5v14l11-7z" />
            </svg>
          </div>
        </div>

        <!-- 中间：元信息 -->
        <div class="min-w-0 flex-1 space-y-1">
          <div class="flex items-center gap-2">
            <span :class="statusBadgeClass(item.status)" class="rounded px-1.5 py-0.5 text-[10px] font-medium">
              {{ t(`videoModels.playground.historyStatus.${normalizedStatus(item.status)}`) }}
            </span>
            <span class="truncate text-xs text-gray-500 dark:text-gray-400">
              {{ formatTime(item.created_at) }}
            </span>
          </div>
          <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-600 dark:text-gray-300">
            <span v-if="item.resolution">
              {{ item.resolution }}
            </span>
            <span v-if="item.duration_seconds > 0">
              {{ item.duration_seconds }}s
            </span>
            <span v-if="item.aspect_ratio">
              {{ item.aspect_ratio }}
            </span>
            <span class="font-mono text-[11px] text-emerald-600 dark:text-emerald-400">
              ${{ (item.final_cost || item.held_cost || 0).toFixed(4) }}
            </span>
          </div>
          <div v-if="item.error_reason" class="truncate text-xs text-red-600 dark:text-red-400" :title="item.error_reason">
            {{ item.error_reason }}
          </div>
        </div>

        <!--
          右侧：操作栏 = 「查看详情 >」+ 竖向分隔线 + 「重放」按钮。
          - 「查看详情 >」引导：不承担点击，纯视觉提示整行可点（整行 <li> 已绑 openDetail）；放在左边紧贴中间元信息一侧。
          - 竖向分隔线：用 border-l 画一条 1px 淡灰线，模拟"表格竖线"，视觉上把"整行点击的引导"与"独立按钮"区隔开。
          - 「重放」按钮：单独可点击，@click.stop 阻止冒泡到整行的 openDetail，直接把 request_payload 回抛给父组件；
            request_payload 为空时禁用（沿用 canReplay 语义）。
        -->
        <div class="flex shrink-0 items-center gap-3">
          <div class="flex items-center text-xs text-gray-400 dark:text-gray-500">
            {{ t('videoModels.playground.historyOpenDetail') }}
            <svg class="ml-1 h-3 w-3" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
              <path fill-rule="evenodd" d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z" clip-rule="evenodd" />
            </svg>
          </div>
          <div class="h-6 border-l border-gray-200 dark:border-gray-700" aria-hidden="true" />
          <button
            type="button"
            class="btn btn-secondary btn-xs"
            :disabled="!canReplay(item)"
            @click.stop="onReplay(item)"
          >
            {{ t('videoModels.playground.historyReplay') }}
          </button>
        </div>
      </li>
    </ul>

    <!-- 分页 -->
    <div v-if="total > pageSize" class="flex items-center justify-between text-xs">
      <span class="text-gray-500 dark:text-gray-400">
        {{ t('videoModels.playground.historyPageInfo', { page, total, pageSize }) }}
      </span>
      <div class="flex items-center gap-2">
        <button type="button" class="btn btn-secondary btn-xs" :disabled="page <= 1 || loading" @click="prevPage">
          {{ t('videoModels.playground.historyPrev') }}
        </button>
        <button type="button" class="btn btn-secondary btn-xs" :disabled="page * pageSize >= total || loading" @click="nextPage">
          {{ t('videoModels.playground.historyNext') }}
        </button>
      </div>
    </div>

    <p v-if="errorMsg" class="text-xs text-red-600 dark:text-red-400">
      {{ errorMsg }}
    </p>

    <!--
      详情弹窗：展示单条任务的完整元信息 + 输入 payload + 输出 payload。
      结构分四块：
        1) 基础信息（状态/时间/耗时/模型 slug）
        2) 参数信息（resolution/duration/aspect）
        3) 花费信息（预扣 held_cost / 实扣 final_cost）
        4) 输入 request_payload / 输出 result_payload（<pre> JSON 展示）
      底部次要按钮：重放（把 request_payload 回抛给父组件）。
    -->
    <BaseDialog
      :show="!!detailTask"
      :title="detailTask ? t('videoModels.playground.historyDetailTitle') : ''"
      width="wide"
      @close="closeDetail"
    >
      <div v-if="detailTask" class="space-y-4 text-sm">
        <!-- 状态 + 时间 + 耗时 -->
        <section class="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.status') }}
            </div>
            <span :class="statusBadgeClass(detailTask.status)" class="mt-1 inline-block rounded px-1.5 py-0.5 text-[11px] font-medium">
              {{ t(`videoModels.playground.historyStatus.${normalizedStatus(detailTask.status)}`) }}
            </span>
          </div>
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.createdAt') }}
            </div>
            <div class="mt-1 font-mono text-xs">{{ formatTime(detailTask.created_at) }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.elapsed') }}
            </div>
            <div class="mt-1 font-mono text-xs">{{ formatElapsed(detailTask) }}</div>
          </div>
          <div class="col-span-2 sm:col-span-3">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.model') }}
            </div>
            <div class="mt-1 truncate font-mono text-xs" :title="detailTask.requested_model">
              {{ detailTask.requested_model }}
            </div>
          </div>
        </section>

        <!-- 参数信息 -->
        <section class="grid grid-cols-3 gap-3">
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.resolution') }}
            </div>
            <div class="mt-1 font-mono text-xs">{{ detailTask.resolution || '-' }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.duration') }}
            </div>
            <div class="mt-1 font-mono text-xs">{{ detailTask.duration_seconds > 0 ? `${detailTask.duration_seconds}s` : '-' }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.aspect') }}
            </div>
            <div class="mt-1 font-mono text-xs">{{ detailTask.aspect_ratio || '-' }}</div>
          </div>
        </section>

        <!-- 花费信息：预扣 vs 实扣 -->
        <section class="grid grid-cols-2 gap-3">
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.heldCost') }}
            </div>
            <div class="mt-1 font-mono text-xs">${{ (detailTask.held_cost || 0).toFixed(4) }}</div>
          </div>
          <div>
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.finalCost') }}
            </div>
            <div class="mt-1 font-mono text-xs text-emerald-600 dark:text-emerald-400">
              ${{ (detailTask.final_cost || 0).toFixed(4) }}
            </div>
          </div>
        </section>

        <!-- 错误原因（仅失败时展示） -->
        <section v-if="detailTask.error_reason">
          <div class="text-xs text-gray-500 dark:text-gray-400">
            {{ t('videoModels.playground.historyDetail.error') }}
          </div>
          <div class="mt-1 whitespace-pre-wrap rounded border border-red-200 bg-red-50 p-2 text-xs text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-300">
            {{ detailTask.error_reason }}
          </div>
        </section>

        <!-- 输入 payload -->
        <section>
          <div class="mb-1 flex items-center justify-between">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.input') }}
            </div>
            <button
              type="button"
              class="text-[11px] text-blue-600 hover:underline dark:text-blue-400"
              @click="copyJson(detailTask.request_payload)"
            >
              {{ t('common.copy') }}
            </button>
          </div>
          <pre class="max-h-64 overflow-auto rounded border border-gray-200 bg-gray-50 p-2 text-[11px] leading-relaxed text-gray-800 dark:border-gray-800 dark:bg-gray-950 dark:text-gray-200">{{ stringifyJson(detailTask.request_payload) }}</pre>
        </section>

        <!-- 输出 payload -->
        <section>
          <div class="mb-1 flex items-center justify-between">
            <div class="text-xs text-gray-500 dark:text-gray-400">
              {{ t('videoModels.playground.historyDetail.output') }}
            </div>
            <button
              type="button"
              class="text-[11px] text-blue-600 hover:underline dark:text-blue-400"
              @click="copyJson(detailTask.result_payload)"
            >
              {{ t('common.copy') }}
            </button>
          </div>
          <pre class="max-h-64 overflow-auto rounded border border-gray-200 bg-gray-50 p-2 text-[11px] leading-relaxed text-gray-800 dark:border-gray-800 dark:bg-gray-950 dark:text-gray-200">{{ stringifyJson(detailTask.result_payload) }}</pre>
        </section>
      </div>

      <!-- 弹窗底部操作：重放 + 关闭 -->
      <template #footer>
        <div class="flex items-center justify-end gap-2">
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            :disabled="!canReplay(detailTask)"
            @click="onReplay(detailTask)"
          >
            {{ t('videoModels.playground.historyReplay') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" @click="closeDetail">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>

    <!--
      视频播放弹窗：只承担"播放当前视频"这一件事，故 UI 极简。
      使用 <video controls autoplay> 让用户能拖拽/暂停/全屏。
    -->
    <BaseDialog
      :show="!!videoUrl"
      :title="t('videoModels.playground.historyVideoTitle')"
      width="wide"
      @close="closeVideo"
    >
      <div v-if="videoUrl" class="flex justify-center bg-black">
        <video
          :src="videoUrl"
          controls
          autoplay
          class="block h-auto max-h-[70vh] max-w-full"
        />
      </div>
      <template #footer>
        <div class="flex items-center justify-between gap-2">
          <a
            v-if="videoUrl"
            :href="videoUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ t('videoModels.playground.historyOpenInNewTab') }}
          </a>
          <button type="button" class="btn btn-primary btn-sm" @click="closeVideo">
            {{ t('common.close') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import videoModelsAPI, { type VideoTaskItem } from '@/api/videoModels'
import BaseDialog from '@/components/common/BaseDialog.vue'
import { useAppStore } from '@/stores/app'

const props = defineProps<{ slug: string }>()
const emit = defineEmits<{
  (e: 'replay', payload: Record<string, unknown> | null): void
  (e: 'loaded', total: number): void
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const errorMsg = ref('')
const items = ref<VideoTaskItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)

// 详情弹窗当前展示的任务；null 表示关闭
const detailTask = ref<VideoTaskItem | null>(null)
// 视频弹窗当前播放的 URL；空字符串表示关闭
const videoUrl = ref<string>('')

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))
// 保留占位，未来加"跳页"。读 .value 而不是 void totalPages ——
// 后者把 ref 本身当操作数，会触发 vue/no-ref-as-operand。
void totalPages.value

/**
 * load：拉取指定 page 的数据。slug 为空时不发请求（后端会 400，前端直接空态）。
 */
async function load() {
  if (!props.slug) {
    items.value = []
    total.value = 0
    return
  }
  loading.value = true
  errorMsg.value = ''
  try {
    const resp = await videoModelsAPI.listTasks(props.slug, page.value, pageSize.value)
    const data = resp.data
    items.value = data.items || []
    total.value = data.total || 0
    emit('loaded', total.value)
  } catch (err) {
    const anyErr = err as { message?: string }
    errorMsg.value = anyErr?.message || 'Failed to load history.'
    items.value = []
  } finally {
    loading.value = false
  }
}

function reload() {
  page.value = 1
  void load()
}
function prevPage() {
  if (page.value <= 1) return
  page.value -= 1
  void load()
}
function nextPage() {
  if (page.value * pageSize.value >= total.value) return
  page.value += 1
  void load()
}

/**
 * openDetail / closeDetail：详情弹窗开关。
 * 详情弹窗数据完全来自当前列表项，不额外拉接口（后端 list 已经把全部字段吐回来）。
 */
function openDetail(item: VideoTaskItem) {
  detailTask.value = item
}
function closeDetail() {
  detailTask.value = null
}

/**
 * openVideo / closeVideo：视频播放弹窗开关。
 * 仅使用 cos_urls[0] || video_urls[0]；两者都为空时函数不会被触发（模板上有守卫）。
 */
function openVideo(item: VideoTaskItem) {
  const url = firstUrl(item)
  if (url) videoUrl.value = url
}
function closeVideo() {
  videoUrl.value = ''
}

/**
 * canReplay：只有 request_payload 非空对象时才允许重放，否则重放没有意义。
 */
function canReplay(item: VideoTaskItem | null): boolean {
  if (!item) return false
  const p = item.request_payload
  return !!p && Object.keys(p).length > 0
}

function onReplay(item: VideoTaskItem | null) {
  if (!item) return
  emit('replay', item.request_payload || null)
  closeDetail()
}

function firstUrl(item: VideoTaskItem): string {
  const cos = item.cos_urls || []
  if (cos.length > 0 && cos[0]) return cos[0]
  const urls = item.video_urls || []
  return urls[0] || ''
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    if (Number.isNaN(d.getTime())) return iso
    return d.toLocaleString()
  } catch {
    return iso
  }
}

/**
 * formatElapsed：计算从 created_at 到 finished_at 的耗时（秒/分钟）。
 *   - finished_at 缺失时（任务未结束）返回 '-'
 *   - < 60s 显示 "12.3s"
 *   - >= 60s 显示 "2m 15s"
 */
function formatElapsed(item: VideoTaskItem): string {
  if (!item.finished_at) return '-'
  const s = new Date(item.created_at).getTime()
  const e = new Date(item.finished_at).getTime()
  if (Number.isNaN(s) || Number.isNaN(e) || e < s) return '-'
  const ms = e - s
  const secs = ms / 1000
  if (secs < 60) return `${secs.toFixed(1)}s`
  const m = Math.floor(secs / 60)
  const rest = Math.floor(secs % 60)
  return `${m}m ${rest}s`
}

/**
 * stringifyJson：安全打印 JSON；null 或空对象时展示占位符 '-'。
 * 用于 <pre> 展示 request_payload / result_payload。
 */
function stringifyJson(v: Record<string, unknown> | null): string {
  if (!v || Object.keys(v).length === 0) return '-'
  try {
    return JSON.stringify(v, null, 2)
  } catch {
    return String(v)
  }
}

/**
 * copyJson：把 payload 复制到剪贴板；失败给出 toast。
 */
async function copyJson(v: Record<string, unknown> | null) {
  const text = stringifyJson(v)
  try {
    await navigator.clipboard.writeText(text)
    appStore.showSuccess(t('common.copied'))
  } catch {
    appStore.showError(t('common.error'))
  }
}

/**
 * normalizedStatus：把后端各种状态归一到 i18n key 支持的 5 类：
 *   succeeded / running / pending / failed / refunded
 * 未识别值统一归到 pending，避免 i18n key not found。
 */
function normalizedStatus(s: string): string {
  switch (s) {
    case 'succeeded':
    case 'running':
    case 'pending':
    case 'failed':
    case 'refunded':
      return s
    case 'expired':
      return 'refunded'
    default:
      return 'pending'
  }
}

function statusBadgeClass(s: string): string {
  switch (normalizedStatus(s)) {
    case 'succeeded':
      return 'bg-green-100 text-green-700 dark:bg-green-950 dark:text-green-300'
    case 'running':
    case 'pending':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
    case 'failed':
      return 'bg-red-100 text-red-700 dark:bg-red-950 dark:text-red-300'
    case 'refunded':
      return 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'
    default:
      return 'bg-gray-100 text-gray-500 dark:bg-gray-800 dark:text-gray-400'
  }
}

// slug 变化（切模型）时重置到第一页
watch(
  () => props.slug,
  () => {
    page.value = 1
    void load()
  }
)

onMounted(() => {
  void load()
})
</script>
