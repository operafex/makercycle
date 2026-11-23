package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// secrets_env.go — Tasks 1 e 2: secrets.env (herança da home) e carregamento
// em camadas project.env -> models.env -> secrets.env -> ambiente do SO.
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: SE-*, EL-*)
// ---------------------------------------------------------------------------

// SE-H1: caminhos e camadas são contrato.
func TestSecretsEnvPathConstants(t *testing.T) {
	if secretsEnvPath != ".makercycle/config/secrets.env" {
		t.Errorf("secretsEnvPath = %q", secretsEnvPath)
	}
	want := []string{".makercycle/config/project.env", ".makercycle/config/models.env", ".makercycle/config/secrets.env"}
	if strings.Join(envLayerPaths, "|") != strings.Join(want, "|") {
		t.Errorf("envLayerPaths = %v; want %v", envLayerPaths, want)
	}
	if initActionMissing != "ausente na home" {
		t.Errorf("initActionMissing = %q", initActionMissing)
	}
}

// SE-H2: caminho global do secrets.env.
func TestGlobalSecretsEnvPath(t *testing.T) {
	home := filepath.Join(t.TempDir(), "u")
	got, err := globalSecretsEnvPath(home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".makercycle", "config", "secrets.env"); got != want {
		t.Errorf("globalSecretsEnvPath = %q; want %q", got, want)
	}
}

// SE-X1: home vazia (ou só espaços) => erro.
func TestGlobalSecretsEnvPathEmptyHomeErrors(t *testing.T) {
	for _, h := range []string{"", "   "} {
		if _, err := globalSecretsEnvPath(h); err == nil {
			t.Errorf("home %q: esperava erro", h)
		}
	}
}

// SE-H3: projeto sem secrets.env + global presente => cópia byte a byte, permissão 0600.
func TestProvisionSecretsEnvCopiesGlobal(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "home", "secrets.env")
	writeFiles(t, filepath.Join(dir, "home"), map[string]string{"secrets.env": "ANTHROPIC_API_KEY=sk-global\n"})
	proj := filepath.Join(dir, "proj", ".makercycle", "config", "secrets.env")
	if err := os.MkdirAll(filepath.Dir(proj), 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := provisionSecretsEnv(proj, global)
	if err != nil {
		t.Fatalf("provisionSecretsEnv: %v", err)
	}
	if res.Path != proj || res.Action != initActionFromGlobal {
		t.Errorf("res = %+v; want {%s %s}", res, proj, initActionFromGlobal)
	}
	if got := readString(t, proj); got != "ANTHROPIC_API_KEY=sk-global\n" {
		t.Errorf("conteúdo = %q", got)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(proj)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissão = %o; want 600", perm)
		}
	}
}

// SE-H4: secrets.env já existente no projeto nunca é sobrescrito.
func TestProvisionSecretsEnvKeepsExisting(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"home/secrets.env": "ANTHROPIC_API_KEY=sk-global\n",
		"proj/secrets.env": "ANTHROPIC_API_KEY=sk-projeto\n",
	})
	proj := filepath.Join(dir, "proj", "secrets.env")
	res, err := provisionSecretsEnv(proj, filepath.Join(dir, "home", "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != initActionKept {
		t.Errorf("ação = %q; want %q", res.Action, initActionKept)
	}
	if got := readString(t, proj); got != "ANTHROPIC_API_KEY=sk-projeto\n" {
		t.Errorf("secrets.env do projeto alterado: %q", got)
	}
}

// SE-E1: sem global (caminho vazio, inexistente ou diretório) => nada é criado, ação initActionMissing.
func TestProvisionSecretsEnvNoGlobal(t *testing.T) {
	dir := t.TempDir()
	globalDir := filepath.Join(dir, "global-dir")
	if err := os.MkdirAll(globalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, global := range []string{"", filepath.Join(dir, "nao-existe.env"), globalDir} {
		proj := filepath.Join(dir, "proj", "secrets.env")
		res, err := provisionSecretsEnv(proj, global)
		if err != nil {
			t.Fatalf("global %q: %v", global, err)
		}
		if res.Path != proj || res.Action != initActionMissing {
			t.Errorf("global %q: res = %+v; want {%s %s}", global, res, proj, initActionMissing)
		}
		if isFile(proj) {
			t.Errorf("global %q: secrets.env não deveria ser criado", global)
		}
	}
}

// SE-E2: secrets.env vazio na home também é copiado (o usuário decide o conteúdo).
func TestProvisionSecretsEnvCopiesEmptyGlobal(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"home/secrets.env": ""})
	proj := filepath.Join(dir, "secrets.env")
	res, err := provisionSecretsEnv(proj, filepath.Join(dir, "home", "secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != initActionFromGlobal || !isFile(proj) {
		t.Errorf("res = %+v, existe=%v; want cópia do global vazio", res, isFile(proj))
	}
}

