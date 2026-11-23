<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useIntervalFn, usePreferredReducedMotion } from '@vueuse/core'
import { useAppStore, useAuthStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import SiteLogo from '@/components/common/SiteLogo.vue'
import { FeatureFlags, resolveFeatureFlag } from '@/utils/featureFlags'
import { resolveDocsUrl } from '@/utils/docsUrl'
import { resolveSiteBillingMode } from '@/utils/siteBillingMode'
import { sanitizeUrl } from '@/utils/url'

const { t, locale } = useI18n()
const appStore = useAppStore()
const authStore = useAuthStore()
const menuOpen = ref(false)
const menuButton = ref<HTMLButtonElement | null>(null)
const isDark = ref(document.documentElement.classList.contains('dark'))
const settings = computed(() => appStore.cachedPublicSettings)
const simpleMode = computed(() => authStore.isSimpleMode)
const siteName = computed(() => settings.value?.site_name || appStore.siteName || 'Sub2API')
const siteNameCharacters = computed(() => Array.from(siteName.value))
const reducedMotion = usePreferredReducedMotion()
const revealedCharacters = ref(0)
const { pause: pauseTyping, resume: resumeTyping } = useIntervalFn(() => {
  revealedCharacters.value += 1
  if (revealedCharacters.value >= siteNameCharacters.value.length) pauseTyping()
}, 120, { immediate: false })

watch([siteName, reducedMotion], () => {
  pauseTyping()
  revealedCharacters.value = reducedMotion.value === 'reduce' ? siteNameCharacters.value.length : 0
  if (reducedMotion.value !== 'reduce') resumeTyping()
}, { immediate: true })

const siteLogo = computed(() => sanitizeUrl(settings.value?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const docsBase = computed(() => appStore.docUrl || settings.value?.doc_url)
const docsUrl = computed(() => resolveDocsUrl(docsBase.value, '', locale.value))
const docsConfigToolUrl = computed(() => resolveDocsUrl(docsBase.value, '', locale.value))
const docsFaqUrl = computed(() => resolveDocsUrl(docsBase.value, 'faq', locale.value))
const registrationEnabled = computed(() => appStore.publicSettingsLoaded && settings.value?.registration_enabled === true)
const modelPlazaEnabled = computed(() => appStore.publicSettingsLoaded && resolveFeatureFlag(settings.value, FeatureFlags.modelPlaza))
const paymentEnabled = computed(() => appStore.publicSettingsLoaded && !simpleMode.value && resolveFeatureFlag(settings.value, FeatureFlags.payment))
const billingMode = computed(() => resolveSiteBillingMode(settings.value))
const contactInfo = computed(() => settings.value?.contact_info?.trim() || '')
const legalDocuments = computed(() => settings.value?.login_agreement_documents?.filter(document => document.content_md.trim()) || [])
const accountPath = computed(() => authStore.isAuthenticated
  ? (authStore.isAdmin ? '/admin/dashboard' : '/dashboard')
  : (registrationEnabled.value ? '/register' : '/login'))
const accountLabel = computed(() => authStore.isAuthenticated ? t('home.goToDashboard') : t(`home.builtin.${registrationEnabled.value ? 'createAccount' : 'loginToStart'}`))
const plazaPath = computed(() => settings.value?.model_plaza_require_auth ? protectedPath('/model-plaza') : '/model-plaza')
const plazaLabel = computed(() => {
  const requiresLogin = !authStore.isAuthenticated && settings.value?.model_plaza_require_auth
  return t(`home.builtin.${simpleMode.value ? (requiresLogin ? 'loginForModelsSimple' : 'viewModelsSimple') : (requiresLogin ? 'loginForModels' : 'viewModels')}`)
})
const purchasePath = computed(() => protectedPath(billingMode.value === 'subscription_only' ? '/purchase?tab=subscription' : '/purchase'))
const capabilities = computed(() => [
  { id: 'tools', icon: 'terminal' as const, href: docsConfigToolUrl.value },
  { id: 'keys', icon: 'key' as const, path: protectedPath('/keys') },
  ...(!simpleMode.value ? [{ id: 'usage', icon: 'chart' as const, path: protectedPath('/usage') }] : [])
])
const steps = computed(() => [
  { id: 'account', icon: 'user' as const, path: accountPath.value,
    title: t(`home.builtin.steps.${!registrationEnabled.value && !authStore.isAuthenticated ? 'existingAccount' : simpleMode.value ? 'login' : 'accountTitle'}`),
    description: t(`home.builtin.steps.${simpleMode.value ? 'accountSimple' : registrationEnabled.value ? 'accountDescription' : 'existingDescription'}`) },
  { id: 'key', icon: 'key' as const, path: protectedPath('/keys'), title: t('home.builtin.steps.keyTitle'), description: t('home.builtin.steps.keyDescription') },
  { id: 'connect', icon: 'terminal' as const, href: docsConfigToolUrl.value, title: t('home.builtin.steps.connectTitle'), description: t('home.builtin.steps.connectDescription') }
])
const faqs = computed(() => [
  { id: 'models', answer: t('home.builtin.faq.modelsAnswer') },
  { id: 'tools', answer: t('home.builtin.faq.toolsAnswer') },
  ...(!simpleMode.value ? [{ id: 'cost', answer: (modelPlazaEnabled.value ? t('home.builtin.faq.plazaAnswer') + ' ' : '') + t('home.builtin.faq.costAnswer') }] : []),
  { id: 'errors', answer: t(`home.builtin.faq.${simpleMode.value ? 'errorsSimple' : 'errorsAnswer'}`) + (contactInfo.value ? ' ' + t('home.builtin.faq.support') : '') }
])
const year = new Date().getFullYear()

function protectedPath(path: string) {
  return authStore.isAuthenticated ? path : { path: '/login', query: { redirect: path } }
}

function setTheme(dark: boolean) {
  isDark.value = dark
  document.documentElement.classList.toggle('dark', dark)
  localStorage.setItem('theme', dark ? 'dark' : 'light')
}

function closeMenu() {
  menuOpen.value = false
  menuButton.value?.focus()
}
</script>

<template>
  <div data-test="builtin-home" class="builtin-home">
    <a class="skip-link pill" href="#home-content">{{ t('home.builtin.skipContent') }}</a>
    <header class="site-header" @keydown.esc="closeMenu">
      <div class="app-header-bar nav-shell shell">
        <RouterLink to="/" class="brand" :title="siteName">
          <SiteLogo :src="siteLogo" :title="siteName" class="h-[30px]" :class="siteLogo ? 'w-[30px]' : 'w-[100px]'" /><span v-if="siteLogo">{{ siteName }}</span>
        </RouterLink>
        <nav class="desktop-nav" :aria-label="t('home.builtin.navigation')">
          <a href="#capabilities">{{ t('home.builtin.capabilitiesNav') }}</a>
          <RouterLink v-if="modelPlazaEnabled" :to="plazaPath">{{ plazaLabel }}</RouterLink>
          <a :href="docsUrl" target="_blank" rel="noopener noreferrer">{{ t('home.builtin.docs') }}</a>
        </nav>
        <div class="nav-actions">
          <div class="desktop-controls"><LocaleSwitcher /></div>
          <button class="pill header-theme" :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')" @click="setTheme(!isDark)"><Icon :name="isDark ? 'sun' : 'moon'" size="sm" /></button>
          <RouterLink :to="authStore.isAuthenticated ? accountPath : '/login'" class="pill nav-account" data-test="header-account">{{ authStore.isAuthenticated ? t('home.goToDashboard') : t('home.login') }}<Icon name="arrowRight" size="sm" /></RouterLink>
          <button ref="menuButton" class="pill menu-toggle" :aria-label="t('home.builtin.menu')" :aria-expanded="menuOpen" aria-controls="home-mobile-menu" data-test="mobile-toggle" @click="menuOpen = !menuOpen"><Icon :name="menuOpen ? 'x' : 'menu'" /></button>
        </div>
      </div>
      <nav v-if="menuOpen" id="home-mobile-menu" class="mobile-nav shell" :aria-label="t('home.builtin.navigation')" data-test="mobile-menu">
        <a href="#capabilities" @click="menuOpen = false">{{ t('home.builtin.capabilitiesNav') }}</a>
        <RouterLink v-if="modelPlazaEnabled" :to="plazaPath" @click="menuOpen = false">{{ plazaLabel }}</RouterLink>
        <a :href="docsUrl" target="_blank" rel="noopener noreferrer" @click="menuOpen = false">{{ t('home.builtin.docs') }}</a>
        <LocaleSwitcher />
      </nav>
    </header>

    <main id="home-content">
      <section class="hero" aria-labelledby="hero-title">
        <div class="hero-grid" aria-hidden="true" />
        <div class="shell hero-content">
          <p class="eyebrow">{{ t('home.builtin.eyebrow') }}</p>
          <p :key="siteName" class="hero-wordmark" :class="{ 'long-brand': siteName.length > 16 }" :aria-label="siteName">
            <span
              v-for="(character, index) in siteNameCharacters"
              :key="index"
              class="wordmark-character"
              :class="{ 'is-hidden': index >= revealedCharacters, 'is-cursor': reducedMotion !== 'reduce' && index === revealedCharacters - 1 }"
              aria-hidden="true"
            >{{ character }}</span>
          </p>
          <h1 id="hero-title">{{ t('home.builtin.title') }}<br /><span>{{ t('home.builtin.titleEnd') }}</span></h1>
          <p class="intro">{{ t(`home.builtin.${simpleMode ? 'descriptionSimple' : 'description'}`) }}</p>
          <div class="actions">
            <RouterLink :to="accountPath" class="pill primary" data-test="hero-account">{{ accountLabel }}<Icon name="arrowRight" size="sm" /></RouterLink>
            <a :href="docsConfigToolUrl" target="_blank" rel="noopener noreferrer" class="pill">{{ t('home.builtin.guide') }}<Icon name="book" size="sm" /></a>
          </div>
          <RouterLink v-if="!authStore.isAuthenticated && registrationEnabled" to="/login" class="login-link">{{ t('home.builtin.existingLogin') }}</RouterLink>

          <div class="connection-stage" :aria-label="t('home.builtin.flowLabel')">
            <article class="glass flow-card flow-models">
              <div class="card-overline"><span>01 / MODEL</span><Icon name="cube" size="sm" /></div>
              <h2>{{ t('home.builtin.flow.models') }}</h2>
              <div class="model-option"><Icon name="cube" /><span>{{ t('home.builtin.flow.group') }}</span><Icon name="checkCircle" size="sm" /></div>
              <div class="model-option muted"><Icon name="shield" /><span>{{ t('home.builtin.flow.permissions') }}</span></div>
              <p>{{ t('home.builtin.flow.modelsNote') }}</p>
            </article>
            <article class="glass flow-card flow-key">
              <div class="card-overline"><span>02 / API KEY</span><Icon name="key" size="sm" /></div>
              <div class="key-orbit" aria-hidden="true"><Icon name="key" size="xl" /></div>
              <h2>{{ t('home.builtin.flow.key') }}</h2>
              <p>{{ t('home.builtin.flow.keyNote') }}</p>
              <div class="key-placeholder" aria-hidden="true">•••• &nbsp; •••• &nbsp; •••• &nbsp; ••••</div>
            </article>
            <article class="glass flow-card flow-connect">
              <div class="card-overline"><span>03 / CONNECT</span><Icon name="terminal" size="sm" /></div>
              <h2>{{ t('home.builtin.flow.connect') }}</h2>
              <div class="connection-row"><Icon name="terminal" /><span>{{ t('home.builtin.flow.tool') }}</span><Icon name="arrowRight" size="sm" /></div>
              <div class="connection-row"><Icon name="grid" /><span>{{ t('home.builtin.flow.app') }}</span><Icon name="arrowRight" size="sm" /></div>
              <p>{{ t('home.builtin.flow.connectNote') }}</p>
            </article>
          </div>
          <div class="hero-tags">
            <span><Icon name="cube" size="sm" />{{ t('home.builtin.tags.models') }}</span>
            <span><Icon name="key" size="sm" />{{ t('home.builtin.tags.keys') }}</span>
            <span v-if="!simpleMode"><Icon name="chart" size="sm" />{{ t('home.builtin.tags.usage') }}</span>
          </div>
          <div class="theme-switch" role="group" :aria-label="t('home.builtin.theme')">
            <button :aria-pressed="isDark" :aria-label="t('home.switchToDark')" data-test="theme-dark" @click="setTheme(true)"><Icon name="moon" size="sm" /></button>
            <button :aria-pressed="!isDark" :aria-label="t('home.switchToLight')" data-test="theme-light" @click="setTheme(false)"><Icon name="sun" size="sm" /></button>
          </div>
        </div>
      </section>

      <section id="capabilities" class="section shell" aria-labelledby="capabilities-title">
        <div class="section-heading">
          <p class="eyebrow">{{ t('home.builtin.capabilitiesNav') }}</p>
          <h2 id="capabilities-title">{{ t('home.builtin.capabilitiesTitle') }}</h2>
          <p>{{ t('home.builtin.capabilitiesDescription') }}</p>
        </div>
        <div class="capabilities-grid" :class="{ 'two-columns': simpleMode }">
          <article v-for="capability in capabilities" :key="capability.id" class="capability" :data-test="`capability-${capability.id}`">
            <div class="icon-disc"><Icon :name="capability.icon" size="lg" /></div>
            <h3>{{ t(`home.builtin.capabilities.${capability.id}.title`) }}</h3>
            <p>{{ t(`home.builtin.capabilities.${capability.id}.description`) }}</p>
            <a
              v-if="capability.href"
              :href="capability.href"
              target="_blank"
              rel="noopener noreferrer"
              class="text-link"
            >{{ t(`home.builtin.capabilities.${capability.id}.action`) }}<Icon name="arrowRight" size="sm" /></a>
            <RouterLink
              v-else
              :to="capability.path!"
              class="text-link"
            >{{ t(`home.builtin.capabilities.${capability.id}.action`) }}<Icon name="arrowRight" size="sm" /></RouterLink>
          </article>
        </div>
      </section>

      <section class="section shell" aria-labelledby="models-title">
        <div class="glass models-panel">
          <div class="models-copy">
            <p class="eyebrow align-left">{{ t(`home.builtin.${simpleMode ? 'modelsLabel' : 'modelsCostsLabel'}`) }}</p>
            <h2 id="models-title">{{ t(`home.builtin.${simpleMode ? 'modelsTitleSimple' : 'modelsTitle'}`) }}</h2>
            <p>{{ t(`home.builtin.${modelPlazaEnabled ? (simpleMode ? 'modelsDescriptionSimple' : 'modelsDescription') : 'modelsClosed'}`) }}</p>
            <div class="actions align-left">
              <RouterLink v-if="modelPlazaEnabled" :to="plazaPath" class="pill primary" data-test="model-plaza">{{ plazaLabel }}<Icon name="arrowRight" size="sm" /></RouterLink>
              <RouterLink v-if="paymentEnabled" :to="purchasePath" class="pill" data-test="purchase">{{ t(`home.builtin.billing.${billingMode}.action`) }}</RouterLink>
            </div>
          </div>
          <div class="model-notes">
            <div><Icon name="cube" /><div><h3>{{ t('home.builtin.tags.models') }}</h3><p>{{ t('home.builtin.modelRights') }}</p></div></div>
            <div v-if="!simpleMode"><Icon name="chart" /><div><h3>{{ t('home.builtin.tags.usage') }}</h3><p>{{ t('home.builtin.usageNote') }}</p></div></div>
            <div v-if="!simpleMode"><Icon name="creditCard" /><div><h3>{{ t('home.builtin.quota') }}</h3><p>{{ paymentEnabled ? t(`home.builtin.billing.${billingMode}.description`) : t('home.builtin.billing.closed') }}</p></div></div>
          </div>
        </div>
      </section>

      <section class="section shell" aria-labelledby="steps-title">
        <div class="section-heading"><p class="eyebrow">{{ t('home.builtin.stepsLabel') }}</p><h2 id="steps-title">{{ t('home.builtin.stepsTitle') }}</h2></div>
        <ol class="steps-grid">
          <li v-for="(step, index) in steps" :key="step.id" class="glass step-card">
            <span class="step-number">0{{ index + 1 }}</span><Icon :name="step.icon" size="lg" />
            <h3>{{ step.title }}</h3><p>{{ step.description }}</p>
            <a
              v-if="step.href"
              :href="step.href"
              target="_blank"
              rel="noopener noreferrer"
              class="text-link"
            >{{ t(`home.builtin.steps.${step.id}Action`) }}<Icon name="arrowRight" size="sm" /></a>
            <RouterLink
              v-else
              :to="step.path!"
              class="text-link"
            >{{ t(`home.builtin.steps.${step.id}Action`) }}<Icon name="arrowRight" size="sm" /></RouterLink>
          </li>
        </ol>
        <aside class="tool-strip" aria-labelledby="tool-title" data-test="image-tool">
          <div class="tool-icon"><Icon name="sparkles" size="lg" /></div>
          <div><p class="tool-eyebrow">{{ t('home.builtin.additionalTools') }}</p><h3 id="tool-title">{{ t('home.builtin.imageTitle') }}</h3><p>{{ t('home.builtin.imageDescription') }}</p><p class="helper">{{ t('home.builtin.imagePermissions') }}</p></div>
          <RouterLink :to="protectedPath('/image-studio')" class="text-link">{{ t(`home.builtin.${authStore.isAuthenticated ? 'openTool' : 'loginForTool'}`) }}<Icon name="arrowRight" size="sm" /></RouterLink>
        </aside>
      </section>

      <section class="section faq-section shell" aria-labelledby="faq-title">
        <div class="section-heading"><p class="eyebrow">{{ t('home.builtin.faqLabel') }}</p><h2 id="faq-title">{{ t('home.builtin.faqTitle') }}</h2></div>
        <div class="faq-list">
          <details v-for="faq in faqs" :key="faq.id" :data-test="`faq-${faq.id}`"><summary>{{ t(`home.builtin.faq.${faq.id}Question`) }}<Icon name="plus" size="sm" /></summary><p>{{ faq.answer }}</p></details>
        </div>
        <a :href="docsFaqUrl" target="_blank" rel="noopener noreferrer" class="text-link more-help">{{ t('home.builtin.moreHelp') }}<Icon name="arrowRight" size="sm" /></a>
      </section>

      <section class="closing section shell" aria-labelledby="closing-title">
        <div class="closing-glow" aria-hidden="true" /><div class="icon-disc"><Icon name="key" size="lg" /></div>
        <h2 id="closing-title">{{ t('home.builtin.closingTitle') }}</h2><p>{{ t('home.builtin.closingDescription') }}</p>
        <div class="actions"><RouterLink :to="accountPath" class="pill primary">{{ accountLabel }}<Icon name="arrowRight" size="sm" /></RouterLink><a :href="docsUrl" target="_blank" rel="noopener noreferrer" class="pill">{{ t('home.builtin.docs') }}</a></div>
      </section>
    </main>

    <footer class="site-footer shell">
      <div class="footer-top">
        <RouterLink to="/" class="brand"><SiteLogo :src="siteLogo" :title="siteName" class="h-6" :class="siteLogo ? 'w-6' : 'w-20'" /><span v-if="siteLogo">{{ siteName }}</span></RouterLink>
        <nav :aria-label="t('home.builtin.footerNavigation')"><a :href="docsUrl" target="_blank" rel="noopener noreferrer">{{ t('home.builtin.docs') }}</a><RouterLink to="/key-usage">{{ t('home.builtin.keyUsage') }}</RouterLink><RouterLink v-for="document in legalDocuments" :key="document.id" :to="`/legal/${encodeURIComponent(document.id)}`">{{ document.title }}</RouterLink></nav>
      </div>
      <p v-if="contactInfo" class="contact" data-test="contact">{{ t('home.builtin.contact') }} · {{ contactInfo }}</p>
      <p class="copyright">© {{ year }} {{ siteName }}</p>
    </footer>
  </div>
</template>

<style scoped>
.builtin-home { --canvas: #05060f; --surface: rgba(186,214,247,.035); --raised: #101522; --ink: #d1e4fa; --muted: #a7b3c8; --line: rgba(186,215,247,.14); --glow: rgba(143,180,229,.12); --sky-start: #d8ecf8; --sky-end: #98c0ef; background: var(--canvas); color: var(--ink); font-family: Inter, ui-sans-serif, system-ui, sans-serif; font-size: 15px; line-height: 1.7; overflow: clip; }
:global(html:not(.dark) .builtin-home) { --canvas: #f5f8fc; --surface: rgba(255,255,255,.6); --raised: #fff; --ink: #263c58; --muted: #52647b; --line: rgba(61,91,131,.17); --glow: rgba(122,162,216,.14); --sky-start: #294565; --sky-end: #577ba9; }
.shell { width: min(1200px, calc(100% - 64px)); margin-inline: auto; }
.builtin-home :is(h1,h2,h3,p) { margin: 0; }
.builtin-home :is(h1,h2,h3) { font-weight: 500; line-height: 1.35; text-wrap: balance; }
.builtin-home h2 { font-size: clamp(28px, 3.5vw, 44px); letter-spacing: -.035em; }
.builtin-home h3 { font-size: 20px; }
.builtin-home a { text-decoration: none; }
.builtin-home :is(a,button,summary):focus-visible { outline: 2px solid var(--sky-end); outline-offset: 5px; }
.builtin-home :is(a,button) { -webkit-tap-highlight-color: transparent; }
.builtin-home :is(p,a,h1,h2,h3,span) { overflow-wrap: anywhere; }
.builtin-home section[id], #home-content { scroll-margin-top: calc(var(--app-header-height) + 25px); }
.skip-link { position: fixed; top: -100px; left: 24px; z-index: 100; background: var(--raised) !important; }
.skip-link:focus { top: 12px; }
.site-header { position: sticky; top: 0; z-index: 30; background: color-mix(in srgb, var(--canvas) 88%, transparent); border-bottom: 1px solid var(--line); backdrop-filter: blur(24px); }
.nav-shell { display: flex; align-items: center; justify-content: space-between; gap: 24px; }
.brand { display: flex; align-items: center; gap: 10px; font-size: 18px; font-weight: 500; min-width: 0; }
.brand img { flex-shrink: 0; object-fit: contain; }
.brand span { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 230px; }
.desktop-nav, .nav-actions { display: flex; align-items: center; gap: 28px; }
.desktop-nav { font-size: 13px; color: var(--muted); }
.nav-actions { gap: 12px; flex-shrink: 0; }
.desktop-nav a:hover, .text-link:hover, .site-footer a:hover { color: var(--sky-end); }
.pill { display: inline-flex; justify-content: center; align-items: center; gap: 12px; padding: 11px 22px; border-radius: 999px; border: 1px solid var(--line); background: var(--surface); font-size: 14px; font-weight: 500; transition: background .2s, border-color .2s; }
.pill:hover { background: var(--glow); border-color: var(--sky-end); }
.pill.primary { background: linear-gradient(180deg, var(--glow), var(--surface)); box-shadow: inset 0 1px 1px rgba(216,236,248,.2), 0 0 24px var(--glow); }
.nav-account { padding: 8px 16px; font-size: 12px; }
.header-theme { padding: 9px; }
.menu-toggle, .mobile-nav { display: none; }
.hero { position: relative; padding-top: 68px; background: radial-gradient(ellipse at 50% 0, var(--glow), transparent 58%); }
.hero::before { content: ''; position: absolute; inset: 0; background: conic-gradient(from 145deg at 50% -15%, transparent 0deg, var(--glow) 30deg, transparent 70deg); pointer-events: none; }
.hero-grid { position: absolute; inset: 0; background-image: linear-gradient(var(--line) 1px, transparent 1px), linear-gradient(90deg, var(--line) 1px, transparent 1px); background-size: 88px 88px; opacity: .36; mask-image: radial-gradient(ellipse at 50% 40%, #000, transparent 72%); pointer-events: none; }
.hero-content { position: relative; text-align: center; }
.eyebrow { display: flex; align-items: center; justify-content: center; gap: 18px; color: var(--muted); font: 11px/1.7 'JetBrains Mono', ui-monospace, monospace; letter-spacing: .13em; text-transform: uppercase; }
.eyebrow::before, .eyebrow::after { content: ''; width: 64px; height: 1px; background: linear-gradient(90deg, transparent, var(--line)); }
.eyebrow::after { transform: rotate(180deg); }
.hero-wordmark { padding: 16px 0 18px; font-size: clamp(64px, 11vw, 150px); font-weight: 500; line-height: 1.1; letter-spacing: .015em; }
.hero-wordmark.long-brand { font-size: clamp(34px, 5.5vw, 80px); }
.wordmark-character { position: relative; display: inline-block; white-space: pre; background: linear-gradient(180deg, var(--sky-start) 20%, var(--sky-end)); background-clip: text; -webkit-text-fill-color: transparent; }
.wordmark-character.is-hidden { visibility: hidden; }
.wordmark-character::after { content: ''; position: absolute; right: 0; top: .14em; width: 2px; height: .8em; background: var(--sky-end); opacity: 0; }
.wordmark-character.is-cursor::after { animation: wordmark-caret 600ms step-end 3; }
@keyframes wordmark-caret { 0%, 70% { opacity: 1; } 71%, 100% { opacity: 0; } }
.hero h1 { margin-top: 8px; font-size: clamp(29px, 3.8vw, 48px); letter-spacing: -.025em; }
.hero h1 span { color: var(--sky-end); }
.intro { max-width: 610px; margin: 24px auto 0 !important; color: var(--muted); line-height: 1.9; }
.actions { display: flex; justify-content: center; gap: 12px; flex-wrap: wrap; margin-top: 28px; }
.login-link { display: inline-block; margin-top: 16px; font-size: 12px; color: var(--muted); }
.glass { border: 1px solid var(--line); border-radius: 16px; background: var(--surface); box-shadow: inset 0 1px 1px rgba(216,236,248,.16), inset 0 24px 48px var(--glow), 0 16px 32px rgba(0,0,0,.08); }
.connection-stage { display: grid; grid-template-columns: 1fr 1.08fr 1fr; align-items: center; max-width: 960px; margin: 64px auto 30px; padding: 16px; }
.flow-card { padding: 24px; text-align: left; background: color-mix(in srgb, var(--raised) 85%, transparent); backdrop-filter: blur(12px); }
.flow-card h2 { font-size: 19px; letter-spacing: 0; margin: 22px 0 16px; }
.flow-card p { color: var(--muted); font-size: 12px; margin-top: 16px; }
.card-overline { display: flex; align-items: center; justify-content: space-between; color: var(--muted); font: 10px/1.5 ui-monospace, monospace; letter-spacing: .12em; }
.flow-models { transform: rotate(-5deg) translateX(10px); }
.flow-connect { transform: rotate(5deg) translateX(-10px); }
.flow-key { z-index: 1; text-align: center; min-height: 300px; }
.key-orbit { display: grid; place-items: center; width: 64px; height: 64px; margin: 22px auto 0; border-radius: 50%; border: 1px solid var(--line); background: var(--glow); box-shadow: 0 0 38px var(--glow); }
.flow-key h2 { margin: 14px 0 8px; }
.flow-key p { margin: 0; }
.key-placeholder { border: 1px solid var(--line); border-radius: 6px; padding: 10px; margin-top: 20px; color: var(--muted); letter-spacing: .12em; }
.model-option, .connection-row { display: flex; align-items: center; gap: 10px; padding: 10px 12px; border: 1px solid var(--line); border-radius: 6px; margin-top: 8px; font-size: 12px; }
.model-option svg:last-child, .connection-row svg:last-child { margin-left: auto; flex-shrink: 0; }
.model-option.muted { border-color: transparent; color: var(--muted); }
.hero-tags { display: flex; gap: 28px; justify-content: center; flex-wrap: wrap; font-size: 12px; color: var(--muted); }
.hero-tags span { display: flex; align-items: center; gap: 8px; }
.theme-switch { display: inline-flex; padding: 4px; border: 1px solid var(--line); border-radius: 999px; margin-top: 28px; }
.theme-switch button { padding: 7px 15px; border-radius: 999px; color: var(--muted); }
.theme-switch button[aria-pressed=true] { background: var(--glow); color: var(--ink); }
.section { margin-top: 120px; }
.section-heading { text-align: center; margin-bottom: 48px; }
.section-heading h2 { margin-top: 20px; }
.section-heading > p:last-child:not(.eyebrow) { margin-top: 18px; color: var(--muted); }
.capabilities-grid, .steps-grid { display: grid; grid-template-columns: repeat(3,minmax(0,1fr)); gap: 24px; }
.capabilities-grid { gap: 48px; }
.capabilities-grid.two-columns { grid-template-columns: repeat(2,minmax(0,1fr)); max-width: 850px; margin-inline: auto; }
.capability { text-align: center; position: relative; }
.icon-disc { display: grid; place-items: center; width: 64px; height: 64px; margin: 0 auto 28px; border: 1px solid var(--line); background: var(--surface); border-radius: 50%; box-shadow: inset 0 1px 1px rgba(216,236,248,.2), inset 0 12px 24px var(--glow); }
.capability:not(:last-child)::after { content: ''; position: absolute; top: 32px; left: calc(50% + 50px); width: calc(100% - 52px); height: 1px; background: linear-gradient(90deg, var(--line), transparent, var(--line)); }
.capability p { color: var(--muted); margin-top: 16px; font-size: 14px; }
.text-link { display: inline-flex; align-items: center; gap: 10px; font-size: 13px; font-weight: 500; }
.capability .text-link { margin-top: 24px; }
.models-panel { display: grid; grid-template-columns: 1.1fr 1fr; gap: 72px; padding: 56px; }
.align-left { justify-content: flex-start; }
.eyebrow.align-left::before { display: none; }
.models-copy h2 { margin-top: 20px; font-size: 34px; }
.models-copy > p:not(.eyebrow) { color: var(--muted); margin-top: 20px; font-size: 14px; }
.model-notes { display: flex; flex-direction: column; justify-content: center; }
.model-notes > div { display: flex; gap: 16px; padding: 24px 0; border-bottom: 1px solid var(--line); }
.model-notes > div:first-child { padding-top: 0; }
.model-notes > div:last-child { border: 0; padding-bottom: 0; }
.model-notes svg { flex-shrink: 0; margin-top: 4px; }
.model-notes h3 { font-size: 16px; }
.model-notes p { margin-top: 8px; color: var(--muted); font-size: 13px; }
.steps-grid { list-style: none; padding: 0; margin-bottom: 0; }
.step-card { position: relative; display: flex; flex-direction: column; align-items: flex-start; padding: 28px; }
.step-number { position: absolute; right: 24px; top: 16px; font: 48px/1.2 ui-monospace, monospace; color: var(--line); }
.step-card h3 { margin-top: 28px; font-size: 18px; }
.step-card p { color: var(--muted); margin-top: 14px; font-size: 14px; flex: 1; }
.step-card .text-link { margin-top: 26px; }
.tool-strip { display: flex; align-items: center; gap: 24px; margin-top: 40px; border-block: 1px solid var(--line); padding: 28px 8px; }
.tool-icon { flex-shrink: 0; padding: 16px; background: var(--surface); border: 1px solid var(--line); border-radius: 50%; }
.tool-strip h3 { font-size: 17px; margin: 4px 0 8px; }
.tool-strip p { color: var(--muted); font-size: 13px; }
.tool-strip .tool-eyebrow, .tool-strip .helper { font-size: 11px; }
.tool-strip .helper { margin-top: 4px; }
.tool-strip .text-link { flex-shrink: 0; margin-left: auto; }
.faq-section { max-width: 800px; }
.faq-list details { border-bottom: 1px solid var(--line); }
.faq-list summary { display: flex; align-items: center; justify-content: space-between; gap: 24px; cursor: pointer; padding: 24px 4px; list-style: none; font-size: 16px; }
.faq-list summary::-webkit-details-marker { display: none; }
.faq-list summary svg { flex-shrink: 0; transition: transform .2s; }
.faq-list details[open] summary svg { transform: rotate(45deg); }
.faq-list details p { color: var(--muted); padding: 0 36px 24px 4px; font-size: 14px; }
.more-help { display: flex; width: fit-content; margin: 28px auto 0; }
.closing { position: relative; text-align: center; padding: 64px 0 100px; }
.closing-glow { position: absolute; inset: 0; pointer-events: none; background: radial-gradient(ellipse at 50% 65%, var(--glow), transparent 65%); }
.closing > :not(.closing-glow) { position: relative; }
.closing p { color: var(--muted); margin: 20px auto 0; max-width: 620px; }
.site-footer { border-top: 1px solid var(--line); padding-block: 32px; font-size: 12px; color: var(--muted); }
.footer-top { display: flex; justify-content: space-between; gap: 24px; align-items: center; }
.footer-top nav { display: flex; flex-wrap: wrap; justify-content: flex-end; gap: 16px 24px; }
.contact { margin-top: 24px !important; white-space: pre-line; }
.copyright { margin-top: 24px !important; }
@media (max-width: 1023px) {
  .desktop-nav, .desktop-controls { display: none; }
  .menu-toggle { display: inline-flex; padding: 9px; }
  .mobile-nav { display: flex; flex-direction: column; gap: 20px; padding-block: 20px; }
  .models-panel { padding: 36px; gap: 36px; }
  .connection-stage { padding-inline: 0; }
  .flow-card { padding: 20px; }
}
@media (max-width: 767px) {
  .shell { width: calc(100% - 40px); }
  .nav-shell { gap: 12px; }
  .brand { font-size: 15px; gap: 8px; }
  .brand span { max-width: 150px; }
  .nav-shell .brand { flex: 1; }
  .nav-actions { gap: 8px; }
  .nav-account { padding: 8px 12px; font-size: 11px; }
  .nav-account svg { display: none; }
  .hero { padding-top: 48px; }
  .hero-wordmark { padding-block: 24px; }
  .intro { font-size: 14px; }
  .connection-stage { grid-template-columns: 1fr; gap: 14px; max-width: 420px; margin-top: 40px; }
  .flow-models, .flow-connect { transform: none; }
  .flow-card { padding: 20px 24px; }
  .flow-key { min-height: auto; }
  .flow-card h2 { margin-top: 16px; }
  .flow-key .key-orbit { margin-top: 16px; width: 48px; height: 48px; }
  .hero-tags { gap: 12px 18px; font-size: 11px; }
  .section { margin-top: 80px; }
  .section-heading { margin-bottom: 32px; }
  .capabilities-grid, .capabilities-grid.two-columns, .steps-grid { grid-template-columns: 1fr; gap: 24px; }
  .capability { padding: 20px 12px; }
  .capability:not(:last-child)::after { display: none; }
  .models-panel { grid-template-columns: 1fr; padding: 28px 24px; gap: 36px; }
  .models-copy h2 { font-size: 28px; }
  .tool-strip { flex-wrap: wrap; gap: 16px; padding-block: 24px; }
  .tool-icon { display: none; }
  .tool-strip .text-link { margin-left: 0; }
  .faq-list summary { font-size: 15px; }
  .closing { padding: 24px 0 72px; }
  .footer-top { align-items: flex-start; flex-direction: column; }
  .footer-top nav { justify-content: flex-start; }
  .eyebrow { gap: 12px; font-size: 10px; }
  .eyebrow::before, .eyebrow::after { width: 36px; }
}
@media (prefers-reduced-motion: reduce) {
  .builtin-home *, .builtin-home *::before, .builtin-home *::after { transition: none !important; animation: none !important; scroll-behavior: auto !important; }
}
</style>
