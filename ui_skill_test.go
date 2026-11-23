package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Skill de UI condicional e múltiplas skills (ui_skill.go)
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: UI-*)
// Referência: docs/references/especifica_o_t_cnica_skill_de_ui_condicional.md
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Task 1 — MC_UI_SKILL no template models.env e leitura do ambiente
// ---------------------------------------------------------------------------

// UI-T1-H1: o template embutido declara MC_UI_SKILL="" (vazio por padrão).
func TestModelsEnvDeclaresEmptyUISkill(t *testing.T) {
	content := string(embeddedModelsEnv)
	found := false
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == `MC_UI_SKILL=""` {
			found = true
		}
	}
	if !found {
		t.Fatalf("models.env deve conter a linha exata MC_UI_SKILL=\"\"")
	}
	env := parseEnvContent(content)
	v, ok := env["MC_UI_SKILL"]
	if !ok || v != "" {
		t.Errorf("parseEnvContent(models.env)[MC_UI_SKILL] = %q (ok=%v); want \"\" presente", v, ok)
	}
}

// UI-T1-H2: a sugestão emilkowalski/apple-design aparece só em comentário.
func TestModelsEnvSuggestsAppleDesignInComment(t *testing.T) {
	inComment := false
	for _, line := range strings.Split(string(embeddedModelsEnv), "\n") {
		l := strings.TrimSpace(line)
		if !strings.Contains(l, uiSkillSuggestion) {
			continue
		}
		if strings.HasPrefix(l, "#") {
			inComment = true
		} else {
			t.Errorf("a sugestão %q só pode aparecer em comentário; linha ativa: %q", uiSkillSuggestion, l)
		}
	}
	if !inComment {
		t.Errorf("models.env deve sugerir %q num comentário", uiSkillSuggestion)
	}
}

// UI-T1-E1: constantes públicas da feature.
func TestUISkillConstants(t *testing.T) {
	if uiSkillEnvKey != "MC_UI_SKILL" {
		t.Errorf("uiSkillEnvKey = %q; want MC_UI_SKILL", uiSkillEnvKey)
	}
	if uiSkillSuggestion != "emilkowalski/apple-design" {
		t.Errorf("uiSkillSuggestion = %q", uiSkillSuggestion)
	}
	if uiSkillRequiredMarker != "UI_SKILL: REQUIRED" || uiSkillNoneMarker != "UI_SKILL: NONE" {
		t.Errorf("marcadores = %q / %q", uiSkillRequiredMarker, uiSkillNoneMarker)
	}
	if planFilePath != ".makercycle/state/PLAN.md" {
		t.Errorf("planFilePath = %q", planFilePath)
	}
	if !skillEnvRe.MatchString(uiSkillEnvKey) {
		t.Errorf("skillEnvRe deve casar %q (para o -init instalar a skill de UI)", uiSkillEnvKey)
	}
}

