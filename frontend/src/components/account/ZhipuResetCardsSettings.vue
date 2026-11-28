<template>
  <section v-if="compact" class="contents" data-testid="zhipu-reset-compact">
    <UsageQueryButton :loading="querying" :disabled="busy" :title="t(`${prefix}.countTooltip`)" data-testid="zhipu-reset-query" @click="refresh">
      {{ t(`${prefix}.listCount`) }}<span v-if="hasFreshCount" class="ml-0.5 tabular-nums">{{ cardCount }}</span>
    </UsageQueryButton>
    <button
      type="button"
      class="inline-flex items-center gap-0.5 rounded px-1.5 py-0.5 text-[10px] font-medium text-orange-600 transition-colors hover:bg-orange-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-orange-400 dark:hover:bg-orange-900/30"
      :disabled="busy || !canReset"
      :title="resetButtonTitle"
      data-testid="zhipu-reset-action"
      @click="openResetConfirm"
    >
      <svg aria-hidden="true" class="h-2.5 w-2.5" :class="{ 'animate-spin': busy && !querying }" fill="none" stroke="currentColor" viewBox="0 0 24 24">
        <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M20 12a8 8 0 11-2.343-5.657L20 8m0 0V4m0 4h-4" />
      </svg>
      {{ t(`${prefix}.listReset`) }}
    </button>
    <p v-if="state?.status === 'unbound'" class="basis-full text-[10px] text-amber-600 dark:text-amber-400">{{ t(`${prefix}.bindInEdit`) }}</p>
    <p v-if="feedback" role="status" class="basis-full text-[10px] text-emerald-600 dark:text-emerald-400">{{ t(`${prefix}.${feedback}`) }}</p>
    <p v-if="error" role="alert" class="basis-full text-[10px] text-red-600 dark:text-red-400">{{ t(`${prefix}.${error}`) }}</p>
  </section>
  <section v-else class="space-y-3 border-t border-gray-200 pt-4 dark:border-dark-600" data-testid="zhipu-reset-cards">
    <h3 class="text-sm font-semibold">{{ t(`${prefix}.title`) }}</h3>
    <p class="text-xs text-gray-500 dark:text-gray-400">{{ t(`${prefix}.hint`) }}</p>

    <div v-if="device" class="space-y-2 text-sm" aria-live="polite">
      <a :href="device.authorize_url" target="_blank" rel="noopener noreferrer" class="text-primary-600 hover:underline">
        {{ t(`${prefix}.openAuthorization`) }}
      </a>
      <p>{{ t(`${prefix}.pending`) }}</p>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="unbind">
        {{ t('common.cancel') }}
      </button>
    </div>
    <div v-else class="flex flex-wrap gap-2">
      <button v-if="state?.status === 'unbound'" type="button" class="btn btn-primary btn-sm"
        :disabled="busy" data-testid="zhipu-reset-auth" @click="start">
        {{ t(`${prefix}.authorize`) }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="busy" @click="refresh">
        {{ t(`${prefix}.query`) }}
      </button>
      <button v-if="state && state.status !== 'unbound'" type="button" class="btn btn-secondary btn-sm"
        :disabled="busy" @click="unbind">
        {{ t(`${prefix}.unbind`) }}
      </button>
      <button v-if="state?.status === 'bound'" type="button" class="btn btn-secondary btn-sm"
        :disabled="busy || needsRefresh || !state.cards || !!nextTryAt" @click="confirm = 'claim'">
        {{ t(`${prefix}.claim`) }}
      </button>
    </div>

    <div v-if="state?.cards" class="space-y-2">
      <div v-for="window in windows" :key="window.type" class="flex items-start justify-between gap-3 rounded bg-gray-50 p-2 text-sm dark:bg-dark-700">
        <div class="min-w-0">
          <p class="font-medium">{{ t(`${prefix}.${window.type}`) }} · {{ t(`${prefix}.count`, { count: window.cards.length }) }}</p>
          <p v-for="(card, i) in window.cards" :key="i" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t(`${prefix}.expires`, { time: format(card.expire_at) }) }}
          </p>
          <p v-if="window.history" class="text-xs text-gray-500 dark:text-gray-400">
            {{ t(`${prefix}.lastUsed`, { time: format(window.history.used_at) }) }}
          </p>
        </div>
        <button type="button" class="btn btn-secondary btn-sm shrink-0" :disabled="busy || needsRefresh || !window.cards.length"
          :data-testid="`zhipu-reset-use-${window.type}`" @click="confirm = window.type">
          {{ t(`${prefix}.use`) }}
        </button>
      </div>
    </div>
    <p v-if="nextTryAt" class="text-xs text-gray-500 dark:text-gray-400">
      {{ t(`${prefix}.nextTry`, { time: format(nextTryAt) }) }}
    </p>
    <p v-if="feedback" role="status" class="text-sm">{{ t(`${prefix}.${feedback}`) }}</p>
    <p v-if="error" role="alert" class="text-xs text-red-600 dark:text-red-400">{{ t(`${prefix}.${error}`) }}</p>

  </section>
  <ConfirmDialog
    v-if="confirm"
    :show="true"
    :title="t(`${prefix}.title`)"
    :message="confirm === 'claim' ? t(`${prefix}.confirmClaim`) : t(`${prefix}.confirmUse`, { window: t(`${prefix}.${confirm}`) })"
    :confirm-text="compact ? t(`${prefix}.listReset`) : t('common.confirm')"
    :cancel-text="t('common.cancel')"
    :danger="confirm !== 'claim'"
    @confirm="submit"
    @cancel="confirm = undefined"
  >
    <fieldset v-if="compact && resettableWindows.length > 1" class="space-y-2">
      <legend class="mb-1 text-sm font-medium">{{ t(`${prefix}.chooseWindow`) }}</legend>
      <label v-for="window in resettableWindows" :key="window.type" class="flex items-center gap-2 text-sm">
        <input
          v-model="confirm"
          type="radio"
          :name="`zhipu-reset-window-${accountId}`"
          :value="window.type"
          :data-testid="`zhipu-reset-window-${window.type}`"
        />
        {{ t(`${prefix}.${window.type}`) }} · {{ t(`${prefix}.count`, { count: window.cards.length }) }}
      </label>
    </fieldset>
  </ConfirmDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import UsageQueryButton from './UsageQueryButton.vue'
