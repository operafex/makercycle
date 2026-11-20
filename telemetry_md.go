package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const telemetryMDPath = telemetryDir + "/TELEMETRY.md" // ".makercycle/telemetry/TELEMETRY.md"

// telemetryRun é uma linha de runs.csv enriquecida com os eventos (events.csv) do mesmo RunID.
type telemetryRun struct {
	RunID, Timestamp, Stack, DemandFile, Status, Note string
	MaxAttempts, Attempts                             int
	TokensIn, TokensOut, Tokens                       int
	DurationSeconds                                   float64
	StageTokens                                       [6]int // índices 1..5 = Etapas; 0 = SCAN (não se aplica)
}

// stageIndex converte o código de etapa de events.csv (S0..S5, com sufixo livre) em índice; -1 se inválido.
func stageIndex(stage string) int {
	if len(stage) < 2 || stage[0] != 'S' || stage[1] < '0' || stage[1] > '5' {
		return -1
	}
	return int(stage[1] - '0')
}

// formatTelemetryDuration formata segundos como time.Duration arredondada ao segundo (ex.: 2m51s).
func formatTelemetryDuration(seconds float64) string {
	if seconds < 0 {
		return "0s"
	}
	return (time.Duration(math.Round(seconds)) * time.Second).String()
}

// readCSVTable lê um CSV inteiro; arquivo vazio devolve nenhuma linha.
func readCSVTable(path string) ([][]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return rows, nil
}

// columnIndex mapeia o nome de cada coluna do cabeçalho para a sua posição.
func columnIndex(header []string) map[string]int {
	idx := make(map[string]int, len(header))
	for i, name := range header {
		if _, ok := idx[name]; !ok {
			idx[name] = i
		}
	}
	return idx
}

