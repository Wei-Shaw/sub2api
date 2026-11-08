import { describe, expect, it } from 'vitest'
import {
  HOT_LAYOUT_PROMPT,
  buildHotLayoutCards,
  extractJsonObject,
  isPartialShot,
  maskReferenceNumbers,
  normalizeShotKind,
  parseHotLayout,
  type HotLayout,
} from '../aiImageHot'

describe('爆款拆解：从模型回复里抠 JSON', () => {
  it('能抠出干净的 JSON', () => {
    const parsed = extractJsonObject('{"canvas":"3:4","background":"纯白"}')
    expect(parsed).toEqual({ canvas: '3:4', background: '纯白' })
  })

  it('JSON 前后夹带解释文字也能抠出来', () => {
    const parsed = extractJsonObject('好的，这是分析结果：\n{"canvas":"3:4"}\n希望有帮助！')
    expect(parsed).toEqual({ canvas: '3:4' })
  })

  it('能剥掉 Markdown 代码块围栏', () => {
    const parsed = extractJsonObject('```json\n{"canvas":"3:4"}\n```')
    expect(parsed).toEqual({ canvas: '3:4' })
  })

  // 桌面程序用的是贪婪正则 \{.*\}，出现两段 JSON 时会抠成一大段导致解析失败。
  it('回复里有两段 JSON 时只取第一段（贪婪正则会在这里翻车）', () => {
    const parsed = extractJsonObject('示例：{"a":"示例"}\n\n实际结果：{"canvas":"3:4","background":"纯白"}')
    expect(parsed).toEqual({ a: '示例' })
  })

  it('字符串里的花括号不会把括号计数带偏', () => {
    const parsed = extractJsonObject('{"notes":"用 { 和 } 做装饰","background":"白"}')
    expect(parsed).toEqual({ notes: '用 { 和 } 做装饰', background: '白' })
  })

  it('转义引号不会提前结束字符串', () => {
    const parsed = extractJsonObject('{"notes":"他叫 \\"小明\\"","background":"白"}')
    expect(parsed).toEqual({ notes: '他叫 "小明"', background: '白' })
  })

  it('没有花括号时返回 null', () => {
    expect(extractJsonObject('抱歉，我无法分析这张图')).toBeNull()
  })

  it('空字符串返回 null', () => {
    expect(extractJsonObject('')).toBeNull()
  })

  it('括号不配平时返回 null 而不是抛错', () => {
    expect(extractJsonObject('{"canvas":"3:4"')).toBeNull()
  })

  it('内容是数组时返回 null（我们要的是对象）', () => {
    expect(extractJsonObject('[1,2,3]')).toBeNull()
  })
})

describe('爆款拆解：单张参考图 → 一条版式', () => {
  /** 这是一个「完整」的模型回复，字段名就是提示词里要求的那些。 */
  const full = JSON.stringify({
    canvas: '3:4',
    background: '纯白，无渐变',
    palette: ['#C2185B', '#FFFFFF', '#333333'],
    product_slot: '画面中下部，占 55%，45 度俯视',
    graphics: '三列圆形图标 + 一条底部胶囊条',
    notes: '标题断成三行',
    blocks: [
      {
        zone: 'top',
        kind: 'headline',
        style: '粗体 大红 居中 三行',
        size_pct: 22,
        source_ru: 'ДОРОГИЕ ПОКУПАТЕЛИ',
        text_ru: 'УВАЖАЕМЫЕ ПОКУПАТЕЛИ',
      },
      {
        zone: 'middle',
        kind: 'icon-row',
        style: '三列等宽，图标圆底',
        size_pct: 30,
        source_ru: 'Упаковка / Товар / Доставка',
        text_ru: 'Упаковка / Изделие / Доставка',
      },
      {
        zone: 'bottom',
        kind: 'cta',
        style: '白字压在胶囊条上',
        size_pct: 12,
        source_ru: 'Используйте отзывы',
        text_ru: 'Оставьте отзыв о качестве',
      },
    ],
  })

  it('把完整回复解析成版式（含分区、占比、配色）', () => {
    const layout = parseHotLayout(full, 2)!
    expect(layout).not.toBeNull()
    expect(layout.index).toBe(2)
    expect(layout.canvas).toBe('3:4')
    expect(layout.background).toContain('纯白')
    expect(layout.palette).toEqual(['#C2185B', '#FFFFFF', '#333333'])
    expect(layout.productSlot).toContain('45 度俯视')
    expect(layout.graphics).toContain('圆形图标')
    expect(layout.notes).toContain('三行')
    expect(layout.blocks).toHaveLength(3)
    expect(layout.blocks[0].zone).toBe('top')
    expect(layout.blocks[0].kind).toBe('headline')
    expect(layout.blocks[0].sizePct).toBe(22)
    // 最关键的一条：给我们商品重写的俄语，和同行原文要分开存。
    expect(layout.blocks[0].textRu).toBe('УВАЖАЕМЫЕ ПОКУПАТЕЛИ')
    expect(layout.blocks[0].sourceRu).toBe('ДОРОГИЕ ПОКУПАТЕЛИ')
  })

  it('模型换着写键名也认（pos / type / text / origin）', () => {
    const raw = JSON.stringify({
      ratio: '4:5',
      bg: '灰底',
      sections: [
        { pos: 'upper', type: 'subline', text: 'Новый текст', origin: 'Старый текст' },
      ],
    })
    const layout = parseHotLayout(raw, 1)!
    expect(layout.canvas).toBe('4:5')
    expect(layout.background).toBe('灰底')
    expect(layout.blocks).toHaveLength(1)
    expect(layout.blocks[0].zone).toBe('upper')
    expect(layout.blocks[0].kind).toBe('subline')
    expect(layout.blocks[0].textRu).toBe('Новый текст')
    expect(layout.blocks[0].sourceRu).toBe('Старый текст')
  })

  it('没有改写文案时退回同行原文，而不是留空', () => {
    const raw = JSON.stringify({
      blocks: [{ zone: 'top', kind: 'headline', source_ru: 'Только原文' }],
    })
    const layout = parseHotLayout(raw, 1)!
    // 空着的话脚本里就是一个空引号，模型只能自己编。
    expect(layout.blocks[0].textRu).toBe('Только原文')
  })

  it('两个文案都没有的区块直接丢掉', () => {
    const raw = JSON.stringify({
      blocks: [
        { zone: 'top', kind: 'headline', text_ru: 'Есть текст' },
        { zone: 'middle', kind: 'text' },
        '这不是对象',
        null,
      ],
    })
    const layout = parseHotLayout(raw, 1)!
    expect(layout.blocks).toHaveLength(1)
  })

  it('size_pct 会被夹在 0~100 之间，非数字算 0', () => {
    const raw = JSON.stringify({
      blocks: [
        { zone: 'top', text_ru: 'A', size_pct: 180 },
        { zone: 'middle', text_ru: 'B', size_pct: -20 },
        { zone: 'bottom', text_ru: 'C', size_pct: '不知道' },
      ],
    })
    const layout = parseHotLayout(raw, 1)!
    expect(layout.blocks.map((item) => item.sizePct)).toEqual([100, 0, 0])
  })

  it('一个区块都没有时返回 null（说明模型没看懂图）', () => {
    expect(parseHotLayout(JSON.stringify({ canvas: '3:4', blocks: [] }), 1)).toBeNull()
    expect(parseHotLayout(JSON.stringify({ canvas: '3:4' }), 1)).toBeNull()
  })

  it('回复不是 JSON 时返回 null', () => {
    expect(parseHotLayout('抱歉，我分析不了这张图', 1)).toBeNull()
    expect(parseHotLayout('', 1)).toBeNull()
  })

  it('提示词要求同时看两张图，并要求区分同行原文与改写文案', () => {
    // 这两条是「信息位置不变 + 换成我们的商品」的前提，删掉功能就废了。
    expect(HOT_LAYOUT_PROMPT).toContain('ДВЕ картинки')
    expect(HOT_LAYOUT_PROMPT).toContain('source_ru')
    expect(HOT_LAYOUT_PROMPT).toContain('text_ru')
    // 位置不能变 → 必须要求保留顺序和占比。
    expect(HOT_LAYOUT_PROMPT).toContain('size_pct')
    expect(HOT_LAYOUT_PROMPT).toContain('сверху вниз')
    // 不许抄同行品牌。
    expect(HOT_LAYOUT_PROMPT).toContain('бренд')
  })
})

