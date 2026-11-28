<template>
  <div class="space-y-1">
    <!--
      Same action row layout as OpenAIQuotaResetCell: the parent's local
      "查询" button is passed in via #pre-actions so related buttons share one
      row. The reset count only shows for Anthropic OAuth accounts; the slot always
      renders. The count button is read-only; only the orange reset button,
      after a query shows a redeemable credit and the operator confirms,
      consumes one reset.
    -->
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />

      <button
        v-if="visible"
        type="button"
        data-testid="claude-reset-count"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-blue-600 transition-colors hover:bg-blue-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-blue-400 dark:hover:bg-blue-900/30"
        :disabled="loading || redeeming"
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
        {{ t('admin.accounts.claudeResetCredits.count') }}<template v-if="status">
          <span v-if="kindGroups.length === 0" class="ml-0.5 tabular-nums">0</span>
          <span
            v-for="(group, index) in kindGroups"
            :key="group.kind"
            :data-testid="`claude-reset-kind-${group.kind}`"
            class="ml-0.5 whitespace-nowrap tabular-nums"
          >{{ index > 0 ? '· ' : '' }}{{ kindLabel(group.kind) }} {{ group.held }}</span>
        </template>
      </button>

      <button
        v-if="visible"
        type="button"
        data-testid="claude-reset-redeem"
        class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
        :disabled="redeeming || loading || !canRedeem"
        :title="redeemButtonTitle"
        @click="openRedeemConfirm"
      >
        <svg
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': redeeming }"
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
        {{ t('admin.accounts.claudeResetCredits.reset') }}
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
      <template v-if="status.eligible">
        <span
          v-for="group in unusableGroups"
          :key="group.kind"
          data-testid="claude-reset-not-usable"
          class="text-[10px] text-gray-500 dark:text-gray-400"
        >
          {{ unusableText(group) }}
        </span>
      </template>
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
    <div
      v-if="visible && redeemFeedback"
      data-testid="claude-reset-feedback"
      :role="redeemFeedback.kind === 'success' ? 'status' : 'alert'"
      class="text-[10px]"
      :class="feedbackClass"
      :title="redeemFeedback.text"
    >
      {{ redeemFeedback.text }}
    </div>

    <ConfirmDialog
      v-if="visible"
      :show="showRedeemConfirm"
      :title="t('admin.accounts.claudeResetCredits.confirmTitle')"
      :message="confirmMessage"
      :confirm-text="t('admin.accounts.claudeResetCredits.reset')"
      :cancel-text="t('common.cancel')"
      danger
      @confirm="confirmRedeem"
      @cancel="showRedeemConfirm = false"
    >
      <div class="space-y-2">
        <div class="flex flex-wrap gap-2" role="radiogroup" :aria-label="t('admin.accounts.claudeResetCredits.chooseKind')">
          <button
            v-for="group in kindGroups"
            :key="group.kind"
            type="button"
            role="radio"
            :aria-checked="selectedKind === group.kind"
            :data-testid="`claude-reset-pick-${group.kind}`"
            :disabled="!group.redeemable"
            :title="group.redeemable ? '' : unusableText(group)"
            class="rounded-md border px-3 py-1.5 text-sm transition-colors disabled:cursor-not-allowed disabled:opacity-50"
            :class="selectedKind === group.kind
              ? 'border-orange-500 bg-orange-50 text-orange-700 dark:border-orange-400 dark:bg-orange-900/30 dark:text-orange-300'
              : 'border-gray-300 text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700'"
            @click="selectKind(group.kind)"
          >
            {{ kindLabel(group.kind) }} · {{ group.held }}
          </button>
        </div>
        <p v-if="selectedKind === 'fable'" data-testid="claude-reset-fable-note" class="text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.claudeResetCredits.fableNote') }}
        </p>
        <p v-if="kindRequired" data-testid="claude-reset-kind-required" role="alert" class="text-xs text-red-600 dark:text-red-400">
          {{ t('admin.accounts.claudeResetCredits.chooseKindRequired') }}
        </p>
      </div>
    </ConfirmDialog>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Account } from '@/types'
import {
  getClaudeResetCredits,
  redeemClaudeResetCredit,
  type ClaudeResetCredit,
  type ClaudeResetCredits,
  type ClaudeResetKind,
  type ClaudeResetOutcome
} from '@/api/admin/claudeResetCredits'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{ account: Account }>()
// Fired after a redemption attempt so the parent can refresh the usage row.
const emit = defineEmits<{ redeemed: [outcome: ClaudeResetOutcome] }>()
const { t } = useI18n()
const status = ref<ClaudeResetCredits | null>(null)
const loading = ref(false)
const error = ref(false)
const redeeming = ref(false)
const showRedeemConfirm = ref(false)
const selectedKind = ref<ClaudeResetKind | null>(null)
const kindRequired = ref(false)
const redeemFeedback = ref<{ kind: 'success' | 'warning' | 'error'; text: string } | null>(null)
// One key per operator confirmation. It survives a failed request so retrying the
// same confirmation replays server-side instead of claiming a second reset. Choosing
// another kind is a new confirmation.
let pendingKey: string | null = null
let pendingKind: ClaudeResetKind | null = null
let generation = 0

