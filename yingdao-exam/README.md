# 影刀 RPA 初级认证 · 本地题库

抓取 + 整理 + 导出，一条流水线。**只做初级**，不碰中高级。

## 目录

```
yingdao-exam/
├── scrape.py            抓取公开网页 → data/scraped.json
├── build.py             合并去重 → out/影刀初级题库.{md,xlsx}
├── data/
│   ├── questions.json   手工整理 + 已核对答案（唯一权威）
│   ├── scraped.json     自动抓取结果（答案一律待核对，可随时重跑覆盖）
│   └── cache/           HTTP 缓存，删了下次会重抓
└── out/
    ├── 影刀初级题库.md
    └── 影刀初级题库.xlsx
```

## 用法

```powershell
# 1) 抓取（有缓存用缓存）
python scrape.py

# 2) 强制重新抓
python scrape.py --refresh

# 3) 只列出来源
python scrape.py --list

# 4) 生成 Markdown + Excel
python build.py
```

Windows 下如果 `python` 不在 PATH，用 DSH 自带的：

```powershell
& "C:\Users\Administrator\.dsh\dsh-runtimes\dsh-primary-runtime\dependencies\python\python.exe" build.py
```

## 数据分两层，别混

| 文件 | 谁写的 | 可信度 | 会被覆盖吗 |
|---|---|---|---|
| `data/questions.json` | 人工 | 高（标注了 `多源一致` / `推断` / `待核对`） | 不会 |
| `data/scraped.json` | `scrape.py` | 低（全部 `待核对`） | 每次重跑都会 |

`build.py` 合并时**手工库优先**：抓来的题如果和手工题重复（题干 + 选项集合都相同），直接丢掉。

## 可信度三级

- `多源一致` —— 多篇独立资料给出同一答案，最可信
- `推断` —— 按影刀官方文档的行为推导，未获官方答案背书
- `待核对` —— 题干依赖配图/流程图，脱离原图无法判定

## 加新来源

改 `scrape.py` 顶部的 `SOURCES` 列表，`parser` 三选一：

- `csdn_sections` —— 页面有「初级认证 / 一、单选题」这类章节结构（如 Ttwumian 那篇）
- `ops_page` —— 整页就是一道操作题，顶部可能带「关联链接」导航
- `csdn_images` —— 正文全是截图，抽不出文字（会被记入 `meta.skipped`，提醒人工去看）

**加来源前务必人工看一眼**：搜「影刀题库」排在前面的文档站大量是套壳货，
有的是 Photoshop 的题，有的是来也 UiBot 的题。鉴别特征——真影刀题里高频出现
`循环相似元素(web)`、`二分支`、`最大相似原则`、`影刀商城`、`题型A~F`。

## 已知缺口

- 四篇 `lingyaoJUN` 的「IT速成课程_1/2/3/多选_4」正文全是截图，文字抓不到，
  已记入 `meta.skipped`。要看只能人工打开原文。
- 56 道 `待核对` 题没有答案（多为带图题），需要对着原题图片判断。
- 官方没有公开题库，本库覆盖度有限。要全量请自行评估付费题库。
