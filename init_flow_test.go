package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// ---------------------------------------------------------------------------
// init.go — Task 4: orquestração pura do -init (provisionamento, detecção de
// código, skills, binários, rebind de agentes e prompt do SCOUT)
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: IF-*)
// ---------------------------------------------------------------------------

func fullAgentsMapFS() fstest.MapFS {
	m := fstest.MapFS{}
	for _, n := range []string{"00_SCOUT.md", "01_PLANNER.md", "02_CRITIC.md", "03_MAKER.md", "04_AUDITOR.md", "05_GATEKEEPER.md"} {
		m[".makercycle/agents/"+n] = &fstest.MapFile{Data: []byte("# " + n + "\n")}
	}
	return m
}

// IF-H1: provisionInitAssets cria agentes, .gitkeep, models.env, secrets.env e .gitignore na ordem do contrato.
// [REGRESSÃO JUSTIFICADA] 11 -> 12 resultados: o -init passa a reportar secrets.env
// (docs/references/team-scale-and-regression.md §1). As posições anteriores a models.env não mudam.
func TestProvisionInitAssetsFreshProject(t *testing.T) {
	root := t.TempDir()
	p := initPaths{Root: root, DocsDir: "docs", GlobalModelsEnv: ""}
	res, err := provisionInitAssets(p, fullAgentsMapFS(), []byte("MC_TEMPLATE=1\n"), false)
	if err != nil {
		t.Fatalf("provisionInitAssets: %v", err)
	}
	if len(res) != 12 {
		t.Fatalf("len(res) = %d; want 12 (6 agentes + 3 .gitkeep + models.env + secrets.env + .gitignore): %+v", len(res), res)
	}
	wantPaths := []string{
		filepath.Join(root, ".makercycle", "agents", "00_SCOUT.md"),
		filepath.Join(root, ".makercycle", "agents", "05_GATEKEEPER.md"),
		filepath.Join(root, "docs", "demands", ".gitkeep"),
		filepath.Join(root, "docs", "references", ".gitkeep"),
		filepath.Join(root, ".makercycle", "config", "models.env"),
		filepath.Join(root, ".makercycle", "config", "secrets.env"),
		filepath.Join(root, ".gitignore"),
	}
	wantIdx := []int{0, 5, 6, 8, 9, 10, 11}
	for i, idx := range wantIdx {
		if res[idx].Path != wantPaths[i] {
			t.Errorf("res[%d].Path = %q; want %q", idx, res[idx].Path, wantPaths[i])
		}
	}
	if res[9].Action != initActionFromEmbedded {
		t.Errorf("models.env ação = %q; want %q", res[9].Action, initActionFromEmbedded)
	}
	if res[10].Action != initActionMissing {
		t.Errorf("secrets.env ação = %q; want %q (sem global)", res[10].Action, initActionMissing)
	}
	if isFile(filepath.Join(root, ".makercycle", "config", "secrets.env")) {
		t.Error("secrets.env não deve ser criado sem global")
	}
	if res[11].Action != initActionCreated {
		t.Errorf(".gitignore ação = %q; want %q", res[11].Action, initActionCreated)
	}
	for _, sub := range []string{"agents", "config", "state", "telemetry"} {
		if !isDir(filepath.Join(root, ".makercycle", sub)) {
			t.Errorf(".makercycle/%s não criado", sub)
		}
	}
	gi := readString(t, filepath.Join(root, ".gitignore"))
	for _, e := range gitignoreEntries {
		if !strings.Contains(gi, e) {
			t.Errorf(".gitignore sem %q", e)
		}
	}
}

