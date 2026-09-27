package app

import (
	"context"
	"testing"

	"lidoo/internal/files"
	"lidoo/internal/profile"
)

func TestCreateProfileStoresVersionWithoutContainer(t *testing.T) {
	t.Chdir(t.TempDir())
	service := &Service{}

	if err := service.CreateProfile(context.Background(), "fresh", "18"); err != nil {
		t.Fatal(err)
	}

	state, err := files.ReadState()
	if err != nil {
		t.Fatal(err)
	}
	config, found, err := profile.Lookup(state, "fresh")
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("profile was not created")
	}
	if config.Version == nil || *config.Version != "18" {
		t.Fatalf("version = %v, want 18", config.Version)
	}
	if config.Prefix != "fresh__" {
		t.Fatalf("prefix = %q, want fresh__", config.Prefix)
	}
}

func TestCreateProfileRejectsInvalidVersion(t *testing.T) {
	t.Chdir(t.TempDir())
	service := &Service{}

	if err := service.CreateProfile(context.Background(), "fresh", "not-a-version"); err == nil {
		t.Fatal("expected an error for an invalid version")
	}
	if err := service.CreateProfile(context.Background(), "fresh", ""); err == nil {
		t.Fatal("expected an error for a missing version")
	}
}
