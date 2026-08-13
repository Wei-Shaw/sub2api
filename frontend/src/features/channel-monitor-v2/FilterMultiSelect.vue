<template>
  <div
    ref="containerRef"
    class="filter-menu relative"
    :class="compact ? 'min-w-[6.5rem] sm:min-w-[7.25rem]' : 'min-w-[150px] sm:min-w-[160px]'"
  >
    <button
      ref="triggerRef"
      type="button"
      class="select-trigger flex w-full cursor-pointer list-none items-center justify-between gap-1.5 rounded border bg-surface px-2 text-left text-ink transition-colors duration-fast ease-out hover:border-line-strong"
      :class="[
        isOpen ? 'border-accent' : 'border-line',
        compact ? 'h-7 text-xs' : 'h-9 px-3 text-sm',
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
      <span
        class="select-icon shrink-0 text-ink-tertiary transition-transform duration-fast"
        :class="isOpen ? 'rotate-180' : ''"
      >
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
          <!-- Accent marks SELECTION here and nothing else — never a status. -->
          <button
            type="button"
            class="dropdown-item select-option select-option-group w-full justify-between border-b border-line font-medium"
            :class="modelValue.length === 0 ? 'text-accent' : ''"
            @click="clear"
          >
            <span class="truncate">{{ allLabel }}</span>
            <Icon v-if="modelValue.length === 0" name="check" size="xs" class="shrink-0" />
          </button>

          <button
            v-for="option in options"
            :key="option.value"
            type="button"
            role="option"
            class="dropdown-item select-option w-full justify-between gap-3"
            :class="modelValue.includes(option.value) ? 'select-option-selected text-accent' : ''"
            :aria-selected="modelValue.includes(option.value)"
            @click="toggle(option.value)"
          >
            <span class="flex min-w-0 flex-1 items-center gap-2">
              <span
                class="checkbox flex h-3.5 w-3.5 items-center justify-center rounded-sm border"
                :class="modelValue.includes(option.value)
                  ? 'border-accent-solid bg-accent-solid text-accent-on'
                  : 'border-line bg-surface'"
              >
                <Icon v-if="modelValue.includes(option.value)" name="check" size="xs" />
              </span>
              <span class="min-w-0 flex-1 truncate">{{ option.label }}</span>
            </span>
            <NumCell v-if="option.count != null" :value="option.count" compact />
          </button>
          <p v-if="options.length === 0" class="px-3 py-3 text-center text-xs text-ink-tertiary">
            {{ t('channelMonitorV2.filters.empty') }}
          </p>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import type { CSSProperties } from 'vue'
import { useI18n } from 'vue-i18n'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import NumCell from '@/components/common/NumCell.vue'
import Icon from '@/components/icons/Icon.vue'

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
const { t } = useI18n()

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
/*
 * The trigger chrome moved into the template so it reads as tokens rather than
 * an `@apply` block that duplicated `.input`. What went away with it: a
 * `focus:ring-2 ring-primary-500/30` (focus is the global `outline` in
 * style.css, which an `overflow: hidden` ancestor cannot clip), a
 * `transition: all`, and five hand-written `dark:` pairs.
 *
 * `.dropdown` in style.css already supplies the surface, hairline and popover
 * elevation.
 */
.filter-menu summary::-webkit-details-marker {
  display: none;
}

.filter-dropdown {
  @apply max-h-[min(50vh,360px)] w-max min-w-[200px] overflow-y-auto;
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
