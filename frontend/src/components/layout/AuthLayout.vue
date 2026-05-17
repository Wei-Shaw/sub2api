<template>
  <div class="auth-shell relative flex min-h-screen items-center justify-center overflow-hidden bg-[#FCFDFD] p-4 text-slate-950 dark:bg-slate-950 dark:text-white">
    <div class="pointer-events-none absolute inset-0 overflow-hidden">
      <div class="auth-mist auth-mist-violet"></div>
      <div class="auth-mist auth-mist-cyan"></div>
      <div
        class="auth-grid absolute inset-0 bg-[linear-gradient(rgba(15,23,42,0.045)_1px,transparent_1px),linear-gradient(90deg,rgba(15,23,42,0.045)_1px,transparent_1px)] bg-[size:56px_56px] [mask-image:radial-gradient(ellipse_at_center,black_28%,transparent_74%)] dark:bg-[linear-gradient(rgba(255,255,255,0.05)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.05)_1px,transparent_1px)]"
      ></div>
    </div>

    <div class="relative z-10 w-full max-w-md">
      <div class="mb-8 text-center">
        <template v-if="settingsLoaded">
          <div class="mb-3 inline-flex items-center justify-center gap-2.5">
            <span v-if="siteLogo" class="flex h-9 w-9 overflow-hidden rounded-md">
              <img :src="siteLogo" alt="Logo" class="h-full w-full object-contain" />
            </span>
            <span
              v-else
              class="auth-brand-mark inline-flex h-9 w-9 items-center justify-center rounded-md bg-slate-950 font-[Inter,Geist,system-ui,sans-serif] text-[12px] font-extrabold tracking-tight text-white shadow-[0_12px_30px_rgba(15,23,42,0.18)] dark:bg-white dark:text-slate-950"
            >
              DR
            </span>
            <h1 class="auth-brand-name font-[Inter,Geist,system-ui,sans-serif] text-xl font-extrabold tracking-tight text-slate-950 dark:text-white">
              {{ siteName }}
            </h1>
          </div>
        </template>
      </div>

      <div
        class="auth-card rounded-2xl border border-slate-200/70 bg-white/85 p-8 shadow-[0_25px_50px_-12px_rgba(15,23,42,0.05),0_0_0_1px_rgba(0,0,0,0.02),0_30px_80px_-32px_rgba(139,92,246,0.22)] backdrop-blur-2xl dark:border-white/10 dark:bg-white/[0.08] dark:shadow-[inset_0_1px_0_0_rgba(255,255,255,0.05),0_25px_70px_-20px_rgba(0,0,0,0.55),0_0_0_1px_rgba(255,255,255,0.08)]"
      >
        <slot />
      </div>

      <div class="mt-6 text-center text-sm">
        <slot name="footer" />
      </div>

      <div class="mt-8 text-center text-xs text-slate-400 dark:text-slate-600">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'DevRouter')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-mist {
  position: absolute;
  width: 520px;
  height: 520px;
  border-radius: 9999px;
  filter: blur(130px);
  opacity: 0.16;
}

.auth-mist-violet {
  left: 50%;
  top: 42%;
  transform: translate(-62%, -46%);
  background: #8b5cf6;
}

.auth-mist-cyan {
  left: 50%;
  top: 50%;
  transform: translate(-20%, -36%);
  background: #06b6d4;
}
</style>
