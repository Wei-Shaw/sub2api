<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <section class="purchase-hero">
        <div>
          <p class="purchase-eyebrow">{{ copy.eyebrow }}</p>
          <h1>{{ copy.title }}</h1>
          <p>{{ copy.description }}</p>
        </div>

        <div class="account-panel">
          <span>{{ copy.account }}</span>
          <strong>{{ user?.username || user?.email || '-' }}</strong>
          <small>{{ copy.balance }}: {{ userBalance }}</small>
        </div>
      </section>

      <section class="notice-panel">
        <div>
          <h2>{{ copy.externalOnlyTitle }}</h2>
          <p>{{ copy.externalOnlyDescription }}</p>
        </div>
        <a
          :href="externalPurchaseUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="primary-link"
        >
          {{ copy.openPurchase }}
        </a>
      </section>

      <section class="pricing-section">
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
                  <p class="text-base font-black text-slate-950 dark:text-white">{{ plan.name }}</p>
                  <p class="mt-1 text-sm leading-6 text-slate-500 dark:text-slate-400">{{ plan.description }}</p>
                </div>
                <span v-if="plan.badge" class="plan-badge">{{ plan.badge }}</span>
              </div>

              <div class="mt-5">
                <span class="text-3xl font-black text-slate-950 dark:text-white">{{ plan.price }}</span>
                <span class="ml-1 text-sm font-semibold text-slate-500 dark:text-slate-400">{{ plan.period }}</span>
              </div>

              <dl class="plan-metrics">
                <div
                  v-for="item in plan.metrics"
                  :key="item.label"
                  class="plan-metric-row"
                >
                  <dt>{{ item.label }}</dt>
                  <dd>{{ item.value }}</dd>
                </div>
              </dl>

              <a
                :href="externalPurchaseUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="buy-link"
              >
                {{ copy.buyAction }}
              </a>
            </article>
          </div>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores'
import AppLayout from '@/components/layout/AppLayout.vue'
import type { HomePricingLocalizedText } from '@/types'

type TextPair = {
  label: string
  value: string
}

type PricePlan = {
  name: string
  description: string
  price: string
  period: string
  badge?: string
  highlight?: boolean
  metrics: TextPair[]
}

type PriceGroup = {
  title: string
  description: string
  plans: PricePlan[]
}

const fallbackPurchaseUrl = 'https://pay.ldxp.cn/shop/E8WHWMVD'

const { locale } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const user = computed(() => authStore.user)
const isZh = computed(() => locale.value.toLowerCase().startsWith('zh'))
const copy = computed(() => isZh.value ? zhCopy : enCopy)
const homePricingConfig = computed(() => appStore.cachedPublicSettings?.home_pricing_config || null)

const userBalance = computed(() => {
  const balance = Number(user.value?.balance || 0)
  return `$${Number.isFinite(balance) ? balance.toFixed(2) : '0.00'}`
})

const externalPurchaseUrl = computed(() => {
  const configured = homePricingConfig.value?.external_purchase_url?.trim()
  return configured || fallbackPurchaseUrl
})

