<template>
  <div v-if="isHomeContentUrl" class="min-h-screen">
    <iframe
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
  </div>

  <!-- homeContent is an admin-only setting; v-html is intentional for this custom-content mode -->
  <div v-else-if="homeContent" v-html="homeContent"></div>

  <div v-else class="subtoken-home" :class="{ 'is-dark': isDark }">
    <div class="home-particles" aria-hidden="true">
      <span v-for="particle in particles" :key="particle" :class="`particle particle-${particle}`">
        <svg v-if="particle % 3 === 0" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="w-full h-full">
          <path d="M6 1v10M1 6h10" />
        </svg>
        <svg v-else-if="particle % 3 === 1" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" class="w-full h-full">
          <path d="M2 2l8 8M10 2L2 10" />
        </svg>
        <svg v-else viewBox="0 0 12 12" fill="currentColor" class="w-full h-full">
          <path d="M6 1L7.5 4.5L11 6L7.5 7.5L6 11L4.5 7.5L1 6L4.5 4.5Z" />
        </svg>
      </span>
    </div>

    <header class="home-header">
      <router-link class="home-brand" to="/home" aria-label="SUBTOKEN Home">
        <span class="brand-mark" aria-hidden="true">
          <img src="/subtoken-logo.png" alt="SUBTOKEN Logo" class="brand-mark-img" />
        </span>
        <span class="brand-copy">
          <span class="brand-name">SUBTOKEN</span>
          <span class="brand-subtitle">{{ copy.brandSubtitle }}</span>
        </span>
      </router-link>

      <nav class="home-nav" :aria-label="copy.navLabel">
        <a href="#about" @click.prevent="switchToTab('about')">{{ copy.nav.about }}</a>
        <a href="#points" @click.prevent="switchToTab('events')">{{ copy.nav.points }}</a>
        <a v-if="docUrl" :href="docUrl">{{ copy.nav.docs }}</a>
        <router-link to="/key-usage">{{ copy.nav.usage }}</router-link>
      </nav>

      <div class="home-actions">
        <div class="segmented" :aria-label="copy.languageLabel">
          <button
            type="button"
            :class="{ active: homeLocale === 'zh' }"
            :aria-pressed="homeLocale === 'zh'"
            @click="setHomeLocale('zh')"
          >
            中文
          </button>
          <button
            type="button"
            :class="{ active: homeLocale === 'en' }"
            :aria-pressed="homeLocale === 'en'"
            @click="setHomeLocale('en')"
          >
            EN
          </button>
        </div>

        <button type="button" class="theme-toggle" @click="toggleTheme">
          <span>{{ isDark ? copy.theme.light : copy.theme.dark }}</span>
        </button>

        <router-link class="login-button" :to="isAuthenticated ? dashboardPath : '/login'">
          {{ isAuthenticated ? copy.dashboard : copy.login }}
        </router-link>
      </div>
    </header>

    <main>
      <section class="hero-section" aria-labelledby="home-title">
        <div class="hero-copy">
          <p class="eyebrow">{{ copy.eyebrow }}</p>
          <h1 id="home-title">
            {{ copy.heroTitle }}
          </h1>
          <p class="hero-lede">
            {{ copy.heroLead }}
          </p>
          <p class="hero-note">
            {{ copy.heroNote }}
          </p>

          <div class="hero-cta">
            <router-link class="brutal-button primary" :to="isAuthenticated ? dashboardPath : '/login'">
              {{ isAuthenticated ? copy.goDashboard : copy.primaryCta }}
            </router-link>
            <a v-if="docUrl" class="brutal-button secondary" :href="docUrl">
              {{ copy.secondaryCta }}
            </a>
            <router-link class="brutal-button accent" to="/spin">
              {{ copy.spinCta }}
            </router-link>
          </div>
        </div>

        <div class="hero-board" aria-label="AI support map">
          <div class="board-orbit">
            <span class="orbit-line"></span>
            <span class="orbit-line orbit-line--two"></span>
            <span class="orbit-chip chip-one">{{ copy.orbit.items[0] }}</span>
            <span class="orbit-chip chip-two">{{ copy.orbit.items[1] }}</span>
            <span class="orbit-chip chip-three">{{ copy.orbit.items[2] }}</span>
            <span class="orbit-chip chip-four">{{ copy.orbit.items[3] }}</span>
            <div class="orbit-core">
              <strong>{{ copy.orbit.title }}</strong>
              <span>{{ copy.orbit.subtitle }}</span>
            </div>
          </div>
          <div class="board-caption">{{ copy.orbit.caption }}</div>
        </div>
      </section>

      <!-- Details Hub Segmented Tab Selector -->
      <section id="details-hub" ref="hubRef" class="details-hub-section" aria-label="Interactive Hub">
        <div class="hub-tabs-container">
          <div class="hub-tabs" role="tablist">
            <button
              type="button"
              role="tab"
              :aria-selected="activeTab === 'about'"
              :class="{ active: activeTab === 'about' }"
              @click="activeTab = 'about'"
            >
              {{ copy.hubTabs.about }}
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="activeTab === 'events'"
              :class="{ active: activeTab === 'events' }"
              @click="activeTab = 'events'"
            >
              {{ copy.hubTabs.events }}
            </button>
            <button
              type="button"
              role="tab"
              :aria-selected="activeTab === 'faq'"
              :class="{ active: activeTab === 'faq' }"
              @click="activeTab = 'faq'"
            >
              {{ copy.hubTabs.faq }}
            </button>
          </div>
        </div>

        <!-- Transition wrapper for smooth Apple-style softened transitions -->
        <div class="hub-tab-content">
          <Transition name="tab-slide" mode="out-in">
            <div :key="activeTab" class="tab-panel">

              <!-- Tab 1: About Us -->
              <div v-if="activeTab === 'about'" class="space-y-12">
                <!-- Polaroid Info Cards Grid -->
                <div class="card-grid-wrapper">
                  <div class="card-grid" :aria-label="copy.cardsLabel">
                    <article v-for="(card, index) in copy.cards" :key="card.title" class="info-card polaroid-card" :class="`polaroid-${index}`">
                      <div class="washi-tape"></div>
                      <p class="mini-label">{{ card.label }}</p>
                      <h2>{{ card.title }}</h2>
                      <p>{{ card.description }}</p>
                    </article>
                  </div>
                </div>

                <!-- Lined Notebook Mission Section -->
                <div class="mission-section" aria-labelledby="mission-title">
                  <div class="mission-copy">
                    <p class="eyebrow">{{ copy.mission.kicker }}</p>
                    <h2 id="mission-title">{{ copy.mission.title }}</h2>
                    <p>{{ copy.mission.body }}</p>
                  </div>
                  <div class="mission-list notebook-paper" role="list">
                    <div v-for="(item, index) in copy.mission.items" :key="item" role="listitem">
                      <span>{{ index + 1 }}</span>
                      <p>{{ item }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <!-- Tab 2: Events & Credits -->
              <div v-else-if="activeTab === 'events'" class="space-y-12">
                <div class="supply-strip" aria-labelledby="supply-title">
                  <div class="supply-strip-content">
                    <p class="sticker">SUBTOKEN EVENT</p>
                    <h2 id="supply-title">{{ copy.event.title }}</h2>
                    <p>{{ copy.event.description }}</p>
                  </div>
                  <router-link class="event-wheel" to="/spin" aria-label="Spin event">
                    <span></span>
                  </router-link>
                  <router-link class="brutal-button secondary" to="/spin">
                    {{ copy.event.button }}
                  </router-link>
                </div>
              </div>

              <!-- Tab 3: FAQ & Support -->
              <div v-else-if="activeTab === 'faq'" class="space-y-12">
                <div class="disclosure-section">
                  <div class="section-heading">
                    <div>
                      <h2 id="details-title">{{ copy.detailsTitle }}</h2>
                      <p>{{ copy.detailsIntro }}</p>
                    </div>
                  </div>
                  <div class="details-stack">
                    <details v-for="(item, index) in copy.details" :key="item.title" :open="index === 0">
                      <summary>
                        <span>{{ String(index + 1).padStart(2, '0') }}</span>
                        {{ item.title }}
                      </summary>
                      <p>{{ item.body }}</p>
                    </details>
                  </div>
                </div>

                <!-- QQ Support Card -->
                <div class="community-section" aria-labelledby="community-title">
                  <div class="community-copy">
                    <p class="sticker">{{ copy.community.kicker }}</p>
                    <h2 id="community-title">{{ copy.community.title }}</h2>
                    <p>{{ copy.community.body }}</p>
                  </div>
                  <div class="qr-placeholder polaroid-card polaroid-qr">
                    <div class="washi-tape"></div>
                    <img src="/subtoken-qq-group-qr.png" :alt="copy.community.alt" class="qr-image" />
                    <small>{{ copy.community.qr }}</small>
                  </div>
                </div>
              </div>

            </div>
          </Transition>
        </div>
      </section>

      <!-- Final CTA Section with washi tape details -->
      <section class="final-cta polaroid-card">
        <div class="washi-tape"></div>
        <div>
          <h2>{{ copy.final.title }}</h2>
          <p>{{ copy.final.body }}</p>
        </div>
        <router-link class="brutal-button primary" :to="isAuthenticated ? dashboardPath : '/login'">
          {{ copy.final.button }}
        </router-link>
      </section>
    </main>

    <footer class="home-footer">
      <p>Copyright © {{ currentYear }} {{ displayName }}. All Rights Reserved.</p>
      <p>A project of 3E SMART EDUCATION INC</p>
      <p>1911 KRESSON RD, CHERRY HILL, NJ 08003</p>
    </footer>
  </div>

  <Teleport to="body">
    <Transition name="mainland-notice-fade">
      <div
        v-if="showMainlandChinaNotice"
        data-test="mainland-china-notice"
        class="fixed inset-0 z-[1000] flex items-center justify-center bg-black/55 px-4 py-6 backdrop-blur-sm"
        role="dialog"
        aria-modal="true"
        aria-labelledby="mainland-china-notice-title"
      >
        <section class="w-full max-w-md rounded-2xl border border-gray-200 bg-white p-6 text-gray-900 shadow-2xl dark:border-dark-700 dark:bg-dark-900 dark:text-white">
          <p class="mb-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary-600 dark:text-primary-400">
            访问提示
          </p>
          <h2 id="mainland-china-notice-title" class="text-2xl font-bold">
            中国大陆地区暂不支持
          </h2>
          <p class="mt-3 text-sm leading-7 text-gray-600 dark:text-dark-300">
            检测到当前访问 IP 可能位于中国大陆地区。SUBTOKEN 目前暂不向中国大陆地区提供服务，请不要继续使用当前服务。
          </p>
          <button
            data-test="mainland-china-leave"
            type="button"
            class="mt-6 inline-flex w-full items-center justify-center rounded-xl bg-gray-900 px-4 py-3 text-sm font-semibold text-white transition-colors hover:bg-gray-800 dark:bg-white dark:text-gray-900 dark:hover:bg-gray-100"
            @click="closeMainlandChinaNotice"
          >
            离开
          </button>
        </section>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import { detectMainlandChinaAccess } from '@/utils/mainlandChinaAccess'

type HomeLocale = 'zh' | 'en'

const HOME_LOCALE_KEY = 'sub2api_home_locale'
const particles = Array.from({ length: 18 }, (_, index) => index + 1)

const homeCopy = {
  zh: {
    brandSubtitle: '公益 API · 共创项目',
    hubTabs: {
      about: '我们是谁',
      events: '活动 & 积分',
      faq: '常见问题'
    },
    navLabel: '主页导航',
    nav: { about: '我们是谁', points: '积分', docs: '文档', usage: '用量' },
    languageLabel: '语言切换',
    theme: { light: '明亮', dark: '深色' },
    dashboard: '控制台',
    login: '登录',
    goDashboard: '进入控制台',
    eyebrow: 'Hi，我们是 SUBTOKEN',
    heroTitle: '让 AI 的便利，先到你手边',
    heroLead:
      '非盈利导向的公益 AI 共创项目。我们想把门槛降下来，让学生、创作者和正在准备下一步的人，都能先体验新技术带来的便利。',
    heroNote: '小组作业、论文思路、竞赛演讲、求职简历和复习资料，都可以让它先帮你搭把手。',
    primaryCta: '开始体验',
    secondaryCta: '看看怎么接入',
    spinCta: '拼手气大转盘',
    orbit: {
      title: 'AI 小补给站',
      subtitle: '灵感、资料、表达，都来一点点',
      caption: '赞助让服务继续转起来',
      items: ['小组作业', '论文思路', '竞赛演讲', '求职简历']
    },
    event: {
      title: '拼手气大转盘',
      description: '登录后参与，每个账号仅可转一次。活动奖品与有效期由管理员后台统一配置。',
      button: '进入转盘活动'
    },
    cardsLabel: 'SUBTOKEN 核心说明',
    cards: [
      {
        label: '公益共创',
        title: '先让更多人用起来',
        description: '我们更像一群把 AI 便利分享出去的人。先体验、先理解、先用到真实学习里，这件事就很值得。'
      },
      {
        label: '积分消耗',
        title: '用多少，看得清',
        description: '调用会消耗积分，记录清楚、配额清楚，个人和团队都能知道资源花在哪里。'
      },
      {
        label: '赞助去向',
        title: '一起把服务养起来',
        description: '赞助主要用于服务器、上游资源、稳定体验、风控和日常维护。感谢每一份支持。'
      }
    ],
    mission: {
      kicker: '我们想做什么',
      title: '把遥远的新技术，变成随手可用的小工具',
      body:
        'SUBTOKEN 想陪你处理那些真实又琐碎的时刻：讨论没头绪、段落卡住、稿子要润色、简历不知道怎么写。AI 不是替你生活，而是帮你把第一步迈出去。',
      items: [
        '面向学生和普通用户，尽量降低接入、理解和试错成本。',
        '赞助用于维持服务运行、扩容资源和处理必要维护，不做夸张承诺。',
        '鼓励负责任使用 AI。可以辅助学习，但不要拿去代写、作弊或违反规则。'
      ]
    },
    detailsTitle: '想多了解一点？',
    detailsIntro: '先看使命、积分和赞助去向；需要更多背景时，再逐项展开。',
    details: [
      {
        title: '为什么要做这个公益 API',
        body:
          '很多同学不是不想用 AI，而是被门槛、成本和接入方式挡住了。我们想把入口整理得简单一点，让更多人能先体验、先理解、先用起来。'
      },
      {
        title: '赞助会用在哪里',
        body:
          '赞助主要覆盖服务器、上游资源、线路稳定性、风控和日常维护。每一份支持都会回到服务本身。'
      },
      {
        title: '积分怎么消耗',
        body:
          '不同模型和接口会有不同消耗。我们尽量让记录清楚、余额清楚、配额清楚，避免资源被不透明地消耗。'
      },
      {
        title: '适合哪些场景',
        body:
          '适合学习辅助、资料整理、表达润色、竞赛准备、求职材料和轻量开发验证。它不是替代判断，而是提供起步动力。'
      }
    ],
    community: {
      kicker: '加入 QQ 交流群',
      title: '获取公告、使用帮助和维护通知',
      body: '扫码加入 SUBTOKEN 交流群。我们会在这里同步服务状态、活动信息和常见问题。',
      qr: '交流群',
      alt: 'SUBTOKEN QQ 交流群二维码'
    },
    final: {
      title: '一起让 AI 的便利，被更多人体验到',
      body: '如果你认同这个方向，欢迎注册体验，也欢迎用赞助支持服务继续运行。我们慢慢来，把它做好。',
      button: '开始体验'
    }
  },
  en: {
    brandSubtitle: 'Public API · Community Project',
    hubTabs: {
      about: 'About Us',
      events: 'Events & Credits',
      faq: 'FAQ & Support'
    },
    navLabel: 'Home navigation',
    nav: { about: 'About', points: 'Credits', docs: 'Docs', usage: 'Usage' },
    languageLabel: 'Language switcher',
    theme: { light: 'Light', dark: 'Dark' },
    dashboard: 'Dashboard',
    login: 'Log in',
    goDashboard: 'Open dashboard',
    eyebrow: 'Hi, we are SUBTOKEN',
    heroTitle: 'AI access, closer to everyday work',
    heroLead:
      'A public-minded AI access project. We lower the first step so students, makers, and people preparing for what comes next can try new tools without fighting the setup first.',
    heroNote: 'Group work, paper outlines, presentation drafts, resumes, review notes: let AI help you start before the page stays blank.',
    primaryCta: 'Start now',
    secondaryCta: 'Read the docs',
    spinCta: 'Try the spin event',
    orbit: {
      title: 'AI supply desk',
      subtitle: 'Ideas, sources, wording, one small push',
      caption: 'Sponsorship keeps the service running',
      items: ['Group work', 'Paper ideas', 'Pitch prep', 'Resumes']
    },
    event: {
      title: 'Spin event',
      description: 'Sign in to participate. Each account can spin once. Rewards and validity are managed by the admin panel.',
      button: 'Enter event'
    },
    cardsLabel: 'SUBTOKEN essentials',
    cards: [
      {
        label: 'Community',
        title: 'Let more people try it first',
        description: 'We are a group sharing access to practical AI. Trying it, understanding it, and using it in real study work already matters.'
      },
      {
        label: 'Credits',
        title: 'Usage should be visible',
        description: 'Calls consume credits. Records, balances, and quotas are kept clear so individuals and teams know where resources go.'
      },
      {
        label: 'Sponsorship',
        title: 'Keep the service alive',
        description: 'Support helps with servers, upstream resources, reliability, risk controls, and daily maintenance.'
      }
    ],
    mission: {
      kicker: 'What we are building',
      title: 'Turn distant technology into a useful everyday tool',
      body:
        'SUBTOKEN is meant for the real in-between moments: a group discussion has no shape yet, a paragraph is stuck, a speech needs polish, a resume needs a first pass. AI should not live your life for you. It should help you make the first move.',
      items: [
        'Serve students and regular users with a lower cost to access, understand, and experiment.',
        'Use sponsorship for operations, capacity, and maintenance without making exaggerated promises.',
        'Encourage responsible AI use for learning support, not ghostwriting, cheating, or rule-breaking.'
      ]
    },
    detailsTitle: 'Want the deeper version?',
    detailsIntro: 'The essentials stay visible. More context opens only when you need it.',
    details: [
      {
        title: 'Why build a public API like this?',
        body:
          'Many people are interested in AI but blocked by cost, setup, or scattered accounts. We want the entrance to feel simpler so more people can try it first.'
      },
      {
        title: 'Where does sponsorship go?',
        body:
          'It mainly supports servers, upstream resources, reliability work, risk controls, and daily maintenance. Support returns to the service itself.'
      },
      {
        title: 'How do credits work?',
        body:
          'Different models and endpoints can cost different amounts. We keep balance, records, and quotas visible so usage does not feel hidden.'
      },
      {
        title: 'What is it good for?',
        body:
          'Learning support, research organization, writing polish, competition preparation, job materials, and lightweight development tests.'
      }
    ],
    community: {
      kicker: 'Join the QQ group',
      title: 'Announcements, help, and maintenance notes',
      body: 'Join the SUBTOKEN group for service status, event updates, and common questions.',
      qr: 'QQ group',
      alt: 'SUBTOKEN QQ Group QR Code'
    },
    final: {
      title: 'Help more people experience useful AI',
      body: 'If this direction makes sense to you, try it, share feedback, or support the service so it can keep running.',
      button: 'Start now'
    }
  }
} as const

function getInitialHomeLocale(): HomeLocale {
  if (typeof localStorage !== 'undefined') {
    const saved = localStorage.getItem(HOME_LOCALE_KEY)
    if (saved === 'zh' || saved === 'en') {
      return saved
    }
  }

  if (typeof navigator !== 'undefined' && navigator.language.toLowerCase().startsWith('zh')) {
    return 'zh'
  }

  return 'en'
}

const authStore = useAuthStore()
const appStore = useAppStore()
const homeLocale = ref<HomeLocale>(getInitialHomeLocale())
const isDark = ref(document.documentElement.classList.contains('dark'))
const showMainlandChinaNotice = ref(false)

const copy = computed(() => homeCopy[homeLocale.value])
const displayName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'SUBTOKEN')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '/doc')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const currentYear = computed(() => new Date().getFullYear())

