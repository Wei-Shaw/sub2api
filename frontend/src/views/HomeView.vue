<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="hasHomeContent" class="min-h-screen">
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

  <!-- Compact Home Page -->
  <div
    v-else-if="compactHomeEnabled"
    data-testid="compact-home"
    class="flex min-h-screen flex-col bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white"
  >
    <header class="border-b border-gray-200 px-4 py-4 sm:px-6 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 sm:gap-4">
        <div class="flex min-w-0 flex-1 items-center gap-3">
          <img
            :src="siteLogo || '/logo.svg'"
            alt="Logo"
            class="h-9 w-9 shrink-0 rounded-lg object-contain"
          />
          <span class="min-w-0 truncate text-base font-semibold">{{ siteName }}</span>
        </div>
        <div class="flex max-w-full shrink-0 flex-wrap items-center justify-end gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="flex h-10 shrink-0 items-center gap-1.5 rounded-lg px-2.5 text-sm font-medium text-gray-500 hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>
          <button
            class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg text-gray-500 hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex min-h-10 shrink-0 items-center justify-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-200"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex min-w-0 flex-1 items-center justify-center px-4 py-16 sm:px-6">
      <div class="min-w-0 max-w-2xl text-center">
        <img
          :src="siteLogo || '/logo.svg'"
          alt="Logo"
          class="mx-auto mb-6 h-20 w-20 rounded-2xl object-contain"
        />
        <h1 class="[overflow-wrap:anywhere] text-3xl font-bold md:text-4xl">{{ siteName }}</h1>
        <p class="mt-4 whitespace-pre-wrap [overflow-wrap:anywhere] text-base text-gray-600 dark:text-dark-300">{{ siteSubtitle }}</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex min-h-10 items-center justify-center rounded-lg bg-primary-600 px-5 py-2.5 text-sm font-medium text-white hover:bg-primary-700"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.login') }}
        </router-link>
      </div>
    </main>

    <footer class="min-w-0 border-t border-gray-200 px-4 py-5 text-center text-sm text-gray-500 [overflow-wrap:anywhere] sm:px-6 dark:border-dark-800 dark:text-dark-400">
      &copy; {{ currentYear }} {{ siteName }}
    </footer>
  </div>

  <!-- Default Home Page -->
  <div v-else class="premium-home terminal-container relative flex min-h-screen flex-col overflow-hidden">
    <!-- Header -->
    <header class="premium-home-header relative z-20 px-6">
      <nav class="premium-home-nav mx-auto flex max-w-6xl items-center justify-between">
        <!-- Logo -->
        <router-link to="/home" class="premium-brand flex items-center gap-3" aria-label="Home">
          <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="hidden text-sm font-semibold sm:inline">{{ siteName }}</span>
        </router-link>

        <!-- Nav Actions -->
        <div class="premium-home-actions flex items-center gap-2">
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

          <!-- Model Plaza Link -->
          <router-link
            v-if="showModelPlazaEntry"
            to="/model-plaza"
            class="inline-flex items-center gap-1.5 rounded-lg p-2 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
            :title="t('nav.modelPlaza')"
          >
            <Icon name="grid" size="md" />
            <span class="hidden sm:inline">{{ t('nav.modelPlaza') }}</span>
          </router-link>

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
            class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
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
            class="inline-flex items-center rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="premium-home-main relative z-10 flex-1 px-6">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section -->
        <div class="premium-hero mb-12 flex flex-col items-center text-center">
          <div class="premium-hero-copy">
            <div class="premium-eyebrow">
              <span class="premium-live-dot"></span>
              {{ t('home.heroEyebrow') }}
            </div>
            <h1
              class="mb-4 text-4xl font-bold text-gray-900 dark:text-white md:text-5xl lg:text-6xl"
            >
              {{ siteName }}
            </h1>
            <p class="premium-hero-lead mb-3 text-lg md:text-xl">
              {{ t('home.heroSubtitle') }}
            </p>
            <p class="premium-hero-description mb-8 text-base text-gray-600 dark:text-dark-300">
              {{ t('home.heroDescription') }}
            </p>

            <!-- CTA Button -->
            <div>
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="btn btn-primary px-8 py-3 text-base shadow-lg shadow-primary-500/30"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
            </div>
          </div>

          <!-- API request examples -->
          <section class="premium-api-demo-stage" :aria-label="t('home.apiDemo.title')">
            <div class="premium-api-demo">
              <div class="premium-api-demo-tabs" role="tablist">
                <button
                  v-for="tab in apiTabs"
                  :key="tab.key"
                  type="button"
                  role="tab"
                  :aria-selected="activeApiTab === tab.key"
                  :class="['premium-api-tab', 'is-' + tab.key, { 'is-active': activeApiTab === tab.key }]"
                  @click="activeApiTab = tab.key"
                >
                  {{ tab.label }}
                </button>
                <span class="premium-api-status"><i></i>{{ activeDemo.status }}</span>
              </div>

              <div class="premium-api-endpoint">
                <span :class="['premium-api-method', 'is-' + activeDemo.tone]">{{ activeDemo.method }}</span>
                <code>{{ activeDemo.path }}</code>
              </div>

              <div class="premium-api-sections">
                <section class="premium-api-section">
                  <h3>{{ t('home.apiDemo.request') }}</h3>
                  <pre class="premium-api-code"><code><span v-for="(line, lineIndex) in activeDemo.request" :key="'request-' + lineIndex" class="premium-api-line"><span v-for="(segment, segmentIndex) in line" :key="'request-' + lineIndex + '-' + segmentIndex" :class="'premium-api-token-' + (segment.tone || 'muted')">{{ segment.text }}</span></span></code></pre>
                </section>
                <section class="premium-api-section premium-api-response-section">
                  <h3>{{ t('home.apiDemo.response') }}</h3>
                  <pre class="premium-api-code"><code><span v-for="(line, lineIndex) in activeDemo.response" :key="'response-' + lineIndex" class="premium-api-line"><span v-for="(segment, segmentIndex) in line" :key="'response-' + lineIndex + '-' + segmentIndex" :class="'premium-api-token-' + (segment.tone || 'muted')">{{ segment.text }}</span></span></code></pre>
                </section>
              </div>

              <div class="premium-api-demo-footer">
                <div class="premium-api-metrics">
                  <span>{{ activeDemo.latency }}</span><small>ms</small>
                  <b></b>
                  <span>{{ activeDemo.tokens }}</span><small>tokens</small>
                  <b></b>
                  <span>cost {{ activeDemo.cost }}</span>
                </div>
                <span class="premium-api-stream">{{ t('home.apiDemo.stream') }} · {{ t('home.apiDemo.sse') }}</span>
              </div>
            </div>
          </section>
        </div>

        <!-- Feature Tags - Centered -->
        <div class="premium-trust-row mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6">
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="swap" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.subscriptionToApi')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="shield" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.stickySession')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"
          >
            <Icon name="chart" size="sm" class="text-primary-500" />
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{
              t('home.tags.realtimeBilling')
            }}</span>
          </div>
        </div>

        <!-- Features Grid -->
        <div class="premium-feature-grid mb-12 grid gap-6 md:grid-cols-3">
          <!-- Feature 1: Unified Gateway -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-blue-500 to-blue-600 shadow-lg shadow-blue-500/30 transition-transform group-hover:scale-110"
            >
              <Icon name="server" size="lg" class="text-white" />
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.unifiedGateway') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.unifiedGatewayDesc') }}
            </p>
          </div>

          <!-- Feature 2: Account Pool -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.multiAccount') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.multiAccountDesc') }}
            </p>
          </div>

          <!-- Feature 3: Billing & Quota -->
          <div
            class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-purple-500 to-purple-600 shadow-lg shadow-purple-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.balanceQuota') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.balanceQuotaDesc') }}
            </p>
          </div>
        </div>

        <!-- Supported Providers -->
        <div class="mb-8 text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">
            {{ t('home.providers.title') }}
          </h2>
          <p class="text-sm text-gray-600 dark:text-dark-400">
            {{ t('home.providers.description') }}
          </p>
        </div>

        <div class="mb-16 flex flex-wrap items-center justify-center gap-4">
          <!-- Claude - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-orange-400 to-orange-500"
            >
              <span class="text-xs font-bold text-white">C</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.claude') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- GPT - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
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
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-blue-500 to-blue-600"
            >
              <span class="text-xs font-bold text-white">G</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.gemini') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Antigravity - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-primary-200 bg-white/60 px-5 py-3 ring-1 ring-primary-500/20 backdrop-blur-sm dark:border-primary-800 dark:bg-dark-800/60"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br from-rose-500 to-pink-600"
            >
              <span class="text-xs font-bold text-white">A</span>
            </div>
            <span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.antigravity') }}</span>
            <span
              class="rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- More - Coming Soon -->
          <div
            class="flex items-center gap-2 rounded-xl border border-gray-200/50 bg-white/40 px-5 py-3 opacity-60 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/40"
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
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            GitHub
          </a>
        </div>
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
import { sanitizeUrl } from '@/utils/url'
import { FeatureFlags, isFeatureFlagEnabled } from '@/utils/featureFlags'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'BuaGa API')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const hasHomeContent = computed(() => homeContent.value.trim().length > 0)
const compactHomeEnabled = computed(() => appStore.cachedPublicSettings?.compact_home_enabled === true)
const modelPlazaEnabled = computed(() => isFeatureFlagEnabled(FeatureFlags.modelPlaza))

