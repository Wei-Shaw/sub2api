<template>
  <AppLayout>
    <div class="mx-auto flex h-[calc(100vh-9rem)] min-h-[32rem] max-w-5xl flex-col gap-4">
      <!-- Target: account + model -->
      <div class="card p-4">
        <div class="grid gap-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto] sm:items-end">
          <div data-testid="playground-account">
            <label class="input-label">{{ t('admin.playground.account') }}</label>
            <Select
              v-model="accountId"
              :options="accountOptions"
              :placeholder="t('admin.playground.accountPlaceholder')"
              :empty-text="t('admin.playground.noAccounts')"
              remote
              :loading="accountsLoading"
              @search="onAccountSearch"
            />
          </div>
          <div data-testid="playground-model">
            <label class="input-label">{{ t('admin.playground.model') }}</label>
            <Select
              v-model="modelId"
              :options="modelOptions"
              :placeholder="modelPlaceholder"
              :disabled="accountId === null || modelsLoading"
              searchable
              creatable
            />
          </div>
          <div class="flex gap-2">
            <button
              type="button"
              class="btn btn-secondary"
              :aria-expanded="showSettings"
              @click="showSettings = !showSettings"
            >
              {{ t('admin.playground.settings') }}
            </button>
            <button
              type="button"
              class="btn btn-secondary"
              :disabled="isEmpty"
              data-testid="playground-clear"
              @click="clear"
            >
              {{ t('admin.playground.clear') }}
            </button>
          </div>
        </div>

        <p v-if="modelsError" class="mt-2 text-xs text-amber-600 dark:text-amber-400">
          {{ t('admin.playground.modelsLoadFailed') }}
        </p>

        <div v-if="showSettings" class="mt-4 grid gap-3 border-t border-gray-100 pt-4 dark:border-dark-700 sm:grid-cols-4">
          <div class="sm:col-span-4">
            <label class="input-label" for="playground-system">{{ t('admin.playground.system') }}</label>
            <textarea
              id="playground-system"
              v-model="system"
              rows="2"
              class="input"
              :placeholder="t('admin.playground.systemPlaceholder')"
            />
            <p class="input-hint">{{ t('admin.playground.systemHint') }}</p>
          </div>
          <div class="sm:col-span-2">
            <label class="input-label" for="playground-max-tokens">{{ t('admin.playground.maxTokens') }}</label>
            <input
              id="playground-max-tokens"
              v-model.number="maxTokens"
              type="number"
              min="1"
              max="128000"
              step="1"
              class="input"
              :placeholder="t('admin.playground.defaultValue')"
            />
          </div>
          <div class="sm:col-span-2">
            <label class="input-label" for="playground-temperature">{{ t('admin.playground.temperature') }}</label>
            <input
              id="playground-temperature"
              v-model.number="temperature"
              type="number"
              min="0"
              max="2"
              step="0.1"
              class="input"
              :placeholder="t('admin.playground.defaultValue')"
            />
          </div>
          <p class="input-hint sm:col-span-4">{{ t('admin.playground.paramsHint') }}</p>
        </div>
      </div>

      <!-- Conversation -->
      <div class="card flex min-h-0 flex-1 flex-col">
        <div ref="scrollRef" class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4" data-testid="playground-messages">
          <div v-if="isEmpty" class="flex h-full flex-col items-center justify-center text-center">
            <Icon name="chat" size="xl" class="text-gray-300 dark:text-dark-500" />
            <p class="mt-3 text-sm font-medium text-gray-700 dark:text-gray-200">
              {{ t('admin.playground.emptyTitle') }}
            </p>
            <p class="mt-1 max-w-md text-xs text-gray-500 dark:text-gray-400">
              {{ t('admin.playground.emptyHint') }}
            </p>
          </div>

          <div
            v-for="message in messages"
            :key="message.id"
            :class="['flex', message.role === 'user' ? 'justify-end' : 'justify-start']"
          >
            <div class="max-w-[85%]">
              <div
                :class="[
                  'whitespace-pre-wrap break-words rounded-2xl px-4 py-2.5 text-sm leading-relaxed',
                  message.role === 'user'
                    ? 'bg-primary-600 text-white'
                    : 'bg-gray-100 text-gray-900 dark:bg-dark-700 dark:text-gray-100'
                ]"
                :data-role="message.role"
              >
                <template v-if="message.content">{{ message.content }}</template>
                <span v-else-if="message.status === 'streaming'" class="text-gray-400">
                  {{ t('admin.playground.waiting') }}
                </span>
                <span v-else-if="message.status === 'stopped'" class="text-gray-400">
                  {{ t('admin.playground.stopped') }}
                </span>
                <span v-else-if="message.status === 'error'" class="text-gray-400">
                  {{ t('admin.playground.noReply') }}
                </span>
              </div>

              <p
                v-if="message.error"
                class="mt-1.5 whitespace-pre-wrap break-words rounded-lg bg-red-50 px-3 py-2 text-xs text-red-700 dark:bg-red-900/20 dark:text-red-300"
                role="alert"
              >
                {{ message.error }}
              </p>
              <p
                v-if="message.conversationNotApplied"
                class="mt-1.5 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300"
              >
                {{ t('admin.playground.conversationNotApplied') }}
              </p>
              <p
                v-if="message.role === 'assistant' && message.status !== 'streaming'"
                class="mt-1 px-1 text-[11px] text-gray-400"
              >
                {{ formatMeta(message) }}
              </p>
            </div>
          </div>
        </div>

        <!-- Composer -->
        <form class="border-t border-gray-100 p-3 dark:border-dark-700" @submit.prevent="submit">
          <div class="flex items-end gap-2">
            <textarea
              v-model="draft"
              rows="2"
              class="input min-h-[2.75rem] flex-1 resize-none"
              :placeholder="composerPlaceholder"
              :disabled="!canCompose"
              data-testid="playground-input"
              @keydown.enter.exact="onEnter"
            />
            <button
              v-if="sending"
              type="button"
              class="btn btn-danger"
              data-testid="playground-stop"
              @click="stop"
            >
              {{ t('admin.playground.stop') }}
            </button>
            <button
              v-else
              type="submit"
              class="btn btn-primary"
              :disabled="!canSend"
              data-testid="playground-send"
            >
              {{ t('admin.playground.send') }}
            </button>
          </div>
          <p class="mt-1.5 px-1 text-[11px] text-gray-400">{{ t('admin.playground.billingNote') }}</p>
        </form>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import AppLayout from '@/components/layout/AppLayout.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import { usePlaygroundChat, type PlaygroundMessage } from '@/composables/usePlaygroundChat'

