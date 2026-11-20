package main

import "embed"

// embeddedAgentsDir é o caminho dos contratos dos agentes dentro de embeddedAgentsFS.
const embeddedAgentsDir = ".makercycle/agents"

// embeddedAgentsFS carrega os contratos .makercycle/agents/*.md dentro do binário.
//
//go:embed all:.makercycle/agents
var embeddedAgentsFS embed.FS

// embeddedModelsEnv é o template de .makercycle/config/models.env distribuído no binário.
// Nunca deve conter chaves de provedor (o binário é público).
//
//go:embed all:.makercycle/config/models.env
var embeddedModelsEnv []byte
