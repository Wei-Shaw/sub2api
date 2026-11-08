<template>
  <AppLayout>
    <div class="space-y-6">
      <!-- 标题与说明已由页面顶部承载，这里不再重复渲染同一份文案，只保留操作按钮。 -->
      <div class="flex flex-wrap items-center justify-end gap-4">
        <div class="flex items-center gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load()">
            <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
            <span>{{ t('admin.intelligenceCheck.refresh') }}</span>
          </button>
          <button
            v-if="isAdmin"
            type="button"
            class="btn btn-primary btn-sm"
            :disabled="batchRunning || !runnableCount"
            @click="runAll"
          >
            <span>{{ batchRunning ? t('admin.intelligenceCheck.running') : t('admin.intelligenceCheck.runAll') }}</span>
          </button>
        </div>
      </div>

      <p
        v-if="notice"
        class="rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-xs text-emerald-800 dark:border-emerald-800/50 dark:bg-emerald-900/20 dark:text-emerald-200"
        data-testid="intelligence-check-notice"
      >
        {{ notice }}
      </p>

      <!-- 这条提示面向管理员（引导去「账号管理」开开关），普通用户侧不显示。 -->
      <p
        v-if="isAdmin && !loading && !cards.length"
        class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200"
      >
        {{ t('admin.intelligenceCheck.noEnabledAccounts') }}
      </p>

      <div v-if="loading && !cards.length" class="flex h-40 items-center justify-center text-gray-400">
        <Icon name="refresh" size="md" class="animate-spin" />
      </div>
      <!-- 普通用户侧的空态。管理员侧的空态由上面的引导条承担，避免同一件事说两遍。 -->
      <div
        v-else-if="!cards.length && !isAdmin"
        class="rounded-xl border border-dashed border-gray-300 p-10 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400"
      >
        {{ t('admin.intelligenceCheck.card.untestedHint') }}
      </div>
      <div v-else-if="cards.length" class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
        <!-- accountId 现在是「卡片键」：管理员 = 账号 id，普通用户 = 公开作品墙的展示序号。
             属性名保持不变，IntersectionObserver 依赖 data-account-id 取这个键。 -->
        <article
          v-for="card in cards"
          :key="card.accountId"
          :data-account-id="card.accountId"
          class="flex flex-col overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm transition hover:shadow-md dark:border-dark-600 dark:bg-dark-800"
        >
          <header class="flex items-start justify-between gap-3 px-4 py-3">
            <div class="min-w-0">
              <p class="truncate text-sm font-semibold text-gray-900 dark:text-white">
                {{ card.displayName }}
              </p>
              <!-- metaLine 两种身份都有内容：管理员是「#账号id · 模型 · 等级」，
                   普通用户是「模型 · 智力等级」（不带账号 id）；记录里缺模型信息时是空串，整行不渲染。 -->
              <p
                v-if="card.metaLine"
                class="mt-0.5 truncate text-xs text-gray-500 dark:text-gray-400"
              >
                {{ card.metaLine }}
              </p>
            </div>
            <span
              class="whitespace-nowrap rounded-full px-2 py-0.5 text-xs font-medium"
              :class="card.badgeClass"
            >
              {{ card.badgeLabel }}
            </span>
          </header>

          <button
            type="button"
            class="block aspect-[4/3] w-full overflow-hidden border-y border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-900"
            :class="card.hasArtifact && !card.inProgress ? 'cursor-zoom-in' : 'cursor-default'"
            :disabled="!card.hasArtifact || card.inProgress"
            :aria-label="card.displayName"
            @click="openPreview(card)"
          >
            <!-- 跑测中必须盖掉上一版作品：继续显示旧画面的话，用户完全看不出
                 「正在重新生成」，而旧作品恰恰最容易被误读成新结果。
                 转圈中心放实时耗时，一眼就能确认它真的在走。 -->
            <span
              v-if="card.inProgress"
              class="flex h-full flex-col items-center justify-center gap-3 text-xs text-gray-500 dark:text-gray-400"
            >
              <span class="relative inline-flex h-14 w-14 items-center justify-center">
                <svg
                  class="h-14 w-14 animate-spin text-gray-200 dark:text-dark-600"
                  viewBox="0 0 24 24"
                  fill="none"
                  aria-hidden="true"
                >
                  <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="3" />
                  <path class="opacity-75" fill="currentColor" d="M12 4a8 8 0 0 0-8 8h4a4 4 0 0 1 4-4V4z" />
                </svg>
                <span class="absolute text-[11px] font-medium tabular-nums">{{ card.elapsedLabel }}</span>
              </span>
              <span>{{ card.emptyLabel }}</span>
            </span>
            <!-- 作品是模型生成的不可信 HTML：缩略图用完全禁脚本的沙箱渲染，
                 全屏预览见下方弹窗（allow-scripts，但刻意不给 allow-same-origin）。 -->
            <iframe
              v-else-if="card.thumbnailHtml"
              :srcdoc="card.thumbnailHtml"
              sandbox=""
              class="pointer-events-none h-full w-full"
              tabindex="-1"
              :title="card.displayName"
            />
            <span
              v-else
              class="flex h-full flex-col items-center justify-center gap-2 px-4 text-center text-xs text-gray-400"
            >
              <!-- 跑测中 / 作品加载中：转圈动效，明确告诉用户「它在动」，而不是卡死。 -->
              <Icon
                v-if="card.pending"
                name="refresh"
                size="md"
                class="animate-spin text-gray-300 dark:text-dark-500"
              />
              <span>{{ card.emptyLabel }}</span>
            </span>
          </button>

          <footer class="mt-auto space-y-2 px-4 py-3">
            <div class="space-y-0.5">
              <p class="truncate text-xs text-gray-500 dark:text-gray-400">{{ card.detailLine }}</p>
              <!-- 评审状态行：管理员侧才有内容（待评审提示 / 已评审时间），普通用户侧恒为空串。 -->
              <p v-if="card.reviewNote" class="truncate text-xs" :class="card.reviewNoteClass">
                {{ card.reviewNote }}
              </p>
            </div>
            <!-- 人工评审与重跑都是管理员专属入口，普通用户侧整块不渲染。 -->
            <div v-if="isAdmin" class="flex flex-wrap items-center justify-end gap-2">
              <!-- 只有「跑测成功且产出了作品」的记录才谈得上评审：
                   执行失败（含中断）没有作品无从评起，跑测中的结果还没出来。 -->
              <template v-if="card.canReview">
                <button
                  type="button"
                  class="btn btn-sm shrink-0"
                  :class="card.verdict === 'pass' ? 'btn-success' : 'btn-secondary'"
                  :aria-pressed="card.verdict === 'pass'"
                  :disabled="card.reviewing"
                  @click="review(card, 'pass')"
                >
                  <Icon name="check" size="sm" />
                  <span>{{ t('admin.intelligenceCheck.card.approve') }}</span>
                </button>
                <button
                  type="button"
                  class="btn btn-sm shrink-0"
                  :class="card.verdict === 'fail' ? 'btn-danger' : 'btn-secondary'"
                  :aria-pressed="card.verdict === 'fail'"
                  :disabled="card.reviewing"
                  @click="review(card, 'fail')"
                >
                  <Icon name="x" size="sm" />
                  <span>{{ t('admin.intelligenceCheck.card.reject') }}</span>
                </button>
              </template>
              <button
                type="button"
                class="btn btn-secondary btn-sm shrink-0"
                :disabled="card.inProgress || card.reviewing"
                @click="runOne(card)"
              >
                {{ card.inProgress ? t('admin.intelligenceCheck.running') : t('admin.intelligenceCheck.rerun') }}
              </button>
            </div>
          </footer>
        </article>
      </div>

      <BaseDialog :show="preview.show" :title="preview.title" width="full" @close="closePreview">
        <div class="space-y-3">
          <p
            class="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800 dark:border-amber-800/50 dark:bg-amber-900/20 dark:text-amber-200"
          >
            {{ t('admin.intelligenceCheck.previewNotice') }}
          </p>
          <iframe
            :srcdoc="preview.html"
            sandbox="allow-scripts"
            class="h-[70vh] w-full rounded-lg border border-gray-200 bg-white dark:border-dark-600"
            :title="preview.title"
          />
        </div>
      </BaseDialog>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { IntelligenceCheckReviewVerdict } from '@/api/admin/intelligenceCheck'
