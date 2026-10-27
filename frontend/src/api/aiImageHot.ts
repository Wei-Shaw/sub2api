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
import {
  FRAMING_SAFE_AREA,
  FRAMING_SAFE_AREA_EN,
  buildProductPalette,
  describeHumanScale,
  parseSizeCm,
} from './aiImagePrompts'
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
  // 2026-10-27 主人选方案 A：实景照片背景不要换色。
  // 但「背景里就是竞品商品本身」必须换（例如厨房墙面就是竞品瓷砖），所以拆成两个布尔，
  // 由代码判断要不要换色，而不是让模型自己含糊处理。
  // 2026-10-27 主人：「使用类的在背景方面以参考图的背景为主」。
  // 三类商品的背景要求不同，所以先让模型判断这件商品属于哪一类。
  // 这个字段以后也能直接复用到套图的模板判别。
  '"product_class":"одно из трёх: use | decor | storage. ' +
  'use — товар что-то ДЕЛАЕТ или им пользуются, нужен человек/животное/действие ' +
  '(когтеточка, инструмент, техника, мебель, которой сидят/пользуются); ' +
  'decor — товар только СОЗЕРЦАЮТ, им не пользуются (искусственные растения, декор, посуда, светильник); ' +
  'storage — товар что-то ХРАНИТ или ВМЕЩАЕТ (контейнер, органайзер, мусорное ведро, полка, ящик)",' +
  '"bg_is_photo":"true или false — фон это НАСТОЯЩАЯ фотография сцены (окно, мебель, растения, ' +
  'фактура стены, улица), а не однотонная заливка и не градиент",' +
  '"bg_has_competitor_product":"true или false — видно ли НА САМОМ ФОНЕ товар конкурента ' +
  '(например стена выложена его плиткой, на столе стоит его товар, на человеке его одежда)",' +
  '"bg_color_zone":"какая часть фона — это однотонная стена/градиент/крупная заливка (её цвет надо заменить), ' +
  'и есть ли в фоне САМ ТОВАР КОНКУРЕНТА (например стена выложена плиткой конкурента). Если фон — обычное фото ' +
  '(окно, мебель, растения), так и напиши: фото, не перекрашивать",' +
  '"our_product_color":"главный цвет НАШЕГО товара (изображение 2) по-китайски, 2-4 иероглифа, например 浅棕 / 酒红 / 米白",' +
  '"our_product_color_hex":"главный цвет НАШЕГО товара в формате #RRGGBB",' +
  // 商品主体结构清单（2026-10-27 主人：仿生类改完之后必须和原图主体一致，不许增删）。
  // 只让模型「数」它真看得见的东西；数不清就写大概，不许编。
  '"structure_lock":"опиши СТРУКТУРУ НАШЕГО ТОВАРА (изображение 2) так, чтобы по описанию можно было проверить ' +
  'каждый элемент: сколько стеблей/веток, сколько пучков листьев или цветов, форма и цвет горшка/основания, ' +
  'что ещё входит в комплект. Для растений обязательно посчитай: количество стеблей и примерное количество листьев ' +
  'или листовых пучков. Если точное число не видно — напиши «примерно N» и опиши плотность. НЕ придумывай элементов, ' +
  'которых не видно",' +
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
  '- structure_lock заполняй по ВТОРОМУ изображению (наш товар), а не по карточке конкурента. ' +
  '- Это описание будет использовано как «что нельзя менять»: если перечислишь лишнее, модель нарисует лишнее. ' +
  'Поэтому пиши только то, что реально видно.' +
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
  /**
   * 我们商品的结构清单（茎数 / 叶数 / 盆形色 / 配件）。
   *
   * 【2026-10-27 主人】「一些仿生类的需要做到改之后的商品主体与放入产品图片的主体一致，
   * 不能多一些东西少一些东西」。仿生植物是重灾区：模型会把叶子画多、把盆换成更高档的，
   * 结果买家到手觉得和图上不一样。
   */
  structureLock: string
  /** 背景里「应该被换色」的区域说明（纯色墙 / 渐变 / 大面积填色 / 背景里出现竞品商品本身）。 */
  bgColorZone: string
  /**
   * 背景是不是**实景照片**（窗、家具、植物、墙面材质）。
   *
   * 【2026-10-27 主人选方案 A】实景照片的背景**不换色**，只允许极轻微的整体色温偏移。
   * 原因：拿棕榈树那组真图验证时，米色墙被推成了绿墙——偏离原图太多。
   * 竞品花过钱拍的实景照，保住它比强行同色系更安全。
   */
  /**
   * 商品类别（视觉模型判断）：use / decor / storage。
   *
   * 【2026-10-27 主人】「使用类的在背景方面以参考图的背景为主」——
   * 三类商品的背景要求不一样，所以要带上这个字段做分支。
   * 空字符串表示判断不出来，按最保守的「照参考图」处理。
   */
  productClass: "use" | "decor" | "storage" | ""
  bgIsPhoto: boolean
  /**
   * 背景里有没有**竞品商品本身**（例如厨房墙面就是竞品的瓷砖）。
   *
   * 这一条优先级高于 bgIsPhoto：即使背景是实景照片，
   * 只要画面里大面积是竞品商品，就必须换成我们商品的色系，否则等于给竞品做广告。
   */
  bgHasCompetitorProduct: boolean
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
/**
 * 把模型回复里的「是/否」读成真正的布尔。
 *
 * 视觉模型经常把布尔写成字符串（"true" / "да" / "есть" / "1"），
 * 直接 Boolean("false") 会得到 true —— 于是「照片背景不换色」这条就失效了。
 * 所以这里只认明确的肯定词，其余一律当 false（**默认不换色，宁可保守**）。
 */
