<template>
  <div class="relative" ref="rootRef">
    <div
      class="input flex min-h-[42px] items-center gap-2 py-1.5"
      :class="{ 'ring-2 ring-primary-500': focused }"
      @click="focusInput"
    >
      <span v-if="selectedLabel && !focused" class="flex-1 truncate text-sm text-gray-900 dark:text-gray-100">
        {{ selectedLabel }}
        <span class="ml-1 text-xs text-gray-400">#{{ modelValue }}</span>
      </span>
      <input
        v-else
        ref="inputRef"
        v-model="keyword"
        type="text"
        class="flex-1 min-w-0 border-none bg-transparent p-0 text-sm outline-none focus:ring-0"
        :placeholder="selectedLabel ? selectedLabel : placeholder"
        @focus="onFocus"
        @blur="onBlur"
        @input="onInput"
      />
      <button
        v-if="modelValue"
        type="button"
        class="shrink-0 text-gray-400 hover:text-red-500"
        @click.stop="clear"
      >
        <Icon name="x" size="xs" />
      </button>
    </div>
    <div
      v-if="showDropdown && (loading || results.length > 0 || emptyHint)"
      class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
    >
      <div v-if="loading" class="px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="results.length === 0" class="px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
        {{ emptyHint }}
      </div>
      <button
        v-for="item in results"
        :key="item.id"
        type="button"
        class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
        :class="item.id === modelValue ? 'bg-primary-50 dark:bg-primary-900/20' : ''"
        @mousedown.prevent
        @click="select(item)"
      >
        <span class="truncate text-gray-900 dark:text-gray-100">
          {{ item.label }}
          <span v-if="item.sub" class="ml-1 text-xs text-gray-400">{{ item.sub }}</span>
        </span>
        <span class="text-xs text-gray-400">#{{ item.id }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useKeyedDebouncedSearch } from '@/composables/useKeyedDebouncedSearch'

export interface EntitySearchItem {
  id: number
  label: string
  sub?: string
}

const props = defineProps<{
  modelValue: number | null | undefined
  placeholder?: string
  search: (keyword: string, signal: AbortSignal) => Promise<EntitySearchItem[]>
  resolveLabel?: (id: number) => Promise<EntitySearchItem | null>
  resetToken?: string | number
  hintType?: string
  hintNoMatch?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: number | null): void
}>()

const { t } = useI18n()
const rootRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const keyword = ref('')
const results = ref<EntitySearchItem[]>([])
const loading = ref(false)
const focused = ref(false)
const showDropdown = ref(false)
const selectedLabel = ref('')

const emptyHint = computed(() => props.hintNoMatch || t('common.entitySearch.noMatches'))

const runner = useKeyedDebouncedSearch<EntitySearchItem[]>({
  delay: 300,
  search: async (kw, { signal }) => props.search(kw.trim(), signal),
  onSuccess: (_key, data) => {
    results.value = data
    loading.value = false
  },
  onError: () => {
    results.value = []
    loading.value = false
  },
})

watch(
  () => props.modelValue,
  async (id) => {
    if (!id) {
      selectedLabel.value = ''
      return
    }
    if (!props.resolveLabel) return
    // 捕获本次 modelValue 作为 token，async 完成后比对，
    // 防止旧 resolveLabel 在 modelValue 已被更新/清空后写回过期 label
    const token = id
    try {
      const item = await props.resolveLabel(id)
      if (props.modelValue !== token) return
      selectedLabel.value = item?.label ?? ''
    } catch {
      if (props.modelValue !== token) return
      selectedLabel.value = ''
    }
  },
  { immediate: true },
)

watch(
  () => props.resetToken,
  (curr, prev) => {
    if (prev === undefined || curr === prev) return
    if (props.modelValue !== null) emit('update:modelValue', null)
    selectedLabel.value = ''
    keyword.value = ''
    results.value = []
  },
)

// focus 即拉一次（含空关键词）：未输入时返回前 N 条，输入后由 onInput 防抖刷新
function onFocus() {
  focused.value = true
  showDropdown.value = true
  loading.value = true
  runner.trigger('entity', keyword.value)
}

function onBlur() {
  focused.value = false
  setTimeout(() => {
    if (!rootRef.value?.contains(document.activeElement)) {
      showDropdown.value = false
    }
  }, 150)
}

function onInput() {
  loading.value = true
  runner.trigger('entity', keyword.value)
}

function focusInput() {
  inputRef.value?.focus()
}

function select(item: EntitySearchItem) {
  emit('update:modelValue', item.id)
  selectedLabel.value = item.label
  keyword.value = ''
  // 保留 results：下次 focus 时用户仍能看到包含已选项的列表，
  // 避免新搜索的前 N 条不含已选项导致看不到选中态
  showDropdown.value = false
  focused.value = false
}

function clear() {
  emit('update:modelValue', null)
  selectedLabel.value = ''
  keyword.value = ''
  results.value = []
  inputRef.value?.focus()
}

function handleClickOutside(ev: MouseEvent) {
  if (rootRef.value && !rootRef.value.contains(ev.target as Node)) {
    showDropdown.value = false
    focused.value = false
  }
}

onMounted(() => document.addEventListener('mousedown', handleClickOutside))
onUnmounted(() => document.removeEventListener('mousedown', handleClickOutside))
</script>
