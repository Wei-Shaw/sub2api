<template>
  <section class="home-hero" aria-labelledby="home-hero-title">
    <div class="home-hero__environment" aria-hidden="true">
      <span class="home-hero__orb home-hero__orb--blue"></span>
      <span class="home-hero__orb home-hero__orb--violet"></span>
      <span class="home-hero__orb home-hero__orb--cyan"></span>
      <span class="home-hero__dots"></span>
    </div>

    <div class="home-hero__inner">
      <div class="home-hero__copy">
        <p class="home-hero__eyebrow">
          <Icon name="sparkles" size="sm" aria-hidden="true" />
          <span>{{ t('home.macosHero.eyebrow') }}</span>
        </p>

        <h1 id="home-hero-title" class="home-hero__title">{{ siteName }}</h1>
        <p class="home-hero__subtitle">{{ siteSubtitle }}</p>
        <p class="home-hero__description">{{ t('home.macosHero.description') }}</p>

        <div class="home-hero__actions">
          <router-link class="home-hero__cta home-hero__cta--primary" :to="primaryCtaPath">
            <span>{{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}</span>
            <Icon name="arrowRight" size="sm" aria-hidden="true" />
          </router-link>

          <a
            v-if="docUrl"
            class="home-hero__cta home-hero__cta--secondary"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
          >
            <Icon name="book" size="sm" aria-hidden="true" />
            <span>{{ t('home.docs') }}</span>
            <Icon name="externalLink" size="xs" aria-hidden="true" />
          </a>
        </div>
      </div>

      <div class="home-hero__showcase">
        <HomeApiPreview :api-base-url="apiBaseUrl" />
      </div>
    </div>
  </section>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Icon } from '@/components/icons'
import HomeApiPreview from '@/components/home/HomeApiPreview.vue'

const props = defineProps<{
  siteName: string
  siteSubtitle: string
  apiBaseUrl: string
  docUrl: string
  isAuthenticated: boolean
  dashboardPath: string
}>()

const { t } = useI18n()

const primaryCtaPath = computed(() => (props.isAuthenticated ? props.dashboardPath : '/login'))
</script>

<style scoped>
.home-hero {
  position: relative;
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  overflow: hidden;
  padding: 104px 0 88px;
  color: var(--home-text);
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", "SF Pro Text", "Helvetica Neue", Arial, sans-serif;
  isolation: isolate;
}

.home-hero__environment {
  position: absolute;
  z-index: -1;
  inset: 0;
  overflow: hidden;
  pointer-events: none;
}

.home-hero__dots {
  position: absolute;
  inset: 0;
  opacity: 0.35;
  background-image: radial-gradient(color-mix(in srgb, var(--home-muted) 24%, transparent) 1px, transparent 1px);
  background-size: 24px 24px;
  mask-image: linear-gradient(to bottom, transparent, black 20%, black 75%, transparent);
}

.home-hero__orb {
  position: absolute;
  width: clamp(240px, 34vw, 520px);
  aspect-ratio: 1;
  border-radius: 50%;
  filter: blur(80px);
  opacity: 0.18;
  animation: orb-drift 14s ease-in-out infinite alternate;
}

.home-hero__orb--blue {
  top: 4%;
  left: -12%;
  background: #3b82f6;
}

.home-hero__orb--violet {
  top: 18%;
  right: -10%;
  background: #8b5cf6;
  animation-delay: -5s;
}

.home-hero__orb--cyan {
  right: 30%;
  bottom: -28%;
  background: #06b6d4;
  animation-delay: -9s;
}

.home-hero__inner {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 64px;
  align-items: center;
  box-sizing: border-box;
  width: min(100%, 1280px);
  min-width: 0;
  margin-inline: auto;
  padding-inline: clamp(20px, 4vw, 40px);
}

.home-hero__copy,
.home-hero__showcase {
  min-width: 0;
}

.home-hero__copy {
  max-width: 680px;
}

