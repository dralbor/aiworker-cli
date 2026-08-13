// Package app is the interactive aiworker-cli shell: one Bubble Tea program
// with a main menu that opens into MCP server management, skills management
// and an environment doctor. It resizes to the terminal (tea.WindowSizeMsg)
// instead of assuming a fixed width.
package app

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"aiworker/cli/internal/catalog"
	"aiworker/cli/internal/claudecode"
	"aiworker/cli/internal/doctor"
	"aiworker/cli/internal/mcpclient"
	"aiworker/cli/internal/mcpinstall"
	"aiworker/cli/internal/prereq"
	"aiworker/cli/internal/skills"
	"aiworker/cli/internal/skillsmarket"
	"aiworker/cli/internal/styles"
)

// minCheckDisplay is how long the "Comprobando..." screen stays up even
// though exec.LookPath itself resolves near-instantly — long enough for a
// human to actually read "checking npx... ok" instead of a flash.
const minCheckDisplay = 450 * time.Millisecond

// autoAdvanceDelay is how long the success state holds before moving on to
// the MCP list on its own.
const autoAdvanceDelay = 550 * time.Millisecond

type screen int

const (
	screenMenu screen = iota
	screenMCPPrereq
	screenMCP
	screenMCPConfirmRemove
	screenMCPNeedsInstall
	screenMCPInstalling
	screenMCPTarget
	screenMCPForm
	screenSkillsRemotePrompt
	screenSkills
	screenSkillsNewCategory
	screenSkillsNewName
	screenSkillsNewFolder
	screenDoctor
)

type menuItem struct {
	Title string
	Desc  string
}

var menuItems = []menuItem{
	{"Servidores MCP", "Instalar / quitar servidores MCP (DBeaver, Atlassian, ...)"},
	{"Skills", "Crear y explorar skills locales, organizadas por carpeta"},
	{"Doctor", "Chequear que el entorno tenga lo que estas herramientas necesitan"},
	{"Salir", "Cerrar aiworker-cli"},
}

// Model is the single top-level Bubble Tea model for the whole program.
type Model struct {
	screen screen
	width  int
	height int
	toast  string
	err    error

	menuCursor int

	// MCP prereq check screen
	prereqChecking bool
	prereqResults  []prereq.Result
	spin           spinner.Model

	// MCP screen
	mcpCursor          int
	mcpStatus          map[string]mcpinstall.Status
	mcpExternal        []claudecode.Installed // installed but not in our catalog
	mcpExternalErr     error
	selectedID         string // catalog ID, or external server name, currently being installed/removed
	selectedIsExternal bool

	// MCP per-entry prereq-install flow
	installTool             prereq.Result
	installing              bool
	installOutput           string
	installErr              error
	resolvedCommandOverride string // absolute path to use instead of the bare catalog command, "desktop" target only

	targetCursor int

	formInputs []textinput.Model
	formIndex  int
	formTarget mcpclient.Target

	// Skills screen / marketplace
	skillsRoot        string
	skillsCats        []skills.Category
	skillsGitBacked   bool
	skillsSyncing     bool
	skillsSyncErr     error
	skillsPublishing  bool
	catInput          textinput.Model
	nameInput         textinput.Model
	folderInput       textinput.Model
	remotePromptInput textinput.Model

	// Doctor screen
	doctorChecks []doctor.Check

	pendingInitCmd tea.Cmd
}

// New builds the initial model, optionally jumping straight past the main
// menu into "mcp" or "skills" (used by the `aiworker mcp` / `aiworker
// skills` shortcuts).
func New(start string) *Model {
	root, _ := skills.Root()
	m := &Model{
		screen:     screenMenu,
		skillsRoot: root,
	}

	switch start {
	case "mcp":
		m.screen = screenMCPPrereq
	case "skills":
		m.pendingInitCmd = m.enterSkills()
	}
	return m
}

func (m *Model) Init() tea.Cmd {
	if m.screen == screenMCPPrereq {
		return m.startPrereqCheck()
	}
	if m.pendingInitCmd != nil {
		cmd := m.pendingInitCmd
		m.pendingInitCmd = nil
		return cmd
	}
	return nil
}

// --- Update ---------------------------------------------------------------

// prereqDoneMsg carries the result of checking every runtime launcher the
// catalog needs (npx, uvx, ...).
type prereqDoneMsg struct {
	results []prereq.Result
}

// prereqAutoAdvanceMsg fires after a short pause once every check passed, so
// the success state has time to actually be seen before moving on.
type prereqAutoAdvanceMsg struct{}

// startPrereqCheck enters the "Comprobando..." screen and kicks off the
// spinner plus the (artificially paced) check itself.
func (m *Model) startPrereqCheck() tea.Cmd {
	m.screen = screenMCPPrereq
	m.prereqChecking = true
	m.prereqResults = nil
	m.err = nil
	m.spin = spinner.New()
	m.spin.Spinner = spinner.Dot
	m.spin.Style = lipgloss.NewStyle().Foreground(styles.ColorAccent)
	return tea.Batch(m.spin.Tick, checkPrereqsCmd())
}

