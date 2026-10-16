/**
 * 从桌面版「熊管家AI作图 v2.0」搬过来的提示词部分。
 *
 * 【2026-10-15 主人要求精简】原来这两段是原程序**原文照抄**的：
 * PRODUCT_LOCK 330 字符英文，OZON_SYSTEM_PROMPT 620 字中文 markdown
 * （`# Role` / `# Core Requirements` / `# Visual Structure` / `# Layout Rules`）。
 *
 * 实测问题：
 *   1. 生图模型**没有"身份"这个概念**，`# Role 你是一位…专家` 纯属浪费；
 *      markdown 标题和 `**加粗**` 也不被解析，只会变成一串字符，
 *      甚至可能被当成画面元素画出来（主人那段长提示词要求"严禁出现中文"，
 *      却把一大段中文规则喂了进去）。
 *   2. 长提示词明显更慢，在上游抖动时更容易整条超时。
 *   3. 「从 7 种中景里选 / 从 5 种布局里选」等于给模型出一道选择题，
 *      它不会选，只会凑。
 *
 * 所以这里把"给对话模型看的说明书"删掉，只留"画什么"：
 * 商品一致性 + 俄语 + 配色 ≤3 种 + 不许自造数字。
 *
 * ⚠️ **但「要求一条没少」这句当时说错了**：`size ratio`（商品在图里的占比）
 * 被一起删掉了，见下面 PRODUCT_LOCK 上的说明。那是一条真实的要求，不该删。
 * 教训：精简时逐条比一遍「这句话约束的是哪件事」，不要因为看着像重复就删。
 */

/**
 * 商品一致性锁定。
 * 这是「生成的图和客户端显示的商品必须是同一件」这条要求的核心：
 * 明确告诉模型只许换背景/光线/排版/文字，不许动商品本身。
 *
 * 【2026-10-15 补回 size ratio】上一轮精简时这里砍成了 158 字符，
 * 只留下 `same shape, color, material, logo and proportions`。
 * 后来发现**砍坏了一件事**：`proportions` 说的是「商品自身的长宽比例不能变形」，
 * **不等于**「商品在图里该占多大」。原文里的 **`size ratio`** 才是后者，
 * 被删掉之后模型对「商品在画面里的大小」就没有任何约束了 ——
 * 实测表现是商品被放大/缩小，和原图对不上。
 *
 * 所以这里把两件事分开写清楚，并把原文那串禁止项补回来：
 *   1. `same size and scale in frame` —— 在图里的占比要和原图一致；
 *   2. `Do NOT reshape, distort, redesign, recolor, restyle` —— 不许重塑变形改色。
 *
 * 仍然**不**把原文那 330 字符整段搬回来：那会重新引入上一轮已经确认的问题
 * （提示词越长越慢、上游抖动时更容易整条超时）。只补真正丢掉的两点。
 */
export const PRODUCT_LOCK =
  'Keep the product in the reference image exactly as-is: same shape, color, material, ' +
  'logo and proportions, and the same size and scale within the frame. ' +
  'Do NOT reshape, distort, redesign, recolor or restyle it. ' +
  'Only change background, lighting, layout and text.\n\n'

/** Ozon 主图设计规范（精简版：只留真正影响画面的要求）。 */
export const OZON_SYSTEM_PROMPT = `Ozon 商品主图，3:4 竖版。
- 画面文字一律用俄语，不出现中文
- 文字和图形配色不超过 3 种
- 商品保持原样，只换背景、排版和文字
- 不要添加原图没有的数字和尺寸
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
        `第1张：合规主图（平台审核位）。产品：${product}。` +
        '产品45度居中，占画面70-85%，完整入镜不裁切。俄语标注尺寸（Размеры）。' +
        '不要Logo、水印、边框、促销标签。',
    },
    {
      key: '02_lifestyle',
      title: '第 2 张 · 场景图',
      body:
        `第2张：场景图。产品：${product}。场景：${scene}。` +
        '产品在真实场景中自然使用，自然光，不摆拍。画面不要文字。',
    },
    {
      key: '03_feature_infographic',
      title: '第 3 张 · 核心卖点信息图',
      body:
        `第3张：核心卖点信息图。产品：${product}。` +
        `产品居中，图标化俄语标签：${sp1} / ${sp2} / ${sp3}。主文案突出 ${sp1}。`,
    },
    {
      key: '04_pain_comparison',
      title: '第 4 张 · 痛点对比图',
      body:
        `第4张：痛点对比图。产品：${product}。` +
        `左灰调「普通款」vs 右亮调「我们的产品」。俄语主文案：Ты тоже ${pain}?`,
    },
    {
      key: '05_detail_closeup',
      title: '第 5 张 · 细节特写',
      body: `第5张：细节特写。产品：${product}。怼近拍 ${sp1}、${sp2} 的材质/接口/工艺。柔和影棚光。`,
    },
    {
      key: '06_size_chart',
      title: '第 6 张 · 尺寸规格图',
      body: `第6张：尺寸规格图。产品：${product}。产品线稿 + 俄语厘米标注（Размеры, см）。`,
    },
    {
      key: '07_trust_compare',
      title: '第 7 张 · 信任对比图',
      body:
        `第7张：信任对比图。产品：${product}。对比同行或展示认证/检测/买家秀。` +
        `俄语主文案：已售 10 万+。副文案：${diff}。`,
    },
    {
      key: '08_aftersales',
      title: '第 8 张 · 售后保障图',
      body:
        `第8张：售后保障图。产品：${product}。画面打开状态，不要包装盒和说明书。俄语文案：` +
        '«Уважаемый покупатель! Пожалуйста, не спешите оставлять негативный отзыв. Мы можем вам помочь!» ' +
        '«Пожалуйста, свяжитесь с нами на странице поддержки» ' +
        'Бейджи: «Товар в целости и сохранности» / «Товар отправлен» / «Задержка доставки» / «Связаться с поддержкой маркетплейса» ' +
        '«1. Перейдите на страницу товара → 2. Нажмите Вопросы → 3. Выберите Задать вопрос» ' +
        '«Профиль — Сообщения — Поддержка, чтобы связаться с продавцом»',
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
