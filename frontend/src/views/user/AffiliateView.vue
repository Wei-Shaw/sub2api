<template>
  <AppLayout>
    <div class="space-y-6">
      <div v-if="loading" class="flex justify-center py-12">
        <div class="spinner h-8 w-8 text-accent"></div>
      </div>

      <template v-else-if="detail">
        <!-- 返利读数：可转出额度为当前读数 -->
        <div class="meter">
          <div class="meter-cell">
            <p class="meter-label">{{ t('affiliate.stats.rebateRate') }}</p>
            <p class="meter-value">{{ formattedRebateRate }}<span class="ml-0.5 text-label font-medium text-fg-muted">%</span></p>
            <p class="meter-sub">{{ t('affiliate.stats.rebateRateHint') }}</p>
          </div>
          <div class="meter-cell">
            <p class="meter-label">{{ t('affiliate.stats.invitedUsers') }}</p>
            <p class="meter-value">{{ formatCount(detail.aff_count) }}</p>
          </div>
          <div class="meter-cell meter-cell-current">
            <p class="meter-label">{{ t('affiliate.stats.availableQuota') }}</p>
            <p class="meter-value">{{ formatCurrency(detail.aff_quota) }}</p>
          </div>
          <div class="meter-cell">
            <p class="meter-label">{{ t('affiliate.stats.totalQuota') }}</p>
            <p class="meter-value">{{ formatCurrency(detail.aff_history_quota) }}</p>
            <p v-if="detail.aff_frozen_quota > 0" class="meter-sub !text-warning">
              {{ t('affiliate.stats.frozenQuota') }}: {{ formatCurrency(detail.aff_frozen_quota) }}
            </p>
          </div>
        </div>

        <section class="card">
          <div class="card-header">
            <h3 class="card-title">{{ t('affiliate.title') }}</h3>
            <p class="mt-0.5 text-body text-fg-muted">{{ t('affiliate.description') }}</p>
          </div>

          <div class="divide-y divide-border">
            <div class="flex flex-col gap-1.5 px-5 py-3 sm:flex-row sm:items-center sm:gap-4">
              <p class="text-meta font-medium text-fg-muted sm:w-32 sm:shrink-0">{{ t('affiliate.yourCode') }}</p>
              <div class="flex min-w-0 flex-1 flex-col items-stretch gap-2 sm:flex-row sm:items-center">
                <code class="min-w-0 break-all font-mono text-body font-semibold text-fg sm:flex-1 sm:truncate">{{ detail.aff_code }}</code>
                <button class="btn btn-secondary btn-sm w-full sm:w-auto sm:shrink-0" @click="copyCode">
                  <Icon name="copy" size="sm" />
                  <span>{{ t('affiliate.copyCode') }}</span>
                </button>
              </div>
            </div>

            <div class="flex flex-col gap-1.5 px-5 py-3 sm:flex-row sm:items-center sm:gap-4">
              <p class="text-meta font-medium text-fg-muted sm:w-32 sm:shrink-0">{{ t('affiliate.inviteLink') }}</p>
              <div class="flex min-w-0 flex-1 flex-col items-stretch gap-2 sm:flex-row sm:items-center">
                <code class="min-w-0 break-all font-mono text-meta text-fg sm:flex-1 sm:truncate">{{ inviteLink }}</code>
                <button class="btn btn-secondary btn-sm w-full sm:w-auto sm:shrink-0" @click="copyInviteLink">
                  <Icon name="copy" size="sm" />
                  <span>{{ t('affiliate.copyLink') }}</span>
                </button>
              </div>
            </div>

            <div class="px-5 py-3">
              <p class="text-label font-semibold text-fg">{{ t('affiliate.tips.title') }}</p>
              <ol class="mt-1.5 space-y-1 text-body text-fg-muted">
                <li>1. {{ t('affiliate.tips.line1') }}</li>
                <li>2. {{ t('affiliate.tips.line2', { rate: `${formattedRebateRate}%` }) }}</li>
                <li>3. {{ t('affiliate.tips.line3') }}</li>
                <li v-if="detail.aff_frozen_quota > 0">4. {{ t('affiliate.tips.line4') }}</li>
              </ol>
            </div>
          </div>
        </section>

        <section class="card">
          <div class="card-body flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="card-title">{{ t('affiliate.transfer.title') }}</h3>
              <p class="mt-0.5 text-body text-fg-muted">{{ t('affiliate.transfer.description') }}</p>
            </div>
            <button
              class="btn btn-primary"
              :disabled="transferring || detail.aff_quota <= 0"
              @click="transferQuota"
            >
              <Icon v-if="transferring" name="refresh" size="sm" class="animate-spin" />
              <Icon v-else name="dollar" size="sm" />
              <span>{{ transferring ? t('affiliate.transfer.transferring') : t('affiliate.transfer.button') }}</span>
            </button>
          </div>
          <p v-if="detail.aff_quota <= 0" class="card-footer text-body text-warning">
            {{ t('affiliate.transfer.empty') }}
          </p>
        </section>

        <section class="space-y-0">
          <h3 class="bill-section-title">{{ t('affiliate.invitees.title') }}</h3>
          <DataTable :columns="inviteeColumns" :data="detail.invitees" row-key="user_id">
            <template #cell-total_rebate="{ row }">
              <span class="font-semibold tabular-nums text-success">{{ formatCurrency(row.total_rebate) }}</span>
            </template>
            <template #empty>
              <p class="text-body text-fg-muted">{{ t('affiliate.invitees.empty') }}</p>
            </template>
          </DataTable>
        </section>
      </template>

      <EmptyState
        v-else-if="loadError"
        class="card"
        :title="t('affiliate.loadFailed')"
        :description="t('errors.tryAgain')"
        :action-text="t('common.refresh')"
        :action-icon="false"
        @action="loadAffiliateDetail()"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import type { Column } from '@/components/common/types'
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
const loadError = ref(false)
const transferring = ref(false)
const detail = ref<UserAffiliateDetail | null>(null)

