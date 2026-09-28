<template>
  <div class="space-y-5">
    <!-- 页头(独立形态下展示标题;后台形态 AppHeader 已有页面标题) -->
    <div v-if="!embedded">
      <h1 class="text-2xl font-bold tracking-tight text-gray-900 dark:text-white sm:text-3xl">{{ t('modelPlaza.title') }}</h1>
      <p class="mt-1.5 text-sm text-gray-500 dark:text-dark-400">{{ t('modelPlaza.description') }}</p>
    </div>

    <!-- 全局价格说明(管理员配置,Markdown) -->
    <div
      v-if="descriptionHtml"
      class="plaza-description rounded-2xl border border-gray-100 bg-white px-5 py-4 text-sm shadow-card dark:border-dark-700/50 dark:bg-dark-800/50"
      v-html="descriptionHtml"
    ></div>

    <!-- 加载/错误/空 -->
    <div v-if="loading" class="flex min-h-[240px] items-center justify-center">
      <div class="h-8 w-8 animate-spin rounded-full border-2 border-primary-600/25 border-t-primary-600 dark:border-primary-400/25 dark:border-t-primary-400"></div>
    </div>
    <div
      v-else-if="error"
      class="rounded-2xl border border-red-200 bg-red-50 px-5 py-8 text-center text-sm text-red-600 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
    >
      {{ t('modelPlaza.loadFailed') }}
    </div>
    <template v-else>
      <div class="rounded-2xl border border-gray-100 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-800/60">
        <PlazaFilterBar
          :platforms="platforms"
          :groups="groupOptions"
          :rates="rates"
          :platform="selectedPlatform"
          :group-id="selectedGroupId"
          :rate="selectedRate"
          :search="searchQuery"
          :show-rate="true"
          @update:platform="selectedPlatform = $event"
          @update:group-id="selectedGroupId = $event"
          @update:rate="selectedRate = $event"
          @update:search="searchQuery = $event"
        />
      </div>

      <div v-if="filteredModels.length > 0" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
        <ModelMetadataCard
          v-for="model in filteredModels"
          :key="model.name"
          :model="model"
        />
      </div>
      <div
        v-else
        class="rounded-2xl border border-dashed border-gray-300 px-5 py-12 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400"
      >
        {{ searchActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import ModelMetadataCard from './ModelMetadataCard.vue'
import PlazaFilterBar from './PlazaFilterBar.vue'
import type { ModelPlazaGroup, ModelPlazaResponse, PlazaModel } from '@/api/modelPlaza'

const props = withDefaults(defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  /** 后台内嵌形态(AppLayout 内):隐藏页头。 */
  embedded?: boolean
  /** 隐藏客户不需要感知的倍率、订阅和高峰计费细节，价格仍按倍率计算。 */
  showRateDetails?: boolean
}>(), {
  showRateDetails: true
})

const { t } = useI18n()
const selectedPlatform = ref<string>('all')
const selectedGroupId = ref<number | 'all'>('all')
const selectedRate = ref<number | 'all'>('all')
const searchQuery = ref('')

const searchActive = computed(() => {
  return searchQuery.value.trim() !== '' ||
    selectedPlatform.value !== 'all' ||
    selectedGroupId.value !== 'all' ||
    selectedRate.value !== 'all'
})

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  return DOMPurify.sanitize(marked.parse(md) as string)
})

/** 生效倍率：登录用户的专属倍率优先于分组默认倍率。 */
function effectiveRate(group: ModelPlazaGroup): number {
  return group.user_rate_multiplier ?? group.rate_multiplier
}

const platforms = computed(() => {
  const values = new Set((props.response?.groups ?? []).map((group) => group.platform).filter(Boolean))
  // 平台筛选项固定展示，避免某个平台暂时没有已授权模型时整项消失。
  // 有数据的平台可正常点击；暂无数据的平台由筛选栏置灰。
  const preferredOrder = ['anthropic', 'deepseek', 'zhipu', 'openai']
  const excludedPlatforms = new Set(['gemini', 'grok'])
  return [
    ...preferredOrder,
    ...[...values]
      .filter((platform) => !preferredOrder.includes(platform) && !excludedPlatforms.has(platform))
      .sort()
  ]
})

const groupOptions = computed(() =>
  (props.response?.groups ?? []).map((group) => ({
    id: group.id,
    name: group.name,
    platform: group.platform,
    rate: effectiveRate(group)
  }))
)

const rates = computed(() =>
  [...new Set((props.response?.groups ?? []).map(effectiveRate))].sort((a, b) => a - b)
)

type ModelEntry = { model: PlazaModel; groupIds: Set<number> }

/** 同名模型只展示一张卡，但保留它所属的全部分组用于筛选。 */
const modelEntries = computed<ModelEntry[]>(() => {
  const byName = new Map<string, ModelEntry>()
  for (const group of props.response?.groups ?? []) {
    for (const model of group.models) {
      const key = model.name.toLowerCase()
      const existing = byName.get(key)
      if (existing) {
        existing.groupIds.add(group.id)
      } else {
        // 元数据还未补齐展示名时，使用调用名作为兜底，不能因此隐藏模型。
        const displayModel = model.display_name?.trim()
          ? model
          : { ...model, display_name: model.name }
        byName.set(key, { model: displayModel, groupIds: new Set([group.id]) })
      }
    }
  }
  return [...byName.values()].sort(
    (a, b) => b.model.launch_date.localeCompare(a.model.launch_date) || a.model.name.localeCompare(b.model.name)
  )
})

const matchingGroupIds = computed(() => {
  const groups = (props.response?.groups ?? []).filter((group) => {
    if (selectedPlatform.value !== 'all' && group.platform !== selectedPlatform.value) return false
    if (selectedGroupId.value !== 'all' && group.id !== selectedGroupId.value) return false
    if (selectedRate.value !== 'all' && effectiveRate(group) !== selectedRate.value) return false
    return true
  })
  return new Set(groups.map((group) => group.id))
})

const filteredModels = computed(() => {
  let entries = modelEntries.value.filter((entry) =>
    [...entry.groupIds].some((groupId) => matchingGroupIds.value.has(groupId))
  )
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    entries = entries.filter(({ model }) =>
      `${model.name} ${model.display_name}`.toLowerCase().includes(q)
    )
  }
  return entries.map((entry) => entry.model)
})

</script>

<style scoped>
.plaza-description {
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  @apply mb-2 mt-3 font-semibold text-gray-900 first:mt-0 dark:text-white;
}

.plaza-description :deep(p) {
  @apply mb-2 text-gray-700 last:mb-0 dark:text-dark-200;
}

.plaza-description :deep(a) {
  @apply text-primary-600 underline underline-offset-4 hover:text-primary-700 dark:text-primary-300;
}

.plaza-description :deep(ul) {
  @apply mb-2 list-disc pl-5;
}

.plaza-description :deep(ol) {
  @apply mb-2 list-decimal pl-5;
}

.plaza-description :deep(li) {
  @apply mb-0.5 text-gray-700 dark:text-dark-200;
}

.plaza-description :deep(code) {
  @apply rounded bg-gray-100 px-1.5 py-0.5 font-mono text-xs dark:bg-dark-800;
}

.plaza-description :deep(blockquote) {
  @apply my-2 border-l-4 border-gray-300 pl-3 text-gray-600 dark:border-dark-600 dark:text-dark-300;
}
</style>