func checkPrereqsCmd() tea.Cmd {
	return tea.Tick(minCheckDisplay, func(time.Time) tea.Msg {
		return prereqDoneMsg{results: prereq.CheckAll(prereq.RequiredBinaries())}
	})
}

func autoAdvanceCmd() tea.Cmd {
	return tea.Tick(autoAdvanceDelay, func(time.Time) tea.Msg {
		return prereqAutoAdvanceMsg{}
	})
}

// installDoneMsg carries the outcome of running a tool's auto-installer.
type installDoneMsg struct {
	output string
	err    error
}

// startToolInstall runs m.installTool's installer (already confirmed by the
// user) as a background command, spinner visible meanwhile.
func (m *Model) startToolInstall() tea.Cmd {
	m.screen = screenMCPInstalling
	m.installing = true
	m.installOutput = ""
	m.installErr = nil
	m.spin = spinner.New()
	m.spin.Spinner = spinner.Dot
	m.spin.Style = lipgloss.NewStyle().Foreground(styles.ColorAccent)
	tool := m.installTool
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		cmd := tool.Tool.Installer.Command()
		out, err := cmd.CombinedOutput()
		return installDoneMsg{output: string(out), err: err}
	})
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case spinner.TickMsg:
		if m.prereqChecking || m.installing || m.skillsSyncing || m.skillsPublishing {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil
	case skillsSyncDoneMsg:
		m.skillsSyncing = false
		m.skillsSyncErr = msg.err
		if msg.err == nil {
			m.skillsCats, _ = skills.List(m.skillsRoot)
		}
		return m, nil
	case skillsPublishDoneMsg:
		m.skillsPublishing = false
		if msg.err != nil {
			m.err = fmt.Errorf("no se pudo subir %s (queda guardado local): %w", msg.what, msg.err)
		} else {
			m.toast = "✓ " + msg.what + " subido al repo compartido"
		}
		return m, nil
	case prereqDoneMsg:
		m.prereqChecking = false
		m.prereqResults = msg.results
		if prereq.AllOK(m.prereqResults) {
			return m, autoAdvanceCmd()
		}
		return m, nil
	case prereqAutoAdvanceMsg:
		if m.screen == screenMCPPrereq && prereq.AllOK(m.prereqResults) {
			m.enterMCPList()
		}
		return m, nil
	case installDoneMsg:
		m.installing = false
		m.installOutput = msg.output
		if msg.err != nil {
			m.installErr = fmt.Errorf("instalando %s: %w", m.installTool.Tool.Label, msg.err)
			return m, nil
		}
		path, ok := prereq.Resolve(m.installTool.Tool.Bin)
		if !ok {
			m.installErr = fmt.Errorf("%s se instalo pero no lo encuentro ni en PATH ni en las rutas conocidas - probá abrir una terminal nueva y reintentar", m.installTool.Tool.Label)
			return m, nil
		}
		m.resolvedCommandOverride = path
		m.toast = fmt.Sprintf("✓ %s instalado", m.installTool.Tool.Label)
		m.targetCursor = 0
		m.screen = screenMCPTarget
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Text-entry screens capture almost all keys themselves.
	switch m.screen {
	case screenSkillsNewCategory:
		return m.updateCatInput(msg)
	case screenSkillsNewName:
		return m.updateNameInput(msg)
	case screenSkillsNewFolder:
		return m.updateFolderInput(msg)
	case screenSkillsRemotePrompt:
		return m.updateRemotePromptInput(msg)
	case screenMCPForm:
		return m.updateFormInput(msg)
	}

	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "q", "esc":
		return m.back()
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "n":
		if m.screen == screenSkills {
			return m.startNewSkill()
		}
	case "f":
		if m.screen == screenSkills {
			return m.startNewFolder()
		}
	case "enter":
		return m.handleEnter()
	case "y":
		switch m.screen {
		case screenMCPConfirmRemove:
			return m.confirmRemove()
		case screenMCPNeedsInstall:
			return m, m.startToolInstall()
		}
	}
	return m, nil
}

// enterMCPList shows the MCP servers screen with freshly-read install
// status - never the value cached from whenever the app started or the last
// time this screen was open. Installed/removed state can change from
// outside this session (another terminal running `claude mcp add/remove`
// directly, omnisql installed by hand months ago, a previous aiworker run),
// so every entry into this screen re-reads it rather than trusting
// stale in-memory state.
func (m *Model) enterMCPList() {
	m.mcpStatus, _ = mcpinstall.GetStatus()
	m.mcpExternal, m.mcpExternalErr = mcpinstall.External()
	m.screen = screenMCP
	m.mcpCursor = 0
}

// mcpRow is one selectable line on the MCP screen: either a catalog entry
// (our curated "Disponibles" section) or something Claude Code has
// configured that isn't in our catalog ("Instalados manualmente").
type mcpRow struct {
	Catalog    catalog.MCPServer // valid when External is zero
	External   claudecode.Installed
	IsExternal bool
}

