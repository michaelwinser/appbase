package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/michaelwinser/appbase/deploy"
)

// calendarSyncSandbox is the actual drifted script from calendar-sync
// at the time issue #48 was filed — the concrete migration target.
const calendarSyncSandbox = `#!/bin/sh
set -e

PROJECT_DIR="$(cd "$(dirname "$0")" && pwd)"

if ! command -v nono >/dev/null 2>&1; then
    echo "nono not found — running unsandboxed"
    exec "$@"
fi

for p in /opt/homebrew/bin /usr/local/bin /usr/local/go/bin "$HOME/go/bin"; do
    case ":$PATH:" in
        *":$p:"*) ;;
        *) [ -d "$p" ] && export PATH="$p:$PATH" ;;
    esac
done

GO_BIN_DIR=""
if command -v go >/dev/null 2>&1; then
    GO_BIN_DIR="$(dirname "$(command -v go)")"
fi

if command -v go >/dev/null 2>&1 && [ -z "$GOROOT" ]; then
    export GOROOT="$(go env GOROOT)"
fi

EXEC_FLAG=""
case "$1" in
    claude|bash|zsh|sh|vim|nvim)
        EXEC_FLAG="--exec"
        ;;
esac

GO_FLAG=""
if [ -n "$GO_BIN_DIR" ] && [ "$GO_BIN_DIR" != "/usr/bin" ]; then
    GO_FLAG="--read $GO_BIN_DIR"
fi

DOCKER_DIR="$PROJECT_DIR/.docker"
mkdir -p "$DOCKER_DIR" 2>/dev/null
[ -f "$DOCKER_DIR/config.json" ] || echo '{}' > "$DOCKER_DIR/config.json"
export DOCKER_CONFIG="$DOCKER_DIR"

exec nono run \
    --profile claude-code \
    --allow "$PROJECT_DIR" \
    --allow "$HOME/.config/calendar-sync" \
    --allow "$HOME/go" \
    --allow-bind 4004 \
    $GO_FLAG \
    $EXEC_FLAG \
    -- "$@"
`

