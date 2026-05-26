<template>
  <div class="relative" ref="containerRef">
    <button
      ref="triggerRef"
      type="button"
      @click="toggle"
      :disabled="disabled"
      :aria-expanded="isOpen"
      :aria-haspopup="true"
      aria-label="Select option"
      :class="[
        'select-trigger',
        isOpen && 'select-trigger-open',
        error && 'select-trigger-error',
        disabled && 'select-trigger-disabled'
      ]"
      @keydown.down.prevent="onTriggerKeyDown"
      @keydown.up.prevent="onTriggerKeyDown"
    >
      <span class="select-value">
        <slot name="selected" :option="selectedOption">
          {{ selectedLabel }}
        </slot>
      </span>
      <span class="select-icon">
        <Icon
          name="chevronDown"
          size="md"
          :class="['transition-transform duration-200', isOpen && 'rotate-180']"
        />
      </span>
    </button>

    <Teleport :to="teleportTarget">
      <Transition name="select-dropdown">
        <SelectDropdown
          v-if="isOpen"
          ref="dropdownComp"
          :filtered-options="filteredOptions"
          :searchable="searchable"
          :search-query="searchQuery"
          :search-placeholder-text="searchPlaceholderText"
          :empty-text-display="emptyTextDisplay"
          :focused-index="focusedIndex"
          :instance-id="instanceId"
          :dropdown-style="dropdownStyle"
          :is-selected="isSelected"
          :get-value="getValue"
          :get-label="getLabel"
          :on-dropdown-key-down="onDropdownKeyDown"
          @select="selectOption"
          @mouseenter="handleOptionMouseEnter"
          @update:search-query="searchQuery = $event"
        >
          <template #option="optData">
            <slot name="option" v-bind="optData" />
          </template>
        </SelectDropdown>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, inject, toRef } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from './Icon.vue'
import SelectDropdown from './SelectDropdown.vue'
import type { SelectOption } from '../types'
import { PLUGIN_TELEPORT_TARGET } from '../host-sdk'
import { useSelect } from '../composables/useSelect'

export type { SelectOption }

const { t } = useI18n()

const teleportTarget = inject<HTMLElement | string>(PLUGIN_TELEPORT_TARGET, 'body')

interface Props {
  modelValue: string | number | boolean | null | undefined
  options: SelectOption[] | Array<Record<string, unknown>>
  placeholder?: string
  disabled?: boolean
  error?: boolean
  searchable?: boolean
  searchPlaceholder?: string
  emptyText?: string
  valueKey?: string
  labelKey?: string
  creatable?: boolean
  creatablePrefix?: string
}

interface Emits {
  (e: 'update:modelValue', value: string | number | boolean | null): void
  (e: 'change', value: string | number | boolean | null, option: SelectOption | null): void
}

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  error: false,
  searchable: false,
  creatable: false,
  creatablePrefix: '',
  valueKey: 'value',
  labelKey: 'label',
})

const emit = defineEmits<Emits>()

// ── DOM refs ──────────────────────────────────────────────────────────
const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const dropdownComp = ref<InstanceType<typeof SelectDropdown> | null>(null)

const dropdownRef = computed(() => dropdownComp.value?.dropdownRef ?? null)
const searchInputRef = computed(() => dropdownComp.value?.searchInputRef ?? null)
const optionsListRef = computed(() => dropdownComp.value?.optionsListRef ?? null)

// ── i18n placeholders ─────────────────────────────────────────────────
const placeholderText = computed(() => props.placeholder ?? t('common.selectOption'))
const searchPlaceholderText = computed(() => props.searchPlaceholder ?? t('common.searchPlaceholder'))
const emptyTextDisplay = computed(() => props.emptyText ?? t('common.noOptionsFound'))
const searchLabel = computed(() => t('common.search'))

