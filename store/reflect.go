package store

import (
	"fmt"
	"log"
	"reflect"
	"strings"
	"sync"
)

// fieldInfo describes a single persisted field.
type fieldInfo struct {
	GoName    string       // Go struct field name
	Column    string       // Storage column / Firestore field name
	GoType    reflect.Type // Go type
	IsPK      bool         // Primary key (document ID in Firestore)
	HasIndex  bool         // Create an index on this column
	FieldIdx  int          // Index in the struct for reflect access
}

// structMeta holds parsed metadata for a struct type.
type structMeta struct {
	Fields []fieldInfo
	PK     *fieldInfo // Pointer to the PK field in Fields
}

var metaCache sync.Map // map[reflect.Type]*structMeta

// parseStruct extracts store metadata from struct tags on T.
// Results are cached per type.
func parseStruct[T any]() (*structMeta, error) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	if cached, ok := metaCache.Load(t); ok {
		return cached.(*structMeta), nil
	}

	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("store: %s is not a struct", t.Name())
	}

	meta := &structMeta{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("store")
		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")
		colName := parts[0]
		if !validIdentifier.MatchString(colName) {
			return nil, fmt.Errorf("store: field %s has invalid column name %q (must be alphanumeric/underscore)", f.Name, colName)
		}
		fi := fieldInfo{
			GoName:   f.Name,
			Column:   colName,
			GoType:   f.Type,
			FieldIdx: i,
		}
		for _, opt := range parts[1:] {
			switch opt {
			case "pk":
				fi.IsPK = true
			case "index":
				fi.HasIndex = true
			}
		}
		meta.Fields = append(meta.Fields, fi)
	}

	if len(meta.Fields) == 0 {
		return nil, fmt.Errorf("store: %s has no store-tagged fields", t.Name())
	}

	// Find PK
	for i := range meta.Fields {
		if meta.Fields[i].IsPK {
			meta.PK = &meta.Fields[i]
			break
		}
	}
	if meta.PK == nil {
		return nil, fmt.Errorf("store: %s has no primary key (add ,pk to a store tag)", t.Name())
	}

	metaCache.Store(t, meta)
	return meta, nil
}

// goTypeToSQL maps a Go type to a SQL column type.
func goTypeToSQL(t reflect.Type) string {
	switch t.Kind() {
	case reflect.String:
		return "TEXT"
	case reflect.Bool:
		return "INTEGER"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "INTEGER"
	case reflect.Float32, reflect.Float64:
		return "REAL"
	default:
		// time.Time and other types stored as TEXT
		if t.String() == "time.Time" {
			return "TEXT"
		}
		return "TEXT"
	}
}

// getPK extracts the primary key value from an entity.
func getPK[T any](meta *structMeta, entity *T) string {
	v := reflect.ValueOf(entity).Elem()
	return fmt.Sprintf("%v", v.Field(meta.PK.FieldIdx).Interface())
}

// toMap converts an entity to a map[string]any using store tags.
// If skipPK is true, the primary key field is omitted (for Firestore documents
// where the PK is the document ID, not a field).
func toMap[T any](meta *structMeta, entity *T, skipPK bool) map[string]any {
	v := reflect.ValueOf(entity).Elem()
	m := make(map[string]any, len(meta.Fields))
	for _, fi := range meta.Fields {
		if skipPK && fi.IsPK {
			continue
		}
		val := v.Field(fi.FieldIdx).Interface()
		m[fi.Column] = val
	}
	return m
}

// fromMap populates an entity from a map[string]any using store tags.
func fromMap[T any](meta *structMeta, m map[string]any, id string) T {
	var entity T
	v := reflect.ValueOf(&entity).Elem()

	for _, fi := range meta.Fields {
		if fi.IsPK {
			v.Field(fi.FieldIdx).SetString(id)
			continue
		}
		val, ok := m[fi.Column]
		if !ok {
			continue
		}
		setField(v.Field(fi.FieldIdx), val)
	}
	return entity
}

// setField sets a reflect.Value from an interface{} value, handling type conversions.
func setField(field reflect.Value, val interface{}) {
	if val == nil {
		return
	}
	rv := reflect.ValueOf(val)

	// Direct assignability
	if rv.Type().AssignableTo(field.Type()) {
		field.Set(rv)
		return
	}

	// Common conversions
	switch field.Kind() {
	case reflect.String:
		field.SetString(fmt.Sprintf("%v", val))
	case reflect.Bool:
		switch v := val.(type) {
		case bool:
			field.SetBool(v)
		case int64:
			field.SetBool(v != 0)
		case float64:
			field.SetBool(v != 0)
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		switch v := val.(type) {
		case int64:
			field.SetInt(v)
		case int:
			field.SetInt(int64(v))
		case float64:
			field.SetInt(int64(v))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		switch v := val.(type) {
		case int64:
			field.SetUint(uint64(v))
		case uint64:
			field.SetUint(v)
		case float64:
			field.SetUint(uint64(v))
		}
	case reflect.Float32, reflect.Float64:
		switch v := val.(type) {
		case float64:
			field.SetFloat(v)
		case float32:
			field.SetFloat(float64(v))
		case int64:
			field.SetFloat(float64(v))
		}
	default:
		// Unsupported type — log a warning. This covers struct types (like time.Time)
		// that didn't match via direct assignability above.
		log.Printf("store: cannot set field of type %s from %T value", field.Type(), val)
	}
}