// SE-X2: global ilegível => erro (e nenhum arquivo criado no projeto).
func TestProvisionSecretsEnvUnreadableGlobalErrors(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permissões POSIX necessárias (e não-root)")
	}
	dir := t.TempDir()
	global := filepath.Join(dir, "secrets.env")
	if err := os.WriteFile(global, []byte("X=1\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	proj := filepath.Join(dir, "proj", "secrets.env")
	if err := os.MkdirAll(filepath.Dir(proj), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := provisionSecretsEnv(proj, global); err == nil {
		t.Fatal("esperava erro com global ilegível")
	}
	if isFile(proj) {
		t.Error("secrets.env não deveria ser criado após erro")
	}
}

// ---------------------------------------------------------------------------
// mergeEnvLayers / loadEnvLayers
// ---------------------------------------------------------------------------

func writeLayers(t *testing.T, project, models, secrets string) []string {
	t.Helper()
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "project.env"), filepath.Join(dir, "models.env"), filepath.Join(dir, "secrets.env")}
	for i, c := range []string{project, models, secrets} {
		if c == "<ausente>" {
			continue
		}
		if err := os.WriteFile(paths[i], []byte(c), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return paths
}

// EL-H1: precedência crescente project.env < models.env < secrets.env < ambiente do SO.
func TestMergeEnvLayersPrecedence(t *testing.T) {
	paths := writeLayers(t,
		"A=project\nB=project\nC=project\nD=project\n",
		"B=models\nC=models\nD=models\n",
		"C=secrets\nD=secrets\n",
	)
	got, err := mergeEnvLayers(paths, fakeGetenv(map[string]string{"D": "so"}))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "project", "B": "models", "C": "secrets"}
	if len(got) != len(want) {
		t.Errorf("got = %v; want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q; want %q", k, got[k], v)
		}
	}
	if _, ok := got["D"]; ok {
		t.Errorf("D já exportada no SO não pode ser sobrescrita: %v", got)
	}
}

// EL-H2: sintaxe aceita — comentários, linhas vazias, export, aspas, espaços, '=' no valor, CRLF.
func TestMergeEnvLayersParsing(t *testing.T) {
	paths := writeLayers(t,
		"# comentário\n\nexport A=\"aspas duplas\"\r\n  B = 'simples'  \nC=x=y\nsem_igual\n=semchave\n",
		"<ausente>", "<ausente>",
	)
	got, err := mergeEnvLayers(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"A": "aspas duplas", "B": "simples", "C": "x=y"}
	if len(got) != len(want) {
		t.Errorf("got = %v; want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q; want %q", k, got[k], v)
		}
	}
}