type ApiDemoKey = 'chat' | 'responses' | 'claude' | 'gemini'
type ApiDemoTone = 'command' | 'blue' | 'yellow' | 'cyan' | 'green' | 'purple' | 'muted'
type ApiDemoSegment = { text: string; tone?: ApiDemoTone }
type ApiDemo = {
  method: string
  path: string
  tone: ApiDemoTone
  status: string
  latency: string
  tokens: string
  cost: string
  request: ApiDemoSegment[][]
  response: ApiDemoSegment[][]
}

const apiTabs: Array<{ key: ApiDemoKey; label: string }> = [
  { key: 'chat', label: 'Chat' },
  { key: 'responses', label: 'Responses' },
  { key: 'claude', label: 'Claude' },
  { key: 'gemini', label: 'Gemini' }
]

const apiDemos: Record<ApiDemoKey, ApiDemo> = {
  chat: {
    method: 'POST',
    path: '/v1/chat/completions',
    tone: 'green',
    status: '200 OK',
    latency: '142',
    tokens: '27',
    cost: '$0.00081',
    request: [
      [{ text: 'curl -X POST', tone: 'command' }, { text: ' "' }, { text: '/v1/chat/completions', tone: 'yellow' }, { text: '" \\' }],
      [{ text: '  -H ', tone: 'blue' }, { text: '"Authorization: Bearer sk-••••', tone: 'yellow' }, { text: '" \\' }],
      [{ text: '  -d ', tone: 'blue' }, { text: "'{", tone: 'yellow' }],
      [{ text: '    "model": ', tone: 'cyan' }, { text: '"your-model"', tone: 'yellow' }, { text: ',' }],
      [{ text: '    "messages": ', tone: 'cyan' }, { text: '[' }],
      [{ text: '      { "role": ', tone: 'muted' }, { text: '"user"', tone: 'yellow' }, { text: ', "content": ', tone: 'muted' }, { text: '"..."', tone: 'yellow' }, { text: ' }', tone: 'muted' }],
      [{ text: '    ]' }],
      [{ text: "}'", tone: 'yellow' }]
    ],
    response: [
      [{ text: '{' }],
      [{ text: '  "choices": [{ "message": { "content": ', tone: 'cyan' }, { text: '"Chat request routed."', tone: 'green' }, { text: ' } }],', tone: 'cyan' }],
      [{ text: '  "usage": { "total_tokens": ', tone: 'cyan' }, { text: '27', tone: 'purple' }, { text: ' }', tone: 'cyan' }],
      [{ text: '}' }]
    ]
  },
  responses: {
    method: 'POST',
    path: '/v1/responses',
    tone: 'yellow',
    status: '200 OK',
    latency: '168',
    tokens: '31',
    cost: '$0.00093',
    request: [
      [{ text: 'curl -X POST', tone: 'command' }, { text: ' "' }, { text: '/v1/responses', tone: 'yellow' }, { text: '" \\' }],
      [{ text: '  -H ', tone: 'blue' }, { text: '"Authorization: Bearer sk-••••', tone: 'yellow' }, { text: '" \\' }],
      [{ text: '  -d ', tone: 'blue' }, { text: "'{", tone: 'yellow' }],
      [{ text: '    "model": ', tone: 'cyan' }, { text: '"your-model"', tone: 'yellow' }, { text: ',' }],
      [{ text: '    "input": ', tone: 'cyan' }, { text: '"..."', tone: 'yellow' }],
      [{ text: "}'", tone: 'yellow' }]
    ],
    response: [
      [{ text: '{' }],
      [{ text: '  "output": [{ "type": ', tone: 'cyan' }, { text: '"output_text"', tone: 'yellow' }, { text: ', "text": ', tone: 'cyan' }, { text: '"Response workflow ready."', tone: 'green' }, { text: ' }],', tone: 'cyan' }],
      [{ text: '  "usage": { "total_tokens": ', tone: 'cyan' }, { text: '31', tone: 'purple' }, { text: ' }', tone: 'cyan' }],
      [{ text: '}' }]
    ]
  },
  claude: {
    method: 'POST',
    path: '/v1/messages',
    tone: 'blue',
    status: '200 OK',
    latency: '156',
    tokens: '29',
    cost: '$0.00087',
    request: [
      [{ text: 'curl -X POST', tone: 'command' }, { text: ' "' }, { text: '/v1/messages', tone: 'yellow' }, { text: '" \\' }],
      [{ text: '  -H ', tone: 'blue' }, { text: '"x-api-key: sk-••••"', tone: 'yellow' }, { text: ' \\' }],
      [{ text: '  -H ', tone: 'blue' }, { text: '"anthropic-version: 2023-06-01"', tone: 'yellow' }, { text: ' \\' }],
      [{ text: '  -d ', tone: 'blue' }, { text: "'{", tone: 'yellow' }],
      [{ text: '    "model": ', tone: 'cyan' }, { text: '"your-model"', tone: 'yellow' }, { text: ',' }],
      [{ text: '    "max_tokens": ', tone: 'cyan' }, { text: '1024', tone: 'purple' }, { text: ',' }],
      [{ text: '    "messages": [{ "role": ', tone: 'cyan' }, { text: '"user"', tone: 'yellow' }, { text: ', "content": ', tone: 'cyan' }, { text: '"..."', tone: 'yellow' }, { text: ' }]', tone: 'cyan' }],
      [{ text: "}'", tone: 'yellow' }]
    ],
    response: [
      [{ text: '{' }],
      [{ text: '  "content": [{ "type": ', tone: 'cyan' }, { text: '"text"', tone: 'yellow' }, { text: ', "text": ', tone: 'cyan' }, { text: '"Claude message routed."', tone: 'green' }, { text: ' }],', tone: 'cyan' }],
      [{ text: '  "usage": { "input_tokens": ', tone: 'cyan' }, { text: '11', tone: 'purple' }, { text: ', "output_tokens": ', tone: 'cyan' }, { text: '18', tone: 'purple' }, { text: ' }', tone: 'cyan' }],
      [{ text: '}' }]
    ]
  },
  gemini: {
    method: 'POST',
    path: '/v1beta/models/{model}:generateContent',
    tone: 'purple',
    status: '200 OK',
    latency: '93',
    tokens: '25',
    cost: '$0.00075',
    request: [
      [{ text: 'curl -X POST', tone: 'command' }, { text: ' "' }, { text: '/v1beta/models/{model}:generateContent', tone: 'yellow' }, { text: '" \\' }],
      [{ text: '  -H ', tone: 'blue' }, { text: '"x-goog-api-key: sk-••••"', tone: 'yellow' }, { text: ' \\' }],
      [{ text: '  -d ', tone: 'blue' }, { text: "'{", tone: 'yellow' }],
      [{ text: '    "contents": [', tone: 'cyan' }],
      [{ text: '      { "role": ', tone: 'muted' }, { text: '"user"', tone: 'yellow' }, { text: ',' }],
      [{ text: '        "parts": [{ "text": ', tone: 'cyan' }, { text: '"..."', tone: 'yellow' }, { text: ' }] }', tone: 'cyan' }],
      [{ text: '    ]' }],
      [{ text: "}'", tone: 'yellow' }]
    ],
    response: [
      [{ text: '{' }],
      [{ text: '  "candidates": [{ "content": { "parts": [{ "text": ', tone: 'cyan' }, { text: '"Gemini request served."', tone: 'green' }, { text: ' }] } }],', tone: 'cyan' }],
      [{ text: '  "usageMetadata": { "totalTokenCount": ', tone: 'cyan' }, { text: '25', tone: 'purple' }, { text: ' }', tone: 'cyan' }],
      [{ text: '}' }]
    ]
  }
}

