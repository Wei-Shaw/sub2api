"""Read-only replay. Never call PriceMonitor.run_once or any update API."""
import json
from pathlib import Path
import sys

sys.path.insert(0, '/app')
import monitor

state = json.loads(Path('/app/data/latest.json').read_text())
assert state['accounts_available']
accounts = state['accounts']
pricing = state['upstream_pricing']
candidates, stable = monitor.plan_account_renames(accounts, pricing, state.get('account_rename_candidates', {}))
for key, account in accounts.items():
    name = account.get('name', '')
    ratio = monitor.single_group_ratio(pricing.get(key, {}))
    desired = monitor.account_name_for_ratio(name, ratio)
    if desired:
        assert desired.rsplit('-', 1)[0] == name.strip().rsplit('-', 1)[0]
for name in ('seedance2.0', 'zhongysd2.5', 'gpt-5.6', 'bkclaudemax', 'ziyouxie-plus', 'hx0.04'):
    assert monitor.account_name_for_ratio(name, '0.08') is None, name
print(json.dumps({'checked_at': state['checked_at'], 'accounts': len(accounts),
                  'candidate_count': len(candidates), 'stable_count': len(stable),
                  'writes': 0, 'version_and_unlabelled_name_cases': 'passed'}, ensure_ascii=False))
