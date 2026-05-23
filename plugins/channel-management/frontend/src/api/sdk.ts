/**
 * Plugin host SDK accessor.
 *
 * install() 把 host 注入的 SDK 保存到模块单例, 视图文件通过 getSdk() 拿到
 * notify / http / auth 等能力. 与 client.ts 同型, 区分不同关注点 (http vs sdk).
 *
 * 用 module-scope 状态而不是 vue provide/inject 是因为:
 *   - plugin 入口 install(sdk) 在 vue app 之外被调用 (host runtime)
 *   - 视图文件可能在 install 之后立即被构造 (PluginView mount)
 *   - install 同步 setSdk + 视图 getSdk 比 provide 更直接, 不依赖 vue 上下文
 */
import { createSdkAccessor } from '@sub2api/plugin-sdk'

export const { setSdk, getSdk } = createSdkAccessor('plugin-channel-management')
