/**
 * Shared bookkeeping for stacked (nested) dialogs.
 *
 * Every `BaseDialog` registers itself here while it is open and unregisters on
 * close *and* on unmount. The stack — not each instance — owns the three pieces
 * of global state that used to be duplicated per instance:
 *
 *  1. A single `keydown` listener on `document`. It is attached when the stack
 *     becomes non-empty and dispatched **only** to the topmost entry, so one
 *     Escape closes one dialog instead of every open dialog at once.
 *  2. The `body.modal-open` scroll lock, derived from the stack depth. Closing a
 *     nested dialog therefore keeps the lock as long as anything is still open.
 *  3. Which dialog is the real modal (`aria-modal` / `inert` / focus trap), so
 *     instances never have to guess from z-index or DOM order.
 *
 * Entries are removed by identity with `splice`, not `pop`: an outer dialog can
 * legitimately be closed by code while a dialog above it is still open.
 */
import { ref } from 'vue'

export interface DialogStackEntry {
  /** Root element of the dialog panel. Resolved lazily — it only exists while open. */
  element: () => HTMLElement | null
  /** Moves focus into this dialog. Called when the dialog above it closes. */
  focus: () => void
  /** Receives `keydown` only while this entry is the topmost dialog. */
  onKeydown: (event: KeyboardEvent) => void
}

const SCROLL_LOCK_CLASS = 'modal-open'

const stack: DialogStackEntry[] = []

/**
 * Bumped on every mutation. Reactive consumers read it before touching `stack`
 * so `computed`s such as `isTopmost` re-evaluate. A plain array + revision
 * counter is used instead of a reactive array because entries hold callbacks
 * and element getters that must not be wrapped in a proxy.
 */
const revision = ref(0)

/** Registers the reactive dependency; the value itself carries no meaning. */
function track(): void {
  void revision.value
}

function dispatchKeydown(event: KeyboardEvent): void {
  stack[stack.length - 1]?.onKeydown(event)
}

function syncGlobals(previousDepth: number): void {
  revision.value += 1

  if (typeof document === 'undefined') return

  if (previousDepth === 0 && stack.length > 0) {
    document.addEventListener('keydown', dispatchKeydown)
  } else if (previousDepth > 0 && stack.length === 0) {
    document.removeEventListener('keydown', dispatchKeydown)
  }

  document.body.classList.toggle(SCROLL_LOCK_CLASS, stack.length > 0)
}

/** Pushes an entry on top of the stack. Re-registering the same entry is a no-op. */
export function registerDialog(entry: DialogStackEntry): void {
  if (stack.includes(entry)) return

  const previousDepth = stack.length
  stack.push(entry)
  syncGlobals(previousDepth)
}

/**
 * Removes an entry from anywhere in the stack. Safe to call for an entry that is
 * not registered, so `close` followed by `unmount` cannot double-release the
 * scroll lock — and an unmount while still open cannot leave an orphan behind.
 */
export function unregisterDialog(entry: DialogStackEntry): void {
  const index = stack.indexOf(entry)
  if (index === -1) return

  const previousDepth = stack.length
  stack.splice(index, 1)
  syncGlobals(previousDepth)
}

/** True when `entry` is the topmost open dialog. Reactive. */
export function isTopDialog(entry: DialogStackEntry): boolean {
  track()
  return stack.length > 0 && stack[stack.length - 1] === entry
}

/** The topmost open dialog, or `null` when nothing is open. Reactive. */
export function topDialog(): DialogStackEntry | null {
  track()
  return stack.length > 0 ? (stack[stack.length - 1] as DialogStackEntry) : null
}

/** Number of open dialogs. Reactive. Mainly useful for assertions in tests. */
export function dialogStackDepth(): number {
  track()
  return stack.length
}

/** Test helper: drops every entry and releases the global state it owned. */
export function resetDialogStack(): void {
  if (stack.length === 0) return

  const previousDepth = stack.length
  stack.length = 0
  syncGlobals(previousDepth)
}
