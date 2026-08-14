import { readdirSync, readFileSync, statSync } from 'node:fs'
import { dirname, join, relative, resolve, sep } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '../..')

/**
 * The orphan-key ratchet.
 *
 * A message tree only ever grows on its own. A key whose call site was deleted
 * leaves no trace: nothing fails, nothing warns, the string just sits there and
 * gets translated forever. This spec makes that debt a number.
 *
 * How a key counts as REACHED:
 *   - its full dotted path appears as a string literal in any non-spec source
 *     file (broader than modelling `t(...)` call shapes, so it cannot miss a
 *     call site), or
 *   - it sits under a prefix that some file builds a key from dynamically —
 *     `'admin.groups.platforms.' + platform`, `` `payment.status.${s}` `` and
 *     friends. Those keys are unresolvable statically, so the whole subtree is
 *     treated as reached. Deleting one of them would render the key path into
 *     the UI, silently.
 *
 * The one shape a call-site scan genuinely cannot see is a key assembled inside
 * a helper, where neither half is written next to the other — the namespace
 * arrives as an argument and the code as data. The interior-node rule below
 * covers the case that occurs in this tree. A key whose namespace is ALSO
 * computed would still slip through, and nothing here would notice.
 *
 * ADDING A NUMBER HERE IS NOT A FIX. The only correct direction is down, and
 * the counts are exact on purpose: paying debt off means editing this file,
 * which puts it in the diff where a reviewer can see it.
 */
