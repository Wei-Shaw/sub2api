/**
 * 爆款复刻（桌面程序的「爆款复刻模式」）。
 *
 * 思路：拿几张同行的爆款主图 → 让视觉模型拆出「版式特征」→
 * 再把这份版式套到我们自己的商品上，生成一整套主图。
 *
 * 参数照抄桌面程序（已反汇编确认）：
 *   - 拆解 9 个维度，多张图**逐字段取出现次数最多的值**合并
 *   - 拆解温度 0.2；识别卖点温度 0.3
 *   - 脚本用 **AIDA 漏斗**（注意→兴趣→信任→欲望→对比→行动），
 *     白底合规图放在**第 8 张**（和单产品模式的脚本正好相反）
 */
import type { OzonScriptCard } from './aiImagePrompts'

/** 拆解的 9 个维度。key 是模型要返回的 JSON 字段名。 */
export const HOT_STYLE_DIMENSIONS = [
  { key: 'layout', label: '布局' },
  { key: 'bg', label: '背景' },
  { key: 'main_color', label: '主色' },
  { key: 'accent_color', label: '点缀色' },
  { key: 'text_pos', label: '文案位置' },
  { key: 'text_style', label: '字体' },
  { key: 'decor', label: '装饰元素' },
  { key: 'hook', label: '点击钩子' },
  { key: 'composition', label: '构图' },
] as const

export type HotStyleDimensionKey = (typeof HOT_STYLE_DIMENSIONS)[number]['key']

/** 拆解爆款的提示词。桌面程序用的是俄语，这里保持一致。 */
export const HOT_ANALYZE_PROMPT =
  'Ты топ e-commerce designer. Разбери эту главную карточку Ozon/WB и верни СТРОГО JSON ' +
  'без пояснений: {"layout":"布局","bg":"背景风格","main_color":"主色","accent_color":"点缀色",' +
  '"text_pos":"文案位置","text_style":"字体风格","decor":"装饰元素","hook":"点击钩子",' +
  '"composition":"构图规律"}.'

/** 拆解用温度（桌面程序是 0.2）。 */
export const HOT_ANALYZE_TEMPERATURE = 0.2
/** 识别卖点用温度（桌面程序是 0.3）。 */
export const HOT_EXTRACT_TEMPERATURE = 0.3
/** 视觉请求超时（桌面程序是 90 秒）。 */
export const HOT_VISION_TIMEOUT_MS = 90000

/**
 * 爆款复刻专用的「看我的商品图 → 出卖点/痛点/差异化」提示词。
 *
 * 【2026-10-16 主人说明的流程】主人讲的爆款复刻是：
 *   ① 放同行的爆款图 → 拆版式；
 *   ② 放**我自己的商品图** → 用我的商品主体套同版式生成；
 *   ③ **卖点**由视觉模型（Gemini）生成；
 *   ④ **痛点 / 差异化**靠「跟我放的商品图做对比」得出。
 *
 * 和套图那份 `VISION_EXTRACT_PROMPT` 的区别就在这里：
 * 套图那份只描述「照片上是什么」，这一份要求模型**做对比**——
 * 拿这件商品跟同类里的普通款比，比出「买家现在受的什么罪」（痛点）
 * 和「这件强在哪」（差异化）。所以单独立一份，不去动套图那条链路。
 */
export const HOT_EXTRACT_PROMPT =
  'Ты эксперт по продажам на Ozon/Wildberries. ' +
  'Посмотри на товар на фото и СРАВНИ его с обычными похожими товарами этой категории. ' +
  'Выведи СТРОГО JSON без пояснений: ' +
  '{"product":"中文产品名","audience":"目标人群中文","sp1":"卖点1中文","sp2":"卖点2中文","sp3":"卖点3中文",' +
  '"sp1_ru":"卖点1俄语","sp2_ru":"卖点2俄语","sp3_ru":"卖点3俄语",' +
  '"pain":"用户痛点中文","diff":"差异化中文"}. ' +
  'pain — какую проблему обычных товаров решает этот товар (главная боль покупателя). ' +
  'diff — чем именно этот товар отличается от обычных (материал, конструкция, комплект). ' +
  'sp1/sp2/sp3 — по-китайски (для интерфейса). sp1_ru/sp2_ru/sp3_ru — тот же смысл по-русски ' +
  '(этот текст печатается на картинке, поэтому обязательно по-русски). ' +
  'Не выдумывай, опирайся на фото.'

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