import {
  intelligenceCheckPublicAPI,
  type IntelligenceCheckPublicCard
} from '@/api/intelligenceCheckPublic'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import type { AccountListItem, IntelligenceCheckRun } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
// 同一页面服务两种身份：管理员看账号维度的真身数据，普通登录用户只看脱敏作品墙。
const isAdmin = computed(() => authStore.isAdmin)

/**
 * 卡片状态。它同时吸收「跑测执行状态」与「人工评审结论」两件事，两者绝不能混为一谈：
 *   - unknown    = 已产出作品，等待管理员评审（后端 verdict=unknown）
 *   - pass/fail  = 管理员评的通过 / 不通过
 *   - runFailed  = 跑测执行失败（超时 / 未配模型 / 没抽出作品 / 中断），没有作品，无从评审
 *   - running    = 排队中或跑测中
 *   - untested   = 还没有跑测记录
 */
type CardVerdict = 'pass' | 'fail' | 'unknown' | 'runFailed' | 'running' | 'untested'

interface CheckCard {
  /** 卡片键：管理员 = 账号 id，普通用户 = 公开作品墙的展示序号。 */
  accountId: number
  displayName: string
  metaLine: string
  detailLine: string
  badgeLabel: string
  badgeClass: string
  verdict: CardVerdict
  enabled: boolean
  running: boolean
  /** 评审请求提交中（仅管理员侧可能为 true）。 */
  reviewing: boolean
  /** 该卡片是否可评审：跑测成功且产出了作品，与后端评审接口的准入条件一致。 */
  canReview: boolean
  /** 评审状态提示行（待评审 / 已完成评审）；无话可说时为空串，整行不渲染。 */
  reviewNote: string
  reviewNoteClass: string
  latestRunId: number | null
  /** 普通用户侧的作品地址（由公开接口下发，run id 从中解析）；管理员侧为空串。 */
  artifactUrl: string
  hasArtifact: boolean
  thumbnailHtml: string
  emptyLabel: string
  /** 作品尚未就绪（跑测中或作品加载中）：展示区显示转圈动效，而不是干巴巴一句文案。 */
  pending: boolean
  /** 跑测中（服务端事实，不是本地点击标记）：展示区要盖掉旧作品、显示转圈与实时计时。 */
  inProgress: boolean
  /** 跑测中的实时耗时（形如 18.1s），显示在转圈中心；非跑测中时为空串。 */
  elapsedLabel: string
}

