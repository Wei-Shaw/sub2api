<template>
  <div class="home-shell min-h-screen text-slate-950">
    <section class="hero-stage">
      <header class="relative z-20">
        <nav class="mx-auto flex max-w-[1120px] items-center justify-between gap-5 px-5 py-[22px]">
          <router-link :to="homePath" class="flex min-w-0 flex-1 items-center gap-3">
            <span :class="['brand-mark', siteLogo ? 'brand-mark-image' : '']">
              <img
                v-if="siteLogo"
                :src="siteLogo"
                alt=""
                class="brand-logo-image"
              />
              <span v-else>{{ logoLetters }}</span>
            </span>
            <span class="min-w-0">
              <span class="block truncate text-base font-semibold leading-5 text-slate-700">
                {{ pageCopy.brand }}
              </span>
              <span class="hidden truncate text-sm leading-5 text-slate-500 sm:block">
                {{ pageCopy.subtitle }}
              </span>
            </span>
          </router-link>

          <div class="flex shrink-0 items-center gap-2 sm:gap-4">
            <LocaleSwitcher />
            <router-link
              to="/agents"
              class="text-xs font-medium text-slate-500 transition hover:text-slate-900 sm:text-sm"
            >
              Agents Hub
            </router-link>
            <a
              :href="docHref"
              :target="docUrl ? '_blank' : undefined"
              :rel="docUrl ? 'noopener noreferrer' : undefined"
              class="text-xs font-medium text-slate-500 transition hover:text-slate-900 sm:text-sm"
            >
              {{ pageCopy.docsLabel }}
            </a>
            <a
              :href="pageCopy.navAnchor"
              class="hidden text-sm font-medium text-slate-500 transition hover:text-slate-900 lg:inline"
            >
              {{ pageCopy.navAnchorLabel }}
            </a>
            <a
              v-if="!isInternalHome && pageCopy.contactTitle"
              href="#contact"
              class="hidden text-sm font-medium text-slate-500 transition hover:text-slate-900 lg:inline"
            >
              {{ pageCopy.contactNavLabel }}
            </a>
            <router-link
              v-if="!isInternalHome"
              :to="publicEntryRoute"
              class="nav-button"
            >
              {{ pageCopy.navAction }}
            </router-link>
            <a
              v-else
              :href="primaryHref"
              class="nav-button"
            >
              {{ pageCopy.navAction }}
            </a>
          </div>
        </nav>
      </header>

      <div class="hero-section mx-auto grid max-w-[1120px] gap-14 px-5 lg:grid-cols-[minmax(0,1.02fr)_minmax(340px,0.78fr)] lg:items-center">
        <div class="max-w-4xl">
          <div class="eyebrow-pill">
            <span class="h-2 w-2 rounded-full bg-sky-500"></span>
            {{ pageCopy.eyebrow }}
          </div>

          <h1 class="hero-title mt-7 max-w-4xl font-black tracking-normal text-slate-950">
            {{ pageCopy.titleLead }}
            <span class="hero-highlight">{{ pageCopy.titleHighlight }}</span>
          </h1>

          <p class="hero-copy mt-7 max-w-3xl text-slate-600">
            {{ pageCopy.description }}
          </p>

          <div class="mt-9 flex flex-wrap gap-3">
            <router-link
              v-if="!isInternalHome"
              :to="publicEntryRoute"
              class="primary-action"
            >
              {{ pageCopy.primaryAction }}
            </router-link>
            <a
              v-else
              :href="primaryHref"
              class="primary-action"
            >
              {{ pageCopy.primaryAction }}
            </a>

            <a
              :href="pageCopy.secondaryHref"
              :target="pageCopy.secondaryExternal ? '_blank' : undefined"
              :rel="pageCopy.secondaryExternal ? 'noopener noreferrer' : undefined"
              class="secondary-action"
            >
              {{ pageCopy.secondaryAction }}
            </a>
          </div>

          <div class="hero-meta">
            <span
              v-for="item in pageCopy.meta"
              :key="item.label"
            >
              <b>{{ item.label }}</b> {{ item.value }}
            </span>
          </div>
        </div>

        <aside
          v-if="!isInternalHome"
          class="hero-panel"
          aria-label="OceanWay API example"
        >
          <div class="hero-panel-head">
            <div class="hero-dots">
              <i></i>
              <i></i>
              <i></i>
            </div>
            <span>oceanway.api</span>
          </div>
          <div class="hero-code">
            <div><span class="code-muted">$</span> curl {{ endpointBase }}/chat/completions</div>
            <div>-H <span class="code-gold">"Authorization: Bearer sk-..."</span></div>
            <div>-d <span class="code-gold">'{ "model": "your-model", "messages": [...] }'</span></div>
            <br />
            <div><span class="code-ok">{{ pageCopy.codeStatus }}</span> <span class="code-muted">{{ pageCopy.codeNote }}</span></div>
          </div>
          <dl class="hero-metrics">
            <div
              v-for="fact in pageCopy.facts"
              :key="fact.label"
              class="hero-metric"
            >
              <dt>{{ fact.label }}</dt>
              <dd>{{ fact.value }}</dd>
            </div>
          </dl>
        </aside>

        <aside
          v-else
          class="internal-hero-panel"
          :aria-label="pageCopy.panelTitle"
        >
          <div class="internal-panel-head">
            <p>{{ pageCopy.panelEyebrow }}</p>
            <h2>{{ pageCopy.panelTitle }}</h2>
          </div>
          <div class="internal-task-grid">
            <article
              v-for="task in pageCopy.heroTasks"
              :key="task.title"
              class="internal-task-card"
            >
              <h3>{{ task.title }}</h3>
              <p>{{ task.description }}</p>
            </article>
          </div>
          <p class="internal-panel-note">{{ pageCopy.panelNote }}</p>
        </aside>
      </div>
    </section>

    <main class="relative z-10">
      <section id="intro" class="border-y border-slate-200/75 bg-white/70">
        <div class="mx-auto max-w-7xl px-5 py-10">
          <div class="section-heading">
            <p>{{ pageCopy.introEyebrow }}</p>
            <h2>{{ pageCopy.introTitle }}</h2>
            <span>{{ pageCopy.introDescription }}</span>
          </div>

          <div class="mt-7 grid gap-4 md:grid-cols-3">
            <article
              v-for="feature in pageCopy.features"
              :key="feature.title"
              class="feature-panel"
            >
              <p class="text-sm font-black text-slate-950">{{ feature.title }}</p>
              <p class="mt-2 text-sm leading-6 text-slate-600">{{ feature.description }}</p>
            </article>
          </div>
        </div>
      </section>

      <section
        v-if="pageCopy.managedOffers.length"
        id="scenarios"
        class="mx-auto max-w-7xl px-5 py-12"
      >
        <div class="section-heading">
          <p>{{ pageCopy.managedEyebrow }}</p>
          <h2>{{ pageCopy.managedTitle }}</h2>
          <span>{{ pageCopy.managedDescription }}</span>
        </div>

        <div class="scenario-grid mt-7">
          <article
            v-for="offer in pageCopy.managedOffers"
            :key="offer.title"
            class="scenario-card"
          >
            <p class="text-base font-black text-slate-950">{{ offer.title }}</p>
            <p class="mt-3 text-sm leading-6 text-slate-600">{{ offer.description }}</p>
            <p class="mt-5 text-sm font-bold text-sky-800">{{ offer.note }}</p>
          </article>
        </div>
      </section>

      <section
        v-if="renderedPricingGroups.length"
        id="pricing"
        class="mx-auto max-w-7xl px-5 py-12"
      >
        <div class="section-heading">
          <p>{{ pricingHeader.eyebrow }}</p>
          <h2>{{ pricingHeader.title }}</h2>
          <span>{{ pricingHeader.description }}</span>
        </div>

        <div
          v-for="group in renderedPricingGroups"
          :key="group.title"
          class="pricing-group"
        >
          <div class="pricing-group-heading">
            <h3>{{ group.title }}</h3>
            <p>{{ group.description }}</p>
          </div>

          <div class="pricing-grid">
            <article
              v-for="plan in group.plans"
              :key="plan.name"
              class="pricing-card"
              :class="{ 'pricing-card-highlight': plan.highlight }"
            >
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="text-base font-black text-slate-950">{{ plan.name }}</p>
                  <p class="mt-1 text-sm leading-6 text-slate-500">{{ plan.description }}</p>
                </div>
                <span v-if="plan.badge" class="plan-badge">{{ plan.badge }}</span>
              </div>

              <div class="mt-5">
                <span class="text-3xl font-black text-slate-950">{{ plan.price }}</span>
                <span class="ml-1 text-sm font-semibold text-slate-500">{{ plan.period }}</span>
              </div>

              <dl class="plan-metrics">
                <div
                  v-for="item in plan.metrics"
                  :key="item.label"
                  class="plan-metric-row"
                >
                  <dt class="text-slate-500">{{ item.label }}</dt>
                  <dd class="text-right font-bold text-slate-900">{{ item.value }}</dd>
                </div>
              </dl>

              <a
                :href="getPricingPlanHref(plan)"
                :target="isPricingPlanExternal(plan) ? '_blank' : undefined"
                :rel="isPricingPlanExternal(plan) ? 'noopener noreferrer' : undefined"
                class="primary-action pricing-buy-action w-full"
              >
                {{ pageCopy.buyAction }}
              </a>
            </article>
          </div>
        </div>
      </section>

      <section id="workflow" class="border-y border-slate-200/75 bg-white/70">
        <div class="mx-auto max-w-7xl px-5 py-10">
          <div class="section-heading">
            <p>{{ pageCopy.workflowEyebrow }}</p>
            <h2>{{ pageCopy.workflowTitle }}</h2>
            <span>{{ pageCopy.workflowDescription }}</span>
          </div>

          <div class="mt-7 grid gap-4 md:grid-cols-4">
            <article
              v-for="(step, index) in pageCopy.workflow"
              :key="step.title"
              class="step-panel"
            >
              <span class="step-number">{{ index + 1 }}</span>
              <h3>{{ step.title }}</h3>
              <p>{{ step.description }}</p>
            </article>
          </div>
        </div>
      </section>

      <section
        v-if="pageCopy.contactTitle"
        id="contact"
        class="contact-section mx-auto max-w-7xl px-5 py-12"
      >
        <div class="contact-card">
          <div class="section-heading">
            <p>{{ pageCopy.contactEyebrow }}</p>
            <h2>{{ pageCopy.contactTitle }}</h2>
            <span>{{ pageCopy.contactDescription }}</span>
          </div>
          <div class="contact-qr-wrap">
            <img
              :src="contactQrSrc"
              :alt="pageCopy.contactQrAlt"
              class="contact-qr"
            />
            <p>{{ pageCopy.contactQrNote }}</p>
          </div>
        </div>
      </section>

      <section id="faq" class="mx-auto max-w-7xl px-5 py-12">
        <div class="section-heading">
          <p>{{ pageCopy.faqEyebrow }}</p>
          <h2>{{ pageCopy.faqTitle }}</h2>
        </div>

        <div class="mt-7 grid gap-3 md:grid-cols-2">
          <details
            v-for="item in pageCopy.faq"
            :key="item.question"
            class="faq-item"
          >
            <summary>{{ item.question }}</summary>
            <p>{{ item.answer }}</p>
          </details>
        </div>
      </section>
    </main>

    <footer class="relative z-10 border-t border-slate-200/80 bg-white/75 px-5 py-6">
      <div class="mx-auto flex max-w-7xl flex-col gap-3 text-sm text-slate-500 sm:flex-row sm:items-center sm:justify-between">
        <p>&copy; {{ currentYear }} {{ pageCopy.brand }}.</p>
        <div class="flex flex-wrap gap-4">
          <router-link to="/agents" class="transition hover:text-slate-900">Agents Hub</router-link>
          <a
            :href="docHref"
            :target="docUrl ? '_blank' : undefined"
            :rel="docUrl ? 'noopener noreferrer' : undefined"
            class="transition hover:text-slate-900"
          >
            {{ pageCopy.docsLabel }}
          </a>
          <a
            v-if="contactFooterHref"
            :href="contactFooterHref"
            class="transition hover:text-slate-900"
          >
            {{ contactLabel }}
          </a>
          <span v-else>{{ contactLabel }}</span>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import type { HomePricingConfig, HomePricingLocalizedText } from '@/types'

