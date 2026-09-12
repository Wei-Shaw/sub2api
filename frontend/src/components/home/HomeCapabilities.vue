<template>
  <div class="home-capabilities">
    <section id="capabilities" class="home-section" aria-labelledby="home-capabilities-title">
      <header class="section-heading">
        <p class="section-kicker">{{ t('home.macosCapabilities.eyebrow') }}</p>
        <h2 id="home-capabilities-title">{{ t('home.macosCapabilities.title') }}</h2>
        <p class="section-description">{{ t('home.macosCapabilities.description') }}</p>
      </header>

      <div class="glass-panel capability-stage" role="list">
        <article class="capability-item" role="listitem">
          <div class="capability-item__topline">
            <span class="feature-symbol" aria-hidden="true">
              <Icon name="server" size="md" :stroke-width="1.8" />
            </span>
            <span class="capability-item__signal" aria-hidden="true"></span>
          </div>
          <h3>{{ t('home.features.unifiedGateway') }}</h3>
          <p>{{ t('home.features.unifiedGatewayDesc') }}</p>
        </article>

        <article class="capability-item" role="listitem">
          <div class="capability-item__topline">
            <span class="feature-symbol" aria-hidden="true">
              <Icon name="shield" size="md" :stroke-width="1.8" />
            </span>
            <span class="capability-item__signal" aria-hidden="true"></span>
          </div>
          <h3>{{ t('home.features.multiAccount') }}</h3>
          <p>{{ t('home.features.multiAccountDesc') }}</p>
        </article>

        <article class="capability-item" role="listitem">
          <div class="capability-item__topline">
            <span class="feature-symbol" aria-hidden="true">
              <Icon name="chart" size="md" :stroke-width="1.8" />
            </span>
            <span class="capability-item__signal" aria-hidden="true"></span>
          </div>
          <h3>{{ t('home.features.balanceQuota') }}</h3>
          <p>{{ t('home.features.balanceQuotaDesc') }}</p>
        </article>
      </div>
    </section>

    <section id="models" class="home-section models-section" aria-labelledby="home-models-title">
      <header class="section-heading section-heading--centered">
        <h2 id="home-models-title">{{ t('home.macosCapabilities.modelsTitle') }}</h2>
        <p class="section-description">{{ t('home.macosCapabilities.modelsDescription') }}</p>
      </header>

      <ul class="glass-panel provider-dock" role="list">
        <li v-for="provider in providers" :key="provider.id" class="provider-item">
          <span class="provider-item__logo">
            <ProviderLogo :provider="provider.id" />
          </span>
          <span class="provider-item__copy">
            <strong>{{ provider.label }}</strong>
            <span class="provider-item__status">
              <span class="provider-item__status-dot" aria-hidden="true"></span>
              {{ t('home.providers.supported') }}
            </span>
          </span>
        </li>
      </ul>
    </section>

    <section class="home-section workflow" aria-labelledby="home-workflow-title">
      <header class="section-heading section-heading--centered">
        <h2 id="home-workflow-title">{{ t('home.macosCapabilities.workflowTitle') }}</h2>
        <p class="section-description">{{ t('home.macosCapabilities.workflowDescription') }}</p>
      </header>

      <ol class="workflow-list">
        <li class="workflow-step">
          <span class="workflow-step__number" aria-hidden="true">01</span>
          <div>
            <h3>{{ t('home.macosCapabilities.steps.keyTitle') }}</h3>
            <p>{{ t('home.macosCapabilities.steps.keyDesc') }}</p>
          </div>
          <Icon name="key" size="md" :stroke-width="1.8" aria-hidden="true" />
        </li>
        <li class="workflow-step">
          <span class="workflow-step__number" aria-hidden="true">02</span>
          <div>
            <h3>{{ t('home.macosCapabilities.steps.connectTitle') }}</h3>
            <p>{{ t('home.macosCapabilities.steps.connectDesc') }}</p>
          </div>
          <Icon name="link" size="md" :stroke-width="1.8" aria-hidden="true" />
        </li>
        <li class="workflow-step">
          <span class="workflow-step__number" aria-hidden="true">03</span>
          <div>
            <h3>{{ t('home.macosCapabilities.steps.observeTitle') }}</h3>
            <p>{{ t('home.macosCapabilities.steps.observeDesc') }}</p>
          </div>
          <Icon name="chart" size="md" :stroke-width="1.8" aria-hidden="true" />
        </li>
      </ol>
    </section>

    <section class="glass-panel closing-cta" aria-labelledby="home-closing-cta-title">
      <div class="closing-cta__copy">
        <h2 id="home-closing-cta-title">{{ t('home.macosCapabilities.ctaTitle') }}</h2>
        <p>{{ t('home.macosCapabilities.ctaDescription') }}</p>
      </div>
      <RouterLink :to="isAuthenticated ? dashboardPath : '/login'" class="closing-cta__link">
        <span>{{ t(isAuthenticated ? 'home.goToDashboard' : 'home.getStarted') }}</span>
        <Icon name="arrowRight" size="sm" :stroke-width="2" aria-hidden="true" />
      </RouterLink>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'
