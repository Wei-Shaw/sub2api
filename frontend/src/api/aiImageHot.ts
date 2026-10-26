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
import { FRAMING_SAFE_AREA, FRAMING_SAFE_AREA_EN, buildProductPalette } from './aiImagePrompts'
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
  '"notes":"другие важные особенности вёрстки",' +
  // 顺手读一下「НАШ товар（изображение 2）」上到底印着什么品牌/字样。
  // 主人 2026-10-26 问「不能识别我放入的产品上的品牌吗」——能，就在同一次请求里读。
  // 背景换色要用的两个字段（2026-10-26 主人：背景要跟我们的商品配色，
  // 但文字/图标/胶囊条保持参考图原样，不做同色化）。
  '"bg_color_zone":"какая часть фона — это однотонная стена/градиент/крупная заливка (её цвет надо заменить), ' +
  'и есть ли в фоне САМ ТОВАР КОНКУРЕНТА (например стена выложена плиткой конкурента). Если фон — обычное фото ' +
  '(окно, мебель, растения), так и напиши: фото, не перекрашивать",' +
  '"our_product_color":"главный цвет НАШЕГО товара (изображение 2) по-китайски, 2-4 иероглифа, например 浅棕 / 酒红 / 米白",' +
  '"our_product_color_hex":"главный цвет НАШЕГО товара в формате #RRGGBB",' +
  '"our_brand":"что за бренд/логотип/надпись видно НА НАШЕМ ТОВАРЕ (изображение 2): сам текст как есть. ' +
  'Если на товаре и на фото нашего товара никаких брендов/логотипов/водяных знаков не видно — верни пустую строку. ' +
  'Не придумывай бренд, которого не видно",' +
  '"forbidden_marks":"перечисли через ; всё, по чему можно узнать ТОВАР ИЛИ МАГАЗИН КОНКУРЕНТА и что переносить НЕЛЬЗЯ: ' +
  'бренд, логотип, название магазина, водяные знаки, штрихкоды, артикулы, а также надписи/тиснение/бирки/уникальный узор НА САМОМ ТОВАРЕ — и где именно они стоят",' +
  '"blocks":[{"zone":"top|upper|middle|lower|bottom",' +
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
  '- Не копируй название бренда, логотип, название магазина и водяные знаки конкурента.\n' +
  '- our_brand бери ТОЛЬКО со второго изображения (наш товар). Никогда не подставляй туда бренд конкурента. ' +
  '- Если на нашем товаре виден водяной знак поставщика/магазина — всё равно верни его в our_brand как есть, ' +
  'его всё равно перепроверит человек.' +
  '- В forbidden_marks обязательно перечисли ВСЁ, что выдаёт товар конкурента: логотип на самом товаре, ' +
  'выдавленную надпись, бирку, характерный узор, артикул, водяной знак — и укажи зону, где это стоит.'

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
  /**
   * 参考图上「能认出对方商品/店铺」的东西（品牌、logo、水印、商品上的印字、吊牌…），
   * 以及它们所在的位置。
   *
   * 【2026-10-26 主人】「消除产品的特有信息是必须的」——
   * 光靠一句「别抄品牌」不够，必须先把**具体是哪几处**列出来，再逐条要求清掉。
   */
  forbiddenMarks: string
  /**
   * 从**我们自己那张商品图**上读到的品牌 / logo / 字样（原样返回）。
   *
   * 【2026-10-26 主人问「不能识别我放入的产品上的品牌吗」】
   * 能，就在同一次视觉请求里读。但要注意：商品图上经常是**供应商的水印**，
   * 不是我们自己的品牌（那 5 组里，棕榈树上是「锦和」、包上是 huagingone）。
   * 所以读出来只用来**预填**，界面上必须可见可改。
   */
  ourBrand: string
  /** 背景里「应该被换色」的区域说明（纯色墙 / 渐变 / 大面积填色 / 背景里出现竞品商品本身）。 */
  bgColorZone: string
  /** 我们商品主色的中文叫法 */
  ourColorName: string
  /** 我们商品主色的 #RRGGBB */
  ourColorHex: string
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
    forbiddenMarks: pickString(parsed, [
      'forbidden_marks',
      'forbiddenMarks',
      'must_remove',
      'mustRemove',
      'competitor_marks',
    ]),
    ourBrand: pickString(parsed, ['our_brand', 'ourBrand', 'our_product_brand', 'own_brand']),
    bgColorZone: pickString(parsed, ['bg_color_zone', 'bgColorZone', 'background_zone', 'backgroundZone']),
    ourColorName: pickString(parsed, ['our_product_color', 'ourProductColor', 'our_color', 'ourColor']),
    ourColorHex: pickString(parsed, [
      'our_product_color_hex',
      'ourProductColorHex',
      'our_color_hex',
      'ourColorHex',
    ]),
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
function layoutCardBodyZh(layout: HotLayout, brandName = ''): string {
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
    forbiddenBlockZh(layout, brandName) +
    bgRecolorZh(layout)
  )
}

