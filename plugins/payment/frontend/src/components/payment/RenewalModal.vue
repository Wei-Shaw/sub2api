<!-- RenewalModal: modal dialog for selecting a renewal plan for a specific group. -->
<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" @click.self="$emit('close')">
        <div class="relative w-full max-w-lg rounded-2xl border border-gray-200 bg-white p-6 shadow-2xl dark:border-dark-700 dark:bg-dark-900">
          <!-- Close button -->
          <button class="absolute right-4 top-4 rounded-lg p-1 text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200" @click="$emit('close')">
            <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
          </button>
          <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.selectPlan') }}</h3>
          <div class="space-y-4">
            <SubscriptionPlanCard v-for="plan in plans" :key="plan.id" :plan="plan" :active-subscriptions="activeSubscriptions" @select="$emit('select', $event)" />
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import SubscriptionPlanCard from './SubscriptionPlanCard.vue'
import type { SubscriptionPlan } from '../../types/payment'

const { t } = useI18n()

defineProps<{
  show: boolean
  plans: SubscriptionPlan[]
  activeSubscriptions: { id: number; group_id: number; expires_at?: string; group?: Record<string, unknown> }[]
}>()

defineEmits<{
  close: []
  select: [plan: SubscriptionPlan]
}>()
</script>