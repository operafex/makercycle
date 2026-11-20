package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers compartilhados pelos testes de auth/quota/resume
// ---------------------------------------------------------------------------

// fakeGetenv devolve um getenv injetável baseado em mapa (chave ausente => "").
func fakeGetenv(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// envValues devolve todos os valores atribuídos a key em env (formato "K=V").
func envValues(env []string, key string) []string {
	var vals []string
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			vals = append(vals, strings.TrimPrefix(kv, key+"="))
		}
	}
	return vals
}

func assertEnvSingle(t *testing.T, env []string, key, want string) {
	t.Helper()
	vals := envValues(env, key)
	if len(vals) != 1 || vals[0] != want {
		t.Fatalf("%s: esperado exatamente um valor %q, obtido %v (env=%v)", key, want, vals, env)
	}
}

func assertEnvAbsent(t *testing.T, env []string, key string) {
	t.Helper()
	if vals := envValues(env, key); len(vals) != 0 {
		t.Fatalf("%s deveria estar ausente, obtido %v", key, vals)
	}
}

var testOmni = OmniRouteConfig{
	Enabled: true,
	URL:     "http://localhost:20128",
	APIKey:  "omniroute-local",
	Model:   "claude-sonnet-4-6",
}

// ---------------------------------------------------------------------------
// Task 1 — resolveAuthMode(raw string) (string, error)
// ---------------------------------------------------------------------------

func TestResolveAuthMode(t *testing.T) {
	cases := []struct {
		id, in, want string
		wantErr      bool
	}{
		{"AM-H1", "api_key", "api_key", false},
		{"AM-H2", "another", "another", false},
		{"AM-E1", "", "api_key", false},
		{"AM-E2", "   ", "api_key", false},
		{"AM-E3", "  API_KEY ", "api_key", false},
		{"AM-E4", "Another", "another", false},
		{"AM-X1", "oauth", "", true},
		{"AM-X2", "apikey", "", true},
		{"AM-X3", "api_key;rm -rf", "", true},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			got, err := resolveAuthMode(c.in)
			if c.wantErr {
				if err == nil {
					t.Fatalf("resolveAuthMode(%q) deveria falhar", c.in)
				}
				if !strings.Contains(err.Error(), "MC_AUTH_MODE") {
					t.Fatalf("erro deve citar MC_AUTH_MODE: %v", err)
				}
				if !strings.Contains(err.Error(), strings.TrimSpace(c.in)) {
					t.Fatalf("erro deve citar o valor recebido %q: %v", c.in, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("resolveAuthMode(%q) = %q, %v; want %q", c.in, got, err, c.want)
			}
		})
	}
}

func TestAuthModeConstants(t *testing.T) {
	if authModeAPIKey != "api_key" || authModeAnother != "another" {
		t.Fatalf("constantes divergentes: %q / %q", authModeAPIKey, authModeAnother)
	}
}

// ---------------------------------------------------------------------------
// Task 2 — loadOmniRouteConfig(getenv func(string) string) OmniRouteConfig
// ---------------------------------------------------------------------------

