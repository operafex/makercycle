package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

const (
	authModeAPIKey  = "api_key"
	authModeAnother = "another"
)

var providerAPIKeyNames = []string{"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GEMINI_API_KEY", "MC_API_KEY"}

func isProviderKey(key string) bool {
	for _, p := range providerAPIKeyNames {
		if p == key {
			return true
		}
	}
	return false
}

// readProjectAPIKeys lê as chaves de API dos provedores configuradas em path (project.env).
// Mantida como fallback legado: as chaves atuais ficam em secrets.env e chegam via os.Getenv (loadEnvLayers).
// Sempre retorna um mapa não nil (vazio se o arquivo não existir ou se não houver chaves válidas).
func readProjectAPIKeys(path string) (map[string]string, error) {
	result := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return result, nil
		}
		return result, fmt.Errorf("lendo %s: %w", path, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if !isProviderKey(k) {
			continue
		}
		cleaned := cleanEnvValue(v)
		if cleaned == "" {
			continue
		}
		if _, exists := result[k]; !exists {
			result[k] = cleaned
		}
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("lendo %s: %w", path, err)
	}
	return result, nil
}

// projectAwareGetenv retorna uma função de consulta de variáveis que prioriza o ambiente real (getenv)
// e recorre às chaves lidas do project.env caso a variável esteja ausente ou vazia no ambiente.
func projectAwareGetenv(keys map[string]string, getenv func(string) string) func(string) string {
	return func(k string) string {
		if getenv != nil {
			v := getenv(k)
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
		if keys != nil {
			if v, ok := keys[k]; ok {
				return v
			}
		}
		return ""
	}
}

// resolveAuthMode normaliza e valida o modo de autenticação informado.
// Retorna "api_key" como padrão se raw estiver vazio.
func resolveAuthMode(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return authModeAPIKey, nil
	}
	normalized := strings.ToLower(trimmed)
	switch normalized {
	case authModeAPIKey:
		return authModeAPIKey, nil
	case authModeAnother:
		return authModeAnother, nil
	default:
		return "", fmt.Errorf("MC_AUTH_MODE inválido: %q (use %q)", trimmed, authModeAPIKey)
	}
}

// validateAuthConfig valida as configurações de autenticação para o modo selecionado.
func validateAuthConfig(authMode string, getenv func(string) string) error {
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	switch authMode {
	case authModeAnother:
		return nil
	case authModeAPIKey:
		if strings.EqualFold(strings.TrimSpace(getenv("MC_OMNIROUTE_ENABLED")), "true") {
			return nil
		}
		for _, k := range providerAPIKeyNames {
			if cleanEnvValue(getenv(k)) != "" {
				return nil
			}
		}
		return errors.New("nenhuma API key encontrada: adicione ANTHROPIC_API_KEY, OPENAI_API_KEY ou GEMINI_API_KEY em .makercycle/config/secrets.env (MC_API_KEY também é aceita)")
	default:
		return fmt.Errorf("modo de autenticação inválido: %q", authMode)
	}
}

// OmniRouteConfig armazena a configuração de fallback via OmniRoute.
type OmniRouteConfig struct {
	Enabled bool
	URL     string
	APIKey  string
	Model   string
}

// loadOmniRouteConfig lê as variáveis de ambiente MC_OMNIROUTE_* aplicando defaults.
func loadOmniRouteConfig(getenv func(string) string) OmniRouteConfig {
	cfg := OmniRouteConfig{
		Enabled: false,
		URL:     "http://localhost:20128",
		APIKey:  "omniroute-local",
		Model:   "claude-sonnet-4-6",
	}
	if getenv == nil {
		return cfg
	}
	if strings.EqualFold(strings.TrimSpace(getenv("MC_OMNIROUTE_ENABLED")), "true") {
		cfg.Enabled = true
	}
	if v := strings.TrimSpace(getenv("MC_OMNIROUTE_URL")); v != "" {
		cfg.URL = v
	}
	if v := strings.TrimSpace(getenv("MC_OMNIROUTE_API_KEY")); v != "" {
		cfg.APIKey = v
	}
	if v := strings.TrimSpace(getenv("MC_OMNIROUTE_MODEL")); v != "" {
		cfg.Model = v
	}
	return cfg
}

