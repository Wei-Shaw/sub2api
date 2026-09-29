<template>
  <div class="space-y-1">
    <!--
      Same action row layout as OpenAIQuotaResetCell: the parent's local
      "查询" button is passed in via #pre-actions so related buttons share one
      row. The reset count only shows for Anthropic OAuth accounts; the slot always
      renders. This cell is read-only — it never redeems a reset.
    -->
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />

      <button
        v-if="visible"
        type="button"
        data-testid="claude-reset-count"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading"
        :title="countButtonTitle"
        @click="refresh"
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
        {{ t('admin.accounts.claudeResetCredits.count') }}<span v-if="status" class="ml-0.5 tabular-nums">{{ totalResets }}</span>
      </button>
    </div>

    <div v-if="visible && status && (primaryCredit || !status.eligible || cooldownActive)" class="flex flex-wrap items-center gap-1">
      <span
        v-if="primaryCredit?.expires_at"
        data-testid="claude-reset-expiry"
        class="inline-flex max-w-full items-center rounded bg-gray-100 px-1.5 py-0.5 text-[10px] leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
        :title="creditTitle"
      >
        {{ t('admin.accounts.claudeResetCredits.expiresAt', { time: formatTime(primaryCredit.expires_at, 'short') }) }}
      </span>
      <span
        v-if="status.eligible && totalResets > 0 && status.available_count === 0"
        data-testid="claude-reset-not-usable"
        class="text-[10px] text-gray-500 dark:text-gray-400"
      >
        {{ t('admin.accounts.claudeResetCredits.notUsableNow') }}
      </span>
      <span v-if="!status.eligible" class="text-[10px] text-amber-600 dark:text-amber-400">
        {{ t('admin.accounts.claudeResetCredits.ineligible') }}
      </span>
      <span v-if="cooldownActive" data-testid="claude-reset-cooldown" class="text-[10px] text-amber-600 dark:text-amber-400">
        {{ t('admin.accounts.claudeResetCredits.cooldown', { time: formatTime(status.cooldown_until!, 'short') }) }}
      </span>
    </div>

    <div v-if="visible && error" role="alert" class="text-[10px] text-red-600 dark:text-red-400">
      {{ t('admin.accounts.claudeResetCredits.error') }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import { getClaudeResetCredits, type ClaudeResetCredits } from '@/api/admin/claudeResetCredits'

const props = defineProps<{ account: Account }>()
const { t } = useI18n()
const status = ref<ClaudeResetCredits | null>(null)
const loading = ref(false)
const error = ref(false)
let generation = 0

const visible = computed(() => props.account.platform === 'anthropic' && props.account.type === 'oauth')

watch(() => [props.account.id, props.account.platform, props.account.type], () => { generation++; status.value = null; loading.value = false; error.value = false })

// 与 OpenAIQuotaResetCell 的到期时间格式保持一致
const formatTime = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const options: Intl.DateTimeFormatOptions = { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }
  if (style === 'full') options.year = 'numeric'
  return new Intl.DateTimeFormat(undefined, options).format(date)
}

// 与 Codex 的「次数」一致：显示持有的剩余次数，而不是此刻可兑换的次数
const totalResets = computed(() => (status.value?.credits ?? []).reduce((sum, c) => sum + c.resets_left, 0))

// 优先展示下一张会被使用的券（仅它可能 redeemable），否则取最早到期的一张
const primaryCredit = computed(() => {
  const credits = status.value?.credits ?? []
  return credits.find(c => c.redeemable) ?? [...credits].sort((a, b) => expiryMs(a.expires_at) - expiryMs(b.expires_at))[0] ?? null
})

// 缺失或无法解析的到期时间排在最后
function expiryMs(value?: string): number {
  const ms = value ? new Date(value).getTime() : NaN
  return Number.isNaN(ms) ? Number.POSITIVE_INFINITY : ms
}

// 仅在冷却时间仍在未来时提示（后端已清理过期值，这里防御性再判断一次）
const cooldownActive = computed(() => {
  const until = status.value?.cooldown_until
  return !!until && new Date(until).getTime() > Date.now()
})

const creditTitle = computed(() => {
  const credit = primaryCredit.value
  if (!credit) return ''
  const lines = [credit.label]
  if (credit.expires_at) lines.push(t('admin.accounts.claudeResetCredits.expiresAtFull', { time: formatTime(credit.expires_at, 'full') }))
  if (credit.clears.length) lines.push(t('admin.accounts.claudeResetCredits.clears', { windows: credit.clears.join(', ') }))
  if (credit.use_requires_limit) lines.push(t('admin.accounts.claudeResetCredits.requiresLimit'))
  return lines.join('\n')
})

const countButtonTitle = computed(() => {
  if (!status.value) return t('admin.accounts.claudeResetCredits.countTooltipLoad')
  return [
    t('admin.accounts.claudeResetCredits.countTooltipRefresh'),
    t('admin.accounts.claudeResetCredits.fetched', { time: formatTime(status.value.fetched_at, 'full') })
  ].join('\n')
})

async function refresh() {
  if (loading.value) return
  const current = ++generation
  loading.value = true
  error.value = false
  try {
    const result = await getClaudeResetCredits(props.account.id)
    if (current === generation) status.value = result
  } catch {
    if (current === generation) { error.value = true; status.value = null }
  } finally {
    if (current === generation) loading.value = false
  }
}
</script>