type TextPair = {
  label: string
  value: string
}

type HomeCard = {
  title: string
  description: string
}

type PricePlan = {
  name: string
  description: string
  price: string
  period: string
  badge?: string
  highlight?: boolean
  purchaseHref?: string
  purchaseExternal?: boolean
  metrics: TextPair[]
}

type PriceGroup = {
  title: string
  description: string
  plans: PricePlan[]
}

type WorkflowStep = {
  title: string
  description: string
}

type ManagedOffer = {
  title: string
  description: string
  note: string
}

type FaqItem = {
  question: string
  answer: string
}

const appStore = useAppStore()
const route = useRoute()
const { locale } = useI18n()

const purchaseUrl = 'https://pay.ldxp.cn/shop/E8WHWMVD'
const contactQrSrc = '/wechat-contact-qr.png'

const isInternalHome = computed(() => route.name === 'InternalHome')
const isZh = computed(() => locale.value.toLowerCase().startsWith('zh'))
const homePath = computed(() => isInternalHome.value ? '/internal-home' : '/home')
const publicEntryRoute = '/login?redirect=/dashboard'

const siteName = computed(() => {
  const configured = appStore.cachedPublicSettings?.site_name || appStore.siteName
  return configured?.trim() || 'OceanWay AI'
})
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const contactInfo = computed(() => appStore.cachedPublicSettings?.contact_info || appStore.contactInfo || '')
const apiBaseUrl = computed(() => appStore.cachedPublicSettings?.api_base_url || appStore.apiBaseUrl || '')

const logoLetters = computed(() => {
  const uppercaseLetters = siteName.value.match(/[A-Z]/g)?.slice(0, 2).join('')
  return uppercaseLetters || siteName.value.slice(0, 2).toUpperCase()
})

const endpointBase = computed(() => {
  const currentOrigin = getCurrentOrigin()
  if (isInternalHome.value) {
    return isLocalOrigin(currentOrigin) ? 'https://oceanwayai.site/v1' : `${currentOrigin}/v1`
  }

  const configured = apiBaseUrl.value.trim().replace(/\/+$/, '')
  if (configured) return configured
  return isLocalOrigin(currentOrigin) ? 'https://oceanway.site/v1' : `${currentOrigin}/v1`
})

