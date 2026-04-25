package framework

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type bindArgsOptions struct {
	required map[string]struct{}
}

func BindArgs[T any](args map[string]interface{}) (T, error) {
	var result T

	if args == nil {
		return result, nil
	}

	b, err := json.Marshal(args)
	if err != nil {
		return result, fmt.Errorf("args serialisation failed: %w", err)
	}

	if err := json.Unmarshal(b, &result); err != nil {
		return result, fmt.Errorf("args binding failed: %w", err)
	}

	if err := validateRequired(result); err != nil {
		return result, err
	}

	return result, nil
}

func validateRequired(v any) error {
	t := reflect.TypeOf(v)
	if t == nil {
		return nil
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}

		bindingTag := field.Tag.Get("binding")
		if bindingTag != "required" {
			continue
		}

		val := reflect.ValueOf(v)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}

		fieldVal := val.Field(i)
		if isZero(fieldVal) {
			jsonName := getJSONName(field)
			return fmt.Errorf("required field %s is missing", jsonName)
		}
	}

	return nil
}

func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.String:
		return v.String() == ""
	case reflect.Ptr:
		return v.IsNil()
	case reflect.Interface:
		return v.IsNil()
	case reflect.Slice, reflect.Map, reflect.Func, reflect.Chan:
		return v.IsNil()
	default:
		return false
	}
}

func getJSONName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	for i := 0; i < len(tag); i++ {
		if tag[i] == ',' {
			return tag[:i]
		}
	}
	return tag
}