import ProviderLogo from '@/components/home/ProviderLogo.vue'

defineProps<{
  isAuthenticated: boolean
  dashboardPath: string
}>()

const { t } = useI18n()
const providers = computed(() => [
  { id: 'claude' as const, label: t('home.providers.claude') },
  { id: 'openai' as const, label: 'GPT' },
  { id: 'gemini' as const, label: t('home.providers.gemini') },
  { id: 'antigravity' as const, label: t('home.providers.antigravity') }
])
</script>

<style scoped>
.home-capabilities {
  width: min(calc(100% - 40px), 1200px);
  margin-inline: auto;
  color: var(--home-text);
}

.home-section {
  padding-block: clamp(4.5rem, 9vw, 8rem);
  scroll-margin-top: 6rem;
}

.section-heading { max-width: 46rem; margin-bottom: clamp(2rem, 5vw, 3.5rem); }
.section-heading--centered { margin-inline: auto; text-align: center; }
.section-kicker { margin: 0 0 .85rem; color: var(--home-accent); font-size: .82rem; font-weight: 720; line-height: 1.4; }

.section-heading h2,
.closing-cta h2 {
  margin: 0;
  color: var(--home-text);
  font-size: clamp(2rem, 5vw, 3.75rem);
  font-weight: 760;
  letter-spacing: -.035em;
  line-height: 1.04;
  text-wrap: balance;
}

.section-description { max-width: 42rem; margin: 1rem 0 0; color: var(--home-muted); font-size: clamp(1rem, 1.5vw, 1.125rem); line-height: 1.75; text-wrap: pretty; }
.section-heading--centered .section-description { margin-inline: auto; }

.glass-panel {
  background: var(--home-glass);
  border: 1px solid var(--home-glass-border);
  border-radius: 16px;
  box-shadow: inset 0 1px 0 var(--home-glass-highlight), 0 6px 8px rgb(7 16 36 / 8%);
  backdrop-filter: blur(24px) saturate(135%);
  -webkit-backdrop-filter: blur(24px) saturate(135%);
}

.capability-stage { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); overflow: hidden; }
.capability-item { min-width: 0; padding: clamp(1.5rem, 3vw, 2.2rem); transition: background-color 200ms ease, transform 200ms cubic-bezier(.22, 1, .36, 1); }
.capability-item + .capability-item { border-left: 1px solid var(--home-glass-divider); }
.capability-item:hover { background: var(--home-glass-hover); transform: translateY(-2px); }
.capability-item__topline { display: flex; align-items: center; justify-content: space-between; margin-bottom: 2.75rem; }

.feature-symbol { display: inline-flex; width: 2.5rem; height: 2.5rem; align-items: center; justify-content: center; color: var(--home-accent); background: var(--home-accent-soft); border-radius: 50%; }
.capability-item__signal { width: 3.25rem; height: 1px; background: linear-gradient(90deg, var(--home-accent), transparent); opacity: .55; transform-origin: left; transition: opacity 200ms ease, transform 200ms cubic-bezier(.22, 1, .36, 1); }
.capability-item:hover .capability-item__signal { opacity: .9; transform: scaleX(1.18); }
.capability-item h3,
.workflow-step h3 { margin: 0; color: var(--home-text); font-size: 1.18rem; font-weight: 720; letter-spacing: -.02em; line-height: 1.3; }
.capability-item p,
.workflow-step p { margin: .75rem 0 0; color: var(--home-muted); line-height: 1.7; text-wrap: pretty; }

.models-section { position: relative; }
.models-section::before { position: absolute; z-index: -1; top: 50%; left: 50%; width: min(72vw, 44rem); height: 15rem; background: var(--home-accent-soft); border-radius: 50%; content: ""; filter: blur(76px); opacity: .7; transform: translate(-50%, -50%); }

