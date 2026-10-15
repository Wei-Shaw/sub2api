/**
 * 从桌面版「熊管家AI作图 v2.0」搬过来的提示词部分。
 *
 * 这些字符串是原程序里的原文，没有改写——包括那段英文的
 * PRODUCT CONSISTENCY LOCK（商品一致性锁定）和 Ozon 八张主图脚本。
 * 保留原文的原因是：提示词的效果对措辞敏感，改一个字效果就可能变。
 */

/**
 * 商品一致性锁定。
 * 这是「生成的图和客户端显示的商品必须是同一件」这条要求的核心：
 * 明确告诉模型只许换背景/光线/排版/文字，不许动商品本身。
 * 原程序把它拼在每一张图的提示词最前面。
 */
export const PRODUCT_LOCK =
  'PRODUCT CONSISTENCY LOCK: The product in the reference image must be reproduced EXACTLY as-is. ' +
  'Do NOT reshape, distort, redesign, recolor, restyle, or alter proportions, materials, or details. ' +
  'Keep the same product shape, color, size ratio, and features. ' +
  'Only change background, lighting, layout, and text. Maintain photorealistic accuracy.\n\n'

/** Ozon 主图设计规范（原程序的系统提示词）。 */
export const OZON_SYSTEM_PROMPT = `# Role
你是一位精通俄罗斯 Ozon 电商平台的主图视觉设计专家。你的任务是根据提供的产品图（以及补充的文字描述），重新设计并生成一张符合 Ozon 平台高点击率（CTR）标准的高品质商品主图。

# Core Requirements（严格执行）
1. 尺寸比例：必须严格 3:4 竖版。
2. 产品一致性：产品外形、材质、颜色、品牌 Logo 和核心细节必须与参考图高度一致，不可扭曲、变形。
3. 语言限制：画面中严禁出现任何中文（除产品机身上自带无法去除的中文外）。所有新设计的文案、卖点、标签必须全部使用符合俄罗斯消费者阅读习惯的俄语（Russian）。
4. 原创度：在保持产品主体一致的前提下，背景、排版、构图需重新设计，原创重构度 ≥60%。
5. 配色克制：整张图的文字/图形配色不得超过 3 种颜色，保持干净专业。
6. 主体颜色一致性：本套图后续所有图中产品主体颜色必须与第一张保持一致。

# Visual Structure（三层构图）
- 前景：用精准地道的俄语展示产品核心卖点（USP）、用户痛点、核心功能、适用人群。
- 中景：产品主体展示，可组合使用：单品图 / 细节放大图 / 俄罗斯模特使用场景图 / 精美包装图 / Before-After 对比图 / 使用步骤图 / 部件拆解图。
- 背景：与产品调性相符的真实生活场景，或干净的高级纯色/渐变色。必须保证逼真光影、自然投影、合理透视。

# Layout Rules
- 中心布局 / 左右布局 / 上下布局 / 分栏混合。

# 本次具体任务
`

/** 场景库：产品名里命中关键词就自动配这个场景。 */
export const SCENE_LIBRARY: ReadonlyArray<readonly [string, string, string]> = [
  ['保温杯', 'modern office desk beside a laptop, natural window light', '办公室桌面 + 笔记本电脑 + 窗边自然光'],
  ['水杯', 'clean kitchen countertop, morning light, coffee beans nearby', '厨房台面 + 晨光 + 咖啡豆'],
  ['咖啡', 'cozy cafe wooden table, latte art, warm tone', '咖啡馆木桌 + 拉花 + 暖调'],
  ['壶', 'tea ceremony table, traditional Chinese style, soft light', '中式茶桌 + 柔和光线'],
  ['护肤', 'bright bathroom vanity, soft white towels, mirror reflection', '明亮浴室梳妆台 + 白毛巾 + 镜面'],
  ['面膜', 'spa-like bathroom, candle light, marble surface', 'SPA 感浴室 + 烛光 + 大理石台面'],
  ['口红', 'elegant dressing table, makeup mirror, jewelry box', '精致梳妆台 + 化妆镜 + 首饰盒'],
  ['香水', 'vanity table by window, morning light, silk fabric', '窗边梳妆台 + 晨光 + 丝绸布料'],
  ['T恤', 'urban street style, cafe window seat, candid', '城市街拍 + 咖啡馆窗边 + 抓拍'],
  ['衬衫', 'modern office, business casual, conference table', '现代办公室 + 商务休闲 + 会议桌'],
  ['鞋', 'city walking, subway commute, leather shoes on feet', '城市行走 + 地铁通勤'],
  ['包', 'lifestyle street photography, handbag on arm, city background', '街拍 + 手臂挎包 + 城市背景'],
  ['耳机', 'open-plan office, focused person wearing them, laptop', '开放办公室 + 专注佩戴者 + 笔记本'],
  ['音箱', 'living room cozy evening, sofa, warm lamp light', '客厅沙发 + 暖色台灯 + 夜晚'],
  ['充电', 'nightstand beside bed, cable plugged in, soft light', '床头柜台面 + 插线 + 柔光'],
  ['零食', 'breakfast table, hands reaching, morning toast', '早餐桌 + 手取食物 + 晨间吐司'],
  ['坚果', 'wooden bowl on kitchen island, scattered nuts', '厨房岛台木碗 + 散落坚果'],
  ['奶瓶', 'nursery room, soft crib light, pastel tone', '婴儿房 + 婴儿床柔光 + 马卡龙色'],
  ['灯', 'bedside table, warm evening glow, open book', '床头柜 + 夜晚暖光 + 翻开的书'],
  ['键盘', 'minimal desk setup, mechanical keyboard, wrist rest', '极简桌面 + 机械键盘 + 腕托'],
  ['鼠标', 'clean white desk, mouse on pad, laptop beside', '白色桌面 + 鼠标垫 + 笔记本'],
]

