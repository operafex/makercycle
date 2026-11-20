package main

import (
	"fmt"
	"io"
	"strings"
)

// exitSpecRejected é o código de saída quando o CRITIC reprova a especificação em todas as tentativas.
const exitSpecRejected = 2

const (
	specReviewPath     = ".makercycle/state/SPEC_REVIEW.md"
	specApprovedMarker = "SPEC_STATUS: APPROVED"
)

// specRevisionInstruction é concatenada ao prompt do PLANNER a partir da 2ª rodada do loop.
const specRevisionInstruction = " ATENÇÃO: Rodada de revisão da especificação. Leia @.makercycle/state/SPEC_REVIEW.md e corrija .makercycle/state/PLAN.md, .makercycle/state/TEST_CASES.md e os testes RED conforme todos os apontamentos do CRITIC."

type specLoopAction int

const (
	specActionApprove specLoopAction = iota // segue para a Etapa 3
	specActionRetry                         // reexecuta a Etapa 1
	specActionAbort                         // tentativas esgotadas: sai com código 2
)

// decideSpecLoop decide o próximo passo do loop a partir do parecer do CRITIC.
func decideSpecLoop(review string, specAttempt, maxAttempts int) specLoopAction {
	if strings.Contains(review, specApprovedMarker) {
		return specActionApprove
	}
	if specAttempt < maxAttempts {
		return specActionRetry
	}
	return specActionAbort
}

// specRevisionWarning é o aviso exibido antes de reexecutar a Etapa 1.
func specRevisionWarning(nextAttempt, maxAttempts int) string {
	return fmt.Sprintf("[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa %d/%d)...", nextAttempt, maxAttempts)
}

// specAbortNote é a nota gravada na telemetria quando as tentativas se esgotam.
func specAbortNote(attempts int) string {
	return fmt.Sprintf("Especificação reprovada pelo CRITIC após %d tentativa(s)", attempts)
}

// buildPlannerPrompt monta o prompt da Etapa 1. A partir da 2ª rodada, acrescenta a instrução de revisão.
func buildPlannerPrompt(skill, docsDir, stackName, task, demandFile string, refs []string, specAttempt int) string {
	prompt := fmt.Sprintf(
		"Siga estritamente as instruções de @.makercycle/agents/01_PLANNER.md aplicando a skill %s. "+
			"Diretório de documentação: %s/. Stack: %s. Demanda: %s. Arquivo da demanda: %s. "+
			"Grave .makercycle/state/PLAN.md, .makercycle/state/TEST_CASES.md e os testes em estado RED.",
		skill, docsDir, stackName, task, demandFile,
	) + referencesPrompt(refs)
	if specAttempt > 1 {
		prompt += specRevisionInstruction
	}
	return prompt
}

// buildCriticPrompt monta o prompt da Etapa 2.
func buildCriticPrompt(skill, docsDir string) string {
	return fmt.Sprintf(
		"Siga estritamente as instruções de @.makercycle/agents/02_CRITIC.md aplicando a skill %s. "+
			"Diretório de documentação: %s/. Audite @.makercycle/state/PLAN.md, @.makercycle/state/TEST_CASES.md e os arquivos de teste recém-criados. "+
			"Grave seu veredito em .makercycle/state/SPEC_REVIEW.md.",
		skill, docsDir,
	)
}

type specLoopStatus int

const (
	specLoopApproved specLoopStatus = iota // SPEC_STATUS: APPROVED
	specLoopRejected                       // tentativas esgotadas sem aprovação
	specLoopQuota                          // interrupção por cota (AgentResult.Quota)
	specLoopFailed                         // erro de processo ou SPEC_REVIEW.md ausente
)

type specLoopConfig struct {
	StartStage  int // 1 ou 2
	MaxAttempts int // <= 0 é tratado como 1
}

type specLoopDeps struct {
	RunPlanner  func(specAttempt int) AgentResult // monta o prompt, executa e registra telemetria S1
	RunCritic   func(specAttempt int) AgentResult // executa e registra telemetria S2
	ClearReview func()                            // remove SPEC_REVIEW.md antes de cada CRITIC (nil = no-op)
	ReadReview  func() (string, error)            // lê SPEC_REVIEW.md
	Out         io.Writer                         // terminal (nil = descarta)
}

type specLoopOutcome struct {
	Status   specLoopStatus
	Attempts int         // última specAttempt executada (1..max)
	Tokens   TokenStats  // soma de TODAS as rodadas, inclusive a interrompida
	Stage    int         // 1 ou 2: etapa onde parou (Quota/Failed)
	Result   AgentResult // AgentResult que causou Quota/Failed
	Review   string      // último parecer lido
	Err      error       // Failed: erro (envolve res.Err via %w, ou cita SPEC_REVIEW.md)
}

// runSpecLoop executa o loop PLAN (PLANNER) <-> CHALLENGE (CRITIC) até a aprovação ou o limite de tentativas.
func runSpecLoop(cfg specLoopConfig, deps specLoopDeps) specLoopOutcome {
	maxAttempts := cfg.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	var out specLoopOutcome

	// stop encerra o loop numa interrupção (cota ou falha de processo) da etapa informada.
	stop := func(stage int, res AgentResult) (specLoopOutcome, bool) {
		out.Tokens.Add(res.Stats)
		switch {
		case res.Quota:
			out.Status, out.Stage, out.Result = specLoopQuota, stage, res
			return out, true
		case res.Err != nil:
			name := "PLANNER"
			if stage == 2 {
				name = "CRITIC"
			}
			out.Status, out.Stage, out.Result = specLoopFailed, stage, res
			out.Err = fmt.Errorf("Etapa %d (%s): %w", stage, name, res.Err)
			return out, true
		}
		return out, false
	}

	for a := 1; a <= maxAttempts; a++ {
		out.Attempts = a

		if a > 1 || cfg.StartStage <= 1 {
			if o, stopped := stop(1, deps.RunPlanner(a)); stopped {
				return o
			}
		}

		if deps.ClearReview != nil {
			deps.ClearReview()
		}
		if o, stopped := stop(2, deps.RunCritic(a)); stopped {
			return o
		}

		review, err := deps.ReadReview()
		if err != nil {
			out.Status, out.Stage = specLoopFailed, 2
			out.Err = fmt.Errorf("o CRITIC não gerou %s: %w", specReviewPath, err)
			return out
		}
		out.Review = review

		switch decideSpecLoop(review, a, maxAttempts) {
		case specActionApprove:
			out.Status = specLoopApproved
			return out
		case specActionAbort:
			out.Status = specLoopRejected
			return out
		}
		msg := specRevisionWarning(a+1, maxAttempts)
		if deps.Out != nil {
			fmt.Fprintln(deps.Out, msg)
		}
		logEvent("%s", msg)
	}

	// Inalcançável: decideSpecLoop aborta na última tentativa.
	out.Status = specLoopRejected
	return out
}
