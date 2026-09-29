<template>
  <div
    v-if="visible"
    data-test="cn-provider-quota"
    class="min-w-[220px] space-y-1"
  >
    <!-- Tier rows: 5h + weekly utilization bars (snapshot renders on mount).
         复用账号页 UsageProgressBar：同阈值配色、同倒计时格式。 -->
    <div v-if="data?.success && data.tiers?.length" class="space-y-1">
      <UsageProgressBar
        v-for="tier in data.tiers"
        :key="tier.window"
        data-test="cn-provider-quota-tier"
        :label="windowLabel(tier.window)"
        :color="tier.window === 'weekly' ? 'emerald' : 'indigo'"
        :utilization="tier.used_percent"
        :resets-at="tier.reset_at"
      />
    </div>

    <!-- Explicit refresh action (aligned with the OpenAI "Query" / Grok "Probe"
         buttons): a verb label tells users this chip is clickable. The previous
         noun label ("5h/weekly") read as a passive caption and users could not
         discover the manual refresh. -->
    <div class="flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        data-test="cn-provider-quota-probe"
        class="inline-flex items-center gap-0.5 whitespace-nowrap rounded px-1.5 py-0.5 text-[10px] font-medium leading-4 text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading"
        :title="t('admin.accounts.cnProviders.probeTooltip')"
        @click="handleProbe(true)"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M4 4v5h.582m15.356 2A8.001 8.001 0 004.582 9m0 0H9m11 11v-5h-.581m0 0a8.003 8.003 0 01-15.357-2m15.357 2H15"
          />
        </svg>
        {{ t('admin.accounts.cnProviders.probe') }}
      </button>
    </div>

    <!-- 智谱 GLM Coding Plan（国内站个人版）重置卡：卡数摘要 + 手动用卡。 -->
    <ZhipuResetCardActions
      v-if="resetCardsSupported"
      :account="account"
      :cards="resetCards"
      :tiers="data?.tiers ?? null"
      @used="handleResetCardUsed"
    />

    <div
      v-if="error"
      class="truncate text-[10px] leading-4 text-red-600 dark:text-red-400"
      :title="error"
    >
      {{ truncatedError }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CNProviderQuotaProbeResult,
  ZhipuResetCards,
  ZhipuResetCardUseResult
} from '@/api/admin/cnProviders'
import type { Account } from '@/types'
import { cnQuotaCellVisible, zhipuResetCardsSupported } from './credentialsBuilder'
import UsageProgressBar from './UsageProgressBar.vue'
import ZhipuResetCardActions from './ZhipuResetCardActions.vue'

const props = defineProps<{
  account: Account
}>()

const { t } = useI18n()

const readMode = (): string => {
  const mode = props.account.credentials?.account_mode
  return typeof mode === 'string' ? mode : ''
}

const visible = computed(() => cnQuotaCellVisible(props.account.platform, readMode()))

const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<CNProviderQuotaProbeResult | null>(null)

const resetCardsSupported = computed(() =>
  zhipuResetCardsSupported(props.account.platform, props.account.credentials)
)
// 最近一次查询 / 用卡返回的重置卡列表（后端已同步落 extra 快照）。为空时子组件读
// account.extra；父组件刷新账号（extra 里的卡快照时间变化）后回到读快照。
const resetCards = ref<ZhipuResetCards | null>(null)

// 后端周期任务/手动探测写入的 extra 快照键（<provider>_ 前缀，与后端
// cnQuotaExtraUpdates 对齐）。页面加载即有数据，无需等待探测。
const SNAPSHOT_STALE_MS = 15 * 60 * 1000

// 自动探测去抖窗口与最近一次自动探测时间（模块级，跨实例共享）。
const AUTO_PROBE_DEBOUNCE_MS = 5 * 60 * 1000
const lastAutoProbeAt = new Map<number, number>()

const readExtraNumber = (key: string): number | null => {
  const v = (props.account.extra as Record<string, unknown> | undefined)?.[key]
  return typeof v === 'number' && Number.isFinite(v) ? v : null
}

const readExtraString = (key: string): string => {
  const v = (props.account.extra as Record<string, unknown> | undefined)?.[key]
  return typeof v === 'string' ? v : ''
}

