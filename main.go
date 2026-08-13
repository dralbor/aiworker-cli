package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"aiworker/cli/internal/catalog"
	"aiworker/cli/internal/doctor"
	"aiworker/cli/internal/secretstore"
	"aiworker/cli/internal/skills"
	"aiworker/cli/internal/skillsmarket"
	"aiworker/cli/internal/styles"
	"aiworker/cli/internal/tui/app"
)

var version = "dev" // set at build time via -ldflags -X main.version=...

func main() {
	if len(os.Args) < 2 {
		runTUI("")
		return
	}

	switch os.Args[1] {
	case "mcp":
		runTUI("mcp")
	case "skills":
		if len(os.Args) >= 3 && os.Args[2] == "set-remote" {
			runSkillsSetRemote(os.Args[3:])
			return
		}
		runTUI("skills")
	case "doctor":
		runDoctor()
	case "mcp-run":
		runMCPProxy(os.Args[2:])
	case "version", "--version":
		fmt.Println("aiworker-cli " + version)
	case "help", "--help":
		printHelp()
	default:
		fmt.Printf("Comando desconocido: %s\n", os.Args[1])
		printHelp()
		os.Exit(1)
	}
}

func runTUI(start string) {
	m := app.New(start)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

// runSkillsSetRemote configures (or reconfigures) the shared skills repo
// without going through the interactive one-time prompt - useful for the
// very first setup on a machine and for switching repos later (e.g. moving
// from a personal placeholder repo to the team's once it's decided).
func runSkillsSetRemote(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: aiworker skills set-remote <url-del-repo-git>")
		os.Exit(1)
	}
	remote := args[0]
	if err := skillsmarket.SetRemote(remote); err != nil {
		fmt.Fprintln(os.Stderr, "aiworker-cli: no pude guardar el remoto:", err)
		os.Exit(1)
	}
	fmt.Println(styles.Success.Render("✓ Remoto configurado: ") + remote)

	root, err := skills.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "aiworker-cli:", err)
		os.Exit(1)
	}
	fmt.Println(styles.Dim.Render("Conectando " + root + " ..."))
	if err := skillsmarket.Prepare(root); err != nil {
		fmt.Fprintln(os.Stderr, styles.Warn.Render("aviso: no se pudo sincronizar todavia: ")+err.Error())
		return
	}
	fmt.Println(styles.Success.Render("✓ Listo, ") + root + styles.Success.Render(" esta conectado al repo compartido."))
}

func runDoctor() {
	fmt.Println(styles.Title.Render("aiworker-cli doctor"))
	for _, c := range doctor.Run() {
		fmt.Printf("%s %s\n    %s\n", styles.Check(c.OK), styles.ItemName.Render(c.Label), styles.Dim.Render(c.Detail))
	}
}

// runMCPProxy is what the client configs actually invoke for MCP servers
// that need secrets: it fetches secret env vars from the OS keychain, sets
// them in its own process env, and execs straight into the real MCP server
// so nothing sensitive ever sat in a config file on disk.
func runMCPProxy(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "uso: aiworker mcp-run <catalog-id> [args...]")
		os.Exit(1)
	}
	id := args[0]
	mcp, ok := catalog.Find(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "aiworker-cli: %s no esta en el catalogo\n", id)
		os.Exit(1)
	}

	env := os.Environ()
	for _, ev := range mcp.Env {
		if !ev.Secret {
			continue
		}
		val, err := secretstore.Get(mcp.ID, ev.Name)
		if err != nil {
			fmt.Fprintf(os.Stderr, "aiworker-cli: no pude leer %s del llavero: %v\n", ev.Name, err)
			os.Exit(1)
		}
		env = append(env, ev.Name+"="+val)
	}

	if err := execServer(mcp.Command, mcp.Args, env); err != nil {
		fmt.Fprintf(os.Stderr, "aiworker-cli: fallo iniciando %s: %v\n", mcp.Command, err)
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`
aiworker-cli - Setup de herramientas de developer (MCP servers, skills)

Uso:
  aiworker                          Abre el menu interactivo
  aiworker mcp                       Va directo a Servidores MCP
  aiworker skills                     Va directo a Skills
  aiworker skills set-remote <url>     Configura/cambia el repo git compartido de skills
  aiworker doctor                     Chequea el entorno (git, uvx, claude, ...)
  aiworker version                    Muestra la version
  aiworker help                        Esta ayuda`)
}
