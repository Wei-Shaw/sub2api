<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else class="relative min-h-screen overflow-hidden bg-[#FCFDFD] text-slate-900 dark:bg-slate-950 dark:text-white">
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="home-mist home-mist-breathe home-mist-violet"></div>
      <div class="home-mist home-mist-breathe home-mist-cyan"></div>
    </div>

    <header
      class="home-glass-nav fixed inset-x-0 top-0 z-40 transition-all duration-300 ease-out"
      :class="navScrolled ? 'border-b border-white/60 bg-white/80 shadow-[0_12px_40px_rgba(15,23,42,0.06)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-950/70' : 'bg-transparent'"
    >
      <nav class="mx-auto flex h-16 max-w-6xl items-center justify-between gap-8 px-5 sm:px-8">
        <router-link to="/home" class="home-brand flex shrink-0 items-center gap-2.5">
          <span v-if="siteLogo" class="flex h-8 w-8 overflow-hidden rounded-md">
            <img :src="siteLogo" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span v-else class="home-brand-mark inline-flex h-8 w-8 items-center justify-center rounded-md bg-slate-950 font-[Inter,Geist,system-ui,sans-serif] text-[11px] font-extrabold tracking-tight text-white shadow-[0_10px_24px_rgba(15,23,42,0.18)] dark:bg-white dark:text-slate-950">DR</span>
          <span class="home-brand-name font-[Inter,Geist,system-ui,sans-serif] text-base font-extrabold tracking-tight text-slate-950 dark:text-white">{{ siteName }}</span>
        </router-link>

        <div class="home-primary-nav hidden flex-1 items-center justify-center gap-1.5 text-sm font-medium text-slate-500 md:flex">
          <router-link to="/home" class="rounded-md px-2.5 py-1.5 transition-colors hover:bg-slate-100/70 hover:text-slate-950 dark:hover:bg-white/10 dark:hover:text-white">首页</router-link>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="rounded-md px-2.5 py-1.5 transition-colors hover:bg-slate-100/70 hover:text-slate-950 dark:hover:bg-white/10 dark:hover:text-white">文档</a>
          <router-link to="/monitor" target="_blank" rel="noopener noreferrer" class="rounded-md px-2.5 py-1.5 transition-colors hover:bg-slate-100/70 hover:text-slate-950 dark:hover:bg-white/10 dark:hover:text-white">服务状态</router-link>
        </div>

        <div class="home-nav-actions flex shrink-0 items-center gap-2.5">
          <div class="flex items-center gap-1.5">
            <LocaleSwitcher />
          </div>
          <span class="home-nav-divider hidden h-5 w-px bg-slate-200/80 dark:bg-white/10 sm:block"></span>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex h-9 items-center rounded-md bg-slate-950 px-3.5 text-sm font-semibold text-white shadow-[0_10px_30px_rgba(139,92,246,0.18)] transition-all duration-300 hover:-translate-y-0.5 hover:shadow-[0_16px_42px_rgba(139,92,246,0.26)] dark:bg-white dark:text-slate-950"
          >
            进入控制台
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10">
      <section class="home-hero mx-auto flex min-h-[92dvh] max-w-7xl flex-col items-center px-5 pb-16 pt-32 text-center sm:px-8 lg:pt-36">
        <h1 class="max-w-6xl whitespace-nowrap font-[Inter,Geist,system-ui,sans-serif] text-5xl font-black leading-[0.98] tracking-[-0.04em] text-slate-950 sm:text-7xl lg:text-[96px] dark:text-white">
          LLM 的统一接口
        </h1>
        <p class="home-hero-subtitle mt-7 max-w-2xl bg-gradient-to-r from-violet-700 via-slate-700 to-cyan-600 bg-clip-text text-lg font-medium leading-8 text-transparent sm:text-2xl dark:from-violet-300 dark:via-slate-200 dark:to-cyan-300">
          让每个人都用得起顶尖大模型
        </p>

        <div class="mt-9 flex flex-col items-center gap-3 sm:flex-row">
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex h-12 items-center justify-center rounded-md bg-slate-950 px-6 text-sm font-semibold text-white shadow-[0_16px_48px_rgba(139,92,246,0.22)] transition-all duration-300 hover:-translate-y-0.5 hover:shadow-[0_20px_60px_rgba(139,92,246,0.30)] dark:bg-white dark:text-slate-950"
          >
            {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
            <Icon name="arrowRight" size="sm" class="ml-2" />
          </router-link>
          <router-link
            to="/models"
            class="home-secondary-cta inline-flex h-12 items-center justify-center rounded-md border border-slate-200/80 bg-white/30 px-6 text-sm font-semibold text-slate-600 backdrop-blur transition-all duration-300 hover:-translate-y-0.5 hover:bg-white/70 hover:text-slate-950 dark:border-white/10 dark:bg-white/5 dark:text-slate-300 dark:hover:bg-white/10 dark:hover:text-white"
          >
            查看模型广场
          </router-link>
        </div>

        <div class="home-stage mt-12 w-full max-w-3xl overflow-hidden rounded-2xl border border-white/70 bg-white/70 p-1.5 shadow-[0_18px_54px_rgba(15,23,42,0.08)] backdrop-blur-2xl dark:border-white/10 dark:bg-white/10 dark:shadow-[0_18px_54px_rgba(0,0,0,0.28)]">
          <div class="home-shell overflow-hidden rounded-xl border border-white/10 bg-slate-950 text-left text-white shadow-[0_30px_64px_-16px_rgba(139,92,246,0.22),0_22px_42px_-22px_rgba(0,0,0,0.42),inset_0_1px_0_rgba(255,255,255,0.08)]">
            <div class="flex items-center justify-between border-b border-white/10 bg-white/[0.03] px-3.5 py-2.5">
              <div class="flex items-center gap-2">
                <span class="home-shell-dot h-2.5 w-2.5 rounded-full bg-red-400 opacity-80"></span>
                <span class="home-shell-dot h-2.5 w-2.5 rounded-full bg-amber-300 opacity-80"></span>
                <span class="home-shell-dot h-2.5 w-2.5 rounded-full bg-emerald-400 opacity-80"></span>
              </div>
              <span class="font-mono text-xs text-slate-500">api-call.sh</span>
              <span class="w-14"></span>
            </div>

            <pre class="overflow-x-auto whitespace-normal px-3.5 py-3 text-[11px] leading-5 text-slate-300 sm:px-4 sm:py-3.5 sm:text-[12px]"><code><span class="home-shell-line home-shell-line-1"><span class="text-emerald-300">$</span> <span class="text-cyan-200">curl</span> http://localhost:3000/v1/chat/completions \</span>
<span class="home-shell-line home-shell-line-2">  -H <span class="text-violet-200">"Authorization: Bearer $DEVROUTER_API_KEY"</span> \</span>
<span class="home-shell-line home-shell-line-3">  -H <span class="text-violet-200">"Content-Type: application/json"</span> \</span>
<span class="home-shell-line home-shell-line-4">  -d <span class="text-slate-400">'{</span></span>
<span class="home-shell-line home-shell-line-5">    <span class="text-cyan-200">"model"</span>: <span class="text-amber-200">"gpt-4.1"</span>,</span>
<span class="home-shell-line home-shell-line-6">    <span class="text-cyan-200">"messages"</span>: [</span>
<span class="home-shell-line home-shell-line-7">      { <span class="text-cyan-200">"role"</span>: <span class="text-amber-200">"user"</span>, <span class="text-cyan-200">"content"</span>: <span class="text-amber-200">"Hello from DevRouter."</span> }</span>
<span class="home-shell-line home-shell-line-8">    ]</span>
<span class="home-shell-line home-shell-line-9">  <span class="text-slate-400">}'</span><span class="home-shell-cursor"></span></span>
<span class="home-shell-status home-shell-line text-slate-500"># --- Terminal Output ---</span>
<span class="home-shell-summary home-shell-line text-slate-500"># [DevRouter] Request routed to: openai:gpt-4.1</span>
<span class="home-shell-output-1 home-shell-line text-slate-500"># [DevRouter] Latency: 284ms | Cost: optimized</span>
<span class="home-shell-output-2 home-shell-line">{</span>
<span class="home-shell-output-3 home-shell-line">  <span class="text-cyan-200">"id"</span>: <span class="text-amber-200">"chatcmpl-123"</span>,</span>
<span class="home-shell-output-4 home-shell-line">  <span class="text-cyan-200">"choices"</span>: [{ <span class="text-cyan-200">"message"</span>: { <span class="text-cyan-200">"content"</span>: <span class="text-amber-200">"Hello! How can I assist you today?"</span> } }]</span>
<span class="home-shell-output-5 home-shell-line">}</span></code></pre>
          </div>
        </div>
      </section>

      <section class="mx-auto max-w-5xl px-5 py-24 sm:px-8">
        <div class="grid gap-x-10 gap-y-12 md:grid-cols-2 lg:grid-cols-4">
          <div v-for="feature in features" :key="feature.title" class="home-feature-item">
            <div class="home-feature-icon relative mb-4 inline-flex h-11 w-11 items-center justify-center text-slate-800 dark:text-slate-100">
              <span class="home-feature-icon-glow absolute inset-1 rounded-full bg-cyan-400/30 blur-xl"></span>
              <Icon :name="feature.icon" size="lg" :stroke-width="1.5" />
            </div>
            <h3 class="text-base font-semibold text-slate-950 dark:text-white">{{ feature.title }}</h3>
            <p class="mt-2 text-sm leading-7 text-slate-600">{{ feature.description }}</p>
          </div>
        </div>
      </section>

      <section class="mx-auto max-w-6xl px-5 py-12 sm:px-8">
        <div class="home-provider-wall flex flex-wrap items-center justify-center gap-x-10 gap-y-6 text-slate-400/50">
          <div v-for="provider in providers" :key="provider.name" class="home-provider-logo inline-flex items-center gap-2 opacity-50 grayscale">
            <PlatformIcon :platform="provider.platform" size="lg" />
            <span class="text-sm font-semibold tracking-wide">{{ provider.name }}</span>
          </div>
        </div>
      </section>

      <section class="home-footer-cta home-cta-grid relative mx-auto max-w-5xl overflow-hidden px-5 py-28 text-center [mask-image:radial-gradient(ellipse_at_center,black_40%,transparent_72%)] sm:px-8">
        <div class="pointer-events-none absolute bottom-0 right-0 h-80 w-80 rounded-full bg-cyan-400/10 blur-[110px]"></div>
        <h2 class="text-balance text-3xl font-bold tracking-[-0.02em] text-slate-950 sm:text-5xl dark:text-white">准备好构建你的 AI 应用了吗？</h2>
        <p class="mx-auto mt-5 max-w-2xl text-base leading-8 text-slate-500">用一个统一入口连接模型、计费、监控与团队权限。</p>
        <router-link
          :to="isAuthenticated ? dashboardPath : '/login'"
          class="mt-8 inline-flex h-12 items-center justify-center rounded-md bg-slate-950 px-6 text-sm font-semibold text-white shadow-[0_16px_48px_rgba(139,92,246,0.20)] transition-all duration-300 hover:-translate-y-0.5 dark:bg-white dark:text-slate-950"
        >
          进入控制台
        </router-link>
      </section>
    </main>

    <footer class="relative z-10 border-t border-slate-200/70 px-5 py-8 text-xs text-slate-400 dark:border-white/10 sm:px-8">
      <div class="mx-auto flex max-w-7xl flex-col items-center justify-between gap-4 sm:flex-row">
        <p>&copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}</p>
        <div class="flex items-center gap-5">
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="transition-colors hover:text-slate-700 dark:hover:text-white">文档</a>
          <router-link to="/legal/terms" class="transition-colors hover:text-slate-700 dark:hover:text-white">条款</router-link>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import type { GroupPlatform } from '@/types'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'DevRouter')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const navScrolled = ref(false)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())

