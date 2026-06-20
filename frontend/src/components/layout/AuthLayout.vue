<template>
  <div class="flex min-h-screen items-center justify-center bg-white px-4 py-10 text-gray-900 dark:bg-dark-950 dark:text-gray-100">
    <div class="w-full max-w-md">
      <div class="mb-8 text-center">
        <template v-if="settingsLoaded">
          <div class="mb-5 inline-flex h-14 w-14 items-center justify-center overflow-hidden rounded-md border border-gray-200 bg-white shadow-soft dark:border-dark-700 dark:bg-dark-900 dark:shadow-soft-dark">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="mb-2 font-display text-4xl font-semibold italic text-gray-950 dark:text-gray-100">
            {{ siteName }}
          </h1>
          <p class="text-sm text-gray-500 dark:text-gray-400">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <div class="rounded-lg border border-gray-200 bg-white p-8 shadow-soft dark:border-dark-700 dark:bg-dark-900 dark:shadow-soft-dark">
        <slot />
      </div>

      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <div class="mt-8 text-center text-xs text-gray-400 dark:text-gray-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { DEFAULT_SITE_NAME, DEFAULT_SITE_SUBTITLE } from '@/constants/branding'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || DEFAULT_SITE_NAME)
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || DEFAULT_SITE_SUBTITLE
)
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
