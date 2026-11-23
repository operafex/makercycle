package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// ---------------------------------------------------------------------------
// init.go — Task 3: primitivas de provisionamento do -init
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: IP-*)
// ---------------------------------------------------------------------------

func agentsMapFS() fstest.MapFS {
	return fstest.MapFS{
		".makercycle/agents/00_SCOUT.md":   {Data: []byte("# scout\n")},
		".makercycle/agents/01_PLANNER.md": {Data: []byte("# planner\n")},
		".makercycle/agents/README.txt":    {Data: []byte("ignorar\n")},
		".makercycle/agents/sub/x.md":      {Data: []byte("ignorar subpasta\n")},
	}
}

func listDirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("lendo %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func actionsByBase(res []initFileResult) map[string]string {
	m := map[string]string{}
	for _, r := range res {
		m[filepath.Base(r.Path)] = r.Action
	}
	return m
}

// IP-H0: os rótulos de ação são contrato (aparecem no terminal e no log).
func TestInitActionLabels(t *testing.T) {
	cases := map[string]string{
		initActionCreated:      "criado",
		initActionKept:         "mantido",
		initActionOverwritten:  "sobrescrito",
		initActionUpdated:      "atualizado",
		initActionFromGlobal:   "copiado da home",
		initActionFromEmbedded: "template embutido",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("rótulo = %q; want %q", got, want)
		}
	}
}

// IP-H1: extrai apenas os *.md do nível superior de .makercycle/agents.
func TestExtractEmbeddedAgentsWritesOnlyTopLevelMarkdown(t *testing.T) {
	dest := filepath.Join(t.TempDir(), ".makercycle", "agents")
	res, err := extractEmbeddedAgents(agentsMapFS(), dest, false)
	if err != nil {
		t.Fatalf("extractEmbeddedAgents: %v", err)
	}
	if got, want := listDirNames(t, dest), []string{"00_SCOUT.md", "01_PLANNER.md"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("arquivos extraídos = %v; want %v", got, want)
	}
	if got := readString(t, filepath.Join(dest, "01_PLANNER.md")); got != "# planner\n" {
		t.Errorf("conteúdo = %q; want %q", got, "# planner\n")
	}
	acts := actionsByBase(res)
	if len(res) != 2 || acts["00_SCOUT.md"] != initActionCreated || acts["01_PLANNER.md"] != initActionCreated {
		t.Errorf("resultados = %+v; want 2 itens %q", res, initActionCreated)
	}
	if res[0].Path != filepath.Join(dest, "00_SCOUT.md") {
		t.Errorf("Path = %q; want %q (ordem alfabética, caminho de destino)", res[0].Path, filepath.Join(dest, "00_SCOUT.md"))
	}
}

// IP-E1: sem force, arquivos existentes não são sobrescritos.
func TestExtractEmbeddedAgentsKeepsExistingWithoutForce(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "agents")
	writeFiles(t, dest, map[string]string{"00_SCOUT.md": "personalizado\n"})
	res, err := extractEmbeddedAgents(agentsMapFS(), dest, false)
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, filepath.Join(dest, "00_SCOUT.md")); got != "personalizado\n" {
		t.Errorf("arquivo existente alterado: %q", got)
	}
	acts := actionsByBase(res)
	if acts["00_SCOUT.md"] != initActionKept || acts["01_PLANNER.md"] != initActionCreated {
		t.Errorf("ações = %v; want 00_SCOUT=%q, 01_PLANNER=%q", acts, initActionKept, initActionCreated)
	}
}

// IP-E2: com force, arquivos existentes são sobrescritos pelo template.
func TestExtractEmbeddedAgentsOverwritesWithForce(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "agents")
	writeFiles(t, dest, map[string]string{"00_SCOUT.md": "personalizado\n"})
	res, err := extractEmbeddedAgents(agentsMapFS(), dest, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, filepath.Join(dest, "00_SCOUT.md")); got != "# scout\n" {
		t.Errorf("conteúdo = %q; want template", got)
	}
	if acts := actionsByBase(res); acts["00_SCOUT.md"] != initActionOverwritten {
		t.Errorf("ação = %q; want %q", acts["00_SCOUT.md"], initActionOverwritten)
	}
}

// IP-X1: FS sem .makercycle/agents gera erro.
func TestExtractEmbeddedAgentsMissingDirErrors(t *testing.T) {
	if _, err := extractEmbeddedAgents(fstest.MapFS{}, t.TempDir(), false); err == nil {
		t.Fatal("esperava erro com FS sem .makercycle/agents")
	}
}

