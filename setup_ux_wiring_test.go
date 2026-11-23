package main

import (
	"bufio"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 8 — SCOUT em projeto novo e contexto MC_DATABASE (init_run.go)
// ---------------------------------------------------------------------------

// WR-H1: sem banco e sem projeto novo, o prompt do SCOUT é exatamente o legado.
func TestBuildScoutPromptForLegacyWhenNothingToAdd(t *testing.T) {
	if got, want := buildScoutPromptFor("wiki", "wiki", "Go", "", false), buildScoutPrompt("wiki", "wiki", "Go"); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// WR-H2: com banco definido, o SCOUT recebe MC_DATABASE.
func TestBuildScoutPromptForInjectsDatabase(t *testing.T) {
	base := buildScoutPrompt("", "docs", "Go")
	if got, want := buildScoutPromptFor("", "docs", "Go", "SQLite", false), withDatabaseContext(base, "SQLite"); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// WR-H3: em projeto novo, o SCOUT é instruído a gerar PROJECT_CONTEXT.md inicial com stack e banco, sem falhar por diretório vazio.
func TestBuildScoutPromptForGreenfield(t *testing.T) {
	base := withDatabaseContext(buildScoutPrompt("", "docs", "Rust"), "PostgreSQL")
	got := buildScoutPromptFor("", "docs", "Rust", "PostgreSQL", true)
	if !strings.HasPrefix(got, base) {
		t.Fatalf("o prompt de projeto novo deve estender o prompt com banco:\n%q", got)
	}
	for _, s := range []string{"Projeto novo", "Não falhe por diretório vazio", "docs/PROJECT_CONTEXT.md", "'Rust'", "'PostgreSQL'"} {
		if !strings.Contains(got, s) {
			t.Errorf("prompt de projeto novo sem %q:\n%s", s, got)
		}
	}
}

// WR-E1: projeto novo sem stack nem banco escolhidos usa "(a definir)".
func TestBuildScoutPromptForGreenfieldUndefined(t *testing.T) {
	got := buildScoutPromptFor("", "docs", "", "", true)
	if strings.Count(got, "'(a definir)'") != 2 {
		t.Errorf("stack e banco indefinidos devem aparecer como '(a definir)':\n%s", got)
	}
	if strings.Contains(got, "MC_DATABASE): ") {
		t.Errorf("sem banco não há linha de MC_DATABASE:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Task 9 — -setup: presets + banco (setup_run.go)
// ---------------------------------------------------------------------------

func runSetupFlow(t *testing.T, input string) (setupOutcome, string, []string, error) {
	t.Helper()
	closeGlobalRunLog(t)
	t.Setenv("MC_DATABASE", "")
	orig := defaultPromptTimeout
	defaultPromptTimeout = 200 * time.Millisecond
	t.Cleanup(func() { defaultPromptTimeout = orig })
	var detected []string
	runScout := func(_ AgentBinding, _, _, detectedName string, _ bool) (TokenStats, error) {
		detected = append(detected, detectedName)
		return TokenStats{Input: 1, Output: 1, Total: 2}, nil
	}
	now, _ := fakeClock(setupT0)
	var out setupOutcome
	var err error
	stdout := captureStdout(t, func() {
		out, err = runProjectSetupWith(bufio.NewReader(strings.NewReader(input)), setupScout, "docs", false, runScout, now)
	})
	return out, stdout, detected, err
}

// WR-H4: projeto novo — menu de presets, depois menu de banco; tudo gravado em project.env.
func TestRunProjectSetupGreenfieldPresetAndDatabase(t *testing.T) {
	chdirTemp(t)
	out, stdout, detected, err := runSetupFlow(t, "\n1\n2\n") // docs padrão; preset Go; SQLite
	if err != nil {
		t.Fatalf("erro inesperado: %v\n%s", err, stdout)
	}
	if !strings.Contains(stdout, stackPresetMenuTitle) || !strings.Contains(stdout, databaseMenuTitle) {
		t.Errorf("projeto novo deve exibir presets e banco:\n%s", stdout)
	}
	env := parseEnvContent(readString(t, projectEnvPath))
	for k, v := range map[string]string{
		"MC_STACK_NAME": "Go", "MC_BUILD_CMD": "go build ./...", "MC_TEST_CMD": "go test -race ./...",
		"MC_LINT_CMD": "go vet ./...", "MC_DATABASE": "SQLite",
	} {
		if env[k] != v {
			t.Errorf("project.env %s = %q, want %q", k, env[k], v)
		}
	}
	if len(detected) != 1 || detected[0] != "Go" || out.Stack != "Go" || out.Status != "SETUP_OK" {
		t.Errorf("SCOUT/telemetria incorretos: detected=%v out=%+v", detected, out)
	}
}

// WR-E2: projeto novo não-interativo (EOF) — sem preset, banco decidido pelo contexto e persistido, sem falhar.
func TestRunProjectSetupGreenfieldNonInteractive(t *testing.T) {
	chdirTemp(t)
	out, stdout, _, err := runSetupFlow(t, "\n")
	if err != nil || out.Status != "SETUP_OK" {
		t.Fatalf("o -setup não-interativo não pode falhar: %+v err=%v\n%s", out, err, stdout)
	}
	if !strings.Contains(stdout, "[AVISO]") {
		t.Errorf("fallback do banco deve avisar:\n%s", stdout)
	}
	if env := parseEnvContent(readString(t, projectEnvPath)); env["MC_DATABASE"] != "SQLite" {
		t.Errorf("MC_DATABASE = %q, want SQLite (CLI/ferramenta local sem stack)", env["MC_DATABASE"])
	}
}

// WR-H5: projeto existente sem MC_DATABASE — a pergunta de banco aparece após a confirmação da stack.
func TestRunProjectSetupExistingWithoutDatabaseAsks(t *testing.T) {
	chdirTemp(t)
	writeGoMod(t)
	_, stdout, _, err := runSetupFlow(t, "\n\n4\n") // docs; confirma Go; MongoDB
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !strings.Contains(stdout, databaseMenuTitle) {
		t.Errorf("sem MC_DATABASE o -setup deve perguntar o banco:\n%s", stdout)
	}
	if env := parseEnvContent(readString(t, projectEnvPath)); env["MC_DATABASE"] != "MongoDB" || env["MC_STACK_NAME"] != "Go" {
		t.Errorf("project.env = %v", env)
	}
}

// WR-E3: projeto existente com MC_DATABASE definido — sem pergunta de banco e valor preservado.
func TestRunProjectSetupExistingWithDatabaseSkips(t *testing.T) {
	chdirTemp(t)
	writeGoMod(t)
	writeFiles(t, ".", map[string]string{".makercycle/config/project.env": "MC_DATABASE=\"PostgreSQL\"\n"})
	_, stdout, _, err := runSetupFlow(t, "\n\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if strings.Contains(stdout, databaseMenuTitle) {
		t.Errorf("com MC_DATABASE definido não deve perguntar:\n%s", stdout)
	}
	if env := parseEnvContent(readString(t, projectEnvPath)); env["MC_DATABASE"] != "PostgreSQL" {
		t.Errorf("MC_DATABASE deve ser preservado: %v", env)
	}
}

// ---------------------------------------------------------------------------
// Task 10 — fiação em makercycle.go / init_run.go / setup_run.go (contratos de código-fonte)
// ---------------------------------------------------------------------------

func assertSourceContains(t *testing.T, file string, wants ...string) {
	t.Helper()
	src := readRepoFile(t, file)
	for _, w := range wants {
		if !strings.Contains(src, w) {
			t.Errorf("%s deve conter %q", file, w)
		}
	}
}

// WR-H6: resumo por etapa impresso ao fim de cada etapa do -task e do SCAN (E0 incluída no acumulado).
func TestStageSummaryWiring(t *testing.T) {
	assertSourceContains(t, "makercycle.go",
		"tracker := newStageTracker(os.Stdout)",
		`tracker.Finish("S1", `, `tracker.Finish("S2", `, `tracker.Finish("S3", `,
		`tracker.Finish("S4B", `, `tracker.Finish("S5", `)
	assertSourceContains(t, "init_run.go", `newStageTracker(os.Stdout).Finish("S0", `)
}

// WR-H7: pergunta de banco no -setup e antes da Etapa 1 do -task; MC_DATABASE nos prompts dos agentes.
func TestDatabaseWiring(t *testing.T) {
	assertSourceContains(t, "setup_run.go",
		"shouldPromptDatabaseOnSetup(", "chooseDatabase(reader, os.Stdout, projectEnvPath,", "defaultPromptTimeout")
	assertSourceContains(t, "makercycle.go",
		"shouldPromptDatabaseOnTask(", "chooseDatabase(reader, os.Stdout, projectEnvPath,", "defaultPromptTimeout",
		"withDatabaseContext(buildPlannerPrompt(", "withDatabaseContext(buildMakerPrompt(",
		"withDatabaseContext(buildGatekeeperPrompt(")
	assertSourceContains(t, "init_run.go", "buildScoutPromptFor(", "currentDatabase(projectEnvPath, os.Getenv)", `isGreenfieldProject(".")`)
}
