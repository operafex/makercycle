package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 9 — confirmSetupStack (-setup) e resolveRunStack (-task/-stage/-demand)
// Já implementados em stack.go: estes testes travam o contrato da referência
// docs/references/autonomous-spec-telemetry-stack.md §3 (regressão, não RED).
// ---------------------------------------------------------------------------

const setupConfirmPrompt = "Confirmar esta stack como padrão do projeto? [S/n] (ou digite 'p' para personalizar):"

func goProjectRoot(t *testing.T) (root, envPath string) {
	t.Helper()
	root = t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n\ngo 1.22\n"})
	return root, filepath.Join(root, ".makercycle", "config", "project.env")
}

func runConfirm(t *testing.T, root, envPath, input string) (StackConfig, bool, string, error) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(envPath), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	s, ok, err := confirmSetupStack(bufio.NewReader(strings.NewReader(input)), &out, envPath, root, "docs", "")
	return s, ok, out.String(), err
}

func TestConfirmSetupStackDefaultsConfirm(t *testing.T) {
	for _, input := range []string{"\n", "S\n", "s\n", ""} {
		root, envPath := goProjectRoot(t)
		s, ok, out, err := runConfirm(t, root, envPath, input)
		if err != nil || !ok {
			t.Fatalf("entrada %q: ok=%v err=%v", input, ok, err)
		}
		if s.Name != "Go" {
			t.Errorf("entrada %q: stack = %q, esperava Go", input, s.Name)
		}
		if !strings.Contains(out, "[STACK] Detectada: Go") || !strings.Contains(out, setupConfirmPrompt) {
			t.Errorf("entrada %q: pergunta exata ausente:\n%s", input, out)
		}
		saved, err := loadProjectStack(envPath)
		if err != nil || saved.Name != "Go" {
			t.Errorf("entrada %q: project.env deve conter a stack Go: %+v err=%v", input, saved, err)
		}
		env := parseEnvContent(readString(t, envPath))
		if env["MC_DOCS_DIR"] != "docs" {
			t.Errorf("entrada %q: MC_DOCS_DIR deve ser gravado: %v", input, env)
		}
	}
}

func TestConfirmSetupStackCustomizeOpensMenu(t *testing.T) {
	for _, first := range []string{"p", "P", "n", "N"} {
		root, envPath := goProjectRoot(t)
		// project.env ainda não existe => menu manual; opção 1 = Ruby on Rails
		s, ok, out, err := runConfirm(t, root, envPath, first+"\n1\n")
		if err != nil || !ok {
			t.Fatalf("entrada %q: ok=%v err=%v", first, ok, err)
		}
		if s.Name != "Ruby on Rails" {
			t.Errorf("entrada %q: stack = %q, esperava a escolhida no menu", first, s.Name)
		}
		if !strings.Contains(out, "Qual stack deve ser utilizada nesta execução?") {
			t.Errorf("entrada %q: menu não exibido:\n%s", first, out)
		}
		saved, _ := loadProjectStack(envPath)
		if saved.Name != "Ruby on Rails" {
			t.Errorf("entrada %q: escolha do menu deve ser gravada em project.env: %+v", first, saved)
		}
	}
}

func TestConfirmSetupStackInvalidAnswerWritesNothing(t *testing.T) {
	root, envPath := goProjectRoot(t)
	_, ok, _, err := runConfirm(t, root, envPath, "talvez\n")
	if err == nil || ok {
		t.Fatalf("resposta inválida deve retornar erro: ok=%v err=%v", ok, err)
	}
	if _, statErr := os.Stat(envPath); statErr == nil {
		t.Errorf("project.env não deve ser gravado após resposta inválida")
	}
}

func TestConfirmSetupStackWithoutDetectionDoesNotAsk(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, "project.env")
	s, ok, out, err := runConfirm(t, root, envPath, "")
	if err != nil || ok || s.Name != "" {
		t.Fatalf("sem stack detectada: s=%+v ok=%v err=%v", s, ok, err)
	}
	if strings.Contains(out, setupConfirmPrompt) {
		t.Errorf("sem stack detectada não deve haver pergunta:\n%s", out)
	}
	env := parseEnvContent(readString(t, envPath))
	if env["MC_DOCS_DIR"] != "docs" || env["MC_STACK_NAME"] != "" {
		t.Errorf("deve gravar só as chaves de documentação: %v", env)
	}
}

func runResolve(t *testing.T, envPath, input string, force bool) (StackConfig, string, error) {
	t.Helper()
	var out bytes.Buffer
	s, err := resolveRunStack(bufio.NewReader(strings.NewReader(input)), &out, envPath, force)
	return s, out.String(), err
}