func (m *Model) mcpRows() []mcpRow {
	rows := make([]mcpRow, 0, len(catalog.MCPServers)+len(m.mcpExternal))
	for _, c := range catalog.MCPServers {
		rows = append(rows, mcpRow{Catalog: c})
	}
	for _, e := range m.mcpExternal {
		rows = append(rows, mcpRow{External: e, IsExternal: true})
	}
	return rows
}

func (m *Model) moveCursor(delta int) {
	switch m.screen {
	case screenMenu:
		m.menuCursor = clamp(m.menuCursor+delta, 0, len(menuItems)-1)
	case screenMCP:
		if n := len(m.mcpRows()); n > 0 {
			m.mcpCursor = clamp(m.mcpCursor+delta, 0, n-1)
		}
	case screenMCPTarget:
		m.targetCursor = clamp(m.targetCursor+delta, 0, len(mcpclient.Targets())-1)
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (m *Model) back() (tea.Model, tea.Cmd) {
	m.toast = ""
	m.err = nil
	switch m.screen {
	case screenMenu:
		return m, tea.Quit
	case screenMCPConfirmRemove:
		m.screen = screenMCP
	case screenMCPNeedsInstall:
		m.screen = screenMCP
	case screenMCPInstalling:
		if !m.installing {
			m.screen = screenMCP
		}
	case screenMCPTarget:
		m.screen = screenMCP
	case screenMCPForm:
		m.screen = screenMCP
	case screenSkillsNewCategory, screenSkillsNewName, screenSkillsNewFolder:
		m.screen = screenSkills
	default:
		m.screen = screenMenu
	}
	return m, nil
}

func (m *Model) handleEnter() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMenu:
		switch m.menuCursor {
		case 0:
			return m, m.startPrereqCheck()
		case 1:
			return m, m.enterSkills()
		case 2:
			// Always re-run fresh on entry, never show a cached result from
			// a previous visit - PATH/installed state can change in a
			// second (see enterMCPList for the same rule on the MCP list).
			m.doctorChecks = doctor.Run()
			m.screen = screenDoctor
		case 3:
			return m, tea.Quit
		}
	case screenMCPPrereq:
		// Don't make the user wait out the auto-advance pause if everything
		// already passed; if something's missing, enter continues anyway
		// (they may only care about the entry that doesn't need it).
		if !m.prereqChecking {
			m.enterMCPList()
		}
	case screenMCP:
		rows := m.mcpRows()
		if m.mcpCursor >= len(rows) {
			return m, nil
		}
		row := rows[m.mcpCursor]
		m.resolvedCommandOverride = ""

		if row.IsExternal {
			m.selectedID = row.External.Name
			m.selectedIsExternal = true
			m.screen = screenMCPConfirmRemove
			return m, nil
		}
		m.selectedIsExternal = false
		mcp := row.Catalog
		m.selectedID = mcp.ID

		if status, installed := m.mcpStatus[mcp.ID]; installed {
			if !status.ManagedByUs {
				// We can still see it's there and can still remove it
				// (generic `claude mcp remove`), we just don't have our own
				// record of how it was set up.
				m.selectedIsExternal = true
			}
			m.screen = screenMCPConfirmRemove
			return m, nil
		}

		if mcp.Command != "" {
			if result := prereq.Check(mcp.Command); !result.OK && result.CanAutoInstall() {
				m.installTool = result
				m.installErr = nil
				m.installOutput = ""
				m.screen = screenMCPNeedsInstall
				return m, nil
			}
		}

		if mcp.Transport == catalog.TransportHTTP && len(mcp.Env) == 0 {
			// No target choice (Claude Code only, for now) and nothing to
			// ask - straight to installing, then remind about `claude mcp
			// login`.
			return m.applyHTTPEntry(mcp)
		}

		m.targetCursor = 0
		m.screen = screenMCPTarget
	case screenMCPTarget:
		m.formTarget = mcpclient.Targets()[m.targetCursor]
		m.startForm()
	case screenMCPInstalling:
		if !m.installing && m.installErr != nil {
			m.screen = screenMCP
		}
	case screenDoctor:
		m.screen = screenMenu
	}
	return m, nil
}

func (m *Model) confirmRemove() (tea.Model, tea.Cmd) {
	var toast string
	var removeErr error
	if m.selectedIsExternal {
		removeErr = mcpinstall.RemoveExternal(m.selectedID)
	} else {
		removeErr = mcpinstall.Remove(m.selectedID)
	}
	if removeErr == nil {
		toast = fmt.Sprintf("Se quito %s", m.selectedID)
	}
	m.enterMCPList()
	m.err = removeErr
	m.toast = toast
	return m, nil
}

