<template>
  <!-- 全部维度都未限定 → 单条 "全部请求" 文案 -->
  <div v-if="isAllStar" class="text-xs text-gray-400">
    {{ t('admin.serviceQuota.scopeDetails.allRequests') }}
  </div>
  <!-- showInternal=false 用户视角 → 仅展示平台 chip（隐藏 channel/group/account 内部拓扑） -->
  <div v-else-if="!showInternal" class="flex flex-wrap items-center gap-1 text-xs">
    <span
      v-if="summary?.platform"
      :class="['inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-medium', platformTextClass(summary.platform)]"
    >
      <PlatformIcon :platform="summary.platform as GroupPlatform" size="xs" />
      <span>{{ formatPlatformLabel(summary.platform) }}</span>
    </span>
    <span v-else class="text-gray-400">*</span>
  </div>
  <!-- admin 视角 → 平台 chip + chevron 链：channel/group/account/model_pattern 缺则灰色 *。 -->
  <div v-else class="flex flex-wrap items-center gap-1 text-xs">
    <template v-for="(seg, idx) in segments" :key="idx">
      <Icon v-if="idx > 0" name="chevronRight" size="xs" class="text-gray-300 dark:text-gray-600" />
      <span
        v-if="seg.kind === 'platform' && seg.value"
        :class="['inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-medium', platformTextClass(seg.value)]"
      >
        <PlatformIcon :platform="seg.value as GroupPlatform" size="xs" />
        <span>{{ formatPlatformLabel(seg.value) }}</span>
      </span>
      <span
        v-else-if="seg.value"
        class="font-mono text-[11px] text-gray-500 dark:text-gray-400"
      >
        {{ seg.value }}
      </span>
      <span v-else class="text-gray-400">*</span>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { platformTextClass } from '@/utils/platformColors'
import type { GroupPlatform } from '@/types'
import {
  isPathAllStar,
  pathTailSegments,
  formatPlatformLabel,
  type PathSummary,
} from './pathRender'

const props = withDefaults(
  defineProps<{
    /** 后端返回的 path_summary；nil 等价于"无限制"，与"全部 nil"行为一致 */
    summary?: PathSummary | null
    /** showInternal=false 表示用户视角，只显示平台 chip 不暴露内部拓扑 */
    showInternal?: boolean
  }>(),
  { showInternal: true }
)

const { t } = useI18n()

const isAllStar = computed<boolean>(() => isPathAllStar(props.summary))

// 5 段链：platform | channel | group | account | model_pattern
const segments = computed(() => {
  const s = props.summary
  return [
    { kind: 'platform' as const, value: s?.platform || '' },
    ...pathTailSegments(s),
  ]
})
</script>