/**
 * 把模型返回的商品类别归一到三个值。
 *
 * 视觉模型可能写中文、俄文或英文（use / decor / storage / использование / 收纳…），
 * 所以这里按关键词认，认不出来返回空串——**空串按「照参考图」处理，最保守**。
 */
function normalizeProductClass(raw: string): "use" | "decor" | "storage" | "" {
  const text = (raw || "").trim().toLowerCase()
  if (!text) return ""
  if (/storage|хране|органайз|контейнер|мусор|хранени/.test(text)) return "storage"
  if (/storage|收纳|放置|储藏/.test(text)) return "storage"
  if (/use|использ|функц|действ|使用|功能/.test(text)) return "use"
  if (/decor|декор|созерц|观赏|装饰|摆放/.test(text)) return "decor"
  return ""
}

function pickBool(source: Record<string, unknown>, keys: string[]): boolean {
  for (const key of keys) {
    const value = source[key]
    if (typeof value === "boolean") return value
    if (typeof value === "number") return value !== 0
    if (typeof value === "string") {
      const text = value.trim().toLowerCase()
      if (["true", "yes", "да", "есть", "1", "y"].includes(text)) return true
      if (["false", "no", "нет", "0", "n", ""].includes(text)) return false
      // 写成一整句话时，看开头是不是否定
      if (text.startsWith("нет") || text.startsWith("no") || text.startsWith("false")) return false
      if (text.startsWith("да") || text.startsWith("yes") || text.startsWith("true")) return true
      return false
    }
  }
  return false
}

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
    structureLock: pickString(parsed, [
      'structure_lock',
      'structureLock',
      'our_structure',
      'ourStructure',
      'product_structure',
    ]),
    bgColorZone: pickString(parsed, ['bg_color_zone', 'bgColorZone', 'background_zone', 'backgroundZone']),
    productClass: normalizeProductClass(
      pickString(parsed, ["product_class", "productClass", "class", "category"]),
    ),
    bgIsPhoto: pickBool(parsed, ["bg_is_photo", "bgIsPhoto", "background_is_photo"]),
    bgHasCompetitorProduct: pickBool(parsed, [
      "bg_has_competitor_product",
      "bgHasCompetitorProduct",
      "background_has_competitor_product",
    ]),
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
function layoutCardBodyZh(layout: HotLayout, brandName = '', productSize = ''): string {
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
    sizeBlockZh(productSize) +
    productFidelityZh(layout) +
    bgRecolorZh(layout)
  )
}

/**
 * 「尺寸数字必须用真实尺寸」这一段（中文版）。
 *
 * 【2026-10-27 主人】「观赏、使用、放置这几类产品都检查一下」。
 * 检查出来的洞：爆款复刻**没有尺寸输入**，卡片上的尺寸数字只能靠模型从商品图上读。
 * 棕榈树那组能对上，是因为商品图上恰好印着「高：90cm」；
 * 换成猫爬架（620mm）或垃圾桶（40cm）这种图上没印数字的，模型就会自己编一个。
 * 所以补一个输入框，并把「人体参照」一起写死。
 */
