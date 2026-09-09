import { readonly, ref } from 'vue'

export interface CustomBackground {
  image: string
  opacity: number
  componentEffect: CustomComponentEffect
  enabled: boolean
}

export type CustomComponentEffect = 'frosted' | 'transparent'

interface StoredBackground {
  blob: Blob
  opacity: number
  componentEffect?: CustomComponentEffect
  enabled?: boolean
}

const STORAGE_KEY = 'custom-background'
const DATABASE_NAME = 'sub2api-local-assets'
const DATABASE_VERSION = 1
const STORE_NAME = 'custom-background'
const RECORD_KEY = 'current'
const DEFAULT_OPACITY = 0.2
const DEFAULT_COMPONENT_EFFECT: CustomComponentEffect = 'frosted'

const customBackground = ref<CustomBackground>({
  image: '',
  opacity: DEFAULT_OPACITY,
  componentEffect: DEFAULT_COMPONENT_EFFECT,
  enabled: false,
})
let activeObjectUrl = ''

function clampOpacity(value: number) {
  return Math.min(1, Math.max(0, Number.isFinite(value) ? value : DEFAULT_OPACITY))
}

function normalizeComponentEffect(value: unknown): CustomComponentEffect {
  return value === 'transparent' ? 'transparent' : DEFAULT_COMPONENT_EFFECT
}

function resolveLegacyBackground(value: string | null): CustomBackground {
  if (!value) {
    return { image: '', opacity: DEFAULT_OPACITY, componentEffect: DEFAULT_COMPONENT_EFFECT, enabled: false }
  }

  try {
    const parsed = JSON.parse(value) as Partial<CustomBackground>
    return {
      image: typeof parsed.image === 'string' ? parsed.image : '',
      opacity: clampOpacity(typeof parsed.opacity === 'number' ? parsed.opacity : DEFAULT_OPACITY),
      componentEffect: normalizeComponentEffect(parsed.componentEffect),
      enabled: typeof parsed.enabled === 'boolean' ? parsed.enabled : Boolean(parsed.image),
    }
  } catch {
    return { image: '', opacity: DEFAULT_OPACITY, componentEffect: DEFAULT_COMPONENT_EFFECT, enabled: false }
  }
}

function applyBackground(background: CustomBackground, revokeObjectUrl = true) {
  if (revokeObjectUrl && activeObjectUrl && activeObjectUrl !== background.image) {
    URL.revokeObjectURL(activeObjectUrl)
    activeObjectUrl = ''
  }

  customBackground.value = background
  const root = document.documentElement
  const imageValue = background.image
    ? `url("${background.image.split('"').join('\\"')}")`
    : 'none'
  root.style.setProperty('--custom-background-image', imageValue)
  const isEnabled = Boolean(background.image) && background.enabled
  root.style.setProperty('--custom-background-opacity', String(isEnabled ? background.opacity : 0))
  root.dataset.customBackground = isEnabled ? 'true' : 'false'
  root.dataset.customComponentEffect = background.componentEffect
}

function openDatabase(): Promise<IDBDatabase | null> {
  if (typeof indexedDB === 'undefined') return Promise.resolve(null)

  return new Promise<IDBDatabase | null>((resolve, reject) => {
    const request = indexedDB.open(DATABASE_NAME, DATABASE_VERSION)
    request.onupgradeneeded = () => {
      if (!request.result.objectStoreNames.contains(STORE_NAME)) {
        request.result.createObjectStore(STORE_NAME)
      }
    }
    request.onsuccess = () => resolve(request.result)
    request.onerror = () => reject(request.error)
  })
}

async function readStoredBackground(): Promise<StoredBackground | null> {
  const database = await openDatabase()
  if (!database) return null

  return new Promise<StoredBackground | null>((resolve, reject) => {
    const request = database.transaction(STORE_NAME, 'readonly').objectStore(STORE_NAME).get(RECORD_KEY)
    request.onsuccess = () => resolve((request.result as StoredBackground | undefined) ?? null)
    request.onerror = () => reject(request.error)
  }).finally(() => database.close())
}

async function writeStoredBackground(background: StoredBackground) {
  const database = await openDatabase()
  if (!database) return false

  return new Promise<boolean>((resolve, reject) => {
    const request = database
      .transaction(STORE_NAME, 'readwrite')
      .objectStore(STORE_NAME)
      .put(background, RECORD_KEY)
    request.onsuccess = () => resolve(true)
    request.onerror = () => reject(request.error)
  }).finally(() => database.close())
}

async function deleteStoredBackground() {
  const database = await openDatabase()
  if (!database) return

  await new Promise<void>((resolve, reject) => {
    const request = database.transaction(STORE_NAME, 'readwrite').objectStore(STORE_NAME).delete(RECORD_KEY)
    request.onsuccess = () => resolve()
    request.onerror = () => reject(request.error)
  }).finally(() => database.close())
}

function setLegacyStorage(background: CustomBackground) {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(background))
  } catch {
    // localStorage is only a backwards-compatible fallback for browsers without IndexedDB.
  }
}

function clearLegacyStorage() {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Ignore storage access errors; the IndexedDB record is the primary source.
  }
}

