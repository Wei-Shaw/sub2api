/**
 * Chinese i18n — `nav` + `common` namespaces.
 *
 * T19 Fix B — split out from monolithic `zh.ts`.
 */
export default {
  nav: {
    channels: '渠道管理',
  },
  common: {
    refresh: '刷新',
    loading: '加载中...',
    // Worker B' — 模板 picker / 管理器 UX key (host common 里没有).
    selectAll: '全选',
    submitting: '提交中...',
    autoRefresh: {
      title: '自动刷新',
      enable: '启用自动刷新',
      countdown: '自动刷新: {seconds}s',
      seconds: '{n} 秒',
    },
  },
}