const ALLOWED_ORPHANS: Record<string, number> = {
  'admin.settings': 13,
  'admin.channels': 9,
  'admin.riskControl': 8,
  'payment.qr': 8,
  'profile.balanceNotify': 8,
  'admin.audit': 6,
  'admin.channelMonitor': 6,
  'channelStatus.columns': 6,
  'admin.backup': 5,
  'channelMonitorV2.metrics': 5,
  'admin.promo': 4,
  'admin.subscriptions': 4,
  'auth.linuxdo': 4,
  'auth.wechatPayment': 4,
  'keys.useKeyModal': 4,
  'payment.stripePopup': 4,
  'admin.usage': 3,
  'auth.dingtalk': 3,
  'auth.oidc': 3,
  'admin.affiliates': 1,
  'admin.tlsFingerprintProfiles': 2,
  'onboarding.user': 2,
  'payment.orders': 2,
  'setup.status': 2,
  'admin.announcements': 1,
  'affiliate.stats': 1,
  'affiliate.transfer': 1,
  'announcements.description': 1,
  'announcements.emptyUnread': 1,
  'announcements.endsAt': 1,
  'announcements.newCount': 1,
  'announcements.readAt': 1,
  'announcements.startsAt': 1,
  'announcements.total': 1,
  'announcements.unreadOnly': 1,
  'announcements.viewAll': 1,
  'auth.captchaLoading': 1,
  'auth.oauthFlow': 1,
  'auth.passkeySigningIn': 1,
  'auth.processing': 1,
  'auth.rememberMe': 1,
  'auth.resettingPassword': 1,
  'auth.sendingCode': 1,
  'auth.sendingResetLink': 1,
  'auth.signingIn': 1,
  'auth.verifying': 1,
  'auth.wechatPaymentCallbackPageTitle': 1,
  'channelMonitorV2.matrix': 1,
  'channelMonitorV2.refreshingFilters': 1,
  'channelMonitorV2.settings': 1,
  'channelMonitorV2.switchingData': 1,
  'channelStatus.allProviders': 1,
  'channelStatus.searchPlaceholder': 1,
  'common.collapse': 1,
  'common.expand': 1,
  'common.export': 1,
  'common.filter': 1,
  'common.import': 1,
  'common.justNow': 1,
  'common.none': 1,
  'common.notAvailable': 1,
  'common.now': 1,
  'common.password': 1,
  'common.sending': 1,
  'common.submit': 1,
  'dashboard.cache': 1,
  'dashboard.cacheToday': 1,
  'dashboard.group': 1,
  'dashboard.groupDistribution': 1,
  'dashboard.noGroup': 1,
  'dashboard.platformBreakdownEmpty': 1,
  'dashboard.tokenUsageTrend': 1,
  'dates.custom': 1,
  'dates.lastWeek': 1,
  'dates.thisWeek': 1,
  'errors.forbidden': 1,
  'errors.serverError': 1,
  'errors.somethingWentWrong': 1,
  'errors.timeout': 1,
  'errors.tryAgain': 1,
  'errors.unauthorized': 1,
  'keyUsage.querying': 1,
  'keyUsage.used': 1,
  'keys.columnAlwaysVisible': 1,
  'keys.quotaAmount': 1,
  'keys.rateLimitUsage': 1,
  'keys.saving': 1,
  'modelPlaza.detail': 1,
  'modelPlaza.loading': 1,
  'monitorCommon.pollEvery': 1,
  'monitorCommon.updatedAt': 1,
  'nav.channels': 1,
  'nav.paymentConfig': 1,
  'nav.riskControl': 1,
  'onboarding.confirmDontShow': 1,
  'onboarding.confirmExit': 1,
  'onboarding.dontShowAgain': 1,
  'onboarding.dontShowAgainTitle': 1,
  'payment.airwallexLoadFailed': 1,
  'payment.airwallexMissingParams': 1,
  'payment.airwallexPay': 1,
  'payment.confirmSubscription': 1,
  'payment.noActiveSubscription': 1,
  'payment.oneMonth': 1,
  'payment.oneYear': 1,
  'payment.perYear': 1,
  'payment.planFeatures': 1,
  'payment.stripeLoadFailed': 1,
  'payment.stripeMissingParams': 1,
  'payment.stripeNotConfigured': 1,
  'payment.stripePay': 1,
  'payment.stripeSuccessProcessing': 1,
  'payment.years': 1,
  'profile.changingPassword': 1,
  'profile.email': 1,
  'profile.overviewDescription': 1,
  'profile.overviewTitle': 1,
  'profile.role': 1,
  'profile.rpmLimit': 1,
  'profile.rpmUnlimited': 1,
  'profile.securityDescription': 1,
  'profile.securityTitle': 1,
  'profile.status': 1,
  'profile.updating': 1,
  'purchase.notConfiguredDesc': 1,
  'purchase.notConfiguredTitle': 1,
  'purchase.notEnabledDesc': 1,
  'purchase.notEnabledTitle': 1,
  'purchase.openInNewTab': 1,
  'purchase.title': 1,
  'redeem.newBalance': 1,
  'redeem.newConcurrency': 1,
  'redeem.redeeming': 1,
  'redeem.subscriptionAssignedDesc': 1,
  'subscriptionProgress.noSubscriptions': 1,
  'table.collapseActions': 1,
  'table.expandActions': 1,
  'usage.actualCost': 1,
  'usage.billed': 1,
  'usage.cacheBreakdown': 1,
  'usage.cacheCreate': 1,
  'usage.cacheHit': 1,
  'usage.cacheHitRate': 1,
  'usage.cacheRead': 1,
  'usage.cacheWrite': 1,
  'usage.exportCancelled': 1,
  'usage.exportExcelFailed': 1,
  'usage.exportExcelSuccess': 1,
  'usage.perRequest': 1,
  'usage.timeRange': 1,
  'userSubscriptions.expiresOn': 1,
  'userSubscriptions.usage': 1,
  'userSubscriptions.usageOf': 1,
  'version.noReleaseNotes': 1,
  'version.releaseNotes': 1,
  'version.sourceMode': 1,
  'version.viewUpdate': 1,
}

function flatten(node: unknown, prefix: string, out: Set<string>, interior: Set<string>): void {
  if (node === null || typeof node !== 'object') return
  for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
    const path = prefix ? `${prefix}.${k}` : k
    if (typeof v === 'string') out.add(path)
    else if (typeof v === 'object' && v !== null) {
      interior.add(path)
      flatten(v, path, out, interior)
    }
  }
}

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === 'node_modules' || entry === 'assets' || entry === '__tests__') continue
    const full = join(dir, entry)
    if (statSync(full).isDirectory()) {
      // The message trees themselves are definitions, not call sites.
      if (relative(SRC, full).split(sep).join('/') === 'i18n/locales') continue
      walk(full, out)
    } else if (/\.(vue|ts)$/.test(entry) && !/\.spec\.ts$/.test(entry)) {
      out.push(full)
    }
  }
  return out
}

