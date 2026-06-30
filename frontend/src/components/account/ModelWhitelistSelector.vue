<template>
  <div>
    <!-- Multi-select Dropdown -->
    <div class="relative mb-3">
      <div
        @click="toggleDropdown"
        class="cursor-pointer rounded-lg border border-gray-300 bg-white px-3 py-2 dark:border-dark-500 dark:bg-dark-700"
      >
        <div class="grid grid-cols-2 gap-1.5">
          <span
            v-for="model in modelValue"
            :key="model"
            class="inline-flex items-center justify-between gap-1 rounded bg-gray-100 px-2 py-1 text-xs text-gray-700 dark:bg-dark-600 dark:text-gray-300"
          >
            <span class="flex items-center gap-1 truncate">
              <ModelIcon :model="model" size="14px" />
              <span class="truncate">{{ model }}</span>
            </span>
            <button
              type="button"
              @click.stop="removeModel(model)"
              class="shrink-0 rounded-full hover:bg-gray-200 dark:hover:bg-dark-500"
            >
              <Icon name="x" size="xs" class="h-3.5 w-3.5" :stroke-width="2" />
            </button>
          </span>
        </div>
        <div class="mt-2 flex items-center justify-between border-t border-gray-200 pt-2 dark:border-dark-600">
          <span class="text-xs text-gray-400">{{ t('admin.accounts.modelCount', { count: modelValue.length }) }}</span>
          <svg class="h-5 w-5 text-gray-400" fill="none" viewBox="0 0 24 24" stroke="currentColor">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 9l-7 7-7-7" />
          </svg>
        </div>
      </div>
      <!-- Dropdown List -->
      <div
        v-if="showDropdown"
        class="absolute left-0 right-0 top-full z-50 mt-1 rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-700"
      >
        <div class="sticky top-0 border-b border-gray-200 bg-white p-2 dark:border-dark-600 dark:bg-dark-700">
          <input
            v-model="searchQuery"
            type="text"
            class="input w-full text-sm"
            :placeholder="t('admin.accounts.searchModels')"
            @click.stop
          />
        </div>
        <div class="max-h-52 overflow-auto">
          <div v-if="isSearchingModels || isSyncingUpstream" class="px-3 py-4 text-center text-sm text-gray-500">
            {{ isSyncingUpstream ? t('admin.accounts.syncUpstreamModelsLoading') : t('admin.accounts.searchingModels') }}
          </div>
          <template v-else>
            <button
              v-for="model in filteredModels"
              :key="model.value"
              type="button"
              @click="toggleModel(model.value)"
              class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
            >
              <span
                :class="[
                  'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                  modelValue.includes(model.value)
                    ? 'border-primary-500 bg-primary-500 text-white'
                    : 'border-gray-300 dark:border-dark-500'
                ]"
              >
                <svg v-if="modelValue.includes(model.value)" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
                </svg>
              </span>
              <ModelIcon :model="model.value" size="18px" />
              <span class="truncate text-gray-900 dark:text-white">{{ model.value }}</span>
            </button>
            <div v-if="filteredModels.length === 0" class="px-3 py-4 text-center text-sm text-gray-500">
              {{ dynamicEmptyHint }}
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- Quick Actions -->
    <div class="mb-4 flex flex-wrap gap-2">
      <button
        v-if="!isDynamicModelPlatform"
        type="button"
        @click="fillRelated"
        class="rounded-lg border border-blue-200 px-3 py-1.5 text-sm text-blue-600 hover:bg-blue-50 dark:border-blue-800 dark:text-blue-400 dark:hover:bg-blue-900/30"
      >
        {{ t('admin.accounts.fillRelatedModels') }}
      </button>
      <button
        v-if="canSyncUpstream"
        type="button"
        @click="syncUpstreamModels"
        :disabled="isSyncingUpstream"
        class="rounded-lg border border-emerald-200 px-3 py-1.5 text-sm text-emerald-600 hover:bg-emerald-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-emerald-800 dark:text-emerald-400 dark:hover:bg-emerald-900/30"
      >
        {{ isSyncingUpstream ? t('admin.accounts.syncUpstreamModelsLoading') : t('admin.accounts.syncUpstreamModels') }}
      </button>
      <button
        type="button"
        @click="clearAll"
        class="rounded-lg border border-red-200 px-3 py-1.5 text-sm text-red-600 hover:bg-red-50 dark:border-red-800 dark:text-red-400 dark:hover:bg-red-900/30"
      >
        {{ t('admin.accounts.clearAllModels') }}
      </button>
    </div>

    <!-- Custom Model Input -->
    <div class="mb-3">
      <label class="mb-1.5 block text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('admin.accounts.customModelName') }}</label>
      <div class="flex gap-2">
        <input
          v-model="customModel"
          type="text"
          class="input flex-1"
          :placeholder="t('admin.accounts.enterCustomModelName')"
          @keydown.enter.prevent="handleEnter"
          @compositionstart="isComposing = true"
          @compositionend="isComposing = false"
        />
        <button
          type="button"
          @click="addCustom"
          class="rounded-lg bg-primary-50 px-4 py-2 text-sm font-medium text-primary-600 hover:bg-primary-100 dark:bg-primary-900/30 dark:text-primary-400 dark:hover:bg-primary-900/50"
        >
          {{ t('admin.accounts.addModel') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { accountsAPI } from '@/api/admin/accounts'
import type { SyncUpstreamPreviewParams, SearchUpstreamPreviewParams } from '@/api/admin/accounts'
import ModelIcon from '@/components/common/ModelIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import { allModels, getModelsByPlatform } from '@/composables/useModelWhitelist'

const { t } = useI18n()

const props = defineProps<{
  modelValue: string[]
  platform?: string
  platforms?: string[]
  accountId?: number
  syncCredentials?: {
    platform: string
    type: string
    base_url?: string
    api_key: string
  }
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const appStore = useAppStore()

const showDropdown = ref(false)
const searchQuery = ref('')
const customModel = ref('')
const isComposing = ref(false)
const isSyncingUpstream = ref(false)
const normalizedPlatforms = computed(() => {
  const rawPlatforms =
    props.platforms && props.platforms.length > 0
      ? props.platforms
      : props.platform
        ? [props.platform]
        : []

  return Array.from(
    new Set(
      rawPlatforms
        .map(platform => platform?.trim())
        .filter((platform): platform is string => Boolean(platform))
    )
  )
})

const upstreamSyncPlatforms = new Set(['anthropic', 'openai', 'gemini', 'antigravity', 'grok', 'fal'])
const canSyncUpstream = computed(() => {
  if (props.accountId) {
    if (normalizedPlatforms.value.length === 0) return true
    return normalizedPlatforms.value.some(platform => upstreamSyncPlatforms.has(platform.toLowerCase()))
  }
  if (props.syncCredentials) {
    return upstreamSyncPlatforms.has(props.syncCredentials.platform.toLowerCase())
  }
  return false
})

// 动态平台：可选模型清单不是硬编码，而是来自上游 models 接口（如 fal）。
// 这类平台打开下拉时按需从接口拉取可选项。
const dynamicModelPlatforms = new Set(['fal'])
const isDynamicModelPlatform = computed(() => {
  if (props.syncCredentials) {
    return dynamicModelPlatforms.has(props.syncCredentials.platform.toLowerCase())
  }
  return normalizedPlatforms.value.some(platform => dynamicModelPlatforms.has(platform.toLowerCase()))
})

// dynamicModels 缓存「同步上游支持的模型」手动拉取的全量结果，作为下拉可选项。
const dynamicModels = ref<string[]>([])
// 动态平台搜索：输入关键词时走上游搜索接口（fal /v1/models?q=），结果存这里。
const searchResults = ref<string[]>([])
const isSearchingModels = ref(false)
let searchTimer: ReturnType<typeof setTimeout> | null = null

const availableOptions = computed(() => {
  if (normalizedPlatforms.value.length === 0) {
    return allModels
  }

  const allowedModels = new Set<string>()
  for (const platform of normalizedPlatforms.value) {
    for (const model of getModelsByPlatform(platform)) {
      allowedModels.add(model)
    }
  }

  return allModels.filter(model => allowedModels.has(model.value))
})

const filteredModels = computed(() => {
  // 动态平台（如 fal）：列表来自上游接口，不做本地子串过滤。
  if (isDynamicModelPlatform.value) {
    const query = searchQuery.value.trim()
    if (query) {
      // 有关键词：仅展示上游搜索结果（服务端已按名称/描述/类别匹配）。
      // 已选模型以上方标签形式始终可见，无需在此重复展示。
      return searchResults.value.map(value => ({ value, label: value }))
    }
    // 无关键词：展示已选模型 + 手动同步缓存的全量清单。
    const merged = new Set<string>(dynamicModels.value)
    for (const model of props.modelValue) merged.add(model)
    return Array.from(merged).map(value => ({ value, label: value }))
  }

  const query = searchQuery.value.toLowerCase().trim()
  if (!query) return availableOptions.value
  return availableOptions.value.filter(
    m => m.value.toLowerCase().includes(query) || m.label.toLowerCase().includes(query)
  )
})

// dynamicEmptyHint 决定动态平台空列表时的提示语：
// 有关键词但无结果 → 无匹配；无关键词 → 引导搜索或同步。
const dynamicEmptyHint = computed(() => {
  if (!isDynamicModelPlatform.value) return t('admin.accounts.noMatchingModels')
  if (searchQuery.value.trim()) return t('admin.accounts.noMatchingModels')
  return t('admin.accounts.dynamicModelsSearchHint')
})

// searchDynamicModels 走上游搜索接口按关键词检索模型（仅动态平台）。
const searchDynamicModels = async (query: string) => {
  if (!isDynamicModelPlatform.value) return
  if (!props.accountId && !props.syncCredentials) return

  isSearchingModels.value = true
  try {
    let result
    if (props.accountId) {
      result = await accountsAPI.searchUpstreamModels(props.accountId, query)
    } else if (props.syncCredentials) {
      result = await accountsAPI.searchUpstreamModelsPreview({
        ...(props.syncCredentials as SyncUpstreamPreviewParams),
        q: query,
      } as SearchUpstreamPreviewParams)
    } else {
      return
    }
    searchResults.value = result.models.map(model => model.trim()).filter(Boolean)
  } catch (error) {
    const message = error instanceof Error ? error.message : t('admin.accounts.syncUpstreamModelsFailed')
    appStore.showError(t('admin.accounts.syncUpstreamModelsError', { message }))
  } finally {
    isSearchingModels.value = false
  }
}

// 动态平台：监听输入框，防抖后走上游搜索接口；非动态平台沿用本地过滤。
watch(searchQuery, value => {
  if (!isDynamicModelPlatform.value) return
  if (searchTimer) clearTimeout(searchTimer)
  const query = value.trim()
  if (!query) {
    searchResults.value = []
    isSearchingModels.value = false
    return
  }
  if (!props.accountId && !props.syncCredentials) return
  // 立即进入加载态：防抖期间 searchResults 仍为空，若不置位会先闪现「无匹配模型」，
  // 等请求返回后再跳出结果。提前置位可让下拉始终显示「搜索中…」直到结果返回。
  isSearchingModels.value = true
  searchTimer = setTimeout(() => {
    void searchDynamicModels(query)
  }, 350)
})

onBeforeUnmount(() => {
  if (searchTimer) clearTimeout(searchTimer)
})

const toggleDropdown = () => {
  showDropdown.value = !showDropdown.value
  if (!showDropdown.value) {
    searchQuery.value = ''
    // 关闭时清空搜索结果，避免下次打开残留上次搜索项。
    searchResults.value = []
  }
  // 动态平台（如 fal）打开下拉时不再自动全量拉取；改为输入关键词时走搜索接口，
  // 或由用户手动点击「同步上游支持的模型」拉取全部。
}

const removeModel = (model: string) => {
  emit('update:modelValue', props.modelValue.filter(m => m !== model))
}

const toggleModel = (model: string) => {
  if (props.modelValue.includes(model)) {
    removeModel(model)
  } else {
    emit('update:modelValue', [...props.modelValue, model])
  }
}

const addCustom = () => {
  const model = customModel.value.trim()
  if (!model) return
  if (props.modelValue.includes(model)) {
    appStore.showInfo(t('admin.accounts.modelExists'))
    return
  }
  emit('update:modelValue', [...props.modelValue, model])
  customModel.value = ''
}

const handleEnter = () => {
  if (!isComposing.value) addCustom()
}

const fillRelated = () => {
  const newModels = [...props.modelValue]
  for (const platform of normalizedPlatforms.value) {
    for (const model of getModelsByPlatform(platform)) {
      if (!newModels.includes(model)) {
        newModels.push(model)
      }
    }
  }
  emit('update:modelValue', newModels)
}

const syncUpstreamModels = async () => {
  if (isSyncingUpstream.value) return
  if (!props.accountId && !props.syncCredentials) return

  isSyncingUpstream.value = true
  try {
    let result
    if (props.accountId) {
      result = await accountsAPI.syncUpstreamModels(props.accountId)
    } else if (props.syncCredentials) {
      result = await accountsAPI.syncUpstreamModelsPreview(props.syncCredentials as SyncUpstreamPreviewParams)
    } else {
      return
    }

    const upstreamModels = result.models.map(model => model.trim()).filter(Boolean)
    if (upstreamModels.length === 0) {
      appStore.showInfo(t('admin.accounts.syncUpstreamModelsEmpty'))
      return
    }

    // 动态平台：缓存拉取结果作为下拉可选项，避免再次请求。
    if (isDynamicModelPlatform.value) {
      dynamicModels.value = upstreamModels
    }

    const newModels = [...props.modelValue]
    let addedCount = 0
    for (const model of upstreamModels) {
      if (!newModels.includes(model)) {
        newModels.push(model)
        addedCount += 1
      }
    }

    emit('update:modelValue', newModels)
    if (addedCount > 0) {
      appStore.showSuccess(t('admin.accounts.syncUpstreamModelsSuccess', { count: addedCount, total: upstreamModels.length }))
    } else {
      appStore.showInfo(t('admin.accounts.syncUpstreamModelsNoChanges', { count: upstreamModels.length }))
    }
  } catch (error) {
    const message = error instanceof Error ? error.message : t('admin.accounts.syncUpstreamModelsFailed')
    appStore.showError(t('admin.accounts.syncUpstreamModelsError', { message }))
  } finally {
    isSyncingUpstream.value = false
  }
}

const clearAll = () => {
  emit('update:modelValue', [])
}

</script>
