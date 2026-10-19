/**
 * 爆款复刻：**逐张拆解参考图，一图一脚本**。
 *
 * 【2026-10-19 主人推翻了原来的做法】
 *   原话：「爆款复刻现在不能有套图的脚本，不能有任何的默认脚本，
 *   脚本必须来自与放入的图片的解析处理来的脚本，同时图片中的产品的所有信息
 *   都替换为本产品的信息且信息的位置不能变。生图的数量必须以放入的参考图的数量一样。」
 *
 * 于是删掉了这些**默认脚本**（它们跟放进去的图无关，等于凭空生成一套 Ozon 八股）：
 *   - `buildHotScript()`（AIDA 八张）
 *   - `buildHotExtraCards()`（额外四张：多角度 / 细节 / 使用 / FAQ）
 *   - `HOT_DEFAULTS`（写死的价格 `1990₽`、`已售 10 万+`、`7 天无理由`）
 *   - 九维度版式投票（`HOT_STYLE_DIMENSIONS` + `mergeStyleSamples`）——
 *     它把多张图**合并成一段文字描述**，结果是「几张图长得一样」，
 *     而且合并之后就说不清哪张图对应哪张出图了。
 *
 * 现在的做法：
 *   ① 每张参考图**单独**送一次视觉模型，同时把**我们自己的商品图**一起送过去；
 *   ② 模型返回这张图的**分块版式**（从上到下：区块位置 / 类型 / 字号样式 / 占比），
 *      并且**顺手把每一块的文案改写成我们商品的说法**（`text_ru`），
 *      原始文案放在 `source_ru` 里只作长度和位置的参照；
 *   ③ 一块参考图 = 一张脚本 = 一张出图。三张参考图就是三张脚本、三张图。
 *
 * 这样「脚本来自图片解析」「信息位置不变」「数量等于参考图数量」三条同时满足。
 */
import { FRAMING_SAFE_AREA, FRAMING_SAFE_AREA_EN } from './aiImagePrompts'
import type { OzonScriptCard } from './aiImagePrompts'

/** 拆解用温度：要的是稳定复现，不要发挥。 */
export const HOT_LAYOUT_TEMPERATURE = 0.2
/** 视觉请求超时（桌面程序是 90 秒；两张图一起送，留同样的余量）。 */
export const HOT_VISION_TIMEOUT_MS = 90000

/**
 * 拆解参考图的提示词。
 *
 * 关键点：同时喂两张图，并且**明确告诉模型哪张是「版式」哪张是「我们的商品」**，
 * 否则模型分不清该抄谁、该换谁。这是「信息位置不变 + 商品换成我们的」的前提。
 *
 * 要求 `text_ru`（给我们的商品重写的俄语）和 `source_ru`（同行原文）分开返回：
 *   - `text_ru` 会用进最终脚本，也就是会印在图上；
 *   - `source_ru` 只用来告诉生图模型「这一块大概多长、在哪个位置」，不会原样发出去。
 */
