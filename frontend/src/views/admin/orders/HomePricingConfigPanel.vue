<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-bold text-gray-900 dark:text-white">主页定价展示</h2>
        <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">控制 public 主页 /home 的定价区域。订阅卡绑定真实套餐，额度卡按固定充值金额展示。</p>
      </div>
      <div class="flex gap-2">
        <button class="btn btn-secondary" :disabled="loading" @click="loadConfig">刷新</button>
        <button class="btn btn-primary" :disabled="saving || loading" @click="saveConfig">{{ saving ? '保存中...' : '保存配置' }}</button>
      </div>
    </div>

    <div v-if="loading" class="card py-12 text-center text-sm text-gray-500 dark:text-gray-400">正在加载主页展示配置...</div>

    <template v-else>
      <div class="card space-y-4 p-5">
        <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_12rem]">
          <div>
            <label class="input-label">统一外部购买链接</label>
            <input v-model="config.external_purchase_url" class="input" placeholder="https://pay.ldxp.cn/shop/E8WHWMVD" />
          </div>
          <div>
            <label class="input-label">CTA 模式</label>
            <select v-model="config.cta_mode" class="input">
              <option value="external">外部链接</option>
              <option value="internal">站内支付预选</option>
            </select>
          </div>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <LocalizedInput v-model="config.eyebrow" label="眉标" />
          <LocalizedInput v-model="config.title" label="标题" />
        </div>
        <LocalizedInput v-model="config.description" label="说明" multiline />
      </div>

      <section class="card space-y-4 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-bold text-gray-900 dark:text-white">订阅套餐分组</h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">每张卡必须绑定一个后台真实订阅套餐，价格会使用真实套餐价格。</p>
          </div>
          <button class="btn btn-secondary" @click="addSubscriptionCard">添加订阅卡</button>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <LocalizedInput v-model="config.subscription_group.title" label="分组标题" />
          <LocalizedInput v-model="config.subscription_group.description" label="分组说明" />
        </div>

        <div class="space-y-3">
          <article v-for="(card, index) in config.subscription_cards" :key="card.id" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
            <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <input v-model="card.enabled" type="checkbox" class="rounded border-gray-300" />
                  启用
                </label>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <input v-model="card.highlight" type="checkbox" class="rounded border-gray-300" />
                  推荐高亮
                </label>
              </div>
              <button class="btn btn-danger btn-sm" @click="removeSubscriptionCard(index)">删除</button>
            </div>

            <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_8rem]">
              <div>
                <label class="input-label">绑定真实套餐</label>
                <select v-model.number="card.subscription_plan_id" class="input">
                  <option :value="0">请选择套餐</option>
                  <option v-for="plan in plans" :key="plan.id" :value="plan.id">
                    {{ plan.name }} - ¥{{ plan.price }}{{ plan.for_sale ? '' : '（已下架）' }}
                  </option>
                </select>
              </div>
              <div>
                <label class="input-label">排序</label>
                <input v-model.number="card.sort_order" type="number" class="input" />
              </div>
            </div>

            <div class="mt-4 grid gap-4 md:grid-cols-2">
              <LocalizedInput v-model="card.name" label="卡片名称" />
              <LocalizedInput v-model="card.badge" label="角标" optional />
              <LocalizedInput v-model="card.period" label="价格周期" optional />
              <LocalizedInput v-model="card.description" label="描述" multiline />
            </div>
            <MetricsEditor v-model="card.metrics" class="mt-4" />
          </article>
        </div>
      </section>

      <section class="card space-y-4 p-5">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-bold text-gray-900 dark:text-white">额度套餐分组</h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">额度卡使用固定人民币充值金额，后续切换站内支付时会预填充值金额。</p>
          </div>
          <button class="btn btn-secondary" @click="addCreditCard">添加额度卡</button>
        </div>

        <div class="grid gap-4 md:grid-cols-2">
          <LocalizedInput v-model="config.credit_group.title" label="分组标题" />
          <LocalizedInput v-model="config.credit_group.description" label="分组说明" />
        </div>

        <div class="space-y-3">
          <article v-for="(card, index) in config.credit_cards" :key="card.id" class="rounded-xl border border-gray-200 p-4 dark:border-dark-600">
            <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
              <div class="flex items-center gap-3">
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <input v-model="card.enabled" type="checkbox" class="rounded border-gray-300" />
                  启用
                </label>
                <label class="flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
                  <input v-model="card.highlight" type="checkbox" class="rounded border-gray-300" />
                  推荐高亮
                </label>
              </div>
              <button class="btn btn-danger btn-sm" @click="removeCreditCard(index)">删除</button>
            </div>

            <div class="grid gap-4 md:grid-cols-3">
              <div>
                <label class="input-label">充值金额 CNY</label>
                <input v-model.number="card.recharge_amount" type="number" min="0.01" step="0.01" class="input" />
              </div>
              <div>
                <label class="input-label">排序</label>
                <input v-model.number="card.sort_order" type="number" class="input" />
              </div>
              <LocalizedInput v-model="card.badge" label="角标" optional />
            </div>

            <div class="mt-4 grid gap-4 md:grid-cols-2">
              <LocalizedInput v-model="card.name" label="卡片名称" />
              <LocalizedInput v-model="card.period" label="价格周期" optional />
              <LocalizedInput v-model="card.description" label="描述" multiline />
            </div>
            <MetricsEditor v-model="card.metrics" class="mt-4" />
          </article>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, defineComponent, h, onMounted, ref } from 'vue'
