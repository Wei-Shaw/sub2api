import landing from './landing'
import common from './common'
import dashboard from './dashboard'
import admin from './admin'
import misc from './misc'
import forkExtras from './_fork_extras'

// Deep-merge helper: recursively merges plain-object sources into target.
// Used to re-inject fork-only i18n keys (support/plaza/rechargePromos/captcha
// etc.) on top of the upstream domain-module split without clobbering sibling
// keys. Fork values win on leaf conflicts.
function deepMerge(target: Record<string, any>, source: Record<string, any>): Record<string, any> {
  for (const key of Object.keys(source)) {
    const sv = source[key]
    const tv = target[key]
    if (
      sv && typeof sv === 'object' && !Array.isArray(sv) &&
      tv && typeof tv === 'object' && !Array.isArray(tv)
    ) {
      deepMerge(tv, sv)
    } else {
      target[key] = sv
    }
  }
  return target
}

const messages = {
  ...landing,
  ...common,
  ...dashboard,
  admin,
  ...misc,
}

export default deepMerge(messages, forkExtras as Record<string, any>)
