// V5/W6 SETTINGS-V2 — translate the admin schema payload into the
// `PropDescriptor[]` shape every widget understands.
//
// `info.schema` is JSONSchema; `info.properties_meta` and `info.secret_keys`
// are the SETTINGS-V2 sidecar fields the host attaches to the GET
// response. Both sidecars MAY be undefined when an older host is running
// (W3-C wires them) — the function defaults them so widget code never
// sees `undefined` metadata.
//
// Schema declaration order is preserved on purpose (see DESIGN §5.7):
// `Object.keys()` returns string keys in insertion order, which is
// exactly the JSONSchema author's intent.

import type { PluginSettingsSchemaInfo } from '@/api/admin/pluginSettings'
import type { PropDescriptor, PropertyMetadata } from './types'

const DEFAULT_META: PropertyMetadata = {
  visibility: 'frontend',
  deprecated: '',
  requires_reload: false,
}

const SCALAR_TYPES = new Set(['boolean', 'string', 'number', 'integer'])

/**
 * SchemaInfoLike is the structural minimum buildPropDescriptors needs.
 * Until W3-C extends `PluginSettingsSchemaInfo` with `properties_meta`
 * and `secret_keys`, those sidecars come in via `Record<string, unknown>`
 * casts and resolve to the defaults above.
 */
type SchemaInfoLike = PluginSettingsSchemaInfo & {
  properties_meta?: Record<string, PropertyMetadata>
  secret_keys?: string[]
}

export function buildPropDescriptors(info: PluginSettingsSchemaInfo, locale = 'en'): PropDescriptor[] {
  const schema = info.schema as Record<string, unknown> | undefined
  if (!schema || typeof schema !== 'object') return []
  const propsMap = schema['properties'] as Record<string, Record<string, unknown>> | undefined
  if (!propsMap) return []

  const sidecar = info as SchemaInfoLike
  const meta = sidecar.properties_meta ?? {}
  const secretSet = new Set(sidecar.secret_keys ?? [])

  return Object.keys(propsMap).map((key) => {
    const node = propsMap[key] || {}
    const m = meta[key] ?? DEFAULT_META

    const enumNode = node['enum']
    const enumVals = Array.isArray(enumNode) ? (enumNode as unknown[]) : undefined
    const enumValues = enumVals
      ? enumVals.map((v) => ({ value: v, label: String(v) }))
      : undefined

    const declaredType = typeof node['type'] === 'string' ? (node['type'] as string) : ''
    let type: PropDescriptor['type']
    if (m.visibility === 'secret') {
      type = 'secret'
    } else if (enumValues && enumValues.length > 0) {
      type = 'enum'
    } else if (SCALAR_TYPES.has(declaredType)) {
      type = declaredType as PropDescriptor['type']
    } else {
      type = 'json'
    }

    // Extract locale-specific overrides from x-i18n if present
    const i18nOverrides = (node['x-i18n'] as Record<string, Record<string, string>> | undefined)?.[locale]
    const title = i18nOverrides?.title ?? (typeof node['title'] === 'string' ? (node['title'] as string) : key)
    const description = i18nOverrides?.description ?? (typeof node['description'] === 'string' ? (node['description'] as string) : '')

    return {
      key,
      type,
      title,
      description,
      enumValues,
      visibility: m.visibility,
      deprecated: m.deprecated,
      requiresReload: m.requires_reload,
      isConfigured: secretSet.has(key),
    }
  })
}
