import { describe, expect, it } from 'vitest'
import {
  DEFAULT_SCENE,
  OZON_CARD_COUNT,
  OZON_SIZE_OPTIONS,
  OZON_SYSTEM_PROMPT,
  PRODUCT_LOCK,
  SCENE_LIBRARY,
  VISION_EXTRACT_PROMPT,
  autoMatchScene,
  buildOzonScript,
  buildProductPalette,
  describeHumanScale,
  parseSizeCm,
  describePaletteEn,
  describePaletteZh,
  normalizeHex,
  parseSellingPoints,
} from '../aiImagePrompts'

describe('autoMatchScene', () => {
  it('matches a keyword inside the product name', () => {
    const scene = autoMatchScene('316 不锈钢保温杯 500ml')
    expect(scene.zh).toContain('办公室桌面')
    expect(scene.en).toContain('office desk')
  })

  it('falls back to the default scene when nothing matches', () => {
    expect(autoMatchScene('某个没见过的商品')).toEqual({
      en: DEFAULT_SCENE[0],
      zh: DEFAULT_SCENE[1],
    })
  })

  it('falls back for an empty name', () => {
    expect(autoMatchScene('')).toEqual({ en: DEFAULT_SCENE[0], zh: DEFAULT_SCENE[1] })
  })

  it('keeps every library entry usable', () => {
    for (const [keyword, en, zh] of SCENE_LIBRARY) {
      expect(keyword.length).toBeGreaterThan(0)
      expect(en.length).toBeGreaterThan(0)
      expect(zh.length).toBeGreaterThan(0)
      // 关键词必须真能命中自己，否则这一条永远不会被用到。
      expect(autoMatchScene(`测试${keyword}商品`).en).toBe(en)
    }
  })
})

describe('PRODUCT_LOCK（商品一致性锁定）', () => {
  /**
   * 这两个约束在英文里是两个词，**不能合并**：
   *   - `proportions`  = 商品自身的长宽比例不能变形
   *   - `size ratio`   = 商品在整张图里占多大
   * 精简时曾经只留下前者，导致「商品被放大/缩小」没有任何约束。
   * 这两条断言就是防止再被合并掉。
   */
  it('明确要求商品在图里的占比和原图一致（不是只保持自身形状）', () => {
    expect(PRODUCT_LOCK).toContain('proportions')
    // 「在图里的大小」这一条必须单独说，不能只靠 proportions 顶替。
    expect(PRODUCT_LOCK.toLowerCase()).toContain('scale')
    expect(PRODUCT_LOCK.toLowerCase()).toContain('frame')
  })

  it('含有「不许重塑/变形/改色」的禁止项', () => {
    const text = PRODUCT_LOCK.toLowerCase()
    expect(text).toContain('distort')
    expect(text).toContain('reshape')
  })

  it('只许改背景/光线/排版/文字', () => {
    expect(PRODUCT_LOCK).toContain('background')
    expect(PRODUCT_LOCK).toContain('lighting')
    expect(PRODUCT_LOCK).toContain('layout')
    expect(PRODUCT_LOCK).toContain('text')
  })

  it('仍然保持精简（没有把原文那 330 字符整段搬回来）', () => {
    // 上限留得比现状宽松一点，只用来挡住「整段搬回」这种退化。
    expect(PRODUCT_LOCK.length).toBeLessThan(360)
  })
})

