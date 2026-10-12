<template>
  <div class="premium-login-shell">
    <header class="premium-login-header">
      <router-link to="/home" class="premium-login-brand" aria-label="Home">
        <span class="premium-login-logo"><img :src="siteLogo || '/logo.svg'" alt="" /></span>
        <span>{{ siteName }}</span>
      </router-link>
      <div class="premium-login-tools">
        <LocaleSwitcher />
        <button type="button" :title="themeTitle" @click="toggleTheme">
          <Icon :name="isDark ? 'sun' : 'moon'" size="sm" />
        </button>
      </div>
    </header>

    <main class="premium-login-main">
      <div class="premium-login-stage">
        <section class="premium-login-showcase">
          <div class="premium-login-showcase-copy">
            <span class="premium-login-kicker"><i></i>{{ t('auth.workspaceFeatureSecurity') }}</span>
            <h1>
              <span>{{ t('auth.workspaceTitleLineOne') }}</span>
              <span class="premium-login-title-accent">{{ t('auth.workspaceTitleLineTwo') }}</span>
            </h1>
            <p>{{ t('auth.workspaceDescription') }}</p>
          </div>
          <div class="premium-login-apps">
            <div class="premium-login-apps-copy">
              <span>{{ t('auth.workspaceAppSupport') }}</span>
              <p>{{ t('auth.workspaceAppSupportDescription') }}</p>
            </div>
            <div class="premium-login-app-list">
              <span class="premium-login-app">
                <i class="premium-login-app-icon premium-login-app-icon-cherry">C</i>
                Cherry Studio
              </span>
              <span class="premium-login-app">
                <i class="premium-login-app-icon premium-login-app-icon-cc">CC</i>
                CC Switch
              </span>
              <span class="premium-login-app premium-login-app-more">
                <i class="premium-login-app-icon">•••</i>
                {{ t('auth.workspaceAppSupportMore') }}
              </span>
            </div>
          </div>
          <div class="premium-login-art" aria-hidden="true">
            <div class="premium-login-wordmark">
              <span>B</span><span>U</span><span>A</span><span>G</span><span>A</span>
              <span>A</span><span>P</span><span>I</span>
            </div>
          </div>
        </section>

        <section class="premium-login-form-column">
          <div class="premium-login-form-wrap">
            <div class="premium-login-panel">
              <div class="premium-login-mobile-brand">
                <span class="premium-login-logo"><img :src="siteLogo || '/logo.svg'" alt="" /></span>
                <span>{{ siteName }}</span>
              </div>
              <div class="premium-login-form-card"><slot /></div>
              <div class="premium-login-footer"><slot name="footer" /></div>
            </div>
            <p class="premium-login-copyright">&copy; {{ currentYear }} {{ siteName }}</p>
          </div>
        </section>
      </div>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const appStore = useAppStore()
