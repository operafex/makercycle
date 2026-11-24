package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// Ações devolvidas pela pausa do modo -manual, status gravados e código de saída do abort.
const (
	manualActionContinue = "continue"
	manualActionRedo     = "redo"
	manualActionSave     = "save"
	manualActionAbort    = "abort"
	manualAbortStatus    = "ABORTED"
	manualSaveStatus     = "MANUAL_PAUSED"
	exitManualAbort      = 6
)

const (
	manualFeedbackMarker        = "FEEDBACK DO REVISOR HUMANO (modo -manual):"
	manualDatabasePromptTimeout = 24 * time.Hour
)

var (
	errManualNonInteractive = errors.New("o modo -manual requer um terminal interativo: a entrada padrão terminou (EOF) sem resposta")
	errManualAborted        = errors.New("execução abortada pelo usuário no modo -manual")
)

// normalizeManualArgs move os tokens -manual/--manual (e -manual=.../--manual=...) para logo após args[0].
// Assim o pacote flag os reconhece mesmo depois de uma flag que consome valor (ex.: -task "x" -manual).
// Para no primeiro "--" e nunca altera o slice de entrada.
func normalizeManualArgs(args []string) []string {
	if len(args) == 0 {
		return args
	}
	isManual := func(a string) bool {
		return a == "-manual" || a == "--manual" || strings.HasPrefix(a, "-manual=") || strings.HasPrefix(a, "--manual=")
	}
	manual := []string{}
	rest := []string{}
	terminated := false
	for _, a := range args[1:] {
		switch {
		case terminated:
			rest = append(rest, a)
		case a == "--":
			terminated = true
			rest = append(rest, a)
		case isManual(a):
			manual = append(manual, a)
		default:
			rest = append(rest, a)
		}
	}
	out := make([]string, 0, len(args))
	out = append(out, args[0])
	out = append(out, manual...)
	return append(out, rest...)
}

// pauseForManualReview exibe o menu de revisão e lê a escolha do humano.
// Devolve a ação (manualAction*) e, em "redo", o feedback aparado. Em erro, action == "".
func pauseForManualReview(stageCode, stageLabel, artifactHint string, in io.Reader, out io.Writer) (action string, feedback string, err error) {
	if in == nil {
		return "", "", errManualNonInteractive
	}
	if out == nil {
		out = io.Discard
	}
	reader, ok := in.(*bufio.Reader)
	if !ok {
		reader = bufio.NewReader(in)
	}

	fmt.Fprintf(out, "\n[MANUAL] Etapa %s %s concluída.\n", stageCode, stageLabel)
	fmt.Fprintf(out, "[MANUAL] Revise: %s\n", artifactHint)
	fmt.Fprintln(out, "  [Enter] / [c] Continuar para a próxima etapa (recarrega os artefatos editados)")
	fmt.Fprintln(out, "  [r] Refazer esta etapa (com feedback opcional)")
	fmt.Fprintln(out, "  [s] Salvar checkpoint e sair (retome com: makercycle -resume)")
	fmt.Fprintln(out, "  [q] Abortar a execução")
	fmt.Fprint(out, "Escolha: ")

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && line == "" {
			return "", "", errManualNonInteractive
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		switch choice {
		case "", "c":
			if choice == "" && readErr != nil {
				return "", "", errManualNonInteractive
			}
			return manualActionContinue, "", nil
		case "r":
			fmt.Fprint(out, "Feedback para a nova tentativa (Enter para nenhum): ")
			fb, _ := reader.ReadString('\n')
			return manualActionRedo, strings.TrimSpace(fb), nil
		case "s":
			return manualActionSave, "", nil
		case "q":
			return manualActionAbort, "", nil
		}
		fmt.Fprintf(out, "[MANUAL] Opção inválida: %q. Use Enter, c, r, s ou q.\n", strings.TrimSpace(line))
		if readErr != nil {
			return "", "", errManualNonInteractive
		}
		fmt.Fprint(out, "Escolha: ")
	}
}

// manualStageLabel devolve o rótulo da etapa ("" se desconhecida).
func manualStageLabel(stageCode string) string {
	switch stageCode {
	case "E0":
		return "SCAN"
	case "E1":
		return "PLAN"
	case "E2":
		return "CHALLENGE"
	case "E3":
		return "MAKE"
	case "E4A":
		return "VERIFY: CHECK"
	case "E4B":
		return "VERIFY: HARDEN"
	}
	return ""
}

