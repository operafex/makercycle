package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 7: instaladores install.sh (macOS/Linux) e install.ps1 (Windows)
// (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: IS-*, IW-*)
// ---------------------------------------------------------------------------

const releaseBaseURL = "https://github.com/operafex/makercycle/releases"

type scriptResult struct {
	Stdout, Stderr string
	Code           int
}

func runInstallSh(t *testing.T, env map[string]string) scriptResult {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh é POSIX")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh indisponível")
	}
	wd, _ := os.Getwd()
	cmd := exec.Command(sh, filepath.Join(wd, "install.sh"))
	cmd.Dir = t.TempDir()
	cmd.Env = []string{"PATH=/usr/bin:/bin", "MAKERCYCLE_DRY_RUN=1"}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	code := 0
	if ee, ok := runErr.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if runErr != nil {
		t.Fatalf("executando install.sh: %v", runErr)
	}
	return scriptResult{Stdout: out.String(), Stderr: errb.String(), Code: code}
}

// IS-H1: install.sh existe, é POSIX sh e passa em `sh -n`.
func TestInstallShSyntax(t *testing.T) {
	c := readRepoFile(t, "install.sh")
	if !strings.HasPrefix(c, "#!/bin/sh\n") {
		t.Error("install.sh deve começar com #!/bin/sh")
	}
	if !strings.Contains(c, "set -eu") {
		t.Error("install.sh deve usar `set -eu`")
	}
	if runtime.GOOS != "windows" {
		if out, err := exec.Command("sh", "-n", "install.sh").CombinedOutput(); err != nil {
			t.Errorf("sh -n install.sh: %v\n%s", err, out)
		}
	}
}

// IS-H2: mapeamento SO/arquitetura → asset (dry-run), com saída exata.
func TestInstallShResolvesAssets(t *testing.T) {
	cases := []struct{ os, arch, asset string }{
		{"Darwin", "arm64", "makercycle-darwin-arm64"},
		{"Darwin", "x86_64", "makercycle-darwin-amd64"},
		{"Linux", "x86_64", "makercycle-linux-amd64"},
		{"Linux", "amd64", "makercycle-linux-amd64"},
		{"Linux", "aarch64", "makercycle-linux-arm64"},
		{"Linux", "arm64", "makercycle-linux-arm64"},
	}
	for _, c := range cases {
		home := t.TempDir()
		r := runInstallSh(t, map[string]string{"HOME": home, "MAKERCYCLE_OS": c.os, "MAKERCYCLE_ARCH": c.arch})
		if r.Code != 0 {
			t.Fatalf("%s/%s: código %d\n%s%s", c.os, c.arch, r.Code, r.Stdout, r.Stderr)
		}
		want := "asset=" + c.asset + "\n" +
			"url=" + releaseBaseURL + "/latest/download/" + c.asset + "\n" +
			"checksums_url=" + releaseBaseURL + "/latest/download/checksums.txt\n" +
			"install_dir=" + filepath.Join(home, ".local", "bin") + "\n"
		if r.Stdout != want {
			t.Errorf("%s/%s: saída =\n%s\nwant\n%s", c.os, c.arch, r.Stdout, want)
		}
	}
}

// IS-E1: versão fixa e diretório de instalação personalizados.
func TestInstallShPinnedVersionAndInstallDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "bin")
	r := runInstallSh(t, map[string]string{
		"HOME": t.TempDir(), "MAKERCYCLE_OS": "Linux", "MAKERCYCLE_ARCH": "x86_64",
		"MAKERCYCLE_VERSION": "v0.2.0", "MAKERCYCLE_INSTALL_DIR": dir,
	})
	if r.Code != 0 {
		t.Fatalf("código %d\n%s", r.Code, r.Stderr)
	}
	for _, s := range []string{
		"url=" + releaseBaseURL + "/download/v0.2.0/makercycle-linux-amd64\n",
		"checksums_url=" + releaseBaseURL + "/download/v0.2.0/checksums.txt\n",
		"install_dir=" + dir + "\n",
	} {
		if !strings.Contains(r.Stdout, s) {
			t.Errorf("saída sem %q:\n%s", s, r.Stdout)
		}
	}
}

// IS-X1: SO ou arquitetura não suportados → código 1 e mensagem em stderr.
func TestInstallShRejectsUnsupportedPlatforms(t *testing.T) {
	for _, c := range [][2]string{{"FreeBSD", "x86_64"}, {"Linux", "i686"}, {"Darwin", "ppc64"}} {
		r := runInstallSh(t, map[string]string{"HOME": t.TempDir(), "MAKERCYCLE_OS": c[0], "MAKERCYCLE_ARCH": c[1]})
		if r.Code != 1 {
			t.Errorf("%s/%s: código = %d; want 1", c[0], c[1], r.Code)
		}
		if !strings.Contains(r.Stderr, "não suportad") {
			t.Errorf("%s/%s: stderr sem 'não suportad...': %q", c[0], c[1], r.Stderr)
		}
		if strings.Contains(r.Stdout, "url=") {
			t.Errorf("%s/%s: não deveria resolver URL", c[0], c[1])
		}
	}
}

