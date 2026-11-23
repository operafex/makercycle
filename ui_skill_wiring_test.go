package main

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Skill de UI condicional — prompts, fiação no main e contratos dos agentes
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: UI-T6..UI-T8)
// ---------------------------------------------------------------------------

const (
	legacyMakerPrompt = "Siga estritamente as instruções de @.makercycle/agents/03_MAKER.md. Diretório de docs: docs/. " +
		"Leia @.makercycle/state/PLAN.md e @.makercycle/state/TEST_CASES.md e implemente as Tasks no código de produção."
	legacyMakerRetry = " ATENÇÃO: Rodada de correção. Leia @.makercycle/state/DECISION.md, @.makercycle/state/VERIFY_REPORT.md " +
		"e @.makercycle/state/SECURITY_REPORT.md e corrija todos os apontamentos."
	gatekeeperBody = "Esta é a tentativa ATTEMPT: 2 de 3. " +
		"Analise @.makercycle/state/PLAN.md, @.makercycle/state/TEST_CASES.md, @.makercycle/state/VERIFY_REPORT.md, @.makercycle/state/SECURITY_REPORT.md e execute git diff. " +
		"Grave seu veredito em .makercycle/state/DECISION.md."
)

// ---------------------------------------------------------------------------
// Task 6 — builders de prompt (MAKER, GATEKEEPER, AUDITOR, PLANNER, CRITIC)
// ---------------------------------------------------------------------------

// UI-T6-H1: sem skill, o prompt do MAKER é idêntico ao texto legado.
func TestBuildMakerPromptWithoutSkillKeepsLegacyText(t *testing.T) {
	if got := buildMakerPrompt("", "docs", 1); got != legacyMakerPrompt {
		t.Errorf("buildMakerPrompt sem skill:\n got: %q\nwant: %q", got, legacyMakerPrompt)
	}
}

// UI-T6-H2: rodada de correção (attempt > 1) acrescenta a instrução legada.
func TestBuildMakerPromptRetry(t *testing.T) {
	if got := buildMakerPrompt("", "docs", 2); got != legacyMakerPrompt+legacyMakerRetry {
		t.Errorf("buildMakerPrompt attempt 2:\n got: %q\nwant: %q", got, legacyMakerPrompt+legacyMakerRetry)
	}
	if got := buildMakerPrompt("", "docs", 0); got != legacyMakerPrompt {
		t.Errorf("attempt 0 não é rodada de correção: %q", got)
	}
}

// UI-T6-H3: com uma ou várias skills, a instrução entra logo após o contrato.
func TestBuildMakerPromptWithSkills(t *testing.T) {
	one := "Siga estritamente as instruções de @.makercycle/agents/03_MAKER.md aplicando a skill emilkowalski/apple-design. Diretório de docs: web/. " +
		"Leia @.makercycle/state/PLAN.md e @.makercycle/state/TEST_CASES.md e implemente as Tasks no código de produção."
	if got := buildMakerPrompt("emilkowalski/apple-design", "web", 1); got != one {
		t.Errorf("uma skill:\n got: %q\nwant: %q", got, one)
	}
	many := "Siga estritamente as instruções de @.makercycle/agents/03_MAKER.md aplicando as skills: x/maker, emilkowalski/apple-design. Diretório de docs: docs/. " +
		"Leia @.makercycle/state/PLAN.md e @.makercycle/state/TEST_CASES.md e implemente as Tasks no código de produção." + legacyMakerRetry
	if got := buildMakerPrompt("x/maker, emilkowalski/apple-design", "docs", 3); got != many {
		t.Errorf("várias skills:\n got: %q\nwant: %q", got, many)
	}
}

// UI-T6-H4: GATEKEEPER sem skill mantém o texto legado; com skills, usa skillInstruction.
func TestBuildGatekeeperPrompt(t *testing.T) {
	legacy := "Siga estritamente as instruções de @.makercycle/agents/05_GATEKEEPER.md. " + gatekeeperBody
	if got := buildGatekeeperPrompt("", 2, 3); got != legacy {
		t.Errorf("sem skill:\n got: %q\nwant: %q", got, legacy)
	}
	withUI := "Siga estritamente as instruções de @.makercycle/agents/05_GATEKEEPER.md aplicando as skills: x/gate, emilkowalski/apple-design. " + gatekeeperBody
	if got := buildGatekeeperPrompt("x/gate, emilkowalski/apple-design", 2, 3); got != withUI {
		t.Errorf("com skills:\n got: %q\nwant: %q", got, withUI)
	}
}

// UI-T6-H5: AUDITOR extraído do main, com o texto legado para uma skill.
func TestBuildAuditorPrompt(t *testing.T) {
	want := "Siga estritamente as instruções de @.makercycle/agents/04_AUDITOR.md aplicando a skill cloudflare/security-audit-skill. " +
		"Audite o git diff atual e grave o relatório em .makercycle/state/SECURITY_REPORT.md."
	if got := buildAuditorPrompt("cloudflare/security-audit-skill"); got != want {
		t.Errorf("buildAuditorPrompt:\n got: %q\nwant: %q", got, want)
	}
	if got := buildAuditorPrompt("a/b,c/d"); !strings.HasPrefix(got, "Siga estritamente as instruções de @.makercycle/agents/04_AUDITOR.md aplicando as skills: a/b, c/d. Audite") {
		t.Errorf("várias skills no AUDITOR: %q", got)
	}
}

