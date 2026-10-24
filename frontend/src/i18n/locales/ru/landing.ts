export default {
  batchImageGuide: {
    title: 'Пакетная генерация изображений',
    description: 'Отправьте несколько промптов в одном задании и скачайте готовые изображения, когда оно завершится'
  },
  // Home Page
  home: {
    viewOnGithub: 'Открыть на GitHub',
    viewDocs: 'Документация',
    docs: 'Документация',
    switchToLight: 'Включить светлую тему',
    switchToDark: 'Включить тёмную тему',
    dashboard: 'Панель',
    login: 'Войти',
    getStarted: 'Начать',
    goToDashboard: 'Перейти в панель',
    // User-focused value proposition
    heroSubtitle: 'Один ключ — все AI-модели',
    heroDescription: 'Не нужно держать несколько подписок. Claude, GPT, Gemini и другие модели доступны по одному API-ключу',
    tags: {
      subscriptionToApi: 'Подписка → API',
      stickySession: 'Закреплённые сессии',
      realtimeBilling: 'Оплата по факту'
    },
    // Pain points section
    painPoints: {
      title: 'Знакомо?',
      items: {
        expensive: {
          title: 'Дорогие подписки',
          desc: 'Платите за несколько AI-подписок, и каждый месяц сумма растёт'
        },
        complex: {
          title: 'Хаос с аккаунтами',
          desc: 'Аккаунты и API-ключи разбросаны по разным платформам'
        },
        unstable: {
          title: 'Перебои в работе',
          desc: 'Один аккаунт упирается в лимит запросов, и работа встаёт'
        },
        noControl: {
          title: 'Нет контроля расходов',
          desc: 'Непонятно, куда уходят деньги, и нельзя ограничить использование для членов команды'
        }
      }
    },
    // Solutions section
    solutions: {
      title: 'Мы решаем эти проблемы',
      subtitle: 'Три простых шага к доступу к AI без забот'
    },
    features: {
      unifiedGateway: 'Доступ в один клик',
      unifiedGatewayDesc: 'Один API-ключ для всех подключённых AI-моделей. Отдельные заявки не нужны.',
      multiAccount: 'Всегда на связи',
      multiAccountDesc: 'Умная маршрутизация между несколькими upstream-аккаунтами с автоматическим переключением при сбоях. Забудьте об ошибках.',
      balanceQuota: 'Платите за то, что используете',
      balanceQuotaDesc: 'Тарификация по использованию с ограничением квот. Полная прозрачность расходов команды.'
    },
    // Comparison section
    comparison: {
      title: 'Почему мы?',
      headers: {
        feature: 'Сравнение',
        official: 'Официальные подписки',
        us: 'Наша платформа'
      },
      items: {
        pricing: {
          feature: 'Цены',
          official: 'Фиксированная плата каждый месяц, даже если не пользуетесь',
          us: 'Платите только за использование'
        },
        models: {
          feature: 'Выбор моделей',
          official: 'Только один провайдер',
          us: 'Свободно переключайтесь между моделями'
        },
        management: {
          feature: 'Управление аккаунтами',
          official: 'Каждый сервис настраивается отдельно',
          us: 'Единый ключ и одна панель'
        },
        stability: {
          feature: 'Стабильность',
          official: 'Лимиты запросов одного аккаунта',
          us: 'Пул аккаунтов с автоматическим переключением'
        },
        control: {
          feature: 'Контроль использования',
          official: 'Нет',
          us: 'Квоты и подробная аналитика'
        }
      }
    },
    providers: {
      title: 'Поддерживаемые AI-модели',
      description: 'Один API — много вариантов',
      supported: 'Поддерживается',
      soon: 'Скоро',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: 'Другие'
    },
    // CTA section
    cta: {
      title: 'Готовы начать?',
      description: 'Зарегистрируйтесь и получите бесплатный пробный баланс, чтобы оценить удобный доступ к AI',
      button: 'Зарегистрироваться бесплатно'
    },
    footer: {
      allRightsReserved: 'Все права защищены.'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'Использование API-ключа',
    subtitle: 'Введите API-ключ, чтобы посмотреть расходы и состояние использования в реальном времени',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Проверить',
    querying: 'Запрос...',
    privacyNote: 'Ключ обрабатывается локально в браузере и нигде не сохраняется',
    dateRange: 'Период:',
    dateRangeToday: 'Сегодня',
    dateRange7d: '7 дней',
    dateRange30d: '30 дней',
    dateRange90d: '90 дней',
    dateRangeCustom: 'Свой период',
    apply: 'Применить',
    used: 'Использовано',
    detailInfo: 'Подробности',
    tokenStats: 'Статистика токенов',
    dailyDetail: 'По дням',
    modelStats: 'Использование по моделям',
    // Table headers
    date: 'Дата',
    model: 'Модель',
    requests: 'Запросы',
    inputTokens: 'Входные токены',
    outputTokens: 'Выходные токены',
    cacheCreationTokens: 'Создание кэша',
    cacheReadTokens: 'Чтение кэша',
    cacheWriteTokens: 'Запись кэша',
    totalTokens: 'Всего токенов',
    cost: 'Стоимость',
    // Status
    quotaMode: 'Режим квоты ключа',
    walletBalance: 'Баланс кошелька',
    // Ring card titles
    totalQuota: 'Общая квота',
    limit5h: 'Лимит на 5 часов',
    limitDaily: 'Дневной лимит',
    limit7d: 'Лимит на 7 дней',
    limitWeekly: 'Недельный лимит',
    limitMonthly: 'Месячный лимит',
    // Detail rows
    remainingQuota: 'Остаток квоты',
    expiresAt: 'Действует до',
    todayExpires: '(истекает сегодня)',
    daysLeft: '(дней: {days})',
    usedQuota: 'Использовано квоты',
    resetNow: 'Скоро сбросится',
    subscriptionType: 'Тип подписки',
    billingType: 'Тип тарификации',
    subscriptionExpires: 'Подписка до',
    // Usage stat cells
    todayRequests: 'Запросов сегодня',
    todayInputTokens: 'Входные сегодня',
    todayOutputTokens: 'Выходные сегодня',
    todayTokens: 'Токенов сегодня',
    todayCacheCreation: 'Создание кэша сегодня',
    todayCacheRead: 'Чтение кэша сегодня',
    todayCost: 'Стоимость сегодня',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Всего запросов',
    totalInputTokens: 'Всего входных',
    totalOutputTokens: 'Всего выходных',
    totalTokensLabel: 'Всего токенов',
    totalCacheCreation: 'Всего создано кэша',
    totalCacheRead: 'Всего прочитано из кэша',
    totalCost: 'Общая стоимость',
    avgDuration: 'Среднее время',
    // Messages
    enterApiKey: 'Введите API-ключ',
    querySuccess: 'Данные получены',
    queryFailed: 'Не удалось выполнить запрос',
    queryFailedRetry: 'Не удалось выполнить запрос, попробуйте позже',
    noDailyUsage: 'Нет данных по дням',
  },

  // Setup Wizard
  setup: {
    title: 'Установка Sub2API',
    description: 'Настройте ваш экземпляр Sub2API',
    database: {
      title: 'Настройка базы данных',
      description: 'Подключение к базе данных PostgreSQL',
      host: 'Хост',
      port: 'Порт',
      username: 'Имя пользователя',
      password: 'Пароль',
      databaseName: 'Имя базы данных',
      sslMode: 'Режим SSL',
      passwordPlaceholder: 'Пароль',
      ssl: {
        disable: 'Отключить',
        require: 'Требовать',
        verifyCa: 'Проверять CA',
        verifyFull: 'Полная проверка'
      }
    },
    redis: {
      title: 'Настройка Redis',
      description: 'Подключение к серверу Redis',
      host: 'Хост',
      port: 'Порт',
      username: 'Имя пользователя (необязательно)',
      password: 'Пароль (необязательно)',
      database: 'База данных',
      usernamePlaceholder: 'Оставьте пустым для пользователя по умолчанию',
      passwordPlaceholder: 'Пароль',
      enableTls: 'Включить TLS',
      enableTlsHint: 'Подключаться к Redis по TLS (публичные сертификаты CA)'
    },
    admin: {
      title: 'Аккаунт администратора',
      description: 'Создайте аккаунт администратора',
      email: 'Email',
      password: 'Пароль',
      confirmPassword: 'Подтверждение пароля',
      passwordPlaceholder: 'Не менее 8 символов',
      confirmPasswordPlaceholder: 'Повторите пароль',
      passwordMismatch: 'Пароли не совпадают'
    },
    ready: {
      title: 'Всё готово к установке',
      description: 'Проверьте настройки и завершите установку',
      database: 'База данных',
      redis: 'Redis',
      adminEmail: 'Email администратора'
    },
    status: {
      testing: 'Проверка...',
      success: 'Подключение установлено',
      testConnection: 'Проверить подключение',
      installing: 'Установка...',
      completeInstallation: 'Завершить установку',
      completed: 'Установка завершена!',
      redirecting: 'Переход на страницу входа...',
      restarting: 'Сервис перезапускается, подождите...',
      timeout: 'Перезапуск сервиса занимает больше времени, чем ожидалось. Обновите страницу вручную.'
    }
  },

  // Common
}