const ACCOUNTS_PAGE_SIZE = 200
const RUNS_PAGE_SIZE = 500
const POLL_INTERVAL_MS = 5000
const NOTICE_TIMEOUT_MS = 5000

const BADGE_CLASSES: Record<CardVerdict, string> = {
  pass: 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-300',
  fail: 'bg-rose-100 text-rose-700 dark:bg-rose-900/30 dark:text-rose-300',
  // 待评审是「等管理员动手」，用天蓝跟同为灰底的中性态（未跑测）区分开。
  unknown: 'bg-sky-100 text-sky-700 dark:bg-sky-900/30 dark:text-sky-300',
  runFailed: 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-300',
  running: 'bg-amber-100 text-amber-700 dark:bg-amber-900/30 dark:text-amber-300',
  untested: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-300',
}

// 评审状态提示行的配色：待评审要显眼，评审完成后回归中性。
const REVIEW_NOTE_CLASSES = {
  pending: 'font-medium text-sky-600 dark:text-sky-400',
  done: 'text-gray-500 dark:text-gray-400',
} as const

/**
 * 列表接口只保证下发 verdict；评审人 / 评审时间属于评审接口的返回体。
 * 这里按可选字段读取，后端若在列表响应里补齐 reviewed_at，「评审于 X」会自动生效，
 * 补不齐就退化成「已完成评审」，不会因为字段缺席而误判成待评审。
 */
type IntelligenceCheckRunWithReview = IntelligenceCheckRun & { reviewed_at?: string | null }

// 「源状态」：账号、每账号最新一次跑测、本地发起中的账号、本地评审中的账号、已取回的作品缩略图。
const accounts = ref<AccountListItem[]>([])
const latestRuns = ref<Record<number, IntelligenceCheckRun>>({})
const runningAccountIds = ref<number[]>([])
const reviewingAccountIds = ref<number[]>([])
const thumbnails = ref<Record<number, string>>({})
// 普通用户侧的脱敏作品墙：只由公开接口填充，不涉及任何 admin 数据。
const publicCards = ref<IntelligenceCheckPublicCard[]>([])

const loading = ref(false)
const batchRunning = ref(false)
const notice = ref('')
const preview = ref({ show: false, title: '', html: '' })

let thumbnailObserver: IntersectionObserver | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
let noticeTimer: ReturnType<typeof setTimeout> | null = null

const showNotice = (message: string) => {
  notice.value = message
  if (noticeTimer) clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => {
    notice.value = ''
    noticeTimer = null
  }, NOTICE_TIMEOUT_MS)
}

