<template>
  <div
    ref="containerRef"
    class="filter-menu relative"
    :class="compact ? 'min-w-[6.5rem] sm:min-w-[7.25rem]' : 'min-w-[150px] sm:min-w-[160px]'"
  >
    <button
      ref="triggerRef"
      type="button"
      class="select-trigger flex cursor-pointer list-none items-center justify-between gap-1.5 text-left"
      :class="[
        isOpen ? 'select-trigger-open' : '',
        compact ? 'h-8 rounded-lg !px-2 !py-1 text-xs' : 'h-[42px]',
      ]"
      :aria-expanded="isOpen"
      aria-haspopup="listbox"
      :aria-label="label"
      @click="toggleOpen"
    >
      <span
        class="select-value min-w-0 truncate"
        :class="compact ? 'max-w-[5.5rem] sm:max-w-[6.5rem]' : 'max-w-[11rem]'"
      >
        {{ t('channelMonitorV2.filters.labelValue', { label, value: selectionLabel }) }}
      </span>
      <span class="select-icon shrink-0 text-gray-400 transition-transform" :class="isOpen ? 'rotate-180' : ''">
        <Icon name="chevronDown" size="sm" />
      </span>
    </button>

    <Teleport to="body">
      <Transition name="select-dropdown">
        <div
          v-if="isOpen"
          ref="dropdownRef"
          class="select-dropdown-portal dropdown filter-dropdown"
          :class="[instanceId]"
          :style="dropdownStyle"
          role="listbox"
          aria-multiselectable="true"
          @click.stop
          @mousedown.stop
        >
          <button
            type="button"
            class="dropdown-item select-option select-option-group flex w-full items-center justify-between border-b border-gray-100 px-4 py-2 text-left text-sm font-semibold text-gray-700 hover:bg-gray-100 dark:border-dark-700 dark:text-gray-300 dark:hover:bg-dark-700"
            @click="clear"
          >
            <span>{{ allLabel }}</span>
            <Icon v-if="modelValue.length === 0" name="check" size="sm" class="text-primary-500" />
          </button>

          <button
            v-for="option in options"
            :key="option.value"
            type="button"
            role="option"
            class="dropdown-item select-option flex w-full items-center justify-between gap-3 px-4 py-2 text-left text-sm text-gray-700 hover:bg-gray-100 dark:text-gray-300 dark:hover:bg-dark-700"
            :class="modelValue.includes(option.value) ? 'select-option-selected' : ''"
            :aria-selected="modelValue.includes(option.value)"
            @click="toggle(option.value)"
          >
            <span class="flex min-w-0 flex-1 items-center gap-2">
              <span
                class="checkbox flex h-4 w-4 items-center justify-center rounded border border-gray-300 bg-white text-primary-500 dark:border-dark-600 dark:bg-dark-900"
                :class="modelValue.includes(option.value) ? 'border-primary-500 bg-primary-50 dark:bg-primary-900/30' : ''"
              >
                <Icon v-if="modelValue.includes(option.value)" name="check" size="sm" class="text-primary-500" />
              </span>
              <span class="min-w-0 flex-1 truncate">{{ option.label }}</span>
            </span>
            <small v-if="option.count != null" class="text-xs text-gray-400">{{ formatCount(option.count) }}</small>
          </button>
          <p v-if="options.length === 0" class="px-4 py-3 text-center text-xs text-gray-400">{{ t('channelMonitorV2.filters.empty') }}</p>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import type { CSSProperties } from 'vue'
import { useI18n } from 'vue-i18n'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { monitorIntlLocale } from '@/features/channel-monitor-v2/monitorFormat'

interface FilterOption {
  value: string
  label: string
  count?: number
}

const props = withDefaults(
  defineProps<{
    label: string
    allLabel: string
    modelValue: string[]
    /** Options for this picker (parent may cascade by platform). */
    options: FilterOption[]
    /** Compact trigger for single-row toolbars. */
    compact?: boolean
  }>(),
  { compact: false },
)
const emit = defineEmits<{ 'update:modelValue': [value: string[]] }>()
const { t, locale } = useI18n()

const containerRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLButtonElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const isOpen = ref(false)
const instanceId = `filter-select-${Math.random().toString(36).slice(2, 9)}`

