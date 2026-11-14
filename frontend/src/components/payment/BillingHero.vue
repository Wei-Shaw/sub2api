<template>
  <section class="billing-hero">
    <div class="billing-hero__art" aria-hidden="true">
      <div class="billing-hero__orbit billing-hero__orbit--back" />
      <div class="billing-hero__orbit" />
      <div class="billing-hero__card billing-hero__card--back" />
      <div class="billing-hero__card">
        <svg class="billing-hero__crown" viewBox="0 0 120 100" fill="none">
          <path d="M17 28 39 46 60 18 81 46 103 28 92 76H28L17 28Z" fill="currentColor" fill-opacity=".18" stroke="currentColor" stroke-width="3" stroke-linejoin="round" />
          <path d="m28 76 11-30 21 30 21-30 11 30H28Z" fill="currentColor" fill-opacity=".65" />
          <path d="m39 46 21-28 21 28-21 30-21-30Z" fill="currentColor" fill-opacity=".4" />
          <path d="M30 84h60" stroke="currentColor" stroke-width="5" stroke-linecap="round" />
          <circle cx="17" cy="26" r="5" fill="currentColor" />
          <circle cx="60" cy="16" r="5" fill="currentColor" />
          <circle cx="103" cy="26" r="5" fill="currentColor" />
        </svg>
      </div>
      <span class="billing-hero__spark billing-hero__spark--one" />
      <span class="billing-hero__spark billing-hero__spark--two" />
    </div>
    <div class="billing-hero__intro">
      <div class="billing-hero__brand"><span />{{ appStore.siteName }}</div>
      <h1>{{ title }}<template v-if="accent"><span class="billing-hero__separator"> / </span><span class="billing-hero__accent">{{ accent }}</span></template></h1>
      <p>{{ description }}</p>
    </div>
    <div class="billing-hero__footer" :class="{ 'billing-hero__footer--compact': !features }">
      <div v-if="features" class="billing-hero__features">
        <div v-for="item in highlights" :key="item.key" class="billing-hero__feature">
          <span class="billing-icon"><Icon :name="item.icon" size="md" /></span>
          <div><strong>{{ t('payment.design.' + item.key) }}</strong><span>{{ t('payment.design.' + item.key + 'Hint') }}</span></div>
        </div>
      </div>
      <div class="billing-hero__stats"><slot /></div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import Icon from '@/components/icons/Icon.vue'
import '@/styles/billing.css'

withDefaults(defineProps<{
  title: string
  accent?: string
  description: string
  features?: boolean
}>(), { accent: '', features: false })

const appStore = useAppStore()
const { t } = useI18n()
const highlights = [
  { key: 'flexible', icon: 'grid' },
  { key: 'clear', icon: 'chart' },
  { key: 'payment', icon: 'creditCard' },
  { key: 'manage', icon: 'cog' },
] as const
</script>
