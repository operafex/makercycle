package main

import "strings"

// Version define a versão atual do makercycle seguindo SemVer.
// É uma variável (e não constante) para ser injetada no build por `-ldflags "-X main.Version=..."`.
var Version = "0.1.0"

// GetVersion retorna o nome da aplicação concatenado com o prefixo 'v' e a versão.
// Espaços são removidos e um único 'v' inicial é descartado, pois a tag do release já chega como "vX.Y.Z".
func GetVersion() string {
	return "makercycle v" + strings.TrimPrefix(strings.TrimSpace(Version), "v")
}
