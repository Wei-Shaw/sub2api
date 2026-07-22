<template>
  <AppLayout>
    <div class="mx-auto max-w-6xl space-y-6">
      <section class="overflow-hidden rounded-3xl border border-emerald-100 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
        <div class="bg-gradient-to-r from-emerald-600 via-teal-600 to-cyan-600 px-6 py-8 text-white">
          <p class="text-sm font-medium uppercase tracking-[0.24em] text-emerald-100">{{ t('playground.eyebrow') }}</p>
          <h1 class="mt-3 text-3xl font-bold">{{ t('playground.title') }}</h1>
          <p class="mt-2 max-w-2xl text-sm text-emerald-50">
            {{ t('playground.description') }}
          </p>
        </div>

        <div class="grid gap-6 p-6 lg:grid-cols-[minmax(0,1fr)_360px]">
          <form class="space-y-5" @submit.prevent="runTest">
            <div class="grid gap-4 md:grid-cols-2">
              <label class="block">
                <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('playground.apiKey') }}</span>
                <select
                  v-model.number="form.api_key_id"
                  class="w-full rounded-xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none transition focus:border-emerald-500 focus:ring-4 focus:ring-emerald-100 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:focus:ring-emerald-900/30"
                >
                  <option :value="null">{{ t('playground.autoSelectKey') }}</option>
                  <option v-for="key in activeKeys" :key="key.id" :value="key.id">
                    {{ key.name }} #{{ key.id }}{{ key.group?.name ? ` · ${key.group.name}` : '' }}
                  </option>
                </select>
              </label>

              <label class="block">
                <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('playground.model') }}</span>
                <input
                  v-model.trim="form.model"
                  list="playground-model-options"
                  class="w-full rounded-xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none transition focus:border-emerald-500 focus:ring-4 focus:ring-emerald-100 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:focus:ring-emerald-900/30"
                  placeholder="gpt-5.4"
                />
                <datalist id="playground-model-options">
                  <option v-for="model in modelOptions" :key="model" :value="model" />
                </datalist>
                <span class="mt-1 block text-xs text-gray-400">
                  {{ modelOptionsLoading ? t('playground.loadingModels') : modelOptionsHint }}
                </span>
              </label>
            </div>

            <label class="block">
              <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('playground.prompt') }}</span>
              <textarea
                v-model="form.prompt"
                rows="8"
                maxlength="4000"
                class="w-full resize-y rounded-2xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none transition focus:border-emerald-500 focus:ring-4 focus:ring-emerald-100 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:focus:ring-emerald-900/30"
                :placeholder="t('playground.promptPlaceholder')"
              ></textarea>
              <span class="mt-1 block text-xs text-gray-400">{{ form.prompt.length }}/4000</span>
            </label>

            <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
              <label class="block w-full sm:w-48">
                <span class="mb-2 block text-sm font-medium text-gray-700 dark:text-gray-200">{{ t('playground.maxTokens') }}</span>
                <input
                  v-model.number="form.max_tokens"
                  type="number"
                  min="1"
                  max="1024"
                  class="w-full rounded-xl border border-gray-200 bg-white px-4 py-3 text-sm text-gray-900 outline-none transition focus:border-emerald-500 focus:ring-4 focus:ring-emerald-100 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:focus:ring-emerald-900/30"
                />
              </label>

              <button
                type="submit"
                :disabled="loading || activeKeysLoading"
                class="inline-flex h-12 items-center justify-center rounded-xl bg-emerald-600 px-6 text-sm font-semibold text-white shadow-lg shadow-emerald-600/20 transition hover:bg-emerald-700 disabled:cursor-not-allowed disabled:opacity-60"
              >
                {{ loading ? t('playground.testing') : t('playground.startTest') }}
              </button>
            </div>
          </form>

          <aside class="space-y-4">
            <div class="rounded-2xl border border-gray-100 bg-gray-50 p-5 dark:border-dark-700 dark:bg-dark-800/60">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('playground.billingTitle') }}</h2>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-gray-300">
                {{ t('playground.billingDescription') }}
              </p>
            </div>

            <div class="rounded-2xl border border-gray-100 bg-gray-50 p-5 dark:border-dark-700 dark:bg-dark-800/60">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('playground.minimalTitle') }}</h2>
              <button
                type="button"
                class="mt-3 rounded-lg border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm font-medium text-emerald-700 hover:bg-emerald-100 dark:border-emerald-900/50 dark:bg-emerald-900/20 dark:text-emerald-200"
                @click="useMinimalPrompt"
              >
                {{ t('playground.useMinimalPrompt') }}
              </button>
            </div>
          </aside>
        </div>
      </section>

      <section v-if="error" class="rounded-2xl border border-red-200 bg-red-50 p-5 text-red-700 dark:border-red-900/50 dark:bg-red-950/30 dark:text-red-200">
        {{ error }}
      </section>

      <section v-if="result" class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_360px]">
        <div class="rounded-3xl border border-gray-100 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <div class="mb-4 flex items-center justify-between gap-3">
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('playground.responseTitle') }}</h2>
            <span
              class="rounded-full px-3 py-1 text-xs font-semibold"
              :class="result.success ? 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-200' : 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-200'"
            >
              HTTP {{ result.raw_status }}
            </span>
          </div>

          <pre v-if="result.success" class="whitespace-pre-wrap rounded-2xl bg-gray-950 p-4 text-sm leading-6 text-gray-100">{{ result.text || t('playground.emptyResponse') }}</pre>
          <div v-else class="rounded-2xl bg-red-950 p-4 text-sm leading-6 text-red-100">
            <p class="font-semibold">{{ result.error?.code || 'error' }}</p>
            <p class="mt-2">{{ result.error?.message }}</p>
            <p v-if="result.error?.suggestion" class="mt-3 text-red-200">{{ result.error.suggestion }}</p>
          </div>
        </div>

        <div class="rounded-3xl border border-gray-100 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('playground.diagnosticTitle') }}</h2>
          <dl class="mt-4 space-y-3 text-sm">
            <div class="flex justify-between gap-4">
              <dt class="text-gray-500">{{ t('playground.key') }}</dt>
              <dd class="text-right font-medium text-gray-900 dark:text-white">{{ result.api_key_name }} #{{ result.api_key_id }}</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-gray-500">{{ t('playground.requestedModel') }}</dt>
              <dd class="text-right font-medium text-gray-900 dark:text-white">{{ result.model }}</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-gray-500">{{ t('playground.returnedModel') }}</dt>
              <dd class="text-right font-medium text-gray-900 dark:text-white">{{ result.resolved_model || '-' }}</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-gray-500">{{ t('playground.duration') }}</dt>
              <dd class="text-right font-medium text-gray-900 dark:text-white">{{ result.duration_ms }} ms</dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-gray-500">{{ t('playground.balanceChange') }}</dt>
              <dd class="text-right font-medium text-gray-900 dark:text-white">
                {{ formatMoney(result.balance_before) }} → {{ formatMoney(result.balance_after) }}
              </dd>
            </div>
            <div class="flex justify-between gap-4">
              <dt class="text-gray-500">{{ t('playground.cost') }}</dt>
              <dd class="text-right font-semibold text-emerald-600">{{ formatMoney(result.cost) }}</dd>
            </div>
          </dl>
          <p v-if="!result.billing_settled && result.success" class="mt-4 rounded-xl bg-amber-50 p-3 text-xs leading-5 text-amber-700 dark:bg-amber-950/30 dark:text-amber-200">
            {{ t('playground.billingPending') }}
          </p>
        </div>
      </section>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import { keysAPI } from '@/api/keys'
