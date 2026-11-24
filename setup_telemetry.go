package main

import (
	"errors"
	"strings"
	"time"
)

// Literais de contrato da telemetria da Etapa 0 (-setup).
const (
	setupStageCode      = "S0"
	setupAgentName      = "SCOUT"
	setupDemandLabel    = "(setup / SCAN)"
	setupRunStatusOK    = "SETUP_OK"
	setupRunStatusError = "ERROR"
	scoutEventSuccess   = "SUCCESS"
	scoutEventError     = "ERROR"
	scoutEventNotRun    = "NOT_RUN"
	setupStackUnknown   = "(não detectada)"
)

// setupStackLabel escolhe a stack registrada na telemetria: a salva em project.env
// (o SCOUT pode ajustá-la) > a detectada/confirmada > "(não detectada)".
func setupStackLabel(detectedName string, saved StackConfig, loadErr error) string {
	if loadErr == nil && strings.TrimSpace(saved.Name) != "" {
		return saved.Name
	}
	if strings.TrimSpace(detectedName) != "" {
		return detectedName
	}
	return setupStackUnknown
}

// setupRunNote monta a Note de runs.csv: status do SCOUT, duração total e erro (em uma linha).
func setupRunNote(scoutStatus string, totalSeconds float64, err error) string {
	if scoutStatus == "" {
		scoutStatus = scoutEventNotRun
	}
	note := "SCOUT: " + scoutStatus + " | Duração total: " + formatTelemetryDuration(totalSeconds)
	if err != nil {
		msg := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(err.Error())
		note += " | Erro: " + msg
	}
	return note
}

// setupTelemetry encapsula a contabilidade da Etapa 0: o evento S0 (SCOUT) e a execução em runs.csv.
type setupTelemetry struct {
	RunID       string           // newRunID()
	Model       string           // modelo do SCOUT (scout.Model)
	now         func() time.Time // relógio injetável
	start       time.Time        // now() na criação
	tokens      TokenStats       // soma dos tokens do SCOUT
	scoutStatus string           // "", scoutEventSuccess ou scoutEventError
	finished    bool
	status      string // status gravado em runs.csv
}

// newSetupTelemetry cria a contabilidade da Etapa 0 sem gravar nada em disco.
func newSetupTelemetry(model string, now func() time.Time) *setupTelemetry {
	if now == nil {
		now = time.Now
	}
	return &setupTelemetry{RunID: newRunID(), Model: model, now: now, start: now()}
}

// RecordScout grava o evento S0 (também em erro, pois os tokens consumidos contam) e devolve o status do SCOUT.
func (s *setupTelemetry) RecordScout(tokens TokenStats, durationSeconds float64, err error) string {
	if durationSeconds < 0 {
		durationSeconds = 0
	}
	s.scoutStatus = scoutEventSuccess
	if err != nil {
		s.scoutStatus = scoutEventError
	}
	s.tokens.Add(tokens)
	recordEventTelemetry(s.RunID, setupStageCode, setupAgentName, s.Model, 1, tokens, durationSeconds)
	return s.scoutStatus
}

// Finish grava a execução em runs.csv (regenera o TELEMETRY.md). É idempotente.
func (s *setupTelemetry) Finish(stack string, setupErr error) string {
	if s.finished {
		return s.status
	}
	status := setupRunStatusOK
	if errors.Is(setupErr, errManualAborted) {
		status = manualAbortStatus
	} else if setupErr != nil {
		status = setupRunStatusError
	}
	note := setupRunNote(s.scoutStatus, s.now().Sub(s.start).Seconds(), setupErr)
	recordRunTelemetry(s.RunID, stack, setupDemandLabel, 1, 1, s.tokens, status, note)
	s.finished = true
	s.status = status
	return status
}
