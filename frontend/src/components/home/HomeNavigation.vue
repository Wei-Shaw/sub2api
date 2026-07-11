<template>
  <header
    class="home-navigation"
    :class="{
      'home-navigation--scrolled': isScrolled,
      'home-navigation--dark': isDark
    }"
  >
    <nav class="home-navigation__bar" :aria-label="siteName">
      <div class="home-navigation__inner">
        <router-link
          to="/home"
          class="home-navigation__brand"
          :aria-label="`${siteName} ${t('home.nav.backHome')}`"
          data-testid="home-brand-link"
          @click="closeMenu"
        >
          <span class="home-navigation__logo-frame" aria-hidden="true">
            <img
              :src="siteLogo || '/logo.png'"
              alt=""
              class="home-navigation__logo"
              data-testid="home-brand-logo"
            >
          </span>
          <span class="home-navigation__title" data-testid="home-brand-title">
            {{ siteName }}
          </span>
        </router-link>

        <div class="home-navigation__desktop-links">
          <a href="#capabilities" class="home-navigation__link">
            {{ t('home.nav.capabilities') }}
          </a>
          <a href="#models" class="home-navigation__link">
            {{ t('home.nav.models') }}
          </a>
          <a
            v-if="docUrl"
            :href="docUrl"
            class="home-navigation__link"
            target="_blank"
            rel="noopener noreferrer"
          >
            {{ t('home.docs') }}
          </a>
        </div>

        <div class="home-navigation__desktop-actions">
          <LocaleSwitcher />
          <button
            type="button"
            class="home-navigation__icon-button"
            :aria-label="themeLabel"
            :title="themeLabel"
            @click="emit('toggle-theme')"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="home-navigation__account-link"
          >
            {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
            <Icon name="arrowRight" size="sm" :stroke-width="2" aria-hidden="true" />
          </router-link>
        </div>

        <div class="home-navigation__mobile-actions">
          <button
            type="button"
            class="home-navigation__icon-button"
            :aria-label="themeLabel"
            :title="themeLabel"
            @click="emit('toggle-theme')"
          >
            <Icon :name="isDark ? 'sun' : 'moon'" size="md" />
          </button>
          <button
            ref="menuButton"
            type="button"
            class="home-navigation__icon-button"
            :aria-label="menuOpen ? t('home.nav.closeMenu') : t('home.nav.menu')"
            :aria-expanded="menuOpen"
            aria-controls="home-mobile-navigation"
            @click="toggleMenu"
          >
            <Icon :name="menuOpen ? 'x' : 'menu'" size="md" />
          </button>
        </div>
      </div>

      <transition name="home-navigation-menu">
        <div
          v-if="menuOpen"
          id="home-mobile-navigation"
          class="home-navigation__mobile-menu"
        >
          <a href="#capabilities" class="home-navigation__mobile-link" @click="closeMenu">
            {{ t('home.nav.capabilities') }}
          </a>
          <a href="#models" class="home-navigation__mobile-link" @click="closeMenu">
            {{ t('home.nav.models') }}
          </a>
          <a
            v-if="docUrl"
            :href="docUrl"
            class="home-navigation__mobile-link"
            target="_blank"
            rel="noopener noreferrer"
            @click="closeMenu"
          >
            {{ t('home.docs') }}
          </a>

          <div class="home-navigation__mobile-footer">
            <LocaleSwitcher />
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="home-navigation__account-link home-navigation__account-link--mobile"
              @click="closeMenu"
            >
              {{ isAuthenticated ? t('home.dashboard') : t('home.login') }}
              <Icon name="arrowRight" size="sm" :stroke-width="2" aria-hidden="true" />
            </router-link>
          </div>
        </div>
      </transition>
    </nav>
  </header>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const props = defineProps<{
  siteName: string
  siteLogo: string
  docUrl: string
  isAuthenticated: boolean
  dashboardPath: string
  isDark: boolean
}>()

const emit = defineEmits<{
  'toggle-theme': []
}>()

const { t } = useI18n()
const isScrolled = ref(false)
const menuOpen = ref(false)
const menuButton = ref<HTMLButtonElement | null>(null)
let scrollFrame: number | null = null

const themeLabel = computed(() => (
  props.isDark ? t('home.switchToLight') : t('home.switchToDark')
))

function updateScrollState() {
  isScrolled.value = window.scrollY > 20
  scrollFrame = null
}

function handleScroll() {
  if (scrollFrame !== null) return
  scrollFrame = window.requestAnimationFrame(updateScrollState)
}

function toggleMenu() {
  menuOpen.value = !menuOpen.value
}

function closeMenu() {
  menuOpen.value = false
}

function handleKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape' || !menuOpen.value) return
  closeMenu()
  menuButton.value?.focus()
}

onMounted(() => {
  updateScrollState()
  window.addEventListener('scroll', handleScroll, { passive: true })
  window.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  window.removeEventListener('scroll', handleScroll)
  window.removeEventListener('keydown', handleKeydown)
  if (scrollFrame !== null) window.cancelAnimationFrame(scrollFrame)
})
</script>

