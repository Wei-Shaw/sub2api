<template>
  <div :class="pageClass">
    <!-- Background -->
    <div
      :class="backgroundClass"
    ></div>

    <!-- Decorative Elements -->
    <div v-if="variant === 'default'" class="pointer-events-none absolute inset-0 overflow-hidden">
      <!-- Gradient Orbs -->
      <div
        class="absolute -right-40 -top-40 h-80 w-80 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/2 top-1/2 h-96 w-96 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary-300/10 blur-3xl"
      ></div>

      <!-- Grid Pattern -->
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>
    <div v-else class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="absolute inset-0 bg-[linear-gradient(rgba(34,211,238,0.10)_1px,transparent_1px),linear-gradient(90deg,rgba(59,130,246,0.10)_1px,transparent_1px)] bg-[size:48px_48px]"></div>
      <div class="absolute inset-x-0 top-0 h-px bg-cyan-300/50"></div>
      <div class="absolute left-0 top-0 h-full w-px bg-cyan-300/25"></div>
      <div class="absolute right-0 top-0 h-full w-px bg-blue-400/20"></div>
      <div class="absolute -left-20 top-1/4 h-40 w-[120%] -rotate-12 bg-cyan-400/10 blur-2xl"></div>
      <div class="absolute bottom-16 right-[-10%] h-px w-[70%] -rotate-12 bg-cyan-300/40"></div>
      <div class="absolute left-12 top-12 h-28 w-28 border-l border-t border-cyan-300/30"></div>
      <div class="absolute bottom-12 right-12 h-28 w-28 border-b border-r border-blue-300/30"></div>
      <div class="absolute inset-0 bg-[linear-gradient(180deg,transparent,rgba(15,23,42,0.08)_50%,transparent)] bg-[length:100%_6px] opacity-60"></div>
    </div>

    <!-- Content Container -->
    <div :class="contentClass">
      <!-- Logo/Brand -->
      <div :class="brandClass">
        <!-- Custom Logo or Default Logo -->
        <template v-if="settingsLoaded">
          <div
            :class="logoClass"
          >
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 :class="titleClass">
            {{ siteName }}
          </h1>
          <p :class="subtitleClass">
            {{ siteSubtitle }}
          </p>
        </template>
      </div>

      <!-- Card Container -->
      <div :class="cardClass">
        <slot />
      </div>

      <!-- Footer Links -->
      <div :class="footerClass">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div :class="copyrightClass">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const props = withDefaults(defineProps<{
  variant?: 'default' | 'cyber'
}>(), {
  variant: 'default'
})

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const variant = computed(() => props.variant)

const currentYear = computed(() => new Date().getFullYear())

const pageClass = computed(() => [
  'relative flex min-h-screen items-center justify-center overflow-hidden p-4',
  variant.value === 'cyber' ? 'bg-slate-950 text-white' : '',
])

const backgroundClass = computed(() =>
  variant.value === 'cyber'
    ? 'absolute inset-0 bg-[radial-gradient(ellipse_at_top,rgba(14,165,233,0.20),transparent_38%),linear-gradient(135deg,#020617_0%,#07111f_48%,#0f172a_100%)]'
    : 'absolute inset-0 bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950'
)

const contentClass = computed(() => [
  'relative z-10 w-full',
  variant.value === 'cyber' ? 'max-w-5xl' : 'max-w-md',
])

const brandClass = computed(() => [
  'text-center',
  variant.value === 'cyber' ? 'mb-6' : 'mb-8',
])

const logoClass = computed(() => [
  'mb-4 inline-flex items-center justify-center overflow-hidden',
  variant.value === 'cyber'
    ? 'h-14 w-14 rounded-2xl border border-cyan-300/30 bg-slate-900/80 p-1 shadow-lg shadow-cyan-500/20'
    : 'h-16 w-16 rounded-2xl shadow-lg shadow-primary-500/30',
])

const titleClass = computed(() => [
  'mb-2 font-bold tracking-normal',
  variant.value === 'cyber'
    ? 'bg-gradient-to-r from-cyan-200 via-white to-blue-200 bg-clip-text text-3xl text-transparent'
    : 'text-gradient text-3xl',
])

const subtitleClass = computed(() => [
  'text-sm',
  variant.value === 'cyber' ? 'text-cyan-100/70' : 'text-gray-500 dark:text-dark-400',
])

const cardClass = computed(() => [
  variant.value === 'cyber'
    ? 'overflow-hidden rounded-[28px] border border-cyan-300/20 bg-slate-950/75 shadow-2xl shadow-cyan-950/50 backdrop-blur-xl'
    : 'card-glass rounded-2xl p-8 shadow-glass',
])

const footerClass = computed(() => [
  'mt-6 text-center text-sm',
  variant.value === 'cyber' ? 'text-cyan-100/70' : '',
])

const copyrightClass = computed(() => [
  'mt-8 text-center text-xs',
  variant.value === 'cyber' ? 'text-cyan-100/35' : 'text-gray-400 dark:text-dark-500',
])

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.text-gradient {
  @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent;
}
</style>
