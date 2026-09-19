<template>
  <section class="card flex flex-wrap items-center justify-between gap-4 p-4">
    <div class="min-w-0">
      <h2 class="text-sm font-medium text-gray-900 dark:text-white">{{ t('upstreamBilling.usageSummary', { month: billingMonth }) }}</h2>
      <p v-if="loading" class="mt-1 text-sm text-gray-500">{{ t('common.loading') }}</p>
      <p v-else-if="failed" class="mt-1 text-sm text-gray-500">{{ t('upstreamBilling.summaryFailed') }}</p>
      <div v-else-if="totals.length" class="mt-2 flex flex-wrap gap-x-5 gap-y-2">
        <span v-for="total in totals" :key="total.currency" class="font-semibold tabular-nums text-primary-600 dark:text-primary-400">{{ total.currency }} {{ formatBillingAmount(total.allocated_cost) }}</span>
      </div>
      <p v-else class="mt-1 text-sm text-gray-500">{{ t('upstreamBilling.summaryUnavailable') }}</p>
      <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('upstreamBilling.usageSummaryHint') }}</p>
      <p v-if="!loading && !failed" class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t(adminScope ? 'upstreamBilling.summaryAdminScope' : 'upstreamBilling.summaryUserScope') }}</p>
    </div>
    <router-link :to="{ path: '/upstream-billing', query: { month: billingMonth } }" class="btn btn-secondary btn-sm">{{ t('upstreamBilling.openReport') }}</router-link>
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { upstreamBillingAPI, type UpstreamBillingTotal } from '@/api/upstreamBilling'
import { currentBillingMonth, formatBillingAmount, isBillingMonth } from '@/utils/upstreamBilling'

const props = defineProps<{ month?: string }>()
const { t } = useI18n()
const billingMonth = computed(() => props.month && isBillingMonth(props.month) ? props.month : currentBillingMonth())
const totals = ref<UpstreamBillingTotal[]>([])
const loading = ref(false)
const failed = ref(false)
const adminScope = ref(false)
let controller: AbortController | undefined

watch(billingMonth, async value => {
  controller?.abort()
  const request = new AbortController()
  controller = request
  totals.value = []
  loading.value = true
  failed.value = false
  try {
    const report = await upstreamBillingAPI.summary(value, request.signal)
    if (!request.signal.aborted) {
      totals.value = report.totals || []
      adminScope.value = report.is_admin === true
    }
  } catch {
    if (!request.signal.aborted) failed.value = true
  } finally {
    if (!request.signal.aborted) loading.value = false
  }
}, { immediate: true })
onBeforeUnmount(() => controller?.abort())
</script>
