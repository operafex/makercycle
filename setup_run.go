package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// scoutRunner abstrai runScoutAgent para testes.
type scoutRunner func(scout AgentBinding, docsDir, existingDocs, detectedName string, jsonMetrics bool) (TokenStats, error)

// setupOutcome resume uma execução do -setup.
type setupOutcome struct {
	RunID   string // RunID gravado em events.csv/runs.csv
	Status  string // setupRunStatusOK ou setupRunStatusError
	Stack   string // stack registrada na telemetria
	DocsDir string // pasta de documentação resolvida ("" se o erro veio antes)
}

// runProjectSetupWith executa o -setup (Etapa 0 / SCAN) registrando telemetria em sucesso e erro.
// Devolve error em vez de chamar fatal para que a execução seja gravada antes de qualquer os.Exit.
func runProjectSetupWith(reader *bufio.Reader, scout AgentBinding, defaultDocs string, jsonMetrics bool,
	runScout scoutRunner, now func() time.Time) (out setupOutcome, err error) {
	if now == nil {
		now = time.Now
	}
	tel := newSetupTelemetry(scout.Model, now)
	out = setupOutcome{RunID: tel.RunID, Stack: setupStackUnknown}
	defer func() { out.Status = tel.Finish(out.Stack, err) }()

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
	out.DocsDir = targetDocsDir

	if e := ensureMakerCycleDirs(); e != nil {
		return out, fmt.Errorf("Erro ao criar estrutura .makercycle/: %v", e)
	}
	if e := ensureDocsDirs(targetDocsDir); e != nil {
		return out, fmt.Errorf("Erro ao criar estrutura de pastas em %s: %v", targetDocsDir, e)
	}
	startRunLog(targetDocsDir, "stage0")
	logBlock("CABEÇALHO DA EXECUÇÃO", buildRunHeader(StackConfig{Name: "(n/a — Etapa 0 Scan)"}, []AgentBinding{scout},
		"", "", 0, 0, targetDocsDir))
	logEvent("RunID da Etapa 0: %s", tel.RunID)

	detected, ok, envErr := confirmSetupStack(reader, os.Stdout, projectEnvPath, ".", targetDocsDir, existingDocs)
	if envErr != nil {
		return out, fmt.Errorf("Erro ao gravar %s: %v", projectEnvPath, envErr)
	}
	detectedName := ""
	if ok {
		detectedName = detected.Name
		out.Stack = detected.Name
		logEvent("STACK CONFIRMADA: %s (testes: %s)", detected.Name, detected.TestCmd)
	} else {
		fmt.Println("[AVISO] Stack não detectada automaticamente. O SCOUT tentará identificá-la.")
		logEvent("AVISO: stack não detectada automaticamente")
	}

	scoutStart := now()
	stats, scoutErr := runScout(scout, targetDocsDir, existingDocs, detectedName, jsonMetrics)
	tel.RecordScout(stats, now().Sub(scoutStart).Seconds(), scoutErr)
	if scoutErr != nil {
		return out, scoutErr
	}

	savedStack, loadErr := loadProjectStack(projectEnvPath)
	out.Stack = setupStackLabel(detectedName, savedStack, loadErr)
	bannerMsg := fmt.Sprintf("Contexto salvo em /%s/PROJECT_CONTEXT.md.", targetDocsDir)
	if loadErr == nil {
		bannerMsg += fmt.Sprintf("\nStack salva: %s", savedStack.Name)
	} else {
		fmt.Printf("[AVISO] Stack não configurada em project.env (motivo: %v). O menu manual será exibido no -task.\n", loadErr)
		logEvent("AVISO: stack não configurada em project.env: %v", loadErr)
	}
	bannerMsg += "\nDispare novas demandas com: makercycle -task \"...\""
	printBanner("ETAPA 0 CONCLUÍDA", bannerMsg)
	return out, nil
}
