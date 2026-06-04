import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import i18n, { initI18n } from './i18n'
import { useAppStore } from '@/stores/app'
import './style.css'

function initThemeClass() {
  const savedTheme = localStorage.getItem('theme')
  const shouldUseDark =
    savedTheme === 'dark' ||
    (!savedTheme &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', shouldUseDark)
}

const COSMIC_FILTER_ID = 'cosmic-cursor-distortion'
const COSMIC_MAP_ID = 'cosmic-cursor-distortion-map'
const COSMIC_DISPLACEMENT_ID = 'cosmic-cursor-displacement'
const COSMIC_VISUAL_ID = 'cosmic-cursor-visual'
const COSMIC_PARTICLES_ID = 'cosmic-pull-particles'
const COSMIC_DISTORTION_SURFACE_SELECTOR = '.cosmic-distortion-surface'
const COSMIC_SVG_NS = 'http://www.w3.org/2000/svg'
const COSMIC_XLINK_NS = 'http://www.w3.org/1999/xlink'
const COSMIC_PULL_BASE_SCALE = 86
const COSMIC_WHITE_HOLE_PULL_BASE_SCALE = 38
const COSMIC_PULL_MAX_SCALE = 185
const COSMIC_BLACK_HOLE_PULL_MAX_SCALE = 220
const COSMIC_PULL_RAMP_MS = 2600
const COSMIC_PULL_MOVE_RESET_PX = 4
const COSMIC_PARTICLE_INTERVAL_MS = 220
const COSMIC_PARTICLE_LIFETIME_MS = 920
const COSMIC_MAX_PARTICLES = 22
const COSMIC_SPIN_DIRECTION = 1
const COSMIC_SPIN_FRAME_COUNT = 14
const COSMIC_SPIN_FRAME_MS = 120
const COSMIC_DISPLACEMENT_MAP_SIZE = 160
// 旋转帧率上限（~30fps），避免空转时整页 SVG 滤镜被高频重栅格化
const COSMIC_SPIN_MIN_FRAME_MS = 33
// 鼠标静止超过该时长后，先平滑收缩再卸下整页滤镜
const COSMIC_IDLE_TIMEOUT_MS = 3500
const COSMIC_COLLAPSE_MS = 1100
// 熄火后需累计移动超过该距离才重新唤醒，过滤触控板/鼠标的微小漂移导致的反复点亮
const COSMIC_WAKE_THRESHOLD_PX = 8

type CosmicParticleSource = {
  color: string
  distance: number
  x: number
  y: number
}

type CosmicCursorMode = 'black-hole' | 'white-hole'
type CosmicCursorField = 'attract' | 'repel'

type CosmicCssColor = {
  alpha: number
  blue: number
  green: number
  red: number
}

function clamp(value: number, min: number, max: number) {
  return Math.max(min, Math.min(max, value))
}

function clampByte(value: number) {
  return Math.max(0, Math.min(255, Math.round(value)))
}

function parseCssColor(value: string): CosmicCssColor | null {
  const normalized = value.trim().toLowerCase()
  if (!normalized || normalized === 'transparent' || normalized === 'currentcolor') return null

  const match = normalized.match(/^rgba?\((.+)\)$/)
  if (!match) return null

  const content = match[1].trim()
  const [channelText, alphaText] = content.split('/').map((part) => part.trim())
  const channelParts = channelText.includes(',')
    ? channelText.split(',').map((part) => part.trim())
    : channelText.split(/\s+/)

  if (channelParts.length < 3) return null

  const parseChannel = (part: string) => {
    const number = Number.parseFloat(part)
    return part.endsWith('%') ? (number / 100) * 255 : number
  }

  const parseAlpha = (part?: string) => {
    if (!part) return 1
    const number = Number.parseFloat(part)
    return part.endsWith('%') ? number / 100 : number
  }

  const commaAlpha = channelText.includes(',') ? channelParts[3] : undefined
  const red = parseChannel(channelParts[0])
  const green = parseChannel(channelParts[1])
  const blue = parseChannel(channelParts[2])
  const alpha = parseAlpha(alphaText || commaAlpha)

  if ([red, green, blue, alpha].some((channel) => Number.isNaN(channel))) return null

  return {
    alpha: Math.max(0, Math.min(1, alpha)),
    blue: clampByte(blue),
    green: clampByte(green),
    red: clampByte(red)
  }
}

function normalizeParsedParticleColor(parsed: CosmicCssColor | null) {
  if (!parsed || parsed.alpha < 0.08) return null

  const luminance = 0.2126 * parsed.red + 0.7152 * parsed.green + 0.0722 * parsed.blue
  if (luminance < 42) return null

  const alpha = Math.min(0.92, Math.max(0.5, parsed.alpha * 0.9))
  return `rgba(${parsed.red}, ${parsed.green}, ${parsed.blue}, ${alpha.toFixed(2)})`
}

function normalizeParticleColor(value: string) {
  return normalizeParsedParticleColor(parseCssColor(value))
}

function mixCssColors(from: CosmicCssColor, to: CosmicCssColor, progress: number): CosmicCssColor {
  const amount = clamp(progress, 0, 1)
  return {
    alpha: from.alpha + (to.alpha - from.alpha) * amount,
    blue: clampByte(from.blue + (to.blue - from.blue) * amount),
    green: clampByte(from.green + (to.green - from.green) * amount),
    red: clampByte(from.red + (to.red - from.red) * amount)
  }
}

function extractCssColorValues(value: string) {
  const colorFunctions = value.match(/rgba?\([^)]*\)/g)
  return colorFunctions?.length ? colorFunctions : [value]
}

