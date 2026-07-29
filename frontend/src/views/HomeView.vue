<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <div v-else ref="landingRoot" class="landing-shell">
    <aside class="identity-panel">
      <div class="identity-glow" aria-hidden="true"></div>
      <div class="identity-rule" aria-hidden="true"></div>

      <router-link
        to="/home"
        class="brand-mark"
        aria-label="EasyHub 首页"
        :data-custom-logo="siteLogo ? 'configured' : undefined"
      >
        <span class="brand-glyph" aria-hidden="true">EH</span>
        <span>
          <strong>EasyHub</strong>
          <small>AI API RELAY</small>
        </span>
      </router-link>

      <div class="identity-copy">
        <p class="service-status"><i></i> GPT / CODEX · 运行正常</p>
        <h1>调度顶尖模型的<br /><b>单一入口</b></h1>
        <p>当前支持 GPT / Codex。智能路由、Token 级计量、<br />企业级可用性 — 更多模型即将接入。</p>
        <div class="hero-actions">
          <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="action-fill">
            进入控制台
            <Icon name="arrowRight" size="sm" />
          </router-link>
          <a href="#capability" class="action-outline" @click.prevent="scrollToSection('capability')">查看能力</a>
          <a :href="docHref" target="_blank" rel="noopener noreferrer" class="action-outline">使用手册</a>
        </div>
      </div>

      <nav class="section-index" aria-label="首页章节">
        <a href="#capability" :class="{ active: activeSection === 'capability' }" @click.prevent="scrollToSection('capability')"><span>01</span>核心能力</a>
        <a href="#models" :class="{ active: activeSection === 'models' }" @click.prevent="scrollToSection('models')"><span>02</span>模型矩阵</a>
        <a href="#protocol" :class="{ active: activeSection === 'protocol' }" @click.prevent="scrollToSection('protocol')"><span>03</span>接口协议</a>
        <a href="#access" :class="{ active: activeSection === 'access' }" @click.prevent="scrollToSection('access')"><span>04</span>接入流程</a>
      </nav>
    </aside>

    <main class="content-panel">
      <div class="ticker" aria-hidden="true">
        <div class="ticker-track">
          <span class="ticker-item"><b>GPT</b> Codex · Responses</span>
          <span class="ticker-item is-soon"><b>Claude</b> 即将接入</span>
          <span class="ticker-item is-soon"><b>Gemini</b> 即将接入</span>
          <span class="ticker-item is-soon"><b>Grok</b> 即将接入</span>
          <span class="ticker-item"><b>Sticky</b> Session</span>
          <span class="ticker-item"><b>Token</b> Level Billing</span>
          <span class="ticker-item"><b>GPT</b> Codex · Responses</span>
          <span class="ticker-item is-soon"><b>Claude</b> 即将接入</span>
          <span class="ticker-item is-soon"><b>Gemini</b> 即将接入</span>
          <span class="ticker-item is-soon"><b>Grok</b> 即将接入</span>
          <span class="ticker-item"><b>Sticky</b> Session</span>
          <span class="ticker-item"><b>Token</b> Level Billing</span>
        </div>
      </div>

      <section id="overview" class="content-block overview-block reveal-section">
        <p class="eyebrow">00 / Overview</p>
        <h2>不是演示环境，<br />是可靠的模型基础设施</h2>
        <p class="lede">从账号池调度到 Token 级计量，EasyHub 承担鉴权、路由与计费，让上游凭证与业务代码彼此解耦。</p>
        <div class="ledger">
          <div><strong>OpenAI</strong><span>协议兼容</span></div>
          <div><strong>SSE</strong><span>流式响应</span></div>
          <div><strong>Token</strong><span>精确计量</span></div>
          <div><strong>24 / 7</strong><span>运行监控</span></div>
        </div>
      </section>

      <section id="capability" class="content-block reveal-section">
        <p class="eyebrow">01 / Capability</p>
        <h2>四层能力，覆盖完整链路</h2>
        <p class="lede">协议、调度、计量与安全各自保持清晰边界，按团队规模逐步开放。</p>
        <div class="capability-rows">
          <article><span>01</span><h3>{{ t('home.features.unifiedGateway') }}</h3><p>{{ t('home.features.unifiedGatewayDesc') }}</p></article>
          <article><span>02</span><h3>{{ t('home.features.multiAccount') }}</h3><p>{{ t('home.features.multiAccountDesc') }}</p></article>
          <article><span>03</span><h3>{{ t('home.features.balanceQuota') }}</h3><p>{{ t('home.features.balanceQuotaDesc') }}</p></article>
          <article><span>04</span><h3>密钥隔离与审计</h3><p>向调用方分发平台 API Key，上游凭证不外泄；请求记录与权限边界便于团队协作。</p></article>
        </div>
      </section>

      <section id="models" class="content-block reveal-section">
        <p class="eyebrow">02 / Matrix</p>
        <h2>{{ t('home.providers.title') }}</h2>
        <p class="lede">{{ t('home.providers.description') }}</p>
        <div class="model-table" role="table" aria-label="支持的模型">
          <div class="model-head" role="row"><span>Provider</span><span>Protocol</span><span>Status</span></div>
          <div role="row"><strong>OpenAI / Codex</strong><code>/v1/responses</code><em><i></i>{{ t('home.providers.supported') }}</em></div>
          <div role="row"><strong>Anthropic Claude</strong><code>/v1/messages</code><em><i></i>{{ t('home.providers.supported') }}</em></div>
          <div role="row"><strong>Google Gemini</strong><code>/v1/chat/completions</code><em><i></i>{{ t('home.providers.supported') }}</em></div>
          <div role="row"><strong>xAI Grok</strong><code>/v1/responses</code><em><i></i>{{ t('home.providers.supported') }}</em></div>
        </div>
      </section>

      <section id="protocol" class="content-block reveal-section">
        <p class="eyebrow">03 / Protocol</p>
        <h2>替换地址，其余照旧</h2>
        <p class="lede">保留熟悉的 OpenAI 请求结构，原生 CLI、自建服务与 IDE 插件都能直接指向网关。</p>
        <div class="console">
          <div class="console-bar"><span>curl</span><span>POST /v1/chat/completions</span></div>
          <pre><span class="dim">curl</span> https://api.easyhub.example<span class="gold">/v1/chat/completions</span> \
  -H <span class="string">"Authorization: Bearer sk-••••"</span> \
  -H <span class="string">"Content-Type: application/json"</span> \
  -d <span class="string">'{ "model": "gpt-5", "messages": [...] }'</span>

