<template>
  <div data-testid="studio-home" class="studio home-style-page">
    <header class="studio-topbar studio-shell">
      <router-link class="studio-brand" to="/home">
        <img :src="context.siteLogo || signalMark" width="34" height="34" :alt="context.siteName" />
        <span>{{ context.siteName }}<small>API</small></span>
      </router-link>
      <nav aria-label="Primary navigation">
        <a href="#models">{{ t('home.styles.studio.modelsNav') }}</a>
        <a href="#features">{{ t('home.styles.studio.featuresNav') }}</a>
        <a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer">
          {{ t('home.styles.studio.docsNav') }}
        </a>
        <button v-else type="button" @click="showToast(t('home.styles.studio.docsSoon'))">
          {{ t('home.styles.studio.docsNav') }}
        </button>
      </nav>
      <div class="studio-top-actions">
        <router-link v-if="context.isAuthenticated" :to="context.dashboardPath" class="studio-login-link">
          {{ t('home.dashboard') }}
        </router-link>
        <router-link v-else to="/login" class="studio-login-link">{{ t('home.login') }}</router-link>
        <button type="button" class="studio-button studio-button-compact" @click="openModal">
          {{ t('home.styles.studio.start') }}
        </button>
      </div>
    </header>

    <main class="studio-shell studio-main">
      <section class="studio-hero">
        <div class="studio-hero-copy">
          <span class="studio-eyebrow"><i></i>{{ t('home.styles.studio.eyebrow') }}</span>
          <h1>
            <span>{{ t('home.styles.studio.heroLineOne') }}</span>
            <em>{{ t('home.styles.studio.heroLineTwo') }}</em>
          </h1>
          <p>{{ t('home.styles.studio.description') }}</p>
          <div class="studio-hero-actions">
            <button type="button" class="studio-button" @click="openModal">
              {{ context.isAuthenticated ? t('home.goToDashboard') : t('home.styles.studio.getKey') }}
              <span aria-hidden="true">&rarr;</span>
            </button>
            <a v-if="context.docUrl" class="studio-doc-link" :href="context.docUrl" target="_blank" rel="noopener noreferrer">
              {{ t('home.styles.studio.viewDocs') }}
            </a>
            <button v-else type="button" class="studio-doc-link" @click="showToast(t('home.styles.studio.docsSoon'))">
              {{ t('home.styles.studio.viewDocs') }}
            </button>
          </div>
        </div>

        <div class="studio-terminal" :class="{ 'is-switching': isSwitching, 'is-routed': isRouted }" :aria-label="t('home.styles.studio.terminalLabel')" aria-live="polite">
          <div class="studio-terminal-head">
            <span><i></i><i></i><i></i></span>
            <b>{{ context.siteName }} / {{ t('home.styles.studio.request') }}</b>
          </div>
          <div class="studio-terminal-body">
            <div class="studio-terminal-command"><strong>$</strong><code>curl -X POST /v1/chat/completions</code></div>
            <p class="studio-terminal-comment"># {{ routeStatus }}</p>
            <div class="studio-terminal-route">
              <span>{{ t('home.styles.studio.yourApp') }}</span><i></i>
              <b>{{ t('home.styles.studio.gateway') }}</b><i></i>
              <span>{{ selectedModel.name }}</span>
            </div>
            <div class="studio-terminal-response"><strong>{{ t('home.styles.studio.responseCode') }}</strong><code>content: {{ t('home.styles.studio.responseBody') }}</code></div>
            <div class="studio-terminal-stats">
              <div><span>{{ t('home.styles.studio.details.model') }}</span><strong>{{ selectedModel.detail }}</strong></div>
              <div><span>{{ t('home.styles.studio.details.route') }}</span><strong>{{ t('home.styles.studio.details.routeValue') }}</strong></div>
              <div><span>{{ t('home.styles.studio.details.interface') }}</span><strong>{{ t('home.styles.studio.details.interfaceValue') }}</strong></div>
            </div>
          </div>
        </div>
      </section>

      <section id="features" class="studio-features" :aria-label="t('home.styles.studio.featuresLabel')">
        <article v-for="(feature, index) in features" :key="feature.title">
          <span>{{ String(index + 1).padStart(2, '0') }}</span>
          <div><h2>{{ t(feature.title) }}</h2><p>{{ t(feature.body) }}</p></div>
        </article>
      </section>

      <section id="models" class="studio-models">
        <div class="studio-models-heading"><span>{{ t('home.styles.studio.modelsTitle') }}</span><p>{{ t('home.styles.studio.modelsSubtitle') }}</p></div>
        <div class="studio-model-list">
          <button
            v-for="model in models"
            :key="model.id"
            type="button"
            class="studio-model"
            :class="{ 'is-active': selectedModel.id === model.id }"
            :aria-pressed="selectedModel.id === model.id"
            @click="selectModel(model.id)"
          >
            <span class="studio-model-mark" :class="model.markClass">{{ model.mark }}</span>
            <span><strong>{{ model.name }}</strong><small>{{ model.description }}</small></span>
            <b><i></i>{{ t('home.styles.studio.available') }}</b>
          </button>
        </div>
        <button type="button" class="studio-text-button" @click="openModal">
          {{ t('home.styles.studio.modelPricing') }} <span aria-hidden="true">&rarr;</span>
        </button>
      </section>
    </main>

    <footer class="studio-shell">
      <span>&copy; {{ context.currentYear }} {{ context.siteName }}</span>
      <p><i></i>{{ t('home.styles.studio.serviceNormal') }}</p>
      <div>
        <a href="#" @click.prevent="showToast(t('home.styles.studio.termsSoon'))">{{ t('home.styles.studio.terms') }}</a>
        <a href="#" @click.prevent="showToast(t('home.styles.studio.privacySoon'))">{{ t('home.styles.studio.privacy') }}</a>
        <a v-if="context.docUrl" :href="context.docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.styles.studio.support') }}</a>
        <a v-else href="#" @click.prevent="showToast(t('home.styles.studio.supportSoon'))">{{ t('home.styles.studio.support') }}</a>
      </div>
    </footer>

    <div class="studio-toast" :class="{ 'is-visible': toastMessage }" role="status" aria-live="polite">{{ toastMessage }}</div>
    <div v-if="isModalOpen" class="studio-modal" role="dialog" aria-modal="true" aria-labelledby="studio-modal-title" @click.self="closeModal">
      <div class="studio-modal-card">
        <button type="button" class="studio-modal-close" :aria-label="t('home.styles.studio.close')" @click="closeModal">&times;</button>
        <span>GET STARTED</span>
        <h2 id="studio-modal-title">{{ t('home.styles.studio.modalTitle', { siteName: context.siteName }) }}</h2>
        <p>{{ t('home.styles.studio.modalDescription') }}</p>
        <router-link class="studio-button studio-modal-submit" :to="context.isAuthenticated ? context.dashboardPath : '/register'" @click="closeModal">
          {{ context.isAuthenticated ? t('home.goToDashboard') : t('home.styles.studio.modalAction') }} <span aria-hidden="true">&rarr;</span>
        </router-link>
        <small v-if="!context.isAuthenticated">
          {{ t('home.styles.studio.modalAccountPrompt') }}
          <router-link to="/login" @click="closeModal">{{ t('home.styles.studio.modalLogin') }}</router-link>
        </small>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import signalMark from '@/assets/signal-mark.svg'
