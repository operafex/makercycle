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
// Task 1 — isGreenfieldProject(dir) (presets.go)
// Referência: docs/references/setup-ux-and-db-prompt.md §2
// ---------------------------------------------------------------------------

// GF-H1/E1..E6/R1: projeto novo = sem código-fonte e sem manifesto de linguagem fora das áreas ignoradas.
func TestIsGreenfieldProject(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"vazio", nil, true},
		{"só arquivos ignorados", map[string]string{
			"README.md": "# x\n", "README.pt-BR.md": "# x\n", ".gitignore": "bin/\n", "LICENSE": "Apache\n",
			".git/HEAD": "ref: refs/heads/main\n", ".makercycle/config/project.env": "MC_DOCS_DIR=\"docs\"\n",
			"docs/PROJECT_CONTEXT.md": "# ctx\n", "docs/demands/INDEX.md": "| a |\n",
		}, true},
		{"código dentro de docs/ é ignorado", map[string]string{"docs/exemplo.go": "package x\n"}, true},
		{"código dentro de .makercycle/ é ignorado", map[string]string{".makercycle/agents/x.py": "print()\n"}, true},
		{"arquivo que não é código nem manifesto", map[string]string{"notas.txt": "ideias\n"}, true},
		{"código na raiz", map[string]string{"main.go": "package main\n"}, false},
		{"código em subpasta", map[string]string{"cmd/app/main.go": "package main\n"}, false},
		{"código python", map[string]string{"app/server.py": "print()\n"}, false},
		{"manifesto go.mod", map[string]string{"go.mod": "module x\n"}, false},
		{"manifesto package.json", map[string]string{"package.json": "{}\n"}, false},
		{"manifesto Cargo.toml", map[string]string{"Cargo.toml": "[package]\n"}, false},
		{"manifesto pyproject.toml", map[string]string{"pyproject.toml": "[project]\n"}, false},
		{"manifesto requirements.txt", map[string]string{"requirements.txt": "flask\n"}, false},
		{"manifesto Gemfile", map[string]string{"Gemfile": "source 'x'\n"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, c.files)
			if got := isGreenfieldProject(root); got != c.want {
				t.Errorf("isGreenfieldProject = %v, want %v (arquivos: %v)", got, c.want, c.files)
			}
		})
	}
}

// GF-E7: diretório inexistente não tem código => novo (não falha nem entra em pânico).
func TestIsGreenfieldProjectMissingDir(t *testing.T) {
	if !isGreenfieldProject(filepath.Join(t.TempDir(), "nao-existe")) {
		t.Error("diretório inexistente deve ser tratado como projeto novo")
	}
}

// ---------------------------------------------------------------------------
// Task 2 — stackPresets, presetEnvValues, selectStackPreset (presets.go)
// ---------------------------------------------------------------------------

// PR-H1: os 5 presets prontos, na ordem e com os comandos exatos da referência.
func TestStackPresetsContract(t *testing.T) {
	want := []StackPreset{
		{Name: "Go", BuildCmd: "go build ./...", TestCmd: "go test -race ./...", LintCmd: "go vet ./..."},
		{Name: "Node.js / TypeScript (Backend API)", BuildCmd: "npm run build", TestCmd: "npm test", LintCmd: "npm run lint"},
		{Name: "Node.js / TypeScript (Frontend)", BuildCmd: "npm run build", TestCmd: "npm test", LintCmd: "npm run lint"},
		{Name: "Python", BuildCmd: "python -m py_compile .", TestCmd: "pytest", LintCmd: "ruff check ."},
		{Name: "Rust", BuildCmd: "cargo build", TestCmd: "cargo test", LintCmd: "cargo clippy"},
	}
	if len(stackPresets) != len(want) {
		t.Fatalf("stackPresets tem %d itens, want %d", len(stackPresets), len(want))
	}
	for i := range want {
		if stackPresets[i] != want[i] {
			t.Errorf("preset %d = %+v, want %+v", i+1, stackPresets[i], want[i])
		}
	}
}

// PR-H2: o preset vira StackConfig com MC_BUILD_CMD no papel de MC_TYPE_CMD (executado na Etapa 4A).
func TestStackPresetToStackConfig(t *testing.T) {
	p := StackPreset{Name: "Rust", BuildCmd: "cargo build", TestCmd: "cargo test", LintCmd: "cargo clippy"}
	assertStack(t, p.StackConfig(), StackConfig{Name: "Rust", TestCmd: "cargo test", TypeCmd: "cargo build", LintCmd: "cargo clippy"})
}

