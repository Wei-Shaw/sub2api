<script setup lang="ts">
import { RouterView, useRouter, useRoute } from 'vue-router'
import { onMounted, onBeforeUnmount, watch } from 'vue'
import Toast from '@/components/common/Toast.vue'
import NavigationProgress from '@/components/common/NavigationProgress.vue'
import AdminComplianceDialog from '@/components/admin/AdminComplianceDialog.vue'
import { resolveRouteDocumentTitle } from '@/router/title'
import AnnouncementPopup from '@/components/common/AnnouncementPopup.vue'
import SupportChatWidget from '@/components/support/SupportChatWidget.vue'
import InboxKickedOverlay from '@/components/common/InboxKickedOverlay.vue'
import { useAppStore, useAuthStore, useSubscriptionStore, useAnnouncementStore, useAdminComplianceStore, useAdminSettingsStore, useTicketUnreadStore, useInboxStore } from '@/stores'
import { getSetupStatus } from '@/api/setup'
import { updateFavicon } from '@/utils/branding'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const authStore = useAuthStore()
const subscriptionStore = useSubscriptionStore()
const announcementStore = useAnnouncementStore()
const adminComplianceStore = useAdminComplianceStore()
const adminSettingsStore = useAdminSettingsStore()
// 工单未读 store：负责红点/铃铛工单 tab 的数据源。lifecycle 挂在 auth 变化里，
// logout 时 reset() 会顺带停止 60s 轮询避免请求泄漏。
const ticketUnreadStore = useTicketUnreadStore()
// 通用信箱（general-inbox）store：受公共设置 inbox_v1_enabled 灰度开关控制。
// 开启后 bootstrap() 会建立 WebSocket + catchup 补齐；logout 时 reset() 断连清状态。
const inboxStore = useInboxStore()

/** 通用信箱灰度是否开启（后端 config.Inbox.V1Enabled 经公共设置下发）。 */
function inboxV1Enabled(): boolean {
  return appStore.cachedPublicSettings?.inbox_v1_enabled === true
}

function updateDocumentTitle() {
  const customMenuItems = [
    ...(appStore.cachedPublicSettings?.custom_menu_items ?? []),
    ...(authStore.isAdmin ? adminSettingsStore.customMenuItems : []),
  ]
  document.title = resolveRouteDocumentTitle(route, appStore.siteName, customMenuItems)
}

// Watch for site settings changes and update favicon/title
watch(
  () => appStore.siteLogo,
  (newLogo) => {
    if (newLogo) {
      updateFavicon(newLogo)
    }
  },
  { immediate: true }
)

watch(
  [
    () => route.fullPath,
    () => route.meta.title,
    () => route.meta.titleKey,
    () => appStore.siteName,
    () => appStore.cachedPublicSettings?.custom_menu_items,
    () => authStore.isAdmin,
    () => adminSettingsStore.customMenuItems,
  ],
  updateDocumentTitle,
  { deep: true }
)

// Watch for authentication state and manage subscription data + announcements
function onVisibilityChange() {
  if (document.visibilityState === 'visible' && authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
  }
}

function onAdminComplianceRequired(event: Event) {
  const detail = (event as CustomEvent<Record<string, string>>).detail || {}
  adminComplianceStore.requireAcknowledgement(detail)
}

watch(
  () => authStore.isAuthenticated,
  (isAuthenticated, oldValue) => {
    if (isAuthenticated) {
      if (authStore.isAdmin) {
        adminComplianceStore.fetchStatus().catch((error) => {
          console.error('Failed to fetch admin compliance status:', error)
        })
      }

      // User logged in: preload subscriptions and start polling
      subscriptionStore.fetchActiveSubscriptions().catch((error) => {
        console.error('Failed to preload subscriptions:', error)
      })
      subscriptionStore.startPolling()

      // Announcements: new login vs page refresh restore
      if (oldValue === false) {
        // New login: delay 3s then force fetch
        setTimeout(() => announcementStore.fetchAnnouncements(true), 3000)
      } else {
        // Page refresh restore (oldValue was undefined)
        announcementStore.fetchAnnouncements()
      }

      // 工单未读：60s 轮询 + visibilitychange 立即刷新。
      // startPolling 内部会检查 support_ticket_enabled，关闭时不发请求；
      // 首次挂载会 force 拉一次 unread-count，让 sidebar/铃铛红点尽快显示。
      ticketUnreadStore.startPolling()

      // 通用信箱：灰度开启时冷启动（建 WS + catchup）。bootstrap 幂等，
      // 页面刷新恢复（oldValue undefined）与新登录都安全。
      if (inboxV1Enabled()) {
        void inboxStore.bootstrap()
      }

      // Register visibility change listener
      document.addEventListener('visibilitychange', onVisibilityChange)
    } else {
      // User logged out: clear data and stop polling
      subscriptionStore.clear()
      announcementStore.reset()
      adminComplianceStore.reset()
      ticketUnreadStore.reset()
      inboxStore.reset()
      document.removeEventListener('visibilitychange', onVisibilityChange)
    }
  },
  { immediate: true }
)

// Route change trigger (throttled by store)
router.afterEach(() => {
  if (authStore.isAuthenticated) {
    announcementStore.fetchAnnouncements()
    // 工单未读数：路由切换后节流拉一次（store 内部 UNREAD_COUNT_FETCH_MIN_INTERVAL_MS 兜底），
    // 让"进入工单详情 → 返回列表页"时红点能立刻更新（不用等 60s tick）。
    void ticketUnreadStore.fetchUnreadCount()
  }
})

onBeforeUnmount(() => {
  document.removeEventListener('visibilitychange', onVisibilityChange)
  window.removeEventListener('admin-compliance-required', onAdminComplianceRequired)
})

onMounted(async () => {
  window.addEventListener('admin-compliance-required', onAdminComplianceRequired)

  // Check if setup is needed
  try {
    const status = await getSetupStatus()
    if (status.needs_setup && route.path !== '/setup') {
      router.replace('/setup')
      return
    }
  } catch {
    // If setup endpoint fails, assume normal mode and continue
  }

  // Load public settings into appStore (will be cached for other components)
  await appStore.fetchPublicSettings()

  // Re-resolve document title now that site settings are available
  updateDocumentTitle()
})
</script>

<template>
  <NavigationProgress />
  <RouterView />
  <Toast />
  <AnnouncementPopup />
  <!-- 客服浮窗（add-support-chat-widget D2）：自带 shouldRender 判定，
       关闭 / 路由排除时不渲染任何节点。 -->
  <SupportChatWidget />
  <AdminComplianceDialog />
  <!-- 通用信箱单例连接遮罩：被其他端踢出时展示，点"在此继续"重连。 -->
  <InboxKickedOverlay />
</template>
