package gorelease_test

import (
	"path/filepath"
	"testing"

	"webtyp.com/ghaction"
	"webtyp.com/gorelease"
)

func TestWorkflowsAreInSync(t *testing.T) {
	// Release workflow
	relPath := filepath.Join("..", ".github", "workflows", "release.yml")
	rel := gorelease.ReleaseWorkflow()
	if changed, err := ghaction.SyncWorkflow(relPath, rel); err != nil {
		t.Fatalf("sync release workflow: %v", err)
	} else if changed {
		t.Fatalf("%s was out of sync and has just been regenerated. Review the diff and commit it.", relPath)
	}

	// Test workflow
	testPath := filepath.Join("..", ".github", "workflows", "test.yml")
	tst := gorelease.TestWorkflow()
	if changed, err := ghaction.SyncWorkflow(testPath, tst); err != nil {
		t.Fatalf("sync test workflow: %v", err)
	} else if changed {
		t.Fatalf("%s was out of sync and has just been regenerated. Review the diff and commit it.", testPath)
	}
}
