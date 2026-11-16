package repository

import (
	"context"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	dbaccount "github.com/Wei-Shaw/sub2api/ent/account"
	dbpredicate "github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/lib/pq"

	entsql "entgo.io/ent/dialect/sql"
)

// Apply the tier before counting and pagination, including inherited shadow plans.
// The optional argument keeps existing account-list callers unfiltered.
func (r *accountRepository) applySubscriptionTierFilter(ctx context.Context, q *dbent.AccountQuery, tiers []string) error {
	if len(tiers) == 0 || strings.TrimSpace(tiers[0]) == "" {
		return nil
	}
	filter := strings.TrimSpace(tiers[0])
	platform, tier, _ := strings.Cut(filter, ":")
	if platform == service.PlatformGrok {
		return r.applyGrokSubscriptionTierFilter(ctx, q, tier)
	}
	q.Where(dbpredicate.Account(func(s *entsql.Selector) {
		credentials := s.C(dbaccount.FieldCredentials)
		extra := s.C(dbaccount.FieldExtra)
		// Ignore non-string JSON values and whitespace-only manual overrides.
		text := func(column, key string) string {
			return "CASE WHEN jsonb_typeof(" + column + "->'" + key + "') = 'string' THEN NULLIF(BTRIM(" + column + "->>'" + key + "'), '') END"
		}
		parentPlan := "(SELECT NULLIF(BTRIM(tier_parent.credentials->>'plan_type'), '') FROM accounts AS tier_parent WHERE tier_parent.id = " + s.C(dbaccount.FieldParentAccountID) + " AND tier_parent.deleted_at IS NULL AND jsonb_typeof(tier_parent.credentials->'plan_type') = 'string')"
		plan := "COALESCE(" + text(credentials, "plan_type") + ", " + parentPlan + ", '')"
		antigravity := "COALESCE(" + text(extra+"->'load_code_assist'->'paidTier'", "id") + ", " + text(extra+"->'load_code_assist'->'currentTier'", "id") + ", " + plan + ")"
		grok := "COALESCE(" + text(credentials, "subscription_tier") + ", " + text(extra+"->'grok_billing_snapshot'", "plan") + ", " + text(extra+"->'grok_usage_snapshot'", "subscription_tier") + ", " + text(extra+"->'grok_quota_snapshot'", "subscription_tier") + ", " + text(extra, "subscription_tier") + ", " + plan + ")"
		effective := "CASE " + s.C(dbaccount.FieldPlatform) + " WHEN 'antigravity' THEN " + antigravity + " WHEN 'grok' THEN " + grok + " ELSE " + plan + " END"
		normalized := "regexp_replace(LOWER(" + effective + "), '[[:space:]_-]+', '', 'g')"
		if filter == "__unset__" {
			s.Where(entsql.ExprP(normalized+" = ?", ""))
			return
		}
		s.Where(entsql.EQ(s.C(dbaccount.FieldPlatform), platform))
		// Group aliases that share the label shown in the account table.
		keys := []string{tier}
		if platform == service.PlatformOpenAI {
			switch tier {
			case "pro":
				keys = []string{"pro", "chatgptpro"}
			case "team":
				keys = []string{"team", "selfservebusinessusagebased"}
			case "enterprise":
				keys = []string{"business", "enterprise", "ent26", "enterprisecbpusagebased"}
			}
		}
		if platform == service.PlatformAnthropic {
			keys = []string{tier, "claude" + tier}
		}
		if platform == service.PlatformAntigravity {
			switch tier {
			case "freetier":
				keys = []string{"freetier", "free"}
			case "g1protier":
				keys = []string{"g1protier", "pro"}
			case "g1ultratier":
				keys = []string{"g1ultratier", "ultra"}
			}
		}
		s.Where(entsql.ExprP(normalized+" = ANY(?::text[])", pq.Array(keys)))
	}))
	return nil
}

// Grok Heavy can be inferred from a recent 4.5 quota window rather than a plain
// credential value. Evaluate persisted snapshots with the same rules as the UI,
// in bounded batches, then constrain the original query before pagination.
func (r *accountRepository) applyGrokSubscriptionTierFilter(ctx context.Context, q *dbent.AccountQuery, tier string) error {
	q.Where(dbaccount.PlatformEQ(service.PlatformGrok))
	ids := []int64{}
	var after int64
	now := time.Now()
	for {
		rows, err := q.Clone().Where(dbaccount.IDGT(after)).
			Select(dbaccount.FieldID, dbaccount.FieldCredentials, dbaccount.FieldExtra).
			Order(dbent.Asc(dbaccount.FieldID)).Limit(500).All(ctx)
		if err != nil {
			return err
		}
		for _, row := range rows {
			if grokListSubscriptionTier(row.Credentials, row.Extra, now) == tier {
				ids = append(ids, row.ID)
			}
			after = row.ID
		}
		if len(rows) < 500 {
			break
		}
	}
	q.Where(dbpredicate.Account(func(s *entsql.Selector) {
		s.Where(entsql.ExprP(s.C(dbaccount.FieldID)+" = ANY(?::bigint[])", pq.Array(ids)))
	}))
	return nil
}

func grokListSubscriptionTier(credentials, extra map[string]any, now time.Time) string {
	key := func(raw string) string {
		return strings.NewReplacer(" ", "", "_", "", "-", "", "\t", "", "\n", "", "\r", "").Replace(strings.ToLower(strings.TrimSpace(raw)))
	}
	text := func(values ...any) string {
		for _, value := range values {
			if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
		return ""
	}
	object := func(value any) map[string]any { m, _ := value.(map[string]any); return m }
	billing := object(extra["grok_billing_snapshot"])
	usage := object(extra["grok_usage_snapshot"])
	legacy := object(extra["grok_quota_snapshot"])
	quota := usage
	if quota == nil {
		quota = legacy
	}
	credential := key(text(credentials["subscription_tier"]))
	if credential != "" && credential != "supergrokpro" {
		if credential == "basic" {
			return "free"
		}
		return credential
	}
	fresh := func(raw any) bool {
		parsed, err := time.Parse(time.RFC3339Nano, text(raw))
		age := now.Sub(parsed)
		return err == nil && age <= 24*time.Hour && age >= -5*time.Minute
	}
	requests := object(quota["requests"])
	tokens := object(quota["tokens"])
	requestLimit, _ := requests["limit"].(float64)
	tokenLimit, _ := tokens["limit"].(float64)
	heavy := key(text(quota["plan_from_45_responses"])) == "supergrokheavy" && fresh(quota["plan_from_45_responses_at"])
	heavy = heavy || (xai.IsGrok45ResponsesQuotaModel(text(quota["model"])) && fresh(text(quota["last_headers_seen_at"], quota["updated_at"])) && (requestLimit >= 8300 || tokenLimit >= 53_000_000))
	billingPlan := key(text(billing["plan"]))
	if heavy && (credential == "supergrokpro" || billingPlan == "supergrok" || billingPlan == "supergrokpro") {
		return "supergrokheavy"
	}
	if credential == "supergrokpro" {
		if billingPlan != "" {
			return billingPlan
		}
		return "supergrok"
	}
	result := key(text(billing["plan"], usage["subscription_tier"], legacy["subscription_tier"], extra["subscription_tier"], credentials["plan_type"]))
	if result == "basic" {
		return "free"
	}
	return result
}
