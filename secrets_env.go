package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// secretsEnvPath é o secrets.env do projeto (git-ignored; só chaves de API).
const secretsEnvPath = ".makercycle/config/secrets.env"

// initActionMissing é a ação do -init quando não há secrets.env no projeto nem na home.
const initActionMissing = "ausente na home"

// envLayerPaths são as camadas de configuração, da mais fraca à mais forte
// (a variável exportada no SO vence todas).
var envLayerPaths = []string{projectEnvPath, modelsEnvPath, secretsEnvPath}

// globalSecretsEnvPath devolve <home>/.makercycle/config/secrets.env.
func globalSecretsEnvPath(home string) (string, error) {
	if strings.TrimSpace(home) == "" {
		return "", errors.New("diretório home indisponível")
	}
	return filepath.Join(home, ".makercycle", "config", "secrets.env"), nil
}

// provisionSecretsEnv garante o secrets.env do projeto: mantém o existente (nunca sobrescreve);
// senão copia o global (arquivo regular, permissão 0600). Sem global, nada é criado.
func provisionSecretsEnv(projectPath, globalPath string) (initFileResult, error) {
	if isFile(projectPath) {
		return initFileResult{Path: projectPath, Action: initActionKept}, nil
	}
	if globalPath == "" || !isFile(globalPath) {
		return initFileResult{Path: projectPath, Action: initActionMissing}, nil
	}
	data, err := os.ReadFile(globalPath)
	if err != nil {
		return initFileResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(projectPath), 0755); err != nil {
		return initFileResult{}, err
	}
	if err := writeFileAtomic(projectPath, data, 0600); err != nil {
		return initFileResult{}, err
	}
	return initFileResult{Path: projectPath, Action: initActionFromGlobal}, nil
}

// mergeEnvLayers lê as camadas em ordem (a posterior prevalece sobre a anterior) e devolve o mapa
// resultante, sem as chaves já exportadas (não vazias) no SO. Valor vazio não altera camadas
// inferiores; arquivo inexistente é ignorado; camada ilegível é pulada e reportada no erro.
func mergeEnvLayers(paths []string, getenv func(string) string) (map[string]string, error) {
	merged := map[string]string{}
	var errs []error
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("lendo %s: %w", path, err))
			}
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
			k, v, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			v = strings.Trim(strings.TrimSpace(v), `"'`)
			if k == "" || v == "" {
				continue
			}
			merged[k] = v
		}
	}
	if getenv != nil {
		for k := range merged {
			if strings.TrimSpace(getenv(k)) != "" {
				delete(merged, k)
			}
		}
	}
	return merged, errors.Join(errs...)
}

// loadEnvLayers aplica as camadas ao ambiente do processo (o SO exportado nunca é sobrescrito).
// Camadas legíveis valem mesmo quando outra falha; o erro é devolvido para aviso.
func loadEnvLayers(paths []string) error {
	m, err := mergeEnvLayers(paths, os.Getenv)
	for k, v := range m {
		_ = os.Setenv(k, v)
	}
	return err
}

// projectEnvKeyWarning avisa (sem valores) que o project.env, agora versionado, contém chaves de API.
func projectEnvKeyWarning(names []string) string {
	if len(names) == 0 {
		return ""
	}
	return "[AVISO] Chaves de API encontradas em .makercycle/config/project.env (versionado): " + strings.Join(names, ", ") + ". Mova-as para .makercycle/config/secrets.env (ignorado pelo git) e remova-as do project.env."
}