const isDark = ref(document.documentElement.classList.contains('dark'))
const siteName = computed(() => appStore.siteName || 'BuaGa API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const currentYear = new Date().getFullYear()
const themeTitle = computed(() => isDark.value ? t('home.switchToLight') : t('home.switchToDark'))

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

onMounted(() => {
  if (typeof appStore.fetchPublicSettings === 'function') {
    void appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.premium-login-shell { min-height: 100vh; background: #f7f9fc; color: #111827; }
.premium-login-header { position: fixed; z-index: 20; inset: 0 0 auto; display: flex; height: 4.25rem; align-items: center; justify-content: space-between; padding: 0 1.5rem; border-bottom: 1px solid #e2e8f0; background: rgba(255,255,255,.94); backdrop-filter: blur(18px); }
.premium-login-brand, .premium-login-mobile-brand { display: inline-flex; align-items: center; gap: .65rem; color: #111827; font-size: .9rem; font-weight: 700; }
.premium-login-logo { display: inline-flex; width: 2rem; height: 2rem; align-items: center; justify-content: center; overflow: hidden; border-radius: .5rem; box-shadow: 0 1px 3px rgba(15,23,42,.18); }
.premium-login-logo img { width: 100%; height: 100%; object-fit: contain; }
.premium-login-tools { display: flex; align-items: center; gap: .25rem; }
.premium-login-tools > button { display: inline-flex; width: 2.25rem; height: 2.25rem; align-items: center; justify-content: center; border-radius: .5rem; color: #64748b; }
.premium-login-tools > button:hover { background: #f1f5f9; color: #0f172a; }
.premium-login-main { display: flex; min-height: 100vh; align-items: center; justify-content: center; padding: 7.5rem 1.5rem 3.5rem; }
.premium-login-stage { display: grid; width: min(100%, 68rem); min-height: 39rem; grid-template-columns: minmax(0,1.12fr) minmax(24rem,.88fr); overflow: hidden; border: 1px solid #e1e8f0; border-radius: 1rem; background: #fff; box-shadow: 0 24px 64px rgba(15,23,42,.1); }
.premium-login-showcase { position: relative; display: flex; min-width: 0; flex-direction: column; overflow: hidden; padding: 4.25rem clamp(2rem,5vw,4.75rem) 2.5rem; background: #f3f6fa; }
.premium-login-showcase::before { position: absolute; inset: 0; content: ''; background-image: linear-gradient(#dfe7f1 1px, transparent 1px), linear-gradient(90deg,#dfe7f1 1px, transparent 1px); background-size: 3.5rem 3.5rem; opacity: .5; mask-image: linear-gradient(to bottom, black, transparent 85%); pointer-events: none; }
.premium-login-showcase-copy { position: relative; z-index: 1; max-width: 30rem; }
.premium-login-kicker { display: inline-flex; align-items: center; gap: .5rem; color: #64748b; font-size: .7rem; font-weight: 650; }
.premium-login-kicker i { width: .45rem; height: .45rem; border-radius: 50%; background: #10b981; box-shadow: 0 0 0 3px rgba(16,185,129,.13); }
.premium-login-showcase h1 { margin-top: 1.15rem; color: #0f172a; font-size: clamp(2.65rem,4vw,3.25rem); line-height: 1.06; font-weight: 760; letter-spacing: 0; }
.premium-login-showcase h1 span { display: block; white-space: nowrap; }
.premium-login-showcase h1 .premium-login-title-accent { margin-top: .15rem; background: linear-gradient(90deg,#4f8fff 0%,#7c6cf5 45%,#c13df0 100%); background-clip: text; color: transparent; font-size: clamp(3rem,4.8vw,4.25rem); line-height: 1.02; -webkit-background-clip: text; }
.premium-login-showcase p { max-width: 28rem; margin-top: 1.15rem; color: #64748b; font-size: .92rem; line-height: 1.75; }
.premium-login-apps { position: relative; z-index: 1; margin-top: 1.7rem; }
.premium-login-apps-copy { display: flex; align-items: baseline; justify-content: space-between; gap: 1rem; }
.premium-login-apps-copy > span { flex: none; color: #64748b; font-size: .68rem; font-weight: 700; }
.premium-login-apps-copy > p { overflow: hidden; margin: 0; color: #94a3b8; font-size: .66rem; line-height: 1.4; text-overflow: ellipsis; white-space: nowrap; }
.premium-login-app-list { display: grid; margin-top: .65rem; grid-template-columns: 1.22fr 1.05fr .72fr; gap: .55rem; }
.premium-login-app { display: flex; min-width: 0; height: 2.35rem; align-items: center; justify-content: center; gap: .45rem; border: 1px solid #dfe6ef; border-radius: 999px; background: rgba(255,255,255,.64); color: #334155; font-size: .7rem; font-weight: 650; white-space: nowrap; }
.premium-login-app-icon { display: inline-flex; min-width: 1.35rem; height: 1.35rem; align-items: center; justify-content: center; border-radius: 50%; color: #94a3b8; font-size: .58rem; font-style: normal; font-weight: 800; }
.premium-login-app-icon-cherry { background: #fff0f1; color: #ef5d67; }
.premium-login-app-icon-cc { background: #eaf8f4; color: #14a88a; }
.premium-login-app-more { color: #64748b; }
.premium-login-art { position: relative; z-index: 1; display: flex; flex: 1; min-height: 9.5rem; align-items: center; justify-content: center; margin-top: 1rem; }
.premium-login-wordmark { display: flex; width: 100%; max-width: 28rem; align-items: center; justify-content: center; gap: clamp(.4rem,1vw,.72rem); }
.premium-login-wordmark span { display: inline-block; color: #f43f5e; font-size: clamp(3rem,5.2vw,4.55rem); font-weight: 800; line-height: 1; }
.premium-login-wordmark span:nth-child(2), .premium-login-wordmark span:nth-child(6) { color: #f59e0b; transform: translateY(1.7rem) rotate(-6deg); }
.premium-login-wordmark span:nth-child(3), .premium-login-wordmark span:nth-child(7) { color: #2563eb; transform: translateY(-1.2rem) rotate(8deg); }
.premium-login-wordmark span:nth-child(4), .premium-login-wordmark span:nth-child(8) { color: #10b981; transform: translateY(1.5rem) rotate(-7deg); }
.premium-login-wordmark span:nth-child(5) { transform: translateY(-.35rem) rotate(7deg); }
.premium-login-wordmark span:nth-child(6) { margin-left: clamp(.35rem,.8vw,.65rem); }
.premium-login-form-column { display: flex; min-width: 0; align-items: center; justify-content: center; padding: 3rem clamp(1.75rem,5vw,4.75rem); background: #fff; }
.premium-login-form-wrap { width: min(100%, 25rem); }
.premium-login-panel { width: 100%; }
.premium-login-mobile-brand { display: none; margin-bottom: 1.8rem; }
.premium-login-form-card { width: 100%; }
.premium-login-footer { margin-top: 1.5rem; text-align: center; font-size: .8rem; }
.premium-login-copyright { margin-top: 1.75rem; color: #a1a1aa; text-align: center; font-size: .65rem; }
.dark .premium-login-shell { background: #0b0d12; color: #f8fafc; }
.dark .premium-login-header { border-color: #27272a; background: rgba(11,13,18,.9); }
.dark .premium-login-brand, .dark .premium-login-mobile-brand { color: #f8fafc; }
.dark .premium-login-tools > button { color: #a1a1aa; }
.dark .premium-login-tools > button:hover { background: #18181b; color: white; }
.dark .premium-login-stage { border-color: #30343d; background: #14161c; box-shadow: 0 24px 64px rgba(0,0,0,.28); }
.dark .premium-login-showcase { background: #151820; }
.dark .premium-login-showcase::before { background-image: linear-gradient(#30343d 1px, transparent 1px), linear-gradient(90deg,#30343d 1px, transparent 1px); }
.dark .premium-login-showcase h1 { color: #f8fafc; }
.dark .premium-login-showcase h1 .premium-login-title-accent { color: transparent; }
.dark .premium-login-showcase p { color: #a1a1aa; }
.dark .premium-login-apps-copy > span { color: #a1a1aa; }
.dark .premium-login-apps-copy > p { color: #71717a; }
.dark .premium-login-app { border-color: #30343d; background: rgba(24,24,27,.54); color: #d4d4d8; }
.dark .premium-login-app-icon-cherry { background: rgba(239,93,103,.14); color: #fb7185; }
.dark .premium-login-app-icon-cc { background: rgba(20,168,138,.14); color: #34d399; }
.dark .premium-login-app-more { color: #a1a1aa; }
.dark .premium-login-form-column { background: #14161c; }
@media (max-width: 800px) { .premium-login-stage { display: block; min-height: 0; max-width: 31rem; } .premium-login-showcase { display: none; } .premium-login-form-column { min-height: 38rem; padding: 2.5rem clamp(1.25rem,6vw,2.5rem); } .premium-login-mobile-brand { display: inline-flex; } }
@media (max-width: 640px) { .premium-login-header { padding-inline: 1rem; } .premium-login-main { padding: 6.5rem 1rem 2.5rem; } .premium-login-stage { border-radius: .8rem; } .premium-login-form-column { min-height: 0; padding: 2rem 1.15rem; } }
</style>
