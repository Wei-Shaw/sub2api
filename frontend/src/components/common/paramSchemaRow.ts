/**
 * paramSchemaRow.ts
 *
 * 递归 JSON Schema 编辑器的数据模型与序列化工具。ParamSchemaEditor.vue
 * 及 AdminModelIntrosView.vue 共享同一份类型与函数，避免各处再手写重复逻辑。
 *
 * 数据流：
 *
 *   default_params (存储 shape)  ⇄  SchemaRow (前端编辑器状态)
 *          rowsToMap                       mapToRows
 *
 * 存储 shape（前后端一致）：
 *   叶子（string/number/boolean）：
 *     { value, required?, description?, enum?, options? }
 *   object：
 *     { properties: { <k>: <schema>, ... }, required?, description? }
 *   array（同构）：
 *     { items: <schema>, required?, description? }
 *
 * 前端 SchemaRow 是一棵可编辑树，每个节点带 key/type + 各类型专属字段，
 * 复合类型 (object/array) 递归到 children/items。
 */

// SchemaRowType：编辑器支持的类型。
export type SchemaRowType = 'string' | 'number' | 'boolean' | 'object' | 'array'

/**
 * SchemaWidget：控件形态声明，决定演练页/试一试渲染时用哪种控件。
 *
 * 适用类型分两组，互不混用：
 *   - string 叶子：'input'（默认）| 'textarea'（配 rows）| 'image'（单张图片输入）
 *   - array：'input'（默认，逐元素递归渲染）| 'imageUrls'（整组图片输入，
 *     值为图片完整 URL 的字符串数组，演练台渲染成一个图库式的多图输入区）
 *
 * 存储侧：默认 'input' 时不写该字段，以保持存储 shape 精简与向后兼容；
 * 其余取值会持久化（'textarea' 时连同 rows 一起写）。
 */
export type SchemaWidget = 'input' | 'textarea' | 'image' | 'imageUrls'

/** textarea 行数默认值（rows 属性缺省时使用）。 */
export const DEFAULT_TEXTAREA_ROWS = 3

/**
 * SchemaRow：编辑器每个节点（顶层字段 or 子字段）的表单状态。
 *
 * 类型专属字段：
 *   - string / number：value（文本表达；number 序列化时 Number() 转回）
 *   - boolean：boolValue
 *   - object：children （子字段列表）
 *   - array：items（元素 schema，唯一一份；数组同构）+ maxItems（元素个数上限）
 *
 * 通用元数据：required / description / isEnum + optionsText（仅叶子）。
 *
 * 渲染元数据：
 *   - widget：string 叶子可选 input/textarea/image；array 可选 input/imageUrls
 *   - textareaRows：仅 widget='textarea' 有意义
 *   - maxItems：仅 array 有意义，限制演练台里最多能填几个元素
 *
 * uid 用于 v-for 稳定 key，避免元素乱序时 DOM 复用导致输入失焦。
 */
export interface SchemaRow {
  uid: number
  key: string
  type: SchemaRowType
  value: string
  boolValue: boolean
  required: boolean
  /**
   * 是否是“高级参数”。为 true 时，演练台会把该字段收纳到“高级参数”折叠区，
   * 默认不展开，避免铺满普通用户不常改的字段。
   * 存储侧：写到 spec.extra.advanced；只在 true 时写入，保持存储 shape 精简，
   * 也兼容不含该字段的老数据（读侧默认 false）。
   */
  advanced: boolean
  description: string
  /** 字段英文说明（可选）；存储时会写到底层 spec 的 description_en 键。
   *  渲染时前端会根据当前 i18n locale 选适当的语种；若目标语种为空则回退
   *  到另一个非空的，保证总能看到说明文字。 */
  descriptionEn: string
  isEnum: boolean
  optionsText: string
  /** string 叶子：input/textarea/image；array：input/imageUrls。默认 'input'。 */
  widget: SchemaWidget
  /** 仅 widget='textarea' 有意义。默认 DEFAULT_TEXTAREA_ROWS。 */
  textareaRows: number
  /**
   * 仅 type='array' 有意义：元素个数上限。0 表示不限制（默认，也不落库）。
   * 演练台会据此禁用"添加"按钮、并在提交前做一次校验。
   */
  maxItems: number
  children: SchemaRow[]
  items: SchemaRow | null
}