import { queryQuota } from '@/api/admin/cnProviders'
import {
  actZhipuResetCard, getZhipuResetCards, pollZhipuResetAuthorization,
  startZhipuResetAuthorization, unbindZhipuResetAuthorization
} from '@/api/admin/zhipuResetCards'
import type { ZhipuResetDevicePrompt, ZhipuResetState, ZhipuResetWindow } from '@/api/admin/zhipuResetCards'

const props = defineProps<{ accountId: number; compact?: boolean }>()
const emit = defineEmits<{ redeemed: []; 'binding-changed': [bound: boolean] }>()
const prefix = 'admin.accounts.zhipuResetCards'
const { t, locale } = useI18n()
const state = ref<ZhipuResetState>()
const device = ref<ZhipuResetDevicePrompt>()
const error = ref('')
const feedback = ref('')
const busy = ref(false)
const querying = ref(false)
const needsRefresh = ref(false)
const nextTryAt = ref<number>()
const confirm = ref<'claim' | ZhipuResetWindow>()
const controller = new AbortController()
let pollTimer: ReturnType<typeof setTimeout> | undefined

const windows = computed(() => state.value?.cards ? ([
  { type: 'FIVE_HOUR', cards: state.value.cards.available_five_hour_resets.filter(card => card.expire_at > Date.now()), history: state.value.cards.latest_five_hour_reset_history },
  { type: 'WEEK', cards: state.value.cards.available_week_resets.filter(card => card.expire_at > Date.now()), history: state.value.cards.latest_week_reset_history }
] as const) : [])
const cardCount = computed(() => windows.value.reduce((sum, window) => sum + window.cards.length, 0))
const hasFreshCount = computed(() => !!state.value?.cards && !needsRefresh.value)
const resettableWindows = computed(() => windows.value.filter(window => window.cards.length > 0))
const canReset = computed(() => hasFreshCount.value && cardCount.value > 0)
const resetButtonTitle = computed(() => t(`${prefix}.${!hasFreshCount.value
  ? 'resetTooltipNeedQuery' : canReset.value ? 'resetTooltipReady' : 'resetTooltipNone'}`))

function openResetConfirm() {
  if (busy.value || !canReset.value) return
  const now = Date.now()
  confirm.value = resettableWindows.value.find(window => window.cards.some(card => card.expire_at > now))?.type
}

function format(unix: number) {
  return new Date(unix).toLocaleString(locale.value)
}

