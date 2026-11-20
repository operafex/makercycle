package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// slugify — Task 1 (ver .makercycle/state/PLAN.md / .makercycle/state/TEST_CASES.md: S-*)
// ---------------------------------------------------------------------------

func TestSlugify(t *testing.T) {
	cases := []struct {
		id, in, want string
	}{
		{"S-H1", "Criar API de Usuários", "criar-api-de-usuários"},
		{"S-H2", "Etapa 2 do Loop", "etapa-2-do-loop"},
		{"S-H3", "já-existe", "já-existe"},
		{"S-E1", "  --Olá,   Mundo!!  ", "olá-mundo"},
		{"S-E2", "a___b...c", "a-b-c"},
		{"S-E3", "UPPER lower", "upper-lower"},
		{"S-E4", "tab\tnew\nline", "tab-new-line"},
		{"S-E5", "emoji 🚀 rocket", "emoji-rocket"},
		{"S-E6", "١٢٣ árabe", "١٢٣-árabe"},
		{"S-X1", "", "demanda"},
		{"S-X2", "!!!", "demanda"},
		{"S-X3", "   ", "demanda"},
		{"S-X4", "🚀🔥", "demanda"},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			if got := slugify(c.in); got != c.want {
				t.Errorf("slugify(%q) = %q; want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSlugifyTruncatesAt45Runes(t *testing.T) {
	// S-E7: corte por runes (não por bytes) — nunca gera UTF-8 inválido.
	got := slugify(strings.Repeat("ã", 100))
	if n := len([]rune(got)); n != 45 {
		t.Errorf("esperava 45 runes, obtive %d (%q)", n, got)
	}
	if !utf8.ValidString(got) {
		t.Errorf("slug com UTF-8 inválido: %q", got)
	}
}

func TestSlugifyTruncationDoesNotEndWithDash(t *testing.T) {
	// S-E8: rune 45 cai num separador → hífen final removido.
	in := strings.Repeat("a", 44) + " bbbb"
	got := slugify(in)
	if got != strings.Repeat("a", 44) {
		t.Errorf("slugify(%q) = %q; want 44 'a' sem hífen final", in, got)
	}
}

func TestSlugifyExactly45RunesUnchanged(t *testing.T) {
	// S-E9: fronteira — 45 runes exatos não são cortados.
	in := strings.Repeat("b", 45)
	if got := slugify(in); got != in {
		t.Errorf("slugify(45 runes) = %q; want inalterado", got)
	}
}

func TestSlugifyIsIdempotent(t *testing.T) {
	// S-E10
	for _, in := range []string{"Criar API de Usuários", "  --Olá,   Mundo!!  ", strings.Repeat("xy ", 40)} {
		once := slugify(in)
		if twice := slugify(once); twice != once {
			t.Errorf("não idempotente: slugify(%q)=%q, slugify(slugify)=%q", in, once, twice)
		}
	}
}

func TestSlugifyKeepsCombiningMarks(t *testing.T) {
	// S-R1 (RED): texto em NFD (comum em nomes de arquivo no macOS) não pode
	// quebrar a palavra no diacrítico combinante. "decisão" em NFD = "decisa"+U+0303+"o".
	cases := []struct{ in, want string }{
		{"decisão final", "decisão-final"},
		{"café com leite", "café-com-leite"},
	}
	for _, c := range cases {
		if got := slugify(c.in); got != c.want {
			t.Errorf("slugify(%q) = %q; want %q (marca combinante deve ser preservada)", c.in, got, c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseJSONUsage — Task 2 (P-*)
// ---------------------------------------------------------------------------

func TestParseJSONUsage(t *testing.T) {
	cases := []struct {
		id       string
		raw      string
		text     string
		stats    TokenStats
		hasUsage bool
		ok       bool
	}{
		{"P-H1", `{"result":"oi","usage":{"input_tokens":10,"output_tokens":5}}`,
			"oi", TokenStats{Input: 10, Output: 5, Total: 15}, true, true},
		{"P-H2", `{"response":"oi","usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":5,"totalTokenCount":15}}`,
			"oi", TokenStats{Input: 10, Output: 5, Total: 15}, true, true},
		{"P-H3", `{"content":"c","usage":{"prompt_tokens":7,"completion_tokens":3,"total_tokens":11}}`,
			"c", TokenStats{Input: 7, Output: 3, Total: 11}, true, true},
		{"P-H4", "banner da CLI\n{\"result\":\"ok\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n",
			"ok", TokenStats{Input: 1, Output: 1, Total: 2}, true, true},
		{"P-E1", `{"usage":{"input_tokens":1,"output_tokens":2},"usageMetadata":{"totalTokenCount":99}}`,
			"", TokenStats{Input: 1, Output: 2, Total: 3}, true, true},
		{"P-E2", `{"result":"","response":"segundo"}`,
			"segundo", TokenStats{}, false, true},
		{"P-E3", `{"result":42,"text":"t"}`,
			"t", TokenStats{}, false, true},
		{"P-E4", `{"result":"r","response":"s","content":"c","text":"t","message":"m"}`,
			"r", TokenStats{}, false, true},
		{"P-E5", `{"message":"m","usage":"não-é-mapa"}`,
			"m", TokenStats{}, false, true},
		{"P-E6", `{"result":"r","usage":{"input_tokens":0,"output_tokens":0}}`,
			"r", TokenStats{}, false, true},
		{"P-E7", `{"result":"r","usage":{"input_tokens":"10","output_tokens":"5"}}`,
			"r", TokenStats{}, false, true},
		{"P-E8", `{"usage":{"total_tokens":50}}`,
			"", TokenStats{Total: 50}, true, true},
		{"P-E9", `{}`, "", TokenStats{}, false, true},
		{"P-X1", "texto sem json", "", TokenStats{}, false, false},
		{"P-X2", "", "", TokenStats{}, false, false},
		{"P-X3", "} invertido {", "", TokenStats{}, false, false},
		{"P-X4", `{"result": "truncado"`, "", TokenStats{}, false, false},
		{"P-X5", `{not json}`, "", TokenStats{}, false, false},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			text, stats, has, ok := parseJSONUsage(c.raw)
			if text != c.text || stats != c.stats || has != c.hasUsage || ok != c.ok {
				t.Errorf("parseJSONUsage(%q)\n got:  text=%q stats=%+v has=%v ok=%v\n want: text=%q stats=%+v has=%v ok=%v",
					c.raw, text, stats, has, ok, c.text, c.stats, c.hasUsage, c.ok)
			}
		})
	}
}

func TestParseJSONUsageNeverMarksEstimated(t *testing.T) {
	// P-E10: dados vindos do JSON são reais, nunca estimados.
	_, s, _, _ := parseJSONUsage(`{"usage":{"input_tokens":3,"output_tokens":4}}`)
	if s.Estimated {
		t.Errorf("Estimated deveria ser false: %+v", s)
	}
}

func TestParseJSONUsageToleratesBraceNoise(t *testing.T) {
	// P-R1 (RED): execCapture mescla stdout+stderr; avisos com chaves antes ou
	// depois do payload não podem invalidar o JSON válido.
	payload := `{"result":"ok","usage":{"input_tokens":10,"output_tokens":5}}`
	cases := map[string]string{
		"P-R1a ruído antes":  "[warn] config {legacy} ignorada\n" + payload,
		"P-R1b ruído depois": payload + "\n[info] done {ok}",
		"P-R1c ambos":        "{x}\n" + payload + "\n{y}",
		"P-R1d log JSON sem chaves conhecidas antes": `{"level":"warn","msg":"slow"}` + "\n" + payload,
	}
	want := TokenStats{Input: 10, Output: 5, Total: 15}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			text, s, has, ok := parseJSONUsage(raw)
			if !ok || !has || text != "ok" || s != want {
				t.Errorf("parseJSONUsage(%q) = text=%q stats=%+v has=%v ok=%v; want text=\"ok\" stats=%+v has=true ok=true",
					raw, text, s, has, ok, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractOrEstimateTokens — Task 3 (T-*)
// ---------------------------------------------------------------------------

func TestExtractOrEstimateTokensParsesReportedUsage(t *testing.T) {
	cases := []struct {
		id, out  string
		in, outN int
	}{
		{"T-H1", `{"input_tokens": 12, "output_tokens": 34}`, 12, 34},
		{"T-H2", "prompt_tokens=7 completion_tokens=8", 7, 8},
		{"T-H3", "Input Tokens: 100\nOutput Tokens: 200", 100, 200},
		{"T-H4", "candidates-tokens: 5 input-tokens: 6", 6, 5},
		{"T-H5", "output_tokens: 9 ... input_tokens: 3", 3, 9},
		{"T-E1", `{"cache_creation_input_tokens": 9000, "input_tokens": 12, "output_tokens": 34}`, 12, 34},
		{"T-E2", `{"cache_read_input_tokens": 500, "max_output_tokens": 4096, "input_tokens": 1, "output_tokens": 2}`, 1, 2},
		{"T-E3", "input_tokens: 0 output_tokens: 0", 0, 0},
		{"T-E4", "INPUT_TOKENS='4' OUTPUT_TOKENS='6'", 4, 6},
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			s := extractOrEstimateTokens("prompt qualquer", c.out)
			want := TokenStats{Input: c.in, Output: c.outN, Total: c.in + c.outN, Estimated: false}
			if s != want {
				t.Errorf("extractOrEstimateTokens(_, %q) = %+v; want %+v", c.out, s, want)
			}
		})
	}
}

func TestExtractOrEstimateTokensFallsBackToEstimate(t *testing.T) {
	prompt := strings.Repeat("p", 400) // 400/4 + 1200 = 1300
	output := strings.Repeat("o", 80)  // 80/4 = 20
	want := TokenStats{Input: 1300, Output: 20, Total: 1320, Estimated: true}
	cases := map[string]string{
		"T-X1 sem métricas":       output,
		"T-X2 só input":           "input_tokens: 5" + strings.Repeat("o", 80-len("input_tokens: 5")),
		"T-X3 só output":          "output_tokens: 5" + strings.Repeat("o", 80-len("output_tokens: 5")),
		"T-X4 overflow int":       "input_tokens: 99999999999999999999999 output_tokens: 1" + strings.Repeat("o", 80-len("input_tokens: 99999999999999999999999 output_tokens: 1")),
		"T-X5 número negativo":    "input_tokens: -5 output_tokens: -9" + strings.Repeat("o", 80-len("input_tokens: -5 output_tokens: -9")),
		"T-X6 prefixo colado (_)": "my_input_tokens: 5 my_output_tokens: 6" + strings.Repeat("o", 80-len("my_input_tokens: 5 my_output_tokens: 6")),
	}
	for name, out := range cases {
		t.Run(name, func(t *testing.T) {
			if len(out) != 80 {
				t.Fatalf("fixture inválida: len=%d", len(out))
			}
			if s := extractOrEstimateTokens(prompt, out); s != want {
				t.Errorf("extractOrEstimateTokens = %+v; want %+v", s, want)
			}
		})
	}
}

func TestExtractOrEstimateTokensEmptyInputs(t *testing.T) {
	// T-E5: prompt e saída vazios → apenas o overhead fixo de 1200.
	want := TokenStats{Input: 1200, Output: 0, Total: 1200, Estimated: true}
	if s := extractOrEstimateTokens("", ""); s != want {
		t.Errorf("got %+v; want %+v", s, want)
	}
}

func TestExtractOrEstimateTokensEstimateUsesBytes(t *testing.T) {
	// T-E6: estimativa é por bytes (len), não por runes — "ã" = 2 bytes.
	s := extractOrEstimateTokens(strings.Repeat("ã", 8), strings.Repeat("ã", 8))
	if s.Input != 1204 || s.Output != 4 || s.Total != 1208 || !s.Estimated {
		t.Errorf("got %+v; want {1204 4 1208 true}", s)
	}
}

func TestExtractOrEstimateTokensThousandsSeparator(t *testing.T) {
	// T-R1 (RED): resumos legíveis de CLIs usam separador de milhar.
	cases := []struct {
		out      string
		in, outN int
	}{
		{"Input tokens: 12,345\nOutput tokens: 1,002", 12345, 1002},
		{"input_tokens: 1,234,567 output_tokens: 89", 1234567, 89},
		{`{"input_tokens": 12,"output_tokens": 34}`, 12, 34}, // vírgula JSON NÃO é separador
	}
	for _, c := range cases {
		s := extractOrEstimateTokens("p", c.out)
		want := TokenStats{Input: c.in, Output: c.outN, Total: c.in + c.outN}
		if s != want {
			t.Errorf("extractOrEstimateTokens(_, %q) = %+v; want %+v", c.out, s, want)
		}
	}
}

// ---------------------------------------------------------------------------
// updateIndexStatus — Task 4 (U-*)
// ---------------------------------------------------------------------------

const indexHeader = "# Índice\n\n| Data | Stack | Demanda | Arquivo | Status |\n| :--- | :--- | :--- | :--- | :--- |\n"

func indexRow(file, status string) string {
	return "| 2026-01-01 | Go | d | [`" + file + "`](" + file + ") | `" + status + "` |\n"
}

// setupIndex cria <tmp>/docs/{demands,logs,references} e demands/INDEX.md com o conteúdo dado.
func setupIndex(t *testing.T, content string) (docsDir, indexPath string) {
	t.Helper()
	docsDir = filepath.Join(t.TempDir(), "docs")
	if err := ensureDocsDirs(docsDir); err != nil {
		t.Fatal(err)
	}
	indexPath = filepath.Join(docsDir, "demands", "INDEX.md")
	if err := os.WriteFile(indexPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return docsDir, indexPath
}

func readString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestUpdateIndexStatus(t *testing.T) {
	// U-H1: apenas a linha da demanda alvo muda.
	docs, index := setupIndex(t, indexHeader+indexRow("2026-01-01-x.md", "IN_PROGRESS")+indexRow("2026-01-01-y.md", "IN_PROGRESS"))
	updateIndexStatus(filepath.Join(docs, "demands", "2026-01-01-x.md"), "APPROVED")
	want := indexHeader + indexRow("2026-01-01-x.md", "APPROVED") + indexRow("2026-01-01-y.md", "IN_PROGRESS")
	if got := readString(t, index); got != want {
		t.Errorf("índice inesperado:\n got:\n%s\n want:\n%s", got, want)
	}
}

func TestUpdateIndexStatusUnicodeFileName(t *testing.T) {
	// U-H2: nome com acentos (como as demandas reais).
	file := "2026-11-15-criar-testes-unitários.md"
	docs, index := setupIndex(t, indexHeader+indexRow(file, "IN_PROGRESS"))
	updateIndexStatus(filepath.Join(docs, "demands", file), "FAILED")
	if got := readString(t, index); got != indexHeader+indexRow(file, "FAILED") {
		t.Errorf("linha unicode não atualizada:\n%s", got)
	}
}

func TestUpdateIndexStatusUpdatesAllInProgressRowsOfSameDemand(t *testing.T) {
	// U-E1: demanda re-executada aparece 2x no índice → ambas atualizadas.
	f := "2026-01-01-x.md"
	docs, index := setupIndex(t, indexHeader+indexRow(f, "IN_PROGRESS")+indexRow(f, "IN_PROGRESS"))
	updateIndexStatus(filepath.Join(docs, "demands", f), "APPROVED")
	if got := readString(t, index); got != indexHeader+indexRow(f, "APPROVED")+indexRow(f, "APPROVED") {
		t.Errorf("todas as linhas IN_PROGRESS deveriam mudar:\n%s", got)
	}
}

func TestUpdateIndexStatusDoesNotTouchFinalizedRows(t *testing.T) {
	// U-E2: linhas já finalizadas (sem IN_PROGRESS) permanecem intactas.
	f := "2026-01-01-x.md"
	content := indexHeader + indexRow(f, "FAILED") + indexRow(f, "IN_PROGRESS")
	docs, index := setupIndex(t, content)
	updateIndexStatus(filepath.Join(docs, "demands", f), "APPROVED")
	if got := readString(t, index); got != indexHeader+indexRow(f, "FAILED")+indexRow(f, "APPROVED") {
		t.Errorf("linha finalizada foi alterada:\n%s", got)
	}
}

func TestUpdateIndexStatusNoSuffixCollision(t *testing.T) {
	// U-E3: "x.md" não pode casar com "2026-01-01-x.md".
	content := indexHeader + indexRow("2026-01-01-x.md", "IN_PROGRESS")
	docs, index := setupIndex(t, content)
	updateIndexStatus(filepath.Join(docs, "demands", "x.md"), "APPROVED")
	if got := readString(t, index); got != content {
		t.Errorf("demanda com sufixo igual foi alterada indevidamente:\n%s", got)
	}
}

func TestUpdateIndexStatusPreservesLayout(t *testing.T) {
	// U-E4: CRLF e ausência de newline final preservados byte a byte.
	f := "2026-01-01-x.md"
	content := "# Índice\r\n| a | [`" + f + "`](demands/" + f + ") | `IN_PROGRESS` |"
	docs, index := setupIndex(t, content)
	updateIndexStatus(filepath.Join(docs, "demands", f), "APPROVED")
	want := "# Índice\r\n| a | [`" + f + "`](demands/" + f + ") | `APPROVED` |"
	if got := readString(t, index); got != want {
		t.Errorf("layout alterado:\n got %q\nwant %q", got, want)
	}
}

func TestUpdateIndexStatusReplacesOnlyFirstStatusToken(t *testing.T) {
	// U-E5: se a descrição contiver `IN_PROGRESS`, só a 1ª ocorrência da linha muda (comportamento atual).
	f := "2026-01-01-x.md"
	row := "| d | [`" + f + "`](demands/" + f + ") | `IN_PROGRESS` | nota `IN_PROGRESS` |\n"
	docs, index := setupIndex(t, indexHeader+row)
	updateIndexStatus(filepath.Join(docs, "demands", f), "APPROVED")
	want := indexHeader + "| d | [`" + f + "`](demands/" + f + ") | `APPROVED` | nota `IN_PROGRESS` |\n"
	if got := readString(t, index); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestUpdateIndexStatusMissingIndexIsNoop(t *testing.T) {
	// U-X1: sem demands/INDEX.md → não cria arquivo nem entra em pânico.
	docs := filepath.Join(t.TempDir(), "docs")
	if err := ensureDocsDirs(docs); err != nil {
		t.Fatal(err)
	}
	updateIndexStatus(filepath.Join(docs, "demands", "x.md"), "APPROVED")
	if _, err := os.Stat(filepath.Join(docs, "demands", "INDEX.md")); !os.IsNotExist(err) {
		t.Errorf("índice não deveria ser criado: %v", err)
	}
}

func TestUpdateIndexStatusNoMatchDoesNotRewrite(t *testing.T) {
	// U-X2: sem linha correspondente → arquivo não é regravado (mtime intacto).
	content := indexHeader + indexRow("2026-01-01-y.md", "IN_PROGRESS")
	docs, index := setupIndex(t, content)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(index, old, old); err != nil {
		t.Fatal(err)
	}
	updateIndexStatus(filepath.Join(docs, "demands", "2026-01-01-x.md"), "APPROVED")
	fi, err := os.Stat(index)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.ModTime().Equal(old) {
		t.Errorf("índice regravado sem necessidade (mtime %v != %v)", fi.ModTime(), old)
	}
	if got := readString(t, index); got != content {
		t.Errorf("conteúdo alterado:\n%s", got)
	}
}

func TestUpdateIndexStatusEmptyDemandPathIsNoop(t *testing.T) {
	// U-X3: demandPath vazio não pode alterar nada (nem um índice no cwd).
	content := indexHeader + indexRow("2026-01-01-x.md", "IN_PROGRESS")
	_, index := setupIndex(t, content)
	updateIndexStatus("", "APPROVED")
	if got := readString(t, index); got != content {
		t.Errorf("conteúdo alterado:\n%s", got)
	}
}

func TestUpdateIndexStatusRejectsBlankStatus(t *testing.T) {
	// U-R1 (RED): status vazio/só espaços corromperia a tabela com "``";
	// deve ser no-op e manter `IN_PROGRESS`.
	f := "2026-01-01-x.md"
	content := indexHeader + indexRow(f, "IN_PROGRESS")
	for _, status := range []string{"", "   "} {
		docs, index := setupIndex(t, content)
		updateIndexStatus(filepath.Join(docs, "demands", f), status)
		if got := readString(t, index); got != content {
			t.Errorf("status %q alterou o índice:\n%s", status, got)
		}
	}
}

func TestEnsureDocsDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	if err := ensureDocsDirs(dir); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"demands", "logs"} {
		if fi, err := os.Stat(filepath.Join(dir, sub)); err != nil || !fi.IsDir() {
			t.Errorf("pasta %s não criada: %v", sub, err)
		}
	}
}

func TestRunLogFileCreationAndContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	l, err := initRunLog(dir, "minha-demanda")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}_\d{6}-minha-demanda\.log$`).MatchString(l.Name) {
		t.Errorf("nome de log inesperado: %s", l.Name)
	}
	if want := filepath.ToSlash(filepath.Join(dir, "logs", l.Name)); want != l.Path {
		t.Errorf("Path = %s; want %s", l.Path, want)
	}

	prev := runLog
	runLog = l
	defer func() { runLog = prev }()

	full := strings.Repeat("linha longa de resposta\n", 500)
	logEvent("evento %d", 42)
	logBlock("RESPOSTA", full)
	logBlock("SEM-NEWLINE", "abc")
	l.Close()

	b, err := os.ReadFile(filepath.Join(dir, "logs", l.Name))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !regexp.MustCompile(`(?m)^\[\d{2}:\d{2}:\d{2}\] evento 42$`).MatchString(got) {
		t.Errorf("evento sem timestamp:\n%s", got)
	}
	if !strings.Contains(got, full) {
		t.Error("resposta completa deveria estar no log sem cortes")
	}
	if !strings.Contains(got, "INÍCIO: SEM-NEWLINE") || !strings.Contains(got, "FIM: SEM-NEWLINE") {
		t.Errorf("delimitadores ausentes:\n%s", got)
	}
	if strings.Contains(got, "\033") {
		t.Error("log não deve conter códigos ANSI")
	}
}

func TestLogFunctionsNoopWithoutActiveLog(t *testing.T) {
	prev := runLog
	runLog = nil
	defer func() { runLog = prev }()
	logEvent("nada")
	logBlock("nada", "nada")
}

func TestRegisterDemandLinksLog(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	logName := "2026-11-15_163000-criar-x.log"
	demandPath := registerDemandInDocs(dir, "Go", "Criar x", "F1:opus", logName, nil)

	b, err := os.ReadFile(demandPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "- **Log de Execução:** [`" + logName + "`](../logs/" + logName + ")"
	if !strings.Contains(string(b), want) {
		t.Errorf("vínculo do log ausente em %s:\n%s", demandPath, b)
	}
	if strings.Contains(string(b), "Detalhamento por") || !strings.Contains(string(b), "## Descrição Original") {
		t.Errorf("estrutura da demanda alterada:\n%s", b)
	}

	idx, _ := os.ReadFile(filepath.Join(dir, "demands", "INDEX.md"))
	lines := strings.Split(strings.TrimSpace(string(idx)), "\n")
	if !strings.HasPrefix(lines[2], "| Data | Stack | Demanda | Arquivo | Status |") {
		t.Errorf("cabeçalho do índice alterado: %q", lines[2])
	}
	row := lines[len(lines)-1]
	if n := strings.Count(row, "|"); n != 6 { // 5 colunas
		t.Errorf("linha do índice deve ter 5 colunas, tem %d separadores: %q", n, row)
	}
	if strings.Contains(row, "logs/") {
		t.Errorf("índice não deve citar o log: %q", row)
	}

	// Status da demanda continua sincronizado com o índice.
	updateDemandStatus(demandPath, "APPROVED", 1, TokenStats{Total: 1})
	idx, _ = os.ReadFile(filepath.Join(dir, "demands", "INDEX.md"))
	if !strings.Contains(string(idx), "`APPROVED`") {
		t.Errorf("índice não sincronizado:\n%s", idx)
	}
}

func TestRegisterDemandExistingFileGetsNewLogLink(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	p := registerDemandInDocs(dir, "Go", "Mesma demanda", "m", "log-a.log", nil)
	registerDemandInDocs(dir, "Go", "Mesma demanda", "m", "log-b.log", nil)
	registerDemandInDocs(dir, "Go", "Mesma demanda", "m", "log-b.log", nil) // idempotente
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "(../logs/log-a.log)") || strings.Count(string(b), "(../logs/log-b.log)") != 1 {
		t.Errorf("vínculos de log incorretos:\n%s", b)
	}
}

func TestRegisterDemandWithoutLogOmitsField(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	p := registerDemandInDocs(dir, "Go", "Sem log", "m", "", nil)
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "Log de Execução") {
		t.Errorf("campo de log não deveria existir:\n%s", b)
	}
}

func TestEnsureDocsStructureSection(t *testing.T) {
	dir := t.TempDir()
	ctx := filepath.Join(dir, "PROJECT_CONTEXT.md")
	_ = os.WriteFile(ctx, []byte("# Contexto"), 0644)
	for i := 0; i < 2; i++ { // idempotente
		if err := ensureDocsStructureSection(ctx, "docs"); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(ctx)
	got := string(b)
	if strings.Count(got, docsStructureMarker) != 1 {
		t.Errorf("seção deveria aparecer uma vez:\n%s", got)
	}
	for _, want := range []string{"docs/demands/", "docs/logs/", "docs/references/", "demands/INDEX.md", "Log de Execução"} {
		if !strings.Contains(got, want) {
			t.Errorf("seção sem %q:\n%s", want, got)
		}
	}
	if err := ensureDocsStructureSection(filepath.Join(dir, "nao-existe.md"), "docs"); err == nil {
		t.Error("esperava erro para arquivo inexistente")
	}
}

func TestWithSpinnerTTY(t *testing.T) {
	var buf bytes.Buffer
	ran := false
	withSpinnerTo(&buf, true, "ETAPA 1: PLAN — PLANNER | opus", func() {
		ran = true
		time.Sleep(250 * time.Millisecond)
	})
	out := buf.String()
	if !ran {
		t.Fatal("fn não executou")
	}
	if !strings.Contains(out, "[ETAPA 1: PLAN — PLANNER | opus] Processando... (00:0") {
		t.Errorf("linha do spinner ausente: %q", out)
	}
	if !strings.Contains(out, "\r\033[K") || !strings.HasSuffix(out, "\r\033[K") {
		t.Errorf("deveria redesenhar na mesma linha e limpar ao final: %q", out)
	}
	if !strings.Contains(out, spinnerFrames[0]) || !strings.Contains(out, spinnerFrames[1]) {
		t.Errorf("frames Braille ausentes: %q", out)
	}
}

func TestWithSpinnerNonTTY(t *testing.T) {
	var buf bytes.Buffer
	withSpinnerTo(&buf, false, "ETAPA 3", func() { time.Sleep(150 * time.Millisecond) })
	out := buf.String()
	if out != "[AGUARDE] [ETAPA 3] Processando...\n" {
		t.Errorf("saída inesperada: %q", out)
	}
	if strings.Contains(out, "\033") || strings.Contains(out, "\r") {
		t.Error("fora de TTY não pode haver ANSI/CR")
	}
}

func TestSpinnerDoesNotInterfereWithCapture(t *testing.T) {
	var buf bytes.Buffer
	var out string
	withSpinnerTo(&buf, true, "x", func() {
		out, _ = runShellCommand("echo olá")
	})
	if strings.TrimSpace(out) != "olá" || strings.Contains(out, "\033") {
		t.Errorf("captura contaminada: %q", out)
	}
}

func TestFormatElapsed(t *testing.T) {
	if got := formatElapsed(74 * time.Second); got != "01:14" {
		t.Errorf("got %s", got)
	}
	if got := formatElapsed(0); got != "00:00" {
		t.Errorf("got %s", got)
	}
}

func TestAgentLabelAndHeader(t *testing.T) {
	a := AgentBinding{Label: "ETAPA 1", Stage: "PLAN", Alias: "PLANNER", Bin: "claude", Model: "opus", Skill: "superpowers"}
	if got := agentLabel(a); got != "ETAPA 1: PLAN — PLANNER | opus" {
		t.Errorf("label = %q", got)
	}
	h := buildRunHeader(StackConfig{Name: "Go", TestCmd: "go test ./..."}, []AgentBinding{a}, "", "", 1, 3, "docs")
	for _, want := range []string{"Stack: Go", "go test ./...", `modelo="opus"`, `skill="superpowers"`, "Demanda: (sem demanda)"} {
		if !strings.Contains(h, want) {
			t.Errorf("cabeçalho sem %q:\n%s", want, h)
		}
	}
}

func TestRunAgentLogsPromptAndFullResponse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	l, err := initRunLog(dir, "stage3")
	if err != nil {
		t.Fatal(err)
	}
	prev := runLog
	runLog = l
	defer func() { runLog = prev }()

	// "echo" faz o papel do binário do agente: devolve os argumentos (inclui o prompt) como resposta.
	a := AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Bin: "echo", Model: "m1"}
	stats, err := runAgent(a, "PROMPT-UNICO-123", false)
	l.Close()
	if err != nil || stats.Total <= 0 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "logs", l.Name))
	got := string(b)
	for _, want := range []string{"PROMPT ENVIADO — ETAPA 3: MAKE — MAKER | m1", "RESPOSTA RECEBIDA", "--model m1 -p PROMPT-UNICO-123", "concluído em"} {
		if !strings.Contains(got, want) {
			t.Errorf("log sem %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\033") {
		t.Error("log com ANSI")
	}
}

func TestCollectConfiguredSkills(t *testing.T) {
	env := []string{
		"MC_PLANNER_SKILL=obra/superpowers",
		"MC_CRITIC_SKILL=google/elephant-goldfish-model",
		"MC_AUDITOR_SKILL=cloudflare/security-audit-skill",
		"MC_OTHER_SKILL=obra/superpowers", // duplicada
		"MC_MAKER_SKILL=  ",               // vazia
		"MC_MAKER_MODEL=Gemini",           // não é skill
		"SKILL=x",
		"MC_SKILL_EXTRA=y",
		"PATH=/usr/bin",
	}
	got := collectConfiguredSkills(env)
	want := []string{"cloudflare/security-audit-skill", "google/elephant-goldfish-model", "obra/superpowers"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v; want %v", got, want)
	}
	if len(collectConfiguredSkills(nil)) != 0 {
		t.Error("sem env deveria retornar vazio")
	}
}

func TestSkillShortName(t *testing.T) {
	cases := map[string]string{
		"obra/superpowers":        "superpowers",
		"superpowers":             "superpowers",
		"a/b/c.git":               "c",
		"owner/repo@1.2.3":        "repo",
		"  cloudflare/sec-skill ": "sec-skill",
		"":                        "",
	}
	for in, want := range cases {
		if got := skillShortName(in); got != want {
			t.Errorf("skillShortName(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSkillInstalledInRoots(t *testing.T) {
	base, home := t.TempDir(), t.TempDir()
	roots := skillSearchRoots(base, home)
	if len(roots) != 4 {
		t.Fatalf("esperava 4 raízes, veio %v", roots)
	}
	for _, want := range []string{
		filepath.Join(base, "skills"), filepath.Join(base, ".agents", "skills"),
		filepath.Join(home, ".claude", "skills"), filepath.Join(home, ".antigravity", "skills"),
	} {
		found := false
		for _, r := range roots {
			found = found || r == want
		}
		if !found {
			t.Errorf("raiz ausente: %s", want)
		}
	}

	if skillInstalled("obra/superpowers", roots, "") {
		t.Error("nada instalado ainda")
	}
	for i, r := range roots {
		name := []string{"superpowers", "elephant-goldfish-model", "security-audit-skill", "outra"}[i]
		if err := os.MkdirAll(filepath.Join(r, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, sk := range []string{"obra/superpowers", "google/elephant-goldfish-model", "cloudflare/security-audit-skill"} {
		if !skillInstalled(sk, roots, "") {
			t.Errorf("%s deveria estar instalada", sk)
		}
	}
	// arquivo com o mesmo nome (não pasta) não conta
	_ = os.WriteFile(filepath.Join(base, "skills", "so-arquivo"), []byte("x"), 0644)
	if skillInstalled("a/so-arquivo", roots, "") {
		t.Error("arquivo não deveria contar como skill")
	}
}

func TestSkillInstalledViaLockFile(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "skills-lock.json")
	_ = os.WriteFile(lock, []byte(`{"version":2,"skills":{"ponytail":{"source":"DietrichGebert/ponytail"}}}`), 0644)
	if !skillInstalled("dietrichgebert/ponytail", nil, lock) {
		t.Error("source do lock deveria casar (case-insensitive)")
	}
	if skillInstalled("obra/superpowers", nil, lock) {
		t.Error("skill fora do lock não deveria casar")
	}
	_ = os.WriteFile(lock, []byte("{quebrado"), 0644)
	if skillInstalled("obra/superpowers", nil, lock) {
		t.Error("lock inválido deve ser ignorado")
	}
}

func TestMissingSkills(t *testing.T) {
	base := t.TempDir()
	roots := skillSearchRoots(base, "")
	_ = os.MkdirAll(filepath.Join(base, "skills", "superpowers"), 0755)
	got := missingSkills([]string{"obra/superpowers", "google/elephant-goldfish-model"}, roots, "")
	if len(got) != 1 || got[0] != "google/elephant-goldfish-model" {
		t.Errorf("missing = %v", got)
	}
}

func TestEnsureSkillsInstallsOnlyMissing(t *testing.T) {
	base := t.TempDir()
	roots := skillSearchRoots(base, "")
	_ = os.MkdirAll(filepath.Join(base, "skills", "superpowers"), 0755)

	var called []string
	install := func(s string) (string, error) {
		called = append(called, s)
		if s == "x/falha" {
			return "boom", os.ErrInvalid
		}
		return "ok", nil
	}
	skills := []string{"obra/superpowers", "google/elephant-goldfish-model", "x/falha"}
	res := ensureSkills(skills, roots, "", false, install)
	if strings.Join(called, ",") != "google/elephant-goldfish-model,x/falha" {
		t.Errorf("instalou: %v", called)
	}
	actions := []string{res[0].Action, res[1].Action, res[2].Action}
	if strings.Join(actions, ",") != "presente,instalada,falha" || res[2].Err == nil {
		t.Errorf("ações: %v", actions)
	}

	called = nil
	ensureSkills(skills, roots, "", true, install) // -force-skills
	if len(called) != 3 {
		t.Errorf("force deveria reinstalar todas: %v", called)
	}
}

func TestSkillInstallArgs(t *testing.T) {
	if got := strings.Join(skillInstallArgs("obra/superpowers"), " "); got != "skills add obra/superpowers -g -y" {
		t.Errorf("args = %q", got)
	}
}

func TestCheckBinaries(t *testing.T) {
	lp := func(n string) (string, error) {
		if n == "agy" {
			return "", os.ErrNotExist
		}
		return "/bin/" + n, nil
	}
	got := checkBinaries([]string{"claude", "agy", "claude", "", "npx", "git"}, lp)
	if len(got) != 4 {
		t.Fatalf("deveria deduplicar: %+v", got)
	}
	if !got[0].Found || got[0].Path != "/bin/claude" || got[1].Found || got[1].Name != "agy" {
		t.Errorf("status incorreto: %+v", got)
	}
}

func TestStageSkills(t *testing.T) {
	a := AgentBinding{Skill: "obra/superpowers"}
	s := AgentBinding{Skill: "google/elephant-goldfish-model"}
	se := AgentBinding{Skill: "cloudflare/security-audit-skill"}
	if got := stageSkills(1, a, s, se); len(got) != 3 {
		t.Errorf("etapa 1: %v", got)
	}
	if got := stageSkills(2, a, s, se); len(got) != 2 || got[0] != "google/elephant-goldfish-model" {
		t.Errorf("etapa 2: %v", got)
	}
	if got := stageSkills(3, a, s, se); len(got) != 1 || got[0] != "cloudflare/security-audit-skill" {
		t.Errorf("etapa 3: %v", got)
	}
}

func TestResolveStage(t *testing.T) {
	cases := []struct {
		stage   int
		want    int
		wantErr bool
	}{
		{1, 1, false},
		{3, 3, false},
		{4, 4, false}, // retomada na Etapa 4 (AUDITOR)
		{5, 5, false}, // retomada na Etapa 5 (GATEKEEPER)
		{6, 0, true},
		{0, 0, true},
	}
	for _, c := range cases {
		got, err := resolveStage(c.stage)
		if (err != nil) != c.wantErr || (!c.wantErr && got != c.want) {
			t.Errorf("resolveStage(%+v) = %d, %v", c, got, err)
		}
	}
}

func TestPreflightSkillsWarnsAndLogs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "docs")
	l, err := initRunLog(dir, "pre")
	if err != nil {
		t.Fatal(err)
	}
	prev := runLog
	runLog = l
	defer func() { runLog = prev }()
	preflightSkills([]string{"zzz/skill-que-nao-existe-xyz"})
	l.Close()
	b, _ := os.ReadFile(filepath.Join(dir, "logs", l.Name))
	if !strings.Contains(string(b), "./makercycle -init") || !strings.Contains(string(b), "zzz/skill-que-nao-existe-xyz") {
		t.Errorf("aviso não registrado:\n%s", b)
	}
}

func TestAgentLabelUsesEtapa(t *testing.T) {
	a := AgentBinding{Label: "ETAPA 3", Stage: "MAKE", Alias: "MAKER", Model: "m"}
	if got := agentLabel(a); got != "ETAPA 3: MAKE — MAKER | m" {
		t.Errorf("label = %q", got)
	}
}

func TestSkillInstalledGlobalLockAndAliases(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, ".skill-lock.json")
	local := filepath.Join(dir, "skills-lock.json")
	_ = os.WriteFile(global, []byte(`{"version":3,"skills":{"writing-plans":{"source":"obra/superpowers"},"egm-review":{"source":"google/elephant-goldfish-model"}}}`), 0644)
	lockPaths := local + string(os.PathListSeparator) + global // local ausente, global presente

	for _, sk := range []string{"obra/superpowers", "google/elephant-goldfish-model"} {
		if !skillInstalled(sk, nil, lockPaths) {
			t.Errorf("%s deveria casar pelo lock global", sk)
		}
	}
	if skillInstalled("cloudflare/security-audit-skill", nil, lockPaths) {
		t.Error("skill fora do lock não deveria casar")
	}

	// alias por chave do lock (sem source casando)
	_ = os.WriteFile(global, []byte(`{"skills":{"security-audit":{"source":"outro/fork"}}}`), 0644)
	if !skillInstalled("cloudflare/security-audit-skill", nil, global) {
		t.Error("alias security-audit-skill -> security-audit deveria casar pela chave do lock")
	}

	// alias por pasta
	roots := []string{filepath.Join(dir, "skills")}
	_ = os.MkdirAll(filepath.Join(roots[0], "writing-plans"), 0755)
	if !skillInstalled("obra/superpowers", roots, "") {
		t.Error("alias superpowers -> writing-plans deveria casar pela pasta")
	}
	if skillInstalled("google/elephant-goldfish-model", roots, "") {
		t.Error("egm-review não está instalada")
	}
}

func TestRunLogStripsANSI(t *testing.T) {
	l, err := initRunLog(t.TempDir(), "ansi")
	if err != nil {
		t.Fatal(err)
	}
	l.write("\x1b[32mverde\x1b[0m \x1b[?25l\x1b[2K sem cor\n")
	l.Close()
	b, _ := os.ReadFile(l.Path)
	if got, want := string(b), "verde  sem cor\n"; got != want {
		t.Errorf("log = %q; want %q", got, want)
	}
}

func TestParseJSONUsageSumsCacheTokens(t *testing.T) {
	_, s, has, ok := parseJSONUsage(`{"result":"r","usage":{"input_tokens":10,"output_tokens":5,"cache_creation_input_tokens":100,"cache_read_input_tokens":1000}}`)
	want := TokenStats{Input: 1110, Output: 5, Total: 1115}
	if !ok || !has || s != want {
		t.Errorf("got %+v has=%v ok=%v; want %+v", s, has, ok, want)
	}
	_, s, _, _ = parseJSONUsage(`{"result":"r","usage":{"input_tokens":10,"output_tokens":5,"cache_read_input_tokens":null}}`)
	if s.Input != 10 {
		t.Errorf("cache nulo não deve alterar Input: %+v", s)
	}
}

func chdirTemp(t *testing.T) string {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	return dir
}

func TestEnsureDocsDirsCreatesReferencesWithGitkeep(t *testing.T) {
	docs := filepath.Join(t.TempDir(), "docs")
	for i := 0; i < 2; i++ { // idempotente
		if err := ensureDocsDirs(docs); err != nil {
			t.Fatal(err)
		}
	}
	if info, err := os.Stat(filepath.Join(docs, "references", ".gitkeep")); err != nil || info.IsDir() {
		t.Errorf("docs/references/.gitkeep ausente: %v", err)
	}
}

func TestEnsureMakerCycleDirs(t *testing.T) {
	chdirTemp(t)
	if err := ensureMakerCycleDirs(); err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"agents", "config", "state", "telemetry"} {
		if info, err := os.Stat(filepath.Join(".makercycle", sub)); err != nil || !info.IsDir() {
			t.Errorf(".makercycle/%s ausente: %v", sub, err)
		}
	}
}

func TestUpdateIndexStatusLegacyDemandsLink(t *testing.T) {
	f := "2026-01-01-x.md"
	row := "| d | [`" + f + "`](demands/" + f + ") | `IN_PROGRESS` |\n"
	docs, index := setupIndex(t, indexHeader+row)
	updateIndexStatus(filepath.Join(docs, "demands", f), "APPROVED")
	if got := readString(t, index); !strings.Contains(got, "`APPROVED`") {
		t.Errorf("link legado não atualizado:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Telemetria CSV relacional: runs.csv (1 linha por execução) e events.csv (1 linha por etapa)
// ---------------------------------------------------------------------------

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll() // também valida que todas as linhas têm o mesmo nº de colunas
	if err != nil {
		t.Fatalf("CSV inválido em %s: %v", path, err)
	}
	return rows
}

func TestRecordRunTelemetryCreatesCSVWithHeaderAndRow(t *testing.T) {
	chdirTemp(t)
	recordRunTelemetry("123", "Go", "docs/demands/a.md", 3, 2, TokenStats{Input: 10, Output: 5, Total: 15}, "APPROVED", "ok")
	recordRunTelemetry("456", "Go", "", 3, 3, TokenStats{Input: 1, Output: 2, Total: 3}, "CIRCUIT_BREAKER_TRIPPED", "limite")

	rows := readCSV(t, filepath.Join(".makercycle", "telemetry", "runs.csv"))
	wantHeader := "RunID,Timestamp,Stack,DemandFile,MaxAttempts,TotalAttempts,TotalTokensIn,TotalTokensOut,TotalTokens,FinalStatus,Note"
	if len(rows) != 3 {
		t.Fatalf("esperava cabeçalho + 2 linhas, got %d: %v", len(rows), rows)
	}
	if got := strings.Join(rows[0], ","); got != wantHeader {
		t.Errorf("cabeçalho = %q; want %q", got, wantHeader)
	}
	want := []string{"123", "", "Go", "docs/demands/a.md", "3", "2", "10", "5", "15", "APPROVED", "ok"}
	for i, w := range want {
		if i == 1 {
			if _, err := time.Parse(time.RFC3339, rows[1][1]); err != nil {
				t.Errorf("Timestamp não é RFC3339: %q", rows[1][1])
			}
			continue
		}
		if rows[1][i] != w {
			t.Errorf("coluna %s = %q; want %q", rows[0][i], rows[1][i], w)
		}
	}
	if rows[2][3] != "(via .makercycle/state/PLAN.md)" {
		t.Errorf("demanda vazia deveria usar o marcador: %q", rows[2][3])
	}
}

func TestRecordEventTelemetryCreatesCSVWithHeaderAndRow(t *testing.T) {
	chdirTemp(t)
	recordEventTelemetry("123", "S3", "MAKER", "Gemini 3.8 Flash (High)", 2, TokenStats{Input: 7, Output: 3, Total: 10}, 12.3456)
	recordEventTelemetry("123", "S5", "GATEKEEPER", "m", 2, TokenStats{}, 0.5)

	rows := readCSV(t, filepath.Join(".makercycle", "telemetry", "events.csv"))
	wantHeader := "RunID,Timestamp,Stage,Agent,Model,Attempt,TokensIn,TokensOut,TotalTokens,DurationSeconds"
	if len(rows) != 3 {
		t.Fatalf("esperava cabeçalho + 2 linhas, got %d: %v", len(rows), rows)
	}
	if got := strings.Join(rows[0], ","); got != wantHeader {
		t.Errorf("cabeçalho = %q; want %q", got, wantHeader)
	}
	r := rows[1]
	for i, w := range map[int]string{0: "123", 2: "S3", 3: "MAKER", 4: "Gemini 3.8 Flash (High)", 5: "2", 6: "7", 7: "3", 8: "10", 9: "12.346"} {
		if r[i] != w {
			t.Errorf("coluna %s = %q; want %q", rows[0][i], r[i], w)
		}
	}
	if rows[2][9] != "0.500" {
		t.Errorf("duração = %q; want 0.500", rows[2][9])
	}
}

func TestTelemetryCSVEscapesSpecialCharsAndKeepsSingleHeader(t *testing.T) {
	chdirTemp(t)
	note := "nota com, vírgula, \"aspas\" e\nquebra"
	for i := 0; i < 3; i++ {
		recordRunTelemetry("1", "Go,Lang", "d.md", 3, 1, TokenStats{Total: 1}, "APPROVED", note)
	}
	rows := readCSV(t, filepath.Join(".makercycle", "telemetry", "runs.csv"))
	if len(rows) != 4 {
		t.Fatalf("esperava 1 cabeçalho + 3 linhas, got %d", len(rows))
	}
	if rows[1][2] != "Go,Lang" || rows[1][10] != note {
		t.Errorf("round-trip CSV falhou: stack=%q note=%q", rows[1][2], rows[1][10])
	}
	for _, r := range rows[1:] {
		if r[0] == "RunID" {
			t.Error("cabeçalho duplicado")
		}
	}
}

func TestTelemetryCSVAppendsToExistingFileWithoutRewritingHeader(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll(filepath.Join(".makercycle", "telemetry"), 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(".makercycle", "telemetry", "events.csv")
	if err := os.WriteFile(path, []byte(strings.Join(eventsCSVHeader, ",")+"\n"+"0,t,F1,A,m,1,1,1,2,0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	recordEventTelemetry("9", "S2", "CRITIC", "m", 1, TokenStats{Total: 1}, 1)
	rows := readCSV(t, path)
	if len(rows) != 3 || rows[2][0] != "9" {
		t.Errorf("append em arquivo existente falhou: %v", rows)
	}
}

func TestNewRunIDIsNumericAndUnique(t *testing.T) {
	a := newRunID()
	time.Sleep(time.Microsecond)
	b := newRunID()
	if !regexp.MustCompile(`^\d+$`).MatchString(a) || a == b {
		t.Errorf("RunID inválido ou repetido: %q %q", a, b)
	}
}

func TestRunAndEventShareRunIDForRelationalJoin(t *testing.T) {
	chdirTemp(t)
	id := newRunID()
	recordEventTelemetry(id, "S1", "PLANNER", "m", 1, TokenStats{Total: 1}, 1)
	recordRunTelemetry(id, "Go", "d.md", 3, 1, TokenStats{Total: 1}, "APPROVED", "ok")
	runs := readCSV(t, filepath.Join(".makercycle", "telemetry", "runs.csv"))
	events := readCSV(t, filepath.Join(".makercycle", "telemetry", "events.csv"))
	if runs[1][0] != id || events[1][0] != id {
		t.Errorf("RunID divergente: runs=%q events=%q", runs[1][0], events[1][0])
	}
}

func TestUpdateDemandStatusHasNoBreakdownLine(t *testing.T) {
	dir := t.TempDir()
	p := registerDemandInDocs(dir, "Go", "Criar y", "F1:opus", "", nil)
	updateDemandStatus(p, "APPROVED", 2, TokenStats{Input: 1, Output: 2, Total: 3})
	got := readString(t, p)
	for _, w := range []string{"- **Status:** `APPROVED`", "- **Tentativas:** `2`", "- **Tokens Totais:** `1 in / 2 out (3 total)`"} {
		if !strings.Contains(got, w) {
			t.Errorf("faltou %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "Detalhamento") {
		t.Errorf("linha de detalhamento não deveria existir:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// -ref: arquivos de referência para enriquecer a demanda
// ---------------------------------------------------------------------------

func TestResolveReferences(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll(filepath.Join("docs", "references"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"docs/references/auth.md", "notas.txt"} {
		if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := resolveReferences([]string{"auth.md", "./docs/references/auth.md", "notas.txt"}, "docs")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"docs/references/auth.md", "notas.txt"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %v; want %v (nome curto em references/, duplicata removida, caminho direto)", got, want)
	}
	if got, err := resolveReferences(nil, "docs"); err != nil || len(got) != 0 {
		t.Errorf("sem refs: got %v err=%v", got, err)
	}
}

func TestResolveReferencesErrors(t *testing.T) {
	chdirTemp(t)
	if err := os.MkdirAll(filepath.Join("docs", "references"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, in string }{{"inexistente", "nao-existe.md"}, {"vazio", "  "}, {"diretorio", "docs/references"}} {
		if _, err := resolveReferences([]string{c.in}, "docs"); err == nil {
			t.Errorf("%s: esperava erro para %q", c.name, c.in)
		}
	}
}

func TestReferencesPrompt(t *testing.T) {
	if referencesPrompt(nil) != "" {
		t.Error("sem refs, o prompt não deve mudar")
	}
	got := referencesPrompt([]string{"docs/references/a.md", "b.md"})
	if !strings.Contains(got, "@docs/references/a.md, @b.md.") {
		t.Errorf("prompt sem as referências: %q", got)
	}
}

func TestStringListFlagIsRepeatable(t *testing.T) {
	var l stringList
	_ = l.Set("a")
	_ = l.Set("b")
	if len(l) != 2 || l.String() != "a,b" {
		t.Errorf("got %v", l)
	}
}

func TestRegisterDemandListsReferences(t *testing.T) {
	dir := t.TempDir()
	refs := []string{"docs/references/auth.md", "openapi.yaml"}
	p := registerDemandInDocs(dir, "Go", "Com refs", "m", "log-a.log", refs)
	got := readString(t, p)
	if !strings.Contains(got, "- **Referências:** `docs/references/auth.md`, `openapi.yaml`\n") {
		t.Errorf("linha de referências ausente:\n%s", got)
	}
	if strings.Index(got, "- **Referências:**") > strings.Index(got, "## Descrição Original") {
		t.Errorf("referências devem ficar no cabeçalho da demanda:\n%s", got)
	}

	// Nova execução no mesmo dia: só as referências inéditas são acrescentadas; idempotente.
	registerDemandInDocs(dir, "Go", "Com refs", "m", "log-a.log", append(refs, "extra.md"))
	registerDemandInDocs(dir, "Go", "Com refs", "m", "log-a.log", append(refs, "extra.md"))
	got = readString(t, p)
	if n := strings.Count(got, "`extra.md`"); n != 1 {
		t.Errorf("extra.md deveria aparecer 1 vez, apareceu %d:\n%s", n, got)
	}
	if n := strings.Count(got, "`openapi.yaml`"); n != 1 {
		t.Errorf("openapi.yaml duplicado:\n%s", got)
	}
}

func TestRegisterDemandWithoutReferencesOmitsField(t *testing.T) {
	p := registerDemandInDocs(t.TempDir(), "Go", "Sem refs", "m", "", nil)
	if got := readString(t, p); strings.Contains(got, "Referências") {
		t.Errorf("campo não deveria existir:\n%s", got)
	}
}

func TestUpdateDemandStatusKeepsReferencesLine(t *testing.T) {
	p := registerDemandInDocs(t.TempDir(), "Go", "Refs status", "m", "", []string{"a.md"})
	updateDemandStatus(p, "APPROVED", 1, TokenStats{Total: 1})
	if got := readString(t, p); !strings.Contains(got, "- **Referências:** `a.md`") || !strings.Contains(got, "`APPROVED`") {
		t.Errorf("got:\n%s", got)
	}
}