<style scoped>
.home-navigation {
  --home-nav-ease: cubic-bezier(0.16, 1, 0.3, 1);
  position: fixed;
  z-index: 50;
  top: 0;
  left: 0;
  width: 100%;
  padding-inline: 16px;
  pointer-events: none;
  transition: top 700ms var(--home-nav-ease);
}

.home-navigation--scrolled {
  top: 12px;
}

.home-navigation__bar {
  position: relative;
  width: calc(100% - 16px);
  max-width: 1200px;
  height: 64px;
  margin-inline: auto;
  color: var(--home-text, #18181b);
  background: transparent;
  border: 1px solid transparent;
  border-radius: 0;
  box-shadow: none;
  pointer-events: auto;
  transition:
    max-width 700ms var(--home-nav-ease),
    height 700ms var(--home-nav-ease),
    background-color 700ms var(--home-nav-ease),
    border-color 700ms var(--home-nav-ease),
    border-radius 700ms var(--home-nav-ease),
    box-shadow 700ms var(--home-nav-ease),
    backdrop-filter 700ms var(--home-nav-ease);
}

.home-navigation--scrolled .home-navigation__bar {
  max-width: 940px;
  height: 52px;
  background: rgba(255, 255, 255, 0.72);
  border-color: rgba(24, 24, 27, 0.1);
  border-radius: 999px;
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.8) inset,
    0 8px 24px rgba(24, 24, 27, 0.08),
    0 24px 64px rgba(24, 24, 27, 0.12);
  backdrop-filter: blur(36px) saturate(150%);
  -webkit-backdrop-filter: blur(36px) saturate(150%);
}

