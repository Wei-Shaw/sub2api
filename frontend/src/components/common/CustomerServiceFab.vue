<template>
  <Teleport to="body">
    <div class="cs-fab-root" :class="{ 'cs-fab-open': isOpen }">
      <!-- Expanded panel -->
      <Transition name="cs-panel">
        <div v-if="isOpen" class="cs-panel">
          <div class="cs-panel-header">
            <span class="cs-panel-title">用户交流群</span>
            <button class="cs-panel-close" @click="close">
              <svg width="16" height="16" viewBox="0 0 16 16" fill="none"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.5" stroke-linecap="round"/></svg>
            </button>
          </div>
          <div class="cs-panel-body">
            <div class="cs-qr-wrap">
              <img src="/aftersales-qq-group.jpg" alt="扫码加入交流群" class="cs-qr-img" />
            </div>
            <p class="cs-panel-hint">QQ 扫码加入「卡卡AI交流群」</p>
            <div class="cs-panel-divider"></div>
            <div class="cs-panel-info">
              <div class="cs-info-row">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor"><path d="M12.003 2c-2.265 0-6.29 1.364-6.29 7.325v1.195S3.55 14.96 3.55 17.474c0 .665.17 1.025.396 1.025.19 0 .46-.18.758-.625.775-1.15 1.525-2.76 1.997-3.775.14.02.282.03.425.03 1.47 0 2.97-.765 3.852-2.115.88 1.35 2.382 2.115 3.852 2.115.143 0 .284-.01.425-.03.472 1.015 1.222 2.625 1.997 3.775.298.445.568.625.758.625.226 0 .396-.36.396-1.025 0-2.514-2.163-6.954-2.163-6.954v-1.195C18.293 3.364 14.268 2 12.003 2z"/></svg>
                <span>群号：774692252</span>
              </div>
              <div class="cs-info-row">
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none"><path d="M6 3h9l3 3v15H6V3Z" stroke="currentColor" stroke-width="1.6"/><path d="M14 3v4h4M8.5 11h7M8.5 15h7" stroke="currentColor" stroke-width="1.6" stroke-linecap="round"/></svg>
                <span>商务合作 / 采购 / 开票：QQ 591719412（银河）</span>
              </div>
            </div>
            <div class="cs-panel-tags">
              <span class="cs-tag">活动福利</span>
              <span class="cs-tag">使用答疑</span>
              <span class="cs-tag">商务采购</span>
            </div>
          </div>
        </div>
      </Transition>

      <!-- FAB button -->
      <button class="cs-fab-btn" :class="{ 'cs-fab-btn-pulse': !isOpen && !dismissed }" :title="isOpen ? '关闭' : '联系客服'" @click="toggle">
        <Transition name="cs-icon" mode="out-in">
          <svg v-if="isOpen" key="close" width="22" height="22" viewBox="0 0 16 16" fill="none"><path d="M4 4l8 8M12 4l-8 8" stroke="currentColor" stroke-width="1.8" stroke-linecap="round"/></svg>
          <svg v-else key="qq" width="24" height="24" viewBox="0 0 24 24" fill="currentColor"><path d="M12.003 2c-2.265 0-6.29 1.364-6.29 7.325v1.195S3.55 14.96 3.55 17.474c0 .665.17 1.025.396 1.025.19 0 .46-.18.758-.625.775-1.15 1.525-2.76 1.997-3.775.14.02.282.03.425.03 1.47 0 2.97-.765 3.852-2.115.88 1.35 2.382 2.115 3.852 2.115.143 0 .284-.01.425-.03.472 1.015 1.222 2.625 1.997 3.775.298.445.568.625.758.625.226 0 .396-.36.396-1.025 0-2.514-2.163-6.954-2.163-6.954v-1.195C18.293 3.364 14.268 2 12.003 2z"/></svg>
        </Transition>
      </button>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, watch } from 'vue'
import { useCustomerService } from '@/composables/useCustomerService'

const { isOpen, close, toggle } = useCustomerService()
const dismissed = ref(false)

function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && isOpen.value) close()
}

function onClickOutside(e: MouseEvent) {
  if (!isOpen.value) return
  const root = document.querySelector('.cs-fab-root')
  const headerBtn = document.querySelector('.cs-header-btn')
  if (root && !root.contains(e.target as Node) && !headerBtn?.contains(e.target as Node)) close()
}

