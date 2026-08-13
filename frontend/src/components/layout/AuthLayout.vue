<template>
  <!--
    Editorial rewrite. What this replaces: a `bg-gradient-to-br` ground, three
    blurred teal orbs at `blur-3xl`, a hardcoded `rgba(20,184,166,0.03)` grid
    overlay, a `rounded-2xl` logo tile with a colored glow shadow, a
    `text-gradient` clip-text wordmark, and a `card-glass … shadow-glass` panel.
    All six were the same gesture, repeated.

    Now: flat canvas, one hairline-ruled panel, typographic lockup, left-aligned
    composition instead of dead-centre.
  -->
  <div class="flex min-h-screen items-center justify-center bg-canvas p-4">
    <div class="w-full max-w-[26rem]">
      <!-- Brand lockup. `settingsLoaded` gates it to avoid a logo swap flash. -->
      <div class="mb-8">
        <template v-if="settingsLoaded">
          <div class="mb-5 flex items-center gap-2.5">
            <img :src="siteLogo || '/logo.svg'" alt="" class="h-6 w-6 shrink-0 object-contain" />
            <span class="text-md font-semibold tracking-tight text-ink">{{ siteName }}</span>
          </div>
          <p class="max-w-[22rem] text-sm text-ink-tertiary">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <div class="border border-line bg-surface p-6">
        <slot />
      </div>

      <div class="mt-5 text-sm">
        <slot name="footer" />
      </div>

      <div class="mt-8 border-t border-line-subtle pt-4 text-2xs text-ink-disabled">
        &copy; {{ currentYear }} {{ siteName }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<!--
  The `<style scoped>` block that used to live here redefined `.text-gradient`
  locally, shadowing the global one. Neutralizing the global rule alone would
  not have reached it — this file would have kept its gradient wordmark while
  every other surface lost theirs. Deleted, not overridden.
-->
