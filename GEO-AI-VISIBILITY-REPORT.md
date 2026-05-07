# TokenProvider — AI 可见性专项报告

**日期:** 2026-05-07  
**站点:** https://tokenprovider.store/  
**说明:** 侧重 **AI 可见性与可引用性**，与根目录 `GEO-AUDIT-REPORT.md` 互补。

---

## 执行摘要

- **静态英文集群（`/en.html`、compare / guides / glossary / examples）可引用性很强**：定义清晰、表格与「短答案 + 依据」段落多，极适合 Perplexity / 带网页检索的问答产品抓取后当引用片段。
- **根路径 `/` 与中文版首页对「非执行 JS 的抓取」仍偏薄**：与 `en.html` 的信息量差距大，会拉低「默认 URL」被模型摘句的概率。
- **`robots.txt` 未单独封禁常见 AI UA**：在默认 `User-agent: *` 下，公开静态页整体 **Allow**，API 与后台路径 **Disallow** 合理；未发现针对 GPTBot / Claude 等的显式封锁组（也未见 `Crawl-delay`）。
- **`/llms.txt` 仍为 404**；**`sitemap.xml` 在自动化抓取工具中曾出现 500**，本机 `curl` 抽检为 **200** 且内容正常——说明 **站点地图可用性不稳定**，对 Bing / Google 的发现链路是真实风险。
- **品牌实体在「百科 / 主流社交证明」侧偏弱**：英文维基无独立 `TokenProvider` 条目；抽检的 LinkedIn `/company/tokenprovider/` 为 404；公开检索中更易看到同类网关竞品而非本品牌——**模型「知不知道你是谁」主要仍依赖官网与少数第三方提及**。

---

## 1）各 AI 体系如何发现 / 引用该域（机制向）

| 体系 | 发现路径（典型） | 对当前站点的含义 |
|------|------------------|------------------|
| **ChatGPT** | 浏览/检索依赖 Bing 索引或实时拉页；训练侧另受 Common Crawl 等影响（`CCBot` 等） | 静态 HTML 质量有利于「被拉页」时的摘句；**首页偏薄**时，模型更少把 `https://tokenprovider.store/` 当作首选引用落点；无 `llms.txt` 少了一条「站方指定摘要」通道。 |
| **Claude（含联网）** | 产品内网页检索 + 通用索引 | 同上；**长文对比页 / 术语页**最像「可被整段引用」的来源。 |
| **Perplexity** | 强检索 + 引用卡片 | **高**：FAQ、对比表、带数字的定价论述（你方 compare/glossary 已具备）与 Perplexity 的 citation UI 匹配度高。 |
| **Gemini** | 强依赖 Google 抓取与索引；`Google-Extended` 仅影响训练子集 | 需要 **稳定 sitemap + 可爬 HTML**；sitemap 若间歇 5xx，会伤害「可预期发现」。 |
| **Bing Copilot** | Bing 索引为主 | 与 sitemap、服务器稳定性、内链结构强相关；**sitemap 500 类问题应视为 P0 级工程债**。 |

**结论**：当前站点 **「深度静态页 ≈ 引用友好」**，**「根域默认落地 ≈ 引用不友好」**；发现链路的最大工程风险是 **sitemap 不稳定**，最大 GEO 缺口是 **无 llms.txt + 品牌实体信号弱**。

---

## 2）可引用性（Citability）：首页 vs `en.html` vs 静态集群

按「**能否被 AI 直接当答案摘句**」评估。

| 页面类型 | 页面级可引用性（粗分） | 说明 |
|----------|------------------------|------|
| **`/`（中文短首页）** | **~22–30 / 100（偏低）** | 实质内容约 1 段陈述，缺独立定义、缺可扫描结构、缺可验证数据点 → **citation-unlikely 为主**。 |
| **`/en.html`** | **~68–76 / 100（良好）** | 有清晰 H1/H2、能力点、提供商列表、FAQ；**多条 FAQ 已达 citation-ready**；仍可增强「可验证数字/第三方背书」。 |
| **`/en/compare/*.html`** | **~85–92 / 100（优秀）** | 短结论前置、对比表、`$/M token`、场景化选型列表、迁移代码块 → **非常适合**被检索答案引用。 |
| **`/en/guides/*.html`** | **~78–85 / 100（优秀）** | 步骤 + 故障表 + 环境变量「可复制块」→ 教程类查询易引用。 |
| **`/en/glossary/*.html`** | **~82–88 / 100（优秀）** | 定义段自包含、类比 + 成本表 + 合规/风险边界 → 术语类问题命中率高。 |

**Top citation-ready 模式（已具备，应保护并复用）**

- 段首 **1–3 句「短答案」**（compare 页最明显）。
- **表格承载对比维度和数字**（token 价格、窗口、工具链成熟度）。
- **FAQ 用独立可剪贴段落**（避免整页只有营销口号）。

**citation-unlikely 区域**

- 根路径与任何「仅 slogan、无结构」的落地页。
- 过度依赖「去注册看价」而缺少「至少一个可公开的锚点数字或区间」的段落（部分 FAQ 仍偏软）。

---

## 3）`robots.txt` 与 AI 爬虫姿态（实测摘要）

**文件结构**：仅见 **`User-agent: *`** 一组；无 `GPTBot`、`ClaudeBot`、`OAI-SearchBot` 等独立段落。

