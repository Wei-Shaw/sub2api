<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe v-if="isHomeContentUrl" :src="homeContent.trim()" class="h-screen w-full border-0" allowfullscreen />
    <!-- SECURITY: homeContent is admin-only setting, XSS risk is accepted by design. -->
    <div v-else v-html="homeContent" />
  </div>

  <div v-else class="home-page">

    <!-- ── HEADER ── -->
    <header ref="headerRef" class="home-header" :class="{ 'is-scrolled': headerScrolled }">
      <div class="home-header__inner">
        <router-link to="/" class="home-header__brand">
          <img v-if="brandLogo" :src="brandLogo" :alt="brandName" class="home-header__logo" />
          <span v-else class="home-header__logo-fallback">GW</span>
          <span class="home-header__brand-text">{{ brandName }}</span>
        </router-link>

        <nav class="home-header__menu">
          <a href="#experience" class="home-header__link">{{ isZh ? '接入体验' : 'Experience' }}</a>
          <a href="#models" class="home-header__link">{{ isZh ? '模型支持' : 'Models' }}</a>
          <a href="#pricing" class="home-header__link">{{ isZh ? '定价' : 'Pricing' }}</a>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer" class="home-header__link">
            {{ isZh ? '文档' : 'Docs' }}
          </a>
        </nav>

        <div class="home-header__actions">
          <LocaleSwitcher />
          <button type="button" class="home-header__theme" :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="toggleTheme">
            <Icon v-if="isDark" name="sun" size="sm" :stroke-width="2.2" />
            <Icon v-else name="moon" size="sm" :stroke-width="2.2" />
          </button>
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="home-header__cta">
            {{ isAuthenticated ? (isZh ? '进入控制台' : 'Dashboard') : (isZh ? '登录' : 'Login') }}
          </router-link>
        </div>
      </div>
    </header>

    <main>
      <!-- ── HERO ── -->
      <section class="hero-section">
        <div class="hero-section__bg" aria-hidden="true"></div>
        <div class="hero-section__container">
          <div class="hero-section__content">
            <div class="hero-section__title">
              <span class="hero-section__kicker">{{ isZh ? 'AI Gateway' : 'AI Gateway' }}</span>
              <h1 class="hero-section__main">{{ isZh ? 'AI 编码中转工作台' : 'AI Coding Gateway' }}</h1>
            </div>
            <p class="hero-section__desc">
              {{ isZh
                ? '一个账号，统一接入 Claude、Codex 与 Gemini CLI。稳定线路、透明计费，让每位开发者都能用上顶级 AI 编码能力。'
                : 'One account. Unified access to Claude, Codex and Gemini CLI. Stable routes, transparent billing — top AI coding for every developer.' }}
            </p>
            <div class="hero-section__actions">
              <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="hero-section__btn hero-section__btn--primary">
                {{ isAuthenticated ? (isZh ? '进入控制台' : 'Dashboard') : (isZh ? '立即体验' : 'Get started') }}
              </router-link>
              <a href="#pricing" class="hero-section__btn hero-section__btn--outline">
                {{ isZh ? '查看定价' : 'View pricing' }}
              </a>
            </div>

            <!-- OS compat badges -->
            <div class="hero-os-compat">
              <span class="hero-os-label">{{ isZh ? '支持' : 'Available on' }}</span>
              <div v-for="os in osList" :key="os.name" class="hero-os-item">
                <img :src="os.logo" :alt="os.name" class="hero-os-logo" />
                <span>{{ os.name }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- ── WHY CHOOSE ── -->
      <section class="why-choose">
        <div class="why-choose__container">
          <div class="why-choose__header mirror-reveal">
            <span class="eyebrow-badge">{{ isZh ? '为什么选择我们' : 'Why choose us' }}</span>
            <h2 class="why-choose__section-title">
              {{ isZh ? '企业真正关心的，我们从第一天就做到了' : 'What enterprises need, built in from day one' }}
              <span class="why-choose__star">✦</span>
            </h2>
          </div>
          <div class="why-choose__grid">
            <div
              v-for="(card, i) in whyCards"
              :key="card.id"
              class="why-choose__card mirror-reveal"
              v-mouse-glow
            >
              <p class="why-choose__number">{{ String(i + 1).padStart(2, '0') }}</p>
              <div class="why-choose__content">
                <h3 class="why-choose__title">{{ card.title }}</h3>
                <p class="why-choose__desc">{{ card.desc }}</p>
              </div>
            </div>
            <div class="why-choose__card why-choose__card--logo mirror-reveal">
              <div class="why-choose__logo">
                <img v-if="brandLogo" :src="brandLogo" :alt="brandName" class="why-choose__logo-img" />
                <span>{{ brandName }}</span>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- ── CORE EXPERIENCE ── -->
      <section id="experience" class="transfer-experience">
        <div class="transfer-experience__container">
          <div class="transfer-experience__eyebrow mirror-reveal">
            <span class="eyebrow-badge">{{ isZh ? '核心体验' : 'Core experience' }}</span>
          </div>
          <h2 class="transfer-experience__title mirror-reveal">
            {{ isZh ? '专为 AI 编码设计的' : 'Built for AI coding,' }}
            <span class="transfer-experience__accent">{{ isZh ? '一站式体验' : 'all in one place' }}</span>
            <span class="transfer-experience__star">✦</span>
          </h2>
          <p class="transfer-experience__desc mirror-reveal">
            {{ isZh ? '更低价格、更简单的配置，让每位开发者都能用上顶级 AI 编码模型。' : 'Better pricing, simpler config — top AI coding models accessible to every developer.' }}
          </p>

          <!-- Main card -->
          <div class="transfer-experience__card mirror-reveal" v-mouse-glow>
            <p class="transfer-experience__card-intro">
              {{ isZh
                ? '一个 gw-link 账号，同时使用 Claude Code、Codex 和 Gemini CLI。统一的 API 接入与线路管理，把时间还给你的代码，而不是配置面板。'
                : 'One gw-link account for Claude Code, Codex and Gemini CLI. Unified API and route management — focus on code, not config.' }}
            </p>
            <p class="transfer-experience__card-note">
              {{ isZh
                ? '统一的域名与密钥格式，覆盖常见 IDE 插件与 CLI 工具。一键配置环境。'
                : 'One base URL and key format. Works with common IDE plugins and CLI tools — configure once.' }}
            </p>

            <!-- AI tools grid -->
            <div class="ai-tools-grid">
              <div v-for="tool in aiTools" :key="tool.name" :class="['ai-tool-card', tool.coming && 'ai-tool-card--coming']">
                <div class="ai-tool-card__logo">
                  <img :src="tool.logo" :alt="tool.name" />
                </div>
                <div class="ai-tool-card__info">
                  <p class="ai-tool-card__name">{{ tool.name }}</p>
                  <p v-if="tool.coming" class="ai-tool-card__badge">{{ isZh ? '即将推出' : 'Coming soon' }}</p>
                </div>
              </div>
            </div>
          </div>

        </div>
      </section>


      <!-- ── IDE INTEGRATION ── -->
      <section class="ide-section">
        <div class="ide-section__container">
          <div class="ide-section__grid">
            <div class="ide-section__visual mirror-reveal">
              <div class="ide-section__glow" aria-hidden="true"></div>
              <div class="claude-screenshot-wrap">
                <img src="/package.png" alt="Claude Code" class="claude-screenshot" />
              </div>
            </div>
            <div class="ide-section__content mirror-reveal">
              <h2 class="ide-section__title">
                {{ isZh ? '与你的开发环境协同工作' : 'Works with your dev environment' }}
                <span class="ide-section__star">✦</span>
              </h2>
              <p class="ide-section__desc">
                {{ isZh
                  ? '统一的域名与密钥格式，覆盖常见 IDE 插件与 CLI 工具。只需替换 base_url，保留现有 SDK，30 秒完成接入。'
                  : 'One base URL and key format. Works with common IDE plugins and CLI tools. Swap base_url, keep your SDK — live in 30 seconds.' }}
              </p>
              <div class="ide-section__platforms">
                <div v-for="p in platforms" :key="p.name" class="platform-item">
                  <img :src="p.logo" :alt="p.name" class="platform-logo-img" />
                  <span>{{ p.name }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      <!-- ── PRICING TABLE ── -->
      <section id="pricing" class="pricing-section">
        <div class="pricing-section__container">
          <h2 class="pricing-section__title mirror-reveal">
            {{ isZh ? '模型定价' : 'Model pricing' }}
            <span class="pricing-section__star">✦</span>
          </h2>
          <p class="pricing-section__subtitle mirror-reveal">
            {{ isZh
              ? '我们的价格以人民币（¥）计价 · 官方原价以美元（$）标注 · 汇率按 1:7.0 折算（单位：百万 tokens）'
              : 'Our prices in CNY (¥) · Official price in USD ($) · Exchange rate 1:7.0 · Per million tokens' }}
          </p>
          <div class="table-wrapper mirror-reveal">
            <table class="pricing-table">
              <thead>
                <tr>
                  <th>{{ isZh ? '模型' : 'Model' }}</th>
                  <th>{{ isZh ? '类型' : 'Type' }}</th>
                  <th>{{ isZh ? '倍率' : 'Rate' }}</th>
                  <th>{{ isZh ? '输入 ¥/M' : 'Input ¥/M' }}</th>
                  <th>{{ isZh ? '输出 ¥/M' : 'Output ¥/M' }}</th>
                  <th>{{ isZh ? '官方原价 ¥/M' : 'Official ¥/M' }}</th>
                  <th>{{ isZh ? '折扣' : 'Discount' }}</th>
                  <th>{{ isZh ? '逆向' : 'Proxy' }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in pricingRows" :key="`${row.model}-${row.type}`">
                  <td class="cell-name">
                    <img v-if="row.logo" :src="row.logo" :alt="row.model" class="pricing-logo" />
                    {{ row.model }}
                  </td>
                  <td><span class="type-badge" :class="`type-badge--${row.typeColor}`">{{ row.type }}</span></td>
                  <td class="cell-rate">{{ row.rate }}</td>
                  <td>{{ row.inputCny }}</td>
                  <td>{{ row.outputCny }}</td>
                  <td class="cell-official">{{ row.official }}</td>
                  <td><span class="discount-badge">{{ row.discount }}</span></td>
                  <td><span :class="row.proxy ? 'proxy-yes' : 'proxy-no'">{{ row.proxy ? '是' : '否' }}</span></td>
                </tr>
              </tbody>
            </table>
          </div>
          <div class="pricing-section__cta mirror-reveal">
            <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="pricing-cta-btn">
              {{ isZh ? '免费注册 立即体验' : 'Sign up free — start now' }}
            </router-link>
          </div>
        </div>
      </section>

      <!-- ── STATS ── -->
      <section ref="statsRef" class="stats-section">
        <div class="stats-section__container">
          <h2 class="stats-section__title mirror-reveal">
            {{ isZh ? '已被开发者广泛信赖' : 'Trusted by developers' }}
            <span class="stats-section__star">✦</span>
          </h2>
          <p class="stats-section__subtitle mirror-reveal">
            {{ isZh ? '持续稳定运行，服务全球开发者与团队。' : 'Running reliably, serving developers and teams worldwide.' }}
          </p>
          <div class="stats-list mirror-reveal">
            <div class="stat-item">
              <p class="stat-num stat-num--xl">{{ statDev.toLocaleString() }}<span class="stat-plus">+</span></p>
              <p class="stat-label">{{ isZh ? '服务开发者' : 'Developers served' }}</p>
            </div>
            <div class="stat-divider"></div>
            <div class="stat-item">
              <p class="stat-num stat-num--xl">{{ statCall }}<span class="stat-unit">{{ isZh ? '万' : 'M' }}</span><span class="stat-plus">+</span></p>
              <p class="stat-label">{{ isZh ? '累计调用次数' : 'Total API calls (M)' }}</p>
            </div>
            <div class="stat-divider"></div>
            <div class="stat-item">
              <p class="stat-num">{{ statResp }}<span class="stat-unit">s</span></p>
              <p class="stat-label">{{ isZh ? '平均响应时间' : 'Avg response time' }}</p>
            </div>
          </div>
        </div>
      </section>
    </main>

    <!-- ── FOOTER ── -->
    <footer class="home-footer">
      <div class="home-footer__container">
        <div class="home-footer__bottom">
          <p>&copy; {{ currentYear }} {{ brandName }}. {{ t('home.footer.allRightsReserved') }}</p>
          <div class="home-footer__links">
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ isZh ? '文档' : 'Docs' }}</a>
            <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
            <span v-if="contactInfo">{{ contactInfo }}</span>
          </div>
        </div>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, type Directive } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore, useAuthStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'