async function migrateLegacyBackground(legacy: CustomBackground) {
  if (!legacy.image.startsWith('data:')) return

  try {
    const blob = await fetch(legacy.image).then((response) => response.blob())
    await writeStoredBackground({
      blob,
      opacity: legacy.opacity,
      componentEffect: legacy.componentEffect,
      enabled: legacy.enabled,
    })
    const objectUrl = URL.createObjectURL(blob)
    activeObjectUrl = objectUrl
    applyBackground(
      { image: objectUrl, opacity: legacy.opacity, componentEffect: legacy.componentEffect, enabled: legacy.enabled },
      false,
    )
    clearLegacyStorage()
  } catch {
    // Keep the legacy data URL active if migration is unavailable or fails.
  }
}

export function initCustomBackground() {
  const legacy = resolveLegacyBackground(localStorage.getItem(STORAGE_KEY))
  applyBackground(legacy)

  void (async () => {
    try {
      const stored = await readStoredBackground()
      if (stored?.blob) {
        const objectUrl = URL.createObjectURL(stored.blob)
        activeObjectUrl = objectUrl
        applyBackground(
          {
            image: objectUrl,
            opacity: clampOpacity(stored.opacity),
            componentEffect: normalizeComponentEffect(stored.componentEffect),
            enabled: typeof stored.enabled === 'boolean' ? stored.enabled : true,
          },
          false,
        )
      } else if (legacy.image) {
        await migrateLegacyBackground(legacy)
      }
    } catch {
      // The legacy localStorage value remains usable when IndexedDB is unavailable.
    }
  })()
}

export function setCustomBackground(
  background: Omit<CustomBackground, 'enabled'> & Partial<Pick<CustomBackground, 'enabled'>>,
) {
  const next = {
    image: background.image,
    opacity: clampOpacity(background.opacity),
    componentEffect: normalizeComponentEffect(background.componentEffect),
    enabled: typeof background.enabled === 'boolean' ? background.enabled : Boolean(background.image),
  }
  applyBackground(next)
  setLegacyStorage(next)
}

export async function setCustomBackgroundFile(
  file: Blob,
  opacity: number,
  componentEffect: CustomComponentEffect = customBackground.value.componentEffect,
  enabled = true,
) {
  const nextOpacity = clampOpacity(opacity)
  const nextEffect = normalizeComponentEffect(componentEffect)
  const stored = await writeStoredBackground({
    blob: file,
    opacity: nextOpacity,
    componentEffect: nextEffect,
    enabled,
  })
  if (activeObjectUrl) URL.revokeObjectURL(activeObjectUrl)
  const objectUrl = URL.createObjectURL(file)
  activeObjectUrl = objectUrl
  applyBackground({ image: objectUrl, opacity: nextOpacity, componentEffect: nextEffect, enabled }, false)
  if (stored) clearLegacyStorage()
}

export async function setCustomBackgroundOpacity(
  opacity: number,
  componentEffect: CustomComponentEffect = customBackground.value.componentEffect,
  enabled = customBackground.value.enabled,
) {
  const nextOpacity = clampOpacity(opacity)
  const nextEffect = normalizeComponentEffect(componentEffect)
  try {
    const stored = await readStoredBackground()
    if (stored) {
      await writeStoredBackground({ ...stored, opacity: nextOpacity, componentEffect: nextEffect, enabled })
      applyBackground({ ...customBackground.value, opacity: nextOpacity, componentEffect: nextEffect, enabled }, false)
      return
    }
  } catch {
    // Fall back to the legacy synchronous representation below.
  }

  setCustomBackground({ ...customBackground.value, opacity: nextOpacity, componentEffect: nextEffect, enabled })
}

export async function setCustomComponentEffect(componentEffect: CustomComponentEffect) {
  const nextEffect = normalizeComponentEffect(componentEffect)
  try {
    const stored = await readStoredBackground()
    if (stored) {
      await writeStoredBackground({ ...stored, componentEffect: nextEffect })
      applyBackground({ ...customBackground.value, componentEffect: nextEffect }, false)
      return
    }
  } catch {
    // Fall back to the legacy synchronous representation below.
  }

  setCustomBackground({ ...customBackground.value, componentEffect: nextEffect })
}

export async function setCustomBackgroundEnabled(enabled: boolean) {
  try {
    const stored = await readStoredBackground()
    if (stored) {
      await writeStoredBackground({ ...stored, enabled })
      applyBackground({ ...customBackground.value, enabled }, false)
      return
    }
  } catch {
    // Fall back to the legacy synchronous representation below.
  }

  setCustomBackground({ ...customBackground.value, enabled })
}

export function clearCustomBackground() {
  applyBackground({ image: '', opacity: DEFAULT_OPACITY, componentEffect: DEFAULT_COMPONENT_EFFECT, enabled: false })
  clearLegacyStorage()
  void deleteStoredBackground().catch(() => undefined)
}

export function useCustomBackground() {
  return {
    customBackground: readonly(customBackground),
    setCustomBackground,
    setCustomBackgroundFile,
    setCustomBackgroundOpacity,
    setCustomComponentEffect,
    setCustomBackgroundEnabled,
    clearCustomBackground,
  }
}
