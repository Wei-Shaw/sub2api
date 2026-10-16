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

/**
 * 生成爆款复刻的 8 张脚本（AIDA 结构）。
 *
 * 和单产品模式脚本的关键差别：
 *   - 第 1 张是「主视觉钩子」（不是白底合规图），并且**把爆款版式塞进画面描述**
 *   - 第 8 张才是「白底合规图」（顺序正好颠倒）
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

  // 版式描述作为前缀插进第 1 张的「画面」里，后面跟一个全角逗号（和桌面程序一致）。
  const stylePrefix = ctx.styleDesc?.trim() ? `${ctx.styleDesc.trim()}，` : ''
  const cards: OzonScriptCard[] = [
    {
      index: 1,
      key: '01_hook',
      title: '第 1 张 · 主视觉钩子（AIDA · 注意）',
      prompt:
        `${stylePrefix}${HOT_STYLE_BRIEF}\n` +
        `第1张：主视觉钩子。产品：${product}。产品45度斜放，背景与同行反着来。` +
        `主文案：${product}。副文案：${sp1 || '核心卖点'} · ${audience} 首选。`,
    },
    {
      index: 2,
      key: '02_pain',
      title: '第 2 张 · 痛点唤醒（AIDA · 注意）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第2张：痛点唤醒。产品：${product}。场景实拍，把痛点拍成真实生活瞬间，不要摆拍感。场景：${scene}。` +
        `主文案：你是不是也${pain}？副文案：这一次，彻底解决它。`,
    },
    {
      index: 3,
      key: '03_interest',
      title: '第 3 张 · 核心效果（AIDA · 兴趣）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第3张：核心效果。产品：${product}。使用后效果特写，把好处可视化，不要堆字。` +
        `产品居中 + 图标化卖点：${sp1 || '卖点1'} / ${sp2 || '卖点2'} / ${sp3 || '卖点3'}。` +
        `主文案：${sp1 || '核心卖点'}。`,
    },
    {
      index: 4,
      key: '04_trust',
      title: '第 4 张 · 信任背书（AIDA · 信任）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第4张：信任背书。产品：${product}。销量截图 + 证书 + 检测报告 + 买家秀拼图。` +
        `主文案：${trust}。副文案：为什么这么多人选它。`,
    },
    {
      index: 5,
      key: '05_detail',
      title: '第 5 张 · 细节特写（AIDA · 欲望）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第5张：细节特写。产品：${product}。怼近拍材质纹理、接口、按键、缝线，` +
        `把 ${sp2 || '卖点2'}、${sp3 || '卖点3'} 拍成看得见的品质。`,
    },
    {
      index: 6,
      key: '06_compare',
      title: '第 6 张 · 对比反超（AIDA · 对比）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第6张：对比反超。产品：${product}。左右分屏VS：左普通款灰调，右我们亮调，中间VS分割线。` +
        `产品线稿 + 俄语厘米标注。主文案：凭什么选我们？副文案：${diff}。`,
    },
    {
      index: 7,
      key: '07_offer',
      title: '第 7 张 · 利益与售后（AIDA · 行动）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第7张：利益与售后。产品：${product}。价格标签 + 赠品堆头 + 售后图标横向排列，` +
        `打开状态但不展示包装盒和说明书。主文案：${price}。副文案：${service}。`,
    },
    {
      index: 8,
      key: '08_compliance',
      title: '第 8 张 · 白底合规图（合规兜底）',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第8张：白底合规图（平台审核/搜索抓取位）。产品：${product}。` +
        '纯白底#FFFFFF正视图，不加任何文字、水印、贴纸。',
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
export function buildHotExtraCards(product: string): OzonScriptCard[] {
  const name = (product || '').trim() || '本产品'
  return [
    {
      index: 9,
      key: '09_side',
      title: '第 9 张 · 侧面/多角度展示图',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第9张：侧面/多角度展示图。产品：${name}。` +
        '同一件商品并排展示 3 个角度（正面 / 侧面 / 45度背面），构图干净、间距均匀，' +
        '让买家看清厚度、轮廓和侧面结构。侧面必须有真实厚度感，不要压扁成纸片。' +
        '主文案：全方位看清每一面。',
    },
    {
      index: 10,
      key: '10_macro',
      title: '第 10 张 · 细节特写补充图',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第10张：细节特写补充图。产品：${name}。` +
        '2x2 四宫格微距——材质纹理、接口/按键、缝线或密封圈、底部细节，每格怼近拍，' +
        '各配一个俄语短标注指向具体位置，突出做工和用料。主文案：细节见真章。',
    },
    {
      index: 11,
      key: '11_usage',
      title: '第 11 张 · 使用场景补充图',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第11张：使用场景补充。产品：${name}。俄罗斯模特在真实生活场景中使用产品，自然抓拍，突出使用体验。`,
    },
    {
      index: 12,
      key: '12_faq',
      title: '第 12 张 · FAQ / 对比图',
      prompt:
        `${HOT_STYLE_BRIEF}\n` +
        `第12张：FAQ/对比。产品：${name}。左右对比：普通款 vs 我们的产品，俄语标注 3 个常见问题答案。`,
    },
  ]
}
