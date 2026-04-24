import { ref, onBeforeUnmount, type Ref REDACTED from 'vue'

export interface UseAutoRefreshOptions {
  storageKey: string
  intervals?: readonly number[]
  defaultInterval?: number
  onRefresh: () => Promise<void> | void
  /** Skip tick when this returns true (e.g. modal open, document hidden). */
  shouldPause?: () => boolean
REDACTED

export function useAutoRefresh(options: UseAutoRefreshOptions) {
  const {
    storageKey,
    intervals = [5, 10, 15, 30] as const,
    defaultInterval,
    onRefresh,
    shouldPause,
  REDACTED = options

  const enabled = ref(false)
  const intervalSeconds = ref(defaultInterval ?? intervals[intervals.length - 1])
  const countdown = ref(0)
  const fetching = ref(false)

  let timerId: number | undefined

  function loadFromStorage() {
    try {
      const saved = localStorage.getItem(storageKey)
      if (!saved) return
      const parsed = JSON.parse(saved) as { enabled?: boolean; interval_seconds?: number REDACTED
      enabled.value = parsed.enabled === true
      const iv = Number(parsed.interval_seconds)
      if (intervals.includes(iv as any)) intervalSeconds.value = iv
    REDACTED catch { /* ignore */ REDACTED
  REDACTED

  function saveToStorage() {
    try {
      localStorage.setItem(storageKey, JSON.stringify({
        enabled: enabled.value,
        interval_seconds: intervalSeconds.value,
      REDACTED))
    REDACTED catch { /* ignore */ REDACTED
  REDACTED

  async function tick() {
    if (!enabled.value) return
    if (shouldPause?.()) return
    if (fetching.value) return

    if (countdown.value <= 0) {
      countdown.value = intervalSeconds.value
      fetching.value = true
      try { await onRefresh() REDACTED finally { fetching.value = false REDACTED
      return
    REDACTED
    countdown.value -= 1
  REDACTED

  function start() {
    if (timerId !== undefined) return
    timerId = setInterval(tick, 1000) as unknown as number
  REDACTED

  function stop() {
    if (timerId !== undefined) {
      clearInterval(timerId)
      timerId = undefined
    REDACTED
  REDACTED

  function setEnabled(value: boolean) {
    enabled.value = value
    saveToStorage()
    if (value) {
      countdown.value = intervalSeconds.value
      start()
    REDACTED else {
      stop()
      countdown.value = 0
    REDACTED
  REDACTED

  function setInterval_(seconds: number) {
    intervalSeconds.value = seconds
    saveToStorage()
    if (enabled.value) countdown.value = seconds
  REDACTED

  function resetCountdown() {
    countdown.value = intervalSeconds.value
  REDACTED

  loadFromStorage()

  onBeforeUnmount(stop)

  return {
    enabled: enabled as Ref<boolean>,
    intervalSeconds: intervalSeconds as Ref<number>,
    countdown: countdown as Ref<number>,
    fetching: fetching as Ref<boolean>,
    intervals,
    setEnabled,
    setInterval: setInterval_,
    resetCountdown,
    start,
    stop,
  REDACTED
REDACTED