export const HOT_LAYOUT_PROMPT =
  'Ты топ e-commerce дизайнер на Ozon/Wildberries. На входе ДВЕ картинки:\n' +
  'изображение 1 — карточка конкурента (эталон вёрстки, её вёрстку нужно повторить);\n' +
  'изображение 2 — НАШ товар (именно его нужно вставить вместо товара конкурента).\n' +
  'Разбери вёрстку изображения 1 по блокам и верни СТРОГО JSON без пояснений:\n' +
  '{"canvas":"соотношение сторон, например 3:4","background":"фон: цвет и характер (плоский/градиент/фото)",' +
  '"palette":["#RRGGBB","#RRGGBB"],"product_slot":"где стоит товар: зона кадра, размер в % кадра, ракурс",' +
  '"graphics":"иконки, стрелки, плашки, линии — что именно и где",' +
  '"notes":"другие важные особенности вёрстки","blocks":[{"zone":"top|upper|middle|lower|bottom",' +
  '"kind":"headline|subline|badge|icon-row|text|label|cta|product|footer",' +
  '"style":"размер/жирность/цвет/выравнивание/в сколько строк","size_pct":12,' +
  '"source_ru":"текст конкурента ДОСЛОВНО","text_ru":"НОВЫЙ текст по-русски для НАШЕГО товара"}]}\n' +
  'Правила:\n' +
  '- Порядок blocks — строго сверху вниз, как на изображении 1.\n' +
  '- size_pct — примерная доля высоты кадра, которую занимает блок (сумма всех ≈ 100).\n' +
  '- source_ru — дословно с изображения 1, нужен только как ориентир по длине и позиции.\n' +
  '- text_ru — перепиши под НАШ товар (изображение 2), сохранив примерно ту же длину и стиль; ' +
  'если блок не про товар (например «спасибо за покупку»), сохрани смысл, но не копируй бренд конкурента.\n' +
  '- ВЕСЬ текст на карточке — только по-русски. Никогда не пиши по-китайски.\n' +
  '- Не выдумывай блоков, которых нет на изображении 1, и не пропускай существующие.\n' +
  '- Не копируй название бренда, логотип, название магазина и водяные знаки конкурента.'

/** 一个版式区块（参考图上从上到下的一块内容）。 */
export interface HotLayoutBlock {
  /** 在画面里的纵向位置：top / upper / middle / lower / bottom */
  zone: string
  /** 这一块是什么：headline / subline / badge / icon-row / text / label / cta / product / footer */
  kind: string
  /** 字号、粗细、颜色、对齐、占几行 */
  style: string
  /** 占画面高度的百分比（估的） */
  sizePct: number
  /** 同行原图上那块文字，只作长度和位置的参照 */
  sourceRu: string
  /** **给我们商品重写的俄语**，这一段才是最终要印在图上、会写进脚本里的字 */
  textRu: string
}

/** 一张参考图拆出来的完整版式。 */
export interface HotLayout {
  /** 第几张参考图（从 1 开始），也决定它是第几张出图 */
  index: number
  canvas: string
  background: string
  palette: string[]
  productSlot: string
  graphics: string
  notes: string
  blocks: HotLayoutBlock[]
}

/**
 * 从模型回复里抠出 JSON。
 *
 * 桌面程序用的是贪婪正则 `\{.*\}`，只要回复里出现两段 JSON
 * （比如 Markdown 代码块里带示例），就会把从第一个 `{` 到最后一个 `}`
 * 的整段都抠出来，然后解析失败。
 * 这里改成**括号配平**：从第一个 `{` 开始数，找到真正配对的 `}`。
 * 顺便剥掉 ```json 代码块围栏。
 */
