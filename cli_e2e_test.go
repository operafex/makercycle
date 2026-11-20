package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 5: fiação em makercycle.go (-version, -save-global, -force, -init unificado)
// Testes ponta a ponta com o binário real compilado num diretório temporário.
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: E2E-*)
// ---------------------------------------------------------------------------

var (
	e2eBuildOnce sync.Once
	e2eBinPath   string
	e2eBuildErr  error
	e2eBuildOut  []byte
)

func skipE2E(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("teste ponta a ponta ignorado em -short")
	}
	if runtime.GOOS == "windows" {
		t.Skip("teste ponta a ponta usa HOME/PATH POSIX")
	}
}

// buildE2EBinary compila o pacote atual uma vez (sem ldflags) e devolve o caminho do binário.
func buildE2EBinary(t *testing.T) string {
	t.Helper()
	skipE2E(t)
	e2eBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "makercycle-e2e-")
		if err != nil {
			e2eBuildErr = err
			return
		}
		e2eBinPath = filepath.Join(dir, "makercycle")
		e2eBuildOut, e2eBuildErr = exec.Command("go", "build", "-o", e2eBinPath, ".").CombinedOutput()
	})
	if e2eBuildErr != nil {
		t.Fatalf("go build falhou: %v\n%s", e2eBuildErr, e2eBuildOut)
	}
	return e2eBinPath
}

type e2eResult struct {
	Out  string
	Code int
}

// runE2E executa o binário em dir com ambiente mínimo e controlado (HOME isolado, PATH sem CLIs de agente).
func runE2E(t *testing.T, bin, dir, home, stdin string, args ...string) e2eResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	emptyPath := t.TempDir()
	cmd.Env = []string{"HOME=" + home, "USERPROFILE=" + home, "PATH=" + emptyPath, "TERM=dumb"}
	cmd.Stdin = strings.NewReader(stdin)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("timeout executando %v:\n%s", args, buf.String())
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("executando %v: %v", args, err)
	}
	return e2eResult{Out: buf.String(), Code: code}
}

// E2E-H1: -version imprime GetVersion() e sai com 0.
func TestE2EVersionFlag(t *testing.T) {
	bin := buildE2EBinary(t)
	r := runE2E(t, bin, t.TempDir(), t.TempDir(), "", "-version")
	if r.Code != 0 {
		t.Fatalf("código = %d; want 0\n%s", r.Code, r.Out)
	}
	if got := strings.TrimSpace(r.Out); got != "makercycle v0.1.0" {
		t.Errorf("-version = %q; want %q", got, "makercycle v0.1.0")
	}
}

// E2E-H2: a versão é injetável pelas mesmas ldflags do release.yml (tag "v9.8.7" → sem "v" duplicado).
func TestE2EVersionInjectedByReleaseLdflags(t *testing.T) {
	skipE2E(t)
	bin := filepath.Join(t.TempDir(), "makercycle")
	out, err := exec.Command("go", "build", "-ldflags=-s -w -X main.Version=v9.8.7", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build com ldflags falhou: %v\n%s", err, out)
	}
	r := runE2E(t, bin, t.TempDir(), t.TempDir(), "", "-version")
	if got := strings.TrimSpace(r.Out); got != "makercycle v9.8.7" {
		t.Errorf("-version = %q; want %q", got, "makercycle v9.8.7")
	}
}

// E2E-H3: -save-global copia o models.env do projeto para ~/.makercycle/config/models.env e sai com 0.
func TestE2ESaveGlobal(t *testing.T) {
	bin := buildE2EBinary(t)
	proj, home := t.TempDir(), t.TempDir()
	writeFiles(t, proj, map[string]string{".makercycle/config/models.env": "MC_MAX_ATTEMPTS=7\n"})
	r := runE2E(t, bin, proj, home, "", "-save-global")
	if r.Code != 0 {
		t.Fatalf("código = %d; want 0\n%s", r.Code, r.Out)
	}
	if got := readString(t, filepath.Join(home, ".makercycle", "config", "models.env")); got != "MC_MAX_ATTEMPTS=7\n" {
		t.Errorf("global = %q; want cópia do projeto", got)
	}
	if !strings.Contains(r.Out, "[OK]") {
		t.Errorf("saída deveria confirmar com [OK]:\n%s", r.Out)
	}
}