**含义（实务解读）**

- 未列名的爬虫 **继承 `*`**：对 `Allow` 的公开路径（含 `/en.html` 与静态 HTML）**默认可抓**；`/api/`、`/v1/`、后台型路径 **统一 Disallow** — 对网关产品这是合理边界。
- **未发现**「全域 `Disallow: /`」类误伤。
- **未发现** `Crawl-delay`（不额外拖慢）。

**风险点不在 robots，而在**：HTML 供给（根路径薄）、**sitemap 稳定性**、以及 **站外实体信号**。

---

## 4）分项得分（内部跟踪）

| 组件 | 分数 | 备注 |
|------|------|------|
| **可引用性（站点有效深度）** | **~73/100** | 长文页拉高；根路径拉低「默认引用落点」期望。 |
| **爬虫可达性（robots + sitemap 工程）** | **~88/100** | robots 友好；**sitemap 间歇失败**扣分。 |
| **llms.txt** | **0/100** | 404。 |
| **品牌/实体提及** | **~18–25/100** | 维基无条目；LinkedIn 抽检 404；公开讨论与 YT **未观测到强官方实体锚点**（轻量检索，非穷尽）。 |

**综合 AI 可见性（加权示意）**：约 **52–56 / 100**（**Fair**）  
权重示意：`Citability 35% + Brand 30% + Crawler 25% + llms.txt 10%`。

---

## 5）优先级建议（P0 / P1 / P2）

### P0（立刻做）

- **消灭 `sitemap.xml` 的 5xx**：对 CDN / 源站 / 生成任务做监控与告警。
- **根路径 `/` 的「非 JS 抓取友好」**：服务端注入与 `en.html` **同级**的核心说明区，或对根路径做稳妥的 **SSR/预渲染**（避免争议性 cloaking）。
- **上线 `/llms.txt`**（见下节大纲）。

### P1（两周内）

- **Organization / SoftwareApplication 级 JSON-LD**：与现有 Article + FAQPage **并存**（注意类型别冲突）。
- **每类旗舰页固定「可引用事实」**：公开计费单位、日志字段边界、最小充值或典型区间等。
- **内链**：从 `/` 与 `en.html` **显式链到** 最强 citation 页。

### P2（持续）

- **可索引的官方身份占位**：LinkedIn、GitHub org、文档/status 子域。
- **官方 YouTube** 短教程（高意图标题）。
- **社区外发**：HN / PH / newsletter — 目标为 **可检索的第三方句子**。

---

## 6）`llms.txt` 大纲（草案）

```markdown
# TokenProvider

> OpenAI-compatible API gateway for Claude, Claude Code, ChatGPT, and Gemini. Pay-as-you-go tokens, per-request billing, sticky sessions.

## Primary
- [English product overview](https://tokenprovider.store/en.html)
- [Chinese overview](https://tokenprovider.store/)

## Comparisons
- [Claude API vs ChatGPT API](https://tokenprovider.store/en/compare/claude-vs-chatgpt-api.html)
- [Claude Code: proxy vs subscription](https://tokenprovider.store/en/compare/claude-code-relay-vs-subscription.html)
- [TokenProvider vs official Claude API](https://tokenprovider.store/en/compare/tokenprovider-vs-official-claude.html)

## Guides
- [Claude Code setup](https://tokenprovider.store/en/guides/claude-code-setup.html)
- [Cursor + Claude proxy](https://tokenprovider.store/en/guides/cursor-claude-proxy.html)
- [Cline + cheap Claude API](https://tokenprovider.store/en/guides/cline-cheap-claude.html)

## Examples
- [Claude API Python](https://tokenprovider.store/en/examples/claude-api-python.html)
- [Claude API Node.js](https://tokenprovider.store/en/examples/claude-api-nodejs.html)
- [ChatGPT API curl](https://tokenprovider.store/en/examples/chatgpt-api-curl.html)
- [Gemini API](https://tokenprovider.store/en/examples/gemini-api-example.html)

## Glossary
- [What is a Claude API proxy?](https://tokenprovider.store/en/glossary/what-is-claude-relay.html)
- [What is Claude Code?](https://tokenprovider.store/en/glossary/what-is-claude-code.html)
- [What is an AI token?](https://tokenprovider.store/en/glossary/what-is-an-ai-token.html)

## Policies & trust
- （补充公开 Terms / Privacy URL 与数据留存说明）

## Optional
- [Sitemap](https://tokenprovider.store/sitemap.xml)
```

---

## 7）实体「TokenProvider」一页策略（创始人向）

- **定位句**：一句话包含 *OpenAI-compatible*、*Claude / Claude Code / ChatGPT / Gemini*、*pay-as-you-go*、*sticky sessions*、与**官方直签的边界**（合规/数据/SLA）。
- **三类必赢查询**：Claude vs GPT 选型 + 价格；Claude Code / `ANTHROPIC_BASE_URL`；relay/proxy 定义与安全。
- **信任栈**：用可机读方式写清日志字段、prompt 是否存储、退款/账单导出、上游故障行为。
- **工程底线**：**sitemap 永远 200**、`llms.txt` **永远 200**、根域 **有可与 `en.html` 匹敌的 SSR 文本块**。

---

*抽检说明：`sitemap.xml` 在部分抓取路径曾报 500，curl 可为 200；品牌扫描为抽样，非全渠道穷尽。*
