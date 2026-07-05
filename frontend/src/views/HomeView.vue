<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <!-- iframe mode -->
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- HTML mode - SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    class="relative flex min-h-screen flex-col overflow-hidden bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950"
  >
    <!-- Background Decorations -->
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div
        class="absolute -right-40 -top-40 h-96 w-96 rounded-full bg-primary-400/20 blur-3xl"
      ></div>
      <div
        class="absolute -bottom-40 -left-40 h-96 w-96 rounded-full bg-primary-500/15 blur-3xl"
      ></div>
      <div
        class="absolute left-1/3 top-1/4 h-72 w-72 rounded-full bg-primary-300/10 blur-3xl"
      ></div>
      <div
        class="absolute bottom-1/4 right-1/4 h-64 w-64 rounded-full bg-primary-400/10 blur-3xl"
      ></div>
      <div
        class="absolute inset-0 bg-[linear-gradient(rgba(20,184,166,0.03)_1px,transparent_1px),linear-gradient(90deg,rgba(20,184,166,0.03)_1px,transparent_1px)] bg-[size:64px_64px]"
      ></div>
    </div>

    <!-- Header -->
    <header class="sticky top-0 z-20 bg-white/80 px-6 py-4 backdrop-blur-md dark:bg-dark-950/80">
      <nav class="mx-auto flex max-w-6xl items-center justify-between">
        <!-- Logo -->
        <div class="flex items-center">
          <div class="h-10 w-10 overflow-hidden rounded-xl shadow-md">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
        </div>

        <!-- Nav Actions -->
        <div class="flex min-w-0 flex-1 items-center justify-end gap-2">
          <!-- Scrollable menu area with left/right arrows -->
          <div class="relative flex min-w-0 max-w-full items-center">
            <!-- Left scroll arrow -->
            <button
              v-show="canScrollLeft"
              type="button"
              class="absolute left-0 top-1/2 z-10 -translate-y-1/2 rounded-full bg-white/90 p-1 text-gray-600 shadow-sm ring-1 ring-gray-200 backdrop-blur transition hover:bg-white hover:text-gray-900 dark:bg-dark-800/90 dark:text-dark-200 dark:ring-dark-600 dark:hover:bg-dark-800 dark:hover:text-white"
              :aria-label="t('common.scrollLeft')"
              @click="scrollNav(-1)"
            >
              <Icon name="chevronLeft" size="sm" />
            </button>

            <!-- Scrollable menu items -->
            <div
              ref="navScrollRef"
              class="nav-actions flex min-w-0 items-center gap-3 overflow-x-auto scroll-smooth"
              :class="{ 'px-6': canScrollLeft || canScrollRight }"
              @scroll.passive="updateScrollIndicators"
            >
            <!-- Pricing Plaza link (anonymous-friendly) -->
            <router-link
              to="/plaza/models"
              class="flex shrink-0 items-center gap-1.5 whitespace-nowrap text-sm font-medium text-gray-600 transition-colors hover:text-gray-900 dark:text-dark-300 dark:hover:text-white"
            >
              <Icon name="dollar" size="sm" />
              {{ t('plaza.title') }}
            </router-link>


            <div
              v-if="homeProductMenuItems.length > 0"
              ref="homeProductsMenuRef"
              data-test="home-products-menu"
              class="shrink-0"
              @mouseenter="openHomeProductsMenuFromEvent"
              @mouseleave="scheduleCloseHomeProductsMenu"
            >
              <button
                ref="homeProductsButtonRef"
                type="button"
                :aria-expanded="homeProductsOpen"
                aria-haspopup="menu"
                class="flex items-center gap-1.5 whitespace-nowrap text-sm font-medium text-gray-600 transition-colors hover:text-gray-900 dark:text-dark-300 dark:hover:text-white"
                @click.stop="toggleHomeProductsMenuFromEvent"
              >
                <Icon name="grid" size="sm" />
                {{ t('home.products') }}
                <Icon
                  name="chevronDown"
                  size="sm"
                  class="transition-transform"
                  :class="{ 'rotate-180': isHomeProductsNavOpen }"
                />
              </button>
            </div>

            <!-- Language Switcher -->
            <LocaleSwitcher class="shrink-0" />

            <!-- Doc Link -->
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="shrink-0 rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
              :title="t('home.viewDocs')"
            >
              <Icon name="book" size="md" />
            </a>

            <!-- Theme Toggle -->
            <button
              @click="toggleTheme"
              class="shrink-0 rounded-lg p-2 text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-white"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            >
              <Icon v-if="isDark" name="sun" size="md" />
              <Icon v-else name="moon" size="md" />
            </button>
            </div>

            <!-- Right scroll arrow -->
            <button
              v-show="canScrollRight"
              type="button"
              class="absolute right-0 top-1/2 z-10 -translate-y-1/2 rounded-full bg-white/90 p-1 text-gray-600 shadow-sm ring-1 ring-gray-200 backdrop-blur transition hover:bg-white hover:text-gray-900 dark:bg-dark-800/90 dark:text-dark-200 dark:ring-dark-600 dark:hover:bg-dark-800 dark:hover:text-white"
              :aria-label="t('common.scrollRight')"
              @click="scrollNav(1)"
            >
              <Icon name="chevronRight" size="sm" />
            </button>
          </div>

          <!-- Login / Dashboard Button (fixed, always visible) -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="shrink-0 inline-flex items-center gap-1.5 rounded-full bg-gray-900 py-1 pl-1 pr-2.5 transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            <span
              class="flex h-5 w-5 items-center justify-center rounded-full bg-gradient-to-br from-primary-400 to-primary-600 text-[10px] font-semibold text-white"
            >
              {{ userInitial }}
            </span>
            <span class="text-xs font-medium text-white">{{ t('home.dashboard') }}</span>
            <svg
              class="h-3 w-3 text-gray-400"
              fill="none"
              viewBox="0 0 24 24"
              stroke="currentColor"
              stroke-width="2"
            >
              <path
                stroke-linecap="round"
                stroke-linejoin="round"
                d="M4.5 19.5l15-15m0 0H8.25m11.25 0v11.25"
              />
            </svg>
          </router-link>
          <router-link
            v-else
            to="/login"
            class="shrink-0 inline-flex items-center rounded-full bg-gray-900 px-3 py-1 text-xs font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>
    </header>

    <!-- Main Content -->
    <main class="relative z-10 px-6 py-16">
      <div class="mx-auto max-w-6xl">
        <!-- Hero Section - Left/Right Layout -->
        <div class="mb-12 flex flex-col items-center justify-between gap-12 lg:flex-row lg:gap-16">
          <!-- Left: Text Content -->
          <div class="flex-1 text-center lg:text-left">
            <h1
              class="mb-4 bg-gradient-to-br from-gray-900 via-primary-700 to-gray-900 bg-clip-text text-4xl font-bold leading-[1.1] text-transparent dark:from-white dark:via-primary-300 dark:to-white md:text-5xl lg:text-6xl"
            >
              {{ siteName }}
            </h1>
            <p class="mb-8 text-lg text-gray-600 dark:text-dark-300 md:text-xl">
              {{ siteSubtitle }}
            </p>

            <!-- CTA Buttons -->
            <div class="flex flex-wrap items-center justify-center gap-3 lg:justify-start">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="btn btn-primary px-6 py-3 text-base shadow-lg shadow-primary-500/30"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
              <!--
                Secondary CTA — funnels anonymous + authenticated visitors
                alike to the public plaza so they can survey models &
                plans without committing. No live API call is made here.
              -->
              <router-link
                to="/plaza/models"
                class="btn btn-secondary px-6 py-3 text-base"
              >
                {{ t('home.cta_view_pricing') }}
                <Icon name="arrowRight" size="md" class="ml-2" :stroke-width="2" />
              </router-link>
            </div>

            <div
              v-if="homeProductMenuItems.length > 0"
              ref="homeProductsHeroMenuRef"
              data-test="home-products-hero-menu"
              class="mt-4 flex justify-center lg:justify-start"
              @mouseenter="openHomeProductsMenuFromEvent"
              @mouseleave="scheduleCloseHomeProductsMenu"
            >
              <button
                ref="homeProductsHeroButtonRef"
                type="button"
                data-test="home-products-hero-button"
                :aria-expanded="homeProductsOpen"
                aria-haspopup="menu"
                class="btn btn-secondary px-6 py-3 text-base"
                @click.stop="toggleHomeProductsMenuFromEvent"
              >
                {{ t('home.products') }}
                <Icon
                  name="chevronDown"
                  size="md"
                  class="ml-2 transition-transform"
                  :class="{ 'rotate-180': isHomeProductsHeroOpen }"
                />
              </button>
            </div>
          </div>

          <!-- Right: Terminal Animation -->
          <div class="flex flex-1 justify-center lg:justify-end">
            <div class="terminal-container">
              <div class="terminal-window">
                <!-- Window header -->
                <div class="terminal-header">
                  <div class="terminal-buttons">
                    <span class="btn-close"></span>
                    <span class="btn-minimize"></span>
                    <span class="btn-maximize"></span>
                  </div>
                  <span class="terminal-title">terminal</span>
                </div>
                <!-- Terminal content -->
                <div class="terminal-body">
                  <div class="code-line line-1">
                    <span class="code-prompt">$</span>
                    <span class="code-cmd">curl</span>
                    <span class="code-flag">-X POST</span>
                    <span class="code-url">/v1/messages</span>
                  </div>
                  <div class="code-line line-2">
                    <span class="code-comment"># Routing to upstream...</span>
                  </div>
                  <div class="code-line line-3">
                    <span class="code-success">200 OK</span>
                    <span class="code-response">{ "content": "Hello!" }</span>
                  </div>
                  <div class="code-line line-4">
                    <span class="code-prompt">$</span>
                    <span class="cursor"></span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!--
          Section header for the Features Grid.

          Earlier iterations of this slot have shrunk progressively:
            v1: three pill-shaped feature tags (订阅转 API / 会话保持 /
                按量计费) — duplicated the cards directly below.
            v2: a bare gradient `<h2>我们的优势</h2>` — read as flat /
                weak next to HomeShowcaseSection's plans + promo
                headers, which both ship the full "eyebrow chip +
                gradient h2 + one-line subtitle" treatment.

          v3 (this version) promotes the header to the same three-
          part composition so all three homepage section anchors
          (核心能力 / 订阅套餐 / 活动专区) read as a matched set:

            • eyebrow chip — small uppercase tracking-wide tag with
              a leading dot. Uses the `sky` palette to differentiate
              from `plans` (primary) and `promo` (amber); the three
              colours together act as visual section markers as the
              user scrolls.
            • gradient h2 — same `bg-clip-text` + responsive
              text-3xl→md:text-4xl rhythm as the other two h2's,
              with a `sky-600` mid-stop matching the eyebrow.
            • subtitle paragraph — distils the three cards below
              into one cadenced line, capped at `max-w-xl` so it
              never wraps awkwardly under the title.
        -->
        <!--
          Spacing rationale (mt-16 / md:mt-24):
          The hero block above ends in a tall code-terminal mockup
          whose bottom edge sits visually flush with whatever follows
          — the previous gap was just `mb-12` from the hero wrapper,
          which read as cramped. Adding a top margin on this header
          (rather than a bottom margin on the hero) keeps the hero's
          internal rhythm untouched and gives the Features section
          its own clear "breathing room" entry, so the eye registers
          a section break before reading the eyebrow.

          Responsive scale (16 → 24): on mobile the hero already
          stacks vertically and consumes the full viewport, so 64px
          is enough separation; on tablet+ the hero compresses and
          the section change needs more vertical air to feel like a
          deliberate scroll target rather than a continuation.
        -->
        <div
          class="mb-12 mt-16 flex flex-col items-center text-center md:mt-24"
        >
          <span
            class="mb-3 inline-flex items-center gap-1.5 rounded-full border border-sky-300/70 bg-sky-50/80 px-3 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-sky-700 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-300"
            data-test="home-features-eyebrow"
          >
            <span class="inline-block h-1 w-1 rounded-full bg-sky-500"></span>
            {{ t('home.features.eyebrow') }}
          </span>
          <h2
            class="bg-gradient-to-r from-gray-900 via-sky-600 to-gray-900 bg-clip-text text-3xl font-bold leading-tight text-transparent dark:from-white dark:via-sky-300 dark:to-white md:text-4xl"
            data-test="home-features-title"
          >
            {{ t('home.features.title') }}
          </h2>
          <p
            class="mt-3 max-w-xl text-sm text-gray-600 dark:text-dark-400 md:text-base"
            data-test="home-features-subtitle"
          >
            {{ t('home.features.subtitle') }}
          </p>
        </div>

        <!-- Features Grid -->
        <div class="mb-12 grid gap-6 md:grid-cols-3">
          <!-- Feature 1: Unified Gateway -->
          <div
            class="feature-card group relative overflow-hidden rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-primary-300/50 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60 dark:hover:border-primary-500/40"
          >
            <span
              aria-hidden="true"
              class="pointer-events-none absolute inset-x-0 top-0 h-1 origin-left scale-x-0 bg-gradient-to-r from-blue-500 to-blue-600 transition-transform duration-300 group-hover:scale-x-100"
            ></span>
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-blue-500 to-blue-600 shadow-lg shadow-blue-500/30 transition-transform group-hover:scale-110"
            >
              <Icon name="server" size="lg" class="text-white" />
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.unifiedGateway') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.unifiedGatewayDesc') }}
            </p>
          </div>

          <!-- Feature 2: Account Pool -->
          <div
            class="feature-card group relative overflow-hidden rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-primary-300/50 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60 dark:hover:border-primary-500/40"
          >
            <span
              aria-hidden="true"
              class="pointer-events-none absolute inset-x-0 top-0 h-1 origin-left scale-x-0 bg-gradient-to-r from-primary-500 to-primary-600 transition-transform duration-300 group-hover:scale-x-100"
            ></span>
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-primary-500 to-primary-600 shadow-lg shadow-primary-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M18 18.72a9.094 9.094 0 003.741-.479 3 3 0 00-4.682-2.72m.94 3.198l.001.031c0 .225-.012.447-.037.666A11.944 11.944 0 0112 21c-2.17 0-4.207-.576-5.963-1.584A6.062 6.062 0 016 18.719m12 0a5.971 5.971 0 00-.941-3.197m0 0A5.995 5.995 0 0012 12.75a5.995 5.995 0 00-5.058 2.772m0 0a3 3 0 00-4.681 2.72 8.986 8.986 0 003.74.477m.94-3.197a5.971 5.971 0 00-.94 3.197M15 6.75a3 3 0 11-6 0 3 3 0 016 0zm6 3a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0zm-13.5 0a2.25 2.25 0 11-4.5 0 2.25 2.25 0 014.5 0z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.multiAccount') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.multiAccountDesc') }}
            </p>
          </div>

          <!-- Feature 3: Billing & Quota -->
          <div
            class="feature-card group relative overflow-hidden rounded-2xl border border-gray-200/50 bg-white/60 p-6 backdrop-blur-sm transition-all duration-300 hover:-translate-y-1 hover:border-primary-300/50 hover:shadow-xl hover:shadow-primary-500/10 dark:border-dark-700/50 dark:bg-dark-800/60 dark:hover:border-primary-500/40"
          >
            <span
              aria-hidden="true"
              class="pointer-events-none absolute inset-x-0 top-0 h-1 origin-left scale-x-0 bg-gradient-to-r from-purple-500 to-purple-600 transition-transform duration-300 group-hover:scale-x-100"
            ></span>
            <div
              class="mb-4 flex h-12 w-12 items-center justify-center rounded-xl bg-gradient-to-br from-purple-500 to-purple-600 shadow-lg shadow-purple-500/30 transition-transform group-hover:scale-110"
            >
              <svg
                class="h-6 w-6 text-white"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
                stroke-width="1.5"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M2.25 18.75a60.07 60.07 0 0115.797 2.101c.727.198 1.453-.342 1.453-1.096V18.75M3.75 4.5v.75A.75.75 0 013 6h-.75m0 0v-.375c0-.621.504-1.125 1.125-1.125H20.25M2.25 6v9m18-10.5v.75c0 .414.336.75.75.75h.75m-1.5-1.5h.375c.621 0 1.125.504 1.125 1.125v9.75c0 .621-.504 1.125-1.125 1.125h-.375m1.5-1.5H21a.75.75 0 00-.75.75v.75m0 0H3.75m0 0h-.375a1.125 1.125 0 01-1.125-1.125V15m1.5 1.5v-.75A.75.75 0 003 15h-.75M15 10.5a3 3 0 11-6 0 3 3 0 016 0zm3 0h.008v.008H18V10.5zm-12 0h.008v.008H6V10.5z"
                />
              </svg>
            </div>
            <h3 class="mb-2 text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('home.features.balanceQuota') }}
            </h3>
            <p class="text-sm leading-relaxed text-gray-600 dark:text-dark-400">
              {{ t('home.features.balanceQuotaDesc') }}
            </p>
          </div>
        </div>

        <!--
          Conversion-oriented showcase that replaces the static "Supported
          Providers" pills. Lazily fetches the active recharge promo (if any)
          and the top 3 plan cards; gracefully hides itself when
          `payment_enabled === false` or all data sources fail.
        -->
        <HomeShowcaseSection />
      </div>
    </main>

    <!--
      Footer.

      Layout: copyright pinned left, secondary cluster (contact strip
      + docs link) pinned right via `justify-between`. The contact
      strip is rendered here — not in <main> — because the operator's
      contact info is "fine print" plumbing, not a conversion CTA;
      tucking it into the footer keeps the page's narrative arc
      (hero → features → plans/promo) uncluttered while still giving
      every visitor a one-glance way to reach a human.

      On mobile the inner row collapses to `flex-col` (`text-center`),
      so the copyright stacks above the strip — unchanged from prior
      footer behaviour, only `justify-center` → `justify-between` was
      flipped to make the right-pinning meaningful at sm+.
    -->
    <footer class="relative z-10 mt-auto border-t border-gray-200/50 px-6 py-8 dark:border-dark-800/50">
      <div
        class="mx-auto flex max-w-6xl flex-col items-center justify-between gap-4 text-center sm:flex-row sm:text-left"
      >
        <p class="text-sm text-gray-500 dark:text-dark-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex flex-wrap items-center justify-center gap-x-4 gap-y-2 sm:justify-end">
          <HomeContactSection />
          <!--
            Vertical divider between the contact strip and the docs
            link, matching the in-strip dividers inside
            HomeContactSection so the whole footer-right reads as a
            single pipe-separated list. `v-if="docUrl"` mirrors the
            link's own guard so we don't end up with a dangling
            divider when the docs URL isn't configured.
          -->
          <span
            v-if="docUrl"
            class="h-4 w-px bg-gray-300/70 dark:bg-dark-700"
            aria-hidden="true"
          ></span>
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-sm text-gray-500 transition-colors hover:text-gray-700 dark:text-dark-400 dark:hover:text-white"
          >
            {{ t('home.docs') }}
          </a>
        </div>
      </div>
    </footer>

    <Teleport to="body">
      <div
        v-if="homeProductsOpen"
        ref="homeProductsDropdownRef"
        class="fixed z-50 max-w-[calc(100vw-2rem)] rounded-lg border border-gray-200 bg-white py-2 shadow-lg dark:border-dark-700 dark:bg-dark-900"
        :style="homeProductsDropdownStyle"
        role="menu"
        @mouseenter="clearHomeProductsCloseTimer"
        @mouseleave="scheduleCloseHomeProductsMenu"
      >
        <a
          v-for="item in homeProductMenuItems"
          :key="item.id || item.url || item.label"
          :href="item.url"
          :target="item.action === 'new_tab' ? '_blank' : undefined"
          :rel="item.action === 'new_tab' ? 'noopener noreferrer' : undefined"
          class="flex items-center gap-2 px-4 py-2 text-sm text-gray-700 transition-colors hover:bg-gray-50 hover:text-gray-900 dark:text-dark-200 dark:hover:bg-dark-800 dark:hover:text-white"
          role="menuitem"
          @click="closeHomeProductsMenu"
        >
          <span
            v-if="item.icon_svg"
            class="h-4 w-4 shrink-0 [&>svg]:h-4 [&>svg]:w-4"
            v-html="item.icon_svg"
          ></span>
          <Icon v-else name="grid" size="sm" class="shrink-0" />
          <span class="truncate">{{ item.label }}</span>
        </a>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, nextTick, onMounted, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'