import type { HomeStyleContext } from './types'

const props = defineProps<{ context: HomeStyleContext }>()
const { t } = useI18n()

const features = [
  { title: 'home.styles.studio.features.unified.title', body: 'home.styles.studio.features.unified.body' },
  { title: 'home.styles.studio.features.reliable.title', body: 'home.styles.studio.features.reliable.body' },
  { title: 'home.styles.studio.features.usage.title', body: 'home.styles.studio.features.usage.body' },
] as const

const models = [
  { id: 'gpt', name: 'ChatGPT', detail: 'GPT-5', description: 'OpenAI compatible', mark: '◎', markClass: 'studio-chatgpt' },
  { id: 'claude', name: 'Claude', detail: 'Claude Sonnet', description: 'OpenAI / Anthropic', mark: 'AI', markClass: 'studio-claude' },
] as const

const selectedModelId = ref<(typeof models)[number]['id']>('gpt')
const isSwitching = ref(false)
const isRouted = ref(false)
const isModalOpen = ref(false)
const toastMessage = ref('')
let routeTimer: ReturnType<typeof setTimeout> | undefined
let toastTimer: ReturnType<typeof setTimeout> | undefined

const context = computed(() => props.context)
const selectedModel = computed(() => models.find((model) => model.id === selectedModelId.value) || models[0])
const routeStatus = computed(() => isSwitching.value
  ? t('home.styles.studio.switching', { model: selectedModel.value.name })
  : t('home.styles.studio.routeComment', { model: selectedModel.value.name }))