const features = [
  {
    icon: 'globe',
    title: '统一入口',
    description: '一个兼容接口聚合主流模型，应用侧无需反复改造。'
  },
  {
    icon: 'sync',
    title: '智能路由',
    description: '按可用性、延迟和成本选择上游，降低维护成本。'
  },
  {
    icon: 'shield',
    title: '稳定治理',
    description: '密钥、分组、限额和状态统一管理，适合团队长期运行。'
  },
  {
    icon: 'chart',
    title: '透明计费',
    description: '用量、余额和模型价格清晰可查，业务成本更可控。'
  },
] as const

const providers: Array<{ name: string, platform: GroupPlatform }> = [
  { name: 'OpenAI', platform: 'openai' },
  { name: 'Anthropic', platform: 'anthropic' },
  { name: 'Gemini', platform: 'gemini' },
  { name: 'Antigravity', platform: 'antigravity' },
]

function updateNavState() {
  navScrolled.value = window.scrollY > 12
}

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  updateNavState()
  window.addEventListener('scroll', updateNavState, { passive: true })
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', updateNavState)
})
</script>

<style scoped>
.home-mist {
  position: absolute;
  width: min(58rem, 120vw);
  height: min(58rem, 120vw);
  border-radius: 9999px;
  filter: blur(150px);
  opacity: 0.11;
}