describe('爆款复刻：一张参考图 = 一张脚本 = 一张出图', () => {
  const layout = (index: number, text: string): HotLayout => ({
    index,
    canvas: '3:4',
    background: '纯白',
    palette: ['#C2185B', '#FFFFFF'],
    productSlot: '中部，占 55%',
    graphics: '三列圆形图标',
    notes: '',
    blocks: [
      { zone: 'top', kind: 'headline', style: '粗体居中', sizePct: 22, sourceRu: '原文', textRu: text },
      { zone: 'bottom', kind: 'cta', style: '胶囊条', sizePct: 12, sourceRu: '原文2', textRu: `${text} 2` },
    ],
  })

  it('几条版式就出几张卡片，数量跟参考图一致', () => {
    const cards = buildHotLayoutCards([layout(1, 'Первый'), layout(2, 'Второй'), layout(3, 'Третий')])
    expect(cards).toHaveLength(3)
    expect(cards.map((card) => card.index)).toEqual([1, 2, 3])
  })

  it('卡片标题按顺序编号，并标明复刻的是第几张参考图', () => {
    const cards = buildHotLayoutCards([layout(1, 'A'), layout(2, 'B')])
    expect(cards[0].title).toContain('第 1 张')
    expect(cards[0].title).toContain('参考图 1')
    expect(cards[1].title).toContain('第 2 张')
    expect(cards[1].title).toContain('参考图 2')
  })

  it('卡片 key 唯一（不然展开/还原会串张）', () => {
    const cards = buildHotLayoutCards([layout(1, 'A'), layout(2, 'B'), layout(3, 'C')])
    expect(new Set(cards.map((card) => card.key)).size).toBe(3)
  })

  it('拆出来的俄语原文会进到中文和英文两份提示词里', () => {
    const [card] = buildHotLayoutCards([layout(1, 'УВАЖАЕМЫЕ ПОКУПАТЕЛИ')])
    expect(card.prompt).toContain('УВАЖАЕМЫЕ ПОКУПАТЕЛИ')
    expect(card.promptEn).toContain('УВАЖАЕМЫЕ ПОКУПАТЕЛИ')
  })

  it('中文版把分区翻译成「顶部 / 底部」和大标题，用户看得懂', () => {
    const [card] = buildHotLayoutCards([layout(1, 'A')])
    expect(card.prompt).toContain('顶部')
    expect(card.prompt).toContain('底部')
    expect(card.prompt).toContain('大标题')
    expect(card.prompt).toContain('22%')
  })

  it('英文版用 zone / kind 原词，并强调不许改顺序和位置', () => {
    const [card] = buildHotLayoutCards([layout(1, 'A')])
    expect(card.promptEn).toContain('zone top')
    expect(card.promptEn).toContain('headline')
    expect(card.promptEn).toContain('do not change them')
  })

  it('两份提示词都带构图安全区（商品不许被裁）', () => {
    const [card] = buildHotLayoutCards([layout(1, 'A')])
    expect(card.prompt).toContain('【构图安全区】')
    expect(card.promptEn).toContain('[Framing safety]')
  })

  it('空数组返回空脚本，不会凭空造卡片', () => {
    expect(buildHotLayoutCards([])).toEqual([])
  })

  it('少了字段的版式也不会出 undefined', () => {
    const bare: HotLayout = {
      index: 1,
      canvas: '',
      background: '',
      palette: [],
      productSlot: '',
      graphics: '',
      notes: '',
      blocks: [{ zone: '', kind: '', style: '', sizePct: 0, sourceRu: '', textRu: 'Текст' }],
    }
    const [card] = buildHotLayoutCards([bare])
    expect(card.prompt).not.toContain('undefined')
    expect(card.promptEn).not.toContain('undefined')
    expect(card.prompt).toContain('Текст')
  })
})


/**
 * 2026-10-26 主人：「消除产品的特有信息是必须的」。
 *
 * 背景：豆包那 5 组效果图里，竞品的品牌/水印**全都留下了**
 * （UPCLOUD / Royal Trees / STARLEK）。只写一句「别抄品牌」不够，
 * 必须先把「具体是哪几处」列出来再逐条要求清掉。
 */
describe('爆款复刻：必须消除对方商品的特有信息', () => {
  const rawWithMarks = JSON.stringify({
    canvas: '3:4',
    forbidden_marks: 'логотип UPCLOUD в правом нижнем углу; водяной знак Royal Trees по центру; бирка на ножке',
    blocks: [
      { zone: 'top', kind: 'headline', style: 'bold', size_pct: 20, source_ru: 'КОМПЛЕКС', text_ru: 'КОМПЛЕКС' },
    ],
  })

  it('视觉拆解会把 forbidden_marks 一起读出来', () => {
    const layout = parseHotLayout(rawWithMarks, 1)!
    expect(layout.forbiddenMarks).toContain('UPCLOUD')
    expect(layout.forbiddenMarks).toContain('Royal Trees')
  })

  it('拆解提示词里要求模型列出「不许带过来的东西」', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('forbidden_marks')
    expect(HOT_LAYOUT_PROMPT).toContain('forbidden_marks обязательно')
  })

  it('卡片正文把「具体那几处」写进去，并要求一处都不许出现', () => {
    const card = buildHotLayoutCards([parseHotLayout(rawWithMarks, 1)!])[0]
    expect(card.prompt).toContain('必须消除对方商品的特有信息')
    expect(card.prompt).toContain('UPCLOUD')
    expect(card.prompt).toContain('一处都不许出现在成品里')
    expect(card.prompt).toContain('就是废片')
    expect(card.promptEn).toContain('remove every identifying trace')
    expect(card.promptEn).toContain('the image is a write-off')
  })

  it('没给品牌名时要求留白，绝不让模型自己编一个', () => {
    const card = buildHotLayoutCards([parseHotLayout(rawWithMarks, 1)!])[0]
    expect(card.prompt).toContain('直接留白或用同色块替代')
    expect(card.prompt).not.toContain('换成我们的品牌')
    expect(card.promptEn).toContain('leave it blank or cover it with a matching colour block')
  })

  it('给了品牌名就在原位置换成我们的（位置和字号照原位）', () => {
    const card = buildHotLayoutCards([parseHotLayout(rawWithMarks, 1)!], 'UPCLOUD-OURS')[0]
    expect(card.prompt).toContain('«UPCLOUD-OURS»')
    expect(card.prompt).toContain('位置和字号照原位')
    expect(card.promptEn).toContain('«UPCLOUD-OURS»')
  })

  it('兜底清单里覆盖了所有常见载体', () => {
    const card = buildHotLayoutCards([parseHotLayout(rawWithMarks, 1)!])[0]
    for (const word of ['品牌名', '店铺名', 'logo', '水印', '二维码', '型号', '吊牌', '专属花纹']) {
      expect(card.prompt).toContain(word)
    }
  })
})


/**
 * 2026-10-26 主人问「不能识别我放入的产品上的品牌吗」。
 * 能——同一次视觉请求里顺手读第二张图（我们的商品）上的品牌/水印。
 *
 * 但商品图上经常是**供应商的水印**（那 5 组里棕榈树是「锦和」、包是 huagingone），
 * 所以只用来预填，界面上必须看得见、改得掉。
 */
