package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type initOptions struct {
	DocsDir     string
	Agents      []AgentBinding // scout, planner, critic, maker, auditor, gatekeeper
	Force       bool
	ForceSkills bool
	JSONMetrics bool
	SetFlags    map[string]bool
	Reader      *bufio.Reader
}

// buildScoutPrompt monta o prompt da Etapa 0 (SCOUT), compartilhado por -setup e -init.
func buildScoutPrompt(existingDocs, docsDir, detectedName string) string {
	sourceHint := existingDocs
	if sourceHint == "" {
		sourceHint = "Nenhuma documentação prévia informada (criar estrutura inicial em " + docsDir + "/)"
	}
	detectedHint := "(nenhuma)"
	if detectedName != "" {
		detectedHint = detectedName
	}
	return fmt.Sprintf(
		"Siga estritamente as instruções de @.makercycle/agents/00_SCOUT.md. "+
			"Documentação existente informada pelo usuário: '%s'. "+
			"Diretório oficial de saída para documentação: '%s/'. "+
			"Stack pré-detectada pelo CLI: '%s'. Valide ou corrija as chaves MC_STACK_NAME, MC_TEST_CMD, MC_TYPE_CMD e MC_LINT_CMD em .makercycle/config/project.env, preservando as demais chaves. "+
			"Analise a estrutura do código existente, leia a documentação informada e gere %s/PROJECT_CONTEXT.md e %s/demands/INDEX.md.",
		sourceHint, docsDir, detectedHint, docsDir, docsDir,
	)
}

// buildScoutPromptFor estende buildScoutPrompt com o banco de dados (MC_DATABASE) e, em projeto novo,
// com a instrução de gerar um PROJECT_CONTEXT.md inicial sem falhar por diretório vazio.
func buildScoutPromptFor(existingDocs, docsDir, detectedName, database string, greenfield bool) string {
	prompt := withDatabaseContext(buildScoutPrompt(existingDocs, docsDir, detectedName), database)
	if !greenfield {
		return prompt
	}
	stack, db := detectedName, strings.TrimSpace(database)
	if stack == "" {
		stack = "(a definir)"
	}
	if db == "" {
		db = "(a definir)"
	}
	return prompt + fmt.Sprintf(" Projeto novo (greenfield): ainda não há código-fonte. Não falhe por diretório vazio: "+
		"gere %s/PROJECT_CONTEXT.md inicial estruturado (visão geral, stack, banco de dados, comandos de build/teste/lint e convenções) "+
		"com a stack '%s' e o banco de dados '%s' definidos.", docsDir, stack, db)
}

// runScoutAgent executa o SCOUT e valida que <docs>/PROJECT_CONTEXT.md foi gerado.
func runScoutAgent(scout AgentBinding, docsDir, existingDocs, detectedName string, jsonMetrics bool) (TokenStats, error) {
	start := time.Now()
	prompt := buildScoutPromptFor(existingDocs, docsDir, detectedName, currentDatabase(projectEnvPath, os.Getenv), isGreenfieldProject("."))
	stats, err := runAgent(scout, prompt, jsonMetrics)
	if err != nil {
		return stats, fmt.Errorf("Falha na Etapa 0 (SCOUT): %w", err)
	}
	reportTokens(scout, stats)
	newStageTracker(os.Stdout).Finish("S0", time.Since(start), stats)

	contextPath := filepath.Join(docsDir, "PROJECT_CONTEXT.md")
	if info, statErr := os.Stat(contextPath); statErr != nil || info.IsDir() {
		return stats, fmt.Errorf("Etapa 0 terminou sem gerar %s. O SCOUT pode ter sido bloqueado por permissões (veja MC_SCOUT_ARGS em .makercycle/config/models.env) ou falhou ao gravar o arquivo.", contextPath)
	}
	if err := ensureDocsStructureSection(contextPath, docsDir); err != nil {
		fmt.Printf("[AVISO] Não foi possível documentar a estrutura de pastas em %s: %v\n", contextPath, err)
		logEvent("AVISO: falha ao documentar estrutura em %s: %v", contextPath, err)
	}
	return stats, nil
}

// initAuthError valida a autenticação como o fluxo principal (resolveAuthMode + validateAuthConfig).
func initAuthError() error {
	authMode, err := resolveAuthMode(os.Getenv("MC_AUTH_MODE"))
	if err != nil {
		return err
	}
	projectKeys, keysErr := readProjectAPIKeys(projectEnvPath)
	if keysErr != nil {
		fmt.Printf("[AVISO] %v\n", keysErr)
	}
	return validateAuthConfig(authMode, projectAwareGetenv(projectKeys, os.Getenv))
}

// installInitSkills instala as skills ausentes (ou todas, com force) e devolve 1 se alguma falhar.
func installInitSkills(skills []string, npxOK, force bool) int {
	code := 0
	roots, lock := currentSkillRoots()
	install := func(skill string) (out string, err error) {
		if !npxOK {
			return "", fmt.Errorf("npx ausente no PATH")
		}
		args := skillInstallArgs(skill)
		fmt.Printf("  -> npx %s\n", strings.Join(args, " "))
		logEvent("Executando: npx %s", strings.Join(args, " "))
		withSpinner("npx "+strings.Join(args, " "), func() { out, err = execCapture("npx", args) })
		fmt.Print(out)
		logBlock("npx "+strings.Join(args, " "), out)
		return out, err
	}
	for _, r := range ensureSkills(skills, roots, lock, force, install) {
		switch r.Action {
		case "presente":
			fmt.Printf("  [OK]     %s já instalada\n", r.Skill)
		case "instalada":
			fmt.Printf("  [OK]     %s instalada/atualizada\n", r.Skill)
		default:
			fmt.Printf("  [FALHA]  %s: %v\n", r.Skill, r.Err)
			code = 1
		}
		logEvent("Skill %s: %s (erro: %v)", r.Skill, r.Action, r.Err)
	}
	return code
}

