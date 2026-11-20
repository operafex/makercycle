package main

import (
  "bufio"
  "bytes"
  "encoding/csv"
  "encoding/json"
  "flag"
  "fmt"
  "io"
  "os"
  "os/exec"
  "path/filepath"
  "regexp"
  "sort"
  "strconv"
  "strings"
  "sync"
  "time"
  "unicode"
)

type StackConfig struct {
  Name    string
  TestCmd string
  TypeCmd string
  LintCmd string
}

type AgentBinding struct {
  Label string // rótulo da Etapa (ex.: "ETAPA 1"); nome do campo mantido por compatibilidade
  Alias string
  Bin   string
  Model string
  Skill string
  Args  string
  Stage string // nome da etapa (ex.: PLAN), usado no spinner e nos logs
}

type TokenStats struct {
  Input     int
  Output    int
  Total     int
  Estimated bool
}

func (t *TokenStats) Add(other TokenStats) {
  t.Input += other.Input
  t.Output += other.Output
  t.Total += other.Total
  if other.Estimated {
    t.Estimated = true
  }
}

func (t TokenStats) Format() string {
  prefix := ""
  if t.Estimated {
    prefix = "~"
  }
  return fmt.Sprintf("%s%d in / %s%d out (%s%d total)", prefix, t.Input, prefix, t.Output, prefix, t.Total)
}

