package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 5 — stageIndex / formatTelemetryDuration
// ---------------------------------------------------------------------------

func TestStageIndex(t *testing.T) {
	cases := map[string]int{
		"S0": 0, "S1": 1, "S2": 2, "S3": 3, "S4A": 4, "S4B": 4, "S5": 5,
		"": -1, "X1": -1, "S": -1, "S6": -1, "S9": -1, "s1": -1,
	}
	for in, want := range cases {
		if got := stageIndex(in); got != want {
			t.Errorf("stageIndex(%q) = %d, esperava %d", in, got, want)
		}
	}
}

func TestFormatTelemetryDuration(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0s"},
		{0.4, "0s"},
		{0.5, "1s"},
		{59.6, "1m0s"},
		{171.079, "2m51s"},
		{3725, "1h2m5s"},
		{-3, "0s"},
	}
	for _, c := range cases {
		if got := formatTelemetryDuration(c.in); got != c.want {
			t.Errorf("formatTelemetryDuration(%v) = %q, esperava %q", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 6 — readTelemetryRuns(runsPath, eventsPath)
// ---------------------------------------------------------------------------

const (
	runsHeaderLine   = "RunID,Timestamp,Stack,DemandFile,MaxAttempts,TotalAttempts,TotalTokensIn,TotalTokensOut,TotalTokens,FinalStatus,Note\n"
	eventsHeaderLine = "RunID,Timestamp,Stage,Agent,Model,Attempt,TokensIn,TokensOut,TotalTokens,DurationSeconds\n"
)

func writeTelemetryFiles(t *testing.T, runs, events string) (dir, runsPath, eventsPath string) {
	t.Helper()
	dir = t.TempDir()
	runsPath = filepath.Join(dir, "runs.csv")
	eventsPath = filepath.Join(dir, "events.csv")
	if runs != "" {
		if err := os.WriteFile(runsPath, []byte(runs), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if events != "" {
		if err := os.WriteFile(eventsPath, []byte(events), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir, runsPath, eventsPath
}

const sampleRuns = runsHeaderLine +
	"100,2026-11-19T22:02:08-03:00,Go,docs/demands/2026-11-19-a.md,3,1,1000,100,1100,APPROVED,\"Testes, AppSec e Diff-Judge aprovados\"\n" +
	"200,2026-11-20T10:00:00-03:00,Go,(via .makercycle/state/PLAN.md),3,0,50,5,55,SPEC_REVISION_REQUIRED,Especificação reprovada\n"

const sampleEvents = eventsHeaderLine +
	"100,2026-11-19T22:00:06-03:00,S1,PLANNER,opus,1,500,50,550,60.5\n" +
	"100,2026-11-19T22:00:54-03:00,S2,CRITIC,gemini,1,100,10,110,40\n" +
	"100,2026-11-19T22:01:19-03:00,S3,MAKER,sonnet,1,200,20,220,20.25\n" +
	"100,2026-11-19T22:01:54-03:00,S4B,AUDITOR,gemini,1,100,10,110,30\n" +
	"100,2026-11-19T22:02:08-03:00,S5,GATEKEEPER,gemini,1,100,10,110,10.25\n" +
	"200,2026-11-20T09:59:00-03:00,S1,PLANNER,opus,1,20,2,22,5\n" +
	"200,2026-11-20T09:59:30-03:00,S2,CRITIC,gemini,1,10,1,11,3\n" +
	"200,2026-11-20T09:59:40-03:00,S1,PLANNER,opus,2,10,1,11,2\n" +
	"200,2026-11-20T09:59:50-03:00,S2,CRITIC,gemini,2,10,1,11,1\n" +
	"999,2026-11-20T11:00:00-03:00,S1,PLANNER,opus,1,7,7,14,9\n" // execução sem linha em runs.csv: ignorada

func findRun(t *testing.T, runs []telemetryRun, id string) telemetryRun {
	t.Helper()
	for _, r := range runs {
		if r.RunID == id {
			return r
		}
	}
	t.Fatalf("RunID %s ausente em %+v", id, runs)
	return telemetryRun{}
}

func TestReadTelemetryRunsAggregatesEvents(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, sampleRuns, sampleEvents)
	runs, err := readTelemetryRuns(rp, ep)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 {
		t.Fatalf("esperava 2 execuções (RunID 999 só existe em events.csv), veio %d", len(runs))
	}
	if runs[0].RunID != "100" || runs[1].RunID != "200" {
		t.Errorf("readTelemetryRuns deve preservar a ordem do arquivo: %s, %s", runs[0].RunID, runs[1].RunID)
	}

	a := findRun(t, runs, "100")
	if a.Stack != "Go" || a.DemandFile != "docs/demands/2026-11-19-a.md" || a.Status != "APPROVED" ||
		a.Note != "Testes, AppSec e Diff-Judge aprovados" || a.Timestamp != "2026-11-19T22:02:08-03:00" {
		t.Errorf("campos textuais incorretos: %+v", a)
	}
	if a.MaxAttempts != 3 || a.Attempts != 1 || a.TokensIn != 1000 || a.TokensOut != 100 || a.Tokens != 1100 {
		t.Errorf("campos numéricos incorretos: %+v", a)
	}
	if a.DurationSeconds != 161 {
		t.Errorf("DurationSeconds = %v, esperava 161 (soma dos eventos)", a.DurationSeconds)
	}
	if a.StageTokens != [6]int{0, 550, 110, 220, 110, 110} {
		t.Errorf("StageTokens = %v", a.StageTokens)
	}

	b := findRun(t, runs, "200")
	if b.StageTokens != [6]int{0, 33, 22, 0, 0, 0} {
		t.Errorf("StageTokens do loop de especificação deve somar as rodadas: %v", b.StageTokens)
	}
	if b.DurationSeconds != 11 {
		t.Errorf("DurationSeconds = %v, esperava 11", b.DurationSeconds)
	}
}

func TestReadTelemetryRunsMissingEventsIsNotAnError(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, sampleRuns, "")
	runs, err := readTelemetryRuns(rp, ep)
	if err != nil {
		t.Fatalf("events.csv ausente não deve ser erro: %v", err)
	}
	if len(runs) != 2 || runs[0].DurationSeconds != 0 || runs[0].StageTokens != [6]int{} {
		t.Errorf("sem eventos, duração e tokens por etapa devem ser zero: %+v", runs)
	}
}

func TestReadTelemetryRunsMissingRunsIsError(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, "", sampleEvents)
	if _, err := readTelemetryRuns(rp, ep); err == nil {
		t.Errorf("runs.csv ausente deve retornar erro")
	}
}

func TestReadTelemetryRunsEmptyFileHasNoRuns(t *testing.T) {
	dir := t.TempDir()
	rp := filepath.Join(dir, "runs.csv")
	if err := os.WriteFile(rp, nil, 0644); err != nil {
		t.Fatal(err)
	}
	runs, err := readTelemetryRuns(rp, filepath.Join(dir, "events.csv"))
	if err != nil || len(runs) != 0 {
		t.Errorf("runs.csv vazio: runs=%v err=%v, esperava nenhuma execução e nenhum erro", runs, err)
	}
}

func TestReadTelemetryRunsHeaderOnly(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, runsHeaderLine, eventsHeaderLine)
	runs, err := readTelemetryRuns(rp, ep)
	if err != nil || len(runs) != 0 {
		t.Errorf("só cabeçalho: runs=%v err=%v", runs, err)
	}
}

func TestReadTelemetryRunsColumnsByHeaderName(t *testing.T) {
	runs := "FinalStatus,RunID,Note,Timestamp,Stack,DemandFile,MaxAttempts,TotalAttempts,TotalTokensIn,TotalTokensOut,TotalTokens\n" +
		"APPROVED,7,ok,2026-11-19T10:00:00Z,Go,d.md,3,2,1,2,3\n"
	_, rp, ep := writeTelemetryFiles(t, runs, "")
	got, err := readTelemetryRuns(rp, ep)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RunID != "7" || got[0].Status != "APPROVED" || got[0].Attempts != 2 || got[0].Tokens != 3 {
		t.Errorf("colunas devem ser mapeadas pelo nome do cabeçalho: %+v", got)
	}
}

func TestReadTelemetryRunsMissingRequiredColumn(t *testing.T) {
	runs := "RunID,Timestamp,Stack\n1,2026-11-19T10:00:00Z,Go\n"
	_, rp, ep := writeTelemetryFiles(t, runs, "")
	_, err := readTelemetryRuns(rp, ep)
	if err == nil || !strings.Contains(err.Error(), "FinalStatus") {
		t.Errorf("coluna obrigatória ausente deve gerar erro citando-a: %v", err)
	}
}

func TestReadTelemetryRunsMalformedCSV(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, runsHeaderLine+"1,2,3\n", "")
	if _, err := readTelemetryRuns(rp, ep); err == nil {
		t.Errorf("linha com número de colunas errado deve gerar erro")
	}
	_, rp, ep = writeTelemetryFiles(t, sampleRuns, eventsHeaderLine+"100,x\n")
	if _, err := readTelemetryRuns(rp, ep); err == nil {
		t.Errorf("events.csv malformado deve gerar erro")
	}
}

func TestReadTelemetryRunsNonNumericBecomesZero(t *testing.T) {
	runs := runsHeaderLine + "1,2026-11-19T10:00:00Z,Go,d.md,abc,,x,y,z,APPROVED,n\n"
	events := eventsHeaderLine + "1,2026-11-19T10:00:00Z,S1,PLANNER,opus,1,a,b,c,d\n"
	_, rp, ep := writeTelemetryFiles(t, runs, events)
	got, err := readTelemetryRuns(rp, ep)
	if err != nil {
		t.Fatal(err)
	}
	r := got[0]
	if r.MaxAttempts != 0 || r.Attempts != 0 || r.Tokens != 0 || r.DurationSeconds != 0 || r.StageTokens[1] != 0 {
		t.Errorf("valores não numéricos devem virar zero: %+v", r)
	}
}

// ---------------------------------------------------------------------------
// Task 7 — buildTelemetryMarkdown(runs)
// ---------------------------------------------------------------------------

const telemetryTableHeader = "| Data | RunID | Status | Demanda | Stack | Tentativas | Duração | Tokens Totais | Tokens E1 PLAN | Tokens E2 CHALLENGE | Tokens E3 MAKE | Tokens E4 VERIFY | Tokens E5 GATE |"

func sampleTelemetryRuns() []telemetryRun {
	return []telemetryRun{
		{RunID: "100", Timestamp: "2026-11-19T22:02:08-03:00", Stack: "Go", DemandFile: "docs/demands/2026-11-19-a.md",
			Status: "APPROVED", MaxAttempts: 3, Attempts: 1, TokensIn: 1000, TokensOut: 100, Tokens: 1100,
			DurationSeconds: 161, StageTokens: [6]int{0, 550, 110, 220, 110, 110}},
		{RunID: "200", Timestamp: "2026-11-20T10:00:00-03:00", Stack: "Go", DemandFile: noDemandMarker,
			Status: "SPEC_REVISION_REQUIRED", MaxAttempts: 3, Attempts: 0, TokensIn: 50, TokensOut: 5, Tokens: 55,
			DurationSeconds: 11, StageTokens: [6]int{0, 33, 22, 0, 0, 0}},
	}
}

func TestBuildTelemetryMarkdownHeaderLinksCSVs(t *testing.T) {
	md := buildTelemetryMarkdown(sampleTelemetryRuns())
	if !strings.HasPrefix(md, "# Telemetria do MakerCycle\n") {
		t.Errorf("título ausente:\n%s", md)
	}
	for _, want := range []string{"[`runs.csv`](runs.csv)", "[`events.csv`](events.csv)"} {
		if !strings.Contains(md, want) {
			t.Errorf("cabeçalho deve referenciar %s:\n%s", want, md)
		}
	}
}

func TestBuildTelemetryMarkdownKPIs(t *testing.T) {
	md := buildTelemetryMarkdown(sampleTelemetryRuns())
	for _, want := range []string{
		"## Indicadores Gerais (KPIs)",
		"| Indicador | Valor |",
		"| Total de Execuções | 2 |",
		"| Taxa de Sucesso | 50.0% (1 APPROVED / 2) |",
		"| Total Acumulado de Tokens | 1155 (entrada: 1050 / saída: 105) |",
		"| Tokens E1 PLAN | 583 |",
		"| Tokens E2 CHALLENGE | 132 |",
		"| Tokens E3 MAKE | 220 |",
		"| Tokens E4 VERIFY | 110 |",
		"| Tokens E5 GATE | 110 |",
		"| Duração Média | 1m26s |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("KPI ausente: %q\n%s", want, md)
		}
	}
}

func TestBuildTelemetryMarkdownHistoryTable(t *testing.T) {
	md := buildTelemetryMarkdown(sampleTelemetryRuns())
	if !strings.Contains(md, "## Histórico de Execuções") {
		t.Errorf("seção de histórico ausente:\n%s", md)
	}
	sep := "|" + strings.Repeat(" :--- |", 13)
	if !strings.Contains(md, telemetryTableHeader+"\n"+sep+"\n") {
		t.Errorf("cabeçalho exato da tabela ausente (Status logo após RunID):\n%s", md)
	}
	rowNew := "| 2026-11-20 10:00 | 200 | SPEC_REVISION_REQUIRED | — | Go | 0/3 | 11s | 55 | 33 | 22 | n/a | n/a | n/a |"
	rowOld := "| 2026-11-19 22:02 | 100 | APPROVED | 2026-11-19-a.md | Go | 1/3 | 2m41s | 1100 | 550 | 110 | 220 | 110 | 110 |"
	iNew, iOld := strings.Index(md, rowNew), strings.Index(md, rowOld)
	if iNew < 0 || iOld < 0 {
		t.Fatalf("linhas esperadas ausentes:\n%s\n%s\n---\n%s", rowNew, rowOld, md)
	}
	if iNew > iOld {
		t.Errorf("histórico deve estar em ordem cronológica decrescente")
	}
}

func TestBuildTelemetryMarkdownDescendingOrderDoesNotMutateInput(t *testing.T) {
	in := []telemetryRun{
		{RunID: "1", Timestamp: "2026-01-01T00:00:00Z", Status: "APPROVED"},
		{RunID: "3", Timestamp: "2026-03-01T00:00:00Z", Status: "APPROVED"},
		{RunID: "2", Timestamp: "2026-02-01T00:00:00Z", Status: "APPROVED"},
	}
	md := buildTelemetryMarkdown(in)
	i1, i2, i3 := strings.Index(md, "| 1 |"), strings.Index(md, "| 2 |"), strings.Index(md, "| 3 |")
	if !(i3 < i2 && i2 < i1) {
		t.Errorf("ordem esperada 3,2,1 (posições %d,%d,%d):\n%s", i3, i2, i1, md)
	}
	if in[0].RunID != "1" || in[1].RunID != "3" || in[2].RunID != "2" {
		t.Errorf("buildTelemetryMarkdown não deve reordenar o slice de entrada: %+v", in)
	}
}

func TestBuildTelemetryMarkdownTieBreaksByRunIDDesc(t *testing.T) {
	in := []telemetryRun{
		{RunID: "10", Timestamp: "2026-01-01T00:00:00Z"},
		{RunID: "20", Timestamp: "2026-01-01T00:00:00Z"},
	}
	md := buildTelemetryMarkdown(in)
	if strings.Index(md, "| 20 |") > strings.Index(md, "| 10 |") {
		t.Errorf("empate de Timestamp deve ordenar por RunID decrescente:\n%s", md)
	}
}

func TestBuildTelemetryMarkdownUnparseableTimestampGoesLastAndRaw(t *testing.T) {
	in := []telemetryRun{
		{RunID: "1", Timestamp: "ontem"},
		{RunID: "2", Timestamp: "2026-01-01T00:00:00Z"},
	}
	md := buildTelemetryMarkdown(in)
	if !strings.Contains(md, "| ontem | 1 |") {
		t.Errorf("timestamp inválido deve aparecer cru:\n%s", md)
	}
	if strings.Index(md, "| ontem | 1 |") < strings.Index(md, "| 2 |") {
		t.Errorf("timestamp inválido deve ir para o fim:\n%s", md)
	}
}

func TestBuildTelemetryMarkdownEscapesPipesAndNewlines(t *testing.T) {
	in := []telemetryRun{{RunID: "1", Timestamp: "2026-01-01T00:00:00Z", Stack: "Node | TS", Status: "A\nB",
		DemandFile: "docs/demands/x|y.md"}}
	md := buildTelemetryMarkdown(in)
	for _, want := range []string{`Node \| TS`, "A B", `x\|y.md`} {
		if !strings.Contains(md, want) {
			t.Errorf("esperava %q escapado:\n%s", want, md)
		}
	}
}

func TestBuildTelemetryMarkdownEmptyDemandShowsDash(t *testing.T) {
	md := buildTelemetryMarkdown([]telemetryRun{{RunID: "1", Timestamp: "2026-01-01T00:00:00Z", Stack: "Go"}})
	if !strings.Contains(md, "| 1 |  | — | Go |") {
		t.Errorf("demanda vazia deve aparecer como —:\n%s", md)
	}
}

func TestBuildTelemetryMarkdownNoRuns(t *testing.T) {
	md := buildTelemetryMarkdown(nil)
	for _, want := range []string{
		"| Total de Execuções | 0 |",
		"| Taxa de Sucesso | 0.0% (0 APPROVED / 0) |",
		"| Total Acumulado de Tokens | 0 (entrada: 0 / saída: 0) |",
		"| Duração Média | 0s |",
		telemetryTableHeader,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("sem execuções, esperava %q:\n%s", want, md)
		}
	}
	if !strings.HasSuffix(md, "\n") {
		t.Errorf("o Markdown deve terminar com quebra de linha")
	}
}

