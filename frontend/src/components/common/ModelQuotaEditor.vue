<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { ModelRateLimit, ModelRateLimits } from '@/types'

const props = defineProps<{
  modelValue: ModelRateLimits | null | undefined
  /** placeholder text for the pattern input */
  patternPlaceholder?: string
  /** caption shown when list is empty */
  emptyHint?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: ModelRateLimits): void
}>()

const { t } = useI18n()

const rules = computed<ModelRateLimit[]>(() => props.modelValue ?? [])

function update(next: ModelRateLimit[]) {
  emit('update:modelValue', next)
}

function addRule() {
  update([...rules.value, { pattern: '', limit_5h: 0, limit_1d: 0, limit_7d: 0 }])
}

function removeRule(idx: number) {
  const next = rules.value.slice()
  next.splice(idx, 1)
  update(next)
}

function patchRule(idx: number, patch: Partial<ModelRateLimit>) {
  const next = rules.value.slice()
  next[idx] = { ...next[idx], ...patch }
  update(next)
}

function numberOrZero(v: unknown): number {
  if (v === '' || v === null || v === undefined) return 0
  const n = Number(v)
  return Number.isFinite(n) && n > 0 ? n : 0
}
</script>

<template>
  <div class="space-y-3">
    <p v-if="rules.length === 0" class="text-xs text-gray-500 dark:text-gray-400">
      {{ emptyHint ?? t('modelQuota.emptyHint') }}
    </p>

    <div
      v-for="(rule, idx) in rules"
      :key="idx"
      class="rounded-lg border border-gray-200 p-3 dark:border-dark-600"
    >
      <div class="flex items-start gap-3">
        <div class="flex-1 space-y-2">
          <div>
            <label class="input-label text-xs">{{ t('modelQuota.pattern') }}</label>
            <input
              :value="rule.pattern"
              @input="(e) => patchRule(idx, { pattern: (e.target as HTMLInputElement).value })"
              type="text"
              class="input text-sm"
              :placeholder="patternPlaceholder ?? t('modelQuota.patternPlaceholder')"
            />
          </div>

          <div class="grid grid-cols-3 gap-2">
            <div>
              <label class="input-label text-xs">{{ t('modelQuota.limit5h') }}</label>
              <input
                :value="rule.limit_5h || ''"
                @input="(e) => patchRule(idx, { limit_5h: numberOrZero((e.target as HTMLInputElement).value) })"
                type="number"
                min="0"
                step="0.01"
                class="input text-sm"
                placeholder="0"
              />
            </div>
            <div>
              <label class="input-label text-xs">{{ t('modelQuota.limit1d') }}</label>
              <input
                :value="rule.limit_1d || ''"
                @input="(e) => patchRule(idx, { limit_1d: numberOrZero((e.target as HTMLInputElement).value) })"
                type="number"
                min="0"
                step="0.01"
                class="input text-sm"
                placeholder="0"
              />
            </div>
            <div>
              <label class="input-label text-xs">{{ t('modelQuota.limit7d') }}</label>
              <input
                :value="rule.limit_7d || ''"
                @input="(e) => patchRule(idx, { limit_7d: numberOrZero((e.target as HTMLInputElement).value) })"
                type="number"
                min="0"
                step="0.01"
                class="input text-sm"
                placeholder="0"
              />
            </div>
          </div>
        </div>
        <button
          type="button"
          @click="removeRule(idx)"
          class="mt-5 rounded-md p-1.5 text-gray-400 hover:bg-gray-100 hover:text-red-500 dark:hover:bg-dark-700"
          :title="t('modelQuota.remove')"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>
    </div>

    <button
      type="button"
      @click="addRule"
      class="inline-flex items-center gap-1.5 rounded-md border border-dashed border-gray-300 px-3 py-1.5 text-sm text-gray-600 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:text-gray-300 dark:hover:border-primary-400"
    >
      <Icon name="plus" size="sm" />
      <span>{{ t('modelQuota.addRule') }}</span>
    </button>
  </div>
</template>