func main() {
  runID := newRunID()

  loadEnvFile(".makercycle/config/models.env")
  loadEnvFile(".makercycle/config/project.env")
  if b, err := os.ReadFile(".makercycle/config/models.env"); err == nil {
    if msg := modelsEnvKeyWarning(apiKeysInModelsEnv(string(b))); msg != "" {
      fmt.Fprintln(yellowWriter(os.Stdout), msg)
    }
  }

  setupMode := flag.Bool("setup", false, "Etapa 0 (Scan): analisa projeto existente e mapeia documentação")
  taskFlag := flag.String("task", "", "Descrição da nova demanda (salva automaticamente em /docs/demands/)")
  demandFlag := flag.String("demand", "", "Arquivo de demanda existente em <docsDir>/demands/ (ex.: 2026-11-17-x.md)")
  resumeFlag := flag.Bool("resume", false, "Retoma a execução interrompida a partir de .makercycle/state/RESUME.env")
  initMode := flag.Bool("init", false, "Verifica binários, instala skills ausentes (npx skills add) e cria a estrutura de pastas")
  forceSkills := flag.Bool("force-skills", false, "Com -init: reinstala/atualiza todas as skills configuradas, mesmo as já presentes")
  selectStackFlag := flag.Bool("select-stack", false, "Abre o menu de seleção de stack mesmo com .makercycle/config/project.env válido e salva a escolha")
  stageFlag := flag.Int("stage", 1, "Etapa inicial do MakerCycle: 1 a 5 (1: Plan, 2: Challenge, 3: Make, 4: Verify, 5: Gate)")
  maxAttempts := flag.Int("max-attempts", getEnvInt("MC_MAX_ATTEMPTS", 3), "Limite máximo de tentativas no loop (Circuit Breaker)")
  docsDirFlag := flag.String("docs-dir", getEnv("MC_DOCS_DIR", getEnv("MC_DEFAULT_DOCS_DIR", "docs")), "Diretório de documentação")
  var refFlags stringList
  flag.Var(&refFlags, "ref", "Arquivo de referência para enriquecer a demanda (repetível; caminho direto ou nome dentro de <DOCS_DIR>/references/)")
  jsonMetrics := flag.Bool("json-metrics", true, "Extrai contagem exata de tokens via --output-format json quando suportado")

  // Bindings desacoplados: troque via .makercycle/config/models.env ou flags CLI
  onbBin := flag.String("scout-bin", getEnv("MC_SCOUT_BIN", "claude"), "Binário da Etapa 0 (SCOUT)")
  onbModel := flag.String("scout-model", getEnv("MC_SCOUT_MODEL", "opus"), "Modelo da Etapa 0 (SCOUT)")
  onbArgs := flag.String("scout-args", getEnv("MC_SCOUT_ARGS", ""), "Argumentos extras do CLI da Etapa 0 (SCOUT)")

  archBin := flag.String("planner-bin", getEnv("MC_PLANNER_BIN", "claude"), "Binário da Etapa 1 (PLANNER)")
  archModel := flag.String("planner-model", getEnv("MC_PLANNER_MODEL", "opus"), "Modelo da Etapa 1 (PLANNER)")
  archSkill := flag.String("planner-skill", getEnv("MC_PLANNER_SKILL", "obra/superpowers"), "Skill da Etapa 1 (PLANNER)")
  archArgs := flag.String("planner-args", getEnv("MC_PLANNER_ARGS", ""), "Argumentos extras do CLI da Etapa 1 (PLANNER)")

  skepBin := flag.String("critic-bin", getEnv("MC_CRITIC_BIN", "agy"), "Binário da Etapa 2 (CRITIC)")
  skepModel := flag.String("critic-model", getEnv("MC_CRITIC_MODEL", "GPT-OSS 120B (Medium)"), "Modelo da Etapa 2 (CRITIC)")
  skepSkill := flag.String("critic-skill", getEnv("MC_CRITIC_SKILL", "google/elephant-goldfish-model"), "Skill da Etapa 2 (CRITIC)")
  skepArgs := flag.String("critic-args", getEnv("MC_CRITIC_ARGS", "--dangerously-skip-permissions"), "Argumentos extras do CLI da Etapa 2 (CRITIC)")

  buildBin := flag.String("maker-bin", getEnv("MC_MAKER_BIN", "agy"), "Binário da Etapa 3 (MAKER)")
  buildModel := flag.String("maker-model", getEnv("MC_MAKER_MODEL", "Gemini 3.8 Flash (High)"), "Modelo da Etapa 3 (MAKER)")
  buildArgs := flag.String("maker-args", getEnv("MC_MAKER_ARGS", "--dangerously-skip-permissions"), "Argumentos extras do CLI da Etapa 3 (MAKER)")

  sentBin := flag.String("auditor-bin", getEnv("MC_AUDITOR_BIN", "agy"), "Binário da Etapa 4B (AUDITOR)")
  sentModel := flag.String("auditor-model", getEnv("MC_AUDITOR_MODEL", "Claude Sonnet 4.6 (Thinking)"), "Modelo da Etapa 4B (AUDITOR)")
  sentSkill := flag.String("auditor-skill", getEnv("MC_AUDITOR_SKILL", "cloudflare/security-audit-skill"), "Skill da Etapa 4B (AUDITOR)")
  sentArgs := flag.String("auditor-args", getEnv("MC_AUDITOR_ARGS", "--dangerously-skip-permissions"), "Argumentos extras do CLI da Etapa 4B (AUDITOR)")

  arbBin := flag.String("gatekeeper-bin", getEnv("MC_GATEKEEPER_BIN", "agy"), "Binário da Etapa 5 (GATEKEEPER)")
  arbModel := flag.String("gatekeeper-model", getEnv("MC_GATEKEEPER_MODEL", "Claude Sonnet 4.6 (Thinking)"), "Modelo da Etapa 5 (GATEKEEPER)")
  arbArgs := flag.String("gatekeeper-args", getEnv("MC_GATEKEEPER_ARGS", "--dangerously-skip-permissions"), "Argumentos extras do CLI da Etapa 5 (GATEKEEPER)")

  flag.Parse()

  setFlags := map[string]bool{}
  flag.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

  if err := validateRunFlags(*taskFlag, *demandFlag, *resumeFlag, setFlags["stage"]); err != nil {
    fatal("%v", err)
  }

  if err := ensureMakerCycleDirs(); err != nil {
    fatal("Erro ao criar estrutura .makercycle/: %v", err)
  }

  scout := AgentBinding{Label: "ETAPA 0", Stage: "SCAN", Alias: "SCOUT", Bin: *onbBin, Model: *onbModel, Args: *onbArgs}
  planner := AgentBinding{Label: "ETAPA 1", Stage: "PLAN", Alias: "PLANNER", Bin: *archBin, Model: *archModel, Skill: *archSkill, Args: *archArgs}
  critic := AgentBinding{Label: "ETAPA 2", Stage: "CHALLENGE", Alias: "CRITIC", Bin: *skepBin, Model: *skepModel, Skill: *skepSkill, Args: *skepArgs}
  maker := AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Bin: *buildBin, Model: *buildModel, Args: *buildArgs}
  auditor := AgentBinding{Label: "ETAPA 4B", Stage: "VERIFY: HARDEN", Alias: "AUDITOR", Bin: *sentBin, Model: *sentModel, Skill: *sentSkill, Args: *sentArgs}
  gatekeeper := AgentBinding{Label: "ETAPA 5", Stage: "GATE", Alias: "GATEKEEPER", Bin: *arbBin, Model: *arbModel, Args: *arbArgs}

  reader := bufio.NewReader(os.Stdin)

  // =========================================================================
  // -init: verificação de binários, instalação de skills e estrutura de pastas
  // =========================================================================
  if *initMode {
    initDocs := strings.Trim(strings.TrimSpace(*docsDirFlag), "/")
    if initDocs == "" {
      initDocs = "docs"
    }
    runInit(initDocs, []AgentBinding{scout, planner, critic, maker, auditor, gatekeeper}, *forceSkills)
    return
  }

  authMode, err := resolveAuthMode(os.Getenv("MC_AUTH_MODE"))
  if err != nil {
    fatal("%v", err)
  }
  projectKeys, keysErr := readProjectAPIKeys(projectEnvPath)
  if keysErr != nil {
    fmt.Printf("[AVISO] %v\n", keysErr)
  }
  if err := validateAuthConfig(authMode, projectAwareGetenv(projectKeys, os.Getenv)); err != nil {
    fatal("%v", err)
  }
  omni := loadOmniRouteConfig(os.Getenv)

  // =========================================================================
  // ETAPA 0 — SCAN (-setup): ANÁLISE DE PROJETO EXISTENTE
  // =========================================================================
  if *setupMode {
    runProjectSetup(reader, scout, *docsDirFlag, *jsonMetrics)
    finish(0)
    return
  }

  docsDir := strings.Trim(strings.TrimSpace(*docsDirFlag), "/")
  if docsDir == "" {
    docsDir = "docs"
  }

  var (
    startStage   int
    firstAttempt = 1
    stack        StackConfig
    demandFile   string
    task         = strings.TrimSpace(*taskFlag)
  )

  if *resumeFlag {
    st, err := loadResumeState(resumeEnvPath)
    if err != nil {
      fatal("%v", err)
    }
    startStage = st.Stage
    firstAttempt = st.Attempt
    if strings.TrimSpace(st.Demand) != "" {
      df, err := resolveDemandFlag(st.Demand, docsDir)
      if err != nil {
        fatal("%v", err)
      }
      demandFile = df
    }
    resStack, err := resumeStack(st.Stack, projectEnvPath, ".")
    if err != nil {
      fatal("%v", err)
    }
    stack = resStack
  } else {
    var stageErr error
    startStage, stageErr = resolveStage(*stageFlag)
    if stageErr != nil {
      fatal("%v", stageErr)
    }

    if *demandFlag != "" {
      df, err := resolveDemandFlag(*demandFlag, docsDir)
      if err != nil {
        fatal("%v", err)
      }
      demandFile = df
    }

    s, err := resolveRunStack(reader, os.Stdout, projectEnvPath, *selectStackFlag)
    if err != nil {
      fatal("%v", err)
    }
    stack = s
  }

  if demandFile != "" {
    t, err := demandTitle(demandFile)
    if err == nil && (startStage == 1 || task == "") {
      task = t
    }
  }

  refs, refErr := resolveReferences(refFlags, docsDir)
  if refErr != nil {
    fatal("%v", refErr)
  }
  if len(refs) > 0 && startStage > 1 {
    fmt.Println("[AVISO] -ref só é usado pelo PLANNER (Etapa 1); ignorado ao iniciar a partir da Etapa", startStage)
    refs = nil
  }

  if startStage == 1 && task == "" && demandFile == "" {
    fmt.Print("\nDescreva a nova demanda ou feature: ")
    input, _ := reader.ReadString('\n')
    task = strings.TrimSpace(input)
    if task == "" {
      fatal("A descrição da demanda não pode estar vazia na Etapa 1.")
    }
  }

  modelsSummary := fmt.Sprintf("F1:%s | F2:%s | F3:%s | F4B:%s | F5:%s",
    planner.Model, critic.Model, maker.Model, auditor.Model, gatekeeper.Model)

  // Log completo da execução em <DOCS_DIR>/logs/ (um arquivo por execução).
  logSuffix := fmt.Sprintf("stage%d", startStage)
  if task != "" {
    logSuffix = slugify(task)
  }
  startRunLog(docsDir, logSuffix)
  logName := ""
  if runLog != nil {
    logName = runLog.Name
  }

  if demandFile == "" && task != "" {
    demandFile = registerDemandInDocs(docsDir, stack.Name, task, modelsSummary, logName, refs)
    fmt.Printf("\n[DOCS] Demanda registrada em: %s\n", demandFile)
  }

  logBlock("CABEÇALHO DA EXECUÇÃO", buildRunHeader(stack, []AgentBinding{planner, critic, maker, auditor, gatekeeper},
    task, demandFile, startStage, *maxAttempts, docsDir))
  if len(refs) > 0 {
    fmt.Printf("[DOCS] Referências anexadas à demanda: %s\n", strings.Join(refs, ", "))
    logEvent("Referências anexadas à demanda: %s", strings.Join(refs, ", "))
  }

  var totalTokens TokenStats

  preflightSkills(stageSkills(startStage, planner, critic, auditor))

  printBanner("MAKERCYCLE INICIADO",
    fmt.Sprintf("Stack: %s | Docs: /%s | Max Tentativas: %d\nBindings: %s", stack.Name, docsDir, *maxAttempts, modelsSummary))

  handleQuotaPause := func(stage int, stageLabel string, res AgentResult, attempt int) {
    st := ResumeState{
      Stage:   stage,
      Demand:  demandFile,
      Stack:   stack.Name,
      Attempt: attempt,
    }
    if err := writeResumeState(resumeEnvPath, st); err != nil {
      fatal("Erro ao salvar estado de retomada: %v", err)
    }
    report := quotaInterruptionReport(st, stageLabel, formatResetETA(time.Now(), res.ResetIn, res.HasReset))
    fmt.Print(report)
    logBlock("INTERRUPÇÃO POR COTA/TOKENS", report)
    if demandFile != "" {
      updateDemandStatus(demandFile, "QUOTA_PAUSED", attempt, totalTokens)
    }
    recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, attempt, totalTokens, "QUOTA_PAUSED", "Limite de cota/tokens atingido")
    finish(exitQuota)
  }

  // =========================================================================
  // ETAPAS 1 e 2 — PLAN (PLANNER) -> CHALLENGE (CRITIC): execução linear, uma passada
  // =========================================================================
  if startStage <= 2 {
    if startStage <= 1 {
      printBanner("ETAPA 1: PLAN — PLANNER",
        fmt.Sprintf("Skill: %s | Modelo: %s (%s)", planner.Skill, planner.Model, planner.Bin))
      planPrompt := fmt.Sprintf(
        "Siga estritamente as instruções de @.makercycle/agents/01_PLANNER.md aplicando a skill %s. "+
          "Diretório de documentação: %s/. Stack: %s. Demanda: %s. Arquivo da demanda: %s. "+
          "Grave .makercycle/state/PLAN.md, .makercycle/state/TEST_CASES.md e os testes em estado RED.",
        planner.Skill, docsDir, stack.Name, task, demandFile,
      ) + referencesPrompt(refs)
      start := time.Now()
      res := runAgentStep(planner, planPrompt, *jsonMetrics, authMode, omni)
      recordEventTelemetry(runID, "S1", planner.Alias, res.Model, 1, res.Stats, time.Since(start).Seconds())
      totalTokens.Add(res.Stats)
      if res.Quota {
        handleQuotaPause(1, planner.Label+": "+planner.Stage, res, 1)
      }
      if res.Err != nil {
        fatal("Falha na Etapa 1 (PLANNER): %v", res.Err)
      }
      reportTokens(planner, res.Stats)
    }

    printBanner("ETAPA 2: CHALLENGE — CRITIC",
      fmt.Sprintf("Skill: %s | Modelo: %s (%s)", critic.Skill, critic.Model, critic.Bin))
    critPrompt := fmt.Sprintf(
      "Siga estritamente as instruções de @.makercycle/agents/02_CRITIC.md aplicando a skill %s. "+
        "Diretório de documentação: %s/. Audite @.makercycle/state/PLAN.md, @.makercycle/state/TEST_CASES.md e os arquivos de teste recém-criados. "+
        "Grave seu veredito em .makercycle/state/SPEC_REVIEW.md.",
      critic.Skill, docsDir,
    )
    _ = os.Remove(".makercycle/state/SPEC_REVIEW.md") // evita ler um parecer de execução anterior
    start := time.Now()
    res := runAgentStep(critic, critPrompt, *jsonMetrics, authMode, omni)
    recordEventTelemetry(runID, "S2", critic.Alias, res.Model, 1, res.Stats, time.Since(start).Seconds())
    totalTokens.Add(res.Stats)
    if res.Quota {
      handleQuotaPause(2, critic.Label+": "+critic.Stage, res, 1)
    }
    if res.Err != nil {
      fatal("Falha na Etapa 2 (CRITIC): %v", res.Err)
    }
    reportTokens(critic, res.Stats)

    review, err := readFile(".makercycle/state/SPEC_REVIEW.md")
    if err != nil {
      fatal("O CRITIC não gerou .makercycle/state/SPEC_REVIEW.md: %v", err)
    }
    logBlock(".makercycle/state/SPEC_REVIEW.md", review)
    if !strings.Contains(review, "SPEC_STATUS: APPROVED") {
      updateDemandStatus(demandFile, "SPEC_REVISION_REQUIRED", 0, totalTokens)
      recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, 0, totalTokens, "SPEC_REVISION_REQUIRED", "Especificação reprovada pelo CRITIC")
      printBanner("BLOQUEIO NA ETAPA 2 (CHALLENGE)", "O CRITIC exigiu ajustes na especificação. Consulte .makercycle/state/SPEC_REVIEW.md e rode novamente a partir da Etapa 1.")
      finish(2)
    }
    fmt.Println("\n[OK] Especificação aprovada (SPEC_STATUS: APPROVED). Entrando no ciclo Make -> Verify -> Gate...")
  }

  // =========================================================================
  // LOOP FECHADO: ETAPA 3 (MAKE) -> ETAPA 4 (VERIFY) -> ETAPA 5 (GATE)
  // =========================================================================
  var checksPassed bool
  for attempt := firstAttempt; attempt <= *maxAttempts; attempt++ {
    // ETAPA 3 — MAKE: MAKER (Contexto Limpo)
    if shouldRunStage(3, startStage, attempt, firstAttempt) {
      printBanner(fmt.Sprintf("ETAPA 3: MAKE — MAKER (Tentativa %d/%d)", attempt, *maxAttempts),
        fmt.Sprintf("Contexto Higienizado | Modelo: %s (%s)", maker.Model, maker.Bin))

      execPrompt := fmt.Sprintf(
        "Siga estritamente as instruções de @.makercycle/agents/03_MAKER.md. Diretório de docs: %s/. "+
          "Leia @.makercycle/state/PLAN.md e @.makercycle/state/TEST_CASES.md e implemente as Tasks no código de produção.",
        docsDir,
      )
      if attempt > 1 {
        execPrompt += " ATENÇÃO: Rodada de correção. Leia @.makercycle/state/DECISION.md, @.makercycle/state/VERIFY_REPORT.md e @.makercycle/state/SECURITY_REPORT.md e corrija todos os apontamentos."
      }

      bStart := time.Now()
      res := runAgentStep(maker, execPrompt, *jsonMetrics, authMode, omni)
      recordEventTelemetry(runID, "S3", maker.Alias, res.Model, attempt, res.Stats, time.Since(bStart).Seconds())
      totalTokens.Add(res.Stats)
      if res.Quota {
        handleQuotaPause(3, maker.Label+": "+maker.Stage, res, attempt)
      }
      if res.Err != nil {
        fatal("Falha na Etapa 3 (MAKER): %v (verifique MC_MAKER_MODEL/MC_MAKER_BIN em .makercycle/config/models.env)", res.Err)
      }
      reportTokens(maker, res.Stats)
    }

    // ETAPA 4A & 4B — VERIFY
    if shouldRunStage(4, startStage, attempt, firstAttempt) {
      // ETAPA 4A — VERIFY: CHECK: Verificação Determinística
      printBanner(fmt.Sprintf("ETAPA 4A: VERIFY: CHECK (Tentativa %d/%d)", attempt, *maxAttempts),
        fmt.Sprintf("Runner Determinístico (%s)", stack.Name))
      checksPassed = runDeterministicChecks(stack)

      // ETAPA 4B — VERIFY: HARDEN: AUDITOR
      printBanner(fmt.Sprintf("ETAPA 4B: VERIFY: HARDEN — AUDITOR (Tentativa %d/%d)", attempt, *maxAttempts),
        fmt.Sprintf("Skill: %s | Modelo: %s", auditor.Skill, auditor.Model))

      secPrompt := fmt.Sprintf(
        "Siga estritamente as instruções de @.makercycle/agents/04_AUDITOR.md aplicando a skill %s. "+
          "Audite o git diff atual e grave o relatório em .makercycle/state/SECURITY_REPORT.md.",
        auditor.Skill,
      )
      secReportPath := ".makercycle/state/SECURITY_REPORT.md"
      var secModBefore time.Time
      if fi, statErr := os.Stat(secReportPath); statErr == nil {
        secModBefore = fi.ModTime()
      }
      sStart := time.Now()
      res := runAgentStep(auditor, secPrompt, *jsonMetrics, authMode, omni)
      recordEventTelemetry(runID, "S4B", auditor.Alias, res.Model, attempt, res.Stats, time.Since(sStart).Seconds())
      totalTokens.Add(res.Stats)
      if res.Quota {
        handleQuotaPause(4, auditor.Label+": "+auditor.Stage, res, attempt)
      }
      if res.Err != nil {
        fmt.Printf("[AVISO] AUDITOR (AppSec) retornou erro de processo: %v\n", res.Err)
        logEvent("AVISO: AUDITOR retornou erro de processo: %v", res.Err)
      }
      if secContent, readErr := readFile(secReportPath); readErr == nil {
        logBlock(".makercycle/state/SECURITY_REPORT.md (tentativa "+strconv.Itoa(attempt)+")", secContent)
      }
      if fi, statErr := os.Stat(secReportPath); statErr != nil {
        fmt.Printf("[AVISO] AUDITOR não gerou %s. A auditoria de segurança NÃO foi registrada (verifique permissões em MC_AUDITOR_ARGS).\n", secReportPath)
      } else if !secModBefore.IsZero() && !fi.ModTime().After(secModBefore) {
        fmt.Printf("[AVISO] AUDITOR não atualizou %s nesta tentativa; o relatório é de uma execução anterior.\n", secReportPath)
      }
      reportTokens(auditor, res.Stats)
    }

    // ETAPA 5 — GATE: GATEKEEPER
    if shouldRunStage(5, startStage, attempt, firstAttempt) {
      if startStage == 5 && attempt == firstAttempt {
        if content, err := readFile(".makercycle/state/VERIFY_REPORT.md"); err == nil {
          checksPassed = strings.Contains(content, "ALL_CHECKS_PASSED")
        }
      }

      printBanner(fmt.Sprintf("ETAPA 5: GATE — GATEKEEPER (Tentativa %d/%d)", attempt, *maxAttempts),
        fmt.Sprintf("Modelo: %s (%s)", gatekeeper.Model, gatekeeper.Bin))

      arbPrompt := fmt.Sprintf(
        "Siga estritamente as instruções de @.makercycle/agents/05_GATEKEEPER.md. "+
          "Esta é a tentativa ATTEMPT: %d de %d. "+
          "Analise @.makercycle/state/PLAN.md, @.makercycle/state/TEST_CASES.md, @.makercycle/state/VERIFY_REPORT.md, @.makercycle/state/SECURITY_REPORT.md e execute git diff. "+
          "Grave seu veredito em .makercycle/state/DECISION.md.",
        attempt, *maxAttempts,
      )

      aStart := time.Now()
      res := runAgentStep(gatekeeper, arbPrompt, *jsonMetrics, authMode, omni)
      recordEventTelemetry(runID, "S5", gatekeeper.Alias, res.Model, attempt, res.Stats, time.Since(aStart).Seconds())
      totalTokens.Add(res.Stats)
      if res.Quota {
        handleQuotaPause(5, gatekeeper.Label+": "+gatekeeper.Stage, res, attempt)
      }
      if res.Err != nil {
        fatal("Falha na Etapa 5 (GATEKEEPER): %v", res.Err)
      }
      reportTokens(gatekeeper, res.Stats)
      fmt.Printf("[TOKENS ACUMULADOS NO MAKERCYCLE] %s\n", totalTokens.Format())
      logEvent("TOKENS ACUMULADOS NO MAKERCYCLE: %s", totalTokens.Format())

      decisionContent, err := readFile(".makercycle/state/DECISION.md")
      if err != nil {
        fatal("Não foi possível ler .makercycle/state/DECISION.md: %v", err)
      }
      logBlock(".makercycle/state/DECISION.md (tentativa "+strconv.Itoa(attempt)+")", decisionContent)

      switch {
      case strings.Contains(decisionContent, "DECISION: APPROVE_MERGE"):
        clearResumeState(resumeEnvPath)
        updateDemandStatus(demandFile, "APPROVED", attempt, totalTokens)
        recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, attempt, totalTokens, "APPROVED", "Testes, AppSec e Diff-Judge aprovados")
        printBanner("MAKERCYCLE CONCLUÍDO — APPROVE_MERGE",
          fmt.Sprintf("Aprovado em %d tentativa(s) | Gasto Total: %s", attempt, totalTokens.Format()))
        finish(0)

      case strings.Contains(decisionContent, "DECISION: AMEND_TEST"):
        updateDemandStatus(demandFile, "AMEND_TEST", attempt, totalTokens)
        recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, attempt, totalTokens, "AMEND_TEST", "Erro detectado no teste da Etapa 1")
        printBanner("INTERVENÇÃO NECESSÁRIA — AMEND_TEST", "O GATEKEEPER identificou bug em um teste da Etapa 1. Revise .makercycle/state/DECISION.md.")
        finish(3)

      case strings.Contains(decisionContent, "DECISION: ESCALATE_PLANNER"):
        updateDemandStatus(demandFile, "ESCALATE_PLANNER", attempt, totalTokens)
        recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, attempt, totalTokens, "ESCALATE_PLANNER", "Escalonado para revisão arquitetural")
        printBanner("CIRCUIT BREAKER — ESCALATE_PLANNER", "Falha estrutural ou limite atingido. Consulte .makercycle/state/DECISION.md.")
        finish(4)

      case strings.Contains(decisionContent, "DECISION: RETRY_EXECUTION"):
        if !checksPassed {
          fmt.Println("\n[LOOP] Testes falharam na Etapa 4A. Reiniciando Etapa 3 (Make) com contexto limpo...")
        } else {
          fmt.Println("\n[LOOP] Apontamento de segurança ou qualidade semântica. Reiniciando Etapa 3 (Make)...")
        }
        continue

      default:
        updateDemandStatus(demandFile, "UNKNOWN_VERDICT", attempt, totalTokens)
        recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, attempt, totalTokens, "UNKNOWN_VERDICT", "Veredito não reconhecido")
        fatal("Veredito não reconhecido em .makercycle/state/DECISION.md.")
      }
    }
  }

  updateDemandStatus(demandFile, "CIRCUIT_BREAKER_TRIPPED", *maxAttempts, totalTokens)
  recordRunTelemetry(runID, stack.Name, demandFile, *maxAttempts, *maxAttempts, totalTokens, "CIRCUIT_BREAKER_TRIPPED", "Limite máximo de tentativas excedido")
  printBanner("CIRCUIT BREAKER ACIONADO",
    fmt.Sprintf("Limite de %d tentativas atingido | Gasto Total: %s", *maxAttempts, totalTokens.Format()))
  finish(1)
}

