import { ref, onMounted, onUnmounted, type Ref REDACTED from 'vue'

export interface SwipeSelectAdapter {
  isSelected: (id: number) => boolean
  select: (id: number) => void
  deselect: (id: number) => void
REDACTED

export function useSwipeSelect(
  containerRef: Ref<HTMLElement | null>,
  adapter: SwipeSelectAdapter
) {
  const isDragging = ref(false)

  let dragMode: 'select' | 'deselect' = 'select'
  let startRowIndex = -1
  let lastEndIndex = -1
  let startY = 0
  let initialSelectedSnapshot = new Map<number, boolean>()
  let cachedRows: HTMLElement[] = []
  let marqueeEl: HTMLDivElement | null = null

  function getDataRows(): HTMLElement[] {
    const container = containerRef.value
    if (!container) return []
    return Array.from(container.querySelectorAll('tbody tr[data-row-id]'))
  REDACTED

  function getRowId(el: HTMLElement): number | null {
    const raw = el.getAttribute('data-row-id')
    if (raw === null) return null
    const id = Number(raw)
    return Number.isFinite(id) ? id : null
  REDACTED

  // --- Marquee overlay ---
  function createMarquee() {
    marqueeEl = document.createElement('div')
    const isDark = document.documentElement.classList.contains('dark')
    Object.assign(marqueeEl.style, {
      position: 'fixed',
      background: isDark ? 'rgba(96, 165, 250, 0.15)' : 'rgba(59, 130, 246, 0.12)',
      border: isDark ? '1.5px solid rgba(96, 165, 250, 0.5)' : '1.5px solid rgba(59, 130, 246, 0.4)',
      borderRadius: '4px',
      pointerEvents: 'none',
      zIndex: '9999',
      transition: 'none'
    REDACTED)
    document.body.appendChild(marqueeEl)
  REDACTED

  function updateMarquee(currentY: number) {
    if (!marqueeEl || !containerRef.value) return
    const containerRect = containerRef.value.getBoundingClientRect()

    const top = Math.min(startY, currentY)
    const bottom = Math.max(startY, currentY)

    // Clamp to container horizontal bounds, extend full width
    marqueeEl.style.left = containerRect.left + 'px'
    marqueeEl.style.width = containerRect.width + 'px'
    marqueeEl.style.top = top + 'px'
    marqueeEl.style.height = (bottom - top) + 'px'
  REDACTED

  function removeMarquee() {
    if (marqueeEl) {
      marqueeEl.remove()
      marqueeEl = null
    REDACTED
  REDACTED

  // --- Row selection logic ---
  function applyRange(endIndex: number) {
    const rangeMin = Math.min(startRowIndex, endIndex)
    const rangeMax = Math.max(startRowIndex, endIndex)
    const prevMin = lastEndIndex >= 0 ? Math.min(startRowIndex, lastEndIndex) : rangeMin
    const prevMax = lastEndIndex >= 0 ? Math.max(startRowIndex, lastEndIndex) : rangeMax

    const lo = Math.min(rangeMin, prevMin)
    const hi = Math.max(rangeMax, prevMax)

    for (let i = lo; i <= hi && i < cachedRows.length; i++) {
      const id = getRowId(cachedRows[i])
      if (id === null) continue

      if (i >= rangeMin && i <= rangeMax) {
        if (dragMode === 'select') {
          adapter.select(id)
        REDACTED else {
          adapter.deselect(id)
        REDACTED
      REDACTED else {
        const wasSelected = initialSelectedSnapshot.get(id) ?? false
        if (wasSelected) {
          adapter.select(id)
        REDACTED else {
          adapter.deselect(id)
        REDACTED
      REDACTED
    REDACTED

    lastEndIndex = endIndex
  REDACTED

  function onMouseDown(e: MouseEvent) {
    if (e.button !== 0) return

    const target = e.target as HTMLElement
    if (target.closest('button, a, input, select, textarea, [role="button"], [role="menuitem"]')) return
    if (!target.closest('tbody')) return

    cachedRows = getDataRows()
    const tr = target.closest('tr[data-row-id]') as HTMLElement | null
    if (!tr) return
    const rowIndex = cachedRows.indexOf(tr)
    if (rowIndex < 0) return

    const rowId = getRowId(tr)
    if (rowId === null) return

    initialSelectedSnapshot = new Map()
    for (const row of cachedRows) {
      const id = getRowId(row)
      if (id !== null) {
        initialSelectedSnapshot.set(id, adapter.isSelected(id))
      REDACTED
    REDACTED

    isDragging.value = true
    startRowIndex = rowIndex
    lastEndIndex = -1
    startY = e.clientY
    dragMode = adapter.isSelected(rowId) ? 'deselect' : 'select'

    applyRange(rowIndex)

    // Create visual marquee
    createMarquee()
    updateMarquee(e.clientY)

    e.preventDefault()
    document.body.style.userSelect = 'none'
    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
  REDACTED

  function onMouseMove(e: MouseEvent) {
    if (!isDragging.value) return

    // Update marquee box
    updateMarquee(e.clientY)

    const el = document.elementFromPoint(e.clientX, e.clientY) as HTMLElement | null
    if (!el) return

    const tr = el.closest('tr[data-row-id]') as HTMLElement | null
    if (!tr) return
    const rowIndex = cachedRows.indexOf(tr)
    if (rowIndex < 0) return

    applyRange(rowIndex)
    autoScroll(e)
  REDACTED

  function onMouseUp() {
    isDragging.value = false
    startRowIndex = -1
    lastEndIndex = -1
    cachedRows = []
    initialSelectedSnapshot.clear()
    stopAutoScroll()
    removeMarquee()
    document.body.style.userSelect = ''

    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  REDACTED

  // --- Auto-scroll ---
  let scrollRAF = 0
  const SCROLL_ZONE = 40
  const SCROLL_SPEED = 8

  function autoScroll(e: MouseEvent) {
    cancelAnimationFrame(scrollRAF)
    const container = containerRef.value
    if (!container) return

    const rect = container.getBoundingClientRect()
    let dy = 0
    if (e.clientY < rect.top + SCROLL_ZONE) {
      dy = -SCROLL_SPEED
    REDACTED else if (e.clientY > rect.bottom - SCROLL_ZONE) {
      dy = SCROLL_SPEED
    REDACTED

    if (dy !== 0) {
      const step = () => {
        container.scrollTop += dy
        scrollRAF = requestAnimationFrame(step)
      REDACTED
      scrollRAF = requestAnimationFrame(step)
    REDACTED
  REDACTED

  function stopAutoScroll() {
    cancelAnimationFrame(scrollRAF)
  REDACTED

  onMounted(() => {
    containerRef.value?.addEventListener('mousedown', onMouseDown)
  REDACTED)

  onUnmounted(() => {
    containerRef.value?.removeEventListener('mousedown', onMouseDown)
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    stopAutoScroll()
    removeMarquee()
  REDACTED)

  return { isDragging REDACTED
REDACTED
