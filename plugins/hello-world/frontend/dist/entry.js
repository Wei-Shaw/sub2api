// hello-world plugin frontend entry (V2 — Shadow DOM 协议).
// hand-written ESM (no bundler).
//
// 这个文件是插件 frontend bundle 的契约样板:
//   - default export 必须是 { install(sdk) }
//   - install 返回 { mount(shadowRoot, ctx) -> PluginInstance }
//   - mount 在 host 给的 ShadowRoot 内通过 sdk.runtime.createApp 起独立 Vue app
//   - PluginInstance.unmount 由 host 在路由切换 / 卸载时调用
//
// 共享上下文 (importmap):
//   - vue / pinia / vue-router / vue-i18n / axios — 与 host 同一份 singleton
//   - @sub2api/plugin-sdk — host 编译的 SDK bundle

const HELLO_NAMESPACE = 'helloWorldPlugin'

const messages = {
  en: {
    welcome: 'Hello from the hello-world plugin!',
    greeting: 'Hi {name}, you are looking great today.',
    notifyMe: 'Notify success',
    toggleTheme: 'Toggle theme',
    currentTheme: 'Current theme: {mode}',
    currentUser: 'Current user: {name}',
    notLoggedIn: '(not signed in)',
    locale: 'Locale: {code}',
  },
  zh: {
    welcome: 'Hello-world 插件渲染成功!',
    greeting: '你好 {name}, 今天看起来很棒.',
    notifyMe: '触发成功通知',
    toggleTheme: '切换主题',
    currentTheme: '当前主题: {mode}',
    currentUser: '当前用户: {name}',
    notLoggedIn: '(未登录)',
    locale: '语言: {code}',
  },
}

function install(sdk) {
  // 1. 注册插件自己的 i18n namespace, 共享 host 的 vue-i18n 实例.
  sdk.i18n.registerNamespace(HELLO_NAMESPACE, messages)

  const { defineComponent, h, computed } = sdk.vue
  const t = (key, params) => sdk.i18n.t(`${HELLO_NAMESPACE}.${key}`, params)

  // 2. 定义 plugin root component (用 h() 渲染, 不依赖 SFC).
  const HelloWorldView = defineComponent({
    name: 'HelloWorldPluginView',
    setup() {
      const themeMode = sdk.theme.mode
      const locale = sdk.i18n.currentLocale
      const user = sdk.auth.user
      const userName = computed(() => (user.value && user.value.username) || '')
      const greetingName = computed(() => userName.value || 'friend')

      function notifyMe() {
        sdk.notify.success(t('welcome'))
      }
      function toggleTheme() {
        sdk.theme.toggle()
      }

      return () =>
        h(
          'div',
          {
            class: 'hello-world-plugin',
            style: {
              padding: '1.5rem',
              maxWidth: '36rem',
              margin: '1rem auto',
              border: '1px solid rgba(0,0,0,0.08)',
              borderRadius: '0.75rem',
              background: themeMode.value === 'dark' ? 'rgba(31,41,55,0.4)' : 'rgba(249,250,251,0.6)',
              color: themeMode.value === 'dark' ? '#e5e7eb' : '#111827',
              display: 'flex',
              flexDirection: 'column',
              gap: '0.75rem',
            },
          },
          [
            h('h2', { style: { margin: 0, fontSize: '1.25rem', fontWeight: 600 } }, t('welcome')),
            h('p', { style: { margin: 0 } }, t('greeting', { name: greetingName.value })),
            h(
              'p',
              { style: { margin: 0, fontSize: '0.85rem', opacity: 0.85 } },
              t('currentTheme', { mode: themeMode.value }),
            ),
            h(
              'p',
              { style: { margin: 0, fontSize: '0.85rem', opacity: 0.85 } },
              t('locale', { code: locale.value }),
            ),
            h(
              'p',
              { style: { margin: 0, fontSize: '0.85rem', opacity: 0.85 } },
              userName.value
                ? t('currentUser', { name: userName.value })
                : t('notLoggedIn'),
            ),
            h(
              'div',
              { style: { display: 'flex', gap: '0.5rem', marginTop: '0.5rem' } },
              [
                h(
                  'button',
                  {
                    type: 'button',
                    onClick: notifyMe,
                    style: btnStyle(themeMode.value),
                  },
                  t('notifyMe'),
                ),
                h(
                  'button',
                  {
                    type: 'button',
                    onClick: toggleTheme,
                    style: btnStyle(themeMode.value),
                  },
                  t('toggleTheme'),
                ),
              ],
            ),
          ],
        )
    },
  })

  // 3. V2 协议: 返回 mount 函数, 由 host 在 ShadowRoot 内调用.
  return {
    mount(shadowRoot, _ctx) {
      // mount-plugin 在 shadow root 里已经创建了 .plugin-shadow-root 容器.
      const target = shadowRoot.querySelector('.plugin-shadow-root')
      if (!target) {
        throw new Error('[hello-world] plugin-shadow-root container not found')
      }
      const instance = sdk.runtime.createApp(HelloWorldView, target)
      return {
        unmount() {
          instance.unmount()
        },
      }
    },
  }
}

function btnStyle(mode) {
  const dark = mode === 'dark'
  return {
    padding: '0.4rem 0.9rem',
    borderRadius: '0.5rem',
    border: dark ? '1px solid rgba(96,165,250,0.5)' : '1px solid rgba(59,130,246,0.4)',
    background: dark ? 'rgba(59,130,246,0.18)' : 'rgba(59,130,246,0.08)',
    color: dark ? '#bfdbfe' : '#2563eb',
    fontSize: '0.875rem',
    cursor: 'pointer',
  }
}

export default { install }
export { install }
