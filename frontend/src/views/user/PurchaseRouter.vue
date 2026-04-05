<template>
  <PaymentView v-if="paymentEnabled" />
  <PurchaseSubscriptionView v-else-if="purchaseSubscriptionEnabled" />
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useAppStore } from '@/stores/app'
import PaymentView from './PaymentView.vue'
import PurchaseSubscriptionView from './PurchaseSubscriptionView.vue'

const appStore = useAppStore()
const paymentEnabled = computed(() => appStore.cachedPublicSettings?.payment_enabled ?? false)
const purchaseSubscriptionEnabled = computed(
  () => appStore.cachedPublicSettings?.purchase_subscription_enabled ?? false
)
</script>
