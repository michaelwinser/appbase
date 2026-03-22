package store

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"time"

	appdb "github.com/michaelwinser/appbase/db"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type firestoreBackend[T any] struct {
	db   *appdb.DB
	name string
	meta *structMeta
}

func (b *firestoreBackend[T]) init() error {
	return nil // schemaless
}

func (b *firestoreBackend[T]) get(id string) (*T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	doc, err := b.db.Firestore().Collection(b.name).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("store.Get: %w", err)
	}

	entity := fromMap[T](b.meta, doc.Data(), doc.Ref.ID)
	return &entity, nil
}

func (b *firestoreBackend[T]) create(entity *T) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	id := getPK(b.meta, entity)
	data := toMap(b.meta, entity, true) // skip PK — it's the doc ID
	_, err := b.db.Firestore().Collection(b.name).Doc(id).Set(ctx, data)
	if err != nil {
		return fmt.Errorf("store.Create: %w", err)
	}
	return nil
}

func (b *firestoreBackend[T]) update(id string, entity *T) error {
	// Firestore Set is an upsert
	return b.create(entity)
}

func (b *firestoreBackend[T]) delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := b.db.Firestore().Collection(b.name).Doc(id).Delete(ctx)
	return err
}

func (b *firestoreBackend[T]) query(wheres []whereClause, orderBy *orderByClause, limit int) ([]T, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	q := b.db.Firestore().Collection(b.name).Query

	// Apply only the first Where to Firestore (single-field, no composite index needed).
	// Remaining Wheres are filtered in memory.
	var memoryWheres []whereClause
	if len(wheres) > 0 {
		q = q.Where(wheres[0].Field, wheres[0].Op, wheres[0].Value)
		memoryWheres = wheres[1:]
	}

	iter := q.Documents(ctx)
	defer iter.Stop()

	var results []T
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("store.Query: %w", err)
		}

		entity := fromMap[T](b.meta, doc.Data(), doc.Ref.ID)

		// Apply remaining filters in memory
		if !matchesWheres(b.meta, &entity, memoryWheres) {
			continue
		}

		results = append(results, entity)
	}

	// Sort in memory (avoids composite index requirement)
	if orderBy != nil {
		sortResults(b.meta, results, orderBy)
	}

	// Apply limit
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	if results == nil {
		results = []T{}
	}
	return results, nil
}

// matchesWheres checks if an entity matches all where clauses.
func matchesWheres[T any](meta *structMeta, entity *T, wheres []whereClause) bool {
	v := reflect.ValueOf(entity).Elem()
	for _, w := range wheres {
		fi := findField(meta, w.Field)
		if fi == nil {
			return false
		}
		fieldVal := v.Field(fi.FieldIdx)

		switch w.Op {
		case "==":
			if compareValues(fieldVal, w.Value) != 0 {
				return false
			}
		case "!=":
			if compareValues(fieldVal, w.Value) == 0 {
				return false
			}
		case "<":
			if compareValues(fieldVal, w.Value) >= 0 {
				return false
			}
		case "<=":
			if compareValues(fieldVal, w.Value) > 0 {
				return false
			}
		case ">":
			if compareValues(fieldVal, w.Value) <= 0 {
				return false
			}
		case ">=":
			if compareValues(fieldVal, w.Value) < 0 {
				return false
			}
		}
	}
	return true
}

// compareValues compares a reflect.Value against an interface{} value.
// Returns -1, 0, or 1 like strings.Compare.
// Falls back to string comparison for unsupported types.
func compareValues(fieldVal reflect.Value, want interface{}) int {
	switch fieldVal.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		a := fieldVal.Int()
		var b int64
		switch v := want.(type) {
		case int:
			b = int64(v)
		case int64:
			b = v
		case float64:
			b = int64(v)
		default:
			// Fall through to string comparison
			goto stringCompare
		}
		if a < b {
			return -1
		} else if a > b {
			return 1
		}
		return 0

	case reflect.Float32, reflect.Float64:
		a := fieldVal.Float()
		var b float64
		switch v := want.(type) {
		case float64:
			b = v
		case float32:
			b = float64(v)
		case int:
			b = float64(v)
		case int64:
			b = float64(v)
		default:
			goto stringCompare
		}
		if a < b {
			return -1
		} else if a > b {
			return 1
		}
		return 0

	case reflect.Bool:
		a := fieldVal.Bool()
		var b bool
		switch v := want.(type) {
		case bool:
			b = v
		default:
			goto stringCompare
		}
		if a == b {
			return 0
		}
		if !a && b {
			return -1
		}
		return 1
	}

stringCompare:
	a := fmt.Sprintf("%v", fieldVal.Interface())
	b := fmt.Sprintf("%v", want)
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}

// sortResults sorts a slice of T by a field.
func sortResults[T any](meta *structMeta, results []T, ob *orderByClause) {
	fi := findField(meta, ob.Field)
	if fi == nil {
		return
	}
	sort.Slice(results, func(i, j int) bool {
		vi := reflect.ValueOf(&results[i]).Elem().Field(fi.FieldIdx)
		vj := reflect.ValueOf(&results[j]).Elem().Field(fi.FieldIdx)
		si := fmt.Sprintf("%v", vi.Interface())
		sj := fmt.Sprintf("%v", vj.Interface())
		if ob.Dir == Desc {
			return si > sj
		}
		return si < sj
	})
}

// findField looks up a fieldInfo by column name.
func findField(meta *structMeta, column string) *fieldInfo {
	for i := range meta.Fields {
		if meta.Fields[i].Column == column {
			return &meta.Fields[i]
		}
	}
	return nil
}
