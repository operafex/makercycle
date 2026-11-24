package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Modo -manual — checkpoint (RESUME.env), fiação no main, READMEs e E2E
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: MR-T6..MR-T9)
// ---------------------------------------------------------------------------

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// Task 6 — ResumeState.Manual (MC_RESUME_MANUAL)
// ---------------------------------------------------------------------------

// MR-T6-H1: Manual=true é persistido e relido.
func TestResumeStateManualRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state", "RESUME.env")
	want := ResumeState{Stage: 3, Demand: "docs/demands/2026-11-24-x.md", Stack: "Go", Attempt: 2, Manual: true}
	if err := writeResumeState(p, want); err != nil {
		t.Fatal(err)
	}
	if c := readRepoFile(t, p); !strings.Contains(c, `MC_RESUME_MANUAL="true"`) {
		t.Fatalf("RESUME.env sem MC_RESUME_MANUAL=\"true\":\n%s", c)
	}
	got, err := loadResumeState(p)
	if err != nil || got != want {
		t.Fatalf("loadResumeState = %+v, %v; want %+v", got, err, want)
	}
}

// MR-T6-H2: Manual=false grava "false"; uma gravação posterior não duplica a chave.
func TestResumeStateManualFalseOverwrites(t *testing.T) {
	p := filepath.Join(t.TempDir(), "RESUME.env")
	_ = writeResumeState(p, ResumeState{Stage: 2, Stack: "Go", Attempt: 1, Manual: true})
	if err := writeResumeState(p, ResumeState{Stage: 4, Stack: "Go", Attempt: 1}); err != nil {
		t.Fatal(err)
	}
	c := readRepoFile(t, p)
	if strings.Count(c, "MC_RESUME_MANUAL=") != 1 || !strings.Contains(c, `MC_RESUME_MANUAL="false"`) {
		t.Fatalf("MC_RESUME_MANUAL duplicada ou não atualizada:\n%s", c)
	}
}

// MR-T6-E1: RESUME.env legado (sem MC_RESUME_MANUAL) carrega Manual=false.
func TestLoadResumeStateLegacyWithoutManual(t *testing.T) {
	p := filepath.Join(t.TempDir(), "RESUME.env")
	writeTestFile(t, p, "MC_RESUME_STAGE=\"3\"\nMC_RESUME_STACK=\"Go\"\nMC_RESUME_ATTEMPT=\"1\"\n")
	got, err := loadResumeState(p)
	if err != nil || got.Manual {
		t.Fatalf("legado: %+v, %v; want Manual=false sem erro", got, err)
	}
}

// MR-T6-X1: valor inválido em MC_RESUME_MANUAL é rejeitado citando a chave.
func TestLoadResumeStateRejectsInvalidManual(t *testing.T) {
	p := filepath.Join(t.TempDir(), "RESUME.env")
	writeTestFile(t, p, "MC_RESUME_STAGE=\"3\"\nMC_RESUME_STACK=\"Go\"\nMC_RESUME_MANUAL=\"talvez\"\n")
	if _, err := loadResumeState(p); err == nil || !strings.Contains(err.Error(), "MC_RESUME_MANUAL") {
		t.Fatalf("esperado erro citando MC_RESUME_MANUAL, veio %v", err)
	}
}

// ---------------------------------------------------------------------------
// Task 7 — fiação em makercycle.go / init_run.go
// ---------------------------------------------------------------------------

// MR-T7-H1: flag -manual registrada e args normalizados antes do flag.Parse().
func TestMainRegistersAndNormalizesManualFlag(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	if !strings.Contains(src, `flag.Bool("manual", false,`) {
		t.Errorf("makercycle.go deve registrar flag.Bool(\"manual\", false, ...)")
	}
	norm := strings.Index(src, "os.Args = normalizeManualArgs(os.Args)")
	parse := strings.Index(src, "flag.Parse()")
	if norm < 0 || parse < 0 || norm > parse {
		t.Errorf("os.Args = normalizeManualArgs(os.Args) deve vir antes de flag.Parse() (norm=%d parse=%d)", norm, parse)
	}
}