func TestResolveRunStackValidEnvIsSilent(t *testing.T) {
	envPath := writeEnv(t, nestEnv)
	s, out, err := runResolve(t, envPath, "", false)
	if err != nil {
		t.Fatal(err)
	}
	assertStack(t, s, wantNest)
	if out != "[INFO] Stack do projeto: NestJS + Jest (via project.env)\n" {
		t.Errorf("saída = %q, esperava apenas a linha [INFO]", out)
	}
}

func TestResolveRunStackForceSelectOpensMenuAndSaves(t *testing.T) {
	envPath := writeEnv(t, nestEnv)
	s, out, err := runResolve(t, envPath, "3\n", true) // [3] Go no menu com stack detectada
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Go" {
		t.Errorf("stack = %q, esperava Go", s.Name)
	}
	if strings.Contains(out, "[INFO] Stack do projeto") || !strings.Contains(out, "Qual stack deve ser utilizada nesta execução?") {
		t.Errorf("-select-stack deve abrir o menu sem a linha [INFO]:\n%s", out)
	}
	saved, _ := loadProjectStack(envPath)
	if saved.Name != "Go" {
		t.Errorf("escolha deve ser salva em project.env: %+v", saved)
	}
}

func TestResolveRunStackMissingOrIncompleteEnvOpensMenu(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "project.env")
	incomplete := writeEnv(t, "MC_STACK_NAME=\"Go\"\n")
	for _, envPath := range []string{missing, incomplete} {
		s, out, err := runResolve(t, envPath, "2\n", false) // menu manual: [2] Go
		if err != nil {
			t.Fatalf("%s: %v", envPath, err)
		}
		if s.Name != "Go" || !strings.Contains(out, "[AVISO] Não foi possível carregar a stack do projeto") {
			t.Errorf("%s: stack=%q saída:\n%s", envPath, s.Name, out)
		}
		saved, _ := loadProjectStack(envPath)
		if saved.Name != "Go" {
			t.Errorf("%s: escolha deve ser salva: %+v", envPath, saved)
		}
	}
}

func TestResolveRunStackSaveFailureOnlyWarns(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "arquivo") // um arquivo onde deveria haver um diretório
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	envPath := filepath.Join(blocker, "project.env")
	s, out, err := runResolve(t, envPath, "2\n", false)
	if err != nil {
		t.Fatalf("falha ao salvar não deve ser erro: %v", err)
	}
	if s.Name != "Go" {
		t.Errorf("stack = %q", s.Name)
	}
	if !strings.Contains(out, "[AVISO] Não foi possível salvar a stack em "+envPath) {
		t.Errorf("esperava aviso de gravação:\n%s", out)
	}
}

func TestResolveRunStackInvalidChoiceIsError(t *testing.T) {
	envPath := writeEnv(t, nestEnv)
	if _, _, err := runResolve(t, envPath, "9\n", true); err == nil {
		t.Errorf("opção inválida no menu deve retornar erro")
	}
}

func TestMainDeclaresSelectStackFlag(t *testing.T) {
	src := readRepoFile(t, "makercycle.go")
	for _, want := range []string{
		`flag.Bool("select-stack", false,`,
		"resolveRunStack(reader, os.Stdout, projectEnvPath, *selectStackFlag)",
		"confirmSetupStack(reader, os.Stdout, projectEnvPath,",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("makercycle.go deve conter %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 10 — READMEs (EN/PT-BR): loop de especificação, TELEMETRY.md e -select-stack
// ---------------------------------------------------------------------------

func TestReadmesDocumentSpecLoopTelemetryAndSelectStack(t *testing.T) {
	want := map[string][]string{
		"README.md": {
			"### Autonomous specification loop",
			"[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)...",
			"@.makercycle/state/SPEC_REVIEW.md",
			"MC_MAX_ATTEMPTS",
			"TELEMETRY.md",
			"| Data | RunID | Status | Demanda | Stack | Tentativas | Duração | Tokens Totais |",
			"-select-stack",
		},
		"README.pt-BR.md": {
			"### Loop autônomo de especificação",
			"[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)...",
			"@.makercycle/state/SPEC_REVIEW.md",
			"MC_MAX_ATTEMPTS",
			"TELEMETRY.md",
			"| Data | RunID | Status | Demanda | Stack | Tentativas | Duração | Tokens Totais |",
			"-select-stack",
		},
	}
	stale := map[string]string{
		"README.md":       "Stages 1 and 2 run once",
		"README.pt-BR.md": "As Etapas 1 e 2 rodam uma única vez",
	}
	for file, items := range want {
		content := readRepoFile(t, file)
		for _, w := range items {
			if !strings.Contains(content, w) {
				t.Errorf("%s deve conter %q", file, w)
			}
		}
		if strings.Contains(content, stale[file]) {
			t.Errorf("%s ainda afirma que as Etapas 1 e 2 rodam uma única vez", file)
		}
	}
}
