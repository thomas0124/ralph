package org

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// countOrgEvents returns how many manifest events carry orgID -- the number
// Purge should report as EventsRemoved.
func countOrgEvents(t *testing.T, o *Org, orgID string) int {
	t.Helper()
	n := 0
	for _, ev := range mustReadEvents(t, o) {
		if ev.OrgID == orgID {
			n++
		}
	}
	return n
}

// seedPromptFile writers a rendered role prompt exactly where Spawn writes
// them: <state-dir>/prompts/<org_id>_<seat_id>.md (promptFilePath,
// spawn.go).
func seedPromptFile(t *testing.T, dir, orgID, seatID string) string {
	t.Helper()
	path := filepath.Join(dir, "prompts", orgID+"_"+seatID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir prompts dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("prompt for "+seatID), 0o644); err != nil {
		t.Fatalf("write prompt file: %v", err)
	}
	return path
}

// TestOrgPurge_NotDisbanded_RejectsBeforeAnyChange pins Purge's delegated
// gate: an org whose manifest has no real `disbanded` event is refused
// (exit-state is up to the caller) and every store is left completely
// untouched.
func TestOrgPurge_NotDisbanded_RejectsBeforeAnyChange(t *testing.T) {
	o, _, _ := testOrg(t)
	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn failed: %+v", r)
	}
	eventsBefore := len(mustReadEvents(t, o))
	dir := filepath.Dir(o.Manifest.Path())

	result := o.Purge(PurgeParams{OrgID: "org-a"})
	if result.Err == nil {
		t.Fatalf("expected purge of an undisbanded org to be rejected, got %+v", result)
	}
	if !strings.Contains(result.Err.Error(), "not disbanded") {
		t.Errorf("expected error to mention 'not disbanded', got: %v", result.Err)
	}

	if got := len(mustReadEvents(t, o)); got != eventsBefore {
		t.Errorf("expected manifest untouched by a rejected purge, %d -> %d events", eventsBefore, got)
	}
	if _, err := os.Stat(o.Manifest.Path()); err != nil {
		t.Errorf("expected manifest file still present after a rejected purge, stat err=%v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("expected state dir still present after a rejected purge, stat err=%v", err)
	}
}

// TestOrgPurge_DryRunDisbanded_DoesNotSatisfyGate documents that a dry-run
// `disbanded` event (which only deactivates dry-run seat entries, never a
// real teardown) is not evidence Purge's gate accepts -- matching the gate
// doc on hasRealDisbandedEvent.
func TestOrgPurge_DryRunDisbanded_DoesNotSatisfyGate(t *testing.T) {
	o, _, _ := testOrg(t)
	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn failed: %+v", r)
	}
	if res := o.Disband(DisbandParams{OrgID: "org-a", DryRun: true}); len(res.Errs) != 0 {
		t.Fatalf("dry-run disband failed: %v", res.Errs)
	}

	result := o.Purge(PurgeParams{OrgID: "org-a"})
	if result.Err == nil {
		t.Fatal("expected purge to be rejected when only a dry-run disbanded event exists")
	}
	if !strings.Contains(result.Err.Error(), "not disbanded") {
		t.Errorf("expected error to mention 'not disbanded', got: %v", result.Err)
	}
}

// TestOrgPurge_Force_RemovesOrgFootprintAcrossAllStores is the full sweep:
// --force purges an undisbanded org across every store -- manifest events,
// model receipts (filtered, other orgs preserved), escalations.jsonl rows
// (filtered), prompt files, and watch-status files.
func TestOrgPurge_Force_RemovesOrgFootprintAcrossAllStores(t *testing.T) {
	o, _, _ := testOrg(t)
	dir := filepath.Dir(o.Manifest.Path())

	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn org-a failed: %+v", r)
	}
	if r := o.Spawn(mustSpawnParams("org-b", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn org-b failed: %+v", r)
	}

	promptPath := seedPromptFile(t, dir, "org-a", "seat-1")
	watchPath := filepath.Join(dir, WatchStatusFileName("org-a"))
	if err := os.WriteFile(watchPath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write watch-status: %v", err)
	}
	escPath := filepath.Join(dir, EscalationsRelName)
	for _, orgID := range []string{"org-a", "org-b"} {
		if err := appendJSONLine(escPath, escalationRecord{
			TS: "2026-08-01T00:00:00Z", OrgID: orgID, AlertID: "a-1", Reason: "test",
		}); err != nil {
			t.Fatalf("append escalation for %s: %v", orgID, err)
		}
	}

	eventsBefore := countOrgEvents(t, o, "org-a")

	result := o.Purge(PurgeParams{OrgID: "org-a", Force: true})
	if result.Err != nil {
		t.Fatalf("expected --force purge to succeed, got %v", result.Err)
	}
	if result.EventsRemoved != eventsBefore {
		t.Errorf("EventsRemoved = %d, want org-a event count %d", result.EventsRemoved, eventsBefore)
	}
	if result.ReceiptsRemoved != 1 {
		t.Errorf("ReceiptsRemoved = %d, want 1 (only org-a's seat had a receipt)", result.ReceiptsRemoved)
	}

	if got := countOrgEvents(t, o, "org-a"); got != 0 {
		t.Errorf("expected no org-a manifest events to remain, got %d", got)
	}
	if got := countOrgEvents(t, o, "org-b"); got == 0 {
		t.Error("expected org-b manifest events to be preserved")
	}

	if _, err := os.Stat(promptPath); !os.IsNotExist(err) {
		t.Errorf("expected org-a prompt file removed, stat err=%v", err)
	}
	if _, err := os.Stat(watchPath); !os.IsNotExist(err) {
		t.Errorf("expected org-a watch-status file removed, stat err=%v", err)
	}

	// Receipts: org-a's line gone, org-b's preserved (file kept).
	receipts := readJSONLFile(t, o.Receipts.Path())
	if len(receipts) != 1 || !strings.Contains(receipts[0], `"org_id":"org-b"`) {
		t.Fatalf("expected exactly the org-b receipt line to remain, got %q", receipts)
	}

	// Escalations: org-a's row gone, org-b's row preserved.
	escLines := readJSONLFile(t, escPath)
	if len(escLines) != 1 || !strings.Contains(escLines[0], `"org_id":"org-b"`) {
		t.Fatalf("expected exactly the org-b escalation row to remain, got %q", escLines)
	}
}