// manualArtifactHint indica o que o humano deve revisar após a etapa ("" se desconhecida).
func manualArtifactHint(stageCode, docsDir string) string {
	switch stageCode {
	case "E0":
		d := strings.Trim(docsDir, "/")
		if d == "" {
			d = "docs"
		}
		return d + "/PROJECT_CONTEXT.md"
	case "E1":
		return ".makercycle/state/PLAN.md"
	case "E2":
		return ".makercycle/state/SPEC_REVIEW.md"
	case "E3":
		return "git diff"
	case "E4A":
		return ".makercycle/state/VERIFY_REPORT.md"
	case "E4B":
		return ".makercycle/state/SECURITY_REPORT.md"
	}
	return ""
}

// appendManualFeedback anexa o feedback do revisor ao prompt do agente (vazio mantém o prompt).
func appendManualFeedback(prompt, feedback string) string {
	fb := strings.TrimSpace(feedback)
	if fb == "" {
		return prompt
	}
	return prompt + " " + manualFeedbackMarker + " " + fb
}

// manualResumeStage devolve a etapa em que o -resume deve retomar após um [s] na etapa informada.
func manualResumeStage(stageCode string, specApproved bool) (int, error) {
	switch stageCode {
	case "E1":
		return 2, nil
	case "E2":
		if specApproved {
			return 3, nil
		}
		return 1, nil
	case "E3", "E4A":
		return 4, nil
	case "E4B":
		return 5, nil
	}
	return 0, fmt.Errorf("a etapa %q não suporta checkpoint no modo -manual", stageCode)
}

// resolveManualMode aplica a precedência: flag explícita > valor salvo no checkpoint.
func resolveManualMode(saved, flagValue, flagSet bool) bool {
	if flagSet {
		return flagValue
	}
	return saved
}

// verifyReportPassed relê o VERIFY_REPORT.md (que o humano pode ter editado); erro de leitura devolve fallback.
func verifyReportPassed(path string, fallback bool) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return strings.Contains(string(data), "ALL_CHECKS_PASSED")
}

// manualReviewer encapsula o laço executar → pausar → refazer de cada etapa.
type manualReviewer struct {
	Enabled bool
	In      io.Reader
	Out     io.Writer
	DocsDir string
}

// Review executa run(feedback); com Enabled e run==true, pausa. "redo" reexecuta run com o novo feedback.
// Devolve continue/save/abort. Desligado: run("") uma vez e continue, sem ler In nem escrever em Out.
func (m manualReviewer) Review(stageCode string, run func(feedback string) bool) (string, error) {
	label := manualStageLabel(stageCode)
	if label == "" {
		return "", fmt.Errorf("etapa desconhecida no modo -manual: %q", stageCode)
	}
	if !m.Enabled {
		run("")
		return manualActionContinue, nil
	}
	hint := manualArtifactHint(stageCode, m.DocsDir)
	feedback := ""
	for {
		if !run(feedback) {
			return manualActionContinue, nil
		}
		action, fb, err := pauseForManualReview(stageCode, label, hint, m.In, m.Out)
		if err != nil {
			return "", err
		}
		if action != manualActionRedo {
			return action, nil
		}
		feedback = fb
	}
}

// scoutFeedbackRunner é o runner do SCOUT que aceita o feedback do revisor humano.
type scoutFeedbackRunner func(scout AgentBinding, docsDir, existingDocs, detectedName string, jsonMetrics bool, feedback string) (TokenStats, error)

// manualScoutRunner adapta a pausa da E0 ao scoutRunner do -setup, somando os tokens de todas as execuções.
func manualScoutRunner(m manualReviewer, base scoutFeedbackRunner) scoutRunner {
	return func(scout AgentBinding, docsDir, existingDocs, detectedName string, jsonMetrics bool) (TokenStats, error) {
		rev := m
		rev.DocsDir = docsDir
		var total TokenStats
		var lastErr error
		action, err := rev.Review("E0", func(feedback string) bool {
			stats, runErr := base(scout, docsDir, existingDocs, detectedName, jsonMetrics, feedback)
			total.Add(stats)
			lastErr = runErr
			return runErr == nil
		})
		if lastErr != nil {
			return total, lastErr
		}
		if err != nil {
			return total, err
		}
		if action == manualActionAbort {
			return total, errManualAborted
		}
		return total, nil
	}
}