const runnableCount = computed(() => cards.value.filter(card => card.enabled).length)

// 可评审 = 后端评审接口的准入条件：跑测完成（status=completed）且产出了作品。
// 判断顺序很关键：执行失败必须排在 verdict 之前，否则「跑测挂了」会被显示成
// 管理员评的「不通过」——那是两个完全不同的结论。
const isReviewable = (
  run: { status?: string; has_html?: boolean } | undefined,
  hasArtifact: boolean
) => run?.status === 'completed' && hasArtifact

// 判定只读 status / verdict / has_html：管理员记录与公开脱敏卡片共用这一套映射。
const verdictOf = (
  run: { status?: string; verdict?: string } | undefined,
  running: boolean,
  hasArtifact: boolean
): CardVerdict => {
  if (running || run?.status === 'queued' || run?.status === 'running') return 'running'
  if (!run) return 'untested'
  // 跑测本身没跑成（超时 / 未配模型 / 没抽出作品 / 被中断）：没有作品，无从评审。
  if (!isReviewable(run, hasArtifact)) return 'runFailed'
  if (run.verdict === 'pass') return 'pass'
  if (run.verdict === 'fail') return 'fail'
  // 已产出作品但 verdict 还是 unknown：等管理员人工评审。
  return 'unknown'
}

const badgeLabelFor = (verdict: CardVerdict) => {
  if (verdict === 'pass') return t('admin.intelligenceCheck.badge.pass')
  if (verdict === 'fail') return t('admin.intelligenceCheck.badge.fail')
  if (verdict === 'runFailed') return t('admin.intelligenceCheck.badge.runFailed')
  if (verdict === 'running') return t('admin.intelligenceCheck.badge.running')
  if (verdict === 'untested') return t('admin.intelligenceCheck.badge.untested')
  return t('admin.intelligenceCheck.badge.unknown')
}

// 评审状态提示行（仅管理员侧调用）：待评审时催一句，评完标出结论时间。
const reviewNoteFor = (verdict: CardVerdict, reviewedAt: string) => {
  if (verdict === 'unknown') return t('admin.intelligenceCheck.card.reviewHint')
  if (verdict !== 'pass' && verdict !== 'fail') return ''
  return reviewedAt
    ? t('admin.intelligenceCheck.card.reviewed', { time: reviewedAt })
    : t('admin.intelligenceCheck.card.reviewedNoTime')
}