function sizeBlockZh(productSize: string): string {
  const raw = productSize.trim()
  const cm = parseSizeCm(raw)
  const scale = cm ? describeHumanScale(cm) : null
  if (!raw) return ""
  return (
    "**尺寸必须用这个真实值**：" +
    `这件商品的实际尺寸是 ${raw}。` +
    (scale ? scale.zh : "") +
    "参考图上如果写着别的尺寸数字，**必须改掉换成这个**；图上没有尺寸数字就不要自己加一个。" +
    "画面里商品和人物、动物、家具的大小关系也必须按这个尺寸来，宁可让商品占满画面，也不许为了构图好看把它缩小。"
  )
}

/** 同上的英文版。 */
function sizeBlockEn(productSize: string): string {
  const raw = productSize.trim()
  const cm = parseSizeCm(raw)
  const scale = cm ? describeHumanScale(cm) : null
  if (!raw) return ""
  return (
    "**Use this real size**: " +
    `the actual size of this product is ${raw}. ` +
    (scale ? scale.en + " " : "") +
    "If the reference shows a different size number, **replace it with this one**; if it shows none, do not invent one. " +
    "The product-to-person, product-to-animal and product-to-furniture scale must follow this size too — better to fill the frame than to shrink it for composition."
  )
}

/**
 * 「商品主体必须和我们的商品图一致」这一段（中文版）。
 *
 * 【2026-10-27 主人】「一些仿生类的需要做到改之后的商品主体与放入产品图片的主体一致，
 * 不能多一些东西少一些东西」。
 *
 * 为什么单独写一段而不是塞进别的规则里：
 *   仿生植物是重灾区——模型会「美化」，把叶子画多、画饱满、把塑料盆换成陶盆。
 *   结果买家到手一比，觉得和图上不是一个东西（跟之前 120cm 那次是同一类投诉）。
 *   所以要**把能变的和不能变的分开写**，再给一个可执行的数数自查。
 */
