<template>
  <!-- 单例连接被踢出时显示非阻塞通知；用户可继续操作页面或切回当前窗口。 -->
  <Teleport to="body">
    <Transition name="toast-slide">
      <div
        v-if="inboxStore.kicked"
        class="fixed right-4 top-4 z-[9998] w-[min(24rem,calc(100vw-2rem))]"
        role="alert"
        aria-live="assertive"
      >
        <div
          class="overflow-hidden rounded-lg border border-yellow-300 bg-yellow-50 shadow-lg dark:border-yellow-700 dark:bg-yellow-950"
        >
          <div class="flex items-start gap-3 p-4">
            <div
              class="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-yellow-100 text-yellow-600 dark:bg-yellow-900 dark:text-yellow-300"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4a2 2 0 00-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z"
                />
              </svg>
            </div>
            <div class="min-w-0 flex-1">
              <h2 class="text-sm font-semibold text-yellow-900 dark:text-yellow-100">{{ t('inbox.kicked.title') }}</h2>
              <p class="mt-1 text-sm leading-relaxed text-yellow-800 dark:text-yellow-200">{{ t('inbox.kicked.description') }}</p>
              <button type="button" class="mt-3 text-sm font-medium text-yellow-900 underline underline-offset-2 hover:text-yellow-700 dark:text-yellow-100 dark:hover:text-yellow-300" @click="inboxStore.resume()">
                {{ t('inbox.kicked.resume') }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useInboxStore } from '@/stores/inbox'

const { t } = useI18n()
const inboxStore = useInboxStore()
</script>

<style scoped>
.toast-slide-enter-active {
  transition: all 0.25s ease-out;
}
.toast-slide-leave-active {
  transition: all 0.2s ease-in;
}
.toast-slide-enter-from,
.toast-slide-leave-to {
  opacity: 0;
  transform: translateX(100%);
}
</style>