const activeTab = ref<'about' | 'events' | 'faq'>('about')

function switchToTab(tab: 'about' | 'events' | 'faq') {
  activeTab.value = tab
  const hubElement = document.getElementById('details-hub')
  if (hubElement) {
    hubElement.scrollIntoView({ behavior: 'smooth', block: 'start' })
  }
}

function setHomeLocale(locale: HomeLocale) {
  homeLocale.value = locale
  localStorage.setItem(HOME_LOCALE_KEY, locale)
  document.documentElement.setAttribute('lang', locale === 'zh' ? 'zh-CN' : 'en')
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

function closeMainlandChinaNotice() {
  showMainlandChinaNotice.value = false
}

async function checkMainlandChinaAccess() {
  showMainlandChinaNotice.value = await detectMainlandChinaAccess()
}

onMounted(() => {
  initTheme()
  setHomeLocale(homeLocale.value)
  authStore.checkAuth()

  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }

  checkMainlandChinaAccess()
})
</script>

<style scoped>
.subtoken-home {
  --home-bg: #f4f0e6;
  --home-paper: #fffdf6;
  --home-ink: #18211b;
  --home-muted: #5f675d;
  --home-border: #172018;
  --home-shadow: #172018;
  --home-mint: #cfe0d1;
  --home-olive: #769878;
  --home-gold: #d8bf62;
  --home-clay: #d98f63;
  --home-blue: #9ebdd2;
  min-height: 100vh;
  overflow: hidden;
  background:
    linear-gradient(90deg, rgba(24, 33, 27, 0.045) 1px, transparent 1px),
    linear-gradient(rgba(24, 33, 27, 0.04) 1px, transparent 1px),
    radial-gradient(circle at 12% 18%, rgba(118, 152, 120, 0.18), transparent 23rem),
    radial-gradient(circle at 85% 7%, rgba(216, 191, 98, 0.2), transparent 22rem),
    linear-gradient(180deg, var(--home-bg), #eef4ea 56%, #f6f1e7);
  background-size: 44px 44px, 44px 44px, auto, auto, auto;
  color: var(--home-ink);
  font-family:
    ui-sans-serif,
    system-ui,
    -apple-system,
    BlinkMacSystemFont,
    "Segoe UI",
    "PingFang SC",
    "Hiragino Sans GB",
    "Microsoft YaHei",
    sans-serif;
  letter-spacing: 0;
}

.subtoken-home.is-dark {
  --home-bg: #141814;
  --home-paper: #20261f;
  --home-ink: #f2eddf;
  --home-muted: #b7c1b2;
  --home-border: #edf0e4;
  --home-shadow: #050705;
  --home-mint: #27372d;
  --home-olive: #9ebd91;
  --home-gold: #d2b967;
  --home-clay: #d49266;
  --home-blue: #90b2ca;
  background:
    linear-gradient(90deg, rgba(237, 240, 228, 0.045) 1px, transparent 1px),
    linear-gradient(rgba(237, 240, 228, 0.035) 1px, transparent 1px),
    radial-gradient(circle at 15% 12%, rgba(158, 189, 145, 0.14), transparent 24rem),
    radial-gradient(circle at 82% 0%, rgba(210, 185, 103, 0.12), transparent 21rem),
    linear-gradient(180deg, var(--home-bg), #0f130f 68%, #171b17);
  background-size: 44px 44px, 44px 44px, auto, auto, auto;
}

.subtoken-home * {
  box-sizing: border-box;
}

.subtoken-home a {
  color: inherit;
}

.home-header {
  position: sticky;
  top: 0;
  z-index: 30;
  display: grid;
  grid-template-columns: minmax(230px, 1fr) auto auto;
  gap: 18px;
  align-items: center;
  width: min(1180px, calc(100% - 32px));
  margin: 0 auto;
  padding: 12px 0;
  border-bottom: 3px solid var(--home-border);
  background: color-mix(in srgb, var(--home-bg) 88%, transparent);
  backdrop-filter: blur(14px);
}

.home-brand,
.home-nav,
.home-actions,
.hero-cta,
.card-grid,
.section-heading,
.final-cta {
  display: flex;
}

.home-brand {
  align-items: center;
  gap: 12px;
  min-width: 0;
  text-decoration: none;
}

.brand-mark {
  position: relative;
  display: flex;
  flex: 0 0 auto;
  width: 58px;
  height: 46px;
  align-items: center;
  justify-content: center;
  border: 3px solid var(--home-border);
  border-radius: 8px;
  background: var(--home-paper);
  box-shadow: 5px 5px 0 var(--home-shadow);
  overflow: hidden;
  padding: 4px;
}

.brand-mark-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.subtoken-home.is-dark .brand-mark {
  background: #fdfaf2; /* Ensure light/paper background in dark mode for high contrast logo */
}

.brand-copy {
  display: grid;
  min-width: 0;
}

.brand-name {
  font-size: 1.65rem;
  font-weight: 950;
  line-height: 1;
}

.brand-subtitle {
  margin-top: 4px;
  color: var(--home-muted);
  font-size: 0.82rem;
  font-weight: 800;
}

.home-nav {
  align-items: center;
  gap: 8px;
}

.home-nav a,
.home-actions button,
.login-button,
.brutal-button {
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-paper);
  color: var(--home-ink);
  box-shadow: 3px 3px 0 var(--home-shadow);
  font-weight: 900;
  text-decoration: none;
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    background-color 180ms ease;
}

.home-nav a {
  padding: 8px 12px;
  font-size: 0.84rem;
}

.home-actions {
  align-items: center;
  gap: 8px;
}

.segmented {
  display: inline-flex;
  padding: 3px;
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-paper);
  box-shadow: 3px 3px 0 var(--home-shadow);
}

