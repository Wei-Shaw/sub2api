<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent.trim()" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    class="ikun-home relative flex min-h-screen flex-col overflow-hidden bg-[#fffaf2] text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden ikun-court-bg">
      <div class="ikun-glow ikun-glow-teal"></div>
      <div class="ikun-glow ikun-glow-gold"></div>
      <div class="ikun-court-line ikun-court-line-main"></div>
      <div class="ikun-court-line ikun-court-line-small"></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 px-6 py-6">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <!-- Logo -->
        <div class="flex items-center gap-3">
          <div class="h-12 w-12 overflow-hidden">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <div class="hidden items-baseline gap-3 sm:flex">
            <span class="text-2xl font-semibold tracking-tight text-gray-950 dark:text-white">ikun.love</span>
            <span class="text-sm font-medium text-amber-700 dark:text-amber-300">{{
              t('home.brandTagline')
            }}</span>
          </div>
        </div>

        <!-- Nav Actions -->
        <div class="flex items-center gap-3">
          <!-- Language Switcher -->
          <LocaleSwitcher />

          <!-- Doc Link -->
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <!-- Login / Dashboard Button -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-950 py-1 pl-1 pr-3 transition-colors hover:bg-gray-800 dark:bg-white dark:text-gray-950 dark:hover:bg-amber-100"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-amber-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white dark:text-gray-950">{{ t('home.dashboard') }}</span>
            <svg
              class="h-3 w-3 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25"
              />
            </svg>
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-full bg-gray-950 px-4 py-2 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-white dark:text-gray-950 dark:hover:bg-amber-100"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-6 pb-8 pt-6 md:pt-12">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section -->
        <div class="mb-8 grid items-center gap-8 lg:grid-cols-[1fr_0.95fr] lg:gap-14">
          <section class="text-center lg:text-left">
            <div
              class="mb-8 inline-flex items-center rounded-full border border-amber-400/70 bg-white/70 px-6 py-2 text-sm font-medium uppercase tracking-wide text-amber-800 shadow-sm backdrop-blur dark:border-amber-400/40 dark:bg-dark-900/70 dark:text-amber-200"
            >
              Basketball API Gateway
            </div>
            <h1 class="mb-3 text-6xl font-semibold leading-none tracking-normal text-gray-950 dark:text-white lg:text-7xl">
              {{ siteName }}
            </h1>
            <p class="mb-5 text-4xl leading-tight tracking-normal text-gray-950 dark:text-white lg:text-5xl">
              {{ t('home.heroSubtitle') }}
            </p>
            <p class="mx-auto mb-7 max-w-2xl text-lg leading-8 text-gray-600 dark:text-dark-300 lg:mx-0">
              {{ t('home.heroDescription') }}
            </p>

            <div class="flex flex-col items-center gap-4 sm:flex-row lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="inline-flex items-center justify-center rounded-2xl bg-gray-950 px-8 py-4 text-base font-medium text-white shadow-xl shadow-gray-950/15 transition hover:-translate-y-0.5 hover:bg-gray-800 dark:bg-white dark:text-gray-950 dark:hover:bg-amber-100"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-3 text-amber-300 dark:text-amber-600" :stroke-width="2" />
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex items-center justify-center rounded-2xl border border-amber-400/70 bg-white/70 px-8 py-4 text-base font-medium text-gray-800 shadow-sm backdrop-blur transition hover:-translate-y-0.5 hover:bg-white dark:border-amber-400/40 dark:bg-dark-900/70 dark:text-white dark:hover:bg-dark-800"
              >
                {{ t('home.docs') }}
              </a>
            </div>
          </section>

          <section class="hero-visual relative mx-auto h-[360px] w-full max-w-[560px] lg:mx-0 lg:ml-auto">
            <div class="mascot-orbit" aria-hidden="true">
              <span v-for="tick in 20" :key="tick" :style="{ transform: `rotate(${tick * 18}deg)` }"></span>
            </div>
            <div class="mascot-card">
              <img :src="siteLogo || '/logo.png'" alt="Ikun logo" class="mascot-logo" />
            </div>
          </section>
        </div>

        <div class="mb-7 grid items-start gap-6 lg:grid-cols-[minmax(0,640px)_minmax(360px,1fr)]">
          <!-- Features Grid -->
          <div class="grid gap-5 md:grid-cols-2">
            <div
              class="group rounded-2xl border border-amber-300/70 bg-white/75 p-5 shadow-xl shadow-amber-950/5 backdrop-blur transition-all duration-300 hover:-translate-y-1 hover:shadow-2xl hover:shadow-amber-500/10 dark:border-amber-400/30 dark:bg-dark-900/75"
            >
              <div class="mb-4 flex items-center gap-4">
                <div
                  class="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-amber-400 to-orange-500 p-3 text-white shadow-lg shadow-amber-500/25 transition-transform group-hover:scale-105"
                >
                  <Icon name="sync" size="lg" />
                </div>
                <h3 class="text-2xl font-semibold text-gray-950 dark:text-white">
                  {{ t('home.features.multiAccount') }}
                </h3>
              </div>
              <p class="text-base leading-relaxed text-gray-600 dark:text-dark-300">
                {{ t('home.features.multiAccountDesc') }}
              </p>
            </div>

            <div
              class="group rounded-2xl border border-amber-300/70 bg-white/75 p-5 shadow-xl shadow-amber-950/5 backdrop-blur transition-all duration-300 hover:-translate-y-1 hover:shadow-2xl hover:shadow-primary-500/10 dark:border-amber-400/30 dark:bg-dark-900/75"
            >
              <div class="mb-4 flex items-center gap-4">
                <div
                  class="flex h-14 w-14 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-primary-500 to-teal-700 p-3 text-white shadow-lg shadow-primary-500/25 transition-transform group-hover:scale-105"
                >
                  <Icon name="calculator" size="lg" />
                </div>
                <h3 class="text-2xl font-semibold text-gray-950 dark:text-white">
                  {{ t('home.features.balanceQuota') }}
                </h3>
              </div>
              <p class="text-base leading-relaxed text-gray-600 dark:text-dark-300">
                {{ t('home.features.balanceQuotaDesc') }}
              </p>
            </div>
          </div>

          <div class="terminal-container w-full max-w-[460px] justify-self-end">
            <div class="terminal-window">
              <div class="terminal-header">
                <div class="terminal-buttons">
                  <span class="btn-close"></span>
                  <span class="btn-minimize"></span>
                  <span class="btn-maximize"></span>
                </div>
                <span class="terminal-title">terminal</span>
              </div>
              <div class="terminal-body">
                <div class="code-line line-1">
                  <span class="code-prompt">$</span>
                  <span class="code-cmd">curl</span>
                  <span class="code-flag">-X POST</span>
                  <span class="code-url">https://ikun.love/v1/messages</span>
                </div>
                <div class="code-line line-2">
                  <span class="code-comment"># {{ t('home.terminal.routing') }}</span>
                </div>
                <div class="code-line line-3">
                  <span class="code-success">200 OK</span>
                  <span class="code-response">{ "content": "{{ t('home.terminal.response') }}" }</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="mb-8 flex flex-wrap items-center gap-4">
          <!-- Claude - Supported -->
          <div
            class="flex items-center gap-2 rounded-2xl border border-primary-200 bg-white/70 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/70"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-400 to-orange-500"
            >
              <span class="text-xs font-bold text-white">C</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.claude') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
          <!-- GPT - Supported -->
          <div
            class="flex items-center gap-2 rounded-2xl border border-primary-200 bg-white/70 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/70"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-green-500 to-green-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">GPT</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Gemini - Supported -->
          <div
            class="flex items-center gap-2 rounded-2xl border border-primary-200 bg-white/70 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/70"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-blue-500 to-blue-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.gemini') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
          <!-- Antigravity - Supported -->
          <div
            class="flex items-center gap-2 rounded-2xl border border-primary-200 bg-white/70 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/70"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-rose-500 to-pink-600"
            >
              <span class="text-xs font-bold text-white">A</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.antigravity') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
          <!-- More - Coming Soon -->
          <div
            class="flex items-center gap-2 rounded-2xl border border-gray-200/50 bg-white/50 px-5 py-3 opacity-60 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/50"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-gray-500 to-gray-600"
            >
              <span class="text-xs font-bold text-white">+</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.more') }}</span>
            <span
              class="rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
        </div>
      </div>
    </main>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div
        class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"
      >
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Ikun')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '/logo.png')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.ikun-home {
  background:
    radial-gradient(circle at 82% 6%, rgba(13, 148, 136, 0.14), transparent 34rem),
    radial-gradient(circle at 8% 88%, rgba(245, 158, 11, 0.24), transparent 34rem),
    linear-gradient(135deg, #fffaf2 0%, #fff7ed 48%, #f7fffc 100%);
}

:deep(.dark) .ikun-home {
  background:
    radial-gradient(circle at 82% 6%, rgba(13, 148, 136, 0.16), transparent 34rem),
    radial-gradient(circle at 8% 88%, rgba(245, 158, 11, 0.14), transparent 34rem),
    linear-gradient(135deg, #080b14 0%, #111827 52%, #0f1f23 100%);
}

.ikun-court-bg {
  background-image:
    linear-gradient(rgba(17, 24, 39, 0.055) 1px, transparent 1px),
    linear-gradient(90deg, rgba(17, 24, 39, 0.055) 1px, transparent 1px);
  background-size: 76px 76px;
}

:deep(.dark) .ikun-court-bg {
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.04) 1px, transparent 1px);
}

