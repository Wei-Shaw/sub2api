/**
 * 错误请求"分类"虚拟维度:phase + error type → 用户侧粗分类码。
 * 镜像后端 service.MapUserErrorCategory(backend/internal/service/ops_user_error.go),
 * 两处修改须同步。返回稳定分类码,展示文案走 i18n `usage.errors.categories.*`。
 */
export function mapErrorCategory(phase?: string | null, errType?: string | null): string {
  switch ((phase || '').toLowerCase()) {
    case 'auth':
      return 'auth'
    case 'routing':
      return 'service_unavailable'
    case 'upstream':
    case 'network':
      return 'upstream'
    case 'internal':
      return 'internal'
    case 'request':
      switch ((errType || '').toLowerCase()) {
        case 'rate_limit_error':
          return 'rate_limit'
        case 'billing_error':
        case 'subscription_error':
          return 'quota'
        case 'invalid_request_error':
          return 'invalid_request'
        case 'cyber_policy':
          return 'cyber'
      }
  }
  return 'other'
}