// ---------------------------------------------------------------------------
// Task 8 — renderTelemetryMarkdown / refreshTelemetryMarkdown / recordRunTelemetry
// ---------------------------------------------------------------------------

func TestRenderTelemetryMarkdownWritesFile(t *testing.T) {
	dir, rp, ep := writeTelemetryFiles(t, sampleRuns, sampleEvents)
	mdPath := filepath.Join(dir, "sub", "TELEMETRY.md")
	if err := renderTelemetryMarkdown(rp, ep, mdPath); err != nil {
		t.Fatal(err)
	}
	got := readString(t, mdPath)
	runs, _ := readTelemetryRuns(rp, ep)
	if got != buildTelemetryMarkdown(runs) {
		t.Errorf("conteúdo gravado difere de buildTelemetryMarkdown:\n%s", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(mdPath))
	if len(entries) != 1 {
		t.Errorf("não deve sobrar arquivo temporário: %v", entries)
	}
}

func TestRenderTelemetryMarkdownReadErrorKeepsPreviousFile(t *testing.T) {
	dir := t.TempDir()
	mdPath := filepath.Join(dir, "TELEMETRY.md")
	if err := os.WriteFile(mdPath, []byte("anterior"), 0644); err != nil {
		t.Fatal(err)
	}
	err := renderTelemetryMarkdown(filepath.Join(dir, "runs.csv"), filepath.Join(dir, "events.csv"), mdPath)
	if err == nil {
		t.Fatalf("runs.csv ausente deve retornar erro")
	}
	if got := readString(t, mdPath); got != "anterior" {
		t.Errorf("TELEMETRY.md anterior não deve ser alterado em caso de erro: %q", got)
	}
}

func TestRenderTelemetryMarkdownWriteError(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, sampleRuns, sampleEvents)
	blocker := filepath.Join(t.TempDir(), "arquivo")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := renderTelemetryMarkdown(rp, ep, filepath.Join(blocker, "TELEMETRY.md")); err == nil {
		t.Errorf("falha de gravação deve retornar erro")
	}
}

