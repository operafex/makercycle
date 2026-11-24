[English](README.md)

# MakerCycle

Orquestrador CLI multiagente autônomo para engenharia de software em ciclo fechado (SCAN → PLAN → CHALLENGE → MAKE → VERIFY → GATE). Orientado a especificações, TDD estrito e auditoria de segurança automatizada. Escrito em Go **sem dependências externas** (apenas a biblioteca padrão).

## Um Ciclo, Seis Etapas

**O MakerCycle (`makercycle`)** é o ciclo completo. Cada fase interna é uma **Etapa**:

| Etapa | Nome | Agente | Cliente | Modelo Padrão | O que faz |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 0 | `SCAN` | `SCOUT` | `claude` | `opus` | Mapeia um projeto existente e consolida a documentação em `<docs>/PROJECT_CONTEXT.md`. |
| 1 | `PLAN` | `PLANNER` | `claude` | `opus` | Planejamento incremental (`obra/superpowers`) e testes automatizados em estado `RED`. |
| 2 | `CHALLENGE` | `CRITIC` | `agy` | `Gemini 3.8 Flash (Medium)` | Revisão crítica a frio e invariantes globais (`google/elephant-goldfish-model`). |
| 3 | `MAKE` | `MAKER` | `agy` | `Gemini 3.8 Flash (High)` | Implementação em contexto higienizado (*stateless*). |
| 4 | `VERIFY: CHECK` / `VERIFY: HARDEN` | `AUDITOR` | `agy` *(4B)* | `Gemini 3.8 Flash (Medium)` *(4B)* | Suíte determinística da stack (4A) + auditoria AppSec no `git diff` (4B, `cloudflare/security-audit-skill`). |
| 5 | `GATE` | `GATEKEEPER` | `agy` | `Gemini 3.8 Flash (High)` | Julgamento semântico, contabilização de tokens e *Circuit Breaker* (`max-attempts`). |

> **Nota:** Na Etapa 4 (`VERIFY`), a fase 4A (`CHECK`) executa a suíte de testes e linters locais deterministicamente (sem uso de LLM), enquanto a fase 4B (`HARDEN`) executa a auditoria AppSec via agente `AUDITOR` com o cliente e modelo indicados.

As Etapas 3 → 4 → 5 rodam em loop fechado até o GATEKEEPER aprovar, pedir correção de teste, escalar para o arquiteto ou o limite de tentativas ser atingido.

## Requisitos

- **Go 1.22+** *(somente para compilar do código-fonte ou rodar com `makercycle`; não é necessário usando os instaladores)*
- **Git** (usado no controle de histórico e auditoria de `git diff`)
- **Pelo menos uma CLI de agente de IA** instalada e configurada (ex.: `claude`, `gemini`, `codex`, `opencode` — configurável livremente por etapa em `.makercycle/config/models.env`)
- **Node.js / `npx`** *(opcional)*: exigido apenas ao rodar `-init` para instalar automaticamente skills externas declaradas em `models.env`

## Instalação

**Com Go (recomendado)** *(requer Go 1.22+)*

```bash
go install github.com/operafex/makercycle@latest
```

