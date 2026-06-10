<template>
  <span :class="['inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium', cls]">
    {{ t(`support.priorityLabel.${priority}`) }}
  </span>
</template>

<script setup lang="ts">
/**
 * SupportPriorityBadge —— 工单优先级彩色 badge。
 *
 * 颜色与含义：
 *   - low(低)     灰色：默认或客服降级
 *   - normal(普通) 蓝色：默认 priority（与 settings.default_priority 对齐）
 *   - high(高)    红色：紧急工单，admin 列表会被排到顶（service 强制 priority CASE-DESC）
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TicketPriority } from '@/api/support'

const props = defineProps<{ priority: TicketPriority }>()
const { t } = useI18n()

const cls = computed(() => {
  switch (props.priority) {
    case 'high':
      return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300'
    case 'normal':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
    case 'low':
    default:
      return 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-gray-400'
  }
})
</script>
