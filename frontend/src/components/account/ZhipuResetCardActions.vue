<template>
  <div data-test="zhipu-reset-cards" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <!-- 卡数摘要：读 extra 快照（或最近一次查询/用卡返回的列表）。
           从未查询过时显示占位，提示点「查询」刷新。 -->
      <span
        v-if="summary"
        data-test="zhipu-reset-cards-chip"
        class="inline-flex items-center gap-1 rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
        :aria-label="t('admin.accounts.cnProviders.resetCards')"
      >
        <span data-test="zhipu-reset-cards-week" :title="chipTitle('WEEK')">
          {{ t('admin.accounts.cnProviders.weekCards', { count: summary.weekCount }) }}
        </span>
        <span aria-hidden="true">·</span>
        <span data-test="zhipu-reset-cards-5h" :title="chipTitle('FIVE_HOUR')">
          {{ t('admin.accounts.cnProviders.fiveHourCards', { count: summary.fiveHourCount }) }}
        </span>
      </span>
      <span
        v-else
        data-test="zhipu-reset-cards-unknown"
        class="inline-flex items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-400 dark:bg-dark-800 dark:text-gray-500"
        :title="t('admin.accounts.cnProviders.resetCardsRefresh')"
      >
        {{ t('admin.accounts.cnProviders.resetCards') }} --
      </span>

      <button
        v-for="option in resetOptions"
        :key="option.type"
        type="button"
        :data-test="`zhipu-reset-${option.type === 'WEEK' ? 'week' : '5h'}`"
        class="inline-flex items-center gap-0.5 whitespace-nowrap rounded px-1.5 py-0.5 text-[10px] font-medium leading-4 text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="resetting || option.idle"
        :title="option.title"
        @click="openConfirm(option.type)"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': resetting && pendingType === option.type }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M20 12a8 8 0 11-2.343-5.657L20 8m0 0V4m0 4h-4"
          />
        </svg>
        {{ option.label }}
      </button>
    </div>

    <div
      v-if="earliestExpire"
      data-test="zhipu-reset-cards-expire"
      class="text-[10px] leading-4 text-gray-500 dark:text-gray-400"
    >
      {{ t('admin.accounts.cnProviders.resetExpire', { time: earliestExpire }) }}
    </div>

    <div
      v-if="error"
      data-test="zhipu-reset-error"
      class="truncate text-[10px] leading-4 text-red-600 dark:text-red-400"
      :title="error"
    >
      {{ error.length > 80 ? `${error.slice(0, 80)}...` : error }}
    </div>
    <div
      v-else-if="warning"
      data-test="zhipu-reset-warning"
      class="text-[10px] leading-4 text-amber-600 dark:text-amber-400"
    >
      {{ warning }}
    </div>
    <div
      v-else-if="message"
      data-test="zhipu-reset-success"
      class="text-[10px] leading-4 text-emerald-600 dark:text-emerald-400"
    >
      {{ message }}
    </div>

    <ConfirmDialog
      :show="showConfirm"
      :title="confirmTitle"
      :message="confirmMessage"
      :confirm-text="t('admin.accounts.cnProviders.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmReset"
      @cancel="showConfirm = false"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type {
  CNQuotaTier,
  ZhipuResetCards,
  ZhipuResetCardUseResult,
  ZhipuResetType
} from '@/api/admin/cnProviders'
import type { Account } from '@/types'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = withDefaults(
  defineProps<{
    account: Account
    /** 最近一次查询 / 用卡返回的卡列表；为空时读 account.extra 快照。 */
    cards?: ZhipuResetCards | null
    /** 当前展示的额度窗口，用于判断窗口用量为 0 时「无需重置」。 */
    tiers?: CNQuotaTier[] | null
  }>(),
  { cards: null, tiers: null }
)

const emit = defineEmits<{
  /** 用卡成功：携带用卡后的卡列表与强制重探结果，父组件据此刷新展示。 */
  used: [result: ZhipuResetCardUseResult]
}>()

