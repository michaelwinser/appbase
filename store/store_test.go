package store

import (
	"database/sql"
	"testing"

	appdb "github.com/michaelwinser/appbase/db"
	_ "modernc.org/sqlite"
)

type testItem struct {
	ID     string `store:"id,pk"`
	Owner  string `store:"owner,index"`
	Name   string `store:"name"`
	Count  int    `store:"count"`
	Active bool   `store:"active"`
}

func testDB(t *testing.T) *appdb.DB {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:?_foreign_keys=on")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	return appdb.NewFromSQL(sqlDB)
}

func testCollection(t *testing.T) *Collection[testItem] {
	t.Helper()
	db := testDB(t)
	coll, err := NewCollection[testItem](db, "items")
	if err != nil {
		t.Fatal(err)
	}
	return coll
}

func TestCreateAndGet(t *testing.T) {
	coll := testCollection(t)

	item := &testItem{ID: "1", Owner: "alice", Name: "thing", Count: 5, Active: true}
	if err := coll.Create(item); err != nil {
		t.Fatal(err)
	}

	got, err := coll.Get("1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected item, got nil")
	}
	if got.Name != "thing" {
		t.Fatalf("expected Name=thing, got %s", got.Name)
	}
	if got.Count != 5 {
		t.Fatalf("expected Count=5, got %d", got.Count)
	}
	if !got.Active {
		t.Fatal("expected Active=true")
	}
}

func TestGetNotFound(t *testing.T) {
	coll := testCollection(t)

	got, err := coll.Get("nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for missing item")
	}
}

func TestAll(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "a"})
	coll.Create(&testItem{ID: "2", Owner: "bob", Name: "b"})

	items, err := coll.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestEmptyResults(t *testing.T) {
	coll := testCollection(t)

	items, err := coll.All()
	if err != nil {
		t.Fatal(err)
	}
	if items == nil {
		t.Fatal("expected empty slice, got nil")
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestWhereFilter(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "a"})
	coll.Create(&testItem{ID: "2", Owner: "bob", Name: "b"})
	coll.Create(&testItem{ID: "3", Owner: "alice", Name: "c"})

	items, err := coll.Where("owner", "==", "alice").All()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 alice items, got %d", len(items))
	}
}

func TestMultipleWhere(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "a", Active: true})
	coll.Create(&testItem{ID: "2", Owner: "alice", Name: "b", Active: false})
	coll.Create(&testItem{ID: "3", Owner: "bob", Name: "c", Active: true})

	items, err := coll.Where("owner", "==", "alice").Where("active", "==", 1).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != "1" {
		t.Fatalf("expected ID=1, got %s", items[0].ID)
	}
}

func TestOrderByAsc(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "a", Name: "z"})
	coll.Create(&testItem{ID: "2", Owner: "a", Name: "a"})
	coll.Create(&testItem{ID: "3", Owner: "a", Name: "m"})

	items, err := coll.Where("owner", "==", "a").OrderBy("name", Asc).All()
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Name != "a" || items[1].Name != "m" || items[2].Name != "z" {
		t.Fatalf("unexpected order: %s, %s, %s", items[0].Name, items[1].Name, items[2].Name)
	}
}

func TestOrderByDesc(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "a", Name: "z"})
	coll.Create(&testItem{ID: "2", Owner: "a", Name: "a"})
	coll.Create(&testItem{ID: "3", Owner: "a", Name: "m"})

	items, err := coll.Where("owner", "==", "a").OrderBy("name", Desc).All()
	if err != nil {
		t.Fatal(err)
	}
	if items[0].Name != "z" || items[1].Name != "m" || items[2].Name != "a" {
		t.Fatalf("unexpected order: %s, %s, %s", items[0].Name, items[1].Name, items[2].Name)
	}
}

func TestLimit(t *testing.T) {
	coll := testCollection(t)

	for i := 0; i < 10; i++ {
		coll.Create(&testItem{ID: string(rune('a' + i)), Owner: "a", Name: "item"})
	}

	items, err := coll.Where("owner", "==", "a").Limit(3).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3, got %d", len(items))
	}
}

func TestUpdate(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "old", Count: 1, Active: true})

	updated := &testItem{ID: "1", Owner: "alice", Name: "new", Count: 2, Active: false}
	if err := coll.Update("1", updated); err != nil {
		t.Fatal(err)
	}

	got, _ := coll.Get("1")
	if got.Name != "new" {
		t.Fatalf("expected Name=new, got %s", got.Name)
	}
	if got.Count != 2 {
		t.Fatalf("expected Count=2, got %d", got.Count)
	}
	if got.Active {
		t.Fatal("expected Active=false")
	}
}

func TestDelete(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "a"})
	if err := coll.Delete("1"); err != nil {
		t.Fatal(err)
	}

	got, _ := coll.Get("1")
	if got != nil {
		t.Fatal("expected nil after delete")
	}
}

func TestBoolRoundTrip(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "t", Owner: "a", Name: "a", Active: true})
	coll.Create(&testItem{ID: "f", Owner: "a", Name: "b", Active: false})

	trueItem, _ := coll.Get("t")
	if !trueItem.Active {
		t.Fatal("expected true")
	}

	falseItem, _ := coll.Get("f")
	if falseItem.Active {
		t.Fatal("expected false")
	}
}

func TestFirst(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "first"})
	coll.Create(&testItem{ID: "2", Owner: "alice", Name: "second"})

	got, err := coll.Where("owner", "==", "alice").OrderBy("name", Asc).First()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "first" {
		t.Fatalf("expected first, got %v", got)
	}

	// First on empty result
	got, err = coll.Where("owner", "==", "nobody").First()
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected nil for no results")
	}
}

