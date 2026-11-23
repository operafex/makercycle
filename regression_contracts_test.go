package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Tasks 3, 4 e 5: fiação do carregamento em camadas, contratos anti-regressão,
// .gitignore/secrets.env do próprio repositório e READMEs.
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: WR-*, AR-*, RH-*)
// ---------------------------------------------------------------------------

// WR-H1: o main carrega as três camadas (e não mais models.env/project.env isolados) e avisa sobre chaves no project.env.
func TestMainWiresEnvLayers(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	for _, s := range []string{
		"loadEnvLayers(envLayerPaths)",
		"projectEnvKeyWarning(apiKeysInModelsEnv(",
	} {
		if !strings.Contains(src, s) {
			t.Errorf("makercycle.go deve conter %q", s)
		}
	}
	for _, s := range []string{
		`loadEnvFile(".makercycle/config/project.env")`,
		"loadEnvFile(modelsEnvPath)",
	} {
		if strings.Contains(src, s) {
			t.Errorf("makercycle.go não deve mais conter %q (substituído por loadEnvLayers)", s)
		}
	}
	main := src[strings.Index(src, "func main() {"):]
	iLoad := strings.Index(main, "loadEnvLayers(envLayerPaths)")
	iFlag := strings.Index(main, "flag.Bool(")
	if iLoad < 0 || iFlag < 0 || iLoad > iFlag {
		t.Error("loadEnvLayers deve rodar no início do main, antes da declaração das flags")
	}
}

// WR-H2: o -init herda o secrets.env da home e recarrega as camadas após o provisionamento.
func TestInitWiresSecretsEnv(t *testing.T) {
	src := readRepoFile(t, "init_run.go")
	for _, s := range []string{
		"globalSecretsEnvPath(home)",
		"GlobalSecretsEnv:",
		"loadEnvLayers(envLayerPaths)",
	} {
		if !strings.Contains(src, s) {
			t.Errorf("init_run.go deve conter %q", s)
		}
	}
	if strings.Contains(src, "loadEnvFile(modelsEnvPath)") {
		t.Error("init_run.go deve usar loadEnvLayers(envLayerPaths) no lugar de loadEnvFile(modelsEnvPath)")
	}
	iProv := strings.Index(src, "provisionInitAssets(")
	iLoad := strings.Index(src, "loadEnvLayers(envLayerPaths)")
	iAuth := strings.Index(src, "else if authErr := initAuthError()")
	if iProv < 0 || iLoad < iProv || iAuth < iLoad {
		t.Error("ordem esperada em runInit: provisionInitAssets -> loadEnvLayers -> initAuthError")
	}
}

// ---------------------------------------------------------------------------
// Contratos anti-regressão (.makercycle/agents/*.md, embutidos no binário)
// ---------------------------------------------------------------------------

func embeddedAgent(t *testing.T, name string) string {
	t.Helper()
	b, err := embeddedAgentsFS.ReadFile(embeddedAgentsDir + "/" + name)
	if err != nil {
		t.Fatalf("lendo %s embutido: %v", name, err)
	}
	return string(b)
}

func assertContainsFold(t *testing.T, file, content string, needles ...string) {
	t.Helper()
	low := strings.ToLower(content)
	for _, n := range needles {
		if !strings.Contains(low, strings.ToLower(n)) {
			t.Errorf("%s deve conter %q", file, n)
		}
	}
}

// AR-H1: o PLANNER exige a seção "Impacto de Regressão" no PLAN.md mapeando módulos existentes afetados.
func TestPlannerContractRequiresRegressionImpact(t *testing.T) {
	c := embeddedAgent(t, "01_PLANNER.md")
	assertContainsFold(t, "01_PLANNER.md", c, "## Impacto de Regressão", "módulos existentes")
	if !strings.Contains(c, "Impacto de Regressão") {
		t.Error("01_PLANNER.md deve citar a seção com acento exato: \"Impacto de Regressão\"")
	}
}