// runInit inicializa o projeto: templates embutidos, models.env (herda da home), .gitignore,
// stack, skills e SCAN (SCOUT, quando há código).
func runInit(opts initOptions) {
	printBanner("MAKERCYCLE -init", "Templates, models.env, .gitignore, stack, skills e SCAN")
	code := 0

	home, _ := os.UserHomeDir()
	gp, err := globalModelsEnvPath(home)
	if err != nil {
		gp = ""
		fmt.Printf("[AVISO] Herança global de models.env desativada: %v\n", err)
	}

	sp, err := globalSecretsEnvPath(home)
	if err != nil {
		sp = ""
	}

	results, err := provisionInitAssets(initPaths{Root: ".", DocsDir: opts.DocsDir, GlobalModelsEnv: gp, GlobalSecretsEnv: sp},
		embeddedAgentsFS, embeddedModelsEnv, opts.Force)
	if err != nil {
		fatal("Erro ao provisionar o projeto: %v", err)
	}

	startRunLog(opts.DocsDir, "init")
	logBlock("CABEÇALHO DA EXECUÇÃO (-init)", fmt.Sprintf("Data/Hora: %s\nDocs: %s/\nForce: %v\nForce skills: %v\n",
		time.Now().Format(time.RFC3339), opts.DocsDir, opts.Force, opts.ForceSkills))
	for _, r := range results {
		fmt.Printf("  [OK] %s (%s)\n", r.Path, r.Action)
		logEvent("[OK] %s (%s)", r.Path, r.Action)
	}
	if b, readErr := os.ReadFile(modelsEnvPath); readErr == nil {
		if msg := modelsEnvKeyWarning(apiKeysInModelsEnv(string(b))); msg != "" {
			fmt.Fprintln(yellowWriter(os.Stdout), msg)
		}
	}

	// project.env, models.env e secrets.env provisionados passam a valer para esta execução (flags explícitas prevalecem).
	if err := loadEnvLayers(envLayerPaths); err != nil {
		fmt.Printf("[AVISO] %v\n", err)
	}
	agents := make([]AgentBinding, len(opts.Agents))
	for i, a := range opts.Agents {
		agents[i] = refreshBindingFromEnv(a, os.Getenv, opts.SetFlags)
	}

	skills := initSkillsToInstall(os.Environ())
	fmt.Println("\nBinários:")
	npxOK := false
	for _, b := range checkBinaries(initRequiredBinaries(agents, skills), exec.LookPath) {
		if b.Found {
			fmt.Printf("  [OK]     %s (%s)\n", b.Name, b.Path)
			logEvent("[OK] binário %s (%s)", b.Name, b.Path)
			if b.Name == "npx" {
				npxOK = true
			}
		} else {
			fmt.Printf("  [AUSENTE] %s\n", b.Name)
			logEvent("[AUSENTE] binário %s", b.Name)
			code = 1
		}
	}

	if len(skills) == 0 {
		msg := "[INFO] Nenhuma skill declarada (MC_*_SKILL) em models.env — instalação de skills ignorada."
		fmt.Println("\n" + msg)
		logEvent("%s", msg)
	} else {
		fmt.Printf("\nSkills configuradas (%d): %s\n", len(skills), strings.Join(skills, ", "))
		logEvent("Skills configuradas: %s", strings.Join(skills, ", "))
		if installInitSkills(skills, npxOK, opts.ForceSkills) != 0 {
			code = 1
		}
	}

	stackName := ""
	detected, ok, stackErr := confirmSetupStack(opts.Reader, os.Stdout, projectEnvPath, ".", opts.DocsDir, "")
	if stackErr != nil {
		fmt.Printf("[FALHA] Stack: %v\n", stackErr)
		logEvent("FALHA stack: %v", stackErr)
		code = 1
	} else if ok {
		stackName = detected.Name
		logEvent("STACK CONFIRMADA: %s (testes: %s)", detected.Name, detected.TestCmd)
	}

	if !hasCodeFiles(".") {
		msg := "[INFO] Nenhum arquivo de código encontrado — SCAN (SCOUT) não executado."
		fmt.Println("\n" + msg)
		logEvent("%s", msg)
	} else if authErr := initAuthError(); authErr != nil {
		fmt.Printf("\n[AVISO] SCAN (SCOUT) não executado: %v. Configure as chaves e rode -setup.\n", authErr)
		logEvent("AVISO: SCAN não executado: %v", authErr)
		code = 1
	} else {
		scout := agents[0]
		printBanner("ETAPA 0: SCAN — SCOUT", fmt.Sprintf("Agente: %s (%s / %s)", scout.Alias, scout.Bin, scout.Model))
		if _, err := runScoutAgent(scout, opts.DocsDir, "", stackName, opts.JSONMetrics); err != nil {
			fmt.Printf("[FALHA] %v\n", err)
			logEvent("FALHA SCAN: %v", err)
			code = 1
		}
	}

	if code == 0 {
		printBanner("-init CONCLUÍDO", "Projeto pronto. Use makercycle -task \"...\" para iniciar uma demanda.")
	} else {
		printBanner("-init CONCLUÍDO COM PENDÊNCIAS", "Há binários ausentes, skills com falha ou SCAN pendente. Corrija e rode makercycle -init novamente.")
	}
	finish(code)
}