func runProjectSetup(reader *bufio.Reader, scout AgentBinding, defaultDocs string, jsonMetrics bool) {
  printBanner("ETAPA 0: SCAN — SETUP DE PROJETO EXISTENTE",
    fmt.Sprintf("Agente: %s (%s / %s)", scout.Alias, scout.Bin, scout.Model))

  fmt.Print("\nOnde está localizada a documentação atual do projeto? (ex: docs, wiki, README.md ou Enter para criar /docs): ")
  existingDocs, _ := reader.ReadString('\n')
  existingDocs = strings.TrimSpace(existingDocs)

  targetDocsDir := defaultDocs
  if existingDocs != "" && !strings.HasSuffix(existingDocs, ".md") {
    targetDocsDir = strings.Trim(existingDocs, "/")
  }
  if targetDocsDir == "" {
    targetDocsDir = "docs"
  }

  if err := ensureMakerCycleDirs(); err != nil {
    fatal("Erro ao criar estrutura .makercycle/: %v", err)
  }
  if err := ensureDocsDirs(targetDocsDir); err != nil {
    fatal("Erro ao criar estrutura de pastas em %s: %v", targetDocsDir, err)
  }
  startRunLog(targetDocsDir, "stage0")
  logBlock("CABEÇALHO DA EXECUÇÃO", buildRunHeader(StackConfig{Name: "(n/a — Etapa 0 Scan)"}, []AgentBinding{scout},
    "", "", 0, 0, targetDocsDir))

  detected, ok, envErr := confirmSetupStack(reader, os.Stdout, projectEnvPath, ".", targetDocsDir, existingDocs)
  if envErr != nil {
    fatal("Erro ao gravar %s: %v", projectEnvPath, envErr)
  }
  if ok {
    logEvent("STACK CONFIRMADA: %s (testes: %s)", detected.Name, detected.TestCmd)
  } else {
    fmt.Println("[AVISO] Stack não detectada automaticamente. O SCOUT tentará identificá-la.")
    logEvent("AVISO: stack não detectada automaticamente")
  }

  sourceHint := existingDocs
  if sourceHint == "" {
    sourceHint = "Nenhuma documentação prévia informada (criar estrutura inicial em " + targetDocsDir + "/)"
  }

  detectedHint := "(nenhuma)"
  if ok {
    detectedHint = detected.Name
  }
  prompt := fmt.Sprintf(
    "Siga estritamente as instruções de @.makercycle/agents/00_SCOUT.md. "+
      "Documentação existente informada pelo usuário: '%s'. "+
      "Diretório oficial de saída para documentação: '%s/'. "+
      "Stack pré-detectada pelo CLI: '%s'. Valide ou corrija as chaves MC_STACK_NAME, MC_TEST_CMD, MC_TYPE_CMD e MC_LINT_CMD em .makercycle/config/project.env, preservando as demais chaves. "+
      "Analise a estrutura do código existente, leia a documentação informada e gere %s/PROJECT_CONTEXT.md e %s/demands/INDEX.md.",
    sourceHint, targetDocsDir, detectedHint, targetDocsDir, targetDocsDir,
  )

  stats, err := runAgent(scout, prompt, jsonMetrics)
  if err != nil {
    fatal("Falha na Etapa 0 (SCOUT): %v", err)
  }

  reportTokens(scout, stats)

  contextPath := filepath.Join(targetDocsDir, "PROJECT_CONTEXT.md")
  if info, statErr := os.Stat(contextPath); statErr != nil || info.IsDir() {
    fatal("Etapa 0 terminou sem gerar %s. O SCOUT pode ter sido bloqueado por permissões (veja MC_SCOUT_ARGS em .makercycle/config/models.env) ou falhou ao gravar o arquivo.", contextPath)
  }

  if err := ensureDocsStructureSection(contextPath, targetDocsDir); err != nil {
    fmt.Printf("[AVISO] Não foi possível documentar a estrutura de pastas em %s: %v\n", contextPath, err)
    logEvent("AVISO: falha ao documentar estrutura em %s: %v", contextPath, err)
  }

  savedStack, loadErr := loadProjectStack(projectEnvPath)
  bannerMsg := fmt.Sprintf("Contexto salvo em /%s/PROJECT_CONTEXT.md.", targetDocsDir)
  if loadErr == nil {
    bannerMsg += fmt.Sprintf("\nStack salva: %s", savedStack.Name)
  } else {
    fmt.Printf("[AVISO] Stack não configurada em project.env (motivo: %v). O menu manual será exibido no -task.\n", loadErr)
    logEvent("AVISO: stack não configurada em project.env: %v", loadErr)
  }
  bannerMsg += "\nDispare novas demandas com: go run . -task \"...\""
  printBanner("ETAPA 0 CONCLUÍDA", bannerMsg)
}