func TestWhereRejectsUnknownField(t *testing.T) {
	coll := testCollection(t)
	_, err := coll.Where("hacked; DROP TABLE items", "==", "x").All()
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
}

func TestWhereRejectsInvalidOperator(t *testing.T) {
	coll := testCollection(t)
	_, err := coll.Where("owner", "OR 1=1 --", "x").All()
	if err == nil {
		t.Fatal("expected error for invalid operator")
	}
}

func TestOrderByRejectsUnknownField(t *testing.T) {
	coll := testCollection(t)
	_, err := coll.Where("owner", "==", "alice").OrderBy("nonexistent", Asc).All()
	if err == nil {
		t.Fatal("expected error for unknown OrderBy field")
	}
}

func TestInvalidCollectionName(t *testing.T) {
	db := testDB(t)
	_, err := NewCollection[testItem](db, "items; DROP TABLE users")
	if err == nil {
		t.Fatal("expected error for invalid collection name")
	}
}

// testItemV2 has an extra field compared to testItem, simulating a schema evolution.
type testItemV2 struct {
	ID       string `store:"id,pk"`
	Owner    string `store:"owner,index"`
	Name     string `store:"name"`
	Count    int    `store:"count"`
	Active   bool   `store:"active"`
	Priority int    `store:"priority,index"` // new field
	Notes    string `store:"notes"`          // new field
}

func TestAutoMigrateColumns(t *testing.T) {
	db := testDB(t)

	// Create table with the original schema (v1)
	coll1, err := NewCollection[testItem](db, "evolving")
	if err != nil {
		t.Fatal(err)
	}

	// Insert a row with v1 schema
	if err := coll1.Create(&testItem{ID: "1", Owner: "alice", Name: "old", Count: 1, Active: true}); err != nil {
		t.Fatal(err)
	}

	// Now open the same table with v2 schema — should auto-add missing columns
	coll2, err := NewCollection[testItemV2](db, "evolving")
	if err != nil {
		t.Fatal(err)
	}

	// Read the old row — new fields should have defaults
	got, err := coll2.Get("1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected item, got nil")
	}
	if got.Name != "old" {
		t.Fatalf("expected Name=old, got %s", got.Name)
	}
	if got.Priority != 0 {
		t.Fatalf("expected Priority=0 (default), got %d", got.Priority)
	}
	if got.Notes != "" {
		t.Fatalf("expected Notes='' (default), got %q", got.Notes)
	}

	// Insert a new row with the new fields populated
	if err := coll2.Create(&testItemV2{ID: "2", Owner: "bob", Name: "new", Priority: 5, Notes: "hi"}); err != nil {
		t.Fatal(err)
	}

	got2, _ := coll2.Get("2")
	if got2.Priority != 5 {
		t.Fatalf("expected Priority=5, got %d", got2.Priority)
	}
	if got2.Notes != "hi" {
		t.Fatalf("expected Notes=hi, got %q", got2.Notes)
	}

	// Query by the new indexed column
	items, err := coll2.Where("priority", "==", 5).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "2" {
		t.Fatalf("expected 1 item with ID=2, got %d items", len(items))
	}
}

func TestReadOnlyCollection(t *testing.T) {
	coll := testCollection(t)

	// Populate via the full collection
	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "first", Count: 1, Active: true})
	coll.Create(&testItem{ID: "2", Owner: "alice", Name: "second", Count: 2, Active: false})
	coll.Create(&testItem{ID: "3", Owner: "bob", Name: "third", Count: 3, Active: true})

	ro := coll.ReadOnly()

	// Get works
	got, err := ro.Get("1")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "first" {
		t.Fatalf("expected first, got %v", got)
	}

	// All works
	all, err := ro.All()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 items, got %d", len(all))
	}

	// Where + OrderBy + All works
	items, err := ro.Where("owner", "==", "alice").OrderBy("name", Asc).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 alice items, got %d", len(items))
	}
	if items[0].Name != "first" || items[1].Name != "second" {
		t.Fatalf("unexpected order: %s, %s", items[0].Name, items[1].Name)
	}

	// First works
	first, err := ro.Where("owner", "==", "bob").First()
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || first.Name != "third" {
		t.Fatalf("expected third, got %v", first)
	}

	// Limit works
	limited, err := ro.Where("owner", "==", "alice").Limit(1).All()
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 1 {
		t.Fatalf("expected 1, got %d", len(limited))
	}

	// Compile-time check: ReadOnlyCollection does NOT have Create, Update, Delete.
	// These lines should NOT compile if uncommented:
	// ro.Create(&testItem{})   // should not compile
	// ro.Update("1", &testItem{})  // should not compile
	// ro.Delete("1")           // should not compile
}

func TestQueryImmutable(t *testing.T) {
	coll := testCollection(t)

	coll.Create(&testItem{ID: "1", Owner: "alice", Name: "a"})
	coll.Create(&testItem{ID: "2", Owner: "bob", Name: "b"})

	base := coll.Where("owner", "==", "alice")
	withOrder := base.OrderBy("name", Asc)

	// Original query should still work without the OrderBy
	items1, _ := base.All()
	items2, _ := withOrder.All()

	if len(items1) != 1 || len(items2) != 1 {
		t.Fatalf("expected 1 and 1, got %d and %d", len(items1), len(items2))
	}
}
