/**
 * validateServiceQuota — service-quota 表单的统一校验函数。
 *
 * 设计：
 * - 返回 `{ ok, errors: ValidationError[] }`，errors 用 path（点路径如 'limiters[0].limit_value'）
 *   定位到字段，code 走 i18n 在调用端拼文案；后端校验失败响应也走同一套 code，便于映射。
 * - 不抛异常：调用方按 ok 分支选择 toast 或行内显示
 *
 * 与后端校验的关系：
 * - 后端 service_quota_validation.go 是底线，前端是 UX
 * - 错误码（code）必须前后端对齐，后端 SERVICE_QUOTA_VALIDATION_ERROR
 *   的 details.fields[*].code 字段直接复用这里的常量
 */
import { limiterUsesTokenComponents } from '@/api/admin/serviceQuota'
import type {
  ServiceQuotaLimiterInput,
  ServiceQuotaPathInput,
  ServiceQuotaRuleInput,
} from '@/api/admin/serviceQuota'
import { parseFieldErrors } from '@/utils/fieldErrors'

/**
 * 校验错误码：与后端 SERVICE_QUOTA_VALIDATION_ERROR.details.fields[*].code 对齐。
 *
 * 调用方根据 code 选 i18n 文案；新增 code 时同步更新 zh/en locale 与后端常量。
 */
export const ServiceQuotaErrorCode = {
  Required: 'REQUIRED',
  InvalidValue: 'INVALID_VALUE',
  MustBePositive: 'MUST_BE_POSITIVE',
  TargetUsersRequired: 'TARGET_USERS_REQUIRED',
  TokenComponentsRequired: 'TOKEN_COMPONENTS_REQUIRED',
  PlatformRequired: 'PLATFORM_REQUIRED',
} as const

export type ServiceQuotaErrorCode = (typeof ServiceQuotaErrorCode)[keyof typeof ServiceQuotaErrorCode]

export interface ValidationError {
  /** 点路径 / 索引访问，例如 'limiters[0].limit_value' / 'paths[2].platform' / 'target_user_ids' */
  path: string
  /** 错误码（与后端对齐），调用方走 i18n 拼文案 */
  code: ServiceQuotaErrorCode | string
}

export interface ValidationResult {
  ok: boolean
  errors: ValidationError[]
}

/** 校验单条 limiter（按 path 前缀返回 errors） */
function validateLimiter(lim: ServiceQuotaLimiterInput, idx: number, errors: ValidationError[]): void {
  const prefix = `limiters[${idx}]`

  // limiter_type 必填且为合法类型
  if (!lim.limiter_type) {
    errors.push({ path: `${prefix}.limiter_type`, code: ServiceQuotaErrorCode.Required })
  }

  // window_mode 必填（concurrency 自动用 fixed，其他类型由用户选）
  if (!lim.window_mode) {
    errors.push({ path: `${prefix}.window_mode`, code: ServiceQuotaErrorCode.Required })
  }

  // limit_value 必须 > 0
  // 注意：limit_value 在表单可能是 null/undefined/NaN/字符串/0/负数；
  //   - 空 / NaN → REQUIRED（输入为空，给"必填"提示更友好）
  //   - 0 / 负数 → MUST_BE_POSITIVE
  const raw = lim.limit_value
  if (raw === null || raw === undefined || Number.isNaN(Number(raw))) {
    errors.push({ path: `${prefix}.limit_value`, code: ServiceQuotaErrorCode.Required })
  } else if (!(Number(raw) > 0)) {
    errors.push({ path: `${prefix}.limit_value`, code: ServiceQuotaErrorCode.MustBePositive })
  }

  // TPM/TPD 必须至少勾 1 项 token_components
  if (limiterUsesTokenComponents(lim.limiter_type)) {
    const comps = lim.token_components ?? []
    if (comps.length === 0) {
      errors.push({ path: `${prefix}.token_components`, code: ServiceQuotaErrorCode.TokenComponentsRequired })
    }
  }
}

/** 校验单条 path：仅 platform 必填，其他字段可选（缺失 = 通配该层） */
function validatePath(path: ServiceQuotaPathInput, idx: number, errors: ValidationError[]): void {
  if (!path.platform || !path.platform.trim()) {
    errors.push({ path: `paths[${idx}].platform`, code: ServiceQuotaErrorCode.PlatformRequired })
  }
}

/**
 * validateServiceQuotaRule 校验整张规则编辑表单。
 *
 * 返回 `{ ok, errors }`。errors 为空时 ok=true。
 * 调用方根据 errors[*].path 把红字提示挂到对应字段下方。
 */
export function validateServiceQuotaRule(form: ServiceQuotaRuleInput): ValidationResult {
  const errors: ValidationError[] = []

  // counter_mode 必填
  if (!form.counter_mode) {
    errors.push({ path: 'counter_mode', code: ServiceQuotaErrorCode.Required })
  }

  // counter_mode='user' 时 target_user_ids 必须非空
  if (form.counter_mode === 'user') {
    const ids = form.target_user_ids || []
    if (ids.length === 0) {
      errors.push({ path: 'target_user_ids', code: ServiceQuotaErrorCode.TargetUsersRequired })
    }
  }

  // limiters 至少 1 条；每条按 validateLimiter
  if (!form.limiters || form.limiters.length === 0) {
    errors.push({ path: 'limiters', code: ServiceQuotaErrorCode.Required })
  } else {
    form.limiters.forEach((lim, i) => validateLimiter(lim, i, errors))
  }

  // paths 至少 1 条；每条 platform 必填
  if (!form.paths || form.paths.length === 0) {
    errors.push({ path: 'paths', code: ServiceQuotaErrorCode.Required })
  } else {
    form.paths.forEach((p, i) => validatePath(p, i, errors))
  }

  return { ok: errors.length === 0, errors }
}

/** reason 常量：后端 service_quota_field_validation.go ReasonServiceQuotaValidationError */
export const SERVICE_QUOTA_VALIDATION_REASON = 'SERVICE_QUOTA_VALIDATION_ERROR'

/**
 * 解析后端 service quota 字段校验错误响应。
 *
 * 薄壳实现：实际解析逻辑在通用的 `parseFieldErrors`（utils/fieldErrors.ts），
 * 本函数只绑定 service quota 专属 reason，让调用方零感知。
 *
 * ValidationError 与通用 FieldError 类型字段相同（path + code），TypeScript
 * 结构兼容（duck typing），无需运行时转换。
 *
 * 任何不符合 envelope 形状的失败返回 null，让调用方走 extractApiErrorMessage
 * 兜底 toast。
 */
export function parseServiceQuotaApiErrors(err: unknown): ValidationError[] | null {
  return parseFieldErrors(err, SERVICE_QUOTA_VALIDATION_REASON)
}