// referencesLine devolve a linha "- **Referências:**" da demanda (vazia quando não há referências).
func referencesLine(refs []string) string {
  if len(refs) == 0 {
    return ""
  }
  quoted := make([]string, len(refs))
  for i, r := range refs {
    quoted[i] = "`" + r + "`"
  }
  return "- **Referências:** " + strings.Join(quoted, ", ") + "\n"
}

func registerDemandInDocs(docsDir, stackName, task, modelsSummary, logName string, refs []string) string {
  demandsDir := filepath.Join(docsDir, "demands")
  _ = os.MkdirAll(demandsDir, 0755)

  datePrefix := time.Now().Format("2006-01-02")
  slug := slugify(task)
  fileName := fmt.Sprintf("%s-%s.md", datePrefix, slug)
  fullPath := filepath.Join(demandsDir, fileName)

  logLine := ""
  if logName != "" {
    logLine = fmt.Sprintf("- **Log de Execução:** [`%s`](../logs/%s)\n", logName, logName)
  }

  if _, err := os.Stat(fullPath); os.IsNotExist(err) {
    content := fmt.Sprintf(
      "# Demanda: %s\n\n"+
        "- **Data:** %s\n"+
        "- **Stack:** %s\n"+
        "- **Bindings:** `%s`\n"+
        "- **Status:** `IN_PROGRESS`\n"+
        "- **Tentativas:** `0`\n"+
        "- **Tokens Totais:** `0`\n"+
        "%s%s\n"+
        "## Descrição Original\n\n%s\n\n"+
        "## Plano & Critérios de Aceite\n\n*(Será preenchido automaticamente pelo PLANNER na Etapa 1)*\n",
      task, time.Now().Format("2006-01-02 15:04"), stackName, modelsSummary, logLine, referencesLine(refs), task,
    )
    _ = os.WriteFile(fullPath, []byte(content), 0644)
  } else if b, readErr := os.ReadFile(fullPath); readErr == nil {
    // Demanda já registrada (nova execução no mesmo dia): vincula também o log e as referências desta execução.
    existing := string(b)
    extra := ""
    if logLine != "" && !strings.Contains(existing, "(../logs/"+logName+")") {
      extra += logLine
    }
    var missing []string
    for _, r := range refs {
      if !strings.Contains(existing, "`"+r+"`") {
        missing = append(missing, r)
      }
    }
    extra += referencesLine(missing)
    if extra != "" {
      sep := ""
      if !strings.HasSuffix(existing, "\n") {
        sep = "\n"
      }
      _ = os.WriteFile(fullPath, append(b, []byte(sep+"\n"+extra)...), 0644)
    }
  }

  indexPath := filepath.Join(demandsDir, "INDEX.md")
  if _, err := os.Stat(indexPath); os.IsNotExist(err) {
    header := "# Índice de Demandas — makercycle\n\n| Data | Stack | Demanda | Arquivo | Status |\n| :--- | :--- | :--- | :--- | :--- |\n"
    _ = os.WriteFile(indexPath, []byte(header), 0644)
  }

  cleanTask := strings.ReplaceAll(task, "|", "-")
  row := fmt.Sprintf("| %s | %s | %s | [`%s`](%s) | `IN_PROGRESS` |\n",
    datePrefix, stackName, cleanTask, fileName, fileName)

  if f, err := os.OpenFile(indexPath, os.O_APPEND|os.O_WRONLY, 0644); err == nil {
    defer f.Close()
    _, _ = f.WriteString(row)
  }

  return filepath.ToSlash(fullPath)
}

func updateDemandStatus(demandPath, status string, attempts int, tokens TokenStats) {
  if demandPath == "" {
    return
  }
  updateIndexStatus(demandPath, status)
  b, err := os.ReadFile(demandPath)
  if err != nil {
    return
  }
  content := strings.Replace(string(b), "- **Status:** `IN_PROGRESS`", fmt.Sprintf("- **Status:** `%s`", status), 1)
  content = strings.Replace(content, "- **Tentativas:** `0`", fmt.Sprintf("- **Tentativas:** `%d`", attempts), 1)
  content = strings.Replace(content, "- **Tokens Totais:** `0`", fmt.Sprintf("- **Tokens Totais:** `%s`", tokens.Format()), 1)
  _ = os.WriteFile(demandPath, []byte(content), 0644)
}

