<template>
  <div class="auth-page" :class="{ 'is-dark': isDark }">
    <!-- Ambient Particles -->
    <div class="auth-particles" aria-hidden="true">
      <span v-for="p in particles" :key="p" :class="`auth-particle auth-particle-${p}`">
        <svg v-if="p % 3 === 0" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M6 1v10M1 6h10" /></svg>
        <svg v-else-if="p % 3 === 1" viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><path d="M2 2l8 8M10 2L2 10" /></svg>
        <svg v-else viewBox="0 0 12 12" fill="currentColor"><path d="M6 1L7.5 4.5L11 6L7.5 7.5L6 11L4.5 7.5L1 6L4.5 4.5Z" /></svg>
      </span>
    </div>

    <!-- Content Container -->
    <div class="auth-container">
      <!-- Top Actions (Lang Switch) -->
      <div class="auth-top-actions">
        <button @click="toggleLocale" class="auth-lang-switch" :aria-label="currentLocale === 'zh' ? 'Switch to English' : '切换到中文'">
          {{ currentLocale === 'zh' ? 'EN' : '中文' }}
        </button>
      </div>

      <!-- Logo/Brand -->
      <div class="auth-brand">
        <router-link to="/home" class="auth-brand-link" aria-label="SUBTOKEN Home">
          <span class="auth-logo-badge" aria-hidden="true">
            <img src="/subtoken-logo.png" alt="Logo" class="auth-logo-img" />
          </span>
          <div class="auth-brand-text">
            <h1 class="auth-site-name">{{ siteName }}</h1>
            <p class="auth-site-subtitle">{{ siteSubtitle }}</p>
          </div>
        </router-link>
      </div>

      <!-- Card Container -->
      <div class="auth-card">
        <div class="auth-washi-tape"></div>
        <slot />
      </div>

      <!-- Footer Links -->
      <div class="auth-footer">
        <slot name="footer" />
      </div>

      <!-- Copyright -->
      <div class="auth-copyright">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { getLocale, setLocale } from '@/i18n'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'subtoken')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')

const currentYear = computed(() => new Date().getFullYear())

const particles = Array.from({ length: 10 }, (_, i) => i + 1)
const isDark = ref(false)
const currentLocale = ref(getLocale())

async function toggleLocale() {
  const nextLocale = currentLocale.value === 'zh' ? 'en' : 'zh'
  await setLocale(nextLocale)
  currentLocale.value = nextLocale
  localStorage.setItem('sub2api_home_locale', nextLocale)
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
  } else {
    isDark.value = document.documentElement.classList.contains('dark')
  }
}

onMounted(() => {
  appStore.fetchPublicSettings()
  initTheme()
})
</script>

<style scoped>
/* ============================================================
   Neo-Brutalist Auth Layout — matches Subtoken home visual system
   ============================================================ */