<span class="ok">200 OK</span>  <span class="dim">routed → billed → streamed</span> <span class="console-caret" aria-hidden="true"></span></pre>
        </div>
      </section>

      <section id="access" class="content-block reveal-section">
        <p class="eyebrow">04 / Access</p>
        <h2>三步进入生产</h2>
        <p class="lede">从注册到第一个成功响应，接入流程刻意保持简短。</p>
        <div class="access-flow">
          <article><span>Step 01</span><h3>创建 API Key</h3><p>注册后在控制台生成密钥，可按项目或成员拆分。</p></article>
          <article><span>Step 02</span><h3>指向网关地址</h3><p>替换 Base URL 与 Authorization，协议保持不变。</p></article>
          <article><span>Step 03</span><h3>选择模型调用</h3><p>通过 model 指定上游，网关自动完成调度与计量。</p></article>
        </div>
      </section>

      <section class="closing-block reveal-section">
        <h2>以更稳的方式，使用最强的模型</h2>
        <p>登录控制台获取密钥，把 AI 能力接入你的产品与工作流。</p>
        <router-link :to="isAuthenticated ? dashboardPath : '/login'" class="action-fill">
          立即开始
          <Icon name="arrowRight" size="sm" />
        </router-link>
      </section>

      <footer class="footer">
        <span>© {{ currentYear }} EasyHub</span>
        <span>
          <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer">{{ t('home.docs') }}</a>
          <a :href="githubUrl" target="_blank" rel="noopener noreferrer">GitHub</a>
        </span>
      </footer>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import { sanitizeUrl } from '@/utils/url'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

