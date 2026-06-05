<template>
  <div ref="rootEl">
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
            class="inline-flex items-center justify-between gap-1 rounded px-2 py-1 text-xs"
            :class="modelTagClass(model)"
          >
            <span class="inline-flex items-center gap-1 truncate">
              <span v-if="modelProtocol(model)" class="protocol-icon-inline" v-html="modelProtocol(model)!.iconSvg"></span>
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
        v-if="showDropdown && hasAvailableModels"
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
          <!-- Protocol-grouped mode -->
          <template v-if="resolvedGroups.length > 0">
            <template v-for="group in filteredGroups" :key="group.protocol.id">
              <div class="sticky top-0 flex items-center gap-1.5 bg-gray-50 px-3 py-1.5 dark:bg-dark-800">
                <span class="protocol-icon-inline" :style="{ color: group.protocol.themeColor }" v-html="group.protocol.iconSvg"></span>
                <span class="text-xs font-semibold" :style="{ color: group.protocol.themeColor }">{{ group.protocol.displayName }}</span>
                <span class="text-[10px] text-gray-400">({{ group.models.length }})</span>
              </div>
              <button
                v-for="model in group.models"
                :key="model.id"
                type="button"
                @click="toggleModel(model.id)"
                class="flex w-full items-center gap-2 px-3 py-2 pl-6 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
              >
                <span
                  :class="[
                    'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                    modelValue.includes(model.id)
                      ? 'border-primary-500 bg-primary-500 text-white'
                      : 'border-gray-300 dark:border-dark-500'
                  ]"
                >
                  <svg v-if="modelValue.includes(model.id)" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
                  </svg>
                </span>
                <span class="truncate text-gray-900 dark:text-white">{{ model.displayName }}</span>
                <span class="ml-auto truncate text-[10px] text-gray-400">{{ model.id }}</span>
              </button>
            </template>
            <div v-if="filteredGroups.length === 0 || filteredGroups.every(g => g.models.length === 0)" class="px-3 py-4 text-center text-sm text-gray-500">
              {{ t('admin.accounts.noMatchingModels') }}
            </div>
          </template>
          <!-- Flat mode (backward compatible) -->
          <template v-else>
            <button
              v-for="model in filteredModels"
              :key="model"
              type="button"
              @click="toggleModel(model)"
              class="flex w-full items-center gap-2 px-3 py-2 text-left text-sm hover:bg-gray-100 dark:hover:bg-dark-600"
            >
              <span
                :class="[
                  'flex h-4 w-4 shrink-0 items-center justify-center rounded border',
                  modelValue.includes(model)
                    ? 'border-primary-500 bg-primary-500 text-white'
                    : 'border-gray-300 dark:border-dark-500'
                ]"
              >
                <svg v-if="modelValue.includes(model)" class="h-3 w-3" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="3" d="M5 13l4 4L19 7" />
                </svg>
              </span>
              <span class="truncate text-gray-900 dark:text-white">{{ model }}</span>
            </button>
            <div v-if="filteredModels.length === 0" class="px-3 py-4 text-center text-sm text-gray-500">
              {{ t('admin.accounts.noMatchingModels') }}
            </div>
          </template>
        </div>
      </div>
    </div>

    <!-- Quick Actions -->
    <div v-if="hasAvailableModels" class="mb-4 flex flex-wrap gap-2">
      <button
        type="button"
        @click="fillRelated"
        class="rounded-lg border border-blue-200 px-3 py-1.5 text-sm text-blue-600 hover:bg-blue-50 dark:border-blue-800 dark:text-blue-400 dark:hover:bg-blue-900/30"
      >
        {{ t('admin.accounts.fillRelatedModels') }}
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
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '../Icon.vue'
import { resolveProtocolModels, findProtocolForModel } from '../../protocols'
import type { ProtocolDefinition, ProtocolModel } from '../../protocols'

const PROTOCOL_TAG_COLORS: Record<string, string> = {
  '#ea580c': 'bg-orange-100 text-orange-700 dark:bg-orange-900/20 dark:text-orange-400',
  '#10b981': 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/20 dark:text-emerald-400',
  '#2563eb': 'bg-blue-100 text-blue-700 dark:bg-blue-900/20 dark:text-blue-400',
}
const DEFAULT_TAG_CLASS = 'bg-gray-100 text-gray-700 dark:bg-dark-600 dark:text-gray-300'

interface FilteredGroup {
  protocol: ProtocolDefinition
  models: ProtocolModel[]
}

const props = withDefaults(defineProps<{
  modelValue: string[]
  availableModels?: string[]
  protocols?: string[]
  platform?: string
  onNotifyInfo?: (msg: string) => void
}>(), {
  availableModels: () => [],
})

const emit = defineEmits<{
  'update:modelValue': [value: string[]]
}>()

const { t } = useI18n()

const rootEl = ref<HTMLElement | null>(null)
const showDropdown = ref(false)
const searchQuery = ref('')
const customModel = ref('')
const isComposing = ref(false)

const resolvedGroups = computed(() =>
  props.protocols?.length ? resolveProtocolModels(props.protocols) : []
)

const allModelIds = computed(() => {
  if (resolvedGroups.value.length > 0) {
    return resolvedGroups.value.flatMap(g => g.models.map(m => m.id))
  }
  return props.availableModels
})

const hasAvailableModels = computed(() => allModelIds.value.length > 0)

const filteredGroups = computed<FilteredGroup[]>(() => {
  const query = searchQuery.value.toLowerCase().trim()
  return resolvedGroups.value
    .map(g => ({
      protocol: g.protocol,
      models: query
        ? g.models.filter(m =>
            m.id.toLowerCase().includes(query) ||
            m.displayName.toLowerCase().includes(query))
        : g.models,
    }))
    .filter(g => g.models.length > 0)
})

const filteredModels = computed(() => {
  const query = searchQuery.value.toLowerCase().trim()
  if (!query) return props.availableModels
  return props.availableModels.filter(m => m.toLowerCase().includes(query))
})

const modelProtocol = (modelId: string): ProtocolDefinition | undefined => {
  if (!props.protocols?.length) return undefined
  return findProtocolForModel(modelId, props.protocols)
}

const modelTagClass = (modelId: string): string => {
  const proto = modelProtocol(modelId)
  if (proto) return PROTOCOL_TAG_COLORS[proto.themeColor] ?? DEFAULT_TAG_CLASS
  return DEFAULT_TAG_CLASS
}

const toggleDropdown = () => {
  showDropdown.value = !showDropdown.value
  if (!showDropdown.value) searchQuery.value = ''
}

const onClickOutside = (event: MouseEvent) => {
  if (showDropdown.value && rootEl.value && !rootEl.value.contains(event.target as Node)) {
    showDropdown.value = false
    searchQuery.value = ''
  }
}

onMounted(() => {
  document.addEventListener('click', onClickOutside, true)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', onClickOutside, true)
})

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
    const notify = props.onNotifyInfo ?? console.info
    notify(t('admin.accounts.modelExists'))
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
  for (const model of allModelIds.value) {
    if (!newModels.includes(model)) {
      newModels.push(model)
    }
  }
  emit('update:modelValue', newModels)
}

const clearAll = () => {
  emit('update:modelValue', [])
}
</script>

<style scoped>
.protocol-icon-inline :deep(svg) {
  width: 0.875rem;
  height: 0.875rem;
  flex-shrink: 0;
  fill: currentColor;
}
</style>