const contactHref = computed(() => {
  const raw = contactInfo.value.trim()
  if (!raw) return ''
  if (/^https?:\/\//i.test(raw) || /^mailto:/i.test(raw) || /^tel:/i.test(raw)) return raw
  if (/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(raw)) return `mailto:${raw}`
  return ''
})

const contactLabel = computed(() => {
  const configured = contactInfo.value.trim()
  if (configured) return configured
  if (isInternalHome.value) return isZh.value ? '联系我们' : 'Contact us'
  return isZh.value ? '微信咨询' : 'WeChat'
})
const contactFooterHref = computed(() => contactHref.value || (pageCopy.value.contactTitle ? '#contact' : ''))
const primaryHref = computed(() => isInternalHome.value ? '#contact' : (contactHref.value || docUrl.value || '#contact'))
const docHref = computed(() => docUrl.value || '/docs')
const currentYear = computed(() => new Date().getFullYear())
const homePricingConfig = computed(() => {
  if (isInternalHome.value) return null
  return appStore.cachedPublicSettings?.home_pricing_config || null
})

const pricingHeader = computed(() => {
  const cfg = homePricingConfig.value
  if (!cfg) {
    return {
      eyebrow: pageCopy.value.pricingEyebrow,
      title: pageCopy.value.pricingTitle,
      description: pageCopy.value.pricingDescription,
    }
  }
  return {
    eyebrow: pickHomePricingText(cfg.eyebrow),
    title: pickHomePricingText(cfg.title),
    description: pickHomePricingText(cfg.description),
  }
})

const renderedPricingGroups = computed(() => {
  const cfg = homePricingConfig.value
  if (!cfg) return pageCopy.value.pricingGroups
  const groups: PriceGroup[] = []
  const subscriptionPlans = (cfg.subscription_cards || [])
    .filter(card => card.enabled && card.for_sale !== false)
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0))
    .map(card => ({
      name: pickHomePricingText(card.name),
      description: pickHomePricingText(card.description),
      price: formatCnyPrice(card.price || 0),
      period: pickHomePricingText(card.period),
      badge: pickHomePricingOptionalText(card.badge),
      highlight: card.highlight,
      purchaseHref: resolveHomePricingHref(cfg, 'subscription', card.subscription_plan_id),
      purchaseExternal: cfg.cta_mode !== 'internal',
      metrics: (card.metrics || []).map(metric => ({
        label: pickHomePricingText(metric.label),
        value: pickHomePricingText(metric.value),
      })),
    }))
  if (subscriptionPlans.length) {
    groups.push({
      title: pickHomePricingText(cfg.subscription_group.title),
      description: pickHomePricingText(cfg.subscription_group.description),
      plans: subscriptionPlans,
    })
  }
  const creditPlans = (cfg.credit_cards || [])
    .filter(card => card.enabled)
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0))
    .map(card => ({
      name: pickHomePricingText(card.name),
      description: pickHomePricingText(card.description),
      price: formatCnyPrice(card.price || card.recharge_amount || 0),
      period: pickHomePricingText(card.period),
      badge: pickHomePricingOptionalText(card.badge),
      highlight: card.highlight,
      purchaseHref: resolveHomePricingHref(cfg, 'credit', card.recharge_amount),
      purchaseExternal: cfg.cta_mode !== 'internal',
      metrics: (card.metrics || []).map(metric => ({
        label: pickHomePricingText(metric.label),
        value: pickHomePricingText(metric.value),
      })),
    }))
  if (creditPlans.length) {
    groups.push({
      title: pickHomePricingText(cfg.credit_group.title),
      description: pickHomePricingText(cfg.credit_group.description),
      plans: creditPlans,
    })
  }
  return groups.length ? groups : pageCopy.value.pricingGroups
})

function getCurrentOrigin() {
  if (typeof window === 'undefined') return ''
  return window.location.origin.replace(/\/+$/, '')
}

function isLocalOrigin(origin: string) {
  return /^https?:\/\/(localhost|127\.0\.0\.1|\[::1\])(?::\d+)?$/i.test(origin)
}

function pickHomePricingText(value?: HomePricingLocalizedText | null) {
  if (!value) return ''
  const preferred = isZh.value ? value.zh : value.en
  const fallback = isZh.value ? value.en : value.zh
  return preferred?.trim() || fallback?.trim() || ''
}

function pickHomePricingOptionalText(value?: HomePricingLocalizedText | null) {
  const text = pickHomePricingText(value)
  return text || undefined
}

function formatCnyPrice(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '¥0'
  return `¥${Number.isInteger(value) ? value.toFixed(0) : value.toFixed(1)}`
}

function resolveHomePricingHref(cfg: HomePricingConfig, kind: 'subscription' | 'credit', value: number) {
  if (cfg.cta_mode === 'internal') {
    if (kind === 'subscription') return `/payment?tab=subscription&plan_id=${encodeURIComponent(String(value))}`
    return `/payment?tab=recharge&amount=${encodeURIComponent(String(value))}`
  }
  return cfg.external_purchase_url || purchaseUrl
}

function getPricingPlanHref(plan: PricePlan) {
  if (isInternalHome.value) return '#contact'
  return plan.purchaseHref || purchaseUrl
}

function isPricingPlanExternal(plan: PricePlan) {
  if (isInternalHome.value) return false
  return plan.purchaseExternal !== false
}

const pageCopy = computed(() => {
  if (isInternalHome.value) {
    return isZh.value ? internalZhCopy.value : internalEnCopy.value
  }
  return isZh.value ? publicZhCopy.value : publicEnCopy.value
})

