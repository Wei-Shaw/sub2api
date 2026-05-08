# GEO Audit Report: TokenProvider

**Audit Date:** 2026-05-07  
**URL:** https://tokenprovider.store/  
**Business Type:** SaaS (AI API gateway / token resale — unified Claude, ChatGPT, Gemini access)  
**Pages Analyzed:** 19 (full sitemap); 5 fetched in depth + homepage sampling  

---

## Executive Summary

**Overall GEO Score: 58/100 (Fair)**

TokenProvider 在 **英文静态落地页**（`en.html`、compare / guides / glossary / examples）上做了扎实的可引用内容：清晰 H1/H2、对比与 FAQ、`Article` + `FAQPage` JSON-LD、canonical 与 hreflang，有利于 AI 摘录「可自包含答案块」。主要短板是 **根路径 `/` 对不执行 JS 的抓取器几乎只有简短 SEO 壳**，与 Vue 主应用之间存在「首屏可见内容断层」，会拉低 AI 爬虫与部分引用场景下的可理解性。站点 **缺少 `llms.txt`**，`sitemap.xml` 在部分客户端上出现 **500**（curl 可 200），技术稳定性与「AI 可读站点地图」仍需加固。第三方品牌声量（百科、Reddit、媒体）未在本次审计中量化，默认按中等偏低计。

### Score Breakdown

| Category | Score | Weight | Weighted Score |
|---|---|---|---|
| AI Citability | 64/100 | 25% | 16.0 |
| Brand Authority | 48/100 | 20% | 9.6 |
| Content E-E-A-T | 60/100 | 20% | 12.0 |
| Technical GEO | 56/100 | 15% | 8.4 |
| Schema & Structured Data | 76/100 | 10% | 7.6 |
| Platform Optimization | 42/100 | 10% | 4.2 |
| **Overall GEO Score** | | | **57.8 → 58/100** |

---

## Critical Issues (Fix Immediately)

1. **根站 `/` 对无 JS 抓取器内容过薄**  
   - **现象：** 通过轻量抓取（类 AI 预检）时，首页几乎只返回 SEO 标题与短段落，缺少 `en.html` 上那种长文、FAQ、内链簇。  
   - **风险：** 依赖 HTML 快照的模型或工具可能 **低估站点价值** 或无法引用完整价值主张。  
   - **建议：** 在 **不破坏现有 SEO 壳** 的前提下，为 `/` 增加与 `en.html` 同级的 **静态可读主内容块**（可折叠/与 SPA 共存），或采用 **轻量 SSR/预渲染** 输出首屏 Markdown 友好段落；至少保证 `<main>` 内有一段 300–600 字的结构化说明 + 指向核心文档的内链。

2. **`/sitemap.xml` 存在不稳定响应**  
   - **现象：** 部分抓取路径返回 **HTTP 500**，同一 URL 用 curl 可 **200**。  
   - **风险：** AI/搜索爬虫偶发拿不到 sitemap，影响发现速度与信任。  
   - **建议：** 排查反向代理、压缩、超时与后端路由；对 sitemap 使用 **纯静态文件** 或 CDN 直出并监控 5xx。

---

## High Priority Issues

1. **缺少 `/llms.txt`（404）**  
   - GEO 技能与行业实践均强调 llms.txt 作为「站点地图 + 政策 + 高价值页」入口。  
   - **建议：** 在站点根目录发布 `llms.txt`，列出：站点简介、允许用途、核心文档 URL、定价/条款链接、`en.html` 与 3–5 篇最高优先级文章。

2. **可引用页面总量偏少（sitemap 约 19 URL）**  
   - 对比/教程/术语已覆盖核心意图，但 **长尾与集群**（集成、各 IDE、各语言示例、故障排查）可扩展。  
   - **建议：** 按主题簇扩展至 40–80 个高质量静态 URL，并保持互链。

