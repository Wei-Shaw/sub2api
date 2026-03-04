import { defineStore REDACTED from 'pinia'
import { ref REDACTED from 'vue'
import { adminAPI REDACTED from '@/api'
import type { CustomMenuItem REDACTED from '@/types'

export const useAdminSettingsStore = defineStore('adminSettings', () => {
  const loaded = ref(false)
  const loading = ref(false)

  const readCachedBool = (key: string, defaultValue: boolean): boolean => {
    try {
      const raw = localStorage.getItem(key)
      if (raw === 'true') return true
      if (raw === 'false') return false
    REDACTED catch {
      // ignore localStorage failures
    REDACTED
    return defaultValue
  REDACTED

  const writeCachedBool = (key: string, value: boolean) => {
    try {
      localStorage.setItem(key, value ? 'true' : 'false')
    REDACTED catch {
      // ignore localStorage failures
    REDACTED
  REDACTED

  const readCachedString = (key: string, defaultValue: string): string => {
    try {
      const raw = localStorage.getItem(key)
      if (typeof raw === 'string' && raw.length > 0) return raw
    REDACTED catch {
      // ignore localStorage failures
    REDACTED
    return defaultValue
  REDACTED

  const writeCachedString = (key: string, value: string) => {
    try {
      localStorage.setItem(key, value)
    REDACTED catch {
      // ignore localStorage failures
    REDACTED
  REDACTED

  // Default open, but honor cached value to reduce UI flicker on first paint.
  const opsMonitoringEnabled = ref(readCachedBool('ops_monitoring_enabled_cached', true))
  const opsRealtimeMonitoringEnabled = ref(readCachedBool('ops_realtime_monitoring_enabled_cached', true))
  const opsQueryModeDefault = ref(readCachedString('ops_query_mode_default_cached', 'auto'))
  const customMenuItems = ref<CustomMenuItem[]>([])

  async function fetch(force = false): Promise<void> {
    if (loaded.value && !force) return
    if (loading.value) return

    loading.value = true
    try {
      const settings = await adminAPI.settings.getSettings()
      opsMonitoringEnabled.value = settings.ops_monitoring_enabled ?? true
      writeCachedBool('ops_monitoring_enabled_cached', opsMonitoringEnabled.value)

      opsRealtimeMonitoringEnabled.value = settings.ops_realtime_monitoring_enabled ?? true
      writeCachedBool('ops_realtime_monitoring_enabled_cached', opsRealtimeMonitoringEnabled.value)

      opsQueryModeDefault.value = settings.ops_query_mode_default || 'auto'
      writeCachedString('ops_query_mode_default_cached', opsQueryModeDefault.value)

      customMenuItems.value = Array.isArray(settings.custom_menu_items) ? settings.custom_menu_items : []

      loaded.value = true
    REDACTED catch (err) {
      // Keep cached/default value: do not "flip" the UI based on a transient fetch failure.
      loaded.value = true
      console.error('[adminSettings] Failed to fetch settings:', err)
    REDACTED finally {
      loading.value = false
    REDACTED
  REDACTED

  function setOpsMonitoringEnabledLocal(value: boolean) {
    opsMonitoringEnabled.value = value
    writeCachedBool('ops_monitoring_enabled_cached', value)
    loaded.value = true
  REDACTED

  function setOpsRealtimeMonitoringEnabledLocal(value: boolean) {
    opsRealtimeMonitoringEnabled.value = value
    writeCachedBool('ops_realtime_monitoring_enabled_cached', value)
    loaded.value = true
  REDACTED

  function setOpsQueryModeDefaultLocal(value: string) {
    opsQueryModeDefault.value = value || 'auto'
    writeCachedString('ops_query_mode_default_cached', opsQueryModeDefault.value)
    loaded.value = true
  REDACTED

  // Keep UI consistent if we learn that ops is disabled via feature-gated 404s.
  // (event is dispatched from the axios interceptor)
  let eventHandlerCleanup: (() => void) | null = null

  function initializeEventListeners() {
    if (eventHandlerCleanup) return

    try {
      const handler = () => {
        setOpsMonitoringEnabledLocal(false)
      REDACTED
      window.addEventListener('ops-monitoring-disabled', handler)
      eventHandlerCleanup = () => {
        window.removeEventListener('ops-monitoring-disabled', handler)
      REDACTED
    REDACTED catch {
      // ignore window access failures (SSR)
    REDACTED
  REDACTED

  if (typeof window !== 'undefined') {
    initializeEventListeners()
  REDACTED

  return {
    loaded,
    loading,
    opsMonitoringEnabled,
    opsRealtimeMonitoringEnabled,
    opsQueryModeDefault,
    customMenuItems,
    fetch,
    setOpsMonitoringEnabledLocal,
    setOpsRealtimeMonitoringEnabledLocal,
    setOpsQueryModeDefaultLocal
  REDACTED
REDACTED)