// UI-T6-H6: PLANNER e CRITIC aceitam várias skills (uma skill continua idêntica — ver spec_loop_test.go).
func TestPlannerAndCriticPromptsAcceptMultipleSkills(t *testing.T) {
	p := buildPlannerPrompt("obra/superpowers, x/y", "docs", "Go", "t", "d.md", nil, 1)
	if !strings.HasPrefix(p, "Siga estritamente as instruções de @.makercycle/agents/01_PLANNER.md aplicando as skills: obra/superpowers, x/y. Diretório de documentação: docs/.") {
		t.Errorf("PLANNER com várias skills: %q", p)
	}
	c := buildCriticPrompt("google/elephant-goldfish-model,x/y", "docs")
	if !strings.HasPrefix(c, "Siga estritamente as instruções de @.makercycle/agents/02_CRITIC.md aplicando as skills: google/elephant-goldfish-model, x/y. Diretório de documentação: docs/.") {
		t.Errorf("CRITIC com várias skills: %q", c)
	}
	if p := buildPlannerPrompt("", "docs", "Go", "t", "d.md", nil, 1); !strings.HasPrefix(p, "Siga estritamente as instruções de @.makercycle/agents/01_PLANNER.md. Diretório") {
		t.Errorf("PLANNER sem skill não deve conter \"aplicando a skill .\": %q", p)
	}
}

// ---------------------------------------------------------------------------
// Task 7 — fiação em makercycle.go (flags e uso dos builders)
// ---------------------------------------------------------------------------

// UI-T7-H1: o main declara as flags e usa a decisão condicional nas Etapas 3 e 5.
func TestMainWiresConditionalUISkill(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	for _, want := range []string{
		`flag.Bool("no-ui", false,`,
		`flag.String("ui-skill", uiSkillFromEnv(os.Getenv),`,
		`flag.String("maker-skill", getEnv("MC_MAKER_SKILL", ""),`,
		`flag.String("gatekeeper-skill", getEnv("MC_GATEKEEPER_SKILL", ""),`,
		"isUISkillRequired()",
		"stageSkillWithUI(maker.Skill,",
		"stageSkillWithUI(gatekeeper.Skill,",
		"buildMakerPrompt(",
		"buildGatekeeperPrompt(",
		"buildAuditorPrompt(auditor.Skill)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("makercycle.go deve conter %q", want)
		}
	}
	for _, legacy := range []string{
		`"Siga estritamente as instruções de @.makercycle/agents/03_MAKER.md. Diretório de docs`,
		`"Siga estritamente as instruções de @.makercycle/agents/05_GATEKEEPER.md. "`,
		`"Siga estritamente as instruções de @.makercycle/agents/04_AUDITOR.md aplicando a skill %s. "`,
	} {
		if strings.Contains(src, legacy) {
			t.Errorf("makercycle.go ainda monta o prompt inline: %q", legacy)
		}
	}
}

// UI-T7-H2 (E2E): -h lista as novas flags.
func TestE2EHelpListsUISkillFlags(t *testing.T) {
	bin := buildE2EBinary(t)
	r := runE2E(t, bin, t.TempDir(), t.TempDir(), "", "-h")
	for _, f := range []string{"-no-ui", "-ui-skill", "-maker-skill", "-gatekeeper-skill"} {
		if !strings.Contains(r.Out, f) {
			t.Errorf("-h deveria listar %s:\n%s", f, r.Out)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 8 — contratos 01_PLANNER.md / CLAUDE.md / 02_CRITIC.md
// ---------------------------------------------------------------------------

// UI-T8-H1: o PLANNER manda gravar o marcador como primeira linha do PLAN.md.
func TestPlannerContractRequiresUISkillMarker(t *testing.T) {
	c := readRepoFile(t, ".makercycle/agents/01_PLANNER.md")
	for _, want := range []string{"UI_SKILL: REQUIRED", "UI_SKILL: NONE", "primeira linha", ".makercycle/state/PLAN.md", "backend", "interface"} {
		if !strings.Contains(c, want) {
			t.Errorf("01_PLANNER.md deve conter %q", want)
		}
	}
}

// UI-T8-H2: CLAUDE.md continua espelho exato de 01_PLANNER.md.
func TestClaudeMdMirrorsPlannerContract(t *testing.T) {
	if readRepoFile(t, "CLAUDE.md") != readRepoFile(t, ".makercycle/agents/01_PLANNER.md") {
		t.Error("CLAUDE.md deve ser idêntico a .makercycle/agents/01_PLANNER.md")
	}
}

// UI-T8-H3: o CRITIC audita o marcador e reprova classificação errada.
func TestCriticContractAuditsUISkillMarker(t *testing.T) {
	c := readRepoFile(t, ".makercycle/agents/02_CRITIC.md")
	for _, want := range []string{"UI_SKILL: REQUIRED", "UI_SKILL: NONE", "SPEC_STATUS: REVISION_REQUIRED", "primeira linha", "backend", "frontend"} {
		if !strings.Contains(c, want) {
			t.Errorf("02_CRITIC.md deve conter %q", want)
		}
	}
}