const selectionLabel = computed(() => {
  if (props.modelValue.length === 0) return props.allLabel
  if (props.modelValue.length === 1) {
    return props.options.find((item) => item.value === props.modelValue[0])?.label || props.modelValue[0]
  }
  return t('channelMonitorV2.filters.selectedCount', { count: props.modelValue.length })
})

const dropdownStyle = computed<CSSProperties>(() => {
  const trigger = triggerRef.value
  if (!trigger) return {}
  const rect = trigger.getBoundingClientRect()
  const padding = 8
  const viewportRight = Math.max(padding, window.innerWidth - padding)
  const left = Math.min(Math.max(padding, rect.left), viewportRight)
  const availableWidth = Math.max(0, viewportRight - left)
  const preferredMinWidth = Math.max(200, rect.width)
  const minWidth = Math.min(preferredMinWidth, availableWidth)
  return {
    position: 'fixed' as const,
    left: `${left}px`,
    top: `${rect.bottom + 4}px`,
    minWidth: `${minWidth}px`,
    maxWidth: `${availableWidth}px`,
    zIndex: '100000020',
  }
})

function clear() {
  emit('update:modelValue', [])
  // Stay open so users can re-select without reopening.
}

function toggle(value: string) {
  const selected = new Set(props.modelValue)
  if (selected.has(value)) selected.delete(value)
  else selected.add(value)
  emit('update:modelValue', [...selected])
  // Stay open on toggle (multi-select).
}

function toggleOpen() {
  isOpen.value ? close() : open()
}

function open() {
  isOpen.value = true
  void nextTick(() => positionDropdown())
}

function close() {
  isOpen.value = false
}

function positionDropdown() {
  const trigger = triggerRef.value
  const dropdown = dropdownRef.value
  if (!trigger || !dropdown) return
  const rect = trigger.getBoundingClientRect()
  const padding = 8
  const viewportRight = Math.max(padding, window.innerWidth - padding)
  const left = Math.min(Math.max(padding, rect.left), viewportRight)
  const availableWidth = Math.max(0, viewportRight - left)
  const preferredMinWidth = Math.max(200, rect.width)
  const minWidth = Math.min(preferredMinWidth, availableWidth)
  dropdown.style.left = `${left}px`
  dropdown.style.top = `${rect.bottom + 4}px`
  dropdown.style.minWidth = `${minWidth}px`
  dropdown.style.maxWidth = `${availableWidth}px`
}

function formatCount(value: number) {
  return Intl.NumberFormat(locale.value || monitorIntlLocale(), {
    notation: value >= 10000 ? 'compact' : 'standard',
  }).format(value)
}

function onDocumentMouseDown(event: MouseEvent) {
  if (!isOpen.value) return
  const target = event.target as Node | null
  if (!target) return
  if (containerRef.value?.contains(target)) return
  if (dropdownRef.value?.contains(target)) return
  close()
}

function onDocumentKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape') close()
}

function onWindowChange() {
  if (isOpen.value) positionDropdown()
}

watch(isOpen, async (open) => {
  if (open) {
    await nextTick()
    positionDropdown()
    document.addEventListener('mousedown', onDocumentMouseDown)
    document.addEventListener('keydown', onDocumentKeyDown)
    window.addEventListener('resize', onWindowChange)
    window.addEventListener('scroll', onWindowChange, true)
  } else {
    document.removeEventListener('mousedown', onDocumentMouseDown)
    document.removeEventListener('keydown', onDocumentKeyDown)
    window.removeEventListener('resize', onWindowChange)
    window.removeEventListener('scroll', onWindowChange, true)
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentMouseDown)
  document.removeEventListener('keydown', onDocumentKeyDown)
  window.removeEventListener('resize', onWindowChange)
  window.removeEventListener('scroll', onWindowChange, true)
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

.filter-menu summary::-webkit-details-marker {
  display: none;
}

.filter-dropdown {
  @apply w-max min-w-[200px] max-h-[min(50vh,360px)] overflow-y-auto rounded-xl border border-gray-200 bg-white shadow-lg dark:border-dark-600 dark:bg-dark-800;
}

.dropdown-item {
  @apply cursor-pointer;
}

.checkbox {
  flex: none;
}

@media (max-width: 640px) {
  .filter-menu {
    min-width: 100%;
  }
}
</style>
