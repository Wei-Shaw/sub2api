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

/** Ozon 主图设计规范（中文版：界面显示给用户看的）。 */
export const OZON_SYSTEM_PROMPT = `Ozon 商品主图，3:4 竖版。
- 画面文字一律用俄语，不出现中文
- 文字和图形配色不超过 3 种
- **配色跟着商品走**：文字、图标、色块、背景元素都取商品主体的主色调（同色系的深浅变化），
  不要用和商品无关的颜色。例：紫色芭蕾包 → 标题和图标都用紫，背景也带一点淡紫
- 商品保持原样，只换背景、排版和文字
- 一张图只画一个画面：一件商品、一个视角。禁止拼图、三联图、九宫格、多视角拼贴；
  只有「对比图」才允许左右各一件：两件必须是同一类产品的两个档次（一件普通款、一件我们的），
  既不能是完全不相干的品类，也不能一模一样看不出差别
- 不要添加原图没有的数字和尺寸
`

/**
 * 上面那段规范的**英文版**——真正发给模型的是这一份。
 *
 * 【2026-10-16 主人要求】「只需要保证用户输入的是中文，出来提示词给用户看到的也是中文，
 * 期间的运行处理以最低的花费去进行」。
 *
 * 做法：模板里**同时写中英两份**，不调用模型翻译 —— **零额外费用、零额外等待**。
 * 界面上显示和编辑的是中文；发给模型的是英文。
 *
 * 为什么值得这么做：模型看到的指令语言和它该输出的画面文字语言一致时，
 * 更不容易「跟着指令的语言」把中文画上去。原来满屏中文指令 + 一句「不要中文」，
 * 本身就是别扭的指令，实测会低概率漏中文。
 */
export const OZON_SYSTEM_PROMPT_EN = `Ozon product main image, 3:4 portrait.
- Every piece of text rendered on the image must be Russian. Never render Chinese.
- Use at most 3 colors for text and graphic elements.
- **Derive the palette from the product**: the headline, icons, colour blocks and background elements should all use the product's own dominant hue (different shades of the same family); never use colours unrelated to the product. Example: a purple ballet bag gets purple type and icons, and a faintly purple-tinted background.
- Keep the product exactly as in the reference; only change background, layout and text.
- One frame, one picture: a single product and a single viewpoint. Never make a collage, triptych, nine-grid or multi-view montage; only the comparison card may show two items side by side, and they must be two grades of the same product category (an ordinary version and ours) — never an unrelated category, and never two identical items.
- Do not add numbers or dimensions that are not present in the reference image.
`

/**
 * 构图安全区：商品必须完整入镜，不许贴边、不许裁切。
 *
 * 【2026-10-17 主人反馈】「爆款复刻和套图都出现图片展示不全」——
 * 商品顶部/边缘被切掉了。查代码发现根因：卡片里只写了
 * 「占画面 70-85%」这类**占比**，但**没有任何一句要求「完整入镜」**。
 * 模型按占比把产品放大后自然就出框了；爆款复刻那边版式说明里还有「大幅产品」，
 * 更是雪上加霜。
 *
 * 所以把它做成一条**独立的硬规则**，拼在每一张需要露出完整商品的卡片前面。
 * 三个局部微距卡片（材质/结构/工艺）**不加**——它们本来就要「看不到完整商品」。
 */
export const FRAMING_SAFE_AREA =
  '- 【构图安全区】商品必须完整入镜：所有部位都在画面内，四周留 8-10% 的安全边距，' +
  '任何部位都不得贴边、不得被裁切。即使版式说明里写了「大幅产品」，也必须先满足这一条。\n'

export const FRAMING_SAFE_AREA_EN =
  '- [Framing safety] The product must be fully in frame: every part inside the image, with 8-10% safe margin on all sides. ' +
  'Nothing may touch the edge and nothing may be cropped. Even if the layout note says "large product", this rule wins.\n'

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

/**
 * 主图的三种版式。
 *
 * 【2026-10-24 主人要求】「主体在图片中的位置可以按照排版换位置，
 * 比如在图片的右边时候信息就在左边。更多的是灵活放置，所有都是同一个位置的话很单调。」
 *
 * 所以主图不再写死「商品居中偏右、文字在左」，而是每次生成脚本时**随机挑一种**，
 * 这样连做几套不会每张都长一个样。挑中的那一种会写进提示词正文，
 * 用户在脚本卡片里能看到，也能自己改。
 */
export type MainImageLayout = 'productRight' | 'productLeft' | 'productBottom'

export const MAIN_IMAGE_LAYOUTS: MainImageLayout[] = ['productRight', 'productLeft', 'productBottom']

/** 每种版式的「商品放哪 / 文字放哪 / 光从哪来」，中英各一份。 */
const MAIN_IMAGE_LAYOUT_SPEC: Record<MainImageLayout, { zh: string; en: string }> = {
  productRight: {
    zh:
      '版式：**商品在画面右侧**，完整入镜，占画面宽度 45-55%、高度 55-70%；左侧留出约 40% 的竖带专门放文字。' +
      '光线：左前方柔和窗光，右侧补一点反光，投影落在右下。' +
      '文字位置：① 左上角 2-3 行大号俄语主标题；② 主标题下一行手写体俄语副标题；' +
      '③ 左侧中部竖排三组「实心圆形图标 + 两行俄语」；④ 左下角一个圆形局部放大镜；' +
      '⑤ 底部一条圆角胶囊条，横跨画面下方。',
    en:
      'Layout: **the product sits on the RIGHT**, fully in frame, taking 45-55% of the width and 55-70% of the height; the left 40% is a vertical band reserved for type. ' +
      'Lighting: soft window light from the front-left with a little bounce on the right; the shadow falls to the lower right. ' +
      'Text placement: (1) 2-3 line large Russian headline at the upper left; (2) one line of handwritten Russian subtitle beneath it; ' +
      '(3) three stacked rows of "filled circular icon + two lines of Russian" down the left-middle; (4) a circular detail magnifier at the lower left; ' +
      '(5) a rounded pill bar across the bottom.',
  },
  productLeft: {
    zh:
      '版式：**商品在画面左侧**，完整入镜，占画面宽度 45-55%、高度 55-70%；右侧留出约 40% 的竖带专门放文字。' +
      '光线：右前方柔和窗光，左侧补一点反光，投影落在左下。' +
      '文字位置：① 右上角 2-3 行大号俄语主标题（右对齐）；② 主标题下一行手写体俄语副标题；' +
      '③ 右侧中部竖排三组「实心圆形图标 + 两行俄语」（右对齐）；④ 右下角一个圆形局部放大镜；' +
      '⑤ 底部一条圆角胶囊条，横跨画面下方。',
    en:
      'Layout: **the product sits on the LEFT**, fully in frame, taking 45-55% of the width and 55-70% of the height; the right 40% is a vertical band reserved for type. ' +
      'Lighting: soft window light from the front-right with a little bounce on the left; the shadow falls to the lower left. ' +
      'Text placement: (1) a 2-3 line large Russian headline at the upper right, right-aligned; (2) one line of handwritten Russian subtitle beneath it; ' +
      '(3) three stacked rows of "filled circular icon + two lines of Russian" down the right-middle, right-aligned; (4) a circular detail magnifier at the lower right; ' +
      '(5) a rounded pill bar across the bottom.',
  },
  productBottom: {
    zh:
      '版式：**商品在画面下半部居中**，完整入镜，占画面宽度 60-70%、高度 50-58%；上半部约 35% 留白专门放文字。' +
      '光线：顶部柔和散射光加左侧一点窗光，投影落在商品正下方。' +
      '文字位置：① 顶部居中 2-3 行大号俄语主标题；② 主标题右下方一行手写体俄语副标题；' +
      '③ 商品左右两侧各排一到两组「实心圆形图标 + 两行俄语」，合计三组；④ 右下角一个圆形局部放大镜；' +
      '⑤ 最底部一条圆角胶囊条。',
    en:
      'Layout: **the product sits centred in the LOWER half**, fully in frame, taking 60-70% of the width and 50-58% of the height; the upper 35% is reserved for type. ' +
      'Lighting: soft diffused light from above plus a little window light from the left; the shadow falls directly under the product. ' +
      'Text placement: (1) a 2-3 line large Russian headline centred across the top; (2) one line of handwritten Russian subtitle at its lower right; ' +
      '(3) one or two rows of "filled circular icon + two lines of Russian" on each side of the product, three rows in total; (4) a circular detail magnifier at the lower right; ' +
      '(5) a rounded pill bar along the very bottom.',
  },
}