.home-mist-violet {
  left: -24rem;
  top: -18rem;
  background: #8b5cf6;
}

.home-mist-cyan {
  right: -24rem;
  bottom: 2rem;
  background: #06b6d4;
}

.home-mist-breathe {
  animation: mist-breathe 10s ease-in-out infinite alternate;
}

.home-cta-grid::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(100, 116, 139, 0.08) 1px, transparent 1px),
    linear-gradient(90deg, rgba(100, 116, 139, 0.08) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(circle at center, black 0%, transparent 68%);
}

.home-shell-line {
  display: block;
  white-space: pre;
  opacity: 0;
  transform: translateY(6px);
  animation: shell-line-in 420ms ease-out forwards;
}

.home-shell-line-1 {
  animation-delay: 120ms;
}

.home-shell-line-2 {
  animation-delay: 260ms;
}

.home-shell-line-3 {
  animation-delay: 400ms;
}

.home-shell-line-4 {
  animation-delay: 540ms;
}

.home-shell-line-5 {
  animation-delay: 680ms;
}

.home-shell-line-6 {
  animation-delay: 820ms;
}

.home-shell-line-7 {
  animation-delay: 960ms;
}

.home-shell-line-8 {
  animation-delay: 1100ms;
}

.home-shell-line-9 {
  animation-delay: 1240ms;
}

