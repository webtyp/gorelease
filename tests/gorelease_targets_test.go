package gorelease_test

import (
	"testing"

	"github.com/tinywasm/gorelease"
)

func TestDefaultTargets_CoversDistributionMatrix(t *testing.T) {
	targets := gorelease.DefaultTargets()

	want := map[string]bool{
		"linux/amd64":   false,
		"linux/arm64":   false,
		"darwin/arm64":  false,
		"darwin/amd64":  false,
		"windows/amd64": false,
	}
	for _, tg := range targets {
		want[tg.GOOS+"/"+tg.GOARCH] = true
	}

	for k, found := range want {
		if !found {
			t.Errorf("DefaultTargets missing %s", k)
		}
	}
}
