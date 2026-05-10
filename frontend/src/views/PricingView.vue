<template>
  <div class="relative flex min-h-screen flex-col bg-gray-50 dark:bg-dark-950">
    <PublicPageHeader active-path="/pricing" />

    <main class="mx-auto w-full max-w-7xl flex-1 px-4 py-6 sm:px-6 sm:py-8">
      <div class="space-y-6">
        <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white sm:text-2xl">{{ t('pricing.title') }}</h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('pricing.description') }}</p>
          </div>
          <button @click="refresh" :disabled="loading"
            class="inline-flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 disabled:opacity-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700">
            <svg class="h-4 w-4" :class="{ 'animate-spin': loading }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5">
              <path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0l3.181 3.183a8.25 8.25 0 0013.803-3.7M4.031 9.865a8.25 8.25 0 0113.803-3.7l3.181 3.182m0-4.991v4.99" />
            </svg>
            {{ t('common.refresh') }}
          </button>
        </div>

        <div v-for="group in data?.groups" :key="group.id" class="rounded-xl border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900 overflow-hidden">
          <div class="flex items-center gap-3 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
            <span class="text-lg font-semibold text-gray-900 dark:text-white">{{ group.name }}</span>
            <span v-if="group.platform" class="rounded-full bg-blue-50 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-950 dark:text-blue-300">{{ group.platform }}</span>
            <span v-if="group.rate_multiplier !== 1" class="rounded-full bg-amber-50 px-2.5 py-0.5 text-xs font-medium text-amber-700 dark:bg-amber-950 dark:text-amber-300">×{{ group.rate_multiplier.toFixed(2) }}</span>
            <span class="rounded-full bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-800 dark:text-dark-300">
              {{ group.models.length }} {{ group.models.length === 1 ? t('pricing.model') : t('pricing.models') }}
            </span>
          </div>
          <div v-if="group.models.length > 0">
            <div class="hidden sm:block overflow-x-auto">
              <table class="w-full" style="table-layout: fixed;">
                <colgroup>
                  <col style="width: 36%;">
                  <col style="width: 16%;">
                  <col style="width: 16%;">
                  <col style="width: 16%;">
                  <col style="width: 16%;">
                </colgroup>
                <thead>
                  <tr class="border-b border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                    <th class="px-6 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('pricing.modelName') }}</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('pricing.inputPrice') }} ($)</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-dark-400">{{ t('pricing.outputPrice') }} ($)</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-blue-600 dark:text-blue-400">{{ t('pricing.effectiveInput') }} ($)</th>
                    <th class="px-4 py-3 text-right text-xs font-medium uppercase tracking-wider text-blue-600 dark:text-blue-400">{{ t('pricing.effectiveOutput') }} ($)</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="m in group.models" :key="m.model_name" class="border-b border-gray-50 last:border-b-0 dark:border-dark-800">
                    <td class="px-6 py-3 text-sm font-medium text-gray-900 dark:text-white font-mono truncate" :title="m.model_name">{{ m.model_name }}</td>
                    <td class="px-4 py-3 text-right text-sm text-gray-600 dark:text-dark-300 tabular-nums">{{ formatPrice(m.input_cost_per_million) }}</td>
                    <td class="px-4 py-3 text-right text-sm text-gray-600 dark:text-dark-300 tabular-nums">{{ formatPrice(m.output_cost_per_million) }}</td>
                    <td class="px-4 py-3 text-right text-sm font-medium tabular-nums" :class="group.rate_multiplier !== 1 ? 'text-blue-600 dark:text-blue-400' : 'text-gray-700 dark:text-dark-200'">{{ formatPrice(m.effective_input) }}</td>
                    <td class="px-4 py-3 text-right text-sm font-medium tabular-nums" :class="group.rate_multiplier !== 1 ? 'text-blue-600 dark:text-blue-400' : 'text-gray-700 dark:text-dark-200'">{{ formatPrice(m.effective_output) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <div class="sm:hidden divide-y divide-gray-100 dark:divide-dark-700">
              <div v-for="m in group.models" :key="m.model_name" class="px-4 py-3 space-y-2">
                <p class="text-sm font-medium text-gray-900 dark:text-white font-mono break-all">{{ m.model_name }}</p>
                <div class="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
                  <div class="flex justify-between"><span class="text-gray-500 dark:text-dark-400">{{ t('pricing.inputPrice') }}</span><span class="tabular-nums text-gray-700 dark:text-dark-200">{{ formatPrice(m.input_cost_per_million) }}</span></div>
                  <div class="flex justify-between"><span class="text-gray-500 dark:text-dark-400">{{ t('pricing.outputPrice') }}</span><span class="tabular-nums text-gray-700 dark:text-dark-200">{{ formatPrice(m.output_cost_per_million) }}</span></div>
                  <div class="flex justify-between"><span class="text-blue-600 dark:text-blue-400">{{ t('pricing.effectiveInput') }}</span><span class="tabular-nums font-medium" :class="group.rate_multiplier !== 1 ? 'text-blue-600 dark:text-blue-400' : 'text-gray-700 dark:text-dark-200'">{{ formatPrice(m.effective_input) }}</span></div>
                  <div class="flex justify-between"><span class="text-blue-600 dark:text-blue-400">{{ t('pricing.effectiveOutput') }}</span><span class="tabular-nums font-medium" :class="group.rate_multiplier !== 1 ? 'text-blue-600 dark:text-blue-400' : 'text-gray-700 dark:text-dark-200'">{{ formatPrice(m.effective_output) }}</span></div>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="py-12 text-center">
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('pricing.noModelsInGroup') }}</p>
          </div>
        </div>

        <div v-if="data && (!data.groups || data.groups.length === 0)" class="rounded-xl border border-gray-200 bg-white py-16 text-center shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('pricing.noData') }}</p>
        </div>

        <div v-if="error" class="rounded-xl border border-red-200 bg-red-50 px-6 py-4 text-center dark:border-red-800 dark:bg-red-950/30">
          <p class="text-sm text-red-600 dark:text-red-400">{{ error }}</p>
        </div>
      </div>
    </main>

    <PublicPageFooter />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import PublicPageHeader from '@/components/common/PublicPageHeader.vue'
import PublicPageFooter from '@/components/common/PublicPageFooter.vue'
import { getPublicPricing, type PublicPricingResponse } from '@/api/pricing'

const { t } = useI18n()

const data = ref<PublicPricingResponse | null>(null)
const loading = ref(false)
const error = ref('')

async function refresh() {
  loading.value = true
  error.value = ''
  try {
    data.value = await getPublicPricing()
  } catch (e) {
    data.value = null
    error.value = (e as Error)?.message || String(e)
  } finally {
    loading.value = false
  }
}

function formatPrice(value: number): string {
  if (!value || value === 0) return '-'
  if (value >= 1) return '$' + value.toFixed(2)
  if (value >= 0.01) return '$' + value.toFixed(4)
  return '$' + value.toFixed(6)
}

onMounted(refresh)
</script>
