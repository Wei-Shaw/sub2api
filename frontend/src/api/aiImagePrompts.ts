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
- 商品保持原样，只换背景、排版和文字
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
- Keep the product exactly as in the reference; only change background, layout and text.
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
 * 生成 Ozon 主图脚本（**10 张**）。
 *
 * 【2026-10-16 主人定的新顺序与结构】
 *   主人指出「9 张全是我放进去的商品主体，太单调，需要一些商品的细节展示」，
 *   并且「多角度不只是正面/侧面/45度，还要俯视等其他角度」。
 *   所以重排成：**核心卖点打头 → 合规 → 场景 → 痛点对比 → 三张细节微距 → 尺寸 → 俯视 → 侧视**。
 *
 *   顺序是主人指定的：把原来的第 3 张（核心卖点）提到第 1 位，
 *   原来的第 1 张（合规）和第 2 张（场景）顺位往下移。
 *   **售后保障图按主人要求删掉**（它那 397 字全是要印的俄语客服文案，画面几乎没描述）。
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
  const pain = (ctx.pain || '').trim() || 'daily hassle'
  const diff = (ctx.diff || '').trim() || 'better material'
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
    {
      key: '01_selling_points',
      title: '第 1 张 · 核心卖点信息图',
      bodyZh:
        `第1张：核心卖点信息图。产品：${product}。` +
        '镜头：45度平视，产品居中偏下，占画面 60-70%。' +
        '背景：干净的浅色几何背景（浅灰或浅米），带极淡渐层，不要花纹和纹理。' +
        '光线：均匀柔光，几乎没有硬阴影，产品表面高光柔和。' +
        `构图：画面上方三分之一留白放卖点，三组卖点均匀分布。卖点：${sp1} / ${sp2} / ${sp3}，主文案突出 ${sp1}。` +
        '文字：三组俄语卖点，每组配一个小图标，图标风格统一。' +
        '禁忌：文字不超过三组，不要促销角标，不要边框，不要堆满文字。',
      bodyEn:
        `Card 1: key selling points infographic. Product: ${product}. ` +
        'Camera: 45-degree eye level. Product centred slightly low, occupying 60-70% of the frame. ' +
        'Background: clean pale geometric backdrop (light grey or light beige) with a very soft gradient; no patterns, no texture. ' +
        'Lighting: even soft light, almost no hard shadow, gentle highlights on the product surface. ' +
        `Composition: keep the upper third empty for the selling points; three groups distributed evenly. Selling points: ${sp1} / ${sp2} / ${sp3}; lead with ${sp1}. ` +
        'Text: three Russian selling points, each with one small icon; icons share the same style. ' +
        'Forbidden: more than three text groups, promo corner badges, borders, walls of text.',
    },
    {
      /**
       * 【2026-10-17 主人要求】原来第 2 张是「白底合规主图」。
       * 主人说「那个白底图片换成对应风格的模特使用的场景，这样显得不单调，重点是突出产品」，
       * 所以换成**模特使用场景**，但产品仍然是画面主角（模特只做陪衬）。
       *
       * ⚠️ 注意：Ozon/WB 的**第 1 张主图**通常要求白底。这张改成场景图之后，
       * 整套里就没有白底图了。如果平台审核卡这一条，需要单独补一张白底。
       */
      key: '02_model_scene',
      title: '第 2 张 · 模特使用场景',
      bodyZh:
        `第2张：模特使用场景。产品：${product}。` +
        '镜头：平视略低机位，产品占画面 55-65%，模特在画面一侧，只占 25-30%，不要抢主体。' +
        `背景：与产品调性一致的真实生活场景（${sceneZh}），有空间层次和自然光。` +
        '光线：自然窗光或柔和环境光，模特脸上有柔和轮廓光，不要硬闪。' +
        '人物：一位符合目标市场的俄罗斯模特，穿着与产品调性匹配，动作自然正在使用产品，' +
        '**手和眼神都要指向产品**，让视线落到产品上。' +
        '重点：产品必须是画面最亮、最清晰、最抓眼的部分；模特虚化一点没关系。' +
        '文字：画面不要任何文字，保持干净。' +
        '禁忌：不要纯白底，不要产品像贴上去的，不要模特占满画面。',
      bodyEn:
        `Card 2: model lifestyle scene. Product: ${product}. ` +
        'Camera: eye level, slightly low angle. Product occupies 55-65% of the frame; the model sits to one side taking only 25-30% and must not dominate. ' +
        `Background: a real-life setting matching the product (${sceneEn}), with spatial depth and natural light. ` +
        'Lighting: natural window light or soft ambient light; a soft rim light on the model, no harsh flash. ' +
        'Model: one Russian model matching the target market, dressed to match the product, naturally using the product. ' +
        '**Hands and gaze must point at the product** so the eye lands on it. ' +
        'Priority: the product must be the brightest, sharpest, most eye-catching element; the model may be slightly out of focus. ' +
        'Text: no text at all; keep it clean. ' +
        'Forbidden: plain white background, product that looks pasted in, model filling the frame.',
    },
    {
      key: '03_lifestyle',
      title: '第 3 张 · 生活场景图',
      bodyZh:
        `第3张：生活场景图。产品：${product}。` +
        '镜头：平视，产品占画面 50-60%，不要占满画面。' +
        `背景：${sceneZh}，要有真实的空间层次（近景道具 / 中景产品 / 远景环境）。` +
        '光线：自然侧光（窗边光），有明确的方向感和柔和投影。' +
        '道具：场景里放 2-3 个相关道具，轻微虚化，不要抢主体。' +
        '文字：画面不要任何文字，保持干净。' +
        '禁忌：不要摆拍感，不要产品像贴上去的，不要纯色背景。',
      bodyEn:
        `Card 3: lifestyle scene. Product: ${product}. ` +
        'Camera: eye level. Product occupies 50-60% of the frame; do not fill the frame. ' +
        `Background: ${sceneEn}, with real spatial depth (near props / mid-ground product / far environment). ` +
        'Lighting: natural side light from a window, with a clear direction and soft shadows. ' +
        'Props: 2-3 relevant props in the scene, slightly out of focus; they must not compete with the product. ' +
        'Text: no text at all; keep it clean. ' +
        'Forbidden: staged look, product that looks pasted in, plain solid background.',
    },
    {
      key: '04_pain_comparison',
      title: '第 4 张 · 痛点对比图',
      bodyZh:
        `第4张：痛点对比图。产品：${product}。` +
        '镜头：平视，左右两个产品各自 40% 画面，四周留边距。' +
        '构图：左右分屏，中间一条细分隔线。' +
        '背景：左侧灰调冷色、背景杂乱偏暗；右侧亮调暖色、背景干净通透。' +
        '左侧：「普通款」，画面偏暗偏闷。' +
        '右侧：「我们的产品」，画面明亮清爽。' +
        '光线：左侧低对比阴天感，右侧明亮自然光。' +
        `文字：俄语主文案「Ты тоже ${pain}?」放在画面上方，字号大、占一行。` +
        `右侧角标补一句俄语差异化：${diff}。` +
        '禁忌：不要写成大段说明文字，两侧产品不要画得一模一样。',
      bodyEn:
        `Card 4: pain point comparison. Product: ${product}. ` +
        'Camera: eye level. Each product takes 40% of the frame, with margins around. ' +
        'Composition: split screen left/right with a thin divider. ' +
        'Background: cluttered, dim, cool grey on the left; clean, airy, warm on the right. ' +
        'Left side: the "ordinary version", dim and dull. ' +
        'Right side: "our product", bright and fresh. ' +
        'Lighting: low-contrast overcast feel on the left, bright natural light on the right. ' +
        `Text: Russian headline "Ты тоже ${pain}?" across the top, large and on a single line. ` +
        `Add one Russian differentiator badge on the right side: ${diff}. ` +
        'Forbidden: paragraphs of explanation; the two sides must not look identical.',
    },
    {
      key: '05_material_macro',
      title: '第 5 张 · 细节·材质微距',
      // 微距卡片**不加**构图安全区：它本来就要「看不到完整商品」。
      cropSafe: false,
      bodyZh:
        `第5张：材质微距特写。产品：${product}。` +
        '镜头：微距怼近，只拍一个局部，局部占画面 95%，**看不到完整商品**。' +
        '焦点：材质纹理（金属拉丝、磨砂、织物、硅胶等），纹理必须清晰可辨。' +
        '背景：完全虚化，只留一片柔和的同色系底色，不能有任何可辨识的物体。' +
        '光线：侧逆光掠过表面，把纹理的凹凸勾出来。' +
        '景深：浅景深，只有对焦那一小块清晰。' +
        '文字：一个俄语短标注，用引线指向纹理位置。' +
        '禁忌：不要出现完整商品，不要出现场景，不要平光把纹理拍平。',
      bodyEn:
        `Card 5: material macro close-up. Product: ${product}. ` +
        'Camera: macro, extremely close. Only one local area is shown, filling 95% of the frame; **the whole product is not visible**. ' +
        'Focus: material texture (brushed metal, matte finish, fabric weave, silicone, etc.); the texture must be clearly readable. ' +
        'Background: fully blurred, just a soft same-tone surface; nothing recognisable. ' +
        'Lighting: side-backlight raking across the surface to reveal the relief of the texture. ' +
        'Depth of field: shallow; only the focused patch is sharp. ' +
        'Text: one short Russian annotation with a leader line pointing at the texture. ' +
        'Forbidden: the whole product, any scene, flat lighting that kills the texture.',
    },
    {
      key: '06_structure_closeup',
      title: '第 6 张 · 细节·结构特写',
      cropSafe: false,
      bodyZh:
        `第6张：结构特写。产品：${product}。` +
        '镜头：俯视 45 度局部特写，只拍结构部位，占画面 95%，**看不到完整商品**。' +
        '焦点：接口、螺纹、卡扣、密封圈、铰链等结构件，要看得出装配关系。' +
        '背景：纯深色或纯浅色单色底，完全虚化，用来把结构件托出来。' +
        '光线：硬侧光，强调层次和缝隙深度。' +
        '景深：中等景深，结构件全部清晰。' +
        '文字：俄语标注结构名称，一条引线。' +
        '禁忌：不要拍成整机图，不要平光。',
      bodyEn:
        `Card 6: structural close-up. Product: ${product}. ` +
        'Camera: 45-degree top-down close-up. Only the structural part is shown, filling 95% of the frame; **the whole product is not visible**. ' +
        'Focus: connector, thread, clip, sealing ring, hinge and similar parts; the assembly relationship must be visible. ' +
        'Background: a single flat tone (dark or light), fully blurred, to lift the structural part out. ' +
        'Lighting: hard side light to emphasise layering and the depth of seams. ' +
        'Depth of field: medium; the structural part is entirely sharp. ' +
        'Text: one Russian annotation naming the structure, with one leader line. ' +
        'Forbidden: showing the whole product, flat lighting.',
    },
    {
      key: '07_craft_closeup',
      title: '第 7 张 · 细节·工艺特写',
      cropSafe: false,
      bodyZh:
        `第7张：工艺特写。产品：${product}。` +
        '镜头：微距，占画面 95%，只拍工艺细节，**看不到完整商品**。' +
        '焦点：缝线、焊点、表面处理、logo 雕刻、边角收口。' +
        '背景：完全虚化的单色底，不要出现任何场景元素。' +
        '光线：斜射光，让工艺痕迹立体。' +
        '景深：浅景深，焦点落在工艺细节上。' +
        '文字：俄语短标注，一条引线。' +
        '禁忌：不要拍成整机图，不要用柔光把细节磨掉。',
      bodyEn:
        `Card 7: craftsmanship close-up. Product: ${product}. ` +
        'Camera: macro, filling 95% of the frame; only the craft detail is shown; **the whole product is not visible**. ' +
        'Focus: stitching, weld points, surface treatment, engraved logo, edge finishing. ' +
        'Background: fully blurred single tone; no scene elements at all. ' +
        'Lighting: raking light so the craft marks read as three-dimensional. ' +
        'Depth of field: shallow, focused on the craft detail. ' +
        'Text: one short Russian annotation with one leader line. ' +
        'Forbidden: showing the whole product, soft light that erases the detail.',
    },
    {
      key: '08_size_chart',
      title: '第 8 张 · 尺寸规格图',
      // 尺寸图必须写数字，要临时覆盖「不要添加原图没有的数字和尺寸」那条规范。
      allowDimensions: true,
      bodyZh:
        `第8张：尺寸规格图。产品：${product}。` +
        '镜头：正视图平视，产品占画面 60%，四周留出标注空间。' +
        '背景：纯白或极浅灰网格底，不要有场景。' +
        '光线：均匀平光，不要强烈投影，避免干扰标注线。' +
        '表现：产品实拍 + 半透明线稿轮廓叠加，标出关键尺寸。' +
        '标题：俄语「Размеры, см」，放在画面上方。' +
        '**每个尺寸都必须写出具体数值，格式是「俄语名称：阿拉伯数字 + см」**，' +
        '例如 «Высота: 12 см»、«Ширина: 8.5 см»、«Глубина: 4 см»。' +
        '数值用阿拉伯数字（12、8.5 这种），单位只写 см。' +
        '每个尺寸配一条双向箭头，箭头两端落在实际测量点上（顶部到顶部、左侧到左侧）。' +
        '数值按参考图里商品的实际长宽比例估算，小数点最多一位。' +
        '**禁止只写文字标签不写数字**，也禁止写成空箭头。' +
        '禁忌：不要出现英制单位（in、inch、дюйм），不要堆无关参数。',
      bodyEn:
        `Card 8: size specification. Product: ${product}. ` +
        'Camera: front view at eye level, product occupying 60% of the frame, with margin around for annotations. ' +
        'Background: pure white or a very light grey grid; no scene. ' +
        'Lighting: even flat light without strong shadows, so it does not fight the annotation lines. ' +
        'Presentation: the real photo with a semi-transparent line-art outline overlaid, marking key dimensions. ' +
        'Heading: Russian "Размеры, см" across the top. ' +
        '**Every dimension must carry an explicit value, formatted as "Russian label: Arabic numeral + см"**, ' +
        'for example «Высота: 12 см», «Ширина: 8.5 см», «Глубина: 4 см». ' +
        'Use Arabic numerals (12, 8.5) and the unit см only. ' +
        'Each dimension gets a double-headed arrow whose ends sit on the real measuring points (top to top, left to left). ' +
        'Estimate the values from the real proportions of the product in the reference image; at most one decimal place. ' +
        '**Never draw a label without a number**, and never leave an empty arrow. ' +
        'Forbidden: imperial units (in, inch, дюйм), unrelated specifications.',
    },
    {
      key: '09_top_view',
      title: '第 9 张 · 角度·正俯视',
      bodyZh:
        `第9张：正俯视角度图。产品：${product}。` +
        '镜头：正上方垂直俯视（top-down），镜头与产品顶面平行。' +
        '产品占画面 80%，居中。' +
        '背景：纯色台面或浅木纹台面，保持干净。' +
        '光线：均匀顶光，投影落在产品四周，方向一致。' +
        '重点：让买家看清整体形状、开口、内部结构或容量。' +
        '文字：不要文字。',
      bodyEn:
        `Card 9: top-down angle. Product: ${product}. ` +
        'Camera: straight down from directly above (top-down); the lens is parallel to the top surface of the product. ' +
        'Product occupies 80% of the frame, centred. ' +
        'Background: solid-colour tabletop or light wood grain, kept clean. ' +
        'Lighting: even top light; the shadow falls around the product with a consistent direction. ' +
        'Purpose: let the buyer see the overall shape, the opening, the interior structure or the capacity. ' +
        'Text: no text.',
    },
    {
      key: '10_side_view',
      title: '第 10 张 · 角度·正侧视',
      bodyZh:
        `第10张：正侧面角度图。产品：${product}。` +
        '镜头：正侧面平视（side view），镜头与产品侧面垂直。' +
        '产品占画面 80%，居中。' +
        '背景：纯色（浅灰或浅米），无杂物。' +
        '光线：侧光从一侧打过来，勾出厚度和轮廓。' +
        '重点：让买家看清厚度、高度、侧面结构与比例。' +
        '文字：不要文字。',
      bodyEn:
        `Card 10: pure side view. Product: ${product}. ` +
        'Camera: eye-level side view; the lens is perpendicular to the side face of the product. ' +
        'Product occupies 80% of the frame, centred. ' +
        'Background: solid colour (light grey or light beige), nothing else. ' +
        'Lighting: side light from one direction, outlining thickness and silhouette. ' +
        'Purpose: let the buyer see thickness, height, side structure and proportions. ' +
        'Text: no text.',
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