// AR-H2: o CRITIC cobra a seção e a cobertura de testes das áreas afetadas (senão REVISION_REQUIRED).
func TestCriticContractEnforcesRegressionCoverage(t *testing.T) {
	c := embeddedAgent(t, "02_CRITIC.md")
	assertContainsFold(t, "02_CRITIC.md", c, "Impacto de Regressão", "cobertura de testes")
	i := strings.Index(c, "Impacto de Regressão")
	if i < 0 || !strings.Contains(c[i:], "SPEC_STATUS: REVISION_REQUIRED") {
		t.Error("02_CRITIC.md deve exigir SPEC_STATUS: REVISION_REQUIRED quando a seção/cobertura faltar (após a regra de Impacto de Regressão)")
	}
}

// AR-H3: AUDITOR e GATEKEEPER proíbem afrouxar, remover ou contornar testes regressivos pré-existentes.
func TestAuditorAndGatekeeperForbidRegressionBypass(t *testing.T) {
	for _, n := range []string{"04_AUDITOR.md", "05_GATEKEEPER.md"} {
		assertContainsFold(t, n, embeddedAgent(t, n),
			"PROIBIDO",
			"testes regressivos pré-existentes",
			"afrouxamento",
			"remoção",
			"bypass",
			"justificativa arquitetural",
		)
	}
}

// AR-H4: o GATEKEEPER roteia a violação para uma decisão que bloqueia o merge.
func TestGatekeeperRoutesRegressionBypassToBlockingDecision(t *testing.T) {
	c := embeddedAgent(t, "05_GATEKEEPER.md")
	i := strings.Index(strings.ToLower(c), "testes regressivos pré-existentes")
	if i < 0 {
		t.Fatal("05_GATEKEEPER.md sem a regra de testes regressivos")
	}
	if !strings.Contains(c[i:], "RETRY_EXECUTION") && !strings.Contains(c[i:], "ESCALATE_PLANNER") {
		t.Error("a regra anti-regressão do GATEKEEPER deve indicar DECISION: RETRY_EXECUTION ou ESCALATE_PLANNER")
	}
}

// AR-H5: o AUDITOR registra a violação como achado no SECURITY_REPORT (SEC_STATUS: VULNERABILITY_FOUND).
func TestAuditorRoutesRegressionBypassToFinding(t *testing.T) {
	c := embeddedAgent(t, "04_AUDITOR.md")
	i := strings.Index(strings.ToLower(c), "testes regressivos pré-existentes")
	if i < 0 || !strings.Contains(c[i:], "VULNERABILITY_FOUND") {
		t.Error("a regra anti-regressão do AUDITOR deve levar a SEC_STATUS: VULNERABILITY_FOUND")
	}
}

// AR-E1: os contratos preservam os vereditos lidos pelo Go (strings.Contains) — nada pode sumir.
func TestAgentContractsKeepVerdictLiterals(t *testing.T) {
	cases := map[string][]string{
		"01_PLANNER.md":    {"UI_SKILL: REQUIRED", "UI_SKILL: NONE", "MC_1_DONE"},
		"02_CRITIC.md":     {"SPEC_STATUS: APPROVED", "SPEC_STATUS: REVISION_REQUIRED", "UI_SKILL: REQUIRED"},
		"04_AUDITOR.md":    {"SEC_STATUS: PASS", "SEC_STATUS: VULNERABILITY_FOUND", ".makercycle/state/SECURITY_REPORT.md"},
		"05_GATEKEEPER.md": {"DECISION: APPROVE_MERGE", "DECISION: RETRY_EXECUTION", "DECISION: AMEND_TEST", "DECISION: ESCALATE_PLANNER", "ATTEMPT:"},
	}
	for n, needles := range cases {
		c := embeddedAgent(t, n)
		for _, s := range needles {
			if !strings.Contains(c, s) {
				t.Errorf("%s perdeu o literal %q", n, s)
			}
		}
	}
}

// AR-E2: o espelho CLAUDE.md == 01_PLANNER.md já é coberto por TestClaudeMdMirrorsPlannerContract (ui_skill_wiring_test.go).

// ---------------------------------------------------------------------------
// Higiene do repositório: project.env passa a ser versionável.
// ---------------------------------------------------------------------------