const publicZhCopy = computed(() => ({
  brand: siteName.value,
  subtitle: 'AI 工具用户的一站式工作台',
  docsLabel: '文档',
  navAction: '注册 / 登录',
  navAnchor: '#pricing',
  navAnchorLabel: '定价',
  contactNavLabel: '微信咨询',
  eyebrow: 'OceanWay AI Workspace',
  titleLead: '为 AI 工具用户准备的',
  titleHighlight: '一站式工作台。',
  description:
    '把稳定模型接入、Agents Hub、额度套餐和控制台管理放在同一个入口里。适合需要长期使用 AI 工具、Agent prompt 和多模型 API 的个人与团队。',
  primaryAction: '开始使用',
  secondaryAction: '查看定价',
  secondaryHref: '#pricing',
  secondaryExternal: false,
  panelEyebrow: '',
  panelTitle: '',
  panelNote: '',
  heroTasks: [] satisfies HomeCard[],
  contactEyebrow: '微信咨询',
  contactTitle: '扫码添加微信，咨询套餐和使用方式。',
  contactDescription: '如果你不确定订阅套餐、额度套餐或具体工具配置是否适合自己，可以先扫码沟通使用场景，我们会按你的任务类型给出建议。',
  contactQrAlt: 'OceanWay AI 微信联系二维码',
  contactQrNote: '微信扫码咨询套餐、额度与上手方式',
  codeStatus: '200 OK',
  codeNote: '已路由 / 已计量 / 可追踪',
  meta: [
    { label: 'Base URL', value: endpointBase.value },
    { label: 'Agent', value: '200 个专业模板' },
    { label: '购买', value: '订阅套餐 / 额度套餐' },
  ] satisfies TextPair[],
  facts: [
    { label: '入口', value: '/v1' },
    { label: 'Agent', value: '200+' },
    { label: '购买', value: '订阅 / 额度' },
  ] satisfies TextPair[],
  introEyebrow: '核心能力',
  introTitle: '围绕 AI 工具工作流，而不是只给一个 API 地址。',
  introDescription: '从模型接入、Agent 选择到额度管理，主页要让用户快速知道这里能帮自己持续使用 AI 工具。',
  features: [
    {
      title: '稳定模型接入',
      description: '使用统一 Base URL 和 API Key 接入可用模型，减少不同工具反复配置的成本。',
    },
    {
      title: 'Agents Hub',
      description: '浏览并复制专业 Agent Markdown，用于 Codex、Claude Code、OpenCode 或自己的工作流。',
    },
    {
      title: '用量与额度管理',
      description: '通过控制台创建 Key、查看用量，并用订阅或额度套餐维持日常 AI 工具调用。',
    },
  ] satisfies HomeCard[],
  pricingEyebrow: '定价',
  pricingTitle: '先选择套餐，注册登录后开始使用。',
  pricingDescription: 'token 数量为营销估算，实际消耗会随模型、输入输出长度和工具行为变化。',
  pricingGroups: [
    {
      title: '订阅套餐',
      description: '适合每天持续使用 AI 工具的用户。',
      plans: [
        {
          name: '日卡',
          description: '短期试用或临时高频使用。',
          price: '¥8.8',
          period: '/ 日',
          metrics: [
            { label: '每日额度', value: '约 $50' },
            { label: 'token 估算', value: '约 5000 万' },
          ],
        },
        {
          name: '月卡 轻量版',
          description: '低门槛长期使用，适合首次付费和日常学习。',
          price: '¥49.9',
          period: '/ 月',
          badge: '入门',
          metrics: [
            { label: '每日额度', value: '约 $20' },
            { label: 'token 估算', value: '约 2000 万 / 天' },
          ],
        },
        {
          name: '月卡 标准版',
          description: '稳定高频使用，适合论文、代码和备考主力场景。',
          price: '¥229.9',
          period: '/ 月',
          badge: '推荐',
          highlight: true,
          metrics: [
            { label: '每日额度', value: '约 $100' },
            { label: 'token 估算', value: '约 1 亿 / 天' },
          ],
        },
        {
          name: '月卡 高阶版',
          description: '适合重度使用、多工具调用和多项目场景。',
          price: '¥399.9',
          period: '/ 月',
          metrics: [
            { label: '每日额度', value: '约 $200' },
            { label: 'token 估算', value: '约 2 亿 / 天' },
          ],
        },
      ],
    },
    {
      title: '额度套餐',
      description: '适合月卡不够用时灵活补充，额度无每日限制。',
      plans: [
        {
          name: '$80 额度',
          description: '不够用时补充。',
          price: '¥20',
          period: '',
          metrics: [
            { label: '额度', value: '$80' },
            { label: '类型', value: '余额额度' },
          ],
        },
        {
          name: '$180 额度',
          description: '常用补充，适合临时项目加量。',
          price: '¥40',
          period: '',
          badge: '常用',
          highlight: true,
          metrics: [
            { label: '额度', value: '$180' },
            { label: '类型', value: '余额额度' },
          ],
        },
        {
          name: '$1000 额度',
          description: '适合高频用户、长期使用或拼车。',
          price: '¥200',
          period: '',
          metrics: [
            { label: '额度', value: '$1000' },
            { label: '类型', value: '余额额度' },
          ],
        },
      ],
    },
  ] satisfies PriceGroup[],
  buyAction: '前往购买',
  managedEyebrow: '',
  managedTitle: '',
  managedDescription: '',
  managedOffers: [] satisfies ManagedOffer[],
  workflowEyebrow: '使用流程',
  workflowTitle: '购买后注册登录，完成 Key 和 Agent 工作流配置。',
  workflowDescription: '主页负责说明与购买入口，实际 Key、用量和调用都在登录后的控制台完成。',
  workflow: [
    { title: '购买套餐', description: '根据使用频率选择日卡、月卡或额度套餐。' },
    { title: '注册 / 登录', description: '进入账号后创建 API Key，并查看套餐、额度和账户状态。' },
    { title: '配置工具', description: '在 AI 工具中填写 Base URL、Key 和模型名。' },
    { title: '使用 Agents', description: '从 Agents Hub 复制模板，放入自己的开发或内容工作流。' },
  ] satisfies WorkflowStep[],
  faqEyebrow: 'FAQ',
  faqTitle: '购买前常见问题',
  faq: [
    {
      question: '订阅套餐和额度套餐有什么区别？',
      answer: '订阅套餐适合固定周期内每天持续使用；额度套餐适合月卡不够用时按需补充余额，覆盖临时加量需求。',
    },
    {
      question: 'token 数量是严格承诺吗？',
      answer: '不是。token 数量是便于理解的估算，实际消耗会受模型、输入输出长度和工具调用方式影响。',
    },
    {
      question: '购买后在哪里使用？',
      answer: '购买后注册或登录账号，在控制台创建 API Key，再按文档把工具指向对应 Base URL。',
    },
    {
      question: 'Agents Hub 是什么？',
      answer: 'Agents Hub 是专业 Agent Markdown 的浏览与复制入口，可以配合 Codex、Claude Code、OpenCode 等工具使用。',
    },
  ] satisfies FaqItem[],
}))

