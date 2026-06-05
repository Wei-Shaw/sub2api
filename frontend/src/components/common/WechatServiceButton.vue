<template>
  <!-- 悬浮按钮 - 使用主题色 -->
  <button
    @click="showModal = true"
    class="fixed bottom-6 right-6 z-50 flex items-center gap-2 rounded-full bg-gradient-to-r from-primary-500 to-primary-600 px-4 py-3 text-white shadow-lg shadow-primary-500/25 transition-all hover:from-primary-600 hover:to-primary-700 hover:shadow-xl hover:shadow-primary-500/30"
  >
    <svg class="h-5 w-5" viewBox="0 0 24 24" fill="currentColor">
      <path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 01.213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.328.328 0 00.186-.059l2.114-1.225a.87.87 0 01.415-.106.807.807 0 01.213.026 10.07 10.07 0 002.696.37c.262 0 .52-.011.776-.028a5.91 5.91 0 01-.193-1.479c0-3.644 3.374-6.6 7.536-6.6.262 0 .52.011.776.028-.628-3.513-4.27-6.472-8.885-6.472zM5.785 5.97a1.1 1.1 0 110 2.2 1.1 1.1 0 010-2.2zm5.813 0a1.1 1.1 0 110 2.2 1.1 1.1 0 010-2.2zm5.192 2.642c-3.703 0-6.71 2.567-6.71 5.73 0 3.163 3.007 5.73 6.71 5.73a7.9 7.9 0 002.126-.288.644.644 0 01.17-.022.69.69 0 01.329.085l1.672.97a.262.262 0 00.147.046c.128 0 .23-.104.23-.233a.403.403 0 00-.038-.168l-.309-1.17a.468.468 0 01.168-.527c1.449-1.065 2.374-2.643 2.374-4.423 0-3.163-3.007-5.73-6.71-5.73h-.159zm-2.434 3.34a.88.88 0 110 1.76.88.88 0 010-1.76zm4.868 0a.88.88 0 110 1.76.88.88 0 010-1.76z"/>
    </svg>
    <span class="text-sm font-medium">客服</span>
  </button>

  <!-- 弹窗 -->
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="showModal"
        class="fixed inset-0 z-[100] flex items-center justify-center bg-black/50 p-4 backdrop-blur-sm"
        @click.self="showModal = false"
      >
        <Transition name="scale">
          <div
            v-if="showModal"
            class="relative w-full max-w-sm rounded-2xl bg-white p-6 shadow-2xl dark:bg-dark-700"
          >
            <!-- 关闭按钮 -->
            <button
              @click="showModal = false"
              class="absolute right-4 top-4 text-gray-400 transition-colors hover:text-gray-600 dark:text-dark-400 dark:hover:text-dark-200"
            >
              <svg class="h-5 w-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M6 18L18 6M6 6l12 12" />
              </svg>
            </button>

            <!-- 标题 -->
            <div class="mb-4 flex items-center gap-3">
              <div class="flex h-10 w-10 items-center justify-center rounded-full bg-gradient-to-br from-primary-500 to-primary-600">
                <svg class="h-6 w-6 text-white" viewBox="0 0 24 24" fill="currentColor">
                  <path d="M8.691 2.188C3.891 2.188 0 5.476 0 9.53c0 2.212 1.17 4.203 3.002 5.55a.59.59 0 01.213.665l-.39 1.48c-.019.07-.048.141-.048.213 0 .163.13.295.29.295a.328.328 0 00.186-.059l2.114-1.225a.87.87 0 01.415-.106.807.807 0 01.213.026 10.07 10.07 0 002.696.37c.262 0 .52-.011.776-.028a5.91 5.91 0 01-.193-1.479c0-3.644 3.374-6.6 7.536-6.6.262 0 .52.011.776.028-.628-3.513-4.27-6.472-8.885-6.472zM5.785 5.97a1.1 1.1 0 110 2.2 1.1 1.1 0 010-2.2zm5.813 0a1.1 1.1 0 110 2.2 1.1 1.1 0 010-2.2zm5.192 2.642c-3.703 0-6.71 2.567-6.71 5.73 0 3.163 3.007 5.73 6.71 5.73a7.9 7.9 0 002.126-.288.644.644 0 01.17-.022.69.69 0 01.329.085l1.672.97a.262.262 0 00.147.046c.128 0 .23-.104.23-.233a.403.403 0 00-.038-.168l-.309-1.17a.468.468 0 01.168-.527c1.449-1.065 2.374-2.643 2.374-4.423 0-3.163-3.007-5.73-6.71-5.73h-.159zm-2.434 3.34a.88.88 0 110 1.76.88.88 0 010-1.76zm4.868 0a.88.88 0 110 1.76.88.88 0 010-1.76z"/>
                </svg>
              </div>
              <div>
                <h3 class="text-lg font-semibold text-gray-900 dark:text-white">联系客服</h3>
                <p class="text-sm text-gray-500 dark:text-dark-400">扫码添加好友</p>
              </div>
            </div>

            <!-- 二维码卡片 -->
            <div class="mb-4 overflow-hidden rounded-xl border border-primary-100 bg-gradient-to-br from-primary-50 to-white p-3 dark:border-primary-800/30 dark:from-primary-900/10 dark:to-dark-800">
              <img
                src="/wechat-qr.jpg"
                alt="微信二维码"
                class="w-full rounded-lg"
              />
            </div>

            <!-- 提示文字 -->
            <div class="text-center">
              <p class="mb-2 text-sm font-medium text-primary-600 dark:text-primary-400">
                微信扫码添加客服
              </p>
              <p class="flex items-center justify-center gap-1 text-xs text-gray-500 dark:text-dark-400">
                <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                </svg>
                工作时间：周一至周五 9:00-18:00
              </p>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup lang="ts">
import { ref } from 'vue'

const showModal = ref(false)
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}

.scale-enter-active,
.scale-leave-active {
  transition: all 0.2s ease;
}

.scale-enter-from,
.scale-leave-to {
  opacity: 0;
  transform: scale(0.95);
}
</style>
