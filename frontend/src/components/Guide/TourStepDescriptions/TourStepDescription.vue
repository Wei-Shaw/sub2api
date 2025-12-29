<template>
  <section
    class="tour-step-description"
    :lang="locale"
    :data-step-key="stepKey"
  >
    <slot />
  </section>
</template>

<script setup lang="ts">
import type { TourStepKey REDACTED from '@/composables/useTourStepDescription'

interface TourStepDescriptionProps {
  stepKey: TourStepKey
  locale: string
REDACTED

defineProps<TourStepDescriptionProps>()
</script>
