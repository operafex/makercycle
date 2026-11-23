package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// modelsEnvPath é o models.env do projeto (versionado, sem segredos).
const modelsEnvPath = ".makercycle/config/models.env"

// Rótulos das ações do -init (aparecem no terminal e no log; são contrato de teste).
const (
	initActionCreated      = "criado"
	initActionKept         = "mantido"
	initActionOverwritten  = "sobrescrito"
	initActionUpdated      = "atualizado"
	initActionFromGlobal   = "copiado da home"
	initActionFromEmbedded = "template embutido"
)

const gitignoreHeader = "# MakerCycle (gerado por makercycle -init)"

// gitignoreEntries são as entradas que o -init garante no .gitignore do projeto.
var gitignoreEntries = []string{".makercycle/state/", ".makercycle/telemetry/", ".makercycle/config/secrets.env"}

// codeFileExtensions são as extensões que indicam a existência de código-fonte (SCAN do -init).
var codeFileExtensions = []string{".go", ".rb", ".py", ".js", ".jsx", ".ts", ".tsx", ".java", ".kt", ".rs", ".php", ".cs", ".swift", ".c", ".h", ".cpp", ".ex", ".exs", ".scala", ".dart"}

// codeScanSkipDirs são pastas (além das ocultas) ignoradas na detecção de código.
var codeScanSkipDirs = map[string]bool{"node_modules": true, "vendor": true, "dist": true, "build": true, "target": true}

type initFileResult struct {
	Path   string // caminho no disco (filepath.Join do destino informado)
	Action string // um dos initAction*
}

type initPaths struct {
	Root             string // raiz do projeto ("." no CLI)
	DocsDir          string // relativo a Root
	GlobalModelsEnv  string // "" = sem home
	GlobalSecretsEnv string // "" = sem home
}

// writeFileAtomic grava data em path via arquivo temporário na mesma pasta + rename.
func writeFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".makercycle-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// extractEmbeddedAgents grava os *.md de nível superior de embeddedAgentsDir em destDir.
// Arquivos existentes são mantidos, salvo com force.
func extractEmbeddedAgents(agents fs.FS, destDir string, force bool) ([]initFileResult, error) {
	entries, err := fs.ReadDir(agents, embeddedAgentsDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, err
	}
	var res []initFileResult
	for _, e := range entries {
		if !e.Type().IsRegular() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := fs.ReadFile(agents, path.Join(embeddedAgentsDir, e.Name()))
		if err != nil {
			return res, err
		}
		dest := filepath.Join(destDir, e.Name())
		action := initActionCreated
		if _, statErr := os.Stat(dest); statErr == nil {
			if !force {
				res = append(res, initFileResult{Path: dest, Action: initActionKept})
				continue
			}
			action = initActionOverwritten
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return res, statErr
		}
		if err := writeFileAtomic(dest, data, 0644); err != nil {
			return res, err
		}
		res = append(res, initFileResult{Path: dest, Action: action})
	}
	return res, nil
}

// ensureDocsGitkeeps cria <docs>/{demands,logs,references} com .gitkeep vazio (existentes são preservados).
func ensureDocsGitkeeps(docsDir string) ([]initFileResult, error) {
	var res []initFileResult
	for _, sub := range []string{"demands", "logs", "references"} {
		dir := filepath.Join(docsDir, sub)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return res, err
		}
		keep := filepath.Join(dir, ".gitkeep")
		_, statErr := os.Stat(keep)
		switch {
		case statErr == nil:
			res = append(res, initFileResult{Path: keep, Action: initActionKept})
		case errors.Is(statErr, fs.ErrNotExist):
			if err := os.WriteFile(keep, nil, 0644); err != nil {
				return res, err
			}
			res = append(res, initFileResult{Path: keep, Action: initActionCreated})
		default:
			return res, statErr
		}
	}
	return res, nil
}

// globalModelsEnvPath devolve <home>/.makercycle/config/models.env.
func globalModelsEnvPath(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		return "", errors.New("diretório home indisponível")
	}
	return filepath.Join(home, ".makercycle", "config", "models.env"), nil
}

// provisionModelsEnv garante o models.env do projeto: mantém o existente; senão copia o global
// (arquivo regular) ou, na ausência dele, grava o template embutido.
func provisionModelsEnv(projectPath, globalPath string, template []byte) (initFileResult, error) {
	if isFile(projectPath) {
		return initFileResult{Path: projectPath, Action: initActionKept}, nil
	}
	if err := os.MkdirAll(filepath.Dir(projectPath), 0755); err != nil {
		return initFileResult{}, err
	}
	data, action := template, initActionFromEmbedded
	if globalPath != "" && isFile(globalPath) {
		b, err := os.ReadFile(globalPath)
		if err != nil {
			return initFileResult{}, err
		}
		data, action = b, initActionFromGlobal
	}
	if err := writeFileAtomic(projectPath, data, 0644); err != nil {
		return initFileResult{}, err
	}
	return initFileResult{Path: projectPath, Action: action}, nil
}

// saveGlobalModelsEnv copia o models.env do projeto para o caminho global (sobrescreve; pasta 0700, arquivo 0600).
func saveGlobalModelsEnv(projectPath, globalPath string) error {
	if strings.TrimSpace(globalPath) == "" {
		return errors.New("caminho global de models.env vazio")
	}
	b, err := os.ReadFile(projectPath)
	if err != nil {
		return fmt.Errorf("lendo %s: %w", projectPath, err)
	}
	if err := os.MkdirAll(filepath.Dir(globalPath), 0700); err != nil {
		return err
	}
	return writeFileAtomic(globalPath, b, 0600)
}