// UI-T1-H3 / UI-T1-E2: uiSkillFromEnv lê só MC_UI_SKILL e apara espaços.
func TestUISkillFromEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"definida", map[string]string{"MC_UI_SKILL": "emilkowalski/apple-design"}, "emilkowalski/apple-design"},
		{"com espaços", map[string]string{"MC_UI_SKILL": "  emilkowalski/apple-design \t"}, "emilkowalski/apple-design"},
		{"vazia", map[string]string{"MC_UI_SKILL": ""}, ""},
		{"só espaços", map[string]string{"MC_UI_SKILL": "   "}, ""},
		{"ausente", map[string]string{"MC_MAKER_SKILL": "x/y"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := func(k string) string { return c.env[k] }
			if got := uiSkillFromEnv(getenv); got != c.want {
				t.Errorf("uiSkillFromEnv = %q; want %q", got, c.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 2 — -init: MC_UI_SKILL recolhida e listas com vírgula
// ---------------------------------------------------------------------------

// UI-T2-H1: MC_UI_SKILL preenchida entra na lista de instalação do -init.
// (Regressão: o regex ^MC_[A-Z0-9]+_SKILL$ já casa; o teste trava o contrato.)
func TestCollectConfiguredSkillsIncludesUISkill(t *testing.T) {
	env := []string{"MC_PLANNER_SKILL=obra/superpowers", "MC_UI_SKILL=emilkowalski/apple-design"}
	got := strings.Join(initSkillsToInstall(env), ",")
	if got != "emilkowalski/apple-design,obra/superpowers" {
		t.Errorf("initSkillsToInstall = %q", got)
	}
}

// UI-T2-E1: MC_UI_SKILL="" (padrão do template) não gera instalação.
func TestCollectConfiguredSkillsIgnoresEmptyUISkill(t *testing.T) {
	if got := collectConfiguredSkills([]string{`MC_UI_SKILL=`}); len(got) != 0 {
		t.Errorf("MC_UI_SKILL vazia não deveria ser instalada: %v", got)
	}
}

// UI-T2-H2: valores com vírgula viram skills separadas, aparadas, únicas e ordenadas.
func TestCollectConfiguredSkillsSplitsCommaLists(t *testing.T) {
	env := []string{
		"MC_MAKER_SKILL=a/b, c/d",
		"MC_UI_SKILL=c/d",
		"MC_GATEKEEPER_SKILL= ,  e/f ,",
		"MC_PLANNER_SKILL=obra/superpowers",
	}
	got := strings.Join(collectConfiguredSkills(env), ",")
	if want := "a/b,c/d,e/f,obra/superpowers"; got != want {
		t.Errorf("collectConfiguredSkills = %q; want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Task 3 — lista de skills: splitSkills / joinSkills / mergeSkills / skillInstruction
// ---------------------------------------------------------------------------

// UI-T3-H1 / UI-T3-E1: splitSkills.
func TestSplitSkills(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"   ":              "",
		",, ,":             "",
		"a/b":              "a/b",
		" a/b ":            "a/b",
		"a/b,c/d":          "a/b|c/d",
		" a/b , c/d ,a/b,": "a/b|c/d",
		"c/d, a/b":         "c/d|a/b", // ordem preservada (não ordena)
	}
	for in, want := range cases {
		if got := strings.Join(splitSkills(in), "|"); got != want {
			t.Errorf("splitSkills(%q) = %q; want %q", in, got, want)
		}
	}
}

// UI-T3-H2: joinSkills usa ", " como separador.
func TestJoinSkills(t *testing.T) {
	if got := joinSkills([]string{"a/b", "c/d"}); got != "a/b, c/d" {
		t.Errorf("joinSkills = %q", got)
	}
	if got := joinSkills([]string{"a/b"}); got != "a/b" {
		t.Errorf("joinSkills única = %q", got)
	}
	if got := joinSkills(nil); got != "" {
		t.Errorf("joinSkills(nil) = %q", got)
	}
}

// UI-T3-H3 / UI-T3-E2: mergeSkills concatena, deduplica e normaliza.
func TestMergeSkills(t *testing.T) {
	cases := []struct {
		base  string
		extra []string
		want  string
	}{
		{"a/b", []string{"c/d"}, "a/b, c/d"},
		{"", []string{"c/d"}, "c/d"},
		{"a/b", []string{""}, "a/b"},
		{"a/b", nil, "a/b"},
		{"a/b, c/d", []string{"c/d"}, "a/b, c/d"},
		{" a/b ,", []string{" e/f ", "g/h, a/b"}, "a/b, e/f, g/h"},
		{"", []string{""}, ""},
	}
	for _, c := range cases {
		if got := mergeSkills(c.base, c.extra...); got != c.want {
			t.Errorf("mergeSkills(%q, %q) = %q; want %q", c.base, c.extra, got, c.want)
		}
	}
}

// UI-T3-H4 / UI-T3-E3 (Spec §3, teste 1): múltiplas skills são formatadas corretamente no prompt.
func TestSkillInstruction(t *testing.T) {
	cases := map[string]string{
		"":                                "",
		"   ":                             "",
		"obra/superpowers":                " aplicando a skill obra/superpowers",
		"a/b, emilkowalski/apple-design":  " aplicando as skills: a/b, emilkowalski/apple-design",
		"a/b,c/d,e/f":                     " aplicando as skills: a/b, c/d, e/f",
		"a/b, a/b":                        " aplicando a skill a/b",
		" , emilkowalski/apple-design , ": " aplicando a skill emilkowalski/apple-design",
	}
	for in, want := range cases {
		if got := skillInstruction(in); got != want {
			t.Errorf("skillInstruction(%q) = %q; want %q", in, got, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 4 — detecção do marcador no PLAN.md
// ---------------------------------------------------------------------------

// UI-T4-*: só a primeira linha não vazia decide; o corpo do plano não interfere.
func TestPlanRequiresUISkill(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    bool
	}{
		{"required na 1ª linha", "UI_SKILL: REQUIRED\n# Plano\n", true},
		{"none na 1ª linha", "UI_SKILL: NONE\n# Plano\n", false},
		{"none com REQUIRED no corpo", "UI_SKILL: NONE\n\nO PLANNER usa `UI_SKILL: REQUIRED` em demandas de UI.\n", false},
		{"required fora da 1ª linha", "# Plano\nUI_SKILL: REQUIRED\n", false},
		{"linhas em branco antes", "\n\n  UI_SKILL: REQUIRED  \n# Plano", true},
		{"BOM e CRLF", "\ufeffUI_SKILL: REQUIRED\r\n# Plano\r\n", true},
		{"só o marcador sem quebra", "UI_SKILL: REQUIRED", true},
		{"vazio", "", false},
		{"só espaços", "  \n\t\n", false},
		{"minúsculas", "ui_skill: required\n", false},
		{"sufixo extra", "UI_SKILL: REQUIRED_LATER\n", false},
		{"sem marcador", "# Plano\n- [ ] Task 1\n", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := planRequiresUISkill(c.content); got != c.want {
				t.Errorf("planRequiresUISkill(%q) = %v; want %v", c.content, got, c.want)
			}
		})
	}
}

// UI-T4-H2 / UI-T4-X1: isUISkillRequiredAt lê o arquivo; erro de leitura vira false.
func TestIsUISkillRequiredAt(t *testing.T) {
	dir := t.TempDir()
	req := filepath.Join(dir, "req.md")
	none := filepath.Join(dir, "none.md")
	if err := os.WriteFile(req, []byte("UI_SKILL: REQUIRED\n# Plano\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(none, []byte("UI_SKILL: NONE\n# Plano\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isUISkillRequiredAt(req) {
		t.Error("REQUIRED deveria retornar true")
	}
	if isUISkillRequiredAt(none) {
		t.Error("NONE deveria retornar false")
	}
	if isUISkillRequiredAt(filepath.Join(dir, "nao-existe.md")) {
		t.Error("arquivo ausente deveria retornar false")
	}
	if isUISkillRequiredAt(dir) {
		t.Error("diretório (erro de leitura) deveria retornar false")
	}
}

// UI-T4-H3: isUISkillRequired() lê .makercycle/state/PLAN.md relativo ao diretório atual.
func TestIsUISkillRequiredReadsStatePlan(t *testing.T) {
	dir := chdirTemp(t)
	if isUISkillRequired() {
		t.Fatal("sem PLAN.md deveria retornar false")
	}
	state := filepath.Join(dir, ".makercycle", "state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(state, "PLAN.md")
	if err := os.WriteFile(plan, []byte("UI_SKILL: REQUIRED\n# Plano\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isUISkillRequired() {
		t.Error("PLAN.md com UI_SKILL: REQUIRED deveria retornar true")
	}
	if err := os.WriteFile(plan, []byte("UI_SKILL: NONE\n# Plano\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isUISkillRequired() {
		t.Error("PLAN.md com UI_SKILL: NONE deveria retornar false")
	}
}

// ---------------------------------------------------------------------------
// Task 5 — decisão de anexar a skill de UI (MAKER e GATEKEEPER)
// ---------------------------------------------------------------------------

// UI-T5-*: tabela-verdade de shouldAppendUISkill.
func TestShouldAppendUISkill(t *testing.T) {
	ui := "emilkowalski/apple-design"
	cases := []struct {
		uiSkill          string
		uiRequired, noUI bool
		want             bool
	}{
		{ui, true, false, true},
		{ui, true, true, false},
		{ui, false, false, false},
		{ui, false, true, false},
		{"", true, false, false},
		{"   ", true, false, false},
		{"", false, false, false},
		{"", true, true, false},
	}
	for _, c := range cases {
		if got := shouldAppendUISkill(c.uiSkill, c.uiRequired, c.noUI); got != c.want {
			t.Errorf("shouldAppendUISkill(%q, %v, %v) = %v; want %v", c.uiSkill, c.uiRequired, c.noUI, got, c.want)
		}
	}
}

// UI-T5-*: stageSkillWithUI anexa a skill de UI à(s) skill(s) da etapa.
func TestStageSkillWithUI(t *testing.T) {
	ui := "emilkowalski/apple-design"
	cases := []struct {
		name             string
		base, uiSkill    string
		uiRequired, noUI bool
		want             string
	}{
		{"anexa a skill existente", "x/maker", ui, true, false, "x/maker, " + ui},
		{"etapa sem skill", "", ui, true, false, ui},
		{"várias skills na etapa", "x/a, x/b", ui, true, false, "x/a, x/b, " + ui},
		{"-no-ui ignora o plano", "x/maker", ui, true, true, "x/maker"},
		{"plano NONE", "x/maker", ui, false, false, "x/maker"},
		{"MC_UI_SKILL vazia", "x/maker", "", true, false, "x/maker"},
		{"sem duplicar", ui, ui, true, false, ui},
		{"normaliza a base", " x/a ,, x/a ", "", false, false, "x/a"},
		{"tudo vazio", "", "", true, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := stageSkillWithUI(c.base, c.uiSkill, c.uiRequired, c.noUI); got != c.want {
				t.Errorf("stageSkillWithUI(%q, %q, %v, %v) = %q; want %q", c.base, c.uiSkill, c.uiRequired, c.noUI, got, c.want)
			}
		})
	}
}

// UI-T5-I1 (Spec §3, teste 2): com PLAN.md NONE ou com -no-ui, a skill de UI não chega ao prompt.
func TestUISkillNotInjectedWithNoneOrNoUIFlag(t *testing.T) {
	ui := "emilkowalski/apple-design"
	dir := chdirTemp(t)
	state := filepath.Join(dir, ".makercycle", "state")
	if err := os.MkdirAll(state, 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(state, "PLAN.md")
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(plan, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("UI_SKILL: NONE\n# Plano backend\n")
	mk := buildMakerPrompt(stageSkillWithUI("", ui, isUISkillRequired(), false), "docs", 1)
	gk := buildGatekeeperPrompt(stageSkillWithUI("", ui, isUISkillRequired(), false), 1, 3)
	if strings.Contains(mk, ui) || strings.Contains(gk, ui) {
		t.Errorf("PLAN NONE não pode injetar %q:\nMAKER: %s\nGATEKEEPER: %s", ui, mk, gk)
	}

	write("UI_SKILL: REQUIRED\n# Plano frontend\n")
	mk = buildMakerPrompt(stageSkillWithUI("", ui, isUISkillRequired(), true), "docs", 1)
	gk = buildGatekeeperPrompt(stageSkillWithUI("", ui, isUISkillRequired(), true), 1, 3)
	if strings.Contains(mk, ui) || strings.Contains(gk, ui) {
		t.Errorf("-no-ui não pode injetar %q:\nMAKER: %s\nGATEKEEPER: %s", ui, mk, gk)
	}

	mk = buildMakerPrompt(stageSkillWithUI("x/maker", ui, isUISkillRequired(), false), "docs", 1)
	if !strings.Contains(mk, "03_MAKER.md aplicando as skills: x/maker, "+ui+". ") {
		t.Errorf("PLAN REQUIRED sem -no-ui deveria anexar %q ao MAKER:\n%s", ui, mk)
	}
	gk = buildGatekeeperPrompt(stageSkillWithUI("", ui, isUISkillRequired(), false), 1, 3)
	if !strings.Contains(gk, "05_GATEKEEPER.md aplicando a skill "+ui+". ") {
		t.Errorf("PLAN REQUIRED sem -no-ui deveria anexar %q ao GATEKEEPER:\n%s", ui, gk)
	}
}
