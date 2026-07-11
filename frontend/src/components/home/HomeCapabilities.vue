<template>
  <div class="home-capabilities">
    <section
      id="capabilities"
      class="home-section"
      aria-labelledby="home-capabilities-title"
    >
      <header class="section-heading">
        <p class="section-eyebrow">{{ t('home.macosCapabilities.eyebrow') }}</p>
        <h2 id="home-capabilities-title">{{ t('home.macosCapabilities.title') }}</h2>
        <p class="section-description">{{ t('home.macosCapabilities.description') }}</p>
      </header>

      <div class="capability-grid">
        <article class="glass-card capability-card">
          <span class="capability-card__icon" aria-hidden="true">
            <Icon name="server" size="lg" :stroke-width="1.8" />
          </span>
          <h3>{{ t('home.features.unifiedGateway') }}</h3>
          <p>{{ t('home.features.unifiedGatewayDesc') }}</p>
        </article>

        <article class="glass-card capability-card">
          <span class="capability-card__icon" aria-hidden="true">
            <Icon name="shield" size="lg" :stroke-width="1.8" />
          </span>
          <h3>{{ t('home.features.multiAccount') }}</h3>
          <p>{{ t('home.features.multiAccountDesc') }}</p>
        </article>

        <article class="glass-card capability-card">
          <span class="capability-card__icon" aria-hidden="true">
            <Icon name="chart" size="lg" :stroke-width="1.8" />
          </span>
          <h3>{{ t('home.features.balanceQuota') }}</h3>
          <p>{{ t('home.features.balanceQuotaDesc') }}</p>
        </article>
      </div>
    </section>

    <section id="models" class="home-section" aria-labelledby="home-models-title">
      <header class="section-heading section-heading--centered">
        <p class="section-eyebrow">{{ t('home.macosCapabilities.modelsEyebrow') }}</p>
        <h2 id="home-models-title">{{ t('home.macosCapabilities.modelsTitle') }}</h2>
        <p class="section-description">{{ t('home.macosCapabilities.modelsDescription') }}</p>
      </header>

      <ul class="glass-card provider-dock" role="list">
        <li class="provider-item">
          <span class="provider-item__mark" aria-hidden="true">C</span>
          <span class="provider-item__copy">
            <strong>{{ t('home.providers.claude') }}</strong>
            <span class="provider-item__status">
              <Icon name="checkCircle" size="xs" :stroke-width="2" aria-hidden="true" />
              {{ t('home.providers.supported') }}
            </span>
          </span>
        </li>
        <li class="provider-item">
          <span class="provider-item__mark provider-item__mark--wide" aria-hidden="true">GPT</span>
          <span class="provider-item__copy">
            <strong>GPT</strong>
            <span class="provider-item__status">
              <Icon name="checkCircle" size="xs" :stroke-width="2" aria-hidden="true" />
              {{ t('home.providers.supported') }}
            </span>
          </span>
        </li>
        <li class="provider-item">
          <span class="provider-item__mark" aria-hidden="true">G</span>
          <span class="provider-item__copy">
            <strong>{{ t('home.providers.gemini') }}</strong>
            <span class="provider-item__status">
              <Icon name="checkCircle" size="xs" :stroke-width="2" aria-hidden="true" />
              {{ t('home.providers.supported') }}
            </span>
          </span>
        </li>
        <li class="provider-item">
          <span class="provider-item__mark" aria-hidden="true">A</span>
          <span class="provider-item__copy">
            <strong>{{ t('home.providers.antigravity') }}</strong>
            <span class="provider-item__status">
              <Icon name="checkCircle" size="xs" :stroke-width="2" aria-hidden="true" />
              {{ t('home.providers.supported') }}
            </span>
          </span>
        </li>
      </ul>
    </section>

    <section class="home-section workflow" aria-labelledby="home-workflow-title">
      <header class="section-heading section-heading--centered">
        <p class="section-eyebrow">{{ t('home.macosCapabilities.workflowEyebrow') }}</p>
        <h2 id="home-workflow-title">{{ t('home.macosCapabilities.workflowTitle') }}</h2>
        <p class="section-description">{{ t('home.macosCapabilities.workflowDescription') }}</p>
      </header>

      <ol class="workflow-grid">
        <li class="glass-card workflow-step">
          <span class="workflow-step__number" aria-hidden="true">01</span>
          <span class="workflow-step__icon" aria-hidden="true">
            <Icon name="key" size="md" :stroke-width="1.8" />
          </span>
          <h3>{{ t('home.macosCapabilities.steps.keyTitle') }}</h3>
          <p>{{ t('home.macosCapabilities.steps.keyDesc') }}</p>
        </li>
        <li class="glass-card workflow-step">
          <span class="workflow-step__number" aria-hidden="true">02</span>
          <span class="workflow-step__icon" aria-hidden="true">
            <Icon name="link" size="md" :stroke-width="1.8" />
          </span>
          <h3>{{ t('home.macosCapabilities.steps.connectTitle') }}</h3>
          <p>{{ t('home.macosCapabilities.steps.connectDesc') }}</p>
        </li>
        <li class="glass-card workflow-step">
          <span class="workflow-step__number" aria-hidden="true">03</span>
          <span class="workflow-step__icon" aria-hidden="true">
            <Icon name="chart" size="md" :stroke-width="1.8" />
          </span>
          <h3>{{ t('home.macosCapabilities.steps.observeTitle') }}</h3>
          <p>{{ t('home.macosCapabilities.steps.observeDesc') }}</p>
        </li>
      </ol>
    </section>

    <section class="glass-card closing-cta" aria-labelledby="home-closing-cta-title">
      <div class="closing-cta__copy">
        <p class="section-eyebrow">{{ t('home.macosCapabilities.ctaEyebrow') }}</p>
        <h2 id="home-closing-cta-title">{{ t('home.macosCapabilities.ctaTitle') }}</h2>
        <p>{{ t('home.macosCapabilities.ctaDescription') }}</p>
      </div>
      <RouterLink
        :to="isAuthenticated ? dashboardPath : '/login'"
        class="closing-cta__link"
      >
        <span>{{ t(isAuthenticated ? 'home.goToDashboard' : 'home.getStarted') }}</span>
        <Icon name="arrowRight" size="sm" :stroke-width="2" aria-hidden="true" />
      </RouterLink>
    </section>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  isAuthenticated: boolean
  dashboardPath: string
}>()

