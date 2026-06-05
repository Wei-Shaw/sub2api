<template>
  <!-- 全部维度都未限定 → 单条 "全部请求" 文案 -->
  <div v-if="isAllStar" class="text-xs text-gray-400">
    {{ t('admin.serviceQuota.scopeDetails.allRequests') }}
  </div>
  <!-- 平台 chip + chevron 链：showInternal 决定是否展示 channel/group/account 段。
       admin 视角全 5 段；user 视角只保留 platform + model（隐藏内部资源拓扑）。
       整链文字（chevron / 名称 / model_pattern）取平台主色，扫一眼能识别归属平台；
       缺失维度统一灰色 *。 -->
  <div v-else class="flex flex-wrap items-center gap-1 text-xs">
    <template v-for="(seg, idx) in renderedSegments" :key="idx">
      <Icon
        v-if="idx > 0"
        name="chevronRight"
        size="xs"
        :class="platformColor || 'text-gray-300 dark:text-gray-600'"
      />
      <span
        v-if="seg.kind === 'platform' && seg.text"
        :class="['inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 font-medium', platformTextClass(seg.text)]"
      >
        <PlatformIcon :platform="seg.text as GroupPlatform" size="xs" />
        <span>{{ formatPlatformLabel(seg.text) }}</span>
      </span>
      <span
        v-else-if="seg.text"
        :class="['inline-block max-w-[8rem] truncate font-mono text-[11px] align-middle', platformColor || 'text-gray-500 dark:text-gray-400']"
        :title="seg.text"
      >
        {{ seg.text }}
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
import { useEntityName } from './entityNames'

interface RenderedSegment {
  kind: 'platform' | 'channel' | 'group' | 'account' | 'model'
  /** 渲染文本：空字符串走灰色 * 占位分支。 */
  text: string
}

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

// 平台主色：用于 chevron 箭头与 channel/group/account/model 文字。
// 平台缺失（混合规则）则不上色，沿用默认灰色，避免误导用户。
const platformColor = computed<string>(() =>
  props.summary?.platform ? platformTextClass(props.summary.platform) : ''
)

// 5 段链：platform | channel | group | account | model_pattern
// channel/group/account 通过 useEntityName 异步解析为名称（首屏先落 ch#id 占位）。
// user 视角下过滤掉 channel/group/account 段（避免泄露内部资源拓扑），
// 仅保留 platform + model_pattern；admin 视角全 5 段。
const renderedSegments = computed<RenderedSegment[]>(() => {
  const platform = props.summary?.platform || ''
  const tail = pathTailSegments(props.summary).map((seg): RenderedSegment => {
    if (seg.kind === 'model') {
      return { kind: 'model', text: seg.literal }
    }
    if (seg.entityId == null) {
      return { kind: seg.kind, text: '' }
    }
    // 读取 ref.value 让 computed 追踪：名称解析回填后会自动重新渲染。
    const r = useEntityName(seg.kind, seg.entityId)
    return { kind: seg.kind, text: r.value }
  })
  const filteredTail = props.showInternal
    ? tail
    : tail.filter((s) => s.kind === 'model')
  return [{ kind: 'platform', text: platform }, ...filteredTail]
})
</script>