// normalizeGitignoreEntry descarta espaços, '\r' e uma barra inicial e uma final.
func normalizeGitignoreEntry(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r", ""))
	s = strings.TrimPrefix(s, "/")
	return strings.TrimSuffix(s, "/")
}

// ensureGitignoreEntries acrescenta (em bloco, após gitignoreHeader) as entradas ausentes e devolve as adicionadas.
// Linhas comentadas (#) ou negadas (!) não contam como presentes.
func ensureGitignoreEntries(path string, entries []string) ([]string, error) {
	data, err := os.ReadFile(path)
	existed := true
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		existed = false
	}

	present := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		l := strings.TrimSpace(line)
		if l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "!") {
			continue
		}
		present[normalizeGitignoreEntry(l)] = true
	}
	var added []string
	for _, e := range entries {
		if !present[normalizeGitignoreEntry(e)] {
			added = append(added, e)
		}
	}
	if len(added) == 0 {
		return nil, nil
	}

	var sb strings.Builder
	if existed && len(data) > 0 {
		sb.Write(data)
		if !strings.HasSuffix(sb.String(), "\n") {
			sb.WriteString("\n")
		}
		if !strings.HasSuffix(sb.String(), "\n\n") {
			sb.WriteString("\n")
		}
	}
	sb.WriteString(gitignoreHeader + "\n")
	for _, e := range added {
		sb.WriteString(e + "\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return nil, err
	}
	return added, nil
}

// provisionInitAssets cria a estrutura .makercycle/ e provisiona agentes, .gitkeep, models.env e .gitignore.
func provisionInitAssets(p initPaths, agents fs.FS, modelsTemplate []byte, force bool) ([]initFileResult, error) {
	mcDir := filepath.Join(p.Root, ".makercycle")
	for _, sub := range []string{"agents", "config", "state", "telemetry"} {
		dir := filepath.Join(mcDir, sub)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("%s: %w", dir, err)
		}
	}

	var all []initFileResult

	agentsDir := filepath.Join(mcDir, "agents")
	res, err := extractEmbeddedAgents(agents, agentsDir, force)
	if err != nil {
		return all, fmt.Errorf("%s: %w", agentsDir, err)
	}
	all = append(all, res...)

	docsPath := filepath.Join(p.Root, p.DocsDir)
	res, err = ensureDocsGitkeeps(docsPath)
	if err != nil {
		return all, fmt.Errorf("%s: %w", docsPath, err)
	}
	all = append(all, res...)

	modelsPath := filepath.Join(p.Root, modelsEnvPath)
	mr, err := provisionModelsEnv(modelsPath, p.GlobalModelsEnv, modelsTemplate)
	if err != nil {
		return all, fmt.Errorf("%s: %w", modelsPath, err)
	}
	all = append(all, mr)

	secretsPath := filepath.Join(p.Root, secretsEnvPath)
	sr, err := provisionSecretsEnv(secretsPath, p.GlobalSecretsEnv)
	if err != nil {
		return all, fmt.Errorf("%s: %w", secretsPath, err)
	}
	all = append(all, sr)

	giPath := filepath.Join(p.Root, ".gitignore")
	_, statErr := os.Stat(giPath)
	existed := statErr == nil
	added, err := ensureGitignoreEntries(giPath, gitignoreEntries)
	if err != nil {
		return all, fmt.Errorf("%s: %w", giPath, err)
	}
	action := initActionKept
	switch {
	case len(added) > 0 && !existed:
		action = initActionCreated
	case len(added) > 0:
		action = initActionUpdated
	}
	all = append(all, initFileResult{Path: giPath, Action: action})
	return all, nil
}

// hasCodeFiles informa se root contém arquivos de código (ignora pastas ocultas e de dependências/build).
func hasCodeFiles(root string) bool {
	exts := map[string]bool{}
	for _, e := range codeFileExtensions {
		exts[e] = true
	}
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != root {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root && (strings.HasPrefix(d.Name(), ".") || codeScanSkipDirs[d.Name()]) {
				return fs.SkipDir
			}
			return nil
		}
		if exts[strings.ToLower(filepath.Ext(d.Name()))] {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// initSkillsToInstall devolve as skills declaradas em MC_*_SKILL (sem fallback para as flags).
func initSkillsToInstall(environ []string) []string {
	return collectConfiguredSkills(environ)
}

// initRequiredBinaries lista os binários exigidos: os dos agentes (únicos), npx (só com skills) e git.
func initRequiredBinaries(agents []AgentBinding, skills []string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	for _, a := range agents {
		add(a.Bin)
	}
	if len(skills) > 0 {
		add("npx")
	}
	add("git")
	return out
}

// refreshBindingFromEnv aplica MC_<ALIAS>_{BIN,MODEL,ARGS,SKILL} não vazios sobre o binding,
// exceto os campos cujas flags (<alias>-bin|-model|-args|-skill) foram passadas explicitamente.
func refreshBindingFromEnv(a AgentBinding, getenv func(string) string, setFlags map[string]bool) AgentBinding {
	prefix := "MC_" + strings.ToUpper(a.Alias) + "_"
	flagPrefix := strings.ToLower(a.Alias) + "-"
	apply := func(suffix string, field *string) {
		if setFlags[flagPrefix+strings.ToLower(suffix)] {
			return
		}
		if v := strings.TrimSpace(getenv(prefix + suffix)); v != "" {
			*field = v
		}
	}
	apply("BIN", &a.Bin)
	apply("MODEL", &a.Model)
	apply("ARGS", &a.Args)
	apply("SKILL", &a.Skill)
	return a
}
