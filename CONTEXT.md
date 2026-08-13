# Contexto de sesión — aiworker-cli

Última actualización: 2026-08-13. Este archivo es para retomar el proyecto
en una sesión nueva sin releer todo el chat. `README.md` documenta el
producto; esto documenta el *por qué* y qué falta.

**El proyecto local (`C:\Users\drodriguez\Desktop\aiworker-cli`) y el repo de
GitHub `github.com/dralbor/aiworker-cli` son, a propósito, el mismo
contenido en dos checkouts distintos**: el código Go vive en la raíz del
repo, y el mismo repo tiene una carpeta `/skills` que es el contenido
compartido que la herramienta sincroniza. El checkout de trabajo (donde se
edita código, este directorio) y el checkout que usa el mecanismo de sync
(`~/.aiworker-cli/skills-repo`, ver decisión #10) son dos clones
independientes del mismo remoto - normal, no un error.

## Qué es

CLI de onboarding para developers de la empresa: instala servidores MCP
(hoy: OmniSQL, Atlassian, Figma) y gestiona skills de Claude compartidas
entre el equipo, todo desde una TUI (Go + Bubble Tea + Lip Gloss), mismo
stack que `vulcan-cli` (repo hermano en `~/Documents/repos/vulcan-cli`, que
sirvió de referencia de estilo/arquitectura).

Está en `C:\Users\drodriguez\Desktop\aiworker-cli`, y **ahora sí es un repo
git**, con remoto en `github.com/dralbor/aiworker-cli` (`main`, pusheado).
Arrancó sin git a propósito (MVP rápido); se inicializó y pusheó recién
cuando el usuario lo pidió explícitamente, no antes.

## Decisiones clave (y por qué)

1. **Claude Code ≠ Claude Desktop.** Son productos distintos con config
   distinta. Claude Code (la terminal) resuelve MCP desde su propio estado
   (`~/.claude.json`), Claude Desktop desde `claude_desktop_config.json`
   (app Electron separada). La v1 de este proyecto instalaba mal — escribía
   en el archivo de Desktop pensando que Claude Code también lo leía. Se
   corrigió: para Claude Code, `aiworker-cli` **nunca** edita
   `~/.claude.json` a mano (es grande, tiene mucho más que MCP config, su
   schema no es un contrato estable) — todo pasa por el CLI oficial `claude
   mcp add/remove/list` (`internal/claudecode`), igual que si el usuario lo
   tipeara.

2. **Sin scope `project` (`.mcp.json` compartido por git).** Pedido
   explícito: "nadie lo usará y no debemos compartir mcp.json entre todos".
   Solo dos destinos: Claude Code global (`--scope user`) y Claude Desktop
   (archivo, local a la máquina). Ver `internal/mcpclient/targets.go`.

3. **Sin versión pineada en el catálogo** (`omnisql`/`atlassian` corren
   `npx`/`uvx` sin `@version`, siempre bajan la última). Fue una decisión
   consciente del usuario después de que yo propusiera pinear +
   `doctor`/outdated-check como alternativa más segura (dado el historial de
   CVEs de `mcp-atlassian`, ver abajo) — el usuario eligió "siempre última"
   explícitamente. Documentado en `catalog.go`/README como decisión, no
   descuido.

4. **Secretos nunca en texto plano en ningún config file.** Los env vars
   marcados `Secret` van al llavero nativo del SO (`internal/secretstore`,
   `zalando/go-keyring`) y el server se registra apuntando al propio binario
   (`aiworker mcp-run <id>`), que en tiempo de ejecución lee el secreto del
   llavero, resuelve el comando real del catálogo y recién ahí exec'ea el
   server real. Probado de punta a punta contra el `claude` real de la
   máquina (no un mock).

5. **"Instalado" se decide contra la máquina real (`claude mcp list`), no
   solo contra nuestro propio `state.json`.** Gap encontrado por el usuario:
   `omnisql` lo tenía instalado a mano (antes de que existiera este
   proyecto) y no lo marcaba. Ahora `mcpinstall.GetStatus()` cruza
   `state.json` (lo que instalamos nosotros, con detalle de secretos/target)
   con `claude mcp list` (la verdad real) en cada entrada a la pantalla, sin
   caché. Si `state.json` dice instalado pero `claude mcp list` ya no lo
   tiene, se poda el registro viejo solo (self-heal).

   **Importante, ajuste del usuario sobre esto**: NO mostrar texto tipo
   "(instalado a mano)" — es raro en una herramienta que van a usar varios
   developers, suena como si le hablara a una sola persona específica. La
   regla final: **punto verde = instalado (se detectó, no importa cómo),
   punto gris = no instalado.** El paréntesis con el nombre del destino
   (ej. "(Claude Code)") solo se muestra cuando *sabemos* el detalle
   (`ManagedByUs == true`); si no lo sabemos, no se inventa texto, el punto
   ya dice todo. Ver `viewMCPRow` en `internal/tui/app/model.go`.

6. **Dos secciones en la pantalla de MCP**: "Disponibles — usados por el
   equipo" (el catálogo) y "Instalados manualmente por vos" (lo que `claude
   mcp list` reporta que no está en el catálogo — otros MCP que el usuario
   agregó, plugins de Claude Code, etc.), con opción de desinstalar también
   esos vía `claude mcp remove` genérico (`mcpinstall.RemoveExternal`).

7. **Figma es HTTP+OAuth, no stdio.** Catálogo ahora soporta
   `catalog.TransportHTTP` (URL en vez de Command/Args, sin Env). Se instala
   directo sin pantalla de target ni de env vars (no hace falta elegir nada).
   El login OAuth (`claude mcp login figma`) **no se automatiza** — abre
   navegador y necesita una terminal interactiva propia, no encaja limpio
   dentro del alt-screen de la TUI. Se le avisa al usuario que lo corra
   aparte.

8. **Regla de refresco**: cualquier pantalla con estado externo (Doctor,
   chequeo de prerequisitos, lista de MCP, y también Skills) relee todo
   fresco en cada entrada, nunca cachea entre visitas — motivado por un
   reporte del usuario de ver info vieja (aunque en Doctor específicamente
   resultó que ya refrescaba bien; el gap real estaba en el estado de MCP
   instalados). Sync de Skills con el repo compartido **arrancó con cooldown
   de 5 min y se sacó por pedido explícito** ("siempre conectado... 100%
   tiempo real, que pullee cada vez que entro a esta sección") — hoy
   `skillsmarket.Prepare` hace `fetch`+merge fast-forward sin throttling en
   cada entrada a la pantalla, en background (nunca bloquea el primer
   render, que sigue mostrando la copia local al instante).

   Aclarado con el usuario: esto es sincronización *pull-based* (se entera
   quien entra a la pantalla), no *push* (nadie recibe una notificación en
   vivo mientras tiene la pantalla abierta sin tocarla). Es la opción
   correcta para el alcance de esta herramienta - un servidor propio con
   websockets sería sobre-ingeniería para "que se vea lo que subieron los
   demás la próxima vez que abrís Skills".

9. **Auto-instalar prerequisitos faltantes con confirmación.** Si eligen
   instalar Atlassian y falta `uvx`, se ofrece instalar `uv` (script oficial
   de astral.sh) ahí mismo, y sigue directo a instalar el MCP. Solo `uv`
   tiene instalador automatizado hoy (`internal/prereq`, campo
   `Tool.Installer`) — Node/`npx` no, porque no hay un one-liner oficial
   único para todos los SO, sería adivinar. Maneja el caso de `PATH` no
   actualizado en el proceso actual (típico en Windows sin abrir terminal
   nueva) con rutas de fallback conocidas.

10. **Un solo repo para código + skills, en carpetas separadas** (decisión
    explícita del usuario, elegida sobre la alternativa de dos repos
    separados que yo había recomendado por defecto). `github.com/dralbor/aiworker-cli`
    (cuenta personal, "capaz a futuro se mueve a grupo-centaurus, por ahora
    no quería crear el repo en el equipo sin saber si lo van a usar") tiene
    el código Go en la raíz y una carpeta `skills/` con el contenido
    compartido. Como Claude Code solo debe ver skills en `~/.claude/skills`
    (nunca el código Go mezclado ahí), la sincronización usa un **link de
    directorio**, no un clone directo:

    - Clone real (código + `skills/`, completo) en `~/.aiworker-cli/skills-repo`.
    - `~/.claude/skills` es una **NTFS junction** en Windows (`mklink /J`,
      no hace falta admin - a diferencia de un symlink real, que sí lo pide)
      apuntando a `~/.aiworker-cli/skills-repo/skills`. En macOS/Linux es un
      symlink normal (`os.Symlink`, sin ese problema de permisos).
      `internal/skillsmarket.ensureLink`/`createDirLink` — usa
      `os.SameFile` para detectar "ya está bien enlazado" en vez de parsear
      el tipo de reparse point a mano (mucho más robusto entre versiones de
      Go/SO).
    - Probado en vivo, dos veces: (1) contra un repo bare local sintético
      con contenido "de código" simulado en la raíz, confirmando que
      escribir a través del link cae adentro de `skills-repo/skills` y que
      un clone nuevo (compañero simulado) ve el código en la raíz y las
      skills en `skills/` sin mezclarse; (2) contra el GitHub real, migrando
      esta máquina de la estructura vieja (clon completo directo en
      `~/.claude/skills`, de antes de esta decisión) a la nueva - confirmado
      con `fsutil reparsepoint query` que es una junction real.
    - Agregué `aiworker skills set-remote <url>` porque no existía forma de
      configurar el remoto después del prompt único inicial - hace falta
      para el día que se mueva a la org. `Prepare()` también **reconcilia
      el remoto git** si `set-remote` apunta a una URL distinta a la que el
      repo local ya tenía (`git remote set-url origin`) - probado en vivo,
      cambia de remoto sin re-clonar. Limitación conocida, no resuelta: si
      el nuevo remoto tiene historia no relacionada (ej. un repo de org
      creado de cero en vez de migrado), el `fetch`+`ff-only merge` va a
      fallar con error claro en vez de intentar algo raro — asumido que una
      migración real preserva historia (`git clone --mirror` + push), no un
      repo nuevo vacío.
    - El primer push del código a este repo tuvo que reconciliar con 2
      commits previos de verificación (de cuando este mismo repo se probó
      *solo* como store de skills, antes de esta decisión) vía `git merge
      --allow-unrelated-histories` — nunca se hizo `push --force` ni se
      reescribió nada; la historia vieja de "verificacion" sigue ahí,
      inofensiva.

11. **La pantalla de Skills no tenía forma de borrar nada** hasta que el
    usuario lo pidió (había creado categorías de prueba - `backend`,
    `frontend`, `helloworld`, ya publicadas de verdad en el repo - y no
    podía sacarlas). Se agregó navegación con cursor sobre categorías+skills
    (`skillRows()`/`skillsCursor`, mismo patrón que `mcpRows()` en MCP) y
    `enter` para confirmar borrado. Una categoría **solo se puede borrar
    vacía** (si tiene skills adentro, mensaje pidiendo vaciarla primero) -
    evita un borrado masivo por accidente. El borrado se publica al repo
    compartido igual que la creación (mismo `publishSkillsChange` en
    background). Dato importante encontrado con el propio test: borrar una
    skill dentro de una categoría **no** borra la categoría (queda vacía,
    visible, borrable aparte) - es el comportamiento correcto, no un bug (el
    test original asumía lo contrario y estaba mal).

    Al pushear este fix hubo que reconciliar (fetch + merge normal, sin
    forzar) porque `~/.aiworker-cli/skills-repo` (el clone que usa el
    mecanismo de sync) ya había pusheado los commits de esas 3 categorías
    directo al remoto - dos clones independientes del mismo repo,
    comportamiento esperado, no un error.

## Estructura del código

```
internal/
  catalog/        catálogo embebido (MCPServer: stdio o http)
  claudecode/      wrapper de `claude mcp add/remove/list` (parsea texto, sin JSON disponible)
  mcpclient/       targets (Claude Code / Claude Desktop) + merge JSON para Desktop
  mcpinstall/      orquesta catalog+claudecode+mcpclient+secretstore+state; GetStatus/External/RemoveExternal
  secretstore/     llavero del SO (zalando/go-keyring)
  state/           ~/.aiworker-cli/state.json (sidecar: qué instalamos nosotros)
  prereq/          chequeo + auto-instalación de runtimes (npx, uvx)
  doctor/          chequeo de entorno general (git, claude, + lo que prereq requiera)
  skills/          filesystem puro: crear/listar skills y categorías
  skillsmarket/    skills + git compartido (bootstrap con link de directorio, reconcile de remoto, sync sin cooldown, publish en background)
  gitrepo/         wrapper genérico de git (nunca fuerza push, nunca pisa historia)
  tui/app/         toda la TUI (un solo Model grande, Bubble Tea)
  styles/          paleta y helpers de Lip Gloss
main.go             entry point + `mcp-run` (proxy que inyecta secretos del llavero)
exec.go             passthrough de stdio al exec'ear el server real
```

## Estado actual: funciona, probado en vivo

Todo lo de arriba se probó contra el `claude`/`git` reales de esta máquina
(no mocks) con entradas descartables, limpiadas después — incluyendo push y
pull reales contra GitHub (`dralbor/aiworker-cli`), no solo un bare repo
local sintético. Build limpio, `go vet` limpio, tests unitarios (`go test
./...`) pasando. Última build: `aiworker.exe` en la raíz del proyecto.

Estado real de esta máquina ahora mismo:
- `~/.claude/skills` → junction → `~/.aiworker-cli/skills-repo/skills`,
  vacío de skills reales (listo para que alguien cree la primera).
- `~/.aiworker-cli/skills-repo` es un clone completo de
  `dralbor/aiworker-cli` (código + skills/), conectado y sincronizado.
- El proyecto en `Desktop\aiworker-cli` es un repo git aparte (mismo
  remoto), rama `main`, working tree limpio, pusheado.
- `~/.aiworker-cli/state.json` tiene `atlassian` instalado en Claude Desktop
  (lo hizo el usuario probando el binario entre sesiones, con secretos
  reales en el llavero de Windows - no tocar/asumir que es descartable).

## Pendiente / próximos pasos posibles

- **Catálogo servido remoto** en vez de hardcodeado en Go — para sumar
  entradas sin release del binario. No implementado, mencionado como "qué
  falta para producción" en el README.
- **`sync`/`status --outdated`** comparando versión instalada vs. catálogo
  (relevante sobre todo si en algún momento se pinea alguna versión).
- **Perfiles por rol** (`--profile backend`) que instalen un set curado.
- **Auto-actualización del propio binario aiworker-cli.**
- **Política push directo vs. PR** para publicar skills — hoy es push
  directo a la rama por defecto (pedido explícito), rama+PR quedó anotado
  como alternativa si el equipo cambia de opinión.
- **CI de release** igual a `vulcan-cli` (`.github/workflows/release.yaml`) —
  no existe todavía. El repo ya existe y tiene el código, pero no hay
  workflow de build/release configurado (el usuario sube el `.exe` a mano a
  Releases, como hace con vulcan-cli).
- Si se agregan más entradas HTTP/OAuth al catálogo, revisar si vale la pena
  soportarlas también en Claude Desktop (hoy `Apply` rechaza esa combinación
  explícitamente en vez de adivinar el schema).

## Cómo seguir

```
cd C:\Users\drodriguez\Desktop\aiworker-cli
git status                                                 # ahora SI es un repo, chequear antes de tocar nada
"C:\Program Files\Go\bin\go.exe" build -o aiworker.exe .   # o agregar Go al PATH de la sesión
"C:\Program Files\Go\bin\go.exe" test ./...
.\aiworker.exe            # TUI completa
.\aiworker.exe doctor     # chequeo no interactivo
```

Cambios de código nuevos van como commits normales sobre `main` (remoto
`origin` ya configurado) - no hay rama de release ni CI todavía, así que un
`git push` a `main` es directo. Recordar la regla del usuario: nunca
"Co-Authored-By" en los mensajes de commit.

Go se instaló en esta sesión (`C:\Program Files\Go`) porque no estaba en la
máquina; si una sesión nueva no lo encuentra en PATH, es la misma causa que
la vez pasada (terminal vieja, no un problema real) — usar la ruta completa
o abrir una terminal nueva.
