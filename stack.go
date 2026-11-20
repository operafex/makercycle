package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const projectEnvPath = ".makercycle/config/project.env"

var (
	errProjectStackIncomplete = errors.New("project.env sem MC_STACK_NAME/MC_TEST_CMD preenchidos")
	railsGemRegex             = regexp.MustCompile(`(?m)^\s*gem\s+["']rails["']`)
	validEnvKey               = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
)

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

func goStack(root string) StackConfig {
	lintCmd := ""
	if isFile(filepath.Join(root, ".golangci.yml")) || isFile(filepath.Join(root, ".golangci.yaml")) {
		lintCmd = "golangci-lint run"
	}
	return StackConfig{
		Name:    "Go",
		TestCmd: "go test -v -race ./...",
		TypeCmd: "go vet ./...",
		LintCmd: lintCmd,
	}
}

func railsStack(root string) StackConfig {
	testCmd := "bin/rails test"
	if isDir(filepath.Join(root, "spec")) {
		testCmd = "bundle exec rspec"
	}
	return StackConfig{
		Name:    "Ruby on Rails",
		TestCmd: testCmd,
		TypeCmd: "",
		LintCmd: "bundle exec rubocop",
	}
}

type packageJSON struct {
	Scripts         map[string]string `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func readPackageJSON(root string) packageJSON {
	var pkg packageJSON
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		return pkg
	}
	_ = json.Unmarshal(data, &pkg)
	return pkg
}

func tsStack(root string) StackConfig {
	pkg := readPackageJSON(root)
	lintCmd := ""
	if _, ok := pkg.Scripts["lint"]; ok {
		lintCmd = "npm run lint"
	}
	return StackConfig{
		Name:    "TypeScript + Node.js",
		TestCmd: "npm test",
		TypeCmd: "npx tsc --noEmit",
		LintCmd: lintCmd,
	}
}

func nodeStack(root string) StackConfig {
	pkg := readPackageJSON(root)
	lintCmd := ""
	if _, ok := pkg.Scripts["lint"]; ok {
		lintCmd = "npm run lint"
	}
	hasTSDep := false
	if _, ok := pkg.Dependencies["typescript"]; ok {
		hasTSDep = true
	}
	if _, ok := pkg.DevDependencies["typescript"]; ok {
		hasTSDep = true
	}
	if isFile(filepath.Join(root, "tsconfig.json")) || hasTSDep {
		return StackConfig{
			Name:    "TypeScript + Node.js",
			TestCmd: "npm test",
			TypeCmd: "npx tsc --noEmit",
			LintCmd: lintCmd,
		}
	}
	return StackConfig{
		Name:    "Node.js",
		TestCmd: "npm test",
		TypeCmd: "",
		LintCmd: lintCmd,
	}
}

func detectStack(root string) (StackConfig, bool) {
	if isFile(filepath.Join(root, "go.mod")) {
		return goStack(root), true
	}
	if isFile(filepath.Join(root, "Gemfile")) {
		if data, err := os.ReadFile(filepath.Join(root, "Gemfile")); err == nil {
			if railsGemRegex.Match(data) {
				return railsStack(root), true
			}
		}
	}
	if isFile(filepath.Join(root, "package.json")) {
		return nodeStack(root), true
	}
	return StackConfig{}, false
}

func parseEnvContent(content string) map[string]string {
	env := make(map[string]string)
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			continue
		}
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"'")
		val = strings.TrimSpace(val)
		env[key] = val
	}
	return env
}

func loadProjectStack(path string) (StackConfig, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return StackConfig{}, fmt.Errorf("lendo %s: %w", path, err)
	}
	if fi.IsDir() {
		return StackConfig{}, fmt.Errorf("lendo %s: caminho é um diretório", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return StackConfig{}, fmt.Errorf("lendo %s: %w", path, err)
	}
	env := parseEnvContent(string(data))
	name := env["MC_STACK_NAME"]
	testCmd := env["MC_TEST_CMD"]
	if strings.TrimSpace(name) == "" || strings.TrimSpace(testCmd) == "" {
		return StackConfig{}, errProjectStackIncomplete
	}
	return StackConfig{
		Name:    name,
		TestCmd: testCmd,
		TypeCmd: env["MC_TYPE_CMD"],
		LintCmd: env["MC_LINT_CMD"],
	}, nil
}

func stackEnvValues(s StackConfig) map[string]string {
	return map[string]string{
		"MC_STACK_NAME": s.Name,
		"MC_TEST_CMD":   s.TestCmd,
		"MC_TYPE_CMD":   s.TypeCmd,
		"MC_LINT_CMD":   s.LintCmd,
	}
}

func writeProjectEnv(path string, updates map[string]string) error {
	for k, v := range updates {
		if !validEnvKey.MatchString(k) {
			return fmt.Errorf("chave inválida %q: deve casar com ^[A-Z][A-Z0-9_]*$", k)
		}
		if strings.ContainsAny(v, "\n\r\"") {
			return fmt.Errorf("valor inseguro para %s: não pode conter quebra de linha ou aspas", k)
		}
	}

	var existingLines []string
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		return fmt.Errorf("caminho %s é um diretório", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("lendo %s: %w", path, err)
		}
	} else {
		content := string(data)
		lines := strings.Split(content, "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		existingLines = lines
	}

	seen := make(map[string]bool)
	var newLines []string

	for _, line := range existingLines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "=") {
			newLines = append(newLines, line)
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		k := strings.TrimSpace(parts[0])
		if k == "" {
			newLines = append(newLines, line)
			continue
		}
		if newVal, exists := updates[k]; exists {
			if !seen[k] {
				seen[k] = true
				newLines = append(newLines, fmt.Sprintf("%s=\"%s\"", k, newVal))
			}
		} else {
			newLines = append(newLines, line)
		}
	}

	var unadded []string
	for k := range updates {
		if !seen[k] {
			unadded = append(unadded, k)
		}
	}
	sort.Strings(unadded)
	for _, k := range unadded {
		newLines = append(newLines, fmt.Sprintf("%s=\"%s\"", k, updates[k]))
	}

	var buf bytes.Buffer
	for _, l := range newLines {
		buf.WriteString(l)
		buf.WriteByte('\n')
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("criando diretório pai de %s: %w", path, err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("gravando %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("ajustando permissão de %s: %w", path, err)
	}
	return nil
}

func setupProjectEnv(envPath, root, docsDir, existingDocs string) (StackConfig, bool, error) {
	updates := map[string]string{
		"MC_DOCS_DIR":      docsDir,
		"MC_EXISTING_DOCS": existingDocs,
	}
	detected, ok := detectStack(root)
	if ok {
		for k, v := range stackEnvValues(detected) {
			updates[k] = v
		}
	}
	if err := writeProjectEnv(envPath, updates); err != nil {
		return StackConfig{}, false, err
	}
	if ok {
		return detected, true, nil
	}
	return StackConfig{}, false, nil
}

// confirmSetupStack pede confirmação da stack detectada no -setup e grava a escolha em project.env.
// Sem stack detectada não há pergunta: apenas as chaves de documentação são gravadas.
func confirmSetupStack(reader *bufio.Reader, out io.Writer, envPath, root, docsDir, existingDocs string) (StackConfig, bool, error) {
	updates := map[string]string{
		"MC_DOCS_DIR":      docsDir,
		"MC_EXISTING_DOCS": existingDocs,
	}
	detected, ok := detectStack(root)
	if !ok {
		return StackConfig{}, false, writeProjectEnv(envPath, updates)
	}

	fmt.Fprintf(out, "[STACK] Detectada: %s (testes: %s)\n", detected.Name, detected.TestCmd)
	fmt.Fprint(out, "Confirmar esta stack como padrão do projeto? [S/n] (ou digite 'p' para personalizar): ")
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)

	chosen := detected
	switch answer {
	case "", "S", "s":
	case "p", "P", "n", "N":
		var err error
		chosen, err = selectStack(reader, out, envPath)
		if err != nil {
			return StackConfig{}, false, err
		}
	default:
		return StackConfig{}, false, fmt.Errorf("opção inválida %q: responda S, n ou p", answer)
	}

	for k, v := range stackEnvValues(chosen) {
		updates[k] = v
	}
	if err := writeProjectEnv(envPath, updates); err != nil {
		return StackConfig{}, false, err
	}
	fmt.Fprintf(out, "[STACK] Stack salva como padrão do projeto: %s\n", chosen.Name)
	return chosen, true, nil
}

// resolveRunStack resolve a stack de uma rodada do MakerCycle: usa project.env em silêncio quando válido
// (e forceSelect é falso); caso contrário abre o menu e salva a escolha.
func resolveRunStack(reader *bufio.Reader, out io.Writer, envPath string, forceSelect bool) (StackConfig, error) {
	if !forceSelect {
		if s, err := loadProjectStack(envPath); err == nil {
			fmt.Fprintf(out, "[INFO] Stack do projeto: %s (via project.env)\n", s.Name)
			return s, nil
		}
	}
	s, err := selectStack(reader, out, envPath)
	if err != nil {
		return StackConfig{}, err
	}
	if err := writeProjectEnv(envPath, stackEnvValues(s)); err != nil {
		fmt.Fprintf(out, "[AVISO] Não foi possível salvar a stack em %s: %v\n", envPath, err)
	}
	return s, nil
}

func selectStack(reader *bufio.Reader, out io.Writer, envPath string) (StackConfig, error) {
	loaded, err := loadProjectStack(envPath)
	if err == nil {
		fmt.Fprintln(out, "\nQual stack deve ser utilizada nesta execução?")
		fmt.Fprintf(out, "  [1] Projeto Atual (Auto-detectado): %s\n", loaded.Name)
		fmt.Fprintln(out, "  [2] Ruby on Rails")
		fmt.Fprintln(out, "  [3] Go")
		fmt.Fprintln(out, "  [4] TypeScript + Node.js")
		fmt.Fprintln(out, "  [5] Personalizada")
		fmt.Fprint(out, "Escolha uma opção (1-5) [Enter = 1]: ")

		choice, _ := reader.ReadString('\n')
		switch strings.TrimSpace(choice) {
		case "", "1":
			return loaded, nil
		case "2":
			return railsStack("."), nil
		case "3":
			return goStack("."), nil
		case "4":
			return tsStack("."), nil
		case "5":
			return promptCustomStack(reader, out)
		default:
			return StackConfig{}, fmt.Errorf("opção inválida %q: escolha entre 1 e 5", strings.TrimSpace(choice))
		}
	}

	fmt.Fprintf(out, "[AVISO] Não foi possível carregar a stack do projeto (motivo: %v). Por favor, selecione manualmente:\n", err)
	fmt.Fprintln(out, "\nQual stack deve ser utilizada nesta execução?")
	fmt.Fprintln(out, "  [1] Ruby on Rails")
	fmt.Fprintln(out, "  [2] Go")
	fmt.Fprintln(out, "  [3] TypeScript + Node.js")
	fmt.Fprintln(out, "  [4] Personalizada")
	fmt.Fprint(out, "Escolha uma opção (1-4): ")

	choice, _ := reader.ReadString('\n')
	switch strings.TrimSpace(choice) {
	case "1":
		return railsStack("."), nil
	case "2":
		return goStack("."), nil
	case "3":
		return tsStack("."), nil
	case "4":
		return promptCustomStack(reader, out)
	default:
		return StackConfig{}, fmt.Errorf("opção inválida %q: escolha entre 1 e 4", strings.TrimSpace(choice))
	}
}

func promptCustomStack(reader *bufio.Reader, out io.Writer) (StackConfig, error) {
	fmt.Fprint(out, "Comando de Testes (ex: pytest): ")
	tCmd, _ := reader.ReadString('\n')
	testCmd := strings.TrimSpace(tCmd)
	if testCmd == "" {
		return StackConfig{}, fmt.Errorf("comando de testes obrigatório para stack personalizada")
	}
	fmt.Fprint(out, "Comando de Tipos/Compilação (opcional, Enter para pular): ")
	typeCmd, _ := reader.ReadString('\n')
	fmt.Fprint(out, "Comando de Linter (opcional, Enter para pular): ")
	lCmd, _ := reader.ReadString('\n')

	return StackConfig{
		Name:    "Personalizada",
		TestCmd: testCmd,
		TypeCmd: strings.TrimSpace(typeCmd),
		LintCmd: strings.TrimSpace(lCmd),
	}, nil
}
