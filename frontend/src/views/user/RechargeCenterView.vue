<template>
  <AppLayout>
    <main class="mx-auto max-w-5xl space-y-6">
      <header class="border-b border-gray-200 pb-4 dark:border-dark-700">
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('payment.rechargeCenter.title') }}</h1>
      </header>

      <section class="rounded-lg border border-amber-300 bg-amber-50 p-5 text-amber-950 dark:border-amber-600/50 dark:bg-amber-950/30 dark:text-amber-100" aria-labelledby="recharge-guide-title">
        <h2 id="recharge-guide-title" class="flex items-center gap-2 text-lg font-semibold">
          <Icon name="exclamationCircle" size="md" />
          {{ t('payment.rechargeCenter.guide') }}
        </h2>
        <div class="mt-3 space-y-2 text-sm leading-6">
          <p>{{ t('payment.rechargeCenter.guidePurchase') }}</p>
          <p>{{ t('payment.rechargeCenter.guideAfterPayment') }}</p>
        </div>
        <div v-if="safeHelpImage" class="mt-4">
          <img :src="safeHelpImage" :alt="t('payment.rechargeCenter.guide')" class="max-h-72 max-w-full rounded-md object-contain" />
        </div>
        <div v-if="guideHtml" class="markdown-body mt-3" v-html="guideHtml"></div>
      </section>

      <p v-if="loadError" role="alert" class="text-sm text-red-600 dark:text-red-400">{{ t('payment.rechargeCenter.configError') }}</p>

      <p v-if="loading" role="status" class="text-sm text-gray-500">{{ t('common.loading') }}</p>
      <section v-else class="grid gap-4 md:grid-cols-2" :aria-label="t('payment.rechargeCenter.title')">
        <article v-for="store in stores" :key="store.key" class="flex min-h-48 flex-col border-b border-gray-200 p-5 dark:border-dark-700">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t(store.title) }}</h2>
          <a v-if="safeUrl(store.url)" :href="safeUrl(store.url)" target="_blank" rel="noopener noreferrer" class="btn mt-5 inline-flex w-full items-center justify-center gap-2" :class="store.key === 'primary' ? 'btn-primary' : 'btn-secondary'">
            {{ t('payment.rechargeCenter.openStore') }} <Icon name="externalLink" size="sm" />
          </a>
          <p v-else class="mt-5 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.rechargeCenter.unavailable') }}</p>
        </article>
      </section>

      <section class="border-t border-gray-200 pt-5 dark:border-dark-700">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.rechargeCenter.orderLookup') }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.rechargeCenter.orderHint') }}</p>
        <div class="mt-4 grid gap-4 sm:grid-cols-2">
          <div v-for="store in orderStores" :key="store.key">
            <h3 class="text-sm font-medium text-gray-800 dark:text-gray-200">{{ t(store.title) }}</h3>
            <a v-if="safeUrl(store.url)" :href="safeUrl(store.url)" target="_blank" rel="noopener noreferrer" class="btn btn-secondary mt-2 inline-flex items-center gap-2">
              {{ t(store.title) }} <Icon name="externalLink" size="sm" />
            </a>
            <p v-else class="mt-2 text-sm text-gray-500 dark:text-gray-400">{{ t('payment.rechargeCenter.unavailable') }}</p>
          </div>
        </div>
      </section>
    </main>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import DOMPurify from 'dompurify'
import { marked } from 'marked'
import '@/styles/announcement-markdown.css'
import { paymentAPI } from '@/api/payment'
import type { PaymentConfig } from '@/types/payment'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const config = ref<PaymentConfig | null>(null)
const loading = ref(true)
const loadError = ref(false)
const stores = computed(() => [
  { key: 'primary', title: 'payment.rechargeCenter.primary', url: config.value?.recharge_primary_url },
  { key: 'backup', title: 'payment.rechargeCenter.backup', url: config.value?.recharge_backup_url },
])
const orderStores = computed(() => [
  { key: 'primary', title: 'payment.rechargeCenter.primaryOrderLookup', url: config.value?.recharge_primary_order_url },
  { key: 'backup', title: 'payment.rechargeCenter.backupOrderLookup', url: config.value?.recharge_backup_order_url },
])
const guideHtml = computed(() => DOMPurify.sanitize(marked.parse(config.value?.help_text || '', { async: false, gfm: true }) as string))
const safeHelpImage = computed(() => sanitizeUrl(config.value?.help_image_url || '', { allowDataUrl: true }))
function safeUrl(value?: string) {
  const sanitized = sanitizeUrl(value || '')
  if (!sanitized) return ''
  const parsed = new URL(sanitized)
  return parsed.username || parsed.password ? '' : sanitized
}

onMounted(async () => {
  try {
    config.value = (await paymentAPI.getConfig()).data
  } catch {
    loadError.value = true
  } finally {
    loading.value = false
  }
})
</script>
