<template>
  <span :class="['inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium', cls]">
    <span class="mr-1.5 h-1.5 w-1.5 rounded-full" :class="dotCls" />
    {{ t(`support.statusLabel.${status}`) }}
  </span>
</template>

<script setup lang="ts">
/**
 * SupportStatusBadge —— 工单状态彩色 badge。
 *
 * 颜色与含义：
 *   - open(待处理)        蓝色：用户刚提交、客服尚未介入
 *   - in_progress(处理中) 黄色：客服已经回复或修改过状态
 *   - closed(已关闭)      灰色：用户主动关闭或客服 PATCH 关闭
 */
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { TicketStatus } from '@/api/support'

const props = defineProps<{ status: TicketStatus }>()
const { t } = useI18n()

const cls = computed(() => {
  switch (props.status) {
    case 'open':
      return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300'
    case 'in_progress':
      return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300'
    case 'closed':
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
  }
})

const dotCls = computed(() => {
  switch (props.status) {
    case 'open':
      return 'bg-blue-500'
    case 'in_progress':
      return 'bg-amber-500'
    case 'closed':
    default:
      return 'bg-gray-400'
  }
})
</script>
