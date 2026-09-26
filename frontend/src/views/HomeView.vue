<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe v-if="isHomeContentUrl" :src="homeContent.trim()" class="h-screen w-full border-0" allowfullscreen></iframe>
    <div v-else v-html="homeContent"></div>
  </div>
  <div v-else class="flex min-h-screen flex-col bg-white dark:bg-dark-950">
    <!-- Header -->
    <header class="sticky top-0 z-50 border-b border-gray-100 bg-white/95 backdrop-blur-sm dark:border-dark-800 dark:bg-dark-950/95">
      <nav class="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
        <div class="flex items-center gap-3">
          <div class="h-9 w-9 overflow-hidden rounded-lg"><img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" /></div>
          <span class="text-lg font-bold text-gray-900 dark:text-white">{{ siteName }}</span>
        </div>
        <div class="flex items-center gap-2">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="hidden items-center gap-1.5 rounded-lg px-3 py-2 text-sm text-gray-600 transition-colors hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800 sm:flex">
            <Icon name="book" size="sm" /><span>{{ t('home.viewDocs') }}</span>
          </a>
          <button @click="toggleTheme" class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 dark:text-dark-400 dark:hover:bg-dark-800">
            <Icon v-if="isDark" name="sun" size="md" /><Icon v-else name="moon" size="md" />
          </button>
          <template v-if="isAuthenticated">
            <router-link :to="dashboardPath" class="ml-2 inline-flex items-center gap-1.5 rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700">{{ t('home.dashboard') }}<Icon name="arrowRight" size="sm" /></router-link>
          </template>
          <template v-else>
            <router-link v-if="registrationEnabled" to="/register" class="ml-2 inline-flex items-center rounded-lg border border-gray-200 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700">{{ t('auth.signUp') }}</router-link>
            <router-link to="/login" class="ml-2 inline-flex items-center rounded-lg bg-primary-600 px-4 py-2 text-sm font-medium text-white hover:bg-primary-700">{{ t('home.login') }}</router-link>
          </template>
        </div>
      </nav>
    </header>

    <!-- Hero -->
    <section class="relative overflow-hidden border-b border-gray-100 dark:border-dark-800">
      <div class="absolute inset-0 bg-gradient-to-br from-primary-50/80 via-white to-gray-50 dark:from-dark-900 dark:via-dark-950 dark:to-dark-900"></div>
      <div class="absolute inset-0 bg-[linear-gradient(rgba(37,99,235,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(37,99,235,0.03)_1px,transparent_1px)] bg-[size:48px_48px]"></div>
      <div class="relative mx-auto max-w-7xl px-6 py-16 md:py-24">
        <div class="flex flex-col items-center gap-12 lg:flex-row lg:items-start">
          <div class="flex-1 text-center lg:text-left">
            <div class="mb-5 inline-flex items-center gap-2 rounded-full border border-primary-200 bg-primary-50 px-4 py-1.5 dark:border-primary-800 dark:bg-primary-950/30">
              <span class="h-1.5 w-1.5 animate-pulse rounded-full bg-primary-500"></span>
              <span class="text-xs font-medium text-primary-700 dark:text-primary-300">{{ t('home.tags.subscriptionToApi') }} · {{ t('home.tags.realtimeBilling') }}</span>
            </div>
            <h1 class="mb-5 text-4xl font-bold leading-tight text-gray-900 dark:text-white md:text-5xl lg:text-[3.5rem]">{{ siteName }}</h1>
            <p class="mb-4 text-xl font-medium text-gray-700 dark:text-dark-200">{{ t('home.heroSubtitle') }}</p>
            <p class="mb-8 max-w-xl text-base text-gray-500 dark:text-dark-400 lg:max-w-none">{{ t('home.heroDescription') }}</p>
            <div class="flex flex-col items-center gap-4 sm:flex-row lg:justify-start">
              <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="inline-flex items-center gap-2 rounded-lg bg-primary-600 px-8 py-3 text-base font-semibold text-white shadow-sm hover:bg-primary-700">{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}<Icon name="arrowRight" size="md" /></router-link>
              <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-2 rounded-lg border border-gray-200 bg-white px-8 py-3 text-base font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-700 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"><Icon name="book" size="md" />{{ t('home.viewDocs') }}</a>
            </div>
          </div>
          <div class="w-full max-w-md flex-shrink-0 lg:max-w-lg">
            <div class="overflow-hidden rounded-xl border border-gray-200 bg-gray-900 shadow-2xl dark:border-dark-700">
              <div class="flex items-center gap-2 border-b border-gray-700 px-4 py-3">
                <span class="h-3 w-3 rounded-full bg-red-500"></span><span class="h-3 w-3 rounded-full bg-yellow-500"></span><span class="h-3 w-3 rounded-full bg-green-500"></span>
                <span class="ml-3 text-xs text-gray-400">API 调用示例</span>
              </div>
              <div class="p-5 font-mono text-sm leading-relaxed">
                <div class="text-gray-400"><span class="text-green-400">POST</span> /v1/chat/completions</div>
                <div class="mt-1 text-gray-500">Content-Type: application/json</div>
                <div class="mt-1 text-gray-500">Authorization: Bearer <span class="text-yellow-300">sk-***</span></div>
                <div class="mt-4 text-gray-300">{</div>
                <div class="ml-4 text-gray-300">"model": "<span class="text-blue-400">deepseek-chat</span>",</div>
                <div class="ml-4 text-gray-300">"messages": [</div>
                <div class="ml-8 text-gray-300">{ "role": "user",</div>
                <div class="ml-10 text-gray-300">"content": "<span class="text-amber-300">你好</span>" }</div>
                <div class="ml-4 text-gray-300">]</div>
                <div class="text-gray-300">}</div>
                <div class="mt-4 border-t border-gray-700 pt-3"><span class="text-green-400">200 OK</span><span class="ml-2 text-gray-500">— 路由至 DeepSeek · 320ms</span></div>
              </div>
            </div>
          </div>
        </div>
        <!-- Stats -->
        <div class="mt-14 grid grid-cols-2 gap-6 border-t border-gray-200/60 pt-10 dark:border-dark-800 sm:grid-cols-4">
          <div class="text-center"><div class="text-3xl font-bold text-primary-600 dark:text-primary-400">10+</div><div class="mt-1 text-sm text-gray-500 dark:text-dark-400">AI 模型</div></div>
          <div class="text-center"><div class="text-3xl font-bold text-primary-600 dark:text-primary-400">99.9%</div><div class="mt-1 text-sm text-gray-500 dark:text-dark-400">服务可用性</div></div>
          <div class="text-center"><div class="text-3xl font-bold text-primary-600 dark:text-primary-400">1</div><div class="mt-1 text-sm text-gray-500 dark:text-dark-400">统一 API 密钥</div></div>
          <div class="text-center"><div class="text-3xl font-bold text-primary-600 dark:text-primary-400">7×24</div><div class="mt-1 text-sm text-gray-500 dark:text-dark-400">稳定运行</div></div>
        </div>
      </div>
    </section>

    <!-- Pain Points -->
    <section class="border-b border-gray-100 bg-gray-50 py-16 dark:border-dark-800 dark:bg-dark-900/30">
      <div class="mx-auto max-w-7xl px-6">
        <div class="mb-10 text-center"><h2 class="text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.painPoints.title') }}</h2></div>
        <div class="grid gap-5 sm:grid-cols-2 lg:grid-cols-4">
          <div class="rounded-xl border border-red-100 bg-white p-6 dark:border-red-900/30 dark:bg-dark-800/50">
            <div class="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-red-50 dark:bg-red-950/30"><svg class="h-5 w-5 text-red-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 6v12m-3-2.818l.879.659c1.171.879 3.07.879 4.242 0 1.172-.879 1.172-2.303 0-3.182C13.536 12.219 12.768 12 12 12c-.725 0-1.45-.22-2.003-.659-1.106-.879-1.106-2.303 0-3.182s2.9-.879 4.006 0l.415.33M21 12a9 9 0 11-18 0 9 9 0 0118 0z" /></svg></div>
            <h3 class="mb-1 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.painPoints.items.expensive.title') }}</h3>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('home.painPoints.items.expensive.desc') }}</p>
          </div>
          <div class="rounded-xl border border-amber-100 bg-white p-6 dark:border-amber-900/30 dark:bg-dark-800/50">
            <div class="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-amber-50 dark:bg-amber-950/30"><svg class="h-5 w-5 text-amber-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.066 2.573c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.573 1.066c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.066-2.573c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" /><path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" /></svg></div>
            <h3 class="mb-1 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.painPoints.items.complex.title') }}</h3>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('home.painPoints.items.complex.desc') }}</p>
          </div>
          <div class="rounded-xl border border-orange-100 bg-white p-6 dark:border-orange-900/30 dark:bg-dark-800/50">
            <div class="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-orange-50 dark:bg-orange-950/30"><svg class="h-5 w-5 text-orange-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M12 9v2m0 4h.01m-6.938 4h13.856c1.54 0 2.502-1.667 1.732-2.5L13.732 4c-.77-.833-1.964-.833-2.732 0L4.082 16.5c-.77.833.192 2.5 1.732 2.5z" /></svg></div>
            <h3 class="mb-1 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.painPoints.items.unstable.title') }}</h3>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('home.painPoints.items.unstable.desc') }}</p>
          </div>
          <div class="rounded-xl border border-purple-100 bg-white p-6 dark:border-purple-900/30 dark:bg-dark-800/50">
            <div class="mb-3 flex h-10 w-10 items-center justify-center rounded-lg bg-purple-50 dark:bg-purple-950/30"><svg class="h-5 w-5 text-purple-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M9 19v-6a2 2 0 00-2-2H5a2 2 0 00-2 2v6a2 2 0 002 2h2a2 2 0 002-2zm0 0V9a2 2 0 012-2h2a2 2 0 012 2v10m-6 0a2 2 0 002 2h2a2 2 0 002-2m0 0V5a2 2 0 012-2h2a2 2 0 012 2v14a2 2 0 01-2 2h-2a2 2 0 01-2-2z" /></svg></div>
            <h3 class="mb-1 text-base font-semibold text-gray-900 dark:text-white">{{ t('home.painPoints.items.noControl.title') }}</h3>
            <p class="text-sm text-gray-500 dark:text-dark-400">{{ t('home.painPoints.items.noControl.desc') }}</p>
          </div>
        </div>
      </div>
    </section>

    <!-- Core Features -->
    <section class="border-b border-gray-100 py-16 dark:border-dark-800">
      <div class="mx-auto max-w-7xl px-6">
        <div class="mb-12 text-center"><h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.features.title') }}</h2><p class="text-gray-500 dark:text-dark-400">{{ t('home.features.description') }}</p></div>
        <div class="grid gap-6 md:grid-cols-3">
          <div class="rounded-xl border border-gray-100 bg-white p-8 dark:border-dark-700 dark:bg-dark-800/50"><div class="mb-5 flex h-12 w-12 items-center justify-center rounded-lg bg-primary-50 dark:bg-primary-950/30"><Icon name="server" size="lg" class="text-primary-600 dark:text-primary-400" /></div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t('home.features.unifiedGateway') }}</h3><p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ t('home.features.unifiedGatewayDesc') }}</p></div>
          <div class="rounded-xl border border-gray-100 bg-white p-8 dark:border-dark-700 dark:bg-dark-800/50"><div class="mb-5 flex h-12 w-12 items-center justify-center rounded-lg bg-primary-50 dark:bg-primary-950/30"><svg class="h-6 w-6 text-primary-600 dark:text-primary-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z" /></svg></div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t('home.features.multiAccount') }}</h3><p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ t('home.features.multiAccountDesc') }}</p></div>
          <div class="rounded-xl border border-gray-100 bg-white p-8 dark:border-dark-700 dark:bg-dark-800/50"><div class="mb-5 flex h-12 w-12 items-center justify-center rounded-lg bg-primary-50 dark:bg-primary-950/30"><svg class="h-6 w-6 text-primary-600 dark:text-primary-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z" /></svg></div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t('home.features.balanceQuota') }}</h3><p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ t('home.features.balanceQuotaDesc') }}</p></div>
        </div>
      </div>
    </section>

    <!-- Tech Highlights - 6 cards -->
    <section class="border-b border-gray-100 bg-gray-50 py-16 dark:border-dark-800 dark:bg-dark-900/30">
      <div class="mx-auto max-w-7xl px-6">
        <div class="mb-12 text-center"><h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.techHighlights.title') }}</h2></div>
        <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          <div v-for="(item, key) in techItems" :key="key" class="flex items-start gap-4 rounded-xl border border-gray-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/50">
            <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-lg bg-primary-50 dark:bg-primary-950/30">
              <component :is="techIcons[key]" />
            </div>
            <div><h3 class="mb-1 text-sm font-semibold text-gray-900 dark:text-white">{{ item.title }}</h3><p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ item.desc }}</p></div>
          </div>
        </div>
      </div>
    </section>

    <!-- How It Works -->
    <section class="border-b border-gray-100 py-16 dark:border-dark-800">
      <div class="mx-auto max-w-7xl px-6">
        <div class="mb-12 text-center"><h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.solutions.title') }}</h2><p class="text-gray-500 dark:text-dark-400">{{ t('home.solutions.subtitle') }}</p></div>
        <div class="relative grid gap-8 md:grid-cols-3">
          <div class="absolute left-0 right-0 top-7 hidden h-0.5 bg-gray-100 dark:bg-dark-800 md:block" style="margin: 0 16.67%;"></div>
          <div class="relative z-10 text-center"><div class="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-full bg-primary-600 text-xl font-bold text-white shadow-lg shadow-primary-500/20">1</div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">注册账号</h3><p class="text-sm text-gray-500 dark:text-dark-400">注册即可获得免费试用额度，立即开始使用</p></div>
          <div class="relative z-10 text-center"><div class="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-full bg-primary-600 text-xl font-bold text-white shadow-lg shadow-primary-500/20">2</div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">获取 API 密钥</h3><p class="text-sm text-gray-500 dark:text-dark-400">在控制台生成密钥，一个密钥调用所有模型</p></div>
          <div class="relative z-10 text-center"><div class="mx-auto mb-5 flex h-14 w-14 items-center justify-center rounded-full bg-primary-600 text-xl font-bold text-white shadow-lg shadow-primary-500/20">3</div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">开始调用</h3><p class="text-sm text-gray-500 dark:text-dark-400">使用标准 OpenAI 格式接入，无需修改代码</p></div>
        </div>
      </div>
    </section>

    <!-- Comparison -->
    <section class="border-b border-gray-100 bg-gray-50 py-16 dark:border-dark-800 dark:bg-dark-900/30">
      <div class="mx-auto max-w-4xl px-6">
        <div class="mb-10 text-center"><h2 class="text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.comparison.title') }}</h2></div>
        <div class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800/50">
          <table class="w-full text-sm">
            <thead><tr class="border-b border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-900/50"><th class="px-6 py-4 text-left font-semibold text-gray-700 dark:text-dark-200">{{ t('home.comparison.headers.feature') }}</th><th class="px-6 py-4 text-center font-semibold text-gray-400 dark:text-dark-500">{{ t('home.comparison.headers.official') }}</th><th class="px-6 py-4 text-center font-semibold text-primary-600 dark:text-primary-400">{{ t('home.comparison.headers.us') }}</th></tr></thead>
            <tbody><tr v-for="(item, key) in comparisonItems" :key="key" class="border-b border-gray-50 last:border-0 dark:border-dark-700/50"><td class="px-6 py-4 font-medium text-gray-900 dark:text-white">{{ item.feature }}</td><td class="px-6 py-4 text-center text-gray-400 dark:text-dark-500"><span class="inline-flex items-center gap-1"><svg class="h-4 w-4 text-gray-300 dark:text-dark-600" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>{{ item.official }}</span></td><td class="px-6 py-4 text-center text-primary-600 dark:text-primary-400"><span class="inline-flex items-center gap-1"><svg class="h-4 w-4 text-primary-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" /></svg>{{ item.us }}</span></td></tr></tbody>
          </table>
        </div>
      </div>
    </section>

    <!-- Providers -->
    <section class="border-b border-gray-100 py-16 dark:border-dark-800">
      <div class="mx-auto max-w-7xl px-6">
        <div class="mb-10 text-center"><h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.providers.title') }}</h2><p class="text-gray-500 dark:text-dark-400">{{ t('home.providers.description') }}</p></div>
        <div class="mx-auto grid max-w-4xl grid-cols-2 gap-4 sm:grid-cols-3 md:grid-cols-5">
          <div class="flex flex-col items-center gap-3 rounded-xl border border-gray-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/50"><div class="flex h-11 w-11 items-center justify-center rounded-lg bg-blue-600"><span class="text-sm font-bold text-white">D</span></div><span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.deepseek') }}</span><span class="rounded bg-emerald-50 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400">{{ t('home.providers.supported') }}</span></div>
          <div class="flex flex-col items-center gap-3 rounded-xl border border-gray-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/50"><div class="flex h-11 w-11 items-center justify-center rounded-lg bg-violet-600"><span class="text-sm font-bold text-white">通</span></div><span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.qwen') }}</span><span class="rounded bg-emerald-50 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400">{{ t('home.providers.supported') }}</span></div>
          <div class="flex flex-col items-center gap-3 rounded-xl border border-gray-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/50"><div class="flex h-11 w-11 items-center justify-center rounded-lg bg-slate-800"><span class="text-sm font-bold text-white">K</span></div><span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.kimi') }}</span><span class="rounded bg-emerald-50 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400">{{ t('home.providers.supported') }}</span></div>
          <div class="flex flex-col items-center gap-3 rounded-xl border border-gray-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/50"><div class="flex h-11 w-11 items-center justify-center rounded-lg bg-indigo-600"><span class="text-sm font-bold text-white">智</span></div><span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t('home.providers.zhipu') }}</span><span class="rounded bg-emerald-50 px-2 py-0.5 text-[10px] font-medium text-emerald-700 dark:bg-emerald-950/30 dark:text-emerald-400">{{ t('home.providers.supported') }}</span></div>
          <div class="flex flex-col items-center gap-3 rounded-xl border border-dashed border-gray-200 bg-white/50 p-5 dark:border-dark-700 dark:bg-dark-800/30"><div class="flex h-11 w-11 items-center justify-center rounded-lg bg-gray-200 dark:bg-dark-700"><span class="text-sm font-bold text-gray-500 dark:text-dark-400">+</span></div><span class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ t('home.providers.more') }}</span><span class="rounded bg-gray-100 px-2 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-400">{{ t('home.providers.soon') }}</span></div>
        </div>
      </div>
    </section>

    <!-- FAQ -->
    <section class="border-b border-gray-100 bg-gray-50 py-16 dark:border-dark-800 dark:bg-dark-900/30">
      <div class="mx-auto max-w-3xl px-6">
        <div class="mb-10 text-center"><h2 class="text-2xl font-bold text-gray-900 dark:text-white md:text-3xl">{{ t('home.faq.title') }}</h2></div>
        <div class="space-y-3">
          <div v-for="(item, key) in faqItems" :key="String(key)" class="overflow-hidden rounded-xl border border-gray-100 bg-white dark:border-dark-700 dark:bg-dark-800/50">
            <button @click="toggleFaq(String(key))" class="flex w-full items-center justify-between px-6 py-4 text-left">
              <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ item.question }}</span>
              <svg class="h-5 w-5 flex-shrink-0 text-gray-400 transition-transform" :class="{ 'rotate-180': openFaq === String(key) }" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M19 9l-7 7-7-7" /></svg>
            </button>
            <div v-show="openFaq === String(key)" class="border-t border-gray-50 px-6 py-4 dark:border-dark-700"><p class="text-sm leading-relaxed text-gray-500 dark:text-dark-400">{{ item.answer }}</p></div>
          </div>
        </div>
      </div>
    </section>

    <!-- CTA -->
    <section class="relative overflow-hidden py-20">
      <div class="absolute inset-0 bg-gradient-to-br from-primary-600 to-primary-700"></div>
      <div class="absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.05)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.05)_1px,transparent_1px)] bg-[size:48px_48px]"></div>
      <div class="relative mx-auto max-w-7xl px-6 text-center">
        <h2 class="mb-4 text-3xl font-bold text-white md:text-4xl">{{ t('home.cta.title') }}</h2>
        <p class="mb-8 text-lg text-primary-100">{{ t('home.cta.description') }}</p>
        <router-link :to="ctaTargetPath" class="inline-flex items-center gap-2 rounded-lg bg-white px-10 py-3.5 text-base font-semibold text-primary-600 shadow-lg hover:bg-primary-50">{{ isAuthenticated ? t('home.goToDashboard') : t('home.cta.button') }}<Icon name="arrowRight" size="md" /></router-link>
      </div>
    </section>

    <!-- Footer -->
    <footer class="border-t border-gray-100 bg-gray-50 py-8 dark:border-dark-800 dark:bg-dark-900/50">
      <div class="mx-auto flex max-w-7xl flex-col items-center justify-between gap-4 px-6 sm:flex-row">
        <p class="text-sm text-gray-500 dark:text-dark-400">&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div v-if="docUrl" class="flex items-center gap-6"><a :href="docUrl" target="_blank" rel="noopener noreferrer" class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white">{{ t('home.docs') }}</a></div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '稳得AI')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => { const c = homeContent.value.trim(); return c.startsWith('http://') || c.startsWith('https://') })