// applyHTTPEntry installs an OAuth/HTTP catalog entry (no target choice, no
// env form - see the screenMCP case in handleEnter) and points the user at
// the follow-up login step, which aiworker-cli does not automate: OAuth
// needs an interactive browser handoff that doesn't fit cleanly inside this
// TUI's own alt-screen session.
func (m *Model) applyHTTPEntry(mcp catalog.MCPServer) (tea.Model, tea.Cmd) {
	var toast string
	applyErr := mcpinstall.Apply(mcp, "claude-user", mcp.ID, nil)
	if applyErr == nil {
		toast = fmt.Sprintf("✓ %s agregado. Corre \"claude mcp login %s\" para conectar tu cuenta.", mcp.Name, mcp.ID)
	}
	m.enterMCPList()
	m.err = applyErr
	m.toast = toast
	return m, nil
}

// enterSkills asks (once, ever) for a shared skills repo remote before
// showing the screen; on every later visit it goes straight to
// openSkillsScreen.
func (m *Model) enterSkills() tea.Cmd {
	needsPrompt, _ := skillsmarket.NeedsRemotePrompt()
	if !needsPrompt {
		return m.openSkillsScreen()
	}
	m.remotePromptInput = textinput.New()
	m.remotePromptInput.Placeholder = "https://github.com/tuempresa/skills.git  (vacio = solo local)"
	m.remotePromptInput.CharLimit = 200
	m.remotePromptInput.Width = 52
	m.remotePromptInput.Focus()
	m.screen = screenSkillsRemotePrompt
	return textinput.Blink
}

// openSkillsScreen shows whatever is on disk right now (instant - no network
// wait) and, if a shared remote is configured, kicks off a background
// clone/pull so teammates' new skills show up without the user asking.
func (m *Model) openSkillsScreen() tea.Cmd {
	remote, _ := skillsmarket.Remote()
	m.skillsGitBacked = remote != ""
	m.skillsCats, _ = skills.List(m.skillsRoot)
	m.screen = screenSkills
	if !m.skillsGitBacked {
		return nil
	}
	m.skillsSyncing = true
	m.skillsSyncErr = nil
	return tea.Batch(m.startSpinner(), skillsSyncCmd(m.skillsRoot))
}

func (m *Model) startSpinner() tea.Cmd {
	m.spin = spinner.New()
	m.spin.Spinner = spinner.Dot
	m.spin.Style = lipgloss.NewStyle().Foreground(styles.ColorAccent)
	return m.spin.Tick
}

type skillsSyncDoneMsg struct{ err error }

func skillsSyncCmd(root string) tea.Cmd {
	return func() tea.Msg {
		return skillsSyncDoneMsg{err: skillsmarket.Prepare(root)}
	}
}

type skillsPublishDoneMsg struct {
	what string
	err  error
}

// publishSkillsChange stages+commits+pushes path under the user's own git
// identity in the background, once the local write already happened (the
// UI never waits on this - it's a "did it make it to the team" follow-up).
func (m *Model) publishSkillsChange(path, message, what string) tea.Cmd {
	if !m.skillsGitBacked {
		return nil
	}
	m.skillsPublishing = true
	return tea.Batch(m.startSpinner(), func() tea.Msg {
		_, err := skillsmarket.Publish(m.skillsRoot, path, message)
		if errors.Is(err, skillsmarket.ErrLocalOnly) {
			err = nil
		}
		return skillsPublishDoneMsg{what: what, err: err}
	})
}

func (m *Model) startNewSkill() (tea.Model, tea.Cmd) {
	m.catInput = textinput.New()
	m.catInput.Placeholder = "frontend / backend / types / ..."
	m.catInput.Focus()
	m.catInput.CharLimit = 40
	m.screen = screenSkillsNewCategory
	return m, textinput.Blink
}

func (m *Model) startNewFolder() (tea.Model, tea.Cmd) {
	m.folderInput = textinput.New()
	m.folderInput.Placeholder = "frontend / backend / types / ..."
	m.folderInput.Focus()
	m.folderInput.CharLimit = 40
	m.screen = screenSkillsNewFolder
	return m, textinput.Blink
}

func (m *Model) updateCatInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.back()
	case "enter":
		if strings.TrimSpace(m.catInput.Value()) == "" {
			return m, nil
		}
		m.nameInput = textinput.New()
		m.nameInput.Placeholder = "nombre de la skill"
		m.nameInput.Focus()
		m.nameInput.CharLimit = 60
		m.screen = screenSkillsNewName
		return m, textinput.Blink
	}
	var cmd tea.Cmd
	m.catInput, cmd = m.catInput.Update(msg)
	return m, cmd
}

func (m *Model) updateNameInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenSkillsNewCategory
		return m, nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		if name == "" {
			return m, nil
		}
		path, err := skills.New(m.skillsRoot, m.catInput.Value(), name)
		m.skillsCats, _ = skills.List(m.skillsRoot)
		m.screen = screenSkills
		if err != nil {
			m.err = err
			return m, nil
		}
		m.toast = "Creada en " + path
		m.err = nil
		what := skills.Slug(m.catInput.Value()) + "/" + skills.Slug(name)
		return m, m.publishSkillsChange(path, "skills: add "+what, what)
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