// MR-T7-H2: cada etapa do ciclo passa pelo manualReviewer e o modo é resolvido/persistido.
func TestMainWiresManualReviewInEveryStage(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	for _, want := range []string{
		`Review("E1"`, `Review("E2"`, `Review("E3"`, `Review("E4A"`, `Review("E4B"`,
		"resolveManualMode(", "appendManualFeedback(", "manualResumeStage(", "verifyReportPassed(",
		"manualAbortStatus", "manualSaveStatus", "exitManualAbort", "manualScoutRunner(",
		"Manual:", "manualDatabasePromptTimeout",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("makercycle.go sem %q", want)
		}
	}
}

// MR-T7-H3: runScoutAgent delega para runScoutAgentWithFeedback (prompt com feedback do revisor).
func TestRunScoutAgentDelegatesToFeedbackVariant(t *testing.T) {
	src := readRepoFile(t, "init_run.go")
	if !strings.Contains(src, "func runScoutAgentWithFeedback(") {
		t.Fatalf("init_run.go sem runScoutAgentWithFeedback")
	}
	if !strings.Contains(src, `return runScoutAgentWithFeedback(scout, docsDir, existingDocs, detectedName, jsonMetrics, "")`) {
		t.Errorf("runScoutAgent deve delegar para runScoutAgentWithFeedback com feedback vazio")
	}
	if !strings.Contains(src, "appendManualFeedback(") {
		t.Errorf("runScoutAgentWithFeedback deve anexar o feedback via appendManualFeedback")
	}
}

// MR-T7-E1: no modo manual o prompt do banco não expira (evita goroutine pendente roubando a 1ª pausa).
func TestManualDatabasePromptTimeoutIsLong(t *testing.T) {
	if manualDatabasePromptTimeout < 12*defaultPromptTimeout {
		t.Errorf("manualDatabasePromptTimeout = %v; deve ser >= 12x defaultPromptTimeout (%v)", manualDatabasePromptTimeout, defaultPromptTimeout)
	}
}

// ---------------------------------------------------------------------------
// Task 8 — READMEs (EN / PT-BR)
// ---------------------------------------------------------------------------

// MR-T8-H1: seção do modo manual com -task e -resume, teclas, ABORTED e código 6.
func TestReadmesDocumentManualMode(t *testing.T) {
	heads := map[string][2]string{
		"README.md":       {"### Manual review mode (`-manual`)", "### 4. Try different model combinations"},
		"README.pt-BR.md": {"### Modo de revisão manual (`-manual`)", "### 4. Testar combinações de modelos"},
	}
	for file, h := range heads {
		t.Run(file, func(t *testing.T) {
			c := readRepoFile(t, file)
			start, next := strings.Index(c, h[0]), strings.Index(c, h[1])
			if start < 0 || next < 0 || start > next {
				t.Fatalf("%s: a seção %q deve existir antes de %q", file, h[0], h[1])
			}
			sec := c[start:next]
			for _, want := range []string{
				`makercycle -task "`, "-manual", "makercycle -resume -manual",
				"[Enter]", "[c]", "[r]", "[s]", "[q]", "ABORTED", "RESUME.env",
			} {
				if !strings.Contains(sec, want) {
					t.Errorf("%s: seção -manual sem %q", file, want)
				}
			}
			if !strings.Contains(c, "| `6` |") {
				t.Errorf("%s: tabela de códigos de saída sem a linha do código `6`", file)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 9 — E2E: o binário aceita -manual em qualquer posição
// ---------------------------------------------------------------------------

// MR-T9-H1: -version -manual não é recusado pelo parser.
func TestE2EManualFlagAcceptedWithVersion(t *testing.T) {
	bin := buildE2EBinary(t)
	r := runE2E(t, bin, t.TempDir(), t.TempDir(), "", "-version", "-manual")
	if r.Code != 0 || strings.TrimSpace(r.Out) != "makercycle v0.1.0" {
		t.Fatalf("-version -manual: código %d\n%s", r.Code, r.Out)
	}
}

// MR-T9-H2: -manual depois do valor de -task e junto de -resume não gera "flag provided but not defined".
func TestE2EManualFlagAcceptedAfterTaskAndResume(t *testing.T) {
	bin := buildE2EBinary(t)
	for _, args := range [][]string{
		{"-task", "fazer x", "-manual"},
		{"-task", "-manual", "fazer x"},
		{"-resume", "-manual"},
	} {
		r := runE2E(t, bin, t.TempDir(), t.TempDir(), "", args...)
		if strings.Contains(r.Out, "flag provided but not defined") || r.Code == 2 {
			t.Errorf("%q recusado pelo parser (código %d):\n%s", args, r.Code, r.Out)
		}
	}
}