import HomeShowcaseSection from '@/components/home/HomeShowcaseSection.vue'
import HomeContactSection from '@/components/home/HomeContactSection.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

// Site settings - directly from appStore (already initialized from injected config)
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const homeProductMenuItems = computed(() =>
  (appStore.cachedPublicSettings?.home_product_menu_items || []).filter(
    (item) => item.label && item.url,
  ),
)
const homeProductsOpen = ref(false)
const homeProductsMenuRef = ref<HTMLElement | null>(null)
const homeProductsButtonRef = ref<HTMLElement | null>(null)
const homeProductsHeroMenuRef = ref<HTMLElement | null>(null)
const homeProductsHeroButtonRef = ref<HTMLElement | null>(null)
const homeProductsActiveButtonRef = ref<HTMLElement | null>(null)
const homeProductsActiveTrigger = ref<'nav' | 'hero' | null>(null)
const homeProductsDropdownRef = ref<HTMLElement | null>(null)
const homeProductsDropdownStyle = ref<Record<string, string>>({
  top: '64px',
  right: '16px',
  minWidth: '12rem',
})
const isHomeProductsNavOpen = computed(
  () => homeProductsOpen.value && homeProductsActiveTrigger.value === 'nav',
)
const isHomeProductsHeroOpen = computed(
  () => homeProductsOpen.value && homeProductsActiveTrigger.value === 'hero',
)
let homeProductsCloseTimer: number | null = null

