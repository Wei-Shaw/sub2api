package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestResolveOpsDashboardQueryMode_RequestTypeForcesRaw(t *testing.T) {
	requestType := int16(service.RequestTypeWSV2)
	filter := &service.OpsDashboardFilter{
		QueryMode:   service.OpsQueryModePreagg,
		RequestType: &requestType,
	}

	if got := resolveOpsDashboardQueryMode(filter); got != service.OpsQueryModeRaw {
		t.Fatalf("resolveOpsDashboardQueryMode() = %s, want %s", got, service.OpsQueryModeRaw)
	}
}

func TestBuildUsageWhere_RequestTypeUsesLegacyCompatibleCondition(t *testing.T) {
	requestType := int16(service.RequestTypeWSV2)
	filter := &service.OpsDashboardFilter{RequestType: &requestType}

	_, where, args, _ := buildUsageWhere(filter, testTimeA, testTimeB, 1)
	if !strings.Contains(where, "ul.request_type") {
		t.Fatalf("where should include request_type condition: %s", where)
	}
	if !strings.Contains(where, "ul.openai_ws_mode = TRUE") {
		t.Fatalf("where should include openai_ws_mode compatibility: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
}

func TestBuildErrorWhere_RequestTypeUsesQualifiedCondition(t *testing.T) {
	requestType := int16(service.RequestTypeStream)
	filter := &service.OpsDashboardFilter{RequestType: &requestType}

	where, args, _ := buildErrorWhere(filter, testTimeA, testTimeB, 1)
	if !strings.Contains(where, "request_type") {
		t.Fatalf("where should include request_type condition: %s", where)
	}
	if !strings.Contains(where, "stream = TRUE") {
		t.Fatalf("where should include stream compatibility: %s", where)
	}
	if len(args) != 3 {
		t.Fatalf("args len = %d, want 3", len(args))
	}
}
