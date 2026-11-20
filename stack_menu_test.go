package main

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 5 — selectStack(reader, out, envPath)
// ---------------------------------------------------------------------------

const nestEnv = "MC_STACK_NAME=\"NestJS + Jest\"\nMC_TEST_CMD=\"npm run test\"\nMC_LINT_CMD=\"npm run lint\"\n"

var wantNest = StackConfig{Name: "NestJS + Jest", TestCmd: "npm run test", LintCmd: "npm run lint"}

func runSelect(t *testing.T, envPath, input string) (StackConfig, string, error) {
	t.Helper()
	var out bytes.Buffer
	got, err := selectStack(bufio.NewReader(strings.NewReader(input)), &out, envPath)
	return got, out.String(), err
}

func TestSelectStackDetectedMenu(t *testing.T) {
	chdirTemp(t)
	got, out, err := runSelect(t, writeEnv(t, nestEnv), "1\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, wantNest)
	for _, s := range []string{
		"[1] Projeto Atual (Auto-detectado): NestJS + Jest",
		"[2] Ruby on Rails", "[3] Go", "[4] TypeScript + Node.js", "[5] Personalizada",
		"(1-5)",
	} {
		if !strings.Contains(out, s) {
			t.Fatalf("menu sem %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "[AVISO]") {
		t.Fatalf("não deve haver aviso com stack válida:\n%s", out)
	}
}

func TestSelectStackDetectedEnterAndEOFDefaultToOption1(t *testing.T) {
	for name, input := range map[string]string{"enter": "\n", "eof": "", "espaços": "  \n"} {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			got, _, err := runSelect(t, writeEnv(t, nestEnv), input)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			assertStack(t, got, wantNest)
		})
	}
}

func TestSelectStackDetectedShiftedPresets(t *testing.T) {
	cases := map[string]string{"2\n": "Ruby on Rails", "3\n": "Go", "4\n": "TypeScript + Node.js"}
	for input, name := range cases {
		t.Run(name, func(t *testing.T) {
			chdirTemp(t)
			got, _, err := runSelect(t, writeEnv(t, nestEnv), input)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if got.Name != name {
				t.Fatalf("opção %q deveria escolher %q, veio %q", strings.TrimSpace(input), name, got.Name)
			}
		})
	}
}

func TestSelectStackPresetGoMatchesDetectStack(t *testing.T) {
	chdirTemp(t)
	got, _, err := runSelect(t, writeEnv(t, nestEnv), "3\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, wantGo)
}

func TestSelectStackDetectedCustom(t *testing.T) {
	chdirTemp(t)
	got, _, err := runSelect(t, writeEnv(t, nestEnv), "5\npytest\nmypy .\nruff check\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, StackConfig{Name: "Personalizada", TestCmd: "pytest", TypeCmd: "mypy .", LintCmd: "ruff check"})
}

func TestSelectStackDetectedInvalidOption(t *testing.T) {
	for _, input := range []string{"9\n", "0\n", "abc\n", "6\n"} {
		chdirTemp(t)
		got, _, err := runSelect(t, writeEnv(t, nestEnv), input)
		if err == nil {
			t.Fatalf("opção %q deveria retornar erro", strings.TrimSpace(input))
		}
		if !strings.Contains(err.Error(), "1 e 5") {
			t.Fatalf("erro deve indicar o intervalo 1 e 5: %v", err)
		}
		assertStack(t, got, StackConfig{})
	}
}

func TestSelectStackWithoutEnvShowsWarningAndManualMenu(t *testing.T) {
	dir := chdirTemp(t)
	got, out, err := runSelect(t, dir+"/nao-existe.env", "2\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, wantGo)
	for _, s := range []string{
		"[AVISO] Não foi possível carregar a stack do projeto (motivo:",
		"Por favor, selecione manualmente:",
		"[1] Ruby on Rails", "[2] Go", "[3] TypeScript + Node.js", "[4] Personalizada", "(1-4)",
	} {
		if !strings.Contains(out, s) {
			t.Fatalf("saída sem %q:\n%s", s, out)
		}
	}
	if strings.Contains(out, "Auto-detectado") || strings.Contains(out, "[5]") {
		t.Fatalf("menu manual não deve ter opção auto-detectada:\n%s", out)
	}
}

func TestSelectStackIncompleteEnvWarningShowsReason(t *testing.T) {
	chdirTemp(t)
	_, out, err := runSelect(t, writeEnv(t, "MC_DOCS_DIR=docs\n"), "2\n")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !strings.Contains(out, "[AVISO]") || !strings.Contains(out, "MC_STACK_NAME") {
		t.Fatalf("aviso deve conter o motivo (chaves ausentes):\n%s", out)
	}
}

func TestSelectStackWithoutEnvInvalidOptions(t *testing.T) {
	for _, input := range []string{"\n", "", "5\n", "x\n"} {
		dir := chdirTemp(t)
		_, _, err := runSelect(t, dir+"/nao-existe.env", input)
		if err == nil {
			t.Fatalf("opção %q sem stack detectada deveria retornar erro", strings.TrimSpace(input))
		}
		if !strings.Contains(err.Error(), "1 e 4") {
			t.Fatalf("erro deve indicar o intervalo 1 e 4: %v", err)
		}
	}
}

func TestSelectStackCustomRequiresTestCommand(t *testing.T) {
	dir := chdirTemp(t)
	if _, _, err := runSelect(t, dir+"/nao-existe.env", "4\n\n\n\n"); err == nil {
		t.Fatal("stack personalizada sem comando de testes deve retornar erro")
	}
}

// ---------------------------------------------------------------------------
// Tasks 6 e 7 — contrato do SCOUT, READMEs e dica de build multi-arquivo
// ---------------------------------------------------------------------------

func readRepoFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lendo %s: %v", path, err)
	}
	return string(b)
}

func TestScoutContractDocumentsStackKeys(t *testing.T) {
	c := readRepoFile(t, ".makercycle/agents/00_SCOUT.md")
	for _, s := range []string{".makercycle/config/project.env", "MC_STACK_NAME", "MC_TEST_CMD", "MC_TYPE_CMD", "MC_LINT_CMD"} {
		if !strings.Contains(c, s) {
			t.Fatalf("00_SCOUT.md deve mencionar %q", s)
		}
	}
	if !strings.Contains(c, "MC_0_DONE") {
		t.Fatal("handoff MC_0_DONE não pode ser removido")
	}
}

func TestReadmesDocumentAutoStack(t *testing.T) {
	cases := map[string]string{
		"README.md":       "### Auto-Detected & Custom Stacks",
		"README.pt-BR.md": "### Detecção Automática e Personalização de Stack",
	}
	for file, heading := range cases {
		t.Run(file, func(t *testing.T) {
			c := readRepoFile(t, file)
			for _, s := range []string{heading, "MC_STACK_NAME", "MC_TEST_CMD", "MC_TYPE_CMD", "MC_LINT_CMD",
				"Auto-detectado", "go build -o makercycle ."} {
				if !strings.Contains(c, s) {
					t.Fatalf("%s deve conter %q", file, s)
				}
			}
			setupIdx := strings.Index(c, "-setup` (")
			headIdx := strings.Index(c, heading)
			taskIdx := strings.Index(c, "-task` (")
			if setupIdx < 0 || taskIdx < 0 || !(setupIdx < headIdx && headIdx < taskIdx) {
				t.Fatalf("%s: a seção deve ficar entre a seção -setup e a seção -task", file)
			}
		})
	}
}

func TestNoSingleFileGoCommands(t *testing.T) {
	for _, file := range []string{"README.md", "README.pt-BR.md", "makercycle.go"} {
		c := readRepoFile(t, file)
		for _, s := range []string{"go run makercycle.go", "go build -o makercycle makercycle.go"} {
			if strings.Contains(c, s) {
				t.Fatalf("%s ainda usa %q, que não compila stack.go/version.go; use `go run .` / `go build -o makercycle .`", file, s)
			}
		}
	}
}
