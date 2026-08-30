package ent

import (
	"database/sql"
	"testing"

	"github.com/Wei-Shaw/sub2api/ent/account"
)

func TestAccountQuerySelectsAndScansOpenAIHealthRouteGeneration(t *testing.T) {
	columnIndex := -1
	for i, column := range account.Columns {
		if column == account.FieldOpenAiHealthRouteGeneration {
			columnIndex = i
			break
		}
	}
	if columnIndex < 0 {
		t.Fatal("account query columns omit durable generation storage key")
	}

	model := &Account{}
	values, err := model.scanValues(account.Columns)
	if err != nil {
		t.Fatalf("scan values: %v", err)
	}
	nullValue, ok := values[columnIndex].(*sql.NullInt64)
	if !ok {
		t.Fatalf("generation scan target = %T, want *sql.NullInt64", values[columnIndex])
	}

	if err := model.assignValues(account.Columns, values); err != nil {
		t.Fatalf("assign NULL values: %v", err)
	}
	if model.OpenAiHealthRouteGeneration != nil {
		t.Fatalf("NULL durable generation hydrated as %v", *model.OpenAiHealthRouteGeneration)
	}

	nullValue.Int64 = 41
	nullValue.Valid = true
	if err := model.assignValues(account.Columns, values); err != nil {
		t.Fatalf("assign non-NULL values: %v", err)
	}
	if model.OpenAiHealthRouteGeneration == nil || *model.OpenAiHealthRouteGeneration != 41 {
		t.Fatalf("non-NULL durable generation = %v, want 41", model.OpenAiHealthRouteGeneration)
	}
}