func (m *Model) updateFolderInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.back()
	case "enter":
		name := strings.TrimSpace(m.folderInput.Value())
		if name == "" {
			return m, nil
		}
		path, err := skills.NewCategory(m.skillsRoot, name)
		m.skillsCats, _ = skills.List(m.skillsRoot)
		m.screen = screenSkills
		if err != nil {
			m.err = err
			return m, nil
		}
		slug := skills.Slug(name)
		m.toast = "Carpeta creada: " + path
		m.err = nil
		return m, m.publishSkillsChange(path, "skills: add category "+slug, slug+"/")
	}
	var cmd tea.Cmd
	m.folderInput, cmd = m.folderInput.Update(msg)
	return m, cmd
}

func (m *Model) updateRemotePromptInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if err := skillsmarket.SetRemote(""); err != nil {
			m.err = err
			return m, nil
		}
		return m, m.openSkillsScreen()
	case "enter":
		remote := strings.TrimSpace(m.remotePromptInput.Value())
		if err := skillsmarket.SetRemote(remote); err != nil {
			m.err = err
			return m, nil
		}
		return m, m.openSkillsScreen()
	}
	var cmd tea.Cmd
	m.remotePromptInput, cmd = m.remotePromptInput.Update(msg)
	return m, cmd
}

func (m *Model) startForm() {
	mcp, _ := catalog.Find(m.selectedID)
	m.formInputs = make([]textinput.Model, len(mcp.Env))
	for i, ev := range mcp.Env {
		ti := textinput.New()
		ti.Placeholder = ev.Description
		ti.CharLimit = 256
		ti.Width = 48
		if ev.Secret {
			ti.EchoMode = textinput.EchoPassword
			ti.EchoCharacter = '•'
		}
		m.formInputs[i] = ti
	}
	m.formIndex = 0
	if len(m.formInputs) > 0 {
		m.formInputs[0].Focus()
	}
	m.screen = screenMCPForm
}

func (m *Model) updateFormInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m.back()
	case "enter":
		if m.formIndex < len(m.formInputs)-1 {
			m.formInputs[m.formIndex].Blur()
			m.formIndex++
			m.formInputs[m.formIndex].Focus()
			return m, textinput.Blink
		}
		return m.submitForm()
	}
	var cmd tea.Cmd
	m.formInputs[m.formIndex], cmd = m.formInputs[m.formIndex].Update(msg)
	return m, cmd
}

func (m *Model) submitForm() (tea.Model, tea.Cmd) {
	mcp, _ := catalog.Find(m.selectedID)
	if m.resolvedCommandOverride != "" {
		// We had to auto-install this tool ourselves: PATH may still be
		// stale for other processes too, so pin the absolute path we just
		// resolved. Every target is machine-local, so this is always safe.
		mcp.Command = m.resolvedCommandOverride
	}
	m.resolvedCommandOverride = ""
	values := make([]mcpinstall.EnvValue, len(mcp.Env))
	for i, ev := range mcp.Env {
		values[i] = mcpinstall.EnvValue{Name: ev.Name, Value: m.formInputs[i].Value(), Secret: ev.Secret}
	}
	var toast string
	var applyErr error
	if err := mcpinstall.Apply(mcp, m.formTarget.ID, mcp.ID, values); err != nil {
		applyErr = err
	} else {
		toast = fmt.Sprintf("✓ %s instalado en %s", mcp.Name, m.formTarget.Label)
	}
	m.enterMCPList()
	m.err = applyErr
	m.toast = toast
	return m, nil
}

// --- View -------------------------------------------------------------

func (m *Model) View() string {
	w := m.width
	if w <= 0 {
		w = 80
	}
	contentWidth := w - 8
	if contentWidth < 30 {
		contentWidth = 30
	}
	if contentWidth > 96 {
		contentWidth = 96
	}

	var body string
	switch m.screen {
	case screenMenu:
		body = m.viewMenu(contentWidth)
	case screenMCPPrereq:
		body = m.viewMCPPrereq(contentWidth)
	case screenMCP:
		body = m.viewMCP(contentWidth)
	case screenMCPConfirmRemove:
		body = m.viewConfirmRemove(contentWidth)
	case screenMCPNeedsInstall:
		body = m.viewMCPNeedsInstall(contentWidth)
	case screenMCPInstalling:
		body = m.viewMCPInstalling(contentWidth)
	case screenMCPTarget:
		body = m.viewTarget(contentWidth)
	case screenMCPForm:
		body = m.viewForm(contentWidth)
	case screenSkills:
		body = m.viewSkills(contentWidth)
	case screenSkillsRemotePrompt:
		body = m.viewSkillsRemotePrompt(contentWidth)
	case screenSkillsNewCategory:
		body = m.viewSkillsNewCategory(contentWidth)
	case screenSkillsNewName:
		body = m.viewSkillsNewName(contentWidth)
	case screenSkillsNewFolder:
		body = m.viewSkillsNewFolder(contentWidth)
	case screenDoctor:
		body = m.viewDoctor(contentWidth)
	}

	panel := styles.Panel.Width(contentWidth).Render(body)
	return styles.Container.Render(panel)
}