const formatDateTime = (value?: string) => {
  if (!value) return '-'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

const formatLatency = (milliseconds: number) =>
  milliseconds > 0 ? `${(milliseconds / 1000).toFixed(1)}s` : '-'

/**
 * 普通用户卡片第二行的「模型 · 智力等级」。
 * 模型名缺失时整行不渲染（不编造「未记录模型」——那会让人以为是系统记错了）；
 * 只有模型、没有智力等级时只显示模型名。
 * 这里刻意不带账号 id / 账号名 / 上游标识，那是管理员侧才给的信息。
 */
const formatPublicMeta = (modelId?: string, effort?: string) => {
  const model = (modelId ?? '').trim()
  if (!model) return ''
  const level = (effort ?? '').trim()
  return level
    ? t('admin.intelligenceCheck.card.publicMetaWithEffort', { model, effort: level })
    : model
}

// 跑测中卡片的耗时必须是「走着」的：后端要等跑完才回填 latency_ms，
// 在此之前界面上只能自己从 created_at 起算，否则用户看到的是一个恒定的「-」，
// 完全无法判断它是在跑还是已经卡死（这正是「以为卡住」的主要来源）。
// 统一按秒展示（哪怕超过一分钟也还是秒），与跑完后的 latency 格式保持一致。
const nowTick = ref(Date.now())
let tickTimer: ReturnType<typeof setInterval> | null = null

const startTicking = () => {
  if (tickTimer) return
  tickTimer = setInterval(() => {
    nowTick.value = Date.now()
  }, 1000)
}

const stopTicking = () => {
  if (tickTimer) {
    clearInterval(tickTimer)
    tickTimer = null
  }
}

// 管理员墙上只出现「已在 账号管理 → 编辑账号 → 智力检测 打开开关」的账号；
// 没开开关的账号一律不上墙，于是卡片数 = 已启用智力检测的账号数。
const enabledAccounts = computed(() =>
  accounts.value.filter(account => account.extra?.intelligence_check_enabled === true)
)

// 卡片完全由源状态派生，避免「本地状态」与「服务器状态」两份真相互相打架。
// 身份不同 → 数据源不同：管理员由账号 + 跑测记录派生，普通用户由脱敏作品墙派生。
const cards = computed<CheckCard[]>(() => {
  // 普通用户分支：展示序号即卡片键，账号 id / 账号名 / 上游身份一律不出现；
  // 模型名与智力等级照实展示（公开接口已带出），这正是这面墙的看点。
  if (!isAdmin.value) {
    return publicCards.value.map(card => {
      const verdict = verdictOf(
        { status: card.status, verdict: card.verdict },
        false,
        card.has_artifact
      )
      return {
        accountId: card.index,
        displayName: t('admin.intelligenceCheck.accountLabel', { index: card.index }),
        // 模型名与智力等级对用户可见；缺模型时留空，而不是编一个「未记录模型」。
        metaLine: formatPublicMeta(card.model_id, card.reasoning_effort),
        detailLine: t('admin.intelligenceCheck.card.detail', {
          // 普通用户侧同样实时起算：跑测中只显示「-」会让人以为根本没在跑。
          latency: formatLatency(
            verdict === 'running'
              ? Math.max(0, nowTick.value - new Date(card.created_at).getTime())
              : card.latency_ms
          ),
          time: formatDateTime(card.created_at),
        }),
        badgeLabel: badgeLabelFor(verdict),
        badgeClass: BADGE_CLASSES[verdict],
        verdict,
        // 普通用户只能看结果：跑测与评审的入口一律不开放，模板再用 isAdmin 兜一层。
        enabled: false,
        running: false,
        reviewing: false,
        canReview: false,
        reviewNote: '',
        reviewNoteClass: REVIEW_NOTE_CLASSES.done,
        latestRunId: null,
        artifactUrl: card.artifact_url ?? '',
        hasArtifact: card.has_artifact,
        thumbnailHtml: thumbnails.value[card.index] ?? '',
        emptyLabel:
          verdict === 'running'
            ? t('admin.intelligenceCheck.card.generating')
            : card.has_artifact
              ? t('admin.intelligenceCheck.card.pendingThumbnail')
              : t('admin.intelligenceCheck.card.noArtifact'),
        pending: verdict === 'running' || card.has_artifact,
        inProgress: verdict === 'running',
        elapsedLabel:
          verdict === 'running'
            ? formatLatency(Math.max(0, nowTick.value - new Date(card.created_at).getTime()))
            : '',
      }
    })
  }
  // 管理员分支：只遍历已开启智力检测的账号（accountId = account.id）。
  return enabledAccounts.value.map(account => {
    const run = latestRuns.value[account.id] as IntelligenceCheckRunWithReview | undefined
    // locallyRunning 只表示「本页点过重跑」；它只是一个给 verdictOf 兜底的本地提示，
    // 真正的跑测状态一律以服务端 run.status 为准（见 inProgress）。
    const locallyRunning = runningAccountIds.value.includes(account.id)
    const hasArtifact = Boolean(run?.has_html)
    const verdict = verdictOf(run, locallyRunning, hasArtifact)
    // 跑测中 = 服务端判定，覆盖定时任务与其它会话发起的跑测。
    const inProgress = verdict === 'running'
    // 只有管理员能评审，且只有「跑测成功 + 有作品」的记录才有得评。
    const canReview = verdict === 'pass' || verdict === 'fail' || verdict === 'unknown'
    const reviewedAt = run?.reviewed_at ? formatDateTime(run.reviewed_at) : ''
    return {
      accountId: account.id,
      // 管理员看真身：账号名 + 账号 ID，方便直接对着「账号管理」定位。
      displayName: account.name,
      metaLine: run?.reasoning_effort
        ? t('admin.intelligenceCheck.card.accountMetaWithEffort', {
            id: account.id,
            model: run.model_id || t('admin.intelligenceCheck.card.noModel'),
            effort: run.reasoning_effort,
          })
        : t('admin.intelligenceCheck.card.accountMeta', {
            id: account.id,
            model: run?.model_id || t('admin.intelligenceCheck.card.noModel'),
          }),
      detailLine: run
        ? t('admin.intelligenceCheck.card.detail', {
            // 跑测中从 created_at 实时起算（每秒推进），跑完后再用后端回填的真实耗时。
            latency: formatLatency(
              inProgress
                ? Math.max(0, nowTick.value - new Date(run.created_at).getTime())
                : run.latency_ms
            ),
            time: formatDateTime(run.created_at),
          })
        : t('admin.intelligenceCheck.card.untestedHint'),
      badgeLabel: badgeLabelFor(verdict),
      badgeClass: BADGE_CLASSES[verdict],
      verdict,
      enabled: account.extra?.intelligence_check_enabled === true,
      // 模板与展示逻辑一律用 inProgress（服务端判定）；
      // running 保留但同义，避免留下第二个真相来源。
      running: inProgress,
      inProgress,
      reviewing: reviewingAccountIds.value.includes(account.id),
      canReview,
      reviewNote: reviewNoteFor(verdict, reviewedAt),
      reviewNoteClass:
        verdict === 'unknown' ? REVIEW_NOTE_CLASSES.pending : REVIEW_NOTE_CLASSES.done,
      latestRunId: run?.id ?? null,
      // 管理员侧作品按 run id 取，用不到公开地址。
      artifactUrl: '',
      hasArtifact,
      thumbnailHtml: thumbnails.value[account.id] ?? '',
      // 三态：跑测中说的是「生成中」，有作品但 HTML 还没取回来是「加载中」，
      // 其余（含执行失败）才是真的「暂无作品」。
      emptyLabel: inProgress
        ? t('admin.intelligenceCheck.card.generating')
        : hasArtifact
          ? t('admin.intelligenceCheck.card.pendingThumbnail')
          : t('admin.intelligenceCheck.card.noArtifact'),
      pending: inProgress || hasArtifact,
      elapsedLabel:
        inProgress && run
          ? formatLatency(Math.max(0, nowTick.value - new Date(run.created_at).getTime()))
          : '',
    }
  })
})

// 后端已按创建时间倒序返回，这里再按时间兜一次底，取每个账号最新的一条。
const indexLatestRuns = (runs: IntelligenceCheckRun[]) => {
  const indexed: Record<number, IntelligenceCheckRun> = {}
  for (const run of runs) {
    const existing = indexed[run.account_id]
    if (!existing || new Date(run.created_at).getTime() > new Date(existing.created_at).getTime()) {
      indexed[run.account_id] = run
    }
  }
  return indexed
}

// 「取作品 HTML」的唯一入口：缩略图与全屏预览都走它，按身份选接口。
const fetchArtifactHtml = async (card: CheckCard): Promise<string> => {
  if (isAdmin.value) {
    if (card.latestRunId == null) throw new Error(t('admin.intelligenceCheck.error.load'))
    return adminAPI.intelligenceCheck.getArtifact(card.latestRunId)
  }
  // 普通用户：公开接口只给 artifact_url（…/runs/123/artifact），run id 从末尾解析。
  const matched = card.artifactUrl.match(/(\d+)(?=[^\d]*$)/)
  const runId = matched ? Number(matched[1]) : 0
  if (!Number.isFinite(runId) || runId <= 0) throw new Error(t('admin.intelligenceCheck.error.load'))
  return intelligenceCheckPublicAPI.getPublicArtifact(runId)
}

// accountId 是卡片键：管理员为账号 id，普通用户为展示序号。
// 缓存必须绑定「具体哪一次跑测」：重跑会产生新的 run id，若只按 accountId 缓存、
// 命中就返回，界面会永远停在上一版作品上（卡片缩略图与详情弹窗都是旧代码），
// 只有整体刷新网页才恢复 —— 那正是因为整页重载把这个缓存清空了。
const thumbnailRunIds = ref<Record<number, number | null>>({})

const loadThumbnail = async (accountId: number) => {
  const card = cards.value.find(item => item.accountId === accountId)
  if (!card?.hasArtifact) return
  // 只有「缓存命中 + 仍属同一次跑测」才复用，否则一律重新取。
  if (thumbnails.value[accountId] && thumbnailRunIds.value[accountId] === card.latestRunId) return
  try {
    const html = await fetchArtifactHtml(card)
    if (!html) return
    thumbnails.value = { ...thumbnails.value, [accountId]: html }
    thumbnailRunIds.value = { ...thumbnailRunIds.value, [accountId]: card.latestRunId }
  } catch {
    // 缩略图是锦上添花，取不到就保留占位文案，不打扰用户。
  }
}

// 丢弃某个账号已缓存的作品。重跑会生成新的 run id，旧缓存必须失效，
// 否则卡片缩略图与详情弹窗都会继续显示上一版作品，直到用户强制刷新整页。
const clearThumbnail = (accountId: number) => {
  if (!(accountId in thumbnails.value) && !(accountId in thumbnailRunIds.value)) return
  const nextHtml = { ...thumbnails.value }
  delete nextHtml[accountId]
  thumbnails.value = nextHtml
  const nextRuns = { ...thumbnailRunIds.value }
  delete nextRuns[accountId]
  thumbnailRunIds.value = nextRuns
}

// 缩略图只在卡片进入视口时才取，避免账号多时一次性打满作品原文接口。
//
// unobserve 的时机是这里的关键：只有「确实拿到作品 HTML」才停止观察。
// 如果卡片还卡在跑测中（暂无作品）就 unobserve，那么等跑测完成、作品真的出现时，
// 观察器早已不再看这张卡片 —— 表现就是「作品加载中…」永远不动，
// 必须整体刷新页面（重新 mount 才会重新 observe）才能显示出来。
const observeThumbnails = () => {
  thumbnailObserver?.disconnect()
  if (typeof IntersectionObserver === 'undefined') return
  thumbnailObserver = new IntersectionObserver(
    entries => {
      for (const entry of entries) {
        if (!entry.isIntersecting) continue
        const accountId = Number((entry.target as HTMLElement).dataset.accountId)
        if (!Number.isFinite(accountId)) continue
        // 加载完成后再决定是否收工：有作品就停，没作品就继续等它出现。
        void loadThumbnail(accountId).then(() => {
          const card = cards.value.find(item => item.accountId === accountId)
          if (card?.thumbnailHtml) thumbnailObserver?.unobserve(entry.target)
        })
      }
    },
    { rootMargin: '200px' }
  )
  document.querySelectorAll('[data-account-id]').forEach(element => thumbnailObserver?.observe(element))
}

const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// 是否还有跑测在途。既包括本页刚触发的（runningAccountIds 是本地状态、刷新即丢），
// 也包括服务端仍在 queued / running 的记录 —— 后者才是刷新页面后唯一可靠的信号。
// 只看本地状态会导致「刷新之后永远不再自动更新」，用户只能靠手动刷新碰运气。
const hasInFlightRuns = computed(
  () =>
    runningAccountIds.value.length > 0 ||
    Object.values(latestRuns.value).some(run => run.status === 'queued' || run.status === 'running')
)

const startPolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(() => {
    if (!hasInFlightRuns.value) {
      stopPolling()
      return
    }
    void load(true)
  }, POLL_INTERVAL_MS)
}