const { t } = useI18n()
const { messages, sending, isEmpty, send, stop, clear } = usePlaygroundChat()

// --- Account picker (remote search) ---
const accountId = ref<number | null>(null)
const accountOptions = ref<SelectOption[]>([])
const accountsLoading = ref(false)
let accountSearchSeq = 0

async function loadAccounts(search: string): Promise<void> {
  const seq = ++accountSearchSeq
  accountsLoading.value = true
  try {
    const page = await adminAPI.accounts.list(1, 30, { search: search.trim() || undefined, lite: '1' })
    if (seq !== accountSearchSeq) return
    const options: SelectOption[] = page.items.map((account) => ({
      value: account.id,
      label: `${account.name} · ${account.platform}/${account.type}`
    }))
    // Keep the current selection visible when a search no longer matches it.
    const selected = accountOptions.value.find((option) => option.value === accountId.value)
    if (selected && !options.some((option) => option.value === selected.value)) {
      options.unshift(selected)
    }
    accountOptions.value = options
  } catch {
    if (seq === accountSearchSeq) accountOptions.value = []
  } finally {
    if (seq === accountSearchSeq) accountsLoading.value = false
  }
}

function onAccountSearch(query: string): void {
  void loadAccounts(query)
}

// --- Model picker (per account) ---
const modelId = ref<string | null>(null)
const modelOptions = ref<SelectOption[]>([])
const modelsLoading = ref(false)
const modelsError = ref(false)
let modelLoadSeq = 0

watch(accountId, async (id) => {
  const seq = ++modelLoadSeq
  modelId.value = null
  modelOptions.value = []
  modelsError.value = false
  // A conversation belongs to one account: switching starts a new one.
  clear()
  if (id === null) return

  modelsLoading.value = true
  try {
    const models = await adminAPI.accounts.getAvailableModels(id)
    if (seq !== modelLoadSeq) return
    modelOptions.value = models.map((model) => ({
      value: model.id,
      label: model.display_name && model.display_name !== model.id ? `${model.display_name} (${model.id})` : model.id
    }))
    modelId.value = models[0]?.id ?? null
  } catch {
    if (seq === modelLoadSeq) modelsError.value = true
  } finally {
    if (seq === modelLoadSeq) modelsLoading.value = false
  }
})

const modelPlaceholder = computed(() =>
  modelsLoading.value ? t('admin.playground.modelsLoading') : t('admin.playground.modelPlaceholder')
)

// --- Parameters ---
const showSettings = ref(false)
const system = ref('')
// v-model.number yields '' for an emptied field; both '' and null mean "default".
const maxTokens = ref<number | '' | null>(null)
const temperature = ref<number | '' | null>(null)

// --- Composer ---
const draft = ref('')
const canCompose = computed(() => accountId.value !== null && !!modelId.value)
const canSend = computed(() => canCompose.value && !sending.value && draft.value.trim().length > 0)
const composerPlaceholder = computed(() =>
  canCompose.value ? t('admin.playground.inputPlaceholder') : t('admin.playground.selectFirst')
)

async function submit(): Promise<void> {
  if (!canSend.value || accountId.value === null || !modelId.value) return
  const text = draft.value
  draft.value = ''
  await send(text, {
    accountId: accountId.value,
    modelId: modelId.value,
    system: system.value,
    maxTokens: typeof maxTokens.value === 'number' ? maxTokens.value : null,
    temperature: typeof temperature.value === 'number' ? temperature.value : null
  })
}

function onEnter(event: KeyboardEvent): void {
  // Leave Enter alone while an IME composition is being confirmed.
  if (event.isComposing) return
  event.preventDefault()
  void submit()
}

// --- Message list ---
const scrollRef = ref<HTMLElement | null>(null)

watch(
  () => [messages.value.length, messages.value[messages.value.length - 1]?.content.length],
  async () => {
    await nextTick()
    const el = scrollRef.value
    if (el) el.scrollTop = el.scrollHeight
  }
)

function formatDuration(ms: number): string {
  return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`
}

function formatMeta(message: PlaygroundMessage): string {
  const parts: string[] = []
  // A stopped reply that already has text shows no placeholder, so say it here.
  if (message.status === 'stopped' && message.content) parts.push(t('admin.playground.stopped'))
  if (message.model) parts.push(message.model)
  if (message.firstTokenMs !== undefined) {
    parts.push(t('admin.playground.firstToken', { time: formatDuration(message.firstTokenMs) }))
  }
  if (message.totalMs !== undefined) {
    parts.push(t('admin.playground.total', { time: formatDuration(message.totalMs) }))
  }
  return parts.join(' · ')
}

onMounted(() => {
  void loadAccounts('')
})
</script>
