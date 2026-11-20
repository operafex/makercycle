package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 1 — decideSpecLoop / specRevisionWarning / specAbortNote
// ---------------------------------------------------------------------------

func TestDecideSpecLoop(t *testing.T) {
	cases := []struct {
		name         string
		review       string
		attempt, max int
		want         specLoopAction
	}{
		{"aprovado na 1ª", "SPEC_STATUS: APPROVED\n", 1, 3, specActionApprove},
		{"aprovado no meio do texto", "# Parecer\n\nSPEC_STATUS: APPROVED\n\nok", 2, 3, specActionApprove},
		{"aprovado mesmo na última", "SPEC_STATUS: APPROVED", 3, 3, specActionApprove},
		{"revisão com tentativas restantes", "SPEC_STATUS: REVISION_REQUIRED", 1, 3, specActionRetry},
		{"revisão na penúltima", "SPEC_STATUS: REVISION_REQUIRED", 2, 3, specActionRetry},
		{"revisão na última", "SPEC_STATUS: REVISION_REQUIRED", 3, 3, specActionAbort},
		{"parecer vazio conta como revisão", "", 1, 3, specActionRetry},
		{"parecer vazio na última", "", 3, 3, specActionAbort},
		{"max 1 sem aprovação aborta", "SPEC_STATUS: REVISION_REQUIRED", 1, 1, specActionAbort},
		{"tentativa acima do max aborta", "x", 4, 3, specActionAbort},
		{"minúsculas não aprovam", "spec_status: approved", 1, 3, specActionRetry},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := decideSpecLoop(c.review, c.attempt, c.max); got != c.want {
				t.Errorf("decideSpecLoop(%q, %d, %d) = %v, esperava %v", c.review, c.attempt, c.max, got, c.want)
			}
		})
	}
}

func TestSpecRevisionWarningExactText(t *testing.T) {
	want := "[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)..."
	if got := specRevisionWarning(2, 3); got != want {
		t.Errorf("specRevisionWarning(2, 3) = %q, esperava %q", got, want)
	}
	if got := specRevisionWarning(5, 5); got != "[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 5/5)..." {
		t.Errorf("specRevisionWarning(5, 5) = %q", got)
	}
}

func TestSpecAbortNote(t *testing.T) {
	want := "Especificação reprovada pelo CRITIC após 3 tentativa(s)"
	if got := specAbortNote(3); got != want {
		t.Errorf("specAbortNote(3) = %q, esperava %q", got, want)
	}
}

func TestExitSpecRejectedIsTwo(t *testing.T) {
	if exitSpecRejected != 2 {
		t.Errorf("exitSpecRejected = %d, esperava 2", exitSpecRejected)
	}
}

// ---------------------------------------------------------------------------
// Task 2 — buildPlannerPrompt / buildCriticPrompt
// ---------------------------------------------------------------------------

func legacyPlannerPrompt(skill, docsDir, stackName, task, demandFile string, refs []string) string {
	return "Siga estritamente as instruções de @.makercycle/agents/01_PLANNER.md aplicando a skill " + skill + ". " +
		"Diretório de documentação: " + docsDir + "/. Stack: " + stackName + ". Demanda: " + task + ". Arquivo da demanda: " + demandFile + ". " +
		"Grave .makercycle/state/PLAN.md, .makercycle/state/TEST_CASES.md e os testes em estado RED." + referencesPrompt(refs)
}

func TestBuildPlannerPromptFirstAttemptIsLegacyPrompt(t *testing.T) {
	refs := []string{"docs/references/a.md"}
	got := buildPlannerPrompt("obra/superpowers", "docs", "Go", "Fazer X", "docs/demands/x.md", refs, 1)
	want := legacyPlannerPrompt("obra/superpowers", "docs", "Go", "Fazer X", "docs/demands/x.md", refs)
	if got != want {
		t.Errorf("prompt da 1ª rodada mudou:\n got: %q\nwant: %q", got, want)
	}
	if strings.Contains(got, "SPEC_REVIEW.md") {
		t.Errorf("1ª rodada não deve citar SPEC_REVIEW.md: %q", got)
	}
}

