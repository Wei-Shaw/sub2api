import json
import tempfile
import unittest
from decimal import Decimal
from email.message import Message
from pathlib import Path
from urllib.error import URLError
from urllib.parse import parse_qs, urlsplit
from unittest.mock import patch

from monitor import (
    PriceMonitor,
    diff_accounts,
    diff_prices,
    diff_upstream_pricing,
    extract_access_token,
    can_refresh_upstream_access_token,
    access_token_state_needs_refresh,
    auth_headers,
    fetch_authorized_key_entries,
    login_upstream,
    format_upstream_change,
    format_message,
    load_browser_pricing_snapshot,
    match_upstream_key,
    normalize_prices,
    normalize_upstream_auth_config,
    account_name_for_ratio,
    account_priority_category,
    format_account_change,
    is_image2_account,
    is_image2_managed_group,
    is_guomo_collection_account,
    plan_account_renames,
    plan_account_priorities,
    plan_account_auto_groups,
    plan_account_safety_stops,
    probe_authorized_account_pricing,
    probe_browser_snapshot_pricing,
    probe_sub2api_billing_pricing,
    send_wecom,
    split_wecom_markdown,
    update_sub2api_account_name,
    update_sub2api_account_priority,
    update_sub2api_account_groups,
    update_sub2api_account_schedulable,
)