.segmented button {
  min-width: 44px;
  border: 0;
  border-radius: 999px;
  box-shadow: none;
  padding: 6px 9px;
  background: transparent;
  color: var(--home-muted);
  font-size: 0.76rem;
}

.segmented button.active {
  background: var(--home-gold);
  color: var(--home-ink);
}

.theme-toggle,
.login-button {
  padding: 8px 12px;
  font-size: 0.82rem;
}

.home-nav a:hover,
.home-actions button:hover,
.login-button:hover,
.brutal-button:hover,
.event-wheel:hover {
  transform: translate(-2px, -2px);
  box-shadow: 6px 6px 0 var(--home-shadow);
}

.hero-section,
.supply-strip,
.card-grid,
.mission-section,
.disclosure-section,
.community-section,
.final-cta,
.home-footer {
  width: min(1120px, calc(100% - 32px));
  margin-inline: auto;
}

.hero-section {
  display: grid;
  grid-template-columns: minmax(0, 1.02fr) minmax(320px, 0.9fr);
  gap: 64px;
  align-items: center;
  min-height: calc(100vh - 80px);
  padding: 78px 0 48px;
}

.eyebrow,
.sticker,
.mini-label {
  display: inline-flex;
  width: fit-content;
  align-items: center;
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-mint);
  box-shadow: 3px 3px 0 var(--home-shadow);
  color: var(--home-ink);
  font-size: 0.76rem;
  font-weight: 950;
  letter-spacing: 0.02em;
  padding: 7px 12px;
}