func TestBuildPlannerPromptZeroAttemptBehavesAsFirst(t *testing.T) {
	got := buildPlannerPrompt("s", "docs", "Go", "t", "d.md", nil, 0)
	if got != legacyPlannerPrompt("s", "docs", "Go", "t", "d.md", nil) {
		t.Errorf("specAttempt 0 deve gerar o prompt legado: %q", got)
	}
}

func TestBuildPlannerPromptRevisionRound(t *testing.T) {
	refs := []string{"docs/references/a.md"}
	base := legacyPlannerPrompt("obra/superpowers", "docs", "Go", "Fazer X", "docs/demands/x.md", refs)
	for _, attempt := range []int{2, 3} {
		got := buildPlannerPrompt("obra/superpowers", "docs", "Go", "Fazer X", "docs/demands/x.md", refs, attempt)
		if !strings.HasPrefix(got, base) {
			t.Errorf("rodada %d deve começar pelo prompt legado (com referências):\n%q", attempt, got)
		}
		if got != base+specRevisionInstruction {
			t.Errorf("rodada %d deve ser prompt legado + specRevisionInstruction:\n%q", attempt, got)
		}
	}
}

func TestSpecRevisionInstructionContent(t *testing.T) {
	for _, want := range []string{
		"@.makercycle/state/SPEC_REVIEW.md",
		".makercycle/state/PLAN.md",
		".makercycle/state/TEST_CASES.md",
		"testes RED",
	} {
		if !strings.Contains(specRevisionInstruction, want) {
			t.Errorf("specRevisionInstruction deve conter %q: %q", want, specRevisionInstruction)
		}
	}
	if !strings.HasPrefix(specRevisionInstruction, " ") {
		t.Errorf("specRevisionInstruction deve começar com espaço (é concatenada ao prompt): %q", specRevisionInstruction)
	}
}

