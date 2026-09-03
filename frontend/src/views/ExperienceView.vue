<template>
  <ProductShell>
    <div class="space-y-6">
      <section class="flex flex-col justify-between gap-5 border-b border-gray-200 pb-6 dark:border-dark-800 md:flex-row md:items-end">
        <div>
          <p class="text-xs font-semibold uppercase tracking-[0.16em] text-primary-600 dark:text-primary-300">{{ t('product.experience.eyebrow') }}</p>
          <h1 class="mt-2 text-3xl font-semibold tracking-tight text-gray-950 dark:text-white sm:text-4xl">{{ t('product.experience.title') }}</h1>
          <p class="mt-2 max-w-2xl text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('product.experience.description') }}</p>
        </div>
        <div v-if="isAuthenticated" class="flex shrink-0 items-center gap-3 rounded-xl border border-gray-200 bg-white px-4 py-3 dark:border-dark-700 dark:bg-dark-900">
          <span class="flex h-9 w-9 items-center justify-center rounded-lg bg-primary-50 text-primary-600 dark:bg-primary-900/30 dark:text-primary-300">
            <Icon name="creditCard" size="sm" />
          </span>
          <div>
            <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('product.balance') }}</p>
            <p class="text-base font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatBalance(balance) }}</p>
          </div>
        </div>
      </section>

      <div v-if="!isAuthenticated" class="rounded-2xl border border-primary-200 bg-primary-50/70 px-5 py-6 dark:border-primary-800/60 dark:bg-primary-900/15">
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div>
            <h2 class="text-base font-semibold text-primary-950 dark:text-primary-100">{{ t('product.experience.loginTitle') }}</h2>
            <p class="mt-1 text-sm text-primary-800/80 dark:text-primary-200/80">{{ t('product.experience.loginDescription') }}</p>
          </div>
          <RouterLink :to="{ path: '/login', query: { redirect: '/experience' } }" class="btn btn-primary shrink-0">
            <Icon name="login" size="sm" />
            {{ t('product.login') }}
          </RouterLink>
        </div>
      </div>

      <section v-else class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_280px]">
        <div class="overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm dark:border-dark-800 dark:bg-dark-900">
          <div class="flex flex-wrap items-center justify-between gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-800 sm:px-6">
            <div>
              <h2 class="text-base font-semibold text-gray-950 dark:text-white">{{ t('product.experience.workspaceTitle') }}</h2>
              <p class="mt-1 text-xs text-gray-500 dark:text-dark-400">{{ t('product.experience.workspaceHint') }}</p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="loadResources">
              <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />
              {{ t('product.refresh') }}
            </button>
          </div>

          <div class="grid gap-4 border-b border-gray-100 px-5 py-4 dark:border-dark-800 sm:grid-cols-2 sm:px-6">
            <label class="block">
              <span class="input-label">{{ t('product.experience.apiKey') }}</span>
              <select v-model="selectedKeyId" class="input" :disabled="loading || keys.length === 0">
                <option value="">{{ keys.length ? t('product.experience.selectKey') : t('product.experience.noKey') }}</option>
                <option v-for="key in keys" :key="key.id" :value="String(key.id)">{{ key.name }} ···{{ key.key.slice(-4) }}</option>
              </select>
            </label>
            <label class="block">
              <span class="input-label">{{ t('product.experience.model') }}</span>
              <select v-model="selectedModel" class="input" :disabled="loading || models.length === 0">
                <option value="">{{ models.length ? t('product.experience.selectModel') : t('product.experience.noModel') }}</option>
                <option v-for="model in models" :key="`${model.name}-${model.groupId}`" :value="model.name">{{ model.name }} · {{ model.platform }}</option>
              </select>
            </label>
          </div>

          <div class="flex min-h-[360px] flex-col">
            <div class="flex-1 space-y-5 px-5 py-6 sm:px-8">
              <div v-if="messages.length === 0" class="flex min-h-[280px] items-center justify-center text-center">
                <div class="max-w-md">
                  <span class="mx-auto flex h-12 w-12 items-center justify-center rounded-2xl bg-gray-100 text-gray-500 dark:bg-dark-800 dark:text-dark-300">
                    <Icon name="chat" size="lg" />
                  </span>
                  <h3 class="mt-4 text-base font-semibold text-gray-900 dark:text-white">{{ t('product.experience.emptyTitle') }}</h3>
                  <p class="mt-2 text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('product.experience.emptyDescription') }}</p>
                </div>
              </div>
              <div v-for="(message, index) in messages" :key="`${message.role}-${index}`" class="flex gap-3" :class="message.role === 'user' ? 'justify-end' : 'justify-start'">
                <div class="max-w-[86%] rounded-2xl px-4 py-3 text-sm leading-6" :class="message.role === 'user' ? 'bg-primary-600 text-white' : 'bg-gray-100 text-gray-800 dark:bg-dark-800 dark:text-dark-100'">
                  <p class="whitespace-pre-wrap break-words">{{ message.content }}</p>
                </div>
              </div>
              <div v-if="sending" class="flex items-center gap-2 text-sm text-gray-500 dark:text-dark-400">
                <span class="flex gap-1" aria-hidden="true"><i class="h-1.5 w-1.5 animate-pulse rounded-full bg-primary-500"></i><i class="h-1.5 w-1.5 animate-pulse rounded-full bg-primary-500 [animation-delay:120ms]"></i><i class="h-1.5 w-1.5 animate-pulse rounded-full bg-primary-500 [animation-delay:240ms]"></i></span>
                {{ t('product.experience.generating') }}
              </div>
            </div>

            <div v-if="errorMessage" class="mx-5 mb-4 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-300 sm:mx-6">
              {{ errorMessage }}
            </div>
            <form class="border-t border-gray-100 p-4 dark:border-dark-800 sm:p-5" @submit.prevent="sendMessage">
              <label class="sr-only" for="experience-prompt">{{ t('product.experience.promptLabel') }}</label>
              <textarea
                id="experience-prompt"
                v-model="prompt"
                class="input min-h-[112px] resize-y leading-6"
                :placeholder="t('product.experience.promptPlaceholder')"
                :disabled="sending || !selectedKeyId || !selectedModel"
                @keydown.enter.exact.prevent="sendMessage"
              ></textarea>
              <div class="mt-3 flex items-center justify-between gap-3">
                <p class="text-xs text-gray-400 dark:text-dark-500">{{ t('product.experience.billingHint') }}</p>
                <button type="submit" class="btn btn-primary shrink-0" :disabled="sending || !prompt.trim() || !selectedKeyId || !selectedModel">
                  <Icon name="play" size="sm" />
                  {{ sending ? t('product.experience.sending') : t('product.experience.send') }}
                </button>
              </div>
            </form>
          </div>
        </div>

        <aside class="space-y-4">
          <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-800 dark:bg-dark-900">
            <h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('product.experience.statusTitle') }}</h2>
            <dl class="mt-4 space-y-3 text-sm">
              <div class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-dark-400">{{ t('product.experience.keyStatus') }}</dt><dd :class="keys.length ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'">{{ keys.length ? t('product.experience.ready') : t('product.experience.needsKey') }}</dd></div>
              <div class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-dark-400">{{ t('product.experience.modelStatus') }}</dt><dd :class="models.length ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'">{{ models.length ? `${models.length} ${t('product.experience.available')}` : t('product.experience.unavailable') }}</dd></div>
              <div class="flex items-center justify-between gap-3"><dt class="text-gray-500 dark:text-dark-400">{{ t('product.balance') }}</dt><dd class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatBalance(balance) }}</dd></div>
            </dl>
          </div>
          <div class="rounded-2xl border border-gray-200 bg-white p-5 dark:border-dark-800 dark:bg-dark-900">
            <div class="flex items-center gap-2"><Icon name="infoCircle" size="sm" class="text-primary-600 dark:text-primary-300" /><h2 class="text-sm font-semibold text-gray-950 dark:text-white">{{ t('product.experience.realUsageTitle') }}</h2></div>
            <p class="mt-3 text-sm leading-6 text-gray-500 dark:text-dark-400">{{ t('product.experience.realUsageDescription') }}</p>
            <RouterLink to="/model-plaza" class="mt-4 inline-flex items-center gap-1.5 text-sm font-medium text-primary-600 hover:text-primary-700 dark:text-primary-300 dark:hover:text-primary-200"><Icon name="grid" size="sm" />{{ t('product.experience.viewModels') }}</RouterLink>
          </div>
        </aside>
      </section>
    </div>
  </ProductShell>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import ProductShell from '@/components/product/ProductShell.vue'
