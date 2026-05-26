<template>
  <div class="border-t border-gray-200 pt-4 dark:border-dark-600">
    <div class="mb-3 flex items-center justify-between">
      <div>
        <label class="input-label mb-0">{{ t('admin.accounts.customErrorCodes') }}</label>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.customErrorCodesHint') }}
        </p>
      </div>
      <button
        type="button"
        @click="emit('update:enabled', !enabled)"
        :class="[
          'relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-primary-500 focus:ring-offset-2',
          enabled ? 'bg-primary-600' : 'bg-gray-200 dark:bg-dark-600'
        ]"
      >
        <span
          :class="[
            'pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out',
            enabled ? 'translate-x-5' : 'translate-x-0'
          ]"
        />
      </button>
    </div>

    <div v-if="enabled" class="space-y-3">
      <div class="rounded-lg bg-amber-50 p-3 dark:bg-amber-900/20">
        <p class="text-xs text-amber-700 dark:text-amber-400">
          <Icon name="exclamationTriangle" size="sm" class="mr-1 inline" :stroke-width="2" />
          {{ t('admin.accounts.customErrorCodesWarning') }}
        </p>
      </div>

      <!-- Error Code Buttons -->
      <div class="flex flex-wrap gap-2">
        <button
          v-for="code in commonErrorCodes"
          :key="code.value"
          type="button"
          @click="toggleErrorCode(code.value)"
          :class="[
            'rounded-lg px-3 py-1.5 text-sm font-medium transition-colors',
            codes.includes(code.value)
              ? 'bg-red-100 text-red-700 ring-1 ring-red-500 dark:bg-red-900/30 dark:text-red-400'
              : 'bg-gray-100 text-gray-600 hover:bg-gray-200 dark:bg-dark-600 dark:text-gray-400 dark:hover:bg-dark-500'
          ]"
        >
          {{ code.value }} {{ code.label }}
        </button>
      </div>

      <!-- Manual input -->
      <div class="flex items-center gap-2">
        <input
          v-model.number="codeInput"
          type="number"
          min="100"
          max="599"
          class="input flex-1"
          :placeholder="t('admin.accounts.enterErrorCode')"
          @keyup.enter="addCustomCode"
        />
        <button type="button" @click="addCustomCode" class="btn btn-secondary px-3">
          <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 4v16m8-8H4" />
          </svg>
        </button>
      </div>

      <!-- Selected codes summary -->
      <div class="flex flex-wrap gap-1.5">
        <span
          v-for="code in sortedCodes"
          :key="code"
          class="inline-flex items-center gap-1 rounded-full bg-red-100 px-2.5 py-0.5 text-sm font-medium text-red-700 dark:bg-red-900/30 dark:text-red-400"
        >
          {{ code }}
          <button
            type="button"
            @click="removeCode(code)"
            class="hover:text-red-900 dark:hover:text-red-300"
          >
            <Icon name="x" size="sm" :stroke-width="2" />
          </button>
        </span>
        <span v-if="codes.length === 0" class="text-xs text-gray-400">
          {{ t('admin.accounts.noneSelectedUsesDefault') }}
        </span>
      </div>
    </div>
  </div>
</template>

<script lang="ts">
const DEFAULT_ERROR_CODES = [
  { value: 401, label: 'Unauthorized' },
  { value: 403, label: 'Forbidden' },
  { value: 429, label: 'Rate Limit' },
  { value: 500, label: 'Server Error' },
  { value: 502, label: 'Bad Gateway' },
  { value: 503, label: 'Unavailable' },
  { value: 529, label: 'Overloaded' }
]
</script>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../Icon.vue'

const props = withDefaults(defineProps<{
  enabled: boolean
  codes: number[]
  /** Available error code presets. Defaults to common HTTP status codes. */
  commonErrorCodes?: Array<{ value: number; label: string }>
  /** Notification callback for errors. Falls back to console.warn. */
  onNotifyError?: (msg: string) => void
  /** Notification callback for info. Falls back to console.info. */
  onNotifyInfo?: (msg: string) => void
}>(), {
  commonErrorCodes: () => DEFAULT_ERROR_CODES,
})

const emit = defineEmits<{
  'update:enabled': [value: boolean]
  'update:codes': [value: number[]]
}>()

const { t } = useI18n()
const codeInput = ref<number | null>(null)

const sortedCodes = computed(() => [...props.codes].sort((a, b) => a - b))

const confirmWarningCode = (code: number): boolean => {
  if (code === 429) return confirm(t('admin.accounts.customErrorCodes429Warning'))
  if (code === 529) return confirm(t('admin.accounts.customErrorCodes529Warning'))
  return true
}

const toggleErrorCode = (code: number) => {
  const idx = props.codes.indexOf(code)
  if (idx === -1) {
    if (!confirmWarningCode(code)) return
    emit('update:codes', [...props.codes, code])
  } else {
    const next = [...props.codes]
    next.splice(idx, 1)
    emit('update:codes', next)
  }
}

const addCustomCode = () => {
  const code = codeInput.value
  if (code === null || code < 100 || code > 599) {
    (props.onNotifyError ?? console.warn)(t('admin.accounts.invalidErrorCode'))
    return
  }
  if (props.codes.includes(code)) {
    (props.onNotifyInfo ?? console.info)(t('admin.accounts.errorCodeExists'))
    return
  }
  if (!confirmWarningCode(code)) return
  emit('update:codes', [...props.codes, code])
  codeInput.value = null
}

const removeCode = (code: number) => {
  const idx = props.codes.indexOf(code)
  if (idx !== -1) {
    const next = [...props.codes]
    next.splice(idx, 1)
    emit('update:codes', next)
  }
}
</script>
