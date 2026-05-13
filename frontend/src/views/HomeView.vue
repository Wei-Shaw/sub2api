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
    class="relative flex min-h-screen flex-col overflow-hidden bg-slate-50 text-slate-950 dark:bg-slate-950 dark:text-white"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="absolute inset-0 bg-[radial-gradient(ellipse_at_top,rgba(14,165,233,0.16),transparent_42%),linear-gradient(135deg,#f8fafc_0%,#ecfeff_48%,#eef2ff_100%)] dark:bg-[radial-gradient(ellipse_at_top,rgba(14,165,233,0.20),transparent_42%),linear-gradient(135deg,#020617_0%,#08111f_48%,#111827_100%)]"></div>
      <div class="absolute inset-0 bg-[linear-gradient(rgba(14,116,144,0.10)_1px,transparent_1px),linear-gradient(90deg,rgba(37,99,235,0.08)_1px,transparent_1px)] bg-[size:56px_56px] dark:bg-[linear-gradient(rgba(34,211,238,0.09)_1px,transparent_1px),linear-gradient(90deg,rgba(59,130,246,0.08)_1px,transparent_1px)]"></div>
      <div class="absolute inset-x-0 top-0 h-px bg-cyan-500/35 dark:bg-cyan-300/60"></div>
      <div class="absolute left-0 top-28 h-px w-full bg-gradient-to-r from-transparent via-cyan-500/25 to-transparent dark:via-cyan-300/40"></div>
      <div class="absolute bottom-40 left-0 h-px w-full -rotate-6 bg-gradient-to-r from-transparent via-blue-500/20 to-transparent dark:via-blue-300/30"></div>
      <div class="absolute left-8 top-24 h-40 w-40 border-l border-t border-cyan-500/20 dark:border-cyan-300/25"></div>
      <div class="absolute bottom-16 right-8 h-40 w-40 border-b border-r border-violet-500/20 dark:border-violet-300/25"></div>
      <div class="absolute inset-0 bg-[linear-gradient(180deg,transparent,rgba(255,255,255,0.18)_50%,transparent)] bg-[length:100%_7px] opacity-60 dark:bg-[linear-gradient(180deg,transparent,rgba(15,23,42,0.12)_50%,transparent)] dark:opacity-70"></div>
    </div>

    <!-- Header -->
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between rounded-2xl border border-cyan-500/15 bg-white/75 px-4 py-3 shadow-lg shadow-cyan-900/10 backdrop-blur-xl dark:border-cyan-300/15 dark:bg-slate-950/55 dark:shadow-cyan-950/20">
        <!-- Logo -->
        <div class="flex items-center gap-3">
          <div class="h-10 w-10 overflow-hidden rounded-xl border border-cyan-500/20 bg-white p-1 shadow-md shadow-cyan-500/10 dark:border-cyan-300/25 dark:bg-slate-900">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <div class="hidden sm:block">
            <p class="text-sm font-semibold text-slate-950 dark:text-white">{{ siteName }}</p>
            <p class="text-xs text-cyan-700/60 dark:text-cyan-100/50">AI Gateway Console</p>
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
            class="rounded-lg border border-cyan-500/10 bg-white/60 p-2 text-cyan-800/70 transition-colors hover:border-cyan-500/30 hover:text-slate-950 dark:border-cyan-300/10 dark:bg-white/[0.03] dark:text-cyan-100/70 dark:hover:border-cyan-300/30 dark:hover:text-white"
            :title="t('home.viewDocs')"
          >
            <Icon name="book" size="md" />
          </a>

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg border border-cyan-500/10 bg-white/60 p-2 text-cyan-800/70 transition-colors hover:border-cyan-500/30 hover:text-slate-950 dark:border-cyan-300/10 dark:bg-white/[0.03] dark:text-cyan-100/70 dark:hover:border-cyan-300/30 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <!-- Login / Dashboard Button -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="inline-flex items-center gap-1.5 rounded-full border border-cyan-500/25 bg-cyan-500/10 py-1 pl-1 pr-2.5 transition-colors hover:bg-cyan-500/15 dark:border-cyan-300/25 dark:bg-cyan-300/10 dark:hover:bg-cyan-300/20"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-slate-900 dark:text-white">{{ t('home.dashboard') }}</span>
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
            class="inline-flex items-center rounded-full border border-cyan-500/25 bg-cyan-500/10 px-3 py-1 text-xs font-medium text-cyan-800 transition-colors hover:bg-cyan-500/15 dark:border-cyan-300/25 dark:bg-cyan-300/10 dark:text-cyan-50 dark:hover:bg-cyan-300/20"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 flex-1 px-6 py-16">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section - Left/Right Layout -->
        <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <!-- Left: Text Content -->
          <div class="flex-1 text-center lg:text-left">
            <div class="mb-5 inline-flex items-center gap-2 rounded-full border border-cyan-500/25 bg-cyan-500/10 px-4 py-2 text-xs font-semibold uppercase tracking-[0.22em] text-cyan-800 dark:border-cyan-300/25 dark:bg-cyan-300/10 dark:text-cyan-100">
              <span class="h-1.5 w-1.5 rounded-full bg-cyan-300 shadow-[0_0_12px_rgba(34,211,238,0.95)]"></span>
              Unified AI Gateway
            </div>
            <h1
              class="mb-5 bg-gradient-to-r from-cyan-700 via-slate-950 to-violet-700 bg-clip-text text-4xl font-bold tracking-normal text-transparent dark:from-cyan-200 dark:via-white dark:to-violet-200 md:text-5xl lg:text-7xl"
            >
              {{ siteName }}
            </h1>
            <p class="mb-8 max-w-xl text-lg leading-8 text-slate-600 dark:text-slate-300 md:text-xl">
              {{ siteSubtitle }}
            </p>

            <!-- CTA Button -->
            <div class="flex flex-col items-center gap-3 sm:flex-row lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="inline-flex h-12 items-center justify-center rounded-xl border border-cyan-500/30 bg-cyan-500/10 px-8 text-base font-semibold text-cyan-800 shadow-md shadow-cyan-900/10 transition-colors hover:bg-cyan-500/15 dark:border-cyan-300/35 dark:bg-cyan-300/15 dark:text-cyan-50 dark:shadow-cyan-950/40 dark:hover:bg-cyan-300/25"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="inline-flex h-12 items-center justify-center rounded-xl border border-slate-300/70 bg-white/60 px-6 text-sm font-medium text-slate-700 transition-colors hover:border-slate-400 hover:bg-white/80 dark:border-white/10 dark:bg-white/[0.04] dark:text-slate-200 dark:hover:border-white/20 dark:hover:bg-white/[0.07]"
              >
                {{ t('home.docs') }}
              </a>
            </div>
          </div>

          <!-- Right: Gateway Deck -->
          <div class="flex flex-1 justify-center lg:justify-end">
            <div class="gateway-deck w-full max-w-[520px]" :class="{ 'gateway-deck-dark': isDark }">
              <div class="gateway-panel">
                <div class="flex items-center justify-between border-b border-cyan-500/15 px-5 py-4 dark:border-cyan-300/15">
                  <div>
                    <p class="text-xs font-semibold uppercase tracking-[0.22em] text-cyan-700/70 dark:text-cyan-100/60">Routing Matrix</p>
                    <p class="mt-1 text-sm font-semibold text-slate-950 dark:text-white">Model access layer</p>
                  </div>
                  <div class="rounded-full border border-emerald-500/20 bg-emerald-500/10 px-3 py-1 text-xs font-semibold text-emerald-700 dark:border-emerald-300/20 dark:bg-emerald-300/10 dark:text-emerald-200">
                    Online
                  </div>
                </div>

                <div class="grid gap-4 p-5">
                  <div class="rounded-2xl border border-cyan-500/15 bg-white/70 p-4 dark:border-cyan-300/15 dark:bg-slate-950/70">
                    <div class="mb-4 flex items-center justify-between">
                      <span class="text-xs font-medium text-slate-500 dark:text-slate-400">Unified Endpoint</span>
                      <span class="rounded-full bg-cyan-500/10 px-2 py-0.5 text-[11px] font-semibold text-cyan-800 dark:bg-cyan-300/10 dark:text-cyan-100">/v1/chat/completions</span>
                    </div>
                    <div class="space-y-2">
                      <div
                        v-for="model in heroModels"
                        :key="model.name"
                        class="flex items-center justify-between rounded-xl border border-slate-200/80 bg-white/70 px-3 py-2.5 dark:border-white/10 dark:bg-white/[0.035]"
                      >
                        <div class="flex items-center gap-3">
                          <span :class="['h-2.5 w-2.5 rounded-full shadow-[0_0_12px_currentColor]', model.dotClass]"></span>
                          <div>
                            <p class="text-sm font-semibold text-slate-950 dark:text-white">{{ model.name }}</p>
                            <p class="text-xs text-slate-500">{{ model.vendor }}</p>
                          </div>
                        </div>
                        <span class="text-xs font-medium text-cyan-700/80 dark:text-cyan-100/70">{{ model.status }}</span>
                      </div>
                    </div>
                  </div>

                  <div class="grid grid-cols-3 gap-3">
                    <div class="rounded-2xl border border-blue-500/15 bg-blue-500/10 p-3 dark:border-blue-300/15 dark:bg-blue-300/10">
                      <p class="text-[11px] uppercase tracking-[0.16em] text-blue-700/60 dark:text-blue-100/60">Requests</p>
                      <p class="mt-2 text-xl font-bold text-blue-800 dark:text-blue-100">2.4M</p>
                    </div>
                    <div class="rounded-2xl border border-emerald-500/15 bg-emerald-500/10 p-3 dark:border-emerald-300/15 dark:bg-emerald-300/10">
                      <p class="text-[11px] uppercase tracking-[0.16em] text-emerald-700/60 dark:text-emerald-100/60">Uptime</p>
                      <p class="mt-2 text-xl font-bold text-emerald-800 dark:text-emerald-100">99.9%</p>
                    </div>
                    <div class="rounded-2xl border border-amber-500/15 bg-amber-500/10 p-3 dark:border-amber-300/15 dark:bg-amber-300/10">
                      <p class="text-[11px] uppercase tracking-[0.16em] text-amber-700/60 dark:text-amber-100/60">Latency</p>
                      <p class="mt-2 text-xl font-bold text-amber-800 dark:text-amber-100">Low</p>
                    </div>
                  </div>

                  <div class="terminal-strip">
                    <span class="text-emerald-300">$</span>
                    <span class="text-cyan-200">curl</span>
                    <span class="text-violet-200">-H</span>
                    <span class="text-slate-600 dark:text-slate-300">"Authorization: Bearer sk-***"</span>
                    <span class="ml-auto text-emerald-300">200 OK</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Feature Tags - Centered -->
        <div class="mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6">
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-cyan-500/20 bg-cyan-500/10 px-5 py-2.5 shadow-sm shadow-cyan-900/10 backdrop-blur-sm dark:border-cyan-300/20 dark:bg-cyan-300/10 dark:shadow-cyan-950/20"
          >
            <Icon name="swap" size="sm" class="text-cyan-700 dark:text-cyan-200" />
            <span class="text-sm font-medium text-cyan-900 dark:text-cyan-50">{{
              t('home.tags.subscriptionToApi')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-violet-500/20 bg-violet-500/10 px-5 py-2.5 shadow-sm shadow-violet-900/10 backdrop-blur-sm dark:border-violet-300/20 dark:bg-violet-300/10 dark:shadow-violet-950/20"
          >
            <Icon name="shield" size="sm" class="text-violet-700 dark:text-violet-200" />
            <span class="text-sm font-medium text-violet-900 dark:text-violet-50">{{
              t('home.tags.stickySession')
            }}</span>
          </div>
          <div
            class="inline-flex items-center gap-2.5 rounded-full border border-emerald-500/20 bg-emerald-500/10 px-5 py-2.5 shadow-sm shadow-emerald-900/10 backdrop-blur-sm dark:border-emerald-300/20 dark:bg-emerald-300/10 dark:shadow-emerald-950/20"
          >
            <Icon name="chart" size="sm" class="text-emerald-700 dark:text-emerald-200" />
            <span class="text-sm font-medium text-emerald-900 dark:text-emerald-50">{{
              t('home.tags.realtimeBilling')
            }}</span>
          </div>
        </div>

        <!-- Features Grid -->
        <div class="mb-12 grid gap-6 md:grid-cols-3">
          <!-- Feature 1: Unified Gateway -->
          <div
            class="group rounded-2xl border border-cyan-500/15 bg-white/65 p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-cyan-500/35 hover:bg-white/80 hover:shadow-xl hover:shadow-cyan-900/10 dark:border-cyan-300/15 dark:bg-white/[0.045] dark:hover:border-cyan-300/35 dark:hover:bg-white/[0.07] dark:hover:shadow-cyan-950/30"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl border border-blue-500/25 bg-blue-500/10 text-blue-700 shadow-lg shadow-blue-900/10 transition-transform group-hover:scale-110 dark:border-blue-300/25 dark:bg-blue-300/10 dark:text-blue-100 dark:shadow-blue-950/20"
            >
              <Icon name="server" size="lg" />
            </div>
            <h3 class="mb-2 text-lg font-semibold text-slate-950 dark:text-white">
              {{ t('home.features.unifiedGateway') }}
            </h3>
            <p class="text-sm leading-relaxed text-slate-600 dark:text-slate-400">
              {{ t('home.features.unifiedGatewayDesc') }}
            </p>
          </div>

          <!-- Feature 2: Account Pool -->
          <div
            class="group rounded-2xl border border-emerald-500/15 bg-white/65 p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-emerald-500/35 hover:bg-white/80 hover:shadow-xl hover:shadow-emerald-900/10 dark:border-emerald-300/15 dark:bg-white/[0.045] dark:hover:border-emerald-300/35 dark:hover:bg-white/[0.07] dark:hover:shadow-emerald-950/30"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl border border-emerald-500/25 bg-emerald-500/10 text-emerald-700 shadow-lg shadow-emerald-900/10 transition-transform group-hover:scale-110 dark:border-emerald-300/25 dark:bg-emerald-300/10 dark:text-emerald-100 dark:shadow-emerald-950/20"
            >
              <svg
                class="h-6 w-6"
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
            <h3 class="mb-2 text-lg font-semibold text-slate-950 dark:text-white">
              {{ t('home.features.multiAccount') }}
            </h3>
            <p class="text-sm leading-relaxed text-slate-600 dark:text-slate-400">
              {{ t('home.features.multiAccountDesc') }}
            </p>
          </div>

          <!-- Feature 3: Billing & Quota -->
          <div
            class="group rounded-2xl border border-violet-500/15 bg-white/65 p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-violet-500/35 hover:bg-white/80 hover:shadow-xl hover:shadow-violet-900/10 dark:border-violet-300/15 dark:bg-white/[0.045] dark:hover:border-violet-300/35 dark:hover:bg-white/[0.07] dark:hover:shadow-violet-950/30"
          >
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl border border-violet-500/25 bg-violet-500/10 text-violet-700 shadow-lg shadow-violet-900/10 transition-transform group-hover:scale-110 dark:border-violet-300/25 dark:bg-violet-300/10 dark:text-violet-100 dark:shadow-violet-950/20"
            >
              <svg
                class="h-6 w-6"
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
            <h3 class="mb-2 text-lg font-semibold text-slate-950 dark:text-white">
              {{ t('home.features.balanceQuota') }}
            </h3>
            <p class="text-sm leading-relaxed text-slate-600 dark:text-slate-400">
              {{ t('home.features.balanceQuotaDesc') }}
            </p>
          </div>
        </div>

        <!-- Supported Providers -->
        <div class="mb-8 text-center">
          <h2 class="mb-3 text-2xl font-bold text-slate-950 dark:text-white">
            {{ t('home.providers.title') }}
          </h2>
          <p class="text-sm text-slate-600 dark:text-slate-400">
            {{ t('home.providers.description') }}
          </p>
        </div>

        <div class="mb-16 flex flex-wrap items-center justify-center gap-4">
          <!-- Claude - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-orange-500/20 bg-orange-500/10 px-5 py-3 ring-1 ring-orange-500/10 backdrop-blur-sm dark:border-orange-300/20 dark:bg-orange-300/10 dark:ring-orange-300/10"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-orange-400/20 ring-1 ring-orange-500/25 dark:ring-orange-300/25"
            >
              <span class="text-xs font-bold text-orange-700 dark:text-orange-100">C</span>
            </div>
            <span class="text-sm font-medium text-orange-900 dark:text-orange-50">{{ t('home.providers.claude') }}</span>
            <span
              class="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-300/10 dark:text-emerald-200"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- GPT - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-emerald-500/20 bg-emerald-500/10 px-5 py-3 ring-1 ring-emerald-500/10 backdrop-blur-sm dark:border-emerald-300/20 dark:bg-emerald-300/10 dark:ring-emerald-300/10"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-400/20 ring-1 ring-emerald-500/25 dark:ring-emerald-300/25"
            >
              <span class="text-xs font-bold text-emerald-700 dark:text-emerald-100">G</span>
            </div>
            <span class="text-sm font-medium text-emerald-900 dark:text-emerald-50">GPT</span>
            <span
              class="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-300/10 dark:text-emerald-200"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Gemini - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-blue-500/20 bg-blue-500/10 px-5 py-3 ring-1 ring-blue-500/10 backdrop-blur-sm dark:border-blue-300/20 dark:bg-blue-300/10 dark:ring-blue-300/10"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-blue-400/20 ring-1 ring-blue-500/25 dark:ring-blue-300/25"
            >
              <span class="text-xs font-bold text-blue-700 dark:text-blue-100">G</span>
            </div>
            <span class="text-sm font-medium text-blue-900 dark:text-blue-50">{{ t('home.providers.gemini') }}</span>
            <span
              class="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-300/10 dark:text-emerald-200"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- Antigravity - Supported -->
          <div
            class="flex items-center gap-2 rounded-xl border border-rose-500/20 bg-rose-500/10 px-5 py-3 ring-1 ring-rose-500/10 backdrop-blur-sm dark:border-rose-300/20 dark:bg-rose-300/10 dark:ring-rose-300/10"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-rose-400/20 ring-1 ring-rose-500/25 dark:ring-rose-300/25"
            >
              <span class="text-xs font-bold text-rose-700 dark:text-rose-100">A</span>
            </div>
            <span class="text-sm font-medium text-rose-900 dark:text-rose-50">{{ t('home.providers.antigravity') }}</span>
            <span
              class="rounded bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-300/10 dark:text-emerald-200"
              >{{ t('home.providers.supported') }}</span
            >
          </div>
          <!-- More - Coming Soon -->
          <div
            class="flex items-center gap-2 rounded-xl border border-slate-300 bg-white/60 px-5 py-3 opacity-80 backdrop-blur-sm dark:border-slate-500/20 dark:bg-white/[0.035] dark:opacity-70"
          >
            <div
              class="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-400/10 ring-1 ring-slate-400/20 dark:ring-slate-300/15"
            >
              <span class="text-xs font-bold text-slate-600 dark:text-slate-200">+</span>
            </div>
            <span class="text-sm font-medium text-slate-600 dark:text-slate-200">{{ t('home.providers.more') }}</span>
            <span
              class="rounded bg-slate-500/10 px-1.5 py-0.5 text-[10px] font-medium text-slate-500 dark:bg-slate-300/10 dark:text-slate-400"
              >{{ t('home.providers.soon') }}</span
            >
          </div>
        </div>
      </div>
    </main>

    <!-- Footer -->
    <footer class="relative z-10 border-t border-cyan-500/10 px-6 py-8 dark:border-cyan-300/10">
      <div
        class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"
      >
        <p class="text-sm text-slate-500">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex items-center gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-slate-500 transition-colors hover:text-cyan-700 dark:hover:text-cyan-100"
          >
            {{ t('home.docs') }}
          </a>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-slate-500 transition-colors hover:text-cyan-700 dark:hover:text-cyan-100"
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

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

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
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

const heroModels = [
  { name: 'GPT-4.1', vendor: 'OpenAI compatible', status: 'Ready', dotClass: 'bg-emerald-300 text-emerald-300' },
  { name: 'Claude 3.7 Sonnet', vendor: 'Anthropic messages', status: 'Routed', dotClass: 'bg-orange-300 text-orange-300' },
  { name: 'Gemini 2.5 Pro', vendor: 'Google models', status: 'Synced', dotClass: 'bg-blue-300 text-blue-300' },
  { name: 'DeepSeek R1', vendor: 'Reasoning endpoint', status: 'Online', dotClass: 'bg-violet-300 text-violet-300' },
]

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
  } else {
    isDark.value = false
    document.documentElement.classList.remove('dark')
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
.gateway-deck {
  position: relative;
  perspective: 1200px;
}

.gateway-deck::before {
  content: '';
  position: absolute;
  inset: 18px -12px -18px 18px;
  border: 1px solid rgba(14, 116, 144, 0.16);
  border-radius: 28px;
  transform: rotate(2deg);
}

.gateway-panel {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(14, 116, 144, 0.22);
  border-radius: 28px;
  background:
    linear-gradient(rgba(14, 116, 144, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(37, 99, 235, 0.06) 1px, transparent 1px),
    rgba(255, 255, 255, 0.72);
  background-size: 32px 32px, 32px 32px, auto;
  box-shadow:
    0 24px 70px rgba(8, 47, 73, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
  transform: rotateX(2deg) rotateY(-4deg);
  transition: transform 0.3s ease, border-color 0.3s ease;
}

.gateway-panel:hover {
  border-color: rgba(14, 116, 144, 0.38);
  transform: rotateX(0deg) rotateY(0deg) translateY(-4px);
}

.terminal-strip {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  overflow: hidden;
  white-space: nowrap;
  border: 1px solid rgba(14, 116, 144, 0.18);
  border-radius: 1rem;
  background: rgba(255, 255, 255, 0.70);
  padding: 0.75rem 1rem;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.75rem;
}

.gateway-deck-dark::before,
:global(html.dark) .gateway-deck::before {
  border-color: rgba(103, 232, 249, 0.16);
}

.gateway-deck-dark .gateway-panel,
:global(html.dark) .gateway-panel {
  border-color: rgba(103, 232, 249, 0.22);
  background:
    linear-gradient(rgba(34, 211, 238, 0.07) 1px, transparent 1px),
    linear-gradient(90deg, rgba(59, 130, 246, 0.06) 1px, transparent 1px),
    rgba(2, 6, 23, 0.92);
  box-shadow:
    0 24px 70px rgba(8, 47, 73, 0.45),
    inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.gateway-deck-dark .gateway-panel:hover,
:global(html.dark) .gateway-panel:hover {
  border-color: rgba(103, 232, 249, 0.38);
}

.gateway-deck-dark .terminal-strip,
:global(html.dark) .terminal-strip {
  border-color: rgba(103, 232, 249, 0.14);
  background: rgba(15, 23, 42, 0.86);
}
</style>