// IF-E1: segunda execução sem force → tudo "mantido".
func TestProvisionInitAssetsSecondRunKeepsEverything(t *testing.T) {
	root := t.TempDir()
	p := initPaths{Root: root, DocsDir: "docs"}
	if _, err := provisionInitAssets(p, fullAgentsMapFS(), []byte("X=1\n"), false); err != nil {
		t.Fatal(err)
	}
	res, err := provisionInitAssets(p, fullAgentsMapFS(), []byte("X=1\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	// [REGRESSÃO JUSTIFICADA] secrets.env sem global nunca é criado, logo segue "ausente na home"
	// (não "mantido"); todos os demais artefatos continuam "mantido".
	secrets := filepath.Join(root, ".makercycle", "config", "secrets.env")
	for _, r := range res {
		want := initActionKept
		if r.Path == secrets {
			want = initActionMissing
		}
		if r.Action != want {
			t.Errorf("%s = %q; want %q", r.Path, r.Action, want)
		}
	}
}

// IF-E2: com global presente e projeto sem models.env → herança da home.
func TestProvisionInitAssetsInheritsGlobal(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	writeFiles(t, home, map[string]string{".makercycle/config/models.env": "MC_GLOBAL=1\n"})
	p := initPaths{Root: root, DocsDir: "wiki", GlobalModelsEnv: filepath.Join(home, ".makercycle", "config", "models.env")}
	res, err := provisionInitAssets(p, fullAgentsMapFS(), []byte("MC_TEMPLATE=1\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, filepath.Join(root, ".makercycle", "config", "models.env")); got != "MC_GLOBAL=1\n" {
		t.Errorf("models.env = %q; want herdado da home", got)
	}
	if res[9].Action != initActionFromGlobal {
		t.Errorf("ação = %q; want %q", res[9].Action, initActionFromGlobal)
	}
	if !isFile(filepath.Join(root, "wiki", "logs", ".gitkeep")) {
		t.Error("DocsDir personalizado (wiki) não respeitado")
	}
}

// IF-E3: force sobrescreve agentes, mas NUNCA o models.env existente nem .gitkeep.
func TestProvisionInitAssetsForceOnlyAffectsAgents(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		".makercycle/agents/01_PLANNER.md": "customizado\n",
		".makercycle/config/models.env":    "MC_PROJETO=1\n",
		"docs/references/.gitkeep":         "manter",
	})
	res, err := provisionInitAssets(initPaths{Root: root, DocsDir: "docs"}, fullAgentsMapFS(), []byte("MC_TEMPLATE=1\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, filepath.Join(root, ".makercycle", "agents", "01_PLANNER.md")); got != "# 01_PLANNER.md\n" {
		t.Errorf("agente não sobrescrito com force: %q", got)
	}
	if got := readString(t, filepath.Join(root, ".makercycle", "config", "models.env")); got != "MC_PROJETO=1\n" {
		t.Errorf("models.env alterado com force: %q", got)
	}
	if got := readString(t, filepath.Join(root, "docs", "references", ".gitkeep")); got != "manter" {
		t.Errorf(".gitkeep alterado com force: %q", got)
	}
	if res[1].Action != initActionOverwritten {
		t.Errorf("01_PLANNER ação = %q; want %q", res[1].Action, initActionOverwritten)
	}
}

// IF-E4: .gitignore existente sem as entradas → ação "atualizado".
func TestProvisionInitAssetsGitignoreUpdated(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{".gitignore": "bin/\n"})
	res, err := provisionInitAssets(initPaths{Root: root, DocsDir: "docs"}, fullAgentsMapFS(), []byte("X=1\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	if last := res[len(res)-1]; last.Action != initActionUpdated {
		t.Errorf(".gitignore ação = %q; want %q", last.Action, initActionUpdated)
	}
	if got := readString(t, filepath.Join(root, ".gitignore")); !strings.HasPrefix(got, "bin/\n") {
		t.Errorf("conteúdo prévio do .gitignore perdido: %q", got)
	}
}

// IF-X1: falha em qualquer etapa interrompe e devolve erro com contexto.
func TestProvisionInitAssetsPropagatesErrors(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"docs": "sou arquivo"})
	_, err := provisionInitAssets(initPaths{Root: root, DocsDir: "docs"}, fullAgentsMapFS(), []byte("X=1\n"), false)
	if err == nil {
		t.Fatal("esperava erro com docs bloqueado por arquivo")
	}
	if !strings.Contains(err.Error(), "docs") {
		t.Errorf("erro %q deveria citar o caminho docs", err)
	}
	if _, err := provisionInitAssets(initPaths{Root: t.TempDir(), DocsDir: "docs"}, fstest.MapFS{}, []byte("X=1\n"), false); err == nil {
		t.Error("esperava erro com FS de agentes vazio")
	}
}

