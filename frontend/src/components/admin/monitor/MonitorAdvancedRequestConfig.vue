<template>
  <div class="space-y-4">
    <!-- Headers textarea -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.headers') }}</label>
      <textarea
        v-model="headersText"
        rows="4"
        :placeholder="t('admin.channelMonitor.advanced.headersPlaceholder')"
        class="input font-mono text-xs"
        @blur="commitHeaders"
      />
      <p v-if="headersError" class="mt-1 text-xs text-red-500">{{ headersError }}</p>
      <p v-else class="mt-1 text-xs text-gray-400">
        {{ t('admin.channelMonitor.advanced.headersHint') }}
      </p>
    </div>

    <!-- Body mode radio -->
    <div>
      <label class="input-label">{{ t('admin.channelMonitor.advanced.bodyMode') }}</label>
      <div class="grid grid-cols-3 gap-3">
        <button
          v-for="opt in bodyModeOptions"
          :key="opt.value"
          type="button"
          class="rounded-lg border-2 px-3 py-2 text-sm font-medium transition-colors"
          :class="bodyModeButtonClass(opt.value)"
          @click="updateBodyMode(opt.value)"
        >
          {{ opt.label }}
        </button>
      </div>
      <p class="mt-1 text-xs text-gray-400">
        {{ bodyModeHint }}
      </p>
    </div>

    <!-- Body JSON (仅当 mode != off) -->
    <div v-if="bodyOverrideMode !== 'off'">
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label !mb-0">{{ t('admin.channelMonitor.advanced.bodyJson') }}</label>
        <button
          type="button"
          class="text-xs text-primary-600 hover:underline disabled:cursor-not-allowed disabled:text-gray-400 disabled:no-underline dark:text-primary-400"
          :disabled="!bodyText.trim()"
          @click="formatBody"
        >
          {{ t('admin.channelMonitor.advanced.bodyJsonFormat') }}
        </button>
      </div>
      <textarea
        v-model="bodyText"
        rows="10"
        :placeholder="bodyPlaceholder"
        class="input font-mono text-xs"
        style="white-space: pre; overflow-wrap: normal; overflow-x: auto;"
        spellcheck="false"
        @blur="commitBody"
      />
      <p v-if="bodyError" class="mt-1 text-xs text-red-500">{{ bodyError }}</p>
      <p v-else class="mt-1 text-xs text-gray-400">
        {{ t('admin.channelMonitor.advanced.bodyJsonHint') }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { BodyOverrideMode } from '@/api/admin/channelMonitor'

const props = defineProps<{
  extraHeaders: Record<string, string>
  bodyOverrideMode: BodyOverrideMode
  bodyOverride: Record<string, unknown> | null
}>()

const emit = defineEmits<{
  (e: 'update:extraHeaders', value: Record<string, string>): void
  (e: 'update:bodyOverrideMode', value: BodyOverrideMode): void
  (e: 'update:bodyOverride', value: Record<string, unknown> | null): void
}>()

const { t } = useI18n()

// ---- Headers textarea (Key: Value per line) ----
const headersText = ref(serializeHeaders(props.extraHeaders))
const headersError = ref('')

watch(
  () => props.extraHeaders,
  (v) => {
    // 外部重置时（如切换平台 / 应用模板）同步文本
    headersText.value = serializeHeaders(v)
    headersError.value = ''
  },
)

function commitHeaders() {
  const parsed = parseHeaders(headersText.value)
  if (parsed.error) {
    headersError.value = parsed.error
    return
  }
  headersError.value = ''
  emit('update:extraHeaders', parsed.headers)
}

function serializeHeaders(h: Record<string, string>): string {
  return Object.entries(h || {})
    .map(([k, v]) => `${k}: ${v}`)
    .join('\n')
}

function parseHeaders(raw: string): { headers: Record<string, string>; error: string } {
  const result: Record<string, string> = {}
  const lines = raw.split(/\r?\n/).map((l) => l.trim()).filter(Boolean)
  for (const line of lines) {
    const idx = line.indexOf(':')
    if (idx <= 0) {
      return { headers: {}, error: t('admin.channelMonitor.advanced.headersParseError', { line }) }
    }
    const key = line.slice(0, idx).trim()
    const value = line.slice(idx + 1).trim()
    if (!key) {
      return { headers: {}, error: t('admin.channelMonitor.advanced.headersParseError', { line }) }
    }
    result[key] = value
  }
  return { headers: result, error: '' }
}

// ---- Body mode + JSON ----
const bodyText = ref(serializeBody(props.bodyOverride))
const bodyError = ref('')

watch(
  () => props.bodyOverride,
  (v) => {
    bodyText.value = serializeBody(v)
    bodyError.value = ''
  },
)

function commitBody() {
  if (props.bodyOverrideMode === 'off') {
    return
  }
  const trimmed = bodyText.value.trim()
  if (trimmed === '') {
    emit('update:bodyOverride', null)
    bodyError.value = ''
    return
  }
  try {
    const parsed = JSON.parse(trimmed)
    if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
      bodyError.value = t('admin.channelMonitor.advanced.bodyJsonObjectError')
      return
    }
    emit('update:bodyOverride', parsed as Record<string, unknown>)
    bodyError.value = ''
  } catch (e) {
    bodyError.value =
      t('admin.channelMonitor.advanced.bodyJsonError') +
      ': ' +
      (e instanceof Error ? e.message : String(e))
  }
}