describe('爆款复刻：读我们自己商品图上的品牌', () => {
  const rawWithBrand = JSON.stringify({
    canvas: '3:4',
    our_brand: '锦和',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'ПАЛЬМА', text_ru: 'ПАЛЬМА' }],
  })

  it('提示词里要求读「НАШ товар」上的品牌，并且明说没看到就返回空', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('our_brand')
    expect(HOT_LAYOUT_PROMPT).toContain('верни пустую строку')
  })

  it('提示词禁止把对方品牌当成我们的', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('our_brand бери ТОЛЬКО со второго изображения')
  })

  it('解析能拿到 our_brand', () => {
    const layout = parseHotLayout(rawWithBrand, 1)!
    expect(layout.ourBrand).toBe('锦和')
  })

  it('没读到品牌时是空串，不会编一个', () => {
    const layout = parseHotLayout(JSON.stringify({ canvas: '3:4', blocks: [{ text_ru: 'X' }] }), 1)!
    expect(layout.ourBrand).toBe('')
  })
})


/**
 * 2026-10-26 主人：「先解决商品配色与背景配色，爆款复刻不需要图片里的一些信息
 * 与商品的颜色一致或相仿」。
 *
 * 真图暴露的问题：瓷砖那组，我们的是棕色瓷砖，背景墙却还是竞品的酒红色。
 * 所以拆成两条相反的要求：
 *   背景 → 换成我们商品的色系
 *   文字/图标/胶囊条 → 保持竞品原样，绝不跟着变色
 */
/**
 * 2026-10-27 主人选**方案 A**：「先解决商品配色与背景配色，爆款复刻不需要图片里的一些信息
 * 与商品的颜色一致或相仿」，收紧为：
 *   - 纯色 / 渐变底 → 换成我们商品的色系
 *   - **实景照片底 → 不换色**（拿棕榈树真图验证时，米色墙被推成绿墙，偏离原图太多）
 *   - 例外：照片里大面积的其实就是竞品商品本身（墙面是竞品瓷砖）→ 必须换
 */
describe('爆款复刻：背景换色（方案 A）', () => {
  const base = {
    canvas: '3:4',
    bg_color_zone: 'фон — однотонная заливка',
    our_product_color: '深棕',
    our_product_color_hex: '#5A4632',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'ПАНЕЛИ', text_ru: 'ПАНЕЛИ' }],
  }

  const build = (extra: Record<string, unknown>) =>
    buildHotLayoutCards([parseHotLayout(JSON.stringify({ ...base, ...extra }), 1)!])[0]

  it('拆解会读 bg_is_photo / bg_has_competitor_product 两个布尔', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('bg_is_photo')
    expect(HOT_LAYOUT_PROMPT).toContain('bg_has_competitor_product')

    const layout = parseHotLayout(
      JSON.stringify({ ...base, bg_is_photo: 'true', bg_has_competitor_product: 'false' }),
      1,
    )!
    expect(layout.bgIsPhoto).toBe(true)
    expect(layout.bgHasCompetitorProduct).toBe(false)
  })

  it('布尔写成俄语/字符串也认，且默认 false（宁可保守不换色）', () => {
    const p1 = parseHotLayout(JSON.stringify({ ...base, bg_is_photo: 'да' }), 1)!
    expect(p1.bgIsPhoto).toBe(true)

    const p2 = parseHotLayout(JSON.stringify({ ...base, bg_is_photo: 'нет' }), 1)!
    expect(p2.bgIsPhoto).toBe(false)

    const p3 = parseHotLayout(JSON.stringify({ ...base }), 1)!
    expect(p3.bgIsPhoto).toBe(false)
    expect(p3.bgHasCompetitorProduct).toBe(false)
  })

  it('纯色底 → 换成我们的色系，并给出具体色值区间', () => {
    const card = build({ bg_is_photo: 'false' })
    expect(card.prompt).toContain('背景换色（只换背景，别的一律不动）')
    expect(card.prompt).toContain('深棕（#5A4632）')
    expect(card.prompt).toContain('不要保留竞品商品的颜色')
    expect(card.promptEn).toContain('Background recolour (background only')
  })

  it('★ 实景照片底 → 不换色，原样保留照片', () => {
    const card = build({ bg_is_photo: 'true', bg_has_competitor_product: 'false' })
    expect(card.prompt).toContain('背景不换色（实景照片原样保留）')
    expect(card.prompt).toContain('必须原样保留这张照片')
    expect(card.prompt).toContain('不许把背景墙或任何大面积区域改成别的颜色')
    // 不能再出现「换成我们色系」那种话
    expect(card.prompt).not.toContain('背景换色（只换背景')
    expect(card.promptEn).toContain('Do NOT recolour the background')
  })

  it('★ 实景照片但背景就是竞品商品 → 仍然要换（优先级更高）', () => {
    const card = build({ bg_is_photo: 'true', bg_has_competitor_product: 'true' })
    expect(card.prompt).toContain('背景换色（只换背景')
    expect(card.prompt).toContain('必须整块换成我们商品的色系')
    expect(card.prompt).not.toContain('背景不换色')
  })

  it('两种分支下，文字/图标/胶囊条都不许跟着变色', () => {
    for (const extra of [
      { bg_is_photo: 'false' },
      { bg_is_photo: 'true', bg_has_competitor_product: 'false' },
    ]) {
      const card = build(extra)
      for (const word of ['文字颜色', '图标颜色', '胶囊条']) {
        expect(card.prompt).toContain(word)
      }
    }
  })
})

describe('爆款复刻：商品主体必须和商品图一致', () => {
  const raw = JSON.stringify({
    canvas: '3:4',
    structure_lock: '5 стеблей, примерно 18 листовых пучков, чёрный матовый цилиндрический горшок',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'ПАЛЬМА', text_ru: 'ПАЛЬМА' }],
  })

  it('拆解要求数清楚茎数/叶数/盆，并禁止编造看不见的元素', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('structure_lock')
    expect(HOT_LAYOUT_PROMPT).toContain('количество стеблей')
    expect(HOT_LAYOUT_PROMPT).toContain('НЕ придумывай элементов')
    expect(HOT_LAYOUT_PROMPT).toContain('structure_lock заполняй по ВТОРОМУ изображению')
  })

  it('解析能拿到结构清单', () => {
    const layout = parseHotLayout(raw, 1)
    expect(layout!.structureLock).toContain('5 стеблей')
    expect(layout!.structureLock).toContain('горшок')
  })

  it('卡片正文把结构清单写进去，并要求逐项对上', () => {
    const card = buildHotLayoutCards([parseHotLayout(raw, 1)!])[0]
    expect(card.prompt).toContain('不许加东西、不许少东西、不许美化')
    expect(card.prompt).toContain('5 стеблей')
    expect(card.prompt).toContain('逐项对得上')
    expect(card.promptEn).toContain('add nothing, drop nothing, beautify nothing')
  })

  it('明确列出禁止项：加叶、少枝、画更饱满、换更好的盆、加原图没有的装饰', () => {
    const card = buildHotLayoutCards([parseHotLayout(raw, 1)!])[0]
    for (const word of ['多加叶片', '少画枝条', '画得更饱满', '换成更好看的盆', '原图没有的装饰']) {
      expect(card.prompt).toContain(word)
    }
  })

  it('把「可以变」写清楚，避免模型什么都不敢动', () => {
    const card = buildHotLayoutCards([parseHotLayout(raw, 1)!])[0]
    for (const word of ['光线方向', '背景', '接触阴影', '整体缩放', '不超过 15 度']) {
      expect(card.prompt).toContain(word)
    }
  })

  it('给一个可执行的数数自查，并说明这是实物复刻不是设计稿', () => {
    const card = buildHotLayoutCards([parseHotLayout(raw, 1)!])[0]
    expect(card.prompt).toContain('从左到右数一遍叶片或部件')
    expect(card.prompt).toContain('实物复刻')
    expect(card.prompt).toContain('宁可画得不那么完美')
  })

  it('没有结构清单时也能出文，不会 undefined', () => {
    const bare = JSON.stringify({ canvas: '3:4', blocks: [{ text_ru: 'X' }] })
    const card = buildHotLayoutCards([parseHotLayout(bare, 1)!])[0]
    expect(card.prompt).not.toContain('undefined')
    expect(card.prompt).toContain('不许加东西、不许少东西、不许美化')
  })
})


