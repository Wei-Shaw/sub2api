/**
 * paramSpec.ts
 *
 * 递归 JSON Schema 版参数声明工具。管理员在"模型介绍"里为每个字段声明
 * required / description / enum / options，并且允许字段类型是 object / array，
 * 从而递归嵌套子 schema：
 *
 *   object → 展开一个"子字段声明列表"（properties）
 *   array  → 展开一个"元素 schema"（items，数组同构）
 *
 * 存储 shape（每个 key 的 value）：
 *   1) 叶子字段声明（string / number / boolean）：
 *      { value, required?, description?, enum?, options? }
 *      —— 不含 properties / items 键。
 *   2) 对象字段声明：
 *      { properties: { <k>: <schema>, ... }, required?, description? }
 *   3) 数组字段声明：
 *      { items: <schema>, value?: [...], required?, description? }
 *   4) 其它任意值（非对象）：视为"无字段声明"，仅作为 curl / 兜底 body。
 *
 * 判定顺序：先看是否含 properties（object） / items（array）；否则按 looksLikeLeafSpec
 * 判定是否是叶子 spec；再否则退化为"无字段声明"。
 *
 * 注意：本模块只做"读侧"解析，不涉及写入侧的编辑器状态。
 */

import {
  normalizeMediaUrlWidget,
  normalizeSingleMediaWidget,
  type MediaUrlWidget,
  type SingleMediaWidget,
} from '@/utils/mediaUrlWidget'

export type FieldRawType = 'string' | 'number' | 'boolean' | 'object' | 'array'

/**
 * FieldSpec 描述一个字段声明。它可以是叶子（string/number/boolean），也可以是
 * 递归的 object（含 children）或 array（含 items）。
 *
 * 为方便前端表单：
 *   - 叶子节点：defaultValue（字符串化后的默认值） + rawDefaultValue（保留类型）
 *   - object：children 是子字段声明数组（每个 child 自己也是一个 FieldSpec）
 *   - array：items 是"数组元素的 schema"（一个匿名 FieldSpec，其 key=''），
 *            rawDefaultValue 保存完整的默认值数组
 *
 * key 语义：
 *   - 顶层字段：default_params 里的 map key
 *   - object 子字段：properties 下的 key
 *   - array 元素：key='' （无名，仅作占位）
 */
export interface FieldSpec {
  /** 字段 key（数组元素为空串） */
  key: string
  /** 是否必填（仅前端表单校验/展示使用） */
  required: boolean
  /** 字段描述（展示为 helper 文本；默认视为中文/默认语言） */
  description: string
  /** 字段英文描述（可选）；渲染时会按 i18n locale 选择合适语种，双端兵底不为空即用。 */
  descriptionEn: string
  /**
   * 是否为“高级参数”。true 时演练台会将该字段收纳进“高级参数”折叠区，
   * 默认不展开。缺失时默认 false（兼容旧数据）。
   */
  advanced: boolean  /** 是否为枚举字段（仅叶子有意义） */
  isEnum: boolean
  /** 枚举选项（字符串化后展示；enum=false 时为空数组） */
  options: string[]
  /** 默认值（来自 spec.value；用于表单初始化，转为字符串以便双向绑定） */
  defaultValue: string
  /** 原始默认值（保持类型信息，用于构建请求体时精确还原） */
  rawDefaultValue: unknown
  /** 参数值的原始类型标签 */
  rawType: FieldRawType
  /**
   * 控件形态声明（管理员在编辑页显式选择），渲染时优先按它决定用哪种控件，
   * 不再依赖内容长度等启发式判断。默认 'input'。
   *   - rawType='string'：'input' | 'textarea' | 'image'
   *   - rawType='array' ：'input'（逐元素递归渲染）| 媒体 URL 组控件
   */
  widget: 'input' | 'textarea' | 'PromptTextArea' | SingleMediaWidget | MediaUrlWidget
  /** 仅 widget==='textarea' 时有意义。默认 3。 */
  textareaRows: number
  /** PromptTextArea 可引用的字段路径；空数组表示不提供 @ 候选。 */
  referenceFields: string[]
  /**
   * 仅 rawType==='array' 时有意义：元素个数上限。0 表示不限制。
   * 演练台据此禁用"添加"按钮，并在提交前做一次校验。
   */
  maxItems: number
  /** object 子字段声明列表（仅 rawType==='object' 时非空） */
  children: FieldSpec[]
  /** array 元素的 schema（仅 rawType==='array' 时非 null；数组同构） */
  items: FieldSpec | null
}

