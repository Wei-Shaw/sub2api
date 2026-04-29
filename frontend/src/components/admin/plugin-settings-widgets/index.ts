// V5/W6 SETTINGS-V2 widget registry.
//
// Public surface of the `plugin-settings-widgets` package:
//   - `resolveWidget(prop)` — picks a Vue component for a PropDescriptor
//   - `DeprecatedBadge` / `RequiresReloadBadge` — decorators
//   - re-exported types for consumers (PluginSettingsForm.vue, tests)
//
// The widget map itself is intentionally NOT exported — callers must go
// through `resolveWidget()` so the resolution rules (secret → enum →
// type) live in exactly one place.

import BooleanCheckbox from './widgets/BooleanCheckbox.vue'
import EnumSelect from './widgets/EnumSelect.vue'
import IntegerInput from './widgets/IntegerInput.vue'
import JsonTextarea from './widgets/JsonTextarea.vue'
import NumberInput from './widgets/NumberInput.vue'
import SecretInput from './widgets/SecretInput.vue'
import StringInput from './widgets/StringInput.vue'

import DeprecatedBadge from './decorators/DeprecatedBadge.vue'
import RequiresReloadBadge from './decorators/RequiresReloadBadge.vue'

import type { PropDescriptor, Widget } from './types'

/**
 * WidgetKey is the closed set of widget identifiers. Adding a new widget
 * requires extending this union, the `widgets` map below, AND the
 * `PropDescriptor.type` union in `./types.ts`.
 */
export type WidgetKey =
  | 'boolean'
  | 'string'
  | 'secret'
  | 'number'
  | 'integer'
  | 'enum'
  | 'json'

const widgets: Record<WidgetKey, Widget> = {
  boolean: BooleanCheckbox,
  string: StringInput,
  secret: SecretInput,
  number: NumberInput,
  integer: IntegerInput,
  enum: EnumSelect,
  json: JsonTextarea,
}

/**
 * resolveWidget picks the rendering component for a property descriptor.
 *
 * Order of precedence (higher wins):
 *   1. visibility === 'secret'        → SecretInput
 *   2. enumValues non-empty           → EnumSelect
 *   3. type matches a known widget    → corresponding widget
 *   4. fallback                       → JsonTextarea
 */
export function resolveWidget(prop: PropDescriptor): Widget {
  if (prop.visibility === 'secret') return widgets.secret
  if (prop.enumValues && prop.enumValues.length > 0) return widgets.enum
  return widgets[prop.type as WidgetKey] ?? widgets.json
}

export { DeprecatedBadge, RequiresReloadBadge }
export { buildPropDescriptors } from './buildPropDescriptors'
export { evaluateConditions, type EvaluatedConditions } from './evaluateConditions'
export type { Decorator, PropDescriptor, PropertyMetadata, Widget, WidgetProps } from './types'
