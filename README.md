# aiworker-cli (MVP)

CLI de onboarding para developers: instala servidores MCP y arma skills de
Claude localmente, sin que nadie tenga que copiar/pegar JSON a mano ni tokens
por Slack. Mismo stack que `vulcan-cli`: Go + [Bubble Tea](https://github.com/charmbracelet/bubbletea) +
[Lip Gloss](https://github.com/charmbracelet/lipgloss), un solo binario.

Código en [github.com/dralbor/aiworker-cli](https://github.com/dralbor/aiworker-cli)
(repo personal por ahora, ver `CONTEXT.md`). Mismo repo que usa la propia
herramienta como "marketplace" de skills compartidas — código en la raíz,
contenido de skills en `/skills` (más abajo).

## Uso

```
aiworker              Abre el menu interactivo (Servidores MCP / Skills / Doctor)
aiworker mcp          Va directo a Servidores MCP
aiworker skills       Va directo a Skills
aiworker doctor       Chequea el entorno (git, uvx, claude, omnisql-mcp en PATH)
aiworker version
aiworker help
```

Navegación: `↑/↓` mover, `Enter` elegir/confirmar, `Esc` volver, `q`/`Ctrl+C` salir.

## Qué hace

### Servidores MCP

Catálogo embebido (`internal/catalog`) con tres entradas:

- `omnisql` — [srthkdev/omnisql-mcp](https://github.com/srthkdev/omnisql-mcp),
  vía `npx -y omnisql-mcp`.
- `atlassian` — [sooperset/mcp-atlassian](https://github.com/sooperset/mcp-atlassian),
  vía `uvx mcp-atlassian`.
- `figma` — servidor oficial de Figma (`https://mcp.figma.com/mcp`), HTTP +
  OAuth. Pide iniciar sesión con tu cuenta de Figma (SSO) la primera vez: al
  instalarlo, `aiworker-cli` avisa que corras `claude mcp login figma` aparte
  — el login abre el navegador y necesita una terminal interactiva propia, no
  algo que la TUI pueda automatizar limpio desde adentro de su alt-screen.

Las descripciones que ves en la pantalla son deliberadamente cortas (qué hace
el servidor, una línea). El detalle de versión/seguridad de cada uno vive acá
en el README y en el historial de commits de `catalog.go`, no compite por
espacio en la lista.

**Sin versión pineada a propósito** en `omnisql`/`atlassian`: ambos comandos
bajan siempre la última versión publicada (npm / PyPI) en cada arranque — es
una decisión consciente, no la única opción. La alternativa (pinear una
versión concreta y que `doctor` avise cuando salga una más nueva) es más
segura contra una release con regresión o comprometida, a costa de necesitar
que alguien la bumpee a mano de tanto en tanto. `mcp-atlassian` en particular
tuvo una tanda grande de CVEs high/critical en jul/2026 (auth bypass en HTTP,
arbitrary file read vía `upload_attachment`) parcheados en 0.22.0+ — correr
siempre "latest" asume que el proyecto sigue reaccionando rápido a lo que
encuentren, no que hoy esté libre de bugs nuevos.

Antes de entrar a la pantalla de Servidores MCP, `aiworker-cli` corre un
chequeo rápido de los runtimes que el catálogo necesita (`npx`, `uvx`) —
pantalla de "Comprobando..." con spinner. Mismo chequeo que expone
`aiworker doctor`.

Si además elegís instalar un servidor puntual (ej. Atlassian) y le falta su
runtime (`uvx`), `aiworker-cli` te lo dice ahí mismo y te ofrece instalarlo:
confirmás (`y`), corre el instalador oficial (`astral.sh/uv/install.ps1` /
`.sh` — hoy solo `uv` tiene instalador automático; `npx`/Node no, porque no
hay un one-liner oficial único para todos los SO) y sigue directo a instalar
el MCP que pediste, sin que tengas que abrir una terminal aparte. Como el
`PATH` del proceso actual puede no enterarse todavía de un binario recién
instalado (típico en Windows sin abrir una terminal nueva), si hace falta usa
la ruta absoluta resuelta - seguro en los dos destinos porque ninguno es
compartido (ver abajo).

### Destinos: Claude Code vs. Claude Desktop

Son productos distintos con configuración distinta - un error fácil de
cometer (lo cometí yo en la primera versión):

- **Claude Code** (la terminal, este mismo programa) *no* lee
  `claude_desktop_config.json`. Resuelve MCP servers desde su propio estado
  interno (`~/.claude.json`). En vez de hand-editear ese archivo nosotros -
  es grande, tiene mucho más que MCP config, y su schema exacto no es un
  contrato estable - `aiworker-cli` corre el comando oficial `claude mcp add`
  / `claude mcp remove` (`internal/claudecode`), exactamente el mismo que
  tipearías vos a mano: `claude mcp add --scope user`, global a todas las
  carpetas de esta máquina. Es lo que ya usabas para lo que te andaba.
  **No hay opción de scope `project` (`.mcp.json` compartido por git)** - a
  propósito, el equipo no quiere MCP config versionado ni compartido entre
  todos.
- **Claude Desktop** es la app de escritorio separada (Electron, no
  terminal). Lee `claude_desktop_config.json`, un archivo propio que sí
  mergeamos directo nosotros (no tiene CLI propia para delegarle esto). Sigue
  siendo local a esta máquina, igual que Claude Code.

Al instalar un servidor:

1. Elegís destino: uno de los dos de arriba (los dos son globales a esta
   máquina, ninguno se comparte).
2. Completás sus variables de entorno una por una. Las marcadas 🔒 son
   secretas.
3. Las variables **no secretas** (URLs, usernames) quedan visibles en el
   config de destino. Las **secretas** (API tokens) se guardan en el llavero
   nativo del sistema operativo (Windows Credential Manager / macOS Keychain /
   Secret Service en Linux) — **nunca en texto plano en el archivo de
   config**. La entrada que queda en el JSON apunta al propio `aiworker-cli`
   (`mcp-run <id>`), que en tiempo de ejecución lee el secreto del llavero,
   resuelve el comando real desde el catálogo y recién ahí lanza el servidor.
4. Cada instalación (y sus claves de llavero) queda registrada en
   `~/.aiworker-cli/state.json`, así "quitar" borra exactamente lo que
   `aiworker-cli` agregó — nunca toca entradas de MCP que hayas puesto a mano.
5. Antes de escribir, se guarda un `.bak` del config anterior. Si el archivo
   existente no es JSON válido, `aiworker-cli` no lo toca y avisa el error.

**Regla de la app**: cualquier pantalla que muestre estado externo (instalado
sí/no, binarios en PATH) lo vuelve a leer fresco cada vez que entrás a esa
pantalla, nunca reusa lo que tenía en memoria de una visita anterior o del
arranque — Doctor, el chequeo de prerequisitos, y la lista de Servidores MCP
(`enterMCPList` en `internal/tui/app`) siguen todos esa regla. El estado
puede cambiar en un segundo (otra terminal corriendo `claude mcp add/remove`,
por ejemplo) y mostrar algo viejo es peor que tardar los milisegundos que
tarda un `exec.LookPath` o una lectura de `state.json`. Skills sigue la misma
regla, sin excepción: cada entrada a esa pantalla también dispara un
`fetch`+merge fresco del repo compartido (ver abajo) - la única diferencia es
que ahí el costo es una llamada de red, así que corre en background en vez
de bloquear el primer render (la copia local se ve al instante igual).

**"Instalado" se decide contra la máquina real, no solo contra nuestro
propio archivo.** `~/.aiworker-cli/state.json` guarda lo que *aiworker-cli*
instaló (para saber qué secretos/target le corresponden a cada uno y poder
desinstalarlo con precisión), pero la pantalla de Servidores MCP cruza eso
con `claude mcp list` en cada entrada (`mcpinstall.GetStatus`, sin caché) y
lo separa en dos secciones:

- **Disponibles — usados por el equipo**: el catálogo de arriba. El punto es
  el único indicador: verde = detectado instalado (no importa cómo - por
  vos a mano, por `aiworker-cli`, por lo que sea), gris = no instalado. Sin
  texto tipo "(instalado a mano)" - varias personas van a usar esto, no
  tiene sentido hablarle a "vos" específicamente. El paréntesis con el
  destino (ej. "(Claude Code)") solo aparece cuando sabemos el detalle
  concreto porque lo instaló `aiworker-cli`.
- **Instalados manualmente por vos**: todo lo que `claude mcp list` reporta
  que *no* es parte de nuestro catálogo (otro MCP que agregaste vos, un
  plugin de Claude Code, etc.) — se lista y se puede quitar desde acá con
  `enter` (usa `claude mcp remove` directo, sin tocar `state.json` porque no
  hay secretos nuestros de por medio).

Si un registro de `state.json` dice "instalado en Claude Code" pero `claude
mcp list` ya no lo tiene (alguien lo sacó con `claude mcp remove` directo,
por fuera de `aiworker-cli`), `GetStatus` lo nota y poda el registro viejo
solo — nunca muestra algo como instalado si la fuente de verdad real dice lo
contrario. Esto no aplica a Claude Desktop: no tiene un `list` que consultar
(es una app de escritorio, no una CLI), así que para ese destino
`state.json` sigue siendo la única fuente.

No hay forma de saber `--output-format json` para `claude mcp list`/`get`
(no existe hoy en el CLI), así que `internal/claudecode` parsea la salida de
texto tal como la ves vos - es un parser chico y con test (`claudecode_test.go`)
pero best-effort por naturaleza: si Anthropic cambia el formato de esa salida,
esto se rompe silenciosamente hasta que alguien lo note y ajuste el parser.

### Skills — marketplace de equipo

`n` crea una skill (`~/.claude/skills/<categoria>/<nombre>/SKILL.md`, plantilla
mínima con frontmatter `name`/`description` + TODO). `f` crea una carpeta de
categoría vacía (con un `.gitkeep` para que exista igual sin ningún skill
adentro todavía). La pantalla lista todo como árbol por categoría, igual a
como Claude Code las descubre.

La primera vez que entrás a Skills te pregunta, una sola vez, la URL de un
repo git compartido para el equipo (dejarlo vacío = seguir 100% local). Para
configurarlo o cambiarlo después - no vuelve a preguntar solo -, `aiworker
skills set-remote <url>`. Hoy conectado a
[dralbor/aiworker-cli](https://github.com/dralbor/aiworker-cli) — **el mismo
repo que este código fuente** (cuenta personal, placeholder hasta decidir si
se mueve a la org - cambiar el remoto el día que se mueva es un solo
comando, no hace falta re-clonar nada a mano).

**El código y las skills conviven en el mismo repo, en carpetas distintas**
(`/` para el código, `/skills` para el contenido compartido) - decisión
explícita para no mantener dos repos separados por ahora. Eso significa que
`~/.claude/skills` **no puede ser directamente** el working copy del repo
completo (Claude Code solo debe ver skills ahí, no el código Go). La solución:

- El clone real vive en `~/.aiworker-cli/skills-repo` (repo completo: código
  + `skills/`). `~/.claude/skills` es un **link de directorio** a
  `~/.aiworker-cli/skills-repo/skills` - junction (`mklink /J`) en Windows,
  symlink en macOS/Linux. Nada se duplica: escribir en `~/.claude/skills/...`
  escribe literalmente adentro del repo, de forma transparente.
- Primer uso: si `~/.aiworker-cli/skills-repo` no existe, se clona el repo
  entero; si `~/.claude/skills` está vacío o no existe, se crea el link. Si
  `~/.claude/skills` ya tenía contenido real que no es del repo (no es el
  link), `aiworker-cli` **no adivina cómo mergear** — devuelve un error
  explicando qué mover a mano, para no arriesgar pisar skills que ya tenías.
- Probado en esta máquina: crear una skill a través de `~/.claude/skills`
  aparece de verdad adentro de `~/.aiworker-cli/skills-repo/skills`, se
  commitea+pushea desde ahí, y un clone nuevo del repo (simulando un
  compañero) trae el código Go en la raíz y la skill en `skills/` — sin
  mezclarse.
- Cada skill/carpeta que creás se escribe local **al instante** (la lista se
  actualiza ya, sin esperar nada) y dispara en background un
  `git add` + `commit` + `push` con tu identidad de git normal (la que ya
  tenés configurada - no hay usuario/bot propio). Un indicador chico
  ("Subiendo cambios...") avisa mientras tanto; si el push falla (sin red, sin
  permisos) el commit local queda igual, nada se pierde, y te avisa que quedó
  solo local.
- **Cada vez** que entrás a la pantalla hace un `fetch` + merge fast-forward
  en background para traer lo que subieron tus compañeros — sin cooldown, a
  propósito: el equipo lo quiere siempre conectado, lo más cerca de
  tiempo real que da un modelo pull-based sin construir un servidor propio
  (no hay push/websocket - la sincronización pasa cuando alguien abre la
  pantalla, no antes). Nunca bloquea: se ve la copia local al instante y se
  actualiza sola si hay novedades; si falla (sin conexión) simplemente se
  queda con lo último que había local.
- Nunca hace `push --force` ni pisa historia: si el remoto avanzó, hace un
  fetch + fast-forward + reintenta el push una vez antes de rendirse.

### Doctor

Chequea `git`, `claude`, y los runtimes que el catálogo necesita (`npx`,
`uvx`) — para detectar en el primer día lo que falta, en vez de un error
críptico dentro de Claude más adelante. La lista de runtimes se deriva del
catálogo (`internal/prereq`), no está hardcodeada dos veces.

## Qué falta para producción (no es parte de este MVP)

- Catálogo servido desde un repo/API interno en vez de hardcodeado en Go, para
  poder sumar/actualizar entradas sin un release del binario.
- `sync`/`status --outdated` comparando versión instalada vs. catálogo.
- Perfiles por rol (`--profile backend`) que instalen un set curado de una.
- Auto-actualización del propio binario.
- Las skills se publican con `push` directo a la rama por defecto del repo
  compartido (lo que se pidió). Si el equipo prefiere revisión antes de
  mergear, la alternativa es rama + PR (`gh pr create`) en vez de push
  directo — no implementado, es un cambio de política, no de mecanismo.
- CI de release igual a `vulcan-cli` (`.github/workflows/release.yaml`).

## Build

```
go build -o aiworker.exe .
go test ./...
```