func TestBuildCriticPromptIsLegacyPrompt(t *testing.T) {
	want := "Siga estritamente as instruções de @.makercycle/agents/02_CRITIC.md aplicando a skill google/elephant-goldfish-model. " +
		"Diretório de documentação: docs/. Audite @.makercycle/state/PLAN.md, @.makercycle/state/TEST_CASES.md e os arquivos de teste recém-criados. " +
		"Grave seu veredito em .makercycle/state/SPEC_REVIEW.md."
	if got := buildCriticPrompt("google/elephant-goldfish-model", "docs"); got != want {
		t.Errorf("prompt do CRITIC mudou:\n got: %q\nwant: %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Task 3 — runSpecLoop(cfg, deps)
// ---------------------------------------------------------------------------

const (
	reviewApproved = "SPEC_STATUS: APPROVED\n"
	reviewRevision = "SPEC_STATUS: REVISION_REQUIRED\n- ajuste X\n"
)

// specHarness registra as chamadas feitas pelo loop e devolve respostas roteirizadas.
type specHarness struct {
	plannerReplies []AgentResult
	criticReplies  []AgentResult
	reviews        []string
	reviewErr      error

	plannerCalls []int
	criticCalls  []int
	clearCalls   int
	events       []string // sequência: P<n>, C<n>, clear, read
	out          bytes.Buffer
}

func (h *specHarness) deps() specLoopDeps {
	return specLoopDeps{
		RunPlanner: func(a int) AgentResult {
			h.plannerCalls = append(h.plannerCalls, a)
			h.events = append(h.events, "P")
			i := len(h.plannerCalls) - 1
			if i < len(h.plannerReplies) {
				return h.plannerReplies[i]
			}
			return AgentResult{}
		},
		RunCritic: func(a int) AgentResult {
			h.criticCalls = append(h.criticCalls, a)
			h.events = append(h.events, "C")
			i := len(h.criticCalls) - 1
			if i < len(h.criticReplies) {
				return h.criticReplies[i]
			}
			return AgentResult{}
		},
		ClearReview: func() {
			h.clearCalls++
			h.events = append(h.events, "clear")
		},
		ReadReview: func() (string, error) {
			h.events = append(h.events, "read")
			if h.reviewErr != nil {
				return "", h.reviewErr
			}
			i := len(h.criticCalls) - 1
			if i >= 0 && i < len(h.reviews) {
				return h.reviews[i], nil
			}
			return "", nil
		},
		Out: &h.out,
	}
}

func tok(in, out int) TokenStats { return TokenStats{Input: in, Output: out, Total: in + out} }

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestRunSpecLoopApprovedFirstRound(t *testing.T) {
	h := &specHarness{
		plannerReplies: []AgentResult{{Stats: tok(100, 10)}},
		criticReplies:  []AgentResult{{Stats: tok(20, 2)}},
		reviews:        []string{reviewApproved},
	}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopApproved {
		t.Fatalf("Status = %v, esperava specLoopApproved", got.Status)
	}
	if got.Attempts != 1 {
		t.Errorf("Attempts = %d, esperava 1", got.Attempts)
	}
	if !equalInts(h.plannerCalls, []int{1}) || !equalInts(h.criticCalls, []int{1}) {
		t.Errorf("chamadas planner=%v critic=%v, esperava [1] e [1]", h.plannerCalls, h.criticCalls)
	}
	if got.Tokens != tok(120, 12) {
		t.Errorf("Tokens = %+v, esperava %+v", got.Tokens, tok(120, 12))
	}
	if got.Review != reviewApproved {
		t.Errorf("Review = %q", got.Review)
	}
	if strings.Contains(h.out.String(), "[AVISO]") {
		t.Errorf("aprovação na 1ª rodada não deve emitir aviso: %q", h.out.String())
	}
}

func TestRunSpecLoopRevisionThenApproved(t *testing.T) {
	h := &specHarness{
		plannerReplies: []AgentResult{{Stats: tok(100, 10)}, {Stats: tok(50, 5)}},
		criticReplies:  []AgentResult{{Stats: tok(20, 2)}, {Stats: tok(30, 3)}},
		reviews:        []string{reviewRevision, reviewApproved},
	}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopApproved || got.Attempts != 2 {
		t.Fatalf("Status=%v Attempts=%d, esperava aprovado na tentativa 2", got.Status, got.Attempts)
	}
	if !equalInts(h.plannerCalls, []int{1, 2}) || !equalInts(h.criticCalls, []int{1, 2}) {
		t.Errorf("chamadas planner=%v critic=%v, esperava [1 2] e [1 2]", h.plannerCalls, h.criticCalls)
	}
	if got.Tokens != tok(200, 20) {
		t.Errorf("Tokens = %+v, esperava soma de todas as rodadas %+v", got.Tokens, tok(200, 20))
	}
	wantMsg := "[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)...\n"
	if h.out.String() != wantMsg {
		t.Errorf("saída = %q, esperava %q", h.out.String(), wantMsg)
	}
}

func TestRunSpecLoopStartStage2SkipsPlannerOnlyInFirstRound(t *testing.T) {
	h := &specHarness{
		reviews: []string{reviewRevision, reviewApproved},
	}
	got := runSpecLoop(specLoopConfig{StartStage: 2, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopApproved || got.Attempts != 2 {
		t.Fatalf("Status=%v Attempts=%d", got.Status, got.Attempts)
	}
	if !equalInts(h.plannerCalls, []int{2}) {
		t.Errorf("PLANNER deve rodar só a partir da 2ª rodada quando StartStage=2; chamadas=%v", h.plannerCalls)
	}
	if !equalInts(h.criticCalls, []int{1, 2}) {
		t.Errorf("critic=%v, esperava [1 2]", h.criticCalls)
	}
}

func TestRunSpecLoopExhaustsAttempts(t *testing.T) {
	h := &specHarness{
		plannerReplies: []AgentResult{{Stats: tok(1, 1)}, {Stats: tok(1, 1)}, {Stats: tok(1, 1)}},
		criticReplies:  []AgentResult{{Stats: tok(1, 1)}, {Stats: tok(1, 1)}, {Stats: tok(1, 1)}},
		reviews:        []string{reviewRevision, reviewRevision, reviewRevision},
	}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopRejected {
		t.Fatalf("Status = %v, esperava specLoopRejected", got.Status)
	}
	if got.Attempts != 3 {
		t.Errorf("Attempts = %d, esperava 3", got.Attempts)
	}
	if len(h.plannerCalls) != 3 || len(h.criticCalls) != 3 {
		t.Errorf("planner=%v critic=%v, esperava 3 rodadas", h.plannerCalls, h.criticCalls)
	}
	if got.Tokens != tok(6, 6) {
		t.Errorf("Tokens = %+v, esperava %+v", got.Tokens, tok(6, 6))
	}
	want := "[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)...\n" +
		"[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 3/3)...\n"
	if h.out.String() != want {
		t.Errorf("avisos = %q, esperava %q (sem aviso após a última tentativa)", h.out.String(), want)
	}
	if got.Review != reviewRevision {
		t.Errorf("Review deve ser o último parecer lido: %q", got.Review)
	}
}

func TestRunSpecLoopMaxOneRejectsWithoutWarning(t *testing.T) {
	h := &specHarness{reviews: []string{reviewRevision}}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 1}, h.deps())
	if got.Status != specLoopRejected || got.Attempts != 1 {
		t.Fatalf("Status=%v Attempts=%d, esperava rejeitado na tentativa 1", got.Status, got.Attempts)
	}
	if h.out.Len() != 0 {
		t.Errorf("não deve haver aviso de reexecução: %q", h.out.String())
	}
}

func TestRunSpecLoopNonPositiveMaxAttemptsMeansOneRound(t *testing.T) {
	for _, max := range []int{0, -2} {
		h := &specHarness{reviews: []string{reviewRevision, reviewApproved}}
		got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: max}, h.deps())
		if got.Status != specLoopRejected || got.Attempts != 1 {
			t.Errorf("max=%d: Status=%v Attempts=%d, esperava 1 rodada rejeitada", max, got.Status, got.Attempts)
		}
		if len(h.plannerCalls) != 1 || len(h.criticCalls) != 1 {
			t.Errorf("max=%d: planner=%v critic=%v", max, h.plannerCalls, h.criticCalls)
		}
	}
}

func TestRunSpecLoopClearsReviewBeforeEachCritic(t *testing.T) {
	h := &specHarness{reviews: []string{reviewRevision, reviewApproved}}
	runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	want := []string{"P", "clear", "C", "read", "P", "clear", "C", "read"}
	if strings.Join(h.events, ",") != strings.Join(want, ",") {
		t.Errorf("ordem = %v, esperava %v", h.events, want)
	}
}

func TestRunSpecLoopNilClearReviewIsAllowed(t *testing.T) {
	h := &specHarness{reviews: []string{reviewApproved}}
	d := h.deps()
	d.ClearReview = nil
	if got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, d); got.Status != specLoopApproved {
		t.Errorf("Status = %v com ClearReview nil", got.Status)
	}
}

