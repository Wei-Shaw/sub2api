import type { UserMaterialKind } from '@/api/userMaterials'

export type MediaUrlWidget = 'ImageUrls' | 'VideoUrls' | 'AudioUrls'
export type SingleMediaWidget = 'image' | 'video' | 'audio'

export const MEDIA_URL_WIDGETS: readonly MediaUrlWidget[] = [
  'ImageUrls',
  'VideoUrls',
  'AudioUrls',
]

const MEDIA_EXTENSIONS: Record<UserMaterialKind, readonly string[]> = {
  image: ['.png', '.jpg', '.jpeg', '.webp', '.gif'],
  video: ['.mp4', '.mov', '.webm', '.m4v'],
  audio: ['.mp3', '.wav', '.ogg', '.m4a', '.aac', '.flac'],
}

/** Read the old lowercase image widget, but always return its canonical name. */
export function normalizeMediaUrlWidget(value: unknown): MediaUrlWidget | null {
  if (value === 'imageUrls') return 'ImageUrls'
  return MEDIA_URL_WIDGETS.includes(value as MediaUrlWidget) ? (value as MediaUrlWidget) : null
}

/** Normalize single-value media widgets, including common legacy spellings. */
export function normalizeSingleMediaWidget(value: unknown): SingleMediaWidget | null {
  if (typeof value !== 'string') return null
  switch (value.toLowerCase()) {
    case 'image':
    case 'imageurl':
      return 'image'
    case 'video':
    case 'videourl':
      return 'video'
    case 'audio':
    case 'audiourl':
      return 'audio'
    default:
      return null
  }
}

export function mediaKindForWidget(widget: MediaUrlWidget): UserMaterialKind {
  if (widget === 'VideoUrls') return 'video'
  if (widget === 'AudioUrls') return 'audio'
  return 'image'
}

export function mediaExtensions(kind: UserMaterialKind): readonly string[] {
  return MEDIA_EXTENSIONS[kind]
}

export function mediaFileAccept(kind: UserMaterialKind): string {
  return MEDIA_EXTENSIONS[kind].join(',')
}

export function hasAllowedMediaExtension(value: string, kind: UserMaterialKind): boolean {
  let path = value
  try {
    path = new URL(value).pathname
  } catch {
    path = value.split(/[?#]/, 1)[0]
  }
  const lower = path.toLowerCase()
  return MEDIA_EXTENSIONS[kind].some((extension) => lower.endsWith(extension))
}
