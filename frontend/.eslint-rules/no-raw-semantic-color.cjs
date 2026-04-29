'use strict'

/**
 * Forbid raw Tailwind semantic colors (emerald/red/blue/amber/green/yellow)
 * in plugin Vue template `class` attributes.
 *
 * Use SDK semantic utility classes instead (.btn-primary, .btn-icon-danger,
 * .input-required, .badge-*, .card-highlight-* etc.).
 *
 * Brand colors `primary-*` and `purple-*` are intentionally NOT forbidden:
 *   - primary-* is sub2api's Tailwind theme palette (defined in
 *     frontend/packages/plugin-sdk/tailwind-preset.cjs), used as the brand
 *     accent color across host and plugins.
 *   - purple-* is reserved for platform branding (e.g. Antigravity).
 *
 * Scope: only static `class="..."` literals in <template>. Dynamic `:class`
 * bindings are not analysed because their values are JS expressions; helper
 * dictionaries in <script> typically encode className mapping there.
 *
 * Implementation note: vue-eslint-parser exposes the template AST via
 * `context.parserServices.defineTemplateBodyVisitor`. Standard ESLint
 * visitors (`Program` walking) only see the <script> AST.
 */

const FORBIDDEN_COLORS = ['emerald', 'red', 'blue', 'amber', 'green', 'yellow']
const SHADES = '50|100|200|300|400|500|600|700|800|900|950'
const PATTERN = new RegExp(
  '\\b(?:' + FORBIDDEN_COLORS.join('|') + ')-(?:' + SHADES + ')\\b',
  'g',
)

module.exports = {
  meta: {
    type: 'problem',
    docs: {
      description:
        'Forbid raw Tailwind semantic colors in plugin Vue template class attributes',
      category: 'Best Practices',
    },
    schema: [],
    messages: {
      raw:
        'Raw Tailwind semantic color "{{match}}" found in plugin template class. ' +
        'Use SDK semantic class (.btn-*, .badge-*, .card-highlight-*, .input-required, etc.) instead.',
    },
  },
  create(context) {
    function checkAttribute(node) {
      // Skip directives (`:class="..."`, `v-bind:class="..."`) — values are
      // JS expressions; helper dictionaries in <script> handle that case.
      if (node.directive) return
      const key = node.key
      if (!key || key.type !== 'VIdentifier' || key.name !== 'class') return
      const value = node.value
      if (!value || value.type !== 'VLiteral') return
      const raw = value.value
      if (typeof raw !== 'string' || raw.length === 0) return
      PATTERN.lastIndex = 0
      let m
      while ((m = PATTERN.exec(raw)) !== null) {
        context.report({
          node: value,
          messageId: 'raw',
          data: { match: m[0] },
        })
      }
    }

    const services =
      context.parserServices ||
      (context.sourceCode && context.sourceCode.parserServices)
    if (services && typeof services.defineTemplateBodyVisitor === 'function') {
      return services.defineTemplateBodyVisitor({
        VAttribute: checkAttribute,
      })
    }
    // Non-Vue file or no template: no-op.
    return {}
  },
}