// updateIndexStatus localiza no INDEX.md (mesma pasta demands/ da demanda) as linhas da demanda
// ainda marcadas como IN_PROGRESS e troca pelo novo status. Aceita links relativos ao índice
// ("(arquivo.md)") e o formato legado "(demands/arquivo.md)".
func updateIndexStatus(demandPath, status string) {
  if strings.TrimSpace(status) == "" {
    return
  }
  fileName := filepath.Base(demandPath)
  indexPath := filepath.Join(filepath.Dir(demandPath), "INDEX.md")
  b, err := os.ReadFile(indexPath)
  if err != nil {
    return
  }
  lines := strings.Split(string(b), "\n")
  changed := false
  for i, line := range lines {
    linked := strings.Contains(line, "("+fileName+")") || strings.Contains(line, "(demands/"+fileName+")")
    if linked && strings.Contains(line, "`IN_PROGRESS`") {
      lines[i] = strings.Replace(line, "`IN_PROGRESS`", fmt.Sprintf("`%s`", status), 1)
      changed = true
    }
  }
  if changed {
    _ = os.WriteFile(indexPath, []byte(strings.Join(lines, "\n")), 0644)
  }
}


func runDeterministicChecks(stack StackConfig) bool {
  var report strings.Builder
  allPassed := true

  commands := []struct {
    label string
    cmd   string
  }{
    {"Suíte de Testes", stack.TestCmd},
    {"Verificação de Tipos / Vet", stack.TypeCmd},
    {"Linter", stack.LintCmd},
  }

  for _, c := range commands {
    if strings.TrimSpace(c.cmd) == "" {
      continue
    }
    fmt.Printf("\n> Executando %s: %s\n", c.label, c.cmd)
    logEvent("ETAPA 4A: executando %s: %s", c.label, c.cmd)
    var output string
    var err error
    t0 := time.Now()
    withSpinner(fmt.Sprintf("ETAPA 4A: VERIFY: CHECK | %s", c.cmd), func() {
      output, err = runShellCommand(c.cmd)
    })
    fmt.Print(output)
    status := "PASSED"
    if err != nil {
      status = "FAILED"
      allPassed = false
    }
    logBlock(fmt.Sprintf("ETAPA 4A — %s (%s) — %s [%s]", c.label, c.cmd, status, formatElapsed(time.Since(t0))), output)
    report.WriteString(fmt.Sprintf("## %s (`%s`) — %s\n\n```text\n%s\n```\n\n", c.label, c.cmd, status, output))
  }

  header := fmt.Sprintf("# Relatório de Verificação Determinística (Etapa 4A)\n\nALL_CHECKS_PASSED: %v\nStack: %s\nTimestamp: %s\n\n",
    allPassed, stack.Name, time.Now().Format(time.RFC3339))

  _ = os.WriteFile(".makercycle/state/VERIFY_REPORT.md", []byte(header+report.String()), 0644)
  logEvent("ETAPA 4A: ALL_CHECKS_PASSED=%v", allPassed)
  return allPassed
}

func agentLabel(a AgentBinding) string {
  stage := a.Label
  if a.Stage != "" {
    stage = a.Label + ": " + a.Stage
  }
  return fmt.Sprintf("%s — %s | %s", stage, a.Alias, a.Model)
}

// runAgent executa o agente preservando a assinatura legada (utilizada no setup da Etapa 0 e testes legados).
func runAgent(agent AgentBinding, prompt string, tryJSON bool) (TokenStats, error) {
  mode, err := resolveAuthMode(os.Getenv("MC_AUTH_MODE"))
  if err != nil {
    mode = authModeAPIKey
  }
  res := runAgentStep(agent, prompt, tryJSON, mode, loadOmniRouteConfig(os.Getenv))
  return res.Stats, res.Err
}

// runAgentStep executa uma etapa de agente com telemetria, fallback OmniRoute e registro limpo de logs.
func runAgentStep(agent AgentBinding, prompt string, tryJSON bool, authMode string, omni OmniRouteConfig) AgentResult {
  runner := func(a AgentBinding, env []string, p string, tj bool) (TokenStats, string, error) {
    label := agentLabel(a)
    start := time.Now()
    logBlock("PROMPT ENVIADO — "+label, p)
    stats, text, err := runAgentCore(a, label, p, tj, env)
    logBlock("RESPOSTA RECEBIDA — "+label, text)
    logEvent("%s concluído em %s (erro: %v)", label, formatElapsed(time.Since(start)), err)
    return stats, text, err
  }
  return runAgentWithFallback(agent, prompt, tryJSON, authMode, omni, os.Environ(), runner, yellowWriter(os.Stdout))
}

func runAgentCore(agent AgentBinding, label, prompt string, tryJSON bool, env []string) (TokenStats, string, error) {
  if env == nil {
    // Nunca deixar o subprocesso herdar o ambiente cru: aplica a mesma montagem de autenticação.
    env = agentEnvFromProcess()
  }
  baseArgs := []string{}
  if strings.TrimSpace(agent.Model) != "" {
    baseArgs = append(baseArgs, "--model", agent.Model)
  }
  baseArgs = append(baseArgs, strings.Fields(agent.Args)...)
  baseArgs = append(baseArgs, "-p", prompt)

  // A saída do agente é sempre capturada (nunca transmitida ao vivo) enquanto o spinner redesenha a linha;
  // ela só é impressa depois que o spinner limpa a linha.
  run := func(args []string) (out string, err error) {
    withSpinner(label, func() { out, err = execCaptureEnv(agent.Bin, args, env) })
    return out, err
  }

  if tryJSON {
    jsonArgs := append(append([]string{}, baseArgs...), "--output-format", "json")
    rawOut, err := run(jsonArgs)
    if err == nil {
      // Comando executou com sucesso: NUNCA reexecutar (evita trabalho duplicado e efeitos colaterais).
      text, stats, hasUsage, ok := parseJSONUsage(rawOut)
      if !ok || text == "" {
        text = rawOut
      }
      fmt.Println(text)
      if !ok || !hasUsage {
        stats = extractOrEstimateTokens(prompt, text)
      }
      return stats, text, nil
    }
    if !isUnsupportedFlagError(rawOut) {
      // Falha real do agente (não é incompatibilidade de flag): não reexecutar.
      fmt.Print(rawOut)
      return extractOrEstimateTokens(prompt, rawOut), rawOut, err
    }
    // Flag --output-format não suportada pelo binário: fallback único para modo texto.
    logEvent("%s: --output-format não suportado; fallback único para modo texto", label)
  }

  rawOut, err := run(baseArgs)
  fmt.Print(rawOut)
  return extractOrEstimateTokens(prompt, rawOut), rawOut, err
}

var unsupportedFlagRe = regexp.MustCompile(`(?i)(unknown|unrecognized|invalid|unexpected)\s+(option|flag|argument)|flag provided but not defined|no such option`)

func isUnsupportedFlagError(output string) bool {
  return unsupportedFlagRe.MatchString(output)
}

// execCaptureEnv executa o comando capturando stdout+stderr (sem escrever no console), permitindo passar variáveis de ambiente customizadas.
func execCaptureEnv(bin string, args, env []string) (string, error) {
  cmd := exec.Command(bin, args...)
  if env != nil {
    cmd.Env = env
  }
  var buf bytes.Buffer
  cmd.Stdout = &buf
  cmd.Stderr = &buf
  err := cmd.Run()
  return buf.String(), err
}

// execCapture executa o comando capturando stdout+stderr (sem escrever no console).
func execCapture(bin string, args []string) (string, error) {
  return execCaptureEnv(bin, args, agentEnvFromProcess())
}

// agentEnvFromProcess monta o ambiente do subprocesso a partir de os.Environ() (já com models.env/project.env carregados)
// usando o modo de autenticação configurado.
func agentEnvFromProcess() []string {
  mode, err := resolveAuthMode(os.Getenv("MC_AUTH_MODE"))
  if err != nil {
    mode = authModeAPIKey
  }
  return buildAgentEnv(os.Environ(), mode, loadOmniRouteConfig(os.Getenv), false)
}

func parseJSONUsage(raw string) (text string, stats TokenStats, hasUsage bool, ok bool) {
  var candidatePayload map[string]any
  var firstValidPayload map[string]any

  targetKeys := []string{"result", "response", "content", "text", "message", "usage", "usageMetadata"}

  for i := 0; i < len(raw); i++ {
    if raw[i] != '{' {
      continue
    }
    dec := json.NewDecoder(strings.NewReader(raw[i:]))
    var obj map[string]any
    if err := dec.Decode(&obj); err != nil {
      continue
    }
    hasKey := false
    for _, k := range targetKeys {
      if _, exists := obj[k]; exists {
        hasKey = true
        break
      }
    }
    if hasKey {
      candidatePayload = obj
      break
    }
    if firstValidPayload == nil {
      firstValidPayload = obj
    }
    if offset := dec.InputOffset(); offset > 0 {
      i += int(offset) - 1
    }
  }

  payload := candidatePayload
  if payload == nil {
    payload = firstValidPayload
  }
  if payload == nil {
    return "", TokenStats{}, false, false
  }

  for _, key := range []string{"result", "response", "content", "text", "message"} {
    if v, isStr := payload[key].(string); isStr && v != "" {
      text = v
      break
    }
  }

  usage, isMap := payload["usage"].(map[string]any)
  if !isMap {
    usage, isMap = payload["usageMetadata"].(map[string]any)
  }
  if isMap {
    stats.Input = intFromMap(usage, "input_tokens", "prompt_tokens", "promptTokenCount")
    stats.Output = intFromMap(usage, "output_tokens", "completion_tokens", "candidatesTokenCount")
    // Claude CLI reporta o prompt em cache à parte; somá-lo reflete o consumo total de entrada.
    stats.Input += intFromMap(usage, "cache_creation_input_tokens") + intFromMap(usage, "cache_read_input_tokens")
    stats.Total = intFromMap(usage, "total_tokens", "totalTokenCount")
    if stats.Total == 0 {
      stats.Total = stats.Input + stats.Output
    }
    hasUsage = stats.Total > 0
  }

  return text, stats, hasUsage, true
}