// IP-X2: destino bloqueado por um arquivo gera erro.
func TestExtractEmbeddedAgentsDestIsFileErrors(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"agents": "sou um arquivo"})
	if _, err := extractEmbeddedAgents(agentsMapFS(), filepath.Join(root, "agents"), false); err == nil {
		t.Fatal("esperava erro quando o destino é um arquivo")
	}
}

// IP-H2: cria demands/, logs/ e references/ com .gitkeep vazio.
func TestEnsureDocsGitkeepsCreatesAllThree(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	res, err := ensureDocsGitkeeps(docs)
	if err != nil {
		t.Fatalf("ensureDocsGitkeeps: %v", err)
	}
	for _, sub := range []string{"demands", "logs", "references"} {
		p := filepath.Join(docs, sub, ".gitkeep")
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatalf("%s ausente: %v", p, err)
		}
		if fi.Size() != 0 {
			t.Errorf("%s deveria ser vazio", p)
		}
	}
	if len(res) != 3 {
		t.Fatalf("len(res) = %d; want 3", len(res))
	}
	for i, sub := range []string{"demands", "logs", "references"} {
		if res[i].Path != filepath.Join(docs, sub, ".gitkeep") || res[i].Action != initActionCreated {
			t.Errorf("res[%d] = %+v; want {%s %s}", i, res[i], filepath.Join(docs, sub, ".gitkeep"), initActionCreated)
		}
	}
}

// IP-E3: .gitkeep existente (mesmo com conteúdo) é preservado; segunda execução é idempotente.
func TestEnsureDocsGitkeepsPreservesExisting(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	writeFiles(t, docs, map[string]string{"logs/.gitkeep": "não apagar"})
	if _, err := ensureDocsGitkeeps(docs); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, filepath.Join(docs, "logs", ".gitkeep")); got != "não apagar" {
		t.Errorf(".gitkeep sobrescrito: %q", got)
	}
	res, err := ensureDocsGitkeeps(docs)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Action != initActionKept {
			t.Errorf("2ª execução: %s = %q; want %q", r.Path, r.Action, initActionKept)
		}
	}
}

// IP-X3: docsDir que é um arquivo gera erro.
func TestEnsureDocsGitkeepsDocsIsFileErrors(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"docs": "arquivo"})
	if _, err := ensureDocsGitkeeps(filepath.Join(root, "docs")); err == nil {
		t.Fatal("esperava erro quando docsDir é um arquivo")
	}
}

// IP-H3: caminho global de models.env.
func TestGlobalModelsEnvPath(t *testing.T) {
	home := filepath.Join(string(filepath.Separator), "home", "u")
	got, err := globalModelsEnvPath(home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".makercycle", "config", "models.env"); got != want {
		t.Errorf("globalModelsEnvPath = %q; want %q", got, want)
	}
}

// IP-X4: home vazio (ou só espaços) gera erro.
func TestGlobalModelsEnvPathEmptyHomeErrors(t *testing.T) {
	for _, h := range []string{"", "   "} {
		if _, err := globalModelsEnvPath(h); err == nil {
			t.Errorf("globalModelsEnvPath(%q): esperava erro", h)
		}
	}
}

// IP-H4: sem models.env no projeto e com global → copia da home.
func TestProvisionModelsEnvCopiesFromGlobal(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "proj", ".makercycle", "config", "models.env")
	global := filepath.Join(dir, "home", ".makercycle", "config", "models.env")
	writeFiles(t, dir, map[string]string{"home/.makercycle/config/models.env": "MC_GLOBAL=1\n"})
	res, err := provisionModelsEnv(project, global, []byte("MC_TEMPLATE=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, project); got != "MC_GLOBAL=1\n" {
		t.Errorf("models.env = %q; want cópia da home", got)
	}
	if res.Path != project || res.Action != initActionFromGlobal {
		t.Errorf("res = %+v; want {%s %s}", res, project, initActionFromGlobal)
	}
}

// IP-H5: sem models.env no projeto e sem global → template embutido.
func TestProvisionModelsEnvUsesEmbeddedTemplate(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, ".makercycle", "config", "models.env")
	for _, global := range []string{filepath.Join(dir, "nao-existe", "models.env"), ""} {
		_ = os.Remove(project)
		res, err := provisionModelsEnv(project, global, []byte("MC_TEMPLATE=1\n"))
		if err != nil {
			t.Fatalf("global=%q: %v", global, err)
		}
		if got := readString(t, project); got != "MC_TEMPLATE=1\n" {
			t.Errorf("global=%q: models.env = %q; want template", global, got)
		}
		if res.Action != initActionFromEmbedded {
			t.Errorf("global=%q: ação = %q; want %q", global, res.Action, initActionFromEmbedded)
		}
	}
}