/**
 * 2026-10-27 主人：「观赏、使用、放置这几类产品都检查一下，以防出现后续继续修改的情况」。
 *
 * 检查出两个洞，都在这里钉住：
 *   ①「不许增删」会被字面理解成「连同参考图里的猫/人/内容物一起删掉」
 *   ② 爆款复刻没有尺寸输入，图上没印数字的商品（猫爬架 620mm、垃圾桶 40cm）会被模型编一个
 */
describe('三类商品（观赏 / 使用 / 放置）的覆盖检查', () => {
  const base = {
    canvas: '3:4',
    bg_is_photo: 'true',
    bg_has_competitor_product: 'false',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'X', text_ru: 'X' }],
  }
  const layoutOf = (extra: Record<string, unknown> = {}) =>
    parseHotLayout(JSON.stringify({ ...base, ...extra }), 1)!

  it('使用类（猫爬架）：明确说猫/人/道具必须按参考图保留', () => {
    const card = buildHotLayoutCards([layoutOf({ structure_lock: '3 уровня, когтеточка, лесенка' })])[0]
    expect(card.prompt).toContain('这一条只约束商品本体')
    expect(card.prompt).toContain('趴在猫爬架上的猫')
    expect(card.prompt).toContain('必须按参考图原样保留')
    expect(card.promptEn).toContain('constrains the product body only')
  })

  it('放置类（收纳）：参考图画了装满东西，就照画，不要清空', () => {
    const card = buildHotLayoutCards([layoutOf({ structure_lock: 'контейнер 40 л, крышка' })])[0]
    expect(card.prompt).toContain('里面装着东西')
    expect(card.prompt).toContain('不要清空')
    expect(card.promptEn).toContain('do not empty it')
  })

  it('观赏类：主体一致性仍在（叶子数量、盆都不能变）', () => {
    const card = buildHotLayoutCards([
      layoutOf({ structure_lock: '6 стеблей, около 18 листовых пучков, чёрный горшок' }),
    ])[0]
    expect(card.prompt).toContain('不许加东西、不许少东西、不许美化')
    expect(card.prompt).toContain('6 стеблей')
  })

  it('填了尺寸：写进正文，并换算成人体参照', () => {
    const card = buildHotLayoutCards([layoutOf()], '', '高 62 厘米')[0]
    expect(card.prompt).toContain('这件商品的实际尺寸是 高 62 厘米')
    expect(card.prompt).toContain('必须改掉换成这个')
    // 62cm / 165 ≈ 38% -> 腰到胸口这一档
    expect(card.prompt).toContain('腰到胸口')
    expect(card.promptEn).toContain('the actual size of this product is')
  })

  it('没填尺寸：不出现尺寸块，也不会 undefined', () => {
    const card = buildHotLayoutCards([layoutOf()])[0]
    expect(card.prompt).not.toContain('这件商品的实际尺寸是')
    expect(card.prompt).not.toContain('undefined')
  })

  it('填了乱七八糟的尺寸也不会崩（解析不出就不加人体参照）', () => {
    const card = buildHotLayoutCards([layoutOf()], '', '大号')[0]
    expect(card.prompt).toContain('这件商品的实际尺寸是 大号')
    expect(card.prompt).not.toContain('undefined')
  })
})


/**
 * 2026-10-27 主人：「使用类的在背景方面以参考图的背景为主」。
 *
 * 使用类（猫爬架、工具、家具）的参考图背景就是「使用现场」，
 * 换成另一个房间等于把「这东西用在哪」这个信息丢了。
 * 所以这一类**一律照参考图，不换色、不换房间**。
 */
describe('商品类别：使用类的背景必须照参考图', () => {
  const base = {
    canvas: '3:4',
    bg_is_photo: 'true',
    bg_has_competitor_product: 'false',
    our_product_color: '原木色',
    our_product_color_hex: '#C89B6A',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'X', text_ru: 'X' }],
  }
  const build = (extra: Record<string, unknown>) =>
    buildHotLayoutCards([parseHotLayout(JSON.stringify({ ...base, ...extra }), 1)!])[0]

  it('拆解要求判断三类之一', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('product_class')
    expect(HOT_LAYOUT_PROMPT).toContain('use | decor | storage')
  })

  it('类别写中文/俄文/英文都能归一', () => {
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: 'use' }), 1)!.productClass).toBe('use')
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: 'использование' }), 1)!.productClass).toBe('use')
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: '使用' }), 1)!.productClass).toBe('use')
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: 'storage' }), 1)!.productClass).toBe('storage')
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: '收纳' }), 1)!.productClass).toBe('storage')
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: 'decor' }), 1)!.productClass).toBe('decor')
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: '观赏' }), 1)!.productClass).toBe('decor')
  })

  it('认不出来就是空串（按最保守处理）', () => {
    expect(parseHotLayout(JSON.stringify({ ...base, product_class: '???' }), 1)!.productClass).toBe('')
    expect(parseHotLayout(JSON.stringify({ ...base }), 1)!.productClass).toBe('')
  })

  it('★ 使用类：背景照参考图，禁止换房间/换墙/换地板/改纯色', () => {
    const card = build({ product_class: 'use' })
    expect(card.prompt).toContain('背景以参考图为主（使用类）')
    expect(card.prompt).toContain('不要换房间')
    for (const word of ['换成另一个房间', '换一面墙', '换地板材质', '把背景改成纯色或渐变']) {
      expect(card.prompt).toContain(word)
    }
    expect(card.prompt).toContain('因为要和我们商品的色系搭配而改动背景颜色')
    expect(card.promptEn).toContain('Background follows the reference (functional product)')
  })

  it('★ 使用类的分支优先于「照片不换色」和「换色」两条', () => {
    // 即使背景是纯色底（本该换色），使用类也不换
    const flat = build({ product_class: 'use', bg_is_photo: 'false' })
    expect(flat.prompt).toContain('背景以参考图为主（使用类）')
    expect(flat.prompt).not.toContain('背景换色（只换背景')

    // 即使背景里有竞品商品（本该换色），使用类仍然照参考图
    const withCompetitor = build({ product_class: 'use', bg_has_competitor_product: 'true' })
    expect(withCompetitor.prompt).toContain('背景以参考图为主（使用类）')
  })

  it('非使用类仍走原来的照片/纯色两条分支', () => {
    const decor = build({ product_class: 'decor' })
    expect(decor.prompt).toContain('背景不换色（实景照片原样保留）')
    expect(decor.prompt).not.toContain('背景以参考图为主（使用类）')

    const storage = build({ product_class: 'storage', bg_is_photo: 'false' })
    expect(storage.prompt).toContain('背景换色（只换背景')
  })
})


/**
 * 2026-10-27 主人：「第一张的图的背景与生成的背景完全不一致」。
 *
 * 根因：模型把**第 2 张（我们的商品图）的背景**也借过来了——
 * 商品图左边有一扇带枝影的窗，出图就多了一扇参考图上根本没有的窗，
 * 地板材质和颜色也跟着变成了商品图那种暖木色。
 *
 * 所以补一条：商品图只取商品本体，它自己的背景一律丢弃。
 */
