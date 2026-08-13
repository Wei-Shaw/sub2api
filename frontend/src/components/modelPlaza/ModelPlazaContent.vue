<template>
  <div class="space-y-6" data-testid="model-plaza-content">
    <!-- 页头(独立形态下展示标题;后台形态 AppHeader 已有页面标题) -->
    <div v-if="!embedded">
      <h1 class="text-2xl font-semibold text-ink">{{ t('modelPlaza.title') }}</h1>
      <p class="mt-2 max-w-2xl text-sm text-ink-secondary">{{ t('modelPlaza.description') }}</p>
    </div>

    <!-- 全局价格说明(管理员配置,Markdown) -->
    <div
      v-if="descriptionHtml"
      class="plaza-description rounded border border-line bg-surface px-4 py-3 text-sm text-ink-secondary"
      data-testid="model-plaza-description"
      v-html="descriptionHtml"
    ></div>

    <!-- 未登录提示 -->
    <p
      v-if="!isAuthenticated"
      class="flex items-center gap-1.5 text-xs text-ink-tertiary"
      data-testid="model-plaza-anonymous-hint"
    >
      <Icon name="infoCircle" size="xs" class="h-3.5 w-3.5 shrink-0" />
      {{ t('modelPlaza.anonymousHint') }}
    </p>

    <!-- Loading — flat hairline panels, not a spinning ring. -->
    <div v-if="loading" class="space-y-4" data-testid="model-plaza-loading">
      <div v-for="n in 2" :key="n" class="rounded border border-line bg-surface">
        <div class="border-b border-line px-4 py-3">
          <div class="skeleton h-3 w-32"></div>
        </div>
        <div class="space-y-3 p-4">
          <div class="skeleton h-3 w-full"></div>
          <div class="skeleton h-3 w-4/5"></div>
          <div class="skeleton h-3 w-2/3"></div>
        </div>
      </div>
    </div>
    <p
      v-else-if="error"
      class="rounded border border-danger/40 bg-danger-tint px-4 py-6 text-center text-sm text-danger"
      data-testid="model-plaza-error"
    >
      {{ t('modelPlaza.loadFailed') }}
    </p>
    <template v-else>
      <!-- 筛选区:平台 → 分组 → 倍率 → 模型名 -->
      <PlazaFilterBar
        :platforms="platforms"
        :groups="groupOptions"
        :rates="rates"
        :platform="selectedPlatform"
        :group-id="selectedGroupId"
        :rate="selectedRate"
        :search="searchQuery"
        @update:platform="selectedPlatform = $event"
        @update:group-id="selectedGroupId = $event"
        @update:rate="selectedRate = $event"
        @update:search="searchQuery = $event"
      />

      <!-- 分组分节的模型清单(默认按生效倍率升序) -->
      <div v-if="filteredGroups.length > 0" class="space-y-4" data-testid="model-plaza-groups">
        <PlazaGroupSection v-for="g in filteredGroups" :key="g.id" :group="g" />
      </div>
      <p
        v-else
        class="rounded border border-line px-4 py-12 text-center text-xs text-ink-tertiary"
        data-testid="model-plaza-empty"
      >
        {{ searchActive ? t('modelPlaza.noSearchResult') : t('modelPlaza.empty') }}
      </p>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import Icon from '@/components/icons/Icon.vue'
import PlazaFilterBar from './PlazaFilterBar.vue'
import PlazaGroupSection from './PlazaGroupSection.vue'
import type { ModelPlazaGroup, ModelPlazaResponse } from '@/api/modelPlaza'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  response: ModelPlazaResponse | null
  loading: boolean
  error?: boolean
  /** 后台内嵌形态(AppLayout 内):隐藏页头。 */
  embedded?: boolean
}>()

const { t } = useI18n()
const authStore = useAuthStore()
const isAuthenticated = computed(() => authStore.isAuthenticated)

const selectedPlatform = ref<string>('all')
const selectedGroupId = ref<number | 'all'>('all')
const selectedRate = ref<number | 'all'>('all')
const searchQuery = ref('')

const searchActive = computed(() => searchQuery.value.trim() !== '')

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  return DOMPurify.sanitize(marked.parse(md) as string)
})

/** 生效倍率 = 用户专属倍率 ?? 分组默认倍率。 */
function effectiveRate(g: ModelPlazaGroup): number {
  return g.user_rate_multiplier ?? g.rate_multiplier
}

const platforms = computed(() =>
  [...new Set((props.response?.groups ?? []).map((g) => g.platform).filter(Boolean))].sort()
)

const groupOptions = computed(() =>
  (props.response?.groups ?? []).map((g) => ({
    id: g.id,
    name: g.name,
    platform: g.platform,
    rate: effectiveRate(g)
  }))
)

/** 全量生效倍率;当前组合下不可用的项由 FilterBar 置灰而非隐藏。 */
const rates = computed(() =>
  [...new Set((props.response?.groups ?? []).map(effectiveRate))].sort((a, b) => a - b)
)

/** 数据刷新后选中的倍率可能不复存在,重置为全部。 */
watch(rates, (list) => {
  if (selectedRate.value !== 'all' && !list.includes(selectedRate.value)) {
    selectedRate.value = 'all'
  }
})

const filteredGroups = computed(() => {
  let groups = props.response?.groups ?? []
  if (selectedPlatform.value !== 'all') {
    groups = groups.filter((g) => g.platform === selectedPlatform.value)
  }
  if (selectedGroupId.value !== 'all') {
    groups = groups.filter((g) => g.id === selectedGroupId.value)
  }
  if (selectedRate.value !== 'all') {
    groups = groups.filter((g) => effectiveRate(g) === selectedRate.value)
  }
  // 模型名搜索:分组内只留命中的模型,整组无命中则隐藏该分组。
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    groups = groups
      .map((g) => ({ ...g, models: g.models.filter((m) => m.name.toLowerCase().includes(q)) }))
      .filter((g) => g.models.length > 0)
  }
  // 专属倍率会改变生效值,不能只依赖后端按默认倍率的排序。
  return [...groups].sort(
    (a, b) => effectiveRate(a) - effectiveRate(b) || a.name.localeCompare(b.name)
  )
})
</script>

<style scoped>
/*
 * Admin-authored Markdown. Every rule here is on Family B tokens, so the block
 * flips with the theme on its own — the previous version paired a light color
 * with an explicit `dark:` half on six selectors, and the link `hover:` state
 * had no dark half at all, so hovering a link in dark mode drove it toward the
 * background instead of away from it.
 */
.plaza-description {
  line-height: 1.7;
  overflow-wrap: anywhere;
}

.plaza-description :deep(h1),
.plaza-description :deep(h2),
.plaza-description :deep(h3) {
  @apply mb-2 mt-3 text-sm font-semibold text-ink first:mt-0;
}

.plaza-description :deep(p) {
  @apply mb-2 text-ink-secondary last:mb-0;
}

.plaza-description :deep(a) {
  @apply text-accent underline underline-offset-2 transition-colors duration-fast hover:text-accent-hover;
}

.plaza-description :deep(ul) {
  @apply mb-2 list-disc pl-5;
}

.plaza-description :deep(ol) {
  @apply mb-2 list-decimal pl-5;
}

.plaza-description :deep(li) {
  @apply mb-0.5 text-ink-secondary;
}

.plaza-description :deep(code) {
  @apply rounded-sm border border-line bg-surface-sunken px-1 py-0.5 font-mono text-xs text-ink;
}

.plaza-description :deep(blockquote) {
  @apply my-2 border-l-2 border-line pl-3 text-ink-tertiary;
}
</style>
