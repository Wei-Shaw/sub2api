<template>
  <div class="relative" ref="containerRef">
    <button
      type="button"
      @click="toggle"
      :disabled="disabled"
      :class="[
        'select-trigger',
        isOpen && 'select-trigger-open',
        error && 'select-trigger-error',
        disabled && 'select-trigger-disabled'
      ]"
    >
      <span class="select-value">
        <slot name="selected" :option="selectedOption">
          {{ selectedLabel REDACTEDREDACTED
        </slot>
      </span>
      <span class="select-icon">
        <svg
          :class="['h-5 w-5 transition-transform duration-200', isOpen && 'rotate-180']"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
          stroke-width="1.5"
        >
          <path stroke-linecap="round" stroke-linejoin="round" d="M19.5 8.25l-7.5 7.5-7.5-7.5" />
        </svg>
      </span>
    </button>

    <Transition name="select-dropdown">
      <div v-if="isOpen" class="select-dropdown">
        <!-- Search input -->
        <div v-if="searchable" class="select-search">
          <svg
            class="h-4 w-4 text-gray-400"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            stroke-width="1.5"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              d="M21 21l-5.197-5.197m0 0A7.5 7.5 0 105.196 5.196a7.5 7.5 0 0010.607 10.607z"
            />
          </svg>
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            type="text"
            :placeholder="searchPlaceholderText"
            class="select-search-input"
            @click.stop
          />
        </div>

        <!-- Options list -->
        <div class="select-options">
          <div
            v-for="option in filteredOptions"
            :key="getOptionValue(option) ?? undefined"
            @click="selectOption(option)"
            :class="['select-option', isSelected(option) && 'select-option-selected']"
          >
            <slot name="option" :option="option" :selected="isSelected(option)">
              <span class="select-option-label">{{ getOptionLabel(option) REDACTEDREDACTED</span>
              <svg
                v-if="isSelected(option)"
                class="h-4 w-4 text-primary-500"
                fill="none"
                stroke="currentColor"
                viewBox="0 0 24 24"
                stroke-width="2"
              >
                <path stroke-linecap="round" stroke-linejoin="round" d="M4.5 12.75l6 6 9-13.5" />
              </svg>
            </slot>
          </div>

          <!-- Empty state -->
          <div v-if="filteredOptions.length === 0" class="select-empty">
            {{ emptyTextDisplay REDACTEDREDACTED
          </div>
        </div>
      </div>
    </Transition>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'

const { t REDACTED = useI18n()

export interface SelectOption {
  value: string | number | null
  label: string
  disabled?: boolean
  [key: string]: unknown
REDACTED

interface Props {
  modelValue: string | number | null | undefined
  options: SelectOption[] | Array<Record<string, unknown>>
  placeholder?: string
  disabled?: boolean
  error?: boolean
  searchable?: boolean
  searchPlaceholder?: string
  emptyText?: string
  valueKey?: string
  labelKey?: string
REDACTED

interface Emits {
  (e: 'update:modelValue', value: string | number | null): void
  (e: 'change', value: string | number | null, option: SelectOption | null): void
REDACTED

const props = withDefaults(defineProps<Props>(), {
  disabled: false,
  error: false,
  searchable: false,
  valueKey: 'value',
  labelKey: 'label'
REDACTED)

// Use computed for i18n default values
const placeholderText = computed(() => props.placeholder ?? t('common.selectOption'))
const searchPlaceholderText = computed(
  () => props.searchPlaceholder ?? t('common.searchPlaceholder')
)
const emptyTextDisplay = computed(() => props.emptyText ?? t('common.noOptionsFound'))

const emit = defineEmits<Emits>()

const isOpen = ref(false)
const searchQuery = ref('')
const containerRef = ref<HTMLElement | null>(null)
const searchInputRef = ref<HTMLInputElement | null>(null)

const getOptionValue = (
  option: SelectOption | Record<string, unknown>
): string | number | null | undefined => {
  if (typeof option === 'object' && option !== null) {
    return option[props.valueKey] as string | number | null | undefined
  REDACTED
  return option as string | number | null
REDACTED

const getOptionLabel = (option: SelectOption | Record<string, unknown>): string => {
  if (typeof option === 'object' && option !== null) {
    return String(option[props.labelKey] ?? '')
  REDACTED
  return String(option ?? '')
REDACTED

const selectedOption = computed(() => {
  return props.options.find((opt) => getOptionValue(opt) === props.modelValue) || null
REDACTED)

const selectedLabel = computed(() => {
  if (selectedOption.value) {
    return getOptionLabel(selectedOption.value)
  REDACTED
  return placeholderText.value
REDACTED)

const filteredOptions = computed(() => {
  if (!props.searchable || !searchQuery.value) {
    return props.options
  REDACTED
  const query = searchQuery.value.toLowerCase()
  return props.options.filter((opt) => {
    const label = getOptionLabel(opt).toLowerCase()
    return label.includes(query)
  REDACTED)
REDACTED)

const isSelected = (option: SelectOption | Record<string, unknown>): boolean => {
  return getOptionValue(option) === props.modelValue
REDACTED

const toggle = () => {
  if (props.disabled) return
  isOpen.value = !isOpen.value
  if (isOpen.value && props.searchable) {
    nextTick(() => {
      searchInputRef.value?.focus()
    REDACTED)
  REDACTED
REDACTED

const selectOption = (option: SelectOption | Record<string, unknown>) => {
  const value = getOptionValue(option) ?? null
  emit('update:modelValue', value)
  emit('change', value, option as SelectOption)
  isOpen.value = false
  searchQuery.value = ''
REDACTED

const handleClickOutside = (event: MouseEvent) => {
  if (containerRef.value && !containerRef.value.contains(event.target as Node)) {
    isOpen.value = false
    searchQuery.value = ''
  REDACTED
REDACTED

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && isOpen.value) {
    isOpen.value = false
    searchQuery.value = ''
  REDACTED
REDACTED

watch(isOpen, (open) => {
  if (!open) {
    searchQuery.value = ''
  REDACTED
REDACTED)

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
  document.addEventListener('keydown', handleEscape)
REDACTED)

onUnmounted(() => {
  document.removeEventListener('click', handleClickOutside)
  document.removeEventListener('keydown', handleEscape)
REDACTED)
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
REDACTED

.select-trigger-open {
  @apply border-primary-500 ring-2 ring-primary-500/30;
REDACTED

.select-trigger-error {
  @apply border-red-500 focus:border-red-500 focus:ring-red-500/30;
REDACTED

.select-trigger-disabled {
  @apply cursor-not-allowed bg-gray-100 opacity-60 dark:bg-dark-900;
REDACTED

.select-value {
  @apply flex-1 truncate text-left;
REDACTED

.select-icon {
  @apply flex-shrink-0 text-gray-400 dark:text-dark-400;
REDACTED

.select-dropdown {
  @apply absolute z-[100] mt-2 w-full;
  @apply bg-white dark:bg-dark-800;
  @apply rounded-xl;
  @apply border border-gray-200 dark:border-dark-700;
  @apply shadow-lg shadow-black/10 dark:shadow-black/30;
  @apply overflow-hidden;
REDACTED

.select-search {
  @apply flex items-center gap-2 px-3 py-2;
  @apply border-b border-gray-100 dark:border-dark-700;
REDACTED

.select-search-input {
  @apply flex-1 bg-transparent text-sm;
  @apply text-gray-900 dark:text-gray-100;
  @apply placeholder:text-gray-400 dark:placeholder:text-dark-400;
  @apply focus:outline-none;
REDACTED

.select-options {
  @apply max-h-60 overflow-y-auto py-1;
REDACTED

.select-option {
  @apply flex items-center justify-between gap-2;
  @apply px-4 py-2.5 text-sm;
  @apply text-gray-700 dark:text-gray-300;
  @apply cursor-pointer transition-colors duration-150;
  @apply hover:bg-gray-50 dark:hover:bg-dark-700;
REDACTED

.select-option-selected {
  @apply bg-primary-50 dark:bg-primary-900/20;
  @apply text-primary-700 dark:text-primary-300;
REDACTED

.select-option-label {
  @apply truncate;
REDACTED

.select-empty {
  @apply px-4 py-8 text-center text-sm;
  @apply text-gray-500 dark:text-dark-400;
REDACTED

/* Dropdown animation */
.select-dropdown-enter-active,
.select-dropdown-leave-active {
  transition: all 0.2s ease;
REDACTED

.select-dropdown-enter-from,
.select-dropdown-leave-to {
  opacity: 0;
  transform: translateY(-8px);
REDACTED
</style>
