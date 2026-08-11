import type { FieldSpec } from './paramSpec'
import {
  mediaKindForWidget,
  normalizeMediaUrlWidget,
  normalizeSingleMediaWidget,
} from '@/utils/mediaUrlWidget'

export type PromptMediaKind = 'image' | 'video' | 'audio'

export interface PromptMediaReference {
  label: string
  kind: PromptMediaKind
  url: string
  fieldKey: string
  itemIndex: number
}

function mediaKindForSpec(spec: FieldSpec): PromptMediaKind | null {
  const groupWidget = normalizeMediaUrlWidget(spec.widget)
  if (groupWidget) return mediaKindForWidget(groupWidget)

  const singleWidget = normalizeSingleMediaWidget(spec.widget)
  if (singleWidget) return singleWidget

  if (spec.rawType === 'array' && spec.items) {
    return normalizeSingleMediaWidget(spec.items.widget)
  }
  return null
}

/** Build stable media aliases in schema order, then array-item order. */
export function collectPromptMediaReferences(
  specs: FieldSpec[],
  values: Record<string, unknown>,
): PromptMediaReference[] {
  const counters: Record<PromptMediaKind, number> = { image: 0, video: 0, audio: 0 }
  const references: PromptMediaReference[] = []

  const append = (kind: PromptMediaKind, url: unknown, fieldKey: string, itemIndex: number) => {
    if (typeof url !== 'string' || !url.trim()) return
    counters[kind] += 1
    references.push({
      label: `@${kind.toUpperCase()}${counters[kind]}`,
      kind,
      url: url.trim(),
      fieldKey,
      itemIndex,
    })
  }

  const visit = (spec: FieldSpec, value: unknown, fieldKey: string) => {
    if (spec.rawType === 'object') {
      const objectValue = value && typeof value === 'object' && !Array.isArray(value)
        ? value as Record<string, unknown>
        : {}
      for (const child of spec.children) {
        visit(child, objectValue[child.key], fieldKey ? `${fieldKey}.${child.key}` : child.key)
      }
      return
    }

    if (spec.rawType === 'array') {
      const arrayValue = Array.isArray(value) ? value : []
      const kind = mediaKindForSpec(spec)
      if (kind) {
        arrayValue.forEach((item, index) => append(kind, item, fieldKey, index))
        return
      }
      if (spec.items) {
        arrayValue.forEach((item, index) => visit(spec.items!, item, `${fieldKey}[${index}]`))
      }
      return
    }

    const kind = mediaKindForSpec(spec)
    if (kind) append(kind, value, fieldKey, 0)
  }

  for (const spec of specs) visit(spec, values[spec.key], spec.key)
  return references
}