import { useAppStore } from '@/stores/app'
import { adminPaymentAPI } from '@/api/admin/payment'
import type {
  HomePricingConfig,
  HomePricingLocalizedText,
  HomePricingMetricConfig,
  HomePricingSubscriptionCardConfig,
  HomePricingCreditCardConfig,
} from '@/types'
import type { SubscriptionPlan } from '@/types/payment'

const props = defineProps<{
  plans: SubscriptionPlan[]
}>()

const appStore = useAppStore()
const loading = ref(false)
const saving = ref(false)
const config = ref<HomePricingConfig>(createDefaultConfig())
const plans = computed(() => props.plans || [])

function localized(zh = '', en = ''): HomePricingLocalizedText {
  return { zh, en }
}

function createDefaultConfig(): HomePricingConfig {
  return {
    external_purchase_url: 'https://pay.ldxp.cn/shop/E8WHWMVD',
    cta_mode: 'external',
    eyebrow: localized('定价', 'Pricing'),
    title: localized('先选择套餐，注册登录后开始使用。', 'Choose a plan, then sign in to start.'),
    description: localized('token 数量为营销估算，实际消耗会随模型、输入输出长度和工具行为变化。', 'Token counts are marketing estimates. Actual usage varies by model, input/output length, and tool behavior.'),
    subscription_group: {
      title: localized('订阅套餐', 'Subscription Plans'),
      description: localized('适合每天持续使用 AI 工具的用户。', 'For users who run AI tools continuously.'),
    },
    credit_group: {
      title: localized('额度套餐', 'Credit Packages'),
      description: localized('适合月卡不够用时灵活补充，额度无每日限制。', 'Flexible top-ups when your monthly plan is not enough. Credits have no daily limit.'),
    },
    subscription_cards: [],
    credit_cards: [
      createCreditCard('$80 额度', '$80 Credit', 20, '$80', 10),
      createCreditCard('$180 额度', '$180 Credit', 40, '$180', 20, true, localized('常用', 'Common')),
      createCreditCard('$1000 额度', '$1000 Credit', 200, '$1000', 30),
    ],
  }
}

function createMetric(labelZh: string, labelEn: string, valueZh: string, valueEn: string): HomePricingMetricConfig {
  return { label: localized(labelZh, labelEn), value: localized(valueZh, valueEn) }
}