const publicEnCopy = computed(() => ({
  brand: siteName.value,
  subtitle: 'AI workspace for tool users',
  docsLabel: 'Docs',
  navAction: 'Sign up / Sign in',
  navAnchor: '#pricing',
  navAnchorLabel: 'Pricing',
  contactNavLabel: 'WeChat',
  eyebrow: 'OceanWay AI Workspace',
  titleLead: 'One workspace for',
  titleHighlight: 'AI tool users.',
  description:
    'Bring stable model access, Agents Hub, credit packages, and console management into one entry point for people who rely on AI tools, agent prompts, and multi-model APIs every day.',
  primaryAction: 'Start using',
  secondaryAction: 'View pricing',
  secondaryHref: '#pricing',
  secondaryExternal: false,
  panelEyebrow: '',
  panelTitle: '',
  panelNote: '',
  heroTasks: [] satisfies HomeCard[],
  contactEyebrow: 'WeChat',
  contactTitle: 'Scan to ask about plans and usage.',
  contactDescription: 'If you are not sure whether a subscription, credit package, or tool setup fits your work, share your scenario first and we will suggest the right path.',
  contactQrAlt: 'OceanWay AI WeChat contact QR code',
  contactQrNote: 'Scan with WeChat to ask about plans and setup',
  codeStatus: '200 OK',
  codeNote: 'routed, metered, logged',
  meta: [
    { label: 'Base URL', value: endpointBase.value },
    { label: 'Agents', value: '200 specialist templates' },
    { label: 'Plans', value: 'subscriptions / credits' },
  ] satisfies TextPair[],
  facts: [
    { label: 'Path', value: '/v1' },
    { label: 'Agents', value: '200+' },
    { label: 'Plans', value: 'subs / credits' },
  ] satisfies TextPair[],
  introEyebrow: 'Core Capabilities',
  introTitle: 'Built around AI tool workflows, not just another API endpoint.',
  introDescription: 'The homepage should quickly show how model access, agent selection, and credit management fit into one practical workspace.',
  features: [
    {
      title: 'Stable model access',
      description: 'Use one Base URL and API key across compatible tools, reducing repeated provider setup.',
    },
    {
      title: 'Agents Hub',
      description: 'Browse and copy specialist agent Markdown for Codex, Claude Code, OpenCode, or your own workflows.',
    },
    {
      title: 'Usage and credit control',
      description: 'Create keys, inspect usage, and keep daily AI tool calls running through subscriptions or credit packages.',
    },
  ] satisfies HomeCard[],
  pricingEyebrow: 'Pricing',
  pricingTitle: 'Choose a plan, then sign in to start.',
  pricingDescription: 'Token counts are marketing estimates. Actual usage varies by model, input/output length, and tool behavior.',
  pricingGroups: [
    {
      title: 'Subscription Plans',
      description: 'For users who run AI tools continuously.',
      plans: [
        {
          name: 'Day Pass',
          description: 'Short trials or temporary intensive usage.',
          price: '¥8.8',
          period: '/ day',
          metrics: [
            { label: 'Daily credit', value: 'about $50' },
            { label: 'Token estimate', value: 'about 50M' },
          ],
        },
        {
          name: 'Monthly Light',
          description: 'A low-friction monthly plan for first purchases and everyday study.',
          price: '¥49.9',
          period: '/ month',
          badge: 'Starter',
          metrics: [
            { label: 'Daily credit', value: 'about $20' },
            { label: 'Token estimate', value: 'about 20M / day' },
          ],
        },
        {
          name: 'Monthly Standard',
          description: 'For frequent papers, coding, exam prep, and steady AI tool usage.',
          price: '¥229.9',
          period: '/ month',
          badge: 'Popular',
          highlight: true,
          metrics: [
            { label: 'Daily credit', value: 'about $100' },
            { label: 'Token estimate', value: 'about 100M / day' },
          ],
        },
        {
          name: 'Monthly Advanced',
          description: 'For heavy usage, multi-tool workflows, and multiple projects.',
          price: '¥399.9',
          period: '/ month',
          metrics: [
            { label: 'Daily credit', value: 'about $200' },
            { label: 'Token estimate', value: 'about 200M / day' },
          ],
        },
      ],
    },
    {
      title: 'Credit Packages',
      description: 'Flexible top-ups when your monthly plan is not enough. Credits have no daily limit.',
      plans: [
        {
          name: '$80 Credit',
          description: 'Top up when your plan is not enough.',
          price: '¥20',
          period: '',
          metrics: [
            { label: 'Credit', value: '$80' },
            { label: 'Type', value: 'balance credit' },
          ],
        },
        {
          name: '$180 Credit',
          description: 'A practical refill for temporary project bursts.',
          price: '¥40',
          period: '',
          badge: 'Common',
          highlight: true,
          metrics: [
            { label: 'Credit', value: '$180' },
            { label: 'Type', value: 'balance credit' },
          ],
        },
        {
          name: '$1000 Credit',
          description: 'For high-frequency users, long-running usage, or shared access.',
          price: '¥200',
          period: '',
          metrics: [
            { label: 'Credit', value: '$1000' },
            { label: 'Type', value: 'balance credit' },
          ],
        },
      ],
    },
  ] satisfies PriceGroup[],
  buyAction: 'Buy now',
  managedEyebrow: '',
  managedTitle: '',
  managedDescription: '',
  managedOffers: [] satisfies ManagedOffer[],
  workflowEyebrow: 'Workflow',
  workflowTitle: 'After purchase, sign in for keys and agents.',
  workflowDescription: 'The homepage explains the offer. Keys, usage, and calls stay inside the signed-in console.',
  workflow: [
    { title: 'Buy a package', description: 'Choose a subscription or credit package based on usage frequency.' },
    { title: 'Sign up / Sign in', description: 'Create an API key and inspect plan, credit, and account status.' },
    { title: 'Configure tools', description: 'Set Base URL, API key, and model name in compatible AI tools.' },
    { title: 'Use agents', description: 'Copy agent templates from Agents Hub into development or content workflows.' },
  ] satisfies WorkflowStep[],
  faqEyebrow: 'FAQ',
  faqTitle: 'Common questions before purchase',
  faq: [
    {
      question: 'What is the difference between subscriptions and credits?',
      answer: 'Subscriptions fit recurring daily usage during a fixed period. Credit packages are balance top-ups for flexible or irregular calls.',
    },
    {
      question: 'Are token counts guaranteed?',
      answer: 'No. Token counts are estimates for readability. Actual usage depends on model, prompt length, output length, and tool behavior.',
    },
    {
      question: 'Where do I use it after purchase?',
      answer: 'Sign up or sign in, create an API key in the console, then follow the docs to configure the matching Base URL.',
    },
    {
      question: 'What is Agents Hub?',
      answer: 'Agents Hub is a browser and copy surface for specialist agent Markdown used with tools such as Codex, Claude Code, and OpenCode.',
    },
  ] satisfies FaqItem[],
}))

const internalZhCopy = computed(() => {
  const brand = `${siteName.value} 专业工作台`
  return {
    brand,
    subtitle: '给专业工作者的 Codex 与 Agents Hub',
    docsLabel: '文档',
    navAction: '联系我们',
    navAnchor: '#pricing',
    navAnchorLabel: '定价',
    contactNavLabel: '联系我们',
    eyebrow: 'OceanWay AI Productivity',
    titleLead: '让专业工作者也能用',
    titleHighlight: 'Codex 解放生产力。',
    description:
      'OceanWay AI 将 Codex、Agents Hub、专业任务模板和使用指导整合到一个专业工作台里，帮助医生、科研人员、律师和办公人员更快完成论文、资料、报告、合同和日常文书。',
    primaryAction: '联系我们',
    secondaryAction: '查看适用场景',
    secondaryHref: '#scenarios',
    secondaryExternal: false,
    panelEyebrow: '可以直接开始的工作',
    panelTitle: '不用理解模型，也能把任务交给 AI 处理。',
    panelNote: '按任务选择合适的 Agent，再用 Codex 逐步完成真实工作。',
    heroTasks: [
      { title: '论文与资料', description: '文献摘要、论文初稿、摘要润色、投稿信和汇报提纲。' },
      { title: '科研与生信', description: '实验思路、代码辅助、结果解释、流程整理和英文表达优化。' },
      { title: '法律与合同', description: '合同条款梳理、案情摘要、材料目录和文书初稿。' },
      { title: '办公与文员', description: '会议纪要、周报月报、通知制度、表格说明和流程文档。' },
    ] satisfies HomeCard[],
    codeStatus: '',
    codeNote: '',
    meta: [
      { label: '适合人群', value: '医生 / 科研 / 律师 / 办公' },
      { label: '核心工具', value: 'Codex + Agents Hub' },
      { label: '上手方式', value: '场景模板 + 使用指导' },
    ] satisfies TextPair[],
    facts: [
      { label: 'Codex', value: '任务协作' },
      { label: 'Agents', value: '专业模板' },
      { label: 'Support', value: '使用指导' },
    ] satisfies TextPair[],
    introEyebrow: '不是技术工具',
    introTitle: '你不需要先学会 AI，才开始提高效率。',
    introDescription: '这个页面面向没有 AI 和编程背景的专业工作者：你只需要带着真实任务进入工作台，OceanWay AI 帮你把工具、模板和使用路径准备好。',
    features: [
      {
        title: 'Codex 工作台',
        description: '把写作、整理、分析和修改变成可以一步步推进的任务，让 AI 帮你处理重复但耗时的脑力劳动。',
      },
      {
        title: 'Agents Hub',
        description: '内置专业 Agent 模板，不用自己研究 prompt，也能让 Codex 按论文、法律、办公等场景工作。',
      },
      {
        title: '使用支持',
        description: '围绕真实任务讲清楚怎么用，帮助非技术用户从“不会用 AI”过渡到“每天用 AI 做事”。',
      },
    ] satisfies HomeCard[],
    pricingEyebrow: '套餐',
    pricingTitle: '选择适合你的使用周期。',
    pricingDescription: '适合想先体验、集中完成任务，或长期把 AI 作为日常工作助手的用户。',
    pricingGroups: [
      {
        title: 'Codex 生产力套餐',
        description: '扫码咨询开通后，即可使用 Codex 与 Agents Hub。',
        plans: [
          {
            name: '日卡',
            description: '适合临时体验，或当天集中处理一批文档。',
            price: '¥16.6',
            period: '/ 日',
            metrics: [
              { label: '周期', value: '1 天' },
              { label: '适合', value: '体验 / 临时任务' },
            ],
          },
          {
            name: '周卡',
            description: '适合一周内集中推进论文、报告或项目材料。',
            price: '¥66.6',
            period: '/ 周',
            metrics: [
              { label: '周期', value: '7 天' },
              { label: '适合', value: '短期项目' },
            ],
          },
          {
            name: '月卡',
            description: '适合持续使用 AI 辅助论文、文书和日常办公。',
            price: '¥199',
            period: '/ 月',
            badge: '推荐',
            highlight: true,
            metrics: [
              { label: '周期', value: '30 天' },
              { label: '适合', value: '长期工作流' },
            ],
          },
        ],
      },
    ] satisfies PriceGroup[],
    buyAction: '联系我们',
    contactEyebrow: '联系我们',
    contactTitle: '扫码添加微信，咨询开通方式。',
    contactDescription: '如果你不确定适合日卡、周卡还是月卡，可以先扫码沟通你的使用场景，我们会按你的工作内容给出建议。',
    contactQrAlt: 'OceanWay AI 微信联系二维码',
    contactQrNote: '微信扫码咨询开通与使用方式',
    managedEyebrow: '适用场景',
    managedTitle: '围绕真实职业任务，而不是让你学习复杂技术。',
    managedDescription: 'OceanWay AI 的价值在于把 Codex 和 Agent 模板放到具体工作里，让不同职业用户都能找到自己的使用方式。',
    managedOffers: [
      {
        title: '医生与医学工作者',
        description: '辅助整理病例学习资料、医学科普、会议发言稿、论文初稿和英文润色。',
        note: '把重复写作和资料整理交给 AI。',
      },
      {
        title: '生信与科研人员',
        description: '辅助文献阅读、实验思路梳理、代码解释、结果讨论和论文表达优化。',
        note: '更快从资料走到可写内容。',
      },
      {
        title: '律师与合规人员',
        description: '辅助合同审阅、条款摘要、案情材料整理、证据目录和法律文书初稿。',
        note: '提高材料处理和表达效率。',
      },
      {
        title: '文员与行政办公',
        description: '辅助会议纪要、周报月报、制度通知、流程说明和表格材料整理。',
        note: '减少低价值重复文书工作。',
      },
    ] satisfies ManagedOffer[],
    workflowEyebrow: '使用流程',
    workflowTitle: '不用研究配置，直接按任务开始。',
    workflowDescription: '你只需要选择合适的工作场景，把资料和目标交给 Codex，再结合 Agent 模板持续迭代。',
    workflow: [
      { title: '联系我们', description: '扫码沟通你的职业场景和主要任务。' },
      { title: '选择套餐', description: '按使用频率选择日卡、周卡或月卡。' },
      { title: '选择 Agent', description: '在 Agents Hub 找到适合论文、法律、办公等任务的模板。' },
      { title: '交付成果', description: '让 Codex 生成、修改、总结和整理，最后由你完成专业判断。' },
    ] satisfies WorkflowStep[],
    faqEyebrow: 'FAQ',
    faqTitle: '购买前常见问题',
    faq: [
      {
        question: '我不懂 AI、不懂代码，可以用吗？',
        answer: '可以。这个页面面向非技术专业用户，核心是用 Codex 和现成 Agent 模板辅助真实工作，而不是要求你理解 API 或模型配置。',
      },
      {
        question: 'Codex 能帮我写论文吗？',
        answer: '它适合辅助文献整理、提纲、初稿、摘要、润色和表达优化，但研究结论、数据解释和投稿内容仍需要你作为专业人士最终确认。',
      },
      {
        question: '医疗和法律内容可以完全依赖 AI 吗？',
        answer: '不可以。AI 适合作为资料整理、初稿生成和表达优化助手，医疗、法律和科研判断必须由具备资质或专业能力的人最终负责。',
      },
      {
        question: 'Agents Hub 是什么？',
        answer: 'Agents Hub 是 OceanWay AI 的特色能力，里面是面向不同工作场景的专业 Agent 模板，可以让 Codex 更像一个懂任务的助手。',
      },
    ] satisfies FaqItem[],
  }
})