.auth-page {
  --auth-bg: #f4f0e6;
  --auth-paper: #fffdf6;
  --auth-ink: #18211b;
  --auth-muted: #5f675d;
  --auth-border: #172018;
  --auth-shadow: #172018;
  --auth-mint: #cfe0d1;
  --auth-olive: #769878;
  --auth-gold: #d8bf62;
  --auth-clay: #d98f63;
  --auth-blue: #9ebdd2;

  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  overflow: hidden;
  padding: 24px 16px;
  background:
    linear-gradient(90deg, rgba(24, 33, 27, 0.045) 1px, transparent 1px),
    linear-gradient(rgba(24, 33, 27, 0.04) 1px, transparent 1px),
    radial-gradient(circle at 12% 18%, rgba(118, 152, 120, 0.18), transparent 23rem),
    radial-gradient(circle at 85% 7%, rgba(216, 191, 98, 0.2), transparent 22rem),
    linear-gradient(180deg, var(--auth-bg), #eef4ea 56%, #f6f1e7);
  background-size: 44px 44px, 44px 44px, auto, auto, auto;
  color: var(--auth-ink);
  font-family:
    ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont,
    "Segoe UI", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif;
  letter-spacing: 0;
}

.auth-page.is-dark {
  --auth-bg: #141814;
  --auth-paper: #20261f;
  --auth-ink: #f2eddf;
  --auth-muted: #b7c1b2;
  --auth-border: #edf0e4;
  --auth-shadow: #050705;
  --auth-mint: #27372d;
  --auth-olive: #9ebd91;
  --auth-gold: #d2b967;
  --auth-clay: #d49266;
  --auth-blue: #90b2ca;

  background:
    linear-gradient(90deg, rgba(237, 240, 228, 0.045) 1px, transparent 1px),
    linear-gradient(rgba(237, 240, 228, 0.035) 1px, transparent 1px),
    radial-gradient(circle at 15% 12%, rgba(158, 189, 145, 0.14), transparent 24rem),
    radial-gradient(circle at 82% 0%, rgba(210, 185, 103, 0.12), transparent 21rem),
    linear-gradient(180deg, var(--auth-bg), #0f130f 68%, #171b17);
  background-size: 44px 44px, 44px 44px, auto, auto, auto;
}

.auth-page * {
  box-sizing: border-box;
}

/* ---- Particles ---- */
.auth-particles {
  pointer-events: none;
  position: fixed;
  inset: 0;
  z-index: 0;
  overflow: hidden;
}

.auth-particle {
  position: absolute;
  display: inline-block;
  width: 10px;
  height: 10px;
  color: var(--auth-gold);
  opacity: 0.38;
  border: 0;
  background: transparent;
  box-shadow: none;
  animation: auth-drift 14s ease-in-out infinite;
}

.auth-particle:nth-child(3n) { color: var(--auth-blue); }
.auth-particle:nth-child(4n) { color: var(--auth-clay); }

.auth-particle-1  { left: 8%;  top: 15%; animation-delay: -1s; }
.auth-particle-2  { left: 22%; top: 72%; animation-delay: -4s; }
.auth-particle-3  { left: 38%; top: 28%; animation-delay: -7s; }
.auth-particle-4  { left: 52%; top: 82%; animation-delay: -2s; }
.auth-particle-5  { left: 65%; top: 12%; animation-delay: -6s; }
.auth-particle-6  { left: 78%; top: 58%; animation-delay: -9s; }
.auth-particle-7  { left: 90%; top: 35%; animation-delay: -3s; }
.auth-particle-8  { left: 15%; top: 48%; animation-delay: -8s; }
.auth-particle-9  { left: 45%; top: 65%; animation-delay: -5s; }
.auth-particle-10 { left: 85%; top: 78%; animation-delay: -10s; }

@keyframes auth-drift {
  0%, 100% { transform: translate3d(0, 0, 0); }
  50% { transform: translate3d(14px, -18px, 0); }
}

/* ---- Top Actions ---- */
.auth-top-actions {
  position: absolute;
  top: 20px;
  right: 20px;
  z-index: 10;
}

.auth-lang-switch {
  background: var(--auth-paper);
  color: var(--auth-muted);
  border: 2px solid var(--auth-border);
  border-radius: 8px;
  padding: 4px 8px;
  font-size: 0.75rem;
  font-weight: 800;
  cursor: pointer;
  box-shadow: 2px 2px 0 var(--auth-shadow);
  transition: all 180ms ease;
}

.auth-lang-switch:hover {
  transform: translate(-1px, -1px);
  box-shadow: 3px 3px 0 var(--auth-shadow);
  color: var(--auth-ink);
}

.auth-lang-switch:active {
  transform: translate(1px, 1px);
  box-shadow: 1px 1px 0 var(--auth-shadow);
}

/* ---- Container ---- */
.auth-container {
  position: relative;
  z-index: 2;
  width: 100%;
  max-width: 440px;
}

/* ---- Brand ---- */
.auth-brand {
  margin-bottom: 28px;
  text-align: center;
}

.auth-brand-link {
  display: inline-flex;
  align-items: center;
  gap: 14px;
  text-decoration: none;
  color: var(--auth-ink);
  transition: transform 180ms ease;
  max-width: 100%;
  text-align: left;
}

.auth-brand-link:hover {
  transform: translateY(-2px);
}

.auth-logo-badge {
  position: relative;
  display: flex;
  flex: 0 0 auto;
  width: 56px;
  height: 48px;
  align-items: center;
  justify-content: center;
  border: 3px solid var(--auth-border);
  border-radius: 8px;
  background: var(--auth-paper);
  box-shadow: 5px 5px 0 var(--auth-shadow);
  overflow: hidden;
  padding: 4px;
}

/* Keep logo badge light in dark mode for contrast */
.auth-page.is-dark .auth-logo-badge {
  background: #fdfaf2;
}

.auth-logo-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.auth-brand-text {
  text-align: left;
  flex: 1;
  min-width: 0;
}

.auth-site-name {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 950;
  line-height: 1;
  letter-spacing: 0;
}

.auth-site-subtitle {
  margin: 3px 0 0;
  color: var(--auth-muted);
  font-size: 0.78rem;
  font-weight: 800;
  letter-spacing: 0;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.3;
}

/* ---- Card ---- */
.auth-card {
  position: relative;
  border: 4px solid var(--auth-border);
  border-radius: 8px;
  background: var(--auth-paper);
  box-shadow: 8px 8px 0 var(--auth-shadow);
  padding: 32px 28px;
  transition: transform 220ms ease, box-shadow 220ms ease;
}

.auth-card:hover {
  transform: translate(-2px, -2px);
  box-shadow: 11px 11px 0 var(--auth-shadow);
}

/* Washi tape decoration */
.auth-washi-tape {
  position: absolute;
  top: -10px;
  left: 50%;
  transform: translateX(-50%) rotate(-2deg);
  width: 80px;
  height: 20px;
  background: rgba(216, 191, 98, 0.65);
  border-left: 1px dashed rgba(24, 33, 27, 0.15);
  border-right: 1px dashed rgba(24, 33, 27, 0.15);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  z-index: 10;
  pointer-events: none;
}

.auth-page.is-dark .auth-washi-tape {
  background: rgba(210, 185, 103, 0.45);
  border-left: 1px dashed rgba(237, 240, 228, 0.15);
  border-right: 1px dashed rgba(237, 240, 228, 0.15);
}

/* ---- Override inner form text colors for brutal palette ---- */
.auth-card :deep(h2) {
  color: var(--auth-ink);
}

.auth-card :deep(.text-gray-500),
.auth-card :deep(.text-gray-400),
.auth-card :deep(.dark\:text-dark-400),
.auth-card :deep(.dark\:text-dark-500) {
  color: var(--auth-muted);
}

.auth-card :deep(.text-gray-900),
.auth-card :deep(.dark\:text-white) {
  color: var(--auth-ink);
}

/* Input focus ring in warm olive */
.auth-card :deep(.input:focus) {
  border-color: var(--auth-olive);
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--auth-olive) 25%, transparent);
}

