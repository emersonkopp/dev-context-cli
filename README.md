# dctx — dev-context CLI

CLI para gerenciar o monorepo [dev-context](https://github.com/emersonkopp/dev-context) — instala, configura, sincroniza e mantém atualizado o conjunto de artefatos de IA (steerings, settings, prompts, agentes) em qualquer máquina.

## Instalação

**Uma linha faz tudo:** instala o binário, clona o monorepo e configura os artefatos.

```bash
curl -fsSL https://raw.githubusercontent.com/emersonkopp/dev-context-cli/main/install.sh | bash
```

O script detecta automaticamente o OS (Linux/macOS) e a arquitetura (amd64/arm64). Não requer interação em condições normais — a única pergunta possível é sobre o caminho de clone, caso `~/git/dev-context` já exista com outro conteúdo.

### Variáveis de ambiente opcionais

| Variável | Padrão | Descrição |
|---|---|---|
| `INSTALL_DIR` | `/usr/local/bin` | Onde instalar o binário `dctx` |
| `REPO_PATH` | `~/git/dev-context` | Onde clonar o monorepo |
| `DCTX_VERSION` | *(latest)* | Versão específica a instalar |
| `SKIP_BOOTSTRAP` | `0` | `1` para instalar só o binário sem configurar |

```bash
# Exemplos
INSTALL_DIR=~/.local/bin curl -fsSL .../install.sh | bash
REPO_PATH=~/work/dev-context curl -fsSL .../install.sh | bash
SKIP_BOOTSTRAP=1 curl -fsSL .../install.sh | bash   # só o binário
```

### Instalar manualmente o binário

Se quiser só o binário sem executar o bootstrap, baixe diretamente de
https://github.com/emersonkopp/dev-context-cli/releases e execute `dctx bootstrap` depois.

## Bootstrap manual (re-setup ou nova máquina sem curl)

```bash
dctx bootstrap
```

Faz a mesma coisa que o `install.sh` pós-instalação: clona o monorepo (se ausente), salva a config e instala os artefatos. Aceita as mesmas flags:

```bash
dctx bootstrap --repo-path ~/outro/caminho   # clone em caminho diferente
dctx bootstrap --yes                          # sem prompts, aceita defaults
```

## Comandos

### `dctx config init`

Inicializa o arquivo `~/.dev-context/config.json` de forma interativa, pedindo o caminho local do monorepo.

```bash
dctx config init
```

### `dctx config show`

Exibe a configuração atual.

```bash
dctx config show
```

### `dctx install`

Instala e configura todos os artefatos do monorepo nas localizações globais de cada ferramenta.

```bash
dctx install
```

**O que é instalado (v1):**

| Artefato | Destino |
|---|---|
| `kiro/steering/*.md` | `~/.kiro/steering/` |
| `kiro/settings/cli.json` | `~/.kiro/settings/cli.json` *(merge — não sobrescreve chaves existentes)* |

É seguro rodar múltiplas vezes. Arquivos já atualizados são ignorados.

### `dctx status`

Exibe o estado de instalação de cada artefato — se está instalado e se está desatualizado em relação ao monorepo.

```bash
dctx status
```

Saída de exemplo:
```
▸ Kiro
  ✓  up-to-date     kiro/steering/non-functional-requirements.md
  ✓  up-to-date     kiro/steering/testing-policy.md
  ⚠  outdated       kiro/steering/auto-update-policy.md
  ✓  up-to-date     kiro/settings/cli.json
```

### `dctx sync`

Sincroniza o monorepo local com o remoto (GitHub). Faz `git pull` ou `git push` conforme necessário.

```bash
dctx sync
```

**Comportamento:**
- Uncommitted changes locais são commitados automaticamente antes do sync.
- Se apenas o remoto tem mudanças: fast-forward.
- Se apenas o local tem mudanças: push direto.
- Se ambos têm mudanças (divergência): exibe menu interativo:
  - **merge** — rebase local no topo do remoto *(recomendado)*
  - **prefer-remote** — descarta commits locais, reseta para o remoto
  - **prefer-local** — force-push do estado local para o remoto
  - **abort** — não faz nada

### `dctx update`

Atualiza o próprio binário `dctx` para a versão mais recente disponível no GitHub Releases.

```bash
dctx update
```

A atualização substitui o binário em-place, sem necessidade de reinstalar.

## Workflow típico entre máquinas

```bash
# Máquina A — faz alterações nos steerings
vim ~/git/dev-context/kiro/steering/testing-policy.md
dctx sync        # commit + push para o GitHub

# Máquina B — recebe as mudanças
dctx sync        # pull do GitHub
dctx install     # aplica os artefatos atualizados
```

## Como fazer um release

1. Crie e empurre uma tag semver:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
2. O GitHub Actions workflow (`.github/workflows/release.yml`) executa o GoReleaser automaticamente, gerando binários para:
   - `linux/amd64`
   - `linux/arm64`
   - `macos/amd64`
   - `macos/arm64`
3. Os assets são publicados em https://github.com/emersonkopp/dev-context-cli/releases.

## Estrutura do projeto

```
dev-context-cli/
├── main.go
├── cmd/
│   ├── root.go       # cobra root + wiring
│   ├── install.go    # dctx install
│   ├── sync.go       # dctx sync
│   ├── status.go     # dctx status
│   ├── update.go     # dctx update (self-update)
│   └── config.go     # dctx config init/show
├── internal/
│   ├── config/       # ~/.dev-context/config.json
│   ├── provider/     # interface Provider
│   ├── git/          # operações git
│   └── update/       # self-update via GitHub Releases
├── providers/
│   └── kiro/         # implementação do Provider para o Kiro
├── .goreleaser.yaml
├── .github/
│   └── workflows/
│       └── release.yml
└── install.sh
```

## Adicionando suporte a uma nova ferramenta

1. Crie `providers/<nome>/` implementando a interface `provider.Provider`.
2. Registre o novo provider em `cmd/install.go` e `cmd/status.go`.
3. Adicione os artefatos no monorepo `dev-context/`.

## Requisitos

- Git instalado e disponível no `$PATH`
- macOS ou Linux (amd64 ou arm64)
- `curl` ou `wget` para o script de instalação
