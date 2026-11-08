<template>
  <section
    class="space-y-4 border-t border-gray-200 pt-4 dark:border-dark-600"
    data-testid="intelligence-check-settings"
  >
    <div class="flex items-start justify-between gap-4">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">
          {{ t('admin.accounts.intelligenceCheck.title') }}
        </h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          {{ t('admin.accounts.intelligenceCheck.description') }}
        </p>
      </div>
      <Toggle
        :model-value="modelValue.enabled"
        data-testid="intelligence-check-enabled"
        :aria-label="t('admin.accounts.intelligenceCheck.title')"
        @update:model-value="handleEnabledChange"
      />
    </div>

    <template v-if="modelValue.enabled">
      <div>
        <label class="input-label" for="intelligence-check-interval">
          {{ t('admin.accounts.intelligenceCheck.intervalLabel') }}
        </label>
        <input
          id="intelligence-check-interval"
          :value="intervalInput"
          type="number"
          :min="MIN_INTERVAL_MINUTES"
          :max="MAX_INTERVAL_MINUTES"
          class="input"
          :placeholder="t('admin.accounts.intelligenceCheck.intervalPlaceholder')"
          data-testid="intelligence-check-interval"
          @input="handleIntervalInput"
        />
        <p class="input-hint">
          {{ t('admin.accounts.intelligenceCheck.intervalHint', { min: MIN_INTERVAL_MINUTES, max: MAX_INTERVAL_MINUTES }) }}
        </p>
      </div>

      <div>
        <label class="input-label" for="intelligence-check-model">
          {{ t('admin.accounts.intelligenceCheck.modelLabel') }}
        </label>
        <input
          id="intelligence-check-model"
          :value="modelValue.modelId"
          type="text"
          class="input"
          :placeholder="t('admin.accounts.intelligenceCheck.modelPlaceholder')"
          data-testid="intelligence-check-model"
          @input="handleModelInput"
        />
        <p class="input-hint">{{ t('admin.accounts.intelligenceCheck.modelHint') }}</p>
      </div>

      <div>
        <label class="input-label" for="intelligence-check-reasoning-effort">
          {{ t('admin.accounts.intelligenceCheck.reasoningEffortLabel') }}
        </label>
        <input
          id="intelligence-check-reasoning-effort"
          :value="modelValue.reasoningEffort"
          type="text"
          list="intelligence-check-reasoning-effort-options"
          class="input"
          :placeholder="t('admin.accounts.intelligenceCheck.reasoningEffortPlaceholder')"
          data-testid="intelligence-check-reasoning-effort"
          @input="handleReasoningEffortInput"
        />
        <datalist id="intelligence-check-reasoning-effort-options">
          <option v-for="option in REASONING_EFFORT_OPTIONS" :key="option" :value="option" />
        </datalist>
        <p class="input-hint">{{ t('admin.accounts.intelligenceCheck.reasoningEffortHint') }}</p>
      </div>
    </template>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import type { IntelligenceCheckAccountConfig } from '@/types'

// 与后端 service.IntelligenceCheckMinIntervalMinutes / MaxIntervalMinutes 保持一致。
const MIN_INTERVAL_MINUTES = 5
const MAX_INTERVAL_MINUTES = 7 * 24 * 60

// 思考强度的常见取值，仅作为输入建议：上游各家取值不完全一致，
// 这里不限制输入，填了啥就原样透传给上游。
// 注意 gpt-6-astra 明确不支持 none（会返回 HTTP 400），minimal 也仅限早期 GPT-5 系列，
// 所以不把它们放进建议列表，避免误导。
const REASONING_EFFORT_OPTIONS = ['low', 'medium', 'high', 'xhigh', 'max']

const props = defineProps<{ modelValue: IntelligenceCheckAccountConfig }>()
const emit = defineEmits<{ 'update:modelValue': [value: IntelligenceCheckAccountConfig] }>()
const { t } = useI18n()

const intervalInput = computed(() => props.modelValue.intervalMinutes ?? '')

const patch = (changes: Partial<IntelligenceCheckAccountConfig>) => {
  emit('update:modelValue', { ...props.modelValue, ...changes })
}

const handleEnabledChange = (enabled: boolean) => {
  patch({ enabled })
}

// 留空即「跟随全局设置」，由提交侧删除对应的 extra 键来表达。
const handleIntervalInput = (event: Event) => {
  const raw = (event.target as HTMLInputElement).value.trim()
  if (!raw) {
    patch({ intervalMinutes: null })
    return
  }
  const parsed = Number(raw)
  if (!Number.isFinite(parsed) || parsed <= 0) {
    patch({ intervalMinutes: null })
    return
  }
  patch({ intervalMinutes: Math.trunc(parsed) })
}

const handleModelInput = (event: Event) => {
  patch({ modelId: (event.target as HTMLInputElement).value })
}

// 同样留空即「跟随全局设置」，由提交侧删除对应的 extra 键来表达。
const handleReasoningEffortInput = (event: Event) => {
  patch({ reasoningEffort: (event.target as HTMLInputElement).value })
}
</script>