describe('商品图的背景一律丢弃（只取商品本体）', () => {
  const base = {
    canvas: '3:4',
    bg_is_photo: 'true',
    bg_has_competitor_product: 'false',
    product_class: 'use',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'X', text_ru: 'X' }],
  }
  const build = (extra: Record<string, unknown> = {}) =>
    buildHotLayoutCards([parseHotLayout(JSON.stringify({ ...base, ...extra }), 1)!])[0]

  it('明确要求丢弃商品图的背景，只取商品本体', () => {
    const card = build()
    expect(card.prompt).toContain('只取商品本体，它的背景一律丢弃')
    expect(card.prompt).toContain('就当这件商品已经被抠出来、放在透明底上')
    expect(card.prompt).toContain('只能来自第 1 张参考图')
    expect(card.promptEn).toContain('discard its background entirely')
  })

  it('给出一句「参考图没有窗户就一处都不许有」的负向清单', () => {
    const card = build()
    expect(card.prompt).toContain('参考图没有窗户就一处都不许有')
    expect(card.prompt).toContain('参考图有窗帘就画窗帘、没有就不画')
    expect(card.promptEn).toContain('there must be no window at all')
  })

  it('使用类分支里也再强调一次，避免两处规则打架', () => {
    const card = build()
    expect(card.prompt).toContain('更不许拿我们商品图的背景来凑')
    expect(card.promptEn).toContain('never borrow the background from our product photo')
  })

  it('观赏类也一样受这条约束（不限使用类）', () => {
    const card = build({ product_class: 'decor' })
    expect(card.prompt).toContain('只取商品本体，它的背景一律丢弃')
  })
})


/**
 * 2026-10-27 实测发现的连锁问题：补了「商品图背景一律丢弃」之后，
 * 猫反而被整只删掉了。所以「动物/人必须出现」要单独写死，不能只说「保留」。
 */
describe('参考图里的动物或人必须出现在成品里', () => {
  const raw = JSON.stringify({
    canvas: '3:4',
    product_class: 'use',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'X', text_ru: 'X' }],
  })
  const card = buildHotLayoutCards([parseHotLayout(raw, 1)!])[0]

  it('中文版：一只/一个都不能少，少了算废片', () => {
    expect(card.prompt).toContain('成品里必须照样出现，一只/一个都不能少')
    expect(card.prompt).toContain('少画了动物或人，这一张同样算废片')
    expect(card.prompt).toContain('商品图里没有他们，不是删掉的理由')
  })

  it('英文版同步', () => {
    expect(card.promptEn).toContain('that animal or person MUST appear in the output')
    expect(card.promptEn).toContain('dropping an animal or a person makes the card a write-off')
  })
})


/**
 * 2026-10-27 主人：「要布偶猫蜷着」。
 *
 * 之前写的是「品种和毛色照参考图」，于是出图把参考图那只橘色缅因猫画进了碗里，
 * 而且比碗还大、两侧溢出碗沿。
 * 正确做法：动物以**我们商品图里那只**为准（品种/毛色/姿态都照商品图），
 * 参考图的动物只说明「这里要有一只动物」。
 */
describe('动物以商品图那只为准', () => {
  const raw = JSON.stringify({
    canvas: '3:4',
    product_class: 'use',
    blocks: [{ zone: 'top', kind: 'headline', size_pct: 20, source_ru: 'X', text_ru: 'X' }],
  })
  const card = buildHotLayoutCards([parseHotLayout(raw, 1)!])[0]

  it('明确要求用商品图里那只，品种毛色姿态都照商品图', () => {
    expect(card.prompt).toContain('动物以我们商品图（第 2 张）里的那只为准')
    expect(card.prompt).toContain('品种、毛色、姿态都照商品图')
    expect(card.prompt).toContain('布偶猫蜷着睡在碗里')
    expect(card.promptEn).toContain('Use the animal from our product photo')
  })

  it('不再要求照抄参考图那只的品种毛色', () => {
    expect(card.prompt).toContain('不要照抄它那只的品种和毛色')
    expect(card.prompt).not.toContain('品种和毛色照参考图')
    expect(card.promptEn).toContain('do not copy that one')
  })

  it('商品图里没有动物时才退回参考图那只', () => {
    expect(card.prompt).toContain('只有我们商品图里根本没有动物时，才用参考图里那只')
  })

  it('体型必须装得进碗，不许骑在碗沿上', () => {
    expect(card.prompt).toContain('完整地躺进去、身体不超出内壁边缘')
    expect(card.prompt).toContain('不要硬塞，也不要让它骑在碗沿上')
  })

  it('提示词里没有 NaN / undefined', () => {
    expect(card.prompt.includes('NaN')).toBe(false)
    expect(card.prompt.includes('undefined')).toBe(false)
    expect(card.promptEn.includes('NaN')).toBe(false)
  })
})


/**
 * 2026-10-31 主人：
 * 「爆款复刻跟套图本质不是同一个东西，但是长久的分类是一致的，
 *   别拿套图的所有要求去限制爆款复刻。在放入商品的图片时由模型去解析图片中商品的展示，
 *   去判断商品是如何展示的，例如展示了商品一个细节、商品的材质等等，
 *   判断出来之后依照我自己的产品去生成对应的图片。」
 *
 * 所以核心是两件事：
 *   ① 拆解时**读得出**这张参考图是怎么展示商品的（shot）；
 *   ② 局部图（detail / scale）**不能**再套「商品必须完整入镜」——
 *      那条是给整件主图用的，套到微距上模型只会缩回去画整件。
 */
describe('参考图怎么展示商品：拆解阶段读出来', () => {
  const base = { canvas: '3:4', blocks: [{ zone: 'top', kind: 'headline', text_ru: 'X' }] }

  it('提示词里要求模型判断展示方式，并列出五种', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('"shot"')
    expect(HOT_LAYOUT_PROMPT).toContain('full | detail | use | scale | set')
  })

  it('提示词要求局部图写清「拍的哪个部位」和「我们商品哪个部位对应」', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('"shot_part"')
    expect(HOT_LAYOUT_PROMPT).toContain('"our_shot_part"')
    expect(HOT_LAYOUT_PROMPT).toContain('такого узла НЕТ')
  })

  it('提示词明确说：参考图只给了一部分，就不要描述整件商品的区块', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('НЕ описывай в blocks несуществующие блоки целого товара')
  })

  it('五种展示方式都能解析出来', () => {
    for (const kind of ['full', 'detail', 'use', 'scale', 'set']) {
      const layout = parseHotLayout(JSON.stringify({ ...base, shot: kind }), 1)!
      expect(layout.shot).toBe(kind)
    }
  })

  it('模型写俄语/中文也能归类', () => {
    expect(normalizeShotKind('деталь крупным планом')).toBe('detail')
    expect(normalizeShotKind('局部')).toBe('detail')
    expect(normalizeShotKind('材质')).toBe('detail')
    expect(normalizeShotKind('габариты')).toBe('scale')
    expect(normalizeShotKind('尺寸')).toBe('scale')
    expect(normalizeShotKind('человек сидит')).toBe('use')
    expect(normalizeShotKind('套装')).toBe('set')
  })

  it('认不出来就是空串（按最保守的「整件」处理，不会误裁商品）', () => {
    expect(normalizeShotKind('???')).toBe('')
    expect(normalizeShotKind('')).toBe('')
    const layout = parseHotLayout(JSON.stringify(base), 1)!
    expect(layout.shot).toBe('')
    expect(isPartialShot(layout)).toBe(false)
  })

  it('部位 / 角度三个字段都能读出来', () => {
    const layout = parseHotLayout(
      JSON.stringify({ ...base, shot: 'detail', shot_part: '坐垫面料', our_shot_part: '坐垫布面', shot_angle: '微距' }),
      1,
    )!
    expect(layout.shotPart).toBe('坐垫面料')
    expect(layout.ourShotPart).toBe('坐垫布面')
    expect(layout.shotAngle).toBe('微距')
  })
})

describe('局部参考图：不再要求「商品完整入镜」', () => {
  const make = (shot: string, extra: Record<string, unknown> = {}): HotLayout =>
    parseHotLayout(
      JSON.stringify({
        canvas: '3:4',
        shot,
        shot_part: '坐垫面料',
        our_shot_part: '坐垫布面',
        shot_angle: '微距怼近',
        blocks: [{ zone: 'top', kind: 'label', text_ru: 'Ткань' }],
        ...extra,
      }),
      1,
    )!

  it('detail：不带构图安全区', () => {
    const [card] = buildHotLayoutCards([make('detail')])
    expect(isPartialShot(make('detail'))).toBe(true)
    expect(card.prompt).not.toContain('【构图安全区】')
    expect(card.promptEn).not.toContain('[Framing safety]')
  })

  // 【2026-10-31 修正】原来这里断言「scale 一律不带安全区」，拿到真图后发现是错的：
  // ГАБАРИТЫ 那张两把椅子整把都在画面里，只是加了尺寸线 —— 它该完整入镜。
  // 现在只有「商品真的被裁掉」才不加，所以这条改成按 product_cropped 断言。
  it('scale 且商品没被裁：仍然要完整入镜（尺寸图不等于局部）', () => {
    const [card] = buildHotLayoutCards([make('scale', { product_cropped: 'false' })])
    expect(card.prompt).toContain('【构图安全区】')
    expect(card.promptEn).toContain('[Framing safety]')
  })

  it('scale 且商品被裁：才不带构图安全区', () => {
    const [card] = buildHotLayoutCards([make('scale', { product_cropped: 'true' })])
    expect(card.prompt).not.toContain('【构图安全区】')
    expect(card.promptEn).not.toContain('[Framing safety]')
  })

  it('full：仍然要完整入镜（这条不能因为改局部而丢掉）', () => {
    const [card] = buildHotLayoutCards([make('full')])
    expect(card.prompt).toContain('【构图安全区】')
    expect(card.promptEn).toContain('[Framing safety]')
  })

  it('detail 的正文要求照参考图裁切，并禁止拉远补出整件', () => {
    const [card] = buildHotLayoutCards([make('detail')])
    expect(card.prompt).toContain('成品也只画那一部分')
    expect(card.prompt).toContain('参考图拍的部位是：坐垫面料')
    expect(card.prompt).toContain('换成我们商品上对应的部位：坐垫布面')
    expect(card.prompt).toContain('禁止拉远')
    expect(card.promptEn).toContain('Match the reference crop exactly')
    expect(card.promptEn).toContain('Not seeing the whole product is correct')
  })

  it('材质必须取我们自己商品图上的真实样子', () => {
    const [card] = buildHotLayoutCards([make('detail')])
    expect(card.prompt).toContain('必须取我们商品图（第 2 张）上的真实样子')
    expect(card.prompt).toContain('不许美化')
  })

  it('我们商品没有那个结构时，不许凭空加一个', () => {
    const [card] = buildHotLayoutCards([make('detail', { our_shot_part: '我们没有扶手，用靠背侧边代替' })])
    expect(card.prompt).toContain('我们的商品上没有')
    expect(card.prompt).toContain('不要凭空给我们的商品加一个它没有的结构')
  })

  it('标题会标出这是局部特写，用户一眼能区分', () => {
    const [card] = buildHotLayoutCards([make('detail')])
    expect(card.title).toContain('局部特写')
    expect(card.title).toContain('坐垫面料')
  })
})

describe('使用场景：人和商品怎么接触，按我们的商品结构来', () => {
  const useLayout = parseHotLayout(
    JSON.stringify({
      canvas: '3:4',
      shot: 'use',
      blocks: [{ zone: 'top', kind: 'headline', text_ru: 'Мягкое кресло' }],
    }),
    1,
  )!

  it('★ 我们商品没有扶手时，不许给商品加扶手，也不许把人删掉', () => {
    const [card] = buildHotLayoutCards([useLayout])
    expect(card.prompt).toContain('人与商品的接触方式必须按我们商品的真实结构来')
    expect(card.prompt).toContain('不要画出那个部件')
    expect(card.prompt).toContain('也禁止因为「没地方坐」就把人删掉')
    expect(card.promptEn).toContain('do not draw that part')
    expect(card.promptEn).toContain('deleting the person')
  })

  it('使用场景仍然要完整入镜（它是整件商品）', () => {
    const [card] = buildHotLayoutCards([useLayout])
    expect(card.prompt).toContain('【构图安全区】')
  })

  it('非使用场景不带这一段（不许到处乱拼）', () => {
    const full = parseHotLayout(
      JSON.stringify({ canvas: '3:4', shot: 'full', blocks: [{ zone: 'top', kind: 'text', text_ru: 'X' }] }),
      1,
    )!
    const [card] = buildHotLayoutCards([full])
    expect(card.prompt).not.toContain('人与商品的接触方式必须按我们商品的真实结构来')
  })
})

describe('局部特写专用的规则：不再沿用整件那套说法', () => {
  const detail = parseHotLayout(
    JSON.stringify({
      canvas: '3:4',
      shot: 'detail',
      shot_part: '扶手侧边布料',
      our_shot_part: '靠背侧边布面',
      shot_angle: '微距',
      product_class: 'use',
      bg_is_photo: 'true',
      blocks: [{ zone: 'top', kind: 'label', text_ru: 'Ткань' }],
    }),
    1,
  )!

  it('★ 局部图不该出现「数一遍叶片」「花盆」这类仿生植物的说法', () => {
    const [card] = buildHotLayoutCards([detail])
    expect(card.prompt).not.toContain('数一遍叶片')
    expect(card.prompt).not.toContain('花盆')
    expect(card.promptEn).not.toContain('count the leaves')
  })

  it('★ 局部图不该要求「重现同一面墙、同一个窗户」（微距里没有屋子）', () => {
    const [card] = buildHotLayoutCards([detail])
    expect(card.prompt).not.toContain('同一个窗户位置')
    expect(card.prompt).toContain('不要')
    expect(card.prompt).toContain('补出房间')
  })

  it('局部图改成只约束这一块的材质与颜色', () => {
    const [card] = buildHotLayoutCards([detail])
    expect(card.prompt).toContain('这一块必须是我们商品真实的表面')
    expect(card.prompt).toContain('不许把布面画得更细更平')
    expect(card.promptEn).toContain('do not make the fabric look finer or smoother')
  })

  it('整件图仍然保留整套商品一致性与背景规则（这些没被改坏）', () => {
    const full = parseHotLayout(
      JSON.stringify({
        canvas: '3:4',
        shot: 'full',
        product_class: 'use',
        blocks: [{ zone: 'top', kind: 'text', text_ru: 'X' }],
      }),
      1,
    )!
    const [card] = buildHotLayoutCards([full])
    expect(card.prompt).toContain('数一遍叶片')
    expect(card.prompt).toContain('同一个窗户位置')
  })
})

describe('一个字都没有的纯局部图也能拆', () => {
  it('★ 只有画面没有文字的微距参考图，不再被当成「没看懂」丢掉', () => {
    const layout = parseHotLayout(
      JSON.stringify({ canvas: '3:4', shot: 'detail', shot_part: '布料纹理', our_shot_part: '坐垫布面' }),
      1,
    )
    expect(layout).not.toBeNull()
    expect(layout!.shot).toBe('detail')
    expect(layout!.blocks).toHaveLength(0)
  })

  it('既没有文字块也判断不出展示方式，才判失败', () => {
    expect(parseHotLayout(JSON.stringify({ canvas: '3:4' }), 1)).toBeNull()
  })
})

describe('没有任何文字的参考图：不许自己加字', () => {
  const layout = parseHotLayout(
    JSON.stringify({ canvas: '3:4', shot: 'detail', shot_part: '靠背布料', our_shot_part: '靠背布面' }),
    1,
  )!
  const [card] = buildHotLayoutCards([layout])

  it('★ 没有文字块时，卡片要求「画面里不要出现任何文字」', () => {
    expect(card.prompt).toContain('一个字的文案都没有')
    expect(card.prompt).toContain('画面里不要出现任何文字')
    expect(card.promptEn).toContain('no copy at all')
    expect(card.promptEn).toContain('no text, no logo, no watermark')
  })

  it('没有文字块时不会留下「就画几个、不许漏」这种空指令', () => {
    expect(card.prompt).not.toContain('参考图上有几个文字块就画几个')
    expect(card.prompt).not.toContain('undefined')
    expect(card.promptEn).not.toContain('undefined')
  })

  it('有文字块时照旧按块数一一对应', () => {
    const withBlocks = parseHotLayout(
      JSON.stringify({
        canvas: '3:4',
        shot: 'full',
        blocks: [
          { zone: 'top', kind: 'headline', text_ru: 'А' },
          { zone: 'bottom', kind: 'cta', text_ru: 'Б' },
        ],
      }),
      1,
    )!
    const [c] = buildHotLayoutCards([withBlocks])
    expect(c.prompt).toContain('参考图上有几个文字块就画几个')
    expect(c.prompt).toContain('А')
    expect(c.prompt).toContain('Б')
  })
})

describe('裁不裁只看 product_cropped，不靠类别猜', () => {
  const mk = (o: Record<string, unknown>) =>
    parseHotLayout(JSON.stringify({ canvas: '3:4', blocks: [{ zone: 'top', kind: 'text', text_ru: 'X' }], ...o }), 1)!

  it('★ 尺寸图但商品整把都在画面里 → 仍然要完整入镜', () => {
    const layout = mk({ shot: 'scale', product_cropped: 'false' })
    expect(isPartialShot(layout)).toBe(false)
    const [card] = buildHotLayoutCards([layout])
    expect(card.prompt).toContain('【构图安全区】')
  })

  it('★ 尺寸图但商品被裁掉 → 不加构图安全区', () => {
    const layout = mk({ shot: 'scale', product_cropped: 'true' })
    expect(isPartialShot(layout)).toBe(true)
    const [card] = buildHotLayoutCards([layout])
    expect(card.prompt).not.toContain('【构图安全区】')
  })

  it('局部特写不管有没有写明被裁，都按局部处理', () => {
    expect(isPartialShot(mk({ shot: 'detail' }))).toBe(true)
    expect(isPartialShot(mk({ shot: 'detail', product_cropped: 'true' }))).toBe(true)
  })

  it('使用场景被裁掉也算局部（腿被桌子挡住那种）', () => {
    expect(isPartialShot(mk({ shot: 'use', product_cropped: 'true' }))).toBe(true)
  })

  it('模型写俄语/中文的布尔也认', () => {
    expect(mk({ shot: 'scale', product_cropped: 'да' }).productCropped).toBe(true)
    expect(mk({ shot: 'scale', product_cropped: 'нет' }).productCropped).toBe(false)
  })

  it('提示词要求判断「商品有没有被裁掉」，并说明尺寸图整把在画面里不算裁', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('"product_cropped"')
    expect(HOT_LAYOUT_PROMPT).toContain('даже если рядом есть размерные линии и подписи')
  })
})

describe('截图里的界面元素不算版式', () => {
  it('提示词要求把「相似商品」角标、工具栏、文件大小那类排掉', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('СКРИНШОТОМ')
    expect(HOT_LAYOUT_PROMPT).toContain('похожие товары')
    expect(HOT_LAYOUT_PROMPT).toContain('размером файла')
  })
})

/**
 * 2026-10-31 拿到六张真图后发现的硬问题：
 * ГАБАРИТЫ 那张写满对方的真实数字（54,5 / 48,5 / 73 см / 150 кг / 6,5 кг），
 * 「舒适高度」那张是 75 см。拆解会把它们原样抄进 text_ru，
 * 而卡片规则又要求「内容就用上面给的俄语」—— 模型必然把**对方的尺寸**印到我们的图上。
 * 卖椅子的把别人尺寸写自己图上，就是给买家错信息。
 */
describe('尺寸图：不许照抄对方的数字', () => {
  const scaleLayout = parseHotLayout(
    JSON.stringify({
      canvas: '3:4',
      shot: 'scale',
      product_cropped: 'false',
      blocks: [
        { zone: 'middle', kind: 'label', text_ru: '54,5 см' },
        { zone: 'lower', kind: 'label', text_ru: 'нагрузка 150 кг, вес 6,5 кг' },
      ],
    }),
    1,
  )!

  it('抹数字：厘米/公斤连着单位的整段换成占位', () => {
    expect(maskReferenceNumbers('54,5 см')).toBe('___ см')
    expect(maskReferenceNumbers('нагрузка 150 кг')).toBe('нагрузка ___ кг')
  })

  it('抹数字：孤立的阿拉伯数字也抹掉', () => {
    expect(maskReferenceNumbers('высота 75')).toBe('высота ___')
  })

  it('没填我们的尺寸时，卡片里不能出现对方的任何数字', () => {
    const [card] = buildHotLayoutCards([scaleLayout], '', '')
    expect(card.prompt).not.toContain('54,5')
    expect(card.prompt).not.toContain('150')
    expect(card.prompt).not.toContain('6,5')
    expect(card.prompt).toContain('___')
    expect(card.promptEn).not.toContain('54,5')
  })

  it('没填尺寸时明确要求「数字全部留空」，且规则里不出现具体数字', () => {
    const [card] = buildHotLayoutCards([scaleLayout], '', '')
    expect(card.prompt).toContain('尺寸数字是对方的，一个都不许照抄')
    expect(card.prompt).toContain('数字全部留空')
    // 规则里也不许拿具体数字举例，否则模型会把例子当成要画的字
    expect(card.prompt).toContain('绝对不许把参考图上的任何数字搬到成品上')
    expect(card.promptEn).toContain('none may be copied')
  })

  it('★ 填了我们的尺寸时，数字保留位由真实值补，并逐个换掉', () => {
    const [card] = buildHotLayoutCards([scaleLayout], '', '高 82 厘米')
    expect(card.prompt).toContain('高 82 厘米')
    expect(card.prompt).toContain('参考图上原来的数字是**对方的**')
    expect(card.prompt).not.toContain('[object Object]')
  })

  it('非尺寸图不抹数字（别误伤正文里的 102° / 12 месяцев 这类）', () => {
    const full = parseHotLayout(
      JSON.stringify({
        canvas: '3:4',
        shot: 'full',
        blocks: [{ zone: 'top', kind: 'badge', text_ru: 'гарантия 12 месяцев' }],
      }),
      1,
    )!
    const [card] = buildHotLayoutCards([full], '', '')
    expect(card.prompt).toContain('гарантия 12 месяцев')
  })
})

/**
 * 2026-10-31 真图暴露的坑：对手写「мягкие подлокотники（软扶手）」，
 * 我们的椅子根本没有扶手；对手写「ткань ВЕЛЮР（天鹅绒）」，我们的是科技布。
 * 拆解只要求「保持长度和风格」，模型就把对方的部件/材质原样留在文案里 ——
 * 成品上写着「软扶手」而画面里没有扶手，买家一眼看出假。
 */
describe('文案不许提我们商品上没有的部件或材质', () => {
  it('拆解提示词要求把对方有、我们没有的部件/材质换掉', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('которых у НАШЕГО товара НЕТ')
    expect(HOT_LAYOUT_PROMPT).toContain('подлокотники')
    expect(HOT_LAYOUT_PROMPT).toContain('велюр')
  })

  it('卡片正文加了「画面里出现了什么，文字就只能写什么」的兜底', () => {
    const layout = parseHotLayout(
      JSON.stringify({ canvas: '3:4', shot: 'full', blocks: [{ zone: 'top', kind: 'text', text_ru: 'X' }] }),
      1,
    )!
    const [card] = buildHotLayoutCards([layout])
    expect(card.prompt).toContain('画面里出现了什么，文字就只能写什么')
    expect(card.prompt).toContain('文字和画面打架的那张是废片')
    expect(card.promptEn).toContain('The copy may only describe what is actually visible')
  })
})

/**
 * 2026-10-31 真图实测暴露的两个瑕疵（主人指定只修这两个，尺寸数字那条不用管）：
 *   ① 局部图里无底色的自由文字块被画到画面下半部、压到图边、一半字在框外；
 *   ② 我们的椅子是细密平滑的科技布，出图却画成粗颗粒的颗粒绒（bouclé）。
 */
