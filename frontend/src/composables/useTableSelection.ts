import { computed, ref, type Ref REDACTED from 'vue'

interface UseTableSelectionOptions<T> {
  rows: Ref<T[]>
  getId: (row: T) => number
REDACTED

export function useTableSelection<T>({ rows, getId REDACTED: UseTableSelectionOptions<T>) {
  const selectedSet = ref<Set<number>>(new Set())

  const selectedIds = computed(() => Array.from(selectedSet.value))
  const selectedCount = computed(() => selectedSet.value.size)

  const isSelected = (id: number) => selectedSet.value.has(id)

  const replaceSelectedSet = (next: Set<number>) => {
    selectedSet.value = next
  REDACTED

  const setSelectedIds = (ids: number[]) => {
    selectedSet.value = new Set(ids)
  REDACTED

  const select = (id: number) => {
    if (selectedSet.value.has(id)) return
    const next = new Set(selectedSet.value)
    next.add(id)
    replaceSelectedSet(next)
  REDACTED

  const deselect = (id: number) => {
    if (!selectedSet.value.has(id)) return
    const next = new Set(selectedSet.value)
    next.delete(id)
    replaceSelectedSet(next)
  REDACTED

  const toggle = (id: number) => {
    if (selectedSet.value.has(id)) {
      deselect(id)
      return
    REDACTED
    select(id)
  REDACTED

  const clear = () => {
    if (selectedSet.value.size === 0) return
    replaceSelectedSet(new Set())
  REDACTED

  const removeMany = (ids: number[]) => {
    if (ids.length === 0 || selectedSet.value.size === 0) return
    const next = new Set(selectedSet.value)
    let changed = false
    ids.forEach((id) => {
      if (next.delete(id)) changed = true
    REDACTED)
    if (changed) replaceSelectedSet(next)
  REDACTED

  const allVisibleSelected = computed(() => {
    if (rows.value.length === 0) return false
    return rows.value.every((row) => selectedSet.value.has(getId(row)))
  REDACTED)

  const toggleVisible = (checked: boolean) => {
    const next = new Set(selectedSet.value)
    rows.value.forEach((row) => {
      const id = getId(row)
      if (checked) {
        next.add(id)
      REDACTED else {
        next.delete(id)
      REDACTED
    REDACTED)
    replaceSelectedSet(next)
  REDACTED

  const selectVisible = () => {
    toggleVisible(true)
  REDACTED

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
    selectVisible
  REDACTED
REDACTED
