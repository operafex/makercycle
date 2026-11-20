# Etapa 0: SCAN — Agente SCOUT
# O MakerCycle (`makercycle`) é o ciclo completo; este arquivo descreve a ETAPA 0.
# Capacidade Ativa: Repo-Mapper (Claude Code Setup)

Você está analisando um **repositório já existente** para preparar o ambiente do `makercycle` antes que qualquer nova demanda seja planejada ou codificada.

## Regras Estritas de Isolamento
1. **Somente Leitura no Código:** Nunca altere código de produção ou testes existentes.
2. **Saída Restrita:** Crie ou atualize apenas `<DOCS_DIR>/PROJECT_CONTEXT.md` e `<DOCS_DIR>/demands/INDEX.md`. É permitido também atualizar somente as chaves `MC_STACK_NAME`, `MC_TEST_CMD`, `MC_TYPE_CMD` e `MC_LINT_CMD` em `.makercycle/config/project.env`, preservando as outras linhas (`MC_DOCS_DIR`, `MC_EXISTING_DOCS` etc.).

## Fluxo Obrigatório de Scan
1. **Inspeção da Documentação Existente:**
   - Leia os arquivos indicados pelo usuário no caminho de documentação informado (`<EXISTING_DOCS_PATH>`), além de `README.md`, arquivos de configuração e schemas de banco/API.
2. **Mapeamento Arquitetural do Código:**
   - Identifique estrutura de pastas, padrões de arquitetura, dependências principais, autenticação, banco de dados e suíte de testes atual.
3. **Geração do Contexto Base (`<DOCS_DIR>/PROJECT_CONTEXT.md`):**
   - Consolide em um guia objetivo para as próximas Etapas:
     - **Visão Geral & Stack:** Linguagem, frameworks, versão e comandos de build/teste.
     - **Mapa de Diretórios:** Onde ficam models, regras de negócio, controllers/handlers e testes.
     - **Convenções & Regras de Ouro:** Padrões obrigatórios do projeto e armadilhas a evitar.
     - **Referências de Documentação:** Onde está a documentação original do sistema.
4. **Inicialização do Índice (`<DOCS_DIR>/demands/INDEX.md`):**
   - Se ainda não existir, crie a tabela inicial para registrar as próximas demandas.
5. **Confirmação da Stack:**
   - Leia a stack pré-detectada pelo CLI (informada no prompt e em `.makercycle/config/project.env`).
   - Se estiver ausente ou incorreta (monorepo, Python, Java, Makefile…), grave os valores corretos para `MC_STACK_NAME`, `MC_TEST_CMD`, `MC_TYPE_CMD` e `MC_LINT_CMD` em `.makercycle/config/project.env` no formato `KEY="valor"` (sem aspas duplas internas nem quebras de linha). `MC_TYPE_CMD` e `MC_LINT_CMD` podem ficar `""`.
   - Registre a stack e os comandos na seção **Visão Geral & Stack** do `PROJECT_CONTEXT.md`.
6. **Handoff:** Encerre declarando: `MC_0_DONE — Contexto mapeado em <DOCS_DIR>/PROJECT_CONTEXT.md.`
