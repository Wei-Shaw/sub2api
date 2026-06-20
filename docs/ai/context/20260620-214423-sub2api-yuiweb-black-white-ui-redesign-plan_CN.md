# Sub2API YuiWeb Black White UI Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Sub2API 全站前端改造成 `yui.web` 作品集式黑白灰 UI，并把默认品牌呈现改为「天才程序员小站」。

**Architecture:** 先收拢品牌默认值、灰阶色板和组件基线，再替换布局壳、入口页和业务色彩入口。保留现有路由、数据流、后端接口和页面信息架构，只改 `frontend/` 的呈现层。

**Tech Stack:** Vue 3、TypeScript、Pinia、Vue Router、Tailwind CSS、Vite、Vitest、Chart.js、vue-chartjs。

---

## 设计来源

- 设计稿：`docs/ai/context/20260620-214101-sub2api-yuiweb-black-white-ui-redesign-design_CN.md`
- 目标风格来源：`/Users/wujianxiang/CodeSpace/yui.web/tailwind.config.js`
- 当前前端根目录：`frontend/`

## 文件结构

- Create: `frontend/src/constants/branding.ts`
  统一默认品牌名、默认副标题、默认文档标题，避免多个文件继续硬编码 `Sub2API`。
- Create: `frontend/src/utils/grayTheme.ts`
  统一灰阶 badge、平台、模型、图表、状态和进度条 class，给业务组件复用。
- Create: `frontend/src/constants/__tests__/branding.spec.ts`
  验证默认品牌常量。
- Create: `frontend/src/utils/__tests__/grayTheme.spec.ts`
  验证平台、计费、模型和图表颜色都走灰阶。
- Create: `frontend/src/components/layout/__tests__/AuthLayout.visual.spec.ts`
  用源码扫描保证认证布局不再包含 mesh、orb、青绿色渐变和玻璃卡片关键类。
- Create: `frontend/src/__tests__/visualThemeSource.spec.ts`
  全局源码扫描，防止核心 UI 文件重新出现青绿色主视觉类。
- Modify: `frontend/tailwind.config.js`
  重写 Tailwind 色板、字体、阴影、背景 token。
- Modify: `frontend/index.html`
  更新默认标题和字体预连接/样式链接。
- Modify: `frontend/src/style.css`
  重写全局基础样式和组件类。
- Modify: `frontend/src/stores/app.ts`
  使用默认品牌常量，不破坏 public settings 覆盖。
- Modify: `frontend/src/router/title.ts`
  使用默认品牌常量。
- Modify: `frontend/src/router/__tests__/title.spec.ts`
  更新默认标题期望。
- Modify: `frontend/src/main.ts`
  使用默认文档标题常量。
- Modify: `frontend/src/components/layout/AppLayout.vue`
  去掉 mesh 背景。
- Modify: `frontend/src/components/layout/AppHeader.vue`
  头部、余额、头像、下拉菜单灰阶化。
- Modify: `frontend/src/components/layout/AppSidebar.vue`
  侧栏品牌和选中态灰阶化。
- Modify: `frontend/src/components/layout/AuthLayout.vue`
  认证页改成 yui.web 风格白底细框。
- Modify: `frontend/src/views/HomeView.vue`
  首页改成作品集式黑白灰入口。
- Modify: `frontend/src/views/NotFoundView.vue`
  404 页去掉渐变光效。
- Modify: `frontend/src/views/public/LegalDocumentView.vue`
  法务页默认品牌与按钮灰阶化。
- Modify: `frontend/src/views/setup/SetupWizardView.vue`
  安装向导跟随黑白灰组件基线。
- Modify: `frontend/src/views/auth/EmailVerifyView.vue`
  默认品牌和成功/警告状态灰阶化。
- Modify: `frontend/src/views/auth/ForgotPasswordView.vue`、`frontend/src/views/auth/ResetPasswordView.vue`
  成功/警告状态灰阶化，错误保持红色。
- Modify: `frontend/src/utils/platformColors.ts`、`frontend/src/utils/billingMode.ts`、`frontend/src/composables/useModelWhitelist.ts`
  平台、计费和模型标签灰阶化。
- Modify: `frontend/src/components/common/StatusBadge.vue`、`frontend/src/components/common/StatCard.vue`、`frontend/src/components/common/NavigationProgress.vue`、`frontend/src/components/common/Toggle.vue`
  公共组件灰阶化。
- Modify: `frontend/src/components/payment/SubscriptionPlanCard.vue`、`frontend/src/components/payment/ProviderCard.vue`、`frontend/src/components/payment/PaymentStatusPanel.vue`、`frontend/src/components/payment/ToggleSwitch.vue`
  支付 UI 灰阶化，支付二维码品牌标识保留最低识别度。
- Modify: `frontend/src/components/charts/TokenUsageTrend.vue`、`frontend/src/components/charts/ModelDistributionChart.vue`、`frontend/src/components/charts/EndpointDistributionChart.vue`
  图表调色盘灰阶化。
- Modify: `frontend/src/components/user/dashboard/UserDashboardStats.vue`、`frontend/src/components/user/dashboard/UserDashboardCharts.vue`、`frontend/src/views/admin/DashboardView.vue`
  用户和管理仪表盘统计卡、图标、金额、进度条灰阶化。
- Modify: `AGENTS.md`
  实现完成后追加结果文档路径和验证结论。
- Create: `docs/ai/context/20260620-214423-sub2api-yuiweb-black-white-ui-redesign-result_CN.md`
  实现完成后的结果记录。

---

## Task 1: 默认品牌常量与标题回退

**Files:**
- Create: `frontend/src/constants/branding.ts`
- Create: `frontend/src/constants/__tests__/branding.spec.ts`
- Modify: `frontend/src/stores/app.ts`
- Modify: `frontend/src/router/title.ts`
- Modify: `frontend/src/router/__tests__/title.spec.ts`
- Modify: `frontend/src/main.ts`
- Modify: `frontend/index.html`

- [ ] **Step 1: 写默认品牌测试**

Create `frontend/src/constants/__tests__/branding.spec.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
  DEFAULT_DOCUMENT_TITLE,
  DEFAULT_SITE_NAME,
  DEFAULT_SITE_SUBTITLE,
} from '@/constants/branding'

describe('branding defaults', () => {
  it('uses the Yui-facing default site name', () => {
    expect(DEFAULT_SITE_NAME).toBe('天才程序员小站')
  })

  it('keeps a concise default subtitle for the API console', () => {
    expect(DEFAULT_SITE_SUBTITLE).toBe('AI API Gateway')
  })

  it('builds the default browser title from the default site name', () => {
    expect(DEFAULT_DOCUMENT_TITLE).toBe('天才程序员小站 - AI API Gateway')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd frontend
pnpm vitest run src/constants/__tests__/branding.spec.ts
```

Expected: FAIL，错误包含 `Failed to resolve import "@/constants/branding"`。

- [ ] **Step 3: 创建品牌常量**

