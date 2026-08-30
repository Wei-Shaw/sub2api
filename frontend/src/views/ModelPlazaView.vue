<template>
  <AppLayout>
    <ModelPlazaContent
      :response="data"
      :loading="loading"
      :error="loadFailed"
      :refreshing="refreshing"
      :last-updated="lastUpdated"
      embedded
      @refresh="loadData(false)"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import ModelPlazaContent from '@/components/modelPlaza/ModelPlazaContent.vue'
import { getModelPrices, type ModelPricesResponse } from '@/api/modelPrices'
import { subscribeDisplayPricingUpdates } from '@/utils/displayPricingSync'

const AUTO_REFRESH_MS = 30_000

const data = ref<ModelPricesResponse | null>(null)
const loading = ref(true)
const refreshing = ref(false)
const loadFailed = ref(false)
const lastUpdated = ref<Date | null>(null)

let refreshTimer: number | undefined
let requestInFlight = false
let refreshQueued = false
let stopPricingSync: (() => void) | undefined

async function loadData(initial: boolean): Promise<void> {
  if (requestInFlight) {
    refreshQueued = true
    return
  }
  requestInFlight = true
  if (initial) loading.value = true
  else refreshing.value = true

  try {
    data.value = await getModelPrices()
    loadFailed.value = false
    lastUpdated.value = new Date()
  } catch {
    if (!data.value) loadFailed.value = true
  } finally {
    loading.value = false
    refreshing.value = false
    requestInFlight = false
    if (refreshQueued) {
      refreshQueued = false
      void loadData(false)
    }
  }
}

function refreshWhenVisible(): void {
  if (document.visibilityState === 'visible') void loadData(false)
}

onMounted(() => {
  void loadData(true)
  refreshTimer = window.setInterval(() => void loadData(false), AUTO_REFRESH_MS)
  window.addEventListener('focus', refreshWhenVisible)
  document.addEventListener('visibilitychange', refreshWhenVisible)
  stopPricingSync = subscribeDisplayPricingUpdates(() => void loadData(false))
})

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) window.clearInterval(refreshTimer)
  window.removeEventListener('focus', refreshWhenVisible)
  document.removeEventListener('visibilitychange', refreshWhenVisible)
  stopPricingSync?.()
})
</script>
