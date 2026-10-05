<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6">
      <!-- Page header -->
      <div>
        <h1 class="text-2xl font-bold text-gray-900 dark:text-white">
          {{ t('tutorial.title') }}
        </h1>
        <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
          {{ t('tutorial.description') }}
        </p>
      </div>

      <!-- Main card: guide / faq tabs -->
      <div class="card">
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
          <div v-if="activeTab === 'guide'" class="space-y-6 p-5 sm:p-6">
            <!-- Model selector -->
            <div class="flex flex-wrap items-center gap-3">
              <span class="text-sm text-gray-500 dark:text-dark-400">{{ t('tutorial.guide.model') }}</span>
              <select v-model="selectedModel" class="input w-56">
                <option v-for="m in TUTORIAL_MODELS" :key="m" :value="m">{{ m }}</option>
              </select>
            </div>

            <!-- Client chips -->
            <div class="flex flex-wrap items-center gap-2">
              <span class="mr-1 text-sm text-gray-500 dark:text-dark-400">{{ t('tutorial.guide.client') }}</span>
              <button
                v-for="guide in guides"
                :key="guide.key"
                type="button"
                class="inline-flex items-center gap-1.5 rounded-full border px-3.5 py-1.5 text-sm font-medium transition-colors"
                :class="
                  selectedClient === guide.key
                    ? 'border-primary-500 bg-primary-50 text-primary-700 dark:border-primary-500 dark:bg-primary-900/25 dark:text-primary-300'
                    : 'border-gray-200 bg-white text-gray-600 hover:border-gray-300 hover:text-gray-900 dark:border-dark-600 dark:bg-dark-800/60 dark:text-dark-300 dark:hover:text-white'
                "
                @click="selectedClient = guide.key"
              >
                <img
                  v-if="guide.icon"
                  :src="guide.icon"
                  :alt="guide.name"
                  class="h-4 w-4 shrink-0 rounded-[3px] object-contain dark:bg-white dark:p-[2px]"
                />
                <span>{{ guide.name }}</span>
              </button>
            </div>

            <div class="border-t border-gray-100 dark:border-dark-700"></div>

            <!-- Active guide -->
            <div v-if="activeGuide" class="space-y-5">
              <p v-if="activeGuide.intro" class="text-sm leading-relaxed text-gray-600 dark:text-dark-300">
                {{ activeGuide.intro }}
              </p>

              <!-- API client: tabbed code samples -->
              <template v-if="activeGuide.key === 'api'">
                <div class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-600">
                  <div class="flex items-center justify-between border-b border-gray-200 px-4 py-2.5 dark:border-dark-600">
                    <div class="ml-1 flex gap-1">
                      <button
                        v-for="tab in CODE_TABS"
                        :key="tab"
                        type="button"
                        class="rounded px-2.5 py-1 text-xs transition-colors"
                        :class="
                          codeTab === tab
                            ? 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-white'
                            : 'text-gray-500 hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200'
                        "
                        @click="codeTab = tab"
                      >
                        {{ codeTabLabel(tab) }}
                      </button>
                    </div>
                    <button
                      type="button"
                      class="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-gray-500 transition-colors hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200"
                      @click="copyText(apiSample, 'api')"
                    >
                      <Icon :name="copiedKey === 'api' ? 'check' : 'copy'" size="sm" />
                      {{ copiedKey === 'api' ? t('tutorial.guide.copied') : t('tutorial.guide.copy') }}
                    </button>
                  </div>
                  <pre class="overflow-x-auto bg-white p-5 font-mono text-xs leading-relaxed text-gray-900 sm:text-sm dark:bg-dark-900 dark:text-gray-100">{{ apiSample }}</pre>
                </div>
              </template>

              <!-- Step-by-step guide -->
              <ol v-else class="space-y-6">
                <li
                  v-for="(step, si) in activeGuide.steps"
                  :key="si"
                  class="flex gap-3"
                >
                  <span class="pt-0.5 text-sm font-semibold text-gray-900 dark:text-white">{{ si + 1 }}.</span>
                  <div class="min-w-0 flex-1 space-y-3">
                    <p class="text-sm leading-relaxed text-gray-800 dark:text-dark-200">{{ step.text }}</p>
                    <div
                      v-for="(cmd, ci) in step.commands ?? []"
                      :key="ci"
                      class="group relative flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800/60"
                    >
                      <code class="overflow-x-auto whitespace-pre font-mono text-sm text-gray-900 dark:text-gray-100">{{ cmd }}</code>
                      <button
                        type="button"
                        class="shrink-0 text-gray-400 opacity-0 transition-opacity hover:text-gray-700 group-hover:opacity-100 dark:hover:text-dark-200"
                        @click="copyText(cmd, `cmd-${si}-${ci}`)"
                      >
                        <Icon :name="copiedKey === `cmd-${si}-${ci}` ? 'check' : 'copy'" size="sm" />
                      </button>
                    </div>
                    <p v-if="step.appendText" class="text-sm leading-relaxed text-gray-800 dark:text-dark-200">{{ step.appendText }}</p>
                    <div
                      v-for="(cmd, ci) in step.appendCommands ?? []"
                      :key="`a-${ci}`"
                      class="group relative flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-gray-50 px-4 py-3 dark:border-dark-600 dark:bg-dark-800/60"
                    >
                      <code class="overflow-x-auto whitespace-pre font-mono text-sm text-gray-900 dark:text-gray-100">{{ cmd }}</code>
                      <button
                        type="button"
                        class="shrink-0 text-gray-400 opacity-0 transition-opacity hover:text-gray-700 group-hover:opacity-100 dark:hover:text-dark-200"
                        @click="copyText(cmd, `acmd-${si}-${ci}`)"
                      >
                        <Icon :name="copiedKey === `acmd-${si}-${ci}` ? 'check' : 'copy'" size="sm" />
                      </button>
                    </div>
                    <!-- Config file card (mac-style dots + filename + copy) -->
                    <div v-if="step.file" class="overflow-hidden rounded-xl border border-gray-200 dark:border-dark-600">
                      <div class="flex items-center justify-between border-b border-gray-200 bg-gray-50 px-4 py-2.5 dark:border-dark-600 dark:bg-dark-800/60">
                        <div class="flex items-center gap-3">
                          <span class="flex gap-1.5">
                            <span class="h-3 w-3 rounded-full bg-[#ff5f57]"></span>
                            <span class="h-3 w-3 rounded-full bg-[#febc2e]"></span>
                            <span class="h-3 w-3 rounded-full bg-[#28c840]"></span>
                          </span>
                          <span class="font-mono text-xs text-gray-500 dark:text-dark-400">{{ step.file.path }}</span>
                        </div>
                        <button
                          type="button"
                          class="flex items-center gap-1.5 rounded px-2 py-1 text-xs text-gray-500 transition-colors hover:text-gray-800 dark:text-dark-400 dark:hover:text-dark-200"
                          @click="copyText(step.file.content, `file-${si}`)"
                        >
                          <Icon :name="copiedKey === `file-${si}` ? 'check' : 'copy'" size="sm" />
                          {{ copiedKey === `file-${si}` ? t('tutorial.guide.copied') : t('tutorial.guide.copy') }}
                        </button>
                      </div>
                      <pre class="overflow-x-auto bg-white p-5 font-mono text-xs leading-relaxed text-gray-900 sm:text-sm dark:bg-dark-900 dark:text-gray-100">{{ step.file.content }}</pre>
                    </div>
                  </div>
                </li>
              </ol>

              <!-- Extra section (e.g. env var usage) -->
              <div v-if="activeGuide.extra" class="space-y-3 rounded-xl border border-gray-100 bg-gray-50/60 p-4 dark:border-dark-600 dark:bg-dark-800/40">
                <p class="text-sm leading-relaxed text-gray-800 dark:text-dark-200">{{ activeGuide.extra.text }}</p>
                <div
                  v-for="(cmd, ci) in activeGuide.extra.commands"
                  :key="ci"
                  class="group relative flex items-center justify-between gap-3 rounded-lg border border-gray-200 bg-white px-4 py-3 dark:border-dark-600 dark:bg-dark-800/60"
                >
                  <code class="overflow-x-auto whitespace-pre font-mono text-sm text-gray-900 dark:text-gray-100">{{ cmd }}</code>
                  <button
                    type="button"
                    class="shrink-0 text-gray-400 opacity-0 transition-opacity hover:text-gray-700 group-hover:opacity-100 dark:hover:text-dark-200"
                    @click="copyText(cmd, `extra-${ci}`)"
                  >
                    <Icon :name="copiedKey === `extra-${ci}` ? 'check' : 'copy'" size="sm" />
                  </button>
                </div>
              </div>

              <!-- Tip -->
              <div
                v-if="activeGuide.tip"
                class="rounded-lg border border-primary-100 bg-primary-50 px-4 py-3 text-xs leading-relaxed text-primary-700 dark:border-primary-900/40 dark:bg-primary-900/20 dark:text-primary-300"
              >
                {{ activeGuide.tip }}
              </div>

              <p class="text-xs leading-relaxed text-gray-400 dark:text-dark-500">{{ t('tutorial.guide.configNote') }}</p>
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
                  v-for="cat in FAQ_CATEGORIES"
                  :key="cat.key"
                  type="button"
                  class="rounded-full px-3 py-1.5 text-xs font-medium transition-colors"
                  :class="
                    activeCategory === cat.key
                      ? 'bg-primary-600 text-white'
                      : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-700 dark:text-dark-300 dark:hover:bg-dark-600'
                  "
                  @click="activeCategory = cat.key"
                >
                  {{ cat.label }}
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
                  class="flex w-full items-center justify-between gap-3 px-5 py-4 text-left"
                  @click="toggleFaq(item.id)"
                >
                  <span class="text-sm font-semibold text-gray-900 dark:text-white">{{ item.q }}</span>
                  <svg
                    class="h-4 w-4 flex-shrink-0 text-gray-400 transition-transform duration-200"
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
                  <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-300">{{ item.a }}</p>
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
import { buildGuides, FAQ_CATEGORIES, FAQ_ITEMS, TUTORIAL_MODELS } from './tutorialContent'