// ── Composable ────────────────────────────────────────────────────────
const {
  isOpen, searchQuery, focusedIndex, instanceId,
  selectedOption, selectedLabel,
  filteredOptions, dropdownStyle,
  isSelected, toggle, selectOption,
  handleOptionMouseEnter,
  onTriggerKeyDown, onDropdownKeyDown,
  getValue, getLabel,
} = useSelect({
  modelValue: toRef(props, 'modelValue'),
  options: toRef(props, 'options'),
  disabled: props.disabled,
  searchable: props.searchable,
  creatable: props.creatable,
  creatablePrefix: props.creatablePrefix,
  valueKey: props.valueKey,
  labelKey: props.labelKey,
  placeholderText,
  searchPlaceholderText,
  emptyTextDisplay,
  containerRef,
  triggerRef,
  searchInputRef: searchInputRef as unknown as import('vue').Ref<HTMLInputElement | null>,
  dropdownRef: dropdownRef as unknown as import('vue').Ref<HTMLElement | null>,
  optionsListRef: optionsListRef as unknown as import('vue').Ref<HTMLElement | null>,
  onUpdate: (v) => emit('update:modelValue', v),
  onChange: (v, o) => emit('change', v, o),
  searchLabel,
})
</script>

<style scoped>
.select-trigger {
  @apply flex w-full items-center justify-between gap-2;
  @apply rounded-xl px-4 py-2.5 text-sm;
  @apply bg-white dark:bg-dark-800;
  @apply border border-gray-200 dark:border-dark-600;
  @apply text-gray-900 dark:text-gray-100;
  @apply transition-all duration-200;
  @apply focus:border-primary-500 focus:outline-none focus:ring-2 focus:ring-primary-500/30;
  @apply hover:border-gray-300 dark:hover:border-dark-500;
  @apply cursor-pointer;
}

.select-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
}

.select-trigger-error {
  @apply border-red-500 focus:border-red-500 focus:ring-red-500/30;
}

.select-trigger-disabled {
  @apply cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900;
}

.select-value {
  @apply flex-1 truncate text-left;
}

.select-icon {
  @apply flex-shrink-0 text-gray-400 dark:text-dark-400;
}
</style>

<style>
.select-dropdown-portal {
  @apply w-max min-w-[200px];
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-hidden;
  pointer-events: auto !important;
}

.select-dropdown-portal .select-search {
  @apply flex items-center gap-2 px-3 py-2;
  @apply border-b border-gray-100 dark:border-dark-700;
}

.select-dropdown-portal .select-search-input {
  @apply flex-1 bg-transparent text-sm;
  @apply text-gray-900 dark:text-gray-100;
  @apply placeholder:text-gray-400 dark:placeholder:text-dark-400;
  @apply focus:outline-none;
}

.select-dropdown-portal .select-options {
  @apply max-h-60 overflow-y-auto py-1 outline-none;
}

.select-dropdown-portal .select-option {
  @apply flex items-center justify-between gap-2;
  @apply px-4 py-2.5 text-sm;
  @apply text-gray-700 dark:text-gray-300;
  @apply cursor-pointer transition-colors duration-150;
  @apply hover:bg-gray-50 dark:hover:bg-dark-700;
  pointer-events: auto !important;
}

.select-dropdown-portal .select-option-selected {
  @apply bg-primary-50 dark:bg-primary-900/20;
  @apply text-primary-700 dark:text-primary-300;
}

.select-dropdown-portal .select-option-focused {
  @apply bg-gray-100 dark:bg-dark-700;
}

.select-dropdown-portal .select-option-disabled {
  @apply cursor-not-allowed opacity-40;
}

.select-dropdown-portal .select-option-group {
  @apply cursor-default select-none;
  @apply bg-gray-50 dark:bg-dark-900;
  @apply text-[11px] font-bold uppercase tracking-wider;
  @apply text-gray-500 dark:text-gray-400;
}

.select-dropdown-portal .select-option-group:hover {
  @apply bg-gray-50 dark:bg-dark-900;
}

.select-dropdown-portal .select-option-label {
  @apply flex-1 min-w-0 truncate text-left;
}

.select-dropdown-portal .select-empty {
  @apply px-4 py-8 text-center text-sm;
  @apply text-gray-500 dark:text-dark-400;
}

.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition: all 0.2s ease;
}

.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
