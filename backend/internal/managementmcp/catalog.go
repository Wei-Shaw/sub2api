// Package managementmcp exposes a bounded set of platform management operations.
package managementmcp

import "strings"

// ToolSpec binds one MCP tool to an existing authenticated management route.
// InputSchema permits only named path parameters, query parameters and body fields;
// clients never supply a URL, HTTP method or credentials in tool arguments.
type ToolSpec struct {
	Name             string
	Description      string
	Method           string
	Path             string
	Admin            bool
	ReadOnly         bool
	Destructive      bool
	RevealCreatedKey bool
	InputSchema      map[string]any
}

// ToolCatalog returns fresh schemas so filtering one connection's tools cannot
// alter another connection's permissions or input validation.
func ToolCatalog() []ToolSpec {
	keyList := catalogPage(map[string]any{
		"search":   catalogText("Search API key names", 100),
		"status":   catalogEnum("Key status", "active", "inactive", "quota_exhausted", "expired"),
		"group_id": catalogID("Group ID"),
	})
	keyCreate := catalogObject(map[string]any{
		"name":            catalogNonemptyText("Name for the new API key", 100),
		"group_id":        catalogID("Accessible group ID; use user_list_groups to find one"),
		"quota":           catalogNumber("Lifetime spending limit in USD; 0 means unlimited", 0),
		"expires_in_days": catalogInteger("Lifetime in days", 1, 36500),
		"ip_whitelist":    catalogStrings("Allowed IP addresses or CIDR ranges; empty means no allowlist", 100, 128),
		"ip_blacklist":    catalogStrings("Blocked IP addresses or CIDR ranges", 100, 128),
		"rate_limit_5h":   catalogNumber("Rolling 5-hour spending limit in USD; 0 means unlimited", 0),
		"rate_limit_1d":   catalogNumber("Rolling 1-day spending limit in USD; 0 means unlimited", 0),
		"rate_limit_7d":   catalogNumber("Rolling 7-day spending limit in USD; 0 means unlimited", 0),
	}, "name")
	keyUpdate := catalogObject(map[string]any{
		"name":          catalogNonemptyText("New API key name", 100),
		"group_id":      catalogID("Accessible destination group ID"),
		"status":        catalogEnum("Enable or disable this key", "active", "inactive"),
		"quota":         catalogNumber("Lifetime spending limit in USD; 0 means unlimited", 0),
		"expires_at":    catalogText("Expiry in RFC3339 format; empty string removes expiry", 64),
		"ip_whitelist":  catalogStrings("Replace allowed IP addresses/CIDRs; empty clears the list", 100, 128),
		"ip_blacklist":  catalogStrings("Replace blocked IP addresses/CIDRs; empty clears the list", 100, 128),
		"rate_limit_5h": catalogNumber("Rolling 5-hour spending limit in USD; 0 means unlimited", 0),
		"rate_limit_1d": catalogNumber("Rolling 1-day spending limit in USD; 0 means unlimited", 0),
		"rate_limit_7d": catalogNumber("Rolling 7-day spending limit in USD; 0 means unlimited", 0),
	})
	keyUpdate["minProperties"] = 1

	userUpdate := catalogObject(map[string]any{
		"username":               catalogText("Display name", 100),
		"notes":                  catalogText("Administrator notes", 2000),
		"concurrency":            catalogInteger("Maximum concurrent requests", 1, 100000),
		"rpm_limit":              catalogInteger("Requests per minute; 0 means unlimited", 0, 1000000),
		"status":                 catalogEnum("User status; disabling blocks access", "active", "disabled"),
		"allowed_groups":         catalogIDs("Replace the user's allowed group IDs"),
		"restrict_public_groups": catalogBool("Restrict public groups to allowed_groups"),
	})
	userUpdate["minProperties"] = 1

	groupCreateFields := catalogGroupFields()
	groupCreateFields["platform"] = catalogNonemptyText("Platform identifier, for example anthropic, openai, gemini or composite", 64)
	groupCreate := catalogObject(groupCreateFields, "name", "platform", "rate_multiplier")
	groupUpdateFields := catalogGroupFields()
	groupUpdateFields["status"] = catalogEnum("Group availability", "active", "inactive")
	groupUpdate := catalogObject(groupUpdateFields)
	groupUpdate["minProperties"] = 1

	accountUpdate := catalogObject(map[string]any{
		"name":                  catalogNonemptyText("Upstream account name", 100),
		"notes":                 catalogText("Administrator notes", 2000),
		"concurrency":           catalogInteger("Maximum concurrent requests", 1, 100000),
		"priority":              catalogInteger("Scheduling priority; lower values are preferred", 0, 100000),
		"rate_multiplier":       catalogNumber("Account cost multiplier; 0 means free", 0),
		"status":                catalogEnum("Account availability", "active", "inactive", "error"),
		"expires_at":            catalogInteger("Unix expiry timestamp in seconds; 0 removes expiry", 0, 253402300799),
		"auto_pause_on_expired": catalogBool("Stop scheduling after expiry"),
	})
	accountUpdate["minProperties"] = 1

	tools := []ToolSpec{
		catalogTool("user_get_profile", "Get your profile, balance and account limits. Always uses the authenticated user.", "GET", "/user/profile", nil, nil),
		catalogTool("user_get_platform_quotas", "Get your spending quota and usage windows for each platform.", "GET", "/user/platform-quotas", nil, nil),
		catalogTool("user_list_groups", "List the groups available to your API keys, including platform and pricing information.", "GET", "/groups/available", nil, nil),
		catalogTool("user_list_channels", "List channels and supported models visible to you. Returns an empty list when channel discovery is disabled.", "GET", "/channels/available", nil, nil),
		catalogTool("user_list_api_keys", "List your API key metadata and limits with pagination. Existing key secrets are omitted.", "GET", "/keys", keyList, nil),
		catalogTool("user_get_api_key", "Get one of your API keys by ID, including status and limits. The key secret is omitted.", "GET", "/keys/:id", nil, nil),
		catalogTool("user_create_api_key", "Create an API key owned by you. Returns the newly created secret once in this tool result; store it securely.", "POST", "/keys", nil, keyCreate),
		catalogTool("user_update_api_key", "Update one of your API keys. Only supplied fields change; disabling a key immediately blocks its use. Returns metadata without its secret.", "PUT", "/keys/:id", nil, keyUpdate),
		catalogTool("user_delete_api_key", "Permanently delete one of your API keys. Applications using it will lose access. Returns the deletion result.", "DELETE", "/keys/:id", nil, nil),
		catalogTool("user_list_usage", "List your request usage, tokens and costs with pagination and optional date, model and key filters.", "GET", "/usage", catalogPage(catalogUsageFields(false)), nil),
		catalogTool("user_get_usage_stats", "Get aggregate request counts, token usage and costs for your account; defaults to the last seven days. Dates are inclusive YYYY-MM-DD.", "GET", "/usage/stats", catalogObject(catalogUsageFields(false)), nil),
		catalogTool("user_get_dashboard", "Get your dashboard totals, request counts and spending summary.", "GET", "/usage/dashboard/stats", nil, nil),
		catalogTool("user_list_subscriptions", "List your subscriptions, associated groups, validity and current usage.", "GET", "/subscriptions", nil, nil),
		catalogTool("user_get_subscription_summary", "Get a summary of your active subscription limits and remaining allowance.", "GET", "/subscriptions/summary", nil, nil),

		catalogTool("admin_get_dashboard", "Get platform-wide user/account counts, usage, spending and service uptime.", "GET", "/admin/dashboard/stats", nil, nil),
		catalogTool("admin_list_users", "List platform users and account limits with pagination. May include contact information and administrator notes.", "GET", "/admin/users", catalogPage(map[string]any{
			"search":     catalogText("Search email or display name", 100),
			"status":     catalogEnum("User status", "active", "disabled"),
			"role":       catalogEnum("User role", "admin", "user"),
			"group_name": catalogText("Search allowed group name", 100),
		}), nil),
		catalogTool("admin_get_user", "Get a platform user's profile, balance and limits by ID. Existing API key secrets are omitted.", "GET", "/admin/users/:id", nil, nil),
		catalogTool("admin_update_user", "Update a user's display name, notes, limits, status or group access. Does not change passwords, roles or identity bindings. Returns the updated user.", "PUT", "/admin/users/:id", nil, userUpdate),
		catalogTool("admin_update_user_balance", "Set, add to or subtract from a user's balance in USD. This changes real billing credit; verify the user, amount and operation before calling. Returns the updated result.", "POST", "/admin/users/:id/balance", nil, catalogObject(map[string]any{
			"balance":   map[string]any{"type": "number", "exclusiveMinimum": 0, "description": "Positive amount in USD"},
			"operation": catalogEnum("How to apply the amount", "set", "add", "subtract"),
			"notes":     catalogText("Reason for the balance adjustment", 2000),
		}, "balance", "operation", "notes")),
		catalogTool("admin_list_groups", "List platform groups, pricing multipliers and status with pagination.", "GET", "/admin/groups", catalogPage(map[string]any{
			"search": catalogText("Search group name", 100), "platform": catalogText("Platform identifier", 64),
			"status": catalogEnum("Group status", "active", "inactive"), "is_exclusive": catalogBool("Filter exclusive groups"),
		}), nil),
		catalogTool("admin_get_group", "Get one group's configuration and model allowlist by ID.", "GET", "/admin/groups/:id", nil, nil),
		catalogTool("admin_create_group", "Create a platform group with explicit platform and price multiplier. Returns the group; attach upstream accounts in the web console before serving traffic.", "POST", "/admin/groups", nil, groupCreate),
		catalogTool("admin_update_group", "Update group name, pricing, spending limits, availability or model allowlist. Changes affect every user of this group. Only supplied fields change.", "PUT", "/admin/groups/:id", nil, groupUpdate),
		catalogTool("admin_delete_group", "Delete a platform group. This can disrupt API keys and subscriptions associated with it. Returns the deletion result or a dependency error.", "DELETE", "/admin/groups/:id", nil, nil),
		catalogTool("admin_list_accounts", "List upstream account metadata, status and scheduling settings with pagination. Credentials are omitted.", "GET", "/admin/accounts", catalogPage(map[string]any{
			"search": catalogText("Search account name", 100), "platform": catalogText("Platform identifier", 64),
			"status": catalogEnum("Account status", "active", "inactive", "error", "rate_limited", "temp_unschedulable", "unschedulable"),
			"type":   catalogEnum("Upstream authentication type", "oauth", "setup-token", "apikey", "upstream", "bedrock", "service_account"),
			"group":  catalogID("Group ID"), "lite": catalogBool("Request a lightweight account list"),
		}), nil),
		catalogTool("admin_get_account", "Get one upstream account's metadata and runtime status. Credentials are omitted.", "GET", "/admin/accounts/:id", nil, nil),
		catalogTool("admin_update_account", "Update upstream account name, notes, concurrency, priority, cost multiplier, status or expiry. Does not accept credentials or network settings.", "PUT", "/admin/accounts/:id", nil, accountUpdate),
		catalogTool("admin_set_account_schedulable", "Enable or disable scheduling for an upstream account. Disabling prevents selection for new requests. Returns updated account metadata.", "POST", "/admin/accounts/:id/schedulable", nil, catalogObject(map[string]any{
			"schedulable": catalogBool("Whether the scheduler may select this account"),
		}, "schedulable")),
		catalogTool("admin_clear_account_error", "Clear an upstream account's error state so it can recover. Investigate and fix the cause first. Returns the recovery result.", "POST", "/admin/accounts/:id/clear-error", nil, nil),
		catalogTool("admin_list_usage", "List platform-wide request usage, tokens and costs with pagination. Filter by user, account, group, model or inclusive date range.", "GET", "/admin/usage", catalogPage(catalogUsageFields(true)), nil),
		catalogTool("admin_get_usage_stats", "Get platform-wide aggregate usage and costs; defaults to today. Both start_date and end_date should be supplied for a custom range.", "GET", "/admin/usage/stats", catalogObject(catalogUsageFields(true)), nil),
		catalogTool("admin_list_subscriptions", "List platform subscriptions and their user/group associations, expiry and quota usage with pagination.", "GET", "/admin/subscriptions", catalogPage(map[string]any{
			"user_id": catalogID("User ID"), "group_id": catalogID("Group ID"),
			"status": catalogEnum("Subscription status", "active", "expired", "revoked"), "platform": catalogText("Platform identifier", 64),
		}), nil),
		catalogTool("admin_get_subscription", "Get one platform subscription and its user/group association by ID.", "GET", "/admin/subscriptions/:id", nil, nil),
		catalogTool("admin_assign_subscription", "Assign a subscription group to a user for the specified number of days. An existing subscription may be extended; do not retry blindly. Returns the resulting subscription.", "POST", "/admin/subscriptions/assign", nil, catalogObject(map[string]any{
			"user_id": catalogID("Recipient user ID"), "group_id": catalogID("Subscription group ID"),
			"validity_days": catalogInteger("Validity in days", 1, 36500), "notes": catalogText("Assignment notes", 2000),
		}, "user_id", "group_id", "validity_days")),
		catalogTool("admin_extend_subscription", "Extend a subscription by the specified positive number of days. Changes billing entitlement; returns the updated subscription.", "POST", "/admin/subscriptions/:id/extend", nil, catalogObject(map[string]any{
			"days": catalogInteger("Additional validity in days", 1, 36500),
		}, "days")),
		catalogTool("admin_revoke_subscription", "Revoke a subscription immediately. The user loses the associated entitlement. Returns the revocation result.", "POST", "/admin/subscriptions/:id/revoke", nil, nil),
		catalogTool("admin_get_ops_overview", "Get platform operational health, traffic, latency and error overview for troubleshooting.", "GET", "/admin/ops/dashboard/overview", nil, nil),
	}
	for i := range tools {
		if tools[i].Name == "user_create_api_key" {
			tools[i].RevealCreatedKey = true
		}
		// Creation does not remove or overwrite existing state. Subscription
		// assignment is deliberately excluded because it may extend an existing one.
		if tools[i].Name == "user_create_api_key" || tools[i].Name == "admin_create_group" {
			tools[i].Destructive = false
		}
	}
	return tools
}

