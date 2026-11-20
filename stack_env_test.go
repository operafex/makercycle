package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 3 — stackEnvValues / writeProjectEnv
// ---------------------------------------------------------------------------

func TestStackEnvValues(t *testing.T) {
	got := stackEnvValues(StackConfig{Name: "Go", TestCmd: "go test ./...", TypeCmd: "go vet ./...", LintCmd: ""})
	want := map[string]string{
		"MC_STACK_NAME": "Go",
		"MC_TEST_CMD":   "go test ./...",
		"MC_TYPE_CMD":   "go vet ./...",
		"MC_LINT_CMD":   "",
	}
	if len(got) != len(want) {
		t.Fatalf("esperava %d chaves, veio %d: %v", len(want), len(got), got)
	}
	for k, v := range want {
		if gv, ok := got[k]; !ok || gv != v {
			t.Fatalf("chave %s: esperava %q, veio %q (presente=%v)", k, v, gv, ok)
		}
	}
}

func TestWriteProjectEnvCreatesFileAndParents(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".makercycle", "config", "project.env")
	err := writeProjectEnv(p, map[string]string{"MC_TEST_CMD": "go test ./...", "MC_STACK_NAME": "Go"})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := "MC_STACK_NAME=\"Go\"\nMC_TEST_CMD=\"go test ./...\"\n"
	if got := readString(t, p); got != want {
		t.Fatalf("conteúdo divergente:\n got: %q\nwant: %q", got, want)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	// project.env guarda chaves de API: somente o dono pode ler/escrever.
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissão esperada 0600, veio %v", info.Mode().Perm())
	}
}

func TestWriteProjectEnvMergesPreservingOtherLines(t *testing.T) {
	p := writeEnv(t, "# config do projeto\nMC_DOCS_DIR=docs\n\nMC_STACK_NAME=Antigo\nMEU_TOKEN_CUSTOM=abc\n")
	err := writeProjectEnv(p, map[string]string{
		"MC_STACK_NAME": "Go",
		"MC_TEST_CMD":   "go test ./...",
		"MC_LINT_CMD":   "",
	})
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := "# config do projeto\nMC_DOCS_DIR=docs\n\nMC_STACK_NAME=\"Go\"\nMEU_TOKEN_CUSTOM=abc\n" +
		"MC_LINT_CMD=\"\"\nMC_TEST_CMD=\"go test ./...\"\n"
	if got := readString(t, p); got != want {
		t.Fatalf("merge divergente:\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteProjectEnvCollapsesDuplicateKeys(t *testing.T) {
	p := writeEnv(t, "MC_STACK_NAME=A\nX=1\nMC_STACK_NAME=B\n")
	if err := writeProjectEnv(p, map[string]string{"MC_STACK_NAME": "Go"}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := "MC_STACK_NAME=\"Go\"\nX=1\n"
	if got := readString(t, p); got != want {
		t.Fatalf("duplicatas não removidas:\n got: %q\nwant: %q", got, want)
	}
}

func TestWriteProjectEnvAddsTrailingNewline(t *testing.T) {
	p := writeEnv(t, "MC_DOCS_DIR=docs")
	if err := writeProjectEnv(p, map[string]string{"MC_STACK_NAME": "Go"}); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	want := "MC_DOCS_DIR=docs\nMC_STACK_NAME=\"Go\"\n"
	if got := readString(t, p); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestWriteProjectEnvRejectsInvalidKeys(t *testing.T) {
	for _, key := range []string{"", "mc_stack", "MC STACK", "MC=X", "1MC", "MC-STACK"} {
		t.Run(key, func(t *testing.T) {
			original := "MC_DOCS_DIR=docs\n"
			p := writeEnv(t, original)
			if err := writeProjectEnv(p, map[string]string{key: "x"}); err == nil {
				t.Fatalf("chave %q deveria ser rejeitada", key)
			}
			if got := readString(t, p); got != original {
				t.Fatalf("arquivo não pode ser alterado em erro: %q", got)
			}
		})
	}
}

func TestWriteProjectEnvRejectsUnsafeValues(t *testing.T) {
	for name, val := range map[string]string{
		"quebra de linha": "go test\nrm -rf /",
		"carriage return": "go test\r",
		"aspas duplas":    `echo "oi"`,
	} {
		t.Run(name, func(t *testing.T) {
			original := "MC_DOCS_DIR=docs\n"
			p := writeEnv(t, original)
			if err := writeProjectEnv(p, map[string]string{"MC_TEST_CMD": val}); err == nil {
				t.Fatalf("valor %q deveria ser rejeitado", val)
			}
			if got := readString(t, p); got != original {
				t.Fatalf("arquivo não pode ser alterado em erro: %q", got)
			}
		})
	}
}

func TestWriteProjectEnvMissingFileNotCreatedOnError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "project.env")
	if err := writeProjectEnv(p, map[string]string{"bad key": "x"}); err == nil {
		t.Fatal("esperava erro")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("arquivo não deveria ser criado quando a validação falha")
	}
}

func TestWriteThenLoadProjectStackRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "project.env")
	want := StackConfig{Name: "TypeScript + Node.js", TestCmd: "npm test -- --runInBand", TypeCmd: "npx tsc --noEmit", LintCmd: "npm run lint"}
	if err := writeProjectEnv(p, stackEnvValues(want)); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	got, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	assertStack(t, got, want)
}