/** 把单张图的拆解结果收成「字段 → 值」，去掉空值。 */
export function normalizeStyleSample(parsed: Record<string, unknown> | null): Partial<Record<HotStyleDimensionKey, string>> {
  const result: Partial<Record<HotStyleDimensionKey, string>> = {}
  if (!parsed) return result
  for (const dimension of HOT_STYLE_DIMENSIONS) {
    const value = parsed[dimension.key]
    if (typeof value === 'string' && value.trim()) {
      result[dimension.key] = value.trim()
    }
  }
  return result
}

/**
 * 把多张图的拆解结果合并成一条版式描述。
 *
 * 每个维度**取出现次数最多的那个值**（桌面程序用 Counter.most_common）。
 * 出现次数相同时，保留**先出现**的那个 —— 这样结果稳定、可复现，
 * 不会因为遍历顺序变化而每次不一样。
 */
export function mergeStyleSamples(
  samples: Array<Partial<Record<HotStyleDimensionKey, string>>>,
): { description: string; used: number } {
  const usable = samples.filter((sample) => Object.keys(sample).length > 0)
  if (!usable.length) return { description: '', used: 0 }

  const parts: string[] = []
  for (const dimension of HOT_STYLE_DIMENSIONS) {
    const counts = new Map<string, number>()
    for (const sample of usable) {
      const value = sample[dimension.key]
      if (!value) continue
      counts.set(value, (counts.get(value) || 0) + 1)
    }
    if (!counts.size) continue

    let bestValue = ''
    let bestCount = 0
    for (const [value, count] of counts) {
      if (count > bestCount) {
        bestCount = count
        bestValue = value
      }
    }
    parts.push(`${dimension.label}=${bestValue}`)
  }

  if (!parts.length) return { description: '', used: usable.length }
  return {
    description: `【批量拆解${usable.length}张爆款共同版式】${parts.join('；')}`,
    used: usable.length,
  }
}

export interface HotScriptContext {
  product: string
  sp1?: string
  sp2?: string
  sp3?: string
  pain?: string
  diff?: string
  /** 拆解出来的爆款版式描述 */
  styleDesc?: string
  sceneZh?: string
  /** 桌面程序里是硬编码的默认值 */
  audience?: string
  price?: string
  trust?: string
  service?: string
}

/** 桌面程序里这几个字段是写死的默认值。 */
export const HOT_DEFAULTS = {
  audience: '目标用户',
  price: '1990₽',
  trust: '已售 10 万+ / SGS 检测',
  service: '7 天无理由 + 1 年质保',
} as const

/**
 * 每张卡片共用的精简前缀。
 *
 * 【2026-10-15 主人要求精简】原来每条卡片末尾都重复一遍
 * 「俄语文案，配色≤3种，3:4竖版1200x1600。Photorealistic.」——
 * 八张就是八遍。提到这里来只写一次，卡片里只留「这一张画什么」。
 */
export const HOT_STYLE_BRIEF =
  'Ozon 主图，3:4 竖版 1200x1600。画面文字用俄语。文字和图形配色≤3种。商品保持原样。Photorealistic.'

/** 上面那段共用前缀的**英文版**——真正发给模型的是这一份（理由同套图的 OZON_SYSTEM_PROMPT_EN）。 */
export const HOT_STYLE_BRIEF_EN =
  'Ozon main image, 3:4 portrait 1200x1600. Every piece of on-image text must be Russian; never render Chinese. ' +
  'Use at most 3 colors for text and graphics. Keep the product exactly as in the reference. Photorealistic.'

