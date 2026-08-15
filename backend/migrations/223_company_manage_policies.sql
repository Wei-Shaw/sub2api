-- Add two new managed policies to the organization authorization system:
--   * CompanyFinanceManage – bundles read-only finance visibility plus the
--     ability to allocate/reclaim member balance, manage per-member spend
--     limits and purchase / manage company subscription plans.
--   * IAMUserManage – grants the ability to create and administer IAM users
--     of the organization.
-- Migration is idempotent so it can be reapplied safely.

INSERT INTO managed_policies (policy_key, display_name, policy_type, description, version)
VALUES
    ('CompanyFinanceManage', 'Company finance manage', 'system',
     'Read root balance, allocate/reclaim member balance, manage member spend limits, and purchase or cancel company subscription plans.',
     1),
    ('IAMUserManage', 'IAM user manage', 'system',
     'Create IAM users of the organization and reset their login password (does not include disabling or archiving members).',
     1)
ON CONFLICT (policy_key) DO UPDATE SET
    display_name = EXCLUDED.display_name,
    description  = EXCLUDED.description;

INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.finance.balance.read' FROM managed_policies WHERE policy_key = 'CompanyFinanceManage'
ON CONFLICT DO NOTHING;
INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.balance.allocate' FROM managed_policies WHERE policy_key = 'CompanyFinanceManage'
ON CONFLICT DO NOTHING;
INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.spend_limit.manage' FROM managed_policies WHERE policy_key = 'CompanyFinanceManage'
ON CONFLICT DO NOTHING;
INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.subscription.manage' FROM managed_policies WHERE policy_key = 'CompanyFinanceManage'
ON CONFLICT DO NOTHING;

INSERT INTO managed_policy_actions (policy_id, action)
SELECT id, 'organization.iam.member.manage' FROM managed_policies WHERE policy_key = 'IAMUserManage'
ON CONFLICT DO NOTHING;
