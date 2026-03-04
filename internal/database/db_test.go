package database

import "testing"

func TestNormalizeDatabaseURLForMigrate_AddsSSLModeDisableForLocalhost(t *testing.T) {
	in := "postgres://comu:comu@localhost:5432/comuapi"
	got := normalizeDatabaseURLForMigrate(in)

	want := "postgres://comu:comu@localhost:5432/comuapi?sslmode=disable"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestNormalizeDatabaseURLForMigrate_PreservesExistingSSLMode(t *testing.T) {
	in := "postgres://comu:comu@localhost:5432/comuapi?sslmode=require"
	got := normalizeDatabaseURLForMigrate(in)
	if got != in {
		t.Fatalf("expected %q, got %q", in, got)
	}
}

func TestNormalizeDatabaseURLForMigrate_DoesNotModifyRemoteHostWithoutSSLMode(t *testing.T) {
	in := "postgres://comu:comu@db.internal:5432/comuapi"
	got := normalizeDatabaseURLForMigrate(in)
	if got != in {
		t.Fatalf("expected %q, got %q", in, got)
	}
}
