<template>
  <div data-testid="classic-home" class="home-style-page relative flex min-h-screen min-w-0 flex-col overflow-x-hidden bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950">
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="absolute -right-40 -top-40 h-96 w-96 rounded-full bg-primary-400/20 blur-3xl"></div>
      <div class="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary-500/15 blur-3xl"></div>
      <div class="absolute left-1/3 top-1/4 h-72 w-72 rounded-full bg-primary-300/10 blur-3xl"></div>
      <div class="absolute bottom-1/4 right-1/4 h-64 w-64 rounded-full bg-primary-400/10 blur-3xl"></div>
      <div class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"></div>
    </div>
    <header class="relative z-20 px-6 py-4">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <div class="flex items-center"><div class="h-10 w-10 overflow-hidden rounded-xl shadow-md"><img :src="context.siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" /></div></div>
        <div class="flex items-center gap-3">
          <router-link v-if="context.showModelPlazaEntry" to="/model-plaza" class="text-sm font-medium text-gray-600 transition-colors hover:text-primary-600 dark:text-dark-300 dark:hover:text-primary-400">{{ t('common.nav.modelPlaza') }}</router-link>
          <LocaleSwitcher />
          <a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer" class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white" :title="t('home.viewDocs')"><Icon name="book" size="md" /></a>
          <button @click="context.toggleTheme" class="rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white" :title="context.isDark ? t('home.switchToLight') : t('home.switchToDark')"><Icon v-if="context.isDark" name="sun" size="md" /><Icon v-else name="moon" size="md" /></button>
          <router-link v-if="context.isAuthenticated" :to="context.dashboardPath" class="inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"><span class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white">{{ context.userInitial }}</span><span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span><svg class="h-3 w-3 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25" /></svg></router-link>
          <router-link v-else to="/login" class="inline-flex items-center rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700">{{ t('home.login') }}</router-link>
        </div>
      </nav>
    </header>
    <main class="relative z-10 flex min-w-0 flex-1 px-6 py-16">
      <div class="mx-auto min-w-0 max-w-6xl">
        <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <div class="min-w-0 flex-1 text-center lg:text-left">
            <h1 class="mb-4 [overflow-wrap:anywhere] text-4xl font-bold text-gray-900 dark:text-white md:text-5xl lg:text-6xl">{{ context.siteName }}</h1>
            <p class="mb-8 whitespace-pre-wrap [overflow-wrap:anywhere] text-lg text-gray-600 dark:text-dark-300 md:text-xl">{{ context.siteSubtitle }}</p>
            <div><router-link :to="destination" class="btn btn-primary px-8 py-3 text-base shadow-lg shadow-primary-500/30">{{ context.isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}<Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" /></router-link></div>
          </div>
          <div class="flex min-w-0 flex-1 justify-center lg:justify-end"><div class="terminal-container max-w-full"><div class="terminal-window">
            <div class="terminal-header"><div class="terminal-buttons"><span class="btn-close"></span><span class="btn-minimize"></span><span class="btn-maximize"></span></div><span class="terminal-title">terminal</span></div>
            <div class="terminal-body"><div class="code-line line-1"><span class="code-prompt">$</span><span class="code-cmd">curl</span><span class="code-flag">-X POST</span><span class="code-url">/v1/messages</span></div><div class="code-line line-2"><span class="code-comment"># Routing to upstream...</span></div><div class="code-line line-3"><span class="code-success">200 OK</span><span class="code-response">{ "content": "Hello!" }</span></div><div class="code-line line-4"><span class="code-prompt">$</span><span class="cursor"></span></div></div>
          </div></div></div>
        </div>
        <div class="mb-12 flex flex-wrap items-center justify-center gap-4 md:gap-6"><div v-for="tag in tags" :key="tag.icon" class="inline-flex items-center gap-2.5 rounded-full border border-gray-200/50 bg-white/80 px-5 py-2.5 shadow-sm backdrop-blur-sm dark:border-dark-700/50 dark:bg-dark-800/80"><Icon :name="tag.icon" size="sm" class="text-primary-500" /><span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ t(tag.key) }}</span></div></div>
        <div class="mb-12 grid gap-6 md:grid-cols-3"><article v-for="feature in features" :key="feature.title" class="group rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60"><div :class="['mb-4 flex h-12 w-12 items-center justify-center rounded-xl shadow-lg transition-transform group-hover:scale-110', feature.color]"><Icon v-if="feature.icon" :name="feature.icon" size="lg" class="text-white" /><svg v-else-if="feature.svg" class="h-6 w-6 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="1.5"><path stroke-linecap="round" stroke-linejoin="round" :d="feature.svg" /></svg></div><h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">{{ t(feature.title) }}</h3><p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">{{ t(feature.description) }}</p></article></div>
        <div class="mb-8 text-center"><h2 class="mb-3 text-2xl font-bold text-gray-900 dark:text-white">{{ t('home.providers.title') }}</h2><p class="text-sm text-gray-600 dark:text-dark-400">{{ t('home.providers.description') }}</p></div>
        <div class="mb-16 flex flex-wrap items-center justify-center gap-4"><div v-for="provider in providers" :key="provider.name" :class="['flex items-center gap-2 rounded-xl border px-5 py-3 ring-1 backdrop-blur-sm', provider.muted ? 'border-gray-200/50 bg-white/40 opacity-60 ring-0 dark:border-dark-700/50 dark:bg-dark-800/40' : 'border-primary-200 bg-white/60 ring-primary-500/20 dark:border-primary-800 dark:bg-dark-800/60']"><div :class="['flex h-8 w-8 items-center justify-center rounded-lg bg-gradient-to-br', provider.color]"><span class="text-xs font-bold text-white">{{ provider.letter }}</span></div><span class="text-sm font-medium text-gray-700 dark:text-dark-200">{{ provider.name === 'Claude' ? t('home.providers.claude') : provider.name === 'Gemini' ? t('home.providers.gemini') : provider.name === 'Antigravity' ? t('home.providers.antigravity') : provider.name === 'More' ? t('home.providers.more') : provider.name }}</span><span :class="provider.muted ? 'rounded bg-gray-100 px-1.5 py-0.5 text-[10px] font-medium text-gray-500 dark:bg-dark-700 dark:text-dark-400' : 'rounded bg-primary-100 px-1.5 py-0.5 text-[10px] font-medium text-primary-600 dark:bg-primary-900/30 dark:text-primary-400'">{{ provider.muted ? t('home.providers.soon') : t('home.providers.supported') }}</span></div></div>
      </div>
    </main>
    <footer class="relative z-10 border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50"><div class="mx-auto flex max-w-6xl flex-col items-center justify-center gap-4 text-center sm:flex-row sm:text-left"><p class="text-sm text-gray-500 dark:text-dark-400">&copy; {{ context.currentYear }} {{ context.siteName }}. {{ t('home.footer.allRightsReserved') }}</p><div class="flex items-center gap-4"><a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer" class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white">{{ t('home.docs') }}</a></div></div></footer>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import type { HomeStyleContext } from './types'