// ── mouse-follow radial glow ─────────────────────────────────────────────────
const vMouseGlow: Directive<HTMLElement> = {
  mounted(el) {
    const handler = (e: MouseEvent) => {
      const rect = el.getBoundingClientRect()
      el.style.setProperty('--mouse-x', `${e.clientX - rect.left}px`)
      el.style.setProperty('--mouse-y', `${e.clientY - rect.top}px`)
    }
    el.addEventListener('mousemove', handler)
    ;(el as any).__mouseHandler = handler
  },
  unmounted(el) { el.removeEventListener('mousemove', (el as any).__mouseHandler) },
}

const { t, locale } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const fallbackBrand = 'gw-link'
const fallbackDocUrl = 'https://github.com/Wei-Shaw/sub2api#readme'

const brandName = computed(() => {
  const configured = appStore.cachedPublicSettings?.site_name || appStore.siteName
  if (!configured || configured === 'Sub2API') return fallbackBrand
  return configured
})
const brandLogo = computed(() => '/gw-link-logo.svg')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || fallbackDocUrl)
const contactInfo = computed(() => appStore.cachedPublicSettings?.contact_info || appStore.contactInfo || '')

const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const c = homeContent.value.trim()
  return c.startsWith('http://') || c.startsWith('https://')
})

const githubUrl = 'https://github.com/Wei-Shaw/sub2api'
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const currentYear = computed(() => new Date().getFullYear())
const isZh = computed(() => locale.value.startsWith('zh'))

// ── theme ───────────────────────────────────────────────────────────────────
const isDark = ref(document.documentElement.classList.contains('dark'))