import Icon from '@/components/icons/Icon.vue'
import { keysAPI } from '@/api/keys'
import { createChatCompletion, extractAssistantContent } from '@/api/gateway'
import { getModelPlaza, type ModelPlazaResponse } from '@/api/modelPlaza'
import type { ApiKey } from '@/types'
import { useAuthStore } from '@/stores/auth'
import { DEFAULT_PAYMENT_CURRENCY, formatPaymentAmount } from '@/components/payment/currency'

type ChatMessage = { role: 'user' | 'assistant'; content: string }
type ExperienceModel = { name: string; platform: string; groupId: number }

const { t } = useI18n()
const authStore = useAuthStore()

const keys = ref<ApiKey[]>([])
const plaza = ref<ModelPlazaResponse | null>(null)
const selectedKeyId = ref('')
const selectedModel = ref('')
const prompt = ref('')
const messages = ref<ChatMessage[]>([])
const loading = ref(false)
const sending = ref(false)
const errorMessage = ref('')

const isAuthenticated = computed(() => authStore.isAuthenticated)
const balance = computed(() => authStore.user?.balance ?? 0)

const selectedKey = computed(() => keys.value.find((key) => String(key.id) === selectedKeyId.value))
const models = computed<ExperienceModel[]>(() => {
  const groupId = selectedKey.value?.group_id
  const source = (plaza.value?.groups ?? []).filter((group) => groupId == null || group.id === groupId)
  const seen = new Set<string>()
  const result: ExperienceModel[] = []
  for (const group of source) {
    for (const model of group.models) {
      if (!isLikelyTextModel(model.name) || seen.has(model.name)) continue
      seen.add(model.name)
      result.push({ name: model.name, platform: model.platform, groupId: group.id })
    }
  }
  return result.sort((a, b) => a.name.localeCompare(b.name))
})

