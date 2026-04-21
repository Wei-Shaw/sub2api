"""
严格按模型拆分 upstream 的 token 和 quota；并按【我们的定价表】重算每个模型的 token 应得金额。
对比 upstream provider-side (/group_ratio) 与我们 Anthropic 官方价的计算结果。
"""
import re
import json
from collections import defaultdict

# 按账号(token_name) + 模型拆
by_key = defaultdict(lambda: {
    'count': 0,
    'prompt': 0,
    'completion': 0,
    'cache_create': 0,
    'cache_read': 0,
    'quota_pre_group_sum': 0.0,
    'flat_price_reqs': 0,
    'flat_price_value': 0.0,
    'model_ratios': set(),
    'model_prices': set(),
})

with open(r"C:\Users\16790\xwechat_files\wxid_8tc8tfooo5rs22_fef8\msg\file\2026-04\asakifeng_consume.txt", 'r', encoding='utf-8') as f:
    for line in f:
        m = re.match(r'\[INFO\] (\d{4}/\d{2}/\d{2} - \d{2}:\d{2}:\d{2}) \|.*params=(\{.*\})\s*$', line.strip())
        if not m: continue
        try: p = json.loads(m.group(2))
        except Exception: continue
        tn = p.get('token_name', '')
        model = p.get('model_name', '')
        other = p.get('other') or {}
        gr = other.get('group_ratio', 1.0) or 1.0
        q = p.get('quota', 0) or 0
        k = (tn, model)
        d = by_key[k]
        d['count'] += 1
        d['prompt'] += p.get('prompt_tokens', 0) or 0
        d['completion'] += p.get('completion_tokens', 0) or 0
        d['cache_create'] += other.get('cache_creation_tokens', 0) or 0
        d['cache_read'] += other.get('cache_tokens', 0) or 0
        d['quota_pre_group_sum'] += q / gr if gr else q
        mp = other.get('model_price') or 0
        mr = other.get('model_ratio')
        if mr is not None: d['model_ratios'].add(mr)
        d['model_prices'].add(mp)
        if mp and mp > 0:
            d['flat_price_reqs'] += 1
            d['flat_price_value'] += mp  # flat $ per request

# 我们定价表（从 backend/resources/.../model_prices_and_context_window.json 读的真实值）
OUR_PRICE = {
    'claude-haiku-4-5-20251001':  {'input': 1e-6, 'output': 5e-6, 'cc5m': 1.25e-6, 'cr': 1e-7},
    'claude-sonnet-4-6':          {'input': 3e-6, 'output': 1.5e-5, 'cc5m': 3.75e-6, 'cr': 3e-7},
    'claude-sonnet-4-5-20250929': {'input': 3e-6, 'output': 1.5e-5, 'cc5m': 3.75e-6, 'cr': 3e-7},
    'claude-opus-4-6':            {'input': 5e-6, 'output': 2.5e-5, 'cc5m': 6.25e-6, 'cr': 5e-7},
    'claude-opus-4-5-20251101':   {'input': 5e-6, 'output': 2.5e-5, 'cc5m': 6.25e-6, 'cr': 5e-7},
    'claude-opus-4-7':            {'input': 5e-6, 'output': 2.5e-5, 'cc5m': 6.25e-6, 'cr': 5e-7},  # 我们回退到 opus-4-6 价
}

print("%-40s %-28s %5s %12s %12s %12s %12s" % ("TOKEN", "MODEL", "req", "upstream$", "our_calc$", "diff$", "note"))
print("-" * 150)
total_up = 0.0; total_ours = 0.0
for (tn, model), d in sorted(by_key.items()):
    up = d['quota_pre_group_sum'] / 500000
    p = OUR_PRICE.get(model)
    if p:
        ours = (d['prompt']*p['input'] + d['completion']*p['output']
              + d['cache_create']*p['cc5m'] + d['cache_read']*p['cr'])
    else:
        ours = 0.0
    diff = up - ours
    note = ""
    if d['flat_price_reqs']:
        note = f"flat_price {d['flat_price_reqs']}/{d['count']}"
    total_up += up; total_ours += ours
    print("%-40s %-28s %5d %12.4f %12.4f %+12.4f  %s" % (tn[:40], model, d['count'], up, ours, diff, note))
print("-" * 150)
print("%-40s %-28s %5s %12.4f %12.4f %+12.4f" % ("TOTAL", "", "", total_up, total_ours, total_up - total_ours))
