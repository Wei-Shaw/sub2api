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
        <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div class="flex flex-wrap gap-2" role="group" :aria-label="t('modelPlaza.filters.categoryLabel')">
            <button
              v-for="category in MODEL_CATEGORIES"
              :key="category"
              type="button"
              class="rounded-full border px-3.5 py-1.5 text-sm font-medium transition"
              :class="selectedCategories.includes(category)
                ? 'border-primary-600 bg-primary-600 text-white'
                : 'border-gray-200 bg-white text-gray-600 hover:border-primary-300 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-300'"
              :aria-pressed="selectedCategories.includes(category)"
              @click="toggleCategory(category)"
            >
              {{ category }}
            </button>
          </div>
          <input
            v-model="searchQuery"
            type="search"
            class="input w-full lg:w-72"
            :placeholder="t('modelPlaza.filters.searchPlaceholder')"
          />
        </div>
        <p v-if="selectedCategories.length > 1" class="mt-2 text-xs text-gray-400 dark:text-dark-500">
          {{ t('modelPlaza.filters.orHint') }}
        </p>
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
import {
  MODEL_CATEGORIES,
  type ModelCategory,
  type ModelPlazaResponse,
  type PlazaModel
} from '@/api/modelPlaza'

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
const selectedCategories = ref<ModelCategory[]>([])
const searchQuery = ref('')

const searchActive = computed(() => searchQuery.value.trim() !== '' || selectedCategories.value.length > 0)

const descriptionHtml = computed(() => {
  const md = props.response?.description?.trim()
  if (!md) return ''
  return DOMPurify.sanitize(marked.parse(md) as string)
})

const allModels = computed<PlazaModel[]>(() => {
  const byName = new Map<string, PlazaModel>()
  for (const group of props.response?.groups ?? []) {
    for (const model of group.models) {
      if (!model.display_name) continue
      const key = model.name.toLowerCase()
      if (!byName.has(key)) byName.set(key, model)
    }
  }
  return [...byName.values()].sort(
    (a, b) => b.launch_date.localeCompare(a.launch_date) || a.name.localeCompare(b.name)
  )
})

const filteredModels = computed(() => {
  let models = allModels.value
  if (selectedCategories.value.length > 0) {
    models = models.filter((model) =>
      model.categories.some((category) => selectedCategories.value.includes(category))
    )
  }
  const q = searchQuery.value.trim().toLowerCase()
  if (q) {
    models = models.filter((model) =>
      `${model.name} ${model.display_name}`.toLowerCase().includes(q)
    )
  }
  return models
})

function toggleCategory(category: ModelCategory) {
  selectedCategories.value = selectedCategories.value.includes(category)
    ? selectedCategories.value.filter((item) => item !== category)
    : [...selectedCategories.value, category]
}
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