O binário é instalado em `$(go env GOPATH)/bin` (normalmente `~/go/bin`). Adicione essa pasta ao seu `PATH` (exemplo em zsh; no bash use `~/.bashrc`), valide o binário e inicialize o projeto:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
which makercycle
makercycle -init
```


**Sem Go** — binários pré-compilados são publicados a cada release para `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64`, `windows/amd64` e `windows/arm64`. Use o instalador de uma linha para o seu sistema:

*macOS / Linux*

```bash
curl -fsSL https://raw.githubusercontent.com/operafex/makercycle/main/install.sh | sh
```

*Windows (PowerShell)*

```powershell
irm https://raw.githubusercontent.com/operafex/makercycle/main/install.ps1 | iex
```

Os instaladores baixam do GitHub Releases o binário do seu SO/arquitetura, **verificam o SHA-256 contra o `checksums.txt`** (abortando se divergir), instalam e adicionam a pasta ao `PATH`.

| Variável | Padrão | Finalidade |
| :--- | :--- | :--- |
| `MAKERCYCLE_VERSION` | `latest` | Fixa uma release, ex.: `MAKERCYCLE_VERSION=v0.2.0` (formato `vX.Y.Z[-pre]`) |
| `MAKERCYCLE_INSTALL_DIR` | `~/.local/bin` (macOS/Linux) · `%LOCALAPPDATA%\Programs\makercycle` (Windows) | Onde o binário é instalado |

Verifique a instalação:

```bash
makercycle -version
```

## Uso Rápido

### 1. Inicializar em um comando — `makercycle -init`

```bash
makercycle -init                # provisiona o projeto (pode ser repetido com segurança)
makercycle -init -force         # também restaura os contratos dos agentes para os templates embutidos
makercycle -init -force-skills  # também reinstala/atualiza todas as skills
```

O binário é autossuficiente: os contratos dos agentes (`.makercycle/agents/*.md`) e o template do `models.env` vão embutidos nele. O `makercycle -init`:

1. cria `.makercycle/{agents,config,state,telemetry}/` e extrai os seis contratos de agente para `.makercycle/agents/`. Arquivos existentes são **mantidos**; o `-force` sobrescreve somente `.makercycle/agents/*.md`;
2. cria `<docs>/demands/`, `<docs>/logs/` e `<docs>/references/` (cada uma com `.gitkeep`) e grava o próprio log em `<docs>/logs/`;
3. provisiona `.makercycle/config/models.env`: um arquivo existente **nunca** é sobrescrito (nem com `-force`); caso contrário, é copiado de `~/.makercycle/config/models.env` quando este existir (herança global) ou gravado a partir do template embutido;
4. acrescenta `.makercycle/state/`, `.makercycle/telemetry/` e `.makercycle/config/secrets.env` ao `.gitignore` (idempotente; entradas equivalentes como `/.makercycle/state` são reconhecidas). Também copia `~/.makercycle/config/secrets.env` para o projeto quando ele ainda não tem um (`0600`; um arquivo já existente no projeto nunca é sobrescrito, nem com `-force`);
5. verifica no `$PATH` os binários configurados (e o `git`), mostrando `[OK]` ou `[AUSENTE]`. O `npx` só é exigido quando há skills declaradas;
6. instala as skills declaradas em `MC_*_SKILL` (todas, com `-force-skills`). Sem nenhuma declarada, a instalação de skills é ignorada. Uma skill conta como presente se existir uma pasta com o nome dela em `skills/`, `.agents/skills/`, `~/.claude/skills/` ou `~/.antigravity/skills/`, ou se o `skills-lock.json` a listar como origem;
7. detecta e confirma a stack do projeto (salva em `.makercycle/config/project.env`);
8. se o projeto já tiver arquivos de código, executa o **SCAN** (Etapa 0, SCOUT) para gerar `<docs>/PROJECT_CONTEXT.md`. Sem chaves de API válidas o SCAN é ignorado com um aviso (conclua depois com `-setup`).

Termina com código `1` se faltar algum binário, a instalação de alguma skill falhar ou o SCAN não puder rodar.

#### `models.env` global (`-save-global`)

Ajuste o `.makercycle/config/models.env` uma vez e reaproveite em todo projeto novo:

```bash
makercycle -save-global   # copia .makercycle/config/models.env para ~/.makercycle/config/models.env
```

O arquivo é gravado de forma atômica com permissão `0600` (pasta `0700`) e sobrescreve qualquer cópia global anterior. Projetos novos inicializados com `makercycle -init` herdam automaticamente o `~/.makercycle/config/models.env`. Nunca coloque chaves de API nesse arquivo — elas ficam em `.makercycle/config/secrets.env` (ignorado pelo Git, `0600`).

### 2. Onboarding de projeto existente — `-setup` (Etapa 0)

```bash
makercycle -setup
```

Cria `<docs>/PROJECT_CONTEXT.md`, `<docs>/demands/INDEX.md`, `<docs>/demands/`, `<docs>/logs/` e `<docs>/references/`.

### Detecção Automática e Personalização de Stack

Durante o `-setup`, o `makercycle` inspeciona deterministicamente marcadores na raiz do projeto:
- `go.mod` → Go (`go test -v -race ./...`, `go vet ./...` e `golangci-lint run` se existir `.golangci.yml` ou `.golangci.yaml`);
- `Gemfile` com `gem 'rails'` → Ruby on Rails (`bundle exec rspec` se existir o diretório `spec/`, senão `bin/rails test`, além de `bundle exec rubocop`);
- `package.json` → Node.js ou TypeScript + Node.js (se existir `tsconfig.json` ou `typescript` em dependencies/devDependencies; `npm run lint` se a chave `lint` estiver em `scripts`).

Durante o `-setup`, a stack detectada é exibida e a confirmação é solicitada antes de qualquer gravação:

```text
[STACK] Detectada: Go (testes: go test -v -race ./...)
Confirmar esta stack como padrão do projeto? [S/n] (ou digite 'p' para personalizar): 
```

Enter, `S` ou `s` confirma a stack detectada; `p` (ou `n`) abre o menu de stacks para escolher outra; qualquer outra resposta aborta o setup sem gravar. Quando nenhuma stack é detectada, não há pergunta. Em seguida, o agente `SCOUT` confirma ou ajusta a stack, e a configuração é gravada em `.makercycle/config/project.env`:

```env
MC_STACK_NAME="Go"
MC_TEST_CMD="go test -v -race ./..."
MC_TYPE_CMD="go vet ./..."
MC_LINT_CMD=""
```

Ao iniciar uma demanda com `-task`, `-stage` ou `-demand`, o `makercycle` usa a stack salva em `.makercycle/config/project.env` em silêncio — o fluxo não exige confirmação de stack a cada rodada:

```text
[INFO] Stack do projeto: Go (via project.env)
```

Para trocar a stack, use `-select-stack`: ele abre o menu mesmo com um `project.env` válido e salva a escolha. O menu também abre automaticamente quando o `project.env` está ausente ou incompleto.

```bash
makercycle -task "Implementar autenticação JWT" -select-stack
```

O menu do `-select-stack` é assim:

```text
Qual stack deve ser utilizada nesta execução?
  [1] Projeto Atual (Auto-detectado): Go
  [2] Ruby on Rails
  [3] Go
  [4] TypeScript + Node.js
  [5] Personalizada
Escolha uma opção (1-5) [Enter = 1]: 
```

Pressionar Enter seleciona a opção `1`. Se `.makercycle/config/project.env` estiver ausente ou incompleto, um aviso é exibido e o menu manual (opções 1 a 4) é apresentado como fallback. Você também pode editar `.makercycle/config/project.env` manualmente para customizar comandos ou definir outras stacks (como Python, Java ou Makefile).

### 3. Iniciar nova demanda — `-task` (Etapas 1 a 5)

```bash
makercycle -task "Implementar autenticação JWT com refresh token"
```

Enriqueça a demanda com material de `<docs>/references/` usando `-ref` (repetível). Cada arquivo é validado antes de tudo e entregue ao PLANNER como `@caminho`, então é lido antes do planejamento:

```bash
makercycle -task "Implementar autenticação JWT com refresh token" \
  -ref auth-spec.md -ref docs/references/openapi.yaml
```

`-ref` aceita um caminho (relativo ao diretório atual ou absoluto) ou apenas o nome de um arquivo dentro de `<docs>/references/` (acompanha `-docs-dir` / `MC_DOCS_DIR`). Arquivo inexistente aborta antes de qualquer agente rodar. `-ref` só afeta a Etapa 1, então é ignorado (com aviso) ao iniciar com `-stage 2` ou superior.

Comece de uma Etapa posterior com `-stage`:

```bash
makercycle -stage 2    # a partir de CHALLENGE (reaproveita .makercycle/state/PLAN.md / testes existentes)
makercycle -stage 3    # a partir de MAKE
```

### Loop autônomo de especificação

As Etapas 1 (PLANNER) e 2 (CRITIC) formam um ciclo fechado. Quando o CRITIC grava `SPEC_STATUS: REVISION_REQUIRED`, a Etapa 1 é reexecutada automaticamente: o PLANNER recebe `@.makercycle/state/SPEC_REVIEW.md` e deve corrigir `PLAN.md`, `TEST_CASES.md` e os testes RED. O loop se repete até o CRITIC aprovar (`SPEC_STATUS: APPROVED`) ou esgotar `MC_MAX_ATTEMPTS` / `-max-attempts` rodadas. A cada nova rodada é exibido:

```text
[AVISO] CRITIC solicitou revisão. Reexecutando Etapa 1 (Tentativa 2/3)...
```

O código de saída `2` só ocorre ao esgotar as tentativas. Os tokens de todas as rodadas (inclusive a interrompida por cota) entram no total da execução.

### Retomada de Execução e Vinculação de Demandas

Você pode retomar execuções interrompidas ou vincular demandas existentes:

- `-resume`: Restaura o estado exato gravado em `.makercycle/state/RESUME.env` após interrupções (como limites de cota).
- `-stage <1..5>`: Inicia a execução diretamente em qualquer etapa (1 a 5).
- `-demand <arquivo.md>`: Vincula uma demanda existente em `<docsDir>/demands/` (ex.: `-demand 2026-11-17-feature.md`) sem gerar duplicações no índice.

### Modo de revisão manual (`-manual`)

Com `-manual`, o MakerCycle pausa ao fim de cada etapa para você revisar o artefato, editá-lo e decidir o próximo passo:

```bash
makercycle -task "Adicionar endpoint X" -manual
makercycle -task -manual "Adicionar endpoint X"   # a flag pode vir em qualquer posição
makercycle -resume -manual
```

Etapas que pausam e o artefato a revisar:

| Etapa | Artefato |
| :--- | :--- |
| E0 SCAN (`-setup`) | `<docsDir>/PROJECT_CONTEXT.md` |
| E1 PLAN | `.makercycle/state/PLAN.md` |
| E2 CHALLENGE | `.makercycle/state/SPEC_REVIEW.md` |
| E3 MAKE | `git diff` |
| E4A VERIFY: CHECK | `.makercycle/state/VERIFY_REPORT.md` |
| E4B VERIFY: HARDEN | `.makercycle/state/SECURITY_REPORT.md` |

Teclas em cada pausa:

- `[Enter]` / `[c]`: continua para a próxima etapa (artefatos editados são recarregados).
- `[r]`: refaz a etapa, com feedback opcional de uma linha anexado ao prompt do agente (a E4A não usa LLM: apenas reexecuta os checks).
- `[s]`: salva o checkpoint e sai com código `0`. O `RESUME.env` guarda o modo manual (status `MANUAL_PAUSED`); continue com `makercycle -resume`. Use `-resume -manual=false` para desligar o modo na retomada.
- `[q]`: aborta. A execução é registrada como `ABORTED` em `runs.csv`/`TELEMETRY.md` e na demanda, e o código de saída é `6`.

O `-manual` exige um terminal interativo: com a entrada padrão fechada (pipe, `< /dev/null`), a primeira pausa falha com um erro claro. Etapa que falha (cota ou erro) não pausa.

### 4. Testar combinações de modelos

Edite `.makercycle/config/models.env` ou sobrescreva via variáveis de ambiente (veja detalhes completos na seção [Personalização de Modelos](#personalização-de-modelos)):

```bash
MC_MAKER_MODEL="GPT-OSS 120B (Medium)" makercycle -task "Adicionar paginação por cursor"
```

Cada agente tem `MC_<AGENTE>_BIN`, `_MODEL` e `_ARGS` (flags extras do CLI, ex.: permissões necessárias no modo headless). `--dangerously-skip-permissions` desativa os prompts de permissão: use apenas em repositório confiável.

## Personalização de Modelos

Os clientes de CLI, modelos e parâmetros de cada agente são centralizados e personalizáveis no arquivo:

📁 **`.makercycle/config/models.env`**

Esse arquivo é lido automaticamente na inicialização do `makercycle` e pode ser versionado no repositório com as preferências do projeto.

### Variáveis de Configuração por Agente

Cada agente possui variáveis dedicadas para configurar o cliente de CLI (`_BIN`), o modelo (`_MODEL`), argumentos extras de execução (`_ARGS`) e, quando aplicável, a skill utilizada (`_SKILL`):

| Etapa | Agente | Cliente / Binário | Modelo | Argumentos Extras | Skill |
| :--- | :--- | :--- | :--- | :--- | :--- |
| 0 | `SCOUT` | `MC_SCOUT_BIN` | `MC_SCOUT_MODEL` | `MC_SCOUT_ARGS` | — |
| 1 | `PLANNER` | `MC_PLANNER_BIN` | `MC_PLANNER_MODEL` | `MC_PLANNER_ARGS` | `MC_PLANNER_SKILL` |
| 2 | `CRITIC` | `MC_CRITIC_BIN` | `MC_CRITIC_MODEL` | `MC_CRITIC_ARGS` | `MC_CRITIC_SKILL` |
| 3 | `MAKER` | `MC_MAKER_BIN` | `MC_MAKER_MODEL` | `MC_MAKER_ARGS` | — |
| 4B | `AUDITOR` | `MC_AUDITOR_BIN` | `MC_AUDITOR_MODEL` | `MC_AUDITOR_ARGS` | `MC_AUDITOR_SKILL` |
| 5 | `GATEKEEPER` | `MC_GATEKEEPER_BIN` | `MC_GATEKEEPER_MODEL` | `MC_GATEKEEPER_ARGS` | — |

Além dos agentes, o `.makercycle/config/models.env` controla variáveis globais do fluxo:
- `MC_MAX_ATTEMPTS`: Limite de tentativas no loop de correção (padrão: `3`).
- `MC_DEFAULT_DOCS_DIR`: Diretório padrão de documentação (padrão: `docs`).

### Autenticação e Fallback OmniRoute

Por padrão (`MC_AUTH_MODE="api_key"`, definido em `.makercycle/config/models.env`), os agentes se autenticam com chaves de API dos provedores que residem **exclusivamente** em `.makercycle/config/secrets.env` (ignorado pelo git, gravado com permissão `0600`), isolando os custos por projeto:
- `ANTHROPIC_API_KEY`: chave da Anthropic (`MC_API_KEY` é aceita como alias).
- `OPENAI_API_KEY`: chave da OpenAI.
- `GEMINI_API_KEY`: chave do Google Gemini.

Todas as chaves configuradas são injetadas no subprocesso de cada agente. A validação pre-flight do modo `api_key` exige ao menos uma delas (variáveis do ambiente têm precedência sobre o arquivo). O `makercycle` emite um aviso se encontrar chaves de API no `.makercycle/config/models.env` versionado.

Em caso de limite de cota ou rate limit, o `makercycle` realiza fallback transparente para o `OmniRoute`:
- `MC_OMNIROUTE_ENABLED`: Ativa o fallback automático via OmniRoute quando `true` (padrão: `false`).
- `MC_OMNIROUTE_URL`: Endereço do servidor OmniRoute (padrão: `http://localhost:20128`).
- `MC_OMNIROUTE_MODEL`: Modelo roteado via OmniRoute (padrão: `claude-sonnet-4-6`).

### Segredos e precedência de configuração

A configuração fica em três arquivos em `.makercycle/config/`:

| Arquivo | Versionado? | Conteúdo |
| :--- | :--- | :--- |
| `project.env` | sim | Stack e comandos (`MC_STACK_NAME`, `MC_TEST_CMD`, …) e configurações de docs |
| `models.env` | sim | Modelos, binários, argumentos e skills dos agentes (`MC_<AGENTE>_*`) |
| `secrets.env` | **não** (ignorado pelo git, `0600`) | Somente chaves de API |

No `makercycle -init`, o `~/.makercycle/config/secrets.env` é copiado para o projeto quando ele ainda não tem um, de modo que uma chave configurada uma vez na home vale em todo projeto novo. Os valores nunca são impressos nem registrados em log — apenas os nomes das variáveis aparecem nos avisos.

As variáveis são carregadas nesta ordem, cada camada sobrepondo as anteriores: `project.env -> models.env -> secrets.env -> variáveis exportadas no SO`. Uma variável não vazia já exportada no shell sempre vence, e um valor vazio nunca apaga o de uma camada inferior. Se um arquivo versionado ainda contiver chaves de API, o `makercycle` avisa a cada execução e pede para movê-las ao `secrets.env`.

### Cross-Auditing (Multi-LLM)

Utilize até 5 modelos diferentes nas etapas. É fortemente recomendado usar no mínimo 2 provedores distintos — ex.: Anthropic no MAKER e OpenAI/Gemini no GATEKEEPER — para evitar viés de confirmação na auditoria do código. Configure cada etapa via `MC_<AGENTE>_BIN` / `MC_<AGENTE>_MODEL` no `.makercycle/config/models.env` e guarde as chaves dos provedores correspondentes no `.makercycle/config/secrets.env` (ignorado pelo git, `0600`).

### Formas de Personalização

A precedência de configuração segue: **Flags CLI > Variáveis de Ambiente > `.makercycle/config/models.env` > Valores padrão em código**.

1. **Editar o arquivo `.makercycle/config/models.env`** (recomendado para configurações permanentes do projeto):
   ```env
   # Exemplo: ajustar modelos da Etapa 3 e Etapa 5
   MC_MAKER_MODEL="Gemini 3.8 Flash (High)"
   MC_GATEKEEPER_MODEL="Gemini 3.8 Flash (High)"
   ```

2. **Sobrescrever via variáveis de ambiente** (ideal para testes pontuais sem alterar arquivos):
   ```bash
   MC_MAKER_MODEL="GPT-OSS 120B (Medium)" makercycle -task "Adicionar paginação por cursor"
   ```

3. **Sobrescrever via flags do CLI**:
   ```bash
   makercycle -maker-model "Gemini 3.8 Flash (High)" -gatekeeper-model "Gemini 3.8 Flash (High)" -task "Adicionar paginação por cursor"
   ```

## Estrutura gerada

```text
<docs>/
├── PROJECT_CONTEXT.md     # contexto do projeto (Etapa 0)
├── demands/               # um arquivo por demanda, ligado ao log da execução
│   └── INDEX.md           # | Data | Stack | Demanda | Arquivo | Status |
├── logs/                  # um log completo por execução: AAAA-MM-DD_HHMMSS-<slug>.log
└── references/            # material de referência (criada por -init / -setup)
.makercycle/
├── agents/                # contratos dos agentes (00_SCOUT.md ... 05_GATEKEEPER.md)
├── config/                # models.env (versionado), project.env (versionado), secrets.env (ignorado pelo git)
├── state/                 # artefatos transitórios: PLAN, TEST_CASES, DECISION, ... (ignorado)
└── telemetry/             # métricas em CSV (cabeçalho criado no primeiro uso, depois só append)
    ├── runs.csv           # uma linha por execução: tentativas, tokens in/out/total, status final
    ├── events.csv         # uma linha por etapa/agente: tokens, duração, tentativa (junta com runs.csv via RunID)
    └── TELEMETRY.md       # relatório legível gerado a partir dos dois CSVs (não edite)
```

Os logs trazem o prompt e a resposta completa de cada agente, a saída integral de testes/linters, os relatórios de revisão/segurança/decisão, durações, contagem de tokens e o código de saída final.

O `TELEMETRY.md` é regenerado ao final de cada execução a partir de `runs.csv` e `events.csv`. Ele começa pelos KPIs (total de execuções, taxa de sucesso, tokens acumulados no geral e por etapa E1 a E5, duração média) e segue com o histórico das execuções, da mais recente para a mais antiga, em uma tabela com o cabeçalho `| Data | RunID | Status | Demanda | Stack | Tentativas | Duração | Tokens Totais | Tokens E1 PLAN | ... | Tokens E5 GATE |`. Se o relatório não puder ser gerado, apenas um aviso é exibido e a execução não é interrompida.

## Códigos de saída

As Etapas 1 e 2 rodam como um loop autônomo (veja [Loop autônomo de especificação](#loop-autônomo-de-especificação)). O código `2` só ocorre quando o CRITIC ainda responde `SPEC_STATUS: REVISION_REQUIRED` (ou não grava o parecer) após `MC_MAX_ATTEMPTS` rodadas; corrija o plano e recomece pela Etapa 1.

| Código | Significado |
| :--- | :--- |
| `0` | `APPROVE_MERGE` (ou `-init`/`-setup` bem-sucedido) |
| `1` | Erro fatal ou circuit breaker acionado |
| `2` | Especificação bloqueada pelo CRITIC (Etapa 2) após esgotar `MC_MAX_ATTEMPTS` |
| `3` | `AMEND_TEST` — teste da Etapa 1 precisa de correção |
| `4` | `ESCALATE_PLANNER` |
| `5` | Limite de cota ou tokens atingido (estado gravado em `RESUME.env`, retomável com `-resume`) |
| `6` | Execução abortada pelo usuário com `[q]` no modo `-manual` (`ABORTED`) |

## Testes

```bash
go test -v -race ./... && go vet ./... && go build -o makercycle .
```

## Licença

Distribuído sob a **Apache License 2.0**. Veja `LICENSE`.