Create `frontend/src/constants/branding.ts`:

```ts
export const DEFAULT_SITE_NAME = '天才程序员小站'
export const DEFAULT_SITE_SUBTITLE = 'AI API Gateway'
export const DEFAULT_DOCUMENT_TITLE = `${DEFAULT_SITE_NAME} - ${DEFAULT_SITE_SUBTITLE}`
```

- [ ] **Step 4: 更新 app store 默认值**

Modify `frontend/src/stores/app.ts`:

```ts
import { DEFAULT_SITE_NAME } from '@/constants/branding'
```

Replace the initial site name:

```ts
const siteName = ref<string>(DEFAULT_SITE_NAME)
```

Replace the public settings application fallback:

```ts
siteName.value = config.site_name || DEFAULT_SITE_NAME
```

- [ ] **Step 5: 更新标题解析**

Modify `frontend/src/router/title.ts`:

```ts
import { DEFAULT_SITE_NAME } from '@/constants/branding'
import { i18n } from '@/i18n'
```

Replace the fallback line:

```ts
const normalizedSiteName =
  typeof siteName === 'string' && siteName.trim() ? siteName.trim() : DEFAULT_SITE_NAME
```

- [ ] **Step 6: 更新标题测试**

Modify `frontend/src/router/__tests__/title.spec.ts` default-site test:

```ts
it('站点名为空时，回退默认站点名', () => {
  expect(resolveDocumentTitle('Dashboard', '')).toBe('Dashboard - 天才程序员小站')
  expect(resolveDocumentTitle(undefined, '   ')).toBe('天才程序员小站')
})
```

- [ ] **Step 7: 更新初始化文档标题**

Modify `frontend/src/main.ts`:

```ts
import { DEFAULT_DOCUMENT_TITLE, DEFAULT_SITE_NAME } from '@/constants/branding'
```

Replace:

```ts
if (appStore.siteName && appStore.siteName !== DEFAULT_SITE_NAME) {
  document.title = `${appStore.siteName} - AI API Gateway`
} else {
  document.title = DEFAULT_DOCUMENT_TITLE
}
```

- [ ] **Step 8: 更新 HTML 默认标题**

Modify `frontend/index.html`:

```html
<title>天才程序员小站 - AI API Gateway</title>
<link href="https://fonts.googleapis.com" rel="preconnect">
<link href="https://fonts.gstatic.com" rel="preconnect" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Playfair+Display:ital,wght@0,400..900;1,400..900&family=Inter:wght@300..700&display=swap" rel="stylesheet">
```

- [ ] **Step 9: 运行品牌和标题测试**

Run:

```bash
cd frontend
pnpm vitest run src/constants/__tests__/branding.spec.ts src/router/__tests__/title.spec.ts
```

Expected: PASS。

---

## Task 2: 灰阶主题工具与业务色彩入口

**Files:**
- Create: `frontend/src/utils/grayTheme.ts`
- Create: `frontend/src/utils/__tests__/grayTheme.spec.ts`
- Modify: `frontend/src/utils/platformColors.ts`
- Modify: `frontend/src/utils/billingMode.ts`
- Modify: `frontend/src/composables/useModelWhitelist.ts`

- [ ] **Step 1: 写灰阶主题工具测试**

Create `frontend/src/utils/__tests__/grayTheme.spec.ts`:

```ts
import { describe, expect, it } from 'vitest'
import {
  chartGrayPalette,
  grayBadgeClass,
  grayBorderClass,
  grayButtonClass,
  grayIconClass,
  grayProgressBarClass,
  grayTextClass,
  modelChipClass,
} from '@/utils/grayTheme'

describe('grayTheme', () => {
  it('returns neutral classes for platform-like badges', () => {
    expect(grayBadgeClass()).toBe(
      'border-gray-300 bg-gray-100 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
    )
    expect(grayBorderClass()).toBe('border-gray-200 dark:border-dark-600')
  })

  it('keeps buttons monochrome', () => {
    expect(grayButtonClass()).toBe(
      'bg-gray-900 text-white hover:bg-gray-800 active:bg-black dark:bg-gray-100 dark:text-dark-950 dark:hover:bg-white'
    )
  })

  it('uses neutral text and icon classes', () => {
    expect(grayTextClass()).toBe('text-gray-900 dark:text-gray-100')
    expect(grayIconClass()).toBe('text-gray-600 dark:text-gray-300')
  })

  it('uses the same model chip class for all presets', () => {
    expect(modelChipClass()).toContain('bg-gray-100')
    expect(modelChipClass()).toContain('dark:bg-dark-700')
  })

  it('uses grayscale chart colors', () => {
    expect(chartGrayPalette).toEqual([
      '#111827',
      '#374151',
      '#4b5563',
      '#6b7280',
      '#9ca3af',
      '#d1d5db',
      '#525252',
      '#737373',
    ])
  })

  it('keeps quota progress neutral until danger state', () => {
    expect(grayProgressBarClass(20)).toBe('bg-gray-700 dark:bg-gray-300')
    expect(grayProgressBarClass(91)).toBe('bg-red-500')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd frontend
pnpm vitest run src/utils/__tests__/grayTheme.spec.ts
```

Expected: FAIL，错误包含 `Failed to resolve import "@/utils/grayTheme"`。

- [ ] **Step 3: 创建灰阶主题工具**

Create `frontend/src/utils/grayTheme.ts`:

```ts
export const chartGrayPalette = [
  '#111827',
  '#374151',
  '#4b5563',
  '#6b7280',
  '#9ca3af',
  '#d1d5db',
  '#525252',
  '#737373',
] as const

export function grayBadgeClass(): string {
  return 'border-gray-300 bg-gray-100 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
}

export function grayBadgeLightClass(): string {
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200'
}

export function grayBorderClass(): string {
  return 'border-gray-200 dark:border-dark-600'
}

export function grayAccentBarClass(): string {
  return 'bg-gray-900 dark:bg-gray-100'
}

export function grayTextClass(): string {
  return 'text-gray-900 dark:text-gray-100'
}

export function grayMutedTextClass(): string {
  return 'text-gray-500 dark:text-gray-400'
}

export function grayIconClass(): string {
  return 'text-gray-600 dark:text-gray-300'
}

export function grayButtonClass(): string {
  return 'bg-gray-900 text-white hover:bg-gray-800 active:bg-black dark:bg-gray-100 dark:text-dark-950 dark:hover:bg-white'
}

export function grayDiscountClass(): string {
  return 'bg-gray-200 text-gray-800 dark:bg-dark-600 dark:text-gray-100'
}

export function grayGradientClass(): string {
  return 'from-gray-900 to-gray-700 dark:from-gray-100 dark:to-gray-300'
}

export function grayGradientTextClass(): string {
  return 'text-gray-100 dark:text-dark-950'
}

export function grayGradientSubtextClass(): string {
  return 'text-gray-300 dark:text-gray-700'
}

export function modelChipClass(): string {
  return 'bg-gray-100 text-gray-700 hover:bg-gray-200 dark:bg-dark-700 dark:text-gray-200 dark:hover:bg-dark-600'
}

export function grayProgressBarClass(percent: number): string {
  return percent >= 90 ? 'bg-red-500' : 'bg-gray-700 dark:bg-gray-300'
}
```