const internalEnCopy = computed(() => {
  const brand = `${siteName.value} Professional Workspace`
  return {
    brand,
    subtitle: 'Codex and Agents Hub for professional work',
    docsLabel: 'Docs',
    navAction: 'Contact us',
    navAnchor: '#pricing',
    navAnchorLabel: 'Pricing',
    contactNavLabel: 'Contact',
    eyebrow: 'OceanWay AI Productivity',
    titleLead: 'Codex productivity for',
    titleHighlight: 'non-technical professionals.',
    description:
      'OceanWay AI brings Codex, Agents Hub, professional task templates, and practical guidance into one workspace for doctors, researchers, lawyers, and office teams who want faster papers, reports, contracts, and daily documents.',
    primaryAction: 'Contact us',
    secondaryAction: 'View use cases',
    secondaryHref: '#scenarios',
    secondaryExternal: false,
    panelEyebrow: 'Work you can start with',
    panelTitle: 'Use AI for real tasks without learning model settings.',
    panelNote: 'Choose an agent for the task, then use Codex to iterate toward usable output.',
    heroTasks: [
      { title: 'Papers and reading', description: 'Literature summaries, first drafts, abstracts, polishing, cover letters, and presentation outlines.' },
      { title: 'Research and bioinformatics', description: 'Experimental ideas, code assistance, result interpretation, workflow notes, and English expression.' },
      { title: 'Legal and contracts', description: 'Clause review, case summaries, document organization, evidence lists, and first-draft legal writing.' },
      { title: 'Office documents', description: 'Meeting notes, weekly reports, notices, process documents, spreadsheet explanations, and admin writing.' },
    ] satisfies HomeCard[],
    codeStatus: '',
    codeNote: '',
    meta: [
      { label: 'For', value: 'doctors / researchers / lawyers / office teams' },
      { label: 'Tools', value: 'Codex + Agents Hub' },
      { label: 'Start with', value: 'task templates + guidance' },
    ] satisfies TextPair[],
    facts: [
      { label: 'Codex', value: 'task work' },
      { label: 'Agents', value: 'templates' },
      { label: 'Support', value: 'guided use' },
    ] satisfies TextPair[],
    introEyebrow: 'Not a technical tool',
    introTitle: 'You do not need to become an AI expert before saving time.',
    introDescription: 'This page is for professional users without AI or programming background. Bring a real task, and OceanWay AI prepares the tools, templates, and usage path.',
    features: [
      {
        title: 'Codex workspace',
        description: 'Turn writing, summarizing, analysis, and revision into step-by-step tasks that AI can help move forward.',
      },
      {
        title: 'Agents Hub',
        description: 'Use ready-made specialist agent templates instead of learning prompt engineering from scratch.',
      },
      {
        title: 'Guided support',
        description: 'Learn through real work scenarios, so non-technical users can go from unsure to using AI every day.',
      },
    ] satisfies HomeCard[],
    pricingEyebrow: 'Plans',
    pricingTitle: 'Choose the usage period that fits your work.',
    pricingDescription: 'Use a short pass to try it, a weekly pass for focused projects, or a monthly pass for daily AI-assisted work.',
    pricingGroups: [
      {
        title: 'Codex Productivity Plans',
        description: 'Scan to ask about access, then use Codex and Agents Hub for your work.',
        plans: [
          {
            name: 'Day Pass',
            description: 'For a quick trial or one focused day of document work.',
            price: '¥16.6',
            period: '/ day',
            metrics: [
              { label: 'Period', value: '1 day' },
              { label: 'Best for', value: 'trial / urgent tasks' },
            ],
          },
          {
            name: 'Week Pass',
            description: 'For papers, reports, or project materials you want to finish within a week.',
            price: '¥66.6',
            period: '/ week',
            metrics: [
              { label: 'Period', value: '7 days' },
              { label: 'Best for', value: 'short projects' },
            ],
          },
          {
            name: 'Monthly Pass',
            description: 'For ongoing papers, documents, and daily professional work.',
            price: '¥199',
            period: '/ month',
            badge: 'Recommended',
            highlight: true,
            metrics: [
              { label: 'Period', value: '30 days' },
              { label: 'Best for', value: 'daily workflow' },
            ],
          },
        ],
      },
    ] satisfies PriceGroup[],
    buyAction: 'Contact us',
    contactEyebrow: 'Contact',
    contactTitle: 'Scan the QR code to ask about access.',
    contactDescription: 'If you are not sure whether a day, week, or monthly pass fits you, share your work scenario first and we will suggest the right path.',
    contactQrAlt: 'OceanWay AI WeChat contact QR code',
    contactQrNote: 'Scan with WeChat to contact us',
    managedEyebrow: 'Use Cases',
    managedTitle: 'Built around professional tasks, not technical setup.',
    managedDescription: 'OceanWay AI is valuable because it places Codex and agent templates directly into the work people already need to finish.',
    managedOffers: [
      {
        title: 'Doctors and medical workers',
        description: 'Support case-study notes, patient education material, talks, paper drafts, and English polishing.',
        note: 'Let AI handle repetitive writing and organization.',
      },
      {
        title: 'Bioinformatics and researchers',
        description: 'Support literature reading, experimental thinking, code explanation, result discussion, and paper expression.',
        note: 'Move faster from source material to usable writing.',
      },
      {
        title: 'Lawyers and compliance teams',
        description: 'Support contract review, clause summaries, case material organization, evidence lists, and legal drafts.',
        note: 'Improve document handling and expression.',
      },
      {
        title: 'Office and admin work',
        description: 'Support meeting minutes, weekly reports, policies, notices, process notes, and spreadsheet explanations.',
        note: 'Reduce repetitive low-value writing work.',
      },
    ] satisfies ManagedOffer[],
    workflowEyebrow: 'Workflow',
    workflowTitle: 'Start from the task instead of configuration.',
    workflowDescription: 'Choose a work scenario, bring your material and goal to Codex, then iterate with the right agent template.',
    workflow: [
      { title: 'Contact us', description: 'Scan the QR code and tell us your role and main work tasks.' },
      { title: 'Choose a plan', description: 'Choose a day, week, or monthly pass based on your usage rhythm.' },
      { title: 'Choose an agent', description: 'Pick a template for papers, legal work, office writing, or research.' },
      { title: 'Produce output', description: 'Ask Codex to draft, revise, summarize, and organize, then apply your professional judgment.' },
    ] satisfies WorkflowStep[],
    faqEyebrow: 'FAQ',
    faqTitle: 'Questions before purchase',
    faq: [
      {
        question: 'Can I use it without AI or coding knowledge?',
        answer: 'Yes. This page is built for non-technical professional users. The goal is to use Codex and ready-made agents for real work, not to understand APIs or model settings.',
      },
      {
        question: 'Can Codex help me write papers?',
        answer: 'It can help with literature organization, outlines, first drafts, abstracts, polishing, and expression. Research conclusions, data interpretation, and submission content still need your final professional review.',
      },
      {
        question: 'Can medical or legal output be fully trusted?',
        answer: 'No. AI is useful for organizing material, drafting, and improving expression. Medical, legal, and research decisions must remain under qualified human responsibility.',
      },
      {
        question: 'What is Agents Hub?',
        answer: 'Agents Hub is a key OceanWay AI feature: it provides specialist agent templates for different tasks so Codex behaves more like a task-aware assistant.',
      },
    ] satisfies FaqItem[],
  }
})

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.home-shell {
  position: relative;
  overflow: hidden;
  background: #f6f8fb;
}

