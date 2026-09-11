<template>
  <ProductShell>
    <div class="space-y-6">
      <section class="border-b border-gray-200 pb-6 dark:border-dark-800">
        <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-300">{{ t('product.pricing.eyebrow') }}</p>
        <h1 class="mt-2 text-3xl font-semibold tracking-tight text-gray-950 dark:text-white sm:text-4xl">{{ t('product.pricing.title') }}</h1>
        <p class="mt-2 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('product.pricing.description') }}</p>
      </section>

      <section class="grid gap-4 sm:grid-cols-3">
        <div v-for="stat in stats" :key="stat.label" class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-800 dark:bg-dark-900">
          <div class="flex items-center justify-between gap-3"><span class="text-sm text-gray-500 dark:text-dark-400">{{ stat.label }}</span><Icon :name="stat.icon" size="sm" class="text-primary-600 dark:text-primary-300" /></div>
          <p class="mt-3 text-2xl font-semibold tabular-nums text-gray-950 dark:text-white">{{ stat.value }}</p>
        </div>
      </section>

      <div v-if="loading" class="flex min-h-[300px] items-center justify-center rounded-2xl border border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900"><div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600"></div></div>
      <div v-else-if="error" class="rounded-2xl border border-red-200 bg-red-50 px-5 py-10 text-center text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300">{{ t('product.pricing.loadFailed') }}</div>
      <section v-else class="overflow-hidden rounded-2xl border border-gray-200 bg-white dark:border-dark-800 dark:bg-dark-900">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-gray-100 px-5 py-4 dark:border-dark-800 sm:px-6">
          <div><h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('product.pricing.catalogTitle') }}</h2><p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('product.pricing.catalogHint') }}</p></div>
          <button type="button" class="btn btn-secondary btn-sm" @click="loadPricing"><Icon name="refresh" size="sm" />{{ t('product.refresh') }}</button>
        </div>
        <ModelPlazaContent :response="data" :loading="false" embedded />
      </section>
    </div>
  </ProductShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import ProductShell from '@/components/product/ProductShell.vue'
import ModelPlazaContent from '@/components/modelPlaza/ModelPlazaContent.vue'
import Icon from '@/components/icons/Icon.vue'
import { getModelPlaza, type ModelPlazaResponse } from '@/api/modelPlaza'

const { t } = useI18n()
const data = ref<ModelPlazaResponse | null>(null)
const loading = ref(true)
const error = ref(false)

const totalModels = computed(() => new Set((data.value?.groups ?? []).flatMap((group) => group.models.map((model) => model.name))).size)
const totalGroups = computed(() => data.value?.groups.length ?? 0)
const platforms = computed(() => new Set((data.value?.groups ?? []).map((group) => group.platform).filter(Boolean)).size)
const stats = computed(() => [
  { label: t('product.pricing.models'), value: totalModels.value, icon: 'cube' as const },
  { label: t('product.pricing.groups'), value: totalGroups.value, icon: 'grid' as const },
  { label: t('product.pricing.platforms'), value: platforms.value, icon: 'globe' as const },
])

async function loadPricing() {
  loading.value = true
  error.value = false
  try {
    data.value = await getModelPlaza()
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadPricing()
})
</script>