function selectModel(id: (typeof models)[number]['id']) {
  selectedModelId.value = id
  isRouted.value = false
  isSwitching.value = true
  clearTimeout(routeTimer)
  routeTimer = setTimeout(() => {
    isSwitching.value = false
    isRouted.value = true
  }, 420)
}

function openModal() {
  isModalOpen.value = true
  document.body.style.overflow = 'hidden'
}

function closeModal() {
  isModalOpen.value = false
  document.body.style.overflow = ''
}

function showToast(message: string) {
  toastMessage.value = message
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toastMessage.value = '' }, 1600)
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') closeModal()
}

onMounted(() => document.addEventListener('keydown', handleKeydown))
onBeforeUnmount(() => {
  clearTimeout(routeTimer)
  clearTimeout(toastTimer)
  document.removeEventListener('keydown', handleKeydown)
  document.body.style.overflow = ''
})
</script>

<style scoped>
* { box-sizing: border-box; }
.studio { --studio-ink: #111827; --studio-muted: #566376; --studio-border: #dce2e9; min-height: 100svh; display: grid; grid-template-rows: 72px 1fr 54px; overflow: hidden; background-color: #fff; background-image: linear-gradient(rgba(27,58,96,.035) 1px, transparent 1px), linear-gradient(90deg, rgba(27,58,96,.035) 1px, transparent 1px); background-size: 48px 48px; color: var(--studio-ink); font-family: Inter, "Noto Sans SC", "Microsoft YaHei UI", sans-serif; }
.studio::before { content: ""; position: fixed; z-index: 0; top: -210px; right: -160px; width: 560px; height: 560px; border-radius: 50%; background: #eef7ff; filter: blur(2px); pointer-events: none; }
.studio a { color: inherit; text-decoration: none; }
.studio button { font: inherit; }
.studio-shell { position: relative; z-index: 1; width: min(1220px, calc(100% - 64px)); margin-inline: auto; }
.studio-topbar { display: flex; align-items: center; justify-content: space-between; border-bottom: 1px solid #edf0f4; }
.studio-brand { display: inline-flex; align-items: center; gap: 10px; font-size: 20px; font-weight: 850; }
.studio-brand img { border-radius: 8px; box-shadow: 0 5px 14px rgba(20,48,80,.08); }
.studio-brand small { margin-left: 6px; color: #7d8898; font-size: 9px; letter-spacing: .08em; }
.studio nav { display: flex; gap: 34px; margin-left: 95px; color: var(--studio-muted); font-size: 14px; }
.studio nav button { border: 0; background: transparent; color: inherit; cursor: pointer; padding: 0; }
.studio nav a:hover, .studio nav button:hover, .studio-login-link:hover, .studio-doc-link:hover { color: var(--studio-ink); }
.studio-top-actions { display: flex; align-items: center; gap: 20px; color: var(--studio-muted); font-size: 14px; }
.studio-button { min-height: 48px; display: inline-flex; align-items: center; justify-content: center; gap: 28px; padding: 0 23px; border: 1px solid var(--studio-ink); border-radius: 7px; background: var(--studio-ink); color: #fff; cursor: pointer; font-size: 14px; font-weight: 750; box-shadow: 0 8px 20px rgba(17,24,39,.12); transition: transform 150ms ease, background 150ms ease; }
.studio .studio-button { color: #fff; }
.studio-button:hover { background: #253043; transform: translateY(-1px); }
.studio-button-compact { min-height: 40px; padding-inline: 18px; font-size: 13px; box-shadow: none; }
.studio-main { display: grid; grid-template-rows: minmax(350px, 1fr) 146px 112px; align-content: center; padding-block: 12px 10px; }
.studio-hero { min-height: 0; display: grid; grid-template-columns: .94fr 1.06fr; align-items: center; gap: clamp(58px, 6vw, 84px); }
.studio-hero-copy { min-width: 0; }
.studio-eyebrow { display: flex; align-items: center; gap: 10px; color: #59677a; font: 650 11px ui-monospace, Consolas, monospace; letter-spacing: .1em; }
.studio-eyebrow i { width: 7px; height: 7px; border-radius: 50%; background: #33b687; box-shadow: 0 0 0 4px #e7f8f1; }
.studio h1 { max-width: 620px; margin: 21px 0 20px; font-size: clamp(50px, 4.5vw, 66px); line-height: 1.05; font-weight: 800; }
.studio h1 span, .studio h1 em { display: block; }
.studio h1 em { color: #087f67; font-style: normal; white-space: nowrap; }
.studio-hero-copy > p { max-width: 540px; margin: 0; color: #5f6c7e; font-size: 15px; line-height: 1.78; white-space: pre-wrap; }
.studio-hero-actions { display: flex; align-items: center; gap: 28px; margin-top: 28px; }
.studio-doc-link { padding-block: 12px; border: 0; border-bottom: 1px solid #c5ccd5; background: transparent; color: #59677a; cursor: pointer; font-size: 13px; }
.studio-terminal { min-width: 0; overflow: hidden; border: 1px solid var(--studio-border); border-radius: 9px; background: #151923; color: #f2f5f9; box-shadow: 0 24px 55px rgba(31,45,66,.18), 12px 12px 0 #eeedf9; }
.studio-terminal-head { height: 48px; display: grid; grid-template-columns: 1fr auto; align-items: center; padding: 0 18px; border-bottom: 1px solid #303644; color: #96a2b4; font: 650 11px ui-monospace, Consolas, monospace; }
.studio-terminal-head > span { display: flex; gap: 6px; }.studio-terminal-head i { width: 7px; height: 7px; border-radius: 50%; background: #ff6b67; }.studio-terminal-head i:nth-child(2) { background: #f4bb40; }.studio-terminal-head i:nth-child(3) { background: #35c675; }
.studio-terminal-head b { justify-self: end; font-weight: 500; }.studio-terminal-body { padding: 26px 27px; font: 11px/1.6 ui-monospace, "SFMono-Regular", Consolas, monospace; }
.studio-terminal-command { display: flex; align-items: center; gap: 8px; }.studio-terminal-command strong { color: #b5ff3d; }.studio-terminal-command code { color: #f7f8fa; font: inherit; font-weight: 700; }.studio-terminal-comment { margin: 17px 0; color: #7f8ba0; }
.studio-terminal-route { display: grid; grid-template-columns: auto 1fr auto 1fr auto; align-items: center; gap: 11px; color: #f1f4f8; font-size: 10px; }.studio-terminal-route i { height: 1px; background: #536075; }.studio-terminal-route b { padding: 8px 13px; border: 1px solid #74b700; color: #b5ff3d; font-weight: 600; }.studio-terminal-route span:last-child { text-align: right; }
.studio-terminal-response { min-height: 46px; display: flex; align-items: center; gap: 13px; margin-top: 20px; padding: 0 14px; border: 1px solid #4e596e; border-radius: 4px; }.studio-terminal-response strong { color: #39f4a6; }.studio-terminal-response code { color: #f0d261; font: inherit; }
.studio-terminal-stats { display: grid; grid-template-columns: repeat(3, 1fr); gap: 9px; margin-top: 14px; }.studio-terminal-stats div { min-width: 0; padding: 12px 13px; border: 1px solid #4e596e; border-radius: 4px; }.studio-terminal-stats span { display: block; margin-bottom: 8px; color: #8996aa; font-size: 9px; }.studio-terminal-stats strong { display: block; overflow: hidden; color: #f2f5f9; text-overflow: ellipsis; white-space: nowrap; font-size: 10px; }
.studio-terminal-route b, .studio-terminal-response, .studio-terminal-stats div { transition: border-color 180ms ease, background-color 180ms ease, opacity 180ms ease; }.studio-terminal.is-switching .studio-terminal-route b { border-color: #f0d261; color: #f0d261; }.studio-terminal.is-switching .studio-terminal-response, .studio-terminal.is-switching .studio-terminal-stats { opacity: .56; }.studio-terminal.is-routed .studio-terminal-response { border-color: #3c765f; background: rgba(57,244,166,.035); }
.studio-features { display: grid; grid-template-columns: repeat(3, 1fr); gap: 12px; }.studio-features article { display: grid; grid-template-columns: 40px 1fr; align-items: start; gap: 15px; padding: 21px; border: 1px solid var(--studio-border); border-radius: 8px; background: rgba(255,255,255,.82); }.studio-features article > span { width: 38px; height: 38px; display: grid; place-items: center; border-radius: 7px; background: #eef3f8; color: #53647a; font: 700 10px ui-monospace, Consolas, monospace; }.studio-features h2 { margin: 1px 0 8px; font-size: 16px; }.studio-features p { margin: 0; color: #647184; font-size: 12px; line-height: 1.6; }
.studio-models { display: grid; grid-template-columns: 190px 1fr auto; align-items: center; gap: 24px; }.studio-models-heading > span { font-size: 17px; font-weight: 780; }.studio-models-heading p { margin: 7px 0 0; color: #7e8998; font-size: 11px; }.studio-model-list { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
.studio-model { min-width: 0; height: 72px; display: grid; grid-template-columns: 42px 1fr auto; align-items: center; gap: 12px; padding: 0 15px; border: 1px solid #d7dee7; border-radius: 8px; background: rgba(255,255,255,.86); color: #152033; text-align: left; cursor: pointer; transition: 150ms ease; }.studio-model:hover, .studio-model.is-active { border-color: #829db9; background: #fff; box-shadow: 0 7px 18px rgba(38,60,84,.07); }.studio-model.is-active { box-shadow: inset 3px 0 var(--studio-ink), 0 7px 18px rgba(38,60,84,.07); }.studio-model-mark { width: 40px; height: 40px; display: grid; place-items: center; border-radius: 7px; color: #fff; font-size: 13px; font-weight: 800; }.studio-chatgpt { background: #171f2d; }.studio-claude { background: #a84c2e; }.studio-model > span:nth-child(2) { min-width: 0; }.studio-model strong, .studio-model small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.studio-model strong { font-size: 14px; }.studio-model small { margin-top: 5px; color: #647286; font-size: 10px; }.studio-model > b { display: flex; align-items: center; gap: 6px; color: #36785a; font-size: 10px; font-weight: 650; }.studio-model > b i { width: 6px; height: 6px; border-radius: 50%; background: #35b982; }
.studio-text-button { min-height: 40px; border: 0; border-bottom: 1px solid #c8d0d9; background: transparent; color: #4f5d70; cursor: pointer; font-size: 12px; white-space: nowrap; }
.studio footer { display: flex; align-items: center; justify-content: space-between; border-top: 1px solid #e6ebf0; color: #717d8d; font-size: 11px; }.studio footer p { display: flex; align-items: center; gap: 8px; margin: 0; }.studio footer p i { width: 7px; height: 7px; border-radius: 50%; background: #39ba84; }.studio footer div { display: flex; gap: 22px; }
.studio-toast { position: fixed; z-index: 90; left: 50%; bottom: 22px; padding: 10px 14px; border-radius: 6px; background: #111827; color: #fff; opacity: 0; pointer-events: none; transform: translate(-50%, 8px); transition: 160ms ease; font-size: 10px; }.studio-toast.is-visible { opacity: 1; transform: translate(-50%, 0); }
.studio-modal { position: fixed; z-index: 80; inset: 0; display: grid; place-items: center; padding: 20px; background: rgba(17,24,39,.42); backdrop-filter: blur(5px); }.studio-modal-card { position: relative; width: min(420px, 100%); padding: 30px; border: 1px solid #e0e4ea; border-radius: 10px; background: #fff; box-shadow: 0 28px 80px rgba(17,24,39,.2); }.studio-modal-close { position: absolute; top: 13px; right: 13px; width: 34px; height: 34px; border: 0; background: transparent; color: #7c8796; cursor: pointer; font-size: 20px; }.studio-modal-card > span { color: #8a95a4; font: 700 8px ui-monospace, Consolas, monospace; letter-spacing: .1em; }.studio-modal h2 { margin: 13px 0 8px; font-size: 24px; }.studio-modal p { margin: 0 0 22px; color: #6f7a8a; font-size: 12px; line-height: 1.7; }.studio-modal-submit { width: 100%; }.studio-modal-card small { display: block; margin-top: 13px; color: #919aa7; text-align: center; font-size: 9px; }.studio-modal-card small a { margin-left: 4px; color: #4f5d70; font-weight: 700; }.studio-modal-card small a:hover { color: #111827; }
.studio button:focus-visible, .studio a:focus-visible { outline: 3px solid rgba(45,114,177,.25); outline-offset: 3px; }
@media (min-width: 1600px) and (min-height: 900px) { .studio-shell { width: min(1360px, calc(100% - 96px)); }.studio-hero { grid-template-columns: .92fr 1.08fr; gap: 100px; }.studio h1 { max-width: 670px; font-size: 70px; }.studio-hero-copy > p { max-width: 590px; font-size: 16px; }.studio-terminal-body { padding: 29px 30px; font-size: 12px; }.studio-features h2 { font-size: 17px; }.studio-features p { font-size: 13px; }.studio-models-heading > span { font-size: 18px; } }
@media (min-width: 2200px) and (min-height: 1200px) { .studio { grid-template-rows: 88px 1fr 66px; }.studio-shell { width: min(1520px, calc(100% - 128px)); }.studio-brand { font-size: 23px; }.studio-brand img { width: 40px; height: 40px; }.studio nav, .studio-top-actions { font-size: 15px; }.studio-main { grid-template-rows: minmax(500px, 1fr) 166px 132px; padding-block: 24px 18px; }.studio-hero { gap: 120px; }.studio-eyebrow { font-size: 12px; }.studio h1 { max-width: 750px; font-size: 78px; }.studio-hero-copy > p { max-width: 650px; font-size: 17px; }.studio-terminal-head { height: 54px; font-size: 12px; }.studio-terminal-body { padding: 34px; font-size: 13px; }.studio-terminal-route { font-size: 11px; }.studio-terminal-stats strong { font-size: 11px; }.studio-features article { padding: 25px; }.studio-features h2 { font-size: 18px; }.studio-features p { font-size: 14px; }.studio-model { height: 80px; }.studio footer { font-size: 12px; } }
@media (max-width: 1100px) and (min-width: 761px) { .studio-shell { width: calc(100% - 48px); }.studio nav { gap: 24px; margin-left: 40px; }.studio-hero { grid-template-columns: .9fr 1.1fr; gap: 38px; }.studio h1 { font-size: clamp(47px, 5vw, 54px); }.studio-hero-copy > p { font-size: 14px; }.studio-terminal-body { padding-inline: 20px; }.studio-features article { grid-template-columns: 36px 1fr; gap: 11px; padding: 16px; }.studio-features article > span { width: 34px; height: 34px; }.studio-features h2 { font-size: 15px; }.studio-features p { font-size: 11px; }.studio-models { grid-template-columns: 150px 1fr auto; gap: 16px; }.studio-model { padding-inline: 11px; }.studio-text-button { font-size: 11px; } }
@media (max-height: 800px) and (min-width: 761px) { .studio { grid-template-rows: 64px 1fr 46px; }.studio-topbar { height: 64px; }.studio-main { grid-template-rows: minmax(310px, 1fr) 124px 96px; padding-block: 8px 6px; }.studio h1 { margin-block: 14px; font-size: clamp(45px, 4.1vw, 56px); }.studio-hero-copy > p { line-height: 1.65; }.studio-hero-actions { margin-top: 20px; }.studio-terminal-head { height: 42px; }.studio-terminal-body { padding-block: 18px; }.studio-terminal-comment { margin-block: 12px; }.studio-terminal-response { min-height: 40px; margin-top: 14px; }.studio-terminal-stats { margin-top: 10px; }.studio-terminal-stats div { padding-block: 9px; }.studio-model { height: 64px; }.studio footer { font-size: 10px; } }
@media (max-height: 730px) and (min-width: 761px) { .studio { grid-template-rows: 58px 1fr 40px; }.studio-topbar { height: 58px; }.studio-main { grid-template-rows: minmax(286px, 1fr) 112px 86px; padding-block: 5px; }.studio-brand { font-size: 18px; }.studio-brand img { width: 30px; height: 30px; }.studio nav, .studio-top-actions { font-size: 12px; }.studio-button-compact { min-height: 34px; }.studio h1 { font-size: clamp(42px, 3.8vw, 50px); }.studio-hero-copy > p { font-size: 13px; }.studio-button { min-height: 42px; font-size: 12px; }.studio-terminal-body { padding-block: 14px; font-size: 10px; }.studio-terminal-comment { margin-block: 9px; }.studio-terminal-route { font-size: 9px; }.studio-terminal-response { min-height: 36px; margin-top: 10px; }.studio-terminal-stats div { padding: 7px 9px; }.studio-features article { padding: 12px 15px; }.studio-features article > span { width: 32px; height: 32px; }.studio-features h2 { font-size: 14px; }.studio-features p { font-size: 10px; }.studio-models-heading > span { font-size: 15px; }.studio-model { height: 58px; }.studio-model-mark { width: 34px; height: 34px; } }
@media (max-width: 760px) { .studio { min-height: 100svh; display: block; overflow: visible; }.studio::before { width: 340px; height: 340px; }.studio-shell { width: calc(100% - 32px); }.studio-topbar { min-height: 64px; }.studio nav, .studio-login-link { display: none; }.studio-brand { min-height: 44px; }.studio-top-actions .studio-button { min-height: 44px; }.studio-main { display: block; padding-block: 38px 28px; }.studio-hero { display: grid; grid-template-columns: 1fr; gap: 34px; }.studio h1 { margin-block: 18px; font-size: clamp(38px, 11.2vw, 49px); line-height: 1.09; }.studio h1 em { margin-top: 5px; }.studio-hero-copy > p { font-size: 14px; line-height: 1.72; }.studio-hero-actions { gap: 22px; margin-top: 24px; }.studio-hero-actions .studio-button { min-height: 46px; padding-inline: 18px; font-size: 13px; }.studio-doc-link { min-height: 44px; display: inline-flex; align-items: center; font-size: 12px; }.studio-terminal-head { height: 44px; font-size: 10px; }.studio-terminal-body { padding: 20px 15px; font-size: 10px; }.studio-terminal-route { gap: 5px; font-size: 8px; }.studio-terminal-route b { padding: 6px 8px; }.studio-terminal-response { gap: 9px; padding-inline: 10px; }.studio-terminal-stats div { padding: 10px 7px; }.studio-terminal-stats span { font-size: 8px; }.studio-terminal-stats strong { font-size: 8px; }.studio-features { grid-template-columns: 1fr; gap: 10px; margin-top: 36px; }.studio-features article { min-height: 112px; padding: 18px; }.studio-features h2 { font-size: 16px; }.studio-features p { font-size: 12px; }.studio-models { grid-template-columns: 1fr; gap: 15px; padding-block: 30px 8px; }.studio-model-list { grid-template-columns: 1fr; }.studio-text-button { min-height: 44px; justify-self: start; padding: 8px 0; }.studio footer { min-height: 128px; align-items: flex-start; flex-direction: column; justify-content: center; gap: 7px; padding-block: 16px; }.studio footer div { flex-wrap: wrap; gap: 4px 14px; }.studio footer a { min-height: 44px; display: inline-flex; align-items: center; } }
@media (max-width: 380px) { .studio-shell { width: calc(100% - 28px); }.studio-brand { font-size: 18px; }.studio-brand img { width: 32px; height: 32px; }.studio-button-compact { padding-inline: 14px; font-size: 12px; }.studio h1 { font-size: 37px; }.studio-hero-actions { align-items: stretch; flex-direction: column; gap: 10px; }.studio-hero-actions .studio-button { width: 100%; }.studio-doc-link { align-self: flex-start; }.studio-terminal-command code { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; } }
@media (prefers-reduced-motion: reduce) { .studio *, .studio *::before, .studio *::after { scroll-behavior: auto !important; transition-duration: .01ms !important; animation-duration: .01ms !important; animation-iteration-count: 1 !important; } }
</style>