/**
 * 「只把背景换成我们商品的色系」这一段（中文版）。
 *
 * 【2026-10-26 主人】「先解决商品配色与背景配色，爆款复刻不需要图片里的一些信息
 * 与商品的颜色一致或相仿」。
 *
 * 拆成两句是因为真图暴露了两个问题：
 *   ① 瓷砖那组：我们的是棕色瓷砖，背景墙却还是竞品的酒红色 → **背景必须跟我们的商品**
 *   ② 但文字颜色、图标、胶囊条是版式的一部分，跟着变就不是复刻了 → **其它一律不动**
 */
function bgRecolorZh(layout: HotLayout): string {
  const zone = (layout.bgColorZone || '').trim()
  const palette = buildProductPalette(layout.ourColorName, layout.ourColorHex)
  const colorLine = palette
    ? `我们商品的主色是 ${palette.name}（${palette.hex}），背景就从 ${palette.tint}（浅）到 ${palette.accent}（深）这个区间里取。`
    : (layout.ourColorName ? `我们商品的主色是 ${layout.ourColorName}。` : '')
  return (
    '**背景换色（只换背景，别的一律不动）**：' +
    (zone ? `参考图里需要换色的背景区域是：${zone}。` : '') +
    colorLine +
    '这些背景区域的颜色要换成**我们商品主色的同色系**，不要保留竞品商品的颜色。' +
    '如果背景里出现的其实就是竞品商品本身（例如背景墙面就是竞品的瓷砖、板材、布料），必须一并换成我们商品的色系。' +
    '如果这块背景是一张实景照片（窗、家具、植物、墙面材质），**保留照片内容**，只把整体色调往我们商品的色系偏一点，不要重画背景。' +
    '**但下面这些必须保持参考图原样、绝对不要跟着变色**：文字颜色、图标颜色与描边、' +
    '胶囊条和角标的底色、边框与分隔线、按钮底色。' +
    '一句话：版式配色照抄竞品，**只有背景色换成我们的**。'
  )
}

/** 同上的英文版。 */
function bgRecolorEn(layout: HotLayout): string {
  const zone = (layout.bgColorZone || '').trim()
  const palette = buildProductPalette(layout.ourColorName, layout.ourColorHex)
  const colorLine = palette
    ? `Our product's dominant colour is ${palette.name} (${palette.hex}); pick the background within the range from ${palette.tint} (pale) to ${palette.accent} (deep). `
    : (layout.ourColorName ? `Our product's dominant colour is ${layout.ourColorName}. ` : '')
  return (
    '**Background recolour (background only — nothing else changes)**: ' +
    (zone ? `The background areas that need recolouring are: ${zone}. ` : '') +
    colorLine +
    'Recolour those background areas into **our product\'s hue family**; never keep the competitor product\'s colour. ' +
    'If the background literally shows the competitor product itself (for example a wall clad in the competitor\'s tiles, boards or fabric), it must be recoloured into our product\'s family too. ' +
    'If that background is a real photograph (window, furniture, plants, wall material), **keep the photo content** and only push the overall cast toward our product colour — do not repaint the scene. ' +
    '**Everything listed here must stay exactly as in the reference and must NOT be recoloured**: text colours, icon colours and strokes, ' +
    'pill and badge fill colours, borders and dividers, button fills. ' +
    'In short: the layout palette is the competitor\'s; **only the background colour becomes ours**.'
  )
}