function supportsHomeProductsHover(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return true
  }
  return window.matchMedia('(hover: hover) and (pointer: fine)').matches
}

function clearHomeProductsCloseTimer() {
  if (homeProductsCloseTimer) {
    window.clearTimeout(homeProductsCloseTimer)
    homeProductsCloseTimer = null
  }
}

function closeHomeProductsMenu() {
  clearHomeProductsCloseTimer()
  homeProductsOpen.value = false
  homeProductsActiveTrigger.value = null
}

function resolveHomeProductsTrigger(event?: Event): {
  button: HTMLElement | null
  trigger: 'nav' | 'hero' | null
} {
  const target = event?.currentTarget as HTMLElement | null
  if (target && homeProductsHeroMenuRef.value?.contains(target)) {
    return { button: homeProductsHeroButtonRef.value, trigger: 'hero' }
  }
  if (target && homeProductsMenuRef.value?.contains(target)) {
    return { button: homeProductsButtonRef.value, trigger: 'nav' }
  }
  return {
    button: homeProductsActiveButtonRef.value || homeProductsButtonRef.value,
    trigger: homeProductsActiveTrigger.value || 'nav',
  }
}

async function openHomeProductsMenu(
  trigger?: { button: HTMLElement | null; trigger: 'nav' | 'hero' | null },
) {
  clearHomeProductsCloseTimer()
  homeProductsActiveButtonRef.value =
    trigger?.button || homeProductsActiveButtonRef.value || homeProductsButtonRef.value
  homeProductsActiveTrigger.value = trigger?.trigger || homeProductsActiveTrigger.value || 'nav'
  homeProductsOpen.value = true
  await nextTick()
  updateHomeProductsDropdownPosition()
}

