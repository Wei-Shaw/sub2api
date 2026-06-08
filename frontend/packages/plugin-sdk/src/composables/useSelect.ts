/**
 * Composable: filtering, keyboard navigation, dropdown positioning,
 * and open/close lifecycle for the Select component.
 */
import { ref, computed, watch, onMounted, onUnmounted, nextTick, type Ref } from 'vue'
import type { SelectOption } from '../types'
import { getOptionValue, getOptionLabel, isOptionDisabled, isGroupHeaderOption } from '../utils/selectHelpers'

// Re-export so consumers can import from a single module
// Re-export so consumers can import from a single module
export { isOptionDisabled, isGroupHeaderOption } from '../utils/selectHelpers'

export type SelectValue = string | number | boolean | null | undefined

export interface UseSelectOptions {
  modelValue: Ref<SelectValue>
  options: Ref<SelectOption[] | Array<Record<string, unknown>>>
  disabled?: boolean
  searchable?: boolean
  creatable?: boolean
  creatablePrefix?: string
  valueKey?: string
  labelKey?: string
  placeholderText: Ref<string>
  searchPlaceholderText: Ref<string>
  emptyTextDisplay: Ref<string>
  containerRef: Ref<HTMLElement | null>
  triggerRef: Ref<HTMLButtonElement | null>
  searchInputRef: Ref<HTMLInputElement | null>
  dropdownRef: Ref<HTMLElement | null>
  optionsListRef: Ref<HTMLElement | null>
  onUpdate: (value: string | number | boolean | null) => void
  onChange: (value: string | number | boolean | null, option: SelectOption | null) => void
  searchLabel: Ref<string>
}

// ── Composable ─────────────────────────────────────────────────────────

