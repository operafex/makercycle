package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 1 — setup_telemetry.go: constantes, rótulos e nota da execução do -setup
// ---------------------------------------------------------------------------

func TestSetupTelemetryConstants(t *testing.T) {
	cases := map[string][2]string{
		"setupStageCode":      {setupStageCode, "S0"},
		"setupAgentName":      {setupAgentName, "SCOUT"},
		"setupDemandLabel":    {setupDemandLabel, "(setup / SCAN)"},
		"setupRunStatusOK":    {setupRunStatusOK, "SETUP_OK"},
		"setupRunStatusError": {setupRunStatusError, "ERROR"},
		"scoutEventSuccess":   {scoutEventSuccess, "SUCCESS"},
		"scoutEventError":     {scoutEventError, "ERROR"},
		"scoutEventNotRun":    {scoutEventNotRun, "NOT_RUN"},
		"setupStackUnknown":   {setupStackUnknown, "(não detectada)"},
	}
	for name, c := range cases {
		if c[0] != c[1] {
			t.Errorf("%s = %q; want %q", name, c[0], c[1])
		}
	}
}

func TestSetupStackLabel(t *testing.T) {
	loadErr := errors.New("project.env sem stack")
	cases := []struct {
		id       string
		detected string
		saved    StackConfig
		err      error
		want     string
	}{
		{"ST-H1 stack salva prevalece", "Go", StackConfig{Name: "Ruby on Rails"}, nil, "Ruby on Rails"},
		{"ST-H2 sem project.env usa a detectada", "Go", StackConfig{}, loadErr, "Go"},
		{"ST-E1 nome salvo em branco usa a detectada", "Go", StackConfig{Name: "   "}, nil, "Go"},
		{"ST-E2 nada detectado nem salvo", "", StackConfig{}, loadErr, "(não detectada)"},
		{"ST-E3 detectada em branco", "  ", StackConfig{}, loadErr, "(não detectada)"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := setupStackLabel(c.detected, c.saved, c.err); got != c.want {
				t.Errorf("setupStackLabel(%q, %+v, %v) = %q; want %q", c.detected, c.saved, c.err, got, c.want)
			}
		})
	}
}