.hero-copy h1 {
  max-width: 720px;
  margin: 18px 0 18px;
  font-size: 5rem;
  font-weight: 950;
  letter-spacing: 0;
  line-height: 0.94;
}

.hero-lede {
  max-width: 650px;
  margin: 0;
  color: var(--home-muted);
  font-size: 1.25rem;
  font-weight: 760;
  line-height: 1.75;
}

.hero-note {
  max-width: 620px;
  margin: 22px 0 0;
  border: 3px solid var(--home-border);
  border-radius: 8px;
  background: var(--home-paper);
  box-shadow: 5px 5px 0 var(--home-shadow);
  color: var(--home-ink);
  font-weight: 900;
  line-height: 1.65;
  padding: 14px 18px;
}

.hero-cta {
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 28px;
}

.brutal-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 44px;
  padding: 11px 18px;
}

.brutal-button.primary {
  background: var(--home-olive);
  color: #08110a;
}

.brutal-button.secondary {
  background: var(--home-paper);
}

.brutal-button.accent {
  background: var(--home-gold);
  color: #14100a;
}

.hero-board {
  position: relative;
  display: grid;
  place-items: center;
  min-height: 440px;
}

.board-orbit {
  position: relative;
  width: min(420px, 88vw);
  aspect-ratio: 1.12;
  border: 4px solid var(--home-border);
  border-radius: 50%;
  background:
    radial-gradient(circle at center, var(--home-paper) 0 30%, transparent 31%),
    conic-gradient(from 18deg, var(--home-mint), var(--home-gold), #eef1e6, var(--home-blue), var(--home-mint));
  box-shadow: 9px 9px 0 var(--home-shadow);
  animation: float-board 7s ease-in-out infinite;
}

.orbit-line {
  position: absolute;
  inset: 32px;
  border: 2px dashed color-mix(in srgb, var(--home-border) 55%, transparent);
  border-radius: 50%;
}

.orbit-line--two {
  inset: 66px;
  transform: rotate(-12deg);
}

.orbit-core {
  position: absolute;
  inset: 50%;
  display: grid;
  width: 138px;
  height: 112px;
  place-items: center;
  transform: translate(-50%, -50%);
  border: 4px solid var(--home-border);
  border-radius: 50%;
  background: var(--home-paper);
  text-align: center;
  box-shadow: 5px 5px 0 var(--home-shadow);
}

.orbit-core strong {
  display: block;
  font-size: 1.12rem;
  font-weight: 950;
}

.orbit-core span,
.board-caption {
  color: var(--home-muted);
  font-size: 0.78rem;
  font-weight: 850;
}

.orbit-chip {
  position: absolute;
  border: 3px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-paper);
  box-shadow: 4px 4px 0 var(--home-shadow);
  font-weight: 950;
  padding: 12px 18px;
  animation: drift-chip 5.5s ease-in-out infinite;
}