/**
 * 构图安全区：商品必须完整入镜，不许贴边、不许裁切。
 *
 * 【2026-10-17 主人反馈】「爆款复刻出来的图片展示不全」——商品顶部被切掉了。
 * 根因有两条叠加：
 *   ① 卡片里只写了占比（如「居中大幅产品」），**没有一句要求「完整入镜」**；
 *   ② 爆款版式说明里常带「大幅产品」，模型按「大幅」放大后就出框了。
 * 所以单独加一条硬规则，并且明确写「版式说明要服从这一条」。
 */
export const HOT_FRAMING_SAFE_AREA =
  '- 【构图安全区】商品必须完整入镜：所有部位都在画面内，四周留 8-10% 的安全边距，' +
  '任何部位都不得贴边、不得被裁切。版式说明里若写了「大幅产品」「满屏」，一律服从这一条。\n'

export const HOT_FRAMING_SAFE_AREA_EN =
  '- [Framing safety] The product must be fully in frame: every part inside the image, with 8-10% safe margin on all sides. ' +
  'Nothing may touch the edge and nothing may be cropped. If the layout note says "large product" or "full-bleed", this rule wins.\n'

/**
 * 把拆解出来的版式包成一段「必须沿用」的说明，**每一张都带**。
 *
 * 【2026-10-16 主人反馈】「要识别的放入的商品图片在生成排版之后没有把这些排版融入
 * 图片生成的提示词中」——查证属实：原来的 `stylePrefix` **只拼在第 1 张**，
 * 第 2~12 张完全没有版式，等于除第 1 张外都是「没套版式的通用脚本」。
 *
 * 现在改成每一张都带，并额外给一句「本张侧重」，
 * 免得 12 张因为共用同一段版式而长得一模一样。
 *
 * 版式描述是视觉模型生成的中文，所以英文版里要额外加一句：
 * 这段只是构图参考，**不要把它里面的字当成画面文字画出来**。
 */
function styleBlockZh(styleDesc: string, focus: string): string {
  const desc = (styleDesc || '').trim()
  if (!desc) return ''
  return `【沿用同行爆款版式】${desc}\n本张侧重：${focus}\n`
}

function styleBlockEn(styleDesc: string, focusEn: string): string {
  const desc = (styleDesc || '').trim()
  if (!desc) return ''
  return (
    `[Clone the competitor layout] ${desc}\n` +
    'The layout note above is a composition reference only — translate its meaning into camera, framing, background, colour and text placement. ' +
    'Never render the words of that note as image text.\n' +
    `Focus for this card: ${focusEn}\n`
  )
}

/**
 * 每张卡片的开头：同行版式 + 构图安全区 + 共用风格说明。
 *
 * `cropSafe = false` 用于局部微距那种「本来就要看不到完整商品」的卡片。
 */
function cardHeadZh(styleDesc: string, focus: string, cropSafe = true): string {
  return `${styleBlockZh(styleDesc, focus)}${cropSafe ? HOT_FRAMING_SAFE_AREA : ''}${HOT_STYLE_BRIEF}\n`
}

function cardHeadEn(styleDesc: string, focusEn: string, cropSafe = true): string {
  return `${styleBlockEn(styleDesc, focusEn)}${cropSafe ? HOT_FRAMING_SAFE_AREA_EN : ''}${HOT_STYLE_BRIEF_EN}\n`
}

/**
 * 生成爆款复刻的 8 张脚本（AIDA 结构）。
 *
 * 和单产品模式脚本的关键差别：
 *   - 第 1 张是「主视觉钩子」（不是白底合规图）
 *   - 第 8 张才是「白底合规图」（顺序正好颠倒）
 *
 * 【2026-10-16】每一张都带上同行版式（原来只有第 1 张带），
 * 并且每张同时产出中文版（给用户看/改）和英文版（发给模型）。
 */