// PR-H3: chaves gravadas em project.env (MC_STACK_NAME = MC_STACK da referência).
func TestPresetEnvValues(t *testing.T) {
	got := presetEnvValues(stackPresets[0])
	want := map[string]string{
		"MC_STACK_NAME": "Go",
		"MC_BUILD_CMD":  "go build ./...",
		"MC_TYPE_CMD":   "go build ./...",
		"MC_TEST_CMD":   "go test -race ./...",
		"MC_LINT_CMD":   "go vet ./...",
	}
	if len(got) != len(want) {
		t.Fatalf("presetEnvValues = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func runPresetMenu(t *testing.T, input string) (StackPreset, bool, string, error) {
	t.Helper()
	var out bytes.Buffer
	p, ok, err := selectStackPreset(bufio.NewReader(strings.NewReader(input)), &out)
	return p, ok, out.String(), err
}

// PR-H4: menu com as 6 opções exatas e escolha de cada preset pronto.
func TestSelectStackPresetMenuAndChoices(t *testing.T) {
	for i, want := range stackPresets {
		input := string(rune('1'+i)) + "\n"
		p, ok, out, err := runPresetMenu(t, input)
		if err != nil || !ok {
			t.Fatalf("opção %q: ok=%v err=%v", input, ok, err)
		}
		if p != want {
			t.Errorf("opção %q = %+v, want %+v", input, p, want)
		}
		for _, s := range []string{
			stackPresetMenuTitle,
			"[1] Go", "[2] Node.js / TypeScript (Backend API)", "[3] Node.js / TypeScript (Frontend)",
			"[4] Python", "[5] Rust", "[6] Personalizado", "(1-6)",
		} {
			if !strings.Contains(out, s) {
				t.Fatalf("menu de presets sem %q:\n%s", s, out)
			}
		}
	}
}

// PR-H5: Personalizado pergunta MC_STACK, MC_BUILD_CMD, MC_TEST_CMD e MC_LINT_CMD, nessa ordem.
func TestSelectStackPresetCustom(t *testing.T) {
	p, ok, out, err := runPresetMenu(t, "6\nElixir\nmix compile\nmix test\nmix credo\n")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := StackPreset{Name: "Elixir", BuildCmd: "mix compile", TestCmd: "mix test", LintCmd: "mix credo"}
	if p != want {
		t.Errorf("personalizado = %+v, want %+v", p, want)
	}
	for _, s := range []string{"MC_STACK", "MC_BUILD_CMD", "MC_TEST_CMD", "MC_LINT_CMD"} {
		if !strings.Contains(out, s) {
			t.Errorf("pergunta do personalizado sem %q:\n%s", s, out)
		}
	}
}

// PR-E1: Personalizado sem nome vira "Personalizado"; build e lint são opcionais.
func TestSelectStackPresetCustomDefaults(t *testing.T) {
	p, ok, _, err := runPresetMenu(t, "6\n\n\nmake test\n\n")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	want := StackPreset{Name: "Personalizado", TestCmd: "make test"}
	if p != want {
		t.Errorf("personalizado = %+v, want %+v", p, want)
	}
}

// PR-R1: Personalizado sem comando de testes é erro.
func TestSelectStackPresetCustomRequiresTestCmd(t *testing.T) {
	if _, ok, _, err := runPresetMenu(t, "6\nX\nb\n\n\n"); err == nil || ok {
		t.Fatalf("sem MC_TEST_CMD deve falhar: ok=%v err=%v", ok, err)
	}
}

// PR-E2: Enter, espaços ou EOF = nenhum preset escolhido (sem erro), preservando o -setup não-interativo.
func TestSelectStackPresetEnterOrEOFSkips(t *testing.T) {
	for name, input := range map[string]string{"enter": "\n", "eof": "", "espaços": "   \n"} {
		t.Run(name, func(t *testing.T) {
			p, ok, _, err := runPresetMenu(t, input)
			if err != nil || ok || p != (StackPreset{}) {
				t.Errorf("entrada %q: p=%+v ok=%v err=%v", input, p, ok, err)
			}
		})
	}
}

// PR-R2: opção fora de 1..6 é erro com a faixa válida.
func TestSelectStackPresetInvalid(t *testing.T) {
	for _, input := range []string{"0\n", "7\n", "go\n"} {
		_, ok, _, err := runPresetMenu(t, input)
		if err == nil || ok || !strings.Contains(err.Error(), "1 e 6") {
			t.Errorf("entrada %q: ok=%v err=%v", input, ok, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Task 3 — confirmSetupStack com presets (stack.go)
// ---------------------------------------------------------------------------

// PR-H6: sem stack detectada (projeto novo) o menu de presets é exibido e a escolha vai para project.env.
func TestConfirmSetupStackGreenfieldShowsPresets(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".makercycle", "config", "project.env")
	s, ok, out, err := runConfirm(t, root, envPath, "1\n")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v\n%s", ok, err, out)
	}
	assertStack(t, s, StackConfig{Name: "Go", TestCmd: "go test -race ./...", TypeCmd: "go build ./...", LintCmd: "go vet ./..."})
	if !strings.Contains(out, stackPresetMenuTitle) {
		t.Errorf("menu de presets não exibido:\n%s", out)
	}
	if strings.Contains(out, setupConfirmPrompt) {
		t.Errorf("sem stack detectada não há o que confirmar:\n%s", out)
	}
	env := parseEnvContent(readString(t, envPath))
	for k, v := range map[string]string{
		"MC_DOCS_DIR": "docs", "MC_STACK_NAME": "Go", "MC_BUILD_CMD": "go build ./...",
		"MC_TYPE_CMD": "go build ./...", "MC_TEST_CMD": "go test -race ./...", "MC_LINT_CMD": "go vet ./...",
	} {
		if env[k] != v {
			t.Errorf("project.env %s = %q, want %q", k, env[k], v)
		}
	}
	if saved, err := loadProjectStack(envPath); err != nil || saved.Name != "Go" {
		t.Errorf("stack do preset deve ser carregável: %+v err=%v", saved, err)
	}
}

// PR-H7: stack detectada + 'p' abre o menu de presets (não mais o menu de execução).
func TestConfirmSetupStackDetectedCustomizeUsesPresets(t *testing.T) {
	root, envPath := goProjectRoot(t)
	s, ok, out, err := runConfirm(t, root, envPath, "p\n5\n")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if s.Name != "Rust" || s.TypeCmd != "cargo build" {
		t.Errorf("stack = %+v, want preset Rust", s)
	}
	if !strings.Contains(out, stackPresetMenuTitle) {
		t.Errorf("'p' deve abrir o menu de presets:\n%s", out)
	}
	if env := parseEnvContent(readString(t, envPath)); env["MC_BUILD_CMD"] != "cargo build" || env["MC_STACK_NAME"] != "Rust" {
		t.Errorf("preset Rust não gravado: %v", env)
	}
}

// PR-E3: stack detectada + 'p' + Enter no menu de presets mantém a stack detectada.
func TestConfirmSetupStackDetectedCustomizeEnterKeepsDetected(t *testing.T) {
	root, envPath := goProjectRoot(t)
	s, ok, _, err := runConfirm(t, root, envPath, "p\n\n")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	assertStack(t, s, wantGo)
	if saved, _ := loadProjectStack(envPath); saved.Name != "Go" {
		t.Errorf("stack detectada deve ser gravada: %+v", saved)
	}
}

// PR-R3: opção de preset inválida não grava project.env.
func TestConfirmSetupStackGreenfieldInvalidPresetWritesNothing(t *testing.T) {
	root := t.TempDir()
	envPath := filepath.Join(root, ".makercycle", "config", "project.env")
	if _, ok, _, err := runConfirm(t, root, envPath, "9\n"); err == nil || ok {
		t.Fatalf("preset inválido deve falhar: ok=%v err=%v", ok, err)
	}
	if _, err := os.Stat(envPath); err == nil {
		t.Error("project.env não deve ser gravado após preset inválido")
	}
}

// PR-E4: projeto sem manifesto detectável mas com código (ex.: Python) também recebe o menu de presets.
func TestConfirmSetupStackUndetectedWithCodeShowsPresets(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"app.py": "print()\n"})
	envPath := filepath.Join(root, ".makercycle", "config", "project.env")
	s, ok, out, err := runConfirm(t, root, envPath, "4\n")
	if err != nil || !ok || s.Name != "Python" || s.TestCmd != "pytest" {
		t.Fatalf("s=%+v ok=%v err=%v\n%s", s, ok, err, out)
	}
}