/** 没命中任何关键词时用的兜底场景。 */
export const DEFAULT_SCENE: readonly [string, string] = [
  'clean modern lifestyle scene, natural lighting, minimal interior',
  '现代简约生活场景 + 自然光',
]

export interface MatchedScene {
  /** 拼进提示词的英文场景描述 */
  en: string
  /** 界面上显示给用户看的中文场景描述 */
  zh: string
}

/** 按产品名自动配场景；匹配不到就用默认场景。 */
export function autoMatchScene(productName: string): MatchedScene {
  const name = (productName || '').trim()
  for (const [keyword, en, zh] of SCENE_LIBRARY) {
    if (name.includes(keyword)) return { en, zh }
  }
  return { en: DEFAULT_SCENE[0], zh: DEFAULT_SCENE[1] }
}

/** Ozon 主图可用的尺寸（原程序下拉里的三个）。 */
export const OZON_SIZE_OPTIONS = [
  { label: '1200 × 1600（3:4 竖版，推荐）', value: '1200x1600' },
  { label: '900 × 1200（3:4 竖版）', value: '900x1200' },
  { label: '1500 × 2000（3:4 竖版，高清）', value: '1500x2000' },
]

/** 生成脚本时用的产品资料。 */
export interface OzonScriptContext {
  product: string
  sp1?: string
  sp2?: string
  sp3?: string
  pain?: string
  diff?: string
  /** 自动匹配或用户指定的场景 */
  sceneEn?: string
  sceneZh?: string
}

export interface OzonScriptCard {
  /** 第几张（1–8） */
  index: number
  /** 用途标识，与桌面程序里的 01_compliance … 08_aftersales 对应 */
  key: string
  /** 界面上显示的标题，例如「第 1 张 · 合规主图」 */
  title: string
  /** 完整提示词 */
  prompt: string
}

/**
 * 生成 Ozon 八张主图脚本。
 * 每张的提示词 = 商品一致性锁定 + Ozon 设计规范 + 这一张的具体任务。
 * 没有参考图时不会带一致性锁定（纯文生图没有「参考图里的商品」可锁）。
 */