func TestRefreshTelemetryMarkdownWarnsWithoutPanicking(t *testing.T) {
	closeLog := withTestRunLog(t)
	dir := t.TempDir()
	var out bytes.Buffer
	mdPath := filepath.Join(dir, "TELEMETRY.md")
	refreshTelemetryMarkdown(&out, filepath.Join(dir, "runs.csv"), filepath.Join(dir, "events.csv"), mdPath)
	logContent := closeLog()
	if !strings.HasPrefix(out.String(), "[AVISO] Falha ao gerar "+mdPath+":") {
		t.Errorf("esperava aviso no terminal, veio %q", out.String())
	}
	if !strings.Contains(logContent, "[AVISO] Falha ao gerar "+mdPath+":") {
		t.Errorf("esperava aviso no log de execução:\n%s", logContent)
	}
	if _, err := os.Stat(mdPath); err == nil {
		t.Errorf("TELEMETRY.md não deve ser criado em caso de erro")
	}
}

func TestRefreshTelemetryMarkdownSuccessIsSilent(t *testing.T) {
	dir, rp, ep := writeTelemetryFiles(t, sampleRuns, sampleEvents)
	var out bytes.Buffer
	refreshTelemetryMarkdown(&out, rp, ep, filepath.Join(dir, "TELEMETRY.md"))
	if out.Len() != 0 {
		t.Errorf("sucesso não deve emitir saída: %q", out.String())
	}
}