func catalogTool(name, description, method, path string, query, body map[string]any) ToolSpec {
	properties := map[string]any{}
	required := []string{}
	if strings.Contains(path, ":id") {
		properties["id"] = catalogID("Resource ID")
		required = append(required, "id")
	}
	if query != nil {
		properties["query"] = query
	}
	if body != nil {
		properties["body"] = body
		required = append(required, "body")
	}
	// These existing routes require an idempotency key. Keep retries within
	// their established coordinator rather than inventing MCP-side settlement.
	switch name {
	case "user_create_api_key", "admin_update_user_balance", "admin_extend_subscription":
		properties["idempotency_key"] = map[string]any{
			"type": "string", "minLength": 1, "maxLength": 128, "pattern": "^[!-~]+$",
			"description": "Unique operation ID (for example a UUID). Reuse the same value and arguments when retrying this operation; use a new value for a new intended operation.",
		}
		required = append(required, "idempotency_key")
	}
	return ToolSpec{
		Name: name, Description: description, Method: method, Path: "/api/v1" + path,
		Admin: strings.HasPrefix(path, "/admin/"), ReadOnly: method == "GET",
		Destructive: method != "GET", InputSchema: catalogObject(properties, required...),
	}
}

func catalogObject(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func catalogText(description string, maxLength int) map[string]any {
	return map[string]any{"type": "string", "description": description, "maxLength": maxLength}
}

func catalogNonemptyText(description string, maxLength int) map[string]any {
	schema := catalogText(description, maxLength)
	schema["minLength"] = 1
	return schema
}

func catalogEnum(description string, values ...string) map[string]any {
	return map[string]any{"type": "string", "description": description, "enum": values}
}

func catalogInteger(description string, min, max int64) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": min, "maximum": max}
}