.hero-stage {
  position: relative;
  z-index: 1;
  display: flex;
  min-height: 92vh;
  flex-direction: column;
  background:
    linear-gradient(90deg, rgba(246, 248, 251, 0.98) 0%, rgba(246, 248, 251, 0.9) 42%, rgba(246, 248, 251, 0.3) 72%),
    url("https://images.unsplash.com/photo-1507525428034-b723cf961d3e?auto=format&fit=crop&w=1800&q=82")
      center / cover no-repeat;
  border-bottom: 1px solid rgba(16, 24, 39, 0.12);
}

.hero-section {
  flex: 1;
  padding-top: 52px;
  padding-bottom: 72px;
}

.hero-title {
  font-size: clamp(46px, 7vw, 76px);
  line-height: 0.96;
}

.hero-copy {
  font-size: 1.125rem;
  line-height: 1.72;
}

.brand-mark {
  display: inline-flex;
  width: 2.375rem;
  height: 2.375rem;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: linear-gradient(135deg, #002080 0%, #001040 78%);
  color: rgba(255, 255, 255, 0.78);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0;
  box-shadow: 0 10px 26px rgba(15, 23, 42, 0.18);
}

.brand-mark-image {
  overflow: hidden;
  border-radius: 0.65rem;
  background: #071f66;
  box-shadow: none;
}

.brand-logo-image {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
  transform: scale(1.02);
}

.nav-button,
.primary-action,
.secondary-action {
  display: inline-flex;
  min-height: 2.5rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  font-size: 0.875rem;
  font-weight: 800;
  white-space: nowrap;
  transition:
    border-color 0.16s ease,
    background-color 0.16s ease,
    color 0.16s ease,
    transform 0.16s ease,
    box-shadow 0.16s ease;
}

.nav-button,
.secondary-action {
  border: 1px solid rgba(148, 163, 184, 0.35);
  background: rgba(255, 255, 255, 0.82);
  padding: 0.5rem 1rem;
  color: #111827;
  box-shadow: 0 8px 20px rgba(15, 23, 42, 0.05);
}

.primary-action {
  background: #001040;
  padding: 0.72rem 1.05rem;
  color: white;
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.12);
}

.nav-button:hover,
.primary-action:hover,
.secondary-action:hover {
  transform: translateY(-1px);
}

.primary-action:hover {
  background: #002080;
}

.secondary-action:hover,
.nav-button:hover {
  border-color: rgba(71, 85, 105, 0.45);
}

@media (max-width: 460px) {
  .brand-mark {
    width: 2.125rem;
    height: 2.125rem;
  }

  .nav-button {
    min-height: 2.25rem;
    padding: 0.44rem 0.7rem;
    font-size: 0.8125rem;
  }
}

.eyebrow-pill {
  display: inline-flex;
  min-height: 2.25rem;
  align-items: center;
  gap: 0.6rem;
  border: 1px solid rgba(0, 160, 255, 0.24);
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.7);
  padding: 0.45rem 0.85rem;
  color: #005db8;
  font-size: 0.875rem;
  font-weight: 800;
  box-shadow: 0 12px 28px rgba(15, 23, 42, 0.04);
}

.hero-highlight {
  color: #006fd6;
}

.hero-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem 1.125rem;
  margin-top: 38px;
  color: #64748b;
  font-size: 0.8125rem;
  line-height: 1.5;
}

.hero-meta b {
  color: #0f172a;
}

.hero-panel {
  width: 100%;
  overflow: hidden;
  justify-self: end;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.86);
  box-shadow: 0 28px 70px rgba(16, 24, 39, 0.18);
  backdrop-filter: blur(14px);
}

.hero-panel-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid rgba(16, 24, 39, 0.12);
  padding: 14px 16px;
}

.hero-panel-head span {
  color: #64748b;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 0.75rem;
}

.hero-dots {
  display: flex;
  gap: 7px;
}

.hero-dots i {
  display: block;
  width: 10px;
  height: 10px;
  border-radius: 50%;
}

.hero-dots i:nth-child(1) {
  background: #e05858;
}

.hero-dots i:nth-child(2) {
  background: #d9912b;
}

