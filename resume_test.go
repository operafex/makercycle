package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 5 — resolveStage aceita 1..5 (ver também TestResolveStage em makercycle_test.go)
// ---------------------------------------------------------------------------

func TestResolveStageAcceptsOneToFive(t *testing.T) {
	for v := 1; v <= 5; v++ {
		got, err := resolveStage(v)
		if err != nil || got != v {
			t.Fatalf("RS-H%d: resolveStage(%d) = %d, %v", v, v, got, err)
		}
	}
	for _, v := range []int{0, 6, -1} {
		if _, err := resolveStage(v); err == nil {
			t.Fatalf("RS-X: etapa %d deveria ser rejeitada", v)
		} else if !strings.Contains(err.Error(), "1 a 5") {
			t.Fatalf("mensagem deve orientar \"1 a 5\": %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 5 — shouldRunStage(stage, startStage, attempt, firstAttempt int) bool
// ---------------------------------------------------------------------------

func TestShouldRunStage(t *testing.T) {
	cases := []struct {
		id                                   string
		stage, startStage, attempt, firstAtt int
		want                                 bool
	}{
		{"SR-H1", 1, 1, 1, 1, true},
		{"SR-H2", 3, 1, 1, 1, true},
		{"SR-H3", 3, 4, 1, 1, false}, // retomada na Etapa 4: pula MAKER na 1ª rodada
		{"SR-H4", 4, 4, 1, 1, true},
		{"SR-H5", 4, 5, 2, 2, false}, // retomada na Etapa 5 (tentativa 2): pula AUDITOR
		{"SR-H6", 5, 5, 2, 2, true},
		{"SR-E1", 3, 4, 2, 1, true}, // rodadas seguintes executam tudo
		{"SR-E2", 4, 5, 3, 2, true},
		{"SR-E3", 1, 2, 1, 1, false},
		{"SR-E4", 2, 3, 1, 1, false},
		{"SR-E5", 2, 2, 1, 1, true},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := shouldRunStage(c.stage, c.startStage, c.attempt, c.firstAtt); got != c.want {
				t.Fatalf("shouldRunStage(%d,%d,%d,%d) = %v; want %v", c.stage, c.startStage, c.attempt, c.firstAtt, got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 6 — ResumeState: writeResumeState / loadResumeState / clearResumeState
// ---------------------------------------------------------------------------

func TestResumeStateRoundTrip(t *testing.T) {
	// RE-H1
	p := filepath.Join(t.TempDir(), "state", "RESUME.env")
	want := ResumeState{Stage: 4, Demand: "docs/demands/2026-11-17-x.md", Stack: "Go", Attempt: 2}
	if err := writeResumeState(p, want); err != nil {
		t.Fatalf("writeResumeState: %v", err)
	}
	c := readRepoFile(t, p)
	for _, s := range []string{`MC_RESUME_STAGE="4"`, `MC_RESUME_DEMAND="docs/demands/2026-11-17-x.md"`,
		`MC_RESUME_STACK="Go"`, `MC_RESUME_ATTEMPT="2"`} {
		if !strings.Contains(c, s) {
			t.Fatalf("RESUME.env sem %q:\n%s", s, c)
		}
	}
	got, err := loadResumeState(p)
	if err != nil || got != want {
		t.Fatalf("loadResumeState = %+v, %v; want %+v", got, err, want)
	}
}

func TestResumeStatePathConstant(t *testing.T) {
	if resumeEnvPath != ".makercycle/state/RESUME.env" {
		t.Fatalf("resumeEnvPath = %q", resumeEnvPath)
	}
}

func TestWriteResumeStateOverwritesPrevious(t *testing.T) {
	// RE-E1: segunda gravação substitui valores sem duplicar chaves.
	p := filepath.Join(t.TempDir(), "RESUME.env")
	_ = writeResumeState(p, ResumeState{Stage: 3, Stack: "Go", Attempt: 1})
	if err := writeResumeState(p, ResumeState{Stage: 5, Stack: "Go", Attempt: 3}); err != nil {
		t.Fatal(err)
	}
	c := readRepoFile(t, p)
	if strings.Count(c, "MC_RESUME_STAGE=") != 1 || !strings.Contains(c, `MC_RESUME_STAGE="5"`) {
		t.Fatalf("chave duplicada ou não atualizada:\n%s", c)
	}
}

func TestWriteResumeStateRejectsInvalid(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]ResumeState{
		"RE-X1 etapa 0":          {Stage: 0, Stack: "Go", Attempt: 1},
		"RE-X2 etapa 6":          {Stage: 6, Stack: "Go", Attempt: 1},
		"RE-X3 aspas na demanda": {Stage: 3, Demand: `docs/x".md`, Stack: "Go", Attempt: 1},
		"RE-X4 newline na stack": {Stage: 3, Stack: "Go\nMC_X=1", Attempt: 1},
		"RE-X5 tentativa 0":      {Stage: 3, Stack: "Go", Attempt: 0},
	}
	for name, st := range cases {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".env")
			if err := writeResumeState(p, st); err == nil {
				t.Fatal("esperado erro")
			}
			if _, err := os.Stat(p); err == nil {
				t.Fatal("arquivo não deveria ser criado em caso de erro")
			}
		})
	}
}

func TestLoadResumeStateErrors(t *testing.T) {
	dir := t.TempDir()
	if _, err := loadResumeState(filepath.Join(dir, "nao-existe.env")); err == nil {
		t.Fatal("RE-X6: arquivo ausente deve gerar erro")
	} else if !strings.Contains(err.Error(), "-stage") {
		t.Fatalf("RE-X6: erro deve sugerir -stage/-demand: %v", err)
	}
	bad := map[string]string{
		"RE-X7 etapa inválida":     "MC_RESUME_STAGE=\"9\"\nMC_RESUME_STACK=\"Go\"\n",
		"RE-X8 etapa não numérica": "MC_RESUME_STAGE=\"abc\"\nMC_RESUME_STACK=\"Go\"\n",
		"RE-X9 tentativa inválida": "MC_RESUME_STAGE=\"3\"\nMC_RESUME_STACK=\"Go\"\nMC_RESUME_ATTEMPT=\"x\"\n",
		"RE-X10 sem etapa":         "MC_RESUME_STACK=\"Go\"\n",
	}
	for name, content := range bad {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(name, " ", "_")+".env")
			_ = os.WriteFile(p, []byte(content), 0o644)
			if _, err := loadResumeState(p); err == nil {
				t.Fatal("esperado erro")
			}
		})
	}
}

func TestLoadResumeStateDefaultsAttempt(t *testing.T) {
	// RE-E2: MC_RESUME_ATTEMPT ausente => 1.
	p := filepath.Join(t.TempDir(), "RESUME.env")
	_ = os.WriteFile(p, []byte("MC_RESUME_STAGE=\"2\"\nMC_RESUME_STACK=\"Go\"\n"), 0o644)
	got, err := loadResumeState(p)
	if err != nil || got != (ResumeState{Stage: 2, Stack: "Go", Attempt: 1}) {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestClearResumeState(t *testing.T) {
	p := filepath.Join(t.TempDir(), "RESUME.env")
	_ = writeResumeState(p, ResumeState{Stage: 3, Stack: "Go", Attempt: 1})
	if err := clearResumeState(p); err != nil {
		t.Fatalf("RE-H2: %v", err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("RE-H2: RESUME.env deveria ter sido removido")
	}
	if err := clearResumeState(p); err != nil {
		t.Fatalf("RE-E3: remover arquivo inexistente não é erro: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Task 6 — resumeStack(name, envPath, root string) (StackConfig, error)
// ---------------------------------------------------------------------------

func TestResumeStack(t *testing.T) {
	root := t.TempDir()
	envPath := writeEnv(t, "MC_STACK_NAME=\"NestJS + Jest\"\nMC_TEST_CMD=\"npm run test\"\nMC_LINT_CMD=\"npm run lint\"\n")
	got, err := resumeStack("NestJS + Jest", envPath, root)
	if err != nil {
		t.Fatalf("RK-H1: %v", err)
	}
	assertStack(t, got, StackConfig{Name: "NestJS + Jest", TestCmd: "npm run test", LintCmd: "npm run lint"})

	got, err = resumeStack("Go", envPath, root)
	if err != nil {
		t.Fatalf("RK-H2: %v", err)
	}
	assertStack(t, got, wantGo)

	got, err = resumeStack("Go", filepath.Join(root, "ausente.env"), root)
	if err != nil {
		t.Fatalf("RK-E1 (project.env ausente): %v", err)
	}
	assertStack(t, got, wantGo)

	for _, name := range []string{"", "Cobol"} {
		if _, err := resumeStack(name, envPath, root); err == nil {
			t.Fatalf("RK-X: stack %q deveria falhar", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 7 — resolveDemandFlag(path, docsDir string) (string, error) e demandTitle(path string) (string, error)
// ---------------------------------------------------------------------------

func setupDemands(t *testing.T) string {
	t.Helper()
	chdirTemp(t)
	if err := os.MkdirAll("docs/demands", 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile("docs/demands/2026-11-17-x.md", []byte("# Demanda: Fazer X\n\n- **Status:** `IN_PROGRESS`\n"), 0o644)
	_ = os.WriteFile("docs/demands/INDEX.md", []byte("# Índice\n"), 0o644)
	_ = os.WriteFile("docs/outside.md", []byte("# Demanda: fora\n"), 0o644)
	return "docs/demands/2026-11-17-x.md"
}

func TestResolveDemandFlag(t *testing.T) {
	want := setupDemands(t)
	for _, in := range []string{want, "./" + want, "2026-11-17-x.md", "  " + want + "  "} {
		got, err := resolveDemandFlag(in, "docs")
		if err != nil || got != want {
			t.Fatalf("DM-H: resolveDemandFlag(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "docs/demands/nao-existe.md", "docs/demands/INDEX.md",
		"docs/outside.md", "docs/demands/../outside.md", "docs/demands", "/etc/passwd"} {
		if _, err := resolveDemandFlag(in, "docs"); err == nil {
			t.Fatalf("DM-X: resolveDemandFlag(%q) deveria falhar", in)
		}
	}
}

func TestResolveDemandFlagDoesNotTouchIndex(t *testing.T) {
	// DM-E1: vincular demanda existente não cria arquivo nem duplica linha no INDEX.md.
	want := setupDemands(t)
	before := readRepoFile(t, "docs/demands/INDEX.md")
	entries, _ := os.ReadDir("docs/demands")
	if _, err := resolveDemandFlag(want, "docs"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadDir("docs/demands")
	if readRepoFile(t, "docs/demands/INDEX.md") != before || len(after) != len(entries) {
		t.Fatal("resolveDemandFlag não pode alterar docs/demands/")
	}
}

func TestDemandTitle(t *testing.T) {
	p := setupDemands(t)
	got, err := demandTitle(p)
	if err != nil || got != "Fazer X" {
		t.Fatalf("DT-H1: demandTitle = %q, %v", got, err)
	}
	_ = os.WriteFile("docs/demands/sem-titulo.md", []byte("texto livre\n"), 0o644)
	if _, err := demandTitle("docs/demands/sem-titulo.md"); err == nil {
		t.Fatal("DT-X1: arquivo sem '# Demanda:' deve falhar")
	}
	if _, err := demandTitle("docs/demands/nao-existe.md"); err == nil {
		t.Fatal("DT-X2: arquivo ausente deve falhar")
	}
}

// ---------------------------------------------------------------------------
// Task 7 — validateRunFlags(task, demand string, resume, stageSet bool) error
// ---------------------------------------------------------------------------

func TestValidateRunFlags(t *testing.T) {
	cases := []struct {
		id            string
		task, demand  string
		resume, stage bool
		wantErr       bool
	}{
		{"VF-H1", "nova", "", false, false, false},
		{"VF-H2", "", "docs/demands/x.md", false, true, false},
		{"VF-H3", "", "", true, false, false},
		{"VF-H4", "", "", false, true, false},
		{"VF-X1", "nova", "docs/demands/x.md", false, false, true},
		{"VF-X2", "nova", "", true, false, true},
		{"VF-X3", "", "docs/demands/x.md", true, false, true},
		{"VF-X4", "", "", true, true, true},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			err := validateRunFlags(c.task, c.demand, c.resume, c.stage)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateRunFlags = %v; wantErr %v", err, c.wantErr)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 6 — quotaInterruptionReport(st ResumeState, stageLabel, eta string) string e exitQuota
// ---------------------------------------------------------------------------

func TestQuotaInterruptionReport(t *testing.T) {
	st := ResumeState{Stage: 3, Demand: "docs/demands/2026-11-17-x.md", Stack: "Go", Attempt: 2}
	eta := "45m10s (liberação prevista em 17/11/2026 às 22:45)"
	got := quotaInterruptionReport(st, "ETAPA 3: MAKE", eta)
	for _, s := range []string{
		"Etapa interrompida: ETAPA 3: MAKE",
		"Tentativa atual: 2",
		"Motivo: bloqueio por limite de tokens/cota",
		"Tempo para continuar: " + eta,
		"./makercycle -resume",
		"./makercycle -stage 3 -demand docs/demands/2026-11-17-x.md",
	} {
		if !strings.Contains(got, s) {
			t.Fatalf("QR-H1: relatório sem %q:\n%s", s, got)
		}
	}
}

func TestQuotaInterruptionReportWithoutDemand(t *testing.T) {
	// QR-E1: sem demanda vinculada => comando sem -demand.
	got := quotaInterruptionReport(ResumeState{Stage: 5, Stack: "Go", Attempt: 1}, "ETAPA 5: GATE", "tempo de reset não informado pelo provedor")
	if !strings.Contains(got, "./makercycle -stage 5") || strings.Contains(got, "-demand") {
		t.Fatalf("relatório inesperado:\n%s", got)
	}
	if !strings.Contains(got, "tempo de reset não informado pelo provedor") {
		t.Fatalf("ETA ausente:\n%s", got)
	}
}

func TestExitQuotaCode(t *testing.T) {
	if exitQuota != 5 {
		t.Fatalf("exitQuota = %d; want 5", exitQuota)
	}
}
