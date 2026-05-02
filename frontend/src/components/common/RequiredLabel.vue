<template>
  <span class="input-label">
    {{ label }}
    <span v-if="required" class="ml-0.5 text-red-500" aria-hidden="true">*</span>
  </span>
</template>

<script setup lang="ts">
/**
 * RequiredLabel — 表单字段 label，required=true 时在文字后追加红色 *。
 *
 * 复用 .input-label 全局样式，让所有表单 label 视觉一致；调用方只需关心
 * "这个字段是不是必填"，不用各自手写 span 红星。
 */
withDefaults(
  defineProps<{
    label: string
    required?: boolean
  }>(),
  { required: false },
)
</script>