let __uidSeq = 1
export function nextSchemaRowUid(): number {
  return __uidSeq++
}

/**
 * makeSchemaRow：构造一个空 SchemaRow，可以选择性覆盖字段。默认 type='string'。
 */
export function makeSchemaRow(overrides: Partial<SchemaRow> = {}): SchemaRow {
  return {
    uid: nextSchemaRowUid(),
    key: overrides.key ?? '',
    type: overrides.type ?? 'string',
    value: overrides.value ?? '',
    boolValue: overrides.boolValue ?? false,
    required: overrides.required ?? false,
    advanced: overrides.advanced ?? false,
    description: overrides.description ?? '',
    descriptionEn: overrides.descriptionEn ?? '',
    isEnum: overrides.isEnum ?? false,
    optionsText: overrides.optionsText ?? '',
    widget: overrides.widget ?? 'input',
    textareaRows: overrides.textareaRows ?? DEFAULT_TEXTAREA_ROWS,
    maxItems: overrides.maxItems ?? 0,
    children: overrides.children ?? [],
    items: overrides.items ?? null,
  }
}

/**
 * resetRowForType：类型切换时，按目标类型初始化必要字段。
 * 保留其它可复用字段（例如 required / description）避免误清空。
 *
 *   - object：确保 children 存在（若已有则保留）；清 enum（object 不支持枚举）
 *   - array：确保 items 存在（默认一个 string 子 schema）；清 enum
 *   - 叶子：仅确保 value / boolValue 存在；不动 enum/options
 *
 * widget 的归位不在这里做（调用方 onTypeChange 负责），因为 string 与 array
 * 各有自己的候选集，需要结合"切换前后的类型"判断。
 */
export function resetRowForType(row: SchemaRow): void {
  switch (row.type) {
    case 'object':
      if (!row.children) row.children = []
      row.isEnum = false
      row.optionsText = ''
      break
    case 'array':
      if (!row.items) {
        row.items = makeSchemaRow({ key: '', type: 'string' })
      }
      row.isEnum = false
      row.optionsText = ''
      break
    case 'string':
    case 'number':
      // 保留 value 原有内容
      break
    case 'boolean':
      // 保留 boolValue 原有内容
      break
  }
}

/**
 * splitOptions：宽松解析枚举候选值输入框。支持换行 / 逗号 / 中文逗号 /
 * 顿号 / 分号 / 竖线 / Tab 混用。冒号不能拆（否则 "21:9" 会被切成两段）。
 */
function splitOptions(text: string): string[] {
  return text
    .split(/[\n,，、;；|\t]+/)
    .map((s) => s.trim())
    .filter((s) => s.length > 0)
}

/**
 * coerceOption：把 option 字符串按叶子类型还原。
 */
function coerceOption(s: string, type: SchemaRowType): unknown {
  switch (type) {
    case 'number': {
      const n = Number(s)
      return Number.isFinite(n) ? n : s
    }
    case 'boolean': {
      const low = s.toLowerCase()
      if (low === 'true') return true
      if (low === 'false') return false
      return s
    }
    default:
      return s
  }
}

/**
 * ORDER_STEP：字段排序步长。序列化时每个子字段按数组下标 * 10 写入 `extra['x-order']`。
 * 用 10 而非 1 是为了在未来支持“手动改 order 数字插入到中间”的场景下留出插槽。
 *
 * 存储位置：从早期的顶层 `x-order` 迁移到 `spec.extra['x-order']` 里，与新增的
 * `spec.extra.advanced` 并列，让所有“非标 JSON Schema 扩展字段”都集中在一个
 * 命名空间下，方便演练台 prompt 里做整体剥离（extra 里的东西对 AI 生成代码无意义）。
 * 读侧仍兼容顶层 `x-order`（旧数据），下次保存时自动迁移进 extra。
 */
const ORDER_STEP = 10

/**
 * writeExtra：把一组“非标扩展字段”统一挂到 spec.extra 下。
 *   - x-order：必写（保持字段顺序稳定）
 *   - advanced：仅 true 时写入（默认 false 不落库，减少 diff 噪声）
 * 若未来还有其它演练台/编辑器专用扩展字段，也应该在这里追加，而不是散落到顶层。
 */