describe('buildOzonScript', () => {
  const base = { product: '316 不锈钢保温杯', sp1: '24小时保温', sp2: '316医用钢', sp3: '一键开盖' }

  /**
   * 2026-10-24 主人先重排、当天下午又按转达的修改意见减到 8 张：
   *   1 主图（商品 + 模特使用 + 大字卖点）
   *   2 痛点对比（**同一件商品**）
   *   3 生活场景（**正在使用中**）
   *   4 细节①材质纹理
   *   5 细节②结构部位（结构点自行判断）
   *   6 细节③功能效果（功能点自行判断）
   *   7 尺寸图（要有背景、只许一件商品）
   *   8 联系卖家提醒图
   */
  it('八张的顺序和 2026-10-24 最终定的一致', () => {
    const cards = buildOzonScript(base)
    expect(cards).toHaveLength(8)
    expect(cards).toHaveLength(OZON_CARD_COUNT)
    expect(cards.map((card) => card.index)).toEqual([1, 2, 3, 4, 5, 6, 7, 8])
    expect(cards.map((card) => card.key)).toEqual([
      '01_main_image',
      '02_pain_compare',
      '03_lifestyle',
      '04_detail_texture',
      '05_detail_structure',
      '06_detail_feature',
      '07_size_chart',
      '08_contact_seller',
    ])
  })

  /**
   * 【2026-10-24 主人要求】「套图不显示出图顺序这些内容，防止别的照抄」。
   * 原来导出的 OZON_CARD_PLAN 就是给界面列顺序用的，现在整块删掉。
   * 这条守卫防止以后有人「为了方便」又把它加回来。
   */
  it('不再导出出图顺序清单（防止配方被照抄）', async () => {
    const mod = await import('../aiImagePrompts')
    expect('OZON_CARD_PLAN' in mod).toBe(false)
    // 但顺序本身还在：key 顺序必须稳定
    expect(buildOzonScript(base).map((card) => card.key)).toEqual([
      '01_main_image',
      '02_pain_compare',
      '03_lifestyle',
      '04_detail_texture',
      '05_detail_structure',
      '06_detail_feature',
      '07_size_chart',
      '08_contact_seller',
    ])
  })

  it('标题里的张号是 1..8 连续，不会重号也不会跳号', () => {
    for (const [i, card] of buildOzonScript(base).entries()) {
      expect(card.title.startsWith(`第 ${i + 1} 张 · `)).toBe(true)
      expect(card.promptEn).toContain(`Card ${i + 1}:`)
    }
  })

  /**
   * 2026-10-24 主人的最终定稿：「主图现在不加模特使用……更多的是在产品主体不变的情况下
   * 要大标题和介绍产品的详情」，而「生活场景里面加入模特，最好展示出模特正在使用」。
   * 也就是说：模特从主图挪到了第 3 张。
   */
  it('主图不放模特，模特挪到第 3 张生活场景', () => {
    const cards = buildOzonScript(base)
    const main = cards.find((card) => card.key === '01_main_image')!
    expect(main.prompt).toContain('画面里不要出现人物、模特、手')
    expect(main.prompt).toContain('不要人物或手')
    expect(main.promptEn).toContain('No people, no models, no hands')
    // 标题里也不该再提模特
    expect(main.title).not.toContain('模特')

    const life = cards.find((card) => card.key === '03_lifestyle')!
    expect(life.title).toContain('模特正在使用')
    expect(life.prompt).toContain('画面里要有一位俄罗斯模特')
    expect(life.promptEn).toContain('a Russian model must be present')

    // 原来那张独立的模特场景依旧不该存在
    expect(cards.some((card) => card.key.includes('model_scene'))).toBe(false)
  })

  it('主图的版式会在三种里变，不是每套都一个样', () => {
    // 三个版式都写得出对应的话
    const byLayout = (layout: 'productRight' | 'productLeft' | 'productBottom') =>
      buildOzonScript({ ...base, mainLayout: layout }).find((card) => card.key === '01_main_image')!

    expect(byLayout('productRight').prompt).toContain('商品在画面右侧')
    expect(byLayout('productRight').prompt).toContain('左侧留出约 40%')
    expect(byLayout('productLeft').prompt).toContain('商品在画面左侧')
    expect(byLayout('productLeft').prompt).toContain('右侧留出约 40%')
    expect(byLayout('productBottom').prompt).toContain('商品在画面下半部居中')
    expect(byLayout('productBottom').prompt).toContain('上半部约 35%')

    // 英文版也要对上
    expect(byLayout('productRight').promptEn).toContain('sits on the RIGHT')
    expect(byLayout('productLeft').promptEn).toContain('sits on the LEFT')
    expect(byLayout('productBottom').promptEn).toContain('LOWER half')

    // 不指定时必须是三者之一（随机，但不能跑到列表外）
    for (let i = 0; i < 12; i += 1) {
      const main = buildOzonScript(base).find((card) => card.key === '01_main_image')!
      const hit = ['商品在画面右侧', '商品在画面左侧', '商品在画面下半部居中'].filter((t) =>
        main.prompt.includes(t),
      )
      expect(hit).toHaveLength(1)
    }
  })

  it('第 1 张是主图：商品不许改、空白处放大字、只一件商品', () => {
    const main = buildOzonScript(base).find((card) => card.key === '01_main_image')!
    expect(main.prompt).toContain('一个像素都不许改')
    expect(main.prompt).toContain('字体要大')
    expect(main.prompt).toContain('24小时保温')
    expect(main.prompt).toContain('绝不压在商品主体上')
    expect(main.prompt).toContain('只能出现**一件商品**')
    // 文字也不能压模特脸
    expect(main.prompt).toContain('画面里不要出现人物')
  })

  /**
   * 2026-10-24 主人第三次改这一张：给了三张成品图，说
   * 「对比图片可以是同一类产品，不能是完全不相干的产品，也不能是完全一致的产品」。
   * 所以既不许串到别的品类，也不许两边一模一样。
   */
  it('第 2 张痛点对比：同类不同款，既不许不相干也不许一模一样', () => {
    const compare = buildOzonScript(base).find((card) => card.key === '02_pain_compare')!
    expect(compare.title).toContain('同类不同款')
    // 必须是同一类产品的普通款
    expect(compare.prompt).toContain('同一类产品的普通款')
    expect(compare.prompt).toContain('明显更差一档')
    // 三条禁止都在
    expect(compare.prompt).toContain('完全不相干的品类')
    expect(compare.prompt).toContain('一模一样、看不出差别')
    expect(compare.prompt).toContain('另一个完全不同的款式')
    // 右边仍然要原样
    expect(compare.prompt).toContain('就是参考图里那一件商品')
    // 英文版也要有
    expect(compare.promptEn).toContain('ordinary version of the same product category')
    expect(compare.promptEn).toContain('an unrelated category')
  })

  it('第 3 张生活场景：产品必须在使用中，不是摆着看', () => {
    const life = buildOzonScript(base).find((card) => card.key === '03_lifestyle')!
    expect(life.title).toContain('模特正在使用')
    // 主人转达的原话：「生活场景图要使用产品，不要光放着」
    expect(life.prompt).toContain('正在使用这件商品')
    expect(life.prompt).toContain('画面里要有一位俄罗斯模特')
    expect(life.promptEn).toContain('actually using the product')
    expect(life.prompt).toContain('不要只有一只手没有人物')
  })

  it('三张细节图角度不同、部位不同、都不许出现 logo', () => {
    const cards = buildOzonScript(base)
    const details = ['04_detail_texture', '05_detail_structure', '06_detail_feature'].map(
      (key) => cards.find((card) => card.key === key)!,
    )
    expect(details).toHaveLength(3)

    for (const card of details) {
      expect(card.prompt).toContain('看不到完整商品')
      expect(card.prompt).not.toContain('【构图安全区】')
      expect(card.prompt).toContain('logo')
      expect(card.prompt).toContain('吊牌')
      expect(card.prompt).toContain('一致')
    }

    // 三个角度必须互不相同
    expect(details[0].prompt).toContain('45 度斜上方')
    expect(details[1].prompt).toContain('正侧面贴近')
    expect(details[2].prompt).toContain('俯视贴近')

    expect(details[1].prompt).toContain('和第 4 张不同')
    expect(details[2].prompt).toContain('防水')
    expect(details[2].prompt).toContain('防油污')
    expect(details[2].prompt).toContain('一擦')
  })

  it('结构与功能点交给模型按商品自行判断，不再绑死卖点文字', () => {
    const cards = buildOzonScript(base)
    const structure = cards.find((card) => card.key === '05_detail_structure')!
    const feature = cards.find((card) => card.key === '06_detail_feature')!
    // 主人转达：「结构和功能让他根据产品来自行填写」
    expect(structure.prompt).toContain('结构点由你自己按这件商品判断')
    expect(feature.prompt).toContain('功能点由你自己按这件商品判断')
    // 卖点原文不该再被硬塞进这两张
    expect(structure.prompt).not.toContain('316医用钢')
    expect(feature.prompt).not.toContain('一键开盖')
    expect(structure.promptEn).not.toContain('316医用钢')
    expect(feature.promptEn).not.toContain('一键开盖')
  })

  it('尺寸图：要有真实背景、只许一件商品、禁止拼图', () => {
    const size = buildOzonScript(base).find((card) => card.key === '07_size_chart')!
    expect(size.prompt).toContain('必须有真实背景')
    expect(size.prompt).toContain('不要空白')
    expect(size.prompt).not.toContain('纯白或极浅灰网格底')
    expect(size.prompt).toContain('禁止拼图')
    expect(size.prompt).toContain('三联图')
    expect(size.prompt).toContain('九宫格')
    expect(size.prompt).toContain('只许出现一件商品')
    expect(size.prompt).toContain('阿拉伯数字')
    expect(size.prompt).toContain('см')
  })

  it('整套规范里有一条硬规则：一张图只画一个画面（禁止拼图/三联图/九宫格）', () => {
    for (const card of buildOzonScript(base)) {
      expect(card.prompt).toContain('禁止拼图')
      expect(card.promptEn).toContain('Never make a collage')
    }
  })

  /**
   * 2026-10-24 主人又改回来了：「售后的生图要求按照之前的那一版生成如后两张一样，
   * 页面的颜色依照产品主体的颜色来定」。
   * 所以恢复 0.2.9.36 那版详细的八段版式，底色改由商品主色决定。
   */
  it('第 8 张售后图恢复详细版式（八段 + 三条白长条 + 三列图标 + 编号步骤）', () => {
    const contact = buildOzonScript(base).find((card) => card.key === '08_contact_seller')!
    expect(contact.title).toContain('售后')
    expect(contact.prompt).toContain('УВАЖАЕМЫЕ ПОКУПАТЕЛИ!')
    expect(contact.prompt).toContain('ЕСЛИ У ВАС ВОЗНИКЛИ ВОПРОСЫ')
    expect(contact.prompt).toContain('НАПИШИТЕ В ПОДДЕРЖКУ МАРКЕТПЛЕЙСА')
    expect(contact.prompt).toContain('ЧТОБЫ СВЯЗАТЬСЯ С ПРОДАВЦОМ')
    expect(contact.prompt).toContain('Повреждена упаковка')
    expect(contact.prompt).toContain('Профиль → Сообщения → Поддержка')
    expect(contact.prompt).toContain('Зайдите в карточку товара')
    expect(contact.prompt).toContain('В корзину')
    // 三条白长条必须等宽
    expect(contact.prompt).toContain('三条白长条必须等宽')
    expect(contact.promptEn).toContain('three white bars must share one width')
  })

  it('第 8 张的底色跟着商品主色的深→浅渐变走', () => {
    const palette = buildProductPalette('淡紫色', '#C8B6E2')!
    const withColor = buildOzonScript({ ...base, mainColorName: '淡紫色', mainColorHex: '#C8B6E2' }).find(
      (card) => card.key === '08_contact_seller',
    )!
    expect(withColor.prompt).toContain(palette.ink)
    expect(withColor.prompt).toContain(palette.tint)
    expect(withColor.prompt).toContain('竖向渐变')
    expect(withColor.promptEn).toContain('vertical gradient')

    // 没有主色时退回「用商品主色的深→浅渐变」，不会留空
    const noColor = buildOzonScript(base).find((card) => card.key === '08_contact_seller')!
    expect(noColor.prompt).toContain('用商品主体主色的深→浅竖向渐变')
    expect(noColor.prompt).not.toContain('#')
  })

  it('每张实拍图都写清了镜头和背景', () => {
    const photoKeys = [
      '01_main_image',
      '02_pain_compare',
      '03_lifestyle',
      '04_detail_texture',
      '05_detail_structure',
      '06_detail_feature',
      '07_size_chart',
    ]
    for (const card of buildOzonScript(base).filter((item) => photoKeys.includes(item.key))) {
      expect(card.prompt).toContain('背景：')
    }
    // 微距三张 + 尺寸图仍然要有明确的镜头说明
    for (const key of ['04_detail_texture', '05_detail_structure', '06_detail_feature', '07_size_chart']) {
      expect(buildOzonScript(base).find((card) => card.key === key)!.prompt).toContain('镜头：')
    }
  })

  it('中文版给用户看，英文版给模型：两套都在，且英文版不含中文正文', () => {
    for (const card of buildOzonScript(base)) {
      expect(card.promptEn).toBeTruthy()
      expect(card.promptEn.length).toBeGreaterThan(200)
      expect(card.promptEn).not.toContain('镜头：')
      expect(card.promptEn.toLowerCase()).toContain('never render chinese')
    }
  })

  it('每张都带 Ozon 规范；尺寸图把「不要添加数字」那条换掉', () => {
    const cards = buildOzonScript(base)
    for (const card of cards) {
      expect(card.prompt.startsWith(PRODUCT_LOCK)).toBe(false)
      expect(card.prompt).toContain('3:4')
      expect(card.prompt).toContain('俄语')
      expect(card.prompt).toContain('配色')
    }
    for (const card of cards.filter((item) => item.key !== '07_size_chart')) {
      expect(card.prompt).toContain(OZON_SYSTEM_PROMPT)
    }
    const size = cards.find((card) => card.key === '07_size_chart')!
    expect(size.prompt).not.toContain('不要添加原图没有的数字和尺寸')
    expect(size.promptEn).not.toContain('Do not add numbers or dimensions')
    expect(size.promptEn).toContain('Arabic numerals')
  })

  it('需要完整入镜的卡片都带构图安全区（微距三张 + 联系卖家图除外）', () => {
    const noFraming = ['04_detail_texture', '05_detail_structure', '06_detail_feature', '08_contact_seller']
    for (const card of buildOzonScript(base)) {
      if (noFraming.includes(card.key)) {
        expect(card.prompt).not.toContain('【构图安全区】')
        continue
      }
      expect(card.prompt).toContain('【构图安全区】')
      expect(card.prompt).toContain('完整入镜')
      expect(card.promptEn).toContain('[Framing safety]')
    }
  })

  it('用上了匹配到的场景', () => {
    const life = buildOzonScript({
      ...base,
      sceneEn: 'clean kitchen countertop',
      sceneZh: '厨房台面 + 晨光 + 咖啡豆',
    }).find((card) => card.key === '03_lifestyle')!
    expect(life.promptEn).toContain('clean kitchen countertop')
    expect(life.prompt).toContain('厨房台面 + 晨光 + 咖啡豆')
  })

  it('不会出现 undefined 或空占位，每张都够详细', () => {
    const cards = buildOzonScript({ product: '杯子' })
    for (const card of cards) {
      expect(card.prompt).not.toContain('undefined')
      expect(card.prompt).not.toContain('${')
      expect(card.promptEn).not.toContain('undefined')
      expect(card.promptEn).not.toContain('${')
      expect(card.prompt.length).toBeGreaterThan(200)
    }
    expect(cards.find((card) => card.key === '01_main_image')!.prompt).toContain('feature 1')
  })

  it('中文的「用户痛点 / 差异化」不再拼进提示词（避免图上出现中文）', () => {
    for (const card of buildOzonScript({ ...base, pain: '水到下午就凉了', diff: '别人用 201 钢' })) {
      expect(card.prompt).not.toContain('水到下午就凉了')
      expect(card.promptEn).not.toContain('水到下午就凉了')
      expect(card.prompt).not.toContain('别人用 201 钢')
    }
  })
})


/**
 * 2026-10-24 主人问「是否保证了图片的一些背景、文字与商品的颜色一致或相近」。
 *
 * 原来只有一句「取商品主色调」，模型得自己猜色。
 * 现在让视觉模型在识别卖点时顺手把商品主色读出来，
 * 再把深/中/浅三个**具体色值**写进第 1 张主图，从「你猜」变成「照这个用」。
 */
describe('商品主色 → 第 1 张主图的配色', () => {
  const base = { product: '紫色芭蕾包', sp1: 'a', sp2: 'b', sp3: 'c' }

  it('normalizeHex 只认 6 位十六进制', () => {
    expect(normalizeHex('#c8b6e2')).toBe('#C8B6E2')
    expect(normalizeHex('c8b6e2')).toBe('#C8B6E2')
    expect(normalizeHex('  #C8B6E2  ')).toBe('#C8B6E2')
    expect(normalizeHex('#fff')).toBeNull()
    expect(normalizeHex('purple')).toBeNull()
    expect(normalizeHex('')).toBeNull()
    expect(normalizeHex(undefined)).toBeNull()
  })

  it('给定主色时推出「深 / 中 / 浅」三个不同颜色，且都合法', () => {
    const palette = buildProductPalette('淡紫色', '#C8B6E2')!
    expect(palette).not.toBeNull()
    expect(palette.neutral).toBe(false)
    expect(palette.hex).toBe('#C8B6E2')
    expect(palette.name).toBe('淡紫色')

    for (const value of [palette.ink, palette.accent, palette.tint]) {
      expect(value).toMatch(/^#[0-9A-F]{6}$/)
    }
    // 三个色必须互不相同，否则「同色系深浅」就没意义
    expect(new Set([palette.ink, palette.accent, palette.tint]).size).toBe(3)

    // 亮度关系：深色 < 中间色 < 浅色
    const lum = (hex: string) => {
      const h = hex.replace('#', '')
      const r = parseInt(h.slice(0, 2), 16)
      const g = parseInt(h.slice(2, 4), 16)
      const b = parseInt(h.slice(4, 6), 16)
      return 0.2126 * r + 0.7152 * g + 0.0722 * b
    }
    expect(lum(palette.ink)).toBeLessThan(lum(palette.accent))
    expect(lum(palette.accent)).toBeLessThan(lum(palette.tint))
  })

  it('商品是灰/白/黑时退化成中性灰阶，并说明「唯一彩色取自商品点缀」', () => {
    const grey = buildProductPalette('中灰', '#9A9A9A')!
    expect(grey.neutral).toBe(true)
    expect(grey.ink).toBe('#2B2B2B')
    expect(grey.accent).toBe('#8A8A8A')
    expect(grey.tint).toBe('#F3F2F0')
    expect(describePaletteZh(grey)).toContain('中性色')
    expect(describePaletteZh(grey)).toContain('点缀')
    expect(describePaletteEn(grey)).toContain('neutral grey/white/black')
  })

  it('没给主色时返回 null（调用方退回笼统要求）', () => {
    expect(buildProductPalette(undefined, undefined)).toBeNull()
    expect(buildProductPalette('紫色', 'not-a-color')).toBeNull()
  })

  it('describePalette 的话里带上了三个具体色值', () => {
    const palette = buildProductPalette('淡紫色', '#C8B6E2')!
    const zh = describePaletteZh(palette)
    expect(zh).toContain('#C8B6E2')
    expect(zh).toContain(palette.ink)
    expect(zh).toContain(palette.accent)
    expect(zh).toContain(palette.tint)
    expect(zh).toContain('不要做成白色或灰色')

    const en = describePaletteEn(palette)
    expect(en).toContain('#C8B6E2')
    expect(en).toContain('never white or grey')
  })

  it('第 1 张主图吃到具体色值，别的卡不吃（主人：主图实现即可）', () => {
    const cards = buildOzonScript({ ...base, mainColorName: '淡紫色', mainColorHex: '#C8B6E2' })
    const palette = buildProductPalette('淡紫色', '#C8B6E2')!
    const main = cards.find((card) => card.key === '01_main_image')!

    expect(main.prompt).toContain('商品主色是淡紫色（#C8B6E2）')
    expect(main.prompt).toContain(palette.ink)
    expect(main.prompt).toContain(palette.tint)
    expect(main.promptEn).toContain('#C8B6E2')

    // 只有主图和第 8 张售后图吃主色，其余 6 张不吃
    const paletteCards = ['01_main_image', '08_contact_seller']
    for (const card of cards.filter((item) => !paletteCards.includes(item.key))) {
      expect(card.prompt).not.toContain('#C8B6E2')
      expect(card.promptEn).not.toContain('#C8B6E2')
    }
  })

  it('没有主色时第 1 张退回那句笼统要求，不会留空', () => {
    const main = buildOzonScript(base).find((card) => card.key === '01_main_image')!
    expect(main.prompt).toContain('取商品主体的主色调')
    expect(main.prompt).not.toContain('#')
  })

  it('识别卖点的提示词里要模型顺手返回主色', () => {
    expect(VISION_EXTRACT_PROMPT).toContain('main_color')
    expect(VISION_EXTRACT_PROMPT).toContain('main_color_hex')
    expect(VISION_EXTRACT_PROMPT).toContain('#RRGGBB')
  })
})

/**
 * 2026-10-26 主人反馈：「人工制作的仿生植物，物品有 120cm，模特的比例就不对了」——
 * 出图里那棵 120 厘米的仿真植物被画成桌上小盆栽。
 * 模型从照片里看不出多高，所以改成由用户在界面填实际尺寸，我们换算成人体参照写进提示词。
 */
describe('商品实际尺寸 → 模特比例 / 尺寸图数字', () => {
  const base = { product: '仿真天堂鸟植物', sp1: 'a', sp2: 'b', sp3: 'c' }

  it('尺寸文本能解析成厘米，几种写法都认', () => {
    expect(parseSizeCm('高 120 厘米')).toBe(120)
    expect(parseSizeCm('120cm')).toBe(120)
    expect(parseSizeCm('120 CM')).toBe(120)
    expect(parseSizeCm('1.2 m')).toBe(120)
    expect(parseSizeCm('1.2米')).toBe(120)
    expect(parseSizeCm('1200 mm')).toBe(120)
    expect(parseSizeCm('60×40×30 厘米')).toBe(60) // 取第一个数（主尺寸）
  })

  it('解析不出来就返回 null，不抛错', () => {
    expect(parseSizeCm('')).toBeNull()
    expect(parseSizeCm(undefined)).toBeNull()
    expect(parseSizeCm('大号')).toBeNull()
    expect(parseSizeCm('0')).toBeNull()
  })

  it('120cm 要判成「到胸口以上」，不能是小件', () => {
    const scale = describeHumanScale(120)
    expect(scale.zh).toContain('120 厘米')
    expect(scale.zh).toContain('胸口以上')
    expect(scale.zh).toContain('绝不能被画成捧在手里的小盆栽')
    expect(scale.en).toContain('above her chest')
  })

  it('中等件和小件各有对应说法', () => {
    expect(describeHumanScale(80).zh).toContain('腰到胸口')
    expect(describeHumanScale(20).zh).toContain('单手拿住的中小件')
  })

  it('第 3 张：填了尺寸就要把比例写死进去', () => {
    const card = buildOzonScript({ ...base, productSize: '高 120 厘米' }).find((c) => c.key === '03_lifestyle')!
    expect(card.prompt).toContain('商品实际尺寸：高 120 厘米')
    expect(card.prompt).toContain('胸口以上')
    expect(card.prompt).toContain('不许为了构图好看把它缩小')
    expect(card.promptEn).toContain('Actual product size')
    expect(card.promptEn).toContain('above her chest')
  })

  it('第 3 张：没填尺寸就退回「按实际大小推算」，不留空', () => {
    const card = buildOzonScript(base).find((c) => c.key === '03_lifestyle')!
    expect(card.prompt).not.toContain('商品实际尺寸')
    // 原来那句「按真实尺寸推算手」还在
    expect(card.prompt).toContain('手的比例必须按商品的真实尺寸来定')
  })

  it('第 7 张：填了尺寸，尺寸标注必须和它一致', () => {
    const card = buildOzonScript({ ...base, productSize: '高 120 厘米' }).find((c) => c.key === '07_size_chart')!
    expect(card.prompt).toContain('这件商品的实际尺寸是 高 120 厘米')
    expect(card.prompt).toContain('不要自己另估一个数')
    expect(card.promptEn).toContain('The actual size of this product is')
  })

  it('第 7 张：没填尺寸就不加这句', () => {
    const card = buildOzonScript(base).find((c) => c.key === '07_size_chart')!
    expect(card.prompt).not.toContain('这件商品的实际尺寸是')
  })

  it('尺寸只影响第 3、7 张，别的卡不带', () => {
    const cards = buildOzonScript({ ...base, productSize: '高 120 厘米' })
    for (const card of cards.filter((c) => !['03_lifestyle', '07_size_chart'].includes(c.key))) {
      expect(card.prompt).not.toContain('高 120 厘米')
    }
  })
})

describe('第 2 张对比图：必须看得出不是同一个东西', () => {
  const base = { product: '仿真天堂鸟植物', sp1: 'a', sp2: 'b', sp3: 'c' }

  it('给了五个可辨差异 + 一个自查标准', () => {
    const card = buildOzonScript(base).find((c) => c.key === '02_pain_compare')!
    expect(card.prompt).toContain('至少三处肉眼可辨的不同')
    expect(card.prompt).toContain('颜色：左边更暗、更灰、更土')
    expect(card.prompt).toContain('材质：左边塑料感')
    expect(card.prompt).toContain('体量：左边更小、更单薄')
    // 自查：缩到看不清文字也要分得出来
    expect(card.prompt).toContain('缩到看不清文字的尺寸时')
    // 明确禁止「复制右边只改背景」这种偷懒画法
    expect(card.prompt).toContain('不许把右边那张图原样复制到左边只改背景')
    expect(card.promptEn).toContain('differ in at least three visually obvious ways')
    expect(card.promptEn).toContain('Never take the lazy shortcut')
  })
})

describe('parseSellingPoints', () => {
  it('parses a bare JSON object', () => {
    const parsed = parseSellingPoints('{"product":"保温杯","sp1":"24 часа тепло"}')
    expect(parsed).toEqual({ product: '保温杯', sp1: '24 часа тепло' })
  })

  it('parses JSON wrapped in prose or a code fence', () => {
    const raw = '好的，结果如下：\n```json\n{"product":"保温杯","pain":"水凉得快"}\n```\n希望有帮助。'
    expect(parseSellingPoints(raw)).toEqual({ product: '保温杯', pain: '水凉得快' })
  })

  it('returns null when there is no JSON at all', () => {
    expect(parseSellingPoints('模型没有按要求返回')).toBeNull()
    expect(parseSellingPoints('')).toBeNull()
  })

  it('returns null on malformed JSON instead of throwing', () => {
    expect(parseSellingPoints('{"product": }')).toBeNull()
  })
})