.home-navigation--dark.home-navigation--scrolled .home-navigation__bar {
  color: var(--home-text, #f4f4f5);
  background: rgba(13, 17, 23, 0.72);
  border-color: rgba(255, 255, 255, 0.12);
  box-shadow:
    0 1px 0 rgba(255, 255, 255, 0.08) inset,
    0 10px 28px rgba(0, 0, 0, 0.28),
    0 28px 72px rgba(0, 0, 0, 0.32);
}

.home-navigation__inner {
  height: 100%;
  display: flex;
  align-items: center;
  gap: 20px;
  padding-inline: 10px;
  transition: padding 700ms var(--home-nav-ease);
}

.home-navigation--scrolled .home-navigation__inner {
  padding-inline: 8px;
}

.home-navigation__brand {
  min-width: 0;
  display: inline-flex;
  align-items: center;
  gap: 9px;
  border-radius: 10px;
  color: inherit;
  text-decoration: none;
  transition: color 180ms ease, opacity 180ms ease;
}

.home-navigation__brand:hover {
  color: var(--home-accent, #0f766e);
}

.home-navigation__logo-frame {
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  display: grid;
  place-items: center;
  overflow: hidden;
  border: 1px solid var(--home-border, rgba(24, 24, 27, 0.12));
  border-radius: 10px;
  background: var(--home-panel, rgba(255, 255, 255, 0.72));
  transition:
    width 700ms var(--home-nav-ease),
    height 700ms var(--home-nav-ease),
    flex-basis 700ms var(--home-nav-ease),
    border-radius 700ms var(--home-nav-ease);
}

.home-navigation--scrolled .home-navigation__logo-frame {
  width: 30px;
  height: 30px;
  flex-basis: 30px;
  border-radius: 8px;
}

.home-navigation__logo {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.home-navigation__title {
  max-width: 210px;
  overflow: hidden;
  font-size: 1rem;
  font-weight: 760;
  letter-spacing: -0.025em;
  line-height: 1;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.home-navigation__desktop-links {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 2px;
}

.home-navigation__desktop-actions,
.home-navigation__mobile-actions,
.home-navigation__mobile-footer {
  display: flex;
  align-items: center;
}

.home-navigation__desktop-actions {
  gap: 6px;
}

.home-navigation__mobile-actions {
  display: none;
  gap: 4px;
  margin-left: auto;
}

.home-navigation__link,
.home-navigation__mobile-link {
  border-radius: 9px;
  color: var(--home-muted, #52525b);
  font-size: 0.82rem;
  font-weight: 680;
  text-decoration: none;
  transition: color 180ms ease, background-color 180ms ease;
}

.home-navigation__link {
  padding: 8px 10px;
}

.home-navigation__link:hover,
.home-navigation__mobile-link:hover {
  color: var(--home-text, #18181b);
  background: var(--home-accent-soft, rgba(15, 118, 110, 0.1));
}

.home-navigation--dark .home-navigation__link,
.home-navigation--dark .home-navigation__mobile-link {
  color: var(--home-muted, #a1a1aa);
}

.home-navigation--dark .home-navigation__link:hover,
.home-navigation--dark .home-navigation__mobile-link:hover {
  color: var(--home-text, #f4f4f5);
}

.home-navigation__icon-button {
  width: 36px;
  height: 36px;
  display: inline-grid;
  flex: 0 0 36px;
  place-items: center;
  padding: 0;
  border: 1px solid transparent;
  border-radius: 10px;
  color: var(--home-muted, #52525b);
  background: transparent;
  cursor: pointer;
  transition:
    width 700ms var(--home-nav-ease),
    height 700ms var(--home-nav-ease),
    flex-basis 700ms var(--home-nav-ease),
    color 180ms ease,
    background-color 180ms ease,
    border-color 180ms ease;
}

.home-navigation--scrolled .home-navigation__icon-button {
  width: 34px;
  height: 34px;
  flex-basis: 34px;
}

.home-navigation__icon-button:hover {
  color: var(--home-text, #18181b);
  background: var(--home-accent-soft, rgba(15, 118, 110, 0.1));
  border-color: var(--home-border, rgba(24, 24, 27, 0.1));
}

.home-navigation__account-link {
  min-height: 36px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 7px;
  padding: 8px 13px;
  border: 1px solid var(--home-accent, #0f766e);
  border-radius: 999px;
  color: var(--home-accent-contrast, #ffffff);
  background: var(--home-accent, #0f766e);
  font-size: 0.78rem;
  font-weight: 720;
  line-height: 1;
  text-decoration: none;
  transition:
    min-height 700ms var(--home-nav-ease),
    padding 700ms var(--home-nav-ease),
    background-color 180ms ease,
    border-color 180ms ease,
    box-shadow 180ms ease;
}

.home-navigation--scrolled .home-navigation__account-link {
  min-height: 34px;
  padding-block: 7px;
}

.home-navigation__account-link:hover {
  background: color-mix(in srgb, var(--home-accent, #0f766e) 86%, black);
  border-color: color-mix(in srgb, var(--home-accent, #0f766e) 86%, black);
  box-shadow: 0 8px 20px color-mix(in srgb, var(--home-accent, #0f766e) 24%, transparent);
}

.home-navigation__brand:focus-visible,
.home-navigation__link:focus-visible,
.home-navigation__mobile-link:focus-visible,
.home-navigation__icon-button:focus-visible,
.home-navigation__account-link:focus-visible {
  outline: 3px solid var(--home-accent, #0f766e);
  outline-offset: 3px;
}

.home-navigation__mobile-menu {
  position: absolute;
  top: calc(100% + 8px);
  left: 0;
  right: 0;
  display: none;
  padding: 10px;
  color: var(--home-text, #18181b);
  background: rgba(255, 255, 255, 0.92);
  border: 1px solid rgba(24, 24, 27, 0.1);
  border-radius: 18px;
  box-shadow: 0 18px 56px rgba(24, 24, 27, 0.18);
  backdrop-filter: blur(36px) saturate(150%);
  -webkit-backdrop-filter: blur(36px) saturate(150%);
}

.home-navigation--dark .home-navigation__mobile-menu {
  color: var(--home-text, #f4f4f5);
  background: rgba(13, 17, 23, 0.92);
  border-color: rgba(255, 255, 255, 0.12);
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.4);
}

.home-navigation__mobile-link {
  min-height: 42px;
  display: flex;
  align-items: center;
  padding: 9px 11px;
}

.home-navigation__mobile-footer {
  justify-content: space-between;
  gap: 10px;
  margin-top: 8px;
  padding-top: 10px;
  border-top: 1px solid var(--home-border, rgba(24, 24, 27, 0.1));
}

.home-navigation__account-link--mobile {
  min-height: 38px;
}

.home-navigation-menu-enter-active,
.home-navigation-menu-leave-active {
  transition: opacity 220ms ease, transform 220ms var(--home-nav-ease);
}

.home-navigation-menu-enter-from,
.home-navigation-menu-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

@media (max-width: 900px) {
  .home-navigation {
    padding-inline: 8px;
  }

  .home-navigation__bar {
    width: calc(100% - 8px);
  }

  .home-navigation__desktop-links,
  .home-navigation__desktop-actions {
    display: none;
  }

  .home-navigation__mobile-actions,
  .home-navigation__mobile-menu {
    display: flex;
  }

  .home-navigation__mobile-menu {
    flex-direction: column;
  }

  .home-navigation__title {
    max-width: min(42vw, 220px);
  }
}

@media (max-width: 360px) {
  .home-navigation__inner {
    gap: 8px;
    padding-inline: 7px;
  }

  .home-navigation__brand {
    gap: 7px;
  }

  .home-navigation__title {
    max-width: 34vw;
    font-size: 0.9rem;
  }

  .home-navigation__logo-frame {
    width: 32px;
    height: 32px;
    flex-basis: 32px;
  }
}

@media (prefers-reduced-motion: reduce) {
  .home-navigation,
  .home-navigation__bar,
  .home-navigation__inner,
  .home-navigation__logo-frame,
  .home-navigation__brand,
  .home-navigation__link,
  .home-navigation__mobile-link,
  .home-navigation__icon-button,
  .home-navigation__account-link,
  .home-navigation-menu-enter-active,
  .home-navigation-menu-leave-active {
    transition: none;
  }
}
</style>
