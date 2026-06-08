<template>
  <section class="space-y-2">
    <button
      type="button"
      class="flex w-full items-baseline justify-between text-left hover:opacity-80"
      @click="emit('update:collapsed', !collapsed)"
    >
      <span class="input-label flex items-center gap-1">
        <svg
          class="h-3 w-3 shrink-0 transition-transform"
          :class="{ '-rotate-90': collapsed }"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
        </svg>
        {{ title }}
        <span v-if="collapsed && typeof count === 'number'" class="text-xs text-gray-400">({{ count }})</span>
      </span>
      <span v-if="hint" class="text-xs text-gray-500 dark:text-gray-400">{{ hint }}</span>
    </button>
    <div v-show="!collapsed">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
/**
 * 可折叠区域：复用 ConfigView 中"chevron + 标题 + 折叠时显示数量 + 右侧 hint"的样式块。
 *
 * 设计原则：
 * - 受控组件：collapsed 由父级管理，便于多个 section 各自记忆状态
 * - count 仅在 collapsed=true 时显示，提示展开后能看到几条
 * - 内容用 v-show（非 v-if），避免每次折叠/展开重新挂载子表单丢状态
 */
defineProps<{
  title: string
  hint?: string
  count?: number
  collapsed: boolean
}>()

const emit = defineEmits<{
  (e: 'update:collapsed', value: boolean): void
}>()
</script>