// IF-H2: detecção de arquivos de código para decidir se o SCOUT roda.
func TestHasCodeFiles(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"vazio", map[string]string{}, false},
		{"só go.mod", map[string]string{"go.mod": "module x\n"}, false},
		{"só docs", map[string]string{"docs/README.md": "# x", "Makefile": "all:"}, false},
		{"go na raiz", map[string]string{"main.go": "package main"}, true},
		{"ts aninhado", map[string]string{"src/app/x.ts": "export {}"}, true},
		{"extensão maiúscula", map[string]string{"MAIN.PY": "print(1)"}, true},
		{"ruby", map[string]string{"app/models/user.rb": ""}, true},
		{"só node_modules", map[string]string{"node_modules/a/index.js": ""}, false},
		{"só vendor", map[string]string{"vendor/x/y.go": ""}, false},
		{"só pasta oculta", map[string]string{".makercycle/x.go": "", ".git/hooks/a.py": ""}, false},
		{"build/dist/target", map[string]string{"dist/a.js": "", "build/b.js": "", "target/c.rs": ""}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, c.files)
			if got := hasCodeFiles(root); got != c.want {
				t.Errorf("hasCodeFiles = %v; want %v", got, c.want)
			}
		})
	}
}

// IF-E5: raiz inexistente → false (sem pânico).
func TestHasCodeFilesMissingRoot(t *testing.T) {
	if hasCodeFiles(filepath.Join(t.TempDir(), "nao-existe")) {
		t.Error("raiz inexistente deveria retornar false")
	}
}

// IF-H3: lista exata de extensões consideradas código.
func TestCodeFileExtensionsContract(t *testing.T) {
	want := []string{".go", ".rb", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".kt", ".rs", ".php", ".cs", ".swift", ".c", ".h", ".cpp", ".ex", ".exs", ".scala", ".dart"}
	if strings.Join(codeFileExtensions, " ") != strings.Join(want, " ") {
		t.Errorf("codeFileExtensions = %v; want %v", codeFileExtensions, want)
	}
}

// IF-H4: skills do -init vêm SOMENTE das variáveis MC_*_SKILL declaradas (sem fallback de flags).
func TestInitSkillsToInstall(t *testing.T) {
	if got := initSkillsToInstall(nil); len(got) != 0 {
		t.Errorf("sem env: %v; want vazio", got)
	}
	if got := initSkillsToInstall([]string{"MC_PLANNER_SKILL=", "MC_PLANNER_MODEL=opus", "PATH=/bin"}); len(got) != 0 {
		t.Errorf("skill vazia: %v; want vazio", got)
	}
	env := []string{"MC_PLANNER_SKILL=obra/superpowers", "MC_CRITIC_SKILL=google/elephant-goldfish-model", "MC_AUDITOR_SKILL=obra/superpowers"}
	want := []string{"google/elephant-goldfish-model", "obra/superpowers"}
	if got := initSkillsToInstall(env); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("initSkillsToInstall = %v; want %v", got, want)
	}
}

// IF-H5: npx só é exigido quando há skills declaradas; git sempre; sem duplicatas nem vazios.
func TestInitRequiredBinaries(t *testing.T) {
	agents := []AgentBinding{{Bin: "claude"}, {Bin: " claude "}, {Bin: "agy"}, {Bin: ""}}
	if got, want := initRequiredBinaries(agents, nil), []string{"claude", "agy", "git"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("sem skills = %v; want %v", got, want)
	}
	if got, want := initRequiredBinaries(agents, []string{"obra/superpowers"}), []string{"claude", "agy", "npx", "git"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("com skills = %v; want %v", got, want)
	}
}

// IF-H6: rebind dos agentes a partir do models.env recém-provisionado.
func TestRefreshBindingFromEnv(t *testing.T) {
	a := AgentBinding{Alias: "SCOUT", Bin: "claude", Model: "opus", Args: "", Skill: ""}
	env := fakeGetenv(map[string]string{
		"MC_SCOUT_BIN":   "gemini",
		"MC_SCOUT_MODEL": "  ",
		"MC_SCOUT_ARGS":  "--yolo",
		"MC_SCOUT_SKILL": "x/y",
	})
	got := refreshBindingFromEnv(a, env, map[string]bool{})
	want := AgentBinding{Alias: "SCOUT", Bin: "gemini", Model: "opus", Args: "--yolo", Skill: "x/y"}
	if got != want {
		t.Errorf("refreshBindingFromEnv = %+v; want %+v", got, want)
	}
}