const visible = computed(() => props.account.platform === 'anthropic' && props.account.type === 'oauth')

watch(() => [props.account.id, props.account.platform, props.account.type], () => {
  generation++
  status.value = null
  loading.value = false
  error.value = false
  redeeming.value = false
  showRedeemConfirm.value = false
  selectedKind.value = null
  kindRequired.value = false
  redeemFeedback.value = null
  pendingKey = null
  pendingKind = null
})

// 与 OpenAIQuotaResetCell 的到期时间格式保持一致
const formatTime = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const options: Intl.DateTimeFormatOptions = { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }
  if (style === 'full') options.year = 'numeric'
  return new Intl.DateTimeFormat(undefined, options).format(date)
}

// 与 Codex 的「次数」一致：显示持有的剩余次数，而不是此刻可兑换的次数；按类型分开，
// 因为 Fable 重置只退还 Fable 用掉的周额度，和周重置不能合成一个数
const kindOrder: ClaudeResetKind[] = ['session', 'weekly', 'full', 'fable']
interface KindGroup { kind: ClaudeResetKind; held: number; redeemable: boolean; nothingToReset: boolean; credits: ClaudeResetCredit[] }
const kindGroups = computed<KindGroup[]>(() => {
  const credits = status.value?.credits ?? []
  return kindOrder.flatMap(kind => {
    const own = credits.filter(c => c.kind === kind)
    if (own.length === 0) return []
    return [{
      kind,
      held: own.reduce((sum, c) => sum + c.resets_left, 0),
      redeemable: own.some(c => c.redeemable),
      nothingToReset: own.every(c => c.nothing_to_reset),
      credits: own
    }]
  })
})
const unusableGroups = computed(() => kindGroups.value.filter(g => !g.redeemable))

function kindLabel(kind: ClaudeResetKind): string {
  return t(`admin.accounts.claudeResetCredits.kinds.${kind}`)
}

function unusableText(group: KindGroup): string {
  const key = group.nothingToReset ? 'nothingToReset' : 'kindNotUsableNow'
  return t(`admin.accounts.claudeResetCredits.${key}`, { kind: kindLabel(group.kind) })
}

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
  if (credit.clears.length) lines.push(t('admin.accounts.claudeResetCredits.clears', { windows: windowLabels(credit.clears) }))
  if (credit.use_requires_limit) lines.push(t('admin.accounts.claudeResetCredits.requiresLimit'))
  return lines.join('\n')
})

// 已知窗口显示为可读名称，未知窗口原样显示
const windowKeys: Record<string, string> = {
  five_hour: 'fiveHour',
  seven_day: 'sevenDay',
  seven_day_overage_included: 'sevenDayOverage'
}
function windowLabels(windows?: string[]): string {
  return (windows ?? []).map(w => windowKeys[w] ? t(`admin.accounts.claudeResetCredits.windows.${windowKeys[w]}`) : w).join(', ')
}

const countButtonTitle = computed(() => {
  if (!status.value) return t('admin.accounts.claudeResetCredits.countTooltipLoad')
  return [
    t('admin.accounts.claudeResetCredits.countTooltipRefresh'),
    t('admin.accounts.claudeResetCredits.fetched', { time: formatTime(status.value.fetched_at, 'full') })
  ].join('\n')
})

const canRedeem = computed(() => kindGroups.value.some(g => g.redeemable))

const redeemButtonTitle = computed(() => {
  if (!status.value) return t('admin.accounts.claudeResetCredits.resetTooltipNeedQuery')
  if (!canRedeem.value) return t('admin.accounts.claudeResetCredits.resetTooltipNone')
  return t('admin.accounts.claudeResetCredits.resetTooltipReady')
})

const confirmMessage = computed(() => {
  const group = kindGroups.value.find(g => g.kind === selectedKind.value)
  const next = group?.credits.find(c => c.redeemable)
  if (!group || !next) return t('admin.accounts.claudeResetCredits.chooseKind')
  return t('admin.accounts.claudeResetCredits.confirmMessage', {
    kind: kindLabel(group.kind),
    windows: windowLabels(next?.clears) || '—',
    count: Math.max(group.held - 1, 0)
  })
})