function writeExtra(out: Record<string, unknown>, order: number, advanced: boolean): void {
  const extra: Record<string, unknown> = { 'x-order': order }
  if (advanced) extra.advanced = true
  out.extra = extra
}

/**
 * rowToSchema：把 SchemaRow 递归序列化为后端存储 shape。
 *   - 叶子：{ value, required?, description?, enum?, options? }
 *   - object：{ properties, required?, description? }
 *   - array：{ items, required?, description? }
 *
 * 子字段（object 内部）若 key 为空则跳过（避免生成 "" key）。
 *
 * 排序：object.properties 的每个子 spec 会带上 `x-order: 序号 * 10`，
 * 反序列化端按 x-order 升序还原编辑器里的字段顺序（Map 序 = 字母序，
 * 不能承载“管理员自定义顺序”，只能靠这个额外字段承载）。
 */
/**
 * writeDescription：把 SchemaRow 里 description（默认中文）+ descriptionEn
 * （英文可选）写到底层 spec 对象上。存储时只写非空值，避免历史无该字段的
 * 行升级后 diff 变噪。
 *
 *   - description   → 保持既有键名，兼容存量数据（历史都视为默认语言）
 *   - description_en → 仅当英文说明非空时写入，视为附加字段
 */
function writeDescription(out: Record<string, unknown>, row: SchemaRow): void {
  const desc = row.description.trim()
  if (desc) out.description = desc
  const descEn = row.descriptionEn.trim()
  if (descEn) out.description_en = descEn
}

export function rowToSchema(row: SchemaRow): Record<string, unknown> {
  const out: Record<string, unknown> = {}
  if (row.type === 'object') {
    const properties: Record<string, unknown> = {}
    let idx = 0
    for (const ch of row.children) {
      const k = (ch.key || '').trim()
      if (!k) continue
      const childSpec = rowToSchema(ch)
      // 注入排序号 + advanced：统一走 extra，避免污染 JSON Schema 顶层。
      writeExtra(childSpec as Record<string, unknown>, idx * ORDER_STEP, ch.advanced)
      properties[k] = childSpec
      idx++
    }
    out.properties = properties
    if (row.required) out.required = true
    writeDescription(out, row)
    return out
  }
  if (row.type === 'array') {
    // items 缺省时给一个默认 string schema，保证存储 shape 稳定。
    const items = row.items ?? makeSchemaRow({ key: '', type: 'string' })
    out.items = rowToSchema(items)
    if (row.required) out.required = true
    writeDescription(out, row)
    // widget='imageUrls'：整组图片输入（值为图片 URL 字符串数组）。
    // 默认 'input' 不写入，保持存储 shape 与旧数据一致。
    if (row.widget === 'imageUrls') out.widget = 'imageUrls'
    // maxItems：元素个数上限；<=0 视为不限制，不落库。
    const max = Number.isFinite(row.maxItems) ? Math.trunc(row.maxItems) : 0
    if (max > 0) out.maxItems = max
    return out
  }
  // 叶子：value + 元数据
  let value: unknown
  switch (row.type) {
    case 'boolean':
      value = row.boolValue
      break
    case 'number': {
      const n = Number(row.value)
      value = Number.isFinite(n) ? n : 0
      break
    }
    case 'string':
    default:
      value = row.value
      break
  }
  out.value = value
  if (row.required) out.required = true
  writeDescription(out, row)
  if (row.isEnum) {
    out.enum = true
    out.options = splitOptions(row.optionsText).map((s) => coerceOption(s, row.type))
  }
  // widget/rows 仅对 string 叶子有意义；widget='input' 视为默认，不写入以保持
  // 存储 shape 与旧数据一致（避免历史无该字段的行升级后 diff 变噪）。
  if (row.type === 'string') {
    if (row.widget === 'textarea') {
      out.widget = 'textarea'
      const r = Number.isFinite(row.textareaRows) ? Math.trunc(row.textareaRows) : DEFAULT_TEXTAREA_ROWS
      // 行数下限1、上限 20：既避免负数/0 导致 <textarea rows='0'> 塌陷，
      // 也避免手滑填 999 生成一个占满整屏的输入区。
      out.rows = Math.min(20, Math.max(1, r))
    } else if (row.widget === 'image') {
      out.widget = 'image'
    }
  }
  return out
}

