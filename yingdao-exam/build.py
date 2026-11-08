#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
build.py —— 把 data/questions.json（手工核对）+ data/scraped.json（自动抓取）
合并去重后，导出成 Markdown 和 Excel 题库。

用法:
    python build.py

产物:
    out/影刀初级题库.md
    out/影刀初级题库.xlsx
"""
from __future__ import annotations

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DATA = ROOT / "data"
MANUAL = DATA / "questions.json"
SCRAPED = DATA / "scraped.json"
OUT = ROOT / "out"

TYPE_ORDER = {"单选": 0, "多选": 1, "操作题": 2}


def norm_key(stem: str) -> str:
    """题干归一化，用于跨来源去重。"""
    s = re.sub(r"[\s\u3000，。、；：？！“”‘’（）【】《》,.;:?!\"'()\[\]<>]", "", stem)
    s = re.sub(r"^【题型[A-F]】", "", s)
    s = re.sub(r"^[0-9]{1,2}[、.．]", "", s)
    return s[:40]


def sig(q: dict) -> str:
    """
    去重签名 = 归一化题干 + 排序后的选项集合。

    影刀初级里「流程如图，运行结果为？」这种题干重复出现，靠配图区分，
    只按题干去重会误合并；带上选项就能区分开。
    """
    opts = "|".join(sorted(norm_key(o) for o in q.get("options", []) if o))
    return norm_key(q["stem"]) + "#" + opts


# 题干里的「噪声词」：这些字眼在不同来源/不同人的转写里加不加都正常
_NOISE = re.compile(r"(如下图|如图|流程|运行结果|所示|下图|以上|以下)")


def loose_stem(stem: str) -> str:
    """
    更狠的题干归一化，专门用来判断「这是不是同一道题」。

    影刀初级同一道题在不同博客里常被写成：
        关于【IF条件】的说法，错误的是？
        如下图，关于【IF 条件】说法错误的是?
    只差空格、标点和「如下图」三个字。用这个函数把它们认成同一道。
    """
    s = _NOISE.sub("", stem)
    s = re.sub(r"[^\u4e00-\u9fa5A-Za-z0-9]", "", s)
    return s[:24]


# 宽松题干短于这个长度就不可靠了：像「流程如图，运行结果为？」剥掉噪声词后
# 只剩「为」，4 道不同的题会全撞在一起。这种一律退回严格签名比较。
LOOSE_MIN_LEN = 8


def op_label(stem: str) -> str | None:
    """操作题的真实身份是题型编号（A~F），题干措辞在不同来源会有出入。"""
    m = re.search(r"【题型\s*([A-F])\s*】", stem)
    return m.group(1) if m else None


def load() -> dict:
    """读手工库；若存在抓取结果，把新题并进去（手工库优先）。"""
    with MANUAL.open(encoding="utf-8") as f:
        doc = json.load(f)

    if not SCRAPED.exists():
        doc["meta"]["scrapedNote"] = "未找到 scraped.json，仅导出手工整理部分"
        return doc

    with SCRAPED.open(encoding="utf-8") as f:
        scraped = json.load(f)

    existing = {sig(q) for q in doc["questions"]}
    # 操作题按题型编号去重：手工库已经写过【题型A】，抓来的 A 卷就不再重复收
    existing_ops = {op_label(q["stem"]) for q in doc["questions"] if q["type"] == "操作题"}
    existing_ops.discard(None)
    # 选择题用宽松题干认重：同一道题在不同来源里标点/「如下图」写法不一致
    existing_loose = {loose_stem(q["stem"]) for q in doc["questions"]
                      if q["type"] != "操作题" and len(loose_stem(q["stem"])) >= LOOSE_MIN_LEN}

    added = 0
    skipped_dup = 0
    for q in scraped.get("questions", []):
        k = sig(q)
        if k in existing:
            skipped_dup += 1
            continue
        if q["type"] == "操作题":
            lbl = op_label(q["stem"])
            if lbl and lbl in existing_ops:
                skipped_dup += 1
                continue
            if lbl:
                existing_ops.add(lbl)
        else:
            ls = loose_stem(q["stem"])
            if len(ls) >= LOOSE_MIN_LEN and ls in existing_loose:
                skipped_dup += 1
                continue
            if len(ls) >= LOOSE_MIN_LEN:
                existing_loose.add(ls)
        existing.add(k)
        # 抓来的题一律待核对，答案留空
        q = dict(q)
        q["answer"] = q.get("answer", "")
        q["confidence"] = "待核对"
        q["note"] = q.get("note") or "自动抓取，答案需人工核对"
        q["source"] = q.get("source", "")
        q["origin"] = f"抓取 · {q.get('sourceName', '')}"
        doc["questions"].append(q)
        added += 1

    doc["meta"]["scrapedNote"] = (
        f"合并自动抓取结果，新增 {added} 道、跳过重复 {skipped_dup} 道"
        f"（抓取时间 {scraped.get('meta', {}).get('generatedAt', '?')}）"
    )
    doc["meta"]["scrapedSkipped"] = scraped.get("meta", {}).get("skipped", [])
    return doc


def group_by_type(questions: list[dict]) -> dict[str, list[dict]]:
    buckets: dict[str, list[dict]] = {}
    for q in questions:
        buckets.setdefault(q["type"], []).append(q)
    for v in buckets.values():
        v.sort(key=lambda q: (q.get("topic", ""), q["id"]))
    return dict(sorted(buckets.items(), key=lambda kv: TYPE_ORDER.get(kv[0], 99)))


def stats(questions: list[dict]) -> dict:
    total = len(questions)
    by_type: dict[str, int] = {}
    by_conf: dict[str, int] = {}
    for q in questions:
        by_type[q["type"]] = by_type.get(q["type"], 0) + 1
        by_conf[q["confidence"]] = by_conf.get(q["confidence"], 0) + 1
    answered = sum(1 for q in questions if q.get("answer"))
    return {
        "total": total,
        "by_type": by_type,
        "by_confidence": by_conf,
        "answered": answered,
        "unanswered": total - answered,
    }


def render_md(doc: dict) -> str:
    meta = doc["meta"]
    qs = doc["questions"]
    st = stats(qs)
    lines: list[str] = []

    lines.append(f"# {meta['title']}")
    lines.append("")
    lines.append(f"> 整理日期：{meta['version']}　|　范围：{meta['scope']}")
    lines.append("")
    lines.append("## 一、考试基本信息（官方口径）")
    lines.append("")
    lines.append("| 项目 | 说明 |")
    lines.append("|---|---|")
    for k, label in [
        ("passScore", "及格分"),
        ("duration", "考试时长"),
        ("entry", "考试入口"),
        ("types", "题型"),
        ("multiChoiceRule", "多选判分"),
        ("review", "出证审核"),
    ]:
        lines.append(f"| {label} | {meta['examFacts'][k]} |")
    lines.append("")
    lines.append(f"来源：{meta['examFacts']['source']}")
    lines.append("")

    lines.append("## 二、本库统计")
    lines.append("")
    lines.append(f"- 总题数：**{st['total']}**（有答案 {st['answered']} / 待核对 {st['unanswered']}）")
    lines.append("- 按题型：" + "，".join(f"{k} {v} 道" for k, v in st["by_type"].items()))
    lines.append("- 按可信度：" + "，".join(f"{k} {v} 道" for k, v in st["by_confidence"].items()))
    if meta.get("scrapedNote"):
        lines.append(f"- {meta['scrapedNote']}")
    lines.append("")
    lines.append("**可信度说明**")
    lines.append("")
    for k, v in meta["confidenceLegend"].items():
        lines.append(f"- `{k}`：{v}")
    lines.append("")
    lines.append("> ⚠️ 带图题（`待核对`）的答案依赖原题截图或流程图，本库只保留题干与选项，"
                 "答案请回原题对照。**多选题漏选即 0 分，务必按判断标准推，不要背字母。**")
    lines.append("")

    skipped = meta.get("scrapedSkipped") or []
    if skipped:
        lines.append("**抓取时跳过的来源**（正文是截图，文字抽不出来，需要人工去看）")
        lines.append("")
        for s in skipped:
            lines.append(f"- [{s['name']}]({s['url']}) — {s['reason']}")
        lines.append("")

    idx = 1
    for qtype, items in group_by_type(qs).items():
        title = f"## 三、{qtype}题（{len(items)} 道）" if qtype != "操作题" else f"## 四、操作题（{len(items)} 道）"
        lines.append(title)
        lines.append("")
        for q in items:
            head = f"### {idx}. [{q['id']}]"
            if q.get("topic"):
                head += f" {q['topic']}"
            if q.get("origin"):
                head += f"　<sub>{q['origin']}</sub>"
            lines.append(head)
            lines.append("")
            for i, para in enumerate(q["stem"].split("\n")):
                lines.append(para if i == 0 else f"　　{para}")
            lines.append("")
            if q["options"]:
                for letter, opt in zip("ABCDEFG", q["options"]):
                    lines.append(f"- **{letter}.** {opt}")
                lines.append("")
            ans = q.get("answer") or "—（见备注）"
            lines.append(f"**答案：{ans}**　`{q['confidence']}`")
            lines.append("")
            if q.get("note"):
                lines.append(f"**备注：** {q['note']}")
                lines.append("")
            lines.append(f"<sub>来源：{q['source']}</sub>")
            lines.append("")
            lines.append("---")
            lines.append("")
            idx += 1

    lines.append("## 五、考场避坑清单")
    lines.append("")
    lines.append("- [ ] 题干问的是「正确的是」还是「错误的是」——先圈出来再动笔")
    lines.append("- [ ] 带题图的题，图看了没有")
    lines.append("- [ ] 多选题是不是有漏选（漏一个即零分）")
    lines.append("- [ ] 流程推演题是在**数**，还是在猜")
    lines.append("- [ ] 概念题记住的是原理，还是答案字母（同一题干不同卷答案会变）")
    lines.append("- [ ] 操作题有没有亲手搭过一遍")
    lines.append("- [ ] 操作题提交前：影刀客户端停在初始界面、账号一致")
    lines.append("")
    return "\n".join(lines)


def render_xlsx(doc: dict, path: Path) -> None:
    from openpyxl import Workbook
    from openpyxl.styles import Alignment, Font, PatternFill
    from openpyxl.utils import get_column_letter

    wb = Workbook()
    ws = wb.active
    ws.title = "题库"

    headers = ["编号", "题型", "考点", "题干", "选项A", "选项B", "选项C", "选项D",
               "答案", "可信度", "备注", "来源"]
    widths = [8, 8, 18, 60, 28, 28, 28, 28, 12, 10, 48, 42]

    head_fill = PatternFill("solid", fgColor="1F4E79")
    head_font = Font(color="FFFFFF", bold=True)
    for c, (h, w) in enumerate(zip(headers, widths), start=1):
        cell = ws.cell(row=1, column=c, value=h)
        cell.fill = head_fill
        cell.font = head_font
        cell.alignment = Alignment(horizontal="center", vertical="center")
        ws.column_dimensions[get_column_letter(c)].width = w
    ws.freeze_panes = "A2"

    for r, q in enumerate(doc["questions"], start=2):
        opts = list(q["options"])[:4]
        opts += [""] * (4 - len(opts))
        row = [q["id"], q["type"], q.get("topic", ""), q["stem"],
               opts[0], opts[1], opts[2], opts[3],
               q.get("answer", ""), q["confidence"], q.get("note", ""), q.get("source", "")]
        for c, val in enumerate(row, start=1):
            cell = ws.cell(row=r, column=c, value=val)
            cell.alignment = Alignment(wrap_text=True, vertical="top")
        # 待核对的整行标黄，方便一眼看出哪些还没答案
        if q["confidence"] == "待核对":
            for c in range(1, len(headers) + 1):
                ws.cell(row=r, column=c).fill = PatternFill("solid", fgColor="FFF2CC")

    ws.auto_filter.ref = f"A1:{get_column_letter(len(headers))}{len(doc['questions']) + 1}"

    # 第二个 sheet：统计
    ws2 = wb.create_sheet("统计")
    st = stats(doc["questions"])
    ws2.append(["指标", "数值"])
    ws2["A1"].font = Font(bold=True)
    ws2["B1"].font = Font(bold=True)
    ws2.append(["总题数", st["total"]])
    ws2.append(["有答案", st["answered"]])
    ws2.append(["待核对", st["unanswered"]])
    for k, v in st["by_type"].items():
        ws2.append([f"题型-{k}", v])
    for k, v in st["by_confidence"].items():
        ws2.append([f"可信度-{k}", v])
    ws2.column_dimensions["A"].width = 18
    ws2.column_dimensions["B"].width = 12

    path.parent.mkdir(parents=True, exist_ok=True)
    wb.save(path)


def main() -> int:
    doc = load()
    OUT.mkdir(parents=True, exist_ok=True)

    md_path = OUT / "影刀初级题库.md"
    md_path.write_text(render_md(doc), encoding="utf-8")

    xlsx_path = OUT / "影刀初级题库.xlsx"
    render_xlsx(doc, xlsx_path)

    st = stats(doc["questions"])
    print(f"[ok] 共 {st['total']} 道（有答案 {st['answered']}，待核对 {st['unanswered']}）")
    print(f"[ok] {md_path}")
    print(f"[ok] {xlsx_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