async function openHomeProductsMenuFromEvent(event: Event) {
  // Skip synthetic mouseenter fired right before click on touch devices,
  // which would immediately re-toggle the menu and swallow the first tap.
  if (event.type === 'mouseenter' && !supportsHomeProductsHover()) {
    return
  }
  await openHomeProductsMenu(resolveHomeProductsTrigger(event))
}

async function toggleHomeProductsMenuFromEvent(event: Event) {
  const trigger = resolveHomeProductsTrigger(event)
  // If already open from the same trigger, close it (click-to-collapse).
  if (
    homeProductsOpen.value &&
    trigger.trigger &&
    homeProductsActiveTrigger.value === trigger.trigger
  ) {
    closeHomeProductsMenu()
    return
  }
  await openHomeProductsMenu(trigger)
}

function scheduleCloseHomeProductsMenu() {
  clearHomeProductsCloseTimer()
  homeProductsCloseTimer = window.setTimeout(() => {
    closeHomeProductsMenu()
  }, 120)
}

function updateHomeProductsDropdownPosition() {
  const button = homeProductsActiveButtonRef.value || homeProductsButtonRef.value
  if (!button) return

  const rect = button.getBoundingClientRect()
  const minWidth = Math.max(192, Math.ceil(rect.width))
  const viewportWidth = window.innerWidth || document.documentElement.clientWidth
  const left = Math.max(16, Math.min(rect.right - minWidth, viewportWidth - minWidth - 16))

  homeProductsDropdownStyle.value = {
    top: `${Math.ceil(rect.bottom + 8)}px`,
    left: `${Math.ceil(left)}px`,
    minWidth: `${minWidth}px`,
  }
}

