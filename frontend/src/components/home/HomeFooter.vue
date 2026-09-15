<template>
  <footer class="home-footer">
    <div class="home-footer__inner">
      <RouterLink to="/home" class="home-footer__brand">
        <span v-if="siteLogo" class="home-footer__logo">
          <img :src="siteLogo" alt="" />
        </span>
        <span class="home-footer__site-name">{{ siteName }}</span>
      </RouterLink>

      <p class="home-footer__copyright">
        © {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
      </p>

      <nav v-if="docUrl" class="home-footer__links" :aria-label="t('home.docs')">
        <a
          :href="docUrl"
          target="_blank"
          rel="noopener noreferrer"
        >
          <span>{{ t('home.docs') }}</span>
          <Icon name="externalLink" size="xs" :stroke-width="2" aria-hidden="true" />
        </a>
      </nav>
    </div>
  </footer>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import Icon from '@/components/icons/Icon.vue'

defineProps<{
  siteName: string
  siteLogo: string
  docUrl: string
  currentYear: number
}>()

const { t } = useI18n()
</script>

<style scoped>
.home-footer {
  border-top: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  color: var(--home-muted, #5f665f);
  background: var(--home-glass-sheen);
  backdrop-filter: blur(18px) saturate(125%);
  -webkit-backdrop-filter: blur(18px) saturate(125%);
}

.home-footer__inner {
  width: min(calc(100% - 40px), 1200px);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1.5rem;
  min-height: 6rem;
  margin-inline: auto;
  padding-block: 1.5rem;
}

.home-footer__brand {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 0.625rem;
  border-radius: 10px;
  color: var(--home-text, #1f2937);
  text-decoration: none;
  transition: color 180ms ease;
}

.home-footer__brand:hover {
  color: var(--home-accent, #d45f28);
}

.home-footer__logo {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  overflow: hidden;
  border: 1px solid var(--home-border, rgb(31 41 55 / 12%));
  border-radius: 9px;
  background: rgb(255 255 255 / 60%);
}

.home-footer__logo img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.home-footer__site-name {
  max-width: 16rem;
  overflow: hidden;
  font-size: 0.9375rem;
  font-weight: 750;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home-footer__copyright {
  margin: 0;
  font-size: 0.8125rem;
  line-height: 1.6;
  text-align: center;
}

.home-footer__links {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 1.125rem;
}

.home-footer__links a {
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  border-radius: 7px;
  color: var(--home-muted, #5f665f);
  font-size: 0.8125rem;
  font-weight: 650;
  line-height: 1.5;
  text-decoration: none;
  transition: color 180ms ease;
}

.home-footer__links a:hover {
  color: var(--home-accent, #d45f28);
}

.home-footer__brand:focus-visible,
.home-footer__links a:focus-visible {
  outline: 3px solid var(--home-accent-soft, rgb(212 95 40 / 25%));
  outline-offset: 4px;
}

@media (max-width: 720px) {
  .home-footer__inner,
  .home-footer__links {
    justify-content: center;
  }

  .home-footer__inner {
    width: calc(100% - 24px);
    flex-direction: column;
    gap: 1rem;
    padding-block: 2rem;
    text-align: center;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-footer__brand,
  .home-footer__links a {
    transition: none;
  }
}
</style>
