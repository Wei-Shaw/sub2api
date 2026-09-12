export default {
  batchImageGuide: {
    title: 'Generación de imágenes por lotes',
    description: 'Envía varios prompts en un mismo trabajo y descarga las imágenes generadas cuando termine'
  },
  // Home Page
  home: {
    viewOnGithub: 'Ver en GitHub',
    viewDocs: 'Ver la documentación',
    docs: 'Documentación',
    switchToLight: 'Cambiar al modo claro',
    switchToDark: 'Cambiar al modo oscuro',
    dashboard: 'Panel',
    login: 'Iniciar sesión',
    getStarted: 'Empezar',
    goToDashboard: 'Ir al panel',
    // User-focused value proposition
    heroSubtitle: 'Una sola clave, todos los modelos de IA',
    heroDescription: 'Sin necesidad de gestionar varias suscripciones. Accede a Claude, GPT, Gemini y más con una única clave API',
    tags: {
      subscriptionToApi: 'De suscripción a API',
      stickySession: 'Sesión persistente',
      realtimeBilling: 'Pago por uso'
    },
    // Pain points section
    painPoints: {
      title: '¿Te suena?',
      items: {
        expensive: {
          title: 'Suscripciones caras',
          desc: 'Pagar varias suscripciones de IA que se van sumando cada mes'
        },
        complex: {
          title: 'Caos de cuentas',
          desc: 'Gestionar cuentas y claves API repartidas entre distintas plataformas'
        },
        unstable: {
          title: 'Cortes de servicio',
          desc: 'Cuentas individuales que llegan a su límite y frenan tu trabajo'
        },
        noControl: {
          title: 'Sin control del gasto',
          desc: 'No poder ver en qué se va tu dinero ni limitar el uso de tu equipo'
        }
      }
    },
    // Solutions section
    solutions: {
      title: 'Nosotros resolvemos esos problemas',
      subtitle: 'Tres pasos sencillos para acceder a la IA sin dolores de cabeza'
    },
    features: {
      unifiedGateway: 'Acceso con un clic',
      unifiedGatewayDesc: 'Consigue una única clave API para usar todos los modelos de IA conectados. Sin trámites por separado.',
      multiAccount: 'Siempre disponible',
      multiAccountDesc: 'Reparto inteligente entre varias cuentas de proveedor con conmutación automática. Adiós a los errores.',
      balanceQuota: 'Paga lo que uses',
      balanceQuotaDesc: 'Facturación por consumo con límites de cuota. Visibilidad total del gasto de tu equipo.'
    },
    // Comparison section
    comparison: {
      title: '¿Por qué elegirnos?',
      headers: {
        feature: 'Comparación',
        official: 'Suscripciones oficiales',
        us: 'Nuestra plataforma'
      },
      items: {
        pricing: {
          feature: 'Precio',
          official: 'Cuota mensual fija, pagas aunque no la uses',
          us: 'Pagas solo por lo que consumes'
        },
        models: {
          feature: 'Elección de modelos',
          official: 'Un solo proveedor',
          us: 'Cambia de modelo libremente'
        },
        management: {
          feature: 'Gestión de cuentas',
          official: 'Gestionar cada servicio por separado',
          us: 'Una clave unificada, un solo panel'
        },
        stability: {
          feature: 'Estabilidad',
          official: 'Límites de una sola cuenta',
          us: 'Grupo de varias cuentas con conmutación automática'
        },
        control: {
          feature: 'Control del consumo',
          official: 'No disponible',
          us: 'Cuotas y estadísticas detalladas'
        }
      }
    },
    providers: {
      title: 'Modelos de IA compatibles',
      description: 'Una API, muchas opciones',
      supported: 'Disponible',
      soon: 'Próximamente',
      claude: 'Claude',
      gemini: 'Gemini',
      antigravity: 'Antigravity',
      more: 'Más'
    },
    // CTA section
    cta: {
      title: '¿Listo para empezar?',
      description: 'Regístrate ahora y recibe saldo de prueba gratis para experimentar el acceso a la IA sin complicaciones',
      button: 'Registrarse gratis'
    },
    footer: {
      allRightsReserved: 'Todos los derechos reservados.'
    }
  },

  // Key Usage Query Page
  keyUsage: {
    title: 'Consumo de la clave API',
    subtitle: 'Introduce tu clave API para ver el gasto y el estado de consumo en tiempo real',
    placeholder: 'sk-ant-mirror-xxxxxxxxxxxx',
    query: 'Consultar',
    querying: 'Consultando...',
    privacyNote: 'Tu clave se procesa localmente en el navegador y no se guarda en ningún sitio',
    dateRange: 'Rango de fechas:',
    dateRangeToday: 'Hoy',
    dateRange7d: '7 días',
    dateRange30d: '30 días',
    dateRange90d: '90 días',
    dateRangeCustom: 'Personalizado',
    apply: 'Aplicar',
    used: 'Usado',
    detailInfo: 'Información detallada',
    tokenStats: 'Estadísticas de tokens',
    dailyDetail: 'Detalle diario',
    modelStats: 'Estadísticas de consumo por modelo',
    // Table headers
    date: 'Fecha',
    model: 'Modelo',
    requests: 'Peticiones',
    inputTokens: 'Tokens de entrada',
    outputTokens: 'Tokens de salida',
    cacheCreationTokens: 'Creación de caché',
    cacheReadTokens: 'Lectura de caché',
    cacheWriteTokens: 'Escritura de caché',
    totalTokens: 'Tokens totales',
    cost: 'Coste',
    // Status
    quotaMode: 'Modo de cuota de la clave',
    walletBalance: 'Saldo del monedero',
    // Ring card titles
    totalQuota: 'Cuota total',
    limit5h: 'Límite de 5 horas',
    limitDaily: 'Límite diario',
    limit7d: 'Límite de 7 días',
    limitWeekly: 'Límite semanal',
    limitMonthly: 'Límite mensual',
    // Detail rows
    remainingQuota: 'Cuota restante',
    expiresAt: 'Caduca el',
    todayExpires: '(caduca hoy)',
    daysLeft: '({days} días)',
    usedQuota: 'Cuota usada',
    resetNow: 'Se restablece en breve',
    subscriptionType: 'Tipo de suscripción',
    subscriptionExpires: 'La suscripción caduca',
    // Usage stat cells
    todayRequests: 'Peticiones de hoy',
    todayInputTokens: 'Entrada de hoy',
    todayOutputTokens: 'Salida de hoy',
    todayTokens: 'Tokens de hoy',
    todayCacheCreation: 'Creación de caché de hoy',
    todayCacheRead: 'Lectura de caché de hoy',
    todayCost: 'Coste de hoy',
    rpmTpm: 'RPM / TPM',
    totalRequests: 'Peticiones totales',
    totalInputTokens: 'Entrada total',
    totalOutputTokens: 'Salida total',
    totalTokensLabel: 'Tokens totales',
    totalCacheCreation: 'Creación de caché total',
    totalCacheRead: 'Lectura de caché total',
    totalCost: 'Coste total',
    avgDuration: 'Duración media',
    // Messages
    enterApiKey: 'Introduce una clave API',
    querySuccess: 'Consulta realizada',
    queryFailed: 'La consulta falló',
    queryFailedRetry: 'La consulta falló, inténtalo de nuevo más tarde',
    noDailyUsage: 'No hay datos de consumo diario',
  },

  // Setup Wizard
  setup: {
    title: 'Instalación de Sub2API',
    description: 'Configura tu instancia de Sub2API',
    database: {
      title: 'Configuración de la base de datos',
      description: 'Conecta con tu base de datos PostgreSQL',
      host: 'Servidor',
      port: 'Puerto',
      username: 'Usuario',
      password: 'Contraseña',
      databaseName: 'Nombre de la base de datos',
      sslMode: 'Modo SSL',
      passwordPlaceholder: 'Contraseña',
      ssl: {
        disable: 'Desactivado',
        require: 'Obligatorio',
        verifyCa: 'Verificar CA',
        verifyFull: 'Verificación completa'
      }
    },
    redis: {
      title: 'Configuración de Redis',
      description: 'Conecta con tu servidor Redis',
      host: 'Servidor',
      port: 'Puerto',
      username: 'Usuario (opcional)',
      password: 'Contraseña (opcional)',
      database: 'Base de datos',
      usernamePlaceholder: 'Déjalo vacío para el usuario por defecto',
      passwordPlaceholder: 'Contraseña',
      enableTls: 'Activar TLS',
      enableTlsHint: 'Usar TLS al conectar con Redis (certificados de CA públicas)'
    },
    admin: {
      title: 'Cuenta de administrador',
      description: 'Crea tu cuenta de administrador',
      email: 'Correo electrónico',
      password: 'Contraseña',
      confirmPassword: 'Confirmar la contraseña',
      passwordPlaceholder: 'Mínimo 8 caracteres',
      confirmPasswordPlaceholder: 'Confirma la contraseña',
      passwordMismatch: 'Las contraseñas no coinciden'
    },
    ready: {
      title: 'Listo para instalar',
      description: 'Revisa tu configuración y completa la instalación',
      database: 'Base de datos',
      redis: 'Redis',
      adminEmail: 'Correo del administrador'
    },
    status: {
      testing: 'Probando...',
      success: 'Conexión correcta',
      testConnection: 'Probar la conexión',
      installing: 'Instalando...',
      completeInstallation: 'Completar la instalación',
      completed: '¡Instalación completada!',
      redirecting: 'Redirigiendo a la página de inicio de sesión...',
      restarting: 'El servicio se está reiniciando, espera un momento...',
      timeout: 'El reinicio del servicio está tardando más de lo esperado. Actualiza la página manualmente.'
    }
  },

  // Common
}