// RH-H1: o .gitignore do repositório ignora secrets.env e NÃO ignora mais project.env.
func TestRepoGitignoreVersionsProjectEnv(t *testing.T) {
	present := map[string]bool{}
	for _, line := range strings.Split(readRepoFile(t, ".gitignore"), "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "!") {
			continue
		}
		present[normalizeGitignoreEntry(l)] = true
	}
	if !present[".makercycle/config/secrets.env"] {
		t.Error(".gitignore deve ignorar .makercycle/config/secrets.env")
	}
	for _, banned := range []string{".makercycle/config/project.env", ".makercycle/config", ".makercycle"} {
		if present[banned] {
			t.Errorf(".gitignore não pode ignorar %q (project.env deve ser versionável)", banned)
		}
	}
}

var (
	providerKeyLine = regexp.MustCompile(`(?m)^[#\s]*(export\s+)?(ANTHROPIC_API_KEY|OPENAI_API_KEY|GEMINI_API_KEY|MC_API_KEY|LLM_API_KEY)\s*=\s*["']?[^"'\s]+`)
	secretLiteral   = regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]{10,}|sk-proj-[A-Za-z0-9_-]{10,}|sk-[A-Za-z0-9]{32,}|AIza[0-9A-Za-z_-]{30,}`)
)

// RH-X1: um project.env presente no repositório não contém chaves (nem em linhas comentadas) — ele é versionado.
func TestRepoProjectEnvHasNoSecrets(t *testing.T) {
	b, err := os.ReadFile(projectEnvPath)
	if err != nil {
		t.Skipf("project.env ausente neste checkout: %v", err)
	}
	c := string(b)
	if m := providerKeyLine.FindAllStringSubmatch(c, -1); len(m) > 0 {
		var names []string
		for _, x := range m {
			names = append(names, x[2]) // só o nome: nunca imprima o valor
		}
		t.Errorf("project.env (versionado) declara chaves de API: %v — mova-as para %s", names, secretsEnvPath)
	}
	if secretLiteral.MatchString(c) {
		t.Error("project.env contém um literal com formato de chave de API — mova-o para secrets.env")
	}
}

// RH-X2: os templates versionados/embutidos também não carregam literais de chave.
func TestVersionedConfigHasNoSecretLiterals(t *testing.T) {
	for _, f := range []string{".makercycle/config/models.env", ".makercycle/config/another_model.env"} {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if secretLiteral.Match(b) || providerKeyLine.Match(b) {
			t.Errorf("%s (versionado) contém chave de API", f)
		}
	}
}

// RH-H2: READMEs documentam secrets.env, a herança da home e a ordem de precedência.
func TestReadmesDocumentSecretsEnv(t *testing.T) {
	for _, file := range []string{"README.md", "README.pt-BR.md"} {
		c := readRepoFile(t, file)
		for _, s := range []string{
			".makercycle/config/secrets.env",
			"~/.makercycle/config/secrets.env",
			"project.env -> models.env -> secrets.env",
		} {
			if !strings.Contains(c, s) {
				t.Errorf("%s deve conter %q", file, s)
			}
		}
	}
}

// RH-X3: READMEs não descrevem mais project.env como ignorado pelo git nem como lugar das chaves.
func TestReadmesNoLongerIgnoreProjectEnv(t *testing.T) {
	for _, file := range []string{"README.md", "README.pt-BR.md"} {
		c := strings.ToLower(readRepoFile(t, file))
		for _, s := range []string{
			"project.env` (git-ignored",
			"project.env` (ignored by git)",
			"project.env` (ignorado pelo git",
			"project.env (git-ignored)",
			"project.env (ignorado pelo git)",
			"`.makercycle/config/project.env` to `.gitignore`",
			"`.makercycle/config/project.env` ao `.gitignore`",
			"exclusively** in `.makercycle/config/project.env`",
			"exclusivamente** em `.makercycle/config/project.env`",
		} {
			if strings.Contains(c, s) {
				t.Errorf("%s ainda contém %q (project.env agora é versionado; chaves vão para secrets.env)", file, s)
			}
		}
	}
}
