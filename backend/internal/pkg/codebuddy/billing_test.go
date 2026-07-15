package codebuddy

import (
	"encoding/json"
	"testing"
)

func TestSumBillingUsage(t *testing.T) {
	data := BillingUserResourceData{
		TotalCount: 6,
		TotalDosage: 4300,
		Accounts: []BillingAccount{
			{AccountID: "1", CycleCapacitySizePrecise: "500", CycleCapacityRemainPrecise: "496.75", CycleCapacityUsedPrecise: "3.25"},
			{AccountID: "2", CycleCapacitySizePrecise: "3000", CycleCapacityRemainPrecise: "3000", CycleCapacityUsedPrecise: "0"},
			{AccountID: "3", CycleCapacitySizePrecise: "100", CycleCapacityRemainPrecise: "100", CycleCapacityUsedPrecise: "0"},
			{AccountID: "4", CycleCapacitySizePrecise: "300", CycleCapacityRemainPrecise: "300", CycleCapacityUsedPrecise: "0"},
			{AccountID: "5", CycleCapacitySizePrecise: "300", CycleCapacityRemainPrecise: "300", CycleCapacityUsedPrecise: "0"},
			{AccountID: "6", CycleCapacitySizePrecise: "100", CycleCapacityRemainPrecise: "100", CycleCapacityUsedPrecise: "0"},
		},
	}
	got := SumBillingUsage(data)
	if got.TotalCapacity != 4300 {
		t.Errorf("TotalCapacity = %v, want 4300", got.TotalCapacity)
	}
	if got.Remaining != 4296.75 {
		t.Errorf("Remaining = %v, want 4296.75", got.Remaining)
	}
	if got.Used != 3.25 {
		t.Errorf("Used = %v, want 3.25", got.Used)
	}
	if got.AccountCount != 6 {
		t.Errorf("AccountCount = %v, want 6", got.AccountCount)
	}
}

func TestParseBillingResponse(t *testing.T) {
	raw := `{
		"code": 0,
		"msg": "OK",
		"data": {"Response": {"Data": {
			"TotalCount": 6,
			"TotalDosage": 4300,
			"Accounts": [
				{"AccountId":"1","CycleCapacitySizePrecise":"500","CycleCapacityRemainPrecise":"496.75","CycleCapacityUsedPrecise":"3.25"},
				{"AccountId":"2","CycleCapacitySizePrecise":"3000","CycleCapacityRemainPrecise":"3000","CycleCapacityUsedPrecise":"0"}
			]
		}}}
	}`
	var resp BillingUserResourceResponse
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Code != 0 {
		t.Fatalf("code = %d", resp.Code)
	}
	got := SumBillingUsage(resp.Data.Response.Data)
	if got.TotalCapacity != 3500 || got.Remaining != 3496.75 || got.Used != 3.25 {
		t.Errorf("unexpected sums: %+v", got)
	}
}