// cleanEnvValue remove espaços e aspas externas (simples ou duplas) de um valor de variável de ambiente.
func cleanEnvValue(v string) string {
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(v), `"'`))
}

// buildAgentEnv cria uma nova slice de variáveis de ambiente para a execução do agente.
// Nunca muta base.
func buildAgentEnv(base []string, authMode string, omni OmniRouteConfig, useOmni bool) []string {
	if useOmni {
		var res []string
		for _, kv := range base {
			if strings.HasPrefix(kv, "ANTHROPIC_BASE_URL=") || strings.HasPrefix(kv, "ANTHROPIC_AUTH_TOKEN=") {
				continue
			}
			res = append(res, kv)
		}
		res = append(res, "ANTHROPIC_BASE_URL="+omni.URL)
		res = append(res, "ANTHROPIC_AUTH_TOKEN="+omni.APIKey)
		return res
	}

	if authMode == authModeAnother {
		var res []string
		for _, kv := range base {
			if strings.HasPrefix(kv, "LLM_API_KEY=") {
				continue
			}
			res = append(res, kv)
		}
		return res
	}

	if authMode == authModeAPIKey {
		vals := make(map[string]string)
		for _, kv := range base {
			k, v, ok := strings.Cut(kv, "=")
			if !ok {
				continue
			}
			if !isProviderKey(k) {
				continue
			}
			if cleaned := cleanEnvValue(v); cleaned != "" && vals[k] == "" {
				vals[k] = cleaned
			}
		}

		anthropic := vals["ANTHROPIC_API_KEY"]
		if anthropic == "" {
			anthropic = vals["MC_API_KEY"]
		}

		res := make([]string, 0, len(base)+3)
		for _, kv := range base {
			if strings.HasPrefix(kv, "LLM_API_KEY=") ||
				strings.HasPrefix(kv, "ANTHROPIC_API_KEY=") ||
				strings.HasPrefix(kv, "OPENAI_API_KEY=") ||
				strings.HasPrefix(kv, "GEMINI_API_KEY=") {
				continue
			}
			res = append(res, kv)
		}

		if anthropic != "" {
			res = append(res, "ANTHROPIC_API_KEY="+anthropic)
		}
		if v := vals["OPENAI_API_KEY"]; v != "" {
			res = append(res, "OPENAI_API_KEY="+v)
		}
		if v := vals["GEMINI_API_KEY"]; v != "" {
			res = append(res, "GEMINI_API_KEY="+v)
		}
		return res
	}

	res := make([]string, len(base))
	copy(res, base)
	return res
}

// apiKeysInModelsEnv vasculha content em busca de chaves de API dos provedores ou LLM_API_KEY.
// Retorna os nomes únicos e ordenados que possuam valor não vazio, ou nil caso nenhuma seja encontrada.
func apiKeysInModelsEnv(content string) []string {
	seen := make(map[string]bool)
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if !isProviderKey(k) && k != "LLM_API_KEY" {
			continue
		}
		if cleanEnvValue(v) != "" {
			seen[k] = true
		}
	}

	if len(seen) == 0 {
		return nil
	}

	var res []string
	for k := range seen {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

// modelsEnvKeyWarning gera a mensagem de aviso caso chaves de API sejam encontradas em models.env.
func modelsEnvKeyWarning(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return "[AVISO] Chaves de API encontradas em .makercycle/config/models.env (versionado): " + strings.Join(names, ", ") + ". Mova-as para .makercycle/config/secrets.env (ignorado pelo git) e remova-as do models.env."
}
