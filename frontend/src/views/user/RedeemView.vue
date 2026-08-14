<template>
  <AppLayout>
    <div class="mx-auto max-w-2xl space-y-6">
      <!--
        Was a 160px ultramarine gradient hero with a rounded icon tile and a
        4xl white price. Two numbers is two Metrics: label, then the value in
        mono tabular figures. The type scale is the hierarchy.
      -->
      <Surface>
        <div class="grid gap-6 sm:grid-cols-2">
          <Metric
            :label="t('redeem.currentBalance')"
            :value="user?.balance ?? null"
            :precision="2"
            unit="USD"
          />
          <Metric
            :label="t('redeem.concurrency')"
            :value="user?.concurrency ?? null"
            :unit="t('redeem.requests')"
          />
        </div>
      </Surface>

      <Surface>
        <form @submit.prevent="handleRedeem">
          <!--
            Every outcome of this form — the empty-input complaint, the server's
            rejection, and the success line — lands in the field's own message
            row, which reserves its line box. Nothing below the button moves,
            and no result card grows the form by 40px after a submit.
          -->
          <FormField
            id="redeem-code"
            :label="t('redeem.redeemCodeLabel')"
            :hint="t('redeem.redeemCodeHint')"
            :error="errorMessage"
          >
            <template #default="{ describedBy, invalid }">
              <div class="flex items-start gap-2">
                <input
                  id="redeem-code"
                  v-model="redeemCode"
                  type="text"
                  autocomplete="off"
                  spellcheck="false"
                  class="input font-mono"
                  :class="invalid && 'input-error'"
                  :aria-describedby="describedBy"
                  :aria-invalid="invalid || undefined"
                  :placeholder="t('redeem.redeemCodePlaceholder')"
                  :disabled="submitting"
                  data-testid="redeem-code-input"
                />
                <Button
                  type="submit"
                  tone="accent"
                  variant="solid"
                  size="md"
                  class="h-9 shrink-0"
                  :loading="submitting"
                  data-testid="redeem-submit"
                >
                  {{ t('redeem.redeemButton') }}
                </Button>
              </div>
            </template>

            <template #message>
              <span v-if="errorMessage">{{ errorMessage }}</span>
              <span v-else-if="successMessage" class="text-success">{{ successMessage }}</span>
              <span v-else>{{ t('redeem.redeemCodeHint') }}</span>
            </template>
          </FormField>
        </form>
      </Surface>

      <Surface :title="t('redeem.aboutCodes')" flush>
        <ul class="divide-y divide-line-subtle">
          <li class="px-4 py-2 text-xs text-ink-secondary">{{ t('redeem.codeRule1') }}</li>
          <li class="px-4 py-2 text-xs text-ink-secondary">{{ t('redeem.codeRule2') }}</li>
          <li class="px-4 py-2 text-xs text-ink-secondary">
            {{ t('redeem.codeRule3') }}
            <span v-if="contactInfo" class="code ml-1">{{ contactInfo }}</span>
          </li>
          <li class="px-4 py-2 text-xs text-ink-secondary">{{ t('redeem.codeRule4') }}</li>
        </ul>
      </Surface>

      <Surface :title="t('redeem.recentActivity')" flush>
        <div v-if="loadingHistory" class="space-y-3 p-4" data-testid="redeem-history-loading">
          <div class="skeleton h-3 w-full"></div>
          <div class="skeleton h-3 w-4/5"></div>
          <div class="skeleton h-3 w-2/3"></div>
        </div>

        <div v-else-if="history.length > 0" class="overflow-x-auto">
          <table class="table min-w-[36rem]" data-testid="redeem-history-table">
            <thead>
              <tr>
                <th scope="col">{{ t('usage.type') }}</th>
                <th scope="col">{{ t('redeem.redeemCodeLabel') }}</th>
                <th scope="col" class="is-numeric">{{ t('payment.orders.amount') }}</th>
                <th scope="col" class="is-numeric">{{ t('keyUsage.date') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in history" :key="item.id">
                <td class="min-w-0">
                  <span class="text-ink">{{ getHistoryItemTitle(item) }}</span>
                  <span
                    v-if="historyCaption(item)"
                    class="ml-1 text-2xs text-ink-tertiary"
                    :title="item.notes"
                  >
                    {{ historyCaption(item) }}
                  </span>
                </td>
                <td class="whitespace-nowrap font-mono text-xs text-ink-tertiary">
                  <template v-if="isAdminAdjustment(item.type)">
                    {{ t('redeem.adminAdjustment') }}
                  </template>
                  <template v-else>{{ item.code.slice(0, 8) }}…</template>
                </td>
                <td class="is-numeric">
                  <!--
                    Sign is the channel, not colour: a deduction is not an error,
                    and painting every credit green leaves nothing to say when
                    something is actually wrong.
                  -->
                  <NumCell
                    :value="historyAmount(item).value"
                    :precision="historyAmount(item).precision"
                    :unit="historyAmount(item).unit"
                  />
                </td>
                <td class="is-numeric whitespace-nowrap">{{ formatDateTime(item.used_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <p v-else class="px-4 py-8 text-center text-xs text-ink-tertiary">
          {{ t('redeem.historyWillAppear') }}
        </p>
      </Surface>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'

import { redeemAPI, authAPI, type RedeemHistoryItem } from '@/api'
import Button from '@/components/common/Button.vue'
import FormField from '@/components/common/FormField.vue'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useSubscriptionStore } from '@/stores/subscriptions'
import { formatDateTime } from '@/utils/format'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()
const subscriptionStore = useSubscriptionStore()

const user = computed(() => authStore.user)

const redeemCode = ref('')
const submitting = ref(false)
const redeemResult = ref<{
  message: string
  type: string
  value: number
  new_balance?: number
  new_concurrency?: number
  group_name?: string
  validity_days?: number
} | null>(null)
const errorMessage = ref('')

// History data
const history = ref<RedeemHistoryItem[]>([])
const loadingHistory = ref(false)
const contactInfo = ref('')

// Helper functions for history display
const isBalanceType = (type: string) => {
  return type === 'balance' || type === 'admin_balance'
}

const isSubscriptionType = (type: string) => {
  return type === 'subscription'
}

const isAdminAdjustment = (type: string) => {
  return type === 'admin_balance' || type === 'admin_concurrency'
}

const getHistoryItemTitle = (item: RedeemHistoryItem) => {
  if (item.type === 'balance') {
    return t('redeem.balanceAddedRedeem')
  } else if (item.type === 'admin_balance') {
    return item.value >= 0 ? t('redeem.balanceAddedAdmin') : t('redeem.balanceDeductedAdmin')
  } else if (item.type === 'concurrency') {
    return t('redeem.concurrencyAddedRedeem')
  } else if (item.type === 'admin_concurrency') {
    return item.value >= 0 ? t('redeem.concurrencyAddedAdmin') : t('redeem.concurrencyReducedAdmin')
  } else if (item.type === 'subscription') {
    return t('redeem.subscriptionAssigned')
  }
  return t('common.unknown')
}

/** The group a subscription grant landed in, or an admin's note. */
const historyCaption = (item: RedeemHistoryItem): string => {
  if (isSubscriptionType(item.type) && item.group?.name) return item.group.name
  return item.notes || ''
}

interface HistoryAmount {
  value: number | null
  unit: string
  precision?: number
}

/**
 * Quantities, not pre-formatted strings. `NumCell` owns the locale formatting,
 * the tabular alignment and the null-vs-zero distinction, and the unit sits a
 * step down so it never competes with the digits.
 */
const historyAmount = (item: RedeemHistoryItem): HistoryAmount => {
  if (isBalanceType(item.type)) {
    return { value: item.value, unit: 'USD', precision: 2 }
  }
  if (isSubscriptionType(item.type)) {
    return { value: item.validity_days ?? Math.round(item.value), unit: t('redeem.days').trim() }
  }
  return { value: item.value, unit: t('redeem.requests') }
}

/**
 * The success line. It replaces a bordered emerald card carrying five rows of
 * text — the balance and concurrency Metrics at the top of this page are
 * refreshed by the same submit, so restating them below the form was telling
 * the user something the page had already shown them.
 */
const successMessage = computed(() => {
  const result = redeemResult.value
  if (!result) return ''

  if (result.type === 'balance') {
    return `${t('redeem.redeemSuccess')} ${t('redeem.added')}: $${result.value.toFixed(2)}`
  }
  if (result.type === 'concurrency') {
    return `${t('redeem.redeemSuccess')} ${t('redeem.added')}: ${result.value} ${t('redeem.concurrentRequests')}`
  }
  if (result.type === 'subscription') {
    const group = result.group_name ? ` – ${result.group_name}` : ''
    const days = result.validity_days
      ? ` (${t('redeem.subscriptionDays', { days: result.validity_days })})`
      : ''
    return `${t('redeem.subscriptionAssigned')}${group}${days}`
  }
  return result.message || t('redeem.redeemSuccess')
})

const fetchHistory = async () => {
  loadingHistory.value = true
  try {
    history.value = await redeemAPI.getHistory()
  } catch (error) {
    console.error('Failed to fetch history:', error)
  } finally {
    loadingHistory.value = false
  }
}

const handleRedeem = async () => {
  redeemResult.value = null

  /*
   * This used to reach the user only as a toast, which is the one place a
   * field-level complaint must never live: it is transient, it is nowhere near
   * the input, and it leaves the field itself looking valid. It is now the
   * field's error, wired to `aria-invalid` and `aria-describedby` by FormField.
   */
  if (!redeemCode.value.trim()) {
    errorMessage.value = t('redeem.pleaseEnterCode')
    return
  }

  submitting.value = true
  errorMessage.value = ''

  try {
    const result = await redeemAPI.redeem(redeemCode.value.trim())

    redeemResult.value = result

    // Refresh user data to get updated balance/concurrency
    await authStore.refreshUser()

    // If subscription type, immediately refresh subscription status
    if (result.type === 'subscription') {
      try {
        await subscriptionStore.fetchActiveSubscriptions(true) // force refresh
      } catch (error) {
        console.error('Failed to refresh subscriptions after redeem:', error)
        appStore.showWarning(t('redeem.subscriptionRefreshFailed'))
      }
    }

    // Clear the input
    redeemCode.value = ''

    // Refresh history
    await fetchHistory()

    // Show success toast
    appStore.showSuccess(t('redeem.codeRedeemSuccess'))
  } catch (error: any) {
    errorMessage.value = error.response?.data?.detail || t('redeem.failedToRedeem')

    appStore.showError(t('redeem.redeemFailed'))
  } finally {
    submitting.value = false
  }
}

onMounted(async () => {
  fetchHistory()
  try {
    const settings = await authAPI.getPublicSettings()
    contactInfo.value = settings.contact_info || ''
  } catch (error) {
    console.error('Failed to load contact info:', error)
  }
})
</script>