function initTheme() {
  const saved = localStorage.getItem('theme')
  isDark.value = saved === 'dark' || (!saved && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', isDark.value)
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// ── header scroll ────────────────────────────────────────────────────────────
const headerRef = ref<HTMLElement | null>(null)
const headerScrolled = ref(false)

// ── mirror-reveal (IntersectionObserver for .mirror-reveal elements) ─────────
let revealObs: IntersectionObserver | null = null

// ── stats counter ────────────────────────────────────────────────────────────
const statsRef = ref<HTMLElement | null>(null)
const statDev = ref(0)
const statCall = ref(0)
const statResp = ref('0.0')
let statsObs: IntersectionObserver | null = null

function animateInt(to: number, ms: number, set: (v: number) => void) {
  const start = performance.now()
  const tick = (now: number) => {
    const p = Math.min((now - start) / ms, 1)
    set(Math.round(to * (1 - Math.pow(1 - p, 3))))
    if (p < 1) requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
}

function animateFloat(to: number, ms: number, set: (v: string) => void) {
  const start = performance.now()
  const tick = (now: number) => {
    const p = Math.min((now - start) / ms, 1)
    set((to * (1 - Math.pow(1 - p, 3))).toFixed(1))
    if (p < 1) requestAnimationFrame(tick)
  }
  requestAnimationFrame(tick)
}

// ── data ─────────────────────────────────────────────────────────────────────
const osList = [
  { name: 'macOS', logo: '/logos/apple.svg' },
  { name: 'Windows', logo: '/logos/windows.svg' },
  { name: 'Linux', logo: '/logos/linux.svg' },
]

const aiTools = computed(() => [
  { name: 'Claude Code', logo: '/logos/claude.svg', coming: false },
  { name: 'Codex / GPT', logo: '/logos/openai.svg', coming: false },
  { name: 'Gemini CLI', logo: '/logos/gemini.svg', coming: true },
  { name: isZh.value ? '兼容接口' : 'OpenAI API', logo: '/logos/openai.svg', coming: false },
])


const whyCards = computed(() => isZh.value
  ? [
      { id: 1, title: '🔒 数据安全，合规可信', desc: '请求体与响应内容过网关即丢弃，永不留存。TLS 全程加密，密钥按团队、成员独立隔离，满足企业数据合规要求。' },
      { id: 2, title: '⚡ 99.9% 可用，秒级容灾', desc: '账号池自动轮询，单点限流秒级切换至备用线路，业务无感中断。适合长期生产部署，不担心某个账号突然挂掉。' },
      { id: 3, title: '📊 调用全程可审计', desc: '每笔请求按用户、密钥、模型留存明细，Token 用量实时统计，支持配额预警与导出，月底对账一目了然。' },
      { id: 4, title: '🚀 30 秒完成接入', desc: '保留 OpenAI 兼容 SDK，只替换 base_url 和 api_key，老代码无需任何改动，团队成员按文档即可独立接入。' },
    ]
  : [
      { id: 1, title: '🔒 Enterprise-grade security', desc: 'Request and response bodies are discarded on transit — never stored. TLS end-to-end, keys isolated per team and member to meet enterprise compliance.' },
      { id: 2, title: '⚡ 99.9% uptime, instant failover', desc: 'Account pool rotation with second-level failover on rate limits. Zero perceived downtime. Built for long-running production workloads.' },
      { id: 3, title: '📊 Full audit trail', desc: 'Every request logged by user, key and model. Real-time token stats, quota alerts and report export. Month-end billing reconciliation made easy.' },
      { id: 4, title: '🚀 Live in 30 seconds', desc: 'Keep your OpenAI-compatible SDK. Swap base_url and api_key only — no other code changes. Team members onboard independently with the docs.' },
    ],
)


const platforms = [
  { name: 'VS Code', logo: '/logos/vscode.svg' },
  { name: 'Cursor', logo: '/logos/cursor.svg' },
  { name: 'JetBrains', logo: '/logos/jetbrains.svg' },
  { name: 'Continue', logo: '/logos/continue.svg' },
  { name: 'Claude Code', logo: '/logos/claude.svg' },
]

const pricingRows = [
  { model: 'Opus 4.7',   logo: '/logos/claude.svg', type: '企业稳定版', typeColor: 'blue',   rate: '2.5x',  inputCny: '¥12.50', outputCny: '¥62.50', official: '$5 / $25',    discount: '3.6折', proxy: false },
  { model: 'Sonnet 4.6', logo: '/logos/claude.svg', type: '企业稳定版', typeColor: 'blue',   rate: '2.5x',  inputCny: '¥7.50',  outputCny: '¥37.50', official: '$3 / $15',    discount: '3.6折', proxy: false },
  { model: 'Opus 4.7',   logo: '/logos/claude.svg', type: 'Kiro 逆向',  typeColor: 'orange', rate: '0.65x', inputCny: '¥3.25',  outputCny: '¥16.25', official: '$5 / $25',    discount: '0.9折', proxy: true  },
  { model: 'Sonnet 4.6', logo: '/logos/claude.svg', type: 'Kiro 逆向',  typeColor: 'orange', rate: '0.65x', inputCny: '¥1.95',  outputCny: '¥9.75',  official: '$3 / $15',    discount: '0.9折', proxy: true  },
  { model: 'Opus 4.6',   logo: '/logos/claude.svg', type: '反重力逆向', typeColor: 'purple', rate: '0.85x', inputCny: '¥4.25',  outputCny: '¥21.25', official: '$5 / $25',    discount: '1.2折', proxy: true  },
  { model: 'Sonnet 4.6', logo: '/logos/claude.svg', type: '反重力逆向', typeColor: 'purple', rate: '0.85x', inputCny: '¥2.55',  outputCny: '¥12.75', official: '$3 / $15',    discount: '1.2折', proxy: true  },
  { model: 'GPT 5.4',    logo: '/logos/openai.svg', type: 'Codex',      typeColor: 'green',  rate: '0.5x',  inputCny: '¥2.50',  outputCny: '¥12.25', official: '$5 / $22.5',  discount: '0.7折', proxy: true  },
  { model: 'GPT 5.5',    logo: '/logos/openai.svg', type: 'Codex',      typeColor: 'green',  rate: '0.5x',  inputCny: '¥2.50',  outputCny: '¥14.00', official: '$5 / $30',    discount: '0.7折', proxy: true  },
]

const partnersRow1 = [
  { id: 'tcl',      name: 'TCL',    logo: '/logos/enterprise/tcl.svg' },
  { id: 'kaadas',   name: '凯迪仕', logo: '/logos/enterprise/kaadas.svg' },
  { id: 'tianfeng', name: '天风证券', logo: '/logos/enterprise/tianfeng.svg' },
  { id: 'zhaochi',  name: '兆驰股份', logo: '/logos/enterprise/zhaochi.svg' },
  { id: 'xiaomi',   name: '小米',   logo: '/logos/enterprise/xiaomi.svg' },
  { id: 'huawei',   name: '华为',   logo: '/logos/enterprise/huawei.svg' },
  { id: 'byd',      name: '比亚迪', logo: '/logos/enterprise/byd.svg' },
]

const partnersRow2 = [
  { id: 'baidu',     name: '百度',   logo: '/logos/enterprise/baidu.svg' },
  { id: 'tencent',   name: '腾讯',   logo: '/logos/enterprise/tencent.svg' },
  { id: 'bytedance', name: '字节跳动', logo: '/logos/enterprise/bytedance.svg' },
  { id: 'cmb',       name: '招商银行', logo: '/logos/enterprise/cmb.svg' },
  { id: 'pingan',    name: '中国平安', logo: '/logos/enterprise/pingan.svg' },
  { id: 'midea',     name: '美的',   logo: '/logos/enterprise/midea.svg' },
  { id: 'haier',     name: '海尔',   logo: '/logos/enterprise/haier.svg' },
]

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()

  // header scroll
  const onScroll = () => { headerScrolled.value = window.scrollY > 10 }
  window.addEventListener('scroll', onScroll, { passive: true })
  ;(headerRef.value as any)?.__scrollHandler || (document as any).__scrollCleanup
  ;(document as any).__scrollCleanup = () => window.removeEventListener('scroll', onScroll)

  // mirror-reveal via IntersectionObserver
  revealObs = new IntersectionObserver(
    (entries) => {
      entries.forEach(entry => {
        if (entry.isIntersecting) {
          entry.target.classList.add('is-visible')
          revealObs?.unobserve(entry.target)
        }
      })
    },
    { threshold: 0.12, rootMargin: '0px 0px -40px 0px' },
  )
  document.querySelectorAll('.mirror-reveal').forEach(el => revealObs?.observe(el))

  // stats counter
  if (statsRef.value) {
    statsObs = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          animateInt(5000, 1800, v => (statDev.value = v))
          animateInt(2000, 2000, v => (statCall.value = v))
          animateFloat(10.0, 1400, v => (statResp.value = v))
          statsObs?.disconnect()
        }
      },
      { threshold: 0.3 },
    )
    statsObs.observe(statsRef.value)
  }
})

onUnmounted(() => {
  revealObs?.disconnect()
  statsObs?.disconnect()
  ;(document as any).__scrollCleanup?.()
})
</script>

<style scoped>
@import url('https://fonts.googleapis.com/css2?family=Manrope:wght@400;500;600;700;800&family=Noto+Sans+SC:wght@300;400;500;700;800;900&display=swap');