function productFidelityZh(layout: HotLayout): string {
  const structure = (layout.structureLock || '').trim()
  return (
    '**商品主体必须和我们提供的商品图（第 2 张）一致：不许加东西、不许少东西、不许美化**。' +
    (structure ? `我们这件商品的结构是：${structure}。` : '') +
    '画出来的商品必须和商品图里那件**逐项对得上**：枝条 / 叶片 / 花头 / 部件的' +
    '**数量、形状、朝向、疏密、比例、颜色、材质**全部照原图；' +
    '花盆 / 底座 / 配件的形状、颜色、材质也照原图。' +
    '**明确禁止**：多加叶片或花头、少画枝条、把商品画得更饱满更茂盛、' +
    '把普通花盆换成更好看的盆、给商品加原图没有的装饰（丝带、吊牌、蝴蝶结、托盘、小摆件）。' +
    '**允许变化**：光线方向、背景、接触阴影、整体缩放，以及为了配合版式做的轻微角度调整（不超过 15 度）。' +
    // 2026-10-27 补：这条只约束「商品本体」。
    // 使用类（猫爬架）参考图上常有猫/人，放置类（收纳）常画成装满东西的样子。
    // 如果只说「不许加东西」，模型会把猫、人、内容物一起删掉——那是错的。
    // 2026-10-27 主人：「第一张的图的背景与生成的背景完全不一致」。
    // 根因：模型把第 2 张（我们的商品图）的背景也借过来了——
    // 商品图左边有扇窗带枝影，出图就多了一扇参考图上根本没有的窗。
    // 所以必须明说：商品图只取本体，它的背景一律丢弃。
    "**第 2 张（我们的商品图）只取商品本体，它的背景一律丢弃**——" +
    "就当这件商品已经被抠出来、放在透明底上。商品图里的桌子、窗、墙面、地面、光影、道具，" +
    "**一样都不许带进成品**。成品里的墙面、地面、窗户、窗帘、家具、光影，**只能来自第 1 张参考图**。" +
    "**逐项对照**：参考图背景里有窗户就画窗户，**参考图没有窗户就一处都不许有**；" +
    "参考图有窗帘就画窗帘、没有就不画；地面是什么材质、什么颜色，就照参考图。" +
    "**这一条只约束商品本体**：参考图里本来就有的人物、动物（例如趴在猫爬架上的猫）、" +
    '手、道具和背景陈设，**必须按参考图原样保留**，不要因为我们的商品图里没有就把它们删掉或换掉；' +
    // 2026-10-27 实测：补了「商品图背景一律丢弃」之后，猫反而被整只删掉了。
// 所以必须把「动物/人必须出现」单独写死，而不是只说「保留」。
'收纳类如果参考图画了「里面装着东西」，也照参考图保留，不要清空。' +
'**参考图里如果有动物或人，成品里必须照样出现，一只/一个都不能少**；' +
'少画了动物或人，这一张同样算废片。商品图里没有他们，不是删掉的理由。' +
// 2026-10-27 主人：「要布偶猫蜷着」——动物用我们商品图里那只，不是参考图那只。
'**动物以我们商品图（第 2 张）里的那只为准**：品种、毛色、姿态都照商品图' +
'（例如商品图里是一只布偶猫蜷着睡在碗里，成品就画布偶猫蜷着睡在碗里）。' +
'参考图里的动物只说明「这里应该有一只动物」，**不要照抄它那只的品种和毛色**。' +
'只有我们商品图里根本没有动物时，才用参考图里那只。' +
// 2026-10-27 实测：猫回来了，但缅因猫比碗大，身体两侧溢出碗沿，像「坐在碗上面」。
// 所以还要约束：动物的大小和姿势必须和商品的**结构**匹配。
'**动物的大小和姿势必须和我们的商品结构匹配**：' +
'如果商品有碗 / 窝 / 吊床这类内凹结构，动物要**完整地躺进去、身体不超出内壁边缘**，看起来真的装得下；' +
'装不下就让它坐在平台或地面上，**不要硬塞，也不要让它骑在碗沿上**。' +
'姿势要自然（趴着、躺着、蜷着），不要正对镜头端坐。' +
    '**自查**：把成品和商品图并排放大，从左到右数一遍叶片或部件，数量必须一致。' +
    '这一张是**实物复刻**，不是设计稿：宁可画得不那么完美，也不许和实物不一样。'
  )
}