// E2E-X1: -save-global sem models.env no projeto falha (código != 0) e não cria o global.
func TestE2ESaveGlobalWithoutProjectFileFails(t *testing.T) {
	bin := buildE2EBinary(t)
	proj, home := t.TempDir(), t.TempDir()
	r := runE2E(t, bin, proj, home, "", "-save-global")
	if r.Code == 0 {
		t.Fatalf("código = 0; want != 0\n%s", r.Out)
	}
	if isFile(filepath.Join(home, ".makercycle", "config", "models.env")) {
		t.Error("global não deveria ser criado")
	}
}

// E2E-H4: -init num projeto novo herda o models.env global, extrai agentes, cria .gitkeep,
// atualiza .gitignore, confirma a stack e NÃO instala skills (global sem MC_*_SKILL) nem roda o SCOUT (sem código).
func TestE2EInitInheritsGlobalModelsEnv(t *testing.T) {
	bin := buildE2EBinary(t)
	proj, home := t.TempDir(), t.TempDir()
	writeFiles(t, proj, map[string]string{"go.mod": "module exemplo\n\ngo 1.22\n"})
	writeFiles(t, home, map[string]string{".makercycle/config/models.env": "MC_MAX_ATTEMPTS=4\n"})

	r := runE2E(t, bin, proj, home, "\n", "-init", "-docs-dir", "docs")

	for _, n := range []string{"00_SCOUT.md", "01_PLANNER.md", "02_CRITIC.md", "03_MAKER.md", "04_AUDITOR.md", "05_GATEKEEPER.md"} {
		got := readString(t, filepath.Join(proj, ".makercycle", "agents", n))
		if want := readRepoFile(t, filepath.Join(".makercycle", "agents", n)); got != want {
			t.Errorf("%s extraído difere do template do repositório", n)
		}
	}
	for _, sub := range []string{"demands", "logs", "references"} {
		if !isFile(filepath.Join(proj, "docs", sub, ".gitkeep")) {
			t.Errorf("docs/%s/.gitkeep ausente", sub)
		}
	}
	if got := readString(t, filepath.Join(proj, ".makercycle", "config", "models.env")); got != "MC_MAX_ATTEMPTS=4\n" {
		t.Errorf("models.env = %q; want herdado da home", got)
	}
	gi := readString(t, filepath.Join(proj, ".gitignore"))
	for _, e := range []string{".makercycle/state/", ".makercycle/telemetry/", ".makercycle/config/project.env"} {
		if !strings.Contains(gi, e) {
			t.Errorf(".gitignore sem %q", e)
		}
	}
	s, err := loadProjectStack(filepath.Join(proj, ".makercycle", "config", "project.env"))
	if err != nil || s.Name != "Go" {
		t.Errorf("stack confirmada = %+v (err %v); want Go\n%s", s, err, r.Out)
	}
	if strings.Contains(r.Out, "npx skills add") {
		t.Errorf("não deveria instalar skills sem MC_*_SKILL declaradas:\n%s", r.Out)
	}
	if strings.Contains(r.Out, "ETAPA 0") {
		t.Errorf("não deveria rodar o SCOUT sem arquivos de código:\n%s", r.Out)
	}
}

// E2E-H5: sem global → template embutido idêntico ao models.env do repositório;
// -init repetido preserva agentes customizados; -force restaura o template.
func TestE2EInitTemplateAndForce(t *testing.T) {
	bin := buildE2EBinary(t)
	proj, home := t.TempDir(), t.TempDir()

	runE2E(t, bin, proj, home, "\n", "-init")
	if got, want := readString(t, filepath.Join(proj, ".makercycle", "config", "models.env")), readRepoFile(t, ".makercycle/config/models.env"); got != want {
		t.Error("models.env extraído difere do template embutido do repositório")
	}

	planner := filepath.Join(proj, ".makercycle", "agents", "01_PLANNER.md")
	if err := os.WriteFile(planner, []byte("customizado\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runE2E(t, bin, proj, home, "\n", "-init")
	if got := readString(t, planner); got != "customizado\n" {
		t.Errorf("-init sem -force sobrescreveu agente customizado: %q", got)
	}

	runE2E(t, bin, proj, home, "\n", "-init", "-force")
	if got, want := readString(t, planner), readRepoFile(t, ".makercycle/agents/01_PLANNER.md"); got != want {
		t.Error("-init -force não restaurou o template do agente")
	}
	if isFile(filepath.Join(home, ".makercycle", "config", "models.env")) {
		t.Error("-init não deve gravar na home sem -save-global")
	}
}