// 保留站点 Logo 的安全归一化，供自定义首页配置与既有安全约束复用。
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})
const githubUrl = 'https://github.com/Wei-Shaw/sub2api'
const docHref = computed(() => docUrl.value || `${githubUrl}#readme`)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())
const landingRoot = ref<HTMLElement | null>(null)
const activeSection = ref('capability')
let revealObserver: IntersectionObserver | null = null
let sectionObserver: IntersectionObserver | null = null

function scrollToSection(id: string): void {
  const section = landingRoot.value?.querySelector<HTMLElement>(`#${id}`)
  if (!section) return
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  section.scrollIntoView({ behavior: reducedMotion ? 'auto' : 'smooth', block: 'start' })
  window.history.replaceState(null, '', `#${id}`)
  activeSection.value = id
}

onMounted(async () => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) appStore.fetchPublicSettings()

  await nextTick()
  const root = landingRoot.value
  if (!root) return

  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches
  const revealSections = root.querySelectorAll<HTMLElement>('.reveal-section')
  if (reducedMotion || !('IntersectionObserver' in window)) {
    revealSections.forEach((section) => section.classList.add('is-visible'))
  } else {
    revealObserver = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return
        entry.target.classList.add('is-visible')
        revealObserver?.unobserve(entry.target)
      })
    }, { threshold: 0.1, rootMargin: '0px 0px -60px 0px' })
    revealSections.forEach((section) => revealObserver?.observe(section))
  }

  sectionObserver = new IntersectionObserver((entries) => {
    const visibleSection = entries.find((entry) => entry.isIntersecting)
    if (visibleSection?.target.id) activeSection.value = visibleSection.target.id
  }, { rootMargin: '-45% 0px -50% 0px' })
  root.querySelectorAll<HTMLElement>('#capability, #models, #protocol, #access').forEach((section) => sectionObserver?.observe(section))
})

onUnmounted(() => {
  revealObserver?.disconnect()
  sectionObserver?.disconnect()
})
</script>

