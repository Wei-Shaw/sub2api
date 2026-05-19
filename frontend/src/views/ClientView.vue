<template>
  <div class="min-h-screen bg-gradient-to-br from-gray-50 via-primary-50/30 to-gray-100 text-gray-900 dark:from-dark-950 dark:via-dark-900 dark:to-dark-950 dark:text-white">
    <header class="border-b border-gray-200/50 bg-white/40 backdrop-blur-md dark:border-dark-800/50 dark:bg-dark-950/40">
      <nav class="mx-auto flex max-w-6xl items-center justify-between px-6 py-3">
        <router-link to="/home" class="flex items-center gap-2.5">
          <div class="h-8 w-8 overflow-hidden rounded-lg">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </div>
          <span class="text-lg font-bold">{{ siteName }}</span>
        </router-link>
        <div class="flex items-center gap-2">
          <LocaleSwitcher />
          <router-link
            to="/home"
            class="rounded-lg px-3 py-1.5 text-sm text-gray-500 transition-colors hover:bg-gray-100 hover:text-gray-700 dark:text-dark-300 dark:hover:bg-dark-800 dark:hover:text-white"
          >
            返回首页
          </router-link>
          <router-link
            :to="isAuthenticated ? dashboardPath : '/login'"
            class="inline-flex items-center rounded-lg bg-gray-900 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-gray-800 dark:bg-gray-800 dark:hover:bg-gray-700"
          >
            {{ isAuthenticated ? '进入控制台' : '登录' }}
          </router-link>
        </div>
      </nav>
    </header>

    <main class="px-6 py-16">
      <section class="mx-auto grid max-w-6xl items-center gap-12 lg:grid-cols-[1.05fr_0.95fr]">
        <div>
          <p class="mb-3 text-sm font-medium uppercase tracking-wider text-primary-500">起源 AI 客户端</p>
          <h1 class="mb-5 text-4xl font-bold leading-tight md:text-5xl">
            下载客户端，一键配置 AI 工具
          </h1>
          <p class="mb-8 max-w-2xl text-lg leading-relaxed text-gray-500 dark:text-dark-300">
            自动写入 API Base URL 和密钥配置，快速接入 Codex CLI、Claude Code、OpenAI SDK 等常用开发工具。
          </p>
          <div class="flex flex-wrap items-center gap-3">
            <a
              :href="windowsSetupUrl"
              class="inline-flex items-center rounded-lg bg-gray-900 px-6 py-3 text-sm font-medium text-white transition-colors hover:bg-gray-800"
            >
              下载 Windows 安装包
            </a>
            <a
              :href="windowsMsiUrl"
              class="inline-flex items-center rounded-lg border border-gray-300 bg-white px-6 py-3 text-sm font-medium text-gray-700 transition-colors hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-800 dark:text-dark-200 dark:hover:bg-dark-700"
            >
              下载 MSI
            </a>
          </div>
          <p class="mt-4 text-xs text-gray-400 dark:text-dark-300">
            当前支持 Windows x64，macOS / Linux 版本准备中。
          </p>
        </div>

        <div class="rounded-lg border border-gray-200/60 bg-white/80 p-5 shadow-xl shadow-gray-900/5 backdrop-blur dark:border-dark-700/60 dark:bg-dark-800/80">
          <div class="mb-4 flex items-center justify-between border-b border-gray-100 pb-3 dark:border-dark-700">
            <div class="flex items-center gap-2">
              <span class="h-2.5 w-2.5 rounded-full bg-red-400"></span>
              <span class="h-2.5 w-2.5 rounded-full bg-yellow-400"></span>
              <span class="h-2.5 w-2.5 rounded-full bg-green-400"></span>
            </div>
            <span class="font-mono text-xs text-gray-400">mcorgai.com/client</span>
          </div>
          <div class="space-y-4">
            <div class="rounded-lg bg-gray-50 p-4 dark:bg-dark-900/70">
              <p class="text-xs uppercase tracking-wider text-gray-400">配置状态</p>
              <p class="mt-1 text-lg font-semibold">一键完成</p>
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
                <p class="text-xs text-gray-400">Base URL</p>
                <p class="mt-1 font-mono text-sm text-primary-500">https://api.mcorgai.com</p>
              </div>
              <div class="rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
                <p class="text-xs text-gray-400">工具支持</p>
                <p class="mt-1 text-sm font-semibold">Codex / Claude / OpenAI</p>
              </div>
            </div>
            <ol class="space-y-3 text-sm text-gray-600 dark:text-dark-300">
              <li class="flex gap-3">
                <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary-500 text-xs font-semibold text-white">1</span>
                <span>登录起源 AI 账号并选择 API Key。</span>
              </li>
              <li class="flex gap-3">
                <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary-500 text-xs font-semibold text-white">2</span>
                <span>选择要配置的开发工具。</span>
              </li>
              <li class="flex gap-3">
                <span class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-primary-500 text-xs font-semibold text-white">3</span>
                <span>客户端自动写入配置，打开工具即可使用。</span>
              </li>
            </ol>
          </div>
        </div>
      </section>

      <section class="mx-auto mt-16 grid max-w-6xl gap-4 md:grid-cols-3">
        <div class="rounded-lg border border-gray-200/60 bg-white/70 p-5 dark:border-dark-700/60 dark:bg-dark-800/70">
          <h2 class="mb-2 text-base font-semibold">少复制</h2>
          <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">
            不再手动找配置文件，减少 Base URL、环境变量和模型名填错的情况。
          </p>
        </div>
        <div class="rounded-lg border border-gray-200/60 bg-white/70 p-5 dark:border-dark-700/60 dark:bg-dark-800/70">
          <h2 class="mb-2 text-base font-semibold">多工具</h2>
          <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">
            同一个客户端统一管理 Codex CLI、Claude Code、OpenAI SDK 等工具配置。
          </p>
        </div>
        <div class="rounded-lg border border-gray-200/60 bg-white/70 p-5 dark:border-dark-700/60 dark:bg-dark-800/70">
          <h2 class="mb-2 text-base font-semibold">可回滚</h2>
          <p class="text-sm leading-relaxed text-gray-500 dark:text-dark-300">
            配置前保留原始设置，切换账号或恢复默认配置更稳妥。
          </p>
        </div>
      </section>
    </main>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || '起源 AI')
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')

const windowsSetupUrl = '/downloads/qiyuan-ai-client-windows-x64-setup.exe'
const windowsMsiUrl = '/downloads/qiyuan-ai-client-windows-x64.msi'

onMounted(() => {
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>
