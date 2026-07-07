import { ref, onMounted, onUnmounted, type Ref } from 'vue'
import type { Virtualizer } from '@tanstack/vue-virtual'

/**
 * WeChat-style swipe/drag to select rows in a DataTable,
 * with a semi-transparent marquee overlay showing the selection area.
 *
 * Features:
 *  - Start dragging inside the current table-page layout's non-text area
 *  - Mouse wheel scrolling continues selecting new rows
 *  - Auto-scroll when dragging near viewport edges
 *  - 5px drag threshold to avoid accidental selection on click
 *
 * Usage:
 *   const containerRef = ref<HTMLElement | null>(null)
 *   useSwipeSelect(containerRef, {
 *     isSelected: (id) => selIds.value.includes(id),
 *     select: (id) => { if (!selIds.value.includes(id)) selIds.value.push(id) },
 *     deselect: (id) => { selIds.value = selIds.value.filter(x => x !== id) },
 *   })
 *
 * Wrap <DataTable> with <div ref="containerRef">...</div>
 * DataTable rows must have data-row-id attribute.
 */
export interface SwipeSelectAdapter {
  isSelected: (id: number) => boolean
  select: (id: number) => void
  deselect: (id: number) => void
  batchUpdate?: (updater: (draft: Set<number>) => void) => void
}

export interface SwipeSelectVirtualContext {
  /** Get the virtualizer instance */
  getVirtualizer: () => Virtualizer<HTMLElement, Element> | null
  /** Get all sorted data */
  getSortedData: () => any[]
  /** Get row ID from data row */
  getRowId: (row: any, index: number) => number
}