func intFromMap(m map[string]any, keys ...string) int {
  for _, k := range keys {
    if val, ok := m[k].(float64); ok {
      return int(val)
    }
  }
  return 0
}

var (
  tokenInRe  = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(?:input|prompt)[_\s-]*tokens["'\s:=]+(\d{1,3}(?:,\d{3})+|\d+)`)
  tokenOutRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9_])(?:output|completion|candidates)[_\s-]*tokens["'\s:=]+(\d{1,3}(?:,\d{3})+|\d+)`)
)

func extractOrEstimateTokens(prompt, output string) TokenStats {
  mIn := tokenInRe.FindStringSubmatch(output)
  mOut := tokenOutRe.FindStringSubmatch(output)
  if mIn != nil && mOut != nil {
    inVal, errIn := strconv.Atoi(strings.ReplaceAll(mIn[1], ",", ""))
    outVal, errOut := strconv.Atoi(strings.ReplaceAll(mOut[1], ",", ""))
    if errIn == nil && errOut == nil {
      return TokenStats{Input: inVal, Output: outVal, Total: inVal + outVal, Estimated: false}
    }
  }

  estIn := (len(prompt) / 4) + 1200
  estOut := len(output) / 4
  return TokenStats{
    Input:     estIn,
    Output:    estOut,
    Total:     estIn + estOut,
    Estimated: true,
  }
}

func runShellCommand(command string) (string, error) {
  cmd := exec.Command("sh", "-c", command)
  var buf bytes.Buffer
  cmd.Stdout = &buf
  cmd.Stderr = &buf
  err := cmd.Run()
  return buf.String(), err
}

const (
  telemetryDir   = ".makercycle/telemetry"
  runsCSVPath    = telemetryDir + "/runs.csv"
  eventsCSVPath  = telemetryDir + "/events.csv"
  noDemandMarker = "(via .makercycle/state/PLAN.md)"
)

var (
  runsCSVHeader = []string{"RunID", "Timestamp", "Stack", "DemandFile", "MaxAttempts", "TotalAttempts",
    "TotalTokensIn", "TotalTokensOut", "TotalTokens", "FinalStatus", "Note"}
  eventsCSVHeader = []string{"RunID", "Timestamp", "Stage", "Agent", "Model", "Attempt",
    "TokensIn", "TokensOut", "TotalTokens", "DurationSeconds"}
)

// newRunID gera o identificador único da execução corrente (timestamp em nanossegundos).
func newRunID() string {
  return strconv.FormatInt(time.Now().UnixNano(), 10)
}

// appendCSVRecord acrescenta uma linha ao CSV em path (usando encoding/csv para o escape correto),
// criando o diretório e escrevendo o cabeçalho quando o arquivo ainda não existe ou está vazio.
func appendCSVRecord(path string, header, record []string) error {
  if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
    return err
  }
  f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
  if err != nil {
    return err
  }
  w := csv.NewWriter(f)
  if fi, statErr := f.Stat(); statErr == nil && fi.Size() == 0 {
    _ = w.Write(header)
  }
  _ = w.Write(record)
  w.Flush()
  if err := w.Error(); err != nil {
    _ = f.Close()
    return err
  }
  return f.Close()
}

// recordRunTelemetry registra em .makercycle/telemetry/runs.csv o consolidado de uma execução do MakerCycle
// (uma linha por demanda completa). Falhas de escrita não interrompem o MakerCycle.
func recordRunTelemetry(runID, stack, demandFile string, maxAttempts, attempts int, tokens TokenStats, status, note string) {
  if demandFile == "" {
    demandFile = noDemandMarker
  }
  record := []string{
    runID, time.Now().Format(time.RFC3339), stack, demandFile,
    strconv.Itoa(maxAttempts), strconv.Itoa(attempts),
    strconv.Itoa(tokens.Input), strconv.Itoa(tokens.Output), strconv.Itoa(tokens.Total),
    status, note,
  }
  if err := appendCSVRecord(runsCSVPath, runsCSVHeader, record); err != nil {
    fmt.Printf("[AVISO] Falha ao gravar %s: %v\n", runsCSVPath, err)
  }
}

// recordEventTelemetry registra em .makercycle/telemetry/events.csv uma linha por etapa/agente executado.
// duration é o tempo de execução da etapa, em segundos.
func recordEventTelemetry(runID, stage, agent, model string, attempt int, tokens TokenStats, duration float64) {
  record := []string{
    runID, time.Now().Format(time.RFC3339), stage, agent, model, strconv.Itoa(attempt),
    strconv.Itoa(tokens.Input), strconv.Itoa(tokens.Output), strconv.Itoa(tokens.Total),
    strconv.FormatFloat(duration, 'f', 3, 64),
  }
  if err := appendCSVRecord(eventsCSVPath, eventsCSVHeader, record); err != nil {
    fmt.Printf("[AVISO] Falha ao gravar %s: %v\n", eventsCSVPath, err)
  }
}

func loadEnvFile(path string) {
  b, err := os.ReadFile(path)
  if err != nil {
    return
  }
  for _, line := range strings.Split(string(b), "\n") {
    line = strings.TrimSpace(line)
    if line == "" || strings.HasPrefix(line, "#") {
      continue
    }
    line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
    parts := strings.SplitN(line, "=", 2)
    if len(parts) != 2 {
      continue
    }
    key := strings.TrimSpace(parts[0])
    val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
    if os.Getenv(key) == "" {
      _ = os.Setenv(key, val)
    }
  }
}

func getEnv(key, fallback string) string {
  if v := strings.TrimSpace(os.Getenv(key)); v != "" {
    return v
  }
  return fallback
}

func getEnvInt(key string, fallback int) int {
  if v := strings.TrimSpace(os.Getenv(key)); v != "" {
    if i, err := strconv.Atoi(v); err == nil {
      return i
    }
  }
  return fallback
}

func slugify(s string) string {
  var b strings.Builder
  lastDash := false
  for _, r := range strings.ToLower(s) {
    if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) {
      b.WriteRune(r)
      lastDash = false
    } else if !lastDash {
      b.WriteRune('-')
      lastDash = true
    }
  }
  res := strings.Trim(b.String(), "-")
  if runes := []rune(res); len(runes) > 45 {
    res = strings.Trim(string(runes[:45]), "-")
  }
  if res == "" {
    return "demanda"
  }
  return res
}

func readFile(path string) (string, error) {
  b, err := os.ReadFile(path)
  if err != nil {
    return "", err
  }
  return string(b), nil
}

func printBanner(title, subtitle string) {
  fmt.Printf("\n==============================================================================\n")
  fmt.Printf(" %s\n %s\n", title, subtitle)
  fmt.Printf("==============================================================================\n")
  logEvent("BANNER: %s | %s", title, strings.ReplaceAll(subtitle, "\n", " / "))
}

func fatal(format string, args ...any) {
  msg := fmt.Sprintf(format, args...)
  fmt.Fprintf(os.Stderr, "\n[ERRO FATAL] %s\n", msg)
  logEvent("ERRO FATAL: %s", msg)
  finish(1)
}

// finish registra o código de saída, informa o caminho do log e encerra o processo.
func finish(code int) {
  if runLog != nil {
    logEvent("Código de saída final: %d", code)
    fmt.Printf("\n[LOG] Registro completo salvo em: %s\n", runLog.Path)
    runLog.Close()
    runLog = nil
  }
  os.Exit(code)
}

func reportTokens(a AgentBinding, s TokenStats) {
  fmt.Printf("[TOKENS %s - %s] %s\n", a.Label, a.Alias, s.Format())
  logEvent("TOKENS %s - %s: %s", a.Label, a.Alias, s.Format())
}

// ----------------------------------------------------------------------------
// Estrutura de pastas e log de execução
// ----------------------------------------------------------------------------

// stringList é um flag repetível (-ref a -ref b).
type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(v string) error {
  *l = append(*l, v)
  return nil
}