describe('文字块必须完整在画面内（2026-10-31 实测瑕疵）', () => {
  const mk = (shot: string, extra: Record<string, unknown> = {}) =>
    buildHotLayoutCards([
      parseHotLayout(
        JSON.stringify({
          canvas: '3:4',
          shot,
          blocks: [{ zone: 'lower', kind: 'label', style: '无底色白色粗体两行', text_ru: 'ТАКТИЛЬНО МЯГКИЙ' }],
          ...extra,
        }),
        1,
      )!,
    ])[0]

  it('★ 要求无底色文字块整行可见、不许压边、不许出框', () => {
    const card = mk('detail', { shot_part: '布面', our_shot_part: '布面' })
    expect(card.prompt).toContain('没有底色的文字块也要完整在画面内')
    expect(card.prompt).toContain('不许有一半字压到画面边缘')
    expect(card.prompt).toContain('宁可把它整体挪进画面里，也不许让它出框')
    expect(card.promptEn).toContain('must sit fully inside the frame')
    expect(card.promptEn).toContain('move the whole block inward')
  })

  it('整件图同样带这条（不只是局部图）', () => {
    const card = mk('full')
    expect(card.prompt).toContain('没有底色的文字块也要完整在画面内')
  })

  it('★ 第二轮：文字回到画面里还不够，还要留白、不许贴角、不许压在商品上', () => {
    const card = mk('detail', { shot_part: '布面', our_shot_part: '布面' })
    expect(card.prompt).toContain('留白要求')
    expect(card.prompt).toContain('不许贴着画面四角')
    expect(card.prompt).toContain('不许压在商品主体上')
    expect(card.prompt).toContain('缩小字号也要保住留白')
    expect(card.promptEn).toContain('Whitespace requirement')
    expect(card.promptEn).toContain('shrink the type size rather than cramming it in')
  })

  it('拆解提示词要求模型标明每块有没有底色', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('есть подложка / без подложки')
    expect(HOT_LAYOUT_PROMPT).toContain('без подложки')
  })
})

describe('布料种类不许换（2026-10-31 实测瑕疵）', () => {
  const detail = buildHotLayoutCards([
    parseHotLayout(
      JSON.stringify({
        canvas: '3:4',
        shot: 'detail',
        shot_part: '靠背布面',
        our_shot_part: '靠背科技布面',
        blocks: [{ zone: 'top', kind: 'headline', text_ru: 'ПРЕМИУМ ТКАНЬ' }],
      }),
      1,
    )!,
  ])[0]

  it('★ 点名禁止改成颗粒绒 / 羊羔绒 / 毛圈布 / 天鹅绒 / 亚麻 / 皮革', () => {
    for (const bad of ['颗粒绒', '羊羔绒', '毛圈布', '天鹅绒', '亚麻', '皮革']) {
      expect(detail.prompt).toContain(bad)
    }
    expect(detail.promptEn).toContain('bouclé')
    expect(detail.promptEn).toContain('sherpa')
    expect(detail.promptEn).toContain('terry')
  })

  it('要求先读我们商品图上这块布的织法，按同一种织法画', () => {
    expect(detail.prompt).toContain('先看我们商品图（第 2 张）上这块布的织法')
    expect(detail.prompt).toContain('出图必须用同一种织法、同一种光泽度')
    expect(detail.promptEn).toContain('First read the weave of the fabric in our product photo')
  })

  // 【2026-10-31 修正】这条不该只管局部图：整件主图里的布面同样是买家下单依据，
  // 六张真图里出问题的是局部那张，但风险在整件图上一样存在。所以改成所有卡片都加。
  it('★ 所有卡片都带这条，不只在局部图上', () => {
    const full = buildHotLayoutCards([
      parseHotLayout(
        JSON.stringify({ canvas: '3:4', shot: 'full', blocks: [{ zone: 'top', kind: 'text', text_ru: 'X' }] }),
        1,
      )!,
    ])[0]
    expect(full.prompt).toContain('不许换布料种类')
    expect(full.prompt).toContain('颗粒绒')
    expect(full.promptEn).toContain('bouclé')
  })
})

/**
 * 2026-10-31 主人第二次反馈（原话）：
 * 「产品的一些内容没有完全按照图片去展示，我放入的图片产品的靠背的形状是蝴蝶结形状、
 *   有的图片是四四方方的」
 *
 * 实测确认：我们的椅子靠背是**两瓣 + 中间收腰的花瓣/蝴蝶结形**，
 * 出图里有几张被画成一整块方方正正的弧形靠背（也就是对手那把椅子的形状）。
 * 根因：structure_lock 只让模型「数部件」（茎/叶/盆），家具的外轮廓根本没人管。
 */
describe('外轮廓（靠背形状）必须照商品图，不许简化成方块', () => {
  const mk = (o: Record<string, unknown> = {}) =>
    buildHotLayoutCards([
      parseHotLayout(
        JSON.stringify({
          canvas: '3:4',
          shot: 'full',
          blocks: [{ zone: 'top', kind: 'text', text_ru: 'X' }],
          ...o,
        }),
        1,
      )!,
    ])[0]

  it('拆解提示词要模型用几何语言描述外轮廓（瓣数 / 收腰 / 顶部线条 / 两侧）', () => {
    expect(HOT_LAYOUT_PROMPT).toContain('"silhouette"')
    expect(HOT_LAYOUT_PROMPT).toContain('ВНЕШНИЙ КОНТУР')
    expect(HOT_LAYOUT_PROMPT).toContain('из скольких частей/лепестков/сегментов')
    expect(HOT_LAYOUT_PROMPT).toContain('бабочка')
  })

  it('解析能拿到 silhouette', () => {
    const layout = parseHotLayout(
      JSON.stringify({
        canvas: '3:4',
        blocks: [{ text_ru: 'X' }],
        silhouette: '两瓣花瓣形靠背，中间收腰，顶部弧形，左右外扩，无扶手',
      }),
      1,
    )!
    expect(layout.silhouette).toContain('两瓣花瓣形')
  })

  it('★ 卡片写死「轮廓一个像素都不许改」，并点名禁止方块化', () => {
    const card = mk({ silhouette: '两瓣花瓣形靠背，中间收腰，顶部弧形' })
    expect(card.prompt).toContain('外轮廓形状必须和商品图（第 2 张）一模一样，一个像素都不许改')
    expect(card.prompt).toContain('我们这件商品的轮廓是：两瓣花瓣形靠背，中间收腰，顶部弧形')
    expect(card.prompt).toContain('方方正正的弧形板')
    expect(card.prompt).toContain('不许把花瓣形、蝴蝶结形、扇形、波浪形改成圆角矩形')
    expect(card.promptEn).toContain('Never simplify the silhouette into a conventional shape')
  })

  it('要求画之前数一遍瓣数、画完再对一遍（可执行的自查）', () => {
    const card = mk()
    expect(card.prompt).toContain('画之前先对照商品图数一遍轮廓')
    expect(card.prompt).toContain('少一瓣多一瓣都算废片')
    expect(card.promptEn).toContain('count the silhouette against the photo')
  })

  it('没说轮廓也不能出 undefined，规则本身仍然生效', () => {
    const card = mk()
    expect(card.prompt).not.toContain('undefined')
    expect(card.prompt).not.toContain('NaN')
    expect(card.prompt).toContain('商品的外轮廓形状必须和商品图')
  })

  it('局部图也带这条（局部图同样会把花盆/靠背画成方块）', () => {
    const card = mk({ shot: 'detail', shot_part: '靠背布面', our_shot_part: '靠背布面', product_cropped: 'true' })
    expect(card.prompt).toContain('商品的外轮廓形状必须和商品图')
    expect(card.promptEn).toContain('The outer silhouette of the product must match')
  })
})