func TestTelemetryMDPathConstant(t *testing.T) {
	if telemetryMDPath != ".makercycle/telemetry/TELEMETRY.md" {
		t.Errorf("telemetryMDPath = %q", telemetryMDPath)
	}
}

func TestRecordRunTelemetryRegeneratesTelemetryMarkdown(t *testing.T) {
	chdirTemp(t)
	recordEventTelemetry("42", "S1", "PLANNER", "opus", 1, TokenStats{Input: 10, Output: 5, Total: 15}, 3)
	recordRunTelemetry("42", "Go", "docs/demands/x.md", 3, 1, TokenStats{Input: 10, Output: 5, Total: 15}, "APPROVED", "ok")
	md := readString(t, telemetryMDPath)
	for _, want := range []string{"| 42 | APPROVED | x.md | Go | 1/3 | 3s | 15 | 15 |", "| Total de Execuções | 1 |"} {
		if !strings.Contains(md, want) {
			t.Errorf("TELEMETRY.md deve conter %q:\n%s", want, md)
		}
	}

	recordRunTelemetry("43", "Go", "", 3, 0, TokenStats{Total: 7}, "SPEC_REVISION_REQUIRED", "n")
	md = readString(t, telemetryMDPath)
	if !strings.Contains(md, "| Total de Execuções | 2 |") || !strings.Contains(md, "| 43 | SPEC_REVISION_REQUIRED |") {
		t.Errorf("TELEMETRY.md deve ser regenerado a cada execução:\n%s", md)
	}
}