export function useSwipeSelect(
  containerRef: Ref<HTMLElement | null>,
  adapter: SwipeSelectAdapter,
  virtualContext?: SwipeSelectVirtualContext
) {
  const isDragging = ref(false)

  let dragMode: 'select' | 'deselect' = 'select'
  let startRowIndex = -1
  let lastEndIndex = -1
  let startY = 0
  let lastMouseY = 0
  let pendingStartY = 0
  let initialSelectedSnapshot = new Map<number, boolean>()
  let cachedRows: HTMLElement[] = []
  let marqueeEl: HTMLDivElement | null = null
  let cachedScrollParent: HTMLElement | null = null

  const DRAG_THRESHOLD = 5
  const SCROLL_ZONE = 60
  const SCROLL_SPEED = 8

  function getActivationRoot(): HTMLElement | null {
    const container = containerRef.value
    if (!container) return null
    return container.closest('.table-page-layout') as HTMLElement | null || container
  }

  function getDataRows(): HTMLElement[] {
    const container = containerRef.value
    if (!container) return []
    return Array.from(container.querySelectorAll('tbody tr[data-row-id]'))
  }

  function getRowId(el: HTMLElement): number | null {
    const raw = el.getAttribute('data-row-id')
    if (raw === null) return null
    const id = Number(raw)
    return Number.isFinite(id) ? id : null
  }

  /** Find the row index closest to a viewport Y coordinate (binary search). */
  function findRowIndexAtY(clientY: number): number {
    const len = cachedRows.length
    if (len === 0) return -1

    // Boundary checks
    const firstRect = cachedRows[0].getBoundingClientRect()
    if (clientY < firstRect.top) return 0
    const lastRect = cachedRows[len - 1].getBoundingClientRect()
    if (clientY > lastRect.bottom) return len - 1

    // Binary search — rows are vertically ordered
    let lo = 0, hi = len - 1
    while (lo <= hi) {
      const mid = (lo + hi) >>> 1
      const rect = cachedRows[mid].getBoundingClientRect()
      if (clientY < rect.top) hi = mid - 1
      else if (clientY > rect.bottom) lo = mid + 1
      else return mid
    }
    // In a gap between rows — pick the closer one
    if (hi < 0) return 0
    if (lo >= len) return len - 1
    const rHi = cachedRows[hi].getBoundingClientRect()
    const rLo = cachedRows[lo].getBoundingClientRect()
    return (clientY - rHi.bottom < rLo.top - clientY) ? hi : lo
  }

  /** Virtual mode: find row index from Y coordinate using virtualizer data */
  function findRowIndexAtYVirtual(clientY: number): number {
    const virt = virtualContext!.getVirtualizer()
    if (!virt) return -1
    const scrollEl = virt.scrollElement
    if (!scrollEl) return -1

    const scrollRect = scrollEl.getBoundingClientRect()
    const thead = scrollEl.querySelector('thead')
    const theadHeight = thead ? thead.getBoundingClientRect().height : 0
    const contentY = clientY - scrollRect.top - theadHeight + scrollEl.scrollTop

    // Search in rendered virtualItems first
    const items = virt.getVirtualItems()
    for (const item of items) {
      if (contentY >= item.start && contentY < item.end) return item.index
    }

    // Outside visible range: estimate
    const totalCount = virtualContext!.getSortedData().length
    if (totalCount === 0) return -1
    const est = virt.options.estimateSize(0)
    const guess = Math.floor(contentY / est)
    return Math.max(0, Math.min(totalCount - 1, guess))
  }

  // --- Prevent text selection via selectstart (no body style mutation) ---
  function onSelectStart(e: Event) { e.preventDefault() }

  // --- Marquee overlay ---
  function createMarquee() {
    removeMarquee() // defensive: remove any stale marquee
    marqueeEl = document.createElement('div')
    const isDark = document.documentElement.classList.contains('dark')
    Object.assign(marqueeEl.style, {
      position: 'fixed',
      background: isDark ? 'rgba(96, 165, 250, 0.15)' : 'rgba(59, 130, 246, 0.12)',
      border: isDark ? '1.5px solid rgba(96, 165, 250, 0.5)' : '1.5px solid rgba(59, 130, 246, 0.4)',
      borderRadius: '4px',
      pointerEvents: 'none',
      zIndex: '9999',
      transition: 'none',
    })
    document.body.appendChild(marqueeEl)
  }

  function updateMarquee(currentY: number) {
    if (!marqueeEl || !containerRef.value) return
    const containerRect = containerRef.value.getBoundingClientRect()
    const top = Math.min(startY, currentY)
    const bottom = Math.max(startY, currentY)
    marqueeEl.style.left = containerRect.left + 'px'
    marqueeEl.style.width = containerRect.width + 'px'
    marqueeEl.style.top = top + 'px'
    marqueeEl.style.height = (bottom - top) + 'px'
  }

  function removeMarquee() {
    if (marqueeEl) { marqueeEl.remove(); marqueeEl = null }
  }

  // --- Row selection logic ---
  function applyRange(endIndex: number) {
    if (startRowIndex < 0 || endIndex < 0) return
    const rangeMin = Math.min(startRowIndex, endIndex)
    const rangeMax = Math.max(startRowIndex, endIndex)
    const prevMin = lastEndIndex >= 0 ? Math.min(startRowIndex, lastEndIndex) : rangeMin
    const prevMax = lastEndIndex >= 0 ? Math.max(startRowIndex, lastEndIndex) : rangeMax
    const lo = Math.min(rangeMin, prevMin)
    const hi = Math.max(rangeMax, prevMax)

    if (adapter.batchUpdate) {
      adapter.batchUpdate((draft) => {
        for (let i = lo; i <= hi && i < cachedRows.length; i++) {
          const id = getRowId(cachedRows[i])
          if (id === null) continue
          const shouldBeSelected = (i >= rangeMin && i <= rangeMax)
            ? (dragMode === 'select')
            : (initialSelectedSnapshot.get(id) ?? false)
          if (shouldBeSelected) draft.add(id)
          else draft.delete(id)
        }
      })
    } else {
      for (let i = lo; i <= hi && i < cachedRows.length; i++) {
        const id = getRowId(cachedRows[i])
        if (id === null) continue
        if (i >= rangeMin && i <= rangeMax) {
          if (dragMode === 'select') adapter.select(id)
          else adapter.deselect(id)
        } else {
          const wasSelected = initialSelectedSnapshot.get(id) ?? false
          if (wasSelected) adapter.select(id)
          else adapter.deselect(id)
        }
      }
    }
    lastEndIndex = endIndex
  }

  /** Virtual mode: apply selection range using data array instead of DOM */
  function applyRangeVirtual(endIndex: number) {
    if (startRowIndex < 0 || endIndex < 0) return
    const rangeMin = Math.min(startRowIndex, endIndex)
    const rangeMax = Math.max(startRowIndex, endIndex)
    const prevMin = lastEndIndex >= 0 ? Math.min(startRowIndex, lastEndIndex) : rangeMin
    const prevMax = lastEndIndex >= 0 ? Math.max(startRowIndex, lastEndIndex) : rangeMax
    const lo = Math.min(rangeMin, prevMin)
    const hi = Math.max(rangeMax, prevMax)
    const data = virtualContext!.getSortedData()

    if (adapter.batchUpdate) {
      adapter.batchUpdate((draft) => {
        for (let i = lo; i <= hi && i < data.length; i++) {
          const id = virtualContext!.getRowId(data[i], i)
          const shouldBeSelected = (i >= rangeMin && i <= rangeMax)
            ? (dragMode === 'select')
            : (initialSelectedSnapshot.get(id) ?? false)
          if (shouldBeSelected) draft.add(id)
          else draft.delete(id)
        }
      })
    } else {
      for (let i = lo; i <= hi && i < data.length; i++) {
        const id = virtualContext!.getRowId(data[i], i)
        if (i >= rangeMin && i <= rangeMax) {
          if (dragMode === 'select') adapter.select(id)
          else adapter.deselect(id)
        } else {
          const wasSelected = initialSelectedSnapshot.get(id) ?? false
          if (wasSelected) adapter.select(id)
          else adapter.deselect(id)
        }
      }
    }
    lastEndIndex = endIndex
  }

  // --- Scrollable parent ---
  function getScrollParent(el: HTMLElement): HTMLElement {
    let parent = el.parentElement
    while (parent && parent !== document.documentElement) {
      const { overflow, overflowY } = getComputedStyle(parent)
      if (/(auto|scroll)/.test(overflow + overflowY)) return parent
      parent = parent.parentElement
    }
    return document.documentElement
  }

  // --- Scrollbar click detection ---
  /** Check if click lands on a scrollbar of the target element or any ancestor. */
  function isOnScrollbar(e: MouseEvent): boolean {
    let el = e.target as HTMLElement | null
    while (el && el !== document.documentElement) {
      const hasVScroll = el.scrollHeight > el.clientHeight
      const hasHScroll = el.scrollWidth > el.clientWidth
      if (hasVScroll || hasHScroll) {
        const rect = el.getBoundingClientRect()
        // clientWidth/clientHeight exclude scrollbar; offsetWidth/offsetHeight include it
        if (hasVScroll && e.clientX > rect.left + el.clientWidth) return true
        if (hasHScroll && e.clientY > rect.top + el.clientHeight) return true
      }
      el = el.parentElement
    }
    // Document-level scrollbar
    const docEl = document.documentElement
    if (e.clientX >= docEl.clientWidth || e.clientY >= docEl.clientHeight) return true
    return false
  }

  /**
   * If the mousedown starts on inner cell content rather than cell padding,
   * prefer the browser's native text selection so users can copy text normally.
   */
  function shouldPreferNativeTextSelection(target: HTMLElement): boolean {
    const row = target.closest('tbody tr[data-row-id]')
    if (!row) return false

    const cell = target.closest('td, th')
    if (!cell) return false

    return target !== cell && !target.closest('[data-swipe-select-handle]')
  }

  function hasDirectTextContent(target: HTMLElement): boolean {
    return Array.from(target.childNodes).some(
      (node) => node.nodeType === Node.TEXT_NODE && (node.textContent?.trim().length ?? 0) > 0
    )
  }

  function shouldPreferNativeSelectionOutsideRows(target: HTMLElement): boolean {
    const activationRoot = getActivationRoot()
    if (!activationRoot) return false
    if (!activationRoot.contains(target)) return false
    if (target.closest('tbody tr[data-row-id]')) return false

    return hasDirectTextContent(target)
  }

  // =============================================
  // Phase 1: detect drag threshold (5px movement)
  // =============================================
  function onMouseDown(e: MouseEvent) {
    if (e.button !== 0) return
    if (!containerRef.value) return

    const target = e.target as HTMLElement
    const activationRoot = getActivationRoot()
    if (!activationRoot || !activationRoot.contains(target)) return

    // Skip clicks on any scrollbar (inner containers + document)
    if (isOnScrollbar(e)) return

    if (target.closest('button, a, input, select, textarea, [role="button"], [role="menuitem"], [role="combobox"], [role="dialog"]')) return
    if (shouldPreferNativeTextSelection(target)) return
    if (shouldPreferNativeSelectionOutsideRows(target)) return

    if (virtualContext) {
      // Virtual mode: check data availability instead of DOM rows
      const data = virtualContext.getSortedData()
      if (data.length === 0) return
    } else {
      cachedRows = getDataRows()
      if (cachedRows.length === 0) return
    }

    pendingStartY = e.clientY
    // Prevent text selection as soon as the mouse is down,
    // before the drag threshold is reached (Phase 1).
    // Without this, the browser starts selecting text during
    // the 0–5px threshold movement window.
    document.addEventListener('selectstart', onSelectStart)
    document.addEventListener('mousemove', onThresholdMove)
    document.addEventListener('mouseup', onThresholdUp)
  }

  function onThresholdMove(e: MouseEvent) {
    if (Math.abs(e.clientY - pendingStartY) < DRAG_THRESHOLD) return
    // Threshold exceeded — begin actual drag
    document.removeEventListener('mousemove', onThresholdMove)
    document.removeEventListener('mouseup', onThresholdUp)

    if (virtualContext) {
      beginDragVirtual(pendingStartY)
    } else {
      beginDrag(pendingStartY)
    }

    // Process the move that crossed the threshold
    lastMouseY = e.clientY
    updateMarquee(e.clientY)
    const findIdx = virtualContext ? findRowIndexAtYVirtual : findRowIndexAtY
    const apply = virtualContext ? applyRangeVirtual : applyRange
    const rowIdx = findIdx(e.clientY)
    if (rowIdx >= 0) apply(rowIdx)
    autoScroll(e)

    document.addEventListener('mousemove', onMouseMove)
    document.addEventListener('mouseup', onMouseUp)
    document.addEventListener('wheel', onWheel, { passive: true })
  }

  function onThresholdUp() {
    document.removeEventListener('mousemove', onThresholdMove)
    document.removeEventListener('mouseup', onThresholdUp)
    // Phase 1 ended without crossing threshold — remove selectstart blocker
    document.removeEventListener('selectstart', onSelectStart)
    cachedRows = []
  }

  // ============================
  // Phase 2: actual drag session
  // ============================
  function beginDrag(clientY: number) {
    startRowIndex = findRowIndexAtY(clientY)
    const startRowId = startRowIndex >= 0 ? getRowId(cachedRows[startRowIndex]) : null
    dragMode = (startRowId !== null && adapter.isSelected(startRowId)) ? 'deselect' : 'select'

    initialSelectedSnapshot = new Map()
    for (const row of cachedRows) {
      const id = getRowId(row)
      if (id !== null) initialSelectedSnapshot.set(id, adapter.isSelected(id))
    }

    isDragging.value = true
    startY = clientY
    lastMouseY = clientY
    lastEndIndex = -1
    cachedScrollParent = cachedRows.length > 0
      ? getScrollParent(cachedRows[0])
      : (containerRef.value ? getScrollParent(containerRef.value) : null)

    createMarquee()
    updateMarquee(clientY)
    applyRange(startRowIndex)
    // selectstart is already blocked since Phase 1 (onMouseDown).
    // Clear any text selection that the browser may have started
    // before our selectstart handler took effect.
    window.getSelection()?.removeAllRanges()
  }

  /** Virtual mode: begin drag using data array */
  function beginDragVirtual(clientY: number) {
    startRowIndex = findRowIndexAtYVirtual(clientY)
    const data = virtualContext!.getSortedData()
    const startRowId = startRowIndex >= 0 && startRowIndex < data.length
      ? virtualContext!.getRowId(data[startRowIndex], startRowIndex)
      : null
    dragMode = (startRowId !== null && adapter.isSelected(startRowId)) ? 'deselect' : 'select'

    // Build full snapshot from all data rows
    initialSelectedSnapshot = new Map()
    for (let i = 0; i < data.length; i++) {
      const id = virtualContext!.getRowId(data[i], i)
      initialSelectedSnapshot.set(id, adapter.isSelected(id))
    }

    isDragging.value = true
    startY = clientY
    lastMouseY = clientY
    lastEndIndex = -1

    // In virtual mode, scroll parent is the virtualizer's scroll element
    const virt = virtualContext!.getVirtualizer()
    cachedScrollParent = virt?.scrollElement ?? (containerRef.value ? getScrollParent(containerRef.value) : null)

    createMarquee()
    updateMarquee(clientY)
    applyRangeVirtual(startRowIndex)
    window.getSelection()?.removeAllRanges()
  }

  let moveRAF = 0

  function onMouseMove(e: MouseEvent) {
    if (!isDragging.value) return
    lastMouseY = e.clientY
    const findIdx = virtualContext ? findRowIndexAtYVirtual : findRowIndexAtY
    const apply = virtualContext ? applyRangeVirtual : applyRange
    cancelAnimationFrame(moveRAF)
    moveRAF = requestAnimationFrame(() => {
      updateMarquee(lastMouseY)
      const rowIdx = findIdx(lastMouseY)
      if (rowIdx >= 0 && rowIdx !== lastEndIndex) apply(rowIdx)
    })
    autoScroll(e)
  }

  function onWheel() {
    if (!isDragging.value) return
    const findIdx = virtualContext ? findRowIndexAtYVirtual : findRowIndexAtY
    const apply = virtualContext ? applyRangeVirtual : applyRange
    // After wheel scroll, rows shift in viewport — re-check selection
    requestAnimationFrame(() => {
      if (!isDragging.value) return // guard: drag may have ended before this frame
      const rowIdx = findIdx(lastMouseY)
      if (rowIdx >= 0) apply(rowIdx)
    })
  }

  function cleanupDrag() {
    isDragging.value = false
    startRowIndex = -1
    lastEndIndex = -1
    cachedRows = []
    initialSelectedSnapshot.clear()
    cachedScrollParent = null
    cancelAnimationFrame(moveRAF)
    stopAutoScroll()
    removeMarquee()
    document.removeEventListener('selectstart', onSelectStart)
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
    document.removeEventListener('wheel', onWheel)
  }

  function onMouseUp() {
    cleanupDrag()
  }

  // Guard: clean up if mouse leaves window or window loses focus during drag
  function onWindowBlur() {
    if (isDragging.value) cleanupDrag()
    // Also clean up threshold phase (Phase 1)
    document.removeEventListener('mousemove', onThresholdMove)
    document.removeEventListener('mouseup', onThresholdUp)
    document.removeEventListener('selectstart', onSelectStart)
  }

  // --- Auto-scroll logic ---
  let scrollRAF = 0

  function autoScroll(e: MouseEvent) {
    cancelAnimationFrame(scrollRAF)
    const scrollEl = cachedScrollParent
    if (!scrollEl) return

    let dy = 0
    if (scrollEl === document.documentElement) {
      if (e.clientY < SCROLL_ZONE) dy = -SCROLL_SPEED
      else if (e.clientY > window.innerHeight - SCROLL_ZONE) dy = SCROLL_SPEED
    } else {
      const rect = scrollEl.getBoundingClientRect()
      if (e.clientY < rect.top + SCROLL_ZONE) dy = -SCROLL_SPEED
      else if (e.clientY > rect.bottom - SCROLL_ZONE) dy = SCROLL_SPEED
    }

    if (dy !== 0) {
      const findIdx = virtualContext ? findRowIndexAtYVirtual : findRowIndexAtY
      const apply = virtualContext ? applyRangeVirtual : applyRange
      const step = () => {
        const prevScrollTop = scrollEl.scrollTop
        scrollEl.scrollTop += dy
        // Only re-check selection if scroll actually moved
        if (scrollEl.scrollTop !== prevScrollTop) {
          const rowIdx = findIdx(lastMouseY)
          if (rowIdx >= 0 && rowIdx !== lastEndIndex) apply(rowIdx)
        }
        scrollRAF = requestAnimationFrame(step)
      }
      scrollRAF = requestAnimationFrame(step)
    }
  }

  function stopAutoScroll() {
    cancelAnimationFrame(scrollRAF)
  }

  // --- Lifecycle ---
  onMounted(() => {
    document.addEventListener('mousedown', onMouseDown)
    window.addEventListener('blur', onWindowBlur)
  })

  onUnmounted(() => {
    document.removeEventListener('mousedown', onMouseDown)
    window.removeEventListener('blur', onWindowBlur)
    // Clean up any in-progress drag state
    document.removeEventListener('mousemove', onThresholdMove)
    document.removeEventListener('mouseup', onThresholdUp)
    document.removeEventListener('selectstart', onSelectStart)
    cleanupDrag()
  })

  return { isDragging }
}
