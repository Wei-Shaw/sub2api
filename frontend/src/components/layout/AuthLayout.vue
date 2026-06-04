<template>
  <div class="cosmic-shell relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <!-- 星空与鼠标黑洞透镜背景 -->
    <div class="cosmic-backdrop cosmic-backdrop--absolute">
      <div class="starfield">
        <div class="stars-sm"></div>
        <div class="stars-md"></div>
        <div class="stars-lg"></div>
      </div>
      <div class="meteors">
        <i class="meteor" style="--a: 18deg; --dur: 5.5s; --delay: 0s"></i>
        <i class="meteor" style="--a: 75deg; --dur: 7s; --delay: 1.6s"></i>
        <i class="meteor" style="--a: 140deg; --dur: 6s; --delay: 3.2s"></i>
        <i class="meteor" style="--a: 205deg; --dur: 8s; --delay: 0.9s"></i>
        <i class="meteor" style="--a: 268deg; --dur: 6.5s; --delay: 4.4s"></i>
        <i class="meteor" style="--a: 325deg; --dur: 7.5s; --delay: 2.5s"></i>
      </div>
    </div>

    <!-- Content Container -->
    <div class="relative z-10 w-full max-w-md">
      <!-- Logo/Brand -->
      <div class="mb-8 text-center">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl shadow-lg shadow-primary-500/30"
          >
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div class="card-glass rounded-2xl p-8 shadow-glass">
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
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

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