// ---------------------------------------------------------------------------
// Task 5 — S0 (SCAN / -setup) nos KPIs sem coluna própria no histórico
// ---------------------------------------------------------------------------

const setupRuns = runsHeaderLine +
	"300,2026-11-20T21:00:00-03:00,Go,(setup / SCAN),1,1,90,30,120,SETUP_OK,SCOUT: SUCCESS | Duração total: 42s\n" +
	"100,2026-11-19T22:02:08-03:00,Go,docs/demands/2026-11-19-a.md,3,1,1000,100,1100,APPROVED,ok\n"

const setupEvents = eventsHeaderLine +
	"300,2026-11-20T20:59:30-03:00,S0,SCOUT,opus,1,90,30,120,42\n" +
	"100,2026-11-19T22:00:06-03:00,S1,PLANNER,opus,1,500,50,550,60\n"

func TestReadTelemetryRunsAggregatesS0IntoStageZero(t *testing.T) {
	_, rp, ep := writeTelemetryFiles(t, setupRuns, setupEvents)
	runs, err := readTelemetryRuns(rp, ep)
	if err != nil {
		t.Fatal(err)
	}
	s := findRun(t, runs, "300")
	if s.StageTokens != [6]int{120, 0, 0, 0, 0, 0} {
		t.Errorf("tokens S0 devem ir para StageTokens[0]: %v", s.StageTokens)
	}
	if s.DurationSeconds != 42 {
		t.Errorf("a duração do S0 deve ser somada: %v", s.DurationSeconds)
	}
	if d := findRun(t, runs, "100"); d.StageTokens != [6]int{0, 550, 0, 0, 0, 0} {
		t.Errorf("execuções sem S0 mantêm StageTokens[0] = 0: %v", d.StageTokens)
	}
}