function handleHomeProductsDocumentClick(event: MouseEvent) {
  if (!homeProductsOpen.value) return
  const target = event.target as Node | null
  if (target && homeProductsMenuRef.value?.contains(target)) return
  if (target && homeProductsHeroMenuRef.value?.contains(target)) return
  if (target && homeProductsDropdownRef.value?.contains(target)) return
  closeHomeProductsMenu()
}

function handleHomeProductsKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    closeHomeProductsMenu()
  }
}

function handleHomeProductsViewportChange() {
  if (homeProductsOpen.value) {
    updateHomeProductsDropdownPosition()
  }
}

// Check if homeContent is a URL (for iframe display)
const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

// Theme
const isDark = ref(document.documentElement.classList.contains('dark'))

// Auth state
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const userInitial = computed(() => {
  const user = authStore.user
  if (!user || !user.email) return ''
  return user.email.charAt(0).toUpperCase()
})

// Current year for footer
const currentYear = computed(() => new Date().getFullYear())

// Toggle theme
function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

// Initialize theme
function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

// Nav horizontal scroll indicators (for narrow viewports)
const navScrollRef = ref<HTMLElement | null>(null)
const canScrollLeft = ref(false)
const canScrollRight = ref(false)
let navResizeObserver: ResizeObserver | null = null

