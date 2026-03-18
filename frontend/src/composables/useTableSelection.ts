import { computed, ref, type Ref } from 'vue'

interface UseTableSelectionOptions<T> {
  rows: Ref<T[]>
  getId: (row: T) => number
}

export function useTableSelection<T>({ rows, getId }: UseTableSelectionOptions<T>) {
  const selectedSet = ref<Set<number>>(new Set())

  const selectedIds = computed(() => Array.from(selectedSet.value))
  const selectedCount = computed(() => selectedSet.value.size)

  const isSelected = (id: number) => selectedSet.value.has(id)

  const replaceSelectedSet = (next: Set<number>) => {
    selectedSet.value = next
  }

  const setSelectedIds = (ids: number[]) => {
    selectedSet.value = new Set(ids)
  }

  const select = (id: number) => {
    if (selectedSet.value.has(id)) return
    const next = new Set(selectedSet.value)
    next.add(id)
    replaceSelectedSet(next)
  }

  const deselect = (id: number) => {
    if (!selectedSet.value.has(id)) return
    const next = new Set(selectedSet.value)
    next.delete(id)
    replaceSelectedSet(next)
  }

  const toggle = (id: number) => {
    if (selectedSet.value.has(id)) {
      deselect(id)
      return
    }
    select(id)
  }

  const clear = () => {
    if (selectedSet.value.size === 0) return
    replaceSelectedSet(new Set())
  }

  const removeMany = (ids: number[]) => {
    if (ids.length === 0 || selectedSet.value.size === 0) return
    const next = new Set(selectedSet.value)
    let changed = false
    ids.forEach((id) => {
      if (next.delete(id)) changed = true
    })
    if (changed) replaceSelectedSet(next)
  }

  const allVisibleSelected = computed(() => {
    if (rows.value.length === 0) return false
    return rows.value.every((row) => selectedSet.value.has(getId(row)))
  })

  const toggleVisible = (checked: boolean) => {
    const next = new Set(selectedSet.value)
    rows.value.forEach((row) => {
      const id = getId(row)
      if (checked) {
        next.add(id)
      } else {
        next.delete(id)
      }
    })
    replaceSelectedSet(next)
  }

  const batchUpdate = (updater: (draft: Set<number>) => void) => {
    const draft = new Set(selectedSet.value)
    updater(draft)
    replaceSelectedSet(draft)
  }

  const selectVisible = () => {
    toggleVisible(true)
  }

  return {
    selectedSet,
    selectedIds,
    selectedCount,
    allVisibleSelected,
    isSelected,
    setSelectedIds,
    select,
    deselect,
    toggle,
    clear,
    removeMany,
    toggleVisible,
    selectVisible,
    batchUpdate
  }
}