- [ ] **Step 4: 灰阶化平台颜色工具**

Modify `frontend/src/utils/platformColors.ts` by replacing class records with neutral functions while keeping public function names stable:

```ts
import {
  grayAccentBarClass,
  grayBadgeClass,
  grayBadgeLightClass,
  grayBorderClass,
  grayButtonClass,
  grayDiscountClass,
  grayGradientClass,
  grayGradientSubtextClass,
  grayGradientTextClass,
  grayIconClass,
  grayTextClass,
} from '@/utils/grayTheme'
```

Replace exported class functions:

```ts
export function platformBadgeClass(_p: string): string {
  return grayBadgeClass()
}

export function platformBadgeLightClass(_p: string): string {
  return grayBadgeLightClass()
}

export function platformBorderClass(_p: string): string {
  return grayBorderClass()
}

export function platformAccentBarClass(_p: string): string {
  return grayAccentBarClass()
}

export function platformTextClass(_p: string): string {
  return grayTextClass()
}

export function platformIconClass(_p: string): string {
  return grayIconClass()
}

export function platformButtonClass(_p: string): string {
  return grayButtonClass()
}

export function platformDiscountClass(_p: string): string {
  return grayDiscountClass()
}

export function platformGradientClass(_p: string): string {
  return grayGradientClass()
}

export function platformGradientTextClass(_p: string): string {
  return grayGradientTextClass()
}

export function platformGradientSubtextClass(_p: string): string {
  return grayGradientSubtextClass()
}
```

Keep `platformLabel` unchanged.

- [ ] **Step 5: 灰阶化计费模式 badge**

Modify `frontend/src/utils/billingMode.ts`:

```ts
import { grayBadgeLightClass } from '@/utils/grayTheme'
```

Replace `getBillingModeBadgeClass` body:

```ts
export function getBillingModeBadgeClass(_mode: string | null | undefined): string {
  return grayBadgeLightClass()
}
```

- [ ] **Step 6: 灰阶化模型白名单 preset 颜色**

Modify `frontend/src/composables/useModelWhitelist.ts`:

```ts
import { modelChipClass } from '@/utils/grayTheme'

const MODEL_PRESET_CHIP_CLASS = modelChipClass()
```

Replace every preset object field shaped as:

```ts
color: 'bg-blue-100 text-blue-700 hover:bg-blue-200 dark:bg-blue-900/30 dark:text-blue-400'
```

with:

```ts
color: MODEL_PRESET_CHIP_CLASS
```

Use a mechanical replacement over the file for all `color: 'bg-... dark:text-...'` preset values. Do not change labels, `from`, or `to`.

- [ ] **Step 7: 运行工具测试**

Run:

```bash
cd frontend
pnpm vitest run src/utils/__tests__/grayTheme.spec.ts
```

Expected: PASS。

---

## Task 3: Tailwind token 与全局组件基线

**Files:**
- Modify: `frontend/tailwind.config.js`
- Modify: `frontend/src/style.css`
- Modify: `frontend/src/components/common/NavigationProgress.vue`
- Modify: `frontend/src/components/common/Toggle.vue`
- Modify: `frontend/src/components/payment/ToggleSwitch.vue`
- Modify: `frontend/src/components/common/__tests__/NavigationProgress.spec.ts`

- [ ] **Step 1: 替换 Tailwind 主题 token**

Modify `frontend/tailwind.config.js` `extend.colors`:

```js
colors: {
  primary: {
    50: '#fafafa',
    100: '#f5f5f5',
    200: '#e5e5e5',
    300: '#d4d4d4',
    400: '#a3a3a3',
    500: '#737373',
    600: '#525252',
    700: '#404040',
    800: '#262626',
    900: '#171717',
    950: '#0a0a0a'
  },
  accent: {
    50: '#faf9f6',
    100: '#f1eee8',
    200: '#e0d8d0',
    300: '#c9a227',
    400: '#8a8a8a',
    500: '#666666',
    600: '#525252',
    700: '#3f3f3f',
    800: '#292929',
    900: '#1f1f1f',
    950: '#111111'
  },
  dark: {
    50: '#f5f5f5',
    100: '#e5e5e5',
    200: '#d4d4d4',
    300: '#a3a3a3',
    400: '#737373',
    500: '#525252',
    600: '#404040',
    700: '#333333',
    800: '#242424',
    900: '#1a1a1a',
    950: '#0f0f0f'
  }
}
```

Modify `extend.fontFamily`:

```js
fontFamily: {
  display: ['Playfair Display', 'Georgia', 'serif'],
  serif: ['Playfair Display', 'Georgia', 'serif'],
  sans: [
    'Inter',
    'system-ui',
    '-apple-system',
    'BlinkMacSystemFont',
    'Segoe UI',
    'Roboto',
    'Helvetica Neue',
    'Arial',
    'PingFang SC',
    'Hiragino Sans GB',
    'Microsoft YaHei',
    'sans-serif'
  ],
  body: [
    'Inter',
    'system-ui',
    '-apple-system',
    'BlinkMacSystemFont',
    'Segoe UI',
    'Roboto',
    'Helvetica Neue',
    'Arial',
    'PingFang SC',
    'Hiragino Sans GB',
    'Microsoft YaHei',
    'sans-serif'
  ],
  mono: ['ui-monospace', 'SFMono-Regular', 'Menlo', 'Monaco', 'Consolas', 'monospace']
}
```

Modify shadows and background images:

```js
boxShadow: {
  soft: '0 4px 20px rgba(0, 0, 0, 0.03)',
  'soft-dark': '0 4px 20px rgba(0, 0, 0, 0.3)',
  card: '0 1px 2px rgba(0, 0, 0, 0.04)',
  'card-hover': '0 8px 24px rgba(0, 0, 0, 0.08)',
  glass: '0 4px 20px rgba(0, 0, 0, 0.04)',
  'glass-sm': '0 2px 12px rgba(0, 0, 0, 0.04)'
},
backgroundImage: {
  'gradient-primary': 'linear-gradient(135deg, #222222 0%, #333333 100%)',
  'gradient-dark': 'linear-gradient(135deg, #1a1a1a 0%, #0f0f0f 100%)',
  'mesh-gradient': 'linear-gradient(180deg, rgba(250,250,250,0.9) 0%, rgba(255,255,255,1) 100%)'
}
```

Remove the `glow` animation entry and the `glow` keyframes block.

- [ ] **Step 2: 重写全局组件核心类**

Modify `frontend/src/style.css`:

```css
@layer base {
  * {
    @apply border-gray-200 dark:border-dark-700;
  }

  html {
    @apply scroll-smooth antialiased;
  }

  body {
    @apply min-h-screen bg-[#fafafa] font-sans text-gray-900 dark:bg-dark-950 dark:text-gray-100;
  }

  ::selection {
    @apply bg-accent-200 text-gray-950 dark:bg-dark-700 dark:text-gray-100;
  }
}
```

Replace key component classes:

```css
.btn {
  @apply inline-flex items-center justify-center gap-2 rounded-md px-4 py-2.5 text-sm font-medium;
  @apply transition-colors duration-200 ease-out;
  @apply focus:outline-none focus:ring-2 focus:ring-gray-900/20 focus:ring-offset-2 dark:focus:ring-gray-100/30 dark:focus:ring-offset-dark-950;
  @apply disabled:cursor-not-allowed disabled:opacity-50;
}

.btn-primary {
  @apply bg-gray-900 text-white shadow-sm hover:bg-gray-800 active:bg-black;
  @apply dark:bg-gray-100 dark:text-dark-950 dark:hover:bg-white;
}

.btn-secondary {
  @apply border border-gray-300 bg-white text-gray-800 shadow-sm hover:bg-gray-50 hover:border-gray-400;
  @apply dark:border-dark-600 dark:bg-dark-800 dark:text-gray-100 dark:hover:bg-dark-700;
}

.btn-ghost {
  @apply bg-transparent text-gray-600 hover:bg-gray-100 hover:text-gray-950;
  @apply dark:text-gray-300 dark:hover:bg-dark-800 dark:hover:text-white;
}

.input {
  @apply w-full rounded-md border border-gray-300 bg-white px-4 py-2.5 text-sm text-gray-900;
  @apply placeholder:text-gray-400 transition-colors duration-200;
  @apply focus:border-gray-900 focus:outline-none focus:ring-2 focus:ring-gray-900/10;
  @apply disabled:cursor-not-allowed disabled:bg-gray-100;
  @apply dark:border-dark-600 dark:bg-dark-900 dark:text-gray-100 dark:placeholder:text-dark-400 dark:focus:border-gray-100 dark:focus:ring-gray-100/15;
}

.card {
  @apply rounded-lg border border-gray-200 bg-white shadow-card transition-colors duration-200;
  @apply dark:border-dark-700 dark:bg-dark-900;
}

.card-glass {
  @apply rounded-lg border border-gray-200 bg-white shadow-soft;
  @apply dark:border-dark-700 dark:bg-dark-900 dark:shadow-soft-dark;
}

.badge {
  @apply inline-flex items-center gap-1 rounded-md border border-gray-300 bg-gray-100 px-2.5 py-0.5 text-xs font-medium text-gray-700;
  @apply dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200;
}

.sidebar {
  @apply fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-gray-200 bg-white transition-transform duration-300 dark:border-dark-800 dark:bg-dark-950;
  transition-property: width, transform;
}

.sidebar-link {
  @apply flex items-center gap-3 overflow-hidden rounded-md py-2.5 text-sm font-medium text-gray-600 transition-colors duration-200;
  @apply hover:bg-gray-100 hover:text-gray-950 dark:text-gray-300 dark:hover:bg-dark-800 dark:hover:text-white;
  padding-left: 1.0625rem;
  padding-right: 0.875rem;
}

.sidebar-link-active {
  @apply border border-gray-300 bg-gray-100 text-gray-950 hover:bg-gray-100 dark:border-dark-600 dark:bg-dark-800 dark:text-white dark:hover:bg-dark-800;
}

.progress-bar {
  @apply h-full bg-gray-900 transition-all duration-300 dark:bg-gray-100;
}

.switch-active {
  @apply bg-gray-900 dark:bg-gray-100;
}
```

Keep `.btn-danger` red. Change `.btn-success` and `.btn-warning` to gray:

```css
.btn-success,
.btn-warning {
  @apply bg-gray-900 text-white shadow-sm hover:bg-gray-800 dark:bg-gray-100 dark:text-dark-950 dark:hover:bg-white;
}
```

- [ ] **Step 3: 更新顶部加载条**

Modify `frontend/src/components/common/NavigationProgress.vue` scoped CSS:

```css
.navigation-progress-bar {
  height: 100%;
  width: 100%;
  background: linear-gradient(
    90deg,
    transparent 0%,
    theme('colors.gray.400') 20%,
    theme('colors.gray.900') 50%,
    theme('colors.gray.400') 80%,
    transparent 100%
  );
  animation: progress-slide 1.5s ease-in-out infinite;
}

:root.dark .navigation-progress-bar {
  background: linear-gradient(
    90deg,
    transparent 0%,
    theme('colors.dark.600') 20%,
    theme('colors.gray.100') 50%,
    theme('colors.dark.600') 80%,
    transparent 100%
  );
}
```

- [ ] **Step 4: 更新开关组件**

Modify `frontend/src/components/common/Toggle.vue` active classes:

```vue
:class="[modelValue ? 'bg-gray-900 dark:bg-gray-100' : 'bg-gray-200 dark:bg-dark-600']"
```

Modify focus ring:

```vue
class="relative inline-flex h-6 w-11 flex-shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none focus:ring-2 focus:ring-gray-900/20 focus:ring-offset-2 dark:focus:ring-gray-100/30 dark:focus:ring-offset-dark-800"
```

Modify `frontend/src/components/payment/ToggleSwitch.vue`:

```vue
checked ? 'bg-gray-900 dark:bg-gray-100' : 'bg-gray-300 dark:bg-dark-600'
```

- [ ] **Step 5: 运行公共组件测试**

Run:

```bash
cd frontend
pnpm vitest run src/components/common/__tests__/NavigationProgress.spec.ts
```

Expected: PASS。

---

## Task 4: 布局壳、认证页与默认入口

**Files:**
- Create: `frontend/src/components/layout/__tests__/AuthLayout.visual.spec.ts`
- Modify: `frontend/src/components/layout/AppLayout.vue`
- Modify: `frontend/src/components/layout/AppHeader.vue`
- Modify: `frontend/src/components/layout/AppSidebar.vue`
- Modify: `frontend/src/components/layout/AuthLayout.vue`
- Modify: `frontend/src/components/layout/__tests__/AppSidebar.spec.ts`
- Modify: `frontend/src/views/HomeView.vue`
- Modify: `frontend/src/views/NotFoundView.vue`
- Modify: `frontend/src/views/public/LegalDocumentView.vue`

- [ ] **Step 1: 写认证布局源码扫描测试**

Create `frontend/src/components/layout/__tests__/AuthLayout.visual.spec.ts`:

```ts
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AuthLayout.vue')
const source = readFileSync(componentPath, 'utf8')

describe('AuthLayout visual baseline', () => {
  it('does not use mesh backgrounds, decorative orbs, glow shadows, or glass cards', () => {
    expect(source).not.toContain('bg-gradient-to-br from-gray-50 via-primary-50/30')
    expect(source).not.toContain('blur-3xl')
    expect(source).not.toContain('shadow-primary')
    expect(source).not.toContain('card-glass')
    expect(source).not.toContain('text-gradient')
  })

  it('uses the display font for the brand title', () => {
    expect(source).toContain('font-display')
  })
})
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
cd frontend
pnpm vitest run src/components/layout/__tests__/AuthLayout.visual.spec.ts
```

