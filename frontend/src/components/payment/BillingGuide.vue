<template>
  <aside class="billing-panel billing-guide">
    <h2 class="billing-guide__header">
      <span class="billing-icon"><Icon name="document" size="md" /></span>
      {{ t('payment.design.subscriptionGuide') }}
    </h2>
    <div v-for="item in items" :key="item.key" class="billing-guide__item">
      <span class="billing-icon"><Icon :name="item.icon" size="md" /></span>
      <div>
        <h3>{{ t('payment.design.' + item.key) }}</h3>
        <p>{{ t('payment.design.' + item.key + 'Description') }}</p>
      </div>
    </div>
    <RouterLink to="/subscriptions" class="billing-guide__link">
      {{ t('userSubscriptions.title') }}<Icon name="arrowRight" size="sm" />
    </RouterLink>
  </aside>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const items = [
  { key: 'choosePlan', icon: 'grid' },
  { key: 'viewQuota', icon: 'chart' },
  { key: 'renewPlan', icon: 'refresh' },
] as const
</script>
