package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 3 — detectQuotaError(output string) (isQuota bool, resetDuration time.Duration, hasReset bool)
// ---------------------------------------------------------------------------

func TestDetectQuotaError(t *testing.T) {
	cases := []struct {
		id       string
		in       string
		isQuota  bool
		reset    time.Duration
		hasReset bool
	}{
		{"Q-H1", "Error: RESOURCE_EXHAUSTED. Resets in 142h11m42s", true, 142*time.Hour + 11*time.Minute + 42*time.Second, true},
		{"Q-H2", "API Error: code 429", true, 0, false},
		{"Q-H3", "HTTP 429 Too Many Requests — Resets in 45m10s", true, 45*time.Minute + 10*time.Second, true},
		{"Q-H4", "You have hit your quota reached limit. Resets in 30s.", true, 30 * time.Second, true},
		{"Q-H5", `{"type":"rate_limit_error","message":"slow down"}`, true, 0, false},
		{"Q-H6", "Your credit balance is too low to access the API", true, 0, false},
		{"Q-H7", "insufficient_quota: billing", true, 0, false},
		{"Q-E1", "resource_exhausted\nresets IN 2h", true, 2 * time.Hour, true},
		{"Q-E2", "stdout ok\nstderr: Quota Reached", true, 0, false},
		{"Q-E3", "RESOURCE_EXHAUSTED Resets in soon", true, 0, false},
		{"Q-E4", "RESOURCE_EXHAUSTED Resets in 1h0m0s", true, time.Hour, true},
		{"Q-X1", "", false, 0, false},
		{"Q-X2", "panic: nil pointer dereference", false, 0, false},
		{"Q-X3", "Resets in 10m (but no quota marker)", false, 0, false},
		{"Q-X4", "error code 4290 unrelated", false, 0, false},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			isQ, d, has := detectQuotaError(c.in)
			if isQ != c.isQuota || d != c.reset || has != c.hasReset {
				t.Fatalf("detectQuotaError(%q) = (%v, %v, %v); want (%v, %v, %v)",
					c.in, isQ, d, has, c.isQuota, c.reset, c.hasReset)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 3 — formatResetETA(now time.Time, d time.Duration, hasReset bool) string
// ---------------------------------------------------------------------------

func TestFormatResetETA(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	now := time.Date(2026, 11, 17, 22, 0, 0, 0, loc)
	cases := []struct {
		id   string
		d    time.Duration
		has  bool
		want string
	}{
		{"ETA-H1", 45*time.Minute + 10*time.Second, true, "45m10s (liberação prevista em 17/11/2026 às 22:45)"},
		{"ETA-H2", 142*time.Hour + 11*time.Minute + 42*time.Second, true, "142h11m42s (liberação prevista em 23/11/2026 às 20:11)"},
		{"ETA-E1", 3 * time.Hour, true, "3h0m0s (liberação prevista em 18/11/2026 às 01:00)"},
		{"ETA-E2", 30 * time.Second, true, "30s (liberação prevista em 17/11/2026 às 22:00)"},
		{"ETA-X1", 0, false, "tempo de reset não informado pelo provedor"},
		{"ETA-X2", 10 * time.Minute, false, "tempo de reset não informado pelo provedor"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := formatResetETA(now, c.d, c.has); got != c.want {
				t.Fatalf("formatResetETA = %q; want %q", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 4 — OmniRoute: omniRouteBinding, telemetryModel, omniRouteNotice
// ---------------------------------------------------------------------------

func TestOmniRouteBinding(t *testing.T) {
	// OB-H1: usa o binário claude e o modelo do OmniRoute, preservando identidade/args da etapa.
	in := AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Bin: "agy",
		Model: "Gemini 3.8 Flash (High)", Skill: "x/y", Args: "--dangerously-skip-permissions"}
	got := omniRouteBinding(in, testOmni)
	want := in
	want.Bin = "claude"
	want.Model = "claude-sonnet-4-6"
	if got != want {
		t.Fatalf("omniRouteBinding:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestTelemetryModel(t *testing.T) {
	if got := telemetryModel("opus", false); got != "opus" {
		t.Fatalf("TM-H1: got %q", got)
	}
	if got := telemetryModel("claude-sonnet-4-6", true); got != "omniroute:claude-sonnet-4-6" {
		t.Fatalf("TM-H2: got %q", got)
	}
}

func TestOmniRouteNotice(t *testing.T) {
	got := omniRouteNotice("ETAPA 3: MAKE", testOmni)
	want := "[AVISO] Limite de cota/tokens atingido na ETAPA 3: MAKE. Entrando no OmniRoute (http://localhost:20128 | modelo: claude-sonnet-4-6)..."
	if got != want {
		t.Fatalf("omniRouteNotice:\n got: %q\nwant: %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Task 4 — runAgentWithFallback(agent, prompt, tryJSON, authMode, omni, base, run, notify) AgentResult
// ---------------------------------------------------------------------------

type fakeCall struct {
	agent AgentBinding
	env   []string
}

type fakeReply struct {
	stats TokenStats
	out   string
	err   error
}

func fakeRunner(replies []fakeReply, calls *[]fakeCall) agentRunner {
	return func(agent AgentBinding, env []string, prompt string, tryJSON bool) (TokenStats, string, error) {
		i := len(*calls)
		*calls = append(*calls, fakeCall{agent: agent, env: env})
		if i >= len(replies) {
			return TokenStats{}, "", errors.New("chamada inesperada")
		}
		r := replies[i]
		return r.stats, r.out, r.err
	}
}

var fbAgent = AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Bin: "agy", Model: "gem"}

var quotaReply = fakeReply{TokenStats{Input: 10, Output: 0, Total: 10}, "RESOURCE_EXHAUSTED. Resets in 45m10s", errors.New("exit status 1")}

func TestRunAgentWithFallbackSuccessNoFallback(t *testing.T) {
	// FB-H1: sucesso primário => uma única chamada, modelo original, sem aviso.
	var calls []fakeCall
	var notify bytes.Buffer
	run := fakeRunner([]fakeReply{{TokenStats{Input: 5, Output: 7, Total: 12}, "ok", nil}}, &calls)
	res := runAgentWithFallback(fbAgent, "p", true, "api_key", testOmni, []string{"ANTHROPIC_API_KEY=k"}, run, &notify)
	if res.Err != nil || res.Quota || res.UsedOmniRoute || res.Model != "gem" || res.Stats.Total != 12 || res.Output != "ok" {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if len(calls) != 1 || calls[0].agent != fbAgent {
		t.Fatalf("esperada 1 chamada com o binding original: %+v", calls)
	}
	assertEnvSingle(t, calls[0].env, "ANTHROPIC_API_KEY", "k")
	if notify.Len() != 0 {
		t.Fatalf("não deveria avisar: %q", notify.String())
	}
}

func TestRunAgentWithFallbackQuotaWithOmniSuccess(t *testing.T) {
	// FB-H2: cota + OmniRoute ativo => aviso, reexecução via claude com env do OmniRoute.
	var calls []fakeCall
	var notify bytes.Buffer
	run := fakeRunner([]fakeReply{quotaReply, {TokenStats{Input: 3, Output: 4, Total: 7}, "done", nil}}, &calls)
	res := runAgentWithFallback(fbAgent, "p", true, "another", testOmni, []string{"LLM_API_KEY=k"}, run, &notify)
	if res.Err != nil || res.Quota || !res.UsedOmniRoute {
		t.Fatalf("fallback deveria ter sucesso: %+v", res)
	}
	if res.Model != "omniroute:claude-sonnet-4-6" {
		t.Fatalf("Model para telemetria = %q", res.Model)
	}
	if res.Stats.Total != 17 || res.Output != "done" {
		t.Fatalf("stats devem somar as duas chamadas e output deve ser o do OmniRoute: %+v", res)
	}
	if len(calls) != 2 || calls[1].agent.Bin != "claude" || calls[1].agent.Model != "claude-sonnet-4-6" {
		t.Fatalf("segunda chamada deve usar claude + modelo OmniRoute: %+v", calls)
	}
	assertEnvAbsent(t, calls[0].env, "LLM_API_KEY") // modo alternativo na chamada primária
	assertEnvSingle(t, calls[1].env, "ANTHROPIC_BASE_URL", "http://localhost:20128")
	assertEnvSingle(t, calls[1].env, "ANTHROPIC_AUTH_TOKEN", "omniroute-local")
	if !strings.Contains(notify.String(), omniRouteNotice("ETAPA 3: MAKE", testOmni)) {
		t.Fatalf("aviso ausente: %q", notify.String())
	}
}

func TestRunAgentWithFallbackQuotaOmniDisabled(t *testing.T) {
	// FB-X1: cota + OmniRoute desativado => Quota=true, sem segunda chamada, reset preservado.
	var calls []fakeCall
	var notify bytes.Buffer
	off := testOmni
	off.Enabled = false
	run := fakeRunner([]fakeReply{quotaReply}, &calls)
	res := runAgentWithFallback(fbAgent, "p", true, "api_key", off, nil, run, &notify)
	if !res.Quota || res.Err == nil || res.UsedOmniRoute || len(calls) != 1 {
		t.Fatalf("resultado inesperado: %+v (calls=%d)", res, len(calls))
	}
	if !res.HasReset || res.ResetIn != 45*time.Minute+10*time.Second {
		t.Fatalf("reset não propagado: %+v", res)
	}
	if res.Model != "gem" {
		t.Fatalf("Model = %q; want gem", res.Model)
	}
}

func TestRunAgentWithFallbackOmniAlsoFails(t *testing.T) {
	// FB-X2: OmniRoute também falha => Quota=true, Err do OmniRoute, reset da chamada primária.
	var calls []fakeCall
	var notify bytes.Buffer
	omniErr := errors.New("connection refused")
	run := fakeRunner([]fakeReply{quotaReply, {TokenStats{Total: 1}, "dial tcp: connection refused", omniErr}}, &calls)
	res := runAgentWithFallback(fbAgent, "p", true, "api_key", testOmni, nil, run, &notify)
	if !res.Quota || !errors.Is(res.Err, omniErr) || !res.UsedOmniRoute || len(calls) != 2 {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if !res.HasReset || res.ResetIn != 45*time.Minute+10*time.Second {
		t.Fatalf("reset da chamada primária deve ser mantido: %+v", res)
	}
	if res.Stats.Total != 11 {
		t.Fatalf("stats devem somar as duas chamadas: %+v", res.Stats)
	}
}

func TestRunAgentWithFallbackNonQuotaError(t *testing.T) {
	// FB-E1: erro comum (não cota) => sem fallback, Quota=false.
	var calls []fakeCall
	var notify bytes.Buffer
	boom := errors.New("exit status 2")
	run := fakeRunner([]fakeReply{{TokenStats{}, "segfault", boom}}, &calls)
	res := runAgentWithFallback(fbAgent, "p", true, "api_key", testOmni, nil, run, &notify)
	if res.Quota || res.UsedOmniRoute || !errors.Is(res.Err, boom) || len(calls) != 1 {
		t.Fatalf("resultado inesperado: %+v", res)
	}
}

func TestRunAgentWithFallbackQuotaTextOnSuccessIsIgnored(t *testing.T) {
	// FB-E2: saída com marcador de cota mas processo com sucesso => NÃO é cota (evita falso positivo
	// quando o agente apenas discute "429"/RESOURCE_EXHAUSTED no conteúdo).
	var calls []fakeCall
	var notify bytes.Buffer
	run := fakeRunner([]fakeReply{{TokenStats{Total: 2}, "trate RESOURCE_EXHAUSTED e code 429", nil}}, &calls)
	res := runAgentWithFallback(fbAgent, "p", true, "api_key", testOmni, nil, run, &notify)
	if res.Quota || res.UsedOmniRoute || res.Err != nil || len(calls) != 1 {
		t.Fatalf("resultado inesperado: %+v", res)
	}
}

// ---------------------------------------------------------------------------
// Task 4 (revisão SPEC_REVIEW) — ANSI apenas no terminal, nunca no log em disco
// ---------------------------------------------------------------------------

// withTestRunLog ativa um log de execução temporário e devolve uma função que o fecha e retorna o conteúdo.
func withTestRunLog(t *testing.T) func() string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "docs")
	l, err := initRunLog(dir, "fallback")
	if err != nil {
		t.Fatal(err)
	}
	prev := runLog
	runLog = l
	return func() string {
		l.Close()
		runLog = prev
		b, _ := os.ReadFile(filepath.Join(dir, "logs", l.Name))
		return string(b)
	}
}

func TestRunAgentWithFallbackNoticeIsPlainTextInNotifyAndLog(t *testing.T) {
	// FB-E3: o aviso escrito em notify e gravado no log é texto puro (a cor é responsabilidade de yellowWriter).
	closeLog := withTestRunLog(t)
	var calls []fakeCall
	var notify bytes.Buffer
	run := fakeRunner([]fakeReply{quotaReply, {TokenStats{Total: 1}, "ok", nil}}, &calls)
	_ = runAgentWithFallback(fbAgent, "p", true, "api_key", testOmni, nil, run, &notify)
	logged := closeLog()
	notice := omniRouteNotice("ETAPA 3: MAKE", testOmni)
	if notify.String() != notice+"\n" {
		t.Fatalf("notify deve receber exatamente o aviso em texto puro: %q", notify.String())
	}
	if !strings.Contains(logged, notice) {
		t.Fatalf("log sem o aviso do OmniRoute:\n%s", logged)
	}
	if strings.Contains(logged, "\033") {
		t.Fatalf("log em disco não pode conter ANSI:\n%s", logged)
	}
}

func TestRunAgentWithFallbackStageLabelWithoutStage(t *testing.T) {
	// FB-E4: binding sem Stage => rótulo usa apenas Label.
	var calls []fakeCall
	var notify bytes.Buffer
	a := fbAgent
	a.Stage = ""
	run := fakeRunner([]fakeReply{quotaReply, {TokenStats{Total: 1}, "ok", nil}}, &calls)
	_ = runAgentWithFallback(a, "p", true, "api_key", testOmni, nil, run, &notify)
	if !strings.Contains(notify.String(), "atingido na ETAPA 3. Entrando") {
		t.Fatalf("rótulo inesperado: %q", notify.String())
	}
}

func TestYellowWriterWrapsInANSI(t *testing.T) {
	// YW-H1: yellowWriter envolve cada escrita em \033[33m … \033[0m e reporta o tamanho original.
	var buf bytes.Buffer
	w := yellowWriter(&buf)
	n, err := w.Write([]byte("aviso\n"))
	if err != nil || n != len("aviso\n") {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if buf.String() != "\033[33maviso\n\033[0m" {
		t.Fatalf("saída = %q", buf.String())
	}
}

// ---------------------------------------------------------------------------
// Task 4 (revisão SPEC_REVIEW) — execCaptureEnv, runAgent retrocompatível e runAgentStep
// ---------------------------------------------------------------------------

func TestExecCaptureEnvUsesGivenEnv(t *testing.T) {
	// EC-H1: env explícito substitui o ambiente herdado.
	t.Setenv("MC_EC_PARENT", "herdado")
	out, err := execCaptureEnv("sh", []string{"-c", "printf '%s|%s' \"$MC_EC_CHILD\" \"$MC_EC_PARENT\""},
		[]string{"MC_EC_CHILD=filho", "PATH=" + os.Getenv("PATH")})
	if err != nil || out != "filho|" {
		t.Fatalf("execCaptureEnv = %q, %v; want \"filho|\"", out, err)
	}
}

func TestExecCaptureEnvNilInherits(t *testing.T) {
	// EC-E1: env nil => herda o ambiente do processo (comportamento de execCapture).
	t.Setenv("MC_EC_PARENT", "herdado")
	out, err := execCaptureEnv("sh", []string{"-c", "printf '%s' \"$MC_EC_PARENT\""}, nil)
	if err != nil || out != "herdado" {
		t.Fatalf("execCaptureEnv(nil env) = %q, %v", out, err)
	}
}

func TestRunAgentKeepsLegacySignature(t *testing.T) {
	// RA-H1: runAgent mantém (TokenStats, error) — usado por runProjectSetup e TestRunAgentLogsPromptAndFullResponse.
	var _ func(AgentBinding, string, bool) (TokenStats, error) = runAgent
}

func TestRunAgentStepReturnsAgentResultAndLogs(t *testing.T) {
	// RA-H2: runAgentStep executa o binding real, devolve AgentResult e registra prompt/resposta sem ANSI.
	closeLog := withTestRunLog(t)
	a := AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Bin: "echo", Model: "m1"}
	off := testOmni
	off.Enabled = false
	res := runAgentStep(a, "PROMPT-STEP-456", false, "api_key", off)
	logged := closeLog()
	if res.Err != nil || res.Quota || res.UsedOmniRoute || res.Stats.Total <= 0 || res.Model != "m1" {
		t.Fatalf("resultado inesperado: %+v", res)
	}
	if !strings.Contains(res.Output, "PROMPT-STEP-456") {
		t.Fatalf("Output deve conter a resposta do agente: %q", res.Output)
	}
	for _, want := range []string{"PROMPT ENVIADO — ETAPA 3: MAKE — MAKER | m1", "RESPOSTA RECEBIDA", "concluído em"} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log sem %q:\n%s", want, logged)
		}
	}
	if strings.Contains(logged, "\033") {
		t.Fatal("log com ANSI")
	}
}

func TestRunAgentStepNonQuotaFailure(t *testing.T) {
	// RA-X1: binário que falha sem marcador de cota => Err preenchido, Quota=false, sem OmniRoute.
	closeLog := withTestRunLog(t)
	defer closeLog()
	a := AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Bin: "false", Model: "m1"}
	res := runAgentStep(a, "p", false, "api_key", testOmni)
	if res.Err == nil || res.Quota || res.UsedOmniRoute {
		t.Fatalf("resultado inesperado: %+v", res)
	}
}
