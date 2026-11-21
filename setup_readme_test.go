package main

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Task 6 — READMEs: `makercycle -setup` no lugar de `go run . -setup`
// ---------------------------------------------------------------------------

func TestReadmesUseInstalledBinaryForSetup(t *testing.T) {
	for _, file := range []string{"README.md", "README.pt-BR.md"} {
		t.Run(file, func(t *testing.T) {
			c := readRepoFile(t, file)
			if strings.Contains(c, "go run . -setup") {
				t.Errorf("%s ainda sugere `go run . -setup`; use `makercycle -setup`", file)
			}
			if !strings.Contains(c, "```bash\nmakercycle -setup\n```") {
				t.Errorf("%s deve trazer o bloco de onboarding com `makercycle -setup`", file)
			}
			setupHead := strings.Index(c, "-setup` (")
			setupCmd := strings.Index(c, "```bash\nmakercycle -setup\n```")
			if setupHead < 0 || setupCmd < setupHead {
				t.Errorf("%s: o comando `makercycle -setup` deve ficar na seção -setup", file)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Task 7 — READMEs: PATH após `go install`, validação do binário e -init
// ---------------------------------------------------------------------------

const goInstallPathBlock = "```bash\n" +
	"echo 'export PATH=\"$PATH:$(go env GOPATH)/bin\"' >> ~/.zshrc\n" +
	"source ~/.zshrc\n" +
	"which makercycle\n" +
	"makercycle -init\n" +
	"```"

func TestReadmesDocumentPathAfterGoInstall(t *testing.T) {
	nextSection := map[string]string{
		"README.md":       "**Without Go**",
		"README.pt-BR.md": "**Sem Go**",
	}
	for file, next := range nextSection {
		t.Run(file, func(t *testing.T) {
			c := readRepoFile(t, file)
			install := strings.Index(c, "go install github.com/operafex/makercycle@latest")
			block := strings.Index(c, goInstallPathBlock)
			nextIdx := strings.Index(c, next)
			if install < 0 {
				t.Fatalf("%s perdeu o comando go install", file)
			}
			if block < 0 {
				t.Fatalf("%s deve conter o bloco de PATH:\n%s", file, goInstallPathBlock)
			}
			if nextIdx < 0 {
				t.Fatalf("%s perdeu a seção %q", file, next)
			}
			if !(install < block && block < nextIdx) {
				t.Errorf("%s: o bloco de PATH deve vir logo após o go install e antes de %q", file, next)
			}
			if strings.Count(c, goInstallPathBlock) != 1 {
				t.Errorf("%s: o bloco de PATH deve aparecer uma única vez", file)
			}
			if !strings.Contains(c[install:block], "$(go env GOPATH)/bin") {
				t.Errorf("%s: explique entre o go install e o bloco que o binário fica em $(go env GOPATH)/bin", file)
			}
		})
	}
}