func catalogID(description string) map[string]any {
	return catalogInteger(description, 1, 9007199254740991)
}

func catalogNumber(description string, min float64) map[string]any {
	return map[string]any{"type": "number", "description": description, "minimum": min}
}

func catalogBool(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}

func catalogStrings(description string, maxItems, maxLength int) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": catalogNonemptyText("Value", maxLength), "maxItems": maxItems}
}

func catalogIDs(description string) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": catalogID("Resource ID"), "maxItems": 100, "uniqueItems": true}
}

func catalogPage(fields map[string]any) map[string]any {
	fields["page"] = catalogInteger("Page number, starting at 1", 1, 1000000)
	fields["page_size"] = catalogInteger("Results per page; at most 100", 1, 100)
	return catalogObject(fields)
}

func catalogUsageFields(admin bool) map[string]any {
	fields := map[string]any{
		"api_key_id": catalogID("Filter by API key ID"), "group_id": catalogID("Filter by group ID"),
		"model":      catalogText("Filter by requested model", 200),
		"start_date": map[string]any{"type": "string", "description": "Inclusive start date (YYYY-MM-DD)", "pattern": `^\d{4}-\d{2}-\d{2}$`},
		"end_date":   map[string]any{"type": "string", "description": "Inclusive end date (YYYY-MM-DD)", "pattern": `^\d{4}-\d{2}-\d{2}$`},
		"timezone":   catalogText("IANA time zone, for example Asia/Shanghai or UTC", 100),
		"period":     catalogEnum("Stats date range when explicit dates are omitted", "today", "week", "month"),
		"stream":     catalogBool("Filter streaming or non-streaming requests"),
	}
	if admin {
		fields["user_id"] = catalogID("Filter by user ID")
		fields["account_id"] = catalogID("Filter by upstream account ID")
	}
	return fields
}

func catalogGroupFields() map[string]any {
	return map[string]any{
		"name": catalogNonemptyText("Group name", 100), "description": catalogText("Group description", 2000),
		"rate_multiplier":   map[string]any{"type": "number", "exclusiveMinimum": 0, "description": "Positive user billing multiplier; 1 is standard pricing"},
		"is_exclusive":      catalogBool("Limit access to users explicitly assigned this group"),
		"subscription_type": catalogEnum("Group billing mode", "standard", "subscription"),
		"daily_limit_usd":   catalogNumber("Daily subscription spending limit in USD; -1 means unlimited", -1),
		"weekly_limit_usd":  catalogNumber("Weekly subscription spending limit in USD; -1 means unlimited", -1),
		"monthly_limit_usd": catalogNumber("Monthly subscription spending limit in USD; -1 means unlimited", -1),
		"rpm_limit":         catalogInteger("Requests per minute; 0 means unlimited", 0, 1000000),
		"model_allowlist": catalogObject(map[string]any{
			"enabled": catalogBool("Enable the model allowlist; an enabled list must contain at least one model"),
			"models":  catalogStrings("Allowed upstream model IDs or patterns containing *", 1000, 200),
		}, "enabled", "models"),
	}
}