func TestBuildTelemetryMarkdownS0KPIAndHistory(t *testing.T) {
	runs := []telemetryRun{
		{RunID: "300", Timestamp: "2026-11-20T21:00:00-03:00", Stack: "Go", DemandFile: "(setup / SCAN)",
			Status: "SETUP_OK", MaxAttempts: 1, Attempts: 1, TokensIn: 90, TokensOut: 30, Tokens: 120,
			DurationSeconds: 42, StageTokens: [6]int{120, 0, 0, 0, 0, 0}},
		{RunID: "100", Timestamp: "2026-11-19T22:02:08-03:00", Stack: "Go", DemandFile: "docs/demands/2026-11-19-a.md",
			Status: "APPROVED", MaxAttempts: 3, Attempts: 1, TokensIn: 1000, TokensOut: 100, Tokens: 1100,
			DurationSeconds: 60, StageTokens: [6]int{0, 550, 0, 0, 0, 0}},
	}
	md := buildTelemetryMarkdown(runs)
	for _, want := range []string{
		"| Total de Execuções | 2 |",
		"| Total Acumulado de Tokens | 1220 (entrada: 1090 / saída: 130) |",
		"| Tokens E0 SCAN | 120 |",
		"| Tokens E1 PLAN | 550 |",
		"| Duração Média | 51s |",
		"| 2026-11-20 21:00 | 300 | SETUP_OK | (setup / SCAN) | Go | 1/1 | 42s | 120 | n/a | n/a | n/a | n/a | n/a |",
		"| 2026-11-19 22:02 | 100 | APPROVED | 2026-11-19-a.md | Go | 1/3 | 1m0s | 1100 | 550 | n/a | n/a | n/a | n/a |",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("esperava %q:\n%s", want, md)
		}
	}
	if !strings.Contains(md, telemetryTableHeader+"\n|"+strings.Repeat(" :--- |", 13)+"\n") {
		t.Errorf("o histórico continua com 13 colunas (S0 sem coluna própria):\n%s", md)
	}
	if strings.Contains(md, "Tokens E0 SCAN |  |") || strings.Contains(md, "| Tokens E0 SCAN | Tokens E1 PLAN") {
		t.Errorf("S0 não pode ganhar coluna na tabela de histórico:\n%s", md)
	}
	if strings.Index(md, "| Tokens E0 SCAN |") > strings.Index(md, "| Tokens E1 PLAN |") {
		t.Errorf("o KPI E0 SCAN deve vir antes de E1 PLAN")
	}
}

func TestBuildTelemetryMarkdownS0KPIZeroWithoutSetup(t *testing.T) {
	md := buildTelemetryMarkdown(sampleTelemetryRuns())
	if !strings.Contains(md, "| Tokens E0 SCAN | 0 |") {
		t.Errorf("sem -setup o KPI E0 SCAN deve ser 0:\n%s", md)
	}
	if !strings.Contains(buildTelemetryMarkdown(nil), "| Tokens E0 SCAN | 0 |") {
		t.Errorf("sem execuções o KPI E0 SCAN deve ser 0")
	}
}

func TestTelemetryDemandCell(t *testing.T) {
	cases := map[string]string{
		"":                             "—",
		noDemandMarker:                 "—",
		"(setup / SCAN)":               "(setup / SCAN)",
		"docs/demands/2026-11-19-a.md": "2026-11-19-a.md",
		"2026-11-19-a.md":              "2026-11-19-a.md",
	}
	for in, want := range cases {
		if got := telemetryDemandCell(in); got != want {
			t.Errorf("telemetryDemandCell(%q) = %q; want %q", in, got, want)
		}
	}
}