// resolveReferences valida os arquivos passados em -ref. Cada item é aceito como caminho direto
// (relativo ao diretório atual ou absoluto) ou, se não existir, como nome dentro de
// <docsDir>/references/. Retorna os caminhos normalizados (separador "/"), sem duplicatas.
func resolveReferences(refs []string, docsDir string) ([]string, error) {
  var out []string
  seen := map[string]bool{}
  for _, raw := range refs {
    ref := strings.TrimSpace(raw)
    if ref == "" {
      return nil, fmt.Errorf("-ref não pode ser vazio")
    }
    candidates := []string{filepath.Clean(ref)}
    if !filepath.IsAbs(ref) {
      candidates = append(candidates, filepath.Join(docsDir, "references", ref))
    }
    found := ""
    for _, c := range candidates {
      if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
        found = filepath.ToSlash(c)
        break
      }
    }
    if found == "" {
      return nil, fmt.Errorf("arquivo de referência não encontrado: %s (procurado em: %s)", ref, strings.Join(candidates, ", "))
    }
    if !seen[found] {
      seen[found] = true
      out = append(out, found)
    }
  }
  return out, nil
}

// referencesPrompt monta o trecho do prompt do PLANNER que aponta para os arquivos de referência.
func referencesPrompt(refs []string) string {
  if len(refs) == 0 {
    return ""
  }
  at := make([]string, len(refs))
  for i, r := range refs {
    at[i] = "@" + r
  }
  return " Material de referência (leia integralmente antes de planejar e respeite-o no PLAN.md e nos testes): " + strings.Join(at, ", ") + "."
}

// ensureMakerCycleDirs cria .makercycle/{agents,config,state,telemetry}.
func ensureMakerCycleDirs() error {
  for _, sub := range []string{"agents", "config", "state", "telemetry"} {
    if err := os.MkdirAll(filepath.Join(".makercycle", sub), 0755); err != nil {
      return err
    }
  }
  return nil
}

// ensureDocsDirs cria <docs>/demands, <docs>/logs e <docs>/references (com .gitkeep).
func ensureDocsDirs(docsDir string) error {
  for _, sub := range []string{"demands", "logs", "references"} {
    if err := os.MkdirAll(filepath.Join(docsDir, sub), 0755); err != nil {
      return err
    }
  }
  keep := filepath.Join(docsDir, "references", ".gitkeep")
  if _, err := os.Stat(keep); os.IsNotExist(err) {
    return os.WriteFile(keep, nil, 0644)
  }
  return nil
}

const docsStructureMarker = "## Estrutura de Documentação do makercycle"

// ensureDocsStructureSection acrescenta ao PROJECT_CONTEXT.md a descrição da estrutura de pastas (idempotente).
func ensureDocsStructureSection(contextPath, docsDir string) error {
  b, err := os.ReadFile(contextPath)
  if err != nil {
    return err
  }
  if strings.Contains(string(b), docsStructureMarker) {
    return nil
  }
  section := fmt.Sprintf("\n%s\n\n"+
    "- `%s/PROJECT_CONTEXT.md` — contexto do projeto (este arquivo).\n"+
    "- `%s/demands/INDEX.md` — índice das demandas (`| Data | Stack | Demanda | Arquivo | Status |`).\n"+
    "- `%s/demands/` — um arquivo `AAAA-MM-DD-<slug>.md` por demanda (descrição, plano, status, tentativas e tokens).\n"+
    "- `%s/references/` — material de referência do projeto (specs, links, artigos).\n"+
    "- `%s/logs/` — um log completo por execução: `AAAA-MM-DD_HHMMSS-<slug>.log` (com demanda) ou `AAAA-MM-DD_HHMMSS-stage<N>.log` (sem demanda). "+
    "Cada demanda aponta para o seu log no campo **Log de Execução**.\n",
    docsStructureMarker, docsDir, docsDir, docsDir, docsDir, docsDir)
  sep := ""
  if !strings.HasSuffix(string(b), "\n") {
    sep = "\n"
  }
  return os.WriteFile(contextPath, append(b, []byte(sep+section)...), 0644)
}

type RunLog struct {
  mu   sync.Mutex
  f    *os.File
  Path string // caminho de exibição (barras normais)
  Name string // nome do arquivo
}

var runLog *RunLog

// initRunLog cria <docs>/logs/ e o arquivo AAAA-MM-DD_HHMMSS-<suffix>.log desta execução.
func initRunLog(docsDir, suffix string) (*RunLog, error) {
  logsDir := filepath.Join(docsDir, "logs")
  if err := os.MkdirAll(logsDir, 0755); err != nil {
    return nil, err
  }
  name := fmt.Sprintf("%s-%s.log", time.Now().Format("2006-01-02_150405"), suffix)
  path := filepath.Join(logsDir, name)
  f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
  if err != nil {
    return nil, err
  }
  return &RunLog{f: f, Path: filepath.ToSlash(path), Name: name}, nil
}

func startRunLog(docsDir, suffix string) {
  l, err := initRunLog(docsDir, suffix)
  if err != nil {
    fmt.Fprintf(os.Stderr, "[AVISO] Não foi possível criar o log de execução: %v\n", err)
    return
  }
  runLog = l
  fmt.Printf("[LOG] Registro completo salvo em: %s\n", l.Path)
}

// ansiRe casa sequências de escape ANSI (cores, cursor, limpeza de linha) emitidas por subprocessos como o npx.
var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

func (l *RunLog) write(s string) {
  if l == nil {
    return
  }
  s = ansiRe.ReplaceAllString(s, "")
  l.mu.Lock()
  defer l.mu.Unlock()
  _, _ = l.f.WriteString(s)
}

func (l *RunLog) Close() {
  if l == nil {
    return
  }
  l.mu.Lock()
  defer l.mu.Unlock()
  _ = l.f.Sync()
  _ = l.f.Close()
}

func stamp() string { return time.Now().Format("15:04:05") }

// logEvent grava uma linha com timestamp [HH:MM:SS] (no-op se não houver log ativo).
func logEvent(format string, args ...any) {
  runLog.write(fmt.Sprintf("[%s] %s\n", stamp(), fmt.Sprintf(format, args...)))
}

// logBlock grava um bloco delimitado com o conteúdo integral (sem cortes).
func logBlock(title, content string) {
  if !strings.HasSuffix(content, "\n") {
    content += "\n"
  }
  runLog.write(fmt.Sprintf("[%s] ----- INÍCIO: %s -----\n%s[%s] ----- FIM: %s -----\n", stamp(), title, content, stamp(), title))
}

func describeBinding(a AgentBinding) string {
  return fmt.Sprintf("%s (%s) bin=%q modelo=%q skill=%q args=%q", a.Label, a.Alias, a.Bin, a.Model, a.Skill, a.Args)
}

func buildRunHeader(stack StackConfig, agents []AgentBinding, task, demandFile string, startStage, maxAttempts int, docsDir string) string {
  var b strings.Builder
  fmt.Fprintf(&b, "Data/Hora: %s\n", time.Now().Format(time.RFC3339))
  fmt.Fprintf(&b, "Stack: %s\n", stack.Name)
  fmt.Fprintf(&b, "Comando de testes: %s\nComando de tipos/vet: %s\nComando de linter: %s\n", stack.TestCmd, stack.TypeCmd, stack.LintCmd)
  fmt.Fprintf(&b, "Etapa inicial: %d | Max tentativas: %d | Docs: %s/\n", startStage, maxAttempts, docsDir)
  b.WriteString("Bindings:\n")
  for _, a := range agents {
    fmt.Fprintf(&b, "  - %s\n", describeBinding(a))
  }
  if task == "" {
    task = "(sem demanda)"
  }
  fmt.Fprintf(&b, "Demanda: %s\n", task)
  if demandFile != "" {
    fmt.Fprintf(&b, "Arquivo da demanda: %s\n", demandFile)
  }
  return b.String()
}