.chip-one {
  left: 10%;
  top: 20%;
}

.chip-two {
  right: 7%;
  top: 19%;
  background: var(--home-gold);
}

.chip-three {
  bottom: 18%;
  left: 13%;
  background: var(--home-mint);
}

.chip-four {
  bottom: 20%;
  right: 9%;
}

.board-caption {
  margin-top: 18px;
  border: 3px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-olive);
  box-shadow: 4px 4px 0 var(--home-shadow);
  color: #071008;
  padding: 9px 16px;
}

.supply-strip {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 190px auto;
  gap: 22px;
  align-items: center;
  margin-top: 8px;
  border: 4px solid var(--home-border);
  border-radius: 8px;
  background: linear-gradient(110deg, var(--home-mint), var(--home-paper));
  box-shadow: 9px 9px 0 var(--home-shadow);
  padding: 30px;
}

.supply-strip h2,
.info-card h2,
.mission-copy h2,
.section-heading h2,
.community-section h2,
.final-cta h2 {
  margin: 10px 0;
  font-size: 2.4rem;
  font-weight: 950;
  letter-spacing: 0;
  line-height: 1;
}

.supply-strip p,
.info-card p,
.mission-copy p,
.section-heading p,
.community-section p,
.final-cta p,
.details-stack p {
  color: var(--home-muted);
  font-weight: 740;
  line-height: 1.75;
}

