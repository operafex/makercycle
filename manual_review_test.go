package main

import (
	"bufio"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Modo de revisão manual (-manual) — primitivas puras de manual_review.go
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: MR-T1..MR-T4)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Task 1 — normalizeManualArgs
// ---------------------------------------------------------------------------

// MR-T1-H1..H3 / E1..E6: -manual é movido para logo após o nome do programa, em qualquer posição.
func TestNormalizeManualArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{"H1 -task valor -manual", []string{"mc", "-task", "fazer x", "-manual"}, []string{"mc", "-manual", "-task", "fazer x"}},
		{"H2 -task -manual valor", []string{"mc", "-task", "-manual", "fazer x"}, []string{"mc", "-manual", "-task", "fazer x"}},
		{"H3 -resume -manual", []string{"mc", "-resume", "-manual"}, []string{"mc", "-manual", "-resume"}},
		{"E1 já na frente", []string{"mc", "-manual", "-task", "x"}, []string{"mc", "-manual", "-task", "x"}},
		{"E2 sem -manual", []string{"mc", "-task", "x", "-ref", "a.md"}, []string{"mc", "-task", "x", "-ref", "a.md"}},
		{"E3 --manual e -manual=false", []string{"mc", "-task", "x", "--manual", "-manual=false"}, []string{"mc", "--manual", "-manual=false", "-task", "x"}},
		{"E4 após -- não é tocado", []string{"mc", "-task", "x", "--", "-manual"}, []string{"mc", "-task", "x", "--", "-manual"}},
		{"E5 -task=-manual é valor", []string{"mc", "-task=-manual"}, []string{"mc", "-task=-manual"}},
		{"E6 só o programa", []string{"mc"}, []string{"mc"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := normalizeManualArgs(c.in); !reflect.DeepEqual(got, c.want) {
				t.Errorf("normalizeManualArgs(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

// MR-T1-E7: entrada vazia/nil não entra em pânico e não altera o slice original.
func TestNormalizeManualArgsDoesNotMutateInput(t *testing.T) {
	if got := normalizeManualArgs(nil); len(got) != 0 {
		t.Errorf("nil deve devolver vazio, veio %q", got)
	}
	in := []string{"mc", "-task", "x", "-manual"}
	orig := append([]string(nil), in...)
	_ = normalizeManualArgs(in)
	if !reflect.DeepEqual(in, orig) {
		t.Errorf("o slice de entrada foi alterado: %q", in)
	}
}

// ---------------------------------------------------------------------------
// Task 2 — pauseForManualReview
// ---------------------------------------------------------------------------

func runPause(t *testing.T, input string) (string, string, string, error) {
	t.Helper()
	var out strings.Builder
	action, fb, err := pauseForManualReview("E1", "PLAN", ".makercycle/state/PLAN.md", strings.NewReader(input), &out)
	return action, fb, out.String(), err
}

// MR-T2-H1: a pausa mostra etapa, artefato e todas as opções.
func TestPauseForManualReviewShowsStageArtifactAndOptions(t *testing.T) {
	_, _, out, err := runPause(t, "\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"E1", "PLAN", ".makercycle/state/PLAN.md", "[Enter]", "[c]", "[r]", "[s]", "[q]"} {
		if !strings.Contains(out, want) {
			t.Errorf("saída da pausa sem %q:\n%s", want, out)
		}
	}
}

// MR-T2-H2..H6 / E1..E4: mapeamento das teclas para ações.
func TestPauseForManualReviewActions(t *testing.T) {
	cases := []struct {
		name, input, action, feedback string
	}{
		{"H2 Enter continua", "\n", manualActionContinue, ""},
		{"H3 c continua", "c\n", manualActionContinue, ""},
		{"H4 r com feedback", "r\n  cubra o caso de erro  \n", manualActionRedo, "cubra o caso de erro"},
		{"H5 s salva", "s\n", manualActionSave, ""},
		{"H6 q aborta", "q\n", manualActionAbort, ""},
		{"E1 maiúscula e espaços", "  C  \n", manualActionContinue, ""},
		{"E2 c sem quebra de linha antes do EOF", "c", manualActionContinue, ""},
		{"E3 r com feedback vazio", "r\n\n", manualActionRedo, ""},
		{"E4 r seguido de EOF", "r\n", manualActionRedo, ""},
		{"E5 Q maiúsculo", "Q\n", manualActionAbort, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			action, fb, _, err := runPause(t, c.input)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if action != c.action || fb != c.feedback {
				t.Errorf("(%q) = (%q, %q); want (%q, %q)", c.input, action, fb, c.action, c.feedback)
			}
		})
	}
}

// MR-T2-E5: opção inválida é recusada com aviso e a pergunta é repetida.
func TestPauseForManualReviewRepromptsOnInvalidOption(t *testing.T) {
	action, _, out, err := runPause(t, "x\nrefazer\ns\n")
	if err != nil {
		t.Fatal(err)
	}
	if action != manualActionSave {
		t.Errorf("action = %q; want %q", action, manualActionSave)
	}
	if strings.Count(out, "Opção inválida") != 2 {
		t.Errorf("esperado 2 avisos de opção inválida:\n%s", out)
	}
}

// MR-T2-X1..X3: EOF sem resposta válida => erro claro de terminal não interativo.
func TestPauseForManualReviewEOFIsNonInteractiveError(t *testing.T) {
	for name, input := range map[string]string{"X1 EOF imediato": "", "X2 inválida e EOF": "x\n"} {
		t.Run(name, func(t *testing.T) {
			action, _, _, err := runPause(t, input)
			if err == nil {
				t.Fatalf("esperado erro em EOF, veio action %q", action)
			}
			if !errors.Is(err, errManualNonInteractive) {
				t.Errorf("erro deve envolver errManualNonInteractive: %v", err)
			}
			if !strings.Contains(err.Error(), "-manual") || !strings.Contains(err.Error(), "terminal interativo") {
				t.Errorf("mensagem deve citar -manual e terminal interativo: %q", err.Error())
			}
			if action != "" {
				t.Errorf("action em erro deve ser vazia, veio %q", action)
			}
		})
	}
	if _, _, err := pauseForManualReview("E1", "PLAN", "x", nil, nil); !errors.Is(err, errManualNonInteractive) {
		t.Errorf("X3 reader nil deve devolver errManualNonInteractive, veio %v", err)
	}
}

// MR-T2-E6: um *bufio.Reader compartilhado entre pausas não perde linhas (reuso sem rebuffer).
func TestPauseForManualReviewReusesBufioReader(t *testing.T) {
	r := bufio.NewReader(strings.NewReader("r\nmais testes\nc\nq\n"))
	var out strings.Builder
	steps := []struct{ action, fb string }{
		{manualActionRedo, "mais testes"},
		{manualActionContinue, ""},
		{manualActionAbort, ""},
	}
	for i, want := range steps {
		action, fb, err := pauseForManualReview("E3", "MAKE", "git diff", r, &out)
		if err != nil || action != want.action || fb != want.fb {
			t.Fatalf("pausa %d = (%q, %q, %v); want (%q, %q, nil)", i+1, action, fb, err, want.action, want.fb)
		}
	}
}

// MR-T2-E7: out nil não entra em pânico.
func TestPauseForManualReviewNilWriter(t *testing.T) {
	if action, _, err := pauseForManualReview("E1", "PLAN", "x", strings.NewReader("c\n"), nil); err != nil || action != manualActionContinue {
		t.Errorf("out nil: (%q, %v)", action, err)
	}
}

// ---------------------------------------------------------------------------
// Task 3 — metadados das etapas, feedback, retomada e precedência do modo
// ---------------------------------------------------------------------------

// MR-T3-H1/E1: rótulo e artefato de cada etapa.
func TestManualStageLabelAndArtifactHint(t *testing.T) {
	cases := []struct{ code, label, hint string }{
		{"E0", "SCAN", "wiki/PROJECT_CONTEXT.md"},
		{"E1", "PLAN", ".makercycle/state/PLAN.md"},
		{"E2", "CHALLENGE", ".makercycle/state/SPEC_REVIEW.md"},
		{"E3", "MAKE", "git diff"},
		{"E4A", "VERIFY: CHECK", ".makercycle/state/VERIFY_REPORT.md"},
		{"E4B", "VERIFY: HARDEN", ".makercycle/state/SECURITY_REPORT.md"},
		{"E9", "", ""},
	}
	for _, c := range cases {
		if got := manualStageLabel(c.code); got != c.label {
			t.Errorf("manualStageLabel(%q) = %q; want %q", c.code, got, c.label)
		}
		if got := manualArtifactHint(c.code, "wiki"); got != c.hint {
			t.Errorf("manualArtifactHint(%q) = %q; want %q", c.code, got, c.hint)
		}
	}
	if got := manualArtifactHint("E0", ""); got != "docs/PROJECT_CONTEXT.md" {
		t.Errorf("E0 com docsDir vazio = %q; want docs/PROJECT_CONTEXT.md", got)
	}
}

// MR-T3-H2/E2: feedback anexado ao prompt; vazio mantém o prompt intacto.
func TestAppendManualFeedback(t *testing.T) {
	if got := appendManualFeedback("PROMPT", "  foco no erro 404 "); got != "PROMPT "+manualFeedbackMarker+" foco no erro 404" {
		t.Errorf("appendManualFeedback = %q", got)
	}
	if got := appendManualFeedback("PROMPT", "   "); got != "PROMPT" {
		t.Errorf("feedback em branco deve manter o prompt, veio %q", got)
	}
	if !strings.Contains(manualFeedbackMarker, "-manual") {
		t.Errorf("o marcador deve citar o modo -manual: %q", manualFeedbackMarker)
	}
}

// MR-T3-H3/E3/X1: etapa de retomada após [s].
func TestManualResumeStage(t *testing.T) {
	cases := []struct {
		code     string
		approved bool
		want     int
	}{
		{"E1", false, 2},
		{"E2", true, 3},
		{"E2", false, 1},
		{"E3", false, 4},
		{"E4A", false, 4},
		{"E4B", false, 5},
	}
	for _, c := range cases {
		got, err := manualResumeStage(c.code, c.approved)
		if err != nil || got != c.want {
			t.Errorf("manualResumeStage(%q, %v) = (%d, %v); want %d", c.code, c.approved, got, err, c.want)
		}
	}
	for _, code := range []string{"E0", "E5", ""} {
		if _, err := manualResumeStage(code, true); err == nil {
			t.Errorf("manualResumeStage(%q) deveria falhar", code)
		}
	}
}

// MR-T3-H4: o -manual explícito sobrepõe o estado salvo; sem flag, vale o salvo.
func TestResolveManualMode(t *testing.T) {
	cases := []struct{ saved, flagValue, flagSet, want bool }{
		{false, false, false, false},
		{true, false, false, true},
		{false, true, true, true},
		{true, false, true, false},
	}
	for _, c := range cases {
		if got := resolveManualMode(c.saved, c.flagValue, c.flagSet); got != c.want {
			t.Errorf("resolveManualMode(%v, %v, %v) = %v; want %v", c.saved, c.flagValue, c.flagSet, got, c.want)
		}
	}
}

// MR-T3-H5/E4: após [c] na 4A, o resultado é relido do VERIFY_REPORT.md (edições manuais valem).
func TestVerifyReportPassed(t *testing.T) {
	dir := t.TempDir()
	p := dir + "/VERIFY_REPORT.md"
	if got := verifyReportPassed(p, true); got != true {
		t.Errorf("arquivo ausente deve devolver o fallback (true)")
	}
	writeTestFile(t, p, "# Relatório\nALL_CHECKS_PASSED\n")
	if !verifyReportPassed(p, false) {
		t.Errorf("relatório com ALL_CHECKS_PASSED deve passar")
	}
	writeTestFile(t, p, "# Relatório\nFALHOU\n")
	if verifyReportPassed(p, true) {
		t.Errorf("relatório sem ALL_CHECKS_PASSED não deve passar")
	}
}

// MR-T3-H6: constantes de status e código de saída.
func TestManualConstants(t *testing.T) {
	if manualAbortStatus != "ABORTED" {
		t.Errorf("manualAbortStatus = %q; want ABORTED", manualAbortStatus)
	}
	if manualSaveStatus != "MANUAL_PAUSED" {
		t.Errorf("manualSaveStatus = %q; want MANUAL_PAUSED", manualSaveStatus)
	}
	if exitManualAbort != 6 {
		t.Errorf("exitManualAbort = %d; want 6", exitManualAbort)
	}
	for _, code := range []int{0, 1, exitSpecRejected, 3, 4, exitQuota} {
		if exitManualAbort == code {
			t.Errorf("exitManualAbort colide com o código %d", code)
		}
	}
	got := []string{manualActionContinue, manualActionRedo, manualActionSave, manualActionAbort}
	if !reflect.DeepEqual(got, []string{"continue", "redo", "save", "abort"}) {
		t.Errorf("ações = %q", got)
	}
}

// ---------------------------------------------------------------------------
// Task 4 — manualReviewer.Review (laço executar → pausar → refazer)
// ---------------------------------------------------------------------------

type fakeStage struct {
	feedbacks []string
	results   []bool // resultado de cada execução; ausente => true
}

func (f *fakeStage) run(feedback string) bool {
	f.feedbacks = append(f.feedbacks, feedback)
	if i := len(f.feedbacks) - 1; i < len(f.results) {
		return f.results[i]
	}
	return true
}

func newReviewer(enabled bool, input string, out *strings.Builder) manualReviewer {
	return manualReviewer{Enabled: enabled, In: bufio.NewReader(strings.NewReader(input)), Out: out, DocsDir: "docs"}
}

// MR-T4-H1: desligado => executa uma vez sem feedback, não pausa e não lê o stdin.
func TestManualReviewerDisabledRunsOnceWithoutPausing(t *testing.T) {
	var out strings.Builder
	f := &fakeStage{}
	action, err := newReviewer(false, "q\n", &out).Review("E3", f.run)
	if err != nil || action != manualActionContinue {
		t.Fatalf("Review desligado = (%q, %v)", action, err)
	}
	if !reflect.DeepEqual(f.feedbacks, []string{""}) {
		t.Errorf("execuções = %q; want [\"\"]", f.feedbacks)
	}
	if out.Len() != 0 {
		t.Errorf("modo automático não deve imprimir a pausa:\n%s", out.String())
	}
}

// MR-T4-H2..H4: continue, save e abort são devolvidos ao chamador após uma execução.
func TestManualReviewerReturnsFinalAction(t *testing.T) {
	for input, want := range map[string]string{"c\n": manualActionContinue, "s\n": manualActionSave, "q\n": manualActionAbort} {
		var out strings.Builder
		f := &fakeStage{}
		action, err := newReviewer(true, input, &out).Review("E4B", f.run)
		if err != nil || action != want {
			t.Errorf("entrada %q: (%q, %v); want %q", input, action, err, want)
		}
		if len(f.feedbacks) != 1 {
			t.Errorf("entrada %q: %d execuções; want 1", input, len(f.feedbacks))
		}
		if !strings.Contains(out.String(), "VERIFY: HARDEN") || !strings.Contains(out.String(), ".makercycle/state/SECURITY_REPORT.md") {
			t.Errorf("pausa da E4B sem rótulo/artefato:\n%s", out.String())
		}
	}
}

// MR-T4-H5: [r] reexecuta a etapa com o feedback informado; o feedback de cada rodada substitui o anterior.
func TestManualReviewerRedoRerunsWithFeedback(t *testing.T) {
	var out strings.Builder
	f := &fakeStage{}
	action, err := newReviewer(true, "r\nmais testes\nr\n\nc\n", &out).Review("E1", f.run)
	if err != nil || action != manualActionContinue {
		t.Fatalf("Review = (%q, %v)", action, err)
	}
	if want := []string{"", "mais testes", ""}; !reflect.DeepEqual(f.feedbacks, want) {
		t.Errorf("feedbacks = %q; want %q", f.feedbacks, want)
	}
}

// MR-T4-E1/E2: falha da etapa (run=false) não pausa — inclusive após um [r].
func TestManualReviewerSkipsPauseWhenStageFails(t *testing.T) {
	var out strings.Builder
	f := &fakeStage{results: []bool{false}}
	if action, err := newReviewer(true, "q\n", &out).Review("E3", f.run); err != nil || action != manualActionContinue {
		t.Errorf("falha na 1ª execução: (%q, %v); want continue", action, err)
	}
	if out.Len() != 0 {
		t.Errorf("falha não deve abrir a pausa:\n%s", out.String())
	}

	out.Reset()
	f = &fakeStage{results: []bool{true, false}}
	if action, err := newReviewer(true, "r\nx\nq\n", &out).Review("E3", f.run); err != nil || action != manualActionContinue {
		t.Errorf("falha após redo: (%q, %v); want continue", action, err)
	}
	if len(f.feedbacks) != 2 {
		t.Errorf("execuções = %d; want 2", len(f.feedbacks))
	}
}

// MR-T4-X1/X2: EOF na pausa e código de etapa desconhecido viram erro.
func TestManualReviewerErrors(t *testing.T) {
	var out strings.Builder
	f := &fakeStage{}
	if _, err := newReviewer(true, "", &out).Review("E2", f.run); !errors.Is(err, errManualNonInteractive) {
		t.Errorf("EOF: erro = %v; want errManualNonInteractive", err)
	}
	f = &fakeStage{}
	if _, err := newReviewer(true, "c\n", &out).Review("E7", f.run); err == nil {
		t.Errorf("etapa desconhecida deveria falhar")
	}
	if len(f.feedbacks) != 0 {
		t.Errorf("etapa desconhecida não deve executar run")
	}
}

// ---------------------------------------------------------------------------
// Task 5 — E0 SCAN: manualScoutRunner e telemetria ABORTED do -setup
// ---------------------------------------------------------------------------

// garante em tempo de compilação a assinatura do runner com feedback.
var _ scoutFeedbackRunner = runScoutAgentWithFeedback

type fakeScoutCall struct{ docsDir, feedback string }

func fakeScoutBase(calls *[]fakeScoutCall, errs ...error) scoutFeedbackRunner {
	return func(scout AgentBinding, docsDir, existingDocs, detectedName string, jsonMetrics bool, feedback string) (TokenStats, error) {
		*calls = append(*calls, fakeScoutCall{docsDir, feedback})
		var err error
		if i := len(*calls) - 1; i < len(errs) {
			err = errs[i]
		}
		return TokenStats{Input: 10, Output: 5, Total: 15}, err
	}
}

// MR-T5-H1..H4: continue/save devolvem nil; redo soma os tokens; abort devolve errManualAborted.
func TestManualScoutRunner(t *testing.T) {
	cases := []struct {
		name, input string
		calls       int
		abort       bool
	}{
		{"H1 continue", "c\n", 1, false},
		{"H2 save", "s\n", 1, false},
		{"H3 redo", "r\nleia o wiki\nc\n", 2, false},
		{"H4 abort", "q\n", 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var calls []fakeScoutCall
			var out strings.Builder
			m := manualReviewer{Enabled: true, In: bufio.NewReader(strings.NewReader(c.input)), Out: &out}
			run := manualScoutRunner(m, fakeScoutBase(&calls))
			stats, err := run(AgentBinding{}, "wiki", "", "Go", true)
			if c.abort != errors.Is(err, errManualAborted) || (!c.abort && err != nil) {
				t.Fatalf("erro = %v (abort esperado: %v)", err, c.abort)
			}
			if len(calls) != c.calls || stats.Total != 15*c.calls {
				t.Errorf("chamadas = %d, tokens = %d; want %d e %d", len(calls), stats.Total, c.calls, 15*c.calls)
			}
			if c.calls == 2 && calls[1].feedback != "leia o wiki" {
				t.Errorf("feedback do redo = %q", calls[1].feedback)
			}
			if !strings.Contains(out.String(), "wiki/PROJECT_CONTEXT.md") {
				t.Errorf("a pausa da E0 deve apontar <docsDir>/PROJECT_CONTEXT.md do argumento:\n%s", out.String())
			}
		})
	}
}

// MR-T5-X1: erro do SCOUT é devolvido sem pausar.
func TestManualScoutRunnerPropagatesScoutError(t *testing.T) {
	var calls []fakeScoutCall
	var out strings.Builder
	boom := errors.New("scout falhou")
	m := manualReviewer{Enabled: true, In: bufio.NewReader(strings.NewReader("q\n")), Out: &out}
	_, err := manualScoutRunner(m, fakeScoutBase(&calls, boom))(AgentBinding{}, "docs", "", "", true)
	if !errors.Is(err, boom) {
		t.Errorf("erro = %v; want %v", err, boom)
	}
	if out.Len() != 0 {
		t.Errorf("erro do SCOUT não deve abrir a pausa:\n%s", out.String())
	}
}

// MR-T5-H5: -setup abortado com [q] grava FinalStatus ABORTED em runs.csv.
func TestSetupTelemetryFinishAbortedStatus(t *testing.T) {
	chdirTemp(t)
	now, _ := fakeClock(setupT0)
	tel := newSetupTelemetry("opus", now)
	if got := tel.Finish("Go", fmt.Errorf("etapa 0: %w", errManualAborted)); got != manualAbortStatus {
		t.Fatalf("Finish(errManualAborted) = %q; want %q", got, manualAbortStatus)
	}
	rows := readCSV(t, runsPathT())
	last := rows[len(rows)-1]
	if last[9] != manualAbortStatus {
		t.Errorf("FinalStatus em runs.csv = %q; want %q", last[9], manualAbortStatus)
	}
}
