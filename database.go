package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
)

// defaultPromptTimeout é o tempo máximo de espera por uma resposta interativa (var para testes rápidos).
var defaultPromptTimeout = 3 * time.Minute

const (
	databaseMenuTitle = "Qual banco de dados o projeto deve usar?"
	databaseNone      = "Nenhum"
)

var databaseOptions = []string{"PostgreSQL", "SQLite", "MySQL/MariaDB", "MongoDB", "Personalizado", "Sem banco / Decidir depois"}

var persistenceKeywords = []string{"banco", "database", "tabela", "migration", "persistência", "persistencia", "crud", "schema", "repository", "repositório"}

var webContextRe = regexp.MustCompile(`(?i)\b(apis?|web|https?|rest|graphql|grpc|backend|frontend|full-?stack|servidor|server|servi[cç]os?|saas|site|rails|django|flask|fastapi|express|nestjs|next\.?js)\b`)

// promptWithTimeout lê uma linha de os.Stdin com timeout. ok=false em timeout ou EOF sem texto.
func promptWithTimeout(prompt string, timeout time.Duration) (string, bool) {
	return promptWithTimeoutFrom(bufio.NewReader(os.Stdin), os.Stdout, prompt, timeout)
}

// promptWithTimeoutFrom é promptWithTimeout sobre um reader/writer explícitos. Em timeout a goroutine de
// leitura fica pendente no reader (o canal com buffer 1 evita que ela trave ao enviar), portanto esta deve
// ser a última leitura interativa do fluxo.
func promptWithTimeoutFrom(reader *bufio.Reader, out io.Writer, prompt string, timeout time.Duration) (string, bool) {
	if timeout <= 0 {
		timeout = defaultPromptTimeout
	}
	if out != nil {
		fmt.Fprint(out, prompt)
	}

	type lineResult struct {
		line string
		err  error
	}
	ch := make(chan lineResult, 1)
	go func() {
		line, err := reader.ReadString('\n')
		ch <- lineResult{line, err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case r := <-ch:
		t := strings.TrimSpace(r.line)
		if r.err != nil && t == "" {
			return "", false
		}
		return t, true
	case <-timer.C:
		return "", false
	}
}

// autoSelectDatabase decide o banco pelo contexto: PostgreSQL para API/web/full-stack, senão SQLite.
func autoSelectDatabase(stackName, context string) string {
	if webContextRe.MatchString(stackName + " " + context) {
		return "PostgreSQL"
	}
	return "SQLite"
}

// demandMentionsPersistence informa se a demanda cita persistência de dados.
func demandMentionsPersistence(task string) bool {
	lower := strings.ToLower(task)
	for _, kw := range persistenceKeywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

func shouldPromptDatabaseOnSetup(greenfield bool, currentDB string) bool {
	return greenfield || strings.TrimSpace(currentDB) == ""
}

func shouldPromptDatabaseOnTask(currentDB, task string) bool {
	return strings.TrimSpace(currentDB) == "" && demandMentionsPersistence(task)
}

// currentDatabase devolve MC_DATABASE: ambiente real > project.env. Vazio se indefinido.
func currentDatabase(envPath string, getenv func(string) string) string {
	if getenv != nil {
		if v := strings.TrimSpace(getenv("MC_DATABASE")); v != "" {
			return v
		}
	}
	data, err := os.ReadFile(envPath)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parseEnvContent(string(data))["MC_DATABASE"])
}

// withDatabaseContext anexa o banco de dados do projeto ao prompt do agente (sem banco, não altera nada).
func withDatabaseContext(prompt, database string) string {
	db := strings.TrimSpace(database)
	if db == "" {
		return prompt
	}
	return prompt + fmt.Sprintf(" Banco de dados do projeto (MC_DATABASE): %s.", db)
}

// chooseDatabase pergunta o banco de dados (com timeout), decide automaticamente quando não há resposta
// utilizável e persiste a decisão em MC_DATABASE.
func chooseDatabase(reader *bufio.Reader, out io.Writer, envPath, stackName, context string, timeout time.Duration) (string, error) {
	if timeout <= 0 {
		timeout = defaultPromptTimeout
	}
	fmt.Fprintf(out, "\n%s\n", databaseMenuTitle)
	for i, opt := range databaseOptions {
		fmt.Fprintf(out, "  [%d] %s\n", i+1, opt)
	}
	answer, ok := promptWithTimeoutFrom(reader, out,
		fmt.Sprintf("Escolha uma opção (1-6) [sem resposta em %s = decisão automática]: ", timeout), timeout)

	db, reason := "", ""
	switch {
	case !ok:
		reason = fmt.Sprintf("sem resposta em %s", timeout)
	case answer == "1", answer == "2", answer == "3", answer == "4":
		db = databaseOptions[answer[0]-'1']
	case answer == "6":
		db = databaseNone
	case answer == "5":
		name, nameOK := promptWithTimeoutFrom(reader, out, "Nome do banco de dados: ", timeout)
		if nameOK && name != "" {
			db = name
		} else {
			reason = "banco personalizado sem nome"
		}
	case answer == "":
		reason = "nenhuma opção escolhida"
	default:
		reason = fmt.Sprintf("opção inválida %q", answer)
	}

	if db == "" {
		db = autoSelectDatabase(stackName, context)
		fmt.Fprintf(out, "\n[AVISO] %s: banco de dados decidido automaticamente pelo contexto: %s\n", reason, db)
		logEvent("AVISO: %s: banco de dados decidido automaticamente pelo contexto: %s", reason, db)
	}

	if err := writeProjectEnv(envPath, map[string]string{"MC_DATABASE": db}); err != nil {
		return db, err
	}
	fmt.Fprintf(out, "[DB] Banco de dados salvo em %s: %s\n", envPath, db)
	return db, nil
}