// TestOrgPurge_Disbanded_PurgesOrgLeavesOthersAndCorruptLines checks the
// gate is satisfied by a real `disbanded` event, and that the rewrite is
// surgical: other orgs' events and any corrupt/unanonymizable lines survive
// with their bytes intact.
func TestOrgPurge_Disbanded_PurgesOrgLeavesOthersAndCorruptLines(t *testing.T) {
	o, _, _ := testOrg(t)

	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn org-a failed: %+v", r)
	}
	if r := o.Spawn(mustSpawnParams("org-b", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn org-b failed: %+v", r)
	}
	if res := o.Disband(DisbandParams{OrgID: "org-a"}); len(res.Errs) != 0 {
		t.Fatalf("disband org-a failed: %v", res.Errs)
	}

	manifestPath := o.Manifest.Path()
	f, err := os.OpenFile(manifestPath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open manifest for corrupt injection: %v", err)
	}
	if _, err := f.WriteString("{not valid json\n"); err != nil {
		t.Fatalf("write corrupt line: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close manifest: %v", err)
	}

	result := o.Purge(PurgeParams{OrgID: "org-a"})
	if result.Err != nil {
		t.Fatalf("expected disbanded purge to succeed, got %v", result.Err)
	}
	if countOrgEvents(t, o, "org-a") != 0 {
		t.Error("expected no org-a events to remain after a disbanded purge")
	}
	if countOrgEvents(t, o, "org-b") == 0 {
		t.Error("expected org-b events to be preserved")
	}
	lines := readJSONLFile(t, manifestPath)
	foundCorrupt := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "{not valid json" {
			foundCorrupt = true
		}
	}
	if !foundCorrupt {
		t.Errorf("expected the corrupt manifest line to survive the rewrite, got lines: %q", lines)
	}
}

// TestOrgPurge_SoloOrg_DeletesStoresEntirely covers the "no retained lines"
// branch of filterJSONLByOrgID: when the purged org is the only tenant, both
// JSONL files are removed from disk rather than rewritten empty.
func TestOrgPurge_SoloOrg_DeletesStoresEntirely(t *testing.T) {
	o, _, _ := testOrg(t)
	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn failed: %+v", r)
	}
	if res := o.Disband(DisbandParams{OrgID: "org-a"}); len(res.Errs) != 0 {
		t.Fatalf("disband failed: %v", res.Errs)
	}

	result := o.Purge(PurgeParams{OrgID: "org-a"})
	if result.Err != nil {
		t.Fatalf("purge failed: %v", result.Err)
	}
	for _, p := range []string{o.Manifest.Path(), o.Receipts.Path()} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("expected %s removed when it held only the purged org, stat err=%v", p, err)
		}
	}
}

// TestOrgPurge_NoEventsForOrg_Noop pins --force purge idempotence: an org
// id that never existed yields zero removals and no error.
func TestOrgPurge_NoEventsForOrg_Noop(t *testing.T) {
	o, _, _ := testOrg(t)
	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn failed: %+v", r)
	}
	eventsBefore := len(mustReadEvents(t, o))

	result := o.Purge(PurgeParams{OrgID: "never-existed", Force: true})
	if result.Err != nil {
		t.Fatalf("expected a no-op force purge to succeed, got %v", result.Err)
	}
	if result.EventsRemoved != 0 || result.ReceiptsRemoved != 0 {
		t.Errorf("expected zero removals for an unknown org, got events=%d receipts=%d",
			result.EventsRemoved, result.ReceiptsRemoved)
	}
	if got := len(mustReadEvents(t, o)); got != eventsBefore {
		t.Errorf("expected manifest untouched by a no-op purge, %d -> %d", eventsBefore, got)
	}
}

// TestOrgPurge_UnreadableManifest_ReportsIOError pins that a manifest
// open/read failure is surfaced as an I/O error, not mislabeled as
// "not disbanded" (operators must not re-disband an already-torn-down org
// when the real problem is permissions or disk).
func TestOrgPurge_UnreadableManifest_ReportsIOError(t *testing.T) {
	o, _, _ := testOrg(t)
	if r := o.Spawn(mustSpawnParams("org-a", "seat-1")); r.Outcome != SpawnOutcomeSpawned {
		t.Fatalf("spawn failed: %+v", r)
	}
	if res := o.Disband(DisbandParams{OrgID: "org-a"}); len(res.Errs) != 0 {
		t.Fatalf("disband failed: %v", res.Errs)
	}

	manifestPath := o.Manifest.Path()
	if err := os.Chmod(manifestPath, 0o000); err != nil {
		t.Fatalf("Chmod manifest: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(manifestPath, 0o644) })

	result := o.Purge(PurgeParams{OrgID: "org-a"})
	if result.Err == nil {
		t.Fatal("expected purge of an unreadable manifest to fail")
	}
	errMsg := result.Err.Error()
	if strings.Contains(errMsg, "not disbanded") {
		t.Errorf("I/O failure must not be labeled 'not disbanded', got: %v", result.Err)
	}
	if !strings.Contains(errMsg, "open") {
		t.Errorf("expected error to mention open failure, got: %v", result.Err)
	}
}