export function buildHotScript(ctx: HotScriptContext): OzonScriptCard[] {
  const product = (ctx.product || '').trim()
  const sp1 = (ctx.sp1 || '').trim()
  const sp2 = (ctx.sp2 || '').trim()
  const sp3 = (ctx.sp3 || '').trim()
  const pain = (ctx.pain || '').trim() || 'daily hassle'
  const diff = (ctx.diff || '').trim() || 'better material'
  const audience = (ctx.audience || '').trim() || HOT_DEFAULTS.audience
  const price = (ctx.price || '').trim() || HOT_DEFAULTS.price
  const trust = (ctx.trust || '').trim() || HOT_DEFAULTS.trust
  const service = (ctx.service || '').trim() || HOT_DEFAULTS.service
  const scene = (ctx.sceneZh || '').trim() || '真实生活场景'
  const style = (ctx.styleDesc || '').trim()

  const cards: OzonScriptCard[] = [
    {
      index: 1,
      key: '01_hook',
      title: '第 1 张 · 主视觉钩子（AIDA · 注意）',
      prompt:
        `${cardHeadZh(style, '布局、点击钩子、构图、主色')}${HOT_STYLE_BRIEF}\n` +
        `第1张：主视觉钩子。产品：${product}。产品45度斜放，背景与同行反着来。` +
        `主文案：${product}。副文案：${sp1 || '核心卖点'} · ${audience} 首选。`,
      promptEn:
        `${cardHeadEn(style, 'layout, click hook, composition, dominant color')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 1: main visual hook. Product: ${product}. Product placed at a 45-degree tilt; the background deliberately contrasts with competitors'. ` +
        `Headline: ${product}. Subline: ${sp1 || 'key benefit'} · the first choice for ${audience}.`,
    },
    {
      index: 2,
      key: '02_pain',
      title: '第 2 张 · 痛点唤醒（AIDA · 注意）',
      prompt:
        `${cardHeadZh(style, '背景、构图')}${HOT_STYLE_BRIEF}\n` +
        `第2张：痛点唤醒。产品：${product}。场景实拍，把痛点拍成真实生活瞬间，不要摆拍感。场景：${scene}。` +
        `主文案：你是不是也${pain}？副文案：这一次，彻底解决它。`,
      promptEn:
        `${cardHeadEn(style, 'background, composition')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 2: pain awakening. Product: ${product}. Documentary-style scene shot; turn the pain into a real-life moment, no staged feel. Scene: ${scene}. ` +
        `Headline: "Ты тоже ${pain}?" Subline: this time it is solved for good.`,
    },
    {
      index: 3,
      key: '03_interest',
      title: '第 3 张 · 核心效果（AIDA · 兴趣）',
      prompt:
        `${cardHeadZh(style, '布局、文案位置、点缀色')}${HOT_STYLE_BRIEF}\n` +
        `第3张：核心效果。产品：${product}。使用后效果特写，把好处可视化，不要堆字。` +
        `产品居中 + 图标化卖点：${sp1 || '卖点1'} / ${sp2 || '卖点2'} / ${sp3 || '卖点3'}。` +
        `主文案：${sp1 || '核心卖点'}。`,
      promptEn:
        `${cardHeadEn(style, 'layout, text placement, accent color')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 3: core benefit. Product: ${product}. Close-up of the after-use result; make the benefit visible, do not pile up words. ` +
        `Product centred with icon-style selling points: ${sp1 || 'benefit 1'} / ${sp2 || 'benefit 2'} / ${sp3 || 'benefit 3'}. ` +
        `Headline: ${sp1 || 'key benefit'}.`,
    },
    {
      index: 4,
      key: '04_trust',
      title: '第 4 张 · 信任背书（AIDA · 信任）',
      prompt:
        `${cardHeadZh(style, '装饰元素、文案位置')}${HOT_STYLE_BRIEF}\n` +
        `第4张：信任背书。产品：${product}。销量截图 + 证书 + 检测报告 + 买家秀拼图。` +
        `主文案：${trust}。副文案：为什么这么多人选它。`,
      promptEn:
        `${cardHeadEn(style, 'decorative elements, text placement')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 4: social proof. Product: ${product}. Sales screenshot + certificate + test report + buyer photo collage. ` +
        `Headline: ${trust}. Subline: why so many people choose it.`,
    },
    {
      index: 5,
      key: '05_detail',
      title: '第 5 张 · 细节特写（AIDA · 欲望）',
      prompt:
        `${cardHeadZh(style, '背景、构图')}${HOT_STYLE_BRIEF}\n` +
        `第5张：细节特写。产品：${product}。怼近拍材质纹理、接口、按键、缝线，` +
        `把 ${sp2 || '卖点2'}、${sp3 || '卖点3'} 拍成看得见的品质。`,
      promptEn:
        `${cardHeadEn(style, 'background, composition')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 5: detail close-up. Product: ${product}. Macro on material texture, connector, button and stitching; ` +
        `make ${sp2 || 'benefit 2'} and ${sp3 || 'benefit 3'} visible as real quality.`,
    },
    {
      index: 6,
      key: '06_compare',
      title: '第 6 张 · 对比反超（AIDA · 对比）',
      prompt:
        `${cardHeadZh(style, '布局、主色、点缀色')}${HOT_STYLE_BRIEF}\n` +
        `第6张：对比反超。产品：${product}。左右分屏VS：左普通款灰调，右我们亮调，中间VS分割线。` +
        `产品线稿 + 俄语厘米标注。主文案：凭什么选我们？副文案：${diff}。`,
      promptEn:
        `${cardHeadEn(style, 'layout, dominant color, accent color')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 6: head-to-head comparison. Product: ${product}. Split screen: grey-toned ordinary version on the left, bright "ours" on the right, VS divider in the middle. ` +
        `Product line-art with Russian centimetre annotations. Headline: why choose us? Subline: ${diff}.`,
    },
    {
      index: 7,
      key: '07_offer',
      title: '第 7 张 · 利益与售后（AIDA · 行动）',
      prompt:
        `${cardHeadZh(style, '文案位置、字体风格、装饰元素')}${HOT_STYLE_BRIEF}\n` +
        `第7张：利益与售后。产品：${product}。价格标签 + 赠品堆头 + 售后图标横向排列，` +
        `打开状态但不展示包装盒和说明书。主文案：${price}。副文案：${service}。`,
      promptEn:
        `${cardHeadEn(style, 'text placement, typography style, decorative elements')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 7: offer and after-sales. Product: ${product}. Price tag + gift pile + after-sales icons in a horizontal row; ` +
        `shown open, without box or manual. Headline: ${price}. Subline: ${service}.`,
    },
    {
      index: 8,
      key: '08_compliance',
      title: '第 8 张 · 白底合规图（合规兜底）',
      // 白底合规图**不带版式**：合规位要求纯白底、无任何装饰，带版式反而会被平台判不合格。
      // 但**要带构图安全区**——它也是完整商品图，被裁掉一样不合格。
      prompt:
        `${HOT_FRAMING_SAFE_AREA}${HOT_STYLE_BRIEF}\n` +
        `第8张：白底合规图（平台审核/搜索抓取位）。产品：${product}。` +
        '纯白底#FFFFFF正视图，商品居中，占画面 70-85%，不加任何文字、水印、贴纸。',
      promptEn:
        `${HOT_FRAMING_SAFE_AREA_EN}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 8: white-background compliance image (marketplace review and search thumbnail). Product: ${product}. ` +
        'Pure white #FFFFFF front view, product centred, occupying 70-85%; no text, no watermark, no stickers.',
    },
  ]

  return cards
}