const props = defineProps<{ context: HomeStyleContext }>()
const { t } = useI18n()
const destination = computed(() => props.context.isAuthenticated ? props.context.dashboardPath : '/login')
const tags = [{ icon: 'swap', key: 'home.tags.subscriptionToApi' }, { icon: 'shield', key: 'home.tags.stickySession' }, { icon: 'chart', key: 'home.tags.realtimeBilling' }] as const
const features = [
  { icon: 'server', svg: undefined, title: 'home.features.unifiedGateway', description: 'home.features.unifiedGatewayDesc', color: 'bg-gradient-to-br from-blue-500 to-blue-600 shadow-blue-500/30' },
  { icon: undefined, title: 'home.features.multiAccount', description: 'home.features.multiAccountDesc', color: 'bg-gradient-to-br from-primary-500 to-primary-600 shadow-primary-500/30', svg: 'M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z' },
  { icon: undefined, title: 'home.features.balanceQuota', description: 'home.features.balanceQuotaDesc', color: 'bg-gradient-to-br from-purple-500 to-purple-600 shadow-purple-500/30', svg: 'M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z' },
] as const
const providers = [
  { name: 'Claude', letter: 'C', color: 'from-orange-400 to-orange-500', muted: false }, { name: 'GPT', letter: 'G', color: 'from-green-500 to-green-600', muted: false }, { name: 'Gemini', letter: 'G', color: 'from-blue-500 to-blue-600', muted: false }, { name: 'Antigravity', letter: 'A', color: 'from-rose-500 to-pink-600', muted: false }, { name: 'More', letter: '+', color: 'from-gray-500 to-gray-600', muted: true },
] as const
</script>

<style scoped>
.terminal-container { position: relative; display: inline-block; }
.terminal-window { width: 420px; background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%); border-radius: 14px; box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.1), inset 0 1px 0 rgba(255, 255, 255, 0.1); overflow: hidden; transform: perspective(1000px) rotateX(2deg) rotateY(-2deg); transition: transform 0.3s ease; }
.terminal-window:hover { transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px); }
.terminal-header { display: flex; align-items: center; padding: 12px 16px; background: rgba(30, 41, 59, 0.8); border-bottom: 1px solid rgba(255, 255, 255, 0.05); }
.terminal-buttons { display: flex; gap: 8px; }.terminal-buttons span { width: 12px; height: 12px; border-radius: 50%; }.btn-close { background: #ef4444; }.btn-minimize { background: #eab308; }.btn-maximize { background: #22c55e; }
.terminal-title { flex: 1; text-align: center; font-size: 12px; font-family: ui-monospace, monospace; color: #64748b; margin-right: 52px; }
.terminal-body { padding: 20px 24px; font-family: ui-monospace, 'Fira Code', monospace; font-size: 14px; line-height: 2; }.code-line { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; opacity: 0; animation: line-appear 0.5s ease forwards; }.line-1 { animation-delay: 0.3s; }.line-2 { animation-delay: 1s; }.line-3 { animation-delay: 1.8s; }.line-4 { animation-delay: 2.5s; }
@keyframes line-appear { from { opacity: 0; transform: translateY(5px); } to { opacity: 1; transform: translateY(0); } }
.code-prompt { color: #22c55e; font-weight: bold; }.code-cmd { color: #38bdf8; }.code-flag { color: #a78bfa; }.code-url { color: #14b8a6; }.code-comment { color: #64748b; font-style: italic; }.code-success { color: #22c55e; background: rgba(34, 197, 94, 0.15); padding: 2px 8px; border-radius: 4px; font-weight: 600; }.code-response { color: #fbbf24; }
.cursor { display: inline-block; width: 8px; height: 16px; background: #22c55e; animation: blink 1s step-end infinite; } @keyframes blink { 0%, 50% { opacity: 1; } 51%, 100% { opacity: 0; } }
:deep(.dark) .terminal-window { box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.6), 0 0 0 1px rgba(20, 184, 166, 0.2), 0 0 40px rgba(20, 184, 166, 0.1), inset 0 1px 0 rgba(255, 255, 255, 0.1); }
</style>