/** 把色板写成一段能直接塞进提示词的话（中文）。 */
export function describePaletteZh(p: ProductPalette): string {
  if (p.neutral) {
    return (
      `**配色（本张重点，必须严格照做）**：商品本身是灰/白/黑这类中性色（${p.name} ${p.hex}），没有色相可用。` +
      `大标题和胶囊条用深灰 ${p.ink}；副标题和三组图标圆底用中灰 ${p.accent}；背景用极浅暖灰 ${p.tint}（不要做成纯白）。` +
      '整张唯一的彩色只能取自商品自身的一个点缀（拉链、配件、包装上的颜色），没有就不加彩色。'
    )
  }
  return (
    `**配色（本张重点，必须严格照做）**：商品主色是${p.name}（${p.hex}）。` +
    `大标题和胶囊条用它的深色 ${p.ink}；副标题和三组图标圆底用 ${p.accent}；` +
    `背景用极淡的同色 ${p.tint} 做柔和渐变（**不要做成白色或灰色**）。` +
    '除了这几个颜色和黑白，不要再引入别的颜色。'
  )
}

/** 把色板写成一段能直接塞进提示词的话（英文，真正发给模型的那份）。 */
export function describePaletteEn(p: ProductPalette): string {
  if (p.neutral) {
    return (
      `**Palette (the key of this card, follow it strictly)**: the product itself is neutral grey/white/black (${p.name} ${p.hex}), so there is no usable hue. ` +
      `Use dark grey ${p.ink} for the headline and the pill bar; mid grey ${p.accent} for the subtitle and the three icon discs; ` +
      `use a very pale warm grey ${p.tint} for the background (not pure white). ` +
      'The only chromatic accent allowed must come from the product itself (zip, fittings, packaging); if there is none, add no colour at all.'
    )
  }
  return (
    `**Palette (the key of this card, follow it strictly)**: the product's dominant colour is ${p.name} (${p.hex}). ` +
    `Use its dark shade ${p.ink} for the headline and the pill bar; ${p.accent} for the subtitle and the three icon discs; ` +
    `use a very pale tint ${p.tint} of the same hue for a soft gradient background (**never white or grey**). ` +
    'Do not introduce any colour beyond these and black/white.'
  )
}

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
  /**
   * 主图版式。不传就随机挑一种（主人要求「灵活放置，不要都是一个位置」）。
   * 测试里传死值，免得随机导致用例飘。
   */
  mainLayout?: MainImageLayout
  /**
   * 商品主色（视觉模型识别出来的）。
   *
   * 【2026-10-24 主人问「是否保证了背景、文字与商品颜色一致或相近」】
   * 原来只有一句「取商品主体的主色调」，模型得自己猜是什么色，遵守率不高。
   * 现在把识别出来的**具体颜色**写进提示词，从「你猜」变成「照这个用」。
   * 只用在第 1 张主图上（主人指定「主图实现即可」）。
   */
  mainColorName?: string
  /** 形如 #C8B6E2。传了就按它算深/中/浅三个派生色。 */
  mainColorHex?: string
  /**
   * 商品实际尺寸，用户在界面上填的，例如「高 120 厘米」「60×40×30 厘米」。
   *
   * 【2026-10-26 主人反馈】「人工制作的仿生植物，物品有 120cm，模特的比例就不对了」——
   * 出图里那棵 120 厘米的仿真植物被画成桌上小盆栽，跟真人对不上。
   * 模型没法从照片里知道这东西多高，所以必须由人告诉它。
   * 有了这个数字，第 3 张（模特）和第 7 张（尺寸图）就能按真实尺寸画。
   */
  productSize?: string
}

/** 商品主色的派生色板（只给第 1 张主图用）。 */
export interface ProductPalette {
  /** 颜色叫法，例如「淡紫色」 */
  name: string
  /** 原色 #RRGGBB */
  hex: string
  /** 大标题、胶囊条用的深色 */
  ink: string
  /** 副标题、图标圆底用的中间色 */
  accent: string
  /** 背景用的极淡同色 */
  tint: string
  /** 商品本身是灰/白/黑，没有可用色相 */
  neutral: boolean
}

function clampByte(n: number): number {
  return Math.max(0, Math.min(255, Math.round(n)))
}