func header(title string) string {
	return styles.Title.Render(title) + "\n"
}

func (m *Model) footer(help string) string {
	var extra string
	if m.err != nil {
		extra = "\n" + styles.Error.Render("✗ "+m.err.Error())
	} else if m.toast != "" {
		extra = "\n" + styles.Success.Render(m.toast)
	}
	return extra + "\n" + styles.Help.Render(help)
}

func (m *Model) viewMenu(w int) string {
	var b strings.Builder
	b.WriteString(logo())
	b.WriteString("\n")
	b.WriteString(lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(
		styles.Subtitle.Render("Herramientas de developer onboarding")))
	b.WriteString("\n\n")

	for i, item := range menuItems {
		selected := i == m.menuCursor
		marker := "  "
		nameStyle := styles.ItemName
		if selected {
			marker = styles.Title.Render("→ ")
			nameStyle = styles.ItemNameSelected
		}
		b.WriteString(marker + nameStyle.Render(item.Title) + "\n")
		b.WriteString("    " + styles.ItemDesc.Render(item.Desc) + "\n")
	}
	b.WriteString(m.footer("↑/↓ mover • enter elegir • q salir"))
	return b.String()
}

func (m *Model) viewMCPPrereq(w int) string {
	var b strings.Builder
	b.WriteString(header("Preparando Servidores MCP"))

	bins := prereq.RequiredBinaries()
	for i, bin := range bins {
		switch {
		case m.prereqChecking:
			b.WriteString(m.spin.View() + " " + styles.ItemDesc.Render("Comprobando "+bin+"...") + "\n")
		case i < len(m.prereqResults):
			r := m.prereqResults[i]
			b.WriteString(styles.Check(r.OK) + " " + styles.ItemName.Render(r.Tool.Label))
			if r.OK {
				b.WriteString("  " + styles.Dim.Render(r.Path))
			}
			b.WriteString("\n")
			if !r.OK {
				b.WriteString("    " + styles.Warn.Render(bin+" no esta en el PATH") + "\n")
				b.WriteString("    " + styles.Dim.Render(r.Tool.InstallHint) + "\n")
			}
		}
	}
	b.WriteString("\n")

	switch {
	case m.prereqChecking:
		b.WriteString(m.footer("esc cancelar"))
	case prereq.AllOK(m.prereqResults):
		b.WriteString(styles.Success.Render("✓ Todo listo") + "\n")
		b.WriteString(m.footer("continuando... (enter para saltar la espera)"))
	default:
		b.WriteString(styles.Warn.Render("Falta algo del listado de arriba.") + "\n")
		b.WriteString(m.footer("enter continuar igual • esc volver al menu"))
	}
	return b.String()
}

func (m *Model) viewMCP(w int) string {
	var b strings.Builder
	b.WriteString(header("Servidores MCP"))

	rows := m.mcpRows()
	row := 0

	b.WriteString(styles.ItemNameSelected.Render("Disponibles — usados por el equipo") + "\n")
	for _, mcp := range catalog.MCPServers {
		b.WriteString(m.viewMCPRow(row, w, mcpRow{Catalog: mcp}))
		row++
	}

	b.WriteString("\n" + styles.ItemNameSelected.Render("Instalados manualmente por vos") + "\n")
	switch {
	case m.mcpExternalErr != nil:
		b.WriteString("  " + styles.Warn.Render("no pude consultar 'claude mcp list': "+m.mcpExternalErr.Error()) + "\n")
	case len(m.mcpExternal) == 0:
		b.WriteString("  " + styles.Dim.Render("(ninguno)") + "\n")
	default:
		for _, ext := range m.mcpExternal {
			b.WriteString(m.viewMCPRow(row, w, mcpRow{External: ext, IsExternal: true}))
			row++
		}
	}

	action := "enter instalar"
	if len(rows) > 0 && m.mcpCursor < len(rows) {
		r := rows[m.mcpCursor]
		_, installed := m.mcpStatus[r.Catalog.ID]
		if r.IsExternal || installed {
			action = "enter quitar"
		}
	}
	b.WriteString(m.footer("↑/↓ mover • " + action + " • esc volver"))
	return b.String()
}