func TestExtractSandboxFlags_CalendarSync(t *testing.T) {
	got := extractSandboxFlags(calendarSyncSandbox)

	want := []string{
		`APP_FLAGS="$APP_FLAGS --allow "$HOME/.config/calendar-sync""`,
		`APP_FLAGS="$APP_FLAGS --allow "$HOME/go""`,
		`APP_FLAGS="$APP_FLAGS --allow-bind 4004"`,
	}

	if !slices.Equal(got, want) {
		t.Errorf("extracted flags mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExtractSandboxFlags_SkipsStandard(t *testing.T) {
	// $PROJECT_DIR and $GO_BIN_DIR are template boilerplate, not project flags
	got := extractSandboxFlags(calendarSyncSandbox)
	for _, f := range got {
		if strings.Contains(f, "$PROJECT_DIR") {
			t.Errorf("should not extract $PROJECT_DIR flag: %q", f)
		}
		if strings.Contains(f, "$GO_BIN_DIR") {
			t.Errorf("should not extract $GO_BIN_DIR flag: %q", f)
		}
	}
}

func TestExtractSandboxFlags_PreservesAppFlagsBlock(t *testing.T) {
	// Newer-style scripts that already use APP_FLAGS — keep lines verbatim
	script := `#!/bin/sh
APP_FLAGS=""
APP_FLAGS="$APP_FLAGS --allow $HOME/.config/myapp"
APP_FLAGS="$APP_FLAGS --allow-bind 3000"
# APP_FLAGS="$APP_FLAGS --read /commented/out"

exec nono run --allow "$PROJECT_DIR" $APP_FLAGS -- "$@"
`
	got := extractSandboxFlags(script)
	want := []string{
		`APP_FLAGS="$APP_FLAGS --allow $HOME/.config/myapp"`,
		`APP_FLAGS="$APP_FLAGS --allow-bind 3000"`,
	}
	if !slices.Equal(got, want) {
		t.Errorf("APP_FLAGS preservation mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestExtractSandboxFlags_NoCustomFlags(t *testing.T) {
	// Pure boilerplate — should extract nothing
	script := `#!/bin/sh
APP_FLAGS=""
exec nono run --profile claude-code --allow "$PROJECT_DIR" $GO_FLAG $APP_FLAGS -- "$@"
`
	got := extractSandboxFlags(script)
	if len(got) != 0 {
		t.Errorf("expected no flags, got %#v", got)
	}
}

func TestUpdateSandbox_FullMigration(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	// go.mod gates the update command but isn't read by updateSandbox
	os.WriteFile("go.mod", []byte("module test\n"), 0644)
	os.WriteFile("sandbox", []byte(calendarSyncSandbox), 0755)

	result := updateSandbox(false)

	if !strings.Contains(result, "updated") {
		t.Errorf("expected update, got: %q", result)
	}
	if !strings.Contains(result, "migrated 3 flags") {
		t.Errorf("expected 3 migrated flags in result: %q", result)
	}

	// ./sandbox is now byte-identical to the template
	got, _ := os.ReadFile("sandbox")
	if string(got) != deploy.SandboxTemplate {
		t.Error("./sandbox does not match embedded template after update")
	}

	// sandbox.flags exists with the project flags
	flags, err := os.ReadFile("sandbox.flags")
	if err != nil {
		t.Fatalf("sandbox.flags not created: %v", err)
	}
	for _, want := range []string{".config/calendar-sync", "$HOME/go", "--allow-bind 4004"} {
		if !strings.Contains(string(flags), want) {
			t.Errorf("sandbox.flags missing %q:\n%s", want, flags)
		}
	}
}

func TestUpdateSandbox_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	os.WriteFile("sandbox", []byte(deploy.SandboxTemplate), 0755)
	os.WriteFile("sandbox.flags", []byte("APP_FLAGS=\"$APP_FLAGS --allow-bind 9999\"\n"), 0644)

	result := updateSandbox(false)
	if result != "" {
		t.Errorf("expected no-op on current template, got: %q", result)
	}

	// sandbox.flags untouched
	flags, _ := os.ReadFile("sandbox.flags")
	if !strings.Contains(string(flags), "9999") {
		t.Error("sandbox.flags was modified on idempotent run")
	}
}

func TestUpdateSandbox_NeverOverwritesFlags(t *testing.T) {
	// Even when sandbox is stale, an existing sandbox.flags must survive
	dir := t.TempDir()
	t.Chdir(dir)

	os.WriteFile("sandbox", []byte("#!/bin/sh\n# stale\n"), 0755)
	custom := "# my carefully tuned flags\nAPP_FLAGS=\"$APP_FLAGS --allow /precious\"\n"
	os.WriteFile("sandbox.flags", []byte(custom), 0644)

	updateSandbox(false)

	got, _ := os.ReadFile("sandbox.flags")
	if string(got) != custom {
		t.Errorf("sandbox.flags was overwritten:\n got: %q\nwant: %q", got, custom)
	}
}

func TestUpdateSandbox_DryRun(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	os.WriteFile("sandbox", []byte(calendarSyncSandbox), 0755)

	updateSandbox(true)

	// Nothing written
	got, _ := os.ReadFile("sandbox")
	if string(got) != calendarSyncSandbox {
		t.Error("dry-run modified ./sandbox")
	}
	if _, err := os.Stat("sandbox.flags"); err == nil {
		t.Error("dry-run created sandbox.flags")
	}
}

func TestSandboxTemplate_SourcesFlags(t *testing.T) {
	// The embedded template must source sandbox.flags or migration is pointless
	if !strings.Contains(deploy.SandboxTemplate, "sandbox.flags") {
		t.Error("template does not reference sandbox.flags")
	}
	if !strings.Contains(deploy.SandboxTemplate, `. "$PROJECT_DIR/sandbox.flags"`) {
		t.Error("template does not source sandbox.flags")
	}
}

func TestSandboxTemplate_ShellSyntax(t *testing.T) {
	// Verify the template is syntactically valid POSIX sh
	tmp := filepath.Join(t.TempDir(), "sandbox")
	os.WriteFile(tmp, []byte(deploy.SandboxTemplate), 0755)

	out, err := runShellCheck(tmp)
	if err != nil {
		t.Fatalf("sh -n failed: %v\n%s", err, out)
	}
}

func TestMigratedFlags_ShellSyntax(t *testing.T) {
	// The flags we extract must be valid when sourced
	flags := extractSandboxFlags(calendarSyncSandbox)
	content := strings.Join(flags, "\n") + "\n"

	tmp := filepath.Join(t.TempDir(), "sandbox.flags")
	os.WriteFile(tmp, []byte(content), 0644)

	out, err := runShellCheck(tmp)
	if err != nil {
		t.Fatalf("migrated sandbox.flags is not valid sh: %v\n%s\ncontent:\n%s", err, out, content)
	}
}

func runShellCheck(path string) ([]byte, error) {
	return exec.Command("sh", "-n", path).CombinedOutput()
}
