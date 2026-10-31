package gorelease_test

import (
	"testing"

	"webtyp.com/deploy"
	"webtyp.com/gorelease"
	"webtyp.com/keyring"
)

type mapKeyStore map[string]string

func (m mapKeyStore) Get(key string) (string, error) {
	v, ok := m[key]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (m mapKeyStore) Set(key, value string) error {
	m[key] = value
	return nil
}

func TestCI_Validation(t *testing.T) {
	store := make(mapKeyStore)
	_, err := gorelease.NewCI(gorelease.CIConfig{RootDir: "", KeyStore: store})
	if err == nil {
		t.Fatal("expected error on empty RootDir")
	}

	_, err = gorelease.NewCI(gorelease.CIConfig{RootDir: ".", KeyStore: nil})
	if err == nil {
		t.Fatal("expected error on nil KeyStore")
	}
}

func TestCI_Check_KeyAndGit(t *testing.T) {
	store := make(mapKeyStore)
	ci, err := gorelease.NewCI(gorelease.CIConfig{
		RootDir:  ".",
		KeyStore: store,
		GitCmd: func(dir string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "status" {
				return " M file.go\n", nil
			}
			if len(args) > 0 && args[0] == "tag" {
				return "", nil
			}
			return "", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	missing, err := ci.Check(deploy.Plan{}, deploy.Job{})
	if err != nil {
		t.Fatal(err)
	}
	// Expected: 1 key missing (FixAutomatic), 1 dirty tree (FixManual), 1 no tag (FixManual)
	if len(missing) != 3 {
		t.Fatalf("expected 3 missing items, got %d: %+v", len(missing), missing)
	}

	// Run Provision: creates signing key
	if err := ci.Provision(deploy.Plan{}, deploy.Job{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store[gorelease.SigningKey]; !ok {
		t.Fatal("signing key was not provisioned in store")
	}

	// Clean git status & valid tag
	ciClean, _ := gorelease.NewCI(gorelease.CIConfig{
		RootDir:  ".",
		KeyStore: store,
		GitCmd: func(dir string, args ...string) (string, error) {
			if len(args) > 0 && args[0] == "status" {
				return "", nil
			}
			if len(args) > 0 && args[0] == "tag" {
				return "v1.0.0\n", nil
			}
			return "", nil
		},
	})
	missing, err = ciClean.Check(deploy.Plan{}, deploy.Job{})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Fatalf("expected 0 missing items on clean tree with key and tag, got %+v", missing)
	}
}

func TestCI_Status(t *testing.T) {
	store := make(mapKeyStore)
	ci, _ := gorelease.NewCI(gorelease.CIConfig{RootDir: ".", KeyStore: store})
	st, err := ci.Status(deploy.Run{ID: "v1.0.0"})
	if err != nil || st != deploy.RunSucceeded {
		t.Fatalf("expected RunSucceeded, got %v, err: %v", st, err)
	}
}