const keys = new Set<string>()
const interior = new Set<string>()
flatten(en, '', keys, interior)
flatten(zh, '', keys, interior)

const sources = walk(SRC).map((f) => readFileSync(f, 'utf8'))

const literals = new Set<string>()
const dynamicPrefixes = new Set<string>()
for (const source of sources) {
  for (const m of source.matchAll(/['"`]([A-Za-z0-9_]+(?:\.[A-Za-z0-9_]+)+)['"`]/g)) {
    literals.add(m[1])
  }
  // The static head can end mid-segment, not only at a dot: the payment dialog
  // writes `` t(`admin.settings.payment.field_${f.key}`) ``, and a prefix
  // pattern that insisted on a trailing dot reported all thirty of those keys
  // as dead. Capture whatever dotted head precedes the interpolation.
  for (const m of source.matchAll(/['"]((?:[A-Za-z0-9_]+\.)+[A-Za-z0-9_]*)['"]\s*\+/g)) {
    dynamicPrefixes.add(m[1])
  }
  for (const m of source.matchAll(/`((?:[A-Za-z0-9_]+\.)+[A-Za-z0-9_]*)\$\{/g)) {
    dynamicPrefixes.add(m[1])
  }
}

/**
 * A literal that names an INTERIOR node — a subtree, not a leaf — is a
 * namespace being handed to something that will append a code to it.
 * `extractI18nErrorMessage(err, t, 'payment.errors', fallback)` builds
 * `` `${namespace}.${code}` `` inside the helper, so the head never appears
 * next to the interpolation and no amount of pattern-matching at the call site
 * can see it. Thirty-nine backend error codes were reported dead this way.
 *
 * Writing a subtree's own path in code has no other purpose, so treating the
 * whole subtree as reached is both correct here and safe in general: it errs
 * toward keeping keys.
 */
for (const literal of literals) {
  if (interior.has(literal)) dynamicPrefixes.add(`${literal}.`)
}

const prefixes = [...dynamicPrefixes]
const orphans = [...keys]
  .filter((k) => !literals.has(k) && !prefixes.some((p) => k.startsWith(p)))
  .sort()

/** Two segments: fine enough to point at a feature, coarse enough to review. */
const namespaceOf = (key: string) => key.split('.').slice(0, 2).join('.')

describe('i18n orphan keys', () => {
  it('scans a plausible number of files and keys', () => {
    // Guards against the walker matching nothing, which would make every
    // assertion below vacuously pass.
    expect(sources.length).toBeGreaterThan(400)
    expect(keys.size).toBeGreaterThan(5000)
    expect(prefixes.length).toBeGreaterThan(30)
  })

  it('has no orphan outside a declared namespace', () => {
    const undeclared = [...new Set(orphans.map(namespaceOf))]
      .filter((ns) => !(ns in ALLOWED_ORPHANS))
      .sort()

    expect(
      undeclared,
      'A message tree grew a key nothing can reach. Delete the key, or — if a ' +
        'call site is coming — add the namespace to ALLOWED_ORPHANS as debt.'
    ).toEqual([])
  })

  it('keeps every declared count exact', () => {
    const actual = new Map<string, number>()
    for (const key of orphans) {
      const ns = namespaceOf(key)
      actual.set(ns, (actual.get(ns) ?? 0) + 1)
    }

    const drift = Object.entries(ALLOWED_ORPHANS)
      .map(([ns, allowed]) => ({ ns, allowed, found: actual.get(ns) ?? 0 }))
      .filter((row) => row.found !== row.allowed)
      .map((row) =>
        row.found > row.allowed
          ? `${row.ns}: ${row.found} orphans, ${row.allowed} declared — new dead keys`
          : `${row.ns}: ${row.found} orphans, ${row.allowed} declared — lower it to ${row.found}` +
            (row.found === 0 ? ' (or delete the entry)' : '')
      )
      .sort()

    expect(drift, 'ALLOWED_ORPHANS is out of date.').toEqual([])
  })
})