function onError(e: unknown, fallback = 'failed') {
  const reason = (e as { reason?: string })?.reason
  error.value = reason === 'ZHIPU_RESET_AUTH_REQUIRED' ? 'expired'
    : reason === 'ZHIPU_DEVICE_EXPIRED' ? 'deviceExpired'
      : reason === 'ZHIPU_RESET_UNAVAILABLE' ? 'unavailable' : fallback
}

async function refresh() {
  if (busy.value) return
  busy.value = true
  querying.value = true
  error.value = ''
  try {
    state.value = await getZhipuResetCards(props.accountId, controller.signal)
    needsRefresh.value = false
    if (nextTryAt.value && nextTryAt.value <= Date.now()) nextTryAt.value = undefined
    if (state.value.status === 'unauthorized') error.value = 'expired'
  } catch (e) {
    if (!controller.signal.aborted) onError(e)
  } finally {
    querying.value = false
    busy.value = false
  }
}

async function start() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const result = await startZhipuResetAuthorization(props.accountId, controller.signal)
    if (!controller.signal.aborted) {
      device.value = result
      schedulePoll(result)
    }
  } catch (e) {
    if (!controller.signal.aborted) onError(e)
  } finally {
    busy.value = false
  }
}

function schedulePoll(prompt: ZhipuResetDevicePrompt, failures = 0) {
  pollTimer = setTimeout(async () => {
    if (controller.signal.aborted || device.value?.attempt_id !== prompt.attempt_id) return
    if (Date.now() >= Date.parse(prompt.expires_at)) {
      device.value = undefined
      error.value = 'deviceExpired'
      return
    }
    try {
      const result = await pollZhipuResetAuthorization(props.accountId, prompt.attempt_id, controller.signal)
      if (controller.signal.aborted || device.value?.attempt_id !== prompt.attempt_id) return
      if (result.status === 'pending') {
        schedulePoll(prompt)
        return
      }
      device.value = undefined
      if (result.status === 'bound' && result.state) {
        state.value = result.state
        emit('binding-changed', true)
      }
      else error.value = result.status === 'expired' ? 'deviceExpired' : 'failed'
    } catch (e) {
      if (controller.signal.aborted || device.value?.attempt_id !== prompt.attempt_id) return
      const status = (e as { status?: number })?.status
      if (failures < 2 && (status === 429 || status === 502 || status === 503)) {
        schedulePoll(prompt, failures + 1)
      } else {
        device.value = undefined
        onError(e)
      }
    }
  }, Math.max(2, prompt.poll_interval) * 1000 * (2 ** failures))
}

async function unbind() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  feedback.value = ''
  try {
    await unbindZhipuResetAuthorization(props.accountId, controller.signal)
    clearTimeout(pollTimer)
    device.value = undefined
    state.value = { status: 'unbound' }
    nextTryAt.value = undefined
    emit('binding-changed', false)
  } catch (e) {
    if (!controller.signal.aborted) onError(e)
  } finally {
    busy.value = false
  }
}

async function submit() {
  const action = confirm.value
  const binding = state.value?.binding_id
  confirm.value = undefined
  if (!action || !binding || busy.value || needsRefresh.value) return
  if (action !== 'claim' && !windows.value.find(window => window.type === action)?.cards.some(card => card.expire_at > Date.now())) return
  busy.value = true
  error.value = ''
  feedback.value = ''
  try {
    const key = globalThis.crypto?.randomUUID?.() ??
      Array.from(globalThis.crypto.getRandomValues(new Uint8Array(16)), b => b.toString(16).padStart(2, '0')).join('')
    const result = await actZhipuResetCard(props.accountId, binding, action, key)
    if (controller.signal.aborted) return
    feedback.value = result.outcome
    nextTryAt.value = result.next_try_at
    if (result.cards) {
      state.value = { status: 'bound', binding_id: binding, cards: result.cards }
    } else if (result.outcome !== 'not_eligible') {
      needsRefresh.value = true
      error.value = 'refreshRequired'
    }
    if (result.outcome === 'used') {
      // 额度查询失败不改变上游已核销的结果。
      if (props.compact) emit('redeemed')
      else void queryQuota(props.accountId).catch(() => undefined)
    }
  } catch (e) {
    if (controller.signal.aborted) return
    needsRefresh.value = true
    onError(e, 'actionUnknown')
  } finally {
    busy.value = false
  }
}

onMounted(() => { if (!props.compact) void refresh() })
onBeforeUnmount(() => {
  controller.abort()
  clearTimeout(pollTimer)
})
</script>
