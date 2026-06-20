import { readFileSync } from 'node:fs'
import { join } from 'node:path'
import { describe, expect, it } from 'vitest'

const root = process.cwd()

const checkedFiles = [
  'src/style.css',
  'src/components/layout/AppLayout.vue',
  'src/components/layout/AppHeader.vue',
  'src/components/layout/AppSidebar.vue',
  'src/components/layout/AuthLayout.vue',
  'src/components/common/SubscriptionProgressMini.vue',
  'src/views/HomeView.vue',
  'src/views/user/RedeemView.vue',
  'src/views/admin/DashboardView.vue',
  'src/components/user/dashboard/UserDashboardStats.vue',
  'src/components/user/dashboard/UserDashboardCharts.vue',
  'src/components/user/dashboard/UserDashboardRecentUsage.vue',
  'src/components/charts/TokenUsageTrend.vue',
  'src/components/charts/ModelDistributionChart.vue',
  'src/components/charts/EndpointDistributionChart.vue',
  'src/utils/platformColors.ts',
  'src/utils/billingMode.ts',
  'src/i18n/locales/zh.ts',
  'src/i18n/locales/en.ts',
]

const forbiddenPatterns = [
  'bg-mesh-gradient',
  'shadow-glow',
  'shadow-primary',
  'from-primary-500 to-primary-600',
  'rgba(20, 184, 166',
  'rgba(6, 182, 212',
  'bg-emerald-100',
  'text-emerald-600',
  'bg-primary-100',
  'text-primary-600',
  'text-green-600',
  'bg-blue-100',
  'text-blue-600',
  'bg-purple-100',
  'bg-purple-50',
  'text-purple-600',
  'bg-amber-100',
  'text-amber-600',
  '#10b981',
  '#3b82f6',
  '欢迎使用 Sub2API',
  'Welcome to Sub2API',
]

describe('visual theme source guard', () => {
  it('keeps core UI files on the black-white-gray theme', () => {
    const offenders: string[] = []

    for (const file of checkedFiles) {
      const source = readFileSync(join(root, file), 'utf8')
      for (const pattern of forbiddenPatterns) {
        if (source.includes(pattern)) {
          offenders.push(`${file}: ${pattern}`)
        }
      }
    }

    expect(offenders).toEqual([])
  })
})
