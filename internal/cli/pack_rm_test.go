package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/thomas0124/ralph/internal/scaffold"
)

// TestRemovePack_DeletesFootprintAndManifestAndEmptyDirs is AC-1 coverage for
// `ralph pack rm <lang>`: after removing the only installed pack, its payload
// directory, its rule control file, and every namespaced manifest entry must
// be gone, Meta.Packs must be empty, and the then-empty scaffolding parents
// (packs/packs-languages/.claude-rules-ralph) must have been tidied away.
func TestRemovePack_DeletesFootprintAndManifestAndEmptyDirs(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	lang := "golang"
	if err := addPack(dir, lang); err != nil {
		t.Fatalf("addPack(%q): %v", lang, err)
	}

	removed, err := removePack(dir, lang, false)
	if err != nil {
		t.Fatalf("removePack(%q): %v", lang, err)
	}
	if removed == 0 {
		t.Error("removePack returned 0 manifest entries removed")
	}

	// Payload directory and rule file gone.
	for _, p := range []string{
		filepath.Join("packs", "languages", lang, "verify.sh"),
		filepath.Join("packs", "languages", lang, "README.md"),
		filepath.Join(".claude", "rules", "ralph", lang+".md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("%s must be deleted after rm; stat err = %v", p, err)
		}
	}

	// Empty scaffolding parents tidied away.
	for _, p := range []string{
		filepath.Join("packs", "languages"),
		"packs",
		filepath.Join(".claude", "rules", "ralph"),
		filepath.Join(".claude", "rules"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("empty parent %s must be removed after rm; stat err = %v", p, err)
		}
	}

	// .claude itself survives (it still holds settings.json at minimum).
	if _, err := os.Stat(filepath.Join(dir, ".claude")); err != nil {
		t.Errorf(".claude must survive removal; stat err = %v", err)
	}

	m, err := scaffold.ReadManifest(filepath.Join(dir, ".ralph", "manifest.toml"))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	for key := range m.Files {
		if key == filepath.Join("packs", "languages", lang, "verify.sh") ||
			key == filepath.Join("packs", "languages", lang, "README.md") ||
			key == filepath.Join(".claude", "rules", "ralph", lang+".md") {
			t.Errorf("manifest still tracks removed pack key %q", key)
		}
	}
	if len(m.Meta.Packs) != 0 {
		t.Errorf("Meta.Packs = %v, want empty", m.Meta.Packs)
	}
}

// TestRemovePack_LeavesSiblingPacksIntact verifies that removing one pack
// never touches a sibling pack's files or manifest entries.
func TestRemovePack_LeavesSiblingPacksIntact(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	for _, lang := range []string{"golang", "typescript"} {
		if err := addPack(dir, lang); err != nil {
			t.Fatalf("addPack(%q): %v", lang, err)
		}
	}

	if _, err := removePack(dir, "golang", false); err != nil {
		t.Fatalf("removePack(golang): %v", err)
	}

	// Sibling pack must be untouched on disk.
	for _, p := range []string{
		filepath.Join("packs", "languages", "typescript", "verify.sh"),
		filepath.Join("packs", "languages", "typescript", "README.md"),
		filepath.Join(".claude", "rules", "ralph", "typescript.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("sibling pack file %s must survive; stat err = %v", p, err)
		}
	}

	m, err := scaffold.ReadManifest(filepath.Join(dir, ".ralph", "manifest.toml"))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if _, ok := m.Files[filepath.Join("packs", "languages", "typescript", "verify.sh")]; !ok {
		t.Error("manifest lost sibling pack entry packs/languages/typescript/verify.sh")
	}
	if len(m.Meta.Packs) != 1 || m.Meta.Packs[0] != "typescript" {
		t.Errorf("Meta.Packs = %v, want [typescript]", m.Meta.Packs)
	}
}

// TestRemovePack_NotInstalled_RefusesZeroWrites verifies that removing a pack
// that is not listed in Meta.Packs fails fail-closed (no manifest write, no
// directory created).
func TestRemovePack_NotInstalled_RefusesZeroWrites(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	manifestPath := filepath.Join(dir, ".ralph", "manifest.toml")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest before: %v", err)
	}

	_, err = removePack(dir, "golang", false)
	if err == nil {
		t.Fatal("removePack of a pack that is not installed: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("err = %v, want a 'not installed' message", err)
	}

	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest after: %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("removePack must not touch the manifest when the pack is not installed\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "packs")); !os.IsNotExist(statErr) {
		t.Errorf("removePack must not create pack dirs; stat err = %v", statErr)
	}
}

// TestRemovePack_UnknownLang_Refuses verifies that a valid-but-uninstalled
// language name is rejected the same as any other absent pack.
func TestRemovePack_UnknownLang_Refuses(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	if err := addPack(dir, "golang"); err != nil {
		t.Fatalf("addPack: %v", err)
	}

	if _, err := removePack(dir, "nonexistent-lang", false); err == nil {
		t.Fatal("removePack of an unknown lang: expected an error, got nil")
	}

	// golang must be untouched.
	if _, err := os.Stat(filepath.Join(dir, "packs", "languages", "golang", "verify.sh")); err != nil {
		t.Errorf("installed pack must survive an unrelated rm attempt; stat err = %v", err)
	}
}