3. **E-E-A-T：作者与背书偏「组织单一署名」**  
   - 静态页多用 `Organization` 作为 `author`，缺少 **可验证的实体信息**（公司主体、地址/支持渠道、合规声明）。  
   - **建议：** 增加规范的 **About / Trust / Contact** 静态页；在 JSON-LD 中补充 `Organization`（`url`、`sameAs`、客服邮箱）、必要时 `WebSite` + `SearchAction`。

---

## Medium Priority Issues

1. **首页与 SPA 的「anti-FOUC」策略**  
   - 对用户有益，但需确认 **纯文本爬虫** 仍能看到与 `en.html` 等价的实质段落（当前抓取样本偏薄）。  

2. **`robots.txt` 未单独声明 AI 爬虫**  
   - 当前 `User-agent: *` 对公开页为 Allow，整体友好；若未来收紧规则，建议 **显式允许** `GPTBot`、`ClaudeBot`、`Google-Extended` 等对营销与文档路径，避免误伤。  

3. **图片与 OG**  
   - 多页使用 `/logo.png` 作为 `og:image`；可为核心文章生成 **1200×630** 专用图，提高分享与部分 AI 卡片质量。  

---

## Low Priority Issues

1. 部分页面 `dateModified` 可随内容更新同步，传递新鲜度信号。  
2. 内链锚文本可再多样化（避免过度精确匹配堆砌）。  
3. 英文站可补充 `BreadcrumbList` JSON-LD 以增强层级理解。  

---

## Category Deep Dives

### AI Citability (64/100)

**优势：**  
- `en.html` 具备 **清晰问题式 H2/H3、列表、对比段、FAQ**，适合被摘成短答案。  
- 对比页（如 `en/compare/claude-vs-chatgpt-api.html`）含 **可引用数据与结论句**（定价、编码、代理成本等）。  

**差距：**  
- 根路径 `/` 的 **可摘录密度** 明显低于 `en.html`。  
- Sitemap 规模小，**覆盖的查询意图**有限。  

**建议：** 每一篇 compare/guide 末尾增加 **「TL;DR / 三句话结论」** 块；中文首页补充与英文对等的 **静态长内容区**。

### Brand Authority (48/100)

未做全量第三方抓取；基于典型 SaaS 新站假设：维基、主流媒体、高赞 Reddit 线程可能不足。  

**建议：** 技术博客客座、GitHub 示例仓库、独立开发者社区（非垃圾外链）**自然提及**；在站内「客户/案例」页用可验证描述（脱敏）增强实体可信度。

### Content E-E-A-T (60/100)

**优势：** 教程与示例页体现实操，利于「经验」信号。  
**差距：** 缺少独立 **About、团队、法律实体、支持 SLA** 等信任页；作者维度几乎全是 Organization。  

**建议：** 增加 **Trust Center**（隐私、退款、滥用政策、联系方式）；`Organization` schema 填 `sameAs`（GitHub、Twitter/X、产品页）。

### Technical GEO (56/100)

**优势：**  
- `robots.txt` 对公开营销页 **Allow**，对 `/admin`、`/api/` 等 **Disallow**，边界合理。  
- 静态 HTML 子站 **不依赖 JS 即可读全文**，利于 AI 抓取。  

**差距：**  
- **无 llms.txt**。  
- **sitemap 稳定性**存疑。  
- 首页仍以 SPA 为主，**与静态卫星页策略不一致**。  

### Schema & Structured Data (76/100)

**优势：** 抽样页含 **`Article` + `FAQPage`**（及首页 `SoftwareApplication` 类信号）；OG/Twitter 基本齐全。  

**差距：** 可补充 **`WebSite`、`Organization`（全站级）**、`BreadcrumbList`；大页面可评估 **`HowTo`**（分步骤教程）。  

### Platform Optimization (42/100)

未验证 YouTube、Reddit、Wikipedia 实体；默认偏低。  

**建议：** 固定渠道发布 **短教程视频** + **开发者 Reddit 答疑**（遵守各版规），建立可检索的品牌提及。

---

