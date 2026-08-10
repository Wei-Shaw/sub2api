<template>
  <div v-if="variant === 'studio'" class="auth-studio">
    <header class="auth-studio-header">
      <router-link to="/home" class="auth-studio-brand">
        <img :src="siteLogo || signalMark" :alt="siteName" />
        <span>{{ siteName }}<small>API</small></span>
      </router-link>
      <div class="auth-studio-status">
        <i></i>
        {{ t('authStudio.secureGateway') }}
      </div>
    </header>

    <main class="auth-studio-main">
      <section class="auth-studio-context" aria-labelledby="auth-studio-context-title">
        <div>
          <span class="auth-studio-eyebrow"><i></i>{{ t('authStudio.eyebrow') }}</span>
          <h1 id="auth-studio-context-title">
            {{ t('authStudio.titleLineOne') }}
            <em>{{ t('authStudio.titleLineTwo') }}</em>
          </h1>
          <p>{{ t('authStudio.description') }}</p>
        </div>

        <div class="auth-studio-terminal" aria-hidden="true">
          <div class="auth-studio-terminal-head">
            <span><i></i><i></i><i></i></span>
            <b>{{ siteName }} / auth</b>
          </div>
          <div class="auth-studio-terminal-body">
            <p><strong>$</strong> POST /api/v1/auth/session</p>
            <p class="auth-studio-terminal-comment"># {{ t('authStudio.terminalChecking') }}</p>
            <div class="auth-studio-route">
              <span>{{ t('authStudio.client') }}</span><i></i><b>{{ t('authStudio.gateway') }}</b><i></i><span>{{ t('authStudio.workspace') }}</span>
            </div>
            <div class="auth-studio-response"><strong>200 OK</strong><code>session: ready</code></div>
          </div>
        </div>

        <ul class="auth-studio-signals">
          <li><i></i>{{ t('authStudio.signals.unified') }}</li>
          <li><i></i>{{ t('authStudio.signals.protected') }}</li>
          <li><i></i>{{ t('authStudio.signals.auditable') }}</li>
        </ul>
      </section>

      <section class="auth-studio-workspace">
        <div class="auth-studio-panel">
          <div class="auth-studio-panel-label">
            <span>{{ t('authStudio.accessLabel') }}</span>
            <b>HTTPS / TLS</b>
          </div>
          <slot />
          <div class="auth-studio-footer"><slot name="footer" /></div>
        </div>
      </section>
    </main>

    <footer class="auth-studio-copyright">
      <span>&copy; {{ currentYear }} {{ siteName }}</span>
      <span>{{ t('authStudio.footer') }}</span>
    </footer>
  </div>

  <div v-else class="relative flex min-h-screen items-center justify-center overflow-hidden p-4">
    <div class="absolute inset-0 bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"></div>
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="absolute -right-40 -top-40 h-80 w-80 rounded-full bg-primary-400/20 blur-3xl"></div>
      <div class="absolute -bottom-40 -left-40 h-80 w-80 rounded-full bg-primary-500/15 blur-3xl"></div>
      <div class="absolute left-1/2 top-1/2 h-96 w-96 -translate-x-1/2 -translate-y-1/2 rounded-full bg-primary-300/10 blur-3xl"></div>
      <div class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"></div>
    </div>

    <div class="relative z-10 w-full max-w-md">
      <div class="mb-8 text-center">
        <template v-if="settingsLoaded">
          <div class="mb-4 inline-flex h-16 w-16 items-center justify-center overflow-hidden rounded-2xl shadow-lg shadow-primary-500/30">
            <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <h1 class="text-gradient mb-2 text-3xl font-bold">{{ siteName }}</h1>
          <p class="text-sm text-gray-500 dark:text-dark-400">{{ siteSubtitle }}</p>
        </template>
      </div>

      <div class="card-glass rounded-2xl p-8 shadow-glass"><slot /></div>
      <div class="mt-6 text-center text-sm"><slot name="footer" /></div>
      <div class="mt-8 text-center text-xs text-gray-400 dark:text-dark-500">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import signalMark from '@/assets/signal-mark.svg'
