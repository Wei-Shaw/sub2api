<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div v-else class="flex min-h-screen flex-col bg-white dark:bg-dark-950">
    <!-- ==================== Nav ==================== -->
    <header class="border-b border-gray-200 dark:border-dark-800">
      <nav class="mx-auto flex max-w-5xl items-center justify-between px-6 py-3">
        <router-link to="/home" class="flex items-center gap-2">
          <div class="h-7 w-7 overflow-hidden rounded-md">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-base font-semibold text-gray-900 dark:text-white">{{ siteName }}</span>
        </router-link>
        <div class="flex items-center gap-1">
          <LocaleSwitcher />
          <router-link
            to="/guide"
            class="rounded-md px-3 py-1.5 text-sm text-gray-500 hover:bg-gray-50 hover:text-gray-900 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            {{ t('guide.title') }}
          </router-link>
          <button
            @click="toggleTheme"
            class="rounded-md p-2 text-gray-400 hover:bg-gray-50 hover:text-gray-600 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="ml-2 inline-flex items-center gap-1.5 rounded-md bg-gray-900 px-3.5 py-1.5 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.dashboard') }}
          </router-link>
          <router-link
            v-else
            to="/login"
            class="ml-2 rounded-md bg-gray-900 px-3.5 py-1.5 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="flex-1">
      <!-- ==================== Hero (compact) ==================== -->
      <section class="border-b border-gray-100 dark:border-dark-800">
        <div class="mx-auto max-w-5xl px-6 py-16 md:py-20">
          <div class="max-w-2xl">
            <p class="mb-3 text-xs font-medium tracking-widest text-gray-400 uppercase dark:text-dark-400">{{ siteSubtitle }}</p>
            <h1 class="text-3xl font-bold leading-tight text-gray-900 dark:text-white md:text-4xl">
              {{ t('home.hero.title') }}
            </h1>
            <p class="mt-4 text-base leading-relaxed text-gray-500 dark:text-dark-300 md:text-lg">
              {{ t('home.hero.desc') }}
            </p>
            <div class="mt-6 flex flex-wrap gap-3">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="inline-flex items-center gap-1.5 rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" /></svg>
              </router-link>
              <router-link
                to="/guide"
                class="inline-flex items-center rounded-md border border-gray-200 px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
              >
                {{ t('home.hero.viewGuide') }}
              </router-link>
              <router-link
                id="qiyuan-client-home-cta"
                to="/client"
                class="inline-flex items-center rounded-md border border-gray-200 px-4 py-2 text-sm font-medium text-gray-600 hover:bg-gray-50 hover:text-gray-900 dark:border-dark-700 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
              >
                {{ t('home.hero.clientCta') }}
              </router-link>
            </div>
            <div class="mt-6 flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-400 dark:text-dark-400">
              <span>Codex CLI</span>
              <span class="text-gray-200 dark:text-dark-600">/</span>
              <span>Claude Code</span>
              <span class="text-gray-200 dark:text-dark-600">/</span>
              <span>OpenAI SDK</span>
              <span class="text-gray-200 dark:text-dark-600">/</span>
              <span>Cursor</span>
            </div>
          </div>
        </div>
      </section>

      <!-- ==================== Pricing (front and center) ==================== -->
      <section class="border-b border-gray-100 dark:border-dark-800">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <div class="mb-10 flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
            <div>
              <p class="mb-1 text-xs font-medium tracking-widest text-gray-400 uppercase dark:text-dark-400">{{ t('home.pricing.label') }}</p>
              <h2 class="text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.pricing.title') }}</h2>
              <p class="mt-2 text-sm text-gray-500 dark:text-dark-300">{{ t('home.pricing.subtitle') }}</p>
            </div>
            <div v-if="startingPrice !== null" class="shrink-0 rounded-md border border-gray-200 px-4 py-2.5 dark:border-dark-700">
              <span class="text-sm text-gray-500 dark:text-dark-300">{{ t('home.pricing.from') }}</span>
              <span class="ml-1 text-xl font-bold text-gray-900 dark:text-white">¥{{ formatPrice(startingPrice) }}</span>
              <span class="text-sm text-gray-500 dark:text-dark-300">/{{ startingPlanDays }}天</span>
            </div>
          </div>

          <div v-if="plansLoading" class="grid gap-4 md:grid-cols-3 lg:grid-cols-5">
            <div v-for="idx in 5" :key="idx" class="h-52 animate-pulse rounded-lg border border-gray-100 bg-gray-50 dark:border-dark-800 dark:bg-dark-900"></div>
          </div>

          <div v-else-if="visiblePlans.length" class="grid gap-4 md:grid-cols-3 lg:grid-cols-5">
            <article
              v-for="plan in visiblePlans"
              :key="plan.id"
              class="relative rounded-lg border p-5 transition-colors dark:bg-dark-900"
              :class="plan.id === recommendedPlanId ? 'border-gray-900 dark:border-white' : 'border-gray-200 dark:border-dark-700'"
            >
              <div
                v-if="plan.id === recommendedPlanId"
                class="absolute -right-1.5 -top-2.5 rounded bg-gray-900 px-2 py-0.5 text-[10px] font-medium text-white dark:bg-white dark:text-gray-900"
              >
                {{ t('home.pricing.recommended') }}
              </div>
              <h3 class="text-base font-semibold text-gray-900 dark:text-white">{{ plan.name }}</h3>
              <div class="mt-4">
                <p v-if="plan.original_price" class="text-xs text-gray-400 line-through">¥{{ formatPrice(plan.original_price) }}</p>
                <div class="mt-0.5 flex items-baseline gap-0.5">
                  <span class="text-2xl font-bold text-gray-900 dark:text-white">¥{{ formatPrice(plan.price) }}</span>
                  <span class="text-xs text-gray-400">/{{ plan.validity_days === 1 ? '天' : plan.validity_days + '天' }}</span>
                </div>
                <p v-if="formatSavings(plan)" class="mt-1 text-[11px] font-medium text-red-500">{{ t('home.pricing.save', { amount: formatSavings(plan) }) }}</p>
              </div>
              <ul class="mt-4 space-y-1.5">
                <li
                  v-for="feature in parsePlanFeatures(plan.features).slice(0, 2)"
                  :key="feature"
                  class="flex gap-2 text-xs leading-relaxed text-gray-500 dark:text-dark-300"
                >
                  <span class="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-gray-900 dark:bg-white"></span>
                  <span>{{ feature }}</span>
                </li>
              </ul>
              <router-link
                :to="purchaseLink"
                class="mt-5 block w-full rounded-md border py-2 text-center text-xs font-medium transition-colors"
                :class="plan.id === recommendedPlanId
                  ? 'border-gray-900 bg-gray-900 text-white hover:bg-gray-800 dark:border-white dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100'
                  : 'border-gray-200 bg-white text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700'"
              >
                {{ t('home.pricing.cta') }}
              </router-link>
            </article>
          </div>

          <div class="mt-6 text-center">
            <a
              href="https://qm.qq.com/q/IgQv6V7SEw"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center gap-1.5 text-sm font-medium text-primary-500 hover:text-primary-600 dark:text-primary-400"
            >
              加群领取限时优惠额度
              <svg class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" /></svg>
            </a>
          </div>
        </div>
      </section>

      <!-- ==================== Community QQ Group ==================== -->
      <section class="border-b border-gray-100 bg-gray-50/50 dark:border-dark-800 dark:bg-dark-900/30">
        <div class="mx-auto max-w-5xl px-6 py-14">
          <div class="flex flex-col items-center gap-8 md:flex-row md:items-start">
            <div class="flex-1 text-center md:text-left">
              <p class="text-xs font-medium tracking-widest text-gray-400 uppercase dark:text-dark-400">Community</p>
              <h2 class="mt-2 text-xl font-bold text-gray-900 dark:text-white">加入卡卡AI 用户群</h2>
              <p class="mt-3 text-sm text-gray-500 dark:text-dark-300">
                限时不定期发放优惠额度，第一时间获取功能更新和技术答疑
              </p>
              <div class="mt-4 flex flex-wrap justify-center gap-4 text-xs text-gray-400 dark:text-dark-400 md:justify-start">
                <span>优惠福利</span>
                <span>更新通知</span>
                <span>使用答疑</span>
              </div>
              <a
                href="https://qm.qq.com/q/IgQv6V7SEw"
                target="_blank"
                rel="noopener noreferrer"
                class="mt-6 inline-flex items-center gap-2 rounded-md bg-gray-900 px-4 py-2 text-sm font-medium text-white hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
              >
                加入 QQ 群 774692252
                <svg class="h-3.5 w-3.5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" /></svg>
              </a>
            </div>
            <div class="w-full max-w-[200px] shrink-0 overflow-hidden rounded-lg border border-gray-200 bg-white p-2 dark:border-dark-700">
              <img
                src="/aftersales-qq-group.jpg"
                alt="扫码加入卡卡AI用户群"
                class="w-full object-contain"
              />
              <p class="mt-1.5 text-center text-[10px] text-gray-400">扫码加入群聊</p>
            </div>
          </div>
        </div>
      </section>

      <!-- ==================== Features ==================== -->
      <section class="border-b border-gray-100 dark:border-dark-800">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <p class="mb-1 text-center text-xs font-medium tracking-widest text-gray-400 uppercase dark:text-dark-400">{{ t('home.solutions.title') }}</p>
          <h2 class="mb-10 text-center text-xl font-bold text-gray-900 dark:text-white md:text-2xl">
            {{ t('home.hero.title') }}
          </h2>
          <div class="grid gap-6 md:grid-cols-3">
            <div class="rounded-lg border border-gray-200 p-5 dark:border-dark-700">
              <span class="text-xs font-bold text-primary-500">01</span>
              <h3 class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.unifiedGateway') }}
              </h3>
              <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-300">
                {{ t('home.features.unifiedGatewayDesc') }}
              </p>
            </div>
            <div class="rounded-lg border border-gray-200 p-5 dark:border-dark-700">
              <span class="text-xs font-bold text-primary-500">02</span>
              <h3 class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.multiAccount') }}
              </h3>
              <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-300">
                {{ t('home.features.multiAccountDesc') }}
              </p>
            </div>
            <div class="rounded-lg border border-gray-200 p-5 dark:border-dark-700">
              <span class="text-xs font-bold text-primary-500">03</span>
              <h3 class="mt-2 text-sm font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.balanceQuota') }}
              </h3>
              <p class="mt-2 text-xs leading-relaxed text-gray-500 dark:text-dark-300">
                {{ t('home.features.balanceQuotaDesc') }}
              </p>
            </div>
          </div>
        </div>
      </section>

      <!-- ==================== Quick Config ==================== -->
      <section class="border-b border-gray-100 dark:border-dark-800">
        <div class="mx-auto max-w-5xl px-6 py-16">
          <p class="mb-1 text-center text-xs font-medium tracking-widest text-gray-400 uppercase dark:text-dark-400">{{ t('home.quickConfig.label') }}</p>
          <h2 class="mb-2 text-center text-xl font-bold text-gray-900 dark:text-white md:text-2xl">{{ t('home.quickConfig.title') }}</h2>
          <p class="mb-8 text-center text-sm text-gray-500 dark:text-dark-300">{{ t('home.quickConfig.subtitle') }}</p>
          <div class="grid gap-4 md:grid-cols-2">
            <!-- Codex / SDK -->
            <div class="rounded-lg border border-gray-200 bg-gray-900 p-5 dark:border-dark-700 dark:bg-dark-900">
              <h3 class="mb-4 text-xs font-semibold tracking-wider text-gray-300 uppercase">{{ t('home.quickConfig.codexTitle') }}</h3>
              <dl class="space-y-3">
                <div class="flex items-center justify-between">
                  <dt class="text-[10px] uppercase tracking-wider text-gray-500">Base URL</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-xs text-gray-200">https://api.kaiaigo.com/v1</code>
                    <button @click="copy('https://api.kaiaigo.com/v1')" class="text-[10px] text-primary-400 hover:text-primary-300">{{ copied === 'codex-url' ? 'OK' : 'Copy' }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-[10px] uppercase tracking-wider text-gray-500">API Key</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-xs text-gray-200">sk-your-key</code>
                    <button class="text-[10px] text-gray-500">{{ t('home.quickConfig.createKey') }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-[10px] uppercase tracking-wider text-gray-500">Model</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-xs text-gray-200">gpt-5.5</code>
                    <button @click="copy('gpt-5.5')" class="text-[10px] text-primary-400 hover:text-primary-300">{{ copied === 'codex-model' ? 'OK' : 'Copy' }}</button>
                  </dd>
                </div>
              </dl>
            </div>
            <!-- Claude Code -->
            <div class="rounded-lg border border-gray-200 bg-gray-900 p-5 dark:border-dark-700 dark:bg-dark-900">
              <h3 class="mb-4 text-xs font-semibold tracking-wider text-gray-300 uppercase">{{ t('home.quickConfig.claudeTitle') }}</h3>
              <dl class="space-y-3">
                <div class="flex items-center justify-between">
                  <dt class="text-[10px] uppercase tracking-wider text-gray-500">Base URL</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-xs text-gray-200">https://api.kaiaigo.com</code>
                    <button @click="copy('https://api.kaiaigo.com')" class="text-[10px] text-primary-400 hover:text-primary-300">{{ copied === 'claude-url' ? 'OK' : 'Copy' }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-[10px] uppercase tracking-wider text-gray-500">API Key</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-xs text-gray-200">sk-your-key</code>
                    <button class="text-[10px] text-gray-500">{{ t('home.quickConfig.createKey') }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-[10px] uppercase tracking-wider text-gray-500">Model</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-xs text-gray-200">claude-sonnet-4-6</code>
                    <button @click="copy('claude-sonnet-4-6')" class="text-[10px] text-primary-400 hover:text-primary-300">{{ copied === 'claude-model' ? 'OK' : 'Copy' }}</button>
                  </dd>
                </div>
              </dl>
            </div>
          </div>
          <p class="mt-3 text-center text-[11px] text-gray-400">
            {{ t('home.quickConfig.note') }}
          </p>
        </div>
      </section>

      <!-- ==================== FAQ ==================== -->
      <section class="border-b border-gray-100 dark:border-dark-800">
        <div class="mx-auto max-w-3xl px-6 py-16">
          <h2 class="mb-8 text-center text-xl font-bold text-gray-900 dark:text-white">{{ t('guide.faq.title') }}</h2>
          <div class="space-y-2">
            <details
              v-for="(item, idx) in faqItems"
              :key="idx"
              class="group rounded-lg border border-gray-200 transition-colors open:border-gray-900 dark:border-dark-700 dark:open:border-white"
            >
              <summary class="flex cursor-pointer select-none items-center justify-between px-4 py-3.5 text-sm font-medium text-gray-700 dark:text-dark-200">
                {{ item.question }}
                <svg class="h-3.5 w-3.5 shrink-0 text-gray-400 transition-transform group-open:rotate-180" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </summary>
              <div class="border-t border-gray-100 px-4 py-3 text-sm leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-300">
                {{ item.answer }}
              </div>
            </details>
          </div>
        </div>
      </section>

      <!-- ==================== CTA ==================== -->
      <section class="py-16 text-center">
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="inline-flex items-center gap-2 rounded-md bg-gray-900 px-6 py-3 text-sm font-medium text-white hover:bg-gray-800"
        >
          {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
            <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" />
          </svg>
        </router-link>
      </section>
    </main>

    <!-- ==================== Footer ==================== -->
    <footer class="border-t border-gray-200 px-6 py-6 dark:border-dark-800">
      <div class="mx-auto flex max-w-5xl flex-col items-center justify-between gap-3 sm:flex-row">
        <p class="text-xs text-gray-400 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}
        </p>
        <div class="flex items-center gap-4">
          <router-link to="/guide" class="text-xs text-gray-400 hover:text-gray-600 dark:text-dark-400 dark:hover:text-white">
            {{ t('guide.title') }}
          </router-link>
          <a
            href="https://qm.qq.com/q/IgQv6V7SEw"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-gray-400 hover:text-gray-600 dark:text-dark-400 dark:hover:text-white"
          >
            QQ 群
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-gray-400 hover:text-gray-600 dark:text-dark-400 dark:hover:text-white"
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
import { paymentAPI } from '@/api/payment'
import type { SubscriptionPlan } from '@/types/payment'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '卡卡AI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

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
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const purchaseLink = computed(() => (
  isAuthenticated.value
    ? { path: '/purchase', query: { tab: 'subscription' } }
    : { path: '/login', query: { redirect: '/purchase?tab=subscription' } }
))

const currentYear = computed(() => new Date().getFullYear())

const subscriptionPlans = ref<SubscriptionPlan[]>([])
const plansLoading = ref(false)

const visiblePlans = computed(() => subscriptionPlans.value.slice(0, 5))
const recommendedPlanId = computed(() => {
  const advanced = subscriptionPlans.value.find((plan) => plan.name === '进阶')
  return advanced?.id ?? subscriptionPlans.value[Math.min(2, subscriptionPlans.value.length - 1)]?.id
})
const startingPrice = computed(() => {
  if (!subscriptionPlans.value.length) return null
  return Math.min(...subscriptionPlans.value.map((plan) => Number(plan.price) || 0).filter((price) => price > 0))
})
const startingPlanDays = computed(() => {
  if (!subscriptionPlans.value.length) return 30
  const cheapest = subscriptionPlans.value.reduce((min, p) => (Number(p.price) || Infinity) < (Number(min.price) || Infinity) ? p : min, subscriptionPlans.value[0])
  return cheapest.validity_days || 1
})

function parsePlanFeatures(features: SubscriptionPlan['features'] | string | undefined): string[] {
  if (Array.isArray(features)) {
    return features.filter(Boolean)
  }
  if (typeof features === 'string') {
    return features.split('\n').map((line) => line.trim()).filter(Boolean)
  }
  return []
}

function normalizePlan(plan: SubscriptionPlan): SubscriptionPlan {
  return {
    ...plan,
    features: parsePlanFeatures(plan.features)
  }
}

async function fetchSubscriptionPlans() {
  plansLoading.value = true
  try {
    const response = await paymentAPI.getPublicPlans()
    subscriptionPlans.value = (response.data || []).map(normalizePlan)
  } catch (error) {
    console.warn('[home] Failed to load public subscription plans:', error)
    subscriptionPlans.value = []
  } finally {
    plansLoading.value = false
  }
}

function formatPrice(value: number | string | undefined | null): string {
  const numeric = Number(value)
  if (!Number.isFinite(numeric)) return '-'
  return numeric.toFixed(numeric % 1 === 0 ? 0 : 2)
}

function formatSavings(plan: SubscriptionPlan): string | null {
  const original = Number(plan.original_price || 0)
  const price = Number(plan.price || 0)
  if (!original || original <= price) return null
  return formatPrice(original - price)
}

// Copy
const copied = ref('')
async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    copied.value = text
    setTimeout(() => { copied.value = '' }, 1500)
  } catch { /* fallback */ }
}

// FAQ
const faqItems = computed(() => [
  { question: t('guide.faq.q1'), answer: t('guide.faq.a1') },
  { question: t('guide.faq.q2'), answer: t('guide.faq.a2') },
  { question: t('guide.faq.q3'), answer: t('guide.faq.a3') },
  { question: t('guide.faq.q4'), answer: t('guide.faq.a4') },
  { question: t('guide.faq.q5'), answer: t('guide.faq.a5') },
  { question: t('guide.faq.q6'), answer: t('guide.faq.a6') }
])

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

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
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  fetchSubscriptionPlans()
})
</script>
