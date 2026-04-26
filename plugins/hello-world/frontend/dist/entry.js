// hello-world plugin frontend entry — hand-written ESM (no bundler).
//
// 这个文件是插件 frontend bundle 的契约样板:
//   - default export 必须是 { install(sdk) }
//   - install 返回 { components: { [componentPath]: Component } }
//   - 渲染时使用 sdk.vue.{ defineComponent, h, ref, computed, watch }, 避免重复打包 Vue
//   - 使用 sdk.i18n / sdk.notify / sdk.theme / sdk.auth 验证 host 能力都能跑通

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

  // 2. 定义一个最小化的演示组件, 仅用 h() 渲染, 不依赖 SFC.
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

  return {
    components: {
      // key 与 manifest.routes[].component_path 对齐.
      'HelloWorldView.vue': HelloWorldView,
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