.event-wheel {
  position: relative;
  display: grid;
  width: 172px;
  height: 118px;
  place-items: center;
  border: 4px solid var(--home-border);
  border-radius: 8px;
  background: var(--home-paper);
  box-shadow: 6px 6px 0 var(--home-shadow);
}

.event-wheel span {
  width: 84px;
  aspect-ratio: 1;
  border: 4px solid var(--home-border);
  border-radius: 50%;
  background: conic-gradient(var(--home-olive) 0 34%, var(--home-gold) 34% 60%, var(--home-paper) 60% 82%, var(--home-mint) 82% 100%);
  animation: spin-wheel 12s linear infinite;
}

.card-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
  margin-top: 42px;
}

.info-card,
.mission-copy,
.mission-list,
.details-stack details,
.community-section,
.final-cta {
  border: 4px solid var(--home-border);
  border-radius: 8px;
  background: var(--home-paper);
  box-shadow: 7px 7px 0 var(--home-shadow);
}

.info-card {
  padding: 24px;
  transition:
    transform 180ms ease,
    box-shadow 180ms ease;
}

.info-card:nth-child(2) {
  background: var(--home-mint);
}

.info-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 10px 10px 0 var(--home-shadow);
}

.mission-section {
  display: grid;
  grid-template-columns: minmax(0, 1fr) minmax(300px, 0.9fr);
  gap: 18px;
  margin-top: 54px;
  padding-top: 34px;
  border-top: 2px solid color-mix(in srgb, var(--home-border) 26%, transparent);
}

.mission-copy,
.mission-list {
  padding: 26px;
}

.mission-list {
  display: grid;
}

.mission-list div {
  display: grid;
  grid-template-columns: 44px 1fr;
  gap: 14px;
  align-items: center;
  padding: 16px 0;
}

.mission-list div + div {
  border-top: 2px solid var(--home-border);
}

.mission-list span {
  display: grid;
  width: 34px;
  height: 34px;
  place-items: center;
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-mint);
  box-shadow: 3px 3px 0 var(--home-shadow);
  font-weight: 950;
}

.mission-list p {
  margin: 0;
  color: var(--home-muted);
  font-weight: 850;
  line-height: 1.65;
}

.disclosure-section {
  margin-top: 58px;
}

.section-heading {
  align-items: end;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 22px;
}

.section-heading p {
  max-width: 610px;
}

.details-stack {
  display: grid;
  gap: 14px;
}

.details-stack details {
  overflow: hidden;
}

.details-stack summary {
  display: grid;
  grid-template-columns: 52px 1fr;
  gap: 12px;
  align-items: center;
  cursor: pointer;
  padding: 18px 22px;
  font-size: 1rem;
  font-weight: 950;
  list-style: none;
}

.details-stack summary::-webkit-details-marker {
  display: none;
}

.details-stack summary::after {
  content: "+";
  justify-self: end;
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-paper);
}

.details-stack details[open] summary::after {
  content: "-";
}

.details-stack summary span {
  display: grid;
  width: 38px;
  height: 28px;
  place-items: center;
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-mint);
  font-size: 0.78rem;
}

.details-stack details p {
  margin: 0;
  border-top: 2px solid var(--home-border);
  padding: 18px 22px 22px 86px;
}

