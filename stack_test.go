package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFiles cria arquivos (e diretórios pais) relativos a root.
func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertStack(t *testing.T, got, want StackConfig) {
	t.Helper()
	if got != want {
		t.Fatalf("StackConfig divergente:\n got: %+v\nwant: %+v", got, want)
	}
}

var (
	wantGo = StackConfig{Name: "Go", TestCmd: "go test -v -race ./...", TypeCmd: "go vet ./..."}
	wantTS = StackConfig{Name: "TypeScript + Node.js", TestCmd: "npm test", TypeCmd: "npx tsc --noEmit"}
)

// ---------------------------------------------------------------------------
// Task 1 — detectStack
// ---------------------------------------------------------------------------

func TestDetectStackGo(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n\ngo 1.22\n"})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	assertStack(t, got, wantGo)
}

func TestDetectStackGoWithGolangci(t *testing.T) {
	for _, cfg := range []string{".golangci.yml", ".golangci.yaml"} {
		t.Run(cfg, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{"go.mod": "module x\n", cfg: "linters: {}\n"})
			got, ok := detectStack(root)
			if !ok {
				t.Fatal("esperava stack detectada")
			}
			want := wantGo
			want.LintCmd = "golangci-lint run"
			assertStack(t, got, want)
		})
	}
}

func TestDetectStackRailsWithSpec(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"Gemfile":             "source \"https://rubygems.org\"\ngem \"rails\", \"~> 7.1\"\n",
		"spec/spec_helper.rb": "",
	})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	assertStack(t, got, StackConfig{Name: "Ruby on Rails", TestCmd: "bundle exec rspec", LintCmd: "bundle exec rubocop"})
}

func TestDetectStackRailsWithoutSpec(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"Gemfile": "gem 'rails'\n"})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	assertStack(t, got, StackConfig{Name: "Ruby on Rails", TestCmd: "bin/rails test", LintCmd: "bundle exec rubocop"})
}

func TestDetectStackGemfileWithoutRailsIsNotDetected(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"Gemfile": "gem 'sinatra'\n"})
	got, ok := detectStack(root)
	if ok {
		t.Fatalf("Gemfile sem rails não deveria ser detectado, veio %+v", got)
	}
	assertStack(t, got, StackConfig{})
}

func TestDetectStackTypeScriptWithTsconfigAndLintScript(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json":  `{"scripts":{"test":"jest","lint":"eslint ."}}`,
		"tsconfig.json": "{}",
	})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	want := wantTS
	want.LintCmd = "npm run lint"
	assertStack(t, got, want)
}

func TestDetectStackTypeScriptViaDependency(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"package.json": `{"scripts":{"test":"vitest"},"devDependencies":{"typescript":"^5.4.0"}}`,
	})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	assertStack(t, got, wantTS)
}

func TestDetectStackNodeJavaScript(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"package.json": `{"scripts":{"test":"mocha","lint":"eslint ."}}`})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	assertStack(t, got, StackConfig{Name: "Node.js", TestCmd: "npm test", LintCmd: "npm run lint"})
}

func TestDetectStackLintOnlyFromScripts(t *testing.T) {
	root := t.TempDir()
	// "lint" aparece como valor e como chave fora de "scripts": não conta.
	writeFiles(t, root, map[string]string{"package.json": `{"name":"lint","config":{"lint":true},"scripts":{"test":"jest"}}`})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("esperava stack detectada")
	}
	if got.LintCmd != "" {
		t.Fatalf("LintCmd deveria ser vazio sem scripts.lint, veio %q", got.LintCmd)
	}
}

func TestDetectStackInvalidPackageJSON(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"package.json": "{invalid"})
	got, ok := detectStack(root)
	if !ok {
		t.Fatal("package.json inválido ainda indica projeto Node.js")
	}
	assertStack(t, got, StackConfig{Name: "Node.js", TestCmd: "npm test"})
}

func TestDetectStackPriorityGoOverRailsAndNode(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"go.mod":       "module x\n",
		"Gemfile":      "gem 'rails'\n",
		"package.json": `{"scripts":{"test":"jest"}}`,
	})
	got, _ := detectStack(root)
	if got.Name != "Go" {
		t.Fatalf("go.mod deve ter prioridade, veio %q", got.Name)
	}
}

func TestDetectStackPriorityRailsOverNode(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"Gemfile":      "gem 'rails'\n",
		"package.json": `{"scripts":{"test":"jest"}}`,
	})
	got, _ := detectStack(root)
	if got.Name != "Ruby on Rails" {
		t.Fatalf("Gemfile com rails deve ter prioridade sobre package.json, veio %q", got.Name)
	}
}

func TestDetectStackEmptyDir(t *testing.T) {
	got, ok := detectStack(t.TempDir())
	if ok {
		t.Fatalf("diretório vazio não deveria ser detectado, veio %+v", got)
	}
	assertStack(t, got, StackConfig{})
}