/**
 * rowsToMap：把顶层 SchemaRow[] 序列化为 default_params map（{ key: schema }）。
 * key 为空的顶层行跳过。
 *
 * 同样为每个顶层 schema 注入 `x-order`：Go map JSON marshal 会按字母序输出，
 * 无法反映编辑器里的行顺序；反序列化端按 x-order 升序恢复。
 */
export function rowsToMap(rows: SchemaRow[]): Record<string, unknown> {
  const m: Record<string, unknown> = {}
  let idx = 0
  for (const r of rows) {
    const k = (r.key || '').trim()
    if (!k) continue
    const spec = rowToSchema(r)
    writeExtra(spec as Record<string, unknown>, idx * ORDER_STEP, r.advanced)
    m[k] = spec
    idx++
  }
  return m
}

/**
 * readXOrder：读取一个 spec 上的 `x-order` 字段（number）。
 *   - 新格式：spec.extra['x-order']（推荐）
 *   - 旧格式：spec['x-order']（老数据，兼容读；下次保存会自动迁移进 extra）
 * 若两者都缺失或非数字，返回 Number.POSITIVE_INFINITY，让排序端把此类字段挤到末尾。
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
 * readAdvanced：读取一个 spec 上的 “是否高级参数”。
 *   - 新格式：spec.extra.advanced（推荐）
 *   - 缺失或非 boolean → false
 * 旧数据没有此字段，全部当作普通（非高级）参数。
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
 * sortKeysByXOrder：把一个 map 的 key 列表按每个 value 的 `x-order` 升序排序。
 * order 相同或都缺时，回退到字母序（Object.keys 语义等价于原顺序）。
 */
function sortKeysByXOrder(m: Record<string, unknown>, keys: string[]): string[] {
  return keys.slice().sort((a, b) => {
    const oa = readXOrder(m[a])
    const ob = readXOrder(m[b])
    if (oa !== ob) return oa - ob
    return a < b ? -1 : a > b ? 1 : 0
  })
}

/**
 * schemaToRow：把存储 shape 递归反解为 SchemaRow。
 * 判定顺序：
 *   1) 含 properties 且非叶子标记 → object
 *   2) 含 items 且非叶子标记 → array
 *   3) 其它 → 叶子（按 value 的 JS 类型推断 string/number/boolean）
 *
 * 若 raw 不是对象（不含元数据），也做一次兜底：把 raw 当叶子默认值。
 *
 * 排序：object.properties 的子字段会按 `x-order` 升序解析，与 rowToSchema
 * 写入端保持对称。若旧数据不含 x-order 字段，会退化为字母序（Object.keys 顺序）。
 */