func TestRunSpecLoopNilOutIsAllowed(t *testing.T) {
	h := &specHarness{reviews: []string{reviewRevision, reviewApproved}}
	d := h.deps()
	d.Out = nil
	if got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, d); got.Status != specLoopApproved {
		t.Errorf("Status = %v com Out nil", got.Status)
	}
}

func TestRunSpecLoopPlannerQuotaStopsLoop(t *testing.T) {
	h := &specHarness{
		plannerReplies: []AgentResult{{Stats: tok(10, 1)}, {Stats: tok(5, 0), Quota: true, HasReset: true}},
		criticReplies:  []AgentResult{{Stats: tok(2, 2)}},
		reviews:        []string{reviewRevision},
	}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopQuota {
		t.Fatalf("Status = %v, esperava specLoopQuota", got.Status)
	}
	if got.Stage != 1 || got.Attempts != 2 {
		t.Errorf("Stage=%d Attempts=%d, esperava Stage 1 / Attempts 2", got.Stage, got.Attempts)
	}
	if !got.Result.Quota || !got.Result.HasReset {
		t.Errorf("Result deve ser o AgentResult da cota: %+v", got.Result)
	}
	if len(h.criticCalls) != 1 {
		t.Errorf("CRITIC não deve rodar após cota no PLANNER: %v", h.criticCalls)
	}
	if got.Tokens != tok(17, 3) {
		t.Errorf("Tokens = %+v, esperava incluir a rodada interrompida %+v", got.Tokens, tok(17, 3))
	}
}

