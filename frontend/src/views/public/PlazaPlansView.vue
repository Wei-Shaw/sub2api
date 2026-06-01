<template>
  <PlazaLayout>
    <PlanPlazaCards
      :cards="cards"
      :loading="loading"
      :currency-display="currencyToggle.display.value"
      :format-cny="formatCny"
      @currency-change="currencyToggle.set"
    />
  </PlazaLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { plazaAPI, type PlazaPlanCard, type PlazaCurrencyMeta } from '@/api/plaza'
import { useCurrencyToggle } from '@/composables/useCurrencyToggle'
import PlazaLayout from './PlazaLayout.vue'
import PlanPlazaCards from '@/components/plaza/PlanPlazaCards.vue'

const cards = ref<PlazaPlanCard[]>([])
const meta = ref<PlazaCurrencyMeta | null>(null)
const loading = ref(false)

const currencyToggle = useCurrencyToggle(() => meta.value?.balance_recharge_multiplier)

function formatCny(amount: number, digits?: number): string {
  return currencyToggle.format(amount, 'CNY', digits)
}

async function load() {
  loading.value = true
  try {
    const resp = await plazaAPI.listPlans()
    cards.value = resp.cards ?? []
    meta.value = resp.currency_meta
  } catch {
    // Empty cards on transient failure; matches model view behaviour.
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