const pricingHeader = computed(() => {
  const cfg = homePricingConfig.value
  if (!cfg) {
    return {
      eyebrow: copy.value.pricingEyebrow,
      title: copy.value.pricingTitle,
      description: copy.value.pricingDescription,
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
  if (!cfg) return fallbackPricingGroups.value

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

  return groups.length ? groups : fallbackPricingGroups.value
})

const fallbackPricingGroups = computed<PriceGroup[]>(() => isZh.value ? zhFallbackPricingGroups : enFallbackPricingGroups)

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

onMounted(() => {
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
  authStore.refreshUser().catch(() => {})
})

const zhCopy = {
  eyebrow: '外部购买',
  title: '选择套餐后，通过外部页面完成购买。',
  description: '当前站内不再创建充值订单。订阅套餐和额度套餐统一展示在这里，点击购买后会打开外部交易页面。',
  account: '当前账号',
  balance: '当前余额',
  externalOnlyTitle: '当前仅支持外部充值',
  externalOnlyDescription: '支付和开通以外部购买页面及人工处理为准。站内保留套餐展示、账号查看和后续使用入口。',
  openPurchase: '打开购买页面',
  buyAction: '前往购买',
  pricingEyebrow: '定价',
  pricingTitle: '先选择套餐，注册登录后开始使用。',
  pricingDescription: 'token 数量为营销估算，实际消耗会随模型、输入输出长度和工具行为变化。',
}

const enCopy = {
  eyebrow: 'External Purchase',
  title: 'Choose a plan and complete purchase externally.',
  description: 'This page no longer creates internal recharge orders. Subscription and credit plans are shown here, and purchase buttons open the external checkout page.',
  account: 'Current account',
  balance: 'Current balance',
  externalOnlyTitle: 'External recharge only',
  externalOnlyDescription: 'Payment and activation are handled through the external purchase page. The app keeps plan display, account status, and usage entry points.',
  openPurchase: 'Open purchase page',
  buyAction: 'Buy now',
  pricingEyebrow: 'Pricing',
  pricingTitle: 'Choose a plan, then sign in to start.',
  pricingDescription: 'Token counts are marketing estimates. Actual usage varies by model, input/output length, and tool behavior.',
}

const zhFallbackPricingGroups: PriceGroup[] = [
  {
    title: '订阅套餐',
    description: '适合每天持续使用 AI 工具的用户。',
    plans: [
      {
        name: '日卡',
        description: '临时体验、当天高频使用。',
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
]

const enFallbackPricingGroups: PriceGroup[] = [
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
]
</script>

<style scoped>
.purchase-hero {
  display: grid;
  gap: 1rem;
  align-items: stretch;
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.75rem;
  background:
    linear-gradient(135deg, rgba(232, 246, 255, 0.92), rgba(255, 255, 255, 0.95));
  padding: 1.25rem;
  box-shadow: 0 18px 42px rgba(15, 23, 42, 0.06);
}

@media (min-width: 768px) {
  .purchase-hero {
    grid-template-columns: minmax(0, 1fr) 18rem;
  }
}

.purchase-eyebrow,
.section-heading p {
  color: #006fd6;
  font-size: 0.78rem;
  font-weight: 900;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}

.purchase-hero h1 {
  margin-top: 0.5rem;
  color: #0f172a;
  font-size: clamp(1.75rem, 4vw, 3rem);
  font-weight: 950;
  line-height: 1.08;
}

.purchase-hero p:not(.purchase-eyebrow) {
  margin-top: 0.85rem;
  max-width: 46rem;
  color: #475569;
  font-size: 1rem;
  line-height: 1.8;
}

.account-panel {
  display: flex;
  min-width: 0;
  flex-direction: column;
  justify-content: center;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 0.65rem;
  background: rgba(255, 255, 255, 0.74);
  padding: 1rem;
}

.account-panel span,
.account-panel small {
  color: #64748b;
  font-size: 0.8rem;
  font-weight: 800;
}

.account-panel strong {
  margin-top: 0.4rem;
  overflow-wrap: anywhere;
  color: #0f172a;
  font-size: 1.05rem;
}

.account-panel small {
  margin-top: 0.45rem;
}

.notice-panel {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  border: 1px solid rgba(0, 111, 214, 0.18);
  border-radius: 0.75rem;
  background: rgba(255, 255, 255, 0.9);
  padding: 1rem;
}

@media (min-width: 768px) {
  .notice-panel {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
  }
}

.notice-panel h2 {
  color: #0f172a;
  font-size: 1rem;
  font-weight: 900;
}

.notice-panel p {
  margin-top: 0.35rem;
  color: #64748b;
  font-size: 0.9rem;
  line-height: 1.7;
}

.primary-link,
.buy-link {
  display: inline-flex;
  min-height: 2.75rem;
  align-items: center;
  justify-content: center;
  border-radius: 0.5rem;
  background: #001040;
  color: white;
  font-size: 0.9rem;
  font-weight: 900;
  transition:
    background-color 0.16s ease,
    transform 0.16s ease;
}

.primary-link {
  flex: 0 0 auto;
  padding: 0.65rem 1rem;
}

.buy-link {
  width: 100%;
  margin-top: auto;
  padding: 0.65rem 0.9rem;
}

.primary-link:hover,
.buy-link:hover {
  background: #002080;
  transform: translateY(-1px);
}

.pricing-section {
  border: 1px solid rgba(203, 213, 225, 0.72);
  border-radius: 0.75rem;
  background: rgba(255, 255, 255, 0.78);
  padding: 1.25rem;
}

.section-heading h2 {
  margin-top: 0.35rem;
  color: #0f172a;
  font-size: clamp(1.35rem, 3vw, 2rem);
  font-weight: 950;
  line-height: 1.16;
}

.section-heading span {
  display: block;
  margin-top: 0.55rem;
  color: #64748b;
  font-size: 0.95rem;
  line-height: 1.7;
}

.pricing-group {
  margin-top: 1.5rem;
}

.pricing-group-heading h3 {
  color: #0f172a;
  font-size: 1.15rem;
  font-weight: 950;
}

.pricing-group-heading p {
  margin-top: 0.35rem;
  color: #64748b;
  font-size: 0.92rem;
  line-height: 1.65;
}

.pricing-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 15.5rem), 1fr));
  gap: 1rem;
  margin-top: 1rem;
}

.pricing-card {
  display: flex;
  min-height: 18rem;
  flex-direction: column;
  border: 1px solid rgba(203, 213, 225, 0.82);
  border-radius: 0.65rem;
  background: rgba(255, 255, 255, 0.9);
  padding: 1rem;
  box-shadow: 0 14px 34px rgba(15, 23, 42, 0.05);
}

.pricing-card-highlight {
  border-color: rgba(0, 160, 255, 0.45);
  box-shadow: 0 18px 42px rgba(0, 160, 255, 0.1);
}

.plan-badge {
  flex: 0 0 auto;
  border-radius: 999px;
  background: #e8f6ff;
  padding: 0.35rem 0.65rem;
  color: #005db8;
  font-size: 0.76rem;
  font-weight: 900;
}

.plan-metrics {
  display: grid;
  gap: 0.55rem;
  margin: 1.15rem 0 1rem;
}

.plan-metric-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 0.75rem;
  align-items: baseline;
}

.plan-metric-row dt {
  min-width: 0;
  color: #64748b;
  font-size: 0.88rem;
  font-weight: 700;
}

.plan-metric-row dd {
  color: #0f172a;
  font-size: 0.92rem;
  font-weight: 900;
  text-align: right;
  white-space: nowrap;
}

:global(.dark) .purchase-hero,
:global(.dark) .notice-panel,
:global(.dark) .pricing-section,
:global(.dark) .pricing-card,
:global(.dark) .account-panel {
  border-color: rgba(51, 65, 85, 0.9);
  background: rgba(15, 23, 42, 0.84);
}

:global(.dark) .purchase-hero h1,
:global(.dark) .notice-panel h2,
:global(.dark) .section-heading h2,
:global(.dark) .pricing-group-heading h3,
:global(.dark) .account-panel strong,
:global(.dark) .plan-metric-row dd {
  color: #f8fafc;
}

:global(.dark) .purchase-hero p:not(.purchase-eyebrow),
:global(.dark) .notice-panel p,
:global(.dark) .section-heading span,
:global(.dark) .pricing-group-heading p,
:global(.dark) .plan-metric-row dt,
:global(.dark) .account-panel span,
:global(.dark) .account-panel small {
  color: #94a3b8;
}
</style>