export function extractJsonObject(raw: string): Record<string, unknown> | null {
  if (!raw) return null
  // 先剥 Markdown 代码块围栏，免得围栏里的示例 JSON 被当成答案。
  const withoutFence = raw.replace(/```(?:json)?/gi, '')
  const start = withoutFence.indexOf('{')
  if (start < 0) return null

  let depth = 0
  let inString = false
  let escaped = false

  for (let index = start; index < withoutFence.length; index += 1) {
    const char = withoutFence[index]

    if (escaped) {
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    // 字符串里的花括号不算括号，否则 {"a":"{"} 会被误判。
    if (char === '"') {
      inString = !inString
      continue
    }
    if (inString) continue

    if (char === '{') depth += 1
    else if (char === '}') {
      depth -= 1
      if (depth === 0) {
        try {
          const parsed: unknown = JSON.parse(withoutFence.slice(start, index + 1))
          return parsed && typeof parsed === 'object' && !Array.isArray(parsed)
            ? (parsed as Record<string, unknown>)
            : null
        } catch {
          return null
        }
      }
    }
  }

  return null
}

/** 取字符串字段，兼容模型换着写法给键名。 */
function pickString(source: Record<string, unknown>, keys: string[]): string {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === 'string' && value.trim()) return value.trim()
    if (typeof value === 'number') return String(value)
  }
  return ''
}

/**
 * 取数组字段。
 * 模型经常把 `blocks` 写成 `sections` / `layout` / `elements`，都要认。
 */
function pickArray(source: Record<string, unknown>, keys: string[]): unknown[] {
  for (const key of keys) {
    const value = source[key]
    if (Array.isArray(value) && value.length) return value
  }
  return []
}

/**
 * 把模型返回的 JSON 规整成 `HotLayout`。
 *
 * **必须宽容**：视觉模型给键名不稳定（`zone`/`pos`、`text_ru`/`text`），
 * 只要有一处对不上就整张丢弃的话，用户会看到「一张都拆不出来」而不知道为什么。
 * 只有「一个区块都没有」才算失败 —— 那说明它真的没看懂图。
 */
export function parseHotLayout(raw: string, index: number): HotLayout | null {
  const parsed = extractJsonObject(raw)
  if (!parsed) return null

  const rawBlocks = pickArray(parsed, ['blocks', 'sections', 'layout', 'elements', 'items'])
  const blocks: HotLayoutBlock[] = []
  for (const item of rawBlocks) {
    if (!item || typeof item !== 'object' || Array.isArray(item)) continue
    const block = item as Record<string, unknown>
    const textRu = pickString(block, ['text_ru', 'textRu', 'ours_ru', 'new_ru', 'text'])
    const sourceRu = pickString(block, ['source_ru', 'sourceRu', 'source', 'original', 'origin_ru', 'origin'])
    // 两个都没有的区块没有意义（既不知道要写什么，也不知道原来多长）。
    if (!textRu && !sourceRu) continue

    const sizeRaw = block.size_pct ?? block.sizePct ?? block.height_pct ?? block.heightPct
    const sizePct = typeof sizeRaw === 'number' ? sizeRaw : Number.parseFloat(String(sizeRaw ?? ''))
    blocks.push({
      zone: pickString(block, ['zone', 'pos', 'position', 'area', 'place']),
      kind: pickString(block, ['kind', 'type', 'role', 'block']),
      style: pickString(block, ['style', 'typography', 'font']),
      sizePct: Number.isFinite(sizePct) ? Math.max(0, Math.min(100, sizePct)) : 0,
      sourceRu,
      // 模型漏了改写时退回原文：位置和长度至少是对的，比空着强。
      textRu: textRu || sourceRu,
    })
  }

  if (!blocks.length) return null

  const paletteRaw = parsed.palette
  const palette = Array.isArray(paletteRaw)
    ? paletteRaw.map((item) => String(item || '').trim()).filter(Boolean)
    : []

  return {
    index,
    canvas: pickString(parsed, ['canvas', 'ratio', 'aspect', 'size']),
    background: pickString(parsed, ['background', 'bg']),
    palette,
    productSlot: pickString(parsed, ['product_slot', 'productSlot', 'product', 'product_zone']),
    graphics: pickString(parsed, ['graphics', 'decor', 'decorations', 'icons']),
    notes: pickString(parsed, ['notes', 'note', 'remarks']),
    blocks,
  }
}

/** 区块位置的中文说法（给界面看的中文提示词用）。 */
const ZONE_LABELS: Record<string, string> = {
  top: '顶部',
  upper: '上中',
  middle: '中部',
  lower: '下中',
  bottom: '底部',
}

/** 区块类型的中文说法。 */
const KIND_LABELS: Record<string, string> = {
  headline: '大标题',
  subline: '副标题',
  badge: '角标 / 贴纸',
  'icon-row': '图标行',
  text: '正文',
  label: '小标注',
  cta: '行动按钮',
  product: '商品本体',
  footer: '页脚',
}

function zoneLabel(zone: string): string {
  return ZONE_LABELS[zone.toLowerCase()] || zone || '位置未说明'
}

function kindLabel(kind: string): string {
  return KIND_LABELS[kind.toLowerCase()] || kind || '文字块'
}

/** 把一张拆解结果拼成中文脚本（界面显示 / 用户可改的那一份）。 */
function layoutCardBodyZh(layout: HotLayout): string {
  const lines = layout.blocks.map((block, i) => {
    const parts = [`${i + 1}) ${zoneLabel(block.zone)}｜${kindLabel(block.kind)}`]
    if (block.style) parts.push(`样式：${block.style}`)
    if (block.sizePct > 0) parts.push(`约占高度 ${block.sizePct}%`)
    parts.push(`俄语文字：«${block.textRu}»`)
    return parts.join('｜') + '。'
  })

  const meta: string[] = []
  if (layout.canvas) meta.push(`画幅：${layout.canvas}。`)
  if (layout.background) meta.push(`背景：${layout.background}。`)
  if (layout.palette.length) meta.push(`配色：${layout.palette.join(' / ')}。`)
  if (layout.productSlot) meta.push(`商品位置：${layout.productSlot}。`)
  if (layout.graphics) meta.push(`图形元素：${layout.graphics}。`)

  return (
    `第${layout.index}张：照抄第 ${layout.index} 张参考图的版式，只把商品和文字换成我们的。` +
    meta.join('') +
    '版式分区（从上到下，**顺序 / 位置 / 占比 / 字号层级全部照抄，不许改**）：' +
    lines.join('') +
    (layout.notes ? `其他要点：${layout.notes}。` : '') +
    '硬规则：参考图上有几个文字块就画几个，不许合并、不许新增、不许漏；' +
    '所有画面文字必须是俄语并且必须真的写出来，内容就用上面每一条给的俄语，不要自行改写、不要译成中文；' +
    '不要出现参考图里的品牌名、店铺名、logo 和水印。'
  )
}

/** 拼成英文脚本（真正发给模型的那一份）。 */
function layoutCardBodyEn(layout: HotLayout): string {
  const lines = layout.blocks.map((block, i) => {
    const parts = [`${i + 1}) zone ${block.zone || 'unspecified'}, kind ${block.kind || 'text'}`]
    if (block.style) parts.push(`style: ${block.style}`)
    if (block.sizePct > 0) parts.push(`about ${block.sizePct}% of the height`)
    parts.push(`Russian text: "${block.textRu}"`)
    return parts.join(' | ') + '. '
  })

  const meta: string[] = []
  if (layout.canvas) meta.push(`Canvas: ${layout.canvas}. `)
  if (layout.background) meta.push(`Background: ${layout.background}. `)
  if (layout.palette.length) meta.push(`Palette: ${layout.palette.join(' / ')}. `)
  if (layout.productSlot) meta.push(`Product placement: ${layout.productSlot}. `)
  if (layout.graphics) meta.push(`Graphic elements: ${layout.graphics}. `)

  return (
    `Card ${layout.index}: clone the layout of reference image ${layout.index}, replacing only the product and the wording with ours. ` +
    meta.join('') +
    'Layout blocks from top to bottom (**keep the order, position, proportion and type hierarchy exactly as the reference; do not change them**): ' +
    lines.join('') +
    (layout.notes ? `Other notes: ${layout.notes}. ` : '') +
    'Hard rules: as many text blocks as the reference has — do not merge, do not add, do not skip any; ' +
    'every on-image text must be Russian and must actually be rendered, using exactly the Russian given above; do not rewrite it and never translate it into Chinese; ' +
    'never reproduce the competitor brand name, shop name, logo or watermarks.'
  )
}

/**
 * 一个版式 → 一张脚本卡片。
 *
 * `index` 直接决定「第几张出图」，所以顺序必须和参考图的顺序一致 ——
 * 主人的要求是「生图的数量必须以放入的参考图的数量一样」。
 */
export function buildHotLayoutCards(layouts: HotLayout[]): OzonScriptCard[] {
  return layouts.map((layout, position) => ({
    index: position + 1,
    key: `hot_${String(position + 1).padStart(2, '0')}_ref${layout.index}`,
    title: `第 ${position + 1} 张 · 复刻参考图 ${layout.index}`,
    prompt: `${FRAMING_SAFE_AREA}${layoutCardBodyZh(layout)}`,
    promptEn: `${FRAMING_SAFE_AREA_EN}${layoutCardBodyEn(layout)}`,
  }))
}