const load = async (silent = false) => {
  if (!silent) loading.value = true
  try {
    // 普通用户没有 admin 权限：只调脱敏公开接口，绝不触碰任何 admin API。
    if (!isAdmin.value) {
      const wall = await intelligenceCheckPublicAPI.listPublicRuns()
      publicCards.value = wall?.items ?? []
      return
    }
    const [accountPage, runPage] = await Promise.all([
      adminAPI.accounts.list(1, ACCOUNTS_PAGE_SIZE, { sort_by: 'id', sort_order: 'asc' }),
      adminAPI.intelligenceCheck.listRuns({ page: 1, page_size: RUNS_PAGE_SIZE }),
    ])
    accounts.value = accountPage?.items ?? []
    latestRuns.value = indexLatestRuns(runPage?.items ?? [])
    // 本地「发起中」标记必须跟着服务端事实收敛：verdictOf 里它的优先级高于
    // run.status，只要它还挂着，卡片就永远显示「跑测中」，哪怕服务端早已 completed。
    // 以前靠用户手动刷新页面（重新 mount）才会清空，所以这个 bug 一直藏着。
    // 看不到记录时保持原样（刚发起、列表还没刷到），一旦看到明确终态就摘掉标记。
    runningAccountIds.value = runningAccountIds.value.filter(id => {
      const run = latestRuns.value[id]
      if (!run) return true
      return run.status === 'queued' || run.status === 'running'
    })
  } catch (error: any) {
    if (!silent) appStore.showError(error?.message || t('admin.intelligenceCheck.error.load'))
  } finally {
    if (!silent) loading.value = false
  }
  // 数据落地后按最新事实决定是否继续轮询：有在途跑测就自动跟着刷新，跑完自动停，
  // 用户不必盯着页面手动刷（这正是「刷新后一直停在跑测中」的根因）。
  if (hasInFlightRuns.value) startPolling()
  else stopPolling()
  // 计时器与轮询同生共死：只有在途跑测才需要每秒推进耗时，跑完就停，不空转。
  if (hasInFlightRuns.value) startTicking()
  else stopTicking()
}