Expected: FAIL，源码仍包含 `blur-3xl`、`card-glass` 或 `text-gradient`。

- [ ] **Step 3: 修改 AppLayout 背景**

Modify `frontend/src/components/layout/AppLayout.vue` template:

```vue
<div class="min-h-screen bg-[#fafafa] text-gray-900 dark:bg-dark-950 dark:text-gray-100">
  <AppSidebar />
  <div
    class="relative min-h-screen transition-all duration-300"
    :class="[sidebarCollapsed ? 'lg:ml-[72px]' : 'lg:ml-64']"
  >
    <AppHeader />
    <main class="p-4 md:p-6 lg:p-8">
      <slot />
    </main>
  </div>
</div>
```

Remove the fixed `bg-mesh-gradient` decoration block.

- [ ] **Step 4: 修改 AppSidebar 品牌与选中态**

Modify `frontend/src/components/layout/AppSidebar.vue` brand title class:

```vue
<span class="sidebar-brand-title font-display text-lg font-semibold italic tracking-normal text-gray-950 dark:text-gray-100">
  {{ siteName }}
</span>
```

Modify logo wrapper class:

```vue
<div class="sidebar-logo flex h-9 w-9 items-center justify-center overflow-hidden rounded-md border border-gray-200 bg-white shadow-soft dark:border-dark-700 dark:bg-dark-900">
```

Modify mobile overlay:

```vue
class="fixed inset-0 z-30 bg-black/40 lg:hidden"
```

Keep the `.sidebar-svg-icon` scoped style unchanged so uploaded SVG colors are not overridden.

- [ ] **Step 5: 修改 AppHeader 控件灰阶**

Modify `frontend/src/components/layout/AppHeader.vue` header class:

```vue
<header class="sticky top-0 z-30 border-b border-gray-200 bg-white/95 backdrop-blur-sm dark:border-dark-800 dark:bg-dark-950/95">
```

Modify balance pill:

```vue
class="hidden items-center gap-2 rounded-md border border-gray-200 bg-gray-50 px-3 py-1.5 dark:border-dark-700 dark:bg-dark-900 sm:flex"
```

Modify balance icon and text:

```vue
class="h-4 w-4 text-gray-600 dark:text-gray-300"
```

```vue
class="text-sm font-semibold text-gray-900 dark:text-gray-100"
```

Modify avatar fallback:

```vue
class="flex h-8 w-8 items-center justify-center overflow-hidden rounded-md bg-gray-900 text-sm font-medium text-white shadow-sm dark:bg-gray-100 dark:text-dark-950"
```

- [ ] **Step 6: 修改 AuthLayout**

Modify `frontend/src/components/layout/AuthLayout.vue` template wrapper:

```vue
<div class="flex min-h-screen items-center justify-center bg-white px-4 py-10 text-gray-900 dark:bg-dark-950 dark:text-gray-100">
  <div class="w-full max-w-md">
    <div class="mb-8 text-center">
      <template v-if="settingsLoaded">
        <div class="mb-5 inline-flex h-14 w-14 items-center justify-center overflow-hidden rounded-md border border-gray-200 bg-white shadow-soft dark:border-dark-700 dark:bg-dark-900 dark:shadow-soft-dark">
          <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
        </div>
        <h1 class="mb-2 font-display text-4xl font-semibold italic text-gray-950 dark:text-gray-100">
          {{ siteName }}
        </h1>
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ siteSubtitle }}
        </p>
      </template>
    </div>

    <div class="rounded-lg border border-gray-200 bg-white p-8 shadow-soft dark:border-dark-700 dark:bg-dark-900 dark:shadow-soft-dark">
      <slot />
    </div>

    <div class="mt-6 text-center text-sm">
      <slot name="footer" />
    </div>

    <div class="mt-8 text-center text-xs text-gray-400 dark:text-gray-500">
      &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
    </div>
  </div>
</div>
```

Modify script imports:

```ts
import { DEFAULT_SITE_NAME, DEFAULT_SITE_SUBTITLE } from '@/constants/branding'
```

Modify computed defaults:

```ts
const siteName = computed(() => appStore.siteName || DEFAULT_SITE_NAME)
const siteSubtitle = computed(
  () => appStore.cachedPublicSettings?.site_subtitle || DEFAULT_SITE_SUBTITLE
)
```

Remove the scoped `.text-gradient` style block.

- [ ] **Step 7: 修改 HomeView 入口视觉**

Modify `frontend/src/views/HomeView.vue` default page wrapper:

```vue
class="relative flex min-h-screen flex-col overflow-hidden bg-white text-gray-900 dark:bg-dark-950 dark:text-gray-100"
```

Remove all decorative `blur-3xl` and grid pattern blocks.

Use display font for hero title:

```vue
class="mb-4 font-display text-5xl font-semibold italic leading-tight text-gray-950 dark:text-gray-100 md:text-6xl lg:text-7xl"
```

Change CTA button shadow to neutral:

```vue
class="btn btn-primary px-8 py-3 text-base"
```

Change feature cards:

```vue
class="group rounded-lg border border-gray-200 bg-white p-6 transition-colors hover:border-gray-400 dark:border-dark-700 dark:bg-dark-900 dark:hover:border-dark-500"
```

Change feature icon wrappers:

```vue
class="mb-4 flex h-12 w-12 items-center justify-center rounded-md border border-gray-200 bg-gray-100 text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
```

Modify HomeView defaults:

```ts
import { DEFAULT_SITE_NAME, DEFAULT_SITE_SUBTITLE } from '@/constants/branding'

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || DEFAULT_SITE_NAME)
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || DEFAULT_SITE_SUBTITLE)
```

- [ ] **Step 8: 修改 NotFound 和法务页**

Modify `frontend/src/views/NotFoundView.vue`:

```vue
<div class="flex min-h-screen items-center justify-center bg-white px-6 text-gray-900 dark:bg-dark-950 dark:text-gray-100">
```

Replace icon wrapper:

```vue
class="flex h-24 w-24 items-center justify-center rounded-md border border-gray-200 bg-gray-100 text-gray-700 dark:border-dark-700 dark:bg-dark-900 dark:text-gray-200"
```

Modify `frontend/src/views/public/LegalDocumentView.vue` default:

```ts
import { DEFAULT_SITE_NAME } from '@/constants/branding'
const siteName = computed(() => settings.value?.site_name || DEFAULT_SITE_NAME)
```

Change primary button class to `btn btn-primary`.

- [ ] **Step 9: 运行布局测试**

Run:

```bash
cd frontend
pnpm vitest run src/components/layout/__tests__/AuthLayout.visual.spec.ts src/components/layout/__tests__/AppSidebar.spec.ts
```