function updateScrollIndicators() {
  const el = navScrollRef.value
  if (!el) {
    canScrollLeft.value = false
    canScrollRight.value = false
    return
  }
  const maxScroll = el.scrollWidth - el.clientWidth
  canScrollLeft.value = el.scrollLeft > 1
  canScrollRight.value = el.scrollLeft < maxScroll - 1
}

function scrollNav(direction: 1 | -1) {
  const el = navScrollRef.value
  if (!el) return
  const step = Math.max(120, Math.floor(el.clientWidth * 0.6))
  el.scrollBy({ left: direction * step, behavior: 'smooth' })
}

function attachNavScrollObserver() {
  const el = navScrollRef.value
  if (!el || typeof ResizeObserver === 'undefined') return
  navResizeObserver = new ResizeObserver(() => updateScrollIndicators())
  navResizeObserver.observe(el)
  // Also observe children changes affecting scrollWidth
  Array.from(el.children).forEach((child) => {
    if (child instanceof HTMLElement) navResizeObserver!.observe(child)
  })
}

function detachNavScrollObserver() {
  if (navResizeObserver) {
    navResizeObserver.disconnect()
    navResizeObserver = null
  }
}

onMounted(() => {
  initTheme()
  document.addEventListener('click', handleHomeProductsDocumentClick)
  document.addEventListener('keydown', handleHomeProductsKeydown)
  window.addEventListener('resize', handleHomeProductsViewportChange)
  window.addEventListener('scroll', handleHomeProductsViewportChange, true)
  window.addEventListener('resize', updateScrollIndicators)
  attachNavScrollObserver()
  nextTick(updateScrollIndicators)

  // Check auth state
  authStore.checkAuth()

  // Ensure public settings are loaded (will use cache if already loaded from injected config)
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})