// IP-E4: models.env do projeto já existe → mantido, mesmo havendo global.
func TestProvisionModelsEnvKeepsExistingProjectFile(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"proj/models.env":   "MC_PROJETO=1\n",
		"global/models.env": "MC_GLOBAL=1\n",
	})
	project := filepath.Join(dir, "proj", "models.env")
	res, err := provisionModelsEnv(project, filepath.Join(dir, "global", "models.env"), []byte("MC_TEMPLATE=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := readString(t, project); got != "MC_PROJETO=1\n" {
		t.Errorf("models.env do projeto alterado: %q", got)
	}
	if res.Action != initActionKept {
		t.Errorf("ação = %q; want %q", res.Action, initActionKept)
	}
}

// IP-E5: "global" que é diretório não conta como arquivo existente → template.
func TestProvisionModelsEnvGlobalIsDirFallsBackToTemplate(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global-dir")
	if err := os.MkdirAll(global, 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(dir, "models.env")
	res, err := provisionModelsEnv(project, global, []byte("MC_TEMPLATE=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != initActionFromEmbedded || readString(t, project) != "MC_TEMPLATE=1\n" {
		t.Errorf("res = %+v; want template embutido", res)
	}
}

// IP-X5: pasta do models.env bloqueada por arquivo → erro.
func TestProvisionModelsEnvUnwritableErrors(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"config": "arquivo"})
	if _, err := provisionModelsEnv(filepath.Join(dir, "config", "models.env"), "", []byte("X=1\n")); err == nil {
		t.Fatal("esperava erro ao gravar em pasta bloqueada")
	}
}

// IP-H6: -save-global copia o models.env do projeto para a home (cria pastas, permissão 0600).
func TestSaveGlobalModelsEnvCopiesProjectFile(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"proj/models.env": "MC_PROJETO=1\n"})
	global := filepath.Join(dir, "home", ".makercycle", "config", "models.env")
	if err := saveGlobalModelsEnv(filepath.Join(dir, "proj", "models.env"), global); err != nil {
		t.Fatalf("saveGlobalModelsEnv: %v", err)
	}
	if got := readString(t, global); got != "MC_PROJETO=1\n" {
		t.Errorf("global = %q; want cópia do projeto", got)
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(global)
		if err != nil {
			t.Fatal(err)
		}
		if perm := fi.Mode().Perm(); perm != 0o600 {
			t.Errorf("permissão = %o; want 600", perm)
		}
	}
}

// IP-E6: -save-global sobrescreve um global existente (ação explícita do usuário).
func TestSaveGlobalModelsEnvOverwritesGlobal(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"proj/models.env":   "MC_NOVO=1\n",
		"global/models.env": "MC_VELHO=1\n",
	})
	global := filepath.Join(dir, "global", "models.env")
	if err := saveGlobalModelsEnv(filepath.Join(dir, "proj", "models.env"), global); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, global); got != "MC_NOVO=1\n" {
		t.Errorf("global = %q; want %q", got, "MC_NOVO=1\n")
	}
	if names := listDirNames(t, filepath.Join(dir, "global")); len(names) != 1 {
		t.Errorf("sobraram temporários na pasta global: %v", names)
	}
}

// IP-X6: sem models.env no projeto → erro que cita o caminho; global não é criado.
func TestSaveGlobalModelsEnvMissingProjectErrors(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "nao-existe", "models.env")
	global := filepath.Join(dir, "home", "models.env")
	err := saveGlobalModelsEnv(project, global)
	if err == nil {
		t.Fatal("esperava erro sem models.env no projeto")
	}
	if !strings.Contains(err.Error(), project) {
		t.Errorf("erro %q deveria citar %q", err, project)
	}
	if _, statErr := os.Stat(global); statErr == nil {
		t.Error("global não deveria ter sido criado")
	}
}

// IP-X7: caminho global vazio → erro.
func TestSaveGlobalModelsEnvEmptyGlobalErrors(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"models.env": "X=1\n"})
	if err := saveGlobalModelsEnv(filepath.Join(dir, "models.env"), ""); err == nil {
		t.Fatal("esperava erro com caminho global vazio")
	}
}

