package framework

import (
	"reflect"
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

func TestIsZeroBool(t *testing.T) {
	v := reflect.ValueOf(false)
	if !isZero(v) {
		t.Error("false bool should be zero")
	}
	v = reflect.ValueOf(true)
	if isZero(v) {
		t.Error("true bool should not be zero")
	}
}

func TestIsZeroInt(t *testing.T) {
	v := reflect.ValueOf(0)
	if !isZero(v) {
		t.Error("0 int should be zero")
	}
	v = reflect.ValueOf(42)
	if isZero(v) {
		t.Error("42 int should not be zero")
	}
}

func TestIsZeroString(t *testing.T) {
	v := reflect.ValueOf("")
	if !isZero(v) {
		t.Error("empty string should be zero")
	}
	v = reflect.ValueOf("hello")
	if isZero(v) {
		t.Error("non-empty string should not be zero")
	}
}

func TestIsZeroPtr(t *testing.T) {
	var nilPtr *int = nil
	v := reflect.ValueOf(nilPtr)
	if !isZero(v) {
		t.Error("nil pointer should be zero")
	}
	x := 42
	v = reflect.ValueOf(&x)
	if isZero(v) {
		t.Error("non-nil pointer should not be zero")
	}
}

func TestIsZeroSlice(t *testing.T) {
	v := reflect.ValueOf([]int(nil))
	if !isZero(v) {
		t.Error("nil slice should be zero")
	}
}

func TestIsZeroFloat(t *testing.T) {
	v := reflect.ValueOf(0.0)
	if !isZero(v) {
		t.Error("0.0 float should be zero")
	}
	v = reflect.ValueOf(3.14)
	if isZero(v) {
		t.Error("3.14 float should not be zero")
	}
}

func TestIsZeroUint(t *testing.T) {
	v := reflect.ValueOf(uint(0))
	if !isZero(v) {
		t.Error("0 uint should be zero")
	}
	v = reflect.ValueOf(uint(42))
	if isZero(v) {
		t.Error("42 uint should not be zero")
	}
}

func TestIsZeroMap(t *testing.T) {
	v := reflect.ValueOf(map[string]int(nil))
	if !isZero(v) {
		t.Error("nil map should be zero")
	}
}

func TestIsZeroChan(t *testing.T) {
	v := reflect.ValueOf((chan int)(nil))
	if !isZero(v) {
		t.Error("nil chan should be zero")
	}
}
