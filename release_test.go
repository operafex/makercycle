package main

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 1: versão injetável via -ldflags (version.go)
// Task 6: pipeline de release (.github/workflows/release.yml)
// Task 8: documentação (README.md / README.pt-BR.md)
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: VR-*, RL-*, RD-*)
// ---------------------------------------------------------------------------

// VR-H1: Version precisa ser variável (o linker só injeta -X em var string).
func TestVersionIsInjectableVar(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	Version = "9.9.9"
	if got := GetVersion(); got != "makercycle v9.9.9" {
		t.Errorf("GetVersion() = %q; want %q", got, "makercycle v9.9.9")
	}
}

// VR-H2: tag do GitHub ("v1.2.3") injetada em Version não duplica o "v".
func TestGetVersionStripsLeadingV(t *testing.T) {
	orig := Version
	t.Cleanup(func() { Version = orig })
	cases := map[string]string{
		"v1.2.3":      "makercycle v1.2.3",
		"1.2.3":       "makercycle v1.2.3",
		"v2.0.0-rc.1": "makercycle v2.0.0-rc.1",
		" v1.0.0\n":   "makercycle v1.0.0",
	}
	for in, want := range cases {
		Version = in
		if got := GetVersion(); got != want {
			t.Errorf("Version=%q: GetVersion() = %q; want %q", in, got, want)
		}
	}
}

func releaseWorkflow(t *testing.T) string {
	t.Helper()
	return readRepoFile(t, ".github/workflows/release.yml")
}

// RL-H1: disparo apenas por tags v*.
func TestReleaseWorkflowTriggersOnVersionTags(t *testing.T) {
	wf := releaseWorkflow(t)
	for _, s := range []string{"on:", "push:", "tags:", "'v*'"} {
		if !strings.Contains(wf, s) {
			t.Errorf("release.yml sem %q", s)
		}
	}
	if strings.Contains(wf, "pull_request") {
		t.Error("release.yml não deve disparar em pull_request")
	}
}

// RL-H2: testes com race detector antes do build.
func TestReleaseWorkflowRunsRaceTestsBeforeBuild(t *testing.T) {
	wf := releaseWorkflow(t)
	ti := strings.Index(wf, "go test -race ./...")
	bi := strings.Index(wf, "go build")
	if ti < 0 {
		t.Fatal("release.yml sem `go test -race ./...`")
	}
	if bi < 0 || ti > bi {
		t.Errorf("`go test -race ./...` (pos %d) deve vir antes de `go build` (pos %d)", ti, bi)
	}
}

// RL-H3: os seis alvos exigidos.
func TestReleaseWorkflowBuildsAllTargets(t *testing.T) {
	wf := releaseWorkflow(t)
	for _, target := range []string{"darwin/arm64", "darwin/amd64", "linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64"} {
		if !strings.Contains(wf, target) {
			t.Errorf("release.yml sem alvo %q", target)
		}
	}
	if !strings.Contains(wf, "CGO_ENABLED=0") {
		t.Error("release.yml deve compilar com CGO_ENABLED=0 (binário estático)")
	}
}

// RL-H4: ldflags exatas da referência.
func TestReleaseWorkflowLdflagsExact(t *testing.T) {
	want := `-ldflags="-s -w -X main.Version=${{ github.ref_name }}"`
	if !strings.Contains(releaseWorkflow(t), want) {
		t.Errorf("release.yml sem as ldflags exatas %s", want)
	}
}

// RL-H5: nomes dos assets consumidos pelos instaladores + checksums.
func TestReleaseWorkflowAssetNamesAndChecksums(t *testing.T) {
	wf := releaseWorkflow(t)
	for _, s := range []string{"makercycle-${GOOS}-${GOARCH}", ".exe", "checksums.txt", "sha256sum"} {
		if !strings.Contains(wf, s) {
			t.Errorf("release.yml sem %q", s)
		}
	}
}

// RL-H6: publicação em GitHub Releases com permissão mínima e sem actions de terceiros.
func TestReleaseWorkflowPublishesRelease(t *testing.T) {
	wf := releaseWorkflow(t)
	for _, s := range []string{"contents: write", "gh release create", "GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}", "actions/checkout@v4", "actions/setup-go@v5", "go-version-file: go.mod"} {
		if !strings.Contains(wf, s) {
			t.Errorf("release.yml sem %q", s)
		}
	}
	for _, line := range strings.Split(wf, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "uses:") || strings.HasPrefix(l, "- uses:") {
			if !strings.Contains(l, "actions/checkout@") && !strings.Contains(l, "actions/setup-go@") {
				t.Errorf("action não permitida (supply chain): %q", l)
			}
		}
	}
}

// RL-X1: o nome da tag é validado (SemVer) antes de ser interpolado no script de build.
func TestReleaseWorkflowValidatesTagName(t *testing.T) {
	wf := releaseWorkflow(t)
	vi := strings.Index(wf, `^v[0-9]+\.[0-9]+\.[0-9]+`)
	bi := strings.Index(wf, "go build")
	if vi < 0 {
		t.Fatal("release.yml deve validar GITHUB_REF_NAME com a regex SemVer ^v[0-9]+\\.[0-9]+\\.[0-9]+")
	}
	if bi >= 0 && vi > bi {
		t.Error("validação da tag deve vir antes do build")
	}
}

// RD-H1: READMEs documentam instalação em 1 comando, -init, herança global e -save-global.
func TestReadmesDocumentDistribution(t *testing.T) {
	must := []string{
		"curl -fsSL https://raw.githubusercontent.com/operafex/makercycle/main/install.sh | sh",
		"irm https://raw.githubusercontent.com/operafex/makercycle/main/install.ps1 | iex",
		"makercycle -init",
		"-save-global",
		"~/.makercycle/config/models.env",
		"-force",
		"makercycle -version",
	}
	for _, f := range []string{"README.md", "README.pt-BR.md"} {
		c := readRepoFile(t, f)
		for _, s := range must {
			if !strings.Contains(c, s) {
				t.Errorf("%s sem %q", f, s)
			}
		}
	}
}