const { t } = useI18n()

const resetting = ref(false)
const showConfirm = ref(false)
const pendingType = ref<ZhipuResetType>('WEEK')
const error = ref<string | null>(null)
const warning = ref<string | null>(null)
const message = ref<string | null>(null)

interface CardSummary {
  weekCount: number
  fiveHourCount: number
  weekExpireAt: string
  fiveHourExpireAt: string
}

// extra 快照键与后端 zhipuResetCardsExtraUpdates 对齐；没有 updated_at 表示从未查询过。
const summaryFromExtra = (): CardSummary | null => {
  const extra = props.account.extra as Record<string, unknown> | undefined
  if (typeof extra?.zhipu_reset_cards_updated_at !== 'string') return null
  const count = (countKey: string, availableKey: string) => {
    const value = extra[countKey]
    if (typeof value === 'number' && Number.isFinite(value) && value > 0) return Math.floor(value)
    return extra[availableKey] === true ? 1 : 0
  }
  const text = (key: string) => (typeof extra[key] === 'string' ? (extra[key] as string) : '')
  return {
    weekCount: count('zhipu_week_reset_count', 'zhipu_week_reset_available'),
    fiveHourCount: count('zhipu_5h_reset_count', 'zhipu_5h_reset_available'),
    weekExpireAt: text('zhipu_week_reset_expire_at'),
    fiveHourExpireAt: text('zhipu_5h_reset_expire_at')
  }
}

const summary = computed<CardSummary | null>(() => {
  const cards = props.cards
  if (!cards) return summaryFromExtra()
  return {
    weekCount: cards.week_cards?.length ?? 0,
    fiveHourCount: cards.five_hour_cards?.length ?? 0,
    weekExpireAt: cards.week_cards?.[0]?.expire_time ?? '',
    fiveHourExpireAt: cards.five_hour_cards?.[0]?.expire_time ?? ''
  }
})

watch(
  () => props.account.id,
  () => {
    error.value = null
    warning.value = null
    message.value = null
    showConfirm.value = false
    resetting.value = false
  }
)

// 官网到期时间是不带时区的北京时间（2026-10-05 23:59:59）：原样截取展示，不做时区换算；
// 其他可解析的格式按本地时间格式化，解析不了就原样展示。
const BEIJING_TIME_RE = /^(\d{4})-(\d{2})-(\d{2})[ T](\d{2}):(\d{2})(?::(\d{2}))?$/

const formatExpire = (raw: string, style: 'short' | 'full'): string => {
  const value = raw.trim()
  if (!value) return t('admin.accounts.cnProviders.resetExpireUnknown')
  const match = BEIJING_TIME_RE.exec(value)
  if (match) {
    const [, year, month, day, hour, minute] = match
    return style === 'short' ? `${month}-${day} ${hour}:${minute}` : `${year}-${month}-${day} ${hour}:${minute}`
  }
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const options: Intl.DateTimeFormatOptions = { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }
  if (style === 'full') options.year = 'numeric'
  return new Intl.DateTimeFormat(undefined, options).format(date)
}

const countOf = (type: ZhipuResetType) =>
  (type === 'WEEK' ? summary.value?.weekCount : summary.value?.fiveHourCount) ?? 0

const expireOf = (type: ZhipuResetType) =>
  (type === 'WEEK' ? summary.value?.weekExpireAt : summary.value?.fiveHourExpireAt) ?? ''

const cardName = (type: ZhipuResetType) =>
  type === 'WEEK' ? t('admin.accounts.cnProviders.cardWeek') : t('admin.accounts.cnProviders.card5h')

const windowName = (type: ZhipuResetType) =>
  type === 'WEEK'
    ? t('admin.accounts.cnProviders.resetWindowWeek')
    : t('admin.accounts.cnProviders.resetWindow5h')