function isLikelyTextModel(name: string) {
  return !/(^|[-_/])(embedding|embed|rerank|moderation|tts|speech|audio|video|image|vision)([-_/]|$)/i.test(name)
}

function formatBalance(value: number) {
  return formatPaymentAmount(value, DEFAULT_PAYMENT_CURRENCY)
}

async function loadResources() {
  if (!isAuthenticated.value) return
  loading.value = true
  errorMessage.value = ''
  try {
    const [keyResponse, plazaResponse] = await Promise.all([
      keysAPI.list(1, 100, { status: 'active' }),
      getModelPlaza()
    ])
    keys.value = keyResponse.items
    plaza.value = plazaResponse
    if (!keys.value.some((key) => String(key.id) === selectedKeyId.value)) {
      selectedKeyId.value = keys.value[0] ? String(keys.value[0].id) : ''
    }
  } catch {
    errorMessage.value = t('product.experience.loadFailed')
  } finally {
    loading.value = false
  }
}

async function sendMessage() {
  const content = prompt.value.trim()
  const key = selectedKey.value
  if (!content || !key || !selectedModel.value || sending.value) return

  errorMessage.value = ''
  messages.value.push({ role: 'user', content })
  prompt.value = ''
  sending.value = true
  try {
    const response = await createChatCompletion(key.key, selectedModel.value, content)
    const assistantContent = extractAssistantContent(response)
    if (!assistantContent) throw new Error(t('product.experience.emptyResponse'))
    messages.value.push({ role: 'assistant', content: assistantContent })
    await authStore.refreshUser().catch(() => undefined)
  } catch (error) {
    const status = (error as { status?: number }).status
    if (status === 402 || status === 403) {
      errorMessage.value = t('product.experience.insufficientBalance')
    } else if (status === 401) {
      errorMessage.value = t('product.experience.invalidKey')
    } else {
      errorMessage.value = error instanceof Error ? error.message : t('product.experience.requestFailed')
    }
  } finally {
    sending.value = false
  }
}

watch(selectedKeyId, () => {
  if (!models.value.some((model) => model.name === selectedModel.value)) {
    selectedModel.value = models.value[0]?.name || ''
  }
})

watch(models, (availableModels) => {
  if (!availableModels.some((model) => model.name === selectedModel.value)) {
    selectedModel.value = availableModels[0]?.name || ''
  }
})

watch(isAuthenticated, (authenticated) => {
  if (authenticated) {
    void loadResources()
    return
  }
  keys.value = []
  plaza.value = null
  selectedKeyId.value = ''
  selectedModel.value = ''
  messages.value = []
})

onMounted(() => {
  void loadResources()
})
</script>
