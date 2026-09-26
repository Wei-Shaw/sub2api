<template>
  <div class="flex min-h-screen">
    <!-- Left Branding Panel -->
    <div class="hidden flex-1 flex-col justify-between bg-primary-600 p-12 lg:flex">
      <div>
        <div class="flex items-center gap-3">
          <div class="h-10 w-10 overflow-hidden rounded-lg bg-white/20 p-1.5">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-xl font-bold text-white">{{ siteName }}</span>
        </div>
      </div>
      <div>
        <h2 class="mb-4 text-3xl font-bold text-white">{{ siteSubtitle }}</h2>
        <p class="text-primary-100">多模型 AI 集合平台</p>
      </div>
      <div class="text-sm text-primary-200">
        &copy; {{ currentYear }} {{ siteName }}. 保留所有权利
      </div>
    </div>

    <!-- Right Form Panel -->
    <div class="flex flex-1 flex-col items-center justify-center p-6 sm:p-8 lg:p-12">
      <div class="w-full max-w-md">
        <!-- Mobile Logo -->
        <div class="mb-8 text-center lg:hidden">
          <div class="mb-3 inline-flex h-12 w-12 items-center justify-center overflow-hidden rounded-lg bg-primary-50 dark:bg-primary-950/30">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-xl font-bold text-gray-900 dark:text-white">{{ siteName }}</h1>
        </div>

        <!-- Form Card -->
        <div class="rounded-xl border border-gray-100 bg-white p-8 shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <slot />
        </div>

        <!-- Footer Links -->
        <div class="mt-6 text-center text-sm">
          <slot name="footer" />
        </div>

        <!-- Copyright (desktop) -->
        <div class="mt-8 hidden text-center text-xs text-gray-400 dark:text-dark-500 lg:block">
          &copy; {{ currentYear }} {{ siteName }}. 保留所有权利
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || '稳得AI')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || '多模型 AI 集合平台')
const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>
