package service

import (
	"os"
	"strings"
	"testing"
)

func TestBalancePackageOrderUsesBalanceGroupID(t *testing.T) {
	source, err := os.ReadFile("payment_order.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "SetBalanceGroupID(*amounts.BalancePackage.BalanceGroupID)") {
		t.Fatalf("balance package orders must persist the package balance group into payment_orders.balance_group_id")
	}
	if strings.Contains(text, "amounts.BalancePackage.GroupID") || strings.Contains(text, "SetSubscriptionGroupID(*amounts.BalancePackage") {
		t.Fatalf("balance package orders must not use subscription_group_id semantics")
	}
}

func TestBalanceFulfillmentUsesUsageGroupWithoutSubscriptionBinding(t *testing.T) {
	source, err := os.ReadFile("payment_fulfillment.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "applyBalancePackageUsageGroup") {
		t.Fatalf("balance package fulfillment should use explicit usage-group semantics")
	}
	if !strings.Contains(text, "o.BalanceGroupID") {
		t.Fatalf("balance package fulfillment must read payment_orders.balance_group_id")
	}
	if strings.Contains(text, "bindBalancePackageEntitlement") {
		t.Fatalf("balance package fulfillment must not use subscription-entitlement naming")
	}
}

func TestBalancePackageConfigRejectsSubscriptionGroups(t *testing.T) {
	source, err := os.ReadFile("payment_config_balance_packages.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if !strings.Contains(text, "standard balance group") || !strings.Contains(text, "not a subscription group") {
		t.Fatalf("balance package config must reject subscription groups and require standard balance groups")
	}
}
