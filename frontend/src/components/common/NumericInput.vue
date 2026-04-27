<template>
  <input
    ref="inputEl"
    type="number"
    :value="displayValue"
    :min="min"
    :max="max"
    :step="step"
    :placeholder="placeholder"
    :disabled="disabled"
    :class="['input', { 'border-red-400 focus:ring-red-500': hasError }]"
    @input="onInput"
    @blur="onBlur"
  />
</template>

<script setup lang="ts">
/**
 * NumericInput — 表单数字输入控件，统一处理：
 * - 空字符串：emit `null`（让校验显示"必填"，而不是被 Number('') === 0 误判为 0）
 * - 整数 / 小数：按 step 控制；step 缺省 1（整数限额）
 * - 负数：依赖 min（默认 0）拒绝
 * - 失焦时自动 clamp 到 [min, max]
 *
 * 不直接走 v-model.number：v-model.number 在空字符串时返回空字符串 / NaN，
 * 难以与 limit_value: number | null 的语义对齐。这里走 emit 'update:modelValue'，
 * value 与父组件的 number | null 精确同步。
 *
 * hasError 由父组件控制（通常基于校验结果），用于切换红色边框。
 */
import { computed, ref } from 'vue'

const props = withDefaults(
  defineProps<{
    modelValue: number | null | undefined
    min?: number
    max?: number
    step?: number
    placeholder?: string
    disabled?: boolean
    hasError?: boolean
  }>(),
  {
    min: 0,
    step: 1,
    hasError: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: number | null): void
  (e: 'blur'): void
}>()

const inputEl = ref<HTMLInputElement | null>(null)

// displayValue 把 null/undefined 显示为空字符串（让 input 显示空白），
// 数值正常显示。避免 v-bind:value 把 null 转成 "null" 字面量。
const displayValue = computed<string | number>(() => {
  if (props.modelValue === null || props.modelValue === undefined) return ''
  return props.modelValue
})

function onInput(event: Event): void {
  const raw = (event.target as HTMLInputElement).value
  if (raw === '') {
    emit('update:modelValue', null)
    return
  }
  const num = Number(raw)
  if (Number.isNaN(num)) {
    emit('update:modelValue', null)
    return
  }
  emit('update:modelValue', num)
}

function onBlur(): void {
  // 失焦后做一次 clamp（仅当当前值在 min/max 之外时生效）；
  // 不在 input 中 clamp，是因为输入过程中可能临时越界（如先输 0.5 再补成 50）
  const v = props.modelValue
  if (v !== null && v !== undefined) {
    if (props.min !== undefined && v < props.min) {
      emit('update:modelValue', props.min)
    } else if (props.max !== undefined && v > props.max) {
      emit('update:modelValue', props.max)
    }
  }
  emit('blur')
}
</script>
