import { ref } from 'vue'
import { adminAPI } from '@/api/admin'
import { DEFAULT_USD_CNY_RATE, normalizeUsdCnyRate } from '@/composables/useDisplayCurrency'

export function useAdminDisplayCurrencyRate() {
  const usdCnyRate = ref(DEFAULT_USD_CNY_RATE)
  const loadingUsdCnyRate = ref(false)

  const loadUsdCnyRate = async () => {
    loadingUsdCnyRate.value = true
    try {
      const stats = await adminAPI.dashboard.getStats()
      usdCnyRate.value = normalizeUsdCnyRate(stats.usd_cny_rate)
    } catch {
      usdCnyRate.value = DEFAULT_USD_CNY_RATE
    } finally {
      loadingUsdCnyRate.value = false
    }
  }

  return {
    usdCnyRate,
    loadingUsdCnyRate,
    loadUsdCnyRate
  }
}