.ikun-glow {
  position: absolute;
  width: 44rem;
  height: 44rem;
  border-radius: 9999px;
  filter: blur(70px);
}

.ikun-glow-teal {
  right: -13rem;
  top: -18rem;
  background: rgba(13, 148, 136, 0.13);
}

.ikun-glow-gold {
  bottom: -16rem;
  left: -15rem;
  background: rgba(245, 158, 11, 0.2);
}

.ikun-court-line {
  position: absolute;
  border: 2px solid rgba(245, 139, 21, 0.42);
  border-bottom-color: transparent;
  border-left-color: transparent;
  border-radius: 9999px;
}

.ikun-court-line-main {
  right: 11%;
  top: 12rem;
  width: 48rem;
  height: 48rem;
  transform: rotate(142deg);
}

.ikun-court-line-small {
  left: 5rem;
  bottom: -3rem;
  width: 28rem;
  height: 28rem;
  transform: rotate(28deg);
}

.mascot-card {
  position: absolute;
  bottom: 0;
  right: 0.5rem;
  display: grid;
  width: min(26rem, 88vw);
  aspect-ratio: 1;
  place-items: center;
  border: 2px solid rgba(245, 139, 21, 0.42);
  border-radius: 9999px;
  background: rgba(255, 255, 255, 0.48);
  box-shadow: 0 24px 60px rgba(87, 53, 15, 0.15);
  backdrop-filter: blur(14px);
}