/**
 * 判定"叶子字段声明"：
 *   - 是普通对象（不是 null / 不是数组）
 *   - 不含 properties / items 键（否则应识别为 object / array）
 *   - 至少显式声明了 value / required / description / enum / options 中的任意一个
 */
function looksLikeLeafSpec(v: unknown): v is Record<string, unknown> {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false
  const obj = v as Record<string, unknown>
  if ('properties' in obj || 'items' in obj) return false
  return (
    'value' in obj ||
    'required' in obj ||
    'description' in obj ||
    'enum' in obj ||
    'options' in obj
  )
}

/**
 * 判定 object schema：raw 是普通对象，且含 properties 键。
 */
function looksLikeObjectSpec(v: unknown): v is { properties: Record<string, unknown> } {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false
  return 'properties' in (v as Record<string, unknown>)
}

/**
 * 判定 array schema：raw 是普通对象，且含 items 键（一个子 schema）。
 */
function looksLikeArraySpec(v: unknown): v is { items: unknown } {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false
  return 'items' in (v as Record<string, unknown>)
}

/**
 * 推断叶子 spec.value 的 rawType。undefined/null → 'string'（便于表单双向绑定）。
 */
function inferLeafRawType(v: unknown): FieldRawType {
  if (v === null || v === undefined) return 'string'
  if (typeof v === 'boolean') return 'boolean'
  if (typeof v === 'number') return 'number'
  return 'string'
}

/**
 * 把任意值转成字符串（用于表单初始默认值 / 枚举 option 展示 label）。
 */
function toDisplayString(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return v
  if (typeof v === 'number' || typeof v === 'boolean') return String(v)
  try {
    return JSON.stringify(v)
  } catch {
    return String(v)
  }
}

/**
 * readXOrder：读取 spec 上的 x-order。新格式位于 spec.extra['x-order']；旧格式（早期存量数据）
 * 位于顶层 spec['x-order']。两者优先取 extra，兵底取顶层，同时缺失时给 POSITIVE_INFINITY 让这些字段排到末尾。
 */
function readXOrder(v: unknown): number {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return Number.POSITIVE_INFINITY
  const obj = v as Record<string, unknown>
  const extra = obj.extra
  if (extra && typeof extra === 'object' && !Array.isArray(extra)) {
    const ne = (extra as Record<string, unknown>)['x-order']
    if (typeof ne === 'number' && Number.isFinite(ne)) return ne
  }
  const n = obj['x-order']
  return typeof n === 'number' && Number.isFinite(n) ? n : Number.POSITIVE_INFINITY
}

/**
 * readAdvanced：从 spec.extra.advanced 读取“是否高级参数”。旧数据无此字段默认 false。
 */
function readAdvanced(v: unknown): boolean {
  if (!v || typeof v !== 'object' || Array.isArray(v)) return false
  const obj = v as Record<string, unknown>
  const extra = obj.extra
  if (extra && typeof extra === 'object' && !Array.isArray(extra)) {
    return (extra as Record<string, unknown>).advanced === true
  }
  return false
}

/**
 * sortMapKeys：把一个 map 的 key 列表按每个 value 的 x-order 升序；order 相同时保持原序。
 * 非平稳排序结合 index 作第二关键字 → 等价于“字母序 + x-order 优先”的效果。
 */
function sortMapKeys(m: Record<string, unknown>, keys: string[]): string[] {
  const indexed = keys.map((k, i) => ({ k, i, o: readXOrder(m[k]) }))
  indexed.sort((a, b) => {
    if (a.o !== b.o) return a.o - b.o
    return a.i - b.i
  })
  return indexed.map((x) => x.k)
}

/**
 * parseFieldSpec：把 default_params 中的某个 raw value（对应 key）解析为
 * FieldSpec 节点。递归解析 properties / items。
 *
 * 若 raw 既不是 leaf spec 也不是 object/array spec，返回 null（调用方跳过）。
 */