export function schemaToRow(key: string, raw: unknown): SchemaRow {
  if (raw && typeof raw === 'object' && !Array.isArray(raw)) {
    const obj = raw as Record<string, unknown>
    // object
    if ('properties' in obj) {
      const props = obj.properties as Record<string, unknown>
      const children: SchemaRow[] = []
      if (props && typeof props === 'object' && !Array.isArray(props)) {
        const sortedKeys = sortKeysByXOrder(props, Object.keys(props))
        for (const ck of sortedKeys) {
          children.push(schemaToRow(ck, props[ck]))
        }
      }
      return makeSchemaRow({
        key,
        type: 'object',
        required: obj.required === true,
        advanced: readAdvanced(obj),
        description: typeof obj.description === 'string' ? (obj.description as string) : '',
        descriptionEn: typeof obj.description_en === 'string' ? (obj.description_en as string) : '',
        children,
      })
    }
    // array
    if ('items' in obj) {
      const items = schemaToRow('', obj.items)
      // 强制 items.key 为空（数组元素无名）。
      items.key = ''
      // widget：array 仅认 'imageUrls'，其它一切（缺省 / 未知值）归一为 'input'。
      const arrWidget: SchemaWidget = obj.widget === 'imageUrls' ? 'imageUrls' : 'input'
      // maxItems：非法 / <=0 一律归零（不限制）。上限 100 防手滑填出天量输入框。
      const rawMax = Number(obj.maxItems)
      const maxItems =
        Number.isFinite(rawMax) && rawMax > 0 ? Math.min(100, Math.trunc(rawMax)) : 0
      return makeSchemaRow({
        key,
        type: 'array',
        required: obj.required === true,
        advanced: readAdvanced(obj),
        description: typeof obj.description === 'string' ? (obj.description as string) : '',
        descriptionEn: typeof obj.description_en === 'string' ? (obj.description_en as string) : '',
        widget: arrWidget,
        maxItems,
        items,
      })
    }
    // 叶子 spec
    const isLeafSpec =
      'value' in obj || 'required' in obj || 'description' in obj || 'enum' in obj || 'options' in obj
    if (isLeafSpec) {
      const inner = obj.value
      let type: SchemaRowType = 'string'
      let value = ''
      let boolValue = false
      if (typeof inner === 'boolean') {
        type = 'boolean'
        boolValue = inner
      } else if (typeof inner === 'number') {
        type = 'number'
        value = String(inner)
      } else if (typeof inner === 'string') {
        type = 'string'
        value = inner
      } else if (inner === undefined || inner === null) {
        type = 'string'
        value = ''
      } else {
        // 其它复合值（array / object）落到 string 兜底，方便管理员编辑；
        // 实际管理员应该用 object/array 类型声明而非塞在 value 里。
        type = 'string'
        try {
          value = JSON.stringify(inner)
        } catch {
          value = ''
        }
      }
      const optArr = Array.isArray(obj.options) ? (obj.options as unknown[]) : []
      const optionsText = optArr
        .map((o) =>
          typeof o === 'string'
            ? o
            : (() => {
                try {
                  return JSON.stringify(o)
                } catch {
                  return String(o)
                }
              })()
        )
        .join('\n')
      // widget / rows：只对 string 叶子有意义，且 widget 只接受 'textarea'/'image' 显式声明；
      // 其它一切情形（缺省 / 未知值 / 非 string 类型）都归一为 'input'。
      let widget: SchemaWidget = 'input'
      let textareaRows = DEFAULT_TEXTAREA_ROWS
      if (type === 'string') {
        if (obj.widget === 'textarea') {
          widget = 'textarea'
          const rr = Number(obj.rows)
          if (Number.isFinite(rr) && rr > 0) {
            textareaRows = Math.min(100, Math.max(1, Math.trunc(rr)))
          }
        } else if (obj.widget === 'image') {
          widget = 'image'
        }
      }
      return makeSchemaRow({
        key,
        type,
        value,
        boolValue,
        required: obj.required === true,
        advanced: readAdvanced(obj),
        description: typeof obj.description === 'string' ? (obj.description as string) : '',
        descriptionEn: typeof obj.description_en === 'string' ? (obj.description_en as string) : '',
        isEnum: obj.enum === true,
        optionsText,
        widget,
        textareaRows,
      })
    }
  }
  // 完全非结构化的原始值 → 当叶子默认值处理。
  return makeSchemaRow({ key, ...inferLeafFieldsFromPlain(raw) })
}

/**
 * inferLeafFieldsFromPlain：从裸值推断 type 与叶子字段。
 */
function inferLeafFieldsFromPlain(v: unknown): Partial<SchemaRow> {
  if (typeof v === 'boolean') return { type: 'boolean', boolValue: v }
  if (typeof v === 'number') return { type: 'number', value: String(v) }
  if (typeof v === 'string') return { type: 'string', value: v }
  if (v === null || v === undefined) return { type: 'string', value: '' }
  try {
    return { type: 'string', value: JSON.stringify(v) }
  } catch {
    return { type: 'string', value: '' }
  }
}

/**
 * mapToRows：把 default_params map 反解为顶层 SchemaRow[]。
 * 按每个 value 的 `x-order` 升序恢复；旧数据（无 order）自动退化为字母序。
 */
export function mapToRows(m: Record<string, unknown> | null | undefined): SchemaRow[] {
  if (!m || typeof m !== 'object') return []
  const sortedKeys = sortKeysByXOrder(m as Record<string, unknown>, Object.keys(m))
  return sortedKeys.map((k) => schemaToRow(k, m[k]))
}