const triggerRun = async (accountId: number) => {
  // 手动跑测是管理员专属：普通用户侧按钮已隐藏，这里再兜一层，确保永不发出 createRun。
  if (!isAdmin.value) return false
  if (!runningAccountIds.value.includes(accountId)) {
    runningAccountIds.value = [...runningAccountIds.value, accountId]
  }
  try {
    await adminAPI.intelligenceCheck.createRun({ account_id: accountId })
    startPolling()
    // 立刻开始计时，不必等下一轮轮询落地，点下去就能看到秒数在走。
    startTicking()
    // 同时丢掉上一版作品：点下去就应显示「生成中 + 转圈 + 计时」，
    // 而不是继续显示上一次的画面（那最容易被误读成新结果）。
    clearThumbnail(accountId)
    // 立刻拉一次，让新建的 queued 记录马上进列表：否则下一轮 load 可能先看到
    // 上一轮的终态记录，把本地「发起中」标记收敛掉，出现短暂的状态回退。
    void load(true)
    return true
  } catch (error: any) {
    runningAccountIds.value = runningAccountIds.value.filter(id => id !== accountId)
    appStore.showError(error?.message || t('admin.intelligenceCheck.error.trigger'))
    return false
  }
}

const runOne = async (card: CheckCard) => {
  // 提交后不再弹横幅：卡片立刻变成「跑测中 + 转圈 + 实时计时」，
  // 这本身就是最直接、最靠近注意力的反馈，再叠一条提示只是噪音。
  await triggerRun(card.accountId)
}