function parseFieldSpec(key: string, raw: unknown): FieldSpec | null {
  // object：递归解析 properties 下的每个子字段
  if (looksLikeObjectSpec(raw)) {
    const obj = raw as Record<string, unknown>
    const props = obj.properties as Record<string, unknown>
    const children: FieldSpec[] = []
    if (props && typeof props === 'object' && !Array.isArray(props)) {
      // 按 x-order 升序递归，保证“演练台子字段顺序 = 编辑器里行顺序”。
      const sortedKeys = sortMapKeys(props, Object.keys(props))
      for (const ck of sortedKeys) {
        const child = parseFieldSpec(ck, props[ck])
        // 对于 object 子字段：即便 raw 不含 spec 元数据（例如 { childKey: 'a-value' }），
        // 也把 child 当成 leaf 加进去。这里再做一次兜底。
        if (child) {
          children.push(child)
        } else {
          children.push(makeLeafFromPlain(ck, props[ck]))
        }
      }
    }
    return {
      key,
      required: obj.required === true,
      advanced: readAdvanced(obj),
      description: typeof obj.description === 'string' ? obj.description : '',
      descriptionEn: typeof obj.description_en === 'string' ? obj.description_en : '',
      isEnum: false,
      options: [],
      defaultValue: '',
      rawDefaultValue: undefined,
      rawType: 'object',
      widget: 'input',
      textareaRows: 3,
      referenceFields: [],
      maxItems: 0,
      children,
      items: null,
    }
  }
  // array：解析 items 子 schema
  if (looksLikeArraySpec(raw)) {
    const obj = raw as Record<string, unknown>
    const itemsRaw = obj.items
    let items = parseFieldSpec('', itemsRaw)
    if (!items) {
      items = makeLeafFromPlain('', itemsRaw)
    }
    // 兼容旧 imageUrls，读入后统一归一成 canonical 名称。
    const arrWidget: FieldSpec['widget'] = normalizeMediaUrlWidget(obj.widget) ?? 'input'
    // maxItems：非法 / <=0 归 0（不限制）；上限 100，与编辑器写侧保持一致。
    const rawMax = Number(obj.maxItems)
    const maxItems = Number.isFinite(rawMax) && rawMax > 0 ? Math.min(100, Math.trunc(rawMax)) : 0
    const rawDefaults = Array.isArray(obj.value) ? obj.value : []
    return {
      key,
      required: obj.required === true,
      advanced: readAdvanced(obj),
      description: typeof obj.description === 'string' ? obj.description : '',
      descriptionEn: typeof obj.description_en === 'string' ? obj.description_en : '',
      isEnum: false,
      options: [],
      defaultValue: toDisplayString(rawDefaults),
      rawDefaultValue: rawDefaults,
      rawType: 'array',
      widget: arrWidget,
      textareaRows: 3,
      referenceFields: [],
      maxItems,
      children: [],
      items,
    }
  }
  // 叶子
  if (looksLikeLeafSpec(raw)) {
    const spec = raw as Record<string, unknown>
    const value = spec.value
    const isEnum = spec.enum === true
    let options: string[] = []
    if (isEnum && Array.isArray(spec.options)) {
      options = (spec.options as unknown[]).map((o) => toDisplayString(o))
    }
    // widget / rows：仅 string 叶子有意义，其他类型一律归为 'input'/3。
    // 未声明 widget 的旧数据（支持向后兼容）默认依旧逻辑走 'input'，
    // 演练台仍可根据默认值长度启动 textarea fallback 启发式。
    const rawType: FieldRawType = inferLeafRawType(value)
    let widget: FieldSpec['widget'] = 'input'
    let textareaRows = 3
    if (rawType === 'string') {
      const w = (spec as Record<string, unknown>).widget
      if (w === 'textarea' || w === 'PromptTextArea') {
        widget = w
        const rr = Number((spec as Record<string, unknown>).rows)
        if (Number.isFinite(rr) && rr > 0) {
          textareaRows = Math.min(100, Math.max(1, Math.trunc(rr)))
        }
      } else {
        widget = normalizeSingleMediaWidget(w) ?? 'input'
      }
    }
    return {
      key,
      required: spec.required === true,
      advanced: readAdvanced(spec),
      description: typeof spec.description === 'string' ? spec.description : '',
      descriptionEn: typeof spec.description_en === 'string' ? spec.description_en : '',
      isEnum,
      options,
      defaultValue: toDisplayString(value),
      rawDefaultValue: value,
      rawType,
      widget,
      textareaRows,
      referenceFields: Array.isArray(spec.reference_fields)
        ? spec.reference_fields.filter((field): field is string => typeof field === 'string').map((field) => field.trim()).filter(Boolean)
        : [],
      maxItems: 0,
      children: [],
      items: null,
    }
  }
  return null
}

/**
 * makeLeafFromPlain：对不含元数据的原始值（例如 object 子字段直接写成
 * "foo": 1）做兜底，包装为一个匿名叶子 FieldSpec。这样即便管理员偷懒不写
 * { value: 1 }，也能作为默认值展示。
 */