const chipTitle = (type: ZhipuResetType) => {
  if (countOf(type) <= 0) return cardName(type)
  return `${cardName(type)} · ${t('admin.accounts.cnProviders.resetExpire', { time: formatExpire(expireOf(type), 'full') })}`
}

// 到期时间的比较值：北京时间格式按 +08:00 解析，解析不了的排最后。
const expireTimestamp = (raw: string): number => {
  const match = BEIJING_TIME_RE.exec(raw.trim())
  const ts = match
    ? Date.parse(`${match[1]}-${match[2]}-${match[3]}T${match[4]}:${match[5]}:${match[6] ?? '00'}+08:00`)
    : Date.parse(raw)
  return Number.isNaN(ts) ? Number.POSITIVE_INFINITY : ts
}

// 两种卡里最早的到期时间（每种卡的首张就是该种最早到期的）。
const earliestExpire = computed(() => {
  const expires = (['WEEK', 'FIVE_HOUR'] as ZhipuResetType[])
    .filter((type) => countOf(type) > 0 && expireOf(type) !== '')
    .map(expireOf)
  if (expires.length === 0) return ''
  const earliest = expires.reduce((a, b) => (expireTimestamp(b) < expireTimestamp(a) ? b : a))
  return formatExpire(earliest, 'short')
})

// 窗口用量明确为 0 时用卡没有意义：周卡看 weekly 窗口，5h 卡看 5h 窗口；窗口缺失时不拦。
const windowIdle = (type: ZhipuResetType): boolean => {
  const window = type === 'WEEK' ? 'weekly' : '5h'
  const tier = props.tiers?.find((item) => item.window === window)
  return tier != null && tier.used_percent === 0
}

const resetOptions = computed(() =>
  (['WEEK', 'FIVE_HOUR'] as ZhipuResetType[])
    .filter((type) => countOf(type) > 0)
    .map((type) => {
      const idle = windowIdle(type)
      return {
        type,
        idle,
        label:
          type === 'WEEK' ? t('admin.accounts.cnProviders.resetWeek') : t('admin.accounts.cnProviders.reset5h'),
        title: idle
          ? t('admin.accounts.cnProviders.noNeedReset')
          : t('admin.accounts.cnProviders.resetTooltip', {
              card: cardName(type),
              window: windowName(type),
              time: formatExpire(expireOf(type), 'full')
            })
      }
    })
)

const confirmTitle = computed(() =>
  t('admin.accounts.cnProviders.resetConfirmTitle', { card: cardName(pendingType.value) })
)

const confirmMessage = computed(() =>
  t('admin.accounts.cnProviders.resetConfirmMessage', {
    card: cardName(pendingType.value),
    count: countOf(pendingType.value),
    time: formatExpire(expireOf(pendingType.value), 'full'),
    window: windowName(pendingType.value),
    note: pendingType.value === 'WEEK' ? t('admin.accounts.cnProviders.resetConfirmWeekNote') : ''
  })
)

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

const openConfirm = (type: ZhipuResetType) => {
  if (resetting.value || countOf(type) <= 0 || windowIdle(type)) return
  pendingType.value = type
  showConfirm.value = true
}

const confirmReset = async () => {
  showConfirm.value = false
  if (resetting.value) return
  const type = pendingType.value
  const accountID = props.account.id
  resetting.value = true
  error.value = null
  warning.value = null
  message.value = null
  try {
    const result = await adminAPI.cnProviders.useResetCard(accountID, { reset_type: type })
    if (props.account.id !== accountID) return
    if (result.warning_code) {
      warning.value = t('admin.accounts.cnProviders.resetWarning')
    } else {
      message.value = t('admin.accounts.cnProviders.resetSuccess', {
        card: cardName(type),
        week: result.week_resets_left,
        fiveHour: result.five_hour_resets_left
      })
    }
    emit('used', result)
  } catch (e) {
    if (props.account.id !== accountID) return
    error.value = extractErrorMessage(e)
  } finally {
    if (props.account.id === accountID) resetting.value = false
  }
}
</script>