function formatBody() {
  const trimmed = bodyText.value.trim()
  if (trimmed === '') return
  try {
    const parsed = JSON.parse(trimmed)
    bodyText.value = JSON.stringify(parsed, null, 2)
    bodyError.value = ''
    // 同步把校验过的对象提交，避免格式化后焦点未移走时父组件读到旧值
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      emit('update:bodyOverride', parsed as Record<string, unknown>)
    }
  } catch (e) {
    bodyError.value =
      t('admin.channelMonitor.advanced.bodyJsonError') +
      ': ' +
      (e instanceof Error ? e.message : String(e))
  }
}

function serializeBody(body: Record<string, unknown> | null): string {
  if (!body || Object.keys(body).length === 0) return ''
  return JSON.stringify(body, null, 2)
}

function updateBodyMode(mode: BodyOverrideMode) {
  emit('update:bodyOverrideMode', mode)
  // 切换到 off 时清掉 body（提示用户）
  if (mode === 'off') {
    emit('update:bodyOverride', null)
  }
}

const bodyModeOptions = computed<{ value: BodyOverrideMode; label: string }[]>(() => [
  { value: 'off', label: t('admin.channelMonitor.advanced.bodyModeOff') },
  { value: 'merge', label: t('admin.channelMonitor.advanced.bodyModeMerge') },
  { value: 'replace', label: t('admin.channelMonitor.advanced.bodyModeReplace') },
])

function bodyModeButtonClass(mode: BodyOverrideMode): string {
  const active = props.bodyOverrideMode === mode
  if (active) {
    return 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-500/15 dark:text-primary-300 dark:border-primary-400'
  }
  return 'border-gray-200 bg-white text-gray-600 hover:border-primary-300 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-400'
}

const bodyModeHint = computed(() => {
  switch (props.bodyOverrideMode) {
    case 'merge':
      return t('admin.channelMonitor.advanced.bodyModeHintMerge')
    case 'replace':
      return t('admin.channelMonitor.advanced.bodyModeHintReplace')
    default:
      return t('admin.channelMonitor.advanced.bodyModeHintOff')
  }
})

const bodyPlaceholder = computed(() => {
  if (props.bodyOverrideMode === 'merge') {
    return '{\n  "system": "You are Claude Code..."\n}'
  }
  return '{\n  "model": "claude-x",\n  "messages": [{"role":"user","content":"hi"}],\n  "max_tokens": 10\n}'
})
</script>
