package main

import (
	"fmt"
	"os"
	"strings"
)

// Skill de UI condicional e suporte a várias skills por etapa.
// Referência: docs/references/especifica_o_t_cnica_skill_de_ui_condicional.md

const (
	uiSkillEnvKey         = "MC_UI_SKILL"
	uiSkillSuggestion     = "emilkowalski/apple-design"
	uiSkillRequiredMarker = "UI_SKILL: REQUIRED"
	uiSkillNoneMarker     = "UI_SKILL: NONE"
	planFilePath          = ".makercycle/state/PLAN.md"
)

// makerRetryInstruction é concatenada ao prompt do MAKER a partir da 2ª tentativa.
const makerRetryInstruction = " ATENÇÃO: Rodada de correção. Leia @.makercycle/state/DECISION.md, @.makercycle/state/VERIFY_REPORT.md e @.makercycle/state/SECURITY_REPORT.md e corrija todos os apontamentos."

// uiSkillFromEnv devolve MC_UI_SKILL aparada ("" quando ausente ou só espaços).
func uiSkillFromEnv(getenv func(string) string) string {
	return strings.TrimSpace(getenv(uiSkillEnvKey))
}

// splitSkills separa por ",", apara, descarta vazios e duplicatas, preservando a ordem.
func splitSkills(s string) []string {
	seen := map[string]bool{}
	var skills []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		skills = append(skills, item)
	}
	return skills
}

// joinSkills junta com ", " (nil/vazio → "").
func joinSkills(skills []string) string {
	return strings.Join(skills, ", ")
}

// mergeSkills concatena base e extras (cada um pode ser lista com vírgula), normaliza e deduplica.
func mergeSkills(base string, extra ...string) string {
	parts := append([]string{base}, extra...)
	return joinSkills(splitSkills(strings.Join(parts, ",")))
}

// skillInstruction devolve "" | " aplicando a skill X" | " aplicando as skills: X, Y" (com espaço inicial).
func skillInstruction(skill string) string {
	skills := splitSkills(skill)
	switch len(skills) {
	case 0:
		return ""
	case 1:
		return " aplicando a skill " + skills[0]
	default:
		return " aplicando as skills: " + joinSkills(skills)
	}
}

// planRequiresUISkill é true só se a 1ª linha não vazia (sem BOM inicial, TrimSpace) for o marcador REQUIRED.
func planRequiresUISkill(content string) bool {
	content = strings.TrimPrefix(content, "\uFEFF")
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return line == uiSkillRequiredMarker
	}
	return false
}

// isUISkillRequiredAt lê path; qualquer erro (inexistente, diretório) → false.
func isUISkillRequiredAt(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return planRequiresUISkill(string(data))
}

// isUISkillRequired verifica .makercycle/state/PLAN.md relativo ao diretório atual.
func isUISkillRequired() bool {
	return isUISkillRequiredAt(planFilePath)
}

// shouldAppendUISkill decide se a skill de UI entra: preenchida, exigida pelo plano e sem -no-ui.
func shouldAppendUISkill(uiSkill string, uiRequired, noUI bool) bool {
	return strings.TrimSpace(uiSkill) != "" && uiRequired && !noUI
}

// stageSkillWithUI devolve as skills da etapa, com a skill de UI anexada quando aplicável.
func stageSkillWithUI(base, uiSkill string, uiRequired, noUI bool) string {
	if shouldAppendUISkill(uiSkill, uiRequired, noUI) {
		return mergeSkills(base, uiSkill)
	}
	return mergeSkills(base)
}

// buildMakerPrompt monta o prompt da Etapa 3. A partir da 2ª tentativa, acrescenta a instrução de correção.
func buildMakerPrompt(skill, docsDir string, attempt int) string {
	prompt := fmt.Sprintf(
		"Siga estritamente as instruções de @.makercycle/agents/03_MAKER.md%s. Diretório de docs: %s/. "+
			"Leia @.makercycle/state/PLAN.md e @.makercycle/state/TEST_CASES.md e implemente as Tasks no código de produção.",
		skillInstruction(skill), docsDir,
	)
	if attempt > 1 {
		prompt += makerRetryInstruction
	}
	return prompt
}

// buildGatekeeperPrompt monta o prompt da Etapa 5.
func buildGatekeeperPrompt(skill string, attempt, maxAttempts int) string {
	return fmt.Sprintf(
		"Siga estritamente as instruções de @.makercycle/agents/05_GATEKEEPER.md%s. "+
			"Esta é a tentativa ATTEMPT: %d de %d. "+
			"Analise @.makercycle/state/PLAN.md, @.makercycle/state/TEST_CASES.md, @.makercycle/state/VERIFY_REPORT.md, @.makercycle/state/SECURITY_REPORT.md e execute git diff. "+
			"Grave seu veredito em .makercycle/state/DECISION.md.",
		skillInstruction(skill), attempt, maxAttempts,
	)
}

// buildAuditorPrompt monta o prompt da Etapa 4B.
func buildAuditorPrompt(skill string) string {
	return fmt.Sprintf(
		"Siga estritamente as instruções de @.makercycle/agents/04_AUDITOR.md%s. "+
			"Audite o git diff atual e grave o relatório em .makercycle/state/SECURITY_REPORT.md.",
		skillInstruction(skill),
	)
}