.home-hero__eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  margin: 0 0 24px;
  color: var(--home-accent);
  font-size: 0.76rem;
  font-weight: 760;
  letter-spacing: 0.14em;
  line-height: 1.3;
  text-transform: uppercase;
}

.home-hero__title {
  max-width: 14ch;
  margin: 0;
  color: var(--home-text);
  font-size: clamp(3rem, 5.5vw, 5.2rem);
  font-weight: 780;
  letter-spacing: -0.07em;
  line-height: 0.92;
  overflow-wrap: anywhere;
  text-wrap: balance;
}

.home-hero__subtitle {
  max-width: 26ch;
  margin: 26px 0 0;
  color: var(--home-text);
  font-size: clamp(1.2rem, 2vw, 1.65rem);
  font-weight: 620;
  letter-spacing: -0.025em;
  line-height: 1.35;
  text-wrap: balance;
}

.home-hero__description {
  max-width: 58ch;
  margin: 18px 0 0;
  color: var(--home-muted);
  font-size: clamp(0.98rem, 1.2vw, 1.08rem);
  line-height: 1.75;
}

.home-hero__actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 34px;
}

.home-hero__cta {
  display: inline-flex;
  min-height: 48px;
  align-items: center;
  justify-content: center;
  gap: 9px;
  box-sizing: border-box;
  padding: 12px 18px;
  border: 1px solid transparent;
  border-radius: 14px;
  cursor: pointer;
  font-size: 0.92rem;
  font-weight: 720;
  line-height: 1;
  text-decoration: none;
}

.home-hero__cta--primary {
  color: #ffffff;
  background: var(--home-accent);
  box-shadow: 0 14px 32px color-mix(in srgb, var(--home-accent) 25%, transparent);
}

