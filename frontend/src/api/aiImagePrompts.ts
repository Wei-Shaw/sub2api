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
  只有「对比图」才允许左右各一件，而且必须是同一件商品
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
- One frame, one picture: a single product and a single viewpoint. Never make a collage, triptych, nine-grid or multi-view montage; only the comparison card may show two items side by side, and they must be the same product.
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
       * 【2026-10-24 主人转达的修改意见】「模特使用场景跟主图融合就行，不用单另张」、
       * 「主图加上模特」。所以原来独立的「模特使用场景」整张取消，
       * 模特改成主图里的陪衬：站在商品右后方、轻微虚化、正在使用商品，
       * 这样既有人物温度，又不抢商品主体，左侧还是留给大字卖点。
       */
      key: '01_main_image',
      title: '第 1 张 · 主图（商品 + 模特使用 + 俄语卖点大字）',
      bodyZh:
        `第1张：主图。产品：${product}。` +
        '镜头：平视略高机位，商品完整居中偏右，占画面 50-58%。' +
        '**商品本身一个像素都不许改**：形状、颜色、材质、图案、配件、比例全部保持参考图原样。' +
        '模特：右后方一位俄罗斯模特，轻微虚化（比商品模糊一档），正在自然地**使用**这件商品，' +
        '只占画面 18-25%，**不许抢主体**；手和视线都指向商品。模特不要挡住商品，也不要压到左侧文字区。' +
        '背景：干净的浅色实景底（浅木桌面或浅灰布面），有柔和的空间层次，不要纯白、不要渐变纸。' +
        '光线：左前方柔和窗光，右侧一点补光，商品表面有自然高光，投影淡淡落在右下方。' +
        '文字排版（本张重点：**字体要大，要占满空白处**）：' +
        '① 左上方 30% 区域：2-3 行大号俄语主标题，字号是整张最大的，粗体，颜色取商品同色系的深色；' +
        '② 主标题下面：一行手写体俄语副标题，字号约为主标题的 45%；' +
        '③ 左侧中部竖排三组「实心圆形图标 + 两行俄语」，图标直径约画布宽度的 7%，' +
        `文字字号不小于画布宽度的 3.5%，内容分别是：${sp1} / ${sp2} / ${sp3}；` +
        '④ 左下角一个圆形局部放大镜（直径约画布宽度 22%），里面放商品最值得说的一处细节，配一行俄语标注；' +
        '⑤ 底部一条圆角胶囊条，一行俄语短句，字号中等。' +
        '硬规则：所有文字必须真的写出来、必须清晰可读、必须是俄语；' +
        '所有文字都排在**空白处**，不许压在商品主体上，也不许压在模特脸上；画面里只能出现**一件商品**。' +
        '禁忌：不要中文；不要把文字排得很小；不要纯白背景；不要第二件商品；不要模特占满画面；不要边框和台标。',
      bodyEn:
        `Card 1: main image. Product: ${product}. ` +
        'Camera: eye level, slightly high. The product is complete, centred slightly right, occupying 50-58% of the frame. ' +
        '**Do not change the product itself at all**: shape, colour, material, pattern, accessories and proportions must all match the reference exactly. ' +
        'Model: one Russian model in the right background, slightly out of focus (one step softer than the product), naturally **using** the product, ' +
        'taking only 18-25% of the frame and **never dominating**; hands and gaze point at the product. The model must not cover the product or intrude into the left text area. ' +
        'Background: a clean pale real-world surface (light wood or light grey cloth) with soft spatial depth; not pure white, not a gradient backdrop. ' +
        'Lighting: soft window light from the front-left, a little fill from the right, natural highlights on the product, a faint shadow falling to the lower right. ' +
        'Typography (the point of this card: **large type, filling the empty areas**): ' +
        '(1) upper-left 30% area: a 2-3 line large Russian headline, the biggest type on the card, bold, in a dark tone from the product palette; ' +
        '(2) below it: one line of handwritten-style Russian subtitle at about 45% of the headline size; ' +
        '(3) mid-left, three stacked rows of "filled circular icon + two lines of Russian"; icon diameter about 7% of the canvas width, ' +
        `text no smaller than 3.5% of the canvas width; the three items are: ${sp1} / ${sp2} / ${sp3}; ` +
        '(4) lower-left: a circular magnifier inset (diameter about 22% of the canvas width) showing the single most convincing detail, with one Russian caption; ' +
        '(5) bottom: a rounded pill bar with one short Russian sentence at medium size. ' +
        'Hard rules: every piece of text must actually be rendered, must be clearly legible, and must be Russian; ' +
        'all text sits **in the empty areas** and must never overlap the product or the model face; only **one product** in the frame. ' +
        'Forbidden: Chinese text; tiny type; pure white background; a second product; a model filling the frame; borders or channel badges.',
    },
    {
      key: '02_pain_compare',
      title: '第 2 张 · 痛点对比（同一件商品）',
      bodyZh:
        `第2张：痛点对比图。产品：${product}。` +
        '**本张最关键的一条：左右两侧必须是同一件商品**——形状、颜色、材质、图案、配件完全一致，' +
        '**而且都必须是参考图里那一件商品**（不是别的类目、不是别的款式），' +
        '只是呈现状态不同。严禁出现第二种商品（桌布、收纳箱之类的东西一律不许出现），这一条压过其它所有要求。' +
        '镜头：平视，左右各占 40% 画面，中间一条细分隔线，两侧商品大小一致。' +
        '背景：左侧是杂乱的室内一角（偏暗、偏冷、有杂物），右侧是同一位置但收拾干净、明亮通透——' +
        '左右必须是同一个空间，只是状态不同，不要换成两个不同的房间。' +
        '光线：左侧低对比阴天感，右侧明亮自然光，两侧色温一致。' +
        '左侧（普通款 / 糟糕状态）：同一件商品，表面有污渍磨损、摆放随意、整体低对比；' +
        '右侧（我们的产品）：同一件商品，干净崭新、摆放讲究、背景明亮通透、有柔和反光。' +
        '文案：顶部一行大号俄语痛点问句（字号最大）；左侧角标一个红色叉号配一行俄语短句；' +
        '右侧角标一个绿色对勾配一行俄语短句；底部一行俄语结论。' +
        '硬规则：文字必须真的写出来、必须是俄语、必须清晰；左右两件商品的外形要一眼看出是同一款。' +
        '禁忌：不要中文；不要让两侧变成两种不同的商品；不要写成长段说明。',
      bodyEn:
        `Card 2: pain comparison. Product: ${product}. ` +
        '**The single most important rule on this card: both sides must be the same product** — identical shape, colour, material, pattern and accessories, ' +
        '**and both must be the product shown in the reference image** (not another category, not another style), with only the condition differing. ' +
        'Never introduce a second kind of product (no tablecloths, no storage boxes, nothing unrelated); this rule overrides everything else. ' +
        'Camera: eye level, each side taking 40% of the frame, a thin divider between them, both products at the same scale. ' +
        'Background: the left side is a cluttered corner of a room (dim, cool, with odds and ends); the right side is the same corner tidied up, bright and airy — ' +
        'both sides must be the same space in a different state, never two different rooms. ' +
        'Lighting: low-contrast overcast feel on the left, bright natural light on the right, the same colour temperature on both sides. ' +
        'Left (the ordinary one / bad condition): the same product with stains and wear, casually placed, low contrast overall; ' +
        'right (our product): the same product, clean and new, neatly placed, bright airy background, soft reflections. ' +
        'Text: one large Russian pain-point question across the top (largest type); a small red cross badge with one short Russian line on the left; ' +
        'a small green check badge with one short Russian line on the right; one Russian conclusion line at the bottom. ' +
        'Hard rules: every line must actually be rendered, must be Russian, must be legible; the two products must obviously be the same model. ' +
        'Forbidden: Chinese text; two different products; paragraphs of explanation.',
    },
    {
      /**
       * 【2026-10-24 主人转达的修改意见】「生活场景图要使用产品，不要光放着」。
       * 所以这张明确要求画面里**有人在用**这件商品，产品不是静物摆件。
       */
      key: '03_lifestyle',
      title: '第 3 张 · 生活场景图（正在使用中）',
      bodyZh:
        `第3张：生活场景图。产品：${product}。` +
        '镜头：平视，产品占画面 45-55%，不要占满画面。' +
        `背景：${sceneZh}，要有真实的空间层次（近景道具 / 中景产品 / 远景环境）。` +
        '光线：自然侧光（窗边光），有明确的方向感和柔和投影。' +
        '**关键：产品不能只是摆着看——画面里必须有人在用它**（手正在接触/操作商品，' +
        '动作要自然，比如正在取用、正在开合、正在使用），让「这个场景里它真的在用」一眼看得出来。' +
        '只露出手臂或半个人都可以，不必露全脸；人物不要抢主体，商品仍然是最清晰的。' +
        '道具：场景里放 2-3 个相关道具，轻微虚化，不要抢主体。' +
        '文字：画面不要任何文字，保持干净。' +
        '禁忌：不要产品孤零零摆着；不要摆拍感；不要产品像贴上去的；不要纯色背景。',
      bodyEn:
        `Card 3: lifestyle scene. Product: ${product}. ` +
        'Camera: eye level. Product occupies 45-55% of the frame; do not fill the frame. ' +
        `Background: ${sceneEn}, with real spatial depth (near props / mid-ground product / far environment). ` +
        'Lighting: natural side light from a window, with a clear direction and soft shadows. ' +
        '**Critical: the product must not just sit there as a still life — someone must be using it in the frame** ' +
        '(a hand touching or operating the product, a natural everyday action such as taking from it, opening it, using it), so it is obvious at a glance that it is in use. ' +
        'Only an arm or half a person needs to be visible; no full face required. The person must not dominate; the product stays the sharpest element. ' +
        'Props: 2-3 relevant props in the scene, slightly out of focus; they must not compete with the product. ' +
        'Text: no text at all; keep it clean. ' +
        'Forbidden: the product standing alone; a staged look; a product that looks pasted in; a plain solid background.',
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
       * 【2026-10-24 主人要求】「最后两张可以只保留联系卖家图，图中只需要有
       * 『产品有问题请及时与卖家联系不要给差评』的表达的俄语文字就可以，文字内容不需要一致」。
       *
       * 所以原来的第 4 张「售后保障图」整张删掉，这一张也大幅简化：
       * 只要一句核心意思，不要求三列图标、不要求步骤编号、不要求手机界面。
       */
      key: '08_contact_seller',
      title: '第 8 张 · 联系卖家提醒图',
      cropSafe: false,
      bodyZh:
        `第8张：联系卖家提醒图。产品：${product}。` +
        '整体：3:4 竖版，简洁干净的信息图。**这一张只表达一件事**：产品如果有问题，请先联系卖家，不要直接给差评。' +
        '背景：商品同色系的柔和渐变，或干净的浅色实底；不要照片、不要复杂装饰。' +
        '版式只有三块，从上到下：' +
        '① 顶部：一行俄语大标题（粗体，字号是整张最大的），意思是「产品有问题，请先联系我们」；' +
        '② 中部：2-3 行俄语正文，意思是「不要急着留差评，先联系卖家，我们会帮你解决」；' +
        '③ 底部：一条圆角胶囊条，里面一行俄语短句（例如「我们随时在线」）。' +
        '**文字内容不需要和上面写的俄语一字不差，只要意思对就行**，但必须是俄语、必须清晰可读、字号要够大。' +
        '硬规则：画面里不要出现三列图标、不要出现步骤编号、不要出现手机界面、不要出现二维码/电话/网址。' +
        '禁忌：不要中文；不要长段落；不要画太满，留白要多。',
      bodyEn:
        `Card 8: contact-the-seller notice. Product: ${product}. ` +
        'Overall: 3:4 portrait, a clean minimal infographic. **This card says exactly one thing**: if something is wrong with the product, contact the seller first — do not leave a negative review straight away. ' +
        'Background: a soft gradient in the product palette, or a clean pale solid; no photo, no busy decoration. ' +
        'Only three blocks, top to bottom: ' +
        '(1) top: one large Russian headline (bold, the biggest type on the card) meaning "something wrong with the product? contact us first"; ' +
        '(2) middle: 2-3 lines of Russian body copy meaning "do not rush to leave a bad review, contact the seller and we will help"; ' +
        '(3) bottom: a rounded pill bar with one short Russian line (for example "we are always online"). ' +
        '**The wording does not have to match the Russian above word for word** as long as the meaning is right, but it must be Russian, clearly legible and large enough. ' +
        'Hard rules: no three-column icons, no numbered steps, no phone-UI mock, no QR code, phone number or URL. ' +
        'Forbidden: Chinese text, long paragraphs, a crowded layout — leave plenty of white space.',
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
 * 这套脚本的「目录」：每一张的编号和标题，按**出图顺序**排。
 *
 * 【2026-10-24 主人要求】「重新排版套图的生成图片的顺序，顺序需要提前让我知道」——
 * 界面在点「生成脚本」之前就把这 9 行按顺序列出来，用户先看清顺序再决定。
 *
 * 放在这里而不是界面里手写一份，是因为手写的那份迟早会和真正生成的卡片脱节；
 * 这里直接从同一个 `buildOzonScript` 推导，改模板时目录自动跟着变。
 * 标题跟商品名无关，所以用一个空上下文算一次就够了。
 */
export const OZON_CARD_PLAN: ReadonlyArray<{ key: string; title: string }> = buildOzonScript({
  product: '',
}).map(({ key, title }) => ({ key, title }))

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
