#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
scrape.py —— 从公开网页抓取影刀初级考试题目，产出 data/scraped.json。

设计原则
--------
1. 只抓**公开可访问**的博客/文档页，不碰登录态内容，不绕过任何付费墙。
2. 抓取结果单独存放（scraped.json），**不覆盖**手工整理并核对过的
   questions.json —— 抓来的题一律标 `待核对`，因为网页上的选项顺序、
   题干措辞经常和实际考卷不一致，答案更是经常错。
3. 每个来源都写进输出的 `source` 字段，方便回溯原文。
4. 有本地缓存（data/cache/），重复运行不会反复打人家服务器。

用法
----
    python scrape.py              # 有缓存用缓存，没缓存就抓
    python scrape.py --refresh    # 强制重新抓取
    python scrape.py --list       # 只列出配置的来源

依赖：仅标准库（urllib + re + html）。
"""
from __future__ import annotations

import argparse
import hashlib
import html as htmllib
import json
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent
DATA = ROOT / "data"
CACHE = DATA / "cache"
OUT_FILE = DATA / "scraped.json"

UA = ("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
      "(KHTML, like Gecko) Chrome/124.0 Safari/537.36")

# ---------------------------------------------------------------- 来源配置
# 只放已确认「内容确实是影刀真题」的页面。
# 注意：网上大量「影刀题库.doc」其实是 Photoshop 或来也 UiBot 的题，
#      抓之前务必人工看一眼，别把垃圾灌进库里。
SOURCES: list[dict] = [
    {
        "name": "影刀初中高认证试题（Ttwumian）",
        "url": "https://blog.csdn.net/Ttwumian/article/details/161413518",
        "parser": "csdn_sections",
        "note": "初级 24 单选 + 25 多选 + 6 操作题（A~F），另含中高级",
    },
    {
        "name": "影刀工程师认证考试_初级_IT速成课程_1",
        "url": "https://blog.csdn.net/lingyaoJUN/article/details/141305839",
        "parser": "ops_page",
        "note": "正文以截图为主，文字提取不到题，会被记为 skipped",
    },
    {
        "name": "影刀工程师认证考试_初级_IT速成课程_2",
        "url": "https://blog.csdn.net/lingyaoJUN/article/details/141305884",
        "parser": "ops_page",
        "note": "同上",
    },
    {
        "name": "影刀工程师认证考试_初级_IT速成课程_3",
        "url": "https://blog.csdn.net/lingyaoJUN/article/details/141305932",
        "parser": "ops_page",
        "note": "同上",
    },
    {
        "name": "影刀工程师认证考试_初级_多选_4",
        "url": "https://blog.csdn.net/lingyaoJUN/article/details/141306301",
        "parser": "ops_page",
        "note": "同上",
    },
    {
        "name": "影刀初级考试操作题2（题型C）",
        "url": "https://blog.csdn.net/weisili2012/article/details/136614208",
        "parser": "ops_page",
        "note": "题型C 操作题，含步骤描述",
    },
    {
        "name": "影刀初级B级考试大题1",
        "url": "https://blog.csdn.net/qFAFAF/article/details/150466387",
        "parser": "ops_page",
        "note": "Excel 循环 + 批量抓取类操作题",
    },
    {
        "name": "影刀工程师认证考试_操作题_题型D",
        "url": "https://blog.csdn.net/lingyaoJUN/article/details/141306591",
        "parser": "ops_page",
        "note": "题型D 题干文本",
    },
    {
        "name": "影刀工程师认证考试_操作题_题型E",
        "url": "https://blog.csdn.net/lingyaoJUN/article/details/141306388",
        "parser": "ops_page",
        "note": "题型E 题干文本",
    },
]

# ---------------------------------------------------------------- 抓取
def fetch(url: str, refresh: bool = False) -> str | None:
    """带缓存的 GET。返回解码后的 HTML，失败返回 None。"""
    CACHE.mkdir(parents=True, exist_ok=True)
    key = hashlib.sha1(url.encode()).hexdigest()[:16]
    cached = CACHE / f"{key}.html"
    if cached.exists() and not refresh:
        return cached.read_text(encoding="utf-8", errors="ignore")

    req = urllib.request.Request(url, headers={
        "User-Agent": UA,
        "Accept-Language": "zh-CN,zh;q=0.9",
        "Referer": "https://www.google.com/",
    })
    for attempt in range(3):
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                raw = r.read()
            text = raw.decode("utf-8", errors="ignore")
            cached.write_text(text, encoding="utf-8")
            return text
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            print(f"    [retry {attempt + 1}/3] {e}", file=sys.stderr)
            time.sleep(1.5 * (attempt + 1))
    return None


# ---------------------------------------------------------------- 解析
def csdn_body(html: str) -> str | None:
    """取出 CSDN 文章正文容器。"""
    m = re.search(r'<div id="content_views".*?</div>\s*</article>', html, re.S)
    return m.group(0) if m else None


def html_to_lines(fragment: str) -> list[str]:
    """把 HTML 片段转成「一行一段」的纯文本，保留块级边界。"""
    s = re.sub(r"<(script|style)[^>]*>.*?</\1>", "", fragment, flags=re.S)
    s = re.sub(r"<br\s*/?>", "\n", s)
    s = re.sub(r"</(p|li|h[1-6]|div|tr|pre)>", "\n", s)
    s = re.sub(r"<[^>]+>", "", s)
    s = htmllib.unescape(s)
    s = s.replace("\u00a0", " ").replace("\u3000", " ")
    s = re.sub(r"[ \t]+", " ", s)
    return [ln.strip() for ln in s.split("\n") if ln.strip()]


LEVEL_KEYS = ("初级认证", "中级认证", "高级认证")
TYPE_KEYS = {"单选题": "单选", "多选题": "多选", "操作题": "操作题"}
Q_START = re.compile(r"^(\d{1,2})\s*[\.、．]\s*(.+)$")
OP_START = re.compile(r"^【?题型\s*([A-F])】?$")


def parse_sections(lines: list[str], level_want: str = "初级认证") -> list[dict]:
    """
    按「初级认证 / 一、单选题」这类标题切块，逐题抽取。
    只返回 level_want 这一级、且属于 单选/多选/操作题 的题目。
    """
    # 1) 定位正文起点。文章开头通常有一份「目录」把三个层级都列一遍，
    #    所以同一个层级标题会出现两次：第 1 次是目录，第 2 次才是正文。
    #    取第 2 次；只有 1 次时说明没有目录，直接用它。
    positions = [i for i, ln in enumerate(lines) if ln == level_want]
    if not positions:
        return []
    start = positions[1] if len(positions) >= 2 else positions[0]

    # 2) 截到下一个层级标题
    end = len(lines)
    for j in range(start + 1, len(lines)):
        if lines[j] in LEVEL_KEYS and lines[j] != level_want:
            end = j
            break

    body = lines[start + 1:end]

    # 3) 按题型切块
    blocks: list[tuple[str, list[str]]] = []
    cur_type: str | None = None
    cur: list[str] = []
    for ln in body:
        hit = None
        for k, v in TYPE_KEYS.items():
            if ln.replace(" ", "").startswith(("一、", "二、", "三、")) and k in ln:
                hit = v
                break
        if hit:
            if cur_type and cur:
                blocks.append((cur_type, cur))
            cur_type, cur = hit, []
        elif cur_type:
            cur.append(ln)
    if cur_type and cur:
        blocks.append((cur_type, cur))

    # 4) 块内切题
    out: list[dict] = []
    for qtype, blines in blocks:
        if qtype == "操作题":
            out.extend(_parse_ops(blines))
        else:
            out.extend(_parse_choices(blines, qtype))
    return out


def _parse_choices(lines: list[str], qtype: str) -> list[dict]:
    """选择题：`N.题干` 起一条，直到下一条编号。"""
    out: list[dict] = []
    cur: dict | None = None
    for ln in lines:
        m = Q_START.match(ln)
        # 编号要递增才算新题，避免把 "1.0" 这种选项当题号
        if m and (cur is None or int(m.group(1)) == cur["_no"] + 1):
            if cur:
                out.append(cur)
            cur = {"_no": int(m.group(1)), "stem": m.group(2), "options": []}
        elif cur is not None:
            cur["options"].append(ln)
        # cur 为 None 时是块首的杂项文字，丢弃

    if cur:
        out.append(cur)

    cleaned = []
    for q in out:
        stem = q["stem"].strip()
        if not stem or len(stem) < 4:
            continue

        opts = [o for o in q["options"] if o and len(o) < 200]

        # 题干被 <br> 折行时，后半句会混进选项里。
        # 判据：选项里排第一的那行以问号/冒号/逗号收尾 —— 选项极少这么写。
        while len(opts) > 2 and re.search(r"[？?：:，,、]$", opts[0]):
            stem += opts.pop(0)

        # 选项超过 4 个说明前面还有折行残留，一律并回题干
        while len(opts) > 4:
            stem += opts.pop(0)

        if not opts:
            # 原文这一题的选项就是截图，文字里没有——标出来，别让人以为漏抓了
            cleaned.append({
                "type": qtype,
                "stem": stem,
                "options": [],
                "note": "原题选项为截图，需回原文查看（抓取只拿到题干）",
            })
            continue

        cleaned.append({
            "type": qtype,
            "stem": stem,
            "options": opts,
        })
    return cleaned


def _parse_ops(lines: list[str]) -> list[dict]:
    """操作题：以【题型X】或「1. ...」分条，题干就是整段步骤。"""
    out: list[dict] = []
    cur_label = ""
    buf: list[str] = []

    def flush():
        if buf:
            text = "\n".join(buf).strip()
            if len(text) >= 20:
                out.append({"type": "操作题",
                            "stem": (f"【题型{cur_label}】\n" if cur_label else "") + text,
                            "options": []})

    for ln in lines:
        m = OP_START.match(ln)
        if m:
            flush()
            cur_label, buf = m.group(1), []
        elif ln.startswith(("搭建流程", "运行结果", "注意", "注：")):
            # 答案/截图说明，不属于题干
            flush()
            cur_label, buf = "", []
        else:
            buf.append(ln)
    flush()
    return out


def parse_ops_only(lines: list[str]) -> list[dict]:
    """
    整页就是一道操作题、没有「初级认证 / 一、单选题」这类章节标题时用。

    这类页面顶部常有一段「关联链接」导航（一串同系列文章标题），
    必须先剥掉，否则会被当成题干。
    """
    # 页面顶部的导航噪声：关联链接标题 + 一串「影刀工程师认证考试_考试试题_...」
    NAV_RE = re.compile(r"^(关联链接|相关链接|上一篇|下一篇|目录)$|^影刀(工程师认证考试|RPA)")

    buf: list[str] = []
    for ln in lines:
        if ln.startswith(("搭建流程", "运行结果", "关注博主", "题型C相对来说")):
            break
        if NAV_RE.match(ln):
            continue
        if ln in ("目录", "初级认证", "中级认证", "高级认证"):
            continue
        if ln.replace(" ", "").startswith(("一、", "二、", "三、")):
            continue
        buf.append(ln)

    # 「题型C：」这类行只保留题号信息
    label = ""
    if buf and re.match(r"^题型\s*[A-F]\s*[：:]?$", buf[0]):
        label = buf.pop(0).rstrip("：:").replace("题型", "").strip()

    text = "\n".join(buf).strip()
    if len(text) < 20:
        return []
    stem = f"【题型{label}】\n{text}" if label else text
    return [{"type": "操作题", "stem": stem, "options": []}]


# ---------------------------------------------------------------- 去重
def norm_key(stem: str) -> str:
    s = re.sub(r"[\s\u3000，。、；：？！“”‘’（）【】《》,.;:?!\"'()\[\]<>]", "", stem)
    s = re.sub(r"^【题型[A-F]】", "", s)
    return s[:30]


def sig(q: dict) -> str:
    """
    去重签名 = 归一化题干 + 排序后的选项集合。

    为什么要带选项：影刀初级大量题目的题干都是「流程如图，运行结果为？」，
    只有配图不同。只按题干去重会把 4 道不同的题压成 1 道。
    而「同题换选项顺序」的重复（同一知识点在两份卷子里），选项集合相同，
    正好能被合并掉。
    """
    opts = "|".join(sorted(norm_key(o) for o in q.get("options", []) if o))
    return norm_key(q["stem"]) + "#" + opts


def dedupe(items: list[dict]) -> list[dict]:
    seen: dict[str, dict] = {}
    out: list[dict] = []
    for q in items:
        k = sig(q)
        if not k.strip("#"):
            continue
        if k in seen:
            # 同一道题在不同来源出现：把来源记全，方便回溯
            kept = seen[k]
            src = q.get("source", "")
            if src and src not in kept.get("source", ""):
                kept["source"] = kept.get("source", "") + " ; " + src
                kept["note"] = (kept.get("note") or "") + "（该题在多个来源出现）"
            continue
        seen[k] = q
        out.append(q)
    return out


# ---------------------------------------------------------------- 主流程
def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--refresh", action="store_true", help="忽略缓存重新抓取")
    ap.add_argument("--list", action="store_true", help="只列出来源")
    args = ap.parse_args()

    if args.list:
        for s in SOURCES:
            print(f"- {s['name']}\n    {s['url']}\n    {s['note']}")
        return 0

    DATA.mkdir(parents=True, exist_ok=True)
    all_items: list[dict] = []
    skipped: list[dict] = []

    for s in SOURCES:
        print(f"[fetch] {s['name']}")
        html = fetch(s["url"], refresh=args.refresh)
        if not html:
            print("    ✗ 抓取失败，跳过")
            skipped.append({"name": s["name"], "url": s["url"], "reason": "抓取失败"})
            continue

        body = csdn_body(html)
        if not body:
            print("    ✗ 没找到正文容器，跳过")
            skipped.append({"name": s["name"], "url": s["url"], "reason": "无正文容器"})
            continue

        lines = html_to_lines(body)
        items = parse_sections(lines, "初级认证")

        if not items and s["parser"] == "ops_page":
            items = parse_ops_only(lines)

        if not items:
            print("    ⚠ 正文是截图，抽不出文字题（已记入 skipped）")
            skipped.append({"name": s["name"], "url": s["url"],
                            "reason": "正文为截图，需人工查看"})
            continue

        for it in items:
            it["id"] = ""
            it["topic"] = ""
            it["answer"] = ""
            it["confidence"] = "待核对"
            # 解析器已经写好的备注（比如「原题选项为截图」）要保留
            it["note"] = it.get("note") or "自动抓取，答案需人工核对"
            it["source"] = s["url"]
            it["sourceName"] = s["name"]
        print(f"    ✓ 抽取 {len(items)} 条")
        all_items.extend(items)

    all_items = dedupe(all_items)

    # 编号：按题型分组
    counters: dict[str, int] = {}
    prefix = {"单选": "X", "多选": "Y", "操作题": "Z"}
    for q in all_items:
        counters[q["type"]] = counters.get(q["type"], 0) + 1
        q["id"] = f"{prefix.get(q['type'], 'Q')}{counters[q['type']]:03d}"

    OUT_FILE.write_text(
        json.dumps({"meta": {"generatedAt": time.strftime("%Y-%m-%d %H:%M:%S"),
                             "sources": [s["url"] for s in SOURCES],
                             "skipped": skipped,
                             "warning": "自动抓取内容，答案一律待核对，请勿直接背"},
                    "questions": all_items},
                   ensure_ascii=False, indent=2),
        encoding="utf-8")

    by_type: dict[str, int] = {}
    for q in all_items:
        by_type[q["type"]] = by_type.get(q["type"], 0) + 1
    print("\n[ok] 写出", OUT_FILE)
    print("[ok] 统计：" + "，".join(f"{k} {v}" for k, v in by_type.items()))
    if skipped:
        print(f"[ok] 跳过 {len(skipped)} 个来源（详见 meta.skipped）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