// ----------------------------------------------------------------------------
// Spinner de progresso
// ----------------------------------------------------------------------------

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func isTerminal(f *os.File) bool {
  fi, err := f.Stat()
  return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func formatElapsed(d time.Duration) string {
  secs := int(d.Seconds())
  return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

// withSpinner executa fn exibindo um spinner no stdout (apenas em TTY; fora dele imprime uma linha estática).
// O spinner escreve somente no console: nunca toca no arquivo de log nem na captura do comando.
func withSpinner(label string, fn func()) {
  withSpinnerTo(os.Stdout, isTerminal(os.Stdout), label, fn)
}

func withSpinnerTo(w io.Writer, tty bool, label string, fn func()) {
  if !tty {
    fmt.Fprintf(w, "[AGUARDE] [%s] Processando...\n", label)
    fn()
    return
  }

  start := time.Now()
  stop := make(chan struct{})
  done := make(chan struct{})
  go func() {
    defer close(done)
    ticker := time.NewTicker(100 * time.Millisecond)
    defer ticker.Stop()
    i := 0
    draw := func() {
      fmt.Fprintf(w, "\r\033[K%s [%s] Processando... (%s)", spinnerFrames[i%len(spinnerFrames)], label, formatElapsed(time.Since(start)))
      i++
    }
    draw()
    for {
      select {
      case <-stop:
        fmt.Fprint(w, "\r\033[K")
        return
      case <-ticker.C:
        draw()
      }
    }
  }()
  defer func() {
    close(stop)
    <-done
  }()
  fn()
}

// ----------------------------------------------------------------------------
// Etapa inicial (-stage)
// ----------------------------------------------------------------------------

// resolveStage valida a Etapa inicial informada em -stage (1 a 5).
func resolveStage(v int) (int, error) {
  if v < 1 || v > 5 {
    return 0, fmt.Errorf("etapa inicial inválida: %d (use 1 a 5)", v)
  }
  return v, nil
}

// ----------------------------------------------------------------------------
// -init: binários, skills e estrutura de pastas
// ----------------------------------------------------------------------------

type BinStatus struct {
  Name  string
  Path  string
  Found bool
}

// checkBinaries verifica a disponibilidade de cada binário (sem duplicatas, na ordem informada).
func checkBinaries(names []string, lookPath func(string) (string, error)) []BinStatus {
  seen := map[string]bool{}
  var res []BinStatus
  for _, n := range names {
    n = strings.TrimSpace(n)
    if n == "" || seen[n] {
      continue
    }
    seen[n] = true
    path, err := lookPath(n)
    res = append(res, BinStatus{Name: n, Path: path, Found: err == nil})
  }
  return res
}

var skillEnvRe = regexp.MustCompile(`^MC_[A-Z0-9]+_SKILL$`)

// collectConfiguredSkills retorna as skills únicas (ordenadas) definidas em variáveis MC_*_SKILL.
func collectConfiguredSkills(environ []string) []string {
  seen := map[string]bool{}
  var skills []string
  for _, kv := range environ {
    parts := strings.SplitN(kv, "=", 2)
    if len(parts) != 2 || !skillEnvRe.MatchString(parts[0]) {
      continue
    }
    v := strings.TrimSpace(parts[1])
    if v == "" || seen[v] {
      continue
    }
    seen[v] = true
    skills = append(skills, v)
  }
  sort.Strings(skills)
  return skills
}

// skillShortName extrai o nome da skill de um identificador "dono/repositório[@versão]".
func skillShortName(id string) string {
  id = strings.TrimSpace(id)
  if i := strings.LastIndex(id, "/"); i >= 0 {
    id = id[i+1:]
  }
  if i := strings.Index(id, "@"); i >= 0 {
    id = id[:i]
  }
  return strings.TrimSuffix(id, ".git")
}

// skillSearchRoots lista as pastas onde uma skill pode estar instalada (locais e globais).
func skillSearchRoots(baseDir, home string) []string {
  roots := []string{filepath.Join(baseDir, "skills"), filepath.Join(baseDir, ".agents", "skills")}
  if home != "" {
    roots = append(roots, filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".antigravity", "skills"))
  }
  return roots
}

// skillAliases mapeia o nome curto de uma skill configurada para o nome com que o
// `npx skills add` a registra/instala (a pasta ou a chave do lock diferem do nome do repositório).
var skillAliases = map[string]string{
  "security-audit-skill":    "security-audit",
  "superpowers":             "writing-plans",
  "elephant-goldfish-model": "egm-review",
}

// skillInstalled considera a skill presente se existir uma pasta com o seu nome (ou alias) em alguma
// raiz, ou se algum lock registrar o identificador como origem ou o nome (ou alias) como chave.
// lockPaths aceita vários arquivos separados por os.PathListSeparator
// (ex.: skills-lock.json local e ~/.agents/.skill-lock.json global).
func skillInstalled(skill string, roots []string, lockPaths string) bool {
  name := skillShortName(skill)
  if name == "" {
    return false
  }
  names := []string{name}
  if alias, ok := skillAliases[strings.ToLower(name)]; ok {
    names = append(names, alias)
  }
  for _, r := range roots {
    for _, n := range names {
      if fi, err := os.Stat(filepath.Join(r, n)); err == nil && fi.IsDir() {
        return true
      }
    }
  }
  for _, lockPath := range filepath.SplitList(lockPaths) {
    b, err := os.ReadFile(lockPath)
    if err != nil {
      continue
    }
    var lock struct {
      Skills map[string]struct {
        Source string `json:"source"`
      } `json:"skills"`
    }
    if json.Unmarshal(b, &lock) != nil {
      continue
    }
    for k, v := range lock.Skills {
      if strings.EqualFold(v.Source, skill) {
        return true
      }
      for _, n := range names {
        if strings.EqualFold(k, n) {
          return true
        }
      }
    }
  }
  return false
}

func missingSkills(skills, roots []string, lockPath string) []string {
  var missing []string
  for _, sk := range skills {
    if !skillInstalled(sk, roots, lockPath) {
      missing = append(missing, sk)
    }
  }
  return missing
}

type SkillResult struct {
  Skill  string
  Action string // "presente", "instalada" ou "falha"
  Output string
  Err    error
}

// ensureSkills instala (via install) as skills ausentes, ou todas quando force=true.
func ensureSkills(skills, roots []string, lockPath string, force bool, install func(string) (string, error)) []SkillResult {
  var res []SkillResult
  for _, sk := range skills {
    if !force && skillInstalled(sk, roots, lockPath) {
      res = append(res, SkillResult{Skill: sk, Action: "presente"})
      continue
    }
    out, err := install(sk)
    r := SkillResult{Skill: sk, Action: "instalada", Output: out, Err: err}
    if err != nil {
      r.Action = "falha"
    }
    res = append(res, r)
  }
  return res
}

func skillInstallArgs(skill string) []string {
  return []string{"skills", "add", skill, "-g", "-y"}
}

// stageSkills devolve as skills realmente necessárias a partir da etapa inicial.
func stageSkills(startStage int, planner, critic, auditor AgentBinding) []string {
  var list []string
  if startStage <= 1 {
    list = append(list, planner.Skill)
  }
  if startStage <= 2 {
    list = append(list, critic.Skill)
  }
  list = append(list, auditor.Skill)
  seen := map[string]bool{}
  var out []string
  for _, sk := range list {
    sk = strings.TrimSpace(sk)
    if sk != "" && !seen[sk] {
      seen[sk] = true
      out = append(out, sk)
    }
  }
  return out
}

func currentSkillRoots() ([]string, string) {
  cwd, _ := os.Getwd()
  home, _ := os.UserHomeDir()
  locks := "skills-lock.json"
  if home != "" {
    locks += string(os.PathListSeparator) + filepath.Join(home, ".agents", ".skill-lock.json")
  }
  return skillSearchRoots(cwd, home), locks
}

// preflightSkills avisa (sem bloquear) se alguma skill configurada não está instalada.
func preflightSkills(skills []string) {
  roots, lock := currentSkillRoots()
  missing := missingSkills(skills, roots, lock)
  if len(missing) == 0 {
    return
  }
  msg := fmt.Sprintf("Skills configuradas e não instaladas: %s. Rode `./makercycle -init` para instalá-las.", strings.Join(missing, ", "))
  fmt.Printf("\n[AVISO] %s\n", msg)
  logEvent("AVISO (pre-flight): %s", msg)
}

func runInit(docsDir string, agents []AgentBinding, forceSkills bool) {
  printBanner("MAKERCYCLE -init: AMBIENTE E SKILLS", "Binários, skills (npx skills add) e estrutura de pastas")
  code := 0

  // (d) Estrutura de pastas e log primeiro, para que tudo abaixo fique registrado.
  if err := ensureMakerCycleDirs(); err != nil {
    fatal("Erro ao criar .makercycle/: %v", err)
  }
  if err := ensureDocsDirs(docsDir); err != nil {
    fatal("Erro ao criar estrutura de pastas em %s: %v", docsDir, err)
  }
  startRunLog(docsDir, "init")
  logBlock("CABEÇALHO DA EXECUÇÃO (-init)", fmt.Sprintf("Data/Hora: %s\nDocs: %s/\nForce skills: %v\n", time.Now().Format(time.RFC3339), docsDir, forceSkills))
  fmt.Printf("[OK] Pastas prontas: .makercycle/{agents,config,state}/, %s/demands/, %s/logs/, %s/references/\n", docsDir, docsDir, docsDir)
  logEvent("Pastas prontas: .makercycle/{agents,config,state}/, %s/demands/, %s/logs/, %s/references/", docsDir, docsDir, docsDir)

  // (a) Binários
  names := []string{}
  for _, a := range agents {
    names = append(names, a.Bin)
  }
  names = append(names, "npx", "git")
  fmt.Println("\nBinários:")
  npxOK := false
  for _, b := range checkBinaries(names, exec.LookPath) {
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

  // (b) Skills configuradas
  skills := collectConfiguredSkills(os.Environ())
  if len(skills) == 0 {
    for _, a := range agents {
      if sk := strings.TrimSpace(a.Skill); sk != "" {
        skills = append(skills, sk)
      }
    }
    skills = collectConfiguredSkills(skillEnvFrom(skills))
  }
  fmt.Printf("\nSkills configuradas (%d): %s\n", len(skills), strings.Join(skills, ", "))
  logEvent("Skills configuradas: %s", strings.Join(skills, ", "))

  // (c) Instalação das ausentes (ou de todas, com -force-skills)
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
  for _, r := range ensureSkills(skills, roots, lock, forceSkills, install) {
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

  if code == 0 {
    printBanner("-init CONCLUÍDO", "Ambiente pronto. Use ./makercycle -setup (projeto existente) ou ./makercycle -task \"...\".")
  } else {
    printBanner("-init CONCLUÍDO COM PENDÊNCIAS", "Há binários ausentes ou skills com falha. Corrija e rode ./makercycle -init novamente.")
  }
  finish(code)
}

// skillEnvFrom monta pares MC_n_SKILL=valor para reaproveitar collectConfiguredSkills (deduplicação/ordem).
func skillEnvFrom(skills []string) []string {
  env := make([]string, 0, len(skills))
  for i, sk := range skills {
    env = append(env, fmt.Sprintf("MC_%d_SKILL=%s", i, sk))
  }
  return env
}