function createSubscriptionCard(plan?: SubscriptionPlan): HomePricingSubscriptionCardConfig {
  const id = `subscription-${Date.now()}-${Math.random().toString(16).slice(2)}`
  return {
    id,
    enabled: true,
    sort_order: (config.value.subscription_cards.length + 1) * 10,
    subscription_plan_id: plan?.id || 0,
    name: localized(plan?.name || '', plan?.name || ''),
    description: localized(plan?.description || '', plan?.description || ''),
    badge: localized('', ''),
    period: localized('/ 月', '/ month'),
    highlight: false,
    metrics: [
      createMetric('每日额度', 'Daily credit', plan?.daily_limit_usd != null ? `约 $${plan.daily_limit_usd}` : '约 $20', plan?.daily_limit_usd != null ? `about $${plan.daily_limit_usd}` : 'about $20'),
      createMetric('token 估算', 'Token estimate', '按实际模型消耗', 'varies by model'),
    ],
  }
}

function createCreditCard(nameZh: string, nameEn: string, amount: number, credit: string, sortOrder: number, highlight = false, badge = localized('', '')): HomePricingCreditCardConfig {
  return {
    id: `credit-${Date.now()}-${Math.random().toString(16).slice(2)}`,
    enabled: true,
    sort_order: sortOrder,
    recharge_amount: amount,
    name: localized(nameZh, nameEn),
    description: localized('不够用时补充。', 'Top up when your plan is not enough.'),
    badge,
    period: localized('', ''),
    highlight,
    metrics: [
      createMetric('额度', 'Credit', credit, credit),
      createMetric('类型', 'Type', '余额额度', 'balance credit'),
    ],
  }
}

async function loadConfig() {
  loading.value = true
  try {
    const res = await adminPaymentAPI.getHomePricingConfig()
    config.value = normalizeConfig(res.data)
  } catch (error: unknown) {
    appStore.showError(error instanceof Error ? error.message : '加载主页定价配置失败')
  } finally {
    loading.value = false
  }
}

function normalizeConfig(input: HomePricingConfig | null | undefined): HomePricingConfig {
  const fallback = createDefaultConfig()
  if (!input) return fallback
  return {
    ...fallback,
    ...input,
    external_purchase_url: input.external_purchase_url || fallback.external_purchase_url,
    cta_mode: input.cta_mode || 'external',
    eyebrow: input.eyebrow || fallback.eyebrow,
    title: input.title || fallback.title,
    description: input.description || fallback.description,
    subscription_group: input.subscription_group || fallback.subscription_group,
    credit_group: input.credit_group || fallback.credit_group,
    subscription_cards: input.subscription_cards || [],
    credit_cards: input.credit_cards || fallback.credit_cards,
  }
}

function validateConfig() {
  if (config.value.cta_mode === 'external' && !/^https?:\/\/\S+/i.test(config.value.external_purchase_url.trim())) {
    return '外部购买链接必须是 http(s) URL'
  }
  for (const card of config.value.subscription_cards) {
    if (!card.subscription_plan_id) return '订阅卡必须绑定真实套餐'
    if (!required(card.name) || !required(card.description)) return '订阅卡名称和描述必须填写中英文'
  }
  for (const card of config.value.credit_cards) {
    if (!card.recharge_amount || card.recharge_amount <= 0) return '额度卡充值金额必须大于 0'
    if (!required(card.name) || !required(card.description)) return '额度卡名称和描述必须填写中英文'
  }
  return ''
}

function required(value: HomePricingLocalizedText) {
  return !!value?.zh?.trim() && !!value?.en?.trim()
}

async function saveConfig() {
  const validationError = validateConfig()
  if (validationError) {
    appStore.showError(validationError)
    return
  }
  saving.value = true
  try {
    const res = await adminPaymentAPI.updateHomePricingConfig(config.value)
    config.value = normalizeConfig(res.data)
    appStore.showSuccess('主页定价配置已保存')
  } catch (error: unknown) {
    appStore.showError(error instanceof Error ? error.message : '保存主页定价配置失败')
  } finally {
    saving.value = false
  }
}

function addSubscriptionCard() {
  config.value.subscription_cards.push(createSubscriptionCard(plans.value[0]))
}

function removeSubscriptionCard(index: number) {
  config.value.subscription_cards.splice(index, 1)
}

