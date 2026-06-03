<!--
  Pricing teaser block rendered above the Features Grid on `HomeView`.

  Intentionally static — no API call, no live "from $X / M Tokens" copy that
  would otherwise force us to either spinner-block the marketing page or
  hardcode a number that drifts out of sync with the plaza. The link funnels
  anonymous visitors to `/plaza/models` where the real numbers live.
-->
<template>
  <section
    class="mb-12 overflow-hidden rounded-2xl border border-amber-200/60 bg-gradient-to-r from-amber-50 via-amber-50/70 to-orange-50 px-6 py-5 shadow-sm dark:border-amber-500/30 dark:from-amber-900/20 dark:via-amber-900/15 dark:to-orange-900/20"
  >
    <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <div class="flex items-start gap-3">
        <div
          class="mt-0.5 flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-amber-500/15 text-amber-700 dark:text-amber-300"
        >
          <Icon name="chart" size="md" />
        </div>
        <div>
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('home.pricing_teaser_title') }}
          </h3>
          <p class="mt-0.5 text-sm text-gray-600 dark:text-dark-300">
            {{ t('plaza.subtitle') }}
          </p>
        </div>
      </div>
      <router-link
        to="/plaza/models"
        class="inline-flex shrink-0 items-center gap-1 rounded-lg bg-white/80 px-4 py-2 text-sm font-medium text-amber-700 shadow-sm transition-colors hover:bg-white dark:bg-dark-900/40 dark:text-amber-200 dark:hover:bg-dark-900/60"
      >
        {{ t('home.pricing_teaser_link') }}
      </router-link>
    </div>
  </section>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
</script>