/* Primary buttons get the brutal olive treatment */
.auth-card :deep(.btn-primary) {
  border: 3px solid var(--auth-border);
  border-radius: 999px;
  background: var(--auth-olive);
  color: #08110a;
  box-shadow: 4px 4px 0 var(--auth-shadow);
  font-weight: 900;
  transition: transform 180ms ease, box-shadow 180ms ease, background-color 180ms ease;
}

.auth-card :deep(.btn-primary:hover:not(:disabled)) {
  transform: translate(-2px, -2px);
  box-shadow: 7px 7px 0 var(--auth-shadow);
}

.auth-card :deep(.btn-primary:active:not(:disabled)) {
  transform: translate(1px, 1px);
  box-shadow: 2px 2px 0 var(--auth-shadow);
}

.auth-card :deep(.btn-primary:disabled) {
  opacity: 0.55;
  cursor: not-allowed;
}

/* OAuth / divider styling */
.auth-card :deep(.bg-gray-200) {
  background: color-mix(in srgb, var(--auth-border) 22%, transparent);
}

/* Primary link color → olive */
.auth-card :deep(.text-primary-600),
.auth-card :deep(.text-primary-500),
.auth-card :deep(.text-primary-400),
.auth-card :deep(.text-primary-300) {
  color: var(--auth-olive);
}

/* ---- Footer ---- */
.auth-footer {
  margin-top: 20px;
  text-align: center;
  font-size: 0.88rem;
  font-weight: 800;
  color: var(--auth-muted);
}

.auth-footer :deep(a) {
  color: var(--auth-olive);
  font-weight: 900;
  text-decoration: underline;
  text-underline-offset: 3px;
  text-decoration-thickness: 2px;
  transition: color 180ms ease;
}

.auth-footer :deep(a:hover) {
  color: var(--auth-gold);
}

/* ---- Copyright ---- */
.auth-copyright {
  margin-top: 28px;
  text-align: center;
  font-size: 0.72rem;
  font-weight: 800;
  color: var(--auth-muted);
  opacity: 0.7;
}

/* ---- Responsive ---- */
@media (max-width: 480px) {
  .auth-page {
    padding: 16px 12px;
  }

  .auth-top-actions {
    position: relative;
    top: 0;
    right: 0;
    text-align: right;
    margin-bottom: 12px;
  }

  .auth-card {
    padding: 24px 18px;
  }

  .auth-logo-badge {
    width: 46px;
    height: 40px;
  }

  .auth-brand-link {
    flex-wrap: wrap;
    justify-content: center;
    text-align: center;
  }

  .auth-brand-text {
    text-align: center;
  }

  .auth-site-name {
    font-size: 1.3rem;
  }
}

/* ---- Reduced Motion ---- */
@media (prefers-reduced-motion: reduce) {
  .auth-page *,
  .auth-page *::before,
  .auth-page *::after {
    animation-duration: 0.001ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.001ms !important;
  }
  .auth-particle {
    animation: none !important;
  }
}
</style>
