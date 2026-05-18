<template>
  <section
    v-if="status?.enabled || (showDisabled && status && !status.enabled)"
    data-testid="profile-checkin-card"
    class="card border border-amber-100 bg-gradient-to-br from-amber-50 via-white to-orange-50/70 p-6 dark:border-amber-900/30 dark:from-amber-950/30 dark:via-dark-900 dark:to-dark-950"
  >
    <div
      v-if="status?.enabled"
      class="flex flex-col gap-4 md:flex-row md:items-center md:justify-between"
    >
      <div class="space-y-2">
        <div class="flex items-center gap-3">
          <div class="rounded-2xl bg-amber-100 p-3 text-amber-600 dark:bg-amber-900/40 dark:text-amber-300">
            <Icon name="gift" size="lg" />
          </div>
          <div>
            <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ tt('profile.checkIn.title', '每日签到') }}
            </h3>
            <p class="text-sm text-gray-500 dark:text-gray-400">
              {{ tt('profile.checkIn.description', '每天签到一次，领取账户余额奖励。') }}
            </p>
          </div>
        </div>

        <div class="grid gap-3 sm:grid-cols-3">
          <div class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700">
            <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
              {{ tt('profile.checkIn.reward', '今日奖励') }}
            </p>
            <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ formatCurrency(status.reward_amount) }}
            </p>
          </div>
          <div class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700">
            <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
              {{ tt('profile.checkIn.streak', '累计签到') }}
            </p>
            <p class="mt-1 text-lg font-semibold text-gray-900 dark:text-white">
              {{ tt('profile.checkIn.days', `${status.check_in_days} 天`, { count: status.check_in_days }) }}
            </p>
          </div>
          <div class="rounded-2xl bg-white/85 px-4 py-3 shadow-sm ring-1 ring-white/70 dark:bg-dark-900/60 dark:ring-dark-700">
            <p class="text-xs font-medium uppercase tracking-[0.16em] text-gray-400 dark:text-gray-500">
              {{ tt('profile.checkIn.lastCheckIn', '上次签到') }}
            </p>
            <p class="mt-1 text-sm font-semibold text-gray-900 dark:text-white">
              {{ lastCheckInLabel }}
            </p>
          </div>
        </div>
      </div>

      <div class="flex shrink-0 items-center gap-3">
        <span
          :class="[
            'inline-flex items-center rounded-full px-3 py-1 text-xs font-medium',
            status.checked_in_today
              ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300'
              : 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
          ]"
        >
          {{ status.checked_in_today ? tt('profile.checkIn.checkedInToday', '今日已签到') : tt('profile.checkIn.availableToday', '今日可签到') }}
        </span>
        <button
          type="button"
          class="btn btn-primary"
          :disabled="loading || status.checked_in_today"
          @click="handleCheckIn"
        >
          {{ loading ? t('common.processing') : (status.checked_in_today ? tt('profile.checkIn.checkedInButton', '已签到') : tt('profile.checkIn.action', '立即签到')) }}
        </button>
      </div>
    </div>

    <div v-else class="flex items-center gap-4">
      <div class="rounded-2xl bg-amber-100 p-3 text-amber-600 dark:bg-amber-900/40 dark:text-amber-300">
        <Icon name="gift" size="lg" />
      </div>
      <div>
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
          {{ tt('profile.checkIn.disabledTitle', '每日签到未开启') }}
        </h3>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ tt('profile.checkIn.disabledDescription', '管理员暂未开启每日签到功能。') }}
        </p>
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { userAPI, type DailyCheckInStatus } from '@/api/user'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'

defineProps<{
  showDisabled?: boolean
}>()

const emit = defineEmits<{
  checkedIn: []
}>()

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()

const status = ref<DailyCheckInStatus | null>(null)
const loading = ref(false)

const lastCheckInLabel = computed(() => {
  const raw = status.value?.last_check_in_at?.trim()
  if (!raw) {
    return tt('profile.checkIn.never', '从未签到')
  }

  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) {
    return tt('profile.checkIn.never', '从未签到')
  }

  return new Intl.DateTimeFormat(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
})

function formatCurrency(value: number): string {
  return `$${value.toFixed(2)}`
}

function tt(key: string, fallback: string, params?: Record<string, unknown>): string {
  const translated = params ? t(key, params) : t(key)
  return translated === key ? fallback : translated
}

async function loadStatus() {
  status.value = await userAPI.getDailyCheckInStatus()
}

async function handleCheckIn() {
  if (!status.value || status.value.checked_in_today || loading.value) {
    return
  }

  loading.value = true
  try {
    status.value = await userAPI.applyDailyCheckIn()
    await authStore.refreshUser()
    appStore.showSuccess(
      tt('profile.checkIn.success', `签到成功，已发放奖励：${formatCurrency(status.value.reward_amount)}`, {
        amount: formatCurrency(status.value.reward_amount),
      }),
    )
    emit('checkedIn')
  } catch (error: any) {
    appStore.showError(
      error?.response?.data?.detail ||
      error?.response?.data?.message ||
      error?.message ||
      tt('profile.checkIn.failed', '签到失败，请稍后重试'),
    )
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadStatus().catch((error) => {
    console.error('Failed to load daily check-in status:', error)
  })
})
</script>
