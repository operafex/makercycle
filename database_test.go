package main

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Task 4 — promptWithTimeout / promptWithTimeoutFrom (database.go)
// Referência: docs/references/setup-ux-and-db-prompt.md §3
// ---------------------------------------------------------------------------

// PT-H1: o timeout padrão é uma variável de pacote de 3 minutos (substituível nos testes).
func TestDefaultPromptTimeoutIsThreeMinutesVar(t *testing.T) {
	if defaultPromptTimeout != 3*time.Minute {
		t.Fatalf("defaultPromptTimeout = %v, want 3m", defaultPromptTimeout)
	}
	orig := defaultPromptTimeout
	defaultPromptTimeout = time.Millisecond // compila apenas se for var
	defaultPromptTimeout = orig
}

func runPromptFrom(input, prompt string, timeout time.Duration) (string, bool, string) {
	var out bytes.Buffer
	got, ok := promptWithTimeoutFrom(bufio.NewReader(strings.NewReader(input)), &out, prompt, timeout)
	return got, ok, out.String()
}

// PT-H2: resposta lida antes do timeout volta aparada, com ok=true, e a pergunta é exibida.
func TestPromptWithTimeoutFromAnswer(t *testing.T) {
	got, ok, out := runPromptFrom("  2  \n", "Banco? ", time.Second)
	if got != "2" || !ok {
		t.Errorf("got=%q ok=%v, want \"2\" true", got, ok)
	}
	if !strings.Contains(out, "Banco? ") {
		t.Errorf("pergunta não exibida: %q", out)
	}
}

// PT-E1: texto sem quebra de linha seguido de EOF ainda é uma resposta.
func TestPromptWithTimeoutFromPartialLineBeforeEOF(t *testing.T) {
	if got, ok, _ := runPromptFrom("3", "? ", time.Second); got != "3" || !ok {
		t.Errorf("got=%q ok=%v, want \"3\" true", got, ok)
	}
}

// PT-E2: Enter vazio é uma resposta (ok=true) com texto vazio.
func TestPromptWithTimeoutFromEmptyLine(t *testing.T) {
	if got, ok, _ := runPromptFrom("\n", "? ", time.Second); got != "" || !ok {
		t.Errorf("got=%q ok=%v, want \"\" true", got, ok)
	}
}

// PT-E3: EOF imediato (ambiente não-interativo) devolve ("", false) sem esperar o timeout.
func TestPromptWithTimeoutFromImmediateEOFDoesNotBlock(t *testing.T) {
	start := time.Now()
	got, ok, _ := runPromptFrom("", "? ", 10*time.Second)
	if got != "" || ok {
		t.Errorf("got=%q ok=%v, want \"\" false", got, ok)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("EOF imediato não pode bloquear: levou %v", el)
	}
}

// PT-R1: sem resposta dentro do timeout devolve ("", false) logo após o prazo.
func TestPromptWithTimeoutFromTimesOut(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() }) // libera a goroutine de leitura pendente
	var out bytes.Buffer
	start := time.Now()
	got, ok := promptWithTimeoutFrom(bufio.NewReader(pr), &out, "? ", 50*time.Millisecond)
	el := time.Since(start)
	if got != "" || ok {
		t.Errorf("got=%q ok=%v, want \"\" false", got, ok)
	}
	if el < 50*time.Millisecond || el > 2*time.Second {
		t.Errorf("timeout de 50ms levou %v", el)
	}
}

// PT-E4: timeout <= 0 usa defaultPromptTimeout.
func TestPromptWithTimeoutFromNonPositiveUsesDefault(t *testing.T) {
	orig := defaultPromptTimeout
	defaultPromptTimeout = 50 * time.Millisecond
	t.Cleanup(func() { defaultPromptTimeout = orig })
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	start := time.Now()
	if _, ok := promptWithTimeoutFrom(bufio.NewReader(pr), io.Discard, "? ", 0); ok {
		t.Error("sem resposta deve devolver ok=false")
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("timeout 0 deveria cair no padrão de 50ms, levou %v", el)
	}
}