// IF-E6: flags explícitas (-scout-bin etc.) prevalecem sobre o models.env.
func TestRefreshBindingFromEnvRespectsExplicitFlags(t *testing.T) {
	a := AgentBinding{Alias: "PLANNER", Bin: "meu-claude", Model: "opus", Skill: "obra/superpowers"}
	env := fakeGetenv(map[string]string{"MC_PLANNER_BIN": "agy", "MC_PLANNER_MODEL": "sonnet", "MC_PLANNER_SKILL": "outra/skill"})
	got := refreshBindingFromEnv(a, env, map[string]bool{"planner-bin": true, "planner-skill": true})
	if got.Bin != "meu-claude" || got.Skill != "obra/superpowers" {
		t.Errorf("flags explícitas sobrescritas: %+v", got)
	}
	if got.Model != "sonnet" {
		t.Errorf("Model = %q; want sonnet (flag não explícita)", got.Model)
	}
}

// legacyScoutPrompt replica literalmente o prompt montado hoje em runProjectSetup.
func legacyScoutPrompt(existingDocs, docsDir, detectedName string) string {
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

// IF-H7: o prompt do SCOUT extraído para buildScoutPrompt é idêntico ao legado (sem regressão no -setup).
func TestBuildScoutPromptMatchesLegacy(t *testing.T) {
	cases := [][3]string{
		{"", "docs", "Go"},
		{"wiki", "wiki", ""},
		{"README.md", "docs", "Rails"},
	}
	for _, c := range cases {
		if got, want := buildScoutPrompt(c[0], c[1], c[2]), legacyScoutPrompt(c[0], c[1], c[2]); got != want {
			t.Errorf("buildScoutPrompt(%q,%q,%q) =\n%s\nwant\n%s", c[0], c[1], c[2], got, want)
		}
	}
}

// IF-E5: com secrets.env na home e projeto sem ele => cópia 0600; o existente no projeto nunca é sobrescrito.
func TestProvisionInitAssetsInheritsGlobalSecrets(t *testing.T) {
	root, home := t.TempDir(), t.TempDir()
	writeFiles(t, home, map[string]string{".makercycle/config/secrets.env": "ANTHROPIC_API_KEY=sk-home\n"})
	p := initPaths{Root: root, DocsDir: "docs", GlobalSecretsEnv: filepath.Join(home, ".makercycle", "config", "secrets.env")}
	res, err := provisionInitAssets(p, fullAgentsMapFS(), []byte("X=1\n"), false)
	if err != nil {
		t.Fatal(err)
	}
	secrets := filepath.Join(root, ".makercycle", "config", "secrets.env")
	if res[10].Path != secrets || res[10].Action != initActionFromGlobal {
		t.Errorf("res[10] = %+v; want {%s %s}", res[10], secrets, initActionFromGlobal)
	}
	if got := readString(t, secrets); got != "ANTHROPIC_API_KEY=sk-home\n" {
		t.Errorf("secrets.env = %q", got)
	}

	writeFiles(t, home, map[string]string{".makercycle/config/secrets.env": "ANTHROPIC_API_KEY=sk-outra\n"})
	res, err = provisionInitAssets(p, fullAgentsMapFS(), []byte("X=1\n"), true)
	if err != nil {
		t.Fatal(err)
	}
	if res[10].Action != initActionKept {
		t.Errorf("2ª execução (mesmo com force): ação = %q; want %q", res[10].Action, initActionKept)
	}
	if got := readString(t, secrets); got != "ANTHROPIC_API_KEY=sk-home\n" {
		t.Errorf("secrets.env do projeto foi sobrescrito: %q", got)
	}
}

// IF-X2: erro ao copiar o secrets.env global interrompe o -init com o caminho no erro.
func TestProvisionInitAssetsSecretsErrorHasContext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".makercycle", "config", "secrets.env", "x"), 0o755); err != nil {
		t.Fatal(err) // secrets.env do projeto é um diretório não vazio e o global existe
	}
	home := t.TempDir()
	writeFiles(t, home, map[string]string{"secrets.env": "K=1\n"})
	p := initPaths{Root: root, DocsDir: "docs", GlobalSecretsEnv: filepath.Join(home, "secrets.env")}
	_, err := provisionInitAssets(p, fullAgentsMapFS(), []byte("X=1\n"), false)
	if err == nil || !strings.Contains(err.Error(), "secrets.env") {
		t.Fatalf("esperava erro citando secrets.env; got %v", err)
	}
}