const runAll = async () => {
  const targets = cards.value.filter(card => card.enabled && !card.inProgress)
  if (!targets.length) return
  batchRunning.value = true
  try {
    for (const card of targets) {
      // 串行触发：单次批量跑测不该把上游账号同时打满。
      // 同样不弹提示，卡片状态就是反馈。
      await triggerRun(card.accountId)
    }
  } finally {
    batchRunning.value = false
  }
}

// 人工评审：只有管理员能点，后端只接受「跑测成功且产出了作品」的记录。
// 评审成功后静默刷新，让徽章、统计与评审时间一起跟上（不打断管理员连续评审的节奏）。
const review = async (card: CheckCard, verdict: IntelligenceCheckReviewVerdict) => {
  // 评审是管理员专属动作：普通用户侧按钮不渲染，这里再兜一层，确保永不发出 reviewRun。
  if (!isAdmin.value || !card.canReview || card.reviewing) return
  if (card.latestRunId == null) return
  reviewingAccountIds.value = [...reviewingAccountIds.value, card.accountId]
  try {
    const result = await adminAPI.intelligenceCheck.reviewRun(card.latestRunId, verdict)
    await load(true)
    const verdictLabel =
      verdict === 'pass'
        ? t('admin.intelligenceCheck.card.approve')
        : t('admin.intelligenceCheck.card.reject')
    showNotice(
      result?.status_synced
        ? t('admin.intelligenceCheck.reviewSubmittedSynced', { verdict: verdictLabel })
        : t('admin.intelligenceCheck.reviewSubmitted', { verdict: verdictLabel })
    )
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.intelligenceCheck.error.review'))
  } finally {
    reviewingAccountIds.value = reviewingAccountIds.value.filter(id => id !== card.accountId)
  }
}

const openPreview = async (card: CheckCard) => {
  if (!card.hasArtifact) return
  try {
    const html = await fetchArtifactHtml(card)
    preview.value = { show: true, title: card.displayName, html }
  } catch (error: any) {
    appStore.showError(error?.message || t('admin.intelligenceCheck.error.load'))
  }
}

const closePreview = () => {
  preview.value = { show: false, title: '', html: '' }
}

// 重新挂观察器的时机有两类：卡片增删，以及「作品从无到有」。
// 只盯 cards.length 会漏掉后者 —— 跑测完成后卡片是原地变化（数量不变，
// 但 hasArtifact 从 false 变 true），此时新出现的作品需要被重新观察才会加载。
watch(
  () => cards.value.map(card => `${card.accountId}:${card.hasArtifact ? 1 : 0}`).join(','),
  async () => {
    await nextTick()
    observeThumbnails()
  }
)

onMounted(() => {
  void load()
})

onBeforeUnmount(() => {
  thumbnailObserver?.disconnect()
  thumbnailObserver = null
  stopPolling()
  stopTicking()
  if (noticeTimer) clearTimeout(noticeTimer)
})
</script>
