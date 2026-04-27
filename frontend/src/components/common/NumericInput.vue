<template>
  <input
    ref="inputEl"
    type="number"
    :value="displayValue"
    :min="min"
    :max="max"
    :step="effectiveStep"
    :inputmode="integer ? 'numeric' : 'decimal'"
    :placeholder="placeholder"
    :disabled="disabled"
    :class="['input', { 'border-red-400 focus:ring-red-500': hasError }]"
    @input="onInput"
    @keydown="onKeydown"
    @blur="onBlur"
  />
</template>

<script setup lang="ts">
/**
 * NumericInput — 表单数字输入控件，统一处理：
 * - 空字符串：emit `null`（让校验显示"必填"，而不是被 Number('') === 0 误判为 0）
 * - integer=true 时禁止小数点 + 解析走 parseInt + 失焦强制 Math.round，避免
 *   IEEE754 浮点累积误差。
 *   历史背景：旧版 LimiterEditor 用 `step="0.000001"`，浏览器把整数 1000000
 *   量化到该 step 网格时 IEEE754 误差让值变成 999999.999997 写入 DB。
 * - integer=false（默认）：尊重 step prop，允许小数（适合 daily_usd 金额）
 * - 负数：依赖 min（默认 0）拒绝
 * - 失焦时自动 clamp 到 [min, max]
 *
 * 不直接走 v-model.number：v-model.number 在空字符串时返回空字符串 / NaN,
 * 难以与 limit_value: number | null 的语义对齐。这里走 emit 'update:modelValue',
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
    /**
     * true：仅允许整数输入（拦截 . / e / + / -），解析走 parseInt，blur 强制取整。
     * false（默认）：允许小数；step 由调用方传入控制粒度。
     *
     * 用 prop 显式声明而不是看 step 是否 ≥ 1，是因为 step=0.01 也可能是
     * "金额最小单位"语义而非"必须是整数倍"，意图区分由调用方决定。
     */
    integer?: boolean
    placeholder?: string
    disabled?: boolean
    hasError?: boolean
  }>(),
  {
    min: 0,
    step: 1,
    integer: false,
    hasError: false,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: number | null): void
  (e: 'blur'): void
}>()

const inputEl = ref<HTMLInputElement | null>(null)

// integer=true 时强制 step=1，覆盖父组件可能传入的小数 step；
// 否则尊重父组件传入的 step。
const effectiveStep = computed<number>(() => (props.integer ? 1 : props.step))

// displayValue 把 null/undefined 显示为空字符串（让 input 显示空白），
// 数值正常显示。避免 v-bind:value 把 null 转成 "null" 字面量。
const displayValue = computed<string | number>(() => {
  if (props.modelValue === null || props.modelValue === undefined) return ''
  return props.modelValue
})

// integer 模式下拦截会引入小数 / 科学计数法的字符；
// 允许：0-9 + 编辑键（Backspace/Delete/Tab/方向键 + Ctrl+A/C/V/X/Z 等）
function onKeydown(event: KeyboardEvent): void {
  if (!props.integer) return
  // 修饰键组合（复制粘贴 / 剪切 / 撤销 / 全选）放行
  if (event.ctrlKey || event.metaKey || event.altKey) return
  // 控制键 / 功能键 / 方向键 / 编辑键放行（key 长度 > 1 表示命名键如 'Backspace'）
  if (event.key.length > 1) return
  // 仅允许数字 0-9
  if (event.key < '0' || event.key > '9') {
    event.preventDefault()
  }
}

function onInput(event: Event): void {
  const raw = (event.target as HTMLInputElement).value
  if (raw === '') {
    emit('update:modelValue', null)
    return
  }
  // integer 模式：parseInt 截断小数 / 科学计数法（如粘贴入 "1e6" → 1）
  // 普通模式：Number 保留小数
  const num = props.integer ? parseInt(raw, 10) : Number(raw)
  if (Number.isNaN(num)) {
    emit('update:modelValue', null)
    return
  }
  emit('update:modelValue', num)
}

function onBlur(): void {
  const v = props.modelValue
  if (v !== null && v !== undefined) {
    // integer 模式：失焦时强制取整（防御外部传入的非整数值，例如老数据
    // 999999.999997 在编辑框出现时，blur 时归位到 1000000）
    if (props.integer && !Number.isInteger(v)) {
      emit('update:modelValue', Math.round(v))
      emit('blur')
      return
    }
    // 失焦后做一次 clamp（仅当当前值在 min/max 之外时生效）；
    // 不在 input 中 clamp，是因为输入过程中可能临时越界（如先输 0.5 再补成 50）
    if (props.min !== undefined && v < props.min) {
      emit('update:modelValue', props.min)
    } else if (props.max !== undefined && v > props.max) {
      emit('update:modelValue', props.max)
    }
  }
  emit('blur')
}
</script>