.home-shell-status {
  animation-delay: 1700ms;
}

.home-shell-summary {
  animation-delay: 1880ms;
}

.home-shell-output-1 {
  animation-delay: 2060ms;
}

.home-shell-output-2 {
  animation-delay: 2240ms;
}

.home-shell-output-3 {
  animation-delay: 2420ms;
}

.home-shell-output-4 {
  animation-delay: 2600ms;
}

.home-shell-output-5 {
  animation-delay: 2780ms;
}

.home-shell-cursor {
  display: inline-block;
  width: 0.55em;
  height: 1.1em;
  margin-left: 0.35em;
  vertical-align: -0.18em;
  border-radius: 1px;
  background: #67e8f9;
  animation: shell-cursor-blink 900ms steps(2, jump-none) infinite;
}

@keyframes shell-line-in {
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes shell-cursor-blink {
  50% {
    opacity: 0;
  }
}

@keyframes mist-breathe {
  from {
    opacity: 0.08;
    transform: translate3d(0, 0, 0) scale(1);
  }
  to {
    opacity: 0.15;
    transform: translate3d(2rem, 1rem, 0) scale(1.08);
  }
}

@media (prefers-reduced-motion: reduce) {
  * {
    animation-duration: 1ms !important;
    transition-duration: 1ms !important;
  }
}
</style>
