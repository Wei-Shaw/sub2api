<template>
  <!--
    通用信箱单例连接遮罩（general-inbox PR-5）。
    当同一账号在别处打开、当前连接被服务端踢出（收到 {type:"kicked"}）时展示，
    阻止用户在"沉默失联"状态下继续操作；点"在此继续"重连并反向踢出其他端。
  -->
  <Teleport to="body">
    <Transition name="modal-fade">
      <div
        v-if="inboxStore.kicked"
        class="fixed inset-0 z-[200] flex items-center justify-center bg-black/60 p-4 backdrop-blur-sm"
      >
        <div
          class="w-full max-w-[420px] overflow-hidden rounded-2xl bg-white shadow-2xl ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
        >
          <div class="px-6 py-6 text-center">
            <div
              class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-full bg-amber-100 text-amber-600 dark:bg-amber-900/30 dark:text-amber-400"
            >
              <svg class="h-7 w-7" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  d="M12 9v2m0 4h.01M5.07 19h13.86c1.54 0 2.5-1.67 1.73-3L13.73 4a2 2 0 00-3.46 0L3.34 16c-.77 1.33.19 3 1.73 3z"
                />
              </svg>
            </div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
              {{ t('inbox.kicked.title') }}
            </h2>
            <p class="mt-2 text-sm text-gray-600 dark:text-gray-400">
              {{ t('inbox.kicked.description') }}
            </p>
          </div>
          <div class="flex items-center justify-center border-t border-gray-100 px-6 py-4 dark:border-dark-700">
            <button
              type="button"
              class="rounded-xl bg-gradient-to-r from-blue-600 to-indigo-600 px-6 py-2.5 text-sm font-medium text-white shadow-lg shadow-blue-500/30 transition-all hover:shadow-xl hover:scale-105"
              @click="inboxStore.resume()"
            >
              {{ t('inbox.kicked.resume') }}
            </button>
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
.modal-fade-enter-active {
  transition: all 0.25s cubic-bezier(0.16, 1, 0.3, 1);
}
.modal-fade-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 1, 1);
}
.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}
</style>