func TestLoadOmniRouteConfigDefaults(t *testing.T) {
	// OR-H1: sem variáveis => defaults e desativado.
	got := loadOmniRouteConfig(fakeGetenv(nil))
	want := OmniRouteConfig{Enabled: false, URL: "http://localhost:20128", APIKey: "omniroute-local", Model: "claude-sonnet-4-6"}
	if got != want {
		t.Fatalf("defaults divergentes:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestLoadOmniRouteConfigOverrides(t *testing.T) {
	// OR-H2: valores customizados (com espaços) são respeitados.
	got := loadOmniRouteConfig(fakeGetenv(map[string]string{
		"MC_OMNIROUTE_ENABLED": " TRUE ",
		"MC_OMNIROUTE_URL":     " http://omni:9000 ",
		"MC_OMNIROUTE_API_KEY": "sk-omni",
		"MC_OMNIROUTE_MODEL":   "claude-opus-4-6",
	}))
	want := OmniRouteConfig{Enabled: true, URL: "http://omni:9000", APIKey: "sk-omni", Model: "claude-opus-4-6"}
	if got != want {
		t.Fatalf("overrides divergentes:\n got: %+v\nwant: %+v", got, want)
	}
}

func TestLoadOmniRouteConfigEnabledOnlyForTrue(t *testing.T) {
	// OR-E1: apenas "true" (case-insensitive) ativa o fallback.
	for _, v := range []string{"false", "1", "yes", "on", "", "truee"} {
		got := loadOmniRouteConfig(fakeGetenv(map[string]string{"MC_OMNIROUTE_ENABLED": v}))
		if got.Enabled {
			t.Fatalf("MC_OMNIROUTE_ENABLED=%q não deveria ativar o OmniRoute", v)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 1 — validateAuthConfig(authMode string, getenv func(string) string) error
// ---------------------------------------------------------------------------

func TestValidateAuthConfig(t *testing.T) {
	cases := []struct {
		id   string
		mode string
		env  map[string]string
		ok   bool
	}{
		{"VA-H1", "api_key", map[string]string{"ANTHROPIC_API_KEY": "sk-ant"}, true},
		{"VA-H2", "api_key", map[string]string{"MC_API_KEY": "sk-456"}, true},
		{"VA-H3", "another", nil, true},
		{"VA-H4", "api_key", map[string]string{"OPENAI_API_KEY": "sk-oai"}, true},
		{"VA-H5", "api_key", map[string]string{"GEMINI_API_KEY": "gm-1"}, true},
		{"VA-E1", "api_key", map[string]string{"MC_OMNIROUTE_ENABLED": "true"}, true},
		{"VA-E2", "api_key", map[string]string{"MC_OMNIROUTE_ENABLED": "True"}, true},
		{"VA-E3", "api_key", map[string]string{"ANTHROPIC_API_KEY": "   "}, false},
		{"VA-E4", "api_key", map[string]string{"OPENAI_API_KEY": "\"\""}, false},
		{"VA-X1", "api_key", nil, false},
		{"VA-X2", "api_key", map[string]string{"MC_OMNIROUTE_ENABLED": "false"}, false},
		{"VA-X3", "api_key", map[string]string{"LLM_API_KEY": "sk-legacy"}, false},
		{"VA-X4", "bogus", map[string]string{"ANTHROPIC_API_KEY": "sk-123"}, false},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			err := validateAuthConfig(c.mode, fakeGetenv(c.env))
			if c.ok && err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if !c.ok && err == nil {
				t.Fatal("esperado erro, obtido nil")
			}
		})
	}
}

func TestValidateAuthConfigMessageGuidesUser(t *testing.T) {
	// VA-X5: mensagem clara instruindo project.env e as chaves dos provedores; sem LLM_API_KEY legada.
	err := validateAuthConfig("api_key", fakeGetenv(nil))
	if err == nil {
		t.Fatal("esperado erro sem API key")
	}
	for _, s := range []string{".makercycle/config/project.env", "ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("mensagem deve citar %q: %v", s, err)
		}
	}
	if strings.Contains(err.Error(), "LLM_API_KEY") {
		t.Fatalf("mensagem não deve citar a chave legada LLM_API_KEY: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Task 2 — buildAgentEnv(base []string, authMode string, omni OmniRouteConfig, useOmni bool) []string
// ---------------------------------------------------------------------------

func TestBuildAgentEnvAPIKeyStripsLegacyLLMKey(t *testing.T) {
	// BE-H1: LLM_API_KEY (legada, vinha do models.env versionado) é removida e não vira ANTHROPIC_API_KEY.
	base := []string{"PATH=/bin", "LLM_API_KEY=sk-1"}
	got := buildAgentEnv(base, "api_key", testOmni, false)
	assertEnvAbsent(t, got, "LLM_API_KEY")
	assertEnvAbsent(t, got, "ANTHROPIC_API_KEY")
	assertEnvSingle(t, got, "PATH", "/bin")
	assertEnvAbsent(t, got, "ANTHROPIC_BASE_URL")
	assertEnvAbsent(t, got, "ANTHROPIC_AUTH_TOKEN")
}

func TestBuildAgentEnvAPIKeyFallsBackToMCKey(t *testing.T) {
	// BE-H2: ANTHROPIC_API_KEY ausente => usa MC_API_KEY; nunca recria LLM_API_KEY.
	got := buildAgentEnv([]string{"MC_API_KEY=sk-mc"}, "api_key", testOmni, false)
	assertEnvSingle(t, got, "ANTHROPIC_API_KEY", "sk-mc")
	assertEnvAbsent(t, got, "LLM_API_KEY")
}

func TestBuildAgentEnvAPIKeyEmptyLLMUsesMCKey(t *testing.T) {
	// BE-E1: LLM_API_KEY vazia é descartada; MC_API_KEY alimenta ANTHROPIC_API_KEY, sem duplicar.
	got := buildAgentEnv([]string{"LLM_API_KEY=  ", "MC_API_KEY=sk-mc"}, "api_key", testOmni, false)
	assertEnvAbsent(t, got, "LLM_API_KEY")
	assertEnvSingle(t, got, "ANTHROPIC_API_KEY", "sk-mc")
}

func TestBuildAgentEnvAPIKeyPrefersAnthropicOverMC(t *testing.T) {
	// BE-E2
	got := buildAgentEnv([]string{"MC_API_KEY=sk-mc", "ANTHROPIC_API_KEY=sk-ant"}, "api_key", testOmni, false)
	assertEnvSingle(t, got, "ANTHROPIC_API_KEY", "sk-ant")
}

func TestBuildAgentEnvAnotherRemovesLLMKey(t *testing.T) {
	// BE-H3: modo alternativo remove LLM_API_KEY (todas as ocorrências).
	got := buildAgentEnv([]string{"LLM_API_KEY=sk-1", "HOME=/h", "LLM_API_KEY=sk-2"}, "another", testOmni, false)
	assertEnvAbsent(t, got, "LLM_API_KEY")
	assertEnvSingle(t, got, "HOME", "/h")
}

func TestBuildAgentEnvOmniRouteInjectsAnthropicVars(t *testing.T) {
	// BE-H4: fallback ativo => ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN substituídos (sem duplicatas).
	base := []string{"ANTHROPIC_BASE_URL=https://old", "PATH=/bin", "LLM_API_KEY=sk-1"}
	for _, mode := range []string{"api_key", "another"} {
		got := buildAgentEnv(base, mode, testOmni, true)
		assertEnvSingle(t, got, "ANTHROPIC_BASE_URL", "http://localhost:20128")
		assertEnvSingle(t, got, "ANTHROPIC_AUTH_TOKEN", "omniroute-local")
		assertEnvSingle(t, got, "PATH", "/bin")
	}
}

func TestBuildAgentEnvDoesNotMutateBase(t *testing.T) {
	// BE-E3
	base := []string{"LLM_API_KEY=sk-1", "ANTHROPIC_BASE_URL=https://old"}
	snapshot := append([]string{}, base...)
	_ = buildAgentEnv(base, "another", testOmni, true)
	_ = buildAgentEnv(base, "another", testOmni, false)
	for i := range base {
		if base[i] != snapshot[i] {
			t.Fatalf("base foi mutado: %v", base)
		}
	}
}

func TestBuildAgentEnvAPIKeyWithoutAnyKey(t *testing.T) {
	// BE-E4: sem nenhuma chave => não inventa LLM_API_KEY vazia.
	got := buildAgentEnv([]string{"PATH=/bin"}, "api_key", testOmni, false)
	assertEnvAbsent(t, got, "LLM_API_KEY")
}

// ---------------------------------------------------------------------------
// Task 8 — models.env e READMEs (regra estrita de documentação)
// ---------------------------------------------------------------------------

func TestModelsEnvDeclaresAuthAndOmniRoute(t *testing.T) {
	env := parseEnvContent(readRepoFile(t, ".makercycle/config/models.env"))
	want := map[string]string{
		"MC_AUTH_MODE":         "api_key",
		"MC_OMNIROUTE_ENABLED": "false",
		"MC_OMNIROUTE_URL":     "http://localhost:20128",
		"MC_OMNIROUTE_API_KEY": "omniroute-local",
		"MC_OMNIROUTE_MODEL":   "claude-sonnet-4-6",
	}
	for k, v := range want {
		if got, ok := env[k]; !ok || got != v {
			t.Fatalf("models.env: %s = %q (presente=%v); want %q", k, got, ok, v)
		}
	}
	// As chaves de API residem exclusivamente em .makercycle/config/project.env (ver multi_llm_test.go).
	if _, ok := env["LLM_API_KEY"]; ok {
		t.Fatal("models.env não pode mais declarar LLM_API_KEY")
	}
}

func TestReadmesHaveNoForbiddenAuthTerms(t *testing.T) {
	for _, file := range []string{"README.md", "README.pt-BR.md"} {
		c := strings.ToLower(readRepoFile(t, file))
		for _, term := range []string{"oauth", "another"} {
			if strings.Contains(c, term) {
				t.Fatalf("%s não pode mencionar %q", file, term)
			}
		}
	}
}

func TestReadmesDocumentAPIKeyOmniRouteAndResume(t *testing.T) {
	for _, file := range []string{"README.md", "README.pt-BR.md"} {
		c := readRepoFile(t, file)
		for _, s := range []string{
			"ANTHROPIC_API_KEY", ".makercycle/config/project.env", ".makercycle/config/models.env", "OmniRoute",
			"MC_OMNIROUTE_ENABLED", "MC_OMNIROUTE_URL", "MC_OMNIROUTE_MODEL",
			"-resume", "-demand", "-stage", "RESUME.env",
		} {
			if !strings.Contains(c, s) {
				t.Fatalf("%s deve conter %q", file, s)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// ANTHROPIC_API_KEY chega ao subprocesso `claude` no modo api_key
// ---------------------------------------------------------------------------

func TestBuildAgentEnvInjectsAnthropicAPIKey(t *testing.T) {
	cases := []struct {
		id   string
		base []string
		want string
	}{
		{"AK-H1", []string{"PATH=/bin", "ANTHROPIC_API_KEY=", "MC_API_KEY=sk-mc"}, "sk-mc"},
		{"AK-H2", []string{"PATH=/bin", "MC_API_KEY=sk-mc"}, "sk-mc"},
		{"AK-H3", []string{"LLM_API_KEY=sk-llm", "ANTHROPIC_API_KEY=sk-ant"}, "sk-ant"},
		{"AK-E1", []string{"ANTHROPIC_API_KEY=old", "ANTHROPIC_API_KEY=old2", "MC_API_KEY=sk-mc"}, "old"},
		{"AK-E2", []string{"ANTHROPIC_API_KEY=  ", "MC_API_KEY=  \"sk-quoted\"  "}, "sk-quoted"},
		{"AK-E3", []string{"ANTHROPIC_API_KEY='sk-single'"}, "sk-single"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			snapshot := append([]string{}, c.base...)
			got := buildAgentEnv(c.base, "api_key", testOmni, false)
			assertEnvSingle(t, got, "ANTHROPIC_API_KEY", c.want)
			if strings.Join(snapshot, "\x00") != strings.Join(c.base, "\x00") {
				t.Fatalf("buildAgentEnv mutou base: %v -> %v", snapshot, c.base)
			}
		})
	}
}

func TestBuildAgentEnvNoAnthropicKeyWhenNoneConfigured(t *testing.T) {
	got := buildAgentEnv([]string{"PATH=/bin"}, "api_key", testOmni, false)
	assertEnvAbsent(t, got, "ANTHROPIC_API_KEY")
}

// A chave definida só em models.env (ausente do shell) chega ao cmd.Env do subprocesso.
func TestAgentEnvFromModelsEnvReachesSubprocess(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "models.env")
	if err := os.WriteFile(envFile, []byte("export MC_API_KEY=\"sk-from-file\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("MC_API_KEY", "")
	t.Setenv("MC_AUTH_MODE", "api_key")
	loadEnvFile(envFile)

	env := agentEnvFromProcess()
	assertEnvSingle(t, env, "ANTHROPIC_API_KEY", "sk-from-file")

	// Ponta a ponta: execCapture entrega a variável ao processo filho.
	out, err := execCapture("sh", []string{"-c", "printf '%s' \"$ANTHROPIC_API_KEY\""})
	if err != nil || out != "sk-from-file" {
		t.Fatalf("execCapture child ANTHROPIC_API_KEY = %q, %v", out, err)
	}
}

func TestAgentEnvFromProcessAnotherModeSkipsAnthropicKey(t *testing.T) {
	t.Setenv("MC_AUTH_MODE", "another")
	t.Setenv("MC_API_KEY", "sk-mc")
	t.Setenv("ANTHROPIC_API_KEY", "")
	assertEnvAbsent(t, agentEnvFromProcess(), "LLM_API_KEY")
	for _, v := range envValues(agentEnvFromProcess(), "ANTHROPIC_API_KEY") {
		if v != "" {
			t.Fatalf("modo another não deve injetar ANTHROPIC_API_KEY, obtido %q", v)
		}
	}
}