function addCreditCard() {
  config.value.credit_cards.push(createCreditCard('$80 额度', '$80 Credit', 20, '$80', (config.value.credit_cards.length + 1) * 10))
}

function removeCreditCard(index: number) {
  config.value.credit_cards.splice(index, 1)
}

const LocalizedInput = defineComponent({
  props: {
    modelValue: { type: Object as () => HomePricingLocalizedText, required: true },
    label: { type: String, required: true },
    multiline: { type: Boolean, default: false },
    optional: { type: Boolean, default: false },
  },
  emits: ['update:modelValue'],
  setup(componentProps, { emit }) {
    const update = (key: 'zh' | 'en', value: string) => {
      emit('update:modelValue', { ...componentProps.modelValue, [key]: value })
    }
    return () => h('div', [
      h('label', { class: 'input-label' }, `${componentProps.label}${componentProps.optional ? '' : ' *'}`),
      h('div', { class: 'grid gap-2 sm:grid-cols-2' }, ['zh', 'en'].map((key) => h(componentProps.multiline ? 'textarea' : 'input', {
        class: 'input',
        rows: componentProps.multiline ? 2 : undefined,
        value: componentProps.modelValue[key as 'zh' | 'en'] || '',
        placeholder: key === 'zh' ? '中文' : 'English',
        onInput: (event: Event) => update(key as 'zh' | 'en', (event.target as HTMLInputElement).value),
      }))),
    ])
  },
})

const MetricsEditor = defineComponent({
  props: {
    modelValue: { type: Array as () => HomePricingMetricConfig[], required: true },
  },
  emits: ['update:modelValue'],
  setup(componentProps, { emit, attrs }) {
    const updateMetric = (index: number, key: 'label' | 'value', lang: 'zh' | 'en', value: string) => {
      const next = componentProps.modelValue.map(metric => ({
        label: { ...metric.label },
        value: { ...metric.value },
      }))
      next[index][key][lang] = value
      emit('update:modelValue', next)
    }
    const addMetric = () => {
      emit('update:modelValue', [...componentProps.modelValue, createMetric('', '', '', '')])
    }
    const removeMetric = (index: number) => {
      emit('update:modelValue', componentProps.modelValue.filter((_, i) => i !== index))
    }
    return () => h('div', { class: ['space-y-2', attrs.class] }, [
      h('div', { class: 'flex items-center justify-between gap-2' }, [
        h('p', { class: 'text-sm font-semibold text-gray-900 dark:text-white' }, '指标文案'),
        h('button', { class: 'btn btn-secondary btn-sm', type: 'button', onClick: addMetric }, '添加指标'),
      ]),
      ...componentProps.modelValue.map((metric, index) => h('div', { class: 'grid gap-2 rounded-lg bg-gray-50 p-3 dark:bg-dark-800 md:grid-cols-[1fr_1fr_auto]' }, [
        h('div', { class: 'grid gap-2 sm:grid-cols-2' }, [
          h('input', { class: 'input', value: metric.label.zh, placeholder: '标签中文', onInput: (event: Event) => updateMetric(index, 'label', 'zh', (event.target as HTMLInputElement).value) }),
          h('input', { class: 'input', value: metric.label.en, placeholder: 'Label EN', onInput: (event: Event) => updateMetric(index, 'label', 'en', (event.target as HTMLInputElement).value) }),
        ]),
        h('div', { class: 'grid gap-2 sm:grid-cols-2' }, [
          h('input', { class: 'input', value: metric.value.zh, placeholder: '值中文', onInput: (event: Event) => updateMetric(index, 'value', 'zh', (event.target as HTMLInputElement).value) }),
          h('input', { class: 'input', value: metric.value.en, placeholder: 'Value EN', onInput: (event: Event) => updateMetric(index, 'value', 'en', (event.target as HTMLInputElement).value) }),
        ]),
        h('button', { class: 'btn btn-danger btn-sm', type: 'button', onClick: () => removeMetric(index) }, '删除'),
      ])),
    ])
  },
})

onMounted(loadConfig)
</script>
