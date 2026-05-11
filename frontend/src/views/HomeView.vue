<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe v-if="isHomeContentUrl" :src="homeContent.trim()" class="h-screen w-full border-0" allowfullscreen></iframe>
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div v-else class="relative flex h-screen flex-col bg-gray-50 dark:bg-dark-950">
    <PublicPageHeader />

    <!-- Hero Section -->
    <section class="flex flex-1 flex-col items-center justify-center overflow-hidden">
      <div class="absolute inset-0 bg-gradient-to-b from-primary-50/40 to-transparent dark:from-primary-950/20 dark:to-transparent"></div>
      <div class="relative mx-auto max-w-7xl px-6 py-16 sm:py-20">
        <div class="mx-auto max-w-3xl text-center">
          <div class="mb-6 inline-flex items-center gap-2 rounded-full border border-primary-200 bg-primary-50 px-4 py-1.5 dark:border-primary-800 dark:bg-primary-950/40">
            <span class="h-1.5 w-1.5 rounded-full bg-primary-500"></span>
            <span class="text-sm font-medium text-primary-700 dark:text-primary-300">{{ t('home.tags.heroTag') }}</span>
          </div>
          <h1 class="text-4xl font-extrabold tracking-tight text-gray-900 dark:text-white sm:text-5xl lg:text-6xl">
            {{ siteName }}
          </h1>
          <p class="mx-auto mt-6 max-w-2xl text-lg leading-relaxed text-gray-600 dark:text-dark-300">
            {{ siteSubtitle }}
          </p>
          <div class="mt-10 flex flex-col items-center gap-4 sm:flex-row sm:justify-center">
            <router-link :to="isAuthenticated ? dashboardPath : '/login'"
              class="inline-flex items-center gap-2 rounded-lg bg-primary-600 px-8 py-3.5 text-base font-semibold text-white shadow-lg shadow-primary-500/25 transition-all hover:bg-primary-700 hover:shadow-xl hover:shadow-primary-500/30">
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="md" :stroke-width="2" />
            </router-link>
            <a v-if="docUrl" :href="docUrl" target="_blank" rel="noopener noreferrer"
              class="inline-flex items-center gap-2 rounded-lg border border-gray-300 bg-white px-8 py-3.5 text-base font-semibold text-gray-700 transition-all hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700">
              {{ t('home.viewDocs') }}
            </a>
          </div>
        </div>

        <!-- Terminal Preview -->
        <div class="mx-auto mt-16" style="max-width: 72rem; width: 100%;">
          <div class="terminal-window">
            <div class="terminal-header">
              <div class="terminal-buttons">
                <span class="btn-close"></span>
                <span class="btn-minimize"></span>
                <span class="btn-maximize"></span>
              </div>
              <span class="terminal-title">terminal</span>
            </div>
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
    </section>

    <!-- Footer -->
    <PublicPageFooter />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import Icon from '@/components/icons/Icon.vue'
import PublicPageHeader from '@/components/common/PublicPageHeader.vue'
import PublicPageFooter from '@/components/common/PublicPageFooter.vue'

const { t } = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'AI API Gateway Platform')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style scoped>
.terminal-window {
  background: linear-gradient(145deg, #1e293b 0%, #0f172a 100%);
  border-radius: 14px;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.15), 0 0 0 1px rgba(0, 0, 0, 0.05);
  overflow: hidden;
}

:deep(.dark) .terminal-window {
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.4), 0 0 0 1px rgba(255, 255, 255, 0.06);
}

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

.btn-close { background: #ef4444; }
.btn-minimize { background: #eab308; }
.btn-maximize { background: #22c55e; }

.terminal-title {
  flex: 1;
  text-align: center;
  font-size: 12px;
  font-family: ui-monospace, monospace;
  color: #64748b;
  margin-right: 52px;
}

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

.line-1 { animation-delay: 0.3s; }
.line-2 { animation-delay: 1s; }
.line-3 { animation-delay: 1.8s; }
.line-4 { animation-delay: 2.5s; }

@keyframes line-appear {
  from { opacity: 0; transform: translateY(5px); }
  to { opacity: 1; transform: translateY(0); }
}

.code-prompt { color: #22c55e; font-weight: bold; }
.code-cmd { color: #38bdf8; }
.code-flag { color: #a78bfa; }
.code-url { color: #14b8a6; }
.code-comment { color: #64748b; font-style: italic; }
.code-success { color: #22c55e; background: rgba(34, 197, 94, 0.15); padding: 2px 8px; border-radius: 4px; font-weight: 600; }
.code-response { color: #fbbf24; }

.cursor {
  display: inline-block;
  width: 8px;
  height: 16px;
  background: #22c55e;
  animation: blink 1s step-end infinite;
}

@keyframes blink {
  0%, 50% { opacity: 1; }
  51%, 100% { opacity: 0; }
}
</style>