Expected: PASS。

---

## Task 5: 认证流程与安装向导状态灰阶化

**Files:**
- Modify: `frontend/src/views/auth/ForgotPasswordView.vue`
- Modify: `frontend/src/views/auth/ResetPasswordView.vue`
- Modify: `frontend/src/views/auth/EmailVerifyView.vue`
- Modify: `frontend/src/views/auth/__tests__/EmailVerifyView.spec.ts`
- Modify: `frontend/src/views/setup/SetupWizardView.vue`

- [ ] **Step 1: 更新 EmailVerify 默认品牌测试数据**

Modify `frontend/src/views/auth/__tests__/EmailVerifyView.spec.ts` mock defaults:

```ts
if (key === 'auth.accountCreatedSuccess') {
  return `Account created for ${params?.siteName ?? '天才程序员小站'}`
}
```

Replace mocked settings `site_name: 'Sub2API'` with:

```ts
site_name: '天才程序员小站'
```

- [ ] **Step 2: 修改 EmailVerify 默认品牌和状态块**

Modify `frontend/src/views/auth/EmailVerifyView.vue`:

```ts
import { DEFAULT_SITE_NAME } from '@/constants/branding'
```

Replace:

```ts
const siteName = ref<string>(DEFAULT_SITE_NAME)
```

Replace settings fallback:

```ts
siteName.value = settings.site_name || DEFAULT_SITE_NAME
```

Change warning block class:

```vue
class="rounded-lg border border-gray-300 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900"
```

Change warning icon:

```vue
class="text-gray-500 dark:text-gray-300"
```

Change success block class:

```vue
class="rounded-lg border border-gray-300 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900"
```

Change success icon:

```vue
class="text-gray-600 dark:text-gray-300"
```

Keep `input-error` red behavior unchanged.

- [ ] **Step 3: 修改 ForgotPassword 成功状态**

Modify `frontend/src/views/auth/ForgotPasswordView.vue` success block:

```vue
<div class="rounded-lg border border-gray-300 bg-gray-50 p-6 dark:border-dark-700 dark:bg-dark-900">
```

Change icon circle:

```vue
class="flex h-12 w-12 items-center justify-center rounded-full bg-gray-200 dark:bg-dark-700"
```

Change icon and text:

```vue
class="text-gray-700 dark:text-gray-200"
```

Change footer link class:

```vue
class="font-medium text-gray-900 underline underline-offset-4 transition-colors hover:text-gray-600 dark:text-gray-100 dark:hover:text-gray-300"
```

- [ ] **Step 4: 修改 ResetPassword 状态块**

Modify `frontend/src/views/auth/ResetPasswordView.vue` with the same neutral state classes:

```vue
class="rounded-lg border border-gray-300 bg-gray-50 p-6 dark:border-dark-700 dark:bg-dark-900"
```

Use gray icon/text classes for success and warning states. Keep password validation errors red.

- [ ] **Step 5: 修改安装向导**

Modify `frontend/src/views/setup/SetupWizardView.vue`:

```vue
class="mb-4 inline-flex h-16 w-16 items-center justify-center rounded-md border border-gray-200 bg-gray-900 text-white shadow-soft dark:border-dark-700 dark:bg-gray-100 dark:text-dark-950"
```

Replace active step classes:

```vue
? 'bg-gray-900 text-white dark:bg-gray-100 dark:text-dark-950'
```

Replace success message block:

```vue
class="mt-6 rounded-lg border border-gray-300 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900"
```

Use gray check icons except destructive or error states, which remain red.

- [ ] **Step 6: 运行认证相关测试**

Run:

```bash
cd frontend
pnpm vitest run src/views/auth/__tests__/EmailVerifyView.spec.ts
```

Expected: PASS。

---

## Task 6: 用户端和管理端业务组件灰阶化

**Files:**
- Modify: `frontend/src/components/common/StatusBadge.vue`
- Modify: `frontend/src/components/common/StatCard.vue`
- Modify: `frontend/src/components/account/UsageProgressBar.vue`
- Modify: `frontend/src/components/account/QuotaLimitCard.vue`
- Modify: `frontend/src/components/user/dashboard/UserDashboardStats.vue`
- Modify: `frontend/src/components/user/dashboard/UserDashboardCharts.vue`
- Modify: `frontend/src/views/admin/DashboardView.vue`
- Modify: `frontend/src/views/user/RedeemView.vue`
- Modify: `frontend/src/views/user/PaymentView.vue`

- [ ] **Step 1: 修改 StatusBadge**

Modify `frontend/src/components/common/StatusBadge.vue`:

```ts
const variantClass = computed(() => {
  switch (props.status) {
    case 'error':
    case 'danger':
      return 'bg-red-500'
    case 'active':
    case 'success':
      return 'bg-gray-900 dark:bg-gray-100'
    case 'disabled':
    case 'inactive':
    case 'warning':
      return 'bg-gray-400 dark:bg-gray-500'
    default:
      return 'bg-gray-300 dark:bg-gray-600'
  }
})
```

- [ ] **Step 2: 修改 StatCard 语义 icon 类**

Modify `frontend/src/style.css` stat classes:

```css
.stat-icon-primary,
.stat-icon-success,
.stat-icon-warning {
  @apply bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-200;
}

.stat-icon-danger {
  @apply bg-red-100 text-red-600 dark:bg-red-900/30 dark:text-red-400;
}

.stat-trend-up {
  @apply text-gray-900 dark:text-gray-100;
}

.stat-trend-down {
  @apply text-red-600 dark:text-red-400;
}
```

Keep `frontend/src/components/common/StatCard.vue` API unchanged.

- [ ] **Step 3: 修改账户进度条**

Modify `frontend/src/components/account/UsageProgressBar.vue`:

```ts
import { grayBadgeLightClass, grayProgressBarClass } from '@/utils/grayTheme'
```

Replace colored badge map return with:

```ts
return grayBadgeLightClass()
```

Replace progress bar class function:

```ts
return grayProgressBarClass(percent)
```

- [ ] **Step 4: 修改用户仪表盘统计卡**

Modify `frontend/src/components/user/dashboard/UserDashboardStats.vue`:

Use gray icon wrappers:

```vue
<div class="rounded-md border border-gray-200 bg-gray-100 p-2 text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200">
```

Use gray amount emphasis:

```vue
<span class="text-gray-900 dark:text-gray-100" :title="t('dashboard.actual')">
```

Replace quota disabled progress bar red remains:

```vue
<div class="h-full w-full rounded-full bg-red-500" />
```

Replace normal quota progress:

```vue
:class="quotaBarClass(...)"
```

with a `quotaBarClass` implementation:

```ts
import { grayProgressBarClass } from '@/utils/grayTheme'

function quotaBarClass(percent: number): string {
  return grayProgressBarClass(percent)
}
```

- [ ] **Step 5: 修改用户仪表盘图表表格金额色**

Modify `frontend/src/components/user/dashboard/UserDashboardCharts.vue`:

