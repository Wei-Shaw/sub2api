<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="space-y-6" data-testid="affiliate-loading">
        <div class="grid gap-px border border-line bg-line-subtle sm:grid-cols-2 lg:grid-cols-4">
          <div v-for="i in 4" :key="i" class="space-y-2 bg-surface p-4">
            <div class="skeleton h-3 w-24"></div>
            <div class="skeleton h-7 w-20"></div>
          </div>
        </div>
        <div class="rounded border border-line bg-surface p-4">
          <div class="space-y-3">
            <div class="skeleton h-3 w-32"></div>
            <div class="skeleton h-9 w-full"></div>
          </div>
        </div>
      </div>

      <template v-else-if="detail">
        <!--
          Headline numbers. Every one of these was a proportionally-spaced
          `toLocaleString`/`formatCurrency` string in a coloured `text-2xl` —
          emerald for quota, primary for the rate — so a column of figures
          neither aligned nor meant anything by its colour. `NumCell` gives all
          seven mono tabular figures, and none of them is green: a healthy
          balance is not a status.
        -->
        <section
          class="grid gap-px border border-line bg-line-subtle sm:grid-cols-2 lg:grid-cols-4"
          data-testid="affiliate-stats"
        >
          <div class="bg-surface p-4">
            <Metric
              :label="t('affiliate.stats.rebateRate')"
              :value="rebateRatePercent"
              :precision="rebateRatePrecision"
              unit="%"
              :caption="t('affiliate.stats.rebateRateHint')"
            />
          </div>
          <div class="bg-surface p-4">
            <Metric :label="t('affiliate.stats.invitedUsers')" :value="detail.aff_count" />
          </div>
          <div class="bg-surface p-4">
            <Metric
              :label="t('affiliate.stats.availableQuota')"
              :value="detail.aff_quota"
              :precision="2"
              unit="USD"
            />
          </div>
          <div class="bg-surface p-4">
            <Metric
              :label="t('affiliate.stats.totalQuota')"
              :value="detail.aff_history_quota"
              :precision="2"
              unit="USD"
            />
          </div>
        </section>

        <Surface :title="t('affiliate.title')" :description="t('affiliate.description')">
          <div class="grid gap-4 md:grid-cols-2">
            <div class="space-y-1.5">
              <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                {{ t('affiliate.yourCode') }}
              </p>
              <div
                class="flex flex-col items-stretch gap-2 border border-line bg-surface-sunken px-3 py-2 sm:flex-row sm:items-center"
              >
                <code
                  class="min-w-0 break-all font-mono text-sm text-ink sm:flex-1 sm:truncate"
                >{{ detail.aff_code }}</code>
                <!--
                  The label never changes on press: confirmation is an icon swap
                  inside a fixed box, so the control keeps its width. The live
                  region sits outside the button so the accessible name stays
                  stable too.
                -->
                <Button
                  size="md"
                  class="h-9 w-full sm:w-auto sm:shrink-0"
                  :aria-label="t('affiliate.copyCode')"
                  @click="copyCode"
                >
                  <template #icon>
                    <Icon :name="copiedTarget === 'code' ? 'check' : 'copy'" size="xs" />
                  </template>
                  {{ t('affiliate.copyCode') }}
                </Button>
              </div>
              <p class="sr-only" role="status">
                {{ copiedTarget === 'code' ? t('common.copied') : '' }}
              </p>
            </div>

            <div class="space-y-1.5">
              <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
                {{ t('affiliate.inviteLink') }}
              </p>
              <div
                class="flex flex-col items-stretch gap-2 border border-line bg-surface-sunken px-3 py-2 sm:flex-row sm:items-center"
              >
                <code
                  class="min-w-0 break-all font-mono text-sm text-ink-secondary sm:flex-1 sm:truncate"
                >{{ inviteLink }}</code>
                <Button
                  size="md"
                  class="h-9 w-full sm:w-auto sm:shrink-0"
                  :aria-label="t('affiliate.copyLink')"
                  @click="copyInviteLink"
                >
                  <template #icon>
                    <Icon :name="copiedTarget === 'link' ? 'check' : 'copy'" size="xs" />
                  </template>
                  {{ t('affiliate.copyLink') }}
                </Button>
              </div>
              <p class="sr-only" role="status">
                {{ copiedTarget === 'link' ? t('common.copied') : '' }}
              </p>
            </div>
          </div>

          <!--
            Was a tinted primary panel. These are instructions, not a status,
            so they get a rule and ordinary ink.
          -->
          <div class="mt-4 border-t border-line pt-4">
            <p class="text-2xs font-medium uppercase tracking-[0.04em] text-ink-tertiary">
              {{ t('affiliate.tips.title') }}
            </p>
            <ol class="mt-2 space-y-1 text-sm text-ink-secondary">
              <li>1. {{ t('affiliate.tips.line1') }}</li>
              <li>2. {{ t('affiliate.tips.line2', { rate: `${formattedRebateRate}%` }) }}</li>
              <li>3. {{ t('affiliate.tips.line3') }}</li>
              <li v-if="detail.aff_frozen_quota > 0">4. {{ t('affiliate.tips.line4') }}</li>
            </ol>
          </div>

          <!-- Frozen quota is the one figure here that IS a state worth marking. -->
          <div
            v-if="detail.aff_frozen_quota > 0"
            class="mt-4 flex items-baseline justify-between gap-4 border-t border-line pt-3"
          >
            <span class="text-xs text-ink-secondary">{{ t('affiliate.stats.frozenQuota') }}</span>
            <NumCell :value="detail.aff_frozen_quota" :precision="2" unit="USD" tone="warn" />
          </div>
        </Surface>

        <Surface :title="t('affiliate.transfer.title')" :description="t('affiliate.transfer.description')">
          <template #actions>
            <Button
              tone="accent"
              variant="solid"
              size="md"
              :loading="transferring"
              :disabled="detail.aff_quota <= 0"
              data-testid="affiliate-transfer"
              @click="transferQuota"
            >
              {{ t('affiliate.transfer.button') }}
            </Button>
          </template>
          <p v-if="detail.aff_quota <= 0" class="text-xs text-ink-tertiary">
            {{ t('affiliate.transfer.empty') }}
          </p>
          <div v-else class="flex items-baseline justify-between gap-4">
            <span class="text-xs text-ink-secondary">{{ t('affiliate.stats.availableQuota') }}</span>
            <NumCell :value="detail.aff_quota" :precision="2" unit="USD" />
          </div>
        </Surface>

        <Surface :title="t('affiliate.invitees.title')" flush>
          <p
            v-if="detail.invitees.length === 0"
            class="px-4 py-8 text-center text-xs text-ink-tertiary"
          >
            {{ t('affiliate.invitees.empty') }}
          </p>
          <div v-else class="overflow-x-auto">
            <table class="table min-w-[36rem]" data-testid="affiliate-invitees-table">
              <thead>
                <tr>
                  <th scope="col">{{ t('affiliate.invitees.columns.email') }}</th>
                  <th scope="col">{{ t('affiliate.invitees.columns.username') }}</th>
                  <th scope="col" class="is-numeric">
                    {{ t('affiliate.invitees.columns.rebate') }}
                  </th>
                  <th scope="col">{{ t('affiliate.invitees.columns.joinedAt') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="item in detail.invitees" :key="item.user_id">
                  <th scope="row" class="text-left text-xs font-normal text-ink">
                    {{ item.email || '–' }}
                  </th>
                  <td>{{ item.username || '–' }}</td>
                  <td class="is-numeric">
                    <NumCell :value="item.total_rebate" :precision="2" />
                  </td>
                  <td class="whitespace-nowrap font-mono text-xs tabular-nums">
                    {{ formatDateTime(item.created_at) || '–' }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </Surface>
      </template>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Button from '@/components/common/Button.vue'
import Metric from '@/components/common/Metric.vue'
import NumCell from '@/components/common/NumCell.vue'
import Surface from '@/components/common/Surface.vue'
import Icon from '@/components/icons/Icon.vue'
import userAPI from '@/api/user'
import type { UserAffiliateDetail } from '@/types'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useClipboard } from '@/composables/useClipboard'
import { formatCurrency, formatDateTime } from '@/utils/format'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const { copyToClipboard } = useClipboard()

const loading = ref(true)
const transferring = ref(false)
const detail = ref<UserAffiliateDetail | null>(null)

/** Which block was copied last, so the icon can confirm it without a reflow. */
const copiedTarget = ref<'code' | 'link' | null>(null)
let copiedResetTimer: ReturnType<typeof setTimeout> | null = null

function markCopied(target: 'code' | 'link'): void {
  copiedTarget.value = target
  if (copiedResetTimer) clearTimeout(copiedResetTimer)
  copiedResetTimer = setTimeout(() => {
    copiedTarget.value = null
    copiedResetTimer = null
  }, 2000)
}

const inviteLink = computed(() => {
  if (!detail.value) return ''
  if (typeof window === 'undefined') return `/register?aff=${encodeURIComponent(detail.value.aff_code)}`
  return `${window.location.origin}/register?aff=${encodeURIComponent(detail.value.aff_code)}`
})

// Rebate rate is a percentage in the range [0, 100]; backend already clamps it.
const rebateRatePercent = computed(() => {
  const v = detail.value?.effective_rebate_rate_percent ?? 0
  return Math.round(v * 100) / 100
})

/*
 * Trailing zeros are noise on a rate that is usually whole (20%, not 20.00%),
 * so an integer renders with no decimals. `Metric` still formats through
 * `Intl.NumberFormat`, which is what the old hand-rolled `String(rounded)` was
 * skipping — it printed "1234.5" where a zh reader expects "1,234.5".
 */
const rebateRatePrecision = computed(() => (Number.isInteger(rebateRatePercent.value) ? 0 : 2))

/** Only used inside the interpolated tips sentence, where a unit is spelled out. */
const formattedRebateRate = computed(() => String(rebateRatePercent.value))

async function loadAffiliateDetail(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
  }
  try {
    detail.value = await userAPI.getAffiliateDetail()
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

async function copyCode(): Promise<void> {
  if (!detail.value?.aff_code) return
  if (await copyToClipboard(detail.value.aff_code, t('affiliate.codeCopied'))) {
    markCopied('code')
  }
}

async function copyInviteLink(): Promise<void> {
  if (!inviteLink.value) return
  if (await copyToClipboard(inviteLink.value, t('affiliate.linkCopied'))) {
    markCopied('link')
  }
}

async function transferQuota(): Promise<void> {
  if (!detail.value || detail.value.aff_quota <= 0 || transferring.value) return
  transferring.value = true
  try {
    const resp = await userAPI.transferAffiliateQuota()
    appStore.showSuccess(t('affiliate.transfer.success', { amount: formatCurrency(resp.transferred_quota) }))
    await Promise.all([
      loadAffiliateDetail(true),
      authStore.refreshUser().catch(() => undefined),
    ])
  } catch (error) {
    appStore.showError(extractApiErrorMessage(error, t('affiliate.transferFailed')))
  } finally {
    transferring.value = false
  }
}

onMounted(() => {
  void loadAffiliateDetail()
})

onUnmounted(() => {
  if (copiedResetTimer) {
    clearTimeout(copiedResetTimer)
    copiedResetTimer = null
  }
})
</script>