## Quick Wins (Implement This Week)

1. **上线 `https://tokenprovider.store/llms.txt`**，链接 `en.html`、2 篇对比、2 篇指南、条款/隐私。  
2. **修复 sitemap 500** 并加监控告警。  
3. **中文 `/` 增加与 `en.html` 同信息密度的静态 `<main>` 内容**（或 server 侧注入一段可读摘要）。  
4. **全站统一 `Organization` JSON-LD**（含 `sameAs`、logo、`url`）。  
5. **任选 1 篇指南加 `HowTo` schema**（验证后用 Rich Results 测试工具自检）。  

---

## 30-Day Action Plan

### Week 1: Technical baseline
- [ ] 排查并消除 `sitemap.xml` 500；考虑静态化 sitemap  
- [ ] 发布并提交 `llms.txt`（GSC 可顺带提交）  
- [ ] 根路径 `/` 静态可读内容增强或预渲染方案定稿  

### Week 2: Content cluster
- [ ] 新增 4–6 篇长尾（IDE/CLI/语言/排错）静态页并写入 sitemap  
- [ ] 所有 compare/guide 增加 TL;DR 块  

### Week 3: Trust & entity
- [ ] About / Trust / Contact 静态页 + 内链  
- [ ] `Organization` + `WebSite` schema 全站模板化  

### Week 4: Distribution
- [ ] 1 篇深度技术文外发 + 1 个可引用开源示例仓库  
- [ ] 复盘 Search Console + 手动在 ChatGPT/Perplexity 测品牌与 URL 引用率  

---

## Appendix: Pages Analyzed

| URL | Title (live) | GEO Issues |
|---|---|---|
| https://tokenprovider.store/ | TokenProvider — … (SPA+壳) | 无 JS 抓取内容偏薄；需加强 |
| https://tokenprovider.store/en.html | Cheap Claude API & … | 强；建议保持更新日期与内链 |
| https://tokenprovider.store/compare/claude-vs-chatgpt-api.html | （中文对比） | 良好 |
| https://tokenprovider.store/en/compare/claude-vs-chatgpt-api.html | Claude API vs ChatGPT API (2026)… | Article+FAQ；优 |
| https://tokenprovider.store/en/compare/claude-code-relay-vs-subscription.html | （静态） | 建议抽查 schema |
| https://tokenprovider.store/en/compare/tokenprovider-vs-official-claude.html | （静态） | 建议抽查 schema |
| https://tokenprovider.store/en/guides/claude-code-setup.html | （静态） | 可加 HowTo |
| https://tokenprovider.store/en/guides/cursor-claude-proxy.html | （静态） | 可加 HowTo |
| https://tokenprovider.store/en/guides/cline-cheap-claude.html | （静态） | 可加 HowTo |
| https://tokenprovider.store/en/examples/claude-api-python.html | （静态） | 代码块利于引用 |
| https://tokenprovider.store/en/examples/claude-api-nodejs.html | （静态） | 同上 |
| https://tokenprovider.store/en/examples/chatgpt-api-curl.html | （静态） | 同上 |
| https://tokenprovider.store/en/examples/gemini-api-example.html | （静态） | 同上 |
| https://tokenprovider.store/en/glossary/what-is-claude-relay.html | （静态） | 术语定义利于 AI |
| https://tokenprovider.store/en/glossary/what-is-claude-code.html | （静态） | 同上 |
| https://tokenprovider.store/en/glossary/what-is-an-ai-token.html | （静态） | 同上 |
| https://tokenprovider.store/guides/claude-code-setup.html | （中文） | 与 EN 对齐更新 |
| https://tokenprovider.store/examples/claude-api-python.html | （中文） | 与 EN 对齐 |
| https://tokenprovider.store/glossary/what-is-claude-relay.html | （中文） | 与 EN 对齐 |

---

*本报告基于公开 URL 抓取与仓库内静态页结构推断；品牌声量未做全渠道穷尽检索。实施改动后建议复测并更新分数。*