// ---------------------------------------------------------------------------
// Task 4 — setupProjectEnv
// ---------------------------------------------------------------------------

func TestSetupProjectEnvDetectedStack(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n"})
	p := filepath.Join(root, ".makercycle", "config", "project.env")

	got, ok, err := setupProjectEnv(p, root, "docs", "wiki")
	if err != nil || !ok {
		t.Fatalf("esperava detecção sem erro: ok=%v err=%v", ok, err)
	}
	assertStack(t, got, wantGo)

	content := readString(t, p)
	for _, line := range []string{`MC_DOCS_DIR="docs"`, `MC_EXISTING_DOCS="wiki"`, `MC_STACK_NAME="Go"`,
		`MC_TEST_CMD="go test -v -race ./..."`, `MC_TYPE_CMD="go vet ./..."`, `MC_LINT_CMD=""`} {
		if !strings.Contains(content, line+"\n") {
			t.Fatalf("project.env sem %q:\n%s", line, content)
		}
	}
	loaded, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("project.env gerado deve ser carregável: %v", err)
	}
	assertStack(t, loaded, wantGo)
}

func TestSetupProjectEnvUndetectedPreservesManualStack(t *testing.T) {
	root := t.TempDir()
	p := writeEnv(t, "MC_DOCS_DIR=old\nMC_STACK_NAME=\"Python\"\nMC_TEST_CMD=\"pytest\"\n")

	got, ok, err := setupProjectEnv(p, root, "docs", "")
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if ok {
		t.Fatalf("diretório vazio não deveria detectar stack, veio %+v", got)
	}
	assertStack(t, got, StackConfig{})

	content := readString(t, p)
	if !strings.Contains(content, "MC_DOCS_DIR=\"docs\"\n") || !strings.Contains(content, "MC_EXISTING_DOCS=\"\"\n") {
		t.Fatalf("chaves de docs não atualizadas:\n%s", content)
	}
	loaded, err := loadProjectStack(p)
	if err != nil {
		t.Fatalf("stack manual deve ser preservada: %v", err)
	}
	assertStack(t, loaded, StackConfig{Name: "Python", TestCmd: "pytest"})
}

func TestSetupProjectEnvDetectedOverridesStackKeepsCustomKeys(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n"})
	p := writeEnv(t, "MC_STACK_NAME=Python\nMC_TEST_CMD=pytest\nMC_LINT_CMD=ruff\nMEU_CUSTOM=1\n")

	if _, _, err := setupProjectEnv(p, root, "docs", ""); err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	content := readString(t, p)
	if !strings.Contains(content, "MEU_CUSTOM=1\n") {
		t.Fatalf("chave customizada perdida:\n%s", content)
	}
	loaded, err := loadProjectStack(p)
	if err != nil {
		t.Fatal(err)
	}
	assertStack(t, loaded, wantGo)
}

func TestSetupProjectEnvWriteErrorPropagates(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"go.mod": "module x\n"})
	// O caminho do env é um diretório: a escrita deve falhar e o erro deve subir.
	if _, _, err := setupProjectEnv(t.TempDir(), root, "docs", ""); err == nil {
		t.Fatal("esperava erro de escrita")
	}
}

func TestSetupProjectEnvRejectsUnsafeExistingDocs(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(t.TempDir(), "project.env")
	if _, _, err := setupProjectEnv(p, root, "docs", "wiki\"\nMC_TEST_CMD=rm"); err == nil {
		t.Fatal("caminho de docs com aspas/quebra de linha deve ser rejeitado")
	}
}