/* ── TOKENS ───────────────────────────────────────────────────────────────── */
.home-page {
  min-height: 100vh;
  background: #fff;
  color: #1a1a2e;
  font-family: 'Noto Sans SC', 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', Manrope, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}

.dark .home-page {
  background: #0d1117;
  color: #e6edf3;
}

/* ── SCROLL REVEAL ───────────────────────────────────────────────────────── */
.mirror-reveal {
  opacity: 0;
  transform: translateY(24px);
  filter: blur(10px);
  transition:
    opacity 0.8s cubic-bezier(0.16, 1, 0.3, 1),
    transform 0.8s cubic-bezier(0.16, 1, 0.3, 1),
    filter 0.8s cubic-bezier(0.16, 1, 0.3, 1);
}

.mirror-reveal.is-visible {
  opacity: 1;
  transform: translateY(0);
  filter: blur(0);
}

/* stagger helper: apply to nth siblings */
.mirror-reveal:nth-child(2) { transition-delay: 0.08s; }
.mirror-reveal:nth-child(3) { transition-delay: 0.16s; }
.mirror-reveal:nth-child(4) { transition-delay: 0.24s; }
.mirror-reveal:nth-child(5) { transition-delay: 0.32s; }
.mirror-reveal:nth-child(6) { transition-delay: 0.40s; }

/* ── HEADER ──────────────────────────────────────────────────────────────── */
.home-header {
  position: fixed;
  top: 0; left: 0; right: 0;
  z-index: 50;
  background: transparent;
  border-bottom: 1px solid transparent;
  transition: background 0.4s ease, border-color 0.4s ease, backdrop-filter 0.4s ease;
}

.home-header.is-scrolled {
  background: rgba(255, 255, 255, 0.85);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-bottom: 1px solid rgba(0, 0, 0, 0.04);
}

.dark .home-header.is-scrolled {
  background: rgba(13, 17, 23, 0.85);
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
}

.home-header__inner {
  width: min(100% - 4rem, 1400px);
  margin: 0 auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1.5rem;
  padding: 1.25rem 0;
}

.home-header__brand {
  display: flex;
  align-items: center;
  gap: 0.625rem;
  flex: 1;
  text-decoration: none;
  color: inherit;
}

.home-header__logo { width: 9rem; height: 2.25rem; object-fit: contain; flex-shrink: 0; }
.home-header__logo-fallback { width: 1.75rem; height: 1.75rem; background: #1a1a2e; color: #fff; border-radius: 6px; display: flex; align-items: center; justify-content: center; font-size: 0.75rem; font-weight: 900; }
.home-header__brand-text { font-size: 1.125rem; font-weight: 700; letter-spacing: -0.01em; white-space: nowrap; display: none; }

.home-header__menu {
  display: flex;
  align-items: center;
  gap: 2.5rem;
  flex-shrink: 0;
}

.home-header__link {
  color: #1a1a2e;
  font-size: 0.9375rem;
  font-weight: 500;
  text-decoration: none;
  opacity: 0.7;
  transition: opacity 0.2s ease;
}
.dark .home-header__link { color: #e6edf3; }
.home-header__link:hover { opacity: 1; }

.home-header__actions {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  justify-content: flex-end;
  flex: 1;
}

.home-header__theme {
  display: inline-flex;
  width: 2.1rem; height: 2.1rem;
  align-items: center; justify-content: center;
  border-radius: 999px;
  border: 1px solid rgba(26, 26, 46, 0.15);
  background: transparent;
  color: #1a1a2e;
  cursor: pointer;
  transition: background 0.2s ease;
}
.dark .home-header__theme { border-color: rgba(255,255,255,0.12); color: #e6edf3; }
.home-header__theme:hover { background: rgba(26, 26, 46, 0.06); }

.home-header__cta {
  display: inline-flex;
  align-items: center; justify-content: center;
  height: 2.5rem;
  padding: 0 1.75rem;
  border-radius: 999px;
  background: transparent;
  color: #1a1a2e;
  border: 1px solid #1a1a2e;
  font-size: 0.875rem;
  font-weight: 500;
  text-decoration: none;
  transition: all 0.2s ease;
}
.dark .home-header__cta { color: #e6edf3; border-color: rgba(255,255,255,0.3); }
.home-header__cta:hover { background: #1a1a2e; color: #fff; }

/* ── HERO ────────────────────────────────────────────────────────────────── */
@keyframes bgDrift {
  0%   { transform: translate(0, 0) scale(1); }
  100% { transform: translate(6%, 4%) scale(1.06); }
}

@keyframes textShimmer {
  to { background-position: 200% center; }
}

.hero-section {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding-top: 80px;
  overflow: hidden;
  background: #fff;
}

.dark .hero-section { background: #0d1117; }

.hero-section__bg {
  position: absolute;
  inset: -15%;
  width: 130%; height: 130%;
  background:
    radial-gradient(ellipse 80% 60% at 20% 30%, rgba(79, 140, 255, 0.08) 0%, transparent 60%),
    radial-gradient(ellipse 60% 50% at 80% 70%, rgba(99, 102, 241, 0.06) 0%, transparent 55%),
    radial-gradient(ellipse 100% 80% at 50% 50%, rgba(45, 212, 191, 0.04) 0%, transparent 70%);
  background-size: cover;
  background-position: center;
  z-index: 0;
  animation: bgDrift 10s ease-in-out infinite alternate;
  will-change: transform;
}

.hero-section__container {
  position: relative;
  z-index: 1;
  width: min(100% - 4rem, 1200px);
  margin: 0 auto;
  text-align: center;
}

.hero-section__content {
  display: flex;
  flex-direction: column;
  align-items: center;
  max-width: 900px;
  margin: 0 auto;
}

.hero-section__title {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1.5rem;
  margin-bottom: 2rem;
  line-height: 1.2;
  flex-wrap: wrap;
}

.hero-section__kicker {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0.5rem 1.75rem;
  border: 1px solid #1a1a2e;
  border-radius: 999px;
  font-size: 2.25rem;
  font-weight: 500;
  color: #1a1a2e;
  letter-spacing: -0.02em;
  animation: fadeInUp 0.8s cubic-bezier(0.16, 1, 0.3, 1) both;
  animation-delay: 0.1s;
}
.dark .hero-section__kicker { border-color: rgba(255,255,255,0.3); color: #e6edf3; }

.hero-section__main {
  font-size: 4.5rem;
  font-weight: 700;
  color: #1a1a2e;
  letter-spacing: -0.01em;
  background: linear-gradient(to right, #1a1a2e, #1a1a2e 40%, #4f8cff, #1a1a2e 60%, #1a1a2e);
  background-size: 200% auto;
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  animation:
    fadeInUp 0.9s cubic-bezier(0.16, 1, 0.3, 1) both 0.2s,
    textShimmer 6s linear infinite;
}

.dark .hero-section__main {
  background: linear-gradient(to right, #e6edf3, #e6edf3 40%, #7aa7ff, #e6edf3 60%, #e6edf3);
  background-size: 200% auto;
  -webkit-background-clip: text;
  background-clip: text;
  -webkit-text-fill-color: transparent;
  animation:
    fadeInUp 0.9s cubic-bezier(0.16, 1, 0.3, 1) both 0.2s,
    textShimmer 6s linear infinite;
}

.hero-section__desc {
  font-size: 1.125rem;
  line-height: 1.8;
  color: rgba(26, 26, 46, 0.6);
  margin-bottom: 3.5rem;
  animation: fadeInUp 0.8s cubic-bezier(0.16, 1, 0.3, 1) both 0.35s;
}
.dark .hero-section__desc { color: rgba(230, 237, 243, 0.6); }

.hero-section__actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 1.25rem;
  animation: fadeInUp 0.8s cubic-bezier(0.16, 1, 0.3, 1) both 0.5s;
}

.hero-section__btn {
  display: inline-flex;
  align-items: center; justify-content: center;
  height: 3.5rem;
  padding: 0 2.5rem;
  border-radius: 999px;
  font-size: 1rem;
  font-weight: 600;
  text-decoration: none;
  transition: all 0.2s cubic-bezier(0.16, 1, 0.3, 1);
  cursor: pointer;
  border: none;
}

.hero-section__btn--primary {
  background: #000;
  color: #fff;
}
.dark .hero-section__btn--primary { background: #e6edf3; color: #0d1117; }
.hero-section__btn--primary:hover { transform: translateY(-2px); box-shadow: 0 12px 24px rgba(0,0,0,0.22); }

.hero-section__btn--outline {
  background: transparent;
  color: #1a1a2e;
  border: 1px solid rgba(26, 26, 46, 0.3);
}
.dark .hero-section__btn--outline { color: #e6edf3; border-color: rgba(255,255,255,0.25); }
.hero-section__btn--outline:hover { border-color: #1a1a2e; background: rgba(0,0,0,0.03); transform: translateY(-2px); }

/* Hero OS badges */
.hero-os-compat {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.75rem;
  margin-top: 2rem;
  flex-wrap: wrap;
  animation: fadeInUp 0.8s cubic-bezier(0.16, 1, 0.3, 1) both;
  animation-delay: 0.65s;
}

.hero-os-label {
  font-size: 0.8rem;
  font-weight: 500;
  color: rgba(26, 26, 46, 0.45);
  margin-right: 0.25rem;
}
.dark .hero-os-label { color: rgba(230, 237, 243, 0.4); }

.hero-os-item {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.35rem 0.9rem;
  background: rgba(255,255,255,0.7);
  border: 1px solid rgba(0,0,0,0.07);
  border-radius: 999px;
  backdrop-filter: blur(8px);
  transition: all 0.2s ease;
}
.dark .hero-os-item { background: rgba(28, 33, 40, 0.7); border-color: rgba(255,255,255,0.08); }
.hero-os-item:hover { border-color: #4f8cff; box-shadow: 0 3px 10px rgba(79,140,255,0.15); }

.hero-os-logo {
  width: 1rem;
  height: 1rem;
  object-fit: contain;
  opacity: 0.65;
}
.dark .hero-os-logo { filter: brightness(0) invert(1); opacity: 0.6; }

.hero-os-item span {
  font-size: 0.8rem;
  font-weight: 600;
  color: rgba(26, 26, 46, 0.65);
}
.dark .hero-os-item span { color: rgba(230, 237, 243, 0.6); }

@keyframes fadeInUp {
  from { opacity: 0; transform: translateY(20px); }
  to   { opacity: 1; transform: translateY(0); }
}

/* ── TRANSFER EXPERIENCE ─────────────────────────────────────────────────── */
.transfer-experience {
  padding: 8rem 0;
  background-color: #f8f9fa;
}
.dark .transfer-experience { background-color: #161b22; }

.transfer-experience__container {
  width: min(100% - 4rem, 1200px);
  margin: 0 auto;
}

.eyebrow-badge {
  display: inline-flex;
  padding: 0.25rem 1rem;
  background: transparent;
  border: 1px solid rgba(26, 26, 46, 0.15);
  border-radius: 999px;
  font-size: 0.875rem;
  font-weight: 500;
  color: #1a1a2e;
}
.dark .eyebrow-badge { border-color: rgba(255,255,255,0.15); color: #e6edf3; }

.eyebrow-badge--dark { border-color: #1a1a2e; }
.dark .eyebrow-badge--dark { border-color: rgba(255,255,255,0.4); }

.transfer-experience__eyebrow { margin-bottom: 1.5rem; }

@keyframes twinkleStar {
  0%, 100% { transform: scale(0.9) rotate(0deg); opacity: 0.8; }
  50%       { transform: scale(1.1) rotate(15deg); opacity: 1; }
}

.transfer-experience__title {
  font-size: 3rem;
  font-weight: 700;
  color: #1a1a2e;
  margin-bottom: 1.5rem;
}
.dark .transfer-experience__title { color: #e6edf3; }

.transfer-experience__accent { color: #4f8cff; }
.transfer-experience__accent-blue { color: #4f8cff; }

.transfer-experience__star {
  display: inline-block;
  margin-left: 0.5rem;
  font-size: 2rem;
  color: #1a1a2e;
  animation: twinkleStar 4s ease-in-out infinite;
}
.dark .transfer-experience__star { color: #e6edf3; }

.transfer-experience__desc {
  font-size: 1.125rem;
  color: #666;
  margin-bottom: 4rem;
}
.dark .transfer-experience__desc { color: rgba(230,237,243,0.6); }

/* OS compatibility row */
.os-compat {
  display: flex;
  align-items: center;
  gap: 1rem;
  margin-bottom: 2.5rem;
  flex-wrap: wrap;
}

.os-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 1.25rem;
  background: #fff;
  border: 1px solid rgba(0,0,0,0.08);
  border-radius: 999px;
  transition: all 0.2s ease;
  cursor: default;
}
.dark .os-item { background: #1c2128; border-color: rgba(255,255,255,0.08); }
.os-item:hover { border-color: #4f8cff; box-shadow: 0 4px 12px rgba(79,140,255,0.12); }

.os-logo { width: 1.1rem; height: 1.1rem; object-fit: contain; opacity: 0.7; }
.dark .os-logo { opacity: 0.6; filter: brightness(0) invert(1); }
.os-item span { font-size: 0.875rem; font-weight: 600; color: #1a1a2e; }
.dark .os-item span { color: #e6edf3; }

/* Card note */
.transfer-experience__card-note {
  font-size: 0.9375rem;
  color: rgba(26,26,46,0.55);
  line-height: 1.7;
  margin: -0.5rem 0 2rem;
}
.dark .transfer-experience__card-note { color: rgba(230,237,243,0.45); }

/* AI tools grid */
.ai-tools-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 1rem;
}

.ai-tool-card {
  display: flex;
  align-items: center;
  gap: 0.875rem;
  padding: 1.1rem 1.25rem;
  background: #f8f9fa;
  border: 1px solid rgba(0,0,0,0.06);
  border-radius: 1rem;
  transition: all 0.3s cubic-bezier(0.16,1,0.3,1);
}
.dark .ai-tool-card { background: #0d1117; border-color: rgba(255,255,255,0.06); }
.ai-tool-card:hover { background: #fff; box-shadow: 0 8px 24px rgba(0,0,0,0.08); transform: translateY(-3px); }
.dark .ai-tool-card:hover { background: #161b22; }
.ai-tool-card--coming { opacity: 0.5; border-style: dashed; }

.ai-tool-card__logo { width: 2rem; height: 2rem; flex-shrink: 0; display: flex; align-items: center; justify-content: center; }
.ai-tool-card__logo img { width: 100%; height: 100%; object-fit: contain; }
.ai-tool-card__name { font-size: 0.9rem; font-weight: 700; color: #1a1a2e; }
.dark .ai-tool-card__name { color: #e6edf3; }
.ai-tool-card__badge { margin-top: 0.2rem; font-size: 0.7rem; font-weight: 600; color: #6b7280; }

@media (max-width: 768px) {
  .ai-tools-grid { grid-template-columns: repeat(2, 1fr); }
}

.transfer-experience__card {
  background: #fff;
  border-radius: 2rem;
  padding: 4rem;
  box-shadow: 0 4px 24px rgba(0,0,0,0.05);
  margin-bottom: 3rem;
  position: relative;
  overflow: hidden;
  transition: transform 0.4s cubic-bezier(0.16,1,0.3,1), box-shadow 0.4s cubic-bezier(0.16,1,0.3,1);
}
.dark .transfer-experience__card { background: #1c2128; }
.transfer-experience__card:hover { transform: translateY(-4px); box-shadow: 0 16px 40px rgba(0,0,0,0.1); }
.transfer-experience__card::before {
  content: '';
  position: absolute; inset: 0;
  background: radial-gradient(800px circle at var(--mouse-x, -999px) var(--mouse-y, -999px), rgba(79,140,255,0.05), transparent 40%);
  z-index: 0; pointer-events: none; opacity: 0;
  transition: opacity 0.4s ease;
}
.transfer-experience__card:hover::before { opacity: 1; }
.transfer-experience__card > * { position: relative; z-index: 1; }

.transfer-experience__card-intro {
  font-size: 1.125rem;
  line-height: 1.8;
  color: #1a1a2e;
  margin: 0 0 2rem;
}
.dark .transfer-experience__card-intro { color: #e6edf3; }

.transfer-experience__inner {
  display: flex;
  align-items: stretch;
  background: #f1f3f5;
  border-radius: 1rem;
  padding: 2.5rem 3rem;
}
.dark .transfer-experience__inner { background: #0d1117; }

.transfer-experience__systems { flex: 1; display: flex; flex-direction: column; justify-content: center; }

.systems-list { display: flex; flex-wrap: wrap; gap: 2rem; margin-bottom: 1.5rem; }
.system-item { display: flex; align-items: center; gap: 0.5rem; }
.system-icon { width: 1.5rem; height: 1.5rem; object-fit: contain; }
.system-item span { font-size: 0.9375rem; font-weight: 600; color: #1a1a2e; }
.dark .system-item span { color: #e6edf3; }
.system-note { font-size: 0.875rem; color: rgba(26,26,46,0.6); line-height: 1.6; }
.dark .system-note { color: rgba(230,237,243,0.5); }

.transfer-experience__divider { width: 1px; background: rgba(0,0,0,0.1); margin: 0 3rem; }
.dark .transfer-experience__divider { background: rgba(255,255,255,0.08); }

.transfer-experience__tools { flex: 1.2; display: flex; flex-direction: column; justify-content: center; text-align: center; }
.tools-title { font-size: 1.125rem; font-weight: 700; color: #1a1a2e; margin-bottom: 1.5rem; }
.dark .tools-title { color: #e6edf3; }
.tools-grid { display: flex; flex-wrap: wrap; gap: 2rem; justify-content: center; }
.tool-item { display: flex; align-items: center; gap: 0.5rem; }
.tool-icon { color: #1a1a2e; opacity: 0.5; }
.dark .tool-icon { color: #e6edf3; }
.tool-item span { font-size: 0.9375rem; font-weight: 500; color: #666; }
.dark .tool-item span { color: rgba(230,237,243,0.5); }

.enterprise-trust {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
}

.enterprise-trust__item {
  display: flex;
  align-items: flex-start;
  gap: 1rem;
  padding: 1.5rem;
  background: #fff;
  border: 1px solid rgba(0,0,0,0.06);
  border-radius: 1rem;
  transition: transform 0.3s cubic-bezier(0.16,1,0.3,1), box-shadow 0.3s ease;
}
.dark .enterprise-trust__item { background: #1c2128; border-color: rgba(255,255,255,0.06); }
.enterprise-trust__item:hover { transform: translateY(-3px); box-shadow: 0 12px 32px rgba(0,0,0,0.08); }

.enterprise-trust__icon { font-size: 1.4rem; flex-shrink: 0; margin-top: 0.1rem; }

.enterprise-trust__title { font-size: 0.9375rem; font-weight: 700; color: #1a1a2e; margin-bottom: 0.35rem; }
.dark .enterprise-trust__title { color: #e6edf3; }

.enterprise-trust__desc { font-size: 0.875rem; line-height: 1.7; color: rgba(26,26,46,0.55); margin: 0; }
.dark .enterprise-trust__desc { color: rgba(230,237,243,0.48); }

.transfer-experience__footer-title {
  font-size: 3.5rem;
  font-weight: 700;
  color: #1a1a2e;
  text-align: center;
  line-height: 1.4;
}
.dark .transfer-experience__footer-title { color: #e6edf3; }

/* ── WHY CHOOSE ──────────────────────────────────────────────────────────── */
.why-choose { padding: 6rem 0 8rem; background-color: #f8f9fa; }
.dark .why-choose { background-color: #161b22; }

.why-choose__container { width: min(100% - 4rem, 1200px); margin: 0 auto; }

.why-choose__header { margin-bottom: 3rem; }

.why-choose__section-title {
  font-size: 2.75rem;
  font-weight: 700;
  color: #1a1a2e;
  margin-top: 1rem;
  line-height: 1.25;
}
.dark .why-choose__section-title { color: #e6edf3; }

.why-choose__star {
  display: inline-block;
  margin-left: 0.5rem;
  font-size: 1.8rem;
  animation: twinkleStar 4s ease-in-out infinite;
}

.why-choose__grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 1.25rem;
}

.why-choose__card {
  position: relative;
  background: #eef0f3;
  border-radius: 1.5rem;
  padding: 2.5rem;
  min-height: 240px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  transition: transform 0.4s cubic-bezier(0.16,1,0.3,1), box-shadow 0.4s cubic-bezier(0.16,1,0.3,1);
}
.dark .why-choose__card { background: #1c2128; }
.why-choose__card:hover { transform: translateY(-6px); box-shadow: 0 16px 40px rgba(0,0,0,0.12); }
.why-choose__card::before {
  content: ''; position: absolute; inset: 0;
  background: radial-gradient(600px circle at var(--mouse-x, -999px) var(--mouse-y, -999px), rgba(255,255,255,0.6), transparent 40%);
  z-index: 0; pointer-events: none; opacity: 0; transition: opacity 0.4s ease;
}
.dark .why-choose__card::before { background: radial-gradient(600px circle at var(--mouse-x, -999px) var(--mouse-y, -999px), rgba(255,255,255,0.06), transparent 40%); }
.why-choose__card:hover::before { opacity: 1; }
.why-choose__card > * { position: relative; z-index: 1; }

.why-choose__card--logo {
  justify-content: flex-end;
  align-items: flex-start;
}

.why-choose__number {
  font-size: 3rem;
  font-weight: 800;
  color: #4f8cff;
  line-height: 1;
  margin-bottom: 1.5rem;
  transition: transform 0.4s cubic-bezier(0.16,1,0.3,1), color 0.4s ease;
}
.why-choose__card:hover .why-choose__number { transform: scale(1.05); color: #3b76eb; }

.why-choose__content { position: relative; z-index: 2; }
.why-choose__title { font-size: 1.25rem; font-weight: 700; color: #1a1a2e; margin-bottom: 0.75rem; }
.dark .why-choose__title { color: #e6edf3; }
.why-choose__desc { font-size: 0.9375rem; line-height: 1.7; color: rgba(26,26,46,0.55); }
.dark .why-choose__desc { color: rgba(230,237,243,0.5); }

.why-choose__logo { display: flex; align-items: center; gap: 0.625rem; }
.why-choose__logo-img { width: 9rem; height: 2rem; object-fit: contain; }
.why-choose__logo span { font-size: 1rem; font-weight: 600; color: #1a1a2e; }
.dark .why-choose__logo span { color: #e6edf3; }

/* ── MODELS ──────────────────────────────────────────────────────────────── */
.model-section {
  position: relative;
  padding: 6rem 0;
  background: #edf3ff;
}
.dark .model-section { background: #111827; }

.model-section__accent-line {
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 5px;
  background: #3b82f6;
  border-radius: 0 4px 4px 0;
}

.model-section__container {
  width: min(100% - 4rem, 1200px);
  margin: 0 auto;
  text-align: center;
  padding: 2rem 0;
}

.model-section__eyebrow { margin-bottom: 1.5rem; }

.model-section__title {
  font-size: 3rem;
  font-weight: 700;
  color: #1a1a2e;
  margin-bottom: 1.5rem;
}
.dark .model-section__title { color: #e6edf3; }

.model-section__star {
  display: inline-block;
  margin-left: 0.5rem;
  font-size: 2rem;
  animation: twinkleStar 4s ease-in-out infinite;
}

.model-section__desc {
  font-size: 1.125rem;
  color: rgba(26,26,46,0.6);
  margin-bottom: 3rem;
}
.dark .model-section__desc { color: rgba(230,237,243,0.5); }

.model-section__grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 1.25rem;
}

.model-card-new {
  background: #fff;
  border-radius: 1.25rem;
  padding: 2rem 2.5rem;
  text-align: left;
  display: flex;
  flex-direction: column;
  border: 1px solid #e8ecf2;
  position: relative;
  overflow: hidden;
  transition: transform 0.4s cubic-bezier(0.16,1,0.3,1), box-shadow 0.4s cubic-bezier(0.16,1,0.3,1), border-color 0.4s ease;
}
.dark .model-card-new { background: #1c2128; border-color: rgba(255,255,255,0.06); }
.model-card-new:hover { transform: translateY(-6px); box-shadow: 0 16px 40px rgba(0,0,0,0.1); border-color: transparent; }
.model-card-new::before {
  content: ''; position: absolute; inset: 0;
  background: radial-gradient(600px circle at var(--mouse-x, -999px) var(--mouse-y, -999px), rgba(59,130,246,0.06), transparent 40%);
  z-index: 0; pointer-events: none; opacity: 0; transition: opacity 0.4s ease;
}
.model-card-new:hover::before { opacity: 1; }
.model-card-new > * { position: relative; z-index: 1; }

.model-card-new--coming { background: #f9fafb; border: 1.5px dashed #d1d5db; }
.dark .model-card-new--coming { background: #161b22; border-color: rgba(255,255,255,0.1); }
.model-card-new--coming:hover { border-color: #93c5fd; }

.model-card-new__dots { display: flex; gap: 0.5rem; margin-bottom: 1.5rem; }
.model-card-new__dots span {
  width: 6px; height: 6px; border-radius: 50%;
  background: rgba(26,26,46,0.2);
  animation: dotPulse 1.8s ease-in-out infinite;
}
.model-card-new__dots span:nth-child(2) { animation-delay: 0.3s; }
.model-card-new__dots span:nth-child(3) { animation-delay: 0.6s; }
@keyframes dotPulse {
  0%, 100% { opacity: 0.3; transform: scale(1); }
  50%       { opacity: 1; transform: scale(1.3); }
}

.model-card-new__header { display: flex; align-items: center; gap: 0.75rem; margin-bottom: 1.25rem; justify-content: space-between; }
.model-card-new__coming { font-size: 0.75rem; font-weight: 600; color: #6b7280; background: #f3f4f6; padding: 0.25rem 0.75rem; border-radius: 0.5rem; }

.model-card-new__badge {
  display: inline-flex;
  align-items: center;
  gap: 0.45rem;
  padding: 0.3rem 0.75rem;
  border-radius: 0.5rem;
  font-size: 0.78rem;
  font-weight: 700;
  transition: transform 0.4s cubic-bezier(0.16,1,0.3,1);
}
.model-card-new:hover .model-card-new__badge { transform: scale(1.04); }

.badge--orange { background: #fff1e8; color: #c05621; }
.badge--blue   { background: #e0f2fe; color: #0369a1; }
.badge--green  { background: #e9fff3; color: #087443; }
.badge--gray   { background: #f3f4f6; color: #6b7280; }

.model-logo-img { width: 1rem; height: 1rem; object-fit: contain; flex-shrink: 0; }

.model-badge-label { line-height: 1; }

.platform-logo-img {
  width: 1.6rem;
  height: 1.6rem;
  object-fit: contain;
  flex-shrink: 0;
  opacity: 0.75;
}
.dark .platform-logo-img { filter: brightness(0) invert(1); opacity: 0.65; }

.model-card-new__name { font-size: 1.5rem; font-weight: 700; color: #1a1a2e; }
.dark .model-card-new__name { color: #e6edf3; }
.model-card-new__subtitle { font-size: 0.9375rem; font-weight: 500; color: rgba(26,26,46,0.7); margin-bottom: 1rem; }
.dark .model-card-new__subtitle { color: rgba(230,237,243,0.6); }
.model-card-new__desc { font-size: 0.9375rem; line-height: 1.7; color: rgba(26,26,46,0.55); }
.dark .model-card-new__desc { color: rgba(230,237,243,0.45); }
.model-card-new__sub { margin-top: auto; padding-top: 1rem; font-size: 0.8125rem; color: rgba(26,26,46,0.35); letter-spacing: 0.02em; }
.dark .model-card-new__sub { color: rgba(230,237,243,0.25); }

/* ── IDE SECTION ─────────────────────────────────────────────────────────── */
.ide-section { padding: 8rem 0; background-color: #f8f9fa; overflow: hidden; }
.dark .ide-section { background-color: #161b22; }

.ide-section__container { width: min(100% - 4rem, 1200px); margin: 0 auto; }

.ide-section__grid {
  display: grid;
  grid-template-columns: 1fr 0.85fr;
  align-items: center;
  gap: 6rem;
  margin-bottom: 8rem;
}

/* ── Terminal windows ── */
.term-stack {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  width: 100%;
  position: relative;
  z-index: 1;
}

.term-window {
  border-radius: 0.875rem;
  overflow: hidden;
  box-shadow: 0 20px 50px rgba(0,0,0,0.18);
  border: 1px solid rgba(255,255,255,0.07);
  animation: floatPanel 8s ease-in-out infinite;
}
.term-window--codex { animation-delay: 1.2s; }
.term-window:hover { animation-play-state: paused; }

.term-titlebar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.65rem 1rem;
  background: #1e2433;
}

.term-dot {
  width: 0.7rem; height: 0.7rem;
  border-radius: 50%; flex-shrink: 0;
}
.term-dot--red    { background: #ff5f57; }
.term-dot--yellow { background: #ffbd2e; }
.term-dot--green  { background: #28c840; }

.term-titlebar__label {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  margin-left: 0.5rem;
  font-size: 0.8rem;
  font-weight: 600;
  color: rgba(255,255,255,0.55);
  font-family: 'SF Mono', 'Fira Code', monospace;
}

.term-titlebar__logo {
  width: 0.9rem; height: 0.9rem;
  object-fit: contain;
  filter: brightness(0) invert(1) opacity(0.7);
}

.term-body {
  padding: 1rem 1.25rem 1.1rem;
  background: #0d1117;
  font-family: 'SF Mono', 'Fira Code', 'JetBrains Mono', monospace;
  font-size: 0.78rem;
  line-height: 1.75;
}

.term-body p { margin: 0; }

.term-prompt  { color: #7ee787; font-weight: 700; margin-right: 0.5rem; }
.term-str     { color: #a5d6ff; }
.term-dim     { color: rgba(200,210,230,0.5); }
.term-file    { color: #ffa657; }
.term-accent  { color: #79c0ff; }
.term-ok      { color: #7ee787; margin-right: 0.25rem; }
.term-warn    { color: #f0883e; margin-right: 0.25rem; }
.term-key     { color: #d2a8ff; }
.term-progress{ color: #7ee787; letter-spacing: 0.05em; }

.term-cmd    { color: #ffa657; }
.term-brand  { color: #d2a8ff; font-size: 0.62rem; letter-spacing: 0.02em; line-height: 1.4; }

.term-cursor {
  display: inline-block;
  color: #7ee787;
  animation: blink-cursor 1.1s step-end infinite;
}
@keyframes blink-cursor {
  0%, 100% { opacity: 1; }
  50%       { opacity: 0; }
}

.ide-section__visual { position: relative; display: flex; justify-content: center; align-items: flex-start; }

.claude-screenshot-wrap {
  position: relative;
  z-index: 1;
  width: 100%;
  animation: floatPanel 8s ease-in-out infinite;
}

.claude-screenshot {
  width: 100%;
  border-radius: 0.75rem;
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.22);
  display: block;
}

.ide-section__glow {
  position: absolute;
  top: 50%; left: 50%;
  transform: translate(-50%, -50%);
  width: 80%; height: 80%;
  background: radial-gradient(circle, rgba(79,140,255,0.12) 0%, transparent 70%);
  filter: blur(40px);
  z-index: -1;
  animation: pulseGlow 8s ease-in-out infinite;
}
@keyframes pulseGlow {
  0%, 100% { opacity: 0.5; transform: translate(-50%, -50%) scale(1); }
  50%       { opacity: 0.8; transform: translate(-50%, -50%) scale(1.1); }
}

.ide-code-panel {
  width: 100%;
  border-radius: 1rem;
  overflow: hidden;
  border: 1px solid rgba(0,0,0,0.08);
  box-shadow: 0 20px 50px rgba(0,0,0,0.1);
  animation: floatPanel 8s ease-in-out infinite;
  background: #fff;
  will-change: transform;
}
.dark .ide-code-panel { background: #1c2128; border-color: rgba(255,255,255,0.06); }
@keyframes floatPanel {
  0%, 100% { transform: translateY(0); }
  50%       { transform: translateY(-12px); }
}
.ide-code-panel:hover { animation-play-state: paused; box-shadow: 0 40px 80px rgba(0,0,0,0.15); }

.ide-code-toolbar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.875rem 1.25rem;
  background: #f8fafc;
  border-bottom: 1px solid rgba(0,0,0,0.06);
}
.dark .ide-code-toolbar { background: #161b22; border-color: rgba(255,255,255,0.06); }

.ide-code-title { margin-left: auto; font-family: monospace; font-size: 0.75rem; color: rgba(26,26,46,0.5); }
.dark .ide-code-title { color: rgba(230,237,243,0.4); }

.ide-code-body {
  padding: 1.5rem;
  margin: 0;
  font-family: 'Fira Code', 'JetBrains Mono', monospace;
  font-size: 0.8rem;
  line-height: 1.7;
  color: rgba(26,26,46,0.75);
  overflow-x: auto;
  background: #fff;
}
.dark .ide-code-body { background: #1c2128; color: rgba(230,237,243,0.75); }

.ide-terminal-strip {
  padding: 0.75rem 1.25rem;
  background: #f1f3f5;
  font-family: monospace;
  font-size: 0.8rem;
  color: rgba(26,26,46,0.6);
  border-top: 1px solid rgba(0,0,0,0.06);
}
.dark .ide-terminal-strip { background: #0d1117; color: rgba(230,237,243,0.5); border-color: rgba(255,255,255,0.06); }
.term-tag { background: rgba(79,140,255,0.12); color: #4f8cff; padding: 0.1rem 0.4rem; border-radius: 4px; font-weight: 700; }

.ide-section__content { text-align: left; }

.ide-section__title {
  font-size: 3rem;
  font-weight: 800;
  color: #1a1a2e;
  margin-bottom: 2rem;
  line-height: 1.2;
}
.dark .ide-section__title { color: #e6edf3; }

.ide-section__star {
  display: inline-block;
  margin-left: 0.5rem;
  font-size: 2rem;
  animation: twinkleStar 4s ease-in-out infinite;
}

.ide-section__desc {
  font-size: 1.125rem;
  line-height: 1.8;
  color: rgba(26,26,46,0.7);
  margin-bottom: 3rem;
}
.dark .ide-section__desc { color: rgba(230,237,243,0.55); }

.ide-section__platforms { display: flex; gap: 2rem; flex-wrap: wrap; }

.platform-item {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 0.75rem 1.25rem;
  border-radius: 0.75rem;
  transition: all 0.3s cubic-bezier(0.16,1,0.3,1);
  cursor: default;
  background: transparent;
}
.platform-item:hover { background: #fff; transform: translateY(-4px); box-shadow: 0 12px 24px rgba(0,0,0,0.08); }
.dark .platform-item:hover { background: #1c2128; }
.platform-logo { color: rgba(26,26,46,0.5); }
.dark .platform-logo { color: rgba(230,237,243,0.4); }
.platform-item span { font-size: 1.125rem; font-weight: 500; color: #1a1a2e; }
.dark .platform-item span { color: #e6edf3; }

.ide-section__quicktip {
  display: flex;
  align-items: flex-start;
  gap: 0.75rem;
  margin-top: 2rem;
  padding: 1rem 1.5rem;
  background: rgba(79, 140, 255, 0.07);
  border: 1px solid rgba(79, 140, 255, 0.18);
  border-radius: 0.75rem;
}
.dark .ide-section__quicktip { background: rgba(79,140,255,0.1); border-color: rgba(79,140,255,0.2); }
.ide-section__quicktip-icon { font-size: 1.1rem; flex-shrink: 0; line-height: 1.7; }
.ide-section__quicktip p { font-size: 0.9375rem; line-height: 1.7; color: rgba(26,26,46,0.75); margin: 0; }
.dark .ide-section__quicktip p { color: rgba(230,237,243,0.7); }
.ide-section__footer-subtitle { text-align: center; font-size: 1.125rem; color: rgba(26,26,46,0.6); }
.dark .ide-section__footer-subtitle { color: rgba(230,237,243,0.5); }

/* ── PRICING TABLE ───────────────────────────────────────────────────────── */
.pricing-section { padding: 0.5rem 0 4rem; background-color: #f8f9fa; }
.dark .pricing-section { background-color: #161b22; }

.pricing-section__container { width: min(100% - 4rem, 1200px); margin: 0 auto; }

.pricing-section__title { text-align: center; font-size: 3rem; font-weight: 700; color: #1a1a2e; margin-bottom: 1rem; }
.dark .pricing-section__title { color: #e6edf3; }
.pricing-section__star { display: inline-block; margin-left: 0.5rem; font-size: 2rem; }

.pricing-section__subtitle { text-align: center; font-size: 0.9375rem; color: rgba(26,26,46,0.55); margin-bottom: 3rem; line-height: 1.6; }
.dark .pricing-section__subtitle { color: rgba(230,237,243,0.45); }

.table-wrapper {
  background: #f1f3f5;
  border-radius: 0.75rem;
  overflow: hidden;
  margin-bottom: 5rem;
}
.dark .table-wrapper { background: #1c2128; }

.pricing-table { width: 100%; border-collapse: collapse; text-align: left; }
.pricing-table th {
  padding: 1.25rem 1.5rem;
  font-size: 0.875rem;
  font-weight: 600;
  color: #1a1a2e;
  border-bottom: 1px solid rgba(0,0,0,0.05);
  background: #e8eaed;
}
.dark .pricing-table th { color: #e6edf3; background: #161b22; border-color: rgba(255,255,255,0.06); }
.pricing-table td {
  padding: 1.25rem 1.5rem;
  font-size: 0.875rem;
  color: rgba(26,26,46,0.8);
  border-bottom: 1px solid rgba(0,0,0,0.05);
}
.dark .pricing-table td { color: rgba(230,237,243,0.7); border-color: rgba(255,255,255,0.04); }
.pricing-table tr:last-child td { border-bottom: none; }
.pricing-table tr:hover { background: rgba(255,255,255,0.4); }
.dark .pricing-table tr:hover { background: rgba(255,255,255,0.03); }

.pricing-logo { width: 1rem; height: 1rem; object-fit: contain; vertical-align: middle; margin-right: 0.5rem; }
.cell-name { font-weight: 500; color: #1a1a2e; display: flex; align-items: center; }
.dark .cell-name { color: #e6edf3; }

.discount-badge {
  display: inline-flex;
  padding: 0.2rem 0.5rem;
  background: #dcfce7;
  color: #166534;
  border-radius: 0.375rem;
  font-size: 0.75rem;
  font-weight: 700;
}

.type-badge {
  display: inline-flex;
  padding: 0.2rem 0.6rem;
  border-radius: 0.375rem;
  font-size: 0.75rem;
  font-weight: 600;
  white-space: nowrap;
}
.type-badge--blue   { background: #dbeafe; color: #1d4ed8; }
.type-badge--orange { background: #ffedd5; color: #c2410c; }
.type-badge--purple { background: #f3e8ff; color: #7e22ce; }
.type-badge--green  { background: #dcfce7; color: #166534; }

.cell-rate { font-weight: 700; color: #374151; font-size: 0.875rem; }
.dark .cell-rate { color: #e6edf3; }
.cell-official { color: rgba(26,26,46,0.5); font-size: 0.8125rem; }
.dark .cell-official { color: rgba(230,237,243,0.4); }

.proxy-yes { color: #16a34a; font-weight: 600; font-size: 0.875rem; }
.proxy-no  { color: #9ca3af; font-size: 0.875rem; }

.pricing-section__cta { display: flex; justify-content: center; margin-bottom: 4rem; }

.pricing-cta-btn {
  display: inline-flex;
  align-items: center; justify-content: center;
  height: 4rem;
  padding: 0 4rem;
  background: #000;
  color: #fff;
  border-radius: 999px;
  font-size: 1.125rem;
  font-weight: 700;
  text-decoration: none;
  transition: all 0.2s ease;
}
.dark .pricing-cta-btn { background: #e6edf3; color: #0d1117; }
.pricing-cta-btn:hover { transform: scale(1.02); box-shadow: 0 12px 30px rgba(0,0,0,0.2); }

.pricing-note {
  text-align: center;
  padding: 1rem 2rem;
  background: rgba(22, 163, 74, 0.06);
  border: 1px solid rgba(22, 163, 74, 0.15);
  border-radius: 0.75rem;
  font-size: 0.9375rem;
  color: rgba(26,26,46,0.7);
  line-height: 1.7;
}
.dark .pricing-note { background: rgba(22,163,74,0.08); border-color: rgba(22,163,74,0.2); color: rgba(230,237,243,0.65); }

/* ── STATS ───────────────────────────────────────────────────────────────── */
.stats-section {
  position: relative;
  padding: 6rem 0 8rem;
  background:
    radial-gradient(ellipse 80% 60% at 20% 50%, rgba(79,140,255,0.06) 0%, transparent 60%),
    radial-gradient(ellipse 60% 50% at 80% 50%, rgba(99,102,241,0.05) 0%, transparent 55%),
    #f8f9fa;
}
.dark .stats-section { background: radial-gradient(ellipse 80% 60% at 20% 50%, rgba(79,140,255,0.08) 0%, transparent 60%), radial-gradient(ellipse 60% 50% at 80% 50%, rgba(99,102,241,0.06) 0%, transparent 55%), #0d1117; }

.stats-section__container { width: min(100% - 4rem, 1200px); margin: 0 auto; }

.stats-section__title { text-align: center; font-size: 3rem; font-weight: 700; color: #1a1a2e; margin-bottom: 1rem; }
.dark .stats-section__title { color: #e6edf3; }
.stats-section__star { display: inline-block; margin-left: 0.5rem; font-size: 2rem; animation: twinkleStar 4s ease-in-out infinite; }

.stats-section__subtitle { text-align: center; font-size: 1.125rem; color: rgba(26,26,46,0.6); margin-bottom: 5rem; }
.dark .stats-section__subtitle { color: rgba(230,237,243,0.5); }

.stats-list { display: flex; justify-content: center; align-items: center; gap: 0; }

.stat-item { display: flex; flex-direction: column; align-items: center; gap: 0.5rem; padding: 0 4rem; }

.stat-num {
  font-size: 4rem;
  font-weight: 800;
  color: #1a1a2e;
  letter-spacing: -0.02em;
  line-height: 1;
}
.stat-num--xl { font-size: 5.5rem; }
.dark .stat-num { color: #e6edf3; }

.stat-plus { font-size: 2rem; font-weight: 800; color: #4f8cff; vertical-align: super; line-height: 0; }
.stat-unit { font-size: 1.8rem; font-weight: 800; color: #4f8cff; }
.stat-label { font-size: 1rem; font-weight: 500; color: rgba(26,26,46,0.55); }
.dark .stat-label { color: rgba(230,237,243,0.45); }
.stat-divider { width: 1px; height: 60px; background: rgba(26,26,46,0.1); flex-shrink: 0; }
.dark .stat-divider { background: rgba(255,255,255,0.1); }

/* ── FOOTER ──────────────────────────────────────────────────────────────── */
.home-footer { padding: 4rem 0 2rem; background: #f8fafc; border-top: 1px solid rgba(0,0,0,0.06); }
.dark .home-footer { background: #161b22; border-color: rgba(255,255,255,0.06); }

.home-footer__container { width: min(100% - 2rem, 1280px); margin: 0 auto; }

.home-footer__bottom {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 1rem;
  padding: 1.5rem 0;
}
.home-footer__bottom p { font-size: 0.75rem; color: #9ca3af; }

.home-footer__links {
  display: flex;
  align-items: center;
  gap: 1.5rem;
}
.home-footer__links a {
  font-size: 0.8125rem;
  color: #6b7280;
  text-decoration: none;
  transition: color 0.2s ease;
}
.home-footer__links a:hover { color: #4f8cff; }
.home-footer__links span { font-size: 0.8125rem; color: #9ca3af; }

/* ── RESPONSIVE ──────────────────────────────────────────────────────────── */
@media (max-width: 1024px) {
  .ide-section__grid { grid-template-columns: 1fr; gap: 4rem; }
  .ide-section__title { font-size: 2.5rem; }
}

@media (max-width: 968px) {
  .transfer-experience__inner { flex-direction: column; padding: 2rem; }
  .transfer-experience__divider { width: 100%; height: 1px; margin: 2rem 0; }
  .why-choose__grid { grid-template-columns: repeat(2, 1fr); }
  .why-choose__card--logo { display: none; }
}

@media (max-width: 768px) {
  .hero-section__title { flex-direction: column; gap: 1rem; }
  .hero-section__main { font-size: 2.5rem; }
  .hero-section__kicker { font-size: 1.5rem; padding: 0.4rem 1.25rem; }
  .home-header__menu { display: none; }
  .model-section__grid { grid-template-columns: 1fr; }
  .stats-list { flex-wrap: wrap; gap: 2rem; }
  .stat-item { padding: 0 2rem; }
  .pricing-section__title { font-size: 2rem; }
  .table-wrapper { overflow-x: auto; }
  .pricing-table { min-width: 700px; }
}

@media (max-width: 640px) {
  .why-choose__grid { grid-template-columns: 1fr; }
  .home-footer__bottom { flex-direction: column; gap: 0.5rem; text-align: center; }
}

@media (prefers-reduced-motion: reduce) {
  .hero-section__bg { animation: none !important; }
  .mirror-reveal { transition: opacity 0.3s ease !important; filter: none !important; transform: none !important; }
  .ide-code-panel { animation: none !important; }
  .transfer-experience__star, .model-section__star, .ide-section__star, .stats-section__star { animation: none !important; }
}
</style>
