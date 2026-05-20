<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
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
    class="relative flex min-h-screen flex-col overflow-hidden bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -right-40 -top-40 h-96 w-96 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/3 top-1/4 h-72 w-72 rounded-full bg-primary-300/10 blur-3xl"
      ></div>
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(212,165,116,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(212,165,116,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- ==================== Nav ==================== -->
    <header class="relative z-20 border-b border-gray-200/50 bg-white/40 backdrop-blur-md dark:border-dark-800/50 dark:bg-dark-950/40">
      <nav class="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <!-- Logo -->
        <router-link to="/home" class="flex items-center gap-2.5">
          <div class="h-8 w-8 overflow-hidden rounded-lg">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-lg font-bold text-gray-900 dark:text-white">{{ siteName }}</span>
        </router-link>

        <!-- Nav Actions -->
        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg px-3 py-1.5 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
          <router-link
            to="/guide"
            class="rounded-lg px-3 py-1.5 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            {{ t('guide.title') }}
          </router-link>
          <button
            @click="toggleTheme"
            class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-primary-400 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            {{ t('home.dashboard') }}
          </router-link>
          <router-link
            v-else
            to="/login"
            class="inline-flex items-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- ==================== Hero: Left/Right Layout ==================== -->
    <main class="relative z-10 flex-1 px-6 py-20">
      <div class="mx-auto max-w-6xl">
        <div class="flex flex-col items-center gap-16 lg:flex-row lg:gap-20">
          <!-- Left: Copy -->
          <div class="flex-1 text-center lg:text-left">
            <p class="mb-3 text-sm font-medium tracking-wider text-primary-500 uppercase">{{ siteSubtitle }}</p>
            <h1 class="mb-5 text-4xl font-bold leading-tight text-gray-900 dark:text-white md:text-5xl lg:text-[3.25rem]">
              {{ t('home.hero.title') }}
            </h1>
            <p class="mb-8 max-w-lg text-lg leading-relaxed text-gray-500 dark:text-dark-300 lg:text-xl">
              {{ t('home.hero.desc') }}
            </p>
            <div class="flex flex-wrap items-center justify-center gap-3 lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="inline-flex items-center rounded-lg bg-gray-900 px-6 py-3 text-sm font-medium text-white transition-colors hover:bg-gray-800"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <svg class="ml-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                  <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" />
                </svg>
              </router-link>
              <router-link
                to="/guide"
                class="inline-flex items-center rounded-lg border border-gray-300 bg-white px-6 py-3 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
              >
                {{ t('home.hero.viewGuide') }}
              </router-link>
              <router-link
                id="qiyuan-client-home-cta"
                to="/client"
                class="inline-flex items-center rounded-lg border border-gray-300 bg-white px-6 py-3 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
              >
                {{ t('home.hero.clientCta') }}
              </router-link>
            </div>
            <p class="mt-6 text-xs text-gray-400 dark:text-dark-300">
              Codex Plus · Linux · Windows · macOS · 账号池调度
            </p>
          </div>

          <!-- Right: Console Summary Card -->
          <div class="flex flex-1 justify-center lg:justify-end">
            <div class="console-card w-full max-w-md">
              <!-- Card Header -->
              <div class="flex items-center justify-between border-b border-white/10 px-5 py-3">
                <div class="flex items-center gap-2">
                  <div class="h-2.5 w-2.5 rounded-full bg-red-400"></div>
                  <div class="h-2.5 w-2.5 rounded-full bg-yellow-400"></div>
                  <div class="h-2.5 w-2.5 rounded-full bg-green-400"></div>
                </div>
                <span class="font-mono text-xs text-gray-400">api.mcorgai.com</span>
              </div>
              <!-- Card Body -->
              <div class="px-5 py-4">
                <h3 class="mb-1 text-sm font-semibold text-white">{{ t('home.console.title') }}</h3>
                <p class="mb-5 text-xs text-gray-400">{{ t('home.console.subtitle') }}</p>
                <!-- Stats Grid -->
                <div class="grid grid-cols-2 gap-4">
                  <div>
                    <p class="text-[10px] uppercase tracking-wider text-gray-500">{{ t('home.console.baseUrl') }}</p>
                    <p class="mt-1 font-mono text-sm font-semibold text-primary-400">/v1</p>
                  </div>
                  <div>
                    <p class="text-[10px] uppercase tracking-wider text-gray-500">{{ t('home.console.models') }}</p>
                    <p class="mt-1 font-mono text-sm font-semibold text-white">GPT-5.5</p>
                  </div>
                  <div>
                    <p class="text-[10px] uppercase tracking-wider text-gray-500">{{ t('home.console.accounts') }}</p>
                    <p class="mt-1 font-mono text-sm font-semibold text-white">2</p>
                  </div>
                  <div>
                    <p class="text-[10px] uppercase tracking-wider text-gray-500">{{ t('home.console.protocol') }}</p>
                    <p class="mt-1 font-mono text-sm font-semibold text-white">OAuth</p>
                  </div>
                </div>
                <!-- Tool Status -->
                <div class="mt-5 space-y-2.5 border-t border-white/10 pt-4">
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-medium text-gray-300">Codex CLI</span>
                    <span class="flex items-center gap-1.5 text-xs">
                      <span class="h-1.5 w-1.5 rounded-full bg-green-400"></span>
                      <span class="text-green-400">Ready</span>
                    </span>
                  </div>
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-medium text-gray-300">OpenAI SDK</span>
                    <span class="flex items-center gap-1.5 text-xs">
                      <span class="h-1.5 w-1.5 rounded-full bg-green-400"></span>
                      <span class="text-green-400">Ready</span>
                    </span>
                  </div>
                  <div class="flex items-center justify-between">
                    <span class="text-xs font-medium text-gray-300">Claude Code</span>
                    <span class="flex items-center gap-1.5 text-xs">
                      <span class="h-1.5 w-1.5 rounded-full bg-green-400"></span>
                      <span class="text-green-400">Ready</span>
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- ==================== Core Features: 3-col Numbered Cards ==================== -->
        <section class="mt-24">
            <p class="mb-2 text-center text-sm font-medium tracking-wider text-primary-500 uppercase">{{ t('home.solutions.title') }}</p>
          <h2 class="mb-12 text-center text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
            {{ t('home.hero.title') }}
          </h2>
          <div class="grid gap-8 md:grid-cols-3">
            <!-- 01 -->
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-lg hover:shadow-primary-500/5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <span class="mb-4 inline-block bg-gradient-to-r from-primary-400 to-primary-500 bg-clip-text text-3xl font-black text-transparent">01</span>
              <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.unifiedGateway') }}
              </h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">
                {{ t('home.features.unifiedGatewayDesc') }}
              </p>
            </div>
            <!-- 02 -->
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-lg hover:shadow-primary-500/5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <span class="mb-4 inline-block bg-gradient-to-r from-primary-400 to-primary-500 bg-clip-text text-3xl font-black text-transparent">02</span>
              <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.multiAccount') }}
              </h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">
                {{ t('home.features.multiAccountDesc') }}
              </p>
            </div>
            <!-- 03 -->
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-lg hover:shadow-primary-500/5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <span class="mb-4 inline-block bg-gradient-to-r from-primary-400 to-primary-500 bg-clip-text text-3xl font-black text-transparent">03</span>
              <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
                {{ t('home.features.balanceQuota') }}
              </h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">
                {{ t('home.features.balanceQuotaDesc') }}
              </p>
            </div>
          </div>
        </section>

        <!-- ==================== Supported Tools: Tool Tags ==================== -->
        <section class="mt-16">
          <div class="flex flex-wrap items-center justify-center gap-4">
            <div class="flex items-center gap-2 rounded-lg border border-gray-200/50 bg-white/60 px-4 py-2.5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <svg class="h-4 w-4 text-primary-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3l-4.5 16.5" />
              </svg>
              <span class="text-sm font-medium text-gray-700 dark:text-dark-200">Codex CLI</span>
              <span class="text-xs text-gray-400">{{ t('home.providers.supported') }}</span>
            </div>
            <div class="flex items-center gap-2 rounded-lg border border-gray-200/50 bg-white/60 px-4 py-2.5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <svg class="h-4 w-4 text-primary-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M9.75 17L9 20l-1 1h8l-1-1-.75-3M3 13h18M5 17h14a2 2 0 002-2V5a2 2 0 00-2-2H5a2 2 0 00-2 2v10a2 2 0 002 2z" />
              </svg>
              <span class="text-sm font-medium text-gray-700 dark:text-dark-200">Claude Code</span>
              <span class="text-xs text-gray-400">{{ t('home.providers.supported') }}</span>
            </div>
            <div class="flex items-center gap-2 rounded-lg border border-gray-200/50 bg-white/60 px-4 py-2.5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <svg class="h-4 w-4 text-primary-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M15.232 5.232l3.536 3.536m-2.036-5.036a2.5 2.5 0 113.536 3.536L6.5 21.036H3v-3.572L16.732 3.732z" />
              </svg>
              <span class="text-sm font-medium text-gray-700 dark:text-dark-200">Cursor</span>
              <span class="text-xs text-gray-400">{{ t('home.providers.supported') }}</span>
            </div>
            <div class="flex items-center gap-2 rounded-lg border border-gray-200/50 bg-white/60 px-4 py-2.5 dark:border-dark-700/50 dark:bg-dark-800/60">
              <svg class="h-4 w-4 text-primary-500" fill="none" stroke="currentColor" viewBox="0 0 24 24" stroke-width="1.5">
                <path stroke-linecap="round" stroke-linejoin="round" d="M10 20l4-16m4 4l4 4-4 4M6 16l-4-4 4-4" />
              </svg>
              <span class="text-sm font-medium text-gray-700 dark:text-dark-200">OpenAI SDK</span>
              <span class="text-xs text-gray-400">{{ t('home.providers.supported') }}</span>
            </div>
          </div>
        </section>

        <!-- ==================== Pricing Guide ==================== -->
        <section class="mt-24">
          <div class="rounded-3xl border border-gray-200 bg-white/80 p-6 backdrop-blur-sm dark:border-dark-700 dark:bg-dark-900/80 md:p-8">
            <div class="flex flex-col gap-6 lg:flex-row lg:items-end lg:justify-between">
              <div>
                <p class="mb-3 text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.pricing.label') }}</p>
                <h2 class="text-3xl font-bold tracking-tight text-gray-900 dark:text-white md:text-4xl">
                  {{ t('home.pricing.title') }}
                </h2>
                <p class="mt-3 max-w-2xl text-sm leading-relaxed text-gray-500 dark:text-dark-300 md:text-base">
                  {{ t('home.pricing.subtitle') }}
                </p>
              </div>
              <div class="flex flex-col gap-3 sm:flex-row lg:items-center">
                <div
                  v-if="startingPrice !== null"
                  class="rounded-2xl border border-gray-200 bg-gray-50 px-4 py-3 text-sm text-gray-500 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-300"
                >
                  <span>{{ t('home.pricing.from') }}</span>
                  <span class="ml-1 text-2xl font-bold text-gray-900 dark:text-white">¥{{ formatPrice(startingPrice) }}</span>
                  <span class="ml-1">/ 30天</span>
                </div>
                <router-link
                  :to="isAuthenticated ? '/payment' : '/login?redirect=/payment'"
                  class="inline-flex items-center justify-center rounded-xl border border-gray-900 bg-white px-5 py-3 text-sm font-medium text-gray-900 transition-colors hover:bg-gray-50 dark:border-dark-200 dark:bg-dark-900 dark:text-white dark:hover:bg-dark-800"
                >
                  {{ t('home.pricing.cta') }}
                </router-link>
              </div>
            </div>

            <div v-if="plansLoading" class="mt-8 grid gap-4 md:grid-cols-5">
              <div v-for="idx in 5" :key="idx" class="h-44 animate-pulse rounded-2xl border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"></div>
            </div>

            <div v-else-if="visiblePlans.length" class="mt-8 grid gap-4 md:grid-cols-2 lg:grid-cols-5">
              <article
                v-for="plan in visiblePlans"
                :key="plan.id"
                class="relative rounded-2xl border bg-white p-5 transition-colors dark:bg-dark-900"
                :class="plan.id === recommendedPlanId ? 'border-gray-900 dark:border-white' : 'border-gray-200 dark:border-dark-700'"
              >
                <div
                  v-if="plan.id === recommendedPlanId"
                  class="absolute -right-2 -top-3 rounded-full bg-green-100 px-3 py-1 text-xs font-medium text-green-700 dark:bg-green-900/40 dark:text-green-300"
                >
                  {{ t('home.pricing.recommended') }}
                </div>
                <div class="flex items-start justify-between gap-3">
                  <h3 class="text-lg font-semibold text-gray-900 dark:text-white">{{ plan.name }}</h3>
                  <span class="rounded-full border border-gray-200 px-2 py-0.5 text-[11px] text-gray-500 dark:border-dark-700 dark:text-dark-300">OpenAI</span>
                </div>
                <div class="mt-5">
                  <p v-if="plan.original_price" class="text-sm text-gray-400 line-through">¥{{ formatPrice(plan.original_price) }}</p>
                  <div class="mt-1 flex items-end gap-1">
                    <span class="text-3xl font-bold text-gray-900 dark:text-white">¥{{ formatPrice(plan.price) }}</span>
                    <span class="pb-1 text-sm text-gray-500 dark:text-dark-300">/30天</span>
                  </div>
                  <p v-if="formatSavings(plan)" class="mt-1 text-xs font-medium text-red-500">{{ t('home.pricing.save', { amount: formatSavings(plan) }) }}</p>
                </div>
                <p class="mt-4 line-clamp-3 text-sm leading-relaxed text-gray-500 dark:text-dark-300">
                  {{ plan.description }}
                </p>
                <ul class="mt-4 space-y-2">
                  <li
                    v-for="feature in parsePlanFeatures(plan.features).slice(0, 2)"
                    :key="feature"
                    class="flex gap-2 text-xs leading-relaxed text-gray-500 dark:text-dark-300"
                  >
                    <span class="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-gray-900 dark:bg-white"></span>
                    <span>{{ feature }}</span>
                  </li>
                </ul>
              </article>
            </div>
          </div>
        </section>

        <!-- ==================== Why Choose: 2x2 Grid ==================== -->
        <section class="mt-24">
          <p class="mb-2 text-center text-sm font-medium tracking-wider text-primary-500 uppercase">{{ t('home.why.sectionLabel') }}</p>
          <h2 class="mb-12 text-center text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
            {{ t('home.why.sectionTitle') }}
          </h2>
          <div class="grid gap-6 md:grid-cols-2">
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/60">
              <h3 class="mb-2 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.why.security') }}</h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">{{ t('home.why.securityDesc') }}</p>
            </div>
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/60">
              <h3 class="mb-2 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.why.stability') }}</h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">{{ t('home.why.stabilityDesc') }}</p>
            </div>
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/60">
              <h3 class="mb-2 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.why.transparency') }}</h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">{{ t('home.why.transparencyDesc') }}</p>
            </div>
            <div class="rounded-xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/60">
              <h3 class="mb-2 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.why.devFirst') }}</h3>
              <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">{{ t('home.why.devFirstDesc') }}</p>
            </div>
          </div>
        </section>

        <!-- ==================== Quick Config: Dark Panels ==================== -->
        <section class="mt-24">
          <p class="mb-2 text-center text-sm font-medium tracking-wider text-primary-500 uppercase">{{ t('home.quickConfig.label') }}</p>
          <h2 class="mb-4 text-center text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">
            {{ t('home.quickConfig.title') }}
          </h2>
          <p class="mb-10 text-center text-sm text-gray-500 dark:text-dark-300">{{ t('home.quickConfig.subtitle') }}</p>
          <div class="grid gap-6 md:grid-cols-2">
            <!-- Codex / SDK Panel -->
            <div class="config-panel rounded-xl p-5">
              <h3 class="mb-4 text-sm font-semibold text-white">{{ t('home.quickConfig.codexTitle') }}</h3>
              <dl class="space-y-3">
                <div class="flex items-center justify-between">
                  <dt class="text-xs uppercase tracking-wider text-gray-400">Base URL</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-sm text-gray-200">https://api.mcorgai.com/v1</code>
                    <button @click="copy('https://api.mcorgai.com/v1')" class="copy-btn text-xs text-primary-400 hover:text-primary-300">{{ copied === 'codex-url' ? '✓' : '复制' }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-xs uppercase tracking-wider text-gray-400">API Key</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-sm text-gray-200">sk-your-key</code>
                    <button class="text-xs text-gray-500">{{ t('home.quickConfig.createKey') }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-xs uppercase tracking-wider text-gray-400">Model</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-sm text-gray-200">gpt-5.5</code>
                    <button @click="copy('gpt-5.5')" class="copy-btn text-xs text-primary-400 hover:text-primary-300">{{ copied === 'codex-model' ? '✓' : '复制' }}</button>
                  </dd>
                </div>
              </dl>
            </div>
            <!-- Claude Code Panel -->
            <div class="config-panel rounded-xl p-5">
              <h3 class="mb-4 text-sm font-semibold text-white">{{ t('home.quickConfig.claudeTitle') }}</h3>
              <dl class="space-y-3">
                <div class="flex items-center justify-between">
                  <dt class="text-xs uppercase tracking-wider text-gray-400">Base URL</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-sm text-gray-200">https://api.mcorgai.com</code>
                    <button @click="copy('https://api.mcorgai.com')" class="copy-btn text-xs text-primary-400 hover:text-primary-300">{{ copied === 'claude-url' ? '✓' : '复制' }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-xs uppercase tracking-wider text-gray-400">API Key</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-sm text-gray-200">sk-your-key</code>
                    <button class="text-xs text-gray-500">{{ t('home.quickConfig.createKey') }}</button>
                  </dd>
                </div>
                <div class="flex items-center justify-between">
                  <dt class="text-xs uppercase tracking-wider text-gray-400">Model</dt>
                  <dd class="flex items-center gap-2">
                    <code class="font-mono text-sm text-gray-200">claude-sonnet-4-6</code>
                    <button @click="copy('claude-sonnet-4-6')" class="copy-btn text-xs text-primary-400 hover:text-primary-300">{{ copied === 'claude-model' ? '✓' : '复制' }}</button>
                  </dd>
                </div>
              </dl>
            </div>
          </div>
          <p class="mt-4 text-center text-xs text-gray-400">
            {{ t('home.quickConfig.note') }}
          </p>
        </section>

        <!-- ==================== FAQ ==================== -->
        <section class="mt-24">
          <h2 class="mb-8 text-center text-2xl font-bold text-gray-900 dark:text-white">{{ t('guide.faq.title') }}</h2>
          <div class="mx-auto max-w-3xl space-y-3">
            <details
              v-for="(item, idx) in faqItems"
              :key="idx"
              class="group rounded-xl border border-gray-200/50 bg-white/60 backdrop-blur-sm transition-all hover:shadow-md dark:border-dark-700/50 dark:bg-dark-800/60 [&[open]]:border-primary-200 [&[open]]:shadow-lg [&[open]]:shadow-primary-500/5"
            >
              <summary class="flex cursor-pointer select-none items-center justify-between px-5 py-4 text-sm font-medium text-gray-800 dark:text-dark-200">
                {{ item.question }}
                <svg class="h-4 w-4 shrink-0 text-gray-400 transition-transform group-open:rotate-180" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
                </svg>
              </summary>
              <div class="border-t border-gray-100 px-5 py-4 text-sm leading-relaxed text-gray-500 dark:border-dark-700 dark:text-dark-300">
                {{ item.answer }}
              </div>
            </details>
          </div>
        </section>

        <!-- ==================== CTA ==================== -->
        <section class="mt-24 text-center">
          <h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">{{ t('home.cta.title') }}</h2>
            <p class="mb-6 text-gray-500 dark:text-dark-300">{{ t('home.cta.description') }}</p>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex items-center rounded-lg bg-gray-900 px-8 py-3 text-sm font-medium text-white transition-colors hover:bg-gray-800"
          >
            {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
            <svg class="ml-2 h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
              <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" />
            </svg>
          </router-link>
        </section>
      </div>
    </main>

    <!-- ==================== Footer ==================== -->
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 sm:flex-row">
        <p class="text-sm text-gray-400 dark:text-dark-300">
          &copy; {{ currentYear }} {{ siteName }}
        </p>
        <div class="flex items-center gap-4">
          <router-link to="/guide" class="text-sm text-gray-400 transition-colors hover:text-gray-600 dark:text-dark-300 dark:hover:text-white">
            {{ t('guide.title') }}
          </router-link>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-400 transition-colors hover:text-gray-600 dark:text-dark-300 dark:hover:text-white"
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
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '起源AI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
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
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

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
  { question: t('guide.faq.q4'), answer: t('guide.faq.a4') }
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

<style scoped>
.console-card {
  background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%);
  border-radius: 12px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.4),
    0 0 0 1px rgba(255, 255, 255, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
  overflow: hidden;
  transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
  transition: transform 0.4s ease;
}

.console-card:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

.config-panel {
  background: linear-gradient(145deg, #1a1a2e 0%, #16213e 100%);
  border: 1px solid rgba(255, 255, 255, 0.08);
  box-shadow: 0 10px 30px -5px rgba(0, 0, 0, 0.3);
}

:deep(.dark) .console-card {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(212, 165, 116, 0.15),
    0 0 40px rgba(212, 165, 116, 0.08),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}
</style>