import { sanitizeUrl } from '@/utils/url'

withDefaults(defineProps<{ variant?: 'classic' | 'studio' }>(), { variant: 'classic' })

const { t } = useI18n()
const appStore = useAppStore()
const siteName = computed(() => appStore.siteName || 'Sub2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)
const currentYear = computed(() => new Date().getFullYear())

onMounted(() => { appStore.fetchPublicSettings() })
</script>

<style scoped>
.text-gradient { @apply bg-gradient-to-r from-primary-600 to-primary-500 bg-clip-text text-transparent; }
.auth-studio { min-height: 100svh; display: grid; grid-template-rows: 72px minmax(0, 1fr) 46px; overflow-x: hidden; background-color: #fff; background-image: linear-gradient(rgb(27 58 96 / 3.5%) 1px, transparent 1px), linear-gradient(90deg, rgb(27 58 96 / 3.5%) 1px, transparent 1px); background-size: 48px 48px; color: #111827; font-family: Inter, "Noto Sans SC", "Microsoft YaHei UI", sans-serif; }
.auth-studio::before { content: ""; position: fixed; z-index: 0; top: -230px; right: -170px; width: 580px; height: 580px; border-radius: 50%; background: #eef7ff; pointer-events: none; }
.auth-studio-header, .auth-studio-main, .auth-studio-copyright { position: relative; z-index: 1; width: min(1220px, calc(100% - 64px)); margin-inline: auto; }
.auth-studio-header { display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #e6ebf0; }
.auth-studio-brand { display: inline-flex; align-items: center; gap: 10px; color: #111827; font-size: 20px; font-weight: 850; text-decoration: none; }
.auth-studio-brand img { width: 34px; height: 34px; border-radius: 8px; object-fit: contain; box-shadow: 0 5px 14px rgb(20 48 80 / 8%); }
.auth-studio-brand small { margin-left: 6px; color: #7d8898; font-size: 9px; }
.auth-studio-status { display: flex; align-items: center; gap: 8px; color: #637084; font: 650 10px ui-monospace, Consolas, monospace; text-transform: uppercase; }
.auth-studio-status i, .auth-studio-eyebrow i { width: 7px; height: 7px; border-radius: 50%; background: #33b687; box-shadow: 0 0 0 4px #e7f8f1; }
.auth-studio-main { min-height: 0; display: grid; grid-template-columns: minmax(0, 1.08fr) minmax(380px, .92fr); gap: clamp(54px, 7vw, 104px); align-items: center; padding-block: 36px; }
.auth-studio-context { min-width: 0; display: grid; gap: 28px; }
.auth-studio-eyebrow { display: flex; align-items: center; gap: 10px; color: #59677a; font: 650 11px ui-monospace, Consolas, monospace; }
.auth-studio-context h1 { max-width: 650px; margin: 20px 0 18px; color: #111827; font-size: clamp(46px, 4.3vw, 64px); line-height: 1.04; font-weight: 850; }
.auth-studio-context h1 em { display: block; color: #087f67; font-style: normal; }
.auth-studio-context > div > p { max-width: 580px; margin: 0; color: #637084; font-size: 15px; line-height: 1.75; }
.auth-studio-terminal { max-width: 620px; overflow: hidden; border: 1px solid #dce2e9; border-radius: 8px; background: #151923; color: #f2f5f9; box-shadow: 0 22px 50px rgb(31 45 66 / 16%), 10px 10px 0 #eeedf9; }
.auth-studio-terminal-head { height: 42px; display: grid; grid-template-columns: 1fr auto; align-items: center; padding: 0 16px; border-bottom: 1px solid #303644; color: #96a2b4; font: 650 10px ui-monospace, Consolas, monospace; }
.auth-studio-terminal-head > span { display: flex; gap: 6px; }.auth-studio-terminal-head i { width: 7px; height: 7px; border-radius: 50%; background: #ff6b67; }.auth-studio-terminal-head i:nth-child(2) { background: #f4bb40; }.auth-studio-terminal-head i:nth-child(3) { background: #35c675; }.auth-studio-terminal-head b { font-weight: 500; }
.auth-studio-terminal-body { padding: 20px 22px; font: 10px/1.6 ui-monospace, Consolas, monospace; }.auth-studio-terminal-body p { margin: 0; }.auth-studio-terminal-body strong { color: #b5ff3d; }.auth-studio-terminal-comment { margin-top: 14px !important; color: #7f8ba0; }
.auth-studio-route { display: grid; grid-template-columns: auto 1fr auto 1fr auto; align-items: center; gap: 10px; margin-top: 15px; color: #f1f4f8; font-size: 9px; }.auth-studio-route i { height: 1px; background: #536075; }.auth-studio-route b { padding: 6px 10px; border: 1px solid #74b700; color: #b5ff3d; font-weight: 600; }.auth-studio-route span:last-child { text-align: right; }
.auth-studio-response { min-height: 40px; display: flex; align-items: center; gap: 12px; margin-top: 16px; padding: 0 12px; border: 1px solid #3c765f; border-radius: 4px; background: rgb(57 244 166 / 3.5%); }.auth-studio-response strong { color: #39f4a6; }.auth-studio-response code { color: #f0d261; font: inherit; }
.auth-studio-signals { max-width: 620px; display: grid; grid-template-columns: repeat(3, 1fr); gap: 10px; margin: 0; padding: 0; list-style: none; }.auth-studio-signals li { min-width: 0; display: flex; align-items: center; gap: 8px; padding: 11px 12px; border: 1px solid #dce2e9; border-radius: 6px; background: rgb(255 255 255 / 78%); color: #59677a; font-size: 11px; }.auth-studio-signals i { width: 6px; height: 6px; flex: none; border-radius: 50%; background: #33b687; }
.auth-studio-workspace { min-width: 0; display: flex; justify-content: flex-end; }
.auth-studio-panel { width: min(100%, 500px); max-height: calc(100svh - 154px); overflow-y: auto; padding: 28px 30px 26px; border: 1px solid #dce2e9; border-radius: 8px; background: rgb(255 255 255 / 94%); box-shadow: 0 24px 65px rgb(31 45 66 / 14%); scrollbar-width: thin; }
.auth-studio-panel-label { display: flex; align-items: center; justify-content: space-between; margin-bottom: 22px; padding-bottom: 12px; border-bottom: 1px solid #e6ebf0; color: #788495; font: 700 9px ui-monospace, Consolas, monospace; }.auth-studio-panel-label b { color: #36785a; font-weight: 700; }
.auth-studio-footer { margin-top: 22px; padding-top: 18px; border-top: 1px solid #e6ebf0; color: #6f7a8a; text-align: center; font-size: 12px; }
.auth-studio-copyright { display: flex; align-items: center; justify-content: space-between; border-top: 1px solid #e6ebf0; color: #7a8594; font-size: 10px; }
@media (max-width: 960px) { .auth-studio { display: block; min-height: 100svh; }.auth-studio-header, .auth-studio-main, .auth-studio-copyright { width: calc(100% - 32px); }.auth-studio-header { height: 64px; }.auth-studio-main { display: block; padding-block: 28px; }.auth-studio-context { display: none; }.auth-studio-workspace { justify-content: center; }.auth-studio-panel { width: min(100%, 540px); max-height: none; padding: 26px 24px; }.auth-studio-copyright { min-height: 74px; flex-wrap: wrap; gap: 8px; padding-block: 16px; } }
@media (max-width: 420px) { .auth-studio-header, .auth-studio-main, .auth-studio-copyright { width: calc(100% - 24px); }.auth-studio-brand { font-size: 18px; }.auth-studio-status { display: none; }.auth-studio-panel { padding: 22px 18px; } }
</style>
