package main

import (
	"io/fs"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 2: módulo Go e assets embutidos (go.mod, embed.go)
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: EM-*)
// ---------------------------------------------------------------------------

// EM-H1: go.mod declara o módulo público.
func TestGoModModulePath(t *testing.T) {
	first := strings.SplitN(strings.TrimSpace(readRepoFile(t, "go.mod")), "\n", 2)[0]
	if got, want := strings.TrimSpace(first), "module github.com/operafex/makercycle"; got != want {
		t.Errorf("go.mod linha 1 = %q; want %q", got, want)
	}
}

// EM-H2: embed.go usa exatamente as diretivas da referência.
func TestEmbedGoDirectives(t *testing.T) {
	src := readRepoFile(t, "embed.go")
	for _, d := range []string{
		"//go:embed all:.makercycle/agents\n",
		"//go:embed all:.makercycle/config/models.env\n",
		"package main",
	} {
		if !strings.Contains(src, d) {
			t.Errorf("embed.go sem %q", d)
		}
	}
}

// EM-H3: os seis contratos de agente estão embutidos, byte a byte iguais aos do repositório.
func TestEmbeddedAgentsMatchRepo(t *testing.T) {
	entries, err := fs.ReadDir(embeddedAgentsFS, embeddedAgentsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s): %v", embeddedAgentsDir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	want := []string{"00_SCOUT.md", "01_PLANNER.md", "02_CRITIC.md", "03_MAKER.md", "04_AUDITOR.md", "05_GATEKEEPER.md"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("agentes embutidos = %v; want %v", names, want)
	}
	for _, n := range want {
		b, err := fs.ReadFile(embeddedAgentsFS, embeddedAgentsDir+"/"+n)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != readRepoFile(t, ".makercycle/agents/"+n) {
			t.Errorf("%s embutido difere do arquivo do repositório", n)
		}
	}
}

// EM-H4: embeddedAgentsDir é o caminho dentro do FS embutido.
func TestEmbeddedAgentsDirConstant(t *testing.T) {
	if embeddedAgentsDir != ".makercycle/agents" {
		t.Errorf("embeddedAgentsDir = %q; want %q", embeddedAgentsDir, ".makercycle/agents")
	}
}

// EM-H5: o template models.env embutido é idêntico ao do repositório e não está vazio.
func TestEmbeddedModelsEnvMatchesRepo(t *testing.T) {
	if len(embeddedModelsEnv) == 0 {
		t.Fatal("embeddedModelsEnv vazio")
	}
	if string(embeddedModelsEnv) != readRepoFile(t, ".makercycle/config/models.env") {
		t.Error("embeddedModelsEnv difere de .makercycle/config/models.env")
	}
}

// EM-X1: o template embutido (distribuído publicamente no binário) não carrega chaves de provedor.
func TestEmbeddedModelsEnvHasNoProviderKeys(t *testing.T) {
	if keys := apiKeysInModelsEnv(string(embeddedModelsEnv)); len(keys) != 0 {
		t.Errorf("template embutido contém chaves de API com valor: %v", keys)
	}
}

// EM-H6: o template embutido declara as skills padrão (para o -init instalá-las num projeto novo).
func TestEmbeddedModelsEnvDeclaresSkills(t *testing.T) {
	var env []string
	for k, v := range parseEnvContent(string(embeddedModelsEnv)) {
		env = append(env, k+"="+v)
	}
	if got := initSkillsToInstall(env); len(got) == 0 {
		t.Error("template embutido deveria declarar ao menos uma MC_*_SKILL")
	}
}
