package framework

import (
	"testing"
)

type testParams struct {
	Query   string `json:"query"    binding:"required"`
	MaxRows int    `json:"max_rows"`
	Schema  string `json:"schema"`
}

func TestBindArgsSuccess(t *testing.T) {
	args := map[string]interface{}{
		"query":    "SELECT * FROM users",
		"max_rows": float64(10),
		"schema":   "public",
	}

	params, err := BindArgs[testParams](args)
	if err != nil {
		t.Fatalf("BindArgs failed: %v", err)
	}

	if params.Query != "SELECT * FROM users" {
		t.Errorf("Query = %q, want %q", params.Query, "SELECT * FROM users")
	}
	if params.MaxRows != 10 {
		t.Errorf("MaxRows = %d, want 10", params.MaxRows)
	}
	if params.Schema != "public" {
		t.Errorf("Schema = %q, want %q", params.Schema, "public")
	}
}

func TestBindArgsMissingRequired(t *testing.T) {
	args := map[string]interface{}{
		"max_rows": float64(10),
	}

	_, err := BindArgs[testParams](args)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}
	if err.Error() != "required field query is missing" {
		t.Errorf("error = %q, want %q", err.Error(), "required field query is missing")
	}
}

func TestBindArgsNilArgs(t *testing.T) {
	params, err := BindArgs[testParams](nil)
	if err != nil {
		t.Fatalf("BindArgs with nil should succeed: %v", err)
	}
	if params.Query != "" {
		t.Errorf("expected zero value on nil args")
	}
}

func TestBindArgsFloatToInt(t *testing.T) {
	type intParams struct {
		Limit int `json:"limit"`
	}

	args := map[string]interface{}{
		"limit": float64(100),
	}

	params, err := BindArgs[intParams](args)
	if err != nil {
		t.Fatalf("BindArgs failed: %v", err)
	}
	if params.Limit != 100 {
		t.Errorf("Limit = %d, want 100", params.Limit)
	}
}

func TestBindArgsNested(t *testing.T) {
	type nestedFilter struct {
		Field string `json:"field" binding:"required"`
	}
	type nestedParams struct {
		Filter nestedFilter `json:"filter"`
	}

	args := map[string]interface{}{
		"filter": map[string]interface{}{
			"field": "name",
		},
	}

	params, err := BindArgs[nestedParams](args)
	if err != nil {
		t.Fatalf("BindArgs failed: %v", err)
	}
	if params.Filter.Field != "name" {
		t.Errorf("Filter.Field = %q, want %q", params.Filter.Field, "name")
	}
}

func TestBindArgsInvalidJSON(t *testing.T) {
	type badParams struct {
		Query int `json:"query"`
	}

	args := map[string]interface{}{
		"query": "not-an-int",
	}

	_, err := BindArgs[badParams](args)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}
