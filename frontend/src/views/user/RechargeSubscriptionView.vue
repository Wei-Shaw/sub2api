<template>
  <AppLayout>
    <div class="space-y-6" data-testid="recharge-subscription-shell">
      <header class="space-y-1">
        <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">{{ t('rechargeSubscription.title') }}</h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">{{ t('rechargeSubscription.description') }}</p>
      </header>

      <PaymentView embedded layout-mode="stacked" hide-tabs @payment-completed="refreshOrders" />

      <section id="orders" class="space-y-4">
        <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('rechargeSubscription.ordersSection') }}</h2>
        <UserOrdersView ref="ordersViewRef" embedded />
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import PaymentView from '@/views/user/PaymentView.vue'
import UserOrdersView from '@/views/user/UserOrdersView.vue'

const { t } = useI18n()
const ordersViewRef = ref<{ refresh: () => Promise<void> } | null>(null)

function refreshOrders() {
  ordersViewRef.value?.refresh()
}
</script>