/**
 * 「必须消除对方商品的特有信息」这一段（中文版）。
 *
 * 【2026-10-26 主人】「消除产品的特有信息是必须的」。
 * 只写一句「别抄品牌」不顶用——模型会留半透明水印（豆包那 5 组里
 * Royal Trees / STARLEK / UPCLOUD 全都留下了）。所以这里：
 *   1. 把视觉模型拆出来的**具体那几处**列出来（forbiddenMarks）
 *   2. 补一份兜底清单（品牌/店名/logo/水印/二维码/型号/商品上的印字…）
 *   3. 给了我们的品牌名，就要求在原位置换成我们的；没给就留白
 *   4. 留一句「出现就算废片」，把这条的优先级抬到最高
 */
function forbiddenBlockZh(layout: HotLayout, brandName: string): string {
  const marks = (layout.forbiddenMarks || '').trim()
  const brand = brandName.trim()
  return (
    '**必须消除对方商品的特有信息（这一条压过其它所有要求）**：' +
    (marks ? `参考图上能认出对方商品或店铺的具体是：${marks}。` : '') +
    '这些**一处都不许出现在成品里**，另外还包括：品牌名、店铺名、logo、水印、二维码、型号、' +
    '商品本体上印的字 / 烫印 / 雕刻 / 吊牌 / 专属花纹、包装上的品牌。' +
    (brand
      ? `原来品牌或店名所在的位置，换成我们的品牌 «${brand}»，位置和字号照原位。`
      : '原来品牌或店名的位置直接留白或用同色块替代，不要填任何别的字。') +
    '如果那处本来是在展示卖点（比如商品上的 logo 特写），就用我们的商品在同一位置重新表现，不要照抄对方的图案。' +
    '**成品里只要出现对方任何品牌或标识，这一张就是废片。**'
  )
}

/** 同上的英文版（真正发给模型的那一份）。 */
function forbiddenBlockEn(layout: HotLayout, brandName: string): string {
  const marks = (layout.forbiddenMarks || '').trim()
  const brand = brandName.trim()
  return (
    '**You must remove every identifying trace of the competitor product (this overrides all other rules)**: ' +
    (marks ? `The specific giveaway marks on the reference are: ${marks}. ` : '') +
    'none of them may appear in the output. That also covers: brand name, shop name, logo, watermark, QR code, SKU code, ' +
    'any print / emboss / engraving / hang tag / signature pattern on the product itself, and any branding on the packaging. ' +
    (brand
      ? `Wherever the competitor brand or shop name stood, put our brand «${brand}» instead, in the same place at the same size. `
      : 'Where the competitor brand or shop name stood, leave it blank or cover it with a matching colour block — never fill it with other words. ') +
    'If that spot was itself a selling point (for example a close-up of a logo on the product), re-show it with OUR product at the same position instead of copying their graphic. ' +
    '**If any competitor brand or mark appears in the result, the image is a write-off.**'
  )
}

/** 拼成英文脚本（真正发给模型的那一份）。 */
function layoutCardBodyEn(layout: HotLayout, brandName = ''): string {
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
    forbiddenBlockEn(layout, brandName) +
    bgRecolorEn(layout)
  )
}

/**
 * 一个版式 → 一张脚本卡片。
 *
 * `index` 直接决定「第几张出图」，所以顺序必须和参考图的顺序一致 ——
 * 主人的要求是「生图的数量必须以放入的参考图的数量一样」。
 */
export function buildHotLayoutCards(layouts: HotLayout[], brandName = ''): OzonScriptCard[] {
  const brand = brandName.trim()
  return layouts.map((layout, position) => ({
    index: position + 1,
    key: `hot_${String(position + 1).padStart(2, '0')}_ref${layout.index}`,
    title: `第 ${position + 1} 张 · 复刻参考图 ${layout.index}`,
    prompt: `${FRAMING_SAFE_AREA}${layoutCardBodyZh(layout, brand)}`,
    promptEn: `${FRAMING_SAFE_AREA_EN}${layoutCardBodyEn(layout, brand)}`,
  }))
}