.hero-dots i:nth-child(3) {
  background: #0ea5e9;
}

.hero-code {
  overflow-x: auto;
  background: #101827;
  color: #d8e2f0;
  padding: 20px;
  font: 13px/1.8 ui-monospace, SFMono-Regular, Menlo, monospace;
}

.code-muted {
  color: #7f8ea3;
}

.code-ok {
  color: #5eead4;
}

.code-gold {
  color: #f3c36b;
}

.hero-metrics {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  border-top: 1px solid rgba(16, 24, 39, 0.12);
}

.hero-metric {
  min-width: 0;
  border-right: 1px solid rgba(16, 24, 39, 0.12);
  padding: 18px 16px;
}

.hero-metric:last-child {
  border-right: 0;
}

.hero-metric dt {
  margin-top: 4px;
  color: #64748b;
  font-size: 0.75rem;
  line-height: 1.45;
}

.hero-metric dd {
  color: #0f172a;
  font-size: 1.25rem;
  font-weight: 900;
  line-height: 1.2;
  overflow-wrap: anywhere;
}

.internal-hero-panel {
  width: 100%;
  justify-self: end;
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.84);
  padding: 1.25rem;
  box-shadow: 0 28px 70px rgba(16, 24, 39, 0.16);
  backdrop-filter: blur(14px);
}

.internal-panel-head p {
  color: #006fd6;
  font-size: 0.75rem;
  font-weight: 900;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.internal-panel-head h2 {
  margin-top: 0.55rem;
  color: #0f172a;
  font-size: 1.35rem;
  font-weight: 900;
  line-height: 1.25;
}

.internal-task-grid {
  display: grid;
  gap: 0.75rem;
  margin-top: 1rem;
}

.internal-task-card {
  border: 1px solid rgba(203, 213, 225, 0.76);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.74);
  padding: 0.9rem;
}

.internal-task-card h3 {
  color: #0f172a;
  font-size: 0.95rem;
  font-weight: 900;
}

.internal-task-card p {
  margin-top: 0.45rem;
  color: #475569;
  font-size: 0.83rem;
  line-height: 1.6;
}

.internal-panel-note {
  margin-top: 1rem;
  border-top: 1px solid rgba(16, 24, 39, 0.1);
  padding-top: 1rem;
  color: #005db8;
  font-size: 0.86rem;
  font-weight: 800;
  line-height: 1.6;
}

@media (max-width: 900px) {
  .hero-stage {
    min-height: auto;
    background:
      linear-gradient(rgba(246, 248, 251, 0.9), rgba(246, 248, 251, 0.9)),
      url("https://images.unsplash.com/photo-1507525428034-b723cf961d3e?auto=format&fit=crop&w=1200&q=82")
        center / cover no-repeat;
  }

  .hero-section {
    padding-top: 3.25rem;
    padding-bottom: 4.25rem;
  }

  .hero-panel {
    max-width: 35rem;
    justify-self: start;
  }

  .internal-hero-panel {
    max-width: 35rem;
    justify-self: start;
  }
}

@media (max-width: 560px) {
  .hero-section {
    gap: 2.4rem;
  }

  .hero-title {
    font-size: 42px;
  }

  .hero-copy {
    font-size: 1rem;
  }

  .hero-meta {
    margin-top: 1.8rem;
  }

  .hero-metrics {
    grid-template-columns: 1fr;
  }

  .hero-metric {
    border-right: 0;
    border-bottom: 1px solid rgba(16, 24, 39, 0.12);
  }

  .hero-metric:last-child {
    border-bottom: 0;
  }
}

.feature-panel,
.scenario-card,
.pricing-card,
.step-panel,
.faq-item {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.82);
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.04);
}

.section-heading {
  max-width: 48rem;
}

.section-heading p {
  color: #006fd6;
  font-size: 0.78rem;
  font-weight: 900;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.section-heading h2 {
  margin-top: 0.55rem;
  color: #0f172a;
  font-size: 1.75rem;
  font-weight: 900;
  line-height: 1.25;
}

.section-heading span {
  display: block;
  margin-top: 0.7rem;
  color: #475569;
  font-size: 0.95rem;
  line-height: 1.75;
}

.feature-panel,
.scenario-card,
.pricing-card,
.step-panel,
.faq-item {
  padding: 1rem;
}

.scenario-grid {
  display: grid;
  gap: 1rem;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 15rem), 1fr));
}

.contact-card {
  display: grid;
  align-items: center;
  gap: 1.5rem;
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.5rem;
  background: rgba(255, 255, 255, 0.82);
  padding: 1.25rem;
  box-shadow: 0 18px 38px rgba(0, 32, 128, 0.08);
}

.contact-qr-wrap {
  justify-self: start;
  width: min(100%, 18rem);
  border: 1px solid rgba(203, 213, 225, 0.74);
  border-radius: 0.5rem;
  background: white;
  padding: 0.85rem;
  box-shadow: 0 14px 30px rgba(15, 23, 42, 0.05);
}

.contact-qr {
  display: block;
  width: 100%;
  height: auto;
  border-radius: 0.35rem;
}

.contact-qr-wrap p {
  margin-top: 0.75rem;
  color: #005db8;
  font-size: 0.9rem;
  font-weight: 900;
  line-height: 1.55;
  text-align: center;
}

@media (min-width: 768px) {
  .contact-card {
    grid-template-columns: minmax(0, 1fr) auto;
    padding: 1.5rem;
  }

  .contact-qr-wrap {
    justify-self: end;
  }
}

.pricing-group {
  margin-top: 2rem;
}

.pricing-group-heading {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-bottom: 1rem;
}

.pricing-group-heading h3 {
  color: #0f172a;
  font-size: 1.15rem;
  font-weight: 900;
}

.pricing-group-heading p {
  color: #64748b;
  font-size: 0.9rem;
  line-height: 1.6;
}

.pricing-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 16rem), 1fr));
  gap: 1rem;
}

.pricing-card {
  display: flex;
  flex-direction: column;
}

.pricing-card-highlight {
  border-color: rgba(0, 160, 255, 0.42);
  box-shadow: 0 18px 38px rgba(0, 32, 128, 0.1);
}

.plan-badge {
  flex: 0 0 auto;
  border-radius: 999px;
  background: #e8f6ff;
  padding: 0.28rem 0.6rem;
  color: #005db8;
  font-size: 0.7rem;
  font-weight: 900;
}

.plan-metrics {
  display: grid;
  gap: 0.55rem;
  margin: 1.2rem 0 1.35rem;
}

.plan-metric-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  font-size: 0.875rem;
  line-height: 1.35;
}

.plan-metric-row dt,
.plan-metric-row dd {
  min-width: 0;
  overflow-wrap: anywhere;
}

.pricing-buy-action {
  margin-top: auto;
}

.step-number {
  display: inline-flex;
  width: 2rem;
  height: 2rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: #001040;
  color: white;
  font-size: 0.875rem;
  font-weight: 900;
}

.step-panel h3 {
  margin-top: 0.9rem;
  color: #0f172a;
  font-size: 1rem;
  font-weight: 900;
}

.step-panel p {
  margin-top: 0.5rem;
  color: #475569;
  font-size: 0.875rem;
  line-height: 1.7;
}

.faq-item summary {
  cursor: pointer;
  color: #0f172a;
  font-size: 0.95rem;
  font-weight: 900;
}

.faq-item p {
  margin-top: 0.75rem;
  color: #475569;
  font-size: 0.9rem;
  line-height: 1.7;
}

@media (max-width: 640px) {
  .primary-action,
  .secondary-action {
    width: 100%;
  }

  .section-heading h2 {
    font-size: 1.45rem;
  }
}
</style>