.community-section {
  display: grid;
  grid-template-columns: minmax(0, 1fr) 220px;
  gap: 24px;
  align-items: center;
  margin-top: 58px;
  padding: 28px;
}

.qr-placeholder {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 220px;
  border: 4px solid var(--home-border);
  border-radius: 8px;
  background: var(--home-paper);
  box-shadow: 6px 6px 0 var(--home-shadow);
  padding: 16px;
  gap: 12px;
}

.qr-image {
  width: min(160px, 100%);
  height: auto;
  border: 3px solid var(--home-border);
  border-radius: 6px;
  background: #ffffff;
  padding: 8px;
  box-shadow: 3px 3px 0 var(--home-shadow);
  transition: transform 180ms ease, box-shadow 180ms ease;
}

.qr-image:hover {
  transform: scale(1.04);
  box-shadow: 5px 5px 0 var(--home-shadow);
}

.qr-placeholder small {
  font-weight: 950;
}

.final-cta {
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  margin-top: 58px;
  padding: 28px;
  background: var(--home-mint);
}

.home-footer {
  margin-top: 48px;
  border-top: 3px solid var(--home-border);
  padding: 24px 0 34px;
  color: var(--home-muted);
  font-size: 0.78rem;
  font-weight: 850;
}

.home-footer p {
  margin: 4px 0;
}

.home-particles {
  pointer-events: none;
  position: fixed;
  inset: 0;
  z-index: 0;
  overflow: hidden;
}

.subtoken-home > header,
.subtoken-home main,
.subtoken-home footer {
  position: relative;
  z-index: 2;
}

.particle {
  position: absolute;
  width: 6px;
  height: 6px;
  border: 2px solid var(--home-border);
  border-radius: 999px;
  background: var(--home-gold);
  opacity: 0.42;
  animation: particle-drift 9s ease-in-out infinite;
}

.particle:nth-child(3n) {
  background: var(--home-blue);
}

.particle:nth-child(4n) {
  background: var(--home-clay);
}

.particle-1 { left: 6%; top: 18%; animation-delay: -1s; }
.particle-2 { left: 14%; top: 74%; animation-delay: -4s; }
.particle-3 { left: 22%; top: 34%; animation-delay: -7s; }
.particle-4 { left: 29%; top: 88%; animation-delay: -2s; }
.particle-5 { left: 38%; top: 14%; animation-delay: -6s; }
.particle-6 { left: 44%; top: 62%; animation-delay: -9s; }
.particle-7 { left: 53%; top: 24%; animation-delay: -3s; }
.particle-8 { left: 58%; top: 84%; animation-delay: -8s; }
.particle-9 { left: 66%; top: 46%; animation-delay: -5s; }
.particle-10 { left: 72%; top: 12%; animation-delay: -10s; }
.particle-11 { left: 78%; top: 68%; animation-delay: -4s; }
.particle-12 { left: 84%; top: 32%; animation-delay: -7s; }
.particle-13 { left: 90%; top: 78%; animation-delay: -1s; }
.particle-14 { left: 18%; top: 48%; animation-delay: -5s; }
.particle-15 { left: 34%; top: 52%; animation-delay: -3s; }
.particle-16 { left: 48%; top: 8%; animation-delay: -6s; }
.particle-17 { left: 63%; top: 72%; animation-delay: -2s; }
.particle-18 { left: 92%; top: 20%; animation-delay: -8s; }

@keyframes float-board {
  0%, 100% {
    transform: translateY(0) rotate(-1deg);
  }
  50% {
    transform: translateY(-12px) rotate(1deg);
  }
}

@keyframes drift-chip {
  0%, 100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-6px);
  }
}

@keyframes spin-wheel {
  to {
    transform: rotate(360deg);
  }
}

@keyframes particle-drift {
  0%, 100% {
    transform: translate3d(0, 0, 0);
  }
  50% {
    transform: translate3d(16px, -22px, 0);
  }
}

.mainland-notice-fade-enter-active,
.mainland-notice-fade-leave-active {
  transition: opacity 0.18s ease;
}

.mainland-notice-fade-enter-from,
.mainland-notice-fade-leave-to {
  opacity: 0;
}