export function useSelect(opts: UseSelectOptions) {
  const isOpen = ref(false)
  const searchQuery = ref('')
  const focusedIndex = ref(-1)
  const dropdownPosition = ref<'bottom' | 'top'>('bottom')
  const triggerRect = ref<DOMRect | null>(null)
  const vk = opts.valueKey ?? 'value'
  const lk = opts.labelKey ?? 'label'
  const instanceId = `select-${Math.random().toString(36).substring(2, 9)}`

  const getValue = (o: Record<string, unknown> | unknown) => getOptionValue(o, vk)
  const getLabel = (o: Record<string, unknown> | unknown) => getOptionLabel(o, lk)

  const selectedOption = computed(() =>
    opts.options.value.find(o => getValue(o) === opts.modelValue.value) ?? null,
  )
  const selectedLabel = computed(() => {
    if (selectedOption.value) return getLabel(selectedOption.value)
    if (opts.creatable && opts.modelValue.value) return String(opts.modelValue.value)
    return opts.placeholderText.value
  })

  const filteredOptions = computed(() => {
    let list = opts.options.value as Array<Record<string, unknown>>
    if (opts.searchable && searchQuery.value) {
      const query = searchQuery.value.toLowerCase()
      list = list.filter(o => {
        if (getLabel(o).toLowerCase().includes(query)) return true
        if (o.description && String(o.description).toLowerCase().includes(query)) return true
        return false
      })
      if (opts.creatable && searchQuery.value.trim()) {
        const trimmed = searchQuery.value.trim()
        const prefix = opts.creatablePrefix || opts.searchLabel.value
        list = [{ [vk]: trimmed, [lk]: `${prefix} "${trimmed}"`, _creatable: true }, ...list]
      }
    }
    return list
  })

  const isSelected = (option: Record<string, unknown> | unknown): boolean =>
    getValue(option) === opts.modelValue.value

  const findNextEnabled = (start: number): number => {
    const arr = filteredOptions.value
    if (!arr.length) return -1
    for (let off = 0; off < arr.length; off++) {
      const idx = (start + off) % arr.length
      if (!isOptionDisabled(arr[idx])) return idx
    }
    return -1
  }
  const findPrevEnabled = (start: number): number => {
    const arr = filteredOptions.value
    if (!arr.length) return -1
    for (let off = 0; off < arr.length; off++) {
      const idx = (start - off + arr.length) % arr.length
      if (!isOptionDisabled(arr[idx])) return idx
    }
    return -1
  }
  const handleOptionMouseEnter = (opt: Record<string, unknown> | unknown, idx: number) => {
    if (!isOptionDisabled(opt) && !isGroupHeaderOption(opt)) focusedIndex.value = idx
  }

  const updateTriggerRect = () => {
    if (opts.containerRef.value) triggerRect.value = opts.containerRef.value.getBoundingClientRect()
  }
  const dropdownStyle = computed(() => {
    if (!triggerRect.value) return {}
    const r = triggerRect.value
    const s: Record<string, string> = {
      position: 'fixed', left: `${r.left}px`, minWidth: `${r.width}px`, zIndex: '100000020',
    }
    if (dropdownPosition.value === 'top') s.bottom = `${window.innerHeight - r.top + 4}px`
    else s.top = `${r.bottom + 4}px`
    return s
  })
  const calculateDropdownPosition = () => {
    if (!opts.containerRef.value) return
    updateTriggerRect()
    nextTick(() => {
      if (!opts.dropdownRef.value || !triggerRect.value) return
      const h = opts.dropdownRef.value.offsetHeight || 240
      const below = window.innerHeight - triggerRect.value.bottom
      dropdownPosition.value = below < h && triggerRect.value.top > h ? 'top' : 'bottom'
    })
  }

  const toggle = () => { if (!opts.disabled) isOpen.value = !isOpen.value }
  const selectOption = (option: Record<string, unknown> | unknown) => {
    const value = (getValue(option) ?? null) as string | number | boolean | null
    opts.onUpdate(value)
    opts.onChange(value, option as SelectOption | null)
    isOpen.value = false
    opts.triggerRef.value?.focus()
  }

  const scrollToFocused = () => {
    nextTick(() => {
      const list = opts.optionsListRef.value
      if (!list) return
      const el = list.children[focusedIndex.value] as HTMLElement | undefined
      if (!el) return
      if (el.offsetTop < list.scrollTop) list.scrollTop = el.offsetTop
      else if (el.offsetTop + el.offsetHeight > list.scrollTop + list.offsetHeight)
        list.scrollTop = el.offsetTop + el.offsetHeight - list.offsetHeight
    })
  }
  const onTriggerKeyDown = () => { if (!isOpen.value) isOpen.value = true }
  const onDropdownKeyDown = (e: KeyboardEvent) => {
    switch (e.key) {
      case 'ArrowDown': e.preventDefault(); focusedIndex.value = findNextEnabled(focusedIndex.value + 1); if (focusedIndex.value >= 0) scrollToFocused(); break
      case 'ArrowUp':   e.preventDefault(); focusedIndex.value = findPrevEnabled(focusedIndex.value - 1); if (focusedIndex.value >= 0) scrollToFocused(); break
      case 'Enter':     e.preventDefault(); { const opt = filteredOptions.value[focusedIndex.value]; if (opt && !isOptionDisabled(opt)) selectOption(opt) } break
      case 'Escape':    e.preventDefault(); isOpen.value = false; opts.triggerRef.value?.focus(); break
      case 'Tab':       isOpen.value = false; break
    }
  }

  // ── click outside ──
  const handleClickOutside = (event: MouseEvent) => {
    const target = event.target as HTMLElement
    if (!target.closest(`.${instanceId}`) && !opts.containerRef.value?.contains(target) && isOpen.value) {
      isOpen.value = false
    }
  }

  // ── watchers ──
  watch(isOpen, (open) => {
    if (open) {
      calculateDropdownPosition()
      const arr = filteredOptions.value
      if (!arr.length) { focusedIndex.value = -1 } else {
        const si = arr.findIndex(isSelected)
        const ii = si >= 0 ? si : 0
        focusedIndex.value = isOptionDisabled(arr[ii]) ? findNextEnabled(ii + 1) : ii
      }
      if (opts.searchable) nextTick(() => opts.searchInputRef.value?.focus())
      window.addEventListener('scroll', updateTriggerRect, { capture: true, passive: true })
      window.addEventListener('resize', calculateDropdownPosition)
    } else {
      searchQuery.value = ''; focusedIndex.value = -1
      window.removeEventListener('scroll', updateTriggerRect, { capture: true } as EventListenerOptions)
      window.removeEventListener('resize', calculateDropdownPosition)
    }
  })

  onMounted(() => document.addEventListener('click', handleClickOutside))
  onUnmounted(() => {
    document.removeEventListener('click', handleClickOutside)
    window.removeEventListener('scroll', updateTriggerRect, { capture: true } as EventListenerOptions)
    window.removeEventListener('resize', calculateDropdownPosition)
  })

  return {
    isOpen, searchQuery, focusedIndex, instanceId,
    selectedOption, selectedLabel, filteredOptions, dropdownStyle,
    isSelected, toggle, selectOption, handleOptionMouseEnter,
    onTriggerKeyDown, onDropdownKeyDown, getValue, getLabel,
  }
}
