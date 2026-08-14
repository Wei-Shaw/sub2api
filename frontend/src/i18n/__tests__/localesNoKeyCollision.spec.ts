import { readdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

import enAdminAccounts from '../locales/en/admin/accounts'
import enAdminAudit from '../locales/en/admin/audit'
import enAdminChannels from '../locales/en/admin/channels'
import enAdminOps from '../locales/en/admin/ops'
import enAdminOverview from '../locales/en/admin/overview'
import enAdminPromptAudit from '../locales/en/admin/promptAudit'
import enAdminResources from '../locales/en/admin/resources'
import enAdminSettings from '../locales/en/admin/settings'
import enBatchImage from '../locales/en/batchImage'
import enChannelMonitorV2 from '../locales/en/channelMonitorV2'
import enCommon from '../locales/en/common'
import enDashboard from '../locales/en/dashboard'
import enLanding from '../locales/en/landing'
import enMisc from '../locales/en/misc'
import zhAdminAccounts from '../locales/zh/admin/accounts'
import zhAdminAudit from '../locales/zh/admin/audit'
import zhAdminChannels from '../locales/zh/admin/channels'
import zhAdminOps from '../locales/zh/admin/ops'
import zhAdminOverview from '../locales/zh/admin/overview'
import zhAdminPromptAudit from '../locales/zh/admin/promptAudit'
import zhAdminResources from '../locales/zh/admin/resources'
import zhAdminSettings from '../locales/zh/admin/settings'
import zhBatchImage from '../locales/zh/batchImage'
import zhChannelMonitorV2 from '../locales/zh/channelMonitorV2'
import zhCommon from '../locales/zh/common'
import zhDashboard from '../locales/zh/dashboard'
import zhLanding from '../locales/zh/landing'
import zhMisc from '../locales/zh/misc'

// locales/{zh,en}/index.ts 与 admin/index.ts 使用对象展开聚合各域模块，
// 展开模块之间若出现同名顶层键会静默覆盖。本测试将该风险固化为显式失败。
type Modules = Record<string, Record<string, unknown>>

function collisions(modules: Modules): string[] {
  const seen = new Map<string, string>()
  const out: string[] = []
  for (const [name, mod] of Object.entries(modules)) {
    for (const key of Object.keys(mod)) {
      const prev = seen.get(key)
      if (prev) {
        out.push(`"${key}" in both ${prev} and ${name}`)
      } else {
        seen.set(key, name)
      }
    }
  }
  return out
}

const roots: Record<string, Modules> = {
  zh: {
    landing: zhLanding,
    common: zhCommon,
    dashboard: zhDashboard,
    channelMonitorV2: zhChannelMonitorV2,
    batchImage: zhBatchImage,
    misc: zhMisc
  },
  en: {
    landing: enLanding,
    common: enCommon,
    dashboard: enDashboard,
    channelMonitorV2: enChannelMonitorV2,
    batchImage: enBatchImage,
    misc: enMisc
  }
}

const admins: Record<string, Modules> = {
  zh: {
    overview: zhAdminOverview,
    channels: zhAdminChannels,
    accounts: zhAdminAccounts,
    resources: zhAdminResources,
    ops: zhAdminOps,
    settings: zhAdminSettings,
    audit: zhAdminAudit,
    promptAudit: zhAdminPromptAudit
  },
  en: {
    overview: enAdminOverview,
    channels: enAdminChannels,
    accounts: enAdminAccounts,
    resources: enAdminResources,
    ops: enAdminOps,
    settings: enAdminSettings,
    audit: enAdminAudit,
    promptAudit: enAdminPromptAudit
  }
}

const LOCALES_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '../locales')

/** Module names on disk for a locale directory, excluding the index barrel. */
function moduleNamesOnDisk(relativeDir: string): string[] {
  return readdirSync(resolve(LOCALES_DIR, relativeDir), { withFileTypes: true })
    .filter((e) => e.isFile() && e.name.endsWith('.ts') && e.name !== 'index.ts')
    .map((e) => e.name.replace(/\.ts$/, ''))
    .sort()
}

describe.each(Object.keys(roots))('locale %s spread assembly', (locale) => {
  it('root modules have no overlapping top-level keys', () => {
    expect(collisions(roots[locale])).toEqual([])
  })

  it('root modules do not shadow the explicit "admin" namespace', () => {
    for (const [name, mod] of Object.entries(roots[locale])) {
      expect(Object.keys(mod), `module ${name} must not define "admin"`).not.toContain('admin')
    }
  })

  it('admin modules have no overlapping top-level keys', () => {
    expect(collisions(admins[locale])).toEqual([])
  })

  /*
   * The two assertions below are the reason this file grew a filesystem read.
   *
   * The module list above is hand-maintained, and it had silently fallen four
   * modules behind (`batchImage`, `channelMonitorV2`, `admin/audit`,
   * `admin/promptAudit`). A namespace missing from the list is invisible to the
   * collision check — which is precisely the case where a collision would go
   * unnoticed. Comparing against the directory makes the omission itself fail.
   */
  it('covers every root module on disk', () => {
    expect(Object.keys(roots[locale]).sort()).toEqual(moduleNamesOnDisk(locale))
  })

  it('covers every admin module on disk', () => {
    expect(Object.keys(admins[locale]).sort()).toEqual(moduleNamesOnDisk(`${locale}/admin`))
  })
})