@media (max-width: 980px) {
  .home-header {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .home-nav,
  .home-actions {
    flex-wrap: wrap;
  }

  .hero-section,
  .mission-section,
  .community-section,
  .final-cta {
    grid-template-columns: 1fr;
  }

  .hero-section {
    min-height: auto;
    gap: 42px;
    padding: 52px 0 36px;
  }

  .card-grid,
  .supply-strip {
    grid-template-columns: 1fr;
  }

  .supply-strip {
    padding: 22px;
  }

  .event-wheel {
    width: 100%;
  }
}

@media (max-width: 640px) {
  .home-header,
  .hero-section,
  .supply-strip,
  .card-grid,
  .mission-section,
  .disclosure-section,
  .community-section,
  .final-cta,
  .home-footer {
    width: min(100% - 22px, 1120px);
  }

  .brand-mark {
    width: 50px;
    height: 42px;
  }

  .brand-name {
    font-size: 1.3rem;
  }

  .home-nav a,
  .theme-toggle,
  .login-button {
    padding: 7px 10px;
  }

  .hero-copy h1 {
    font-size: 3.2rem;
  }

  .hero-lede {
    font-size: 1.08rem;
  }

  .supply-strip h2,
  .info-card h2,
  .mission-copy h2,
  .section-heading h2,
  .community-section h2,
  .final-cta h2 {
    font-size: 1.8rem;
  }

  .hero-board {
    min-height: 340px;
  }

  .orbit-chip {
    padding: 9px 12px;
    font-size: 0.78rem;
  }

  .details-stack summary {
    grid-template-columns: 44px 1fr;
    padding: 16px;
  }

  .details-stack details p {
    padding: 16px;
  }
}

/* ============================================================
   Interactive Details Hub, Washi Tape, and Polaroid Styles
   ============================================================ */
.details-hub-section {
  margin-top: 48px;
  padding-bottom: 12px;
  width: min(1120px, calc(100% - 32px));
  margin-inline: auto;
}

.hub-tabs-container {
  display: flex;
  justify-content: center;
  margin-bottom: 32px;
}

.hub-tabs {
  display: inline-flex;
  gap: 8px;
  padding: 6px;
  border: 3px solid var(--home-border);
  border-radius: 12px;
  background: var(--home-paper);
  box-shadow: 5px 5px 0 var(--home-shadow);
}

.hub-tabs button {
  padding: 10px 20px;
  border: 2px solid transparent;
  border-radius: 8px;
  background: transparent;
  color: var(--home-muted);
  font-size: 0.94rem;
  font-weight: 950;
  cursor: pointer;
  transition: all 0.22s cubic-bezier(0.16, 1, 0.3, 1);
}

.hub-tabs button:hover {
  color: var(--home-ink);
  background: color-mix(in srgb, var(--home-bg) 60%, transparent);
}

.hub-tabs button.active {
  background: var(--home-olive);
  border-color: var(--home-border);
  color: #08110a;
  box-shadow: 3px 3px 0 var(--home-shadow);
  transform: translate(-2px, -2px);
}

.hub-tab-content {
  position: relative;
  min-height: 380px;
}

/* Washi tape & Polaroid styling */
.polaroid-card {
  position: relative;
  background: var(--home-paper);
  border: 3px solid var(--home-border);
  box-shadow: 6px 6px 0 var(--home-shadow);
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.washi-tape {
  position: absolute;
  top: -10px;
  left: 50%;
  transform: translateX(-50%) rotate(-2deg);
  width: 70px;
  height: 20px;
  background: rgba(216, 191, 98, 0.65); /* translucent warm gold washi tape */
  border-left: 1px dashed rgba(24, 33, 27, 0.15);
  border-right: 1px dashed rgba(24, 33, 27, 0.15);
  box-shadow: 0 1px 3px rgba(0,0,0,0.05);
  z-index: 10;
  pointer-events: none;
}

.subtoken-home.is-dark .washi-tape {
  background: rgba(210, 185, 103, 0.45);
  border-left: 1px dashed rgba(237, 240, 228, 0.15);
  border-right: 1px dashed rgba(237, 240, 228, 0.15);
}

.polaroid-0 {
  transform: rotate(-0.6deg);
}
.polaroid-1 {
  transform: rotate(0.8deg);
}
.polaroid-2 {
  transform: rotate(-0.4deg);
}

.polaroid-card:hover {
  transform: translateY(-6px) rotate(0deg) !important;
  box-shadow: 10px 10px 0 var(--home-shadow);
}

/* Polaroid QR Card specific styles */
.polaroid-qr {
  padding: 16px 16px 24px 16px;
  border-radius: 4px;
}

/* Lined notebook paper */
.notebook-paper {
  background-color: var(--home-paper);
  background-image: linear-gradient(rgba(118, 152, 120, 0.12) 1px, transparent 1px);
  background-size: 100% 48px;
  border: 3px solid var(--home-border);
  border-radius: 8px;
  box-shadow: 6px 6px 0 var(--home-shadow);
  padding: 18px 24px !important;
}

.notebook-paper div {
  padding: 14px 0 !important;
  border-top: 1px dashed rgba(24, 33, 27, 0.15) !important;
}

.notebook-paper div:first-child {
  border-top: 0 !important;
}

.subtoken-home.is-dark .notebook-paper {
  background-image: linear-gradient(rgba(158, 189, 145, 0.08) 1px, transparent 1px);
}

/* Particle styling overrides */
.particle {
  display: inline-block;
  width: 12px;
  height: 12px;
  color: var(--home-gold);
  opacity: 0.45;
  border: 0 !important;
  background: transparent !important;
  box-shadow: none !important;
  animation: particle-drift 14s ease-in-out infinite;
}

.particle:nth-child(3n) {
  color: var(--home-blue);
}

.particle:nth-child(4n) {
  color: var(--home-clay);
}

/* Tab Transitions - smooth Apple-style softened transitions */
.tab-slide-enter-active,
.tab-slide-leave-active {
  transition: all 0.35s cubic-bezier(0.16, 1, 0.3, 1);
}

.tab-slide-enter-from {
  opacity: 0;
  transform: translateY(12px);
}

.tab-slide-leave-to {
  opacity: 0;
  transform: translateY(-12px);
}

.space-y-12 > * + * {
  margin-top: 3rem;
}

@media (max-width: 980px) {
  .details-hub-section {
    width: min(100% - 32px, 1120px);
  }
  .hub-tabs {
    width: 100%;
    display: flex;
  }
  .hub-tabs button {
    flex: 1;
    text-align: center;
    padding: 8px 12px;
    font-size: 0.88rem;
  }
}

@media (prefers-reduced-motion: reduce) {
  .subtoken-home *,
  .subtoken-home *::before,
  .subtoken-home *::after {
    animation-duration: 0.001ms !important;
    animation-iteration-count: 1 !important;
    scroll-behavior: auto !important;
    transition-duration: 0.001ms !important;
  }
  .particle {
    animation: none !important;
  }
}

@media (max-width: 480px) {
  .hero-copy h1 {
    font-size: 2.4rem;
    word-break: break-word;
  }
  .hero-note {
    font-size: 0.95rem;
    padding: 12px 14px;
    word-break: break-word;
  }
  .hero-board {
    min-height: 300px;
  }
  .board-orbit {
    width: min(260px, 65vw);
  }
  .orbit-chip {
    padding: 8px 12px;
    font-size: 0.72rem;
    white-space: normal;
    text-align: center;
  }
  .chip-one { left: 5%; top: 18%; }
  .chip-two { right: 2%; top: 15%; }
  .chip-three { bottom: 15%; left: 8%; }
  .chip-four { bottom: 18%; right: 5%; }
}
</style>
