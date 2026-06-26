<template>
  <div ref="containerRef" class="relative">
    <div class="flex">
      <input
        :value="currentValue"
        type="text"
        class="input rounded-r-none border-r-0"
        :data-testid="inputTestId || undefined"
        :placeholder="placeholder"
        :disabled="disabled"
        @input="handleInput"
        @focus="closeDropdown"
      />
      <button
        type="button"
        class="flex w-11 flex-shrink-0 items-center justify-center rounded-r-xl border border-l-0 border-gray-200 bg-white text-gray-500 transition hover:border-gray-300 hover:bg-gray-50 disabled:cursor-not-allowed disabled:opacity-60 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-300 dark:hover:border-dark-500 dark:hover:bg-dark-700"
        :data-testid="toggleTestId || undefined"
        :disabled="disabled"
        :aria-expanded="isOpen"
        aria-haspopup="listbox"
        @click.stop="toggleDropdown"
      >
        <Icon
          name="chevronDown"
          size="sm"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
          :stroke-width="2"
        />
      </button>
    </div>
    <Transition name="select-dropdown">
      <div
        v-if="isOpen"
        class="absolute left-0 right-0 z-50 mt-1 overflow-hidden rounded-xl border border-gray-200 bg-white shadow-lg shadow-black/10 dark:border-dark-700 dark:bg-dark-800 dark:shadow-black/30"
        role="listbox"
        @click.stop
      >
        <div class="max-h-64 overflow-y-auto py-1">
          <button
            v-for="(option, index) in presetOptions"
            :key="option"
            type="button"
            role="option"
            class="flex w-full items-center gap-2 px-4 py-2.5 text-left text-sm text-gray-700 transition hover:bg-gray-50 dark:text-gray-300 dark:hover:bg-dark-700"
            :class="currentValue === option && 'bg-primary-50 text-primary-700 dark:bg-primary-900/20 dark:text-primary-300'"
            :aria-selected="currentValue === option"
            :data-testid="optionTestIdPrefix ? `${optionTestIdPrefix}-${index}` : undefined"
            @click="applyPreset(option)"
          >
            <span class="min-w-0 flex-1 break-all leading-5">{{ option }}</span>
            <Icon
              v-if="currentValue === option"
              name="check"
              size="sm"
              class="flex-shrink-0 text-primary-500"
              :stroke-width="2"
            />
          </button>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { Icon } from '@/components/icons'

const presetOptions = [
  'codex_cli_rs/0.125.0 (Ubuntu 22.4.0; x86_64) xterm-256color',
  'Claude Code/0.5.0 (Macos 15.5; arm64) iTerm2.app (Claude Code; 1.0.4)',
  'GeminiCLI/0.1.5 (Windows; AMD64)',
  'antigravity/1.23.2 windows/amd64'
]

const props = withDefaults(defineProps<{
  modelValue: string | null | undefined
  placeholder?: string
  disabled?: boolean
  inputTestId?: string
  toggleTestId?: string
  optionTestIdPrefix?: string
}>(), {
  placeholder: '',
  disabled: false
})

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const containerRef = ref<HTMLElement | null>(null)
const isOpen = ref(false)
const currentValue = computed(() => props.modelValue ?? '')

const handleInput = (event: Event) => {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}

const toggleDropdown = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
}

const closeDropdown = () => {
  isOpen.value = false
}

const applyPreset = (value: string) => {
  emit('update:modelValue', value)
  closeDropdown()
}

const handleOutsideClick = (event: MouseEvent) => {
  const target = event.target as Node
  if (containerRef.value && !containerRef.value.contains(target)) {
    closeDropdown()
  }
}

watch(
  () => props.disabled,
  (disabled) => {
    if (disabled) closeDropdown()
  }
)

onMounted(() => {
  document.addEventListener('click', handleOutsideClick)
})

onUnmounted(() => {
  document.removeEventListener('click', handleOutsideClick)
})
</script>
