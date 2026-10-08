export default {
  audit: {
    title: 'Registros de auditoría',
    description: 'Registra las operaciones de administración realizadas por administradores y usuarios. De las credenciales enviadas en las cabeceras solo se conservan los primeros y últimos caracteres, y el contenido de las peticiones se censura. Las entradas no se pueden borrar una por una; vaciarlas todas requiere verificación en dos pasos.',
    clearAll: 'Vaciar todo',
    empty: 'Todavía no hay registros de auditoría',
    loadFailed: 'No se pudieron cargar los registros de auditoría',
    filters: {
      all: 'Todos',
      q: 'Palabra clave',
      qPlaceholder: 'Ruta / acción / correo del actor',
      actorEmail: 'Correo del actor',
      action: 'Acción',
      clientIp: 'IP del cliente',
      method: 'Método',
      authMethod: 'Método de autenticación',
      result: 'Resultado',
      resultSuccess: 'Correcto',
      resultFailure: 'Fallido',
      startTime: 'Hora de inicio',
      endTime: 'Hora de fin'
    },
    columns: {
      time: 'Hora',
      actor: 'Actor',
      action: 'Acción',
      method: 'Método',
      result: 'Resultado',
      clientIp: 'IP del cliente',
      detail: 'Detalle'
    },
    detail: {
      title: 'Detalle del registro de auditoría',
      actorRole: 'Rol',
      methodPath: 'Método / Ruta',
      latency: 'Latencia',
      requestId: 'ID de la petición',
      credential: 'Credencial (enmascarada)',
      userAgent: 'User-Agent',
      requestBody: 'Cuerpo de la petición (censurado)',
      extra: 'Adicional'
    },
    clearConfirm: {
      title: 'Vaciar todos los registros de auditoría',
      message: 'Esto elimina de forma permanente todos los registros de auditoría y no se puede deshacer. La propia acción de vaciado queda registrada. ¿Continuar?',
      totpTitle: 'Introduce el código de dos pasos',
      totpHint: 'Vaciar los registros de auditoría requiere una verificación TOTP reciente.',
      success: 'Se vaciaron {count} registro(s) de auditoría',
      failed: 'No se pudieron vaciar los registros de auditoría'
    }
  }
}