// swapStdin troca os.Stdin por um pipe com o conteúdo informado (writer fechado => EOF ao final).
func swapStdin(t *testing.T, content string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = orig; _ = r.Close() })
	if _, err := w.WriteString(content); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
}

// PT-H3: promptWithTimeout lê de os.Stdin.
func TestPromptWithTimeoutReadsOsStdin(t *testing.T) {
	swapStdin(t, "1\n")
	var got string
	var ok bool
	captureStdout(t, func() { got, ok = promptWithTimeout("Banco? ", time.Second) })
	if got != "1" || !ok {
		t.Errorf("got=%q ok=%v, want \"1\" true", got, ok)
	}
}

// PT-E5: promptWithTimeout com os.Stdin em EOF imediato devolve ("", false) sem bloquear.
func TestPromptWithTimeoutOsStdinEOF(t *testing.T) {
	swapStdin(t, "")
	start := time.Now()
	var got string
	var ok bool
	captureStdout(t, func() { got, ok = promptWithTimeout("Banco? ", 10*time.Second) })
	if got != "" || ok || time.Since(start) > 2*time.Second {
		t.Errorf("got=%q ok=%v em %v", got, ok, time.Since(start))
	}
}

// ---------------------------------------------------------------------------
// Task 5 — regras de decisão do banco (database.go)
// ---------------------------------------------------------------------------

// DB-H1: fallback por contexto — SQLite para CLIs/ferramentas locais, PostgreSQL para APIs/web/full-stack.
func TestAutoSelectDatabase(t *testing.T) {
	cases := []struct{ stack, ctx, want string }{
		{"", "", "SQLite"},
		{"Go", "", "SQLite"},
		{"Go", "CLI de linha de comando local", "SQLite"},
		{"Rust", "ferramenta local que processa rapidamente arquivos", "SQLite"}, // "rapidamente" não é "api"
		{"Node.js / TypeScript (Backend API)", "", "PostgreSQL"},
		{"Node.js / TypeScript (Frontend)", "", "PostgreSQL"},
		{"Python", "criar uma API REST de pedidos", "PostgreSQL"},
		{"Go", "serviço web de pagamentos", "PostgreSQL"},
		{"Go", "Servidor HTTP com endpoints", "PostgreSQL"},
		{"Ruby on Rails", "", "PostgreSQL"},
		{"Personalizado", "aplicação full-stack", "PostgreSQL"},
	}
	for _, c := range cases {
		if got := autoSelectDatabase(c.stack, c.ctx); got != c.want {
			t.Errorf("autoSelectDatabase(%q, %q) = %q, want %q", c.stack, c.ctx, got, c.want)
		}
	}
}