const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
// 宽容策略：设置未加载(undefined)时显示注册按钮，加载完成后再按实际开关收敛，避免闪烁
const registrationEnabled = computed(() => appStore.cachedPublicSettings?.registration_enabled ?? true)
// 底部 CTA 文案是"免费注册"：未登录且开放注册时直达注册页，否则回退登录页
const ctaTargetPath = computed(() => isAuthenticated.value ? dashboardPath.value : (registrationEnabled.value ? '/register' : '/login'))
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())

const comparisonItems = computed(() => { try { const items = t('home.comparison.items') as any; if (!items || typeof items !== 'object') return []; return Object.values(items) as any[] } catch { return [] } })
const techItems = computed(() => { try { const items = t('home.techHighlights.items') as any; if (!items || typeof items !== 'object') return {}; return items } catch { return {} } })
const faqItems = computed(() => { try { const items = t('home.faq.items') as any; if (!items || typeof items !== 'object') return {}; return items } catch { return {} } })

const openFaq = ref<string | null>(null)
function toggleFaq(key: string) { openFaq.value = openFaq.value === key ? null : key }

// Tech icon components
const makeIcon = (path: string) => () => h('svg', { class: 'h-5 w-5 text-primary-600 dark:text-primary-400', fill: 'none', viewBox: '0 0 24 24', stroke: 'currentColor', 'stroke-width': '1.5' }, [h('path', { 'stroke-linecap': 'round', 'stroke-linejoin': 'round', d: path })])
const techIcons: Record<string, any> = {
  compatible: makeIcon('M17.25 6.75L22.5 12l-5.25 5.25m-10.5 0L1.5 12l5.25-5.25m7.5-3l-4.5 16.5'),
  smartRouting: makeIcon('M7.5 21L3 16.5m0 0L7.5 12M3 16.5h13.5m0-13.5L21 7.5m0 0L16.5 12M21 7.5H7.5'),
  sessionSticky: makeIcon('M13.19 8.688a4.5 4.5 0 011.242 7.244l-4.5 4.5a4.5 4.5 0 01-6.364-6.364l1.757-1.757m9.86-2.556a4.5 4.5 0 00-1.242-7.244l4.5-4.5a4.5 4.5 0 016.364 6.364l-1.757 1.757'),
  realtime: makeIcon('M3.75 13.5l10.5-11.25L12 10.5h8.25L9.75 21.75 12 13.5H3.75z'),
  quotaControl: makeIcon('M9 12.75L11.25 15 15 9.75m-3-7.036A11.959 11.959 0 013.598 6 11.99 11.99 0 003 9.749c0 5.592 3.824 10.29 9 11.623 5.176-1.332 9-6.03 9-11.622 0-1.31-.21-2.571-.598-3.751h-.152c-3.196 0-6.1-1.248-8.25-3.285z'),
  monitoring: makeIcon('M3 13.125C3 12.504 3.504 12 4.125 12h2.25c.621 0 1.125.504 1.125 1.125v6.75C7.5 20.496 6.996 21 6.375 21h-2.25A1.125 1.125 0 013 19.875v-6.75zM9.75 8.625c0-.621.504-1.125 1.125-1.125h2.25c.621 0 1.125.504 1.125 1.125v11.25c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V8.625zM16.5 4.125c0-.621.504-1.125 1.125-1.125h2.25C20.496 3 21 3.504 21 4.125v15.75c0 .621-.504 1.125-1.125 1.125h-2.25a1.125 1.125 0 01-1.125-1.125V4.125z')
}

function toggleTheme() { isDark.value = !isDark.value; document.documentElement.classList.toggle('dark', isDark.value); localStorage.setItem('theme', isDark.value ? 'dark' : 'light') }
function initTheme() { const s = localStorage.getItem('theme'); if (s === 'dark' || (!s && window.matchMedia('(prefers-color-scheme: dark)').matches)) { isDark.value = true; document.documentElement.classList.add('dark') } }

onMounted(() => { initTheme(); authStore.checkAuth(); if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings() })
</script>