<style scoped>
.landing-shell{--bg:#0b0b0c;--panel:#121214;--panel-2:#17171a;--line:rgba(255,255,255,.09);--line-strong:rgba(255,255,255,.2);--text:#f2f0ec;--muted:#a5a29c;--faint:#6e6b66;--gold:#c8a96a;--live:#7fbf9a;min-height:100vh;background:var(--bg);color:var(--text);font-family:"Space Grotesk","PingFang SC",system-ui,sans-serif;display:grid;grid-template-columns:minmax(360px,42%) 1fr}
.identity-panel{position:sticky;top:0;height:100vh;display:flex;flex-direction:column;justify-content:space-between;padding:clamp(28px,4vw,56px);border-right:1px solid var(--line);overflow:hidden}.identity-glow{position:absolute;inset:auto -30% -35% -20%;height:70%;background:radial-gradient(ellipse at 30% 100%,rgba(200,169,106,.14),transparent 62%);pointer-events:none}.identity-rule{position:absolute;inset:0 auto 0 24px;width:1px;background:linear-gradient(180deg,transparent,var(--line) 20%,var(--line) 80%,transparent)}.identity-panel>*:not(.identity-glow):not(.identity-rule){position:relative}
.brand-mark{display:flex;align-items:center;gap:14px;width:max-content}.brand-logo{width:40px;height:40px;object-fit:contain;border:1px solid var(--line-strong)}.brand-mark strong{display:block;font-size:15px;letter-spacing:.02em}.brand-mark small{display:block;margin-top:2px;color:var(--faint);font-family:ui-monospace,monospace;font-size:10px;letter-spacing:.2em}
.identity-copy{max-width:520px}.service-status,.eyebrow{color:var(--muted);font-family:ui-monospace,monospace;font-size:11px;letter-spacing:.16em;text-transform:uppercase}.service-status{display:flex;align-items:center;gap:10px}.service-status i,.model-table em i{width:6px;height:6px;border-radius:999px;background:var(--live);box-shadow:0 0 0 4px rgba(127,191,154,.12)}.identity-copy h1{margin:34px 0;font-size:clamp(42px,5vw,70px);font-weight:500;letter-spacing:-.055em;line-height:.98}.identity-copy h1 b{color:var(--gold);font-weight:500}.identity-copy>p:not(.service-status){max-width:420px;color:var(--muted);font-size:16px;line-height:1.8}.hero-actions{display:flex;flex-wrap:wrap;gap:10px;margin-top:32px}.action-fill,.action-outline{display:inline-flex;align-items:center;justify-content:center;gap:9px;min-height:46px;padding:0 22px;border:1px solid transparent;font-size:14px;font-weight:600;transition:.25s ease}.action-fill{background:var(--text);color:var(--bg)}.action-fill:hover{background:var(--gold);transform:translateY(-2px)}.action-outline{border-color:var(--line-strong);color:var(--text)}.action-outline:hover{border-color:var(--gold);color:var(--gold)}.action-fill:focus-visible,.action-outline:focus-visible,.top-link:focus-visible,.top-cta:focus-visible{outline:2px solid var(--gold);outline-offset:3px}
.section-index{display:flex;flex-direction:column}.section-index a{display:flex;gap:14px;padding:9px 0;border-top:1px solid var(--line);color:var(--faint);font-size:13px;transition:.25s}.section-index a span{color:var(--gold);font-family:ui-monospace,monospace;font-size:11px}.section-index a:hover{padding-left:8px;color:var(--text)}
.content-panel{min-width:0;background:var(--panel)}.ticker{padding:20px 0;overflow:hidden;border-bottom:1px solid var(--line);mask-image:linear-gradient(90deg,transparent,#000 8%,#000 92%,transparent)}.ticker-track{display:flex;width:max-content;gap:48px;animation:ticker-slide 26s linear infinite}.ticker:hover .ticker-track{animation-play-state:paused}.ticker-item{display:flex;align-items:center;gap:12px;color:var(--faint);font-family:"IBM Plex Mono",ui-monospace,monospace;font-size:.78rem;letter-spacing:.1em;text-transform:uppercase;white-space:nowrap}.ticker-item b{color:var(--text);font-weight:500}.ticker-item::after{content:'';width:4px;height:4px;background:var(--gold)}.ticker-item.is-soon{opacity:.55}.ticker-item.is-soon b{color:var(--faint);font-weight:400}.ticker-item.is-soon::after{background:var(--faint);opacity:.4}
.content-block{padding:clamp(72px,8vw,112px) clamp(24px,5vw,72px);border-bottom:1px solid var(--line);scroll-margin-top:20px}.content-block h2,.closing-block h2{margin:16px 0 22px;font-size:clamp(34px,4vw,58px);font-weight:500;letter-spacing:-.045em;line-height:1.08}.lede{max-width:660px;color:var(--muted);font-size:15px;line-height:1.8}.ledger{display:grid;grid-template-columns:repeat(4,1fr);margin-top:54px;border-top:1px solid var(--line-strong);border-bottom:1px solid var(--line-strong)}.ledger div{padding:26px 18px;border-right:1px solid var(--line)}.ledger div:last-child{border-right:0}.ledger strong{display:block;font-size:clamp(22px,2.5vw,36px);font-weight:500}.ledger span{display:block;margin-top:8px;color:var(--faint);font-size:11px}
.capability-rows{margin-top:46px;border-top:1px solid var(--line-strong)}.capability-rows article{display:grid;grid-template-columns:48px minmax(150px,.7fr) 1.3fr;gap:22px;padding:24px 0;border-bottom:1px solid var(--line);align-items:start}.capability-rows span{color:var(--gold);font-family:ui-monospace,monospace;font-size:11px}.capability-rows h3{font-size:18px;font-weight:500}.capability-rows p,.access-flow p{color:var(--muted);font-size:13px;line-height:1.75}
.model-table{margin-top:42px;border-top:1px solid var(--line-strong)}.model-table>div{display:grid;grid-template-columns:1fr 1.15fr 120px;align-items:center;gap:20px;padding:18px 0;border-bottom:1px solid var(--line)}.model-table .model-head{padding:10px 0;color:var(--faint);font-family:ui-monospace,monospace;font-size:10px;text-transform:uppercase}.model-table strong{font-size:14px;font-weight:500}.model-table code{color:var(--muted);font-size:12px}.model-table em{display:flex;align-items:center;gap:9px;color:var(--muted);font-size:12px;font-style:normal}.model-table em i{width:5px;height:5px;box-shadow:none}
.console{margin-top:42px;border:1px solid var(--line-strong);background:var(--bg);font-family:ui-monospace,monospace}.console-bar{display:flex;justify-content:space-between;padding:12px 16px;border-bottom:1px solid var(--line);color:var(--faint);font-size:10px;text-transform:uppercase}.console pre{overflow:auto;padding:26px;color:#d8d5cf;font-size:12px;line-height:1.9}.console .dim{color:var(--faint)}.console .gold{color:var(--gold)}.console .string{color:#b8c99f}.console .ok{color:var(--live)}
.access-flow{display:grid;grid-template-columns:repeat(3,1fr);margin-top:44px;border-top:1px solid var(--line-strong)}.access-flow article{padding:26px 24px 26px 0;border-right:1px solid var(--line)}.access-flow article:not(:first-child){padding-left:24px}.access-flow article:last-child{border-right:0}.access-flow span{color:var(--gold);font-family:ui-monospace,monospace;font-size:10px}.access-flow h3{margin:16px 0 12px;font-size:18px;font-weight:500}
.closing-block{padding:clamp(64px,9vw,120px) clamp(24px,5vw,72px);text-align:left;border-bottom:1px solid var(--line)}.closing-block p{max-width:44ch;margin:0 0 32px;color:var(--muted);font-size:16px;line-height:1.8}.footer{display:flex;justify-content:space-between;padding:24px clamp(24px,5vw,72px);color:var(--faint);font-size:11px}.footer span:last-child{display:flex;gap:24px}.footer a:hover{color:var(--gold)}
@media(max-width:999px){.landing-shell{display:block}.identity-panel{position:relative;height:auto;min-height:720px;border-right:0;border-bottom:1px solid var(--line)}.section-index{display:none}.content-panel{width:100%}.ledger{grid-template-columns:repeat(2,1fr)}.ledger div:nth-child(2){border-right:0}.capability-rows article{grid-template-columns:38px 1fr}.capability-rows article p{grid-column:2}.model-table>div{grid-template-columns:1fr 1fr}.model-table>div>*:last-child{display:none}}
@media(max-width:640px){.identity-panel{min-height:650px;padding:24px}.identity-copy h1{font-size:44px}.hero-actions{align-items:stretch}.action-fill,.action-outline{flex:1 1 auto}.content-block{padding:64px 24px}.ledger{grid-template-columns:1fr 1fr}.access-flow{display:block}.access-flow article,.access-flow article:not(:first-child){padding:24px 0;border-right:0;border-bottom:1px solid var(--line)}.model-table>div{grid-template-columns:1fr}.model-table code{display:none}.closing-block{padding:72px 24px}.footer{padding:22px 24px}}
@media(min-width:1000px) and (min-height:1000px){.identity-panel{justify-content:flex-start}.identity-panel>.brand-mark.brand-mark{position:absolute;top:42%;left:clamp(28px,4vw,56px)}.identity-panel>.identity-copy.identity-copy{position:absolute;top:49%;left:clamp(28px,4vw,56px);right:clamp(28px,4vw,56px)}.identity-panel>.section-index.section-index{position:absolute;right:clamp(28px,4vw,56px);bottom:5%;left:clamp(28px,4vw,56px)}}
@media(prefers-reduced-motion:reduce){*{scroll-behavior:auto!important;transition-duration:.01ms!important}}

/* 与参考稿一致的动态反馈 */
.service-status i,
.model-table em i { animation: status-beat 2.6s ease-in-out infinite; }
.section-index a.active { padding-left: 8px; color: var(--text); }
.eyebrow { display: flex; align-items: center; gap: 12px; color: var(--gold); }
.eyebrow::after { content: ''; flex: 1; height: 1px; background: var(--line); }
.ledger div { transition: background .3s cubic-bezier(.16, 1, .3, 1); }
.ledger div:hover { background: var(--panel-2); }
.capability-rows article { transition: padding .35s cubic-bezier(.16, 1, .3, 1), border-color .3s; }
.capability-rows article:hover { padding-left: 10px; border-color: var(--line-strong); }
.model-table > div { padding-left: 12px; padding-right: 12px; transition: background .25s; }
.model-table > div:not(.model-head):hover { background: var(--panel-2); }
.console-caret { display: inline-block; width: 7px; height: 13px; background: var(--gold); vertical-align: middle; animation: caret-flick 1.1s step-end infinite; }
.closing-block { background: radial-gradient(ellipse 70% 90% at 90% 10%, rgba(200,169,106,.12), transparent 60%), var(--bg); }
.reveal-section { opacity: 0; transform: translateY(22px); transition: opacity .8s cubic-bezier(.16, 1, .3, 1), transform .8s cubic-bezier(.16, 1, .3, 1); }
.reveal-section.is-visible { opacity: 1; transform: none; }

@keyframes status-beat { 0%, 100% { opacity: 1; } 50% { opacity: .35; } }
@keyframes ticker-slide { from { transform: translateX(0); } to { transform: translateX(-50%); } }
@keyframes caret-flick { 0%, 50% { opacity: 1; } 51%, 100% { opacity: 0; } }

@media(prefers-reduced-motion:reduce) {
  .reveal-section { opacity: 1; transform: none; }
  .ticker-track,
  .service-status i,
  .model-table em i,
  .console-caret { animation: none !important; }
}

/* 字号严格沿用参考稿的比例 */
.brand-glyph {
  display: grid;
  width: 40px;
  height: 40px;
  place-items: center;
  border: 1px solid var(--line-strong);
  color: var(--gold);
  font-family: "IBM Plex Mono", ui-monospace, monospace;
  font-size: 12px;
  font-weight: 500;
}
.brand-mark small,
.service-status,
.eyebrow,
.section-index a span { font-family: "IBM Plex Mono", ui-monospace, monospace; }
.service-status,
.eyebrow { font-size: .68rem; }
.identity-copy h1 { margin: 36px 0; font-size: clamp(2.6rem, 5.2vw, 4.1rem); letter-spacing: -.045em; }
.identity-copy > p:not(.service-status) { font-size: 1rem; }
.content-block { padding-top: clamp(56px, 8vw, 104px); padding-bottom: clamp(56px, 8vw, 104px); }
.content-block h2 { margin-bottom: 14px; max-width: 20ch; font-size: clamp(1.6rem, 3vw, 2.25rem); font-weight: 600; letter-spacing: -.032em; line-height: 1.14; }
.closing-block h2 { max-width: 22ch; margin: 0 0 16px; font-size: clamp(1.7rem, 3.4vw, 2.6rem); font-weight: 500; letter-spacing: -.038em; line-height: 1.1; }
.lede { max-width: 52ch; font-size: .98rem; }

@media(max-width:640px) {
  .identity-copy h1 { font-size: 2.6rem; }
  .content-block { padding-top: 56px; padding-bottom: 56px; }
  .closing-block { padding-top: 64px; padding-bottom: 64px; }
}
</style>
