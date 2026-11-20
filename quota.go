package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"time"
)

var (
	quotaMarkerRegex = regexp.MustCompile(`(?i)(resource_exhausted|\bcode 429\b|429 too many requests|quota reached|rate_limit_error|credit balance is too low|insufficient_quota)`)
	quotaResetRegex  = regexp.MustCompile(`(?i)resets in\s+((?:\d+[hms])+)`)
)

// detectQuotaError verifica se a saída indica erro de cota/rate limit e extrai o tempo de reset, se disponível.
func detectQuotaError(output string) (isQuota bool, resetDuration time.Duration, hasReset bool) {
	if !quotaMarkerRegex.MatchString(output) {
		return false, 0, false
	}
	m := quotaResetRegex.FindStringSubmatch(output)
	if len(m) >= 2 {
		d, err := time.ParseDuration(m[1])
		if err == nil {
			return true, d, true
		}
	}
	return true, 0, false
}

// formatResetETA formata o tempo estimado para liberação de cota em linguagem humana.
func formatResetETA(now time.Time, d time.Duration, hasReset bool) string {
	if !hasReset {
		return "tempo de reset não informado pelo provedor"
	}
	t := now.Add(d)
	return fmt.Sprintf("%s (liberação prevista em %s às %s)", d.String(), t.Format("02/01/2006"), t.Format("15:04"))
}

// agentRunner abstrai a execução de um comando de agente.
type agentRunner func(agent AgentBinding, env []string, prompt string, tryJSON bool) (TokenStats, string, error)

// AgentResult consolida a resposta de execução de um agente com suporte a fallback e cota.
type AgentResult struct {
	Stats         TokenStats
	Output        string
	Err           error
	Quota         bool // true => interrupção por cota (sem fallback bem-sucedido)
	ResetIn       time.Duration
	HasReset      bool
	UsedOmniRoute bool
	Model         string // modelo para telemetria
}

// omniRouteBinding cria uma cópia do binding do agente adaptada para o OmniRoute (binário claude e modelo do OmniRoute).
func omniRouteBinding(agent AgentBinding, omni OmniRouteConfig) AgentBinding {
	c := agent
	c.Bin = "claude"
	c.Model = omni.Model
	return c
}

// telemetryModel adiciona o prefixo omniroute: ao nome do modelo se executado via OmniRoute.
func telemetryModel(model string, viaOmni bool) string {
	if viaOmni {
		return "omniroute:" + model
	}
	return model
}

// omniRouteNotice gera o texto de aviso puro de transição para o OmniRoute.
func omniRouteNotice(stageLabel string, omni OmniRouteConfig) string {
	return fmt.Sprintf("[AVISO] Limite de cota/tokens atingido na %s. Entrando no OmniRoute (%s | modelo: %s)...", stageLabel, omni.URL, omni.Model)
}

type yellowWriterWrapper struct {
	w io.Writer
}

func (y yellowWriterWrapper) Write(p []byte) (int, error) {
	var buf bytes.Buffer
	buf.WriteString("\033[33m")
	buf.Write(p)
	buf.WriteString("\033[0m")
	_, err := y.w.Write(buf.Bytes())
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

// yellowWriter decora um io.Writer com sequências ANSI amarelas.
func yellowWriter(w io.Writer) io.Writer {
	return yellowWriterWrapper{w: w}
}

// runAgentWithFallback executa o agente primário e realiza fallback transparente para OmniRoute se houver erro de cota.
func runAgentWithFallback(
	agent AgentBinding,
	prompt string,
	tryJSON bool,
	authMode string,
	omni OmniRouteConfig,
	base []string,
	run agentRunner,
	notify io.Writer,
) AgentResult {
	primEnv := buildAgentEnv(base, authMode, omni, false)
	stats1, out1, err1 := run(agent, primEnv, prompt, tryJSON)

	if err1 == nil {
		return AgentResult{
			Stats:         stats1,
			Output:        out1,
			Err:           nil,
			Quota:         false,
			UsedOmniRoute: false,
			Model:         agent.Model,
		}
	}

	isQuota, resetIn, hasReset := detectQuotaError(out1)
	if !isQuota {
		return AgentResult{
			Stats:         stats1,
			Output:        out1,
			Err:           err1,
			Quota:         false,
			UsedOmniRoute: false,
			Model:         agent.Model,
		}
	}

	stageLabel := agent.Label
	if agent.Stage != "" {
		stageLabel = agent.Label + ": " + agent.Stage
	}

	if !omni.Enabled {
		return AgentResult{
			Stats:         stats1,
			Output:        out1,
			Err:           err1,
			Quota:         true,
			ResetIn:       resetIn,
			HasReset:      hasReset,
			UsedOmniRoute: false,
			Model:         agent.Model,
		}
	}

	notice := omniRouteNotice(stageLabel, omni)
	if notify != nil {
		fmt.Fprintln(notify, notice)
	}
	logEvent("%s", notice)

	omniAgent := omniRouteBinding(agent, omni)
	omniEnv := buildAgentEnv(base, authMode, omni, true)
	stats2, out2, err2 := run(omniAgent, omniEnv, prompt, tryJSON)

	combinedStats := TokenStats{
		Input:  stats1.Input + stats2.Input,
		Output: stats1.Output + stats2.Output,
		Total:  stats1.Total + stats2.Total,
	}
	model := telemetryModel(omni.Model, true)

	if err2 == nil {
		return AgentResult{
			Stats:         combinedStats,
			Output:        out2,
			Err:           nil,
			Quota:         false,
			UsedOmniRoute: true,
			Model:         model,
		}
	}

	return AgentResult{
		Stats:         combinedStats,
		Output:        out2,
		Err:           err2,
		Quota:         true,
		ResetIn:       resetIn,
		HasReset:      hasReset,
		UsedOmniRoute: true,
		Model:         model,
	}
}
