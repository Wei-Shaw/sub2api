<template>
  <section class="home-hero" aria-labelledby="home-hero-title">
    <div class="home-hero__environment" aria-hidden="true">
      <span class="home-hero__orb home-hero__orb--blue"></span>
      <span class="home-hero__orb home-hero__orb--violet"></span>
      <span class="home-hero__orb home-hero__orb--cyan"></span>
      <span class="home-hero__light-sweep"></span>
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

.home-hero__environment { position: absolute; z-index: -1; inset: 0; overflow: hidden; pointer-events: none; }
.home-hero__orb { position: absolute; width: clamp(240px, 34vw, 520px); aspect-ratio: 1; border-radius: 50%; filter: blur(86px); opacity: .22; animation: orb-drift 14s ease-in-out infinite alternate; }
.home-hero__orb--blue { top: 4%; left: -12%; background: #3b82f6; }
.home-hero__orb--violet { top: 18%; right: -10%; background: #7768f5; animation-delay: -5s; }
.home-hero__orb--cyan { right: 30%; bottom: -28%; background: #11afbf; animation-delay: -9s; }
.home-hero__light-sweep { position: absolute; top: 4rem; right: 8%; width: min(60vw, 44rem); height: 28rem; background: linear-gradient(120deg, transparent 15%, var(--home-glass-highlight), transparent 72%); filter: blur(24px); opacity: .18; transform: rotate(-8deg); }

.home-hero__inner { display: grid; grid-template-columns: minmax(0, 1fr); gap: 64px; align-items: center; box-sizing: border-box; width: min(100%, 1280px); min-width: 0; margin-inline: auto; padding-inline: clamp(20px, 4vw, 40px); }
.home-hero__copy,
.home-hero__showcase { min-width: 0; }
.home-hero__copy { max-width: 680px; }

.home-hero__eyebrow { display: inline-flex; align-items: center; gap: 9px; margin: 0 0 24px; color: var(--home-accent); font-size: .78rem; font-weight: 740; line-height: 1.3; }
.home-hero__title { max-width: 14ch; margin: 0; color: var(--home-text); font-size: clamp(3rem, 5.5vw, 5.2rem); font-weight: 780; letter-spacing: -.035em; line-height: .96; overflow-wrap: anywhere; text-wrap: balance; }
.home-hero__subtitle { max-width: 26ch; margin: 26px 0 0; color: var(--home-text); font-size: clamp(1.2rem, 2vw, 1.65rem); font-weight: 620; letter-spacing: -.02em; line-height: 1.35; text-wrap: balance; }
.home-hero__description { max-width: 62ch; margin: 18px 0 0; color: var(--home-muted); font-size: clamp(.98rem, 1.2vw, 1.08rem); line-height: 1.75; text-wrap: pretty; }
.home-hero__actions { display: flex; flex-wrap: wrap; gap: 12px; margin-top: 34px; }

.home-hero__cta { display: inline-flex; min-height: 48px; align-items: center; justify-content: center; gap: 9px; box-sizing: border-box; padding: 12px 18px; border-radius: 12px; cursor: pointer; font-size: .92rem; font-weight: 720; line-height: 1; text-decoration: none; transition: transform 180ms cubic-bezier(.22, 1, .36, 1), background-color 180ms ease, color 180ms ease; }
.home-hero__cta--primary { color: #fff; background: var(--home-accent); }
.home-hero__cta--primary:hover { background: color-mix(in srgb, var(--home-accent) 88%, #000); transform: translateY(-2px); }
.home-hero__cta--secondary { color: var(--home-text); background: var(--home-glass); border: 1px solid var(--home-glass-border); backdrop-filter: blur(16px); }
.home-hero__cta--secondary:hover { color: var(--home-accent); background: var(--home-glass-hover); }
.home-hero__cta:focus-visible { outline: 3px solid var(--home-accent-soft); outline-offset: 3px; }
.home-hero__showcase { display: flex; justify-content: center; perspective: 1200px; }

@keyframes orb-drift {
  from { transform: translate3d(-3%, -2%, 0) scale(.96); }
  to { transform: translate3d(5%, 4%, 0) scale(1.04); }
}

@media (min-width: 1120px) {
  .home-hero { padding-top: 128px; padding-bottom: 112px; }
  .home-hero__inner { grid-template-columns: minmax(0, .9fr) minmax(520px, 1.1fr); gap: clamp(52px, 6vw, 92px); }
}

@media (max-width: 480px) {
  .home-hero__inner { gap: 50px; }
  .home-hero__title { max-width: none; font-size: clamp(2.65rem, 13vw, 3.3rem); letter-spacing: -.035em; }
  .home-hero__actions,
  .home-hero__cta { width: 100%; }
}

@media (prefers-reduced-motion: reduce) {
  .home-hero__orb { animation: none; }
  .home-hero__cta { transition: none; }
  .home-hero__cta:hover { transform: none; }
}
</style>