.home-hero__cta--primary:hover {
  background: color-mix(in srgb, var(--home-accent) 88%, #000000);
}

.home-hero__cta--secondary {
  color: var(--home-text);
  background: color-mix(in srgb, var(--home-panel) 88%, transparent);
  border-color: var(--home-border);
}

.home-hero__cta--secondary:hover {
  color: var(--home-accent);
  border-color: color-mix(in srgb, var(--home-accent) 45%, var(--home-border));
}

.home-hero__cta:focus-visible {
  outline: 3px solid color-mix(in srgb, var(--home-accent) 45%, transparent);
  outline-offset: 3px;
}

.home-hero__showcase {
  display: flex;
  justify-content: center;
  perspective: 1200px;
}

.product-window {
  position: relative;
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  overflow: hidden;
  background: color-mix(in srgb, var(--home-panel) 80%, transparent);
  border: 1px solid color-mix(in srgb, #ffffff 44%, var(--home-border));
  border-radius: 30px;
  box-shadow:
    0 70px 80px -44px color-mix(in srgb, #172554 48%, transparent),
    0 24px 50px -30px color-mix(in srgb, var(--home-text) 34%, transparent),
    inset 0 1px 0 color-mix(in srgb, #ffffff 70%, transparent);
  transform: translateZ(0);
  transition:
    transform 260ms ease,
    box-shadow 260ms ease;
  backdrop-filter: blur(24px) saturate(135%);
}

.product-window:hover {
  box-shadow:
    0 78px 88px -44px color-mix(in srgb, #172554 54%, transparent),
    0 28px 54px -30px color-mix(in srgb, var(--home-text) 38%, transparent),
    inset 0 1px 0 color-mix(in srgb, #ffffff 78%, transparent);
  transform: translateY(-4px);
}

.product-window__titlebar {
  display: grid;
  grid-template-columns: 1fr auto 1fr;
  align-items: center;
  gap: 12px;
  min-height: 70px;
  padding: 0 22px;
  color: var(--home-muted);
  background: color-mix(in srgb, var(--home-panel) 72%, transparent);
  border-bottom: 1px solid color-mix(in srgb, #ffffff 38%, var(--home-border));
  box-shadow: inset 0 1px 0 color-mix(in srgb, #ffffff 76%, transparent);
  backdrop-filter: blur(26px) saturate(150%);
}

.product-window__traffic-lights {
  display: flex;
  align-items: center;
  gap: 8px;
}

.product-window__traffic-light {
  width: 12px;
  height: 12px;
  border: 1px solid rgb(0 0 0 / 12%);
  border-radius: 50%;
  box-shadow: inset 0 -1px 1px rgb(0 0 0 / 12%);
}

.product-window__traffic-light--red {
  background: #ff5f57;
}

.product-window__traffic-light--yellow {
  background: #febc2e;
}

.product-window__traffic-light--green {
  background: #28c840;
}

.product-window__title {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  min-width: 0;
  font-size: 0.78rem;
  font-weight: 680;
  letter-spacing: -0.01em;
  white-space: nowrap;
}

.product-window__titlebar-spacer {
  min-width: 68px;
}

.product-window__provider-tabs {
  display: flex;
  gap: 8px;
  min-width: 0;
  overflow-x: auto;
  padding: 14px 18px;
  background: color-mix(in srgb, var(--home-panel) 82%, transparent);
  border-bottom: 1px solid var(--home-border);
  scrollbar-width: none;
}

.product-window__provider-tabs::-webkit-scrollbar {
  display: none;
}

.product-window__provider-tab {
  flex: 0 0 auto;
  padding: 7px 11px;
  color: var(--home-muted);
  border: 1px solid transparent;
  border-radius: 999px;
  cursor: default;
  font-size: 0.72rem;
  font-weight: 680;
  user-select: none;
}

.product-window__provider-tab--active {
  color: var(--home-text);
  background: var(--home-accent-soft);
  border-color: color-mix(in srgb, var(--home-accent) 28%, var(--home-border));
}

.product-window__terminal {
  position: relative;
  min-width: 0;
  overflow: hidden;
  padding: 20px;
  color: #e5edf8;
  background:
    linear-gradient(135deg, rgb(255 255 255 / 4%), transparent 44%),
    #10141e;
  font-family: "SFMono-Regular", "SF Mono", ui-monospace, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
}

.product-window__terminal-line {
  position: absolute;
  z-index: 2;
  top: 0;
  left: 0;
  width: 42%;
  height: 1px;
  opacity: 0.7;
  background: linear-gradient(90deg, transparent, #67e8f9, transparent);
  animation: terminal-scan 5s ease-in-out infinite;
  pointer-events: none;
}

.product-window__endpoint-row {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 12px;
  align-items: center;
  min-width: 0;
  margin-bottom: 16px;
  padding: 10px 12px;
  background: rgb(255 255 255 / 5%);
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 11px;
}

.product-window__terminal-label {
  color: #94a3b8;
  font-size: 0.66rem;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.product-window__endpoint {
  display: block;
  min-width: 0;
  overflow: hidden;
  color: #a5f3fc;
  font-family: inherit;
  font-size: 0.72rem;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.product-window__exchange {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  min-width: 0;
}

.product-window__code-panel {
  min-width: 0;
  overflow: hidden;
  background: rgb(255 255 255 / 3%);
  border: 1px solid rgb(255 255 255 / 8%);
  border-radius: 13px;
}

.product-window__code-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  color: #94a3b8;
  border-bottom: 1px solid rgb(255 255 255 / 8%);
  font-size: 0.67rem;
  font-weight: 700;
  letter-spacing: 0.07em;
  text-transform: uppercase;
}

.product-window__method {
  color: #c4b5fd;
}

.product-window__success-code {
  color: #86efac;
}

.product-window pre {
  box-sizing: border-box;
  width: 100%;
  max-width: 100%;
  margin: 0;
  overflow-x: auto;
  padding: 13px;
  color: #dbeafe;
  font-family: inherit;
  font-size: clamp(0.65rem, 1vw, 0.72rem);
  line-height: 1.65;
  scrollbar-color: rgb(148 163 184 / 35%) transparent;
  scrollbar-width: thin;
  white-space: pre;
}

.product-window code {
  font-family: inherit;
}

.product-window__status {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 14px;
  color: #a7f3d0;
  font-size: 0.68rem;
  font-weight: 650;
}

.product-window__status-dot {
  width: 7px;
  height: 7px;
  background: #34d399;
  border-radius: 50%;
  box-shadow: 0 0 0 0 rgb(52 211 153 / 45%);
  animation: status-pulse 2.8s ease-out infinite;
}

.product-window__dock {
  display: flex;
  gap: 8px;
  min-width: 0;
  overflow-x: auto;
  padding: 16px 18px 18px;
  background: color-mix(in srgb, var(--home-panel) 88%, transparent);
  border-top: 1px solid color-mix(in srgb, #ffffff 34%, var(--home-border));
  scrollbar-width: none;
}

.product-window__dock::-webkit-scrollbar {
  display: none;
}

.product-window__dock-item {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 7px;
  padding: 7px 9px;
  color: var(--home-muted);
  cursor: default;
  font-size: 0.69rem;
  font-weight: 680;
  user-select: none;
}

.product-window__dock-mark {
  display: inline-grid;
  width: 25px;
  height: 25px;
  place-items: center;
  color: #ffffff;
  border-radius: 8px;
  font-size: 0.65rem;
  font-weight: 780;
}

.product-window__dock-item--claude .product-window__dock-mark {
  background: #c26d4f;
}

.product-window__dock-item--gpt .product-window__dock-mark {
  background: #16877a;
}

.product-window__dock-item--gemini .product-window__dock-mark {
  background: linear-gradient(135deg, #4f7ce5, #9b72cb);
}

.product-window__dock-item--antigravity .product-window__dock-mark {
  background: #6366f1;
}

@keyframes orb-drift {
  from {
    transform: translate3d(-3%, -2%, 0) scale(0.96);
  }

  to {
    transform: translate3d(5%, 4%, 0) scale(1.04);
  }
}

@keyframes terminal-scan {
  0%,
  12% {
    opacity: 0;
    transform: translateX(-100%);
  }

  28%,
  58% {
    opacity: 0.72;
  }

  76%,
  100% {
    opacity: 0;
    transform: translateX(340%);
  }
}

@keyframes status-pulse {
  0% {
    box-shadow: 0 0 0 0 rgb(52 211 153 / 45%);
  }

  60%,
  100% {
    box-shadow: 0 0 0 7px rgb(52 211 153 / 0%);
  }
}

@media (min-width: 720px) {
  .product-window__exchange {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }
}

@media (min-width: 1120px) {
  .home-hero {
    padding-top: 128px;
    padding-bottom: 112px;
  }

  .home-hero__inner {
    grid-template-columns: minmax(0, 0.9fr) minmax(520px, 1.1fr);
    gap: clamp(52px, 6vw, 92px);
  }
}

@media (max-width: 480px) {
  .home-hero__inner {
    gap: 50px;
  }

  .home-hero__title {
    max-width: none;
    font-size: clamp(2.65rem, 13vw, 3.3rem);
    letter-spacing: -0.06em;
  }

  .home-hero__actions,
  .home-hero__cta {
    width: 100%;
  }

  .product-window__titlebar {
    grid-template-columns: auto minmax(0, 1fr);
    min-height: 64px;
    padding-inline: 16px;
  }

  .product-window__title {
    overflow: hidden;
    justify-content: flex-end;
    text-overflow: ellipsis;
  }

  .product-window__titlebar-spacer {
    display: none;
  }

  .product-window__terminal {
    padding: 14px;
  }

  .product-window__endpoint-row {
    grid-template-columns: minmax(0, 1fr);
    gap: 6px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-hero__orb,
  .product-window__terminal-line,
  .product-window__status-dot {
    animation: none;
  }

  .product-window {
    transition: none;
  }

  .product-window:hover {
    transform: none;
  }

  .product-window__terminal-line {
    opacity: 1;
    transform: none;
  }
}
</style>