// TestRemovePack_LegacyManifest_FailsClosedZeroWrites is the `ralph pack rm`
// mirror of TestAddPack_LegacyManifest_FailsClosedZeroWrites: a legacy
// (pre-v2) manifest must be refused with errLegacyLayoutFailClosed and
// nothing on disk may change.
func TestRemovePack_LegacyManifest_FailsClosedZeroWrites(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0755); err != nil {
		t.Fatalf("MkdirAll .ralph: %v", err)
	}
	legacy := scaffold.NewManifest(Version)
	legacy.SetFile("AGENTS.md", "sha256:legacy")
	manifestPath := filepath.Join(dir, ".ralph", "manifest.toml")
	if err := legacy.Write(manifestPath); err != nil {
		t.Fatalf("writing legacy manifest: %v", err)
	}
	beforeManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest before: %v", err)
	}

	_, err = removePack(dir, "golang", false)
	if err == nil {
		t.Fatal("removePack on a legacy manifest: expected an error, got nil")
	}
	if !errors.Is(err, errLegacyLayoutFailClosed) {
		t.Errorf("err = %v, want errors.Is(err, errLegacyLayoutFailClosed)", err)
	}

	afterManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest after: %v", err)
	}
	if string(afterManifest) != string(beforeManifest) {
		t.Errorf("removePack must not touch the manifest on a legacy-layout refusal\nbefore:\n%s\nafter:\n%s", beforeManifest, afterManifest)
	}
}

// TestRemovePack_ForkedFile_RefusedThenForceSucceeds verifies the fork guard:
// a pack whose payload file was ejected (ralph eject / owner=fork) refuses
// non-forced removal, then completes once --force is passed.
func TestRemovePack_ForkedFile_RefusedThenForceSucceeds(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	lang := "golang"
	if err := addPack(dir, lang); err != nil {
		t.Fatalf("addPack(%q): %v", lang, err)
	}

	forkedPath := filepath.Join("packs", "languages", lang, "verify.sh")
	var out bytes.Buffer
	if err := runEjectIO(dir, forkedPath, &out); err != nil {
		t.Fatalf("runEjectIO(%s): %v", forkedPath, err)
	}

	// Without force: refuse, and the pack must be fully intact.
	_, err := removePack(dir, lang, false)
	if err == nil {
		t.Fatal("removePack on a forked pack without --force: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "forked/user-customized") {
		t.Errorf("err = %v, want a fork-guard message", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, forkedPath)); statErr != nil {
		t.Errorf("forked file must survive a refused removal; stat err = %v", statErr)
	}

	// With force: removal completes.
	removed, err := removePack(dir, lang, true)
	if err != nil {
		t.Fatalf("removePack with --force: %v", err)
	}
	if removed == 0 {
		t.Error("forced removePack returned 0 manifest entries removed")
	}
	if _, statErr := os.Stat(filepath.Join(dir, forkedPath)); !os.IsNotExist(statErr) {
		t.Errorf("forced removal must delete the forked file; stat err = %v", statErr)
	}
}

// TestRemovePack_SymlinkedPackDir_Refused verifies that a pack payload
// directory replaced by a symlink is always refused (never --force
// overridden), so deletion can never escape the project boundary.
func TestRemovePack_SymlinkedPackDir_Refused(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	lang := "golang"
	if err := addPack(dir, lang); err != nil {
		t.Fatalf("addPack(%q): %v", lang, err)
	}

	packDir := filepath.Join(dir, "packs", "languages", lang)
	outsideDir := t.TempDir()
	outsideSentinel := filepath.Join(outsideDir, "sentinel.txt")
	if err := os.WriteFile(outsideSentinel, []byte("keep me"), 0644); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	if err := os.RemoveAll(packDir); err != nil {
		t.Fatalf("RemoveAll pack dir: %v", err)
	}
	if err := os.Symlink(outsideDir, packDir); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	_, err := removePack(dir, lang, false)
	if err == nil {
		t.Fatal("removePack over a symlinked pack dir: expected an error, got nil")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("err = %v, want a symlink-guard message", err)
	}
	if _, statErr := os.Stat(outsideSentinel); statErr != nil {
		t.Errorf("outside sentinel must survive a symlink refusal; stat err = %v", statErr)
	}

	// --force must NOT override the symlink guard.
	if _, err := removePack(dir, lang, true); err == nil {
		t.Fatal("removePack --force over a symlinked pack dir: expected an error, got nil")
	}
	if _, statErr := os.Stat(outsideSentinel); statErr != nil {
		t.Errorf("outside sentinel must survive a forced symlink refusal; stat err = %v", statErr)
	}
}

// TestRemovePack_TraversalName_Refused verifies that a lang argument shaped
// like a path (../../x, a/b, absolute) is rejected before it reaches any
// path-building call site.
func TestRemovePack_TraversalName_Refused(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}

	for _, bad := range []string{"../../x", "a/b", "/abs", `a\b`, ".", ".."} {
		if _, err := removePack(dir, bad, false); err == nil {
			t.Errorf("removePack(%q): expected a name-validation error, got nil", bad)
		}
	}
}