const activeApiTab = ref<ApiDemoKey>('chat')
const activeDemo = computed(() => apiDemos[activeApiTab.value])

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// GitHub URL
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const modelPlazaRequiresAuth = computed(
  () => appStore.cachedPublicSettings?.model_plaza_require_auth === true,
)
const showModelPlazaEntry = computed(
  () => modelPlazaEnabled.value && (isAuthenticated.value || !modelPlazaRequiresAuth.value),
)
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
.premium-home { background: #f7f9fc; color: #111827; }
.premium-home-header { border-bottom: 1px solid #e2e8f0; background: rgba(255, 255, 255, .94); backdrop-filter: blur(18px); }
.premium-home-nav { min-height: 4.25rem; }
.premium-brand { color: #111827; }
.premium-home-main { padding-top: 3.5rem; }
.premium-hero { min-height: auto; padding-top: 2.5rem; }
.premium-hero-copy { position: relative; z-index: 2; max-width: 48rem; }
.premium-eyebrow { display: inline-flex; align-items: center; gap: .55rem; margin-bottom: 1.25rem; border: 1px solid #dbe4ef; border-radius: 9999px; padding: .38rem .7rem; background: #fff; color: #475569; font-size: .72rem; font-weight: 650; }
.premium-live-dot { width: .42rem; height: .42rem; border-radius: 50%; background: #10b981; box-shadow: 0 0 0 3px rgba(16,185,129,.13); }
.premium-hero h1 { max-width: 100%; color: #0f172a; font-size: clamp(3.25rem, 7vw, 5.8rem); line-height: .98; font-weight: 760; letter-spacing: 0; overflow-wrap: anywhere; }
.premium-hero-lead { color: #1e293b; font-weight: 620; }
.premium-hero-description { max-width: 38rem; margin-inline: auto; line-height: 1.75; }
.premium-hero-copy .btn-primary { border-radius: .55rem; background: #2563eb; box-shadow: 0 12px 28px rgba(37,99,235,.22); }
.premium-hero-copy .btn-primary:hover { background: #1d4ed8; transform: translateY(-1px); }
.premium-api-demo-stage { position: relative; z-index: 2; width: min(100%, 62rem); min-width: 0; margin-top: 3.8rem; }
.premium-api-demo { overflow: hidden; border: 1px solid #dbe4ef; border-radius: 1.25rem; background: #fff; box-shadow: 0 24px 60px rgba(15,23,42,.12), 0 4px 12px rgba(15,23,42,.06); color: #334155; text-align: left; }
.premium-api-demo-tabs { display: flex; min-width: 0; min-height: 3.35rem; align-items: stretch; gap: .15rem; padding: 0 .8rem; border-bottom: 1px solid #e2e8f0; background: #f8fafc; }
.premium-api-tab { position: relative; display: inline-flex; min-height: 3.35rem; flex: 0 0 auto; align-items: center; justify-content: center; padding: 0 1.1rem; color: #64748b; font-size: .88rem; font-weight: 700; transition: color .18s ease; }
.premium-api-tab::after { position: absolute; right: .65rem; bottom: 0; left: .65rem; height: 2px; background: currentColor; content: ''; opacity: 0; transform: scaleX(.55); transition: opacity .18s ease, transform .18s ease; }
.premium-api-tab:hover, .premium-api-tab.is-active { color: #0f172a; }
.premium-api-tab.is-active::after { opacity: 1; transform: scaleX(1); }
.premium-api-tab.is-chat.is-active { color: #00d7a7; }
.premium-api-tab.is-responses.is-active { color: #facc15; }
.premium-api-tab.is-claude.is-active { color: #60a5fa; }
.premium-api-tab.is-gemini.is-active { color: #a78bfa; }
.premium-api-status { display: inline-flex; align-self: center; align-items: center; gap: .55rem; margin-left: auto; padding: 0 .55rem; color: #64748b; font-size: .76rem; font-weight: 650; white-space: nowrap; }
.premium-api-status i { width: .5rem; height: .5rem; border-radius: 50%; background: #00d7a7; box-shadow: 0 0 12px rgba(0,215,167,.45); }
.premium-api-endpoint { display: flex; min-width: 0; align-items: center; gap: .95rem; padding: 1.05rem 1.35rem; border-bottom: 1px solid #e2e8f0; }
.premium-api-method { display: inline-flex; min-width: 3.8rem; height: 2rem; align-items: center; justify-content: center; border-radius: 999px; background: rgba(0,215,167,.12); color: #00d7a7; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: .78rem; font-weight: 800; letter-spacing: .04em; }
.premium-api-method.is-yellow { background: rgba(250,204,21,.12); color: #facc15; }
.premium-api-method.is-blue { background: rgba(96,165,250,.12); color: #60a5fa; }
.premium-api-method.is-purple { background: rgba(167,139,250,.13); color: #a78bfa; }
.premium-api-endpoint code { min-width: 0; overflow: hidden; color: #334155; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: clamp(.88rem,1.4vw,1.12rem); font-weight: 650; text-overflow: ellipsis; white-space: nowrap; }
.premium-api-sections { display: grid; grid-template-rows: 17.5rem 11rem; }
.premium-api-section { min-width: 0; padding: 1.3rem 1.35rem 1.15rem; border-bottom: 1px solid #e2e8f0; }
.premium-api-response-section { background: #f8fafc; }
.premium-api-section h3 { margin: 0 0 .8rem; color: #64748b; font-size: .75rem; font-weight: 800; letter-spacing: .18em; text-transform: uppercase; }
.premium-api-code { max-width: 100%; margin: 0; overflow-x: auto; color: #334155; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: clamp(.72rem,1.45vw,.97rem); line-height: 1.68; tab-size: 2; white-space: pre; }
.premium-api-line { display: block; min-height: 1.15em; }
.premium-api-token-command { color: #059669; }
.premium-api-token-blue { color: #2563eb; }
.premium-api-token-yellow { color: #b45309; }
.premium-api-token-cyan { color: #0891b2; }
.premium-api-token-green { color: #059669; }
.premium-api-token-purple { color: #7c3aed; }
.premium-api-token-muted { color: #475569; }
.premium-api-demo-footer { display: flex; min-height: 3.2rem; align-items: center; justify-content: space-between; gap: 1rem; padding: 0 1.35rem; color: #64748b; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: .72rem; text-transform: uppercase; }
.premium-api-metrics { display: flex; min-width: 0; align-items: center; gap: .45rem; white-space: nowrap; }
.premium-api-metrics span { color: #475569; }
.premium-api-metrics small { color: #64748b; font-size: .66rem; }
.premium-api-metrics b { width: .3rem; height: .3rem; border-radius: 50%; background: #cbd5e1; }
.premium-api-stream { color: #64748b; white-space: nowrap; }
.premium-trust-row > div { border-color: #e2e8f0; border-radius: .5rem; background: rgba(255,255,255,.86); box-shadow: 0 1px 2px rgba(15,23,42,.035); }
.premium-feature-grid > div { border-color: #e2e8f0; border-radius: .65rem; background: rgba(255,255,255,.88); box-shadow: 0 1px 2px rgba(15,23,42,.035); }
.premium-feature-grid > div:hover { box-shadow: 0 12px 30px rgba(15,23,42,.07); }
.premium-feature-grid > div > div:first-child { border-radius: .5rem; background: #eff6ff; color: #2563eb; box-shadow: none; }
.premium-feature-grid > div:nth-child(2) > div:first-child { background: #ecfdf5; color: #059669; }
.premium-feature-grid > div:nth-child(3) > div:first-child { background: #fffbeb; color: #d97706; }
.premium-feature-grid > div > div:first-child svg { color: currentColor; }
.dark .premium-home { background: #0b0d12; color: #f8fafc; }
.dark .premium-home-header { border-color: #27272a; background: rgba(11,13,18,.9); }
.dark .premium-brand, .dark .premium-hero h1 { color: #f8fafc; }
.dark .premium-home-main { background: #0b0d12; }
.dark .premium-api-demo { border-color: #252d3a; background: #0b0f17; box-shadow: 0 24px 60px rgba(15,23,42,.2), 0 4px 12px rgba(15,23,42,.12); color: #cbd5e1; }
.dark .premium-api-demo-tabs { border-color: #1f2733; background: #0c111b; }
.dark .premium-api-tab:hover, .dark .premium-api-tab.is-active { color: #f8fafc; }
.dark .premium-api-status { color: #8a93a3; }
.dark .premium-api-endpoint { border-color: #1f2733; }
.dark .premium-api-endpoint code { color: #cbd5e1; }
.dark .premium-api-section { border-color: #1f2733; }
.dark .premium-api-response-section { background: rgba(17,24,39,.16); }
.dark .premium-api-section h3 { color: #6b7280; }
.dark .premium-api-code { color: #a7afbd; }
.dark .premium-api-token-command, .dark .premium-api-token-green { color: #00d7a7; }
.dark .premium-api-token-blue { color: #55a6ff; }
.dark .premium-api-token-yellow { color: #facc15; }
.dark .premium-api-token-cyan { color: #66d9ff; }
.dark .premium-api-token-purple { color: #b9a0ff; }
.dark .premium-api-token-muted { color: #a7afbd; }
.dark .premium-api-demo-footer { color: #7d8797; }
.dark .premium-api-metrics span { color: #aab2bf; }
.dark .premium-api-metrics small, .dark .premium-api-stream { color: #7d8797; }
.dark .premium-api-metrics b { background: #3b4350; }
.dark .premium-eyebrow { border-color: #30343d; background: rgba(24,24,27,.82); color: #cbd5e1; }
.dark .premium-hero-lead { color: #e2e8f0; }
.dark .premium-trust-row > div, .dark .premium-feature-grid > div { border-color: #30343d; background: rgba(24,24,27,.9); }
.dark .premium-feature-grid > div:first-child > div:first-child { background: rgba(37,99,235,.16); }
.dark .premium-feature-grid > div:nth-child(2) > div:first-child { background: rgba(5,150,105,.16); }
.dark .premium-feature-grid > div:nth-child(3) > div:first-child { background: rgba(217,119,6,.16); }
@media (max-width: 640px) { .premium-home-header { padding-inline: 1rem; } .premium-home-nav { min-height: 4rem; } .premium-home-main { padding: 2.5rem 1rem 0; } .premium-hero { padding-top: 1.75rem; } .premium-hero h1 { font-size: 3.15rem; } .premium-hero-description { font-size: .9rem; } .premium-api-demo-stage { margin-top: 2.75rem; } .premium-api-demo { border-radius: .8rem; } .premium-api-demo-tabs { overflow-x: auto; scrollbar-width: none; } .premium-api-demo-tabs::-webkit-scrollbar { display: none; } .premium-api-tab { padding-inline: .8rem; font-size: .78rem; } .premium-api-status { padding-inline: .25rem; font-size: .68rem; } .premium-api-endpoint { gap: .65rem; padding: .85rem; } .premium-api-method { min-width: 3.4rem; height: 1.8rem; font-size: .68rem; } .premium-api-endpoint code { font-size: .72rem; } .premium-api-sections { grid-template-rows: 12rem 8rem; } .premium-api-section { padding: 1rem .85rem .9rem; } .premium-api-section h3 { margin-bottom: .6rem; font-size: .67rem; } .premium-api-code { font-size: .66rem; line-height: 1.62; } .premium-api-demo-footer { min-height: 2.9rem; gap: .65rem; padding-inline: .85rem; font-size: .61rem; } .premium-api-metrics { gap: .3rem; } .premium-api-metrics small { font-size: .56rem; } .premium-api-stream { font-size: .56rem; } .premium-trust-row { justify-content: flex-start; } .premium-trust-row > div { padding: .55rem .7rem; } }
@media (prefers-reduced-motion: reduce) { .premium-feature-grid > div, .premium-hero-copy .btn-primary { transition: none; } }
</style>