// DB-H2: palavras de persistência da referência, sem diferenciar maiúsculas.
func TestDemandMentionsPersistence(t *testing.T) {
	for _, s := range []string{
		"criar banco de usuários", "add database layer", "nova tabela de pedidos", "write a migration",
		"camada de persistência", "camada de persistencia", "CRUD de clientes", "alterar o Schema",
		"UserRepository em Go", "novo Repositório de pedidos",
	} {
		if !demandMentionsPersistence(s) {
			t.Errorf("demandMentionsPersistence(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "adicionar flag -version", "melhorar o spinner do console"} {
		if demandMentionsPersistence(s) {
			t.Errorf("demandMentionsPersistence(%q) = true, want false", s)
		}
	}
}

// DB-H3: gatilhos do -setup (projeto novo OU MC_DATABASE indefinido) e do -task (vazio E demanda de persistência).
func TestShouldPromptDatabase(t *testing.T) {
	setup := []struct {
		greenfield bool
		db         string
		want       bool
	}{
		{true, "PostgreSQL", true}, {true, "", true}, {false, "", true}, {false, "   ", true}, {false, "SQLite", false},
	}
	for _, c := range setup {
		if got := shouldPromptDatabaseOnSetup(c.greenfield, c.db); got != c.want {
			t.Errorf("shouldPromptDatabaseOnSetup(%v, %q) = %v, want %v", c.greenfield, c.db, got, c.want)
		}
	}
	task := []struct {
		db, task string
		want     bool
	}{
		{"", "criar tabela de pedidos", true}, {"  ", "CRUD de clientes", true},
		{"SQLite", "criar tabela de pedidos", false}, {"", "adicionar flag -version", false},
	}
	for _, c := range task {
		if got := shouldPromptDatabaseOnTask(c.db, c.task); got != c.want {
			t.Errorf("shouldPromptDatabaseOnTask(%q, %q) = %v, want %v", c.db, c.task, got, c.want)
		}
	}
}

// DB-H4/E1: MC_DATABASE do ambiente real prevalece; senão vem de project.env; ausente => "".
func TestCurrentDatabase(t *testing.T) {
	envPath := writeEnv(t, "MC_STACK_NAME=\"Go\"\nMC_DATABASE=\"MongoDB\"\n")
	none := func(string) string { return "" }
	if got := currentDatabase(envPath, none); got != "MongoDB" {
		t.Errorf("via project.env = %q, want MongoDB", got)
	}
	real := func(k string) string {
		if k == "MC_DATABASE" {
			return "PostgreSQL"
		}
		return ""
	}
	if got := currentDatabase(envPath, real); got != "PostgreSQL" {
		t.Errorf("ambiente real deve prevalecer: %q", got)
	}
	if got := currentDatabase(filepath.Join(t.TempDir(), "nada.env"), none); got != "" {
		t.Errorf("arquivo ausente = %q, want vazio", got)
	}
	if got := currentDatabase(writeEnv(t, "MC_DATABASE=\"  \"\n"), none); got != "" {
		t.Errorf("valor em branco = %q, want vazio", got)
	}
}

// DB-H5: contexto injetado nos prompts dos agentes.
func TestWithDatabaseContext(t *testing.T) {
	if got := withDatabaseContext("P.", ""); got != "P." {
		t.Errorf("sem banco o prompt não muda: %q", got)
	}
	if got := withDatabaseContext("P.", "  "); got != "P." {
		t.Errorf("banco em branco não muda o prompt: %q", got)
	}
	if got, want := withDatabaseContext("P.", "SQLite"), "P. Banco de dados do projeto (MC_DATABASE): SQLite."; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// Task 6 — chooseDatabase (database.go)
// ---------------------------------------------------------------------------

func runChoose(t *testing.T, reader *bufio.Reader, stack, ctx string, timeout time.Duration) (string, string, string, error) {
	t.Helper()
	envPath := filepath.Join(t.TempDir(), ".makercycle", "config", "project.env")
	writeFiles(t, filepath.Dir(filepath.Dir(filepath.Dir(envPath))), map[string]string{
		".makercycle/config/project.env": "MC_STACK_NAME=\"Go\"\nMC_TEST_CMD=\"go test ./...\"\n",
	})
	var out bytes.Buffer
	db, err := chooseDatabase(reader, &out, envPath, stack, ctx, timeout)
	return db, out.String(), envPath, err
}

func strReader(s string) *bufio.Reader { return bufio.NewReader(strings.NewReader(s)) }

// DB-H6: cada opção fixa do menu é persistida em project.env preservando as demais chaves.
func TestChooseDatabaseOptions(t *testing.T) {
	cases := map[string]string{"1\n": "PostgreSQL", "2\n": "SQLite", "3\n": "MySQL/MariaDB", "4\n": "MongoDB", "6\n": databaseNone}
	for input, want := range cases {
		db, out, envPath, err := runChoose(t, strReader(input), "Go", "", time.Second)
		if err != nil || db != want {
			t.Fatalf("entrada %q: db=%q err=%v", input, db, err)
		}
		for _, s := range []string{databaseMenuTitle, "[1] PostgreSQL", "[2] SQLite", "[3] MySQL/MariaDB",
			"[4] MongoDB", "[5] Personalizado", "[6] Sem banco / Decidir depois"} {
			if !strings.Contains(out, s) {
				t.Fatalf("menu de banco sem %q:\n%s", s, out)
			}
		}
		env := parseEnvContent(readString(t, envPath))
		if env["MC_DATABASE"] != want || env["MC_STACK_NAME"] != "Go" || env["MC_TEST_CMD"] != "go test ./..." {
			t.Errorf("entrada %q: project.env = %v", input, env)
		}
	}
}

// DB-H7: Personalizado pede o nome do banco.
func TestChooseDatabaseCustom(t *testing.T) {
	db, _, envPath, err := runChoose(t, strReader("5\nCockroachDB\n"), "Go", "", time.Second)
	if err != nil || db != "CockroachDB" {
		t.Fatalf("db=%q err=%v", db, err)
	}
	if env := parseEnvContent(readString(t, envPath)); env["MC_DATABASE"] != "CockroachDB" {
		t.Errorf("MC_DATABASE = %q", env["MC_DATABASE"])
	}
}

// DB-E1: Personalizado sem nome cai no fallback por contexto.
func TestChooseDatabaseCustomEmptyFallsBack(t *testing.T) {
	db, out, _, err := runChoose(t, strReader("5\n\n"), "Go", "", time.Second)
	if err != nil || db != "SQLite" || !strings.Contains(out, "[AVISO]") {
		t.Fatalf("db=%q err=%v\n%s", db, err, out)
	}
}

// DB-E2: EOF imediato (não-interativo) decide pelo contexto, avisa e persiste.
func TestChooseDatabaseEOFFallsBackByContext(t *testing.T) {
	db, out, envPath, err := runChoose(t, strReader(""), "Node.js / TypeScript (Backend API)", "", time.Second)
	if err != nil || db != "PostgreSQL" {
		t.Fatalf("db=%q err=%v", db, err)
	}
	if !strings.Contains(out, "[AVISO]") || !strings.Contains(out, "PostgreSQL") {
		t.Errorf("fallback deve avisar no console:\n%s", out)
	}
	if env := parseEnvContent(readString(t, envPath)); env["MC_DATABASE"] != "PostgreSQL" {
		t.Errorf("fallback não persistido: %v", env)
	}
}

// DB-R1: timeout expirado (timeout curto no teste) avisa, decide pelo contexto e persiste.
func TestChooseDatabaseTimeoutFallsBack(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	start := time.Now()
	db, out, envPath, err := runChoose(t, bufio.NewReader(pr), "Go", "CLI local de notas", 50*time.Millisecond)
	if err != nil || db != "SQLite" {
		t.Fatalf("db=%q err=%v", db, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Errorf("timeout curto não respeitado: %v", time.Since(start))
	}
	if !strings.Contains(out, "[AVISO]") || !strings.Contains(out, "SQLite") {
		t.Errorf("timeout deve avisar no console:\n%s", out)
	}
	if env := parseEnvContent(readString(t, envPath)); env["MC_DATABASE"] != "SQLite" {
		t.Errorf("fallback não persistido: %v", env)
	}
}

// DB-E3: Enter vazio ou opção inválida também caem no fallback por contexto (com aviso).
func TestChooseDatabaseEnterOrInvalidFallsBack(t *testing.T) {
	for _, input := range []string{"\n", "9\n", "postgres\n"} {
		db, out, _, err := runChoose(t, strReader(input), "Python", "API REST", time.Second)
		if err != nil || db != "PostgreSQL" || !strings.Contains(out, "[AVISO]") {
			t.Errorf("entrada %q: db=%q err=%v\n%s", input, db, err, out)
		}
	}
}

// DB-R2: nome personalizado inseguro (aspas) é rejeitado por writeProjectEnv e o erro é propagado.
func TestChooseDatabaseUnsafeCustomValueReturnsError(t *testing.T) {
	_, _, envPath, err := runChoose(t, strReader("5\nfoo\"bar\n"), "Go", "", time.Second)
	if err == nil {
		t.Fatal("valor com aspas deve retornar erro")
	}
	if env := parseEnvContent(readString(t, envPath)); env["MC_DATABASE"] != "" {
		t.Errorf("valor inseguro não pode ser gravado: %v", env)
	}
}
