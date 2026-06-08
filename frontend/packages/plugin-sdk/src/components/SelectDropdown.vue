<template>
  <div
    ref="dropdownRef"
    class="select-dropdown-portal"
    :class="[instanceId]"
    :style="dropdownStyle"
    role="listbox"
    @click.stop
    @mousedown.stop
    @keydown="onDropdownKeyDown"
  >
    <!-- Search input -->
    <div v-if="searchable" class="select-search">
      <Icon name="search" size="sm" class="text-gray-400" />
      <input
        ref="searchInputRef"
        :value="searchQuery"
        @input="$emit('update:searchQuery', ($event.target as HTMLInputElement).value)"
        type="text"
        :placeholder="searchPlaceholderText"
        class="select-search-input"
        @click.stop
      />
    </div>

    <!-- Options list -->
    <div class="select-options" ref="optionsListRef">
      <div
        v-for="(option, index) in filteredOptions"
        :key="`${typeof getValue(option)}:${String(getValue(option) ?? '')}`"
        role="option"
        :aria-selected="isSelected(option)"
        :aria-disabled="isOptionDisabled(option)"
        @click.stop="!isOptionDisabled(option) && $emit('select', option)"
        @mouseenter="$emit('mouseenter', option, index)"
        :class="[
          'select-option',
          isGroupHeaderOption(option) && 'select-option-group',
          isSelected(option) && 'select-option-selected',
          isOptionDisabled(option) && !isGroupHeaderOption(option) && 'select-option-disabled',
          focusedIndex === index && !isGroupHeaderOption(option) && 'select-option-focused'
        ]"
      >
        <slot name="option" :option="option" :selected="isSelected(option)">
          <Icon
            v-if="(option as Record<string, unknown>)._creatable"
            name="search"
            size="sm"
            class="flex-shrink-0 text-gray-400"
          />
          <span
            class="select-option-label"
            :class="(option as Record<string, unknown>)._creatable && 'italic text-gray-500 dark:text-dark-300'"
          >{{ getLabel(option) }}</span>
          <Icon
            v-if="isSelected(option)"
            name="check"
            size="sm"
            class="text-primary-500"
            :stroke-width="2"
          />
        </slot>
      </div>

      <!-- Empty state -->
      <div v-if="filteredOptions.length === 0" class="select-empty">
        {{ emptyTextDisplay }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import Icon from './Icon.vue'
import { isOptionDisabled, isGroupHeaderOption } from '../utils/selectHelpers'

defineEmits<{
  select: [option: Record<string, unknown>]
  mouseenter: [option: Record<string, unknown>, index: number]
  'update:searchQuery': [value: string]
}>()

interface Props {
  filteredOptions: Array<Record<string, unknown>>
  searchable: boolean
  searchQuery: string
  searchPlaceholderText: string
  emptyTextDisplay: string
  focusedIndex: number
  instanceId: string
  dropdownStyle: Record<string, string>
  isSelected: (option: Record<string, unknown> | unknown) => boolean
  getValue: (option: Record<string, unknown> | unknown) => unknown
  getLabel: (option: Record<string, unknown> | unknown) => string
  onDropdownKeyDown: (e: KeyboardEvent) => void
}

defineProps<Props>()

const searchInputRef = ref<HTMLInputElement | null>(null)
const optionsListRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)

defineExpose({ searchInputRef, optionsListRef, dropdownRef })
</script>