function extractGradientColorStops(value: string) {
  return [...value.matchAll(/(rgba?\([^)]*\))\s*(-?\d*\.?\d+%)?/g)]
    .map((match, index) => {
      const color = parseCssColor(match[1])
      const position = match[2] ? clamp(Number.parseFloat(match[2]) / 100, 0, 1) : null
      return color ? { color, index, position } : null
    })
    .filter((stop): stop is { color: CosmicCssColor; index: number; position: number | null } =>
      Boolean(stop)
    )
}

function getLinearGradientProgress(value: string, rect: DOMRect, clientX: number, clientY: number) {
  const localX = clamp((clientX - rect.left) / Math.max(rect.width, 1), 0, 1)
  const localY = clamp((clientY - rect.top) / Math.max(rect.height, 1), 0, 1)

  if (value.includes('to left')) return 1 - localX
  if (value.includes('to right')) return localX
  if (value.includes('to top')) return 1 - localY
  if (value.includes('to bottom')) return localY

  const degreeMatch = value.match(/linear-gradient\(\s*(-?\d*\.?\d+)deg/i)
  if (!degreeMatch) return localX

  const radians = (Number.parseFloat(degreeMatch[1]) * Math.PI) / 180
  const vectorX = Math.sin(radians)
  const vectorY = -Math.cos(radians)
  const centeredX = (localX - 0.5) * rect.width
  const centeredY = (localY - 0.5) * rect.height
  const extent = Math.abs(vectorX) * rect.width * 0.5 + Math.abs(vectorY) * rect.height * 0.5

  return extent ? clamp(0.5 + (centeredX * vectorX + centeredY * vectorY) / (extent * 2), 0, 1) : localX
}

function sampleGradientColor(value: string, rect: DOMRect, clientX: number, clientY: number) {
  if (!value.includes('gradient')) return null

  const stops = extractGradientColorStops(value)
  if (!stops.length) return null
  if (stops.length === 1) return normalizeParsedParticleColor(stops[0].color)

  const fallbackStep = stops.length > 1 ? 1 / (stops.length - 1) : 0
  const positions = stops.map((stop) => stop.position ?? stop.index * fallbackStep)
  const localX = clamp((clientX - rect.left) / Math.max(rect.width, 1), 0, 1)
  const localY = clamp((clientY - rect.top) / Math.max(rect.height, 1), 0, 1)
  const progress = value.includes('radial-gradient')
    ? clamp(Math.hypot(localX - 0.5, localY - 0.5) * 2, 0, 1)
    : getLinearGradientProgress(value, rect, clientX, clientY)

  for (let index = 0; index < stops.length - 1; index += 1) {
    const start = positions[index]
    const end = positions[index + 1]
    if (progress < start || progress > end) continue

    const segmentProgress = end === start ? 0 : (progress - start) / (end - start)
    return normalizeParsedParticleColor(
      mixCssColors(stops[index].color, stops[index + 1].color, segmentProgress)
    )
  }

  const nearestIndex = progress <= positions[0] ? 0 : stops.length - 1
  return normalizeParsedParticleColor(stops[nearestIndex].color)
}

function createCosmicDisplacementMap(
  size = COSMIC_DISPLACEMENT_MAP_SIZE,
  spinPhase = 0,
  field: CosmicCursorField = 'attract'
) {
  const canvas = document.createElement('canvas')
  canvas.width = size
  canvas.height = size

  const context = canvas.getContext('2d')
  if (!context) return ''

  const image = context.createImageData(size, size)
  const center = (size - 1) / 2
  const radius = center * 0.96
  const isRepulsive = field === 'repel'
  const polarity = isRepulsive ? -1 : 1

  for (let y = 0; y < size; y += 1) {
    for (let x = 0; x < size; x += 1) {
      const index = (y * size + x) * 4
      const dx = (x - center) / radius
      const dy = (y - center) / radius
      const distance = Math.sqrt(dx * dx + dy * dy)
      const angle = Math.atan2(dy, dx)

      if (distance >= 1) {
        image.data[index] = 128
        image.data[index + 1] = 128
        image.data[index + 2] = 128
        image.data[index + 3] = 0
        continue
      }

      const radialX = distance > 0.001 ? dx / distance : 0
      const radialY = distance > 0.001 ? dy / distance : 0
      const edgeFalloff = Math.pow(1 - distance, isRepulsive ? 1.08 : 1.35)
      const innerFade = isRepulsive
        ? 0.48 + 0.52 * (1 - Math.exp(-Math.pow(distance / 0.2, 2)))
        : 1 - Math.exp(-Math.pow(distance / 0.16, 2))
      const corePush = isRepulsive ? 0.16 * Math.exp(-Math.pow(distance / 0.2, 2)) : 0
      const lensStrength = Math.sin((1 - distance) * Math.PI) * edgeFalloff * innerFade + corePush
      const mapSpinDirection = -COSMIC_SPIN_DIRECTION
      const displacementSwirlDirection = isRepulsive ? COSMIC_SPIN_DIRECTION : -COSMIC_SPIN_DIRECTION
      const spiralPhase = angle * 3 + (1 - distance) * 8 + spinPhase * mapSpinDirection
      const spiralWave = Math.sin(spiralPhase)
      const shearWave = Math.cos(angle * 2 - (1 - distance) * 5 + spinPhase * mapSpinDirection * 0.72)
      const radialPull = (isRepulsive ? 0.9 + spiralWave * 0.07 : 1.1 + spiralWave * 0.14) * lensStrength
      const swirl =
        (isRepulsive ? 0.22 + spiralWave * 0.08 + shearWave * 0.04 : 0.32 + spiralWave * 0.16 + shearWave * 0.06) *
        lensStrength *
        (1 - distance)
      const baseX = isRepulsive ? radialX : dx
      const baseY = isRepulsive ? radialY : dy
      // feDisplacementMap 反向采样；吸引场的切向量需要反相，视觉自旋才会和黑洞一致
      const vectorX = (baseX * radialPull - baseY * swirl * displacementSwirlDirection) * polarity
      const vectorY = (baseY * radialPull + baseX * swirl * displacementSwirlDirection) * polarity

      image.data[index] = clampByte(128 + vectorX * 128)
      image.data[index + 1] = clampByte(128 + vectorY * 128)
      image.data[index + 2] = 128
      image.data[index + 3] = clampByte(lensStrength * 255)
    }
  }

  context.putImageData(image, 0, 0)
  return canvas.toDataURL('image/png')
}

function ensureCosmicDistortionFilter() {
  let filter = document.getElementById(COSMIC_FILTER_ID) as SVGElement | null
  let map = document.getElementById(COSMIC_MAP_ID) as SVGElement | null
  let displacement = document.getElementById(COSMIC_DISPLACEMENT_ID) as SVGElement | null
  let visual = document.getElementById(COSMIC_VISUAL_ID) as HTMLElement | null
  let particles = document.getElementById(COSMIC_PARTICLES_ID) as HTMLElement | null

  const ensureVisualLayers = () => {
    if (!visual) {
      visual = document.createElement('div')
      visual.id = COSMIC_VISUAL_ID
      visual.classList.add('cosmic-cursor-visual')
      visual.setAttribute('aria-hidden', 'true')
      document.body.append(visual)
    }

    if (!particles) {
      particles = document.createElement('div')
      particles.id = COSMIC_PARTICLES_ID
      particles.classList.add('cosmic-pull-particles')
      particles.setAttribute('aria-hidden', 'true')
      document.body.append(particles)
    }
  }

  if (filter && map && displacement) {
    ensureVisualLayers()
    return { filter, map, displacement, particles: particles as HTMLElement, visual: visual as HTMLElement }
  }

  const svg = document.createElementNS(COSMIC_SVG_NS, 'svg')
  svg.setAttribute('aria-hidden', 'true')
  svg.setAttribute('focusable', 'false')
  svg.classList.add('cosmic-distortion-svg')

  const defs = document.createElementNS(COSMIC_SVG_NS, 'defs')
  filter = document.createElementNS(COSMIC_SVG_NS, 'filter')
  filter.setAttribute('id', COSMIC_FILTER_ID)
  filter.setAttribute('filterUnits', 'userSpaceOnUse')
  filter.setAttribute('color-interpolation-filters', 'sRGB')

  const neutral = document.createElementNS(COSMIC_SVG_NS, 'feFlood')
  neutral.setAttribute('flood-color', 'rgb(128,128,128)')
  neutral.setAttribute('result', 'neutralMap')

  map = document.createElementNS(COSMIC_SVG_NS, 'feImage')
  map.setAttribute('id', COSMIC_MAP_ID)
  map.setAttribute('x', '0')
  map.setAttribute('y', '0')
  map.setAttribute('width', '0')
  map.setAttribute('height', '0')
  map.setAttribute('preserveAspectRatio', 'none')
  map.setAttribute('result', 'cursorMap')

  const mapUrl = createCosmicDisplacementMap()
  if (mapUrl) {
    map.setAttribute('href', mapUrl)
    map.setAttributeNS(COSMIC_XLINK_NS, 'href', mapUrl)
  }

  const composedMap = document.createElementNS(COSMIC_SVG_NS, 'feComposite')
  composedMap.setAttribute('in', 'cursorMap')
  composedMap.setAttribute('in2', 'neutralMap')
  composedMap.setAttribute('operator', 'over')
  composedMap.setAttribute('result', 'displacementMap')

  displacement = document.createElementNS(COSMIC_SVG_NS, 'feDisplacementMap')
  displacement.setAttribute('id', COSMIC_DISPLACEMENT_ID)
  displacement.setAttribute('in', 'SourceGraphic')
  displacement.setAttribute('in2', 'displacementMap')
  displacement.setAttribute('scale', '0')
  displacement.setAttribute('xChannelSelector', 'R')
  displacement.setAttribute('yChannelSelector', 'G')

  filter.append(neutral, map, composedMap, displacement)
  defs.append(filter)
  svg.append(defs)

  document.body.append(svg)
  ensureVisualLayers()

  return { filter, map, displacement, particles: particles as HTMLElement, visual: visual as HTMLElement }
}

function initCosmicCursor() {
  if (typeof window === 'undefined' || typeof document === 'undefined') return

  const root = document.documentElement
  const supportsFinePointer =
    typeof window.matchMedia === 'function' && window.matchMedia('(pointer: fine)').matches
  const prefersReducedMotion =
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches

  if (!supportsFinePointer || prefersReducedMotion) return

  const { filter, map, displacement, particles, visual } = ensureCosmicDistortionFilter()

  let frame = 0
  let pullTimer = 0
  let particleTimer = 0
  let spinFrameRequest = 0
  let collapseFrame = 0
  let spinFrameIndex = 0
  let lastSpinFrameAt = 0
  let particleSeed = 0
  let isActive = false
  let idleTimer = 0
  let nextClientX = window.innerWidth / 2
  let nextClientY = window.innerHeight / 2
  let wakeAnchorX = nextClientX
  let wakeAnchorY = nextClientY
  let anchorClientX = nextClientX
  let anchorClientY = nextClientY
  let holdStartedAt = 0
  let cursorMode: CosmicCursorMode = root.classList.contains('dark') ? 'black-hole' : 'white-hole'
  let cursorField: CosmicCursorField = root.classList.contains('dark') ? 'attract' : 'repel'
  const spinFrames = new Map<string, string[]>()

  const getRadius = () => Math.max(126, Math.min(180, window.innerWidth * 0.12))

  const getEventHorizonRadius = () => {
    const visualRadius = visual.getBoundingClientRect().width / 2
    return visualRadius || 24
  }

  const getCursorField = (mode: CosmicCursorMode): CosmicCursorField =>
    mode === 'white-hole' ? 'repel' : 'attract'

  const syncCursorMode = () => {
    cursorMode = root.classList.contains('dark') ? 'black-hole' : 'white-hole'
    cursorField = getCursorField(cursorMode)
    root.classList.toggle('cosmic-cursor-white-hole', cursorMode === 'white-hole')
    root.classList.toggle('cosmic-cursor-black-hole', cursorMode === 'black-hole')
  }

  const getSpinFrame = (index: number) => {
    const cacheKey = `${cursorMode}:${cursorField}`
    let frames = spinFrames.get(cacheKey)
    if (!frames) {
      frames = []
      spinFrames.set(cacheKey, frames)
    }
    if (!frames[index]) {
      frames[index] = createCosmicDisplacementMap(
        COSMIC_DISPLACEMENT_MAP_SIZE,
        (index / COSMIC_SPIN_FRAME_COUNT) * Math.PI * 2,
        cursorField
      )
    }
    return frames[index]
  }

  const setDisplacementMapUrl = (mapUrl: string) => {
    map.setAttribute('href', mapUrl)
    map.setAttributeNS(COSMIC_XLINK_NS, 'href', mapUrl)
  }

  const isCosmicOverlayElement = (element: Element) =>
    element.id === COSMIC_VISUAL_ID ||
    element.id === COSMIC_PARTICLES_ID ||
    element.classList.contains('cosmic-shell') ||
    element.classList.contains('cosmic-backdrop') ||
    element.classList.contains('starfield') ||
    element.classList.contains('meteors') ||
    Boolean(element.closest(`#${COSMIC_VISUAL_ID}, #${COSMIC_PARTICLES_ID}, .cosmic-pull-particle`))

  const getParticleColorFromElementAt = (
    element: HTMLElement | SVGElement,
    clientX: number,
    clientY: number
  ) => {
    const style = getComputedStyle(element)
    const rect = element.getBoundingClientRect()
    const sampledGradient = sampleGradientColor(style.backgroundImage, rect, clientX, clientY)
    if (sampledGradient) return sampledGradient

    const candidateValues = [
      style.backgroundColor,
      style.fill,
      style.stroke,
      style.color,
      style.borderTopColor,
      style.borderRightColor,
      style.borderBottomColor,
      style.borderLeftColor,
      style.outlineColor,
      style.fill,
      style.stroke,
      style.textDecorationColor,
      style.boxShadow,
      style.textShadow
    ]

    for (const candidateValue of candidateValues) {
      for (const value of extractCssColorValues(candidateValue)) {
        const color = normalizeParticleColor(value)
        if (!color) continue
        return color
      }
    }

    return null
  }

  const isExtractableElement = (element: HTMLElement | SVGElement) => {
    const rect = element.getBoundingClientRect()
    if (rect.width < 5 || rect.height < 5) return false
    if (rect.bottom < 0 || rect.right < 0 || rect.top > window.innerHeight || rect.left > window.innerWidth) {
      return false
    }

    const style = getComputedStyle(element)
    const isLargeLayout =
      rect.width > window.innerWidth * 0.72 && rect.height > window.innerHeight * 0.48
    const hasVectorPaint =
      element instanceof SVGElement &&
      (Boolean(parseCssColor(style.fill)?.alpha) || Boolean(parseCssColor(style.stroke)?.alpha))
    const hasSurface =
      style.backgroundImage !== 'none' ||
      Boolean(parseCssColor(style.backgroundColor)?.alpha) ||
      hasVectorPaint
    const tagName = element.tagName.toLowerCase()
    const hasOwnText =
      /^(a|button|h[1-6]|p|span|label|strong|em|small|li|td|th)$/.test(tagName) &&
      Boolean(element.textContent?.trim())

    return !isLargeLayout || hasSurface || hasOwnText
  }

  const getElementColorSourceAt = (clientX: number, clientY: number): CosmicParticleSource | null => {
    for (const element of document.elementsFromPoint(clientX, clientY)) {
      if (!(element instanceof HTMLElement || element instanceof SVGElement)) continue
      if (isCosmicOverlayElement(element)) continue
      if (element === document.documentElement || element === document.body) continue
      if (!isExtractableElement(element)) continue

      const rect = element.getBoundingClientRect()
      const sourceX = clamp(clientX, rect.left + 1, rect.right - 1)
      const sourceY = clamp(clientY, rect.top + 1, rect.bottom - 1)
      const color = getParticleColorFromElementAt(element, sourceX, sourceY)
      if (!color) continue

      const distance = Math.hypot(nextClientX - sourceX, nextClientY - sourceY)
      if (distance < getEventHorizonRadius() + 6) continue

      return {
        color,
        distance,
        x: sourceX,
        y: sourceY
      }
    }

    return null
  }

  const getPullParticleSource = () => {
    const radius = getRadius()
    const attempts = 30

    for (let attempt = 0; attempt < attempts; attempt += 1) {
      const angle = ((particleSeed * 97 + attempt * 53) * Math.PI) / 180
      const distance = radius * (0.18 + (attempt % 7) * 0.16)
      const sampleX = nextClientX + Math.cos(angle) * distance
      const sampleY = nextClientY + Math.sin(angle) * distance

      if (sampleX < 0 || sampleX > window.innerWidth || sampleY < 0 || sampleY > window.innerHeight) {
        continue
      }

      const source = getElementColorSourceAt(sampleX, sampleY)
      if (source) return source
    }

    return getElementColorSourceAt(nextClientX, nextClientY)
  }

  const updateFilterBounds = () => {
    const padding = getRadius() * 3
    const surfaceOffset = getDistortionSurfaceOffset()
    filter.setAttribute('x', `${window.scrollX - surfaceOffset.x - padding}`)
    filter.setAttribute('y', `${window.scrollY - surfaceOffset.y - padding}`)
    filter.setAttribute('width', `${window.innerWidth + padding * 2}`)
    filter.setAttribute('height', `${window.innerHeight + padding * 2}`)
  }

  const getDistortionSurfaceOffset = () => {
    const surface = document.querySelector<HTMLElement>(COSMIC_DISTORTION_SURFACE_SELECTOR)
    if (!surface) return { x: 0, y: 0 }

    const rect = surface.getBoundingClientRect()
    return {
      x: rect.left + window.scrollX,
      y: rect.top + window.scrollY
    }
  }

  const getPullMaxScale = () =>
    cursorField === 'attract' ? COSMIC_BLACK_HOLE_PULL_MAX_SCALE : COSMIC_PULL_MAX_SCALE

  const getPullBaseScale = () =>
    cursorField === 'repel' ? COSMIC_WHITE_HOLE_PULL_BASE_SCALE : COSMIC_PULL_BASE_SCALE

  const updatePullScale = () => {
    if (!isActive) {
      return
    }

    const progress = Math.min(1, (performance.now() - holdStartedAt) / COSMIC_PULL_RAMP_MS)
    const eased = progress * progress * (3 - 2 * progress)
    const baseScale = getPullBaseScale()
    const maxScale = getPullMaxScale()
    const scale = baseScale + (maxScale - baseScale) * eased
    displacement.setAttribute('scale', scale.toFixed(1))
    root.style.setProperty('--cosmic-pull-strength', eased.toFixed(3))

    if (progress >= 1 && pullTimer) {
      window.clearInterval(pullTimer)
      pullTimer = 0
    }
  }

  const startPullRamp = () => {
    if (!holdStartedAt) holdStartedAt = performance.now()
    updatePullScale()
    if (!pullTimer) pullTimer = window.setInterval(updatePullScale, 80)
  }

  const resetPullRamp = (clientX: number, clientY: number) => {
    anchorClientX = clientX
    anchorClientY = clientY
    holdStartedAt = performance.now()
    root.style.setProperty('--cosmic-pull-strength', '0')
    if (isActive) {
      displacement.setAttribute('scale', `${getPullBaseScale()}`)
      startPullRamp()
    }
  }

  const stopPullRamp = () => {
    if (pullTimer) window.clearInterval(pullTimer)
    pullTimer = 0
    holdStartedAt = 0
    root.style.setProperty('--cosmic-pull-strength', '0')
  }

  const updateSpinFrame = (timestamp: number) => {
    const strength = Number.parseFloat(root.style.getPropertyValue('--cosmic-pull-strength')) || 0
    const frameMs = Math.max(COSMIC_SPIN_MIN_FRAME_MS, COSMIC_SPIN_FRAME_MS - strength * 24)

    if (!lastSpinFrameAt || timestamp - lastSpinFrameAt >= frameMs) {
      spinFrameIndex = (spinFrameIndex + 1) % COSMIC_SPIN_FRAME_COUNT
      setDisplacementMapUrl(getSpinFrame(spinFrameIndex))
      lastSpinFrameAt = timestamp
    }

    spinFrameRequest = window.requestAnimationFrame(updateSpinFrame)
  }

  const startSpinFrameLoop = () => {
    if (!spinFrameRequest) {
      spinFrameRequest = window.requestAnimationFrame(updateSpinFrame)
    }
  }

  const stopSpinFrameLoop = () => {
    if (spinFrameRequest) window.cancelAnimationFrame(spinFrameRequest)
    spinFrameRequest = 0
    lastSpinFrameAt = 0
  }

  const spawnPullParticle = () => {
    particleSeed += 1
    const strength = Number.parseFloat(root.style.getPropertyValue('--cosmic-pull-strength')) || 0
    const source = getPullParticleSource()
    if (!source) return

    while (particles.childElementCount >= COSMIC_MAX_PARTICLES) {
      particles.firstElementChild?.remove()
    }

    const eventHorizonRadius = getEventHorizonRadius() * 0.92
    const sourceAngle = Math.atan2(source.y - nextClientY, source.x - nextClientX)
    const isRepulsive = cursorField === 'repel'
    const spinAdvance =
      COSMIC_SPIN_DIRECTION * Math.min(1.55, 0.72 + strength * 0.46 + source.distance / 360)
    const startAngle = isRepulsive ? sourceAngle - spinAdvance : sourceAngle
    const startRadius = isRepulsive ? eventHorizonRadius : source.distance
    const startX = nextClientX + Math.cos(startAngle) * startRadius
    const startY = nextClientY + Math.sin(startAngle) * startRadius
    const spiralPoint = (progress: number) => {
      const angleProgress = 1 - Math.pow(1 - progress, 1.18)
      const radius = isRepulsive
        ? eventHorizonRadius +
          (source.distance - eventHorizonRadius) * (1 - Math.pow(1 - progress, 1.38))
        : eventHorizonRadius +
          (source.distance - eventHorizonRadius) * Math.pow(1 - progress, 1.45)
      const angle = isRepulsive
        ? sourceAngle - spinAdvance * (1 - angleProgress)
        : sourceAngle + spinAdvance * angleProgress
      return {
        x: nextClientX + Math.cos(angle) * radius - startX,
        y: nextClientY + Math.sin(angle) * radius - startY
      }
    }
    const p1 = spiralPoint(0.18)
    const p2 = spiralPoint(0.42)
    const p3 = spiralPoint(0.66)
    const p4 = spiralPoint(0.84)
    const p5 = spiralPoint(1)
    const segmentAngle = (fromX: number, fromY: number, toX: number, toY: number) =>
      Math.atan2(toY - fromY, toX - fromX)
    const life = COSMIC_PARTICLE_LIFETIME_MS + Math.min(source.distance, 180) * 1.75 - strength * 120
    const particleSize = 1.8 + (particleSeed % 5) * 0.28
    const particleLength = 3.2 + Math.min(source.distance / 80, 2.6)

    const particle = document.createElement('span')
    particle.classList.add('cosmic-pull-particle')
    particle.style.left = `${startX}px`
    particle.style.top = `${startY}px`
    particle.style.setProperty('--particle-p1-x', `${p1.x.toFixed(1)}px`)
    particle.style.setProperty('--particle-p1-y', `${p1.y.toFixed(1)}px`)
    particle.style.setProperty('--particle-p2-x', `${p2.x.toFixed(1)}px`)
    particle.style.setProperty('--particle-p2-y', `${p2.y.toFixed(1)}px`)
    particle.style.setProperty('--particle-p3-x', `${p3.x.toFixed(1)}px`)
    particle.style.setProperty('--particle-p3-y', `${p3.y.toFixed(1)}px`)
    particle.style.setProperty('--particle-p4-x', `${p4.x.toFixed(1)}px`)
    particle.style.setProperty('--particle-p4-y', `${p4.y.toFixed(1)}px`)
    particle.style.setProperty('--particle-end-x', `${p5.x.toFixed(1)}px`)
    particle.style.setProperty('--particle-end-y', `${p5.y.toFixed(1)}px`)
    particle.style.setProperty('--particle-angle', `${segmentAngle(0, 0, p1.x, p1.y)}rad`)
    particle.style.setProperty('--particle-p1-angle', `${segmentAngle(0, 0, p1.x, p1.y)}rad`)
    particle.style.setProperty('--particle-p2-angle', `${segmentAngle(p1.x, p1.y, p2.x, p2.y)}rad`)
    particle.style.setProperty('--particle-p3-angle', `${segmentAngle(p2.x, p2.y, p3.x, p3.y)}rad`)
    particle.style.setProperty('--particle-p4-angle', `${segmentAngle(p3.x, p3.y, p4.x, p4.y)}rad`)
    particle.style.setProperty('--particle-end-angle', `${segmentAngle(p4.x, p4.y, p5.x, p5.y)}rad`)
    particle.style.setProperty('--particle-color', source.color)
    particle.style.setProperty('--particle-life', `${life.toFixed(0)}ms`)
    particle.style.setProperty('--particle-size', `${particleSize.toFixed(1)}px`)
    particle.style.setProperty('--particle-length', particleLength.toFixed(2))

    particle.addEventListener('animationend', () => particle.remove(), { once: true })
    particles.append(particle)
  }

  const emitPullParticles = () => {
    if (!isActive) return

    const strength = Number.parseFloat(root.style.getPropertyValue('--cosmic-pull-strength')) || 0
    const count = strength > 0.72 ? 3 : strength > 0.36 ? 2 : 1
    for (let index = 0; index < count; index += 1) {
      spawnPullParticle()
    }
  }

  const startParticleEmitter = () => {
    emitPullParticles()
    if (!particleTimer) {
      particleTimer = window.setInterval(emitPullParticles, COSMIC_PARTICLE_INTERVAL_MS)
    }
  }

  const stopParticleEmitter = () => {
    if (particleTimer) window.clearInterval(particleTimer)
    particleTimer = 0
  }

  const setActive = (active: boolean) => {
    isActive = active
    root.classList.toggle('cosmic-cursor-active', active)
    root.style.setProperty('--cosmic-cursor-active', active ? '1' : '0')
    root.style.setProperty('--cosmic-cursor-scale', active ? '1' : '0')
    displacement.setAttribute('scale', active ? `${getPullBaseScale()}` : '0')
    if (active) {
      startPullRamp()
      startSpinFrameLoop()
      startParticleEmitter()
    } else {
      wakeAnchorX = nextClientX
      wakeAnchorY = nextClientY
      stopPullRamp()
      stopSpinFrameLoop()
      stopParticleEmitter()
    }
  }

  const cancelCollapse = () => {
    if (collapseFrame) window.cancelAnimationFrame(collapseFrame)
    collapseFrame = 0
    if (isActive) {
      root.style.setProperty('--cosmic-cursor-scale', '1')
    }
  }

  const clearIdleTimer = () => {
    if (idleTimer) {
      window.clearTimeout(idleTimer)
      idleTimer = 0
    }
  }

  const startCollapse = () => {
    if (!isActive || collapseFrame) return

    stopParticleEmitter()
    if (pullTimer) window.clearInterval(pullTimer)
    pullTimer = 0
    const startedAt = performance.now()
    const initialStrength = Number.parseFloat(root.style.getPropertyValue('--cosmic-pull-strength')) || 0
    const initialDisplacementScale =
      Number.parseFloat(displacement.getAttribute('scale') || '') || getPullBaseScale()

    const step = (timestamp: number) => {
      const progress = Math.min(1, (timestamp - startedAt) / COSMIC_COLLAPSE_MS)
      const easedProgress = progress * progress * progress * (progress * (progress * 6 - 15) + 10)
      const remaining = 1 - easedProgress
      const visualScale = Math.max(0.04, Math.pow(remaining, 0.82))
      const pullScale = Math.pow(remaining, 1.12)
      root.style.setProperty('--cosmic-cursor-scale', visualScale.toFixed(3))
      root.style.setProperty('--cosmic-pull-strength', (initialStrength * pullScale).toFixed(3))
      displacement.setAttribute('scale', (initialDisplacementScale * pullScale).toFixed(1))

      if (progress < 1) {
        collapseFrame = window.requestAnimationFrame(step)
        return
      }

      collapseFrame = 0
      setActive(false)
    }

    collapseFrame = window.requestAnimationFrame(step)
  }

  // 鼠标静止超时后先收缩，再卸下整页 SVG 滤镜，避免突然消失和空转发热
  const scheduleIdleStop = () => {
    clearIdleTimer()
    idleTimer = window.setTimeout(() => {
      idleTimer = 0
      startCollapse()
    }, COSMIC_IDLE_TIMEOUT_MS)
  }

  const applyPosition = () => {
    frame = 0
    const radius = getRadius()
    const pageX = nextClientX + window.scrollX
    const pageY = nextClientY + window.scrollY
    const surfaceOffset = getDistortionSurfaceOffset()
    const localX = pageX - surfaceOffset.x
    const localY = pageY - surfaceOffset.y

    root.style.setProperty('--cosmic-cursor-x', `${nextClientX}px`)
    root.style.setProperty('--cosmic-cursor-y', `${nextClientY}px`)
    map.setAttribute('x', `${localX - radius}`)
    map.setAttribute('y', `${localY - radius}`)
    map.setAttribute('width', `${radius * 2}`)
    map.setAttribute('height', `${radius * 2}`)
    if (!isActive) setActive(true)
  }

  const schedulePosition = () => {
    if (!frame) frame = window.requestAnimationFrame(applyPosition)
  }

  syncCursorMode()
  setDisplacementMapUrl(getSpinFrame(spinFrameIndex))
  const modeObserver =
    typeof MutationObserver === 'function'
      ? new MutationObserver(() => {
          const previousMode = cursorMode
          const previousField = cursorField
          syncCursorMode()
          if (previousMode !== cursorMode || previousField !== cursorField) {
            spinFrameIndex = 0
            setDisplacementMapUrl(getSpinFrame(spinFrameIndex))
          }
        })
      : null
  modeObserver?.observe(root, { attributes: true, attributeFilter: ['class'] })

  updateFilterBounds()

  window.addEventListener('pointermove', (event) => {
    // 熄火状态下，微小漂移不唤醒，避免触控板/鼠标抖动反复点亮整页滤镜
    if (!isActive && !collapseFrame) {
      const wakeDistance = Math.hypot(event.clientX - wakeAnchorX, event.clientY - wakeAnchorY)
      if (wakeDistance < COSMIC_WAKE_THRESHOLD_PX) {
        nextClientX = event.clientX
        nextClientY = event.clientY
        return
      }
    }

    const hasMovedAway =
      Math.hypot(event.clientX - anchorClientX, event.clientY - anchorClientY) >
      COSMIC_PULL_MOVE_RESET_PX
    const wasCollapsing = Boolean(collapseFrame)

    if (wasCollapsing) {
      cancelCollapse()
    }

    if (!isActive || hasMovedAway || wasCollapsing) {
      resetPullRamp(event.clientX, event.clientY)
    }

    nextClientX = event.clientX
    nextClientY = event.clientY
    schedulePosition()
    scheduleIdleStop()
  }, { passive: true })

  const deactivateCursor = () => {
    clearIdleTimer()
    cancelCollapse()
    setActive(false)
  }

  document.addEventListener('mouseleave', deactivateCursor, { passive: true })
  window.addEventListener('blur', deactivateCursor, { passive: true })
  document.addEventListener('visibilitychange', () => {
    // 隐藏时冻结所有常驻背景动画（星空/流星），并熄火黑洞，避免后台空转发热
    root.classList.toggle('cosmic-paused', document.hidden)
    if (document.hidden) deactivateCursor()
  }, { passive: true })
  window.addEventListener('scroll', () => {
    updateFilterBounds()
    if (isActive) schedulePosition()
  }, { passive: true })
  window.addEventListener('resize', () => {
    updateFilterBounds()
    if (isActive) schedulePosition()
  }, { passive: true })

  root.style.setProperty('--cosmic-cursor-x', `${nextClientX}px`)
  root.style.setProperty('--cosmic-cursor-y', `${nextClientY}px`)
}

async function bootstrap() {
  // Apply theme class globally before app mount to keep all routes consistent.
  initThemeClass()
  initCosmicCursor()

  const app = createApp(App)
  const pinia = createPinia()
  app.use(pinia)

  // Initialize settings from injected config BEFORE mounting (prevents flash)
  // This must happen after pinia is installed but before router and i18n
  const appStore = useAppStore()
  appStore.initFromInjectedConfig()

  // Set document title immediately after config is loaded
  if (appStore.siteName && appStore.siteName !== 'Sub2API') {
    document.title = `${appStore.siteName} - AI API Gateway`
  }
  appStore.initTheme()

  await initI18n()

  app.use(router)
  app.use(i18n)

  // 等待路由器完成初始导航后再挂载，避免竞态条件导致的空白渲染
  await router.isReady()
  app.mount('#app')
}

bootstrap()
