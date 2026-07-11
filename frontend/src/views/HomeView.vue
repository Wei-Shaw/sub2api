<template>
  <!-- Admin-configured overrides intentionally keep their existing full-page behavior. -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="mac-home" :class="{ 'mac-home--dark': isDark }">
    <div class="mac-home__backdrop" aria-hidden="true">
      <span class="mac-home__wash mac-home__wash--blue"></span>
      <span class="mac-home__wash mac-home__wash--violet"></span>
      <span class="mac-home__wash mac-home__wash--cyan"></span>
      <span class="mac-home__noise"></span>
    </div>

    <HomeNavigation
      :site-name="siteName"
      :site-logo="siteLogo"
      :doc-url="docUrl"
      :is-authenticated="isAuthenticated"
      :dashboard-path="dashboardPath"
      :is-dark="isDark"
      @toggle-theme="toggleTheme"
    />

    <main class="relative z-10">
      <HomeHero
        :site-name="siteName"
        :site-subtitle="siteSubtitle"
        :api-base-url="apiBaseUrl"
        :doc-url="docUrl"
        :is-authenticated="isAuthenticated"
        :dashboard-path="dashboardPath"
      />
      <HomeCapabilities
        :is-authenticated="isAuthenticated"
        :dashboard-path="dashboardPath"
      />
    </main>

    <HomeFooter
      :site-name="siteName"
      :site-logo="siteLogo"
      :doc-url="docUrl"
      :current-year="currentYear"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import HomeCapabilities from '@/components/home/HomeCapabilities.vue'
import HomeFooter from '@/components/home/HomeFooter.vue'
import HomeHero from '@/components/home/HomeHero.vue'
import HomeNavigation from '@/components/home/HomeNavigation.vue'
import { useAppStore, useAuthStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(
  () => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API'
)
const siteLogo = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', {
    allowRelative: true,
    allowDataUrl: true
  })
)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform'
)
const docUrl = computed(() =>
  sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
)
const apiBaseUrl = computed(() => {
  const configured = appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl
  return sanitizeUrl(configured || window.location.origin, { allowRelative: true })
})
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const dashboardPath = computed(() =>
  authStore.isAdmin ? '/admin/dashboard' : '/dashboard'
)
const currentYear = computed(() => new Date().getFullYear())

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  const prefersDark = window.matchMedia('(prefers-color-scheme: dark)').matches
  isDark.value = savedTheme === 'dark' || (!savedTheme && prefersDark)
  document.documentElement.classList.toggle('dark', isDark.value)
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    void appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.mac-home {
  --mac-canvas: #f5f7fb;
  --mac-ink: #0b1020;
  --mac-muted: #526075;
  --mac-glass: rgba(255, 255, 255, 0.7);
  --mac-glass-thick: rgba(255, 255, 255, 0.84);
  --mac-line: rgba(255, 255, 255, 0.78);
  --mac-stroke: rgba(67, 81, 112, 0.14);
  --mac-shadow: 0 30px 90px rgba(43, 57, 91, 0.12);
  --home-text: var(--mac-ink);
  --home-muted: var(--mac-muted);
  --home-faint: #718096;
  --home-accent: #0a84ff;
  --home-accent-soft: rgba(10, 132, 255, 0.12);
  --home-accent-contrast: #ffffff;
  --home-panel: var(--mac-glass);
  --home-border: var(--mac-stroke);
  --home-border-strong: rgba(67, 81, 112, 0.22);
  --home-glass: rgba(255, 255, 255, 0.68);
  --home-glass-strong: rgba(255, 255, 255, 0.86);
  --home-glass-sheen: rgba(244, 248, 255, 0.5);
  --home-glass-hover: rgba(10, 132, 255, 0.07);
  --home-glass-border: rgba(255, 255, 255, 0.88);
  --home-glass-border-strong: rgba(80, 120, 180, 0.3);
  --home-glass-divider: rgba(71, 91, 126, 0.12);
  --home-glass-highlight: rgba(255, 255, 255, 0.96);
  --home-code-text: #34445d;
  --home-success: #087c5b;
  --home-success-soft: rgba(21, 184, 132, 0.13);
  position: relative;
  min-height: 100vh;
  overflow-x: clip;
  color: var(--mac-ink);
  background:
    radial-gradient(circle at 78% 6%, rgba(72, 162, 255, 0.18), transparent 28rem),
    linear-gradient(180deg, rgba(255, 255, 255, 0.48), transparent 38rem),
    var(--mac-canvas);
  font-family:
    -apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Segoe UI",
    sans-serif;
  isolation: isolate;
}

