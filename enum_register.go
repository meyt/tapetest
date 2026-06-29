package tapetest

import (
	"reflect"
	"sync"
)

// --- Enum registry ---
//
// Tests register named string types via RegisterEnum. The recorder reflects
// on each body field, query param, and path param value; if the value's type
// is registered, the allowed values are recorded alongside the request so the
// OpenAPI generator can emit an `enum` constraint on the corresponding schema.
//
//      type StatusType string
//      const (
//          Pending  StatusType = "pending"
//          Active   StatusType = "active"
//      )
//      tapetest.RegisterEnum(Pending, Active)
//

var (
	enumCacheMu sync.RWMutex
	enumCache   map[string][]string
)

// RegisterEnum registers the allowed values for a named string type. Pass the
// const values directly — the type is inferred via reflection.
//
//	tapetest.RegisterEnum(Male, Female)
//	tapetest.RegisterEnum(Pending, Active, Inactive)
//
// Calling RegisterEnum twice for the same type replaces the previous entry.
// Calling with zero values is a no-op.
//
// RegisterEnum is safe for concurrent use and may be called from init() or
// from inside a test function. Values declared inside a test function must
// call RegisterEnum within that function (Go scope rules apply).
func RegisterEnum[T ~string](values ...T) {
	if len(values) == 0 {
		return
	}
	t := reflect.TypeOf(values[0])
	if t.Name() == "" || t.PkgPath() == "" {
		// Unnamed type — nothing to register under.
		return
	}
	key := t.PkgPath() + "." + t.Name()
	strs := make([]string, len(values))
	for i, v := range values {
		strs[i] = string(v)
	}
	enumCacheMu.Lock()
	defer enumCacheMu.Unlock()
	if enumCache == nil {
		enumCache = make(map[string][]string)
	}
	enumCache[key] = strs
}

// Enum is alias for RegisterEnum
//
//	type StatusType string
//	const (
//	    Pending  StatusType = "pending"
//	    Active   StatusType = "active"
//	)
//	tapetest.Enum(Pending, Active)
func Enum[T ~string](values ...T) {
	RegisterEnum(values...)
}

// LookupEnum returns the allowed enum values for the given reflect.Type, or
// nil if the type is not registered.
func LookupEnum(t reflect.Type) []string {
	if t == nil || t.Name() == "" || t.PkgPath() == "" {
		return nil
	}
	enumCacheMu.RLock()
	defer enumCacheMu.RUnlock()
	if enumCache == nil {
		return nil
	}
	return enumCache[t.PkgPath()+"."+t.Name()]
}

// GetEnumCache returns a copy of the current enum cache. Useful for debugging
// when enums aren't being detected — dump it and verify the expected type key
// is present.
//
//	t.Logf("enum cache: %v", tapetest.GetEnumCache())
func GetEnumCache() map[string][]string {
	enumCacheMu.RLock()
	defer enumCacheMu.RUnlock()
	if enumCache == nil {
		return nil
	}
	out := make(map[string][]string, len(enumCache))
	for k, v := range enumCache {
		out[k] = v
	}
	return out
}

// --- Reflection helpers used by recordExchange ---

// lookupEnumValue returns the allowed enum values for v's type, or nil if v
// is not a registered named string type.
func lookupEnumValue(v interface{}) []string {
	if v == nil {
		return nil
	}
	t := reflect.TypeOf(v)
	if t == nil {
		return nil
	}
	// Named string types have Kind() == String and a non-empty Name().
	// Plain strings have Name() == "" and are skipped.
	if t.Kind() != reflect.String || t.Name() == "" {
		return nil
	}
	return LookupEnum(t)
}

// scanMapEnums walks a body map looking for registered named-typed string
// values and returns a map keyed by field name with their allowed enum
// values. Returns nil if the body is not a map or no fields carry enum types.
func scanMapEnums(body interface{}) map[string][]string {
	if body == nil {
		return nil
	}
	m, ok := body.(map[string]interface{})
	if !ok {
		return nil
	}
	var out map[string][]string
	for k, v := range m {
		if allowed := lookupEnumValue(v); len(allowed) > 0 {
			if out == nil {
				out = make(map[string][]string)
			}
			out[k] = allowed
		}
	}
	return out
}