const feedbackClass = computed(() => {
  switch (redeemFeedback.value?.kind) {
    case 'success': return 'text-emerald-600 dark:text-emerald-400'
    case 'warning': return 'text-amber-600 dark:text-amber-400'
    default: return 'text-red-600 dark:text-red-400'
  }
})

function newOperationKey(accountID: number): string {
  const id = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  return `claude-reset-${accountID}-${id}`
}

function openRedeemConfirm() {
  if (redeeming.value || loading.value || !canRedeem.value) return
  const usable = kindGroups.value.filter(g => g.redeemable)
  // Preselect only when there is no choice to make; otherwise the operator picks.
  if (!usable.some(g => g.kind === selectedKind.value)) selectedKind.value = usable.length === 1 ? usable[0].kind : null
  kindRequired.value = false
  showRedeemConfirm.value = true
}

function selectKind(kind: ClaudeResetKind) {
  if (!kindGroups.value.some(g => g.kind === kind && g.redeemable)) return
  selectedKind.value = kind
  kindRequired.value = false
}

function outcomeFeedback(result: ClaudeResetOutcome): { kind: 'success' | 'warning' | 'error'; text: string } {
  const key = 'admin.accounts.claudeResetCredits.outcome'
  switch (result.outcome) {
    case 'reset':
      return { kind: 'success', text: t(`${key}.reset`, { windows: windowLabels(result.cleared) || '—' }) }
    case 'already_used':
      return { kind: 'warning', text: t(`${key}.alreadyUsed`) }
    case 'cooldown':
      return {
        kind: 'warning',
        text: result.cooldown_until
          ? t(`${key}.cooldownUntil`, { time: formatTime(result.cooldown_until, 'short') })
          : t(`${key}.cooldown`)
      }
    case 'not_limited':
      return { kind: 'warning', text: t(`${key}.notLimited`) }
    case 'ineligible':
      return { kind: 'error', text: t(`${key}.ineligible`) }
    default:
      if (result.reason === 'upstream_unavailable') return { kind: 'warning', text: t(`${key}.unavailable`) }
      return { kind: 'warning', text: t(`${key}.unknown`) }
  }
}

// Refusals issued before any claim was sent: the next attempt is a new confirmation.
const preClaimRefusals = new Set(['CLAUDE_RESET_BUSY', 'CLAUDE_RESET_NOT_AVAILABLE', 'CLAUDE_RESET_UNRESOLVED', 'CLAUDE_RESET_UPSTREAM_UNAVAILABLE'])

function errorText(e: unknown): string {
  const err = e as { reason?: string; message?: string }
  switch (err?.reason) {
    case 'CLAUDE_RESET_UNRESOLVED': return t('admin.accounts.claudeResetCredits.outcome.unknown')
    case 'CLAUDE_RESET_BUSY': return t('admin.accounts.claudeResetCredits.outcome.busy')
    case 'CLAUDE_RESET_NOT_AVAILABLE': return t('admin.accounts.claudeResetCredits.outcome.notAvailable')
    case 'CLAUDE_RESET_UPSTREAM_UNAVAILABLE': return t('admin.accounts.claudeResetCredits.outcome.unavailable')
    case 'IDEMPOTENCY_IN_PROGRESS': return t('admin.accounts.claudeResetCredits.outcome.inProgress')
    case 'IDEMPOTENCY_RETRY_BACKOFF': return t('admin.accounts.claudeResetCredits.outcome.retryBackoff')
    default: return err?.message || t('admin.accounts.claudeResetCredits.outcome.failed')
  }
}

async function confirmRedeem() {
  const kind = selectedKind.value
  if (!kind) {
    kindRequired.value = true
    return
  }
  showRedeemConfirm.value = false
  if (redeeming.value || loading.value || !canRedeem.value) return
  const accountID = props.account.id
  const current = generation
  if (pendingKind !== kind) pendingKey = null
  pendingKey ??= newOperationKey(accountID)
  pendingKind = kind
  redeeming.value = true
  redeemFeedback.value = null
  try {
    const result = await redeemClaudeResetCredit(accountID, pendingKey, kind)
    if (current !== generation) return
    // A definite server answer ends this confirmation; the next one gets a new key.
    pendingKey = null
    redeemFeedback.value = outcomeFeedback(result)
    redeeming.value = false
    emit('redeemed', result)
    await refresh()
  } catch (e) {
    if (current !== generation) return
    // Transport or unknown errors keep the key so a retry replays server-side.
    if (preClaimRefusals.has((e as { reason?: string })?.reason ?? '')) pendingKey = null
    redeemFeedback.value = { kind: 'error', text: errorText(e) }
  } finally {
    if (current === generation) redeeming.value = false
  }
}

async function refresh() {
  if (loading.value || redeeming.value) return
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