// EL-E1: arquivos ausentes são ignorados sem erro; mapa nunca é nil.
func TestMergeEnvLayersMissingFiles(t *testing.T) {
	paths := writeLayers(t, "<ausente>", "<ausente>", "<ausente>")
	got, err := mergeEnvLayers(paths, nil)
	if err != nil {
		t.Fatalf("arquivos ausentes não são erro: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Errorf("got = %v; want mapa vazio não nil", got)
	}
}

// EL-E2: valor vazio numa camada superior NÃO apaga o valor de uma camada inferior.
func TestMergeEnvLayersEmptyValueDoesNotOverride(t *testing.T) {
	paths := writeLayers(t, "ANTHROPIC_API_KEY=sk-proj\nVAZIA=\n", "<ausente>", "ANTHROPIC_API_KEY=\"\"\n")
	got, err := mergeEnvLayers(paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got["ANTHROPIC_API_KEY"] != "sk-proj" {
		t.Errorf("ANTHROPIC_API_KEY = %q; want sk-proj", got["ANTHROPIC_API_KEY"])
	}
	if _, ok := got["VAZIA"]; ok {
		t.Errorf("chave cujo valor final é vazio deve ser omitida: %v", got)
	}
}

// EL-E3: variável do SO vazia ou só com espaços conta como ausente (arquivo prevalece).
func TestMergeEnvLayersBlankOSValueIsUnset(t *testing.T) {
	paths := writeLayers(t, "<ausente>", "<ausente>", "GEMINI_API_KEY=g-secret\n")
	got, err := mergeEnvLayers(paths, fakeGetenv(map[string]string{"GEMINI_API_KEY": "  "}))
	if err != nil {
		t.Fatal(err)
	}
	if got["GEMINI_API_KEY"] != "g-secret" {
		t.Errorf("GEMINI_API_KEY = %q; want g-secret", got["GEMINI_API_KEY"])
	}
}

// EL-X1: camada ilegível (diretório) => erro citando o caminho, mas as demais camadas continuam valendo.
func TestMergeEnvLayersUnreadableLayerKeepsOthers(t *testing.T) {
	paths := writeLayers(t, "A=project\n", "B=models\n", "<ausente>")
	if err := os.MkdirAll(paths[2], 0o755); err != nil { // secrets.env é um diretório
		t.Fatal(err)
	}
	got, err := mergeEnvLayers(paths, nil)
	if err == nil {
		t.Fatal("esperava erro para camada ilegível")
	}
	if !strings.Contains(err.Error(), paths[2]) {
		t.Errorf("erro deve citar %s: %v", paths[2], err)
	}
	if got["A"] != "project" || got["B"] != "models" {
		t.Errorf("camadas legíveis devem ser mantidas: %v", got)
	}
}

// EL-H3: loadEnvLayers aplica o merge em os.Environ respeitando variáveis já exportadas.
func TestLoadEnvLayersSetsProcessEnv(t *testing.T) {
	paths := writeLayers(t, "MC_EL_A=project\nMC_EL_B=project\n", "MC_EL_B=models\n", "MC_EL_C=secrets\nMC_EL_D=secrets\n")
	t.Setenv("MC_EL_A", "")
	t.Setenv("MC_EL_B", "")
	t.Setenv("MC_EL_C", "")
	t.Setenv("MC_EL_D", "exportada")
	if err := loadEnvLayers(paths); err != nil {
		t.Fatal(err)
	}
	for k, v := range map[string]string{"MC_EL_A": "project", "MC_EL_B": "models", "MC_EL_C": "secrets", "MC_EL_D": "exportada"} {
		if got := os.Getenv(k); got != v {
			t.Errorf("%s = %q; want %q", k, got, v)
		}
	}
}

// EL-X2: loadEnvLayers devolve o erro da camada ilegível, mas aplica as demais.
func TestLoadEnvLayersPartialOnError(t *testing.T) {
	paths := writeLayers(t, "MC_EL_P=ok\n", "<ausente>", "<ausente>")
	if err := os.MkdirAll(paths[1], 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MC_EL_P", "")
	if err := loadEnvLayers(paths); err == nil {
		t.Fatal("esperava erro")
	}
	if got := os.Getenv("MC_EL_P"); got != "ok" {
		t.Errorf("MC_EL_P = %q; want ok", got)
	}
}

// EL-H4: chave de API só no secrets.env satisfaz a validação de auth (fluxo real do main).
func TestSecretsEnvKeySatisfiesAuth(t *testing.T) {
	paths := writeLayers(t, "MC_STACK_NAME=Go\n", "MC_AUTH_MODE=api_key\n", "ANTHROPIC_API_KEY=sk-only-secrets\n")
	for _, k := range providerAPIKeyNames {
		t.Setenv(k, "")
	}
	t.Setenv("MC_OMNIROUTE_ENABLED", "")
	t.Setenv("MC_AUTH_MODE", "")
	if err := loadEnvLayers(paths); err != nil {
		t.Fatal(err)
	}
	if err := validateAuthConfig(authModeAPIKey, os.Getenv); err != nil {
		t.Fatalf("chave do secrets.env deveria satisfazer a auth: %v", err)
	}
	assertEnvSingle(t, agentEnvFromProcess(), "ANTHROPIC_API_KEY", "sk-only-secrets")
}

// ---------------------------------------------------------------------------
// Avisos: project.env agora é versionado => chaves nele são vazamento em potencial.
// ---------------------------------------------------------------------------

// SE-H5: aviso para chaves em project.env aponta para secrets.env e nunca expõe valores.
func TestProjectEnvKeyWarning(t *testing.T) {
	if got := projectEnvKeyWarning(nil); got != "" {
		t.Errorf("sem chaves => vazio; got %q", got)
	}
	want := "[AVISO] Chaves de API encontradas em .makercycle/config/project.env (versionado): ANTHROPIC_API_KEY, GEMINI_API_KEY. Mova-as para .makercycle/config/secrets.env (ignorado pelo git) e remova-as do project.env."
	if got := projectEnvKeyWarning([]string{"ANTHROPIC_API_KEY", "GEMINI_API_KEY"}); got != want {
		t.Errorf("projectEnvKeyWarning =\n%q\nwant\n%q", got, want)
	}
}

// SE-H6: o aviso do models.env passa a mandar as chaves para secrets.env (project.env é versionado).
func TestModelsEnvKeyWarningPointsToSecretsEnv(t *testing.T) {
	want := "[AVISO] Chaves de API encontradas em .makercycle/config/models.env (versionado): OPENAI_API_KEY. Mova-as para .makercycle/config/secrets.env (ignorado pelo git) e remova-as do models.env."
	if got := modelsEnvKeyWarning([]string{"OPENAI_API_KEY"}); got != want {
		t.Errorf("modelsEnvKeyWarning =\n%q\nwant\n%q", got, want)
	}
}