// viewMCPRow renders one selectable line (catalog or external), at absolute
// row index idx into m.mcpRows().
func (m *Model) viewMCPRow(idx, w int, r mcpRow) string {
	var b strings.Builder
	selected := idx == m.mcpCursor

	marker := "  "
	nameStyle := styles.ItemName
	if selected {
		marker = "→ "
		nameStyle = styles.ItemNameSelected
	}

	if r.IsExternal {
		b.WriteString(marker + styles.Dot(true) + " " + nameStyle.Render(r.External.Name) + "\n")
		b.WriteString("    " + wrap(styles.Dim, r.External.Detail, w-6) + "\n")
		return b.String()
	}

	mcp := r.Catalog
	status, installed := m.mcpStatus[mcp.ID]
	b.WriteString(marker + styles.Dot(installed) + " " + nameStyle.Render(mcp.Name))
	// The dot alone is the installed/not signal. Only add a parenthetical
	// when we actually know something concrete (which target) - no filler
	// text like "instalado a mano" just to say "yes, installed".
	if installed && status.ManagedByUs {
		if t, ok := mcpclient.FindTarget(status.Detail.Target); ok {
			b.WriteString("  " + styles.Dim.Render("("+t.Label+")"))
		}
	}
	b.WriteString("\n")
	b.WriteString("    " + wrap(styles.ItemDesc, mcp.Description, w-6) + "\n")
	return b.String()
}

func (m *Model) viewConfirmRemove(w int) string {
	var b strings.Builder
	b.WriteString(header("Confirmar"))
	if m.selectedIsExternal {
		b.WriteString(fmt.Sprintf(
			"Quitar %s: no fue instalado por aiworker-cli (no hay secretos nuestros que borrar), pero corre 'claude mcp remove' igual.\n\n",
			styles.ItemNameSelected.Render(m.selectedID)))
	} else {
		mcp, _ := catalog.Find(m.selectedID)
		b.WriteString(fmt.Sprintf("Quitar %s: borra la entrada del config y sus secretos guardados.\n\n", styles.ItemNameSelected.Render(mcp.Name)))
	}
	b.WriteString(m.footer("y confirmar • esc cancelar"))
	return b.String()
}

func (m *Model) viewMCPNeedsInstall(w int) string {
	mcp, _ := catalog.Find(m.selectedID)
	t := m.installTool.Tool
	var b strings.Builder
	b.WriteString(header("Falta un requisito"))
	b.WriteString(fmt.Sprintf("%s necesita %s y no lo encuentro en el PATH.\n\n", styles.ItemNameSelected.Render(mcp.Name), styles.ItemNameSelected.Render(t.Label)))
	b.WriteString(styles.ItemDesc.Render(m.installTool.Tool.Installer.Describe) + "\n")
	b.WriteString(styles.Dim.Render(t.InstallHint) + "\n\n")
	b.WriteString(m.footer("y instalar ahora y continuar • esc volver"))
	return b.String()
}

func (m *Model) viewMCPInstalling(w int) string {
	t := m.installTool.Tool
	var b strings.Builder
	b.WriteString(header("Instalando " + t.Label))

	switch {
	case m.installing:
		b.WriteString(m.spin.View() + " " + styles.ItemDesc.Render("Instalando, puede tardar un momento...") + "\n")
	case m.installErr != nil:
		b.WriteString(styles.Error.Render("✗ "+m.installErr.Error()) + "\n")
		if m.installOutput != "" {
			b.WriteString("\n" + wrap(styles.Dim, strings.TrimSpace(m.installOutput), w-4) + "\n")
		}
	default:
		b.WriteString(styles.Success.Render("✓ Listo, continuando...") + "\n")
	}

	if m.installing {
		b.WriteString(m.footer("esc cancelar"))
	} else {
		b.WriteString(m.footer("enter/esc volver"))
	}
	return b.String()
}

func (m *Model) viewTarget(w int) string {
	mcp, _ := catalog.Find(m.selectedID)
	var b strings.Builder
	b.WriteString(header("Donde instalar " + mcp.Name + "?"))
	for i, t := range mcpclient.Targets() {
		selected := i == m.targetCursor
		marker := "  "
		nameStyle := styles.ItemName
		if selected {
			marker = "→ "
			nameStyle = styles.ItemNameSelected
		}
		b.WriteString(marker + nameStyle.Render(t.Label) + "\n")
		b.WriteString("    " + styles.ItemDesc.Render(t.Description) + "\n")
	}
	b.WriteString(m.footer("↑/↓ mover • enter elegir • esc volver"))
	return b.String()
}

func (m *Model) viewForm(w int) string {
	mcp, _ := catalog.Find(m.selectedID)
	var b strings.Builder
	b.WriteString(header("Configurar " + mcp.Name))
	for i, ev := range mcp.Env {
		lbl := ev.Name
		if ev.Secret {
			lbl += " 🔒"
		}
		marker := "  "
		if i == m.formIndex {
			marker = styles.InputPrompt.Render("→ ")
		}
		b.WriteString(marker + styles.Label.Render(lbl) + m.formInputs[i].View() + "\n")
	}
	hint := "enter siguiente/guardar • esc cancelar"
	if len(m.formInputs) > 0 {
		hint = "🔒 = se guarda en el llavero del sistema, nunca en texto plano  •  " + hint
	}
	b.WriteString(m.footer(hint))
	return b.String()
}