function makeLeafFromPlain(key: string, v: unknown): FieldSpec {
  const rawType = inferLeafRawType(v)
  return {
    key,
    required: false,
    advanced: false,
    description: '',
    descriptionEn: '',
    isEnum: false,
    options: [],
    defaultValue: toDisplayString(v),
    rawDefaultValue: v,
    rawType,
    widget: 'input',
    textareaRows: 3,
    referenceFields: [],
    maxItems: 0,
    children: [],
    items: null,
  }
}

/**
 * 从 default_params map 中提取“字段声明”列表（顶层）。
 * 只会收集能被 parseFieldSpec 识别的 entry；其它 entry 跳过。
 *
 * 顺序：顶层同样按每个 spec 的 `x-order` 升序，保证演练台字段展示顺序
 * 与编辑器一致。Go map JSON marshal 按字母序输出，无法直接承载自定义顺序，
 * 必须依靠额外的 x-order 字段。
 */
export function extractFieldSpecs(
  params: Record<string, unknown> | null | undefined
): FieldSpec[] {
  if (!params || typeof params !== 'object') return []
  const out: FieldSpec[] = []
  const sortedKeys = sortMapKeys(
    params as Record<string, unknown>,
    Object.keys(params)
  )
  for (const key of sortedKeys) {
    const spec = parseFieldSpec(key, params[key])
    if (spec) out.push(spec)
  }
  return out
}
/**
 * 将用户在表单中输入的字符串 value 按 rawType 还原为请求体里的值。
 * 仅对叶子类型（string/number/boolean）有意义；object/array 应由调用方
 * 递归自行合成 body。
 */
export function coerceFieldValue(
  spec: FieldSpec,
  input: string
): unknown {
  const trimmed = input == null ? '' : String(input).trim()
  if (trimmed === '') {
    return spec.rawDefaultValue
  }
  switch (spec.rawType) {
    case 'number': {
      const n = Number(trimmed)
      return Number.isFinite(n) ? n : 0
    }
    case 'boolean': {
      const low = trimmed.toLowerCase()
      return low === 'true' || low === '1' || low === 'yes'
    }
    case 'string':
    default:
      return trimmed
  }
}

/**
 * buildDefaultBody：从 default_params 中构建"兜底请求体"。
 * 递归处理 object（→ 对象）与 array（→ 用 items.rawDefaultValue 作单元素样例）。
 * 叶子字段直接取 spec.value。
 *
 * 结果对象即使为空也返回 {}。
 */
export function buildDefaultBody(
  params: Record<string, unknown> | null | undefined
): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  if (!params || typeof params !== 'object') return out
  for (const key of Object.keys(params)) {
    const raw = params[key]
    const spec = parseFieldSpec(key, raw)
    if (spec) {
      const v = fieldSpecToDefaultValue(spec)
      if (v !== undefined) out[key] = v
    } else if (raw !== undefined) {
      // 完全非结构化：保留原样（用户可能是直接写了个字面量作为兜底 body）。
      out[key] = raw
    }
  }
  return out
}

/**
 * fieldSpecToDefaultValue：从一个 FieldSpec 递归产生默认值。
 *   - object：{ childKey: fieldSpecToDefaultValue(child), ... }；子字段值为
 *     undefined 时跳过（避免生成 undefined 值污染 JSON）。
 *   - array：读取 schema.value 中显式配置的默认值数组；缺省时给 []。
 *   - 叶子：spec.rawDefaultValue（可能为 undefined）。
 */
export function fieldSpecToDefaultValue(spec: FieldSpec): unknown {
  if (spec.rawType === 'object') {
    const obj: Record<string, unknown> = {}
    for (const ch of spec.children) {
      if (!ch.key) continue
      const v = fieldSpecToDefaultValue(ch)
      if (v !== undefined) obj[ch.key] = v
    }
    return obj
  }
  if (spec.rawType === 'array') {
    return cloneJSONValue(Array.isArray(spec.rawDefaultValue) ? spec.rawDefaultValue : [])
  }
  return spec.rawDefaultValue
}

function cloneJSONValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(cloneJSONValue)
  if (value && typeof value === 'object') {
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>).map(([key, child]) => [key, cloneJSONValue(child)])
    )
  }
  return value
}

/**
 * 按 result_field 路径从任意 payload 中提取一个或多个 URL 字符串。
 *
 * 支持的语法（尽量鲁棒，容忍上游返回的多种形状）：
 *   - "video.url"       → 沿属性链取，末尾若是字符串则视为 URL；
 *                          若末尾是对象且带 .url 字段，则取其 .url。
 *   - "videos[0].url"   → 数组下标语法，[*] 等价于遍历数组。
 *   - "images"          → 若目标是数组，遍历元素；每个元素若是字符串直接用，
 *                          若是对象则取其 .url。
 *   - ""（空字符串）    → 返回 []，调用方需走兜底逻辑。
 *
 * 该函数只做"提取"，不做校验；调用方可根据 result_type 决定用 <video> 或 <img>。
 */