/** 把 "#c8b6e2" / "c8b6e2" / "#C8B6E2" 规整成 "#C8B6E2"；认不出来返回 null。 */
export function normalizeHex(input?: string): string | null {
  const raw = (input || '').trim().replace(/^#/, '')
  if (!/^[0-9a-fA-F]{6}$/.test(raw)) return null
  return `#${raw.toUpperCase()}`
}

function hexToRgb(hex: string): { r: number; g: number; b: number } {
  const h = hex.replace('#', '')
  return {
    r: parseInt(h.slice(0, 2), 16),
    g: parseInt(h.slice(2, 4), 16),
    b: parseInt(h.slice(4, 6), 16),
  }
}

function rgbToHex(r: number, g: number, b: number): string {
  return `#${[r, g, b].map((v) => clampByte(v).toString(16).padStart(2, '0')).join('').toUpperCase()}`
}

/** RGB → HSL，h 是 0-360，s/l 是 0-1。 */
function rgbToHsl(r: number, g: number, b: number): { h: number; s: number; l: number } {
  const rr = r / 255
  const gg = g / 255
  const bb = b / 255
  const max = Math.max(rr, gg, bb)
  const min = Math.min(rr, gg, bb)
  const l = (max + min) / 2
  if (max === min) return { h: 0, s: 0, l }
  const d = max - min
  const s = l > 0.5 ? d / (2 - max - min) : d / (max + min)
  let h: number
  if (max === rr) h = ((gg - bb) / d + (gg < bb ? 6 : 0)) * 60
  else if (max === gg) h = ((bb - rr) / d + 2) * 60
  else h = ((rr - gg) / d + 4) * 60
  return { h, s, l }
}

/** HSL → RGB（h 0-360，s/l 0-1）。 */
function hslToRgb(h: number, s: number, l: number): { r: number; g: number; b: number } {
  const c = (1 - Math.abs(2 * l - 1)) * s
  const hp = (((h % 360) + 360) % 360) / 60
  const x = c * (1 - Math.abs((hp % 2) - 1))
  let r = 0
  let g = 0
  let b = 0
  if (hp < 1) [r, g, b] = [c, x, 0]
  else if (hp < 2) [r, g, b] = [x, c, 0]
  else if (hp < 3) [r, g, b] = [0, c, x]
  else if (hp < 4) [r, g, b] = [0, x, c]
  else if (hp < 5) [r, g, b] = [x, 0, c]
  else [r, g, b] = [c, 0, x]
  const m = l - c / 2
  return { r: (r + m) * 255, g: (g + m) * 255, b: (b + m) * 255 }
}

/**
 * 从商品主色推出「深 / 中 / 浅」三个能直接写进提示词的颜色。
 *
 * 为什么要推三个而不是直接说「用商品主色」：
 * 大标题压在浅色背景上必须够深才读得清，背景又必须够淡才不抢商品，
 * 只给一个色值模型还是会自己乱调。给死三个值，遵守率高得多。
 *
 * 商品是灰/白/黑时（饱和度过低）**没有色相可用**，这时退化成中性灰阶，
 * 并明确要求从商品上挑一个点缀色当唯一彩色——否则模型会自己编一个颜色上去。
 */
/**
 * 从用户填的尺寸里抠出「厘米数」（取第一个数字）。
 *
 * 主人在界面上填的可能是各种写法：「高 120 厘米」「120cm」「1.2 m」「60×40×30 厘米」。
 * 这里只取第一个数字当「主尺寸」用（通常就是高度），并统一换算成厘米：
 *   - 带 m / 米 且没有 cm → 按米算（1.2 m = 120 cm）
 *   - 带 mm / 毫米        → 除以 10
 *   - 其余按厘米
 *
 * 解析不出来就返回 null，提示词退回「按参考图比例推算」的老说法，不会报错。
 */
export function parseSizeCm(text?: string): number | null {
  const raw = (text || '').trim().toLowerCase()
  if (!raw) return null
  const match = raw.match(/(\d+(?:\.\d+)?)/)
  if (!match) return null
  const value = Number.parseFloat(match[1])
  if (!Number.isFinite(value) || value <= 0) return null

  const isMm = /mm|毫米/.test(raw)
  const hasCm = /cm|厘米|公分/.test(raw)
  const isMetre = !hasCm && !isMm && /(^|[^a-z])(m|米)([^a-z]|$)/.test(raw)

  if (isMm) return value / 10
  if (isMetre) return value * 100
  return value
}

/**
 * 把「商品相对人体的大小」说成人话，给第 3 张（模特）用。
 *
 * 参照成年女性身高 165 cm：商品高度 ÷ 165 得到它相当于人体的哪个位置。
 * 这一步必须由**我们**算好写死，不能让模型自己估——它估不准就会把 120 cm 的
 * 仿真植物画成桌上的小盆栽（主人 2026-10-26 报的就是这个）。
 */
export function describeHumanScale(cm: number): { zh: string; en: string } {
  const ratio = cm / 165
  const pct = Math.round(ratio * 100)
  if (ratio >= 0.65) {
    return {
      zh: `商品高约 ${cm} 厘米，相当于成年女性身高的 ${pct}%，也就是到她**胸口以上、接近下巴**的高度——这是一件大件，绝不能被画成捧在手里的小盆栽。`,
      en: `The product is about ${cm} cm tall — roughly ${pct}% of an adult woman's height, i.e. reaching **above her chest, close to her chin**. This is a large item and must never be drawn as a small potted plant held in the palm.`,
    }
  }
  if (ratio >= 0.35) {
    return {
      zh: `商品高约 ${cm} 厘米，相当于成年女性身高的 ${pct}%，大约到她**腰到胸口**之间。`,
      en: `The product is about ${cm} cm tall — roughly ${pct}% of an adult woman's height, reaching somewhere **between her waist and chest**.`,
    }
  }
  return {
    zh: `商品高约 ${cm} 厘米，相当于成年女性身高的 ${pct}%，是**可以单手拿住的中小件**。`,
    en: `The product is about ${cm} cm tall — roughly ${pct}% of an adult woman's height, a **small-to-medium item she can hold in one hand**.`,
  }
}

export function buildProductPalette(name?: string, hex?: string): ProductPalette | null {
  const normalized = normalizeHex(hex)
  if (!normalized) return null

  const { r, g, b } = hexToRgb(normalized)
  const { h, s, l } = rgbToHsl(r, g, b)

  if (s < 0.08) {
    return {
      name: (name || '').trim() || '中性色',
      hex: normalized,
      ink: '#2B2B2B',
      accent: '#8A8A8A',
      tint: '#F3F2F0',
      neutral: true,
    }
  }

  const inkRgb = hslToRgb(h, Math.min(1, s * 0.9), Math.min(0.3, Math.max(0.16, l * 0.45)))
  const accentRgb = hslToRgb(h, Math.min(1, s), Math.min(0.6, Math.max(0.42, l * 0.8)))
  const tintRgb = hslToRgb(h, Math.min(0.5, s * 0.45), 0.94)

  return {
    name: (name || '').trim() || normalized,
    hex: normalized,
    ink: rgbToHex(inkRgb.r, inkRgb.g, inkRgb.b),
    accent: rgbToHex(accentRgb.r, accentRgb.g, accentRgb.b),
    tint: rgbToHex(tintRgb.r, tintRgb.g, tintRgb.b),
    neutral: false,
  }
}

export interface OzonScriptCard {
  /** 第几张（从 1 开始，按数组顺序） */
  index: number
  /** 用途标识，例如 01_selling_points / 02_compliance */
  key: string
  /** 界面上显示的标题，例如「第 1 张 · 核心卖点信息图」 */
  title: string
  /**
   * 中文提示词：**界面显示 + 用户可以改的就是这一份**。
   * 用户改完，生成时用的就是改后的内容（不再是模板原文）。
   */
  prompt: string
  /**
   * 英文提示词：**真正发给模型的那一份**。
   * 用户没改过时直接发它（零额外花费）；改过时改成「英文硬规则 + 用户改后的中文」。
   */
  promptEn: string
  /** 用户是否手动改过这一张。改过就不再发模板的英文版。 */
  edited?: boolean
}

/**
 * 一套脚本的张数。
 * 单独导出，是为了让界面按钮上的「生成 N 张脚本」不会写死之后再跟模板脱节
 * （2026-10-19 把 10 张改成 9 张、2026-10-24 又改成 8 张时，
 * 按钮文案就是靠它自动跟上的）。
 */
export const OZON_CARD_COUNT = 8

/**
 * 生成 Ozon 主图脚本（**9 张**）。
 *
 * 【2026-10-16 主人定的结构】
 *   主人指出「9 张全是我放进去的商品主体，太单调，需要一些商品的细节展示」，
 *   并且「多角度不只是正面/侧面/45度，还要俯视等其他角度」。
 *
 * 【2026-10-19 主人又改了两处】
 *   ① 「核心痛点和痛点对比合为一张图片」——
 *      原来的第 1 张「核心卖点信息图」和第 4 张「痛点对比图」合并成第 1 张，
 *      所以总数从 10 张变 9 张；
 *   ② 「第九张第十张的角度取消，换为如图片中的内容」——
 *      原来的「正俯视 / 正侧视」两个角度图换成两张售后/客服引导图：
 *      第 4 张售后保障图、第 4 张联系卖家步骤图。
 *
 * 现在的顺序：**卖点+痛点 → 模特场景 → 生活场景 → 三张细节微距 → 尺寸 → 售后保障 → 联系卖家**。
 *
 * 每张的提示词 = 设计规范 + 这一张的具体任务。
 * 中文给用户看/改，英文发给模型（两套都由这里的模板生成，不调用模型翻译）。
 *
 * 注：**商品一致性锁定不在这里拼** —— 它统一在生成时拼（`buildEffectivePrompt`），
 * 因为取决于「这一次到底有没有带参考图」，建脚本时还不知道。
 * 而且那段锁定是英文，不该出现在用户看到的中文提示词里。
 */
export function buildOzonScript(ctx: OzonScriptContext): OzonScriptCard[] {
  const product = (ctx.product || '').trim()
  const sp1 = (ctx.sp1 || '').trim() || 'feature 1'
  const sp2 = (ctx.sp2 || '').trim() || 'feature 2'
  const sp3 = (ctx.sp3 || '').trim() || 'feature 3'
  /**
   * 主图版式：调用方指定就用它，没指定就**随机挑一种**。
   *
   * 为什么随机：主人说「所有都是同一个位置的话很单调」。每次建脚本都在三个版式里
   * 挑一个，连做几套就不会每张主图都长一个样。挑中的那一种会明明白白写进提示词，
   * 用户在脚本卡片里看得到、也能自己改。
   */
  const mainLayout: MainImageLayout =
    ctx.mainLayout ?? MAIN_IMAGE_LAYOUTS[Math.floor(Math.random() * MAIN_IMAGE_LAYOUTS.length)]
  const layoutSpec = MAIN_IMAGE_LAYOUT_SPEC[mainLayout]

  /**
   * 商品主色的具体色板（有识别结果才有）。
   *
   * 【2026-10-24 主人问「是否保证了背景、文字与商品颜色一致或相近」】
   * 只有一句「取商品主色调」时，模型得自己猜色，遵守率不高。
   * 识别到主色之后就把深/中/浅三个具体色值写进第 1 张主图，
   * 从「你猜」变成「照这个用」。没有识别结果就退回原来那句笼统要求。
   */
  const palette = buildProductPalette(ctx.mainColorName, ctx.mainColorHex)
  const paletteZh = palette
    ? describePaletteZh(palette)
    : '配色：文字、图标、色块、背景都取商品主体的主色调（同色系深浅），不要用和商品无关的颜色。'
  const paletteEn = palette
    ? describePaletteEn(palette)
    : 'Palette: use the product’s own dominant hue for type, icons, colour blocks and background; never use colours unrelated to the product. '

  /**
   * 第 8 张（售后/联系卖家图）的底色说明。
   *
   * 【2026-10-24 主人】「页面的颜色依照产品主体的颜色来定」——
   * 样例里粉色椅子的售后图整张是粉的、绿色鱼缸的整张是绿的。
   * 所以底色不再固定，而是从商品主色推出来的深→浅竖向渐变。
   */
  const paletteGroundZh = palette
    ? `底色（必须照做）：从 ${palette.ink} 到 ${palette.tint} 的**竖向渐变**（上方深、下方浅），整张只走这一个色系；不要换成别的颜色。`
    : '底色：用商品主体主色的深→浅竖向渐变，颜色要跟商品主体一致，不要用和商品无关的颜色。'
  const paletteGroundEn = palette
    ? `Ground (follow strictly): a **vertical gradient** from ${palette.ink} at the top to ${palette.tint} at the bottom; stay inside that one hue family and use no other background colour. `
    : 'Ground: a vertical gradient from the product’s own dominant hue (dark at the top, pale at the bottom); never a background colour unrelated to the product. '

  /**
   * 商品实际尺寸（用户在界面填的）。
   *
   * 【2026-10-26 主人】「人工制作的仿生植物，物品有 120cm，模特的比例就不对了」——
   * 模型从照片里看不出这东西多高，所以必须由人给数字，我们再算成「相当于人体哪个位置」。
   */
  const sizeRaw = (ctx.productSize || '').trim()
  const sizeCm = parseSizeCm(sizeRaw)
  const humanScale = sizeCm ? describeHumanScale(sizeCm) : null

  /** 第 3 张（模特）里那段尺寸锚定，没填尺寸就是空串。 */
  const sizeAnchorZh = humanScale
    ? `**商品实际尺寸：${sizeRaw}。**${humanScale.zh}` +
      '**人物与商品的大小关系必须按这个来画；宁可让商品占满画面甚至超出画面，也不许为了构图好看把它缩小。**'
    : ''
  const sizeAnchorEn = humanScale
    ? `**Actual product size: ${sizeRaw}.** ${humanScale.en} ` +
      '**The person-to-product scale must follow this; it is better for the product to fill or even exceed the frame than to shrink it for the sake of composition.** '
    : ''

  /** 第 7 张（尺寸图）里的尺寸锚定，没填尺寸就是空串。 */
  const sizeChartZh = humanScale
    ? `**这件商品的实际尺寸是 ${sizeRaw}**，下面每一个尺寸标注都要和它一致，不要自己另估一个数。`
    : ''
  const sizeChartEn = humanScale
    ? `**The actual size of this product is ${sizeRaw}**; every dimension label below must agree with it — never estimate your own number. `
    : ''
  /**
   * ⚠️ `ctx.pain` 和 `ctx.diff` **故意不再拼进任何一张卡片**。
   *
   * 原因：这两个字段是**中文**（界面里填「用户痛点」「差异化」），
   * 一旦拼进提示词正文，模型就会把汉字当成「要画上去的内容」——
   * 2026-10-19 那次「图上出现一整段中文」就是这么来的。
   *
   * 现在第 1 张（主图）和第 2 张（对比图）的文案都交给模型按商品自己写俄语，
   * 不再把中文原文喂进去。表单字段保留着（用户仍可填），只是不参与出图。
   */
  const sceneZh = (ctx.sceneZh || '').trim() || '真实生活场景'
  const sceneEn = (ctx.sceneEn || '').trim() || DEFAULT_SCENE[0]

  /**
   * 每张卡片都要写清楚「镜头 / 主体占比 / 背景 / 光线 / 构图 / 文字 / 禁忌」。
   *
   * 这是本轮的核心改动：原来每张只有 38~54 个字的画面描述，只够说「产品居中」，
   * 模型只能自由发挥，所以出图雷同。现在把这些维度写实，
   * 再配合不同的镜头与光线，画面才会真的不一样。
   */
  const tasks: Array<{
    key: string
    title: string
    bodyZh: string
    bodyEn: string
    /** 默认要「完整入镜」；三个局部微距卡片设成 false（它们本来就要看不到完整商品）。 */
    cropSafe?: boolean
    /** 只有尺寸规格图要写数字，需要临时覆盖「不要添加原图没有的数字」那条规范。 */
    allowDimensions?: boolean
  }> = [
    /**
     * 【2026-10-24 主人重排整套顺序 + 逐张改要求，当天下午再减到 8 张】
     *
     * 现在的 8 张顺序（也就是出图顺序）：
     *   1. 主图（商品原样 + 模特在使用 + 俄语卖点大字）
     *   2. 痛点对比（**同一件商品**的两种状态）
     *   3. 生活场景（**产品正在使用中**，不是摆着看）
     *   4. 细节①材质纹理
     *   5. 细节②结构部位（结构点由模型按商品自行判断）
     *   6. 细节③功能效果（防水/防油污，一擦即净，功能点也自行判断）
     *   7. 尺寸规格图（**要有背景**、只许一件商品）
     *   8. 联系卖家提醒图（只表达「有问题先联系卖家」）
     *
     * 演变过程（免得下一个人以为是随手砍的）：
     *   - 原第 1 张「卖点+痛点合成」拆成「主图」+「痛点对比」两张，
     *     因为主人要求「对比必须是同一件商品」，混在合成图里模型容易串货；
     *   - 原来的「售后保障图」整张删掉，只留「联系卖家提醒图」并大幅简化；
     *   - 2026-10-24 下午：主人转达「模特使用场景跟主图融合就行，不用单另张」，
     *     于是独立的「模特使用场景」取消、模特并进主图，总数 9 → 8；
     *   - 三张细节图分工：角度不同、部位不同、无 logo、材质观感一致，最后一张做功能效果。
     */
    {
      /**
       * 【2026-10-24 主人的最终版本】
       *
       * 下午先说「主图加上模特」，过了两小时又改了：
       * 「主图现在不加模特使用，更多的是在产品主体不变的情况下要大标题和介绍产品的详情，
       *   比如卖点、材质等，主体在图片中的位置可以按照排版换位置，
       *   比如在图片的右边时候信息就在左边。更多的是灵活放置，所有都是同一个位置的话很单调。」
       *
       * 所以这一张：
       *   1. **不放模特**（模特挪到第 3 张生活场景去展示「正在使用」）；
       *   2. 重点变成「商品原样 + 大标题 + 卖点/材质等详情文字」；
       *   3. 版式在三种里随机挑一种（商品在右 / 在左 / 在下），避免每套都一个样。
       */
      key: '01_main_image',
      title: '第 1 张 · 主图（商品原样 + 大字卖点详情）',
      bodyZh:
        `第1张：主图。产品：${product}。` +
        '**商品本身一个像素都不许改**：形状、颜色、材质、图案、配件、比例全部保持参考图原样。' +
        '**画面里不要出现人物、模特、手**——这一张只讲商品和文字。' +
        layoutSpec.zh +
        paletteZh +
        `背景：${palette ? `在 ${palette.tint} 的基础上做的极淡同色柔光（浅木/浅灰的质感可以保留，但整体要偏向这个色）` : '干净的浅色实景底（浅木桌面或浅灰布面），有柔和的空间层次'}，不要纯白、不要渐变纸。` +
        '文字内容（本张重点：**字体要大，要占满留白**）：' +
        '① 俄语主标题：字号是整张最大的（约画布宽度的 9-12%），粗体，2-3 行；' +
        '② 俄语副标题：一行手写体，字号约为主标题的 45%；' +
        '③ 三组「实心圆形图标 + 两行俄语」卖点：图标直径约画布宽度的 7%，' +
        `文字字号不小于画布宽度的 3.5%，内容分别是：${sp1} / ${sp2} / ${sp3}；` +
        '④ 一个圆形局部放大镜（直径约画布宽度 22%），里面放商品最值得说的一处细节，配一行俄语标注；' +
        '⑤ 一条圆角胶囊条，一行俄语短句，字号中等。' +
        '材质与细节的文字要具体（写清楚是什么材质、什么工艺、什么尺寸感），不要只写「高质量」这种空话。' +
        '硬规则：所有文字必须真的写出来、必须清晰可读、必须是俄语；' +
        '所有文字都排在**留白处**，绝不压在商品主体上；画面里只能出现**一件商品**。' +
        '禁忌：不要中文；不要把文字排得很小；不要纯白背景；不要第二件商品；不要人物或手；不要边框和台标。',
      bodyEn:
        `Card 1: main image. Product: ${product}. ` +
        '**Do not change the product itself at all**: shape, colour, material, pattern, accessories and proportions must all match the reference exactly. ' +
        '**No people, no models, no hands in this card** — this one is only about the product and the type. ' +
        layoutSpec.en +
        paletteEn +
        `Background: ${palette ? `built on ${palette.tint} with a very faint same-hue glow (a light wood or pale cloth texture may remain, but the overall cast must lean to that colour)` : 'a clean pale real-world surface (light wood or light grey cloth) with soft spatial depth'}; not pure white, not a gradient backdrop. ` +
        'Copy (the point of this card: **large type, filling the empty space**): ' +
        '(1) Russian headline: the biggest type on the card (about 9-12% of the canvas width), bold, 2-3 lines; ' +
        '(2) Russian subtitle: one handwritten line at about 45% of the headline size; ' +
        '(3) three "filled circular icon + two lines of Russian" selling points: icon diameter about 7% of the canvas width, ' +
        `text no smaller than 3.5% of the canvas width; the three items are: ${sp1} / ${sp2} / ${sp3}; ` +
        '(4) a circular detail magnifier (diameter about 22% of the canvas width) showing the single most convincing detail, with one Russian caption; ' +
        '(5) a rounded pill bar with one short Russian sentence at medium size. ' +
        'Material and detail copy must be specific (which material, which finish, what size it feels like) — never empty phrases like "high quality". ' +
        'Hard rules: every piece of text must actually be rendered, must be clearly legible, and must be Russian; ' +
        'all text sits **in the empty space** and must never overlap the product; only **one product** in the frame. ' +
        'Forbidden: Chinese text; tiny type; pure white background; a second product; any person or hand; borders or channel badges.',
    },
    {
      /**
       * 【2026-10-24 主人第三次改这一张】
       *
       * 第一版：「普通款 vs 我们的产品」，模型自由发挥 → 出现了桌布、收纳箱这种完全不相干的东西。
       * 第二版（我上一步）：收紧成「左右必须是同一件商品」，结果又太死——两边一模一样，看不出差别。
       * 第三版（主人这张图的定稿）：
       *   「对比图片可以是同一类产品，不能是完全不相干的产品，也不能是完全一致的产品」
       *
       * 也就是：**同一类目、明显差一档的另一个款**。
       * 遮阳棚对比廉价款遮阳棚、垃圾桶对比普通白桶、鱼缸对比有边条的老款——
       * 都还是同一类东西，但一眼能看出谁好谁差。
       */
      key: '02_pain_compare',
      title: '第 2 张 · 痛点对比（同类不同款）',
      bodyZh:
        `第2张：痛点对比图。产品：${product}。` +
        '**本张最关键的一条：左边必须是「同一类产品的普通款」，不是别的东西。**' +
        '左边要和右边是同一个类目（同一个用途），但**明显更差一档**——' +
        '材质更廉价、做工更糙、设计更老、或者用了很久显旧；让人一眼看出「普通款差在哪、我们的好在哪」。' +
        '三个都禁止：① 完全不相干的品类（比如卖垃圾桶却画桌布、卖收纳箱却画别的家居）——绝对不行；' +
        '② 和右边一模一样、看不出差别的同一件商品——也不行，那就没有对比意义；' +
        '③ 把右边那件商品画成另一个完全不同的款式（形状、颜色、功能都变了）——也不行。' +
        '**左边和右边必须在至少三处肉眼可辨的不同上**（这是本张唯一的评分标准，达不到就重画）：' +
        '① 颜色：左边更暗、更灰、更土，右边的颜色鲜亮饱和；' +
        '② 造型：左边更简单、更老式、更笨重，右边更利落、线条更干净；' +
        '③ 材质：左边塑料感/廉价布料/没有纹理，右边有真实纹理和光泽；' +
        '④ 体量：左边更小、更单薄、叶片/部件更稀疏，右边更饱满；' +
        '⑤ 配件：左边的盆/底座/附件更廉价，右边更讲究。' +
        '**自查**：把画面缩到看不清文字的尺寸时，仍然要能一眼分出左右不是同一个东西；' +
        '如果两边看起来只是「同一个东西换了背景和光线」，这张就不合格。' +
        '**明令禁止偷懒画法：不许把右边那张图原样复制到左边只改背景**。' +
        '镜头：平视，左右各占 40% 画面，中间一条细分隔线，两侧商品在画面里的大小一致。' +
        '背景：左侧是杂乱的室内/户外一角（偏暗、偏冷、有杂物），右侧是同一位置但收拾干净、明亮通透——' +
        '左右必须是同一类空间，只是状态不同，不要换成两个完全不同的场景。' +
        '光线：左侧低对比阴天感，右侧明亮自然光，两侧色温一致。' +
        '左侧（普通款）：同类产品的廉价款——样式普通、材质一般、颜色发灰发旧、摆放随意、整体低对比；' +
        '右侧（我们的产品）：**就是参考图里那一件商品**，颜色、材质、设计保持原样，干净崭新、摆放讲究、背景明亮通透、有柔和反光。' +
        '角标：左侧「Обычная версия」或「Обычная модель」，右侧「Наша продукция」或「Наш продукт」，都是俄语圆角小标签；' +
        '右侧再补一个小气泡框，用俄语简短说明它强在哪。' +
        '文案：顶部 2-3 行大号俄语痛点问句（字号最大）；底部一排三组「细线图标 + 两行俄语」卖点。' +
        '硬规则：文字必须真的写出来、必须是俄语、必须清晰；左右两件商品要让人看出是「同类，但档次不同」。' +
        '禁忌：不要中文；不要出现不相干的品类；不要让左右一模一样；不要写成长段说明。',
      bodyEn:
        `Card 2: pain comparison. Product: ${product}. ` +
        '**The single most important rule on this card: the left side must be "the ordinary version of the same product category" — not something else.** ' +
        'The left item must belong to the same category and serve the same purpose as the right one, but be **clearly one grade worse** — ' +
        'cheaper material, rougher build, older design, or visibly worn; the viewer must instantly see what the ordinary version lacks and why ours is better. ' +
        'Three things are forbidden: (1) an unrelated category (selling a bin but drawing a tablecloth, selling a storage box but drawing something else) — never; ' +
        '(2) an item identical to the right side with no visible difference — also no, that defeats the comparison; ' +
        '(3) turning the right-hand product into a completely different model (shape, colour and function all changed) — also no. ' +
        '**The left and right must differ in at least three visually obvious ways** (this is the only pass/fail test for this card): ' +
        '(1) Colour: the left is darker, greyer and duller; the right is vivid and saturated. ' +
        '(2) Shape: the left is plainer, more dated and clunkier; the right is cleaner and more refined. ' +
        '(3) Material: the left looks like cheap plastic or thin fabric with no texture; the right has real texture and sheen. ' +
        '(4) Mass: the left is smaller, thinner, with sparser leaves or parts; the right is fuller. ' +
        '(5) Fittings: the left pot, base or accessories are cheaper; the right ones are nicer. ' +
        '**Self-check**: shrink the image until the text is unreadable — you must still be able to tell at a glance that the two sides are not the same object. ' +
        'If the two sides only read as "the same thing with a different background and lighting", the card fails. ' +
        '**Never take the lazy shortcut of copying the right-hand image to the left and only changing the background.** ' +
        'Camera: eye level, each side taking 40% of the frame, a thin divider between them, both items at the same visual scale. ' +
        'Background: the left side is a cluttered indoor/outdoor corner (dim, cool, with odds and ends); the right side is the same kind of spot tidied up, bright and airy — ' +
        'both sides must be the same kind of space in a different state, never two unrelated scenes. ' +
        'Lighting: low-contrast overcast feel on the left, bright natural light on the right, the same colour temperature on both sides. ' +
        'Left (the ordinary version): a budget item of the same category — plain styling, mediocre material, greyed and dated colour, casually placed, low contrast overall; ' +
        'right (our product): **exactly the product in the reference image**, keeping its colour, material and design, clean and new, neatly placed, bright airy background, soft reflections. ' +
        'Badges: a Russian rounded tag on the left reading "Обычная версия" or "Обычная модель", and on the right "Наша продукция" or "Наш продукт"; ' +
        'add one small Russian speech bubble on the right briefly explaining why it is better. ' +
        'Copy: a 2-3 line large Russian pain-point question across the top (largest type); a row of three "thin-line icon + two lines of Russian" selling points at the bottom. ' +
        'Hard rules: every line must actually be rendered, must be Russian, must be legible; the two items must read as "same category, different grade". ' +
        'Forbidden: Chinese text; an unrelated category; two identical sides; paragraphs of explanation.',
    },
    {
      /**
       * 【2026-10-24 主人的最终版本】
       *   下午：「生活场景图要使用产品，不要光放着」
       *   过两小时：「生活场景里面加入模特，最好展示出模特正在使用」
       *
       * 所以模特的主场挪到这一张（主图已经明确不放人）：
       * 画面里是一个真实的俄罗斯模特，正在自然地**使用**这件商品，
       * 商品仍然是最清晰的，但人物要有真实存在感，不是只露一只手。
       */
      key: '03_lifestyle',
      title: '第 3 张 · 生活场景图（模特正在使用）',
      bodyZh:
        `第3张：生活场景图（模特正在使用）。产品：${product}。` +
        `背景：${sceneZh}，要有真实的空间层次（近景道具 / 中景人物与商品 / 远景环境）。` +
        '光线：自然侧光（窗边光），有明确的方向感和柔和投影，人物脸上有柔和轮廓光，不要硬闪。' +
        '人物：**画面里要有一位俄罗斯模特**（符合目标市场的样子，穿着与商品调性匹配、居家自然的衣服），' +
        '动作必须是**正在使用这件商品**——拿在手里用、正在开合、正在取用、正在操作都行，' +
        '身体朝向和视线都指向商品，让人一眼看出「她正在用它」，而不是站着摆拍。' +
        '半身或大半身构图都可以（露到腰或胸以上即可，不要求全身）；人物占画面 35-45%，商品占 35-45%。' +
        sizeAnchorZh +
        '**商品必须是画面里最清晰、最亮的部分**，人物可以稍微柔一点；两者不要互相遮挡，' +
        '模特的手要真的接触或操作到商品，不能悬空摆姿势。' +
        /**
         * 【2026-10-24 主人要求】「模特的使用可以按照商品的比例去修改人物手的比例，
         * 避免出现买家到手后觉得和图片上的不一样」
         *
         * 这是电商图最常见的一个坑：为了把商品画得显眼，把模特的手画小了，
         * 结果商品看起来比真的大一圈，买家收到说「和图上不一样」。
         * 所以这一条必须把「手与商品的相对大小」写死，并且明确要求按真实比例。
         */
        '**手的比例必须按商品的真实尺寸来定（这一条很重要）**：' +
        '先按这件商品的实际大小去推算手掌和手指该有多大，再画手，' +
        '让手看起来是**正常成年人**的手（成年女性手掌宽约 7-8 厘米），' +
        '**不许为了让商品显大就把手画小**，也**不许为了让商品显小就把手画大**。' +
        '握持处的细节要合理：手指该被商品挡住就挡住、指节要有自然的弯曲和受力，' +
        '商品被手捏住的地方可以有轻微形变或遮挡，但不能变形到认不出。' +
        '最终效果必须是：**买家拿到手之后，商品的大小和图上看到的一致**。' +
        '如果商品很小（首饰、口红这类），手可以占更大面积、按钮级别的细节就是主角；' +
        '如果商品很大（垃圾桶、收纳箱这类），手只占商品的一小部分，别把手画得像巨人的手。' +
        '道具：场景里放 2-3 个相关道具，轻微虚化，不要抢主体。' +
        '文字：画面不要任何文字，保持干净。' +
        '禁忌：不要产品孤零零摆着；不要只有一只手没有人物；不要模特在摆拍而没有真的在用；' +
        '不要产品像贴上去的；不要纯色背景；不要中文；' +
        '**不要手和商品比例失真**（手太小显得商品虚大、手太大显得商品虚小都不行）。',
      bodyEn:
        `Card 3: lifestyle scene with a model using the product. Product: ${product}. ` +
        `Background: ${sceneEn}, with real spatial depth (near props / mid-ground person and product / far environment). ` +
        'Lighting: natural side light from a window with a clear direction and soft shadows; a soft rim light on the model, no harsh flash. ' +
        'Model: **a Russian model must be present in the frame** (matching the target market, in casual clothes that suit the product), ' +
        'and her action must be **actually using the product** — holding it in use, opening it, taking from it, operating it — ' +
        'with her body and gaze directed at the product so it reads instantly as "she is using it", not posing. ' +
        'A half or three-quarter figure is enough (waist-up or chest-up; no full body required); the person takes 35-45% of the frame and the product 35-45%. ' +
        sizeAnchorEn +
        '**The product must be the sharpest, brightest element**; the person may be slightly softer. They must not block each other, ' +
        'and the model hands must genuinely touch or operate the product — never floating in a pose. ' +
        '**Hand scale must follow the product’s real size (this matters a lot)**: ' +
        'work out how large a palm and fingers would actually be for this product, then draw the hand at that size, ' +
        'so it reads as a **normal adult hand** (an adult woman’s palm is about 7-8 cm wide). ' +
        '**Never shrink the hand to make the product look bigger**, and **never enlarge the hand to make the product look smaller**. ' +
        'Contact details must be plausible: fingers correctly occluded by the product, knuckles naturally bent and bearing weight, ' +
        'and the product may show slight compression where it is gripped — but it must stay recognisable. ' +
        'The end result must be: **the product looks the same size in real life as it does in the picture**. ' +
        'If the product is small (jewellery, a lipstick), the hand may take more area and button-level detail becomes the hero; ' +
        'if the product is large (a bin, a storage box), the hand covers only a small part of it — do not draw a giant’s hand. ' +
        'Props: 2-3 relevant props in the scene, slightly out of focus; they must not compete with the product. ' +
        'Text: no text at all; keep it clean. ' +
        'Forbidden: the product standing alone; a lone hand without a person; a model posing instead of really using it; ' +
        'a product that looks pasted in; a plain solid background; Chinese text; ' +
        '**a distorted hand-to-product scale** (a small hand faking a bigger product, or a big hand faking a smaller one).',
    },
    /**
     * 三张细节图的分工（主人 2026-10-24 要求「三个不同角度、细节位置不能一样、
     * 不要有品牌标识、材质看起来要一致，并且能突出商品特性」）：
     *   4 = 材质纹理（45 度斜上方）
     *   5 = 结构做工（正侧面贴近）
     *   6 = 功能效果（俯视贴近，防水/防油污一擦即净）
     * 三张都必须**看不到完整商品**，所以不加构图安全区。
     */
    {
      key: '04_detail_texture',
      title: '第 4 张 · 细节①材质纹理',
      cropSafe: false,
      bodyZh:
        `第4张：细节图①——材质纹理。产品：${product}。` +
        '镜头：**45 度斜上方微距**，怼近到只看得见一小块表面，这一块占画面 92%，**看不到完整商品**。' +
        '位置：拍**商品的主表面**（最大的那块面），不要拍边角、不要拍接口。' +
        '焦点：材质的真实质感（织物经纬、皮面颗粒、磨砂、拉丝、涂层），清晰到能数清纹理。' +
        '背景：完全虚化的同色系柔光底，不能有任何可辨识的物体。' +
        '光线：侧逆光掠过表面，把纹理的凹凸勾出来，高光细而长。' +
        '景深：浅景深，只有对焦那一小块锐利。' +
        '文字：一个小号俄语标注，一条细引线指向纹理，只写材质名，不写品牌。' +
        '硬规则：**不要出现任何 logo、商标、烫印文字、吊牌**——如果有，移出画面或彻底虚化；' +
        '这一张的材质观感必须和第 5、6 张完全一致（同样的颜色、同样的质感），不能看起来像两种材料。' +
        '禁忌：不要完整商品；不要场景；不要平光把纹理拍平；不要 logo。',
      bodyEn:
        `Card 4: detail ① - material texture. Product: ${product}. ` +
        'Camera: **45-degree macro from above**, so close that only one small patch of surface is visible, filling 92% of the frame; **the whole product is not visible**. ' +
        'Position: shoot **the main surface** (the largest face), not a corner and not a joint. ' +
        'Focus: the real material character (fabric weave, leather grain, matte finish, brushed metal, coating); sharp enough to count the texture. ' +
        'Background: fully blurred soft same-tone ground; nothing recognisable. ' +
        'Lighting: side-backlight raking across the surface to reveal the relief; a thin long highlight. ' +
        'Depth of field: shallow; only the focused patch is sharp. ' +
        'Text: one small Russian annotation with a thin leader line naming the material only, no brand. ' +
        'Hard rules: **no logo, trademark, embossed lettering or hang tag anywhere** — move them out of frame or blur them completely; ' +
        'the material must look identical to cards 5 and 6 (same colour, same texture), never like a different material. ' +
        'Forbidden: the whole product, any scene, flat lighting that kills the texture, any logo.',
    },
    {
      key: '05_detail_structure',
      title: '第 5 张 · 细节②结构部位',
      cropSafe: false,
      bodyZh:
        `第5张：细节图②——结构与做工。产品：${product}。` +
        '镜头：**正侧面贴近微距**（必须和第 4 张的 45 度斜上方不同），只拍一个结构部位，占画面 92%，**看不到完整商品**。' +
        '位置：拍**和第 4 张不同的地方**——接缝、走线、拉链、包边、扣件、底部收口，任选一处最有说服力的。' +
        '焦点：结构关系要看得出来（怎么缝的、怎么扣的、怎么加固的）。' +
        '背景：纯深色或纯浅色单色底，完全虚化。' +
        '光线：硬侧光，强调缝线起伏和部件厚度。' +
        '景深：中等景深，结构部位全部清晰。' +
        '文字：一个俄语标注写结构名称，一条引线。' +
        '硬规则：**不要出现任何 logo、商标、烫印文字、吊牌**；材质观感必须和第 4、6 张一致；' +
        '**结构点由你自己按这件商品判断**：挑一处最能体现做工或结实程度的地方拍' +
        '（缝线、加固、接口、卡扣、包边、底部收口都行），不要写死某一种。' +
        '禁忌：不要拍成整机图；不要平光；不要和第 4 张拍同一个位置；不要 logo。',
      bodyEn:
        `Card 5: detail ② - structure and build. Product: ${product}. ` +
        'Camera: **close macro from the side** (must differ from card 5\'s 45-degree top view); one structural area only, filling 92% of the frame; **the whole product is not visible**. ' +
        'Position: shoot **a different area from card 5** — seam, stitching, zip, piping, fastener or bottom hem; pick the most convincing one. ' +
        'Focus: the construction must be readable (how it is stitched, fastened, reinforced). ' +
        'Background: a single flat tone (dark or light), fully blurred. ' +
        'Lighting: hard side light to emphasise stitch relief and part thickness. ' +
        'Depth of field: medium; the structural area entirely sharp. ' +
        'Text: one Russian annotation naming the structure, with one leader line. ' +
        'Hard rules: **no logo, trademark, embossed lettering or hang tag**; the material must match cards 4 and 6; ' +
        '**decide the structural point yourself from this product**: pick whatever best proves the build quality or strength ' +
        '(stitching, reinforcement, joint, clip, piping, bottom hem — any of them); do not hardcode one kind. ' +
        'Forbidden: the whole product, flat lighting, the same position as card 4, any logo.',
    },
    {
      key: '06_detail_feature',
      title: '第 6 张 · 细节③功能效果（防水/防油污）',
      cropSafe: false,
      bodyZh:
        `第6张：细节图③——功能实测效果。产品：${product}。` +
        '镜头：**俯视贴近微距**（必须和第 4、5 张的角度都不同），画面 92% 是商品的表面，**看不到完整商品**。' +
        '位置：拍**商品的表面**，并在表面上方做出「功能正在生效」的效果。' +
        '核心画面：表面落了一滴水或一摊油污，旁边一块干净的布或纸巾刚刚擦过，' +
        '**擦过的地方明显干净、没有残留、没有渗进去**，和没擦的地方形成清晰对比。' +
        '焦点：擦过与没擦过的交界线要锐利，一眼看出「一擦就干净」。' +
        '背景：完全虚化的单色底。' +
        '光线：侧光加一点顶光，让水珠/油滴有立体感，让干净区域有清爽的高光。' +
        '文字：一个俄语标注写功能点（防水 / 防油污 / 一擦即净），一条引线。' +
        '硬规则：**不要出现任何 logo、商标、吊牌**；材质观感必须和第 4、5 张一致；' +
        '**功能点由你自己按这件商品判断**：挑一个它最可能具备的功能来演示' +
        '（防水 / 防油污 / 易清洁 / 耐磨 / 一擦即净），实在判断不出来就做「表面一擦就干净」；' +
        '干净区域要看起来是真的干净，不能只是颜色浅。' +
        '禁忌：不要完整商品；不要场景；不要拍成一片狼藉；不要 logo。',
      bodyEn:
        `Card 6: detail ③ - functional proof. Product: ${product}. ` +
        'Camera: **close macro from directly above** (must differ from both card 4 and card 5); 92% of the frame is the product surface; **the whole product is not visible**. ' +
        'Position: shoot **the product surface** and show the feature taking effect on it. ' +
        'Core scene: a water droplet or an oil patch sits on the surface; a clean cloth or paper towel has just wiped across it; ' +
        '**the wiped area is obviously clean, with no residue and nothing soaked in**, in clear contrast with the untouched area. ' +
        'Focus: the boundary between wiped and un-wiped must be razor sharp so "wipe once and it is clean" is instantly readable. ' +
        'Background: fully blurred single tone. ' +
        'Lighting: side light plus a little top light so the droplet or oil reads three-dimensional and the clean area has a fresh highlight. ' +
        'Text: one Russian annotation naming the feature (water-repellent / oil-repellent / wipes clean), with one leader line. ' +
        'Hard rules: **no logo, trademark or hang tag**; the material must match cards 4 and 5; ' +
        '**decide the feature yourself from this product**: demonstrate whichever benefit it most plausibly has ' +
        '(water-repellent / oil-repellent / easy to clean / scratch-resistant / wipes clean), ' +
        'and if nothing is obvious default to "the surface wipes clean in one pass"; the clean area must look genuinely clean, not merely lighter in colour. ' +
        'Forbidden: the whole product, any scene, a messy surface, any logo.',
    },
    {
      key: '07_size_chart',
      title: '第 7 张 · 尺寸规格图',
      // 尺寸图必须写数字，要临时覆盖「不要添加原图没有的数字和尺寸」那条规范。
      allowDimensions: true,
      bodyZh:
        `第7张：尺寸规格图。产品：${product}。` +
        '镜头：正视图平视，**画面里只有一件商品、一个视角**，商品占画面 55%，四周留出标注空间。' +
        '背景：**必须有真实背景，不能是空白**——商品放在浅木桌面或浅灰台面上，' +
        '背后是同色系柔和的离焦环境（一点绿植、一块布、一面浅墙都可以），整体明亮干净；' +
        '不要纯白、不要纯灰、不要网格底、不要渐变纸。' +
        '光线：均匀柔光，投影很淡，不要压住标注线。' +
        '表现：商品实拍 + 半透明线稿轮廓叠加，标出关键尺寸。' +
        '标题：俄语「Размеры, см」，放在画面上方，字号大。' +
        sizeChartZh +
        '**每个尺寸都必须写出具体数值，格式是「俄语名称：阿拉伯数字 + см」**，' +
        '例如 «Высота: 12 см»、«Ширина: 8.5 см»、«Глубина: 4 см»。' +
        '数值用阿拉伯数字（12、8.5 这种），单位只写 см。' +
        '每个尺寸配一条双向箭头，箭头两端落在实际测量点上（顶部到顶部、左侧到左侧）。' +
        '数值按参考图里商品的实际长宽比例估算，小数点最多一位。' +
        '**本张最重要的一条：画面里只许出现一件商品、一个视角。' +
        '禁止拼图、禁止三联图、禁止九宫格、禁止分屏、禁止在主图里再嵌小图。**' +
        '即使参考图本身是拼好的多视角图，本张也只画其中一个完整视角。' +
        '禁忌：不要出现英制单位（in、inch、дюйм）；不要空白背景；不要多视角拼贴；不要堆无关参数。',
      bodyEn:
        `Card 7: size specification. Product: ${product}. ` +
        'Camera: front view at eye level; **only one product, one viewpoint**, occupying 55% of the frame, with margin around for annotations. ' +
        'Background: **a real background is required, never blank** — the product sits on a light wood tabletop or a pale grey surface, ' +
        'with a soft out-of-focus same-tone environment behind it (a little greenery, a cloth, a pale wall all work); bright and clean overall; ' +
        'not pure white, not plain grey, not a grid, not a gradient sheet. ' +
        'Lighting: even soft light with a faint shadow that never fights the annotation lines. ' +
        'Presentation: the real photo with a semi-transparent line-art outline overlaid, marking key dimensions. ' +
        'Heading: Russian "Размеры, см" across the top, in large type. ' +
        sizeChartEn +
        '**Every dimension must carry an explicit value, formatted as "Russian label: Arabic numeral + см"**, ' +
        'for example «Высота: 12 см», «Ширина: 8.5 см», «Глубина: 4 см». ' +
        'Use Arabic numerals (12, 8.5) and the unit см only. ' +
        'Each dimension gets a double-headed arrow whose ends sit on the real measuring points (top to top, left to left). ' +
        'Estimate the values from the real proportions of the product in the reference image; at most one decimal place. ' +
        '**The single most important rule here: only one product and one viewpoint may appear. ' +
        'No collage, no three-panel layout, no nine-grid, no split screen, no inset thumbnails inside the main image.** ' +
        'Even if the reference image is itself a multi-view collage, draw only one complete viewpoint on this card. ' +
        'Forbidden: imperial units (in, inch, дюйм); a blank background; multi-view collage; unrelated specifications.',
    },
    {
      /**
       * 【2026-10-24 主人又改回来了】
       *
       * 先是要求「最后两张只保留联系卖家图，只要一句俄语就行」，我照做简化成三块。
       * 当天下午主人给了两张成品图，说：
       *   「售后的生图要求按照之前的那一版生成如后两张一样，页面的颜色依照产品主体的颜色来定」
       *
       * 所以把 0.2.9.36 那版详细的「联系卖家步骤图」整张恢复回来，
       * 并且底色明确跟着**商品主体主色**的深→浅渐变走（有识别到主色就用具体色值）。
       *
       * 版式就是那八段：标题 / 副标题 / 白条 1 / 三列图标 / 白条 2 / 三步流程 / 白条 3 / 编号步骤 + 手机卡。
       */
      key: '08_contact_seller',
      title: '第 8 张 · 售后与联系卖家图',
      cropSafe: false,
      bodyZh:
        `第8张：售后与联系卖家图（怎么找到客服）。产品：${product}。` +
        '整体：3:4 竖版，**竖向渐变**底色，不要照片背景、不要纹理。' +
        paletteGroundZh +
        '版式从上到下严格按这八段，位置和占比不要变：' +
        '① 10%：一行大标题，白色粗体居中：«УВАЖАЕМЫЕ ПОКУПАТЕЛИ!»；' +
        '② 10%：两行白色常规字重居中：«Не торопитесь оставлять негативный отзыв.» / «Мы сможем Вам помочь!»；' +
        '③ 8%：一条**纯白圆角长条**，左右留边一致，条内深色俄语粗体居中：«ЕСЛИ У ВАС ВОЗНИКЛИ ВОПРОСЫ»；' +
        '④ 14%：等宽三列，白色细线图标 + 两行白色说明：' +
        '«Повреждена упаковка» / «Перепутан товар» / «Задержалась доставка»；' +
        '三个图标分别画：破损的外箱、被翻转的包裹、带时钟的货车；第一个图标要画成**本商品的箱子**；' +
        '⑤ 8%：第二条**纯白圆角长条**：«НАПИШИТЕ В ПОДДЕРЖКУ МАРКЕТПЛЕЙСА»；' +
        '⑥ 8%：一行三步流程，白色小图标 + 白色文字，中间用右箭头连接：«Профиль → Сообщения → Поддержка»；' +
        '⑦ 8%：第三条**纯白圆角长条**：«ЧТОБЫ СВЯЗАТЬСЯ С ПРОДАВЦОМ»；' +
        '⑧ 26%：左半是三行编号步骤，白色大字，行距均匀：' +
        '«1. Зайдите в карточку товара» / «2. Нажмите "вопросы"» / «3. Выбрать "Задать вопрос"»；' +
        '右半是一个**简化的手机界面示意卡**：白色圆角卡片，上方一行俄语商品名（用本商品的名称），' +
        '中间一行小评分和一个 «Вопросы» 按钮，底部一条蓝色 «В корзину» 按钮。' +
        '硬规则：所有文字必须是俄语而且必须真的写出来；白字压在渐变上要够对比；三条白长条必须等宽、圆角一致、左右留边一致。' +
        '禁忌：不要中文；不要真实平台 logo；不要把三条白色长条合成一句；不要把步骤写成一段；' +
        '不要把底色做成和商品无关的颜色。',
      bodyEn:
        `Card 8: after-sales and how-to-contact-the-seller card. Product: ${product}. ` +
        'Overall: 3:4 portrait with a **vertical gradient** ground; no photographic backdrop, no texture. ' +
        paletteGroundEn +
        'Layout top to bottom, keep these eight bands in this order with these proportions: ' +
        '(1) 10%: one bold white headline, centred: "УВАЖАЕМЫЕ ПОКУПАТЕЛИ!"; ' +
        '(2) 10%: two centred lines of regular white Russian: "Не торопитесь оставлять негативный отзыв." / "Мы сможем Вам помочь!"; ' +
        '(3) 8%: a **pure white rounded bar** with equal side margins, containing dark bold Russian, centred: "ЕСЛИ У ВАС ВОЗНИКЛИ ВОПРОСЫ"; ' +
        '(4) 14%: three equal columns, white thin-line glyphs with two lines of white Russian: ' +
        '"Повреждена упаковка" / "Перепутан товар" / "Задержалась доставка"; ' +
        'the three glyphs are: a dented shipping box, an overturned parcel, a van with a clock; the first box must look like **this product packaging**; ' +
        '(5) 8%: a second **pure white rounded bar**: "НАПИШИТЕ В ПОДДЕРЖКУ МАРКЕТПЛЕЙСА"; ' +
        '(6) 8%: a single three-step row, small white glyphs with white labels joined by right arrows: "Профиль → Сообщения → Поддержка"; ' +
        '(7) 8%: a third **pure white rounded bar**: "ЧТОБЫ СВЯЗАТЬСЯ С ПРОДАВЦОМ"; ' +
        '(8) 26%: the left half holds three numbered steps in large white type with even leading: ' +
        '«1. Зайдите в карточку товара» / «2. Нажмите "вопросы"» / «3. Выбрать "Задать вопрос"»; ' +
        'the right half is a **simplified phone-UI mock**: a white rounded card with one line of Russian product name on top (use this product name), ' +
        'a small rating row and a "Вопросы" button in the middle, and a blue "В корзину" button along the bottom. ' +
        'Hard rules: every piece of text must be Russian and must actually be rendered; white type over the gradient must stay legible; ' +
        'the three white bars must share one width, one corner radius and equal side margins. ' +
        'Forbidden: Chinese text; real marketplace logos; merging the three white bars into one sentence; writing the steps as a paragraph; ' +
        'a background colour unrelated to the product.',
    },
  ]

  return tasks.map((task, index) => {
    // 尺寸图要写数字，得把「不要添加原图没有的数字和尺寸」那条规范换掉，
    // 否则模型会严格遵守它、只在图上画空箭头不写数值（主人报的就是这个）。
    const specZh = task.allowDimensions
      ? OZON_SYSTEM_PROMPT.replace(
          '- 不要添加原图没有的数字和尺寸\n',
          '- 本张是尺寸图：**必须写出阿拉伯数字 + см**（这条覆盖上一条）\n',
        )
      : OZON_SYSTEM_PROMPT
    const specEn = task.allowDimensions
      ? OZON_SYSTEM_PROMPT_EN.replace(
          '- Do not add numbers or dimensions that are not present in the reference image.\n',
          '- This card is the size chart: **Arabic numerals + см are required** (this overrides the rule above).\n',
        )
      : OZON_SYSTEM_PROMPT_EN
    // 需要露出完整商品的卡片，前面加一条构图安全区（微距三张不加）。
    const framingZh = task.cropSafe === false ? '' : FRAMING_SAFE_AREA
    const framingEn = task.cropSafe === false ? '' : FRAMING_SAFE_AREA_EN
    return {
      index: index + 1,
      key: task.key,
      title: task.title,
      // 界面显示 + 用户编辑的是中文这一份。
      // **不带 PRODUCT_LOCK**：那是英文技术约束，不该出现在用户看到的中文提示词里。
      // 它在生成时才拼上去（`buildEffectivePrompt`），而且只在「有参考图」时才拼。
      prompt: `${framingZh}${specZh}${task.bodyZh}`,
      // 真正发给模型的是英文这一份。同样不带 PRODUCT_LOCK，理由同上。
      promptEn: `${framingEn}${specEn}${task.bodyEn}`,
    }
  })
}

/**
 * 【2026-10-24 主人要求】「套图不显示出图顺序这些内容，防止别的照抄」。
 *
 * 原来这里导出一个 OZON_CARD_PLAN，界面拿它把 8 张的标题按顺序列出来。
 * 那等于把整套配方摊在界面上，截个图就能被同行照做，所以整块去掉。
 * 顺序仍然固定写在下面的 tasks 数组里，只是不再暴露到界面。
 */

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
 *
 * 【2026-10-24 加了 main_color / main_color_hex】
 * 主人问「是否保证了背景、文字与商品颜色一致或相近」。
 * 光靠提示词喊「取商品主色」模型得自己猜，所以让视觉模型**顺手把主色读出来**，
 * 再把具体色值写进第 1 张主图的提示词。同一次请求多返回两个字段，不额外花钱。
 */
export const VISION_EXTRACT_PROMPT =
  'Ты эксперт по продажам на Ozon/Wildberries. Проанализируй товар на фото и выведи СТРОГО JSON без пояснений: ' +
  '{"product":"中文产品名","audience":"目标人群中文","sp1":"卖点1中文","sp2":"卖点2中文","sp3":"卖点3中文",' +
  '"sp1_ru":"卖点1俄语","sp2_ru":"卖点2俄语","sp3_ru":"卖点3俄语","pain":"用户痛点中文","diff":"差异化中文",' +
  '"main_color":"商品主体主色的中文叫法（如 淡紫色 / 雾霾蓝 / 米白）","main_color_hex":"该主色的十六进制，格式 #RRGGBB"}. ' +
  'sp1/sp2/sp3 — по-китайски (для интерфейса). sp1_ru/sp2_ru/sp3_ru — тот же смысл по-русски ' +
  '(этот текст печатается на картинке, поэтому обязательно по-русски). ' +
  'main_color_hex — это доминирующий цвет САМОГО ТОВАРА (не фона фото), возьми средний цвет его основной поверхности. ' +
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
  /** 商品主体主色的叫法（中文），例如「淡紫色」 */
  main_color?: string
  /** 商品主体主色的十六进制，形如 #C8B6E2 */
  main_color_hex?: string
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