/** 同上的英文版。 */
function productFidelityEn(layout: HotLayout): string {
  const structure = (layout.structureLock || '').trim()
  return (
    '**The product body must match the product photo we supplied (image 2): add nothing, drop nothing, beautify nothing**. ' +
    (structure ? `The structure of our product is: ${structure}. ` : '') +
    'The drawn product must line up with the photographed one **item by item**: every stem, leaf, flower head and part must keep its ' +
    '**count, shape, orientation, density, proportion, colour and material** exactly as in the photo; ' +
    'the pot / base / accessories keep their shape, colour and material exactly as in the photo. ' +
    '**Explicitly forbidden**: adding leaves or flower heads, dropping stems, drawing the product fuller or lusher than it is, ' +
    'upgrading a plain pot into a nicer one, or adding decorations that are absent from the photo (ribbons, hang tags, bows, trays, small props). ' +
    '**Allowed to change**: light direction, background, contact shadow, overall scale, and a minor angle adjustment of up to 15 degrees to suit the layout. ' +
    "**Image 2 (our product photo) is only a source for the product body — discard its background entirely**: " +
    "treat the product as if it had already been cut out on a transparent background. The table, window, wall, floor, lighting and props from the product photo " +
    "**must not appear in the output at all**. Every wall, floor, window, curtain, furniture piece and shadow in the output **must come from reference image 1 only**. " +
    "**Check item by item**: if the reference background has a window, draw that window; **if it has none, there must be no window at all**; " +
    "if it has a curtain, draw the curtain, otherwise do not; match the floor material and colour to the reference. " +
    '**This rule constrains the product body only**: people, animals (for example the cat lounging on a cat tree), hands, props and set dressing that already exist in the reference must be **kept exactly as in the reference** — do not delete or replace them just because they are absent from our product photo. ' +
    'If the reference shows a storage item filled with contents, keep it filled as shown; do not empty it. ' +
'**If the reference contains an animal or a person, that animal or person MUST appear in the output — none may be missing**; ' +
'dropping an animal or a person makes the card a write-off as well. Their absence from our product photo is never a reason to delete them. ' +
'**Use the animal from our product photo (image 2)**: match its breed, coat colour and pose ' +
'(for example, if our photo shows a ragdoll cat curled up asleep in the bowl, draw a ragdoll cat curled up asleep in the bowl). ' +
'The animal in the reference only tells you that an animal belongs here — **do not copy that one\'s breed or coat colour**. ' +
'Only fall back to the reference animal when our product photo has no animal at all. ' +
'**The animal\'s size and pose must match the structure of our product**: ' +
'if the product has a bowl, nest or hammock-like recess, the animal must **lie fully inside it, its body not spilling over the inner rim**, so that it genuinely fits; ' +
'if it does not fit, let it sit on the platform or the floor — **never force it in, and never let it perch on the rim**. ' +
'The pose must look natural (lying down, curled up), not sitting upright facing the camera. ' +
    '**Self-check**: put the output next to the product photo, zoom in and count the leaves or parts from left to right — the counts must match. ' +
    'This image is a **faithful reproduction of a real product**, not a design concept: better to look less polished than to differ from the real thing.'
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
  const zone = (layout.bgColorZone || "").trim()
  const palette = buildProductPalette(layout.ourColorName, layout.ourColorHex)
  const colorLine = palette
    ? `我们商品的主色是 ${palette.name}（${palette.hex}），背景就从 ${palette.tint}（浅）到 ${palette.accent}（深）这个区间里取。`
    : layout.ourColorName
      ? `我们商品的主色是 ${layout.ourColorName}。`
      : ""

  // 【2026-10-27 主人选方案 A】实景照片背景**不换色**。
  // 拿棕榈树真图验证时，米色墙被推成绿墙——偏离原图太多。
  // 唯一例外：背景里大面积的其实就是竞品商品本身（厨房墙面是竞品瓷砖），那必须换。
  // 【2026-10-27 主人】「使用类的在背景方面以参考图的背景为主」。
  // 使用类（猫爬架、工具、家具）的参考图背景就是「使用现场」，
  // 换掉它等于把「这东西用在哪」这个信息丢了，所以这一类**一律照参考图，不换色**。
  if (layout.productClass === "use") {
    return (
      "**背景以参考图为主（使用类）：照参考图的背景画，不要换房间**。" +
      "参考图的背景就是这个商品的「使用现场」，必须**原样重现**：" +
      "同一面墙和墙色、同一个窗户位置、同一块地面材质、同样的家具与陈设、同样的光照方向。" +
      "**明确禁止**：换成另一个房间、换一面墙、换地板材质、增加或减少家具与陈设、把背景改成纯色或渐变。" +
      "也**不要**因为要和我们商品的色系搭配而改动背景颜色。" +
      "**更不许拿我们商品图的背景来凑**：商品图只给商品本体，参考图没有的窗户、窗帘、家具一律不要加。" +
      "**下面这些保持参考图原样**：文字颜色、图标颜色与描边、胶囊条和角标的底色、边框与分隔线。"
    )
  }

  const shouldRecolor = !layout.bgIsPhoto || layout.bgHasCompetitorProduct

  if (!shouldRecolor) {
    return (
      "**背景不换色（实景照片原样保留）**：" +
      "参考图的背景是一张实拍场景照（窗、家具、植物、墙面材质等），" +
      "**必须原样保留这张照片**：构图、物体、光位、材质、颜色都不许改，也不许重画。" +
      "只允许做**极轻微**的整体色温偏移（让画面不至于和我们商品打架），" +
      "**不许把背景墙或任何大面积区域改成别的颜色**。" +
      "**下面这些同样保持参考图原样**：文字颜色、图标颜色与描边、胶囊条和角标的底色、边框与分隔线。"
    )
  }

  return (
    "**背景换色（只换背景，别的一律不动）**：" +
    (zone ? `参考图里需要换色的背景区域是：${zone}。` : "") +
    colorLine +
    "这些背景区域的颜色要换成**我们商品主色的同色系**，不要保留竞品商品的颜色。" +
    (layout.bgHasCompetitorProduct
      ? "**注意：背景里出现的就是竞品商品本身（例如墙面就是竞品的瓷砖或板材），必须整块换成我们商品的色系。**"
      : "") +
    "**但下面这些必须保持参考图原样、绝对不要跟着变色**：文字颜色、图标颜色与描边、" +
    "胶囊条和角标的底色、边框与分隔线、按钮底色。"
  )
}

/** 同上的英文版。 */
function bgRecolorEn(layout: HotLayout): string {
  const zone = (layout.bgColorZone || "").trim()
  const palette = buildProductPalette(layout.ourColorName, layout.ourColorHex)
  const colorLine = palette
    ? `Our product's dominant colour is ${palette.name} (${palette.hex}); pick the background within the range from ${palette.tint} (pale) to ${palette.accent} (deep). `
    : layout.ourColorName
      ? `Our product's dominant colour is ${layout.ourColorName}. `
      : ""

  // For "use" products the reference background IS the usage scene — never replace it.
  if (layout.productClass === "use") {
    return (
      "**Background follows the reference (functional product)**: reproduce the reference background — do not swap the room. " +
      "That background is where this product is actually used, so reproduce it **exactly**: " +
      "the same wall and wall colour, the same window position, the same floor material, the same furniture and set dressing, the same light direction. " +
      "**Explicitly forbidden**: substituting another room, another wall, another floor material, adding or removing furniture and props, or turning the background into a flat colour or gradient. " +
      "Do **not** change the background colour to match our product hue either. " +
      "**And never borrow the background from our product photo**: it only supplies the product body; any window, curtain or furniture that is absent from the reference must not be added. " +
      "**These stay exactly as in the reference**: text colours, icon colours and strokes, pill and badge fills, borders and dividers."
    )
  }

  const shouldRecolor = !layout.bgIsPhoto || layout.bgHasCompetitorProduct

  if (!shouldRecolor) {
    return (
      "**Do NOT recolour the background (keep the photo as it is)**: " +
      "the reference background is a real scene photograph (window, furniture, plants, wall material). " +
      "**Keep that photograph untouched** — same composition, same objects, same light direction, same materials, same colours; do not repaint it. " +
      "A **very subtle** overall colour-temperature shift is allowed so the scene does not clash with our product, " +
      "but **never turn the background wall or any large area into another colour**. " +
      "**These stay exactly as in the reference too**: text colours, icon colours and strokes, pill and badge fills, borders and dividers."
    )
  }

  return (
    "**Background recolour (background only — nothing else changes)**: " +
    (zone ? `The background areas that need recolouring are: ${zone}. ` : "") +
    colorLine +
    "Recolour those background areas into **our product's hue family**; never keep the competitor product's colour. " +
    (layout.bgHasCompetitorProduct
      ? "**Note: the background itself shows the competitor product (for example a wall clad in their tiles or boards) — that whole area must be recoloured into our product's family.** "
      : "") +
    "**Everything listed here must stay exactly as in the reference and must NOT be recoloured**: text colours, icon colours and strokes, " +
    "pill and badge fill colours, borders and dividers, button fills."
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
function layoutCardBodyEn(layout: HotLayout, brandName = '', productSize = ''): string {
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
    sizeBlockEn(productSize) +
    productFidelityEn(layout) +
    bgRecolorEn(layout)
  )
}

/**
 * 一个版式 → 一张脚本卡片。
 *
 * `index` 直接决定「第几张出图」，所以顺序必须和参考图的顺序一致 ——
 * 主人的要求是「生图的数量必须以放入的参考图的数量一样」。
 */
export function buildHotLayoutCards(
  layouts: HotLayout[],
  brandName = '',
  productSize = '',
): OzonScriptCard[] {
  const brand = brandName.trim()
  const size = productSize.trim()
  return layouts.map((layout, position) => ({
    index: position + 1,
    key: `hot_${String(position + 1).padStart(2, '0')}_ref${layout.index}`,
    title: `第 ${position + 1} 张 · 复刻参考图 ${layout.index}`,
    prompt: `${FRAMING_SAFE_AREA}${layoutCardBodyZh(layout, brand, size)}`,
    promptEn: `${FRAMING_SAFE_AREA_EN}${layoutCardBodyEn(layout, brand, size)}`,
  }))
}