```vue
<td class="py-1.5 text-right text-gray-900 dark:text-gray-100">${{ formatCost(model.actual_cost) }}</td>
```

Use gray chart palette from `grayTheme`:

```ts
import { chartGrayPalette } from '@/utils/grayTheme'
```

```ts
backgroundColor: chartGrayPalette.slice()
```

- [ ] **Step 6: 修改管理仪表盘统计卡**

Modify `frontend/src/views/admin/DashboardView.vue`:

Replace all statistic icon wrappers such as `bg-blue-100`, `bg-purple-100`, `bg-green-100`, `bg-emerald-100`, `bg-amber-100`, `bg-indigo-100`, `bg-violet-100`, `bg-rose-100` with:

```vue
class="rounded-md border border-gray-200 bg-gray-100 p-2 text-gray-700 dark:border-dark-700 dark:bg-dark-800 dark:text-gray-200"
```

Replace nested icon color classes with:

```vue
class="text-current"
```

Replace positive cost text except errors with:

```vue
class="text-gray-900 dark:text-gray-100"
```

Keep error text red:

```vue
class="text-red-500 dark:text-red-400"
```

- [ ] **Step 7: 修改兑换页和支付页主视觉**

Modify `frontend/src/views/user/RedeemView.vue` header:

```vue
<div class="bg-gray-900 px-6 py-8 text-center text-white dark:bg-gray-100 dark:text-dark-950">
```

Replace success block with neutral gray classes from Task 5.

Modify `frontend/src/views/user/PaymentView.vue`:

```vue
<p class="mt-0.5 text-sm font-medium text-gray-700 dark:text-gray-300">
```

Replace selected amount text:

```vue
class="text-lg font-bold text-gray-900 dark:text-gray-100"
```

Keep `amountError` warning visible but neutral:

```vue
class="mt-2 text-xs text-gray-600 dark:text-gray-300"
```

- [ ] **Step 8: 运行业务组件相关测试**

Run:

```bash
cd frontend
pnpm vitest run src/components/account/__tests__/UsageProgressBar.spec.ts src/views/admin/__tests__/DashboardView.spec.ts src/views/user/__tests__/PaymentView.spec.ts
```

Expected: PASS。

---

## Task 7: 图表与支付组件灰阶化

**Files:**
- Modify: `frontend/src/components/charts/TokenUsageTrend.vue`
- Modify: `frontend/src/components/charts/ModelDistributionChart.vue`
- Modify: `frontend/src/components/charts/EndpointDistributionChart.vue`
- Modify: `frontend/src/components/payment/SubscriptionPlanCard.vue`
- Modify: `frontend/src/components/payment/ProviderCard.vue`
- Modify: `frontend/src/components/payment/PaymentStatusPanel.vue`
- Modify: `frontend/src/components/payment/__tests__/SubscriptionPlanCard.spec.ts`
- Modify: `frontend/src/components/payment/__tests__/PaymentProviderDialog.spec.ts`

- [ ] **Step 1: 修改 TokenUsageTrend 图表色**

Modify `frontend/src/components/charts/TokenUsageTrend.vue` imports:

```ts
import { chartGrayPalette } from '@/utils/grayTheme'
```

Replace chart color values:

```ts
const chartColors = computed(() => ({
  text: isDarkMode.value ? '#e5e5e5' : '#222222',
  grid: isDarkMode.value ? '#333333' : '#e5e5e5',
  input: chartGrayPalette[0],
  output: chartGrayPalette[1],
  cacheCreation: chartGrayPalette[2],
  cacheRead: chartGrayPalette[3],
  cacheHitRate: chartGrayPalette[4]
}))
```

- [ ] **Step 2: 修改 ModelDistributionChart 图表色和金额色**

Modify `frontend/src/components/charts/ModelDistributionChart.vue` imports:

```ts
import { chartGrayPalette } from '@/utils/grayTheme'
```

Replace chart color array:

```ts
const chartColors = chartGrayPalette
```

Replace clickable model text:

```vue
class="max-w-[100px] truncate py-1.5 font-medium text-gray-900 hover:text-gray-600 dark:text-gray-100 dark:hover:text-gray-300"
```

Replace actual/account cost colors:

```vue
class="py-1.5 text-right text-gray-900 dark:text-gray-100"
```

- [ ] **Step 3: 修改 EndpointDistributionChart 图表色和链接色**

Modify `frontend/src/components/charts/EndpointDistributionChart.vue`:

```ts
import { chartGrayPalette } from '@/utils/grayTheme'

const chartColors = chartGrayPalette
```

Replace endpoint link cell:

```vue
class="max-w-[180px] truncate py-1.5 font-medium text-gray-900 hover:text-gray-600 dark:text-gray-100 dark:hover:text-gray-300"
```

Replace actual cost cell:

```vue
class="py-1.5 text-right text-gray-900 dark:text-gray-100"
```

- [ ] **Step 4: 修改 SubscriptionPlanCard**

Modify `frontend/src/components/payment/SubscriptionPlanCard.vue`:

```vue
<div :class="['h-1.5', accentClass]" />
```

The imported `platformAccentBarClass`, `platformBadgeLightClass`, `platformBorderClass`, `platformTextClass`, `platformIconClass`, `platformButtonClass`, and `platformDiscountClass` now return gray classes through Task 2. No prop API changes.

Change root classes:

```vue
'group relative flex flex-col overflow-hidden rounded-lg border transition-colors'
```

Remove:

```vue
'hover:shadow-xl hover:-translate-y-0.5'
```

Use:

```vue
'hover:border-gray-400 dark:hover:border-dark-500'
```

- [ ] **Step 5: 修改 ProviderCard**

Modify `frontend/src/components/payment/ProviderCard.vue`:

```vue
provider.enabled && enabled
  ? 'border-gray-300 bg-gray-100 text-gray-700 dark:border-dark-600 dark:bg-dark-700 dark:text-gray-200'
  : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-500'
```

Change selected payment type:

```vue
isSelected(pt.value)
  ? 'bg-gray-900 text-white dark:bg-gray-100 dark:text-dark-950'
  : 'bg-gray-100 text-gray-400 dark:bg-dark-700 dark:text-gray-500'
```

Change edit hover:

```vue
class="flex flex-col items-center gap-0.5 rounded-md p-1.5 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-900 dark:hover:bg-dark-700 dark:hover:text-white"
```

Keep delete hover red.

- [ ] **Step 6: 修改 PaymentStatusPanel**

Modify `frontend/src/components/payment/PaymentStatusPanel.vue` success outcome icon:

```vue
class="flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
```

```vue
<Icon name="check" size="lg" class="text-gray-700 dark:text-gray-200" />
```

Modify expired outcome icon:

```vue
class="flex h-16 w-16 items-center justify-center rounded-full bg-gray-100 dark:bg-dark-700"
```

```vue
class="h-8 w-8 text-gray-500 dark:text-gray-300"
```

Keep QR brand border and logo background for Alipay/WeChat:

