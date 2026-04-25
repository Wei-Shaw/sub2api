<template>
  <div class="relative" ref="rootRef">
    <div
      class="input flex min-h-[42px] flex-wrap items-center gap-1.5 py-1.5"
      :class="{ 'ring-2 ring-primary-500': focused }"
      @click="focusInput"
    >
      <span
        v-for="item in selectedItems"
        :key="item.id"
        class="inline-flex items-center gap-1 rounded-full bg-primary-100 px-2 py-0.5 text-xs text-primary-800 dark:bg-primary-900/30 dark:text-primary-300"
      >
        {{ item.label }}
        <button type="button" class="hover:text-red-500" @click.stop="remove(item.id)">
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        ref="inputRef"
        v-model="keyword"
        type="text"
        class="min-w-[80px] flex-1 border-none bg-transparent p-0 text-sm outline-none focus:ring-0"
        :placeholder="selectedItems.length === 0 ? placeholder : ''"
        @focus="onFocus"
        @blur="onBlur"
        @input="onInput"
      />
    </div>
    <div
      v-if="showDropdown && (loading || filteredResults.length > 0 || emptyHint)"
      class="absolute z-50 mt-1 max-h-60 w-full overflow-auto rounded-lg border border-gray-200 bg-white shadow-lg dark:border-dark-700 dark:bg-dark-800"
    >
      <div v-if="loading" class="px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
        {{ t('common.loading') }}
      </div>
      <div v-else-if="filteredResults.length === 0" class="px-3 py-2 text-xs text-gray-500 dark:text-gray-400">
        {{ emptyHint }}
      </div>
      <button
        v-for="item in filteredResults"
        :key="item.id"
        type="button"
        class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 dark:hover:bg-dark-700"
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
import type { EntitySearchItem } from '@/components/common/EntitySearchSelect.vue'

const props = defineProps<{
  modelValue: number[]
  placeholder?: string
  search: (keyword: string, signal: AbortSignal) => Promise<EntitySearchItem[]>
  resolveLabels?: (ids: number[]) => Promise<EntitySearchItem[]>
  resetToken?: string | number
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: number[]): void
}>()

const { t } = useI18n()
const rootRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const keyword = ref('')
const results = ref<EntitySearchItem[]>([])
const loading = ref(false)
const focused = ref(false)
const showDropdown = ref(false)
const selectedItems = ref<EntitySearchItem[]>([])

const selectedSet = computed(() => new Set(props.modelValue))

const filteredResults = computed(() =>
  results.value.filter((r) => !selectedSet.value.has(r.id)),
)

const emptyHint = computed(() =>
  keyword.value.trim().length === 0
    ? t('common.userSearch.typeToSearch')
    : t('common.userSearch.noMatches'),
)

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
  async (ids) => {
    if (!ids || ids.length === 0) {
      selectedItems.value = []
      return
    }
    const missing = ids.filter((id) => !selectedItems.value.some((s) => s.id === id))
    if (missing.length > 0 && props.resolveLabels) {
      try {
        const resolved = await props.resolveLabels(missing)
        const existing = selectedItems.value.filter((s) => ids.includes(s.id))
        selectedItems.value = [...existing, ...resolved]
      } catch {
        /* keep existing */
      }
    }
    selectedItems.value = selectedItems.value.filter((s) => ids.includes(s.id))
  },
  { immediate: true },
)

watch(
  () => props.resetToken,
  (curr, prev) => {
    if (prev === undefined || curr === prev) return
    if (props.modelValue.length > 0) emit('update:modelValue', [])
    selectedItems.value = []
    keyword.value = ''
    results.value = []
  },
)

function onFocus() {
  focused.value = true
  showDropdown.value = true
  if (keyword.value.trim()) {
    loading.value = true
    runner.trigger('entity', keyword.value)
  }
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
  selectedItems.value = [...selectedItems.value, item]
  emit('update:modelValue', [...props.modelValue, item.id])
  keyword.value = ''
  results.value = []
}

function remove(id: number) {
  selectedItems.value = selectedItems.value.filter((s) => s.id !== id)
  emit('update:modelValue', props.modelValue.filter((v) => v !== id))
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