export function buildOzonScript(ctx: OzonScriptContext, options?: { lockProduct?: boolean }): OzonScriptCard[] {
  const product = (ctx.product || '').trim()
  const sp1 = (ctx.sp1 || '').trim() || 'feature 1'
  const sp2 = (ctx.sp2 || '').trim() || 'feature 2'
  const sp3 = (ctx.sp3 || '').trim() || 'feature 3'
  const pain = (ctx.pain || '').trim() || 'daily hassle'
  const diff = (ctx.diff || '').trim() || 'better material'
  const scene = (ctx.sceneEn || '').trim() || DEFAULT_SCENE[0]
  const lock = options?.lockProduct === false ? '' : PRODUCT_LOCK

  const tasks: Array<{ key: string; title: string; body: string }> = [
    {
      key: '01_compliance',
      title: '第 1 张 · 合规主图',
      body:
        `生成第1张合规主图（Ozon/WB审核位）。产品：${product}。` +
        '产品45度居中，占画面70-85%，完整入镜不裁切。' +
        '主文案用俄语明确标注产品尺寸（Размеры / Габариты）。' +
        '禁止Logo、水印、边框、促销标签。配色≤3种。3:4竖版1200x1600。',
    },
    {
      key: '02_lifestyle',
      title: '第 2 张 · 场景图',
      body:
        `生成第2张：场景图。产品：${product}。场景：${scene}。` +
        '中景产品在真实生活场景中自然使用，自然光，不摆拍。无文字（场景图保持干净）。Photorealistic.',
    },
    {
      key: '03_feature_infographic',
      title: '第 3 张 · 核心卖点信息图',
      body:
        `生成第3张：核心卖点信息图。产品：${product}。` +
        `产品居中+图标化俄语标签展示：${sp1} / ${sp2} / ${sp3}。` +
        `主文案突出${sp1}。配色≤3种。`,
    },
    {
      key: '04_pain_comparison',
      title: '第 4 张 · 痛点对比图',
      body:
        `生成第4张：痛点对比图。产品：${product}。` +
        `左灰调「普通款」vs 右亮调「我们的产品」。俄语主文案：Ты тоже ${pain}?`,
    },
    {
      key: '05_detail_closeup',
      title: '第 5 张 · 细节特写',
      body: `生成第5张：细节特写。产品：${product}。怼近拍：${sp1}、${sp2} 的材质/接口/工艺细节。柔和影棚光。`,
    },
    {
      key: '06_size_chart',
      title: '第 6 张 · 尺寸规格图',
      body: `生成第6张：尺寸规格图。产品：${product}。产品线稿+俄语厘米标注（Размеры, см）。`,
    },
    {
      key: '07_trust_compare',
      title: '第 7 张 · 信任对比图',
      body:
        `生成第7张：信任对比图。产品：${product}。VS同行或认证/检测/买家秀。` +
        `俄语主文案已售10万+，副文案${diff}。`,
    },
    {
      key: '08_aftersales',
      title: '第 8 张 · 售后保障图',
      body:
        `生成第8张：售后保障图。产品：${product}。画面打开状态但不要展示包装盒和说明书。` +
        '必须包含以下俄语客服引导文案（全部俄语，排版清晰，配色≤3种）：' +
        'Заголовок: «Уважаемый покупатель! Пожалуйста, не спешите оставлять негативный отзыв. Мы можем вам помочь!»' +
        'Подзаголовок: «Пожалуйста, свяжитесь с нами на странице поддержки»' +
        'Иконки-бейджи: «Товар в целости и сохранности», «Товар отправлен», «Задержка доставки», «Связаться с поддержкой маркетплейса»' +
        'Инструкция: «1. Перейдите на страницу товара → 2. Нажмите Вопросы → 3. Выберите Задать вопрос»' +
        'Подпись: «Профиль — Сообщения — Поддержка, чтобы связаться с продавцом»' +
        '俄语主文案价格+副文案1年质保。',
    },
  ]

  return tasks.map((task, index) => ({
    index: index + 1,
    key: task.key,
    title: task.title,
    prompt: `${lock}${OZON_SYSTEM_PROMPT}${task.body}`,
  }))
}

/** 视觉模型列表（原程序下拉里的）。 */
export const VISION_MODEL_OPTIONS = [
  'gemini-3.5-flash',
  'gemini-3.8-flash',
  'gemini-3.1-pro-preview',
  'gpt-5.4-mini',
  'gpt-5.4',
  'gpt-5.5',
  'qwen3.8-omni-flash',
  'grok-4.5',
]

export const VISION_DEFAULT_MODEL = 'gemini-3.5-flash'

/**
 * AI 识别卖点的提示词。
 *
 * 要求模型**一次返回两套卖点**：
 *   - `sp1/sp2/sp3` 中文 —— 给**界面**看的，主人要求识别结果用中文显示；
 *   - `sp1_ru/sp2_ru/sp3_ru` 俄语 —— 给**图**用的，这段文字会印在图上。
 * 为什么不直接用中文出图：Ozon/WB 是俄语市场，图上出现中文会被判不合格。
 * 所以显示用中文、出图用俄语，两套都得要。
 */
export const VISION_EXTRACT_PROMPT =
  'Ты эксперт по продажам на Ozon/Wildberries. Проанализируй товар на фото и выведи СТРОГО JSON без пояснений: ' +
  '{"product":"中文产品名","audience":"目标人群中文","sp1":"卖点1中文","sp2":"卖点2中文","sp3":"卖点3中文",' +
  '"sp1_ru":"卖点1俄语","sp2_ru":"卖点2俄语","sp3_ru":"卖点3俄语","pain":"用户痛点中文","diff":"差异化中文"}. ' +
  'sp1/sp2/sp3 — по-китайски (для интерфейса). sp1_ru/sp2_ru/sp3_ru — тот же смысл по-русски ' +
  '(этот текст печатается на картинке, поэтому обязательно по-русски). ' +
  'Не выдумывай, опирайся на фото.'

export interface SellingPoints {
  product?: string
  audience?: string
  /** 中文卖点：界面显示用 */
  sp1?: string
  sp2?: string
  sp3?: string
  /** 俄语卖点：印在图上用 */
  sp1_ru?: string
  sp2_ru?: string
  sp3_ru?: string
  pain?: string
  diff?: string
}

/**
 * 从模型回复里抠出 JSON。
 * 模型经常在 JSON 外面包一层解释文字或 ```json 代码块，所以用最外层大括号截取。
 */
export function parseSellingPoints(raw: string): SellingPoints | null {
  const text = (raw || '').trim()
  if (!text) return null
  const start = text.indexOf('{')
  const end = text.lastIndexOf('}')
  if (start < 0 || end <= start) return null
  try {
    const parsed = JSON.parse(text.slice(start, end + 1))
    if (!parsed || typeof parsed !== 'object') return null
    return parsed as SellingPoints
  } catch {
    return null
  }
}