```ts
if (isAlipay.value) return 'border-[#00AEEF] bg-white dark:border-[#00AEEF]/70 dark:bg-dark-900'
if (isWxpay.value) return 'border-[#2BB741] bg-white dark:border-[#2BB741]/70 dark:bg-dark-900'
```

Change spinner:

```vue
class="h-10 w-10 animate-spin rounded-full border-4 border-gray-900 border-t-transparent dark:border-gray-100 dark:border-t-transparent"
```

- [ ] **Step 7: 运行支付组件测试**

Run:

```bash
cd frontend
pnpm vitest run src/components/payment/__tests__/SubscriptionPlanCard.spec.ts src/components/payment/__tests__/PaymentProviderDialog.spec.ts src/views/user/__tests__/PaymentResultView.spec.ts
```

Expected: PASS。

---

## Task 8: 全局源码扫描、构建和结果记录

**Files:**
- Create: `frontend/src/__tests__/visualThemeSource.spec.ts`
- Modify: `AGENTS.md`
- Create: `docs/ai/context/20260620-214423-sub2api-yuiweb-black-white-ui-redesign-result_CN.md`

- [ ] **Step 1: 创建全局视觉源码扫描测试**

Create `frontend/src/__tests__/visualThemeSource.spec.ts`:

```ts
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
  'src/views/HomeView.vue',
  'src/views/user/RedeemView.vue',
  'src/views/admin/DashboardView.vue',
  'src/components/user/dashboard/UserDashboardStats.vue',
  'src/components/user/dashboard/UserDashboardCharts.vue',
  'src/components/charts/TokenUsageTrend.vue',
  'src/components/charts/ModelDistributionChart.vue',
  'src/components/charts/EndpointDistributionChart.vue',
  'src/utils/platformColors.ts',
  'src/utils/billingMode.ts',
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
  'bg-blue-100',
  'text-blue-600',
  'bg-purple-100',
  'text-purple-600',
  'bg-amber-100',
  'text-amber-600',
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
```

- [ ] **Step 2: 运行扫描测试**

Run:

```bash
cd frontend
pnpm vitest run src/__tests__/visualThemeSource.spec.ts
```

Expected: PASS。

- [ ] **Step 3: 运行目标单元测试集合**

Run:

```bash
cd frontend
pnpm vitest run \
  src/constants/__tests__/branding.spec.ts \
  src/router/__tests__/title.spec.ts \
  src/utils/__tests__/grayTheme.spec.ts \
  src/components/layout/__tests__/AuthLayout.visual.spec.ts \
  src/components/layout/__tests__/AppSidebar.spec.ts \
  src/components/common/__tests__/NavigationProgress.spec.ts \
  src/views/auth/__tests__/EmailVerifyView.spec.ts \
  src/components/payment/__tests__/SubscriptionPlanCard.spec.ts \
  src/components/payment/__tests__/PaymentProviderDialog.spec.ts \
  src/views/user/__tests__/PaymentView.spec.ts \
  src/views/user/__tests__/PaymentResultView.spec.ts \
  src/__tests__/visualThemeSource.spec.ts
```

Expected: PASS。

- [ ] **Step 4: 运行构建**

Run:

```bash
cd frontend
pnpm build
```

Expected: PASS，输出包含 `vite build` 完成信息。

- [ ] **Step 5: 浏览器代表页检查**

Start dev server:

```bash
cd frontend
pnpm dev -- --host 127.0.0.1
```

Open and inspect:

- `http://127.0.0.1:5173/`
- `http://127.0.0.1:5173/login`
- `http://127.0.0.1:5173/dashboard`
- `http://127.0.0.1:5173/keys`
- `http://127.0.0.1:5173/purchase`
- `http://127.0.0.1:5173/admin/dashboard`
- `http://127.0.0.1:5173/admin/users`
- `http://127.0.0.1:5173/admin/accounts`
- `http://127.0.0.1:5173/admin/settings`

Expected:

- 默认品牌显示「天才程序员小站」，有后台配置时仍显示后台配置。
- 浅色模式为白/浅灰/黑文字。
- 深色模式为黑/深灰/浅文字。
- 核心页面没有青绿色主视觉、彩色统计卡、彩色大渐变和发光背景。
- 错误状态仍为红色。
- 支付二维码品牌标识仍可区分支付宝和微信。

- [ ] **Step 6: 写结果文档**

Create `docs/ai/context/20260620-214423-sub2api-yuiweb-black-white-ui-redesign-result_CN.md` with this structure:

```md
# Sub2API 前端 yui.web 黑白灰视觉重设计结果

## 修改范围

- 前端默认品牌显示改为「天才程序员小站」。
- Tailwind token、全局组件基线、布局壳、首页、认证页、用户页、管理后台、支付与图表组件已灰阶化。
- 后端、API、路由、计费逻辑未修改。

## 验证结果

- `pnpm vitest run ...`：通过。
- `pnpm build`：通过。
- 浏览器代表页：`/`、`/login`、`/dashboard`、`/keys`、`/purchase`、`/admin/dashboard`、`/admin/users`、`/admin/accounts`、`/admin/settings` 已检查，浅色和深色模式均符合黑白灰视觉方向。

## 已知取舍

- 错误状态保留红色。
- 支付二维码保留支付宝/微信最低品牌识别色。
- 字体使用 Google Fonts，保留系统字体回退。
```

- [ ] **Step 7: 更新 AGENTS.md 运行记录**

Append one bullet under `## 运行记录` in `AGENTS.md`:

```md
- 2026-06-20：已完成 Sub2API 前端 yui.web 黑白灰视觉重设计，默认品牌呈现为「天才程序员小站」，覆盖用户页和管理后台；后端、API、路由、计费逻辑未修改。结果见 `docs/ai/context/20260620-214423-sub2api-yuiweb-black-white-ui-redesign-result_CN.md`。
```

- [ ] **Step 8: 检查差异**

Run:

```bash
git status --short
git diff --stat
git diff --check
```

Expected:

- Only planned frontend files, docs result file, and `AGENTS.md` changed.
- `git diff --check` exits 0.
- No full API key, token, secret, Gmail app password, HMAC secret, or private credential appears in diff.

---

## Self-Review

**Spec coverage:**
The plan covers full frontend scope, user and admin pages, yui.web black-white-gray style, deep frontend-only branding, existing `siteName/siteLogo` priority, dark mode, grayscale statuses, global component baseline, charts, payment, and verification.

**Placeholder scan:**
The plan contains no unresolved implementation markers. Each task names files, exact code snippets, commands, and expected outcomes.

**Type consistency:**
New constants are imported from `@/constants/branding`. New gray theme utilities are imported from `@/utils/grayTheme`. Existing public function names in `platformColors.ts` remain stable, so current components do not need API changes.

**Execution note:**
Use commits only if the user explicitly authorizes committing during execution. If commits are authorized, commit after Task 1, Task 3, Task 5, Task 7, and Task 8 with concise Chinese messages.
