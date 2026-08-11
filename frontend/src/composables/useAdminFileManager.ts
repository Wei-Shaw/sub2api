/**
 * useAdminFileManager：管理员「文件管理」入口的可见性。
 *
 * 「文件管理」只在图片转存（COS / S3 兼容）启用后才有意义 —— 没配置对象存储时
 * 桶都不存在，菜单进去只能看到一个引导页。因此侧边栏需要知道它是否启用。
 *
 * 为什么不用 utils/featureFlags 那套：那套读的是 **public settings**（下发给所有
 * 已登录用户的公开配置）。COS 配置属于管理员的敏感设置，把"是否配置了对象存储"
 * 塞进公开设置既不必要也不合适（按最小暴露原则）。而且那套注册流程要动后端
 * 7 个文件，语义上也不是"公开功能开关"。
 *
 * 实现选择：模块级单例缓存 + 懒加载。
 *   - 状态放模块作用域，多个组件（侧边栏、页面）共享同一份，不会重复请求；
 *   - 只有管理员会调用 ensure()，普通用户永远不会打到 admin 接口；
 *   - 首帧 enabled 为 undefined，菜单按"隐藏"处理，请求回来后再显示。
 *     这是 opt-in 语义：宁可晚一点出现，也不要闪一下又消失。
 */
import { readonly, ref } from 'vue'
import adminFilesAPI from '@/api/admin/files'

/** enabled：undefined 表示尚未探测。 */
const enabled = ref<boolean | undefined>(undefined)
const bucket = ref('')
/** 进行中的请求，用于并发去重（侧边栏与页面可能同时触发）。 */
let inflight: Promise<void> | null = null

/**
 * ensureAdminFileManagerStatus：确保状态已加载。已加载或正在加载时直接复用。
 * 探测失败按"未启用"处理 —— 拿不到状态时不该露出一个必然报错的入口。
 */
export function ensureAdminFileManagerStatus(): Promise<void> {
  if (enabled.value !== undefined) return Promise.resolve()
  if (inflight) return inflight
  inflight = adminFilesAPI
    .getStatus()
    .then((s) => {
      enabled.value = s.enabled
      bucket.value = s.bucket
    })
    .catch(() => {
      enabled.value = false
      bucket.value = ''
    })
    .finally(() => {
      inflight = null
    })
  return inflight
}

/**
 * refreshAdminFileManagerStatus：强制重新探测。
 * 管理员在系统设置里刚开启/关闭图片转存后调用，菜单即时生效，无需刷新页面。
 */
export function refreshAdminFileManagerStatus(): Promise<void> {
  enabled.value = undefined
  inflight = null
  return ensureAdminFileManagerStatus()
}

export function useAdminFileManager() {
  return {
    enabled: readonly(enabled),
    bucket: readonly(bucket),
    ensure: ensureAdminFileManagerStatus,
    refresh: refreshAdminFileManagerStatus,
  }
}

/** makeFileManagerSidebarFlag：适配 AppSidebar 的 NavItem.featureFlag 契约（false 隐藏）。 */
export function makeFileManagerSidebarFlag(): () => boolean {
  return () => enabled.value === true
}
