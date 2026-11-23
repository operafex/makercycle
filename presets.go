package main

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// greenfieldManifests são manifestos de linguagem que indicam projeto existente.
var greenfieldManifests = map[string]bool{"go.mod": true, "package.json": true, "Gemfile": true,
	"requirements.txt": true, "pyproject.toml": true, "setup.py": true, "Pipfile": true, "Cargo.toml": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true, "composer.json": true, "mix.exs": true,
	"pubspec.yaml": true, "deno.json": true}

// greenfieldIgnoredDirs são pastas da RAIZ ignoradas na detecção.
var greenfieldIgnoredDirs = map[string]bool{".git": true, ".makercycle": true, "docs": true}

// isGreenfieldProject informa se dir é um projeto novo: sem manifestos de linguagem nem código-fonte.
// Erros de leitura são ignorados; diretório inexistente conta como projeto novo.
func isGreenfieldProject(dir string) bool {
	greenfield := true
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if filepath.Dir(path) == filepath.Clean(dir) && greenfieldIgnoredDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if greenfieldManifests[d.Name()] || isCodeFileName(d.Name()) {
			greenfield = false
			return fs.SkipAll
		}
		return nil
	})
	return greenfield
}

// isCodeFileName informa se o nome do arquivo tem extensão de código-fonte conhecida.
func isCodeFileName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range codeFileExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

const stackPresetMenuTitle = "Escolha um preset de stack para o projeto:"

// StackPreset é uma stack pré-configurada oferecida no -setup/-init.
type StackPreset struct {
	Name     string // MC_STACK_NAME
	BuildCmd string // MC_BUILD_CMD (também gravado como MC_TYPE_CMD)
	TestCmd  string // MC_TEST_CMD
	LintCmd  string // MC_LINT_CMD
}

var stackPresets = []StackPreset{
	{Name: "Go", BuildCmd: "go build ./...", TestCmd: "go test -race ./...", LintCmd: "go vet ./..."},
	{Name: "Node.js / TypeScript (Backend API)", BuildCmd: "npm run build", TestCmd: "npm test", LintCmd: "npm run lint"},
	{Name: "Node.js / TypeScript (Frontend)", BuildCmd: "npm run build", TestCmd: "npm test", LintCmd: "npm run lint"},
	{Name: "Python", BuildCmd: "python -m py_compile .", TestCmd: "pytest", LintCmd: "ruff check ."},
	{Name: "Rust", BuildCmd: "cargo build", TestCmd: "cargo test", LintCmd: "cargo clippy"},
}

// StackConfig converte o preset para a configuração usada pelo ciclo (a Etapa 4A executa TypeCmd).
func (p StackPreset) StackConfig() StackConfig {
	return StackConfig{Name: p.Name, TestCmd: p.TestCmd, TypeCmd: p.BuildCmd, LintCmd: p.LintCmd}
}

// presetEnvValues devolve as chaves de project.env do preset (as 4 de stack + MC_BUILD_CMD).
func presetEnvValues(p StackPreset) map[string]string {
	values := stackEnvValues(p.StackConfig())
	values["MC_BUILD_CMD"] = p.BuildCmd
	return values
}

// selectStackPreset exibe o menu de presets. Enter/EOF pula (ok=false, sem erro).
func selectStackPreset(reader *bufio.Reader, out io.Writer) (StackPreset, bool, error) {
	fmt.Fprintf(out, "\n%s\n", stackPresetMenuTitle)
	for i, p := range stackPresets {
		fmt.Fprintf(out, "  [%d] %s\n", i+1, p.Name)
	}
	fmt.Fprintf(out, "  [%d] Personalizado\n", len(stackPresets)+1)
	fmt.Fprint(out, "Escolha uma opção (1-6) [Enter = pular; o SCOUT tentará identificar a stack]: ")

	line, _ := reader.ReadString('\n')
	choice := strings.TrimSpace(line)
	if choice == "" {
		return StackPreset{}, false, nil
	}
	n, convErr := strconv.Atoi(choice)
	switch {
	case convErr != nil || n < 1 || n > len(stackPresets)+1:
		return StackPreset{}, false, fmt.Errorf("opção inválida %q: escolha entre 1 e 6", choice)
	case n == len(stackPresets)+1:
		return promptCustomPreset(reader, out)
	default:
		return stackPresets[n-1], true, nil
	}
}

// promptCustomPreset pergunta os dados de uma stack personalizada. O comando de testes é obrigatório.
func promptCustomPreset(reader *bufio.Reader, out io.Writer) (StackPreset, bool, error) {
	ask := func(prompt string) string {
		fmt.Fprint(out, prompt)
		line, _ := reader.ReadString('\n')
		return strings.TrimSpace(line)
	}
	name := ask("Nome da stack (MC_STACK) [Enter = Personalizado]: ")
	build := ask("Comando de build (MC_BUILD_CMD, opcional): ")
	test := ask("Comando de testes (MC_TEST_CMD, obrigatório): ")
	lint := ask("Comando de lint (MC_LINT_CMD, opcional): ")

	if name == "" {
		name = "Personalizado"
	}
	if test == "" {
		return StackPreset{}, false, fmt.Errorf("comando de testes (MC_TEST_CMD) obrigatório para stack personalizada")
	}
	return StackPreset{Name: name, BuildCmd: build, TestCmd: test, LintCmd: lint}, true, nil
}
