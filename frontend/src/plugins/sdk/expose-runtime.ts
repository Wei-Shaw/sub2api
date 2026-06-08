/**
 * 把 host 的 vue / vue-router / vue-i18n / pinia / axios 单例暴露到
 * window.__SUB2API_HOST_*__, 让浏览器侧通过 importmap (host 在 index.html
 * 注入) 把这些 bare specifier 映射到 /api/v1/plugin-assets/__shared__/<name>.js
 * 之后, 这些动态生成的 ESM proxy 文件能从 window 上读取到本物.
 *
 * 调用时机: main.ts 完成 createApp + use(pinia) + use(router) + use(i18n)
 * 之后, 在 attachHostSdkToWindow 之前 (或之后) 调一次. 必须早于任何 plugin
 * entry.js 的 dynamic import.
 *
 * 安全性:
 *   - 只暴露这五个明确的库, 不向 plugin 透露任意 host 内部模块;
 *   - 暴露的是模块对象本身, plugin 拿到后能获得同一份 reactive 实例,
 *     但无法直接读 host 的 store 状态 (store 仍需走 SDK).
 *   - window 暴露不是隔离边界, plugin 在浏览器内本来就能访问 window;
 *     这里只是让 ESM module specifier 能解析到同一份单例.
 */
import * as Vue from 'vue'
import * as VueRouter from 'vue-router'
import * as VueI18n from 'vue-i18n'
import * as Pinia from 'pinia'
import axios from 'axios'

interface PluginRuntimeWindow extends Window {
  __SUB2API_HOST_VUE__?: typeof Vue
  __SUB2API_HOST_VUE_ROUTER__?: typeof VueRouter
  __SUB2API_HOST_VUE_I18N__?: typeof VueI18n
  __SUB2API_HOST_PINIA__?: typeof Pinia
  __SUB2API_HOST_AXIOS__?: typeof axios
}

export function exposePluginRuntime(): void {
  if (typeof window === 'undefined') {
    return
  }
  const w = window as PluginRuntimeWindow
  // 只在第一次 attach 时挂; 重复调用是幂等的, 因为引用同一份模块对象.
  w.__SUB2API_HOST_VUE__ = Vue
  w.__SUB2API_HOST_VUE_ROUTER__ = VueRouter
  w.__SUB2API_HOST_VUE_I18N__ = VueI18n
  w.__SUB2API_HOST_PINIA__ = Pinia
  w.__SUB2API_HOST_AXIOS__ = axios
}
