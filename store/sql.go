package store

import (
	"database/sql"
	"fmt"
	"log"
	"reflect"
	"strings"

	appdb "github.com/michaelwinser/appbase/db"
)

type sqlBackend[T any] struct {
	db   *appdb.DB
	name string
	meta *structMeta
}

func (b *sqlBackend[T]) init() error {
	if err := b.db.Migrate(b.generateCreateTable()); err != nil {
		return err
	}
	if err := b.migrateColumns(); err != nil {
		return err
	}
	// Create indexes after columns are migrated (new columns may need indexes).
	return b.createIndexes()
}

// migrateColumns adds any columns defined in the struct but missing from the table.
// Handles the common case of adding new fields to an entity struct.
func (b *sqlBackend[T]) migrateColumns() error {
	rows, err := b.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", b.name))
	if err != nil {
		return nil // non-SQLite or PRAGMA not supported — skip
	}
	defer rows.Close()

	existing := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt *string
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			continue
		}
		existing[name] = true
	}

	for _, fi := range b.meta.Fields {
		if existing[fi.Column] {
			continue
		}
		sqlType := goTypeToSQL(fi.GoType)
		dflt := defaultForSQL(sqlType)
		stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s NOT NULL DEFAULT %s",
			b.name, fi.Column, sqlType, dflt)
		if _, err := b.db.Exec(stmt); err != nil {
			return fmt.Errorf("auto-migrate %s.%s: %w", b.name, fi.Column, err)
		}
		log.Printf("store: %s — added column %s %s", b.name, fi.Column, sqlType)

		// Create index if tagged
		if fi.HasIndex {
			idxStmt := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s)",
				b.name, fi.Column, b.name, fi.Column)
			b.db.Exec(idxStmt) // best-effort
		}
	}
	return nil
}

func (b *sqlBackend[T]) generateCreateTable() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", b.name))
	for i, fi := range b.meta.Fields {
		sqlType := goTypeToSQL(fi.GoType)
		sb.WriteString(fmt.Sprintf("  %s %s", fi.Column, sqlType))
		if fi.IsPK {
			sb.WriteString(" PRIMARY KEY")
		} else {
			sb.WriteString(" NOT NULL")
		}
		if i < len(b.meta.Fields)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString(");\n")
	return sb.String()
}

func (b *sqlBackend[T]) createIndexes() error {
	for _, fi := range b.meta.Fields {
		if fi.HasIndex {
			stmt := fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_%s ON %s(%s)",
				b.name, fi.Column, b.name, fi.Column)
			if _, err := b.db.Exec(stmt); err != nil {
				return fmt.Errorf("creating index on %s.%s: %w", b.name, fi.Column, err)
			}
		}
	}
	return nil
}

func (b *sqlBackend[T]) get(id string) (*T, error) {
	cols := b.columnList()
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ?", cols, b.name, b.meta.PK.Column)

	rows, err := b.db.Query(query, id)
	if err != nil {
		return nil, fmt.Errorf("store.Get: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil
	}
	entity, err := b.scanCurrentRow(rows)
	if err != nil {
		return nil, fmt.Errorf("store.Get: %w", err)
	}
	return entity, nil
}

func (b *sqlBackend[T]) create(entity *T) error {
	cols, placeholders, vals := b.insertArgs(entity)
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", b.name, cols, placeholders)
	_, err := b.db.Exec(query, vals...)
	if err != nil {
		return fmt.Errorf("store.Create: %w", err)
	}
	return nil
}

func (b *sqlBackend[T]) update(id string, entity *T) error {
	var sets []string
	var vals []interface{}
	for _, fi := range b.meta.Fields {
		if fi.IsPK {
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = ?", fi.Column))
		vals = append(vals, b.fieldToSQL(entity, fi))
	}
	vals = append(vals, id)
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?",
		b.name, strings.Join(sets, ", "), b.meta.PK.Column)
	_, err := b.db.Exec(query, vals...)
	if err != nil {
		return fmt.Errorf("store.Update: %w", err)
	}
	return nil
}

func (b *sqlBackend[T]) delete(id string) error {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", b.name, b.meta.PK.Column)
	_, err := b.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("store.Delete: %w", err)
	}
	return nil
}

func (b *sqlBackend[T]) query(wheres []whereClause, orderBy *orderByClause, limit int) ([]T, error) {
	cols := b.columnList()
	query := fmt.Sprintf("SELECT %s FROM %s", cols, b.name)

	var args []interface{}
	if len(wheres) > 0 {
		var conditions []string
		for _, w := range wheres {
			op := w.Op
			if op == "==" {
				op = "="
			}
			conditions = append(conditions, fmt.Sprintf("%s %s ?", w.Field, op))
			args = append(args, w.Value)
		}
		query += " WHERE " + strings.Join(conditions, " AND ")
	}

	if orderBy != nil {
		dir := "ASC"
		if orderBy.Dir == Desc {
			dir = "DESC"
		}
		query += fmt.Sprintf(" ORDER BY %s %s", orderBy.Field, dir)
	}

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := b.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("store.Query: %w", err)
	}
	defer rows.Close()

	var results []T
	for rows.Next() {
		entity, err := b.scanCurrentRow(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, *entity)
	}
	if results == nil {
		results = []T{}
	}
	return results, rows.Err()
}

// --- helpers ---

func (b *sqlBackend[T]) columnList() string {
	var cols []string
	for _, fi := range b.meta.Fields {
		cols = append(cols, fi.Column)
	}
	return strings.Join(cols, ", ")
}

func (b *sqlBackend[T]) insertArgs(entity *T) (cols, placeholders string, vals []interface{}) {
	var colList, phList []string
	for _, fi := range b.meta.Fields {
		colList = append(colList, fi.Column)
		phList = append(phList, "?")
		vals = append(vals, b.fieldToSQL(entity, fi))
	}
	return strings.Join(colList, ", "), strings.Join(phList, ", "), vals
}

// fieldToSQL converts a Go field value to its SQL representation.
func (b *sqlBackend[T]) fieldToSQL(entity *T, fi fieldInfo) interface{} {
	v := reflect.ValueOf(entity).Elem().Field(fi.FieldIdx)
	if fi.GoType.Kind() == reflect.Bool {
		if v.Bool() {
			return 1
		}
		return 0
	}
	return v.Interface()
}

// scanCurrentRow scans the current row into a new T.
// Uses intermediate []interface{} scan targets to handle type conversions
// (e.g., SQLite stores bools as INTEGER).
func (b *sqlBackend[T]) scanCurrentRow(rows *sql.Rows) (*T, error) {
	// Create scan targets — one per column
	targets := make([]interface{}, len(b.meta.Fields))
	for i, fi := range b.meta.Fields {
		switch fi.GoType.Kind() {
		case reflect.Bool:
			targets[i] = new(int)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			targets[i] = new(int64)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			targets[i] = new(int64)
		case reflect.Float32, reflect.Float64:
			targets[i] = new(float64)
		default:
			targets[i] = new(string)
		}
	}

	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}

	// Populate the entity struct
	var entity T
	v := reflect.ValueOf(&entity).Elem()
	for i, fi := range b.meta.Fields {
		field := v.Field(fi.FieldIdx)
		switch fi.GoType.Kind() {
		case reflect.Bool:
			field.SetBool(*targets[i].(*int) != 0)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			field.SetInt(*targets[i].(*int64))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			field.SetUint(uint64(*targets[i].(*int64)))
		case reflect.Float32, reflect.Float64:
			field.SetFloat(*targets[i].(*float64))
		default:
			field.SetString(*targets[i].(*string))
		}
	}

	return &entity, nil
}