const { t } = useI18n()
</script>

<style scoped>
.home-capabilities {
  width: min(calc(100% - 40px), 1200px);
  margin-inline: auto;
  color: var(--home-text, #1f2937);
}

.home-section {
  padding-block: clamp(4.5rem, 9vw, 8rem);
  scroll-margin-top: 6rem;
}

.section-heading {
  max-width: 46rem;
  margin-bottom: clamp(2rem, 5vw, 3.5rem);
}

.section-heading--centered {
  margin-inline: auto;
  text-align: center;
}

.section-eyebrow {
  margin: 0 0 0.75rem;
  color: var(--home-accent, #d45f28);
  font-size: 0.75rem;
  font-weight: 750;
  letter-spacing: 0.14em;
  line-height: 1.4;
  text-transform: uppercase;
}

.section-heading h2,
.closing-cta h2 {
  margin: 0;
  color: var(--home-text, #1f2937);
  font-size: clamp(2rem, 5vw, 3.75rem);
  font-weight: 760;
  letter-spacing: -0.05em;
  line-height: 1.02;
  text-wrap: balance;
}

.section-description {
  max-width: 42rem;
  margin: 1rem 0 0;
  color: var(--home-muted, #5f665f);
  font-size: clamp(1rem, 1.5vw, 1.125rem);
  line-height: 1.75;
}

.section-heading--centered .section-description {
  margin-inline: auto;
}

.capability-grid,
.workflow-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 1rem;
}

.glass-card {
  border: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  border-radius: 30px;
  background: rgb(255 255 255 / 72%);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 70%),
    0 5px 14px rgb(56 43 31 / 6%),
    0 24px 64px rgb(56 43 31 / 8%);
  backdrop-filter: blur(24px);
  -webkit-backdrop-filter: blur(24px);
  transition:
    transform 180ms ease,
    border-color 180ms ease,
    box-shadow 180ms ease;
}

:global(.dark) .glass-card {
  background: rgb(15 23 42 / 65%);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 8%),
    0 5px 16px rgb(0 0 0 / 20%),
    0 28px 72px rgb(0 0 0 / 28%);
}

.glass-card:hover {
  transform: translateY(-2px);
  border-color: var(--home-accent, #d45f28);
  box-shadow:
    inset 0 1px 0 rgb(255 255 255 / 72%),
    0 6px 16px rgb(56 43 31 / 8%),
    0 28px 68px rgb(212 95 40 / 16%);
}

.capability-card {
  min-width: 0;
  padding: clamp(1.5rem, 3vw, 2rem);
}

.capability-card__icon,
.workflow-step__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 3.25rem;
  height: 3.25rem;
  border: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  border-radius: 16px;
  background: var(--home-accent-soft, rgb(212 95 40 / 11%));
  color: var(--home-accent, #d45f28);
}

.capability-card__icon {
  margin-bottom: 2.5rem;
}

.capability-card h3,
.workflow-step h3 {
  margin: 0;
  color: var(--home-text, #1f2937);
  font-size: 1.25rem;
  font-weight: 720;
  letter-spacing: -0.025em;
  line-height: 1.3;
}

.capability-card p,
.workflow-step p {
  margin: 0.75rem 0 0;
  color: var(--home-muted, #5f665f);
  line-height: 1.7;
}

.provider-dock {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 0;
  max-width: 68rem;
  margin: 0 auto;
  padding: 0.75rem;
  list-style: none;
}

.provider-item {
  display: flex;
  align-items: center;
  gap: 0.875rem;
  min-width: 0;
  padding: 1rem;
  border-radius: 20px;
}

.provider-item + .provider-item {
  border-top: 1px solid var(--home-border, rgb(31 41 55 / 12%));
}

.provider-item__mark {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  width: 2.75rem;
  height: 2.75rem;
  border-radius: 14px;
  background: var(--home-accent-soft, rgb(212 95 40 / 11%));
  color: var(--home-accent, #d45f28);
  font-size: 1rem;
  font-weight: 800;
  letter-spacing: -0.04em;
}

.provider-item__mark--wide {
  font-size: 0.625rem;
  letter-spacing: -0.02em;
}

.provider-item__copy {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 0.25rem;
}

.provider-item__copy strong {
  overflow: hidden;
  color: var(--home-text, #1f2937);
  font-size: 0.9375rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.provider-item__status {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  color: var(--home-muted, #5f665f);
  font-size: 0.75rem;
  font-weight: 650;
}

.workflow {
  padding-top: clamp(2rem, 5vw, 4.5rem);
}

.workflow-grid {
  margin: 0;
  padding: 0;
  list-style: none;
  counter-reset: workflow-step;
}

.workflow-step {
  position: relative;
  min-width: 0;
  padding: clamp(1.5rem, 3vw, 2rem);
  overflow: hidden;
}

.workflow-step__number {
  position: absolute;
  top: 1.5rem;
  right: 1.5rem;
  color: var(--home-border-strong, rgb(31 41 55 / 24%));
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.workflow-step__icon {
  width: 2.75rem;
  height: 2.75rem;
  margin-bottom: 2rem;
  border-radius: 14px;
}

.closing-cta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 2rem;
  margin-block: clamp(2rem, 6vw, 5rem) clamp(5rem, 10vw, 8rem);
  padding: clamp(1.75rem, 5vw, 3.5rem);
  overflow: hidden;
}

.closing-cta__copy {
  max-width: 43rem;
}

.closing-cta h2 {
  font-size: clamp(1.8rem, 4vw, 3.25rem);
}

.closing-cta__copy > p:last-child {
  margin: 1rem 0 0;
  color: var(--home-muted, #5f665f);
  line-height: 1.7;
}

.closing-cta__link {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  justify-content: center;
  gap: 0.625rem;
  min-height: 3rem;
  padding: 0.75rem 1.25rem;
  border: 1px solid var(--home-accent, #d45f28);
  border-radius: 999px;
  background: var(--home-accent, #d45f28);
  color: var(--home-accent-contrast, #ffffff);
  font-size: 0.9375rem;
  font-weight: 720;
  line-height: 1;
  text-decoration: none;
  cursor: pointer;
  transition:
    transform 180ms ease,
    box-shadow 180ms ease,
    filter 180ms ease;
}

.closing-cta__link:hover {
  transform: translateY(-2px);
  box-shadow: 0 14px 32px -16px var(--home-accent, #d45f28);
  filter: brightness(1.04);
}

.closing-cta__link:focus-visible {
  outline: 3px solid var(--home-accent-soft, rgb(212 95 40 / 25%));
  outline-offset: 4px;
}

@media (min-width: 768px) {
  .capability-grid,
  .workflow-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .provider-dock {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .provider-item:nth-child(even) {
    border-left: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  }

  .provider-item + .provider-item {
    border-top: 0;
  }

  .provider-item:nth-child(n + 3) {
    border-top: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  }
}

@media (min-width: 1024px) {
  .capability-grid,
  .workflow-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .provider-dock {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }

  .provider-item + .provider-item {
    border-top: 0;
    border-left: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  }
}

@media (max-width: 640px) {
  .home-capabilities {
    width: calc(100% - 24px);
  }

  .closing-cta {
    align-items: stretch;
    flex-direction: column;
  }

  .closing-cta__link {
    width: 100%;
  }
}

@media (prefers-reduced-motion: reduce) {
  .glass-card,
  .closing-cta__link {
    transition: none;
  }

  .glass-card:hover,
  .closing-cta__link:hover {
    transform: none;
  }
}
</style>
