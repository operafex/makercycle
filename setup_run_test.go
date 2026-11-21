package main

import (
	"bufio"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 3 — runProjectSetupWith: fluxo do -setup com telemetria S0 em sucesso e erro
// ---------------------------------------------------------------------------

// Garante em tempo de compilação que runScoutAgent devolve os tokens do SCOUT.
var _ scoutRunner = runScoutAgent

var setupScout = AgentBinding{Label: "ETAPA 0: SCAN", Alias: "SCOUT", Bin: "claude", Model: "opus"}

type scoutCall struct {
	docsDir, existingDocs, detectedName string
	jsonMetrics                         bool
}

// closeGlobalRunLog fecha o log singleton aberto por startRunLog dentro do fluxo testado.
func closeGlobalRunLog(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if runLog != nil {
			runLog.Close()
			runLog = nil
		}
	})
}

// captureStdout executa fn redirecionando os.Stdout e devolve o que foi impresso.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stdout = orig }()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func writeGoMod(t *testing.T) {
	t.Helper()
	if err := os.WriteFile("go.mod", []byte("module x\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestRunProjectSetupWithSuccessRecordsS0Telemetry(t *testing.T) {
	chdirTemp(t)
	closeGlobalRunLog(t)
	writeGoMod(t)
	now, advance := fakeClock(setupT0)
	var calls []scoutCall
	runScout := func(scout AgentBinding, docsDir, existingDocs, detectedName string, jsonMetrics bool) (TokenStats, error) {
		calls = append(calls, scoutCall{docsDir, existingDocs, detectedName, jsonMetrics})
		advance(42 * time.Second)
		return TokenStats{Input: 100, Output: 20, Total: 120}, nil
	}
	reader := bufio.NewReader(strings.NewReader("\n\n")) // Enter = docs padrão; Enter = confirma a stack Go

	var out setupOutcome
	var err error
	stdout := captureStdout(t, func() {
		out, err = runProjectSetupWith(reader, setupScout, "docs", true, runScout, now)
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(calls) != 1 || calls[0] != (scoutCall{"docs", "", "Go", true}) {
		t.Fatalf("SCOUT deve ser chamado uma vez com (docs, \"\", Go, true): %+v", calls)
	}
	if out.Status != "SETUP_OK" || out.Stack != "Go" || out.DocsDir != "docs" || out.RunID == "" {
		t.Errorf("setupOutcome incorreto: %+v", out)
	}

	ev := readCSV(t, eventsPathT())
	if len(ev) != 2 {
		t.Fatalf("esperava 1 evento S0, veio %v", ev)
	}
	if e := ev[1]; e[0] != out.RunID || e[2] != "S0" || e[3] != "SCOUT" || e[4] != "opus" || e[5] != "1" ||
		e[6] != "100" || e[7] != "20" || e[8] != "120" || e[9] != "42.000" {
		t.Errorf("evento S0 incorreto: %v", e)
	}

	runs := readCSV(t, runsPathT())
	if len(runs) != 2 {
		t.Fatalf("esperava 1 execução, veio %v", runs)
	}
	r := runs[1]
	if r[0] != out.RunID || r[2] != "Go" || r[3] != "(setup / SCAN)" || r[4] != "1" || r[5] != "1" ||
		r[8] != "120" || r[9] != "SETUP_OK" || r[10] != "SCOUT: SUCCESS | Duração total: 42s" {
		t.Errorf("execução incorreta: %v", r)
	}

	md := readString(t, telemetryMDPath)
	if !strings.Contains(md, "| "+out.RunID+" | SETUP_OK | (setup / SCAN) | Go | 1/1 | 42s | 120 | n/a | n/a | n/a | n/a | n/a |") ||
		!strings.Contains(md, "| Tokens E0 SCAN | 120 |") {
		t.Errorf("TELEMETRY.md não reflete o -setup:\n%s", md)
	}

	if !strings.Contains(stdout, `Dispare novas demandas com: makercycle -task "..."`) {
		t.Errorf("o banner final do -setup deve sugerir `makercycle -task`, não `go run .`:\n%s", stdout)
	}
	if strings.Contains(stdout, "go run .") {
		t.Errorf("nenhuma instrução do -setup pode sugerir `go run .`:\n%s", stdout)
	}
	if _, statErr := os.Stat(filepath.Join("docs", "logs")); statErr != nil {
		t.Errorf("o log da Etapa 0 deve continuar sendo criado: %v", statErr)
	}
}

func TestRunProjectSetupWithScoutErrorRecordsErrorTelemetry(t *testing.T) {
	chdirTemp(t)
	closeGlobalRunLog(t)
	writeGoMod(t)
	now, advance := fakeClock(setupT0)
	runScout := func(AgentBinding, string, string, string, bool) (TokenStats, error) {
		advance(7 * time.Second)
		return TokenStats{Input: 10, Output: 5, Total: 15}, errors.New("Falha na Etapa 0 (SCOUT): boom")
	}
	var out setupOutcome
	var err error
	captureStdout(t, func() {
		out, err = runProjectSetupWith(bufio.NewReader(strings.NewReader("\n\n")), setupScout, "docs", false, runScout, now)
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("o erro do SCOUT deve ser propagado, veio %v", err)
	}
	if out.Status != "ERROR" || out.Stack != "Go" || out.RunID == "" {
		t.Errorf("setupOutcome incorreto: %+v", out)
	}
	ev := readCSV(t, eventsPathT())
	if len(ev) != 2 || ev[1][0] != out.RunID || ev[1][2] != "S0" || ev[1][8] != "15" || ev[1][9] != "7.000" {
		t.Errorf("o evento S0 deve ser gravado mesmo com erro: %v", ev)
	}
	r := readCSV(t, runsPathT())[1]
	if r[0] != out.RunID || r[9] != "ERROR" || r[8] != "15" ||
		r[10] != "SCOUT: ERROR | Duração total: 7s | Erro: Falha na Etapa 0 (SCOUT): boom" {
		t.Errorf("execução com erro incorreta: %v", r)
	}
	if !strings.Contains(readString(t, telemetryMDPath), "| "+out.RunID+" | ERROR | (setup / SCAN) | Go | 1/1 | 7s | 15 |") {
		t.Errorf("TELEMETRY.md deve ser regenerado no erro do SCOUT")
	}
}

func TestRunProjectSetupWithDocsDirErrorRecordsRunWithoutEvent(t *testing.T) {
	chdirTemp(t)
	closeGlobalRunLog(t)
	if err := os.WriteFile("bloqueio", []byte("arquivo, não pasta"), 0644); err != nil {
		t.Fatal(err)
	}
	called := false
	runScout := func(AgentBinding, string, string, string, bool) (TokenStats, error) {
		called = true
		return TokenStats{}, nil
	}
	now, _ := fakeClock(setupT0)
	var out setupOutcome
	var err error
	captureStdout(t, func() {
		out, err = runProjectSetupWith(bufio.NewReader(strings.NewReader("bloqueio\n")), setupScout, "docs", false, runScout, now)
	})
	if err == nil || !strings.Contains(err.Error(), "Erro ao criar estrutura de pastas em bloqueio") {
		t.Fatalf("esperava erro de estrutura de pastas, veio %v", err)
	}
	if called {
		t.Errorf("o SCOUT não pode rodar quando a estrutura falha")
	}
	if out.Status != "ERROR" || out.Stack != "(não detectada)" {
		t.Errorf("setupOutcome incorreto: %+v", out)
	}
	if _, statErr := os.Stat(eventsPathT()); statErr == nil {
		t.Errorf("sem SCOUT não deve haver evento S0")
	}
	r := readCSV(t, runsPathT())[1]
	if r[9] != "ERROR" || r[2] != "(não detectada)" || r[3] != "(setup / SCAN)" ||
		!strings.HasPrefix(r[10], "SCOUT: NOT_RUN | Duração total: 0s | Erro: Erro ao criar estrutura de pastas em bloqueio") {
		t.Errorf("execução incorreta: %v", r)
	}
	if _, statErr := os.Stat(telemetryMDPath); statErr != nil {
		t.Errorf("TELEMETRY.md deve ser gerado também no erro: %v", statErr)
	}
}

func TestRunProjectSetupWithInvalidStackAnswerRecordsError(t *testing.T) {
	chdirTemp(t)
	closeGlobalRunLog(t)
	writeGoMod(t)
	called := false
	runScout := func(AgentBinding, string, string, string, bool) (TokenStats, error) {
		called = true
		return TokenStats{}, nil
	}
	now, _ := fakeClock(setupT0)
	var out setupOutcome
	var err error
	captureStdout(t, func() {
		out, err = runProjectSetupWith(bufio.NewReader(strings.NewReader("\nX\n")), setupScout, "docs", false, runScout, now)
	})
	if err == nil || !strings.Contains(err.Error(), "Erro ao gravar") || !strings.Contains(err.Error(), "opção inválida") {
		t.Fatalf("esperava erro da confirmação de stack, veio %v", err)
	}
	if called {
		t.Errorf("o SCOUT não pode rodar sem stack confirmada")
	}
	r := readCSV(t, runsPathT())[1]
	if out.Status != "ERROR" || r[9] != "ERROR" || r[2] != "(não detectada)" || !strings.HasPrefix(r[10], "SCOUT: NOT_RUN |") {
		t.Errorf("execução incorreta: %+v / %v", out, r)
	}
}

func TestRunProjectSetupWithoutDetectedStackStillSucceeds(t *testing.T) {
	chdirTemp(t)
	closeGlobalRunLog(t)
	var gotDetected = "sentinela"
	runScout := func(_ AgentBinding, _, _, detectedName string, _ bool) (TokenStats, error) {
		gotDetected = detectedName
		return TokenStats{Total: 3}, nil
	}
	now, _ := fakeClock(setupT0)
	var out setupOutcome
	var err error
	captureStdout(t, func() {
		out, err = runProjectSetupWith(bufio.NewReader(strings.NewReader("\n")), setupScout, "docs", false, runScout, now)
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if gotDetected != "" {
		t.Errorf("sem stack detectada o SCOUT recebe detectedName vazio, veio %q", gotDetected)
	}
	r := readCSV(t, runsPathT())[1]
	if out.Status != "SETUP_OK" || r[9] != "SETUP_OK" || r[2] != "(não detectada)" || r[8] != "3" {
		t.Errorf("execução incorreta: %+v / %v", out, r)
	}
}

func TestRunProjectSetupWithStackFromProjectEnvWins(t *testing.T) {
	// O SCOUT pode ajustar project.env; a stack registrada é a salva ao final do -setup.
	chdirTemp(t)
	closeGlobalRunLog(t)
	writeGoMod(t)
	runScout := func(AgentBinding, string, string, string, bool) (TokenStats, error) {
		err := writeProjectEnv(projectEnvPath, map[string]string{
			"MC_STACK_NAME": "Go Custom", "MC_TEST_CMD": "go test ./...", "MC_TYPE_CMD": "", "MC_LINT_CMD": "",
		})
		return TokenStats{Total: 1}, err
	}
	now, _ := fakeClock(setupT0)
	var out setupOutcome
	captureStdout(t, func() {
		out, _ = runProjectSetupWith(bufio.NewReader(strings.NewReader("\n\n")), setupScout, "docs", false, runScout, now)
	})
	if out.Stack != "Go Custom" || readCSV(t, runsPathT())[1][2] != "Go Custom" {
		t.Errorf("a stack salva em project.env deve prevalecer: %+v", out)
	}
}

// ---------------------------------------------------------------------------
// Task 4 — makercycle.go delega o -setup ao fluxo com telemetria
// ---------------------------------------------------------------------------

func TestRunProjectSetupDelegatesToTelemetryAwareFlow(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	call := "runProjectSetupWith(reader, scout, defaultDocs, jsonMetrics, runScoutAgent, time.Now)"
	if !strings.Contains(src, call) {
		t.Fatalf("runProjectSetup deve delegar para %s", call)
	}
	if strings.Contains(src, "runScoutAgent(scout, targetDocsDir") {
		t.Errorf("makercycle.go não deve mais chamar runScoutAgent diretamente no -setup")
	}
}

func TestGoSourcesDoNotSuggestGoRunToUsers(t *testing.T) {
	for _, file := range []string{"makercycle.go", "stack.go", "init.go", "init_run.go", "setup_run.go", "setup_telemetry.go"} {
		c := readRepoFile(t, file)
		for _, s := range []string{"go run . -setup", "go run . -task"} {
			if strings.Contains(c, s) {
				t.Errorf("%s sugere %q; use `makercycle %s`", file, s, strings.TrimPrefix(s, "go run . "))
			}
		}
	}
}