// IS-X2: MAKERCYCLE_VERSION fora do formato vX.Y.Z[-pre] é rejeitada (evita injeção na URL).
func TestInstallShRejectsInvalidVersion(t *testing.T) {
	for _, v := range []string{"latest/../../x", "1.2.3", "v1.2", "v1.2.3;rm -rf /"} {
		r := runInstallSh(t, map[string]string{"HOME": t.TempDir(), "MAKERCYCLE_OS": "Linux", "MAKERCYCLE_ARCH": "x86_64", "MAKERCYCLE_VERSION": v})
		if r.Code != 1 {
			t.Errorf("versão %q: código = %d; want 1", v, r.Code)
		}
		if !strings.Contains(r.Stderr, "versão inválida") {
			t.Errorf("versão %q: stderr sem 'versão inválida': %q", v, r.Stderr)
		}
	}
}

// IS-H3: install.sh verifica checksum, marca executável e configura o PATH no rc do shell.
func TestInstallShContent(t *testing.T) {
	c := readRepoFile(t, "install.sh")
	for _, s := range []string{
		"operafex/makercycle", "checksums.txt", "sha256sum", "shasum -a 256",
		"chmod +x", "curl", "wget", ".zshrc", ".bashrc", ".profile",
		"MAKERCYCLE_INSTALL_DIR", "MAKERCYCLE_VERSION", "makercycle -init",
	} {
		if !strings.Contains(c, s) {
			t.Errorf("install.sh sem %q", s)
		}
	}
}

func runInstallPs1(t *testing.T, env map[string]string) scriptResult {
	t.Helper()
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh indisponível; validação dinâmica do install.ps1 ignorada")
	}
	wd, _ := os.Getwd()
	cmd := exec.Command(pwsh, "-NoProfile", "-NonInteractive", "-File", filepath.Join(wd, "install.ps1"))
	cmd.Env = append(os.Environ(), "MAKERCYCLE_DRY_RUN=1")
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	runErr := cmd.Run()
	code := 0
	if ee, ok := runErr.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if runErr != nil {
		t.Fatalf("executando install.ps1: %v", runErr)
	}
	return scriptResult{Stdout: strings.ReplaceAll(out.String(), "\r\n", "\n"), Stderr: errb.String(), Code: code}
}

// IW-H1: install.ps1 baixa da release, verifica hash e configura o PATH do usuário.
func TestInstallPs1Content(t *testing.T) {
	c := readRepoFile(t, "install.ps1")
	for _, s := range []string{
		"operafex/makercycle", "makercycle-windows-", ".exe", "checksums.txt", "Get-FileHash",
		"PROCESSOR_ARCHITECTURE", "ARM64", "AMD64", "LOCALAPPDATA",
		"[Environment]::SetEnvironmentVariable(", "'User'", "MAKERCYCLE_DRY_RUN",
		"MAKERCYCLE_INSTALL_DIR", "MAKERCYCLE_VERSION", "$ErrorActionPreference = 'Stop'", "makercycle -init",
	} {
		if !strings.Contains(c, s) {
			t.Errorf("install.ps1 sem %q", s)
		}
	}
}

// IW-H2 (dinâmico, só com pwsh): dry-run resolve o asset do Windows.
func TestInstallPs1DryRun(t *testing.T) {
	dir := t.TempDir()
	for arch, asset := range map[string]string{"AMD64": "makercycle-windows-amd64.exe", "ARM64": "makercycle-windows-arm64.exe"} {
		r := runInstallPs1(t, map[string]string{"MAKERCYCLE_ARCH": arch, "MAKERCYCLE_INSTALL_DIR": dir})
		if r.Code != 0 {
			t.Fatalf("%s: código %d\n%s", arch, r.Code, r.Stderr)
		}
		want := "asset=" + asset + "\n" +
			"url=" + releaseBaseURL + "/latest/download/" + asset + "\n" +
			"checksums_url=" + releaseBaseURL + "/latest/download/checksums.txt\n" +
			"install_dir=" + dir + "\n"
		if r.Stdout != want {
			t.Errorf("%s: saída =\n%s\nwant\n%s", arch, r.Stdout, want)
		}
	}
}

// IW-X1 (dinâmico, só com pwsh): arquitetura não suportada → código 1.
func TestInstallPs1RejectsUnsupportedArch(t *testing.T) {
	r := runInstallPs1(t, map[string]string{"MAKERCYCLE_ARCH": "x86", "MAKERCYCLE_INSTALL_DIR": t.TempDir()})
	if r.Code != 1 {
		t.Errorf("código = %d; want 1", r.Code)
	}
}
