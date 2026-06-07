<template>
  <PlazaLayout>
    <PlanPlazaCards :cards="cards" :loading="loading" />
  </PlazaLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { plazaAPI, type PlazaPlanCard } from '@/api/plaza'
import PlazaLayout from './PlazaLayout.vue'
import PlanPlazaCards from '@/components/plaza/PlanPlazaCards.vue'

// 套餐价格 native currency 始终是 CNY，运营定价时不做汇率转换；
// 因此这里不再引入 `useCurrencyToggle`，也不再读取 `currency_meta`。
// 模型 plaza（PlazaModelsView）依然保留 USD/CNY 切换逻辑。
const cards = ref<PlazaPlanCard[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const resp = await plazaAPI.listPlans()
    cards.value = resp.cards ?? []
  } catch {
    // Empty cards on transient failure; matches model view behaviour.
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
