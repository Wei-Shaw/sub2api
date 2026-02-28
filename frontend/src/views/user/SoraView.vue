<template>
  <div class="sora-root">
    <!-- Sora 页面内容 -->
    <div class="sora-page">
      <!-- 功能未启用提示 -->
      <div v-if="!soraEnabled" class="sora-not-enabled">
        <svg class="sora-not-enabled-icon" fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M9.813 15.904L9 18.75l-.813-2.846a4.5 4.5 0 00-3.09-3.09L2.25 12l2.846-.813a4.5 4.5 0 003.09-3.09L9 5.25l.813 2.846a4.5 4.5 0 003.09 3.09L15.75 12l-2.846.813a4.5 4.5 0 00-3.09 3.09zM18.259 8.715L18 9.75l-.259-1.035a3.375 3.375 0 00-2.455-2.456L14.25 6l1.036-.259a3.375 3.375 0 002.455-2.456L18 2.25l.259 1.035a3.375 3.375 0 002.455 2.456L21.75 6l-1.036.259a3.375 3.375 0 00-2.455 2.456z" />
        </svg>
        <h2 class="sora-not-enabled-title">{{ t('sora.notEnabled') REDACTEDREDACTED</h2>
        <p class="sora-not-enabled-desc">{{ t('sora.notEnabledDesc') REDACTEDREDACTED</p>
      </div>

      <!-- Sora 主界面 -->
      <template v-else>
        <!-- 自定义 Sora 头部 -->
        <header class="sora-header">
          <div class="sora-header-left">
            <!-- 返回主页按钮 -->
            <router-link :to="dashboardPath" class="sora-back-btn" :title="t('common.back')">
              <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <path d="M15 19l-7-7 7-7" />
              </svg>
            </router-link>
            <nav class="sora-nav-tabs">
              <button
                v-for="tab in tabs"
                :key="tab.key"
                :class="['sora-nav-tab', activeTab === tab.key && 'active']"
                @click="activeTab = tab.key"
              >
                {{ tab.label REDACTEDREDACTED
              </button>
            </nav>
          </div>
          <div class="sora-header-right">
            <SoraQuotaBar v-if="quota" :quota="quota" />
            <div v-if="activeTaskCount > 0" class="sora-queue-indicator">
              <span class="sora-queue-dot" :class="{ busy: hasGeneratingTask REDACTED"></span>
              <span>{{ activeTaskCount REDACTEDREDACTED {{ t('sora.queueTasks') REDACTEDREDACTED</span>
            </div>
          </div>
        </header>

        <!-- 内容区域 -->
        <main class="sora-main">
          <SoraGeneratePage
            v-show="activeTab === 'generate'"
            @task-count-change="onTaskCountChange"
          />
          <SoraLibraryPage
            v-show="activeTab === 'library'"
            @switch-to-generate="activeTab = 'generate'"
          />
        </main>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted REDACTED from 'vue'
import { useI18n REDACTED from 'vue-i18n'
import { useAppStore, useAuthStore REDACTED from '@/stores'
import SoraQuotaBar from '@/components/sora/SoraQuotaBar.vue'
import SoraGeneratePage from '@/components/sora/SoraGeneratePage.vue'
import SoraLibraryPage from '@/components/sora/SoraLibraryPage.vue'
import soraAPI, { type QuotaInfo REDACTED from '@/api/sora'

const { t REDACTED = useI18n()
const authStore = useAuthStore()
const appStore = useAppStore()

const soraEnabled = computed(() => appStore.cachedPublicSettings?.sora_client_enabled ?? false)

const activeTab = ref<'generate' | 'library'>('generate')
const quota = ref<QuotaInfo | null>(null)
const activeTaskCount = ref(0)
const hasGeneratingTask = ref(false)
const dashboardPath = computed(() => (authStore.isAdmin ? '/admin/dashboard' : '/dashboard'))

const tabs = computed(() => [
  { key: 'generate' as const, label: t('sora.tabGenerate') REDACTED,
  { key: 'library' as const, label: t('sora.tabLibrary') REDACTED
])

function onTaskCountChange(counts: { active: number; generating: boolean REDACTED) {
  activeTaskCount.value = counts.active
  hasGeneratingTask.value = counts.generating
REDACTED

onMounted(async () => {
  if (!soraEnabled.value) return
  try {
    quota.value = await soraAPI.getQuota()
  REDACTED catch {
    // 配额查询失败不阻塞页面
  REDACTED
REDACTED)
</script>

<style scoped>
/* ============================================================
   Sora 主题 CSS 变量 — 亮色模式（跟随应用主题）
   ============================================================ */
.sora-root {
  --sora-bg-primary: #F9FAFB;
  --sora-bg-secondary: #FFFFFF;
  --sora-bg-tertiary: #F3F4F6;
  --sora-bg-elevated: #FFFFFF;
  --sora-bg-hover: #E5E7EB;
  --sora-bg-input: #FFFFFF;
  --sora-text-primary: #111827;
  --sora-text-secondary: #6B7280;
  --sora-text-tertiary: #9CA3AF;
  --sora-text-muted: #D1D5DB;
  --sora-accent-primary: #14b8a6;
  --sora-accent-secondary: #0d9488;
  --sora-accent-gradient: linear-gradient(135deg, #14b8a6 0%, #0d9488 100%);
  --sora-accent-gradient-hover: linear-gradient(135deg, #2dd4bf 0%, #14b8a6 100%);
  --sora-success: #10B981;
  --sora-warning: #F59E0B;
  --sora-error: #EF4444;
  --sora-info: #3B82F6;
  --sora-border-color: #E5E7EB;
  --sora-border-subtle: #F3F4F6;
  --sora-radius-sm: 8px;
  --sora-radius-md: 12px;
  --sora-radius-lg: 16px;
  --sora-radius-xl: 20px;
  --sora-radius-full: 9999px;
  --sora-shadow-sm: 0 1px 2px rgba(0,0,0,0.05);
  --sora-shadow-md: 0 4px 12px rgba(0,0,0,0.08);
  --sora-shadow-lg: 0 8px 32px rgba(0,0,0,0.12);
  --sora-shadow-glow: 0 0 20px rgba(20,184,166,0.25);
  --sora-transition-fast: 150ms ease;
  --sora-transition-normal: 250ms ease;
  --sora-header-height: 56px;
  --sora-header-bg: rgba(249, 250, 251, 0.85);
  --sora-placeholder-gradient: linear-gradient(135deg, #e0e7ff 0%, #dbeafe 50%, #cffafe 100%);
  --sora-modal-backdrop: rgba(0, 0, 0, 0.4);

  min-height: 100vh;
  background: var(--sora-bg-primary);
  color: var(--sora-text-primary);
  font-family: -apple-system, BlinkMacSystemFont, "SF Pro Display", "Segoe UI", "PingFang SC", "Noto Sans SC", sans-serif;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
REDACTED

/* ============================================================
   页面布局
   ============================================================ */
.sora-page {
  width: 100%;
REDACTED

/* ============================================================
   头部导航栏
   ============================================================ */
.sora-header {
  position: sticky;
  top: 0;
  z-index: 30;
  height: var(--sora-header-height);
  background: var(--sora-header-bg);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  border-bottom: 1px solid var(--sora-border-subtle);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
REDACTED

.sora-header-left {
  display: flex;
  align-items: center;
  gap: 24px;
REDACTED

.sora-header-right {
  display: flex;
  align-items: center;
  gap: 16px;
REDACTED

/* 返回按钮 */
.sora-back-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: var(--sora-radius-sm);
  color: var(--sora-text-secondary);
  text-decoration: none;
  transition: all var(--sora-transition-fast);
REDACTED

.sora-back-btn:hover {
  background: var(--sora-bg-tertiary);
  color: var(--sora-text-primary);
REDACTED

/* Tab 导航 */
.sora-nav-tabs {
  display: flex;
  gap: 4px;
  background: var(--sora-bg-secondary);
  border-radius: var(--sora-radius-full);
  padding: 3px;
REDACTED

.sora-nav-tab {
  padding: 6px 20px;
  border-radius: var(--sora-radius-full);
  font-size: 13px;
  font-weight: 500;
  color: var(--sora-text-secondary);
  background: none;
  border: none;
  cursor: pointer;
  transition: all var(--sora-transition-fast);
  user-select: none;
REDACTED

.sora-nav-tab:hover {
  color: var(--sora-text-primary);
REDACTED

.sora-nav-tab.active {
  background: var(--sora-bg-tertiary);
  color: var(--sora-text-primary);
REDACTED

/* 队列指示器 */
.sora-queue-indicator {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  background: var(--sora-bg-secondary);
  border-radius: var(--sora-radius-full);
  font-size: 12px;
  color: var(--sora-text-secondary);
REDACTED

.sora-queue-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--sora-success);
  animation: sora-pulse-dot 2s ease-in-out infinite;
REDACTED

.sora-queue-dot.busy {
  background: var(--sora-warning);
REDACTED

@keyframes sora-pulse-dot {
  0%, 100% { opacity: 1; REDACTED
  50% { opacity: 0.4; REDACTED
REDACTED

/* ============================================================
   主内容区
   ============================================================ */
.sora-main {
  min-height: calc(100vh - var(--sora-header-height));
REDACTED

/* ============================================================
   功能未启用
   ============================================================ */
.sora-not-enabled {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  text-align: center;
  padding: 40px;
REDACTED

.sora-not-enabled-icon {
  width: 64px;
  height: 64px;
  color: var(--sora-text-tertiary);
  margin-bottom: 16px;
REDACTED

.sora-not-enabled-title {
  font-size: 20px;
  font-weight: 600;
  color: var(--sora-text-secondary);
  margin-bottom: 8px;
REDACTED

.sora-not-enabled-desc {
  font-size: 14px;
  color: var(--sora-text-tertiary);
  max-width: 400px;
REDACTED

/* ============================================================
   响应式
   ============================================================ */
@media (max-width: 900px) {
  .sora-header {
    padding: 0 16px;
  REDACTED

  .sora-header-left {
    gap: 12px;
  REDACTED
REDACTED

@media (max-width: 600px) {
  .sora-nav-tab {
    padding: 5px 14px;
    font-size: 12px;
  REDACTED
REDACTED

/* 滚动条 */
.sora-root ::-webkit-scrollbar {
  width: 6px;
  height: 6px;
REDACTED

.sora-root ::-webkit-scrollbar-track {
  background: transparent;
REDACTED

.sora-root ::-webkit-scrollbar-thumb {
  background: var(--sora-bg-hover);
  border-radius: 3px;
REDACTED

.sora-root ::-webkit-scrollbar-thumb:hover {
  background: var(--sora-text-tertiary);
REDACTED
</style>

<style>
/* 暗色模式：必须明确命中 .sora-root，避免被 scoped 编译后的变量覆盖问题 */
html.dark .sora-root {
  --sora-bg-primary: #020617;
  --sora-bg-secondary: #0f172a;
  --sora-bg-tertiary: #1e293b;
  --sora-bg-elevated: #1e293b;
  --sora-bg-hover: #334155;
  --sora-bg-input: #0f172a;
  --sora-text-primary: #f1f5f9;
  --sora-text-secondary: #94a3b8;
  --sora-text-tertiary: #64748b;
  --sora-text-muted: #475569;
  --sora-border-color: #334155;
  --sora-border-subtle: #1e293b;
  --sora-shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.3);
  --sora-shadow-md: 0 4px 12px rgba(0, 0, 0, 0.4);
  --sora-shadow-lg: 0 8px 32px rgba(0, 0, 0, 0.5);
  --sora-shadow-glow: 0 0 20px rgba(20, 184, 166, 0.3);
  --sora-header-bg: rgba(2, 6, 23, 0.85);
  --sora-placeholder-gradient: linear-gradient(135deg, #1e293b 0%, #0f172a 50%, #020617 100%);
  --sora-modal-backdrop: rgba(0, 0, 0, 0.7);
REDACTED
</style>