func TestDetectStackMissingRoot(t *testing.T) {
	got, ok := detectStack(filepath.Join(t.TempDir(), "nao-existe"))
	if ok {
		t.Fatalf("raiz inexistente não deveria ser detectada, veio %+v", got)
	}
	assertStack(t, got, StackConfig{})
}

func TestDetectStackIgnoresMarkerDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "go.mod"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := detectStack(root); ok {
		t.Fatal("go.mod como diretório não deveria indicar projeto Go")
	}
}

// ---------------------------------------------------------------------------
// Task 2 — loadProjectStack
// ---------------------------------------------------------------------------

func writeEnv(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "project.env")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadProjectStackHappyPath(t *testing.T) {
	p := writeEnv(t, "MC_DOCS_DIR=docs\n"+
		"MC_STACK_NAME=\"NestJS + Jest\"\n"+
		"MC_TEST_CMD=\"npm run test\"\n"+
		"MC_TYPE_CMD=\"npx tsc --noEmit\"\n"+
		"MC_LINT_CMD=\"npm run lint\"\n")
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, StackConfig{Name: "NestJS + Jest", TestCmd: "npm run test", TypeCmd: "npx tsc --noEmit", LintCmd: "npm run lint"})
}

func TestLoadProjectStackQuotesCommentsAndCRLF(t *testing.T) {
	p := writeEnv(t, "# gerado pelo -setup\r\n\r\n"+
		"MC_STACK_NAME='Go'\r\n"+
		"  MC_TEST_CMD = go test ./...  \r\n"+
		"MC_LINT_CMD=\r\n")
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, StackConfig{Name: "Go", TestCmd: "go test ./..."})
}

func TestLoadProjectStackOptionalKeysAbsent(t *testing.T) {
	p := writeEnv(t, "MC_STACK_NAME=Python\nMC_TEST_CMD=pytest\n")
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, StackConfig{Name: "Python", TestCmd: "pytest"})
}

func TestLoadProjectStackValueWithEquals(t *testing.T) {
	p := writeEnv(t, "MC_STACK_NAME=Go\nMC_TEST_CMD=\"go test -run=TestX ./...\"\n")
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.TestCmd != "go test -run=TestX ./..." {
		t.Fatalf("valor com '=' truncado: %q", got.TestCmd)
	}
}

func TestLoadProjectStackDuplicateKeyLastWins(t *testing.T) {
	p := writeEnv(t, "MC_STACK_NAME=Antigo\nMC_TEST_CMD=a\nMC_STACK_NAME=Novo\n")
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.Name != "Novo" {
		t.Fatalf("última ocorrência deveria vencer, veio %q", got.Name)
	}
}

func TestLoadProjectStackMalformedLinesIgnored(t *testing.T) {
	p := writeEnv(t, "lixo sem igual\nMC_STACK_NAME=Go\n===\nMC_TEST_CMD=go test ./...\n")
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("linhas sem chave válida devem ser ignoradas: %v", err)
	}
	assertStack(t, got, StackConfig{Name: "Go", TestCmd: "go test ./..."})
}

func TestLoadProjectStackMissingFile(t *testing.T) {
	_, err := loadProjectStack(filepath.Join(t.TempDir(), "project.env"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("esperava erro fs.ErrNotExist, veio %v", err)
	}
}

func TestLoadProjectStackPathIsDirectory(t *testing.T) {
	if _, err := loadProjectStack(t.TempDir()); err == nil {
		t.Fatal("diretório no lugar do arquivo deveria retornar erro")
	}
}

func TestLoadProjectStackIncomplete(t *testing.T) {
	cases := map[string]string{
		"sem nome":           "MC_DOCS_DIR=docs\nMC_TEST_CMD=go test ./...\n",
		"sem teste":          "MC_STACK_NAME=Go\n",
		"teste vazio":        "MC_STACK_NAME=Go\nMC_TEST_CMD=\"\"\n",
		"nome só com espaço": "MC_STACK_NAME=\"   \"\nMC_TEST_CMD=x\n",
		"arquivo vazio":      "",
		"só lixo":            "%%%\n???\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := loadProjectStack(writeEnv(t, content))
			if !errors.Is(err, errProjectStackIncomplete) {
				t.Fatalf("esperava errProjectStackIncomplete, veio %v", err)
			}
			assertStack(t, got, StackConfig{})
		})
	}
}

func TestErrProjectStackIncompleteMentionsKeys(t *testing.T) {
	msg := errProjectStackIncomplete.Error()
	if !strings.Contains(msg, "MC_STACK_NAME") || !strings.Contains(msg, "MC_TEST_CMD") {
		t.Fatalf("mensagem deve citar as chaves obrigatórias: %q", msg)
	}
}