const inviteLink = computed(() => {
  if (!detail.value) return ''
  if (typeof window === 'undefined') return `/register?aff=${encodeURIComponent(detail.value.aff_code)}`
  return `${window.location.origin}/register?aff=${encodeURIComponent(detail.value.aff_code)}`
})

// Rebate rate is a percentage in the range [0, 100]; backend already clamps it.
// We trim trailing zeros (e.g. 20.00 → "20", 12.50 → "12.5") for a cleaner UI.
const formattedRebateRate = computed(() => {
  const v = detail.value?.effective_rebate_rate_percent ?? 0
  const rounded = Math.round(v * 100) / 100
  return Number.isInteger(rounded) ? String(rounded) : rounded.toString()
})

const inviteeColumns = computed<Column[]>(() => [
  { key: 'email', label: t('affiliate.invitees.columns.email'), formatter: (v) => v || '-' },
  { key: 'username', label: t('affiliate.invitees.columns.username'), formatter: (v) => v || '-' },
  { key: 'total_rebate', label: t('affiliate.invitees.columns.rebate'), class: 'text-right' },
  { key: 'created_at', label: t('affiliate.invitees.columns.joinedAt'), formatter: (v) => formatDateTime(v) || '-' }
])

function formatCount(value: number): string {
  return value.toLocaleString()
}

async function loadAffiliateDetail(silent = false): Promise<void> {
  if (!silent) {
    loading.value = true
    loadError.value = false
  }
  try {
    detail.value = await userAPI.getAffiliateDetail()
  } catch (error) {
    if (!silent) loadError.value = true
    appStore.showError(extractApiErrorMessage(error, t('affiliate.loadFailed')))
  } finally {
    if (!silent) {
      loading.value = false
    }
  }
}

async function copyCode(): Promise<void> {
  if (!detail.value?.aff_code) return
  await copyToClipboard(detail.value.aff_code, t('affiliate.codeCopied'))
}

async function copyInviteLink(): Promise<void> {
  if (!inviteLink.value) return
  await copyToClipboard(inviteLink.value, t('affiliate.linkCopied'))
}

async function transferQuota(): Promise<void> {
  if (!detail.value || detail.value.aff_quota <= 0 || transferring.value) return
  transferring.value = true
  try {
    const resp = await userAPI.transferAffiliateQuota()
    appStore.showSuccess(t('affiliate.transfer.success', { amount: formatCurrency(resp.transferred_quota) }))
    await Promise.all([
      loadAffiliateDetail(true),
      authStore.refreshUser({ force: true }).catch(() => undefined),
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
</script>