class MonitorTests(unittest.TestCase):
    def test_guomo_collection_accounts_are_excluded_from_automatic_renames(self):
        account = {
            "name": "llmroute-0.11",
            "managed_groups": [{"group_id": 65, "name": "国模合集分组"}],
        }
        pricing = {"1": {"scope": "account_authorized", "groups": {"国模合集": "0.2"}}}

        self.assertTrue(is_guomo_collection_account(account))
        candidates, stable = plan_account_renames({"1": account}, pricing, {})
        self.assertEqual(candidates, {})
        self.assertEqual(stable, [])

    def test_profitable_groups_only_match_gpt_and_plus_whitelists(self):
        groups = {
            5: {"group_id": 5, "name": "Gpt-plus", "rate_multiplier": "0.088", "status": "active"},
            10: {"group_id": 10, "name": "Gpt-福利", "rate_multiplier": "0.066", "status": "active"},
            46: {"group_id": 46, "name": "gpt-福利0.05", "rate_multiplier": "0.05", "status": "active"},
        }
        accounts = {
            "1": {"name": "qr-gpt-0.05", "schedulable": True, "managed_groups": []},
            "2": {"name": "hx-gptplus-0.07", "schedulable": True, "managed_groups": []},
            "3": {"name": "jis-gptpro-0.01", "schedulable": True, "managed_groups": []},
        }
        pricing = {
            "1": {"scope": "account_authorized", "groups": {"g": "0.05"}},
            "2": {"scope": "account_authorized", "groups": {"g": "0.07"}},
            "3": {"scope": "account_authorized", "groups": {"g": "0.01"}},
        }
        config = {"gpt": [10], "plus": [5]}

        candidates, stable, _ = plan_account_auto_groups(
            accounts, groups, pricing, {}, {}, config, Decimal("15")
        )

        self.assertEqual(candidates["1"]["add_group_ids"], [10])
        self.assertEqual(candidates["2"]["add_group_ids"], [5])
        self.assertNotIn("3", candidates)
        self.assertEqual(stable, [])
        _, stable, _ = plan_account_auto_groups(
            accounts, groups, pricing, candidates, {}, config, Decimal("15")
        )
        self.assertEqual(
            [(item["account_id"], item["add_group_ids"]) for item in stable],
            [("1", [10]), ("2", [5])],
        )

    def test_profitable_group_uses_fifteen_percent_minimum(self):
        groups = {
            10: {"group_id": 10, "name": "Gpt-福利", "rate_multiplier": "0.066", "status": "active"}
        }
        accounts = {"1": {"name": "qr-gpt-0.0561", "schedulable": True, "managed_groups": []}}
        config = {"gpt": [10], "plus": [5]}

        candidates, _, _ = plan_account_auto_groups(
            accounts,
            groups,
            {"1": {"scope": "account_authorized", "groups": {"g": "0.0561"}}},
            {},
            {},
            config,
            Decimal("15"),
        )
        self.assertEqual(candidates["1"]["add_group_ids"], [10])

        candidates, _, _ = plan_account_auto_groups(
            accounts,
            groups,
            {"1": {"scope": "account_authorized", "groups": {"g": "0.0562"}}},
            {},
            {},
            config,
            Decimal("15"),
        )
        self.assertEqual(candidates["1"]["add_group_ids"], [10])

    def test_profitable_group_removes_only_monitor_owned_binding(self):
        group = {"group_id": 10, "name": "Gpt-福利", "rate_multiplier": "0.066", "status": "active"}
        groups = {10: group}
        accounts = {
            "1": {
                "name": "qr-gpt-0.07",
                "schedulable": True,
                "managed_groups": [group],
            }
        }
        pricing = {"1": {"scope": "account_authorized", "groups": {"g": "0.07"}}}
        config = {"gpt": [10]}

        manual_candidates, _, _ = plan_account_auto_groups(
            accounts, groups, pricing, {}, {}, config, Decimal("15")
        )
        self.assertEqual(manual_candidates, {})

        owned = {"1": {"owned_group_ids": [10], "suppressed_group_ids": []}}
        candidates, _, _ = plan_account_auto_groups(
            accounts, groups, pricing, {}, owned, config, Decimal("15")
        )
        self.assertEqual(candidates["1"]["remove_group_ids"], [10])

    def test_profitable_group_respects_manual_removal_of_owned_binding(self):
        groups = {
            10: {"group_id": 10, "name": "Gpt-福利", "rate_multiplier": "0.066", "status": "active"}
        }
        accounts = {"1": {"name": "qr-gpt-0.05", "schedulable": True, "managed_groups": []}}
        pricing = {"1": {"scope": "account_authorized", "groups": {"g": "0.05"}}}
        owned = {"1": {"owned_group_ids": [10], "suppressed_group_ids": []}}

        candidates, _, ownership = plan_account_auto_groups(
            accounts, groups, pricing, {}, owned, {"gpt": [10]}, Decimal("15")
        )

        self.assertEqual(candidates, {})
        self.assertEqual(ownership["1"]["owned_group_ids"], [])
        self.assertEqual(ownership["1"]["suppressed_group_ids"], [10])

    def test_safety_stops_only_authorized_accounts_over_active_group_ratio(self):
        accounts = {
            "1": {
                "name": "vip-gpt-0.06",
                "schedulable": True,
                "managed_groups": [
                    {"group_id": 3, "name": "VIP", "rate_multiplier": "0.055", "status": "active"},
                    {"group_id": 4, "name": "old", "rate_multiplier": "0.01", "status": "disabled"},
                ],
            },
            "2": {
                "name": "vip-gpt-0.05",
                "schedulable": True,
                "managed_groups": [
                    {"group_id": 3, "name": "VIP", "rate_multiplier": "0.055", "status": "active"},
                ],
            },
            "3": {
                "name": "public-gpt-0.06",
                "schedulable": True,
                "managed_groups": [
                    {"group_id": 3, "name": "VIP", "rate_multiplier": "0.055", "status": "active"},
                ],
            },
        }
        pricing = {
            "1": {"scope": "account_authorized", "groups": {"upstream": "0.06x"}},
            "2": {"scope": "account_authorized", "groups": {"upstream": "0.05x"}},
            "3": {"scope": "public_default", "groups": {"upstream": "0.06x"}},
        }

        candidates, stable = plan_account_safety_stops(accounts, pricing, {})
        self.assertEqual(set(candidates), {"1"})
        self.assertEqual(stable, [])
        self.assertEqual(candidates["1"]["violations"][0]["managed_ratio"], "0.055")

        _, stable = plan_account_safety_stops(accounts, pricing, candidates)
        self.assertEqual([item["account_id"] for item in stable], ["1"])

    def test_safety_stop_update_sends_only_schedulable_and_verifies(self):
        with patch(
            "monitor.post_json", return_value={"code": 0, "data": {"schedulable": False}}
        ) as update:
            update_sub2api_account_schedulable("31", False, "http://sub2api:8080", "admin-test")
        update.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31/schedulable",
            {"schedulable": False},
            {"x-api-key": "admin-test"},
            15,
        )

    def test_safety_stops_ignore_only_gpt_image_2_binding(self):
        accounts = {
            "29": {
                "name": "senyu-Pro-0.25",
                "schedulable": True,
                "managed_groups": [
                    {"group_id": 10, "name": "gpt-image-2", "rate_multiplier": "0.055", "status": "active"},
                    {"group_id": 11, "name": "GPT官方-PRO", "rate_multiplier": "0.055", "status": "active"},
                ],
            }
        }
        pricing = {"29": {"scope": "account_authorized", "groups": {"upstream": "0.06x"}}}

        candidates, _ = plan_account_safety_stops(accounts, pricing, {})

        self.assertTrue(is_image2_managed_group(accounts["29"]["managed_groups"][0]))
        self.assertEqual(candidates["29"]["violations"][0]["group_name"], "GPT官方-PRO")

        accounts["29"]["managed_groups"][1]["rate_multiplier"] = "0.07"
        accounts["29"]["managed_groups"] = [accounts["29"]["managed_groups"][0]]
        candidates, _ = plan_account_safety_stops(accounts, pricing, {})
        self.assertEqual(candidates, {})

    def test_safety_preserves_domestic_binding_without_exempting_other_groups(self):
        domestic = {"group_id": 91, "name": "\u56fd\u6a21\u5206\u7ec4", "rate_multiplier": "0.05", "status": "active"}
        other = {"group_id": 92, "name": "GPT", "rate_multiplier": "0.05", "status": "active"}
        accounts = {"1": {"name": "upstream", "managed_groups": [domestic, other]}}
        pricing = {"1": {"scope": "account_authorized", "groups": {"upstream": "0.1x"}}}
        candidates, _ = plan_account_safety_stops(accounts, pricing, {})
        self.assertEqual(candidates["1"]["detach_group_ids"], [92])
        accounts["1"]["managed_groups"] = [domestic]
        self.assertEqual(plan_account_safety_stops(accounts, pricing, {}), ({}, []))

    def test_safety_restores_only_monitor_owned_domestic_binding(self):
        accounts = {"1": {"name": "upstream", "managed_groups": []}}
        pricing = {"1": {"scope": "account_authorized", "groups": {"upstream": "0.1x"}}}
        detached = {"1": {"group_order": [91], "groups": [{"group_id": 91, "name": "\u56fd\u6a21", "managed_ratio": "0.05", "status": "active"}]}}
        candidates, _ = plan_account_safety_stops(accounts, pricing, {}, detached)
        self.assertEqual(candidates["1"]["restore_group_ids"], [91])
        self.assertEqual(plan_account_safety_stops(accounts, pricing, {}), ({}, []))

    def test_safety_detaches_only_overpriced_groups_and_restores_owned_groups(self):
        account = {
            "name": "vip-gpt-0.08",
            "schedulable": True,
            "managed_groups": [
                {"group_id": 3, "name": "VIP", "rate_multiplier": "0.055", "status": "active", "priority": 1},
                {"group_id": 4, "name": "GPT Pro", "rate_multiplier": "0.09", "status": "active", "priority": 2},
                {"group_id": 10, "name": "gpt-image-2", "rate_multiplier": "0.055", "status": "active", "priority": 3},
            ],
        }
        accounts = {"1": account}
        pricing = {"1": {"scope": "account_authorized", "groups": {"upstream": "0.08x"}}}

        candidates, stable = plan_account_safety_stops(accounts, pricing, {})
        self.assertEqual(stable, [])
        self.assertEqual(candidates["1"]["detach_group_ids"], [3])
        self.assertEqual(candidates["1"]["restore_group_ids"], [])
        self.assertNotIn("schedulable", candidates["1"])

        detached = {
            "1": {
                "group_order": [3, 4, 10],
                "groups": [
                    {"group_id": 3, "name": "VIP", "managed_ratio": "0.055", "status": "active", "priority": 1}
                ],
            }
        }
        account["managed_groups"] = [account["managed_groups"][1], account["managed_groups"][2]]
        pricing["1"]["groups"] = {"upstream": "0.04x"}
        candidates, stable = plan_account_safety_stops(accounts, pricing, {}, detached)
        self.assertEqual(stable, [])
        self.assertEqual(candidates["1"]["detach_group_ids"], [])
        self.assertEqual(candidates["1"]["restore_group_ids"], [3])

        _, stable = plan_account_safety_stops(accounts, pricing, candidates, detached)
        self.assertEqual([item["restore_group_ids"] for item in stable], [[3]])

    def test_update_account_groups_sends_complete_list_and_verifies(self):
        with patch(
            "monitor.put_json", return_value={"code": 0, "data": {"group_ids": [4, 10]}}
        ) as update, patch(
            "monitor.fetch_json", return_value={"code": 0, "data": {"group_ids": [4, 10]}}
        ) as fetch:
            update_sub2api_account_groups("31", [4, 10], "http://sub2api:8080", "admin-test")
        update.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31",
            {"group_ids": [4, 10]},
            {"x-api-key": "admin-test"},
            15,
        )
        fetch.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31",
            {"x-api-key": "admin-test"},
            15,
        )

    def test_wecom_markdown_split_respects_utf8_byte_limit(self):
        message = "标题\n" + ("价格变化：gpt-image-2 -> 0.06x\n" * 300)
        chunks = split_wecom_markdown(message)
        self.assertGreater(len(chunks), 1)
        self.assertTrue(all(len(chunk.encode("utf-8")) <= 3000 for chunk in chunks))
        self.assertEqual("".join(chunks), message)

    def test_format_message_keeps_all_changes_instead_of_omitting_tail(self):
        upstream_changes = [
            {
                "kind": "group_ratio",
                "account_name": f"account-{index}-0.05",
                "host": "upstream.example",
                "group": "special",
                "before": "0.05",
                "after": "0.06",
            }
            for index in range(80)
        ]
        message = format_message(
            "test",
            {"changed": [], "added": [], "removed": []},
            "2026-08-23T00:00:00+00:00",
            upstream_changes=upstream_changes,
        )
        self.assertNotIn("其余", message)
        self.assertIn("account-0-0.05", message)
        self.assertIn("account-79-0.05", message)

    def test_send_wecom_splits_oversized_markdown(self):
        message = "价格变化\n" + ("上游分组倍率发生变化\n" * 300)
        response = type(
            "Response",
            (),
            {
                "read": lambda self: b'{"errcode":0}',
                "__enter__": lambda self: self,
                "__exit__": lambda self, *args: None,
            },
        )
        with patch("monitor.urlopen", side_effect=lambda *args, **kwargs: response()) as request:
            send_wecom("https://wecom.example/webhook", message)
        self.assertGreater(request.call_count, 1)
        for index, call in enumerate(request.call_args_list, start=1):
            payload = json.loads(call.args[0].data.decode("utf-8"))
            self.assertLessEqual(
                len(payload["markdown"]["content"].encode("utf-8")), 4096
            )
            self.assertIn(f"第 {index}/{request.call_count} 条", payload["markdown"]["content"])

    def test_send_wecom_retries_only_the_failed_segment(self):
        message = "价格变化\n" + ("上游分组倍率发生变化\n" * 300)
        response = type(
            "Response",
            (),
            {
                "read": lambda self: b'{"errcode":0}',
                "__enter__": lambda self: self,
                "__exit__": lambda self, *args: None,
            },
        )
        calls = []

        def request_side_effect(request, **kwargs):
            content = json.loads(request.data.decode("utf-8"))["markdown"]["content"]
            calls.append(content)
            if "第 2/" in content and sum("第 2/" in item for item in calls) == 1:
                raise URLError("temporary failure")
            return response()

        with patch("monitor.urlopen", side_effect=request_side_effect), patch("monitor.time.sleep"):
            send_wecom("https://wecom.example/webhook", message)

        self.assertEqual(sum("第 1/" in item for item in calls), 1)
        self.assertEqual(sum("第 2/" in item for item in calls), 2)

    def test_account_priority_category_handles_product_names_and_legacy_spellings(self):
        self.assertEqual(account_priority_category("jis-gptpro-0.15"), "pro")
        self.assertEqual(account_priority_category("qr-gpt-pro-0.15"), "pro")
        self.assertEqual(account_priority_category("Hx-gptplus-0.04"), "plus")
        self.assertEqual(account_priority_category("VBL-image2-0.045"), "image")
        self.assertEqual(account_priority_category("5y-iamge-0.03"), "image")
        self.assertEqual(account_priority_category("jis-mage2-0.015"), "image")
        self.assertEqual(account_priority_category("ys-生图-0.08"), "image")
        self.assertEqual(account_priority_category("qr-claudemax-0.8"), "claude")
        self.assertEqual(account_priority_category("ys-视频"), "video")

    def test_image2_accounts_are_excluded_from_account_price_automation(self):
        self.assertTrue(is_image2_account("VBL-image2-0.045"))
        self.assertTrue(is_image2_account("senyu-iamge2-0.3"))
        self.assertTrue(is_image2_account("jis-mage2-1"))
        self.assertTrue(is_image2_account("5y-iamge-1"))
        self.assertFalse(is_image2_account("jis-gpt-0.04"))

        accounts = {
            "1": {
                "name": "senyu-iamge2-0.3",
                "priority": 50,
                "schedulable": True,
                "managed_groups": [
                    {"group_id": 1, "name": "VIP", "rate_multiplier": "0.05", "status": "active"}
                ],
            }
        }
        pricing = {"1": {"scope": "account_authorized", "groups": {"g": "0.3x"}}}

        self.assertEqual(plan_account_renames(accounts, pricing, {}), ({}, []))
        self.assertEqual(plan_account_priorities(accounts, pricing, {}), ({}, []))
        self.assertEqual(plan_account_safety_stops(accounts, pricing, {}), ({}, []))

    def test_account_priorities_are_ranked_independently_by_category(self):
        accounts = {
            "1": {"name": "gpt-a-0.05", "priority": 50, "schedulable": True},
            "2": {"name": "gpt-b-0.01", "priority": 50, "schedulable": True},
            "3": {"name": "plus-a-0.01", "priority": 50, "schedulable": True},
            "4": {"name": "plus-b-0.05", "priority": 50, "schedulable": True},
            "5": {"name": "image2-a-0.03", "priority": 50, "schedulable": True},
            "6": {"name": "claude-a-0.01", "priority": 50, "schedulable": True},
        }
        pricing = {
            account_id: {"scope": "account_authorized", "groups": {"g": ratio}}
            for account_id, ratio in {
                "1": 0.05,
                "2": 0.01,
                "3": 0.01,
                "4": 0.05,
                "5": 0.03,
                "6": 0.01,
            }.items()
        }

        candidates, stable = plan_account_priorities(accounts, pricing, {})

        self.assertEqual(candidates["1"]["desired_priority"], 2)
        self.assertEqual(candidates["2"]["desired_priority"], 1)
        self.assertEqual(candidates["3"]["desired_priority"], 1)
        self.assertEqual(candidates["4"]["desired_priority"], 2)
        self.assertEqual(candidates["6"]["desired_priority"], 1)
        self.assertNotIn("5", candidates)
        self.assertEqual({item["category"] for item in candidates.values()}, {"gpt", "plus", "claude"})
        self.assertEqual(stable, [])

    def test_update_account_priority_sends_only_priority_and_verifies(self):
        with patch(
            "monitor.put_json", return_value={"code": 0, "data": {"priority": 1}}
        ) as update, patch(
            "monitor.fetch_json", return_value={"code": 0, "data": {"priority": 1}}
        ) as fetch:
            update_sub2api_account_priority("31", 1, "http://sub2api:8080", "admin-test")
        update.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31",
            {"priority": 1},
            {"x-api-key": "admin-test"},
            15,
        )
        fetch.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31",
            {"x-api-key": "admin-test"},
            15,
        )

    def test_account_priorities_rank_distinct_ratios_and_share_ties(self):
        accounts = {
            "1": {"name": "cheap", "priority": 50, "schedulable": True},
            "2": {"name": "same", "priority": 50, "schedulable": True},
            "3": {"name": "expensive", "priority": 1, "schedulable": True},
        }
        pricing = {
            "1": {"scope": "account_authorized", "groups": {"g": "0.01x"}},
            "2": {"scope": "account_authorized", "groups": {"g": 0.01}},
            "3": {"scope": "account_authorized", "groups": {"g": 0.05}},
        }
        candidates, stable = plan_account_priorities(accounts, pricing, {})
        self.assertEqual(candidates["1"]["desired_priority"], 1)
        self.assertEqual(candidates["2"]["desired_priority"], 1)
        self.assertEqual(candidates["3"]["desired_priority"], 2)
        self.assertEqual(stable, [])

        _, stable = plan_account_priorities(accounts, pricing, candidates)
        self.assertEqual(
            [(item["account_id"], item["desired_priority"]) for item in stable],
            [("1", 1), ("2", 1), ("3", 2)],
        )

    def test_account_priorities_ignore_public_compound_and_unverified_ratios(self):
        accounts = {
            "1": {"name": "public", "priority": 50},
            "2": {"name": "compound", "priority": 50},
            "3": {"name": "failed", "priority": 50},
        }
        pricing = {
            "1": {"scope": "public_default", "groups": {"g": 0.01}},
            "2": {"scope": "account_authorized", "groups": {"g": "0.85x0.8x"}},
            "3": {"scope": "unverified", "groups": {"g": 0.02}},
        }
        candidates, stable = plan_account_priorities(accounts, pricing, {})
        self.assertEqual(candidates, {})
        self.assertEqual(stable, [])
    def test_update_account_name_sends_only_name_and_verifies(self):
        with patch(
            "monitor.put_json", return_value={"code": 0, "data": {"name": "jis-gpt-0.04"}}
        ) as update, patch(
            "monitor.fetch_json", return_value={"code": 0, "data": {"name": "jis-gpt-0.04"}}
        ) as fetch:
            update_sub2api_account_name("31", "jis-gpt-0.04", "http://sub2api:8080", "admin-test")
        update.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31",
            {"name": "jis-gpt-0.04"},
            {"x-api-key": "admin-test"},
            15,
        )
        fetch.assert_called_once_with(
            "http://sub2api:8080/api/v1/admin/accounts/31",
            {"x-api-key": "admin-test"},
            15,
        )

    def test_account_name_for_ratio_only_rewrites_simple_decimal_suffix(self):
        self.assertEqual(account_name_for_ratio("jis-gpt-0.05", "0.04x"), "jis-gpt-0.04")
        self.assertEqual(account_name_for_ratio("qr-gpt-0.015", 0.02), "qr-gpt-0.02")
        self.assertIsNone(account_name_for_ratio("hx0.04", "0.05x"))
        self.assertEqual(account_name_for_ratio("hx-0.04", "0.05x"), "hx-0.05")
        self.assertIsNone(account_name_for_ratio("claude-0.85x0.8x", "0.8x"))
        self.assertIsNone(account_name_for_ratio("gpt-account", "0.04x"))

    def test_rename_preserves_unlabelled_names_and_model_versions(self):
        for name in (
            "bkclaudemax", "ziyouxie-plus", "hx", "PRO012", "qr-特惠0.07",
            "seedance2.0", "zhongysd2.5", "seedance-2.0", "wan-3", "sd-2.5",
            "gpt-5.6", "vendor-gpt-5.6", "claude-sonnet-4.5", "gemini-3.1",
        ):
            with self.subTest(name=name):
                self.assertIsNone(account_name_for_ratio(name, "0.08"))

    def test_rename_updates_explicit_gpt_and_claude_price_labels(self):
        for name, ratio, expected in (
            ("beikun-0.04", "0.042", "beikun-0.042"),
            ("qr-gpt-0.1", "0.07", "qr-gpt-0.07"),
            ("molifang-max-1", "0.9", "molifang-max-0.9"),
            ("zhongy-ccmax-0.45", "0.28", "zhongy-ccmax-0.28"),
            ("vendor-gpt-5.6-0.05", "0.06", "vendor-gpt-5.6-0.06"),
        ):
            with self.subTest(name=name):
                self.assertEqual(account_name_for_ratio(name, ratio), expected)

    def test_obsolete_candidates_cannot_bypass_explicit_suffix_rule(self):
        accounts = {"1": {"name": "seedance2.0"}, "2": {"name": "bkclaudemax"}}
        pricing = {key: {"scope": "account_authorized", "groups": {"g": "0.1"}} for key in accounts}
        previous = {key: {"current_name": account["name"], "desired_name": "bad-0.1"} for key, account in accounts.items()}
        self.assertEqual(plan_account_renames(accounts, pricing, previous, set(accounts)), ({}, []))

    def test_account_rename_requires_two_identical_observations(self):
        accounts = {"31": {"name": "jis-gpt-0.05"}}
        pricing = {"31": {"scope": "account_authorized", "groups": {"special": 0.04}}}
        candidates, stable = plan_account_renames(accounts, pricing, {})
        self.assertIn("31", candidates)
        self.assertEqual(stable, [])

        _, stable = plan_account_renames(accounts, pricing, candidates)
        self.assertEqual(stable[0]["desired_name"], "jis-gpt-0.04")

    def test_account_rename_ignores_public_or_compound_ratios(self):
        accounts = {"1": {"name": "a-0.05"}, "2": {"name": "b-0.85"}}
        pricing = {
            "1": {"scope": "public_default", "groups": {"g": 0.04}},
            "2": {"scope": "account_authorized", "groups": {"g": "0.85x0.8x"}},
        }
        candidates, stable = plan_account_renames(accounts, pricing, {})
        self.assertEqual(candidates, {})
        self.assertEqual(stable, [])

    def test_normalize_upstream_auth_config_is_keyed_by_host(self):
        result = normalize_upstream_auth_config(
            {
                "upstreams": [
                    {"host": "https://mdkj.lol", "email": "monitor@example.com", "password": "secret"}
                ]
            }
        )
        self.assertEqual(result["mdkj.lol"]["email"], "monitor@example.com")

    def test_extract_access_token_supports_nested_data_response(self):
        self.assertEqual(extract_access_token({"data": {"access_token": "jwt-token"}}), "jwt-token")

    def test_niko_login_persists_access_token_and_refresh_cookie(self):
        config = {
            "login_url": "https://nikoapi.xyz/api/user/login?turnstile=",
            "login_field": "username",
            "session_cookie_name": "new_api_refresh",
            "token_path": "data.access_token",
        }
        token_state = {"version": 1, "upstreams": {}}
        headers = Message()
        headers.add_header(
            "Set-Cookie", "new_api_refresh=refresh-cookie; Path=/; HttpOnly"
        )
        with tempfile.TemporaryDirectory() as directory, patch(
            "monitor.post_json",
            return_value=(
                {
                    "data": {
                        "access_token": "access-token",
                        "access_expires_at": "2099-01-01T00:00:00Z",
                    }
                },
                headers,
            ),
        ):
            token = login_upstream(
                {**config, "username": "monitor@example.com", "password": "secret"},
                "https://nikoapi.xyz/v1",
                token_state=token_state,
                token_state_path=Path(directory) / "token-state.json",
                token_state_key="nikoapi.xyz",
            )

        self.assertEqual(token, "access-token")
        self.assertEqual(token_state["upstreams"]["nikoapi.xyz"]["access_token"], "access-token")
        self.assertEqual(
            token_state["upstreams"]["nikoapi.xyz"]["access_expires_at"],
            "2099-01-01T00:00:00Z",
        )
        self.assertEqual(token_state["upstreams"]["nikoapi.xyz"]["session_cookie"], "new_api_refresh=refresh-cookie")

    def test_niko_cookie_refresh_reuses_persisted_session_without_login(self):
        config = {
            "cookie_refresh": "true",
            "cookie_refresh_url": "https://nikoapi.xyz/api/user/auth/refresh",
            "session_cookie_name": "new_api_refresh",
            "token_path": "data.access_token",
        }
        token_state = {
            "version": 1,
            "upstreams": {
                "nikoapi.xyz": {
                    "access_token": "not-a-jwt",
                    "session_cookie": "new_api_refresh=old-cookie",
                }
            },
        }
        headers = Message()
        headers.add_header(
            "Set-Cookie", "new_api_refresh=new-cookie; Path=/; HttpOnly"
        )
        with tempfile.TemporaryDirectory() as directory, patch(
            "monitor.post_json",
            return_value=(
                {
                    "data": {
                        "access_token": "fresh-token",
                        "access_expires_at": "2099-01-02T00:00:00Z",
                    }
                },
                headers,
            ),
        ) as post:
            token = login_upstream(
                config,
                "https://nikoapi.xyz/v1",
                token_state=token_state,
                token_state_path=Path(directory) / "token-state.json",
                token_state_key="nikoapi.xyz",
                force_refresh=True,
            )

        self.assertEqual(token, "fresh-token")
        self.assertEqual(token_state["upstreams"]["nikoapi.xyz"]["session_cookie"], "new_api_refresh=new-cookie")
        self.assertEqual(
            token_state["upstreams"]["nikoapi.xyz"]["access_expires_at"],
            "2099-01-02T00:00:00Z",
        )
        self.assertEqual(post.call_args.args[0], "https://nikoapi.xyz/api/user/auth/refresh")
        self.assertEqual(post.call_args.args[2]["Cookie"], "new_api_refresh=old-cookie")

    def test_niko_cookie_refresh_is_available_from_persisted_state(self):
        self.assertTrue(
            can_refresh_upstream_access_token(
                {"cookie_refresh": "true"},
                {
                    "upstreams": {
                        "nikoapi.xyz": {
                            "session_cookie": "new_api_refresh=refresh-cookie"
                        }
                    }
                },
                "nikoapi.xyz",
            )
        )

    def test_niko_access_expiration_reuses_valid_token(self):
        self.assertFalse(
            access_token_state_needs_refresh(
                "not-a-jwt", {"access_expires_at": "2099-01-01T00:00:00Z"}
            )
        )

    def test_niko_reuses_persisted_cookie_with_valid_access_token(self):
        config = {
            "cookie_refresh": "true",
            "session_cookie_name": "new_api_refresh",
            "token_header": "Authorization",
            "token_prefix": "Bearer",
        }
        state = {
            "version": 1,
            "upstreams": {
                "nikoapi.xyz": {
                    "access_token": "not-a-jwt",
                    "access_expires_at": "2099-01-01T00:00:00Z",
                    "session_cookie": "new_api_refresh=stored-cookie",
                }
            },
        }

        token = login_upstream(
            config,
            "https://nikoapi.xyz/v1",
            token_state=state,
            token_state_key="nikoapi.xyz",
        )

        self.assertEqual(token, "not-a-jwt")
        self.assertEqual(auth_headers(token, config)["Cookie"], "new_api_refresh=stored-cookie")

    def test_niko_key_fetch_uses_niko_pagination_parameters(self):
        config = {
            "keys_url": "https://nikoapi.xyz/api/token/?p=1&size=1000",
            "keys_page_param": "p",
            "keys_page_size_param": "size",
            "page_size": "1000",
        }
        with patch("monitor.fetch_json", return_value={"data": {"items": []}}) as fetch:
            self.assertEqual(
                fetch_authorized_key_entries(config, "https://nikoapi.xyz/v1", "token"),
                [],
            )

        query = parse_qs(urlsplit(fetch.call_args.args[0]).query)
        self.assertEqual(query, {"p": ["1"], "size": ["1000"]})

    def test_match_upstream_key_reads_group_rate_multiplier(self):
        result = match_upstream_key(
            {
                "items": [
                    {
                        "key": "sk-monitor",
                        "group": {"name": "高特惠", "rate_multiplier": 0.015},
                    }
                ]
            },
            "sk-monitor",
        )
        self.assertEqual(result, {"group": "高特惠", "ratio": 0.015})

    def test_authorized_probe_matches_key_without_persisting_it(self):
        account = {
            "name": "qr-gpt-0.015",
            "base_url": "https://mdkj.lol",
            "api_key": "sk-monitor",
            "model_mapping": {"gpt-5.4": "gpt-5.4"},
        }
        with patch(
            "monitor.fetch_json",
            return_value={
                "items": [
                    {
                        "key": "sk-monitor",
                        "group": {"name": "高特惠", "rate_multiplier": 0.015},
                    }
                ]
            },
        ):
            result = probe_authorized_account_pricing(account, {}, "jwt-token")
        self.assertEqual(result["scope"], "account_authorized")
        self.assertEqual(result["groups"], {"高特惠": 0.015})
        self.assertNotIn("api_key", json.dumps(result))

    def test_browser_snapshot_matches_account_by_key_suffix_only(self):
        account = {
            "name": "5y-gpt-0.03",
            "base_url": "https://5yuantoken.org/v1",
            "api_key": "sk-not-stored-3dbe",
            "model_mapping": {"gpt-5.4": "gpt-5.4"},
        }
        snapshot = {
            "5yuantoken.org": [
                {
                    "key_suffix": "3dbe",
                    "group": "【ChatGPT Plus】特惠 0.06",
                    "ratio": "0.06x",
                }
            ]
        }
        result = probe_browser_snapshot_pricing(account, snapshot)
        self.assertEqual(result["scope"], "account_authorized")
        self.assertEqual(result["groups"], {"【ChatGPT Plus】特惠 0.06": "0.06x"})
        self.assertNotIn("sk-not-stored", json.dumps(result))

    def test_browser_snapshot_loader_normalizes_host_and_suffix(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "snapshot.json"
            path.write_text(
                json.dumps(
                    {
                        "version": 1,
                        "entries": [
                            {
                                "host": "https://5yuantoken.org/v1",
                                "key_suffix": "...3DBE",
                                "group": "特惠",
                                "ratio": "0.06x",
                            }
                        ],
                    }
                ),
                encoding="utf-8",
            )
            result = load_browser_pricing_snapshot(path)
        self.assertEqual(result["5yuantoken.org"][0]["key_suffix"], "3dbe")

    def test_fresh_sub2api_billing_probe_precedes_browser_snapshot(self):
        account = {
            "name": "5y-gpt-0.06",
            "base_url": "https://5yuantoken.org/v1",
            "model_mapping": {"gpt-5.5": "gpt-5.5"},
        }
        extra = {
            "upstream_billing_probe": {
                "status": "ok",
                "fresh_until": "2099-01-01T00:00:00Z",
                "data": {"group_rate_multiplier": 0.08},
            }
        }
        result = probe_sub2api_billing_pricing(account, extra)
        self.assertIsNotNone(result)
        self.assertEqual(result["groups"], {"Sub2API upstream billing probe": "0.08"})
        self.assertEqual(result["source_url"], "sub2api_billing_probe")

    def test_expired_sub2api_billing_probe_is_ignored(self):
        result = probe_sub2api_billing_pricing(
            {"name": "5y-gpt-0.06", "base_url": "https://5yuantoken.org/v1"},
            {
                "upstream_billing_probe": {
                    "status": "ok",
                    "fresh_until": "2020-01-01T00:00:00Z",
                    "data": {"group_rate_multiplier": 0.08},
                }
            },
        )
        self.assertIsNone(result)

    def test_browser_group_ratio_message_does_not_duplicate_x(self):
        message = format_upstream_change(
            {
                "kind": "group_ratio",
                "account_name": "qr-gpt-0.015",
                "host": "mdkj.lol",
                "group": "高特惠",
                "models": ["gpt-5.4"],
                "before": "0.015x",
                "after": "0.016x",
            }
        )
        self.assertIn("0.015x -> 0.016x", message)
        self.assertNotIn("0.015xx", message)
        self.assertNotIn("gpt-5.4", message)

    def test_account_change_message_only_includes_models_when_mapping_changes(self):
        added = format_account_change(
            {
                "account_id": "1",
                "after": {"name": "qr-gpt-0.04", "models": ["gpt-5.4"]},
            }
        )
        renamed = format_account_change(
            {
                "account_id": "1",
                "before": {"name": "qr-gpt-0.035", "models": ["gpt-5.4"]},
                "after": {"name": "qr-gpt-0.04", "models": ["gpt-5.4"]},
            }
        )
        mapping_changed = format_account_change(
            {
                "account_id": "1",
                "before": {"name": "qr-gpt-0.035", "models": ["gpt-5.4"]},
                "after": {"name": "qr-gpt-0.04", "models": ["gpt-5.5"]},
            }
        )
        self.assertNotIn("gpt-5.4", added)
        self.assertNotIn("模型", renamed)
        self.assertIn("模型", mapping_changed)
        self.assertIn("gpt-5.4", mapping_changed)
        self.assertIn("gpt-5.5", mapping_changed)

    def test_upstream_diff_ignores_public_prices_and_local_account_renames(self):
        previous = {
            "1": {
                "name": "qr-gpt-0.015",
                "scope": "public_default",
                "groups": {"高特惠": 0.015},
                "models": ["gpt-5.4"],
            },
            "2": {
                "name": "jis-gpt-0.03",
                "scope": "account_authorized",
                "groups": {"高特惠": 0.015},
                "models": ["gpt-5.4"],
            },
        }
        current = {
            "1": {
                "name": "qr-gpt-0.016",
                "scope": "public_default",
                "groups": {"高特惠": 0.016},
                "models": ["gpt-5.4"],
            },
            "2": {
                "name": "jis-gpt-0.03",
                "scope": "account_authorized",
                "groups": {"高特惠": 0.016},
                "models": ["gpt-5.4"],
            },
        }
        changes = diff_upstream_pricing(previous, current)
        self.assertEqual(len(changes), 1)
        self.assertEqual(changes[0]["account_name"], "jis-gpt-0.03")

    def test_normalize_prices_keeps_only_price_fields(self):
        result = normalize_prices(
            {
                "gpt-test": {
                    "input_cost_per_token": 1e-6,
                    "output_cost_per_token": 2e-6,
                    "context_window": 123,
                },
                "metadata-only": {"context_window": 123},
            }
        )
        self.assertEqual(result["gpt-test"]["input_cost_per_token"], 1e-6)
        self.assertNotIn("context_window", result["gpt-test"])
        self.assertNotIn("metadata-only", result)

    def test_diff_prices_detects_added_removed_and_changed(self):
        previous = {"old": {"input_cost_per_token": 1}, "same": {"output_cost_per_token": 2}}
        current = {"new": {"input_cost_per_token": 3}, "same": {"output_cost_per_token": 4}}
        changes = diff_prices(previous, current)
        self.assertEqual([item["model"] for item in changes["added"]], ["new"])
        self.assertEqual([item["model"] for item in changes["removed"]], ["old"])
        self.assertEqual([item["model"] for item in changes["changed"]], ["same"])

    def test_diff_accounts_detects_name_and_model_mapping_changes(self):
        previous = {
            "1": {"name": "qr-gpt-0.035", "models": ["gpt-5.4"], "status": "active"}
        }
        current = {
            "1": {"name": "qr-gpt-0.04", "models": ["gpt-5.5"], "status": "active"},
            "2": {"name": "jis-gpt-0.03", "models": ["gpt-5.4"], "status": "active"},
        }
        changes = diff_accounts(previous, current)
        self.assertEqual([item["account_id"] for item in changes["added"]], ["2"])
        self.assertEqual([item["account_id"] for item in changes["changed"]], ["1"])
        self.assertEqual(changes["changed"][0]["after"]["name"], "qr-gpt-0.04")

    def test_first_check_creates_baseline_without_notification(self):
        with tempfile.TemporaryDirectory() as directory:
            monitor = PriceMonitor(Path(directory), "test", "https://prices", "", "https://wecom")
            body = json.dumps({"gpt-test": {"input_cost_per_token": 1e-6}}).encode()
            with patch("monitor.fetch_bytes", return_value=body), patch("monitor.send_wecom") as notify:
                monitor.check_once()
                notify.assert_not_called()
            self.assertTrue((Path(directory) / "latest.json").exists())
            self.assertFalse((Path(directory) / "history.jsonl").exists())

    def test_repeated_unchanged_checks_do_not_notify(self):
        with tempfile.TemporaryDirectory() as directory:
            monitor = PriceMonitor(Path(directory), "test", "https://prices", "", "https://wecom")
            body = json.dumps({"gpt-test": {"input_cost_per_token": 1e-6}}).encode()
            with patch("monitor.fetch_bytes", return_value=body), patch("monitor.send_wecom") as notify:
                for _ in range(3):
                    self.assertFalse(monitor.check_once())
                notify.assert_not_called()
            self.assertFalse((Path(directory) / "history.jsonl").exists())

    def test_message_omits_unchanged_statistics(self):
        message = format_message(
            "test", {"changed": [], "added": [], "removed": []}, "now",
            account_rename_changes=[{"current_name": "old", "desired_name": "new", "ratio": 0.04}],
        )
        self.assertIn("old", message)
        self.assertIn("new", message)
        self.assertNotIn("价格变动模型：0", message)
        self.assertNotIn("账号配置变动：0", message)
        self.assertNotIn("上游账号监控", message)
        self.assertNotIn("分组倍率：已读取", message)

    def test_changed_check_sends_notification_and_history(self):
        with tempfile.TemporaryDirectory() as directory:
            monitor = PriceMonitor(Path(directory), "test", "https://prices", "", "https://wecom")
            first = json.dumps({"gpt-test": {"input_cost_per_token": 1e-6}}).encode()
            second = json.dumps({"gpt-test": {"input_cost_per_token": 2e-6}}).encode()
            with patch("monitor.fetch_bytes", return_value=first):
                monitor.check_once()
            with patch("monitor.fetch_bytes", return_value=second), patch("monitor.send_wecom") as notify:
                monitor.check_once()
                notify.assert_called_once()
            history = (Path(directory) / "history.jsonl").read_text(encoding="utf-8").strip()
            self.assertIn("gpt-test", history)

    def test_message_contains_change_summary(self):
        message = format_message(
            "test",
            {"changed": [{"model": "gpt-test", "before": {"x": 1}, "after": {"x": 2}}], "added": [], "removed": []},
            "2026-07-18T00:00:00+00:00",
        )
        self.assertIn("gpt-test", message)
        self.assertIn("1 -> 2", message)

    def test_message_contains_related_sub2api_account_names(self):
        message = format_message(
            "test",
            {"changed": [{"model": "gpt-5.4", "before": {"x": 1}, "after": {"x": 2}}], "added": [], "removed": []},
            "2026-07-18T00:00:00+00:00",
            accounts={
                "1": {"name": "qr-gpt-0.035", "models": ["gpt-5.4"]},
                "2": {"name": "jis-gpt-0.03", "models": ["gpt-5.5"]},
            },
        )
        self.assertIn("qr-gpt-0.035", message)
        self.assertNotIn("jis-gpt-0.03", message)


if __name__ == "__main__":
    unittest.main()