// Mark dismissed when opened via any trigger
watch(isOpen, (val) => {
  if (val) dismissed.value = true
})

onMounted(() => {
  document.addEventListener('keydown', onKeydown)
  document.addEventListener('click', onClickOutside, true)
  setTimeout(() => { dismissed.value = true }, 8000)
})

onUnmounted(() => {
  document.removeEventListener('keydown', onKeydown)
  document.removeEventListener('click', onClickOutside, true)
})
</script>

<style scoped>
.cs-fab-root {
  position: fixed;
  bottom: 24px;
  right: 24px;
  z-index: 9999;
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
}

/* FAB button */
.cs-fab-btn {
  width: 52px;
  height: 52px;
  border-radius: 50%;
  background: #111;
  color: #fff;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 16px rgba(0,0,0,0.2);
  transition: transform 0.2s, box-shadow 0.2s, background 0.2s;
  position: relative;
}
.cs-fab-btn:hover {
  transform: scale(1.08);
  box-shadow: 0 6px 24px rgba(0,0,0,0.28);
}
.cs-fab-btn:active {
  transform: scale(0.95);
}
.cs-fab-open .cs-fab-btn {
  background: #444;
}
.dark .cs-fab-btn {
  background: #fff;
  color: #111;
}
.dark .cs-fab-open .cs-fab-btn {
  background: #666;
  color: #fff;
}

/* Pulse ring */
.cs-fab-btn-pulse::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  border: 2px solid #111;
  opacity: 0;
  animation: cs-pulse 2s ease-out 1s 3;
}
@keyframes cs-pulse {
  0% { transform: scale(1); opacity: 0.5; }
  100% { transform: scale(1.5); opacity: 0; }
}

/* Panel */
.cs-panel {
  position: absolute;
  bottom: 64px;
  right: 0;
  width: 280px;
  background: #fff;
  border: 1px solid #e5e5e5;
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 8px 32px rgba(0,0,0,0.12);
}
.dark .cs-panel {
  background: #1a1a1a;
  border-color: #333;
}
.cs-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 16px 10px;
}
.cs-panel-title {
  font-size: 15px;
  font-weight: 600;
  color: #111;
}
.dark .cs-panel-title {
  color: #fff;
}
.cs-panel-close {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: #999;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.15s, color 0.15s;
}
.cs-panel-close:hover {
  background: #f0f0f0;
  color: #333;
}
.dark .cs-panel-close:hover {
  background: #333;
  color: #fff;
}
.cs-panel-body {
  padding: 0 16px 16px;
}
.cs-qr-wrap {
  width: 100%;
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid #eee;
  background: #fafafa;
}
.dark .cs-qr-wrap {
  border-color: #333;
  background: #222;
}
.cs-qr-img {
  width: 100%;
  display: block;
  object-fit: contain;
}
.cs-panel-hint {
  text-align: center;
  font-size: 13px;
  color: #666;
  margin-top: 10px;
}
.dark .cs-panel-hint {
  color: #999;
}
.cs-panel-divider {
  height: 1px;
  background: #eee;
  margin: 12px 0;
}
.dark .cs-panel-divider {
  background: #333;
}
.cs-panel-info {
  margin-bottom: 10px;
}
.cs-info-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: #555;
}
.dark .cs-info-row {
  color: #aaa;
}
.cs-panel-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.cs-tag {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 4px;
  background: #f5f5f5;
  color: #666;
  font-weight: 500;
}
.dark .cs-tag {
  background: #2a2a2a;
  color: #aaa;
}

/* Transitions */
.cs-panel-enter-active,
.cs-panel-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}
.cs-panel-enter-from,
.cs-panel-leave-to {
  opacity: 0;
  transform: translateY(8px) scale(0.96);
}
.cs-icon-enter-active,
.cs-icon-leave-active {
  transition: opacity 0.15s ease;
}
.cs-icon-enter-from,
.cs-icon-leave-to {
  opacity: 0;
}

/* Mobile */
@media (max-width: 640px) {
  .cs-fab-root {
    bottom: 16px;
    right: 16px;
  }
  .cs-fab-btn {
    width: 46px;
    height: 46px;
  }
  .cs-panel {
    width: 260px;
    right: -8px;
  }
}
</style>