onBeforeUnmount(() => {
  clearHomeProductsCloseTimer()
  document.removeEventListener('click', handleHomeProductsDocumentClick)
  document.removeEventListener('keydown', handleHomeProductsKeydown)
  window.removeEventListener('resize', handleHomeProductsViewportChange)
  window.removeEventListener('scroll', handleHomeProductsViewportChange, true)
  window.removeEventListener('resize', updateScrollIndicators)
  detachNavScrollObserver()
})
</script>

<style scoped>
/* Nav Actions - hide scrollbar but allow horizontal scroll on mobile */
.nav-actions {
  -ms-overflow-style: none;
  scrollbar-width: none;
}
.nav-actions::-webkit-scrollbar {
  display: none;
}

/* Terminal Container */
.terminal-container {
  position: relative;
  display: inline-block;
}

/* Terminal Window */
.terminal-window {
  width: 420px;
  background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%);
  border-radius: 14px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.4),
    0 0 0 1px rgba(255, 255, 255, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  overflow: hidden;
  transform: perspective(1000px) rotateX(2deg) rotateY(-2deg);
  transition: transform 0.3s ease;
}

.terminal-window:hover {
  transform: perspective(1000px) rotateX(0deg) rotateY(0deg) translateY(-4px);
}

/* Terminal Header */
.terminal-header {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: rgba(30, 41, 59, 0.8);
  border-bottom: 1px solid rgba(255, 255, 255, 0.05);
}

