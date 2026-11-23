package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 7 — resumo de tokens por etapa (stage_summary.go)
// Referência: docs/references/setup-ux-and-db-prompt.md §1
// ---------------------------------------------------------------------------

// SS-H1: formato exato da linha de resumo.
func TestFormatStageSummaryExact(t *testing.T) {
	got := formatStageSummary("E1 PLAN", 74*time.Second,
		TokenStats{Input: 1000, Output: 500, Total: 1500}, TokenStats{Input: 1800, Output: 500, Total: 2300})
	want := "✓ E1 PLAN concluída em 01:14 | Tokens: +1500 (in: 1000 / out: 500) | Acumulado: 2300"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// SS-E1: tokens estimados recebem o prefixo "~" (na etapa e no acumulado).
func TestFormatStageSummaryEstimated(t *testing.T) {
	st := TokenStats{Input: 100, Output: 50, Total: 150, Estimated: true}
	got := formatStageSummary("E3 MAKE", 5*time.Second, st, st)
	want := "✓ E3 MAKE concluída em 00:05 | Tokens: +~150 (in: ~100 / out: ~50) | Acumulado: ~150"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
	exact := TokenStats{Input: 10, Output: 5, Total: 15}
	got = formatStageSummary("E5 GATE", 0, exact, TokenStats{Input: 110, Output: 55, Total: 165, Estimated: true})
	want = "✓ E5 GATE concluída em 00:00 | Tokens: +15 (in: 10 / out: 5) | Acumulado: ~165"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// SS-E2: etapa sem consumo continua com o formato completo.
func TestFormatStageSummaryZeroTokens(t *testing.T) {
	got := formatStageSummary("E2 CHALLENGE", 61*time.Second, TokenStats{}, TokenStats{Total: 40})
	want := "✓ E2 CHALLENGE concluída em 01:01 | Tokens: +0 (in: 0 / out: 0) | Acumulado: 40"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// SS-H2: rótulos E0..E5 a partir dos IDs de etapa da telemetria.
func TestStageSummaryLabel(t *testing.T) {
	cases := map[string]string{
		"S0": "E0 SCAN", "S1": "E1 PLAN", "S2": "E2 CHALLENGE", "S3": "E3 MAKE", "S4B": "E4 VERIFY", "S5": "E5 GATE",
		"S9": "S9", "": "",
	}
	for id, want := range cases {
		if got := stageSummaryLabel(id); got != want {
			t.Errorf("stageSummaryLabel(%q) = %q, want %q", id, got, want)
		}
	}
}

// SS-H3: o tracker acumula a partir da Etapa 0 e imprime uma linha por etapa no momento em que ela termina.
func TestStageTrackerAccumulatesIncludingE0(t *testing.T) {
	var buf bytes.Buffer
	tr := newStageTracker(&buf)
	l0 := tr.Finish("S0", 3*time.Second, TokenStats{Input: 80, Output: 20, Total: 100})
	if l0 != "✓ E0 SCAN concluída em 00:03 | Tokens: +100 (in: 80 / out: 20) | Acumulado: 100" {
		t.Errorf("linha E0 = %q", l0)
	}
	if !strings.Contains(buf.String(), l0+"\n") {
		t.Errorf("a linha E0 deve ser impressa imediatamente:\n%s", buf.String())
	}
	l1 := tr.Finish("S1", 65*time.Second, TokenStats{Input: 150, Output: 50, Total: 200})
	if l1 != "✓ E1 PLAN concluída em 01:05 | Tokens: +200 (in: 150 / out: 50) | Acumulado: 300" {
		t.Errorf("linha E1 = %q", l1)
	}
	if got := tr.Total(); got != (TokenStats{Input: 230, Output: 70, Total: 300}) {
		t.Errorf("Total() = %+v", got)
	}
	if lines := strings.Split(strings.TrimSpace(buf.String()), "\n"); len(lines) != 2 {
		t.Errorf("esperava 2 linhas impressas, veio %d:\n%s", len(lines), buf.String())
	}
}

// SS-E3: o tracker propaga Estimated para o acumulado.
func TestStageTrackerPropagatesEstimated(t *testing.T) {
	tr := newStageTracker(nil) // writer nil não imprime nem entra em pânico
	tr.Finish("S1", time.Second, TokenStats{Input: 1, Output: 1, Total: 2, Estimated: true})
	l := tr.Finish("S2", time.Second, TokenStats{Input: 1, Output: 1, Total: 2})
	if !strings.HasSuffix(l, "| Acumulado: ~4") || !tr.Total().Estimated {
		t.Errorf("acumulado deve ficar estimado: %q / %+v", l, tr.Total())
	}
}
