import { defineStore } from 'pinia'
import { ref } from 'vue'
import { adminAPI } from '@/api'

export const useAdminReferralBadgeStore = defineStore('adminReferralBadges', () => {
  const pendingAffiliateCount = ref(0)
  const pendingWithdrawalCount = ref(0)
  const loading = ref(false)
  const loaded = ref(false)
  const requestToken = ref(0)

  function clear(): void {
    pendingAffiliateCount.value = 0
    pendingWithdrawalCount.value = 0
    loading.value = false
    loaded.value = false
  }

  async function refresh(force = false): Promise<void> {
    if (loading.value && !force) return

    const token = requestToken.value + 1
    requestToken.value = token
    loading.value = true

    try {
      const [affiliates, pendingWithdrawals, approvedWithdrawals] = await Promise.all([
        adminAPI.referral.listAffiliates({ status: 'pending', page: 1, page_size: 1 }),
        adminAPI.referral.listWithdrawals({ status: 'pending', page: 1, page_size: 1 }),
        adminAPI.referral.listWithdrawals({ status: 'approved', page: 1, page_size: 1 }),
      ])

      if (requestToken.value !== token) {
        return
      }

      pendingAffiliateCount.value = affiliates.total
      pendingWithdrawalCount.value = pendingWithdrawals.total + approvedWithdrawals.total
      loaded.value = true
    } finally {
      if (requestToken.value === token) {
        loading.value = false
      }
    }
  }

  return {
    pendingAffiliateCount,
    pendingWithdrawalCount,
    loading,
    loaded,
    clear,
    refresh,
  }
})