.terminal-buttons {
  display: flex;
  gap: 8px;
}

.terminal-buttons span {
  width: 12px;
  height: 12px;
  border-radius: 50%;
}

.btn-close {
  background: #ef4444;
}
.btn-minimize {
  background: #eab308;
}
.btn-maximize {
  background: #22c55e;
}

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  margin-right: 52px;
}

/* Terminal Body */
.terminal-body {
  padding: 20px 24px;
  font-family: ui-monospace, 'Fira Code', monospace;
  font-size: 14px;
  line-height: 2;
}

.code-line {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  opacity: 0;
  animation: line-appear 0.5s ease forwards;
}

.line-1 {
  animation-delay: 0.3s;
}
.line-2 {
  animation-delay: 1s;
}
.line-3 {
  animation-delay: 1.8s;
}
.line-4 {
  animation-delay: 2.5s;
}

@keyframes line-appear {
  from {
    opacity: 0;
    transform: translateY(5px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.code-prompt {
  color: #22c55e;
  font-weight: bold;
}
.code-cmd {
  color: #38bdf8;
}
.code-flag {
  color: #a78bfa;
}
.code-url {
  color: #14b8a6;
}
.code-comment {
  color: #64748b;
  font-style: italic;
}
.code-success {
  color: #22c55e;
  background: rgba(34, 197, 94, 0.15);
  padding: 2px 8px;
  border-radius: 4px;
  font-weight: 600;
}
.code-response {
  color: #fbbf24;
}

/* Blinking Cursor */
.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #22c55e;
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%,
  50% {
    opacity: 1;
  }
  51%,
  100% {
    opacity: 0;
  }
}

/* Dark mode adjustments */
:deep(.dark) .terminal-window {
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.6),
    0 0 0 1px rgba(20, 184, 166, 0.2),
    0 0 40px rgba(20, 184, 166, 0.1),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}
</style>