const TABS = ['guide', 'faq'] as const
type Tab = (typeof TABS)[number]

const CODE_TABS = ['curl', 'python', 'sdk'] as const
type CodeTab = (typeof CODE_TABS)[number]

const { t, locale } = useI18n()
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

// ---------- Guide state ----------
const apiBase = computed(() => appStore.apiBaseUrl || window.location.origin)

const selectedModel = ref<string>(TUTORIAL_MODELS[0])
const selectedClient = ref('claude-code')

const guides = computed(() => buildGuides(apiBase.value, selectedModel.value))
const activeGuide = computed(() => guides.value.find((g) => g.key === selectedClient.value) ?? guides.value[0])

const codeTab = ref<CodeTab>('curl')

function codeTabLabel(tab: CodeTab) {
  return t(`tutorial.guide.tab${tab.charAt(0).toUpperCase()}${tab.slice(1)}`)
}

const apiSamples = computed<Record<CodeTab, string>>(() => ({
  curl: `curl ${apiBase.value}/v1/chat/completions \\
  -H "Authorization: Bearer sk-你的密钥" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "${selectedModel.value}",
    "messages": [{ "role": "user", "content": "你好" }]
  }'`,
  python: `import requests

resp = requests.post(
    "${apiBase.value}/v1/chat/completions",
    headers={"Authorization": "Bearer sk-你的密钥"},
    json={
        "model": "${selectedModel.value}",
        "messages": [{"role": "user", "content": "你好"}],
    },
)
print(resp.json()["choices"][0]["message"]["content"])`,
  sdk: `from openai import OpenAI

client = OpenAI(
    api_key="sk-你的密钥",
    base_url="${apiBase.value}/v1",
)

resp = client.chat.completions.create(
    model="${selectedModel.value}",
    messages=[{"role": "user", "content": "你好"}],
)
print(resp.choices[0].message.content)`,
}))

const apiSample = computed(() => apiSamples.value[codeTab.value])

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
const activeCategory = ref<string>('all')
const openFaqId = ref<string | null>(null)

const filteredFaqItems = computed(() => {
  const query = faqQuery.value.trim().toLowerCase()
  return FAQ_ITEMS.filter((item) => {
    if (activeCategory.value !== 'all' && item.category !== activeCategory.value) return false
    if (!query) return true
    return `${item.q}\n${item.a}`.toLowerCase().includes(query)
  })
})

function toggleFaq(id: string) {
  openFaqId.value = openFaqId.value === id ? null : id
}

// Locale switch swaps UI copy; collapse any expanded item to avoid stale state.
watch(locale, () => {
  openFaqId.value = null
})
</script>