.provider-dock { display: flex; max-width: 68rem; margin: 0 auto; padding: .55rem; list-style: none; }
.provider-item { display: flex; flex: 1 1 0; align-items: center; gap: .8rem; min-width: 0; padding: .85rem 1rem; border-radius: 12px; transition: background-color 180ms ease, transform 180ms cubic-bezier(.22, 1, .36, 1); }
.provider-item:hover { background: var(--home-glass-hover); transform: translateY(-2px); }
.provider-item + .provider-item { border-left: 1px solid var(--home-glass-divider); }
.provider-item__logo { display: grid; width: 2.75rem; height: 2.75rem; flex: 0 0 auto; place-items: center; color: var(--home-text); background: var(--home-glass-sheen); border-radius: 12px; transition: transform 180ms cubic-bezier(.22, 1, .36, 1); }
.provider-item:hover .provider-item__logo { transform: scale(1.05); }
.provider-item__copy { display: flex; min-width: 0; flex-direction: column; gap: .28rem; }
.provider-item__copy strong { overflow: hidden; color: var(--home-text); font-size: .94rem; text-overflow: ellipsis; white-space: nowrap; }
.provider-item__status { display: inline-flex; align-items: center; gap: .38rem; color: var(--home-muted); font-size: .74rem; font-weight: 620; }
.provider-item__status-dot { width: 6px; height: 6px; background: var(--home-success); border-radius: 50%; box-shadow: 0 0 0 3px var(--home-success-soft); }

.workflow { padding-top: clamp(2rem, 5vw, 4.5rem); }
.workflow-list { max-width: 62rem; margin: 0 auto; padding: 0; list-style: none; }
.workflow-step { display: grid; grid-template-columns: 3.5rem minmax(0, 1fr) auto; gap: 1.25rem; align-items: start; padding: 1.6rem .5rem; border-top: 1px solid var(--home-glass-divider); }
.workflow-step:last-child { border-bottom: 1px solid var(--home-glass-divider); }
.workflow-step__number { color: var(--home-accent); font: 700 .78rem/1.5 ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; }
.workflow-step > svg { margin-top: .1rem; color: var(--home-faint); }

.closing-cta { display: flex; align-items: center; justify-content: space-between; gap: 2rem; margin-block: clamp(2rem, 6vw, 5rem) clamp(5rem, 10vw, 8rem); padding: clamp(1.75rem, 5vw, 3.5rem); overflow: hidden; }
.closing-cta__copy { max-width: 43rem; }
.closing-cta h2 { font-size: clamp(1.8rem, 4vw, 3.25rem); }
.closing-cta__copy p { margin: 1rem 0 0; color: var(--home-muted); line-height: 1.7; }
.closing-cta__link { display: inline-flex; min-height: 3rem; flex: 0 0 auto; align-items: center; justify-content: center; gap: .625rem; padding: .75rem 1.25rem; color: var(--home-accent-contrast); background: var(--home-accent); border-radius: 999px; font-size: .94rem; font-weight: 720; line-height: 1; text-decoration: none; transition: transform 180ms cubic-bezier(.22, 1, .36, 1), filter 180ms ease; }
.closing-cta__link:hover { transform: translateY(-2px); filter: brightness(1.06); }
.closing-cta__link:focus-visible { outline: 3px solid var(--home-accent-soft); outline-offset: 4px; }

@media (max-width: 900px) {
  .capability-stage { grid-template-columns: 1fr; }
  .capability-item + .capability-item { border-top: 1px solid var(--home-glass-divider); border-left: 0; }
  .provider-dock { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .provider-item + .provider-item { border-left: 0; }
  .provider-item:nth-child(even) { border-left: 1px solid var(--home-glass-divider); }
  .provider-item:nth-child(n + 3) { border-top: 1px solid var(--home-glass-divider); }
}

@media (max-width: 640px) {
  .home-capabilities { width: calc(100% - 24px); }
  .provider-dock { grid-template-columns: 1fr; }
  .provider-item:nth-child(even) { border-left: 0; }
  .provider-item:nth-child(n + 2) { border-top: 1px solid var(--home-glass-divider); }
  .workflow-step { grid-template-columns: 2.5rem minmax(0, 1fr); gap: .8rem; }
  .workflow-step > svg { display: none; }
  .closing-cta { align-items: stretch; flex-direction: column; }
  .closing-cta__link { width: 100%; box-sizing: border-box; }
}

@media (prefers-reduced-motion: reduce) {
  .provider-item,
  .provider-item__logo,
  .capability-item,
  .capability-item__signal,
  .closing-cta__link { transition: none; }
  .capability-item:hover,
  .provider-item:hover,
  .provider-item:hover .provider-item__logo,
  .closing-cta__link:hover { transform: none; }
}
</style>