func missingColumns(idx map[string]int, required []string) []string {
	var missing []string
	for _, name := range required {
		if _, ok := idx[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// readTelemetryRuns lê runs.csv e agrega a duração e os tokens por etapa de events.csv (por RunID).
func readTelemetryRuns(runsPath, eventsPath string) ([]telemetryRun, error) {
	rows, err := readCSVTable(runsPath)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	col := columnIndex(rows[0])
	if missing := missingColumns(col, runsCSVHeader); len(missing) > 0 {
		return nil, fmt.Errorf("%s: colunas obrigatórias ausentes: %s", runsPath, strings.Join(missing, ", "))
	}

	runs := make([]telemetryRun, 0, len(rows)-1)
	byID := make(map[string]int, len(rows)-1)
	for _, r := range rows[1:] {
		run := telemetryRun{
			RunID:       r[col["RunID"]],
			Timestamp:   r[col["Timestamp"]],
			Stack:       r[col["Stack"]],
			DemandFile:  r[col["DemandFile"]],
			Status:      r[col["FinalStatus"]],
			Note:        r[col["Note"]],
			MaxAttempts: atoiOrZero(r[col["MaxAttempts"]]),
			Attempts:    atoiOrZero(r[col["TotalAttempts"]]),
			TokensIn:    atoiOrZero(r[col["TotalTokensIn"]]),
			TokensOut:   atoiOrZero(r[col["TotalTokensOut"]]),
			Tokens:      atoiOrZero(r[col["TotalTokens"]]),
		}
		if _, seen := byID[run.RunID]; !seen {
			byID[run.RunID] = len(runs)
		}
		runs = append(runs, run)
	}

	events, err := readCSVTable(eventsPath)
	if errors.Is(err, fs.ErrNotExist) {
		return runs, nil
	}
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return runs, nil
	}
	ecol := columnIndex(events[0])
	if missing := missingColumns(ecol, []string{"RunID", "Stage", "TotalTokens", "DurationSeconds"}); len(missing) > 0 {
		return nil, fmt.Errorf("%s: colunas obrigatórias ausentes: %s", eventsPath, strings.Join(missing, ", "))
	}
	for _, e := range events[1:] {
		i, ok := byID[e[ecol["RunID"]]]
		if !ok {
			continue
		}
		if d, perr := strconv.ParseFloat(strings.TrimSpace(e[ecol["DurationSeconds"]]), 64); perr == nil && !math.IsNaN(d) && !math.IsInf(d, 0) {
			runs[i].DurationSeconds += d
		}
		if s := stageIndex(e[ecol["Stage"]]); s >= 1 {
			runs[i].StageTokens[s] += atoiOrZero(e[ecol["TotalTokens"]])
		}
	}
	return runs, nil
}

// mdCell neutraliza quebras de linha e pipes para uso dentro de uma célula de tabela Markdown.
func mdCell(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return strings.ReplaceAll(s, "|", `\|`)
}

func stageCell(tokens int) string {
	if tokens > 0 {
		return strconv.Itoa(tokens)
	}
	return "n/a"
}

// telemetryRunLess ordena do mais recente para o mais antigo; timestamps inválidos vão para o fim.
func telemetryRunLess(a, b telemetryRun) bool {
	ta, errA := time.Parse(time.RFC3339, a.Timestamp)
	tb, errB := time.Parse(time.RFC3339, b.Timestamp)
	switch {
	case errA != nil && errB == nil:
		return false
	case errA == nil && errB != nil:
		return true
	case errA == nil && errB == nil && !ta.Equal(tb):
		return ta.After(tb)
	}
	na, errNA := strconv.ParseInt(a.RunID, 10, 64)
	nb, errNB := strconv.ParseInt(b.RunID, 10, 64)
	if errNA == nil && errNB == nil {
		return na > nb
	}
	return a.RunID > b.RunID
}

// buildTelemetryMarkdown gera o conteúdo de TELEMETRY.md (puro; não reordena a entrada).
func buildTelemetryMarkdown(runs []telemetryRun) string {
	sorted := make([]telemetryRun, len(runs))
	copy(sorted, runs)
	sort.SliceStable(sorted, func(i, j int) bool { return telemetryRunLess(sorted[i], sorted[j]) })

	var approved, tokens, tokensIn, tokensOut int
	var duration float64
	var stages [6]int
	for _, r := range runs {
		if r.Status == "APPROVED" {
			approved++
		}
		tokens += r.Tokens
		tokensIn += r.TokensIn
		tokensOut += r.TokensOut
		duration += r.DurationSeconds
		for i := 1; i <= 5; i++ {
			stages[i] += r.StageTokens[i]
		}
	}
	n := len(runs)
	rate, avg := 0.0, 0.0
	if n > 0 {
		rate = float64(approved) / float64(n) * 100
		avg = duration / float64(n)
	}

	var b strings.Builder
	b.WriteString("# Telemetria do MakerCycle\n\n")
	b.WriteString("> Gerado automaticamente a partir de [`runs.csv`](runs.csv) (uma linha por execução) e [`events.csv`](events.csv) (uma linha por etapa/agente). Não edite manualmente.\n\n")
	b.WriteString("## Indicadores Gerais (KPIs)\n\n")
	b.WriteString("| Indicador | Valor |\n| :--- | :--- |\n")
	fmt.Fprintf(&b, "| Total de Execuções | %d |\n", n)
	fmt.Fprintf(&b, "| Taxa de Sucesso | %.1f%% (%d APPROVED / %d) |\n", rate, approved, n)
	fmt.Fprintf(&b, "| Total Acumulado de Tokens | %d (entrada: %d / saída: %d) |\n", tokens, tokensIn, tokensOut)
	fmt.Fprintf(&b, "| Tokens E1 PLAN | %d |\n", stages[1])
	fmt.Fprintf(&b, "| Tokens E2 CHALLENGE | %d |\n", stages[2])
	fmt.Fprintf(&b, "| Tokens E3 MAKE | %d |\n", stages[3])
	fmt.Fprintf(&b, "| Tokens E4 VERIFY | %d |\n", stages[4])
	fmt.Fprintf(&b, "| Tokens E5 GATE | %d |\n", stages[5])
	fmt.Fprintf(&b, "| Duração Média | %s |\n\n", formatTelemetryDuration(avg))

	b.WriteString("## Histórico de Execuções\n\n")
	b.WriteString("| Data | RunID | Status | Demanda | Stack | Tentativas | Duração | Tokens Totais | Tokens E1 PLAN | Tokens E2 CHALLENGE | Tokens E3 MAKE | Tokens E4 VERIFY | Tokens E5 GATE |\n")
	b.WriteString("|" + strings.Repeat(" :--- |", 13) + "\n")
	for _, r := range sorted {
		date := r.Timestamp
		if t, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
			date = t.Format("2006-01-02 15:04")
		}
		demand := "—"
		if r.DemandFile != "" && r.DemandFile != noDemandMarker {
			demand = filepath.Base(r.DemandFile)
		}
		cells := []string{
			mdCell(date), mdCell(r.RunID), mdCell(r.Status), mdCell(demand), mdCell(r.Stack),
			fmt.Sprintf("%d/%d", r.Attempts, r.MaxAttempts),
			formatTelemetryDuration(r.DurationSeconds),
			strconv.Itoa(r.Tokens),
			stageCell(r.StageTokens[1]), stageCell(r.StageTokens[2]), stageCell(r.StageTokens[3]),
			stageCell(r.StageTokens[4]), stageCell(r.StageTokens[5]),
		}
		b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
	}
	return b.String()
}

// renderTelemetryMarkdown lê os CSVs e grava mdPath de forma atômica (arquivo temporário + rename).
// Em caso de erro de leitura, mdPath não é tocado.
func renderTelemetryMarkdown(runsPath, eventsPath, mdPath string) error {
	runs, err := readTelemetryRuns(runsPath, eventsPath)
	if err != nil {
		return err
	}
	content := buildTelemetryMarkdown(runs)

	dir := filepath.Dir(mdPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".TELEMETRY-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0644); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, mdPath); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// refreshTelemetryMarkdown regenera TELEMETRY.md; falhas só geram aviso (terminal e log), nunca interrompem.
func refreshTelemetryMarkdown(out io.Writer, runsPath, eventsPath, mdPath string) {
	err := renderTelemetryMarkdown(runsPath, eventsPath, mdPath)
	if err == nil {
		return
	}
	msg := fmt.Sprintf("[AVISO] Falha ao gerar %s: %v", mdPath, err)
	if out != nil {
		fmt.Fprintln(out, msg)
	}
	logEvent("%s", msg)
}