// IP-H7: entradas obrigatórias do .gitignore — APENAS state/, telemetry/ e secrets.env.
// [REGRESSÃO JUSTIFICADA] project.env saiu da lista: passa a ser versionado pela equipe
// (docs/references/team-scale-and-regression.md §2). As chaves migram para secrets.env.
func TestGitignoreEntriesContract(t *testing.T) {
	want := []string{".makercycle/state/", ".makercycle/telemetry/", ".makercycle/config/secrets.env"}
	if strings.Join(gitignoreEntries, "|") != strings.Join(want, "|") {
		t.Errorf("gitignoreEntries = %v; want %v", gitignoreEntries, want)
	}
}

// IP-H8: .gitignore inexistente é criado com o cabeçalho e as entradas.
func TestEnsureGitignoreEntriesCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	added, err := ensureGitignoreEntries(path, gitignoreEntries)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(added, "|") != strings.Join(gitignoreEntries, "|") {
		t.Errorf("added = %v; want %v", added, gitignoreEntries)
	}
	want := gitignoreHeader + "\n.makercycle/state/\n.makercycle/telemetry/\n.makercycle/config/secrets.env\n"
	if got := readString(t, path); got != want {
		t.Errorf(".gitignore =\n%q\nwant\n%q", got, want)
	}
}

// IP-E7: conteúdo existente sem quebra de linha final é preservado e só as ausentes entram.
func TestEnsureGitignoreEntriesAppendsMissingOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(path, []byte("node_modules/\n.makercycle/state/"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ensureGitignoreEntries(path, gitignoreEntries)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{".makercycle/telemetry/", ".makercycle/config/secrets.env"}; strings.Join(added, "|") != strings.Join(want, "|") {
		t.Errorf("added = %v; want %v", added, want)
	}
	want := "node_modules/\n.makercycle/state/\n\n" + gitignoreHeader + "\n.makercycle/telemetry/\n.makercycle/config/secrets.env\n"
	if got := readString(t, path); got != want {
		t.Errorf(".gitignore =\n%q\nwant\n%q", got, want)
	}
}

// IP-E8: idempotência — segunda chamada não adiciona nada nem altera o arquivo.
func TestEnsureGitignoreEntriesIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if _, err := ensureGitignoreEntries(path, gitignoreEntries); err != nil {
		t.Fatal(err)
	}
	before := readString(t, path)
	added, err := ensureGitignoreEntries(path, gitignoreEntries)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v; want vazio", added)
	}
	if after := readString(t, path); after != before {
		t.Errorf("arquivo alterado na 2ª chamada:\n%q\n->\n%q", before, after)
	}
}

// IP-E9: variações equivalentes (barra inicial, sem barra final, espaços, CRLF) contam como presentes.
func TestEnsureGitignoreEntriesRecognizesEquivalents(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	content := "/.makercycle/state/\r\n  .makercycle/telemetry  \r\n/.makercycle/config/secrets.env\r\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ensureGitignoreEntries(path, gitignoreEntries)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v; want vazio (todas equivalentes já presentes)", added)
	}
	if got := readString(t, path); got != content {
		t.Errorf("arquivo não deveria mudar: %q", got)
	}
}

// IP-E10: entrada comentada ou negada NÃO conta como presente.
func TestEnsureGitignoreEntriesIgnoresCommentsAndNegations(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(path, []byte("# .makercycle/state/\n!.makercycle/telemetry/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ensureGitignoreEntries(path, gitignoreEntries)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 3 {
		t.Errorf("added = %v; want as 3 entradas", added)
	}
}

// IP-E11: o .gitignore real do repositório já cobre as entradas (nenhuma alteração).
func TestRepoGitignoreAlreadyCoversEntries(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(tmp, []byte(readRepoFile(t, ".gitignore")), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := ensureGitignoreEntries(tmp, gitignoreEntries)
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 0 {
		t.Errorf("added = %v; o .gitignore do repositório já deveria cobrir tudo", added)
	}
}

// IP-X8: caminho do .gitignore é um diretório → erro.
func TestEnsureGitignoreEntriesPathIsDirErrors(t *testing.T) {
	if _, err := ensureGitignoreEntries(t.TempDir(), gitignoreEntries); err == nil {
		t.Fatal("esperava erro quando o caminho é um diretório")
	}
}

// IP-H9: writeFileAtomic grava com a permissão pedida e não deixa temporários.
func TestWriteFileAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := writeFileAtomic(path, []byte("v1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readString(t, path); got != "v2" {
		t.Errorf("conteúdo = %q; want v2", got)
	}
	if names := listDirNames(t, dir); len(names) != 1 {
		t.Errorf("arquivos na pasta = %v; want só a.txt", names)
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(path)
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("permissão = %o; want 600", fi.Mode().Perm())
		}
	}
}
