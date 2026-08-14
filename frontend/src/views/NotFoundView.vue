<template>
  <!--
    404, editorial. What used to be here: two blurred `bg-primary-*/10` orbs, a
    12rem ghost numeral, a gradient-filled rounded tile with a warning glyph and
    a glow shadow, and a `href="#"` "Contact support" link that navigated
    nowhere. A page that cannot show the user anything should not be the loudest
    screen in the product — one rule, one line of type, two ways out.
  -->
  <div
    data-testid="not-found"
    class="flex min-h-screen flex-col justify-center bg-canvas px-6 py-16 text-ink"
  >
    <div class="mx-auto w-full max-w-2xl">
      <p class="font-mono text-2xs uppercase tracking-[0.04em] tabular-nums text-ink-tertiary">
        404
      </p>

      <h1 class="mt-5 text-3xl font-semibold">
        {{ t('errors.pageNotFound') }}
      </h1>

      <p class="mt-4 max-w-lg text-md text-ink-secondary">
        {{ t('errors.pageNotFoundDescription') }}
      </p>

      <hr class="mt-8 h-px w-full border-0 bg-line-subtle" />

      <div class="mt-8 flex flex-wrap items-center gap-3">
        <Button variant="outline" size="md" data-testid="not-found-back" @click="goBack">
          <template #icon>
            <Icon name="arrowLeft" size="xs" :stroke-width="2" />
          </template>
          {{ t('common.back') }}
        </Button>

        <Button
          to="/dashboard"
          tone="accent"
          variant="solid"
          size="md"
          data-testid="not-found-dashboard"
        >
          {{ t('home.goToDashboard') }}
          <template #trailing>
            <Icon name="arrowRight" size="xs" :stroke-width="2" />
          </template>
        </Button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import Button from '@/components/common/Button.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const router = useRouter()

function goBack(): void {
  router.back()
}
</script>
