<template>
  <AppLayout>
    <div class="mx-auto max-w-4xl space-y-6">
      <!-- Page header -->
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('tutorial.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
          {{ t('tutorial.description') }}
        </p>
      </div>

      <div class="card">
        <!-- Tabs -->
        <div class="flex border-b border-gray-200 dark:border-dark-700">
          <button
            v-for="tab in TABS"
            :key="tab"
            type="button"
            class="-mb-px shrink-0 border-b-2 px-4 py-2.5 text-sm font-medium transition-colors sm:px-5"
            :class="
              activeTab === tab
                ? 'border-primary-500 text-primary-600 dark:text-primary-400'
                : 'border-transparent text-gray-500 hover:text-gray-700 dark:text-dark-400 dark:hover:text-dark-200'
            "
            @click="activeTab = tab"
          >
            {{ t(`tutorial.tabs.${tab}`) }}
          </button>
        </div>

        <!-- Guide tab -->
        <div v-if="activeTab === 'guide'" class="space-y-8 p-5 sm:p-6">
          <!-- Steps -->
          <div class="grid gap-4 md:grid-cols-3">
            <div
              v-for="(step, index) in guideSteps"
              :key="index"
              class="rounded-xl border border-gray-100 bg-white p-5 dark:border-dark-700 dark:bg-dark-800/50"
            >
              <div
                class="mb-3 flex h-8 w-8 items-center justify-center rounded-full bg-primary-600 text-sm font-bold text-white"
              >
                {{ index + 1 }}
              </div>
              <h3 class="mb-1 text-sm font-semibold text-gray-900 dark:text-white">
                {{ step.title }}
              </h3>
              <p class="text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                {{ step.desc }}
              </p>
            </div>
          </div>

          <!-- Code samples -->
          <div class="overflow-hidden rounded-xl border border-gray-200 bg-gray-900 shadow-sm dark:border-dark-700">
            <div class="flex items-center justify-between border-b border-gray-700/70 px-4 py-2.5">
              <div class="ml-1 flex gap-1">
                <button
                  v-for="tab in CODE_TABS"
                  :key="tab"
                  type="button"
                  class="rounded px-2.5 py-1 text-xs transition-colors"
                  :class="
                    codeTab === tab
                      ? 'bg-gray-700 text-white'
                      : 'text-gray-400 hover:text-gray-200'
                  "
                  @click="codeTab = tab"
                >
                  {{ codeTabLabel(tab) }}
                </button>
              </div>
              <button
                type="button"
                class="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-gray-400 transition-colors hover:text-gray-200"
                @click="copyText(quickstartCode, 'quickstart')"
              >
                <Icon :name="copiedKey === 'quickstart' ? 'check' : 'copy'" size="sm" />
                {{ copiedKey === 'quickstart' ? t('tutorial.guide.copied') : t('tutorial.guide.copy') }}
              </button>
            </div>
            <pre class="overflow-x-auto p-5 font-mono text-xs leading-relaxed text-gray-300 sm:text-sm">{{ quickstartCode }}</pre>
          </div>

          <p class="text-xs leading-relaxed text-gray-400 dark:text-dark-500">
            {{ t('tutorial.guide.compatibilityNote') }}
          </p>

          <!-- Client setups -->
          <div>
            <h2 class="mb-1 text-base font-semibold text-gray-900 dark:text-white">
              {{ t('tutorial.guide.clientsTitle') }}
            </h2>
            <p class="mb-4 text-xs text-gray-400 dark:text-dark-500">
              {{ clientDescNote }}
            </p>
            <div class="grid gap-4 md:grid-cols-3">
              <div
                v-for="client in clients"
                :key="client.key"
                class="flex flex-col overflow-hidden rounded-xl border border-gray-100 bg-white dark:border-dark-700 dark:bg-dark-800/50"
              >
                <div class="flex-1 p-4">
                  <h3 class="mb-1 text-sm font-semibold text-gray-900 dark:text-white">
                    {{ client.name }}
                  </h3>
                  <p class="mb-3 text-xs leading-relaxed text-gray-500 dark:text-dark-400">
                    {{ client.desc }}
                  </p>
                  <pre class="overflow-x-auto rounded-lg bg-gray-900 p-3 font-mono text-[11px] leading-relaxed text-gray-300">{{ clientCode(client) }}</pre>
                </div>
                <button
                  type="button"
                  class="flex items-center justify-center gap-1.5 border-t border-gray-100 px-4 py-2 text-xs text-gray-500 transition-colors hover:bg-gray-50 hover:text-gray-700 dark:border-dark-700 dark:text-dark-400 dark:hover:bg-dark-700/50 dark:hover:text-dark-200"
                  @click="copyText(clientCode(client), `client-${client.key}`)"
                >
                  <Icon :name="copiedKey === `client-${client.key}` ? 'check' : 'copy'" size="sm" />
                  {{ copiedKey === `client-${client.key}` ? t('tutorial.guide.copied') : t('tutorial.guide.copy') }}
                </button>
              </div>
            </div>
          </div>

          <!-- cc-switch tip -->
          <div class="rounded-lg border border-primary-100 bg-primary-50 px-4 py-3 text-xs leading-relaxed text-primary-700 dark:border-primary-900/40 dark:bg-primary-900/20 dark:text-primary-300">
            {{ t('tutorial.guide.ccsTip') }}
          </div>
        </div>

        <!-- FAQ tab -->
        <div v-else class="p-5 sm:p-6">
          <!-- Search + category filter -->
          <div class="mb-5 space-y-3">
            <div class="relative max-w-md">
              <div class="pointer-events-none absolute inset-y-0 left-0 flex items-center pl-3.5">
                <Icon name="search" size="md" class="text-gray-400 dark:text-dark-500" />
              </div>
              <input
                v-model="faqQuery"
                type="text"
                :placeholder="t('tutorial.faq.searchPlaceholder')"
                class="input pl-10"
              />
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <button
                v-for="category in FAQ_CATEGORIES"
                :key="category"
                type="button"
                class="rounded-full px-3 py-1.5 text-xs font-medium transition-colors"
                :class="
                  activeCategory === category
                    ? 'bg-primary-600 text-white'
                    : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-300 dark:hover:bg-dark-600'
                "
                @click="activeCategory = category"
              >
                {{ t(`tutorial.faq.categories.${category}`) }}
              </button>
            </div>
          </div>

          <!-- Accordion list -->
          <div v-if="filteredFaqItems.length" class="space-y-3">
            <p class="text-xs text-gray-400 dark:text-dark-500">
              {{ t('tutorial.faq.resultCount', { count: filteredFaqItems.length }) }}
            </p>
            <div
              v-for="item in filteredFaqItems"
              :key="item.id"
              class="overflow-hidden rounded-xl border border-gray-100 bg-white dark:border-dark-700 dark:bg-dark-800/50"
            >
              <button
                type="button"
                class="flex w-full items-start justify-between gap-3 px-5 py-4 text-left"
                @click="toggleFaq(item.id)"
              >
                <span class="min-w-0">
                  <span class="block text-sm font-semibold text-gray-900 dark:text-white">
                    {{ faqText(item.question) }}
                  </span>
                  <span class="mt-1 block text-xs leading-relaxed text-gray-400 dark:text-dark-500">
                    {{ faqText(item.summary) }}
                  </span>
                </span>
                <svg
                  class="mt-0.5 h-4 w-4 flex-shrink-0 text-gray-400 transition-transform duration-200"
                  :class="{ 'rotate-180': openFaqId === item.id }"
                  fill="none"
                  viewBox="0 0 24 24"
                  stroke="currentColor"
                  stroke-width="2"
                >
                  <path stroke-linecap="round" stroke-linejoin="round" d="m19.5 8.25-7.5 7.5-7.5-7.5" />
                </svg>
              </button>
              <div v-if="openFaqId === item.id" class="border-t border-gray-50 px-5 py-4 dark:border-dark-700/60">
                <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                  {{ faqText(item.answer) }}
                </p>
                <ul v-if="item.points && item.points.length" class="mt-3 space-y-1.5 pl-1">
                  <li
                    v-for="(point, index) in item.points"
                    :key="index"
                    class="flex items-start gap-2 text-sm leading-relaxed text-gray-600 dark:text-dark-300"
                  >
                    <span class="mt-1.5 h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary-400"></span>
                    <span>{{ faqText(point) }}</span>
                  </li>
                </ul>
                <div
                  v-if="item.note"
                  class="mt-4 rounded-lg border-l-4 border-primary-500 bg-primary-50 px-3.5 py-2.5 text-sm leading-relaxed text-primary-800 dark:bg-primary-900/25 dark:text-primary-200"
                >
                  <span class="mr-1.5 text-xs font-bold">TL;DR</span>{{ faqText(item.note) }}
                </div>
              </div>
            </div>
          </div>

          <!-- Empty state -->
          <div v-else class="flex flex-col items-center justify-center py-14 text-center">
            <div class="mb-3 flex h-12 w-12 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700">
              <Icon name="questionCircle" size="lg" class="text-gray-400" />
            </div>
            <p class="text-sm text-gray-500 dark:text-dark-400">
              {{ t('tutorial.faq.noResult') }}
            </p>
          </div>
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'

interface StepItem {
  title: string
  desc: string
}

interface ClientItem {
  key: string
  name: string
  desc: string
  code: string
}

interface FaqItem {
  id: string
  category: string
  question: string
  summary: string
  answer: string
  points: string[]
  note?: string
}

const TABS = ['guide', 'faq'] as const
type Tab = (typeof TABS)[number]

const CODE_TABS = ['curl', 'python', 'sdk'] as const
type CodeTab = (typeof CODE_TABS)[number]

const FAQ_CATEGORIES = ['all', 'billing', 'plans', 'quality', 'support'] as const
type FaqCategory = (typeof FAQ_CATEGORIES)[number]

const { t, tm, locale } = useI18n()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()

// ---------- Tab state (synced with ?tab= query, e.g. /tutorial?tab=faq) ----------
const activeTab = ref<Tab>(route.query.tab === 'faq' ? 'faq' : 'guide')

watch(
  () => route.query.tab,
  (value) => {
    activeTab.value = value === 'faq' ? 'faq' : 'guide'
  },
)

watch(activeTab, (value) => {
  if ((route.query.tab ?? null) === (value === 'faq' ? 'faq' : null)) return
  router.replace({ query: value === 'faq' ? { tab: 'faq' } : {} })
})

// ---------- Guide ----------
const apiBase = computed(() => appStore.apiBaseUrl || window.location.origin)

const guideSteps = computed<StepItem[]>(() => {
  const raw = tm('tutorial.guide.steps') as unknown
  if (Array.isArray(raw)) return raw as StepItem[]
  return raw && typeof raw === 'object' ? (Object.values(raw) as StepItem[]) : []
})

const clients = computed<ClientItem[]>(() => {
  const raw = tm('tutorial.guide.clients') as unknown
  if (Array.isArray(raw)) return raw as ClientItem[]
  return raw && typeof raw === 'object' ? (Object.values(raw) as ClientItem[]) : []
})

const clientDescNote = computed(() =>
  t('tutorial.guide.clientsDesc').replace(/\$BASE/g, apiBase.value),
)

function clientCode(client: ClientItem): string {
  return client.code.replace(/\$BASE/g, apiBase.value)
}

// FAQ 文案中的 $BASE 占位符同样替换为真实接入地址
function faqText(text: string): string {
  return text.replace(/\$BASE/g, apiBase.value)
}

const codeTab = ref<CodeTab>('curl')

function codeTabLabel(tab: CodeTab) {
  return t(`tutorial.guide.tab${tab.charAt(0).toUpperCase()}${tab.slice(1)}`)
}

const quickstartCodes = computed<Record<CodeTab, string>>(() => ({
  curl: `curl ${apiBase.value}/v1/chat/completions \\\n  -H "Authorization: Bearer sk-***" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "deepseek-v4-pro",\n    "messages": [{ "role": "user", "content": "你好" }]\n  }'`,
  python: `import requests\n\nresp = requests.post(\n    "${apiBase.value}/v1/chat/completions",\n    headers={"Authorization": "Bearer sk-***"},\n    json={\n        "model": "qwen3.8-max",\n        "messages": [{"role": "user", "content": "你好"}],\n    },\n)\nprint(resp.json()["choices"][0]["message"]["content"])`,
  sdk: `from openai import OpenAI\n\nclient = OpenAI(\n    api_key="sk-***",\n    base_url="${apiBase.value}/v1",\n)\n\nresp = client.chat.completions.create(\n    model="glm-5.3",\n    messages=[{"role": "user", "content": "你好"}],\n)\nprint(resp.choices[0].message.content)`,
}))

const quickstartCode = computed(() => quickstartCodes.value[codeTab.value])

// ---------- Copy ----------
const copiedKey = ref<string | null>(null)
let copyTimer: ReturnType<typeof setTimeout> | undefined

async function copyText(text: string, key: string) {
  try {
    await navigator.clipboard.writeText(text)
    copiedKey.value = key
    if (copyTimer) clearTimeout(copyTimer)
    copyTimer = setTimeout(() => {
      copiedKey.value = null
    }, 2000)
  } catch {
    // Clipboard unavailable (e.g. insecure context); silently ignore.
  }
}

// ---------- FAQ ----------
const faqQuery = ref('')
const activeCategory = ref<FaqCategory>('all')
const openFaqId = ref<string | null>(null)

const faqItems = computed<FaqItem[]>(() => {
  const raw = tm('tutorial.faq.items') as unknown
  if (Array.isArray(raw)) return raw as FaqItem[]
  return raw && typeof raw === 'object' ? (Object.values(raw) as FaqItem[]) : []
})

const filteredFaqItems = computed(() => {
  const query = faqQuery.value.trim().toLowerCase()
  return faqItems.value.filter((item) => {
    if (activeCategory.value !== 'all' && item.category !== activeCategory.value) return false
    if (!query) return true
    const haystack = [item.question, item.summary, item.answer, item.note ?? '', ...(item.points ?? [])]
      .join('\n')
      .toLowerCase()
    return haystack.includes(query)
  })
})

function toggleFaq(id: string) {
  openFaqId.value = openFaqId.value === id ? null : id
}

// Locale switch swaps FAQ/guide copy; collapse any expanded item to avoid stale state.
watch(locale, () => {
  openFaqId.value = null
})
</script>
