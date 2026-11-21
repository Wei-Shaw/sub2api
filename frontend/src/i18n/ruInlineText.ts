/**
 * Русские переводы для строк, которые зашиты в компоненты парами zh/en
 * (хелперы localText / localizeWeChatHint и мета событий в EmailTemplateEditor)
 * и идут мимо сообщений vue-i18n.
 *
 * Ключ — английская строка ровно как в коде, значение — русский перевод.
 * Если ключа нет, ruInline возвращает английский оригинал.
 */
export const ruInlineText: Record<string, string> = {
  // SettingsView: вход через email OAuth (GitHub / Google)
  'Email OAuth Sign-in': 'Вход через email OAuth',
  'After GitHub or Google email OAuth is enabled, the system reads a verified email, signs in matching users, and auto-registers missing users.':
    'Когда включён вход через email OAuth (GitHub или Google), система получает подтверждённый email: если пользователь с таким адресом есть, он входит, если нет — регистрируется автоматически.',
  'GitHub OAuth App needs read:user user:email scopes. Use the backend callback URL below.':
    'GitHub OAuth App нужны области read:user user:email. В качестве адреса обратного вызова укажите адрес бэкенда ниже.',
  'Secret configured. Leave empty to keep the current value.':
    'Секрет задан. Оставьте поле пустым, чтобы сохранить текущее значение.',
  'Backend Callback URL': 'Адрес обратного вызова (бэкенд)',
  'Generate and copy': 'Сгенерировать и скопировать',
  'Frontend Callback URL': 'Адрес возврата (фронтенд)',
  'Google OAuth client needs openid email profile scopes and the backend callback URL registered in credentials.':
    'OAuth-клиенту Google нужны области openid email profile, а адрес обратного вызова бэкенда должен быть зарегистрирован в учётных данных.',
  'Setup guide: Google Cloud Console → APIs & Services → OAuth consent screen, then Credentials → Create Credentials → OAuth client ID, choose Web application, and add the URL below to Authorized redirect URIs.':
    'Как настроить: в Google Cloud Console откройте APIs & Services → OAuth consent screen и заполните экран согласия, затем Credentials → Create Credentials → OAuth client ID, выберите тип Web application и добавьте адрес ниже в Authorized redirect URIs.',
  'Callback URL set and copied.': 'Адрес обратного вызова записан и скопирован.',

  // SettingsView: WeChat
  'PC App': 'Приложение для ПК',
  'Desktop browsers sign in through WeChat Open Platform QR login. This can coexist with Official Account or Mobile App.':
    'В настольных браузерах вход выполняется по QR-коду через WeChat Open Platform. Можно использовать одновременно с официальным аккаунтом или мобильным приложением.',
  'PC App ID': 'App ID приложения для ПК',
  'WeChat Open Platform PC App ID': 'App ID приложения для ПК в WeChat Open Platform',
  'PC App Secret': 'App Secret приложения для ПК',
  'WeChat Open Platform PC App Secret': 'App Secret приложения для ПК в WeChat Open Platform',
  'Official Account': 'Официальный аккаунт',
  'Only available inside the WeChat browser. It is shown as unavailable outside WeChat.':
    'Работает только во встроенном браузере WeChat. Вне WeChat этот способ отображается как недоступный.',
  'Official Account App ID': 'App ID официального аккаунта',
  'Official Account App Secret': 'App Secret официального аккаунта',
  'Mobile App': 'Мобильное приложение',
  'Native mobile clients start authorization through the WeChat SDK. The web UI does not launch this flow directly.':
    'Нативные мобильные клиенты запускают авторизацию через WeChat SDK. Веб-интерфейс этот процесс напрямую не запускает.',
  'Mobile App ID': 'App ID мобильного приложения',
  'Mobile App Secret': 'App Secret мобильного приложения',
  'When PC App is enabled together with Official Account or Mobile App, they should belong to the same WeChat Open Platform account so UnionID can merge identities reliably.':
    'Если приложение для ПК включено вместе с официальным аккаунтом или мобильным приложением, все они должны принадлежать одному аккаунту WeChat Open Platform — иначе UnionID не сможет надёжно объединять учётные записи.',
  'Browser Redirect URL': 'Адрес возврата для браузера',
  'Used by PC App and Official Account browser callbacks. Native mobile SDK flows do not start from this browser callback directly.':
    'Используется для браузерных обратных вызовов приложения для ПК и официального аккаунта. Вход через нативный мобильный SDK этот адрес напрямую не использует.',
  'Official Account and Mobile App cannot be enabled at the same time.':
    'Официальный аккаунт и мобильное приложение нельзя включить одновременно.',

  // SettingsView: DingTalk
  'DingTalk Name': 'Имя в DingTalk',
  'DingTalk Corporate Email': 'Корпоративный email в DingTalk',
  'DingTalk Department': 'Отдел в DingTalk',

  // SettingsView: настройки по умолчанию для новых пользователей OAuth
  'Applied on first signup or first bind through a verified GitHub email.':
    'Применяется при первой регистрации или первой привязке через подтверждённый email GitHub.',
  'Applied on first signup or first bind through a verified Google email.':
    'Применяется при первой регистрации или первой привязке через подтверждённый email Google.',
  'Applied on first signup or first bind through DingTalk.':
    'Применяется при первой регистрации или первой привязке через DingTalk.',

  // SettingsView: согласие с условиями при входе
  'Login agreement': 'Согласие с условиями при входе',
  'Control whether the login page requires users to accept Markdown policy documents first.':
    'Требовать ли на странице входа, чтобы пользователь сначала принял документы с условиями (Markdown).',
  Enabled: 'Включено',
  Disabled: 'Выключено',
  'Display mode': 'Способ показа',
  Modal: 'Всплывающее окно',
  Checkbox: 'Флажок',
  'The checkbox appears below the login button and gates all login actions.':
    'Флажок появляется под кнопкой входа; пока он не отмечен, все способы входа недоступны.',
  'The modal opens on the login page and gates all login actions until accepted.':
    'Окно открывается на странице входа; пока пользователь не примет условия, все способы входа недоступны.',
  'Updated date': 'Дата обновления',
  'Changing the date or content requires fresh consent.':
    'После изменения даты или текста документов пользователям нужно снова дать согласие.',
  'Agreement documents': 'Документы с условиями',
  'Document titles are customizable and content is saved as Markdown.':
    'Названия документов можно менять, текст сохраняется в формате Markdown.',
  'Add document': 'Добавить документ',
  'Untitled document': 'Документ без названия',
  'Document title': 'Название документа',
  'Example: Terms of Service': 'Например: Условия использования',
  'Route slug': 'Адрес страницы',
  'Markdown content': 'Текст в Markdown',
  'Write the final Markdown content here.': 'Введите здесь окончательный текст в формате Markdown.',
  'Terms of Service': 'Условия использования',
  'Usage Policy': 'Правила использования',
  'Supported Countries and Regions': 'Поддерживаемые страны и регионы',
  'Service-Specific Terms': 'Особые условия сервиса',
  'At least one document is required when login agreement is enabled.':
    'Когда включено согласие с условиями при входе, должен остаться хотя бы один документ.',
  'Login agreement document title cannot be empty.':
    'Название документа с условиями не может быть пустым.',

  // EmailTemplateEditor: признаки и категории
  Optional: 'Можно отписаться',
  Transactional: 'Транзакционное письмо',
  Notification: 'Уведомление',
  Auth: 'Аутентификация',
  Subscription: 'Подписка',
  Billing: 'Оплата',
  Admin: 'Администрирование',
  'Risk Control': 'Контроль рисков',
  Ops: 'Мониторинг',

  // EmailTemplateEditor: события писем
  'Email Verification Code': 'Код подтверждения email',
  'Sent for registration, email binding, OAuth pending email completion, or TOTP email verification.':
    'Отправляется при регистрации, привязке email, указании email после входа через OAuth или подтверждении TOTP по email.',
  'Password Reset': 'Сброс пароля',
  'Sent when a user requests a password reset link.':
    'Отправляется, когда пользователь запрашивает ссылку для сброса пароля.',
  'Notification Email Verification': 'Подтверждение email для уведомлений',
  'Sent when a user adds and verifies an extra notification email address.':
    'Отправляется, когда пользователь добавляет и подтверждает дополнительный email для уведомлений.',
  'Subscription Activated': 'Подписка активирована',
  'Sent after a subscription order is paid and the subscription is activated or extended.':
    'Отправляется после оплаты заказа подписки, когда подписка активирована или продлена.',
  'Subscription Expiry Reminder': 'Напоминание об окончании подписки',
  'Sent by the background job when an active subscription has 7, 3, or 1 day remaining. It can be disabled in Email settings.':
    'Отправляется фоновой задачей, когда до окончания активной подписки остаётся 7, 3 или 1 день. Можно отключить в настройках email.',
  'Low Balance Alert': 'Низкий баланс',
  "Sent when a user's balance drops below the global or personal reminder threshold.":
    'Отправляется, когда баланс пользователя опускается ниже общего или личного порога напоминания.',
  'Balance Recharge Success': 'Баланс пополнен',
  'Sent after a balance recharge order is paid and credited.':
    'Отправляется после оплаты и зачисления пополнения баланса.',
  'Account Quota Alert': 'Квота аккаунта на исходе',
  'Sent to admin notification emails when an upstream account reaches the configured quota alert threshold.':
    'Отправляется на email администраторов для уведомлений, когда аккаунт достигает заданного порога предупреждения по квоте.',
  'Risk Control Violation Notice': 'Уведомление о нарушении правил',
  'Sent when a user request triggers content moderation or risk-control rules but the account is not disabled yet.':
    'Отправляется, когда запрос пользователя срабатывает на модерацию контента или правила контроля рисков, но аккаунт ещё не отключён.',
  'Risk Control Account Disabled': 'Аккаунт отключён контролем рисков',
  'Sent when content moderation reaches the ban threshold and automatically disables the user account.':
    'Отправляется, когда модерация контента достигает порога блокировки и аккаунт пользователя отключается автоматически.',
  'Ops Alert': 'Оповещение мониторинга',
  'Sent to ops recipients when an ops monitoring rule fires and email notification settings allow it.':
    'Отправляется получателям оповещений мониторинга, когда срабатывает правило мониторинга и это разрешено настройками email-уведомлений.',
  'Ops Scheduled Report': 'Плановый отчёт мониторинга',
  'Sent when a configured daily, weekly, error digest, or account health report reaches its scheduled send time. Every daily and weekly summary metric is editable in this template.':
    'Отправляется, когда наступает время отправки настроенного ежедневного или еженедельного отчёта, сводки ошибок или отчёта о состоянии аккаунтов. Все показатели ежедневной и еженедельной сводки можно редактировать в этом шаблоне.',

  // WechatOAuthSection
  'This site only has WeChat mobile app login configured. Continue from the native app through the WeChat SDK.':
    'На этом сайте настроен только вход через мобильное приложение WeChat. Продолжите в нативном приложении через WeChat SDK.',
}

export function isRuLocale(locale: string): boolean {
  return locale.toLowerCase().startsWith('ru')
}

export function ruInline(en: string): string {
  return ruInlineText[en] ?? en
}