export function extractUrlsByPath(
  payload: unknown,
  path: string
): string[] {
  if (!payload || typeof payload !== 'object') return []
  const p = (path || '').trim()
  if (!p) return []

  // 把 "a.b[0].c" / "a[*].b" 切成 tokens: ['a','b','0','c'] / ['a','*','b']
  const tokens: string[] = []
  for (const seg of p.split('.')) {
    if (!seg) continue
    // 处理 seg 中形如 name[0][*] 的下标
    const m = seg.matchAll(/([^[\]]+)|\[([^\]]+)\]/g)
    for (const g of m) {
      const name = g[1]
      const idx = g[2]
      if (name !== undefined) tokens.push(name)
      if (idx !== undefined) tokens.push(idx)
    }
  }

  // 递归求解：给定当前节点和剩余 tokens，返回所有可能的最终节点集合。
  function walk(node: unknown, ts: string[]): unknown[] {
    if (node === null || node === undefined) return []
    if (ts.length === 0) return [node]
    const [head, ...rest] = ts
    // 数组下标 / 通配
    if (Array.isArray(node)) {
      if (head === '*') {
        const acc: unknown[] = []
        for (const el of node) acc.push(...walk(el, rest))
        return acc
      }
      const i = Number(head)
      if (Number.isInteger(i) && i >= 0 && i < node.length) {
        return walk(node[i], rest)
      }
      // token 落在数组上但不是下标：对每个元素继续消费当前 token（宽松处理）。
      const acc: unknown[] = []
      for (const el of node) acc.push(...walk(el, ts))
      return acc
    }
    if (typeof node === 'object') {
      const rec = node as Record<string, unknown>
      if (head === '*') {
        const acc: unknown[] = []
        for (const v of Object.values(rec)) acc.push(...walk(v, rest))
        return acc
      }
      return walk(rec[head], rest)
    }
    return []
  }

  const leaves = walk(payload, tokens)
  const urls: string[] = []
  for (const leaf of leaves) {
    if (typeof leaf === 'string' && leaf) {
      urls.push(leaf)
      continue
    }
    if (leaf && typeof leaf === 'object') {
      const u = (leaf as Record<string, unknown>).url
      if (typeof u === 'string' && u) urls.push(u)
      // 数组形态：如 result_field 指到 "images"，leaf 是单个 image 对象也已处理；
      // 若 leaf 本身是数组，walk 上一层已经展开为多个 leaf，这里不再重复处理。
    }
  }
  return urls
}

/**
 * pickByPath：按路径从 payload 中取一个或多个原始值（不做 URL 提取）。
 * 与 extractUrlsByPath 的差别是这里返回任意类型的叶子节点，供上层
 * 按 OutputFieldSpec.type 决定如何渲染（url / text / json / number 等）。
 *
 * 语法与 extractUrlsByPath 保持一致，支持 "a.b[0].c" / "a[*].b" / "images"。
 */
export function pickByPath(payload: unknown, path: string): unknown[] {
  if (!payload || typeof payload !== 'object') return []
  const p = (path || '').trim()
  if (!p) return []

  const tokens: string[] = []
  for (const seg of p.split('.')) {
    if (!seg) continue
    const m = seg.matchAll(/([^[\]]+)|\[([^\]]+)\]/g)
    for (const g of m) {
      const name = g[1]
      const idx = g[2]
      if (name !== undefined) tokens.push(name)
      if (idx !== undefined) tokens.push(idx)
    }
  }

  function walk(node: unknown, ts: string[]): unknown[] {
    if (node === null || node === undefined) return []
    if (ts.length === 0) return [node]
    const [head, ...rest] = ts
    if (Array.isArray(node)) {
      if (head === '*') {
        const acc: unknown[] = []
        for (const el of node) acc.push(...walk(el, rest))
        return acc
      }
      const i = Number(head)
      if (Number.isInteger(i) && i >= 0 && i < node.length) {
        return walk(node[i], rest)
      }
      const acc: unknown[] = []
      for (const el of node) acc.push(...walk(el, ts))
      return acc
    }
    if (typeof node === 'object') {
      const rec = node as Record<string, unknown>
      if (head === '*') {
        const acc: unknown[] = []
        for (const v of Object.values(rec)) acc.push(...walk(v, rest))
        return acc
      }
      return walk(rec[head], rest)
    }
    return []
  }

  return walk(payload, tokens)
}