.mac-home--dark {
  --mac-canvas: #07090f;
  --mac-ink: #f6f8ff;
  --mac-muted: #a7b0c4;
  --mac-glass: rgba(17, 20, 31, 0.7);
  --mac-glass-thick: rgba(14, 17, 27, 0.86);
  --mac-line: rgba(255, 255, 255, 0.12);
  --mac-stroke: rgba(255, 255, 255, 0.1);
  --mac-shadow: 0 36px 110px rgba(0, 0, 0, 0.45);
  --home-faint: #77839a;
  --home-accent-soft: rgba(10, 132, 255, 0.18);
  --home-border-strong: rgba(255, 255, 255, 0.17);
  --home-glass: rgba(12, 20, 36, 0.58);
  --home-glass-strong: rgba(10, 17, 30, 0.78);
  --home-glass-sheen: rgba(255, 255, 255, 0.035);
  --home-glass-hover: rgba(78, 163, 255, 0.09);
  --home-glass-border: rgba(255, 255, 255, 0.12);
  --home-glass-border-strong: rgba(98, 174, 255, 0.3);
  --home-glass-divider: rgba(255, 255, 255, 0.075);
  --home-glass-highlight: rgba(255, 255, 255, 0.1);
  --home-code-text: #c3d2e7;
  --home-success: #48d6a4;
  --home-success-soft: rgba(72, 214, 164, 0.14);
  background:
    radial-gradient(circle at 82% 8%, rgba(22, 111, 221, 0.28), transparent 32rem),
    radial-gradient(circle at 12% 44%, rgba(78, 74, 214, 0.15), transparent 30rem),
    linear-gradient(180deg, rgba(17, 27, 48, 0.78), transparent 40rem),
    var(--mac-canvas);
}

.mac-home__backdrop {
  position: absolute;
  inset: 0;
  z-index: -1;
  overflow: hidden;
  pointer-events: none;
}

.mac-home__wash {
  position: absolute;
  width: min(54rem, 78vw);
  aspect-ratio: 1;
  border-radius: 50%;
  filter: blur(96px);
  opacity: 0.42;
  animation: mac-drift 16s ease-in-out infinite alternate;
}

.mac-home__wash--blue {
  top: -22rem;
  right: -13rem;
  background: rgba(10, 132, 255, 0.7);
}

.mac-home__wash--violet {
  top: 24rem;
  left: -24rem;
  background: rgba(94, 92, 230, 0.38);
  animation-delay: -5s;
}

.mac-home__wash--cyan {
  top: 72rem;
  right: -25rem;
  background: rgba(50, 196, 220, 0.32);
  animation-delay: -9s;
}

.mac-home__noise {
  position: absolute;
  inset: 0;
  opacity: 0.4;
  background:
    radial-gradient(circle at 50% 28%, transparent 0 17rem, rgba(255, 255, 255, 0.08) 34rem, transparent 52rem),
    linear-gradient(118deg, transparent 12%, rgba(255, 255, 255, 0.1) 48%, transparent 72%);
  mask-image: linear-gradient(to bottom, black, transparent 68%);
}

@keyframes mac-drift {
  from {
    transform: translate3d(-2%, -1%, 0) scale(0.96);
  }
  to {
    transform: translate3d(4%, 3%, 0) scale(1.04);
  }
}

@media (prefers-reduced-motion: reduce) {
  .mac-home__wash {
    animation: none;
  }
}
</style>