/**
 * 8 张主图之外的可选补充图。
 *
 * 主人要求：把商品的**侧面图、细节图**补进去，让买家能看到商品更多细节。
 * 所以这两张排在最前面（紧跟 8 张主图），使用场景和 FAQ 往后顺延。
 * 勾上「额外再生成」时才会一起生成。
 *
 * 之所以做成函数（而不是常量数组）：前两张要点名商品，才能把侧面的厚度、
 * 细节的材质锚定在**这一件**商品上，而不是让模型自己发挥。
 */
export function buildHotExtraCards(product: string, styleDesc = ''): OzonScriptCard[] {
  const name = (product || '').trim() || '本产品'
  const style = (styleDesc || '').trim()
  return [
    {
      index: 9,
      key: '09_angles',
      title: '第 9 张 · 多角度展示图（含俯视）',
      prompt:
        `${cardHeadZh(style, '背景、构图')}${HOT_STYLE_BRIEF}\n` +
        `第9张：多角度展示图。产品：${name}。` +
        '同一件商品并排展示 4 个**不同视角**：正面平视 / 正侧面 / 正上方俯视 / 45度俯视，' +
        '构图干净、间距均匀。**俯视要能看到顶面和内部结构**，侧面要有真实厚度感，不要压扁成纸片。' +
        '四个视角必须是同一件商品，颜色和结构完全一致。主文案：全方位看清每一面。',
      promptEn:
        `${cardHeadEn(style, 'background, composition')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 9: multi-angle showcase. Product: ${name}. ` +
        'Show the same product in 4 **different viewpoints** side by side: front eye-level / pure side / straight top-down / 45-degree top-down. ' +
        'Clean composition, even spacing. **The top-down view must reveal the top face and interior structure**; the side view must convey real thickness, not a flattened sheet. ' +
        'All four viewpoints must be the same item, identical in color and structure. Headline: see every side at a glance.',
    },
    {
      index: 10,
      key: '10_macro',
      title: '第 10 张 · 细节特写补充图',
      // 四宫格微距：本来就要「看不到完整商品」，不加构图安全区。
      prompt:
        `${cardHeadZh(style, '背景、构图', false)}${HOT_STYLE_BRIEF}\n` +
        `第10张：细节特写补充图。产品：${name}。` +
        '2x2 四宫格微距——材质纹理、接口/按键、缝线或密封圈、底部细节，每格怼近拍，' +
        '各配一个俄语短标注指向具体位置，突出做工和用料。主文案：细节见真章。',
      promptEn:
        `${cardHeadEn(style, 'background, composition', false)}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 10: extra detail close-ups. Product: ${name}. ` +
        '2x2 grid of macro shots — material texture, connector/buttons, stitching or sealing ring, bottom detail. Each cell is a tight close-up ' +
        'with a short Russian annotation pointing at the exact spot, emphasising build quality and materials. Headline: the detail is the proof.',
    },
    {
      index: 11,
      key: '11_usage',
      title: '第 11 张 · 使用场景补充图',
      prompt:
        `${cardHeadZh(style, '背景、构图')}${HOT_STYLE_BRIEF}\n` +
        `第11张：使用场景补充。产品：${name}。俄罗斯模特在真实生活场景中使用产品，自然抓拍，突出使用体验。`,
      promptEn:
        `${cardHeadEn(style, 'background, composition')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 11: extra usage scene. Product: ${name}. A Russian model using the product in a real-life setting, candid capture, emphasising the experience.`,
    },
    {
      index: 12,
      key: '12_faq',
      title: '第 12 张 · FAQ / 对比图',
      prompt:
        `${cardHeadZh(style, '布局、文案位置')}${HOT_STYLE_BRIEF}\n` +
        `第12张：FAQ/对比。产品：${name}。左右对比：普通款 vs 我们的产品，俄语标注 3 个常见问题答案。`,
      promptEn:
        `${cardHeadEn(style, 'layout, text placement')}${HOT_STYLE_BRIEF_EN}\n` +
        `Card 12: FAQ / comparison. Product: ${name}. Left-right comparison: ordinary version vs our product, with 3 common questions answered in Russian.`,
    },
  ]
}
