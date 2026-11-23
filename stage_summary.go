package main

import (
	"fmt"
	"io"
	"time"
)

var stageSummaryLabels = map[string]string{"S0": "E0 SCAN", "S1": "E1 PLAN", "S2": "E2 CHALLENGE", "S3": "E3 MAKE", "S4B": "E4 VERIFY", "S5": "E5 GATE"}

// stageSummaryLabel traduz o ID de telemetria da etapa; IDs desconhecidos voltam como vieram.
func stageSummaryLabel(stageID string) string {
	if l, ok := stageSummaryLabels[stageID]; ok {
		return l
	}
	return stageID
}

// formatStageSummary monta a linha de resumo ao fim de uma etapa. Valores estimados levam o prefixo "~".
func formatStageSummary(stage string, d time.Duration, stageTokens, cumulative TokenStats) string {
	p, c := "", ""
	if stageTokens.Estimated {
		p = "~"
	}
	if cumulative.Estimated {
		c = "~"
	}
	return fmt.Sprintf("✓ %s concluída em %s | Tokens: +%s%d (in: %s%d / out: %s%d) | Acumulado: %s%d",
		stage, formatElapsed(d), p, stageTokens.Total, p, stageTokens.Input, p, stageTokens.Output, c, cumulative.Total)
}

// stageTracker acumula os tokens das etapas concluídas e imprime o resumo de cada uma.
type stageTracker struct {
	out   io.Writer
	total TokenStats
}

func newStageTracker(out io.Writer) *stageTracker {
	return &stageTracker{out: out}
}

// Finish registra a etapa concluída, imprime (se houver writer), loga e devolve a linha de resumo.
func (s *stageTracker) Finish(stageID string, d time.Duration, stats TokenStats) string {
	s.total.Add(stats)
	line := formatStageSummary(stageSummaryLabel(stageID), d, stats, s.total)
	if s.out != nil {
		fmt.Fprintln(s.out, line)
	}
	logEvent("%s", line)
	return line
}

// Total devolve os tokens acumulados das etapas concluídas.
func (s *stageTracker) Total() TokenStats {
	return s.total
}
