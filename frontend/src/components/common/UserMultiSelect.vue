<template>
  <div class="relative" ref="rootRef">
    <div
      class="input flex min-h-[42px] flex-wrap items-center gap-1.5 py-1.5"
      :class="{ 'ring-2 ring-primary-500': focused }"
      @click="focusInput"
    >
      <span
        v-for="user in modelValue"
        :key="user.id"
        class="inline-flex items-center gap-1 rounded-md bg-primary-50 px-2 py-0.5 text-xs font-medium text-primary-700 dark:bg-primary-900/30 dark:text-primary-300"
      >
        {{ user.email }}
        <span class="text-primary-400 dark:text-primary-500">#{{ user.id }}</span>
        <button type="button" class="text-primary-500 hover:text-red-500" @click.stop="remove(user)">
          <Icon name="x" size="xs" />
        </button>
      </span>
      <input
        ref="inputRef"
        v-model="keyword"
        type="text"
        class="flex-1 min-w-[120px] border-none bg-transparent p-0 text-sm outline-none focus:ring-0"
        :placeholder="modelValue.length === 0 ? placeholder : ''"
        @focus="onFocus"
        @blur="onBlur"
        @input="onInput"
      />
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
        v-for="user in results"
        :key="user.id"
        type="button"
        :disabled="selectedIds.has(user.id)"
        class="flex w-full items-center justify-between px-3 py-2 text-left text-sm hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-50 dark:hover:bg-dark-700"
        @mousedown.prevent
        @click="select(user)"
      >
        <span class="truncate text-gray-900 dark:text-gray-100">{{ user.email }}</span>
        <span class="text-xs text-gray-400">#{{ user.id }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useKeyedDebouncedSearch } from '@/composables/useKeyedDebouncedSearch'
import adminAPI from '@/api/admin'
import type { SimpleUser } from '@/api/admin/usage'

const props = defineProps<{
  modelValue: SimpleUser[]
  placeholder?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: SimpleUser[]): void
}>()

const { t } = useI18n()
const rootRef = ref<HTMLElement | null>(null)
const inputRef = ref<HTMLInputElement | null>(null)
const keyword = ref('')
const results = ref<SimpleUser[]>([])
const loading = ref(false)
const focused = ref(false)
const showDropdown = ref(false)

const selectedIds = computed(() => new Set(props.modelValue.map((u) => u.id)))

const emptyHint = computed(() =>
  keyword.value.trim().length === 0
    ? t('common.userSearch.typeToSearch')
    : t('common.userSearch.noMatches'),
)

const runner = useKeyedDebouncedSearch<SimpleUser[]>({
  delay: 300,
  search: async (kw) => {
    if (kw.trim().length === 0) return []
    return adminAPI.usage.searchUsers(kw.trim())
  },
  onSuccess: (_key, data) => {
    results.value = data
    loading.value = false
  },
  onError: () => {
    results.value = []
    loading.value = false
  },
})

function onFocus() {
  focused.value = true
  showDropdown.value = true
}

function onBlur() {
  focused.value = false
  // delay to allow click on dropdown item to fire
  setTimeout(() => {
    if (!rootRef.value?.contains(document.activeElement)) {
      showDropdown.value = false
    }
  }, 150)
}

function onInput() {
  if (keyword.value.trim().length === 0) {
    results.value = []
    loading.value = false
    return
  }
  loading.value = true
  runner.trigger('user-multi', keyword.value)
}

function focusInput() {
  inputRef.value?.focus()
}

function select(user: SimpleUser) {
  if (selectedIds.value.has(user.id)) return
  emit('update:modelValue', [...props.modelValue, user])
  keyword.value = ''
  results.value = []
  inputRef.value?.focus()
}

function remove(user: SimpleUser) {
  emit('update:modelValue', props.modelValue.filter((u) => u.id !== user.id))
}

function handleClickOutside(ev: MouseEvent) {
  if (rootRef.value && !rootRef.value.contains(ev.target as Node)) {
    showDropdown.value = false
  }
}

onMounted(() => document.addEventListener('mousedown', handleClickOutside))
onUnmounted(() => document.removeEventListener('mousedown', handleClickOutside))
</script>