// TestPackCmd_RegistersRmSubcommand verifies `ralph pack` exposes add, list,
// and rm subcommands.
func TestPackCmd_RegistersRmSubcommand(t *testing.T) {
	cmd := newPackCmd()
	var names []string
	for _, sub := range cmd.Commands() {
		names = append(names, sub.Name())
	}
	for _, want := range []string{"add", "list", "rm"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("pack subcommands = %v, want %q present", names, want)
		}
	}
}

// TestPackRm_CLI_RemovesInstalledPack exercises the full `ralph pack rm`
// command path (cobra wiring, --force flag default) in a project working
// directory.
func TestPackRm_CLI_RemovesInstalledPack(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	if err := addPack(dir, "golang"); err != nil {
		t.Fatalf("addPack: %v", err)
	}

	t.Chdir(dir)
	root := NewRootCmd()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"pack", "rm", "golang"})
	if err := root.Execute(); err != nil {
		t.Fatalf("ralph pack rm golang: %v (output: %s)", err, buf.String())
	}
	if !strings.Contains(buf.String(), "✓ Pack golang removed") {
		t.Errorf("output missing success line, got:\n%s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "packs", "languages", "golang")); !os.IsNotExist(err) {
		t.Errorf("pack dir must be gone after CLI rm; stat err = %v", err)
	}
}

// TestRemovePack_FSDeleteFailure_WarnsAndSucceeds pins the best-effort
// filesystem sweep: once the manifest has been updated, a subsequent
// RemoveAll failure must not hard-error (that would leave orphaned files
// unreachable via a second `pack rm`, since Meta.Packs no longer lists the
// pack). Instead the call succeeds and warns on stderr.
func TestRemovePack_FSDeleteFailure_WarnsAndSucceeds(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	lang := "golang"
	if err := addPack(dir, lang); err != nil {
		t.Fatalf("addPack(%q): %v", lang, err)
	}

	packDir := filepath.Join(dir, "packs", "languages", lang)
	if err := os.Chmod(packDir, 0o555); err != nil {
		t.Fatalf("Chmod packDir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(packDir, 0o755) })

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = w
	removed, rmErr := removePack(dir, lang, false)
	_ = w.Close()
	os.Stderr = oldStderr
	var stderrBuf bytes.Buffer
	_, _ = stderrBuf.ReadFrom(r)
	_ = r.Close()
	stderr := stderrBuf.String()

	if rmErr != nil {
		t.Fatalf("removePack after FS delete failure: expected nil error, got %v", rmErr)
	}
	if removed == 0 {
		t.Error("removePack returned 0 manifest entries removed")
	}
	if !strings.Contains(stderr, "warning: removing pack directory") {
		t.Errorf("expected stderr warning about pack directory, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "manifest already updated") {
		t.Errorf("expected stderr to mention manifest already updated, got:\n%s", stderr)
	}

	m, err := scaffold.ReadManifest(filepath.Join(dir, ".ralph", "manifest.toml"))
	if err != nil {
		t.Fatalf("ReadManifest: %v", err)
	}
	if slices.Contains(m.Meta.Packs, lang) {
		t.Errorf("Meta.Packs still lists %q after successful manifest-first rm: %v", lang, m.Meta.Packs)
	}
}

// TestRemovePack_EmptyClaudeDir_Survives verifies that removePack never
// tidies .claude itself away, even when the pack rule was the only content
// under it (settings.json / hooks absent).
func TestRemovePack_EmptyClaudeDir_Survives(t *testing.T) {
	setupTestEmbedFS(t)
	Version = "0.1.0-test"

	dir := t.TempDir()
	cfg := initConfig{ProjectName: "test", Packs: nil}
	if err := executeInit(dir, cfg, false); err != nil {
		t.Fatalf("executeInit: %v", err)
	}
	lang := "golang"
	if err := addPack(dir, lang); err != nil {
		t.Fatalf("addPack(%q): %v", lang, err)
	}

	// Strip everything under .claude except the pack rule tree so the only
	// reason .claude exists after tidy would be if we incorrectly removed it.
	claudeDir := filepath.Join(dir, ".claude")
	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		t.Fatalf("ReadDir .claude: %v", err)
	}
	for _, e := range entries {
		if e.Name() == "rules" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(claudeDir, e.Name())); err != nil {
			t.Fatalf("RemoveAll %s: %v", e.Name(), err)
		}
	}

	if _, err := removePack(dir, lang, false); err != nil {
		t.Fatalf("removePack(%q): %v", lang, err)
	}

	if _, err := os.Stat(claudeDir); err != nil {
		t.Errorf(".claude must survive pack rm even when empty; stat err = %v", err)
	}
}