.mascot-logo {
  width: 84%;
  height: 84%;
  object-fit: contain;
  object-position: center bottom;
  filter: drop-shadow(0 18px 18px rgba(17, 24, 39, 0.15));
}

.mascot-orbit {
  position: absolute;
  bottom: 0;
  right: 0.5rem;
  width: min(26rem, 88vw);
  aspect-ratio: 1;
  border-radius: 9999px;
}

.mascot-orbit span {
  position: absolute;
  inset: 0;
}

.mascot-orbit span::before {
  content: '';
  position: absolute;
  left: 50%;
  top: 0.8rem;
  width: 2px;
  height: 2rem;
  background: rgba(245, 139, 21, 0.62);
  transform: translateX(-50%);
}

/* Terminal Container */
.terminal-container {
  position: relative;
  display: inline-block;
}

/* Terminal Window */
.terminal-window {
  width: min(100%, 460px);
  background: linear-gradient(145deg, #111827 0%, #0f172a 100%);
  border-radius: 18px;
  box-shadow:
    0 24px 52px -16px rgba(0, 0, 0, 0.42),
    0 0 0 1px rgba(255, 255, 255, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: translateY(-4px);
}

/* Terminal Header */
.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(30, 41, 59, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 15px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #22c55e;
  font-weight: bold;
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #fbbf24;
}
.code-url {
  color: #67e8f9;
}
.code-comment {
  color: #64748b;
  font-style: italic;
}
.code-success {
  color: #22c55e;
  background: rgba(34, 197, 94, 0.14);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
}
.code-response {
  color: #fbbf24;
}

/* Dark mode adjustments */
:deep(.dark) .terminal-window {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(13, 148, 136, 0.2),
    0 0 40px rgba(13, 148, 136, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}

@media (max-width: 1023px) {
  .mascot-card,
  .mascot-orbit {
    left: 50%;
    right: auto;
    transform: translateX(-50%);
  }
}

@media (max-width: 640px) {
  .hero-visual {
    height: 360px;
  }

  .terminal-container {
    max-width: 90vw;
  }

  .terminal-body {
    padding: 18px 20px;
    font-size: 12px;
    line-height: 1.7;
  }

  .mascot-card,
  .mascot-orbit {
    width: min(20rem, 88vw);
  }

  .mascot-card {
    bottom: 3.5rem;
  }

  .mascot-orbit {
    bottom: 3.5rem;
  }
}
</style>
