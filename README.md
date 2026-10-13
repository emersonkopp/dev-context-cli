# dctx — dev-context CLI

CLI para gerenciar o monorepo [dev-context](https://github.com/emersonkopp/dev-context) — instala, configura, sincroniza e mantém atualizado o conjunto de artefatos de IA (steerings, settings, prompts, agentes) em qualquer máquina.

## Instalação

**Uma linha faz tudo:** instala o binário, clona o monorepo e configura os artefatos.

```bash
curl -fsSL https://raw.githubusercontent.com/emersonkopp/dev-context-cli/main/install.sh | bash
```

O script detecta automaticamente o OS (Linux/macOS) e a arquitetura (amd64/arm64). Não requer interação em condições normais — a única pergunta possível é sobre o caminho de clone, caso `~/git/dev-context` já exista com outro conteúdo.

> **Sem sudo.** Por padrão o binário é instalado em `~/.local/bin` (diretório do
> usuário). Assim a instalação, o uso e o `dctx update` nunca pedem `sudo`.
> Instalar em um local do sistema (ex.: `/usr/local/bin`) com `sudo` deixa o
> binário com dono `root` e faz o `dctx update` falhar por permissão — evite.
> Garanta que `~/.local/bin` esteja no seu `$PATH` (o instalador avisa se não estiver).

### Variáveis de ambiente opcionais

| Variável | Padrão | Descrição |
|---|---|---|
| `INSTALL_DIR` | `~/.local/bin` | Onde instalar o binário `dctx` (local do usuário, sem sudo) |
| `REPO_PATH` | `~/git/dev-context` | Onde clonar o monorepo |
| `DCTX_VERSION` | *(latest)* | Versão específica a instalar |
| `SKIP_BOOTSTRAP` | `0` | `1` para instalar só o binário sem configurar |

```bash
# Exemplos
INSTALL_DIR=~/bin curl -fsSL .../install.sh | bash
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
| `kiro/skills/<nome>/SKILL.md` | `~/.kiro/skills/<nome>/SKILL.md` |
| `kiro/settings/cli.json` | `~/.kiro/settings/cli.json` *(merge — não sobrescreve chaves existentes)* |

É seguro rodar múltiplas vezes. Arquivos já atualizados são ignorados.

**Remoção de órfãos:** artefatos que o `dctx` instalou antes e que foram **renomeados ou
removidos** no monorepo são apagados do destino automaticamente na instalação seguinte (o `dctx`
mantém um manifesto em `~/.dev-context/kiro-manifest.json`). Arquivos que o `dctx` **nunca**
instalou — como skills específicas de organização/cliente mantidas apenas localmente — nunca são
removidos.

> **Migração (uma vez):** artefatos instalados por versões do `dctx` anteriores ao manifesto não
> constam nele e, portanto, não são removidos automaticamente na primeira execução. Se você
> renomeou/removeu artefatos nessa transição, apague os nomes antigos manualmente uma única vez.

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

A atualização substitui o binário em-place, sem necessidade de reinstalar. Antes de
aplicar, o arquivo baixado é verificado contra o checksum SHA256 publicado no release
(`dctx_<versão>_checksums.txt`): se o download estiver corrompido ou truncado, a
atualização é abortada e o binário atual permanece intacto. A troca do binário é
atômica (gravação em arquivo temporário + rename), então uma atualização interrompida
nunca deixa um executável pela metade.

> Instale sempre em um diretório do usuário (padrão `~/.local/bin`) para que o
> `dctx update` funcione sem `sudo`. Veja a nota em **Instalação**.

### `dctx uninstall`

Remove os artefatos que o `dctx` instalou nas localizações globais de cada ferramenta.

```bash
dctx uninstall
```

É conservador por padrão:

- Remove **apenas** os arquivos que o `dctx` instalou (registrados no manifesto). Skills
  locais da organização e qualquer arquivo que o `dctx` nunca instalou são preservados.
- Arquivos com merge que podem conter chaves suas (ex.: `~/.kiro/settings/cli.json`) **não**
  são apagados.
- O monorepo clonado nunca é tocado.
- O estado em `~/.dev-context` (config + manifesto) é **preservado**, de modo que uma
  reinstalação posterior retoma exatamente de onde parou.

**Flags:**

| Flag | Efeito |
|---|---|
| `--purge` | Também apaga `~/.dev-context` (config + manifesto) — estado zerado |
| `--binary` | Também apaga o próprio binário `dctx` |
| `-y`, `--yes` | Não pede confirmação |

```bash
dctx uninstall --purge          # remove artefatos e zera o estado
dctx uninstall --binary --yes   # remove artefatos e o binário, sem prompt
```

**Recuperação de estado:** se você remover o `dctx` sem `--purge` (ou só apagar o binário),
o estado em `~/.dev-context` continua lá; reinstalar e rodar `dctx install` retoma do ponto
anterior. Se você zerar tudo com `--purge` (ou remover `~/.dev-context` manualmente), a
reinstalação via `install.sh`/`dctx bootstrap` reaproveita o monorepo já clonado (ou re-clona
no caminho padrão), recria a config e reconstrói o manifesto — o estado é recuperado.

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
│   ├── uninstall.go  # dctx uninstall
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
