package main

import (
	"regexp"
	"testing"
)

// ---------------------------------------------------------------------------
// version — Task 1 (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: V-*)
// ---------------------------------------------------------------------------

// V-H1: a constante Version deve ser exatamente "0.1.0".
func TestVersionConstantValue(t *testing.T) {
	if Version != "0.1.0" {
		t.Errorf("Version = %q; want %q", Version, "0.1.0")
	}
}

// V-H2: GetVersion deve retornar exatamente "makercycle v0.1.0".
func TestGetVersionReturnsPrefixedVersion(t *testing.T) {
	want := "makercycle v0.1.0"
	if got := GetVersion(); got != want {
		t.Errorf("GetVersion() = %q; want %q", got, want)
	}
}

// V-E1: GetVersion deve ser derivado da constante Version (sem duplicar a string).
func TestGetVersionIsDerivedFromVersionConstant(t *testing.T) {
	want := "makercycle v" + Version
	if got := GetVersion(); got != want {
		t.Errorf("GetVersion() = %q; want %q", got, want)
	}
}

// V-E2: Version deve estar no formato SemVer MAJOR.MINOR.PATCH.
func TestVersionIsSemver(t *testing.T) {
	semver := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !semver.MatchString(Version) {
		t.Errorf("Version = %q; want formato MAJOR.MINOR.PATCH", Version)
	}
}

// V-E3: GetVersion deve obedecer estritamente ao formato "makercycle vX.Y.Z",
// sem espaços nas bordas, quebra de linha, "v" duplicado ou sufixo extra.
func TestGetVersionStrictFormat(t *testing.T) {
	format := regexp.MustCompile(`^makercycle v\d+\.\d+\.\d+$`)
	if got := GetVersion(); !format.MatchString(got) {
		t.Errorf("GetVersion() = %q; want formato %q", got, "makercycle vX.Y.Z")
	}
}

// V-E4: GetVersion deve ser determinística (mesma saída em chamadas sucessivas).
func TestGetVersionIsDeterministic(t *testing.T) {
	first := GetVersion()
	for i := 0; i < 3; i++ {
		if got := GetVersion(); got != first {
			t.Errorf("chamada %d: GetVersion() = %q; want %q", i+2, got, first)
		}
	}
}