func (m *Model) viewSkills(w int) string {
	var b strings.Builder
	b.WriteString(header("Skills"))

	loc := m.skillsRoot
	if m.skillsGitBacked {
		loc += "  " + styles.Success.Render("● compartido")
	} else {
		loc += "  " + styles.Dim.Render("○ solo local")
	}
	b.WriteString(styles.Dim.Render(loc) + "\n")

	switch {
	case m.skillsSyncing:
		b.WriteString(m.spin.View() + " " + styles.ItemDesc.Render("Sincronizando con el repo compartido...") + "\n")
	case m.skillsSyncErr != nil:
		b.WriteString(styles.Warn.Render("⚠ no se pudo sincronizar, mostrando la copia local") + "\n")
	}
	if m.skillsPublishing {
		b.WriteString(m.spin.View() + " " + styles.ItemDesc.Render("Subiendo cambios al repo compartido...") + "\n")
	}
	b.WriteString("\n")

	if len(m.skillsCats) == 0 {
		b.WriteString(styles.ItemDesc.Render("Todavia no hay skills locales. n = nueva skill, f = nueva carpeta.") + "\n")
	}
	for _, cat := range m.skillsCats {
		b.WriteString(styles.ItemNameSelected.Render(cat.Name) + "\n")
		if len(cat.Skills) == 0 {
			b.WriteString("  " + styles.Dim.Render("└── (vacia)") + "\n")
		}
		for i, sk := range cat.Skills {
			branch := "├──"
			if i == len(cat.Skills)-1 {
				branch = "└──"
			}
			b.WriteString("  " + styles.Dim.Render(branch) + " " + sk.Name + "\n")
		}
	}
	b.WriteString(m.footer("n nueva skill • f nueva carpeta • esc volver"))
	return b.String()
}

func (m *Model) viewSkillsRemotePrompt(w int) string {
	var b strings.Builder
	b.WriteString(header("Skills compartidas"))
	b.WriteString(wrap(styles.ItemDesc, "Repo git donde el equipo guarda skills en comun. Cada skill que crees se commitea y sube ahi con tu cuenta de git. Dejalo vacio para trabajar solo local (se puede configurar despues).", w-4) + "\n\n")
	b.WriteString(styles.InputPrompt.Render("→ ") + m.remotePromptInput.View() + "\n")
	b.WriteString(m.footer("enter confirmar (o vacio) • esc = solo local"))
	return b.String()
}

func (m *Model) viewSkillsNewFolder(w int) string {
	var b strings.Builder
	b.WriteString(header("Nueva carpeta de skills"))
	b.WriteString(styles.ItemDesc.Render("Ej: frontend, backend, types") + "\n\n")
	b.WriteString(styles.InputPrompt.Render("→ ") + m.folderInput.View() + "\n")
	b.WriteString(m.footer("enter crear • esc cancelar"))
	return b.String()
}

func (m *Model) viewSkillsNewCategory(w int) string {
	var b strings.Builder
	b.WriteString(header("Nueva skill - categoria"))
	b.WriteString(styles.ItemDesc.Render("Carpeta donde va a vivir, ej: frontend, backend, types") + "\n\n")
	b.WriteString(styles.InputPrompt.Render("→ ") + m.catInput.View() + "\n")
	b.WriteString(m.footer("enter continuar • esc cancelar"))
	return b.String()
}

func (m *Model) viewSkillsNewName(w int) string {
	var b strings.Builder
	b.WriteString(header("Nueva skill - nombre"))
	b.WriteString(styles.ItemDesc.Render("Carpeta: "+skills.Slug(m.catInput.Value())+"/") + "\n\n")
	b.WriteString(styles.InputPrompt.Render("→ ") + m.nameInput.View() + "\n")
	b.WriteString(m.footer("enter crear • esc volver"))
	return b.String()
}

func (m *Model) viewDoctor(w int) string {
	var b strings.Builder
	b.WriteString(header("Doctor"))
	for _, c := range m.doctorChecks {
		b.WriteString(styles.Check(c.OK) + " " + styles.ItemName.Render(c.Label) + "\n")
		b.WriteString("    " + styles.Dim.Render(c.Detail) + "\n")
	}
	b.WriteString(m.footer("enter/esc volver"))
	return b.String()
}

func wrap(style lipgloss.Style, text string, width int) string {
	if width < 10 {
		width = 10
	}
	return style.Width(width).Render(text)
}

func logo() string {
	art := `
 █████╗ ██╗██╗    ██╗ ██████╗ ██████╗ ██╗  ██╗███████╗██████╗
██╔══██╗██║██║    ██║██╔═══██╗██╔══██╗██║ ██╔╝██╔════╝██╔══██╗
███████║██║██║ █╗ ██║██║   ██║██████╔╝█████╔╝ █████╗  ██████╔╝
██╔══██║██║██║███╗██║██║   ██║██╔══██╗██╔═██╗ ██╔══╝  ██╔══██╗
██║  ██║██║╚███╔███╔╝╚██████╔╝██║  ██║██║  ██╗███████╗██║  ██║
╚═╝  ╚═╝╚═╝ ╚══╝╚══╝  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝╚═╝  ╚═╝
`
	return styles.Title.Align(lipgloss.Center).Render(art)
}