func TestSetupRunNote(t *testing.T) {
	cases := []struct {
		id      string
		status  string
		seconds float64
		err     error
		want    string
	}{
		{"NT-H1 sucesso", "SUCCESS", 83, nil, "SCOUT: SUCCESS | Duração total: 1m23s"},
		{"NT-H2 erro do SCOUT", "ERROR", 7, errors.New("Falha na Etapa 0 (SCOUT): boom"),
			"SCOUT: ERROR | Duração total: 7s | Erro: Falha na Etapa 0 (SCOUT): boom"},
		{"NT-E1 SCOUT não executado", "", 0.2, errors.New("Erro ao criar estrutura"),
			"SCOUT: NOT_RUN | Duração total: 0s | Erro: Erro ao criar estrutura"},
		{"NT-E2 erro multilinha vira uma linha", "ERROR", 1, errors.New("linha1\nlinha2\r\nlinha3"),
			"SCOUT: ERROR | Duração total: 1s | Erro: linha1 linha2 linha3"},
		{"NT-E3 duração negativa", "SUCCESS", -5, nil, "SCOUT: SUCCESS | Duração total: 0s"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := setupRunNote(c.status, c.seconds, c.err); got != c.want {
				t.Errorf("setupRunNote = %q; want %q", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 2 — setupTelemetry: evento S0 (events.csv) e execução (runs.csv)
// ---------------------------------------------------------------------------

// fakeClock devolve um relógio controlado pelo teste.
func fakeClock(start time.Time) (now func() time.Time, advance func(time.Duration)) {
	cur := start
	return func() time.Time { return cur }, func(d time.Duration) { cur = cur.Add(d) }
}

var setupT0 = time.Date(2026, 11, 20, 21, 0, 0, 0, time.UTC)

func eventsPathT() string { return filepath.Join(".makercycle", "telemetry", "events.csv") }
func runsPathT() string   { return filepath.Join(".makercycle", "telemetry", "runs.csv") }

func TestNewSetupTelemetry(t *testing.T) {
	chdirTemp(t)
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	if tel == nil {
		t.Fatal("newSetupTelemetry devolveu nil")
	}
	if _, err := strconv.ParseInt(tel.RunID, 10, 64); err != nil || tel.RunID == "" {
		t.Errorf("RunID deve ser gerado por newRunID (numérico), veio %q", tel.RunID)
	}
	if tel.Model != "opus" {
		t.Errorf("Model = %q; want opus", tel.Model)
	}
	if _, err := os.Stat(filepath.Join(".makercycle", "telemetry")); err == nil {
		t.Errorf("newSetupTelemetry não deve gravar nada em disco")
	}
	other := newSetupTelemetry("opus", now)
	if other.RunID == tel.RunID {
		t.Errorf("cada -setup deve ter RunID próprio")
	}
}

func TestNewSetupTelemetryNilClockUsesTimeNow(t *testing.T) {
	chdirTemp(t)
	tel := newSetupTelemetry("m", nil)
	if got := tel.Finish("Go", nil); got != "SETUP_OK" {
		t.Fatalf("Finish com relógio nil deve funcionar, status = %q", got)
	}
}

func TestSetupTelemetryRecordScoutSuccessWritesS0Event(t *testing.T) {
	chdirTemp(t)
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	status := tel.RecordScout(TokenStats{Input: 100, Output: 20, Total: 120}, 42.5, nil)
	if status != "SUCCESS" {
		t.Errorf("status do evento = %q; want SUCCESS", status)
	}
	rows := readCSV(t, eventsPathT())
	if len(rows) != 2 {
		t.Fatalf("esperava cabeçalho + 1 evento, veio %v", rows)
	}
	if got := strings.Join(rows[0], ","); got != strings.Join(eventsCSVHeader, ",") {
		t.Errorf("o schema de events.csv não pode mudar: %q", got)
	}
	r := rows[1]
	want := map[int]string{0: tel.RunID, 2: "S0", 3: "SCOUT", 4: "opus", 5: "1", 6: "100", 7: "20", 8: "120", 9: "42.500"}
	for i, w := range want {
		if r[i] != w {
			t.Errorf("coluna %s = %q; want %q", rows[0][i], r[i], w)
		}
	}
	if _, err := time.Parse(time.RFC3339, r[1]); err != nil {
		t.Errorf("Timestamp não é RFC3339: %q", r[1])
	}
}

func TestSetupTelemetryRecordScoutErrorStillWritesEvent(t *testing.T) {
	chdirTemp(t)
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	if status := tel.RecordScout(TokenStats{Input: 10, Output: 5, Total: 15}, 7, errors.New("boom")); status != "ERROR" {
		t.Errorf("status do evento = %q; want ERROR", status)
	}
	rows := readCSV(t, eventsPathT())
	if len(rows) != 2 || rows[1][2] != "S0" || rows[1][8] != "15" || rows[1][9] != "7.000" {
		t.Errorf("evento S0 com erro deve ser gravado com tokens e duração: %v", rows)
	}
}

func TestSetupTelemetryRecordScoutNegativeDurationIsZero(t *testing.T) {
	chdirTemp(t)
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("m", now)
	tel.RecordScout(TokenStats{}, -3, nil)
	rows := readCSV(t, eventsPathT())
	if rows[1][9] != "0.000" {
		t.Errorf("duração negativa deve virar 0.000, veio %q", rows[1][9])
	}
}

func TestSetupTelemetryFinishSuccessWritesRunAndTelemetryMD(t *testing.T) {
	chdirTemp(t)
	now, advance := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	advance(42 * time.Second)
	tel.RecordScout(TokenStats{Input: 100, Output: 20, Total: 120}, 42, nil)
	advance(1 * time.Second)
	if got := tel.Finish("Go", nil); got != "SETUP_OK" {
		t.Errorf("Finish = %q; want SETUP_OK", got)
	}

	rows := readCSV(t, runsPathT())
	if len(rows) != 2 {
		t.Fatalf("esperava cabeçalho + 1 execução, veio %v", rows)
	}
	want := []string{tel.RunID, "", "Go", "(setup / SCAN)", "1", "1", "100", "20", "120", "SETUP_OK",
		"SCOUT: SUCCESS | Duração total: 43s"}
	for i, w := range want {
		if i == 1 {
			continue
		}
		if rows[1][i] != w {
			t.Errorf("coluna %s = %q; want %q", rows[0][i], rows[1][i], w)
		}
	}

	md := readString(t, telemetryMDPath)
	for _, w := range []string{
		"| Total de Execuções | 1 |",
		"| Tokens E0 SCAN | 120 |",
		"| " + tel.RunID + " | SETUP_OK | (setup / SCAN) | Go | 1/1 | 42s | 120 | n/a | n/a | n/a | n/a | n/a |",
	} {
		if !strings.Contains(md, w) {
			t.Errorf("TELEMETRY.md deve conter %q:\n%s", w, md)
		}
	}
}

func TestSetupTelemetryFinishErrorAfterScout(t *testing.T) {
	chdirTemp(t)
	now, advance := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	scoutErr := errors.New("Falha na Etapa 0 (SCOUT): boom")
	advance(7 * time.Second)
	tel.RecordScout(TokenStats{Input: 10, Output: 5, Total: 15}, 7, scoutErr)
	if got := tel.Finish("Go", scoutErr); got != "ERROR" {
		t.Errorf("Finish = %q; want ERROR", got)
	}
	rows := readCSV(t, runsPathT())
	r := rows[1]
	if r[9] != "ERROR" || r[8] != "15" || r[2] != "Go" || r[3] != "(setup / SCAN)" {
		t.Errorf("execução com erro incorreta: %v", r)
	}
	if r[10] != "SCOUT: ERROR | Duração total: 7s | Erro: Falha na Etapa 0 (SCOUT): boom" {
		t.Errorf("Note = %q", r[10])
	}
	md := readString(t, telemetryMDPath)
	if !strings.Contains(md, "| "+tel.RunID+" | ERROR | (setup / SCAN) | Go | 1/1 | 7s | 15 | n/a | n/a | n/a | n/a | n/a |") {
		t.Errorf("TELEMETRY.md deve listar a execução com ERROR:\n%s", md)
	}
}

func TestSetupTelemetryFinishErrorBeforeScout(t *testing.T) {
	chdirTemp(t)
	now, advance := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	advance(2 * time.Second)
	if got := tel.Finish("(não detectada)", errors.New("Erro ao criar estrutura")); got != "ERROR" {
		t.Errorf("Finish = %q; want ERROR", got)
	}
	if _, err := os.Stat(eventsPathT()); err == nil {
		t.Errorf("sem SCOUT executado não deve haver evento S0")
	}
	r := readCSV(t, runsPathT())[1]
	if r[9] != "ERROR" || r[8] != "0" || r[2] != "(não detectada)" {
		t.Errorf("execução incorreta: %v", r)
	}
	if r[10] != "SCOUT: NOT_RUN | Duração total: 2s | Erro: Erro ao criar estrutura" {
		t.Errorf("Note = %q", r[10])
	}
	if !strings.Contains(readString(t, telemetryMDPath), "| ERROR | (setup / SCAN) | (não detectada) | 1/1 | 0s | 0 |") {
		t.Errorf("TELEMETRY.md deve ser regenerado também no erro")
	}
}

func TestSetupTelemetryFinishIsIdempotent(t *testing.T) {
	chdirTemp(t)
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("m", now)
	first := tel.Finish("Go", nil)
	second := tel.Finish("Go", errors.New("tarde demais"))
	if first != "SETUP_OK" || second != "SETUP_OK" {
		t.Errorf("a 2ª chamada deve devolver o status já registrado: %q, %q", first, second)
	}
	if rows := readCSV(t, runsPathT()); len(rows) != 2 {
		t.Errorf("Finish deve gravar runs.csv uma única vez, veio %d linhas", len(rows)-1)
	}
}

func TestSetupTelemetryWriteFailureOnlyWarns(t *testing.T) {
	chdirTemp(t)
	// .makercycle/telemetry como arquivo: as gravações falham, mas nada pode entrar em pânico.
	if err := os.MkdirAll(".makercycle", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".makercycle", "telemetry"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("m", now)
	if got := tel.RecordScout(TokenStats{Total: 1}, 1, nil); got != "SUCCESS" {
		t.Errorf("RecordScout = %q", got)
	}
	if got := tel.Finish("Go", nil); got != "SETUP_OK" {
		t.Errorf("Finish = %q", got)
	}
}