func TestRunSpecLoopCriticQuotaStopsLoop(t *testing.T) {
	h := &specHarness{
		plannerReplies: []AgentResult{{Stats: tok(10, 1)}},
		criticReplies:  []AgentResult{{Stats: tok(3, 0), Quota: true}},
	}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopQuota || got.Stage != 2 || got.Attempts != 1 {
		t.Fatalf("Status=%v Stage=%d Attempts=%d, esperava cota na Etapa 2 / tentativa 1", got.Status, got.Stage, got.Attempts)
	}
	if got.Tokens != tok(13, 1) {
		t.Errorf("Tokens = %+v", got.Tokens)
	}
	for _, e := range h.events {
		if e == "read" {
			t.Errorf("SPEC_REVIEW não deve ser lido após cota do CRITIC: %v", h.events)
		}
	}
}

func TestRunSpecLoopPlannerErrorFails(t *testing.T) {
	boom := errors.New("exit status 1")
	h := &specHarness{plannerReplies: []AgentResult{{Stats: tok(1, 0), Err: boom}}}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopFailed || got.Stage != 1 {
		t.Fatalf("Status=%v Stage=%d, esperava falha na Etapa 1", got.Status, got.Stage)
	}
	if !errors.Is(got.Err, boom) {
		t.Errorf("Err = %v, esperava envolver %v", got.Err, boom)
	}
	if len(h.criticCalls) != 0 {
		t.Errorf("CRITIC não deve rodar após falha do PLANNER")
	}
	if got.Tokens != tok(1, 0) {
		t.Errorf("Tokens = %+v", got.Tokens)
	}
}

func TestRunSpecLoopCriticErrorFails(t *testing.T) {
	boom := errors.New("exit status 2")
	h := &specHarness{criticReplies: []AgentResult{{Err: boom}}}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopFailed || got.Stage != 2 || !errors.Is(got.Err, boom) {
		t.Fatalf("Status=%v Stage=%d Err=%v, esperava falha na Etapa 2", got.Status, got.Stage, got.Err)
	}
}

func TestRunSpecLoopMissingReviewFails(t *testing.T) {
	h := &specHarness{reviewErr: errors.New("no such file")}
	got := runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 3}, h.deps())
	if got.Status != specLoopFailed || got.Stage != 2 {
		t.Fatalf("Status=%v Stage=%d, esperava falha na Etapa 2", got.Status, got.Stage)
	}
	if got.Err == nil || !strings.Contains(got.Err.Error(), "SPEC_REVIEW.md") {
		t.Errorf("Err deve citar SPEC_REVIEW.md: %v", got.Err)
	}
	if len(h.plannerCalls) != 1 {
		t.Errorf("sem parecer não deve haver nova rodada: planner=%v", h.plannerCalls)
	}
}

func TestRunSpecLoopWarningGoesToRunLog(t *testing.T) {
	closeLog := withTestRunLog(t)
	h := &specHarness{reviews: []string{reviewRevision, reviewApproved}}
	runSpecLoop(specLoopConfig{StartStage: 1, MaxAttempts: 2}, h.deps())
	logContent := closeLog()
	if !strings.Contains(logContent, "[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/2)...") {
		t.Errorf("aviso deve ir para o log de execução:\n%s", logContent)
	}
}

// ---------------------------------------------------------------------------
// Task 4 — fiação no main (makercycle.go)
// ---------------------------------------------------------------------------

func TestMainUsesSpecLoop(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	for _, want := range []string{
		"runSpecLoop(",
		"buildPlannerPrompt(",
		"buildCriticPrompt(",
		"finish(exitSpecRejected)",
		"specAbortNote(",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("makercycle.go deve conter %q", want)
		}
	}
	if strings.Contains(src, "execução linear, uma passada") {
		t.Errorf("makercycle.go ainda descreve as Etapas 1 e 2 como execução linear de uma passada")
	}
}
