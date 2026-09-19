/** Channel Monitor V2 (user + admin passive monitor UI) */
export default {
  channelMonitorV2: {
    title: 'Monitor de canales',
    updating: 'Actualizando los datos',
    updatedTo: 'Actualizado hasta {time}',
    partialCoverage: 'Cobertura histórica parcial',
    bootstrap: {
      title: 'Construyendo el histórico del monitor',
      description:
        'La primera vez que se activa, la agregación pasiva rellena en segundo plano y sin ruido las ventanas de 90 m, 24 h, 7 d y 30 d. Todos los rangos quedan completos cuando termina.',
      progress: '{percent}% completado',
      working: 'Agregando en segundo plano…',
    },
    timeRange: 'Rango de tiempo',
    clearFilters: 'Restablecer',
    refreshingFilters: 'Cambiaron los filtros; actualizando matriz, tendencia y detalles…',
    switchingData: 'Cambiando los datos filtrados…',
    summaryAria: 'Resumen del rango seleccionado',
    loadFailed: 'No se pudo cargar el monitor de canales',
    detailLoadFailed: 'No se pudieron cargar los detalles del monitor de canales',
    otherModels: 'Otros modelos',
    ignored: 'Ignorado',
    currentUser: 'Usuario actual',
    ranges: { '90m': '90 m', '24h': '24 h', '7d': '7 d', '30d': '30 d' },
    filters: {
      platform: 'Plataforma', allPlatforms: 'Todas', group: 'Grupo', allGroups: 'Todos', model: 'Modelo', allModels: 'Todos',
      empty: 'Sin opciones', selectedCount: '{count}', labelValue: '{label}: {value}'
    },
    groupBy: {
      label: 'Agrupar por', platform: 'Plataforma', platformGroup: 'Plataforma / Grupo', platformModel: 'Plataforma / Modelo', platformGroupModel: 'Plataforma / Grupo / Modelo'
    },
    trendView: { label: 'Vista de tendencia', pulse: 'Matriz de pulsos', line: 'Gráfico de líneas' },
    healthMode: { label: 'Indicador de salud', overall: 'General', success: 'Tasa de error', ttft: 'Primer token', cache: 'Tasa de caché' },
    tabs: { aria: 'Dimensión del detalle', models: 'Modelos', errors: 'Motivos de error', users: 'Ranking de usuarios' },
    metrics: {
      rpm: 'RPM',
      tpm: 'TPM',
      tps: 'Tokens/s',
      rpmDetail: 'Peticiones por minuto',
      tpmDetail: 'Tokens por minuto',
      tpsDetail: 'Calculado como TPM ÷ 60',
      errorRate: 'Tasa de error',
      ttft: 'Primer token',
      ttftP50: 'Primer token P50',
      durationP50: 'Duración P50',
      cacheRate: 'Tasa de caché',
      cacheDetail: 'Proporción servida desde caché',
      successRate: 'Tasa de éxito',
      successRateValue: 'Tasa de éxito {value}',
      errorRateValue: 'Tasa de error {value}',
      rpmValue: 'RPM {value}',
      tpmValue: 'TPM {value}',
      tpsValue: 'Tokens/s {value}',
      ttftValue: 'Primer token {value}',
      durationValue: 'Duración {value}',
      cacheRateValue: 'Tasa de caché {value}',
    },
    table: { platformModel: 'Plataforma / Modelo', rank: 'Puesto', user: 'Usuario' },
    empty: { title: 'No hay datos que mostrar', description: 'Prueba a cambiar el rango de tiempo o los filtros' },
    bucket: { minutes: 'Intervalos de {count} minutos', hours: 'Intervalos de {count} horas', days: 'Intervalos de {count} días' },
    matrix: {
      title: 'Tendencia de disponibilidad', description: 'Cada fila es una dimensión del canal y cada bloque un intervalo agregado; pasa el cursor para ver los detalles', wheelZoom: 'Desplaza el ratón sobre los bloques para acercar (rango más corto, bloques más anchos)', wheelZoomX: 'Desplaza el ratón sobre los bloques para acercar (rango más corto, bloques más anchos)', dimension: 'Dimensión del canal', emptyTitle: 'No hay datos de matriz para la ventana seleccionada', legendAria: 'Leyenda de la puntuación de salud', bad: 'Malo', good: 'Bueno', healthyLegend: 'Saludable (≥80)', warningLegend: 'Vigilar (50–79)', criticalLegend: 'Crítico (<50)', unknownLegend: 'Sin tráfico / muestras insuficientes', noTraffic: 'Sin tráfico en este intervalo', noTrafficAt: '{time} · sin tráfico', scoreLine: 'Puntuación de salud {score}', resetZoom: 'Restablecer el zoom'
    },
    chart: {
      title: 'Tendencia de disponibilidad', description: 'Tendencia suavizada: tasa de error · primer token P50 · tasa de caché', emptyTitle: 'No hay datos de tendencia para la ventana seleccionada', errorLegend: 'Tasa de error (eje izquierdo %)', cacheLegend: 'Tasa de caché (eje izquierdo %)', ttftLegend: 'Primer token P50 (eje derecho)', errorDataset: 'Tendencia de la tasa de error %', cacheDataset: 'Tendencia de la tasa de caché %', ttftDataset: 'Tendencia del primer token P50 (ms)', percentAxis: 'Tasa %', resetZoom: 'Restablecer el zoom'
    },
    errorDetail: { http: 'HTTP {code}', upstream: 'Proveedor {code}', noMessage: 'Sin mensaje de error', empty: 'Solo tasas por categoría (los mensajes de ejemplo son solo para administradores)' },
    errorCategories: {
      content_policy: 'Política de contenido', authentication: 'Autenticación', context_limit: 'Límite de contexto', invalid_request: 'Petición no válida', model_unsupported: 'Modelo no compatible', group_access: 'Acceso al grupo', quota_or_balance: 'Cuota o saldo', account_pool_unavailable: 'Grupo de cuentas no disponible', rate_or_capacity: 'Límite de tasa o capacidad', timeout: 'Tiempo agotado', transport_or_stream: 'Transporte o flujo', upstream_forbidden: 'Proveedor prohibido', not_found: 'No encontrado', client_cancelled: 'Cancelado por el cliente', upstream_5xx: 'Error 5xx del proveedor', internal: 'Interno', other: 'Otro'
    },
    rank: {
      gold: 'Puesto 1 oro',
      silver: 'Puesto 2 plata',
      bronze: 'Puesto 3 bronce',
      place: 'Puesto {n}',
      unranked: 'Sin clasificar',
    },
    settings: {
      title: 'Configuración del monitor de datos V2',
      description:
        'Configura las dimensiones de agregación pasiva del consumo (plataforma / modelo / grupo) y la frecuencia de actualización. Los colores de salud y los detalles de la página /monitor del usuario muestran tasas, RPM y TPM, no el volumen absoluto de peticiones.',
      save: 'Guardar',
      loading: 'Cargando…',
      loadFailed: 'No se pudo cargar la configuración V2',
      saveSuccess: 'Configuración del monitor V2 guardada',
      saveFailed: 'No se pudo guardar la configuración V2',
      modeBanner:
        'El modo del sistema es actualmente {mode}. La agregación por minuto de V2 no se ejecutará; esta configuración se puede preparar ahora y se aplicará al cambiar a {modeV2}. Cambia el modo en Ajustes del sistema → Interruptores de funciones.',
      modeClosed: 'Monitor de canales desactivado',
      modeV1: 'Sondas activas V1',
      modeV2: 'Monitorización pasiva V2',
      enableTitle: 'Activar la agregación V2',
      enableHint:
        'Se aplica cuando el modo del sistema es V2. Desactivarlo solo detiene la agregación de esta configuración; el modo del sistema se sigue cambiando en Interruptores de funciones.',
      refreshTitle: 'Intervalo de agregación',
      refreshHint: 'Afecta a la granularidad temporal de la matriz y a la frecuencia de actualización',
      refreshAria: 'Intervalo de agregación',
      platformsTitle: 'Plataformas y modelos',
      platformsHint:
        'Vacío = mostrar todos los nombres reales de modelo; si se rellena, solo los modelos indicados tienen fila propia y el resto se agrupa en «Otros»',
      modelsPlaceholder: 'Vacío = todos los modelos reales; o indica los modelos populares (el resto → Otros)',
      badgeAllModels: 'Todos los modelos',
      badgeOther: '+ Otros',
      groupsTitle: 'Grupos monitorizados',
      groupsSelected: '{count} grupos seleccionados',
      groupsAll: 'Todos los grupos',
      groupsEmpty: 'No hay grupos disponibles',
      errorsTitle: 'Categorías de error e ignorados',
      errorsHint:
        'Las categorías marcadas como «ignorar» se excluyen de la tasa de error y de la puntuación de salud, pero siguen apareciendo atenuadas en el desglose de errores. Los errores sin coincidencia se agrupan en «Otros».',
      ignoredSummary: 'Ignoradas {ignored} categorías · contadas en la tasa de error {counted} categorías',
      healthTitle: 'Umbrales de salud',
      healthHint:
        'Controla las bandas de color que ve el usuario y la puntuación general. Los valores por defecto son tolerantes para que una tasa de error pequeña o una caché baja no se muestren de inmediato como problema.',
      fields: {
        minimumSample: 'Muestras mínimas',
        warningError: 'Tasa de error a vigilar %',
        criticalError: 'Tasa de error crítica %',
        targetTtft: 'TTFT objetivo ms',
        warningTtft: 'TTFT a vigilar ms',
        criticalTtft: 'TTFT crítico ms',
        warningCache: 'Tasa de caché a vigilar %',
        criticalCache: 'Tasa de caché crítica %',
      },
      namedModelsEmpty: 'Las listas de modelos por plataforma están vacías: se mostrarán todos los nombres reales de modelo (no se agruparán en «Otros»).',
      namedModelsCount: 'Mostrando {count} dimensiones de modelo con nombre; los modelos no indicados se agrupan en el «Otros» de cada plataforma.',
      userContractTitle: 'Qué se le muestra al usuario',
      userContract: {
        health: 'Peso de los colores de salud: tasa de error 60 % + primer token P50 20 % + tasa de caché 20 % (umbrales configurables arriba)',
        trend: 'La tendencia puede alternar entre matriz de pulsos y gráfico de líneas (error · caché · primer token)',
        latency: 'La latencia muestra media · P50 · P90; no se muestran los recuentos absolutos de peticiones ni de errores',
        models: 'Con las listas de modelos vacías se muestran los nombres reales y nunca se vuelca todo en «Otros»',
      },
    },
    admin: {
      descriptionV1:
        'El modo del sistema es sondas activas V1: gestiona los monitores de sondeo y lanza comprobaciones ahora; la agregación V2 no se ejecuta.',
      descriptionV2:
        'El modo del sistema es monitorización pasiva V2: configura las dimensiones de agregación; las sondas activas V1 no se ejecutan.',
      tabAria: 'Gestión del monitor',
      tabV2: 'Configuración del monitor de datos V2',
      tabV1Active: 'Sondas activas V1',
      tabV1History: 'Histórico V1 (las sondas no están activas en el modo actual)',
    },
  },
}