import { playgroundAPI, type PlaygroundChatResponse } from '@/api/playground'
import type { ApiKey } from '@/types'

const { t } = useI18n()

const activeKeys = ref<ApiKey[]>([])
const activeKeysLoading = ref(false)
const modelOptions = ref<string[]>([])
const modelOptionsHint = ref('')
const modelOptionsLoading = ref(false)
const loading = ref(false)
const error = ref('')
const result = ref<PlaygroundChatResponse | null>(null)

const form = reactive({
  api_key_id: null as number | null,
  model: 'gpt-5.4',
  prompt: t('playground.promptPlaceholder'),
  max_tokens: 128,
})

const hasActiveKey = computed(() => activeKeys.value.length > 0)

async function loadKeys() {
  activeKeysLoading.value = true
  error.value = ''
  try {
    const res = await keysAPI.list(1, 100, { status: 'active', sort_by: 'created_at', sort_order: 'desc' })
    activeKeys.value = res.items || []
    await loadModelOptions()
  } catch (err: any) {
    error.value = err?.message || t('playground.loadKeysFailed')
  } finally {
    activeKeysLoading.value = false
  }
}

async function loadModelOptions() {
  if (!hasActiveKey.value) {
    modelOptions.value = []
    modelOptionsHint.value = ''
    return
  }
  modelOptionsLoading.value = true
  try {
    const res = await playgroundAPI.getModels(form.api_key_id)
    modelOptions.value = res.models || []
    const sourceLabel = t(`playground.modelSources.${res.source}`)
    modelOptionsHint.value = res.group_name
      ? `${res.group_name} · ${sourceLabel}`
      : sourceLabel
    const preferred = res.default_model || modelOptions.value[0]
    if (preferred && (!form.model || !modelOptions.value.includes(form.model))) {
      form.model = preferred
    }
  } catch (err: any) {
    modelOptions.value = []
    modelOptionsHint.value = err?.message || t('playground.loadModelsFailed')
  } finally {
    modelOptionsLoading.value = false
  }
}

async function runTest() {
  error.value = ''
  result.value = null
  if (!hasActiveKey.value) {
    error.value = t('playground.noActiveKey')
    return
  }
  loading.value = true
  try {
    result.value = await playgroundAPI.testChat({
      api_key_id: form.api_key_id,
      model: form.model,
      prompt: form.prompt,
      max_tokens: form.max_tokens,
    })
  } catch (err: any) {
    error.value = err?.message || t('playground.requestFailed')
  } finally {
    loading.value = false
  }
}

function useMinimalPrompt() {
  form.prompt = t('playground.promptPlaceholder')
  form.max_tokens = 64
}

function formatMoney(value: number) {
  return `$${Number(value || 0).toFixed(6)}`
}

onMounted(loadKeys)

watch(() => form.api_key_id, () => {
  result.value = null
  void loadModelOptions()
})
</script>