// 从持久化快照构造展示数据（缺少 5h/weekly 两档键时返回 null）。
const snapshotData = computed<CNProviderQuotaProbeResult | null>(() => {
  const platform = props.account.platform
  const used5h = readExtraNumber(`${platform}_5h_used_percent`)
  const usedWeekly = readExtraNumber(`${platform}_weekly_used_percent`)
  const usedMonthly = readExtraNumber(`${platform}_monthly_used_percent`)
  if (used5h == null && usedWeekly == null && usedMonthly == null) return null
  const tiers: CNProviderQuotaProbeResult['tiers'] = []
  if (used5h != null) {
    tiers.push({ window: '5h', used_percent: used5h, reset_at: readExtraString(`${platform}_5h_reset_at`) || undefined })
  }
  if (usedWeekly != null) {
    tiers.push({ window: 'weekly', used_percent: usedWeekly, reset_at: readExtraString(`${platform}_weekly_reset_at`) || undefined })
  }
  if (usedMonthly != null) {
    tiers.push({ window: 'monthly', used_percent: usedMonthly, reset_at: readExtraString(`${platform}_monthly_reset_at`) || undefined })
  }
  return { success: true, tiers } as CNProviderQuotaProbeResult
})

// 快照是否过期（无更新时间或超过 staleness 窗口）→ 挂载时需要自动探测。
const snapshotIsStale = computed(() => {
  const updatedAt = readExtraString(`${props.account.platform}_usage_updated_at`)
  if (!updatedAt) return true
  const ts = new Date(updatedAt).getTime()
  return Number.isNaN(ts) || Date.now() - ts > SNAPSHOT_STALE_MS
})

// 挂载时：先用持久化快照渲染；快照缺失或过期再自动探测一次（失败显示错误，
// 避免静默失败导致单元格空白无提示）。
onMounted(() => {
  if (!visible.value) return
  data.value = snapshotData.value
  if (!snapshotIsStale.value) return
  // 模块级去抖：列表页每行一个实例，翻页/筛选/刷新会重复挂载；同一账号
  // 短时间内已自动探测过则跳过，避免对上游形成探测风暴。
  const last = lastAutoProbeAt.get(props.account.id) ?? 0
  if (Date.now() - last < AUTO_PROBE_DEBOUNCE_MS) return
  lastAutoProbeAt.set(props.account.id, Date.now())
  handleProbe()
})

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}...` : error.value
})

const windowLabel = (window: string) => {
  if (window === 'weekly') return t('admin.accounts.cnProviders.windowWeekly')
  if (window === 'monthly') return t('admin.accounts.cnProviders.windowMonthly')
  return t('admin.accounts.cnProviders.window5h')
}

// 刷新重置卡列表，返回错误文案（成功返回 null）。
const refreshResetCards = async (): Promise<string | null> => {
  const accountID = props.account.id
  try {
    const cards = await adminAPI.cnProviders.listResetCards(accountID)
    if (props.account.id === accountID) resetCards.value = cards
    return null
  } catch (e) {
    return t('admin.accounts.cnProviders.resetCardsFailed', { error: extractErrorMessage(e) })
  }
}

// withResetCards：手动「查询」在支持重置卡的智谱账号下顺带刷新卡列表，与额度探测
// 并行、互不影响；挂载时的自动探测只刷新额度。
const handleProbe = async (withResetCards = false) => {
  if (loading.value) return
  loading.value = true
  error.value = null
  const cardsRequest = withResetCards && resetCardsSupported.value ? refreshResetCards() : null
  try {
    const result = await adminAPI.cnProviders.queryQuota(props.account.id)
    // 失败时保留已渲染的快照条形图（仅显示错误行），成功才覆盖。
    if (result.success) {
      data.value = result
    } else {
      error.value = result.error || t('common.error')
    }
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    const cardsError = await cardsRequest
    if (cardsError && !error.value) error.value = cardsError
    loading.value = false
  }
}

// 用卡成功：后端已刷新卡列表、强制重探额度并恢复账号状态，直接用响应刷新展示。
const handleResetCardUsed = (result: ZhipuResetCardUseResult) => {
  if (result.cards) resetCards.value = result.cards
  if (result.probe?.success) data.value = result.probe
}

watch(
  () => (props.account.extra as Record<string, unknown> | undefined)?.zhipu_reset_cards_updated_at,
  () => {
    resetCards.value = null
  }
)

watch(
  () => props.account.id,
  () => {
    data.value = null
    error.value = null
    loading.value = false
    resetCards.value = null
  }
)
</script>
