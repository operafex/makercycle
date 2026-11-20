package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	resumeEnvPath = ".makercycle/state/RESUME.env"
	exitQuota     = 5
)

// shouldRunStage determina se uma determinada etapa deve ser executada na rodada/tentativa atual.
func shouldRunStage(stage, startStage, attempt, firstAttempt int) bool {
	if stage <= 2 {
		return startStage <= stage
	}
	return attempt > firstAttempt || stage >= startStage
}

// ResumeState representa o estado salvo de uma execução interrompida.
type ResumeState struct {
	Stage   int
	Demand  string
	Stack   string
	Attempt int
}

// writeResumeState valida e salva o estado de retomada em RESUME.env.
// Nenhum arquivo é criado se houver erro de validação.
func writeResumeState(path string, st ResumeState) error {
	if st.Stage < 1 || st.Stage > 5 {
		return fmt.Errorf("etapa inválida: %d (deve ser entre 1 e 5)", st.Stage)
	}
	if st.Attempt < 1 {
		return fmt.Errorf("tentativa inválida: %d (deve ser >= 1)", st.Attempt)
	}
	if strings.ContainsAny(st.Demand, "\n\r\"") {
		return fmt.Errorf("valor de demanda contém caracteres inválidos")
	}
	if strings.ContainsAny(st.Stack, "\n\r\"") {
		return fmt.Errorf("valor de stack contém caracteres inválidos")
	}

	updates := map[string]string{
		"MC_RESUME_STAGE":   strconv.Itoa(st.Stage),
		"MC_RESUME_DEMAND":  st.Demand,
		"MC_RESUME_STACK":   st.Stack,
		"MC_RESUME_ATTEMPT": strconv.Itoa(st.Attempt),
	}
	return writeProjectEnv(path, updates)
}

// loadResumeState carrega o estado de retomada de um arquivo .env.
func loadResumeState(path string) (ResumeState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return ResumeState{}, fmt.Errorf("arquivo de retomada %s ausente (use ./makercycle -stage <N> -demand <arquivo>)", path)
		}
		return ResumeState{}, fmt.Errorf("lendo arquivo de retomada %s: %w", path, err)
	}

	env := parseEnvContent(string(data))
	rawStage, ok := env["MC_RESUME_STAGE"]
	if !ok || strings.TrimSpace(rawStage) == "" {
		return ResumeState{}, fmt.Errorf("MC_RESUME_STAGE ausente em %s", path)
	}
	stage, err := strconv.Atoi(strings.TrimSpace(rawStage))
	if err != nil || stage < 1 || stage > 5 {
		return ResumeState{}, fmt.Errorf("MC_RESUME_STAGE inválido em %s: %q", path, rawStage)
	}

	attempt := 1
	if rawAttempt, hasAttempt := env["MC_RESUME_ATTEMPT"]; hasAttempt && strings.TrimSpace(rawAttempt) != "" {
		att, err := strconv.Atoi(strings.TrimSpace(rawAttempt))
		if err != nil || att < 1 {
			return ResumeState{}, fmt.Errorf("MC_RESUME_ATTEMPT inválido em %s: %q", path, rawAttempt)
		}
		attempt = att
	}

	return ResumeState{
		Stage:   stage,
		Demand:  env["MC_RESUME_DEMAND"],
		Stack:   env["MC_RESUME_STACK"],
		Attempt: attempt,
	}, nil
}

// clearResumeState remove o arquivo de estado de retomada se existir.
func clearResumeState(path string) error {
	err := os.Remove(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}

// resumeStack resolve a configuração da stack para a retomada a partir do nome salvo.
func resumeStack(name, envPath, root string) (StackConfig, error) {
	if strings.TrimSpace(name) == "" {
		return StackConfig{}, errors.New("nome da stack vazio")
	}
	if proj, err := loadProjectStack(envPath); err == nil && proj.Name == name {
		return proj, nil
	}
	candidates := []StackConfig{
		goStack(root),
		railsStack(root),
		tsStack(root),
		nodeStack(root),
	}
	for _, c := range candidates {
		if c.Name == name {
			return c, nil
		}
	}
	return StackConfig{}, fmt.Errorf("stack %q não encontrada; selecione uma stack manualmente usando -stage N -demand <arquivo>", name)
}

// quotaInterruptionReport gera o relatório de interrupção por cota em português.
func quotaInterruptionReport(st ResumeState, stageLabel, eta string) string {
	var cmd string
	if st.Demand != "" {
		cmd = fmt.Sprintf("./makercycle -stage %d -demand %s", st.Stage, st.Demand)
	} else {
		cmd = fmt.Sprintf("./makercycle -stage %d", st.Stage)
	}
	return fmt.Sprintf("Etapa interrompida: %s\nTentativa atual: %d\nMotivo: bloqueio por limite de tokens/cota\nTempo para continuar: %s\n\nComandos prontos para continuar:\n  ./makercycle -resume\n  %s\n",
		stageLabel, st.Attempt, eta, cmd)
}

// validateRunFlags valida a combinação de flags passadas na linha de comando.
func validateRunFlags(task, demand string, resume, stageSet bool) error {
	if task != "" && demand != "" {
		return errors.New("as flags -task e -demand são mutuamente exclusivas")
	}
	if resume && (task != "" || demand != "" || stageSet) {
		return errors.New("a flag -resume não pode ser combinada com -task, -demand ou -stage")
	}
	return nil
}

// resolveDemandFlag valida e normaliza o caminho do arquivo de demanda.
// Garante que o arquivo exista, seja regular e resida diretamente em <docsDir>/demands/.
func resolveDemandFlag(path, docsDir string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("caminho de demanda vazio")
	}
	if docsDir == "" {
		docsDir = "docs"
	}
	demandsDir := filepath.Join(docsDir, "demands")

	if !strings.Contains(trimmed, "/") && !strings.Contains(trimmed, string(filepath.Separator)) {
		trimmed = filepath.Join(demandsDir, trimmed)
	}

	cleanPath := filepath.Clean(trimmed)
	rel, err := filepath.Rel(demandsDir, cleanPath)
	if err != nil {
		return "", fmt.Errorf("erro ao calcular caminho relativo para %s: %w", cleanPath, err)
	}
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || strings.Contains(rel, string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("caminho %q fora do diretório de demandas %s", path, demandsDir)
	}

	if filepath.Ext(cleanPath) != ".md" {
		return "", fmt.Errorf("demanda deve ter extensão .md: %s", cleanPath)
	}
	if filepath.Base(cleanPath) == "INDEX.md" {
		return "", fmt.Errorf("INDEX.md não é uma demanda: %s", cleanPath)
	}

	fi, err := os.Stat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("demanda %s não encontrada: %w", cleanPath, err)
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("%s não é um arquivo regular", cleanPath)
	}

	return filepath.ToSlash(cleanPath), nil
}

// demandTitle lê a primeira linha que inicia com '# Demanda:' no arquivo indicado.
func demandTitle(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("lendo demanda %s: %w", path, err)
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "# Demanda:") {
			title := strings.TrimSpace(strings.TrimPrefix(line, "# Demanda:"))
			return title, nil
		}
	}
	return "", fmt.Errorf("título da demanda ausente em %s (esperado '# Demanda: ...')", path)
}
