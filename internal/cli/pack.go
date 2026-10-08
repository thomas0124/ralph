package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/thomas0124/ralph/internal/scaffold"
)

func newPackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pack",
		Short: "Manage language packs",
	}

	cmd.AddCommand(newPackAddCmd())
	cmd.AddCommand(newPackListCmd())
	cmd.AddCommand(newPackRmCmd())

	return cmd
}

func newPackAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <language>",
		Short: "Add a language pack to the project",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return addPack(".", args[0])
		},
	}
}

func newPackListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available language packs",
		RunE: func(cmd *cobra.Command, args []string) error {
			packs, err := scaffold.AvailablePacks()
			if err != nil {
				return err
			}
			fmt.Println("Available language packs:")
			for _, p := range packs {
				fmt.Printf("  - %s\n", p)
			}
			return nil
		},
	}
}

// addPack adds a language pack to an existing project rooted at targetDir.
// Pack payload files are written to packs/languages/<lang>/ and the rule.md
// control file is mapped to .claude/rules/ralph/<lang>.md (matching init.go's layout).
// The shared renderPackInto helper (language_pack.go) is used here so this
// path cannot diverge from ralph init's pack rendering.
//
// A legacy (pre-v2) manifest is rejected fail-closed (zero writes) — see
// legacyLayoutFailClosedMsg (upgrade.go). `ralph pack add` does not perform
// the legacy-to-v2 migration itself — `ralph upgrade` does
// (runMigrateLegacy, internal/cli/migrate.go) — so this points the operator
// there instead.
func addPack(targetDir string, lang string) error {
	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	manifestPath := filepath.Join(absDir, ".ralph", "manifest.toml")
	manifest, err := scaffold.ReadManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}
	if manifest.Meta.Layout != scaffold.LayoutV2 {
		return errLegacyLayoutFailClosed
	}

	// renderPackInto handles directory layout and rule.md mapping — identical
	// to what executeInit does for each pack.
	pr, err := renderPackInto(absDir, lang, true /* overwrite existing files */)
	if err != nil {
		return err
	}

	// Update manifest: merge pack entries and write back. Every reachable
	// manifest here is v2 (the fail-closed check above rejects anything
	// else), so ownership is always assigned — classification is shared with
	// ralph init via ownerForScaffoldPath (init.go) rather than mirrored, so
	// the two entry points cannot diverge on a future pack payload path (e.g.
	// under docs/ or .ralph/local/).
	for path, hash := range pr.hashes {
		manifest.SetFile(path, hash)
		if err := manifest.SetOwner(path, ownerForScaffoldPath(path)); err != nil {
			// SetOwner only fails for an invalid owner (impossible —
			// ownerForScaffoldPath always returns a valid constant) or a
			// manifest entry missing for path (impossible here — SetFile was
			// just called for every key in pr.hashes). Kept as a defensive
			// guard, matching the surrounding ReadManifest/Write style.
			fmt.Printf("⚠ Could not set owner for %s: %v\n", path, err)
		}
	}
	// Record the pack in Meta.Packs if not already present.
	alreadyListed := false
	for _, p := range manifest.Meta.Packs {
		if p == lang {
			alreadyListed = true
			break
		}
	}
	if !alreadyListed {
		manifest.Meta.Packs = append(manifest.Meta.Packs, lang)
	}
	if err := manifest.Write(manifestPath); err != nil {
		fmt.Printf("⚠ Could not write manifest: %v\n", err)
	}

	created := len(pr.result.Created)
	updated := len(pr.result.Overwritten)
	fmt.Printf("✓ Pack %s added (%d created, %d updated)\n", lang, created, updated)

	return nil
}

func newPackRmCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "rm <language>",
		Short: "Remove a language pack from the project",
		Long: `Removes <language>'s entire footprint from the project: the
packs/languages/<lang>/ payload directory, its
.claude/rules/ralph/<lang>.md rule file, and every corresponding entry in
.ralph/manifest.toml (including the Meta.Packs listing). Pass --force to
skip the guard that refuses to remove a pack containing a forked
(user-customized, ralph eject) or otherwise unmanaged file.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := removePack(".", args[0], force)
			if err != nil {
				return err
			}
			writef(cmd.OutOrStdout(), "✓ Pack %s removed (%d files deleted)\n", args[0], removed)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false,
		"skip the fork guard (delete forked/user-customized pack files)")
	return cmd
}

// removePack removes a language pack installed in the project rooted at
// targetDir: the pack payload directory (packs/languages/<lang>/), the pack
// rule control file (.claude/rules/ralph/<lang>.md), and every corresponding
// entry in .ralph/manifest.toml, dropping <lang> from Meta.Packs. It returns
// the number of manifest entries removed.
//
// The manifest is treated as the source of truth and written FIRST: if the
// manifest write fails, removePack returns before touching the filesystem, so
// a failed removal never leaves the manifest disagreeing with the disk. The
// filesystem sweep afterwards is best-effort: an already-absent pack dir or
// rule is not an error, and a delete failure is reported as a warning on
// stderr rather than a hard error — the manifest cleanup has already
// completed, so leftover files are litter, not drift. Callers can re-run
// after fixing permissions, or remove the litter by hand.
//
// Guards (matching the surrounding fail-closed conventions):
//   - A legacy (pre-v2) manifest is refused with errLegacyLayoutFailClosed via
//     requireV2ManifestForOwnership (the same barrier add/eject/adopt share).
//   - A pack not listed in Meta.Packs is refused fail-closed.
//   - A forked (OwnerFork / Managed=false) file under the pack namespace
//     aborts the removal unless force is set.
//   - A symlinked pack directory is always refused: os.RemoveAll would unlink
//     through the link and delete outside targetDir. A symlink *inside* the
//     pack dir is fine — RemoveAll unlinks the link entry itself.
//   - lang is validated as a single path-safe directory name before it is
//     used to build any path.
func removePack(targetDir, lang string, force bool) (int, error) {
	if err := validatePackName(lang); err != nil {
		return 0, err
	}

	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return 0, err
	}
	manifest, manifestPath, err := requireV2ManifestForOwnership(absDir)
	if err != nil {
		return 0, err
	}

	if !slices.Contains(manifest.Meta.Packs, lang) {
		return 0, fmt.Errorf("language pack %q is not installed (Meta.Packs = %v)", lang, manifest.Meta.Packs)
	}

	// Collect every manifest key owned by this pack: the namespaced payload
	// keys plus the rule.md control-file key (and, defensively, the legacy
	// rule location — a v2 manifest should not have it, but sweeping it too
	// keeps a stale footprint from surviving cleanup).
	packPathPrefix := packPrefixFor(lang)
	ruleKey := packRuleRelPath(lang)
	legacyRuleKey := legacyPackRuleRelPath(lang)
	var keys []string
	for key := range manifest.Files {
		if key == ruleKey || key == legacyRuleKey || strings.HasPrefix(key, packPathPrefix) {
			keys = append(keys, key)
		}
	}

	if !force {
		var forked []string
		for _, key := range keys {
			if e := manifest.Files[key]; e.Owner == scaffold.OwnerFork || !e.Managed {
				forked = append(forked, key)
			}
		}
		if len(forked) > 0 {
			return 0, fmt.Errorf(
				"language pack %q contains forked/user-customized files; refusing to delete: %s (pass --force to remove anyway)",
				lang, strings.Join(forked, ", "))
		}
	}

	// Safety preflight on the payload directory itself: a symlink here would
	// make os.RemoveAll delete through the link (escaping targetDir), so it
	// is always refused — even with --force — mirroring the upgrade applyOps
	// Lstat preflight that rejects symlink delete targets (internal/upgrade).
	packDir := filepath.Join(absDir, packRelDir(lang))
	if info, statErr := os.Lstat(packDir); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return 0, fmt.Errorf("refusing to remove pack %q: %s is a symlink (remove it manually if intended)", lang, packDir)
	}

	for _, key := range keys {
		delete(manifest.Files, key)
	}
	var packs []string
	for _, p := range manifest.Meta.Packs {
		if p != lang {
			packs = append(packs, p)
		}
	}
	manifest.Meta.Packs = packs
	if err := manifest.Write(manifestPath); err != nil {
		return 0, fmt.Errorf("writing manifest: %w", err)
	}

	// Filesystem sweep (best-effort after a successful manifest write).
	// os.RemoveAll returns nil for an already-absent path.
	if err := os.RemoveAll(packDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: removing pack directory %s: %v (manifest already updated)\n", packDir, err)
	}
	ruleFile := filepath.Join(absDir, filepath.FromSlash(ruleKey))
	if err := os.Remove(ruleFile); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "warning: removing rule file %s: %v (manifest already updated)\n", ruleFile, err)
	}
	// removeIfEmpty returns error for absent dirs (swallowed below).
	// Stop at .claude/rules — never tidy .claude itself, which may hold
	// settings.json, hooks/, or other tools' state unrelated to this pack.
	_ = removeIfEmpty(filepath.Join(absDir, "packs", "languages"))
	_ = removeIfEmpty(filepath.Join(absDir, "packs"))
	_ = removeIfEmpty(filepath.Join(absDir, ".claude", "rules", "ralph"))
	_ = removeIfEmpty(filepath.Join(absDir, ".claude", "rules"))

	return len(keys), nil
}

// validatePackName rejects a lang argument that could escape the
// packs/languages/<lang> namespace when resolved to a filesystem path
// (e.g. "../../x", "a/b"). A pack name must be a single directory-name
// segment — the same path-safety stance org takes with ValidateIdentifier.
func validatePackName(lang string) error {
	if lang == "" || lang == "." || lang == ".." {
		return fmt.Errorf("invalid language pack name %q", lang)
	}
	if filepath.Base(lang) != lang {
		return fmt.Errorf("invalid language pack name %q (must be a single directory name, no path separators)", lang)
	}
	return nil
}

// removeIfEmpty removes dir if it exists and contains no entries. Non-empty
// directories and already-absent directories are left alone (this is
// best-effort cleanup, not validation).
func removeIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(dir)
}
