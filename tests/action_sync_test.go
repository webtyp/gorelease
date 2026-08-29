package gorelease_test

import (
	"path/filepath"
	"testing"

	"github.com/tinywasm/ghaction"
	"github.com/tinywasm/gorelease"
)

func TestActionYmlIsInSync(t *testing.T) {
	tag, err := gorelease.LatestReleaseTag()
	if err != nil || tag == "" {
		t.Skipf("skipping TestActionYmlIsInSync: git tags not available or empty (err=%v, tag=%q)", err, tag)
	}

	actionPath := filepath.Join("..", gorelease.ActionFilePath)
	a := gorelease.GoreleaseAction(tag)

	changed, err := ghaction.Sync(actionPath, a)
	if err != nil {
		t.Fatal(err)
	}

	if changed {
		t.Fatalf("%s was out of sync and has just been regenerated. "+
			"Review the diff and commit it; this test goes green afterwards.",
			gorelease.ActionFilePath)
	}
}
