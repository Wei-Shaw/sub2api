<template>
  <section v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </section>

  <div
    v-else
    class="relative min-h-screen overflow-x-clip bg-[linear-gradient(180deg,#f7fbff_0%,#f4f7fb_48%,#f7f8fb_100%)] text-slate-900"
  >
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="absolute -left-32 top-0 h-80 w-80 rounded-full bg-sky-200/30 blur-3xl"></div>
      <div class="absolute right-0 top-24 h-96 w-96 rounded-full bg-orange-100/40 blur-3xl"></div>
      <div class="absolute bottom-0 left-1/3 h-72 w-72 rounded-full bg-indigo-200/20 blur-3xl"></div>
      <div class="absolute inset-0 bg-[linear-gradient(rgba(148,163,184,0.06)_1px,transparent_1px),linear-gradient(90deg,rgba(148,163,184,0.06)_1px,transparent_1px)] bg-[size:44px_44px]"></div>
    </div>

    <header class="sticky top-0 z-30 border-b border-slate-200/80 bg-white/88 backdrop-blur-xl">
      <div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-3 lg:px-6">
        <router-link to="/home" class="min-w-0">
          <p class="text-[10px] font-semibold uppercase tracking-[0.18em] text-slate-400">
            {{ localText('Star-X 平台', 'Star-X Platform') }}
          </p>
          <p class="text-lg font-semibold tracking-[-0.03em] text-slate-950">
            {{ siteName }}
          </p>
        </router-link>

        <nav class="hidden items-center gap-1.5 lg:flex" aria-label="Homepage navigation">
          <a
            v-for="item in desktopNavItems"
            :key="item.label"
            :href="item.href"
            class="rounded-full border border-transparent px-3 py-1.5 text-xs font-medium text-slate-500 transition hover:border-sky-200 hover:bg-white hover:text-sky-700"
          >
            {{ item.label }}
          </a>
        </nav>

        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <button
            type="button"
            class="rounded-full border border-slate-200 bg-white px-2.5 py-2 text-slate-500 transition hover:border-slate-300 hover:text-slate-800"
            :title="isDark ? localText('切换到浅色模式', 'Switch to light mode') : localText('切换到深色模式', 'Switch to dark mode')"
            @click="toggleTheme"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="rounded-full border border-slate-200 bg-white px-3 py-2 text-xs font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900"
          >
            {{ isAuthenticated ? localText('控制台', 'Console') : localText('登录', 'Sign in') }}
          </router-link>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/register'"
            class="rounded-full bg-slate-950 px-3.5 py-2 text-xs font-medium text-white shadow-[0_10px_24px_rgba(15,23,42,0.18)] transition hover:bg-slate-800"
          >
            {{ isAuthenticated ? t('home.goToDashboard') : localText('开始使用', 'Get started') }}
          </router-link>
        </div>
      </div>

      <nav class="flex gap-2 overflow-x-auto px-4 pb-3 lg:hidden" aria-label="Compact homepage navigation">
        <a
          v-for="item in mobileNavItems"
          :key="item.label"
          :href="item.href"
          class="shrink-0 rounded-full border border-slate-200 bg-white px-3 py-1.5 text-[11px] font-medium text-slate-600"
        >
          {{ item.label }}
        </a>
      </nav>
    </header>

    <main class="relative z-10 px-4 pb-16 pt-8 lg:px-6 lg:pt-12">
      <div class="mx-auto max-w-6xl">
        <section class="rounded-[28px] border border-white/90 bg-white/75 p-6 shadow-[0_30px_90px_rgba(15,23,42,0.08)] backdrop-blur-xl lg:p-10">
          <div class="mx-auto max-w-4xl text-center">
            <span class="inline-flex items-center gap-2 rounded-full border border-sky-200 bg-sky-50/90 px-3 py-1 text-[11px] font-semibold uppercase tracking-[0.18em] text-sky-700">
              <Icon name="sparkles" size="xs" />
              {{ localText('统一 AI API 网关', 'Unified AI API gateway') }}
            </span>

            <h1 class="mt-6 text-4xl font-semibold tracking-[-0.06em] text-slate-950 md:text-6xl">
              <span>{{ localText('统一 API 网关，服务于', 'One API gateway, built for') }}</span>
              <span class="mt-2 block bg-[linear-gradient(90deg,#4f87da_0%,#8d68ff_52%,#6db6ff_100%)] bg-clip-text text-transparent">
                {{ localText('所有 AI 模型', 'every AI model') }}
              </span>
            </h1>

            <p class="mx-auto mt-6 max-w-3xl text-sm leading-7 text-slate-600 md:text-base md:leading-8">
              {{
                localText(
                  'Star-X 把多上游接入、模型目录、统一密钥、额度、日志、路由与计费收束到同一个控制面，帮助你用一套兼容接口服务更多模型与团队场景。',
                  'Star-X brings multi-upstream access, model catalog, unified keys, quotas, logs, routing, and billing into one control plane so one compatible interface can serve more models and team workflows.',
                )
              }}
            </p>

            <p class="mt-5 text-xs font-semibold uppercase tracking-[0.22em] text-sky-700">
              {{ heroSubtitle }}
            </p>

            <div class="mt-7 flex flex-col justify-center gap-3 sm:flex-row">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/register'"
                class="inline-flex min-h-11 items-center justify-center rounded-full bg-slate-950 px-5 text-sm font-semibold text-white shadow-[0_12px_24px_rgba(15,23,42,0.18)] transition hover:bg-slate-800"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : localText('开始使用', 'Get started') }}
              </router-link>
              <router-link
                :to="pricingPath"
                class="inline-flex min-h-11 items-center justify-center rounded-full border border-slate-200 bg-white px-5 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:text-slate-950"
              >
                {{ localText('查看定价', 'View pricing') }}
              </router-link>
              <a
                :href="resolvedDocUrl"
                :target="docUrl ? '_blank' : undefined"
                rel="noopener noreferrer"
                class="inline-flex min-h-11 items-center justify-center rounded-full border border-sky-200 bg-sky-50 px-5 text-sm font-semibold text-sky-700 transition hover:bg-sky-100"
              >
                {{ localText('查看文档', 'Read docs') }}
              </a>
            </div>
          </div>

          <div class="mt-10 grid gap-6 lg:grid-cols-[1.08fr_0.92fr]">
            <article class="overflow-hidden rounded-[24px] border border-slate-200 bg-white shadow-[0_20px_50px_rgba(15,23,42,0.06)]">
              <header class="flex items-center justify-between gap-3 border-b border-slate-200 px-4 py-3">
                <nav class="flex flex-wrap items-center gap-2 text-[11px] font-semibold">
                  <span
                    v-for="tab in protocolTabs"
                    :key="tab"
                    class="rounded-full px-2.5 py-1"
                    :class="tab === protocolTabs[0] ? 'bg-sky-100 text-sky-700' : 'text-slate-400'"
                  >
                    {{ tab }}
                  </span>
                </nav>
                <span class="inline-flex items-center gap-2 text-[11px] font-medium text-slate-400">
                  <span class="h-2 w-2 rounded-full bg-emerald-500"></span>
                  200 OK
                </span>
              </header>

              <div class="space-y-5 px-4 py-4">
                <div class="grid gap-3 sm:grid-cols-3">
                  <article
                    v-for="item in sampleStats"
                    :key="item.label"
                    class="rounded-2xl border border-slate-200 bg-slate-50/80 p-3"
                  >
                    <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ item.label }}</p>
                    <p class="mt-1 text-lg font-semibold text-slate-950">{{ item.value }}</p>
                  </article>
                </div>

                <div>
                  <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">POST</p>
                  <p class="mt-1 break-all font-mono text-sm text-slate-900">/v1/chat/completions</p>
                </div>

                <div>
                  <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ localText('请求', 'Request') }}</p>
                  <pre class="mt-2 overflow-x-auto rounded-2xl bg-slate-50 p-4 text-[12px] leading-7 text-sky-800">{{ quickCurl }}</pre>
                </div>

                <div>
                  <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ localText('响应', 'Response') }}</p>
                  <pre class="mt-2 overflow-x-auto rounded-2xl bg-slate-50 p-4 text-[12px] leading-7 text-slate-600">{{ sampleResponse }}</pre>
                </div>

                <div class="grid gap-3 border-t border-slate-200 pt-4 sm:grid-cols-2">
                  <article class="rounded-2xl border border-slate-200 bg-white p-4">
                    <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ localText('兼容特性', 'Compatibility') }}</p>
                    <div class="mt-3 flex flex-wrap gap-2">
                      <span
                        v-for="item in compatibilityTags"
                        :key="item"
                        class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-[11px] font-medium text-slate-600"
                      >
                        {{ item }}
                      </span>
                    </div>
                  </article>
                  <article class="rounded-2xl border border-slate-200 bg-white p-4">
                    <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ localText('运行指标', 'Runtime') }}</p>
                    <div class="mt-3 flex flex-wrap gap-2">
                      <span
                        v-for="item in runtimeBadges"
                        :key="item"
                        class="rounded-full border border-slate-200 bg-slate-50 px-2.5 py-1 text-[11px] font-medium text-slate-600"
                      >
                        {{ item }}
                      </span>
                    </div>
                  </article>
                </div>

                <footer class="flex flex-wrap gap-3 border-t border-slate-200 pt-3 text-[11px] font-medium text-slate-400">
                  <span>{{ localText('路由完成', 'Route complete') }}</span>
                  <span>142 ms</span>
                  <span>27 tokens</span>
                  <span>$0.00081</span>
                  <span>SSE</span>
                </footer>
              </div>
            </article>

            <aside class="rounded-[24px] border border-slate-200 bg-white/86 p-5 shadow-[0_20px_50px_rgba(15,23,42,0.05)]">
              <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-sky-700">
                {{ localText('控制面概览', 'Control plane overview') }}
              </p>
              <h2 class="mt-3 text-3xl font-semibold tracking-[-0.05em] text-slate-950">
                {{ localText('统一接入、统一路由、统一计费。', 'One integration surface, one routing layer, one billing view.') }}
              </h2>

              <div class="mt-5 grid gap-3">
                <article
                  v-for="item in sideHighlights"
                  :key="item.title"
                  class="rounded-2xl border border-slate-200 bg-white p-4"
                >
                  <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-sky-700">{{ item.kicker }}</p>
                  <h3 class="mt-1 text-sm font-semibold text-slate-900">{{ item.title }}</h3>
                  <p class="mt-2 text-xs leading-6 text-slate-600">{{ item.copy }}</p>
                </article>
              </div>

              <div class="mt-5 grid gap-3">
                <article class="rounded-2xl border border-slate-200 bg-white p-4">
                  <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ localText('基础地址', 'Base URL') }}</p>
                  <p class="mt-2 break-all font-mono text-[12px] text-sky-800">{{ sampleBaseUrl }}</p>
                </article>
                <article class="rounded-2xl border border-slate-200 bg-white p-4">
                  <p class="text-[10px] font-semibold uppercase tracking-[0.14em] text-slate-400">{{ localText('推荐模型', 'Suggested model') }}</p>
                  <p class="mt-2 break-all font-mono text-[12px] text-sky-800">{{ suggestedModel }}</p>
                </article>
              </div>
            </aside>
          </div>
        </section>

        <section class="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
          <article
            v-for="metric in heroMetrics"
            :key="metric.label"
            class="rounded-3xl border border-white/80 bg-white/88 p-5 shadow-[0_18px_40px_rgba(15,23,42,0.05)]"
          >
            <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-sky-700">{{ metric.label }}</p>
            <p class="mt-3 text-4xl font-semibold tracking-[-0.06em] text-slate-950">{{ metric.value }}</p>
            <p class="mt-2 text-sm leading-6 text-slate-500">{{ metric.note }}</p>
          </article>
        </section>

        <section id="features" class="mt-6 rounded-[28px] border border-white/80 bg-white/90 p-6 shadow-[0_20px_50px_rgba(15,23,42,0.05)] lg:p-8">
          <div class="max-w-3xl">
            <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-sky-700">{{ localText('核心功能', 'Core capabilities') }}</p>
            <h2 class="mt-3 text-3xl font-semibold tracking-[-0.05em] text-slate-950">{{ localText('为开发者打造，为规模而设计。', 'Built for developers, designed for scale.') }}</h2>
          </div>

          <div class="mt-6 grid gap-4 lg:grid-cols-2 xl:grid-cols-4">
            <article
              v-for="item in featureCards"
              :key="item.index"
              class="rounded-3xl border border-slate-200 bg-[linear-gradient(180deg,rgba(255,255,255,0.96),rgba(247,250,255,0.98))] p-5"
            >
              <span class="inline-flex h-9 min-w-9 items-center justify-center rounded-full bg-sky-100 text-sm font-semibold text-sky-700">{{ item.index }}</span>
              <h3 class="mt-4 text-xl font-semibold tracking-[-0.03em] text-slate-950">{{ item.title }}</h3>
              <p class="mt-3 text-sm leading-7 text-slate-600">{{ item.copy }}</p>
              <div class="mt-5 flex flex-wrap gap-2">
                <span
                  v-for="tag in item.tags"
                  :key="`${item.index}-${tag}`"
                  class="rounded-full border border-slate-200 bg-white px-2.5 py-1 text-[11px] font-medium text-slate-500"
                >
                  {{ tag }}
                </span>
              </div>
            </article>
          </div>
        </section>

        <section class="mt-6 grid gap-6 xl:grid-cols-[0.9fr_1.1fr]">
          <article id="workflow" class="rounded-[28px] border border-slate-900 bg-slate-950 p-6 text-white shadow-[0_24px_60px_rgba(15,23,42,0.18)] lg:p-8">
            <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-sky-300">{{ localText('工作流程', 'Workflow') }}</p>
            <h2 class="mt-3 text-3xl font-semibold tracking-[-0.05em]">{{ localText('三步快速上手。', 'Get started in three steps.') }}</h2>

            <div class="mt-6 space-y-3">
              <article
                v-for="step in workflowSteps"
                :key="step.index"
                class="rounded-3xl border border-white/10 bg-white/5 p-5"
              >
                <div class="flex gap-3">
                  <span class="inline-flex h-9 min-w-9 items-center justify-center rounded-full bg-white text-sm font-semibold text-slate-950">{{ step.index }}</span>
                  <div>
                    <h3 class="text-base font-semibold">{{ step.title }}</h3>
                    <p class="mt-2 text-sm leading-7 text-slate-300">{{ step.copy }}</p>
                  </div>
                </div>
              </article>
            </div>
          </article>

          <article id="providers" class="rounded-[28px] border border-white/80 bg-white/90 p-6 shadow-[0_20px_50px_rgba(15,23,42,0.05)] lg:p-8">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div>
                <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-sky-700">{{ localText('模型广场', 'Model plaza') }}</p>
                <h2 class="mt-3 text-3xl font-semibold tracking-[-0.05em] text-slate-950">{{ localText('统一目录下查看当前可用模型。', 'Browse live models from one unified catalog.') }}</h2>
              </div>
              <router-link
                :to="isAuthenticated ? '/dashboard' : '/login'"
                class="inline-flex min-h-10 items-center justify-center rounded-full border border-slate-200 bg-white px-4 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:text-slate-950"
              >
                {{ localText('进入控制台', 'Open console') }}
              </router-link>
            </div>

            <div class="mt-6 grid gap-3 sm:grid-cols-2">
              <article
                v-for="provider in providers"
                :key="provider.name"
                class="rounded-3xl border border-slate-200 bg-slate-50/90 p-5"
              >
                <div class="flex items-center gap-3">
                  <span
                    class="inline-flex h-10 w-10 items-center justify-center rounded-2xl text-sm font-semibold text-white"
                    :style="{ background: provider.color }"
                  >
                    {{ provider.initial }}
                  </span>
                  <div>
                    <h3 class="text-base font-semibold text-slate-950">{{ provider.name }}</h3>
                    <p class="text-xs text-slate-500">{{ provider.status }}</p>
                  </div>
                </div>
                <p class="mt-4 text-sm leading-7 text-slate-600">{{ provider.copy }}</p>
              </article>
            </div>
          </article>
        </section>

        <section id="about" class="mt-6 rounded-[28px] border border-white/80 bg-white/85 p-6 shadow-[0_18px_40px_rgba(15,23,42,0.05)] lg:p-8">
          <div class="flex flex-col gap-5 lg:flex-row lg:items-center lg:justify-between">
            <div class="max-w-2xl">
              <p class="text-[10px] font-semibold uppercase tracking-[0.16em] text-sky-700">{{ localText('准备好开始了吗？', 'Ready to simplify your AI integration?') }}</p>
              <h2 class="mt-3 text-3xl font-semibold tracking-[-0.05em] text-slate-950">
                {{ localText('部署你的网关，通过已配置的上游服务开始转发请求。', 'Deploy your gateway and start routing requests through configured upstream services.') }}
              </h2>
            </div>
            <div class="flex flex-col gap-3 sm:flex-row">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/register'"
                class="inline-flex min-h-11 items-center justify-center rounded-full bg-slate-950 px-5 text-sm font-semibold text-white transition hover:bg-slate-800"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : localText('开始使用', 'Get started') }}
              </router-link>
              <a
                :href="resolvedDocUrl"
                :target="docUrl ? '_blank' : undefined"
                rel="noopener noreferrer"
                class="inline-flex min-h-11 items-center justify-center rounded-full border border-slate-200 bg-white px-5 text-sm font-semibold text-slate-700 transition hover:border-slate-300 hover:text-slate-950"
              >
                {{ localText('查看文档', 'Read docs') }}
              </a>
            </div>
          </div>
        </section>
      </div>
    </main>

    <footer class="relative z-10 border-t border-white/80 px-4 py-8">
      <div class="mx-auto flex max-w-6xl flex-col gap-4 text-center sm:flex-row sm:items-center sm:justify-between sm:text-left">
        <div>
          <p class="text-sm text-slate-500">
            &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
          </p>
        </div>
        <div class="flex flex-wrap items-center justify-center gap-4 text-sm text-slate-500 sm:justify-end">
          <a
            :href="resolvedDocUrl"
            :target="docUrl ? '_blank' : undefined"
            rel="noopener noreferrer"
            class="transition hover:text-slate-900"
          >
            {{ localText('文档', 'Docs') }}
          </a>
          <a href="#providers" class="transition hover:text-slate-900">
            {{ localText('模型', 'Models') }}
          </a>
          <router-link :to="pricingPath" class="transition hover:text-slate-900">
            {{ localText('定价', 'Pricing') }}
          </router-link>
          <a
            :href="githubUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="transition hover:text-slate-900"
          >
            GitHub
          </a>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { locale, t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const isDark = ref(document.documentElement.classList.contains('dark'))

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const pricingPath = computed(() => (isAuthenticated.value ? '/purchase' : '/register'))
const currentYear = computed(() => new Date().getFullYear())
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'
const resolvedDocUrl = computed(() => docUrl.value || '#workflow')
const sampleBaseUrl = computed(() => `${window.location.origin}/v1`)
const suggestedModel = computed(() => 'claude-3-5-sonnet-latest')
const heroSubtitle = computed(() =>
  localText(
    'OpenAI-compatible routing',
    'OpenAI-compatible routing',
  ),
)
const demoPrompt = computed(() =>
  localText(
    '给我一份适用于 AI API 网关上线前的检查清单。',
    'Give me a launch checklist for an AI API gateway.',
  ),
)
const quickCurl = computed(() =>
  [
    `curl ${sampleBaseUrl.value}/chat/completions \\`,
    '  -H "Authorization: Bearer sk-starx-demo" \\',
    '  -H "Content-Type: application/json" \\',
    "  -d '{",
    `    "model": "${suggestedModel.value}",`,
    `    "messages": [{"role":"user","content":"${demoPrompt.value}"}],`,
    '    "stream": true',
    "  }'",
  ].join('\n'),
)
const sampleResponse = computed(() =>
  JSON.stringify(
    {
      id: 'chatcmpl-starx-demo',
      object: 'chat.completion',
      created: 1717329600,
      model: suggestedModel.value,
      choices: [
        {
          index: 0,
          message: {
            role: 'assistant',
            content: localText(
              '已生成上线检查清单，涵盖路由、鉴权、监控、计费与回退策略。',
              'Launch checklist generated, covering routing, auth, monitoring, billing, and failover.',
            ),
          },
          finish_reason: 'stop',
        },
      ],
      usage: {
        prompt_tokens: 17,
        completion_tokens: 10,
        total_tokens: 27,
      },
    },
    null,
    2,
  ),
)

const desktopNavItems = computed(() => [
  { label: localText('主页', 'Home'), href: '#top' },
  { label: localText('控制台', 'Console'), href: isAuthenticated.value ? '/dashboard' : '/login' },
  { label: localText('功能', 'Features'), href: '#features' },
  { label: localText('流程', 'Workflow'), href: '#workflow' },
  { label: localText('模型', 'Models'), href: '#providers' },
  { label: localText('文档', 'Docs'), href: resolvedDocUrl.value },
])

const mobileNavItems = computed(() => [
  { label: localText('主页', 'Home'), href: '#top' },
  { label: localText('功能', 'Features'), href: '#features' },
  { label: localText('流程', 'Workflow'), href: '#workflow' },
  { label: localText('模型', 'Models'), href: '#providers' },
  { label: localText('文档', 'Docs'), href: resolvedDocUrl.value },
])

const protocolTabs = ['Chat', 'Responses', 'Claude', 'Gemini']

const sampleStats = computed(() => [
  { label: localText('状态', 'Status'), value: '200 OK' },
  { label: localText('延迟', 'Latency'), value: '142 ms' },
  { label: localText('总 Tokens', 'Tokens'), value: '27' },
])

const compatibilityTags = computed(() =>
  localTextArray(
    ['OpenAI 兼容', '流式返回', '统一鉴权', '多模型路由'],
    ['OpenAI compatible', 'Streaming', 'Unified auth', 'Model routing'],
  ),
)

const runtimeBadges = computed(() =>
  localTextArray(
    ['负载均衡', '自动回退', '成本跟踪', '统一日志'],
    ['Load balancing', 'Failover', 'Cost tracking', 'Unified logs'],
  ),
)

const sideHighlights = computed(() => [
  {
    kicker: localText('统一接入', 'Unified access'),
    title: localText('一个地址接所有模型', 'One endpoint for all models'),
    copy: localText('统一 OpenAI 兼容入口，减少客户端改造和接入分叉。', 'One OpenAI-compatible surface reduces client rewrites and integration drift.'),
  },
  {
    kicker: localText('统一治理', 'Unified control'),
    title: localText('密钥、额度、路由集中管理', 'Centralized keys, quotas, and routing'),
    copy: localText('把访问控制、分组策略、日志和路由规则收束到同一个控制面。', 'Bring access control, grouping, logs, and routing rules into one control plane.'),
  },
  {
    kicker: localText('统一计费', 'Unified billing'),
    title: localText('模型用量与成本透明', 'Transparent usage and cost'),
    copy: localText('用量、账单和支付状态都从统一视图查看，而不是散在多个上游后台。', 'Track usage, billing, and payment status from one view instead of multiple upstream dashboards.'),
  },
])

const heroMetrics = computed(() =>
  localTextArray(
    [
      { label: '上游服务适配', value: '47+', note: '多上游接入与调度' },
      { label: '模型计费支持', value: '94+', note: '统一模型目录与定价' },
      { label: '兼容 API 路由', value: '47+', note: '支持更多客户端与工作流' },
      { label: '调度控制能力', value: '9+', note: '路由、配额、日志与监控' },
    ],
    [
      { label: 'Upstream integrations', value: '47+', note: 'Multi-upstream routing and switching' },
      { label: 'Pricing entries', value: '94+', note: 'Unified catalog and pricing surface' },
      { label: 'Compatible routes', value: '47+', note: 'Broader client and workflow support' },
      { label: 'Control features', value: '9+', note: 'Routing, quota, logging, and monitoring' },
    ],
  ),
)

const featureCards = computed(() =>
  localTextArray(
    [
      {
        index: '01',
        title: '极速接入',
        copy: '优化过的网关入口和兼容 API 路由，让现有 OpenAI 工作流更快迁移过来。',
        tags: ['OpenAI', 'Claude', 'Gemini'],
      },
      {
        index: '02',
        title: '安全可靠',
        copy: '统一身份、分组、额度和日志边界，避免把权限散落在多个系统。',
        tags: ['权限', '配额', '审计'],
      },
      {
        index: '03',
        title: '全球覆盖',
        copy: '多上游接入、自动回退和统一状态视图，支持稳定的跨区域访问。',
        tags: ['负载均衡', '回退', '监控'],
      },
      {
        index: '04',
        title: '开发者友好',
        copy: '文档、模型广场、代码示例和控制台保持同一套产品语言，减少学习成本。',
        tags: ['API', 'SDK', 'CLI'],
      },
    ],
    [
      {
        index: '01',
        title: 'Fast integration',
        copy: 'An optimized gateway surface and compatible API routes make existing OpenAI workflows easier to migrate.',
        tags: ['OpenAI', 'Claude', 'Gemini'],
      },
      {
        index: '02',
        title: 'Secure and reliable',
        copy: 'Keep identity, groups, quotas, and logs behind one boundary instead of scattering permissions across systems.',
        tags: ['RBAC', 'Quota', 'Audit'],
      },
      {
        index: '03',
        title: 'Global coverage',
        copy: 'Multi-upstream access, automatic fallback, and one status view support reliable cross-region delivery.',
        tags: ['Load balancing', 'Failover', 'Monitoring'],
      },
      {
        index: '04',
        title: 'Developer-friendly',
        copy: 'Docs, catalog, code samples, and console share one product language so the platform is easier to operate.',
        tags: ['API', 'SDK', 'CLI'],
      },
    ],
  ),
)

const workflowSteps = computed(() =>
  localTextArray(
    [
      {
        index: '1',
        title: '配置',
        copy: '添加 API 密钥，接入上游服务，并设置路由和访问策略。',
      },
      {
        index: '2',
        title: '连接',
        copy: '通过 OpenAI 兼容接口、Claude、Gemini 等工作流开始接入。',
      },
      {
        index: '3',
        title: '监控',
        copy: '通过统一的用量、成本、状态和日志视图跟踪运行情况。',
      },
    ],
    [
      {
        index: '1',
        title: 'Configure',
        copy: 'Add API keys, connect upstream services, and define routing plus access policies.',
      },
      {
        index: '2',
        title: 'Connect',
        copy: 'Connect through OpenAI-compatible endpoints, Claude, Gemini, and related workflows.',
      },
      {
        index: '3',
        title: 'Monitor',
        copy: 'Track usage, cost, health, and logs from one shared operational view.',
      },
    ],
  ),
)

const providers = computed(() =>
  localTextArray(
    [
      { initial: 'C', name: 'Claude', status: 'Supported', copy: '高质量通用模型，适合复杂推理与代码工作流。', color: 'linear-gradient(135deg,#fb923c 0%,#f97316 100%)' },
      { initial: 'G', name: 'GPT', status: 'Supported', copy: '覆盖广泛的 OpenAI 兼容调用场景，适合标准客户端接入。', color: 'linear-gradient(135deg,#22c55e 0%,#16a34a 100%)' },
      { initial: 'G', name: 'Gemini', status: 'Supported', copy: '适合多模态与 Google 生态工作流的统一接入。', color: 'linear-gradient(135deg,#3b82f6 0%,#2563eb 100%)' },
      { initial: 'A', name: 'Anthropic / More', status: 'Expandable', copy: '继续扩展更多上游与模型，保持统一路由和治理方式。', color: 'linear-gradient(135deg,#8b5cf6 0%,#7c3aed 100%)' },
    ],
    [
      { initial: 'C', name: 'Claude', status: 'Supported', copy: 'High-quality general-purpose models for reasoning-heavy and coding workflows.', color: 'linear-gradient(135deg,#fb923c 0%,#f97316 100%)' },
      { initial: 'G', name: 'GPT', status: 'Supported', copy: 'Wide OpenAI-compatible coverage for mainstream clients and application flows.', color: 'linear-gradient(135deg,#22c55e 0%,#16a34a 100%)' },
      { initial: 'G', name: 'Gemini', status: 'Supported', copy: 'Unified access for multimodal use cases and Google-oriented workflows.', color: 'linear-gradient(135deg,#3b82f6 0%,#2563eb 100%)' },
      { initial: 'A', name: 'Anthropic / More', status: 'Expandable', copy: 'Extend to more upstreams and models while preserving one routing and governance layer.', color: 'linear-gradient(135deg,#8b5cf6 0%,#7c3aed 100%)' },
    ],
  ),
)

function localText(zh: string, en: string): string {
  return locale.value.startsWith('zh') ? zh : en
}

function localTextArray<T>(zh: T, en: T): T {
  return locale.value.startsWith('zh') ? zh : en
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (savedTheme === 'dark' || (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)) {
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
})
</script>
