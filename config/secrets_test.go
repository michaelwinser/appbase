package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnvFileResolver_BasicKeyValue(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("MY_KEY=my_value\nOTHER=123\n"), 0644)

	r := &EnvFileResolver{Path: path}
	val, err := r.Get("test", "MY_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "my_value" {
		t.Errorf("got %q, want %q", val, "my_value")
	}
}

func TestEnvFileResolver_QuotedValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte(`
DOUBLE="hello world"
SINGLE='single quoted'
UNQUOTED=no quotes
`), 0644)

	r := &EnvFileResolver{Path: path}

	val, _ := r.Get("test", "DOUBLE")
	if val != "hello world" {
		t.Errorf("double quoted: got %q, want %q", val, "hello world")
	}

	val, _ = r.Get("test", "SINGLE")
	if val != "single quoted" {
		t.Errorf("single quoted: got %q, want %q", val, "single quoted")
	}

	val, _ = r.Get("test", "UNQUOTED")
	if val != "no quotes" {
		t.Errorf("unquoted: got %q, want %q", val, "no quotes")
	}
}

func TestEnvFileResolver_ExportPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("export MY_KEY=exported_value\n"), 0644)

	r := &EnvFileResolver{Path: path}
	val, err := r.Get("test", "MY_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "exported_value" {
		t.Errorf("got %q, want %q", val, "exported_value")
	}
}

func TestEnvFileResolver_Comments(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("# comment\nKEY=val\n# another\n"), 0644)

	r := &EnvFileResolver{Path: path}
	val, _ := r.Get("test", "KEY")
	if val != "val" {
		t.Errorf("got %q, want %q", val, "val")
	}
}

func TestEnvFileResolver_NotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("OTHER=val\n"), 0644)

	r := &EnvFileResolver{Path: path}
	_, err := r.Get("test", "MISSING")
	if err == nil {
		t.Error("expected error for missing key")
	}
}

func TestEnvFileResolver_MissingFile(t *testing.T) {
	r := &EnvFileResolver{Path: "/nonexistent/.env"}
	_, err := r.Get("test", "KEY")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestEnvFileResolver_List(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	os.WriteFile(path, []byte("A=1\nB=2\nC=3\n"), 0644)

	r := &EnvFileResolver{Path: path}
	names, err := r.List("test")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 3 {
		t.Errorf("got %d names, want 3", len(names))
	}
}

func TestChainResolver_FirstHitWins(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "first.env")
	path2 := filepath.Join(dir, "second.env")
	os.WriteFile(path1, []byte("KEY=first\n"), 0644)
	os.WriteFile(path2, []byte("KEY=second\n"), 0644)

	chain := NewChainResolver(
		&EnvFileResolver{Path: path1},
		&EnvFileResolver{Path: path2},
	)

	val, err := chain.Get("test", "KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "first" {
		t.Errorf("got %q, want %q (first resolver should win)", val, "first")
	}
}

func TestChainResolver_FallsThrough(t *testing.T) {
	dir := t.TempDir()
	path1 := filepath.Join(dir, "first.env")
	path2 := filepath.Join(dir, "second.env")
	os.WriteFile(path1, []byte("OTHER=nope\n"), 0644)
	os.WriteFile(path2, []byte("KEY=found\n"), 0644)

	chain := NewChainResolver(
		&EnvFileResolver{Path: path1},
		&EnvFileResolver{Path: path2},
	)

	val, err := chain.Get("test", "KEY")
	if err != nil {
		t.Fatal(err)
	}
	if val != "found" {
		t.Errorf("got %q, want %q (should fall through to second resolver)", val, "found")
	}
}

func TestChainResolver_AllMiss(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.env")
	os.WriteFile(path, []byte(""), 0644)

	chain := NewChainResolver(&EnvFileResolver{Path: path})

	_, err := chain.Get("test", "MISSING")
	if err == nil {
		t.Error("expected error when all resolvers miss")
	}
}
