// Package tui implements the dbman Bubble Tea interface: a database list
// pane, a preview pane with Info/Env/URLs tabs, create/delete modals, and a
// keybinding help overlay.
package tui

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/muizzyranking/dbman/internal/adapters"
	"github.com/muizzyranking/dbman/internal/config"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
	"github.com/muizzyranking/dbman/internal/util"
)

type execDoneMsg struct{ err error }

type createDoneMsg struct {
	err    error
	pulled bool
}

type screen int

const (
	sList screen = iota
	sDetail
	sCreate
	sDeleteConfirm
)

// Preview tabs: Info | Env | URLs
const (
	tabInfo = iota
	tabEnv
	tabURLs
)

type Model struct {
	cfg      config.Config
	list     []store.Instance
	status   map[string]docker.Status
	dock     docker.Client
	screen   screen // sList = left focused, sDetail = right focused; sCreate/sDeleteConfirm = modals
	cursor   int
	scroll   int
	filter   string
	tab      int
	showPw   bool
	msg      string
	msgIsErr bool
	width    int
	height   int
	showHelp bool

	filtering   bool
	filterInput textinput.Model

	inputs    []textinput.Model
	typeIdx   int
	types     []string
	focusIdx  int
	keepVol   bool
	delTarget string

	// async create (image pull + docker run without freezing the UI)
	creating    bool
	createStage string
	createSpin  spinner.Model
}

// ---------- theme ----------

type theme struct {
	accent, success, warning, err, muted lipgloss.Color
}

func (m Model) th() theme {
	c := m.cfg.Theme
	def := config.DefaultConfig().Theme
	if c.Accent == "" {
		c.Accent = def.Accent
	}
	if c.Success == "" {
		c.Success = def.Success
	}
	if c.Warning == "" {
		c.Warning = def.Warning
	}
	if c.Error == "" {
		c.Error = def.Error
	}
	if c.Muted == "" {
		c.Muted = def.Muted
	}
	return theme{
		accent:  lipgloss.Color(c.Accent),
		success: lipgloss.Color(c.Success),
		warning: lipgloss.Color(c.Warning),
		err:     lipgloss.Color(c.Error),
		muted:   lipgloss.Color(c.Muted),
	}
}

// Per-engine accent for the type pill + icon.
func typeColor(typ string) lipgloss.Color {
	switch strings.ToLower(typ) {
	case "postgres":
		return lipgloss.Color("#58A6FF")
	case "redis":
		return lipgloss.Color("#F85149")
	case "mysql", "mariadb":
		return lipgloss.Color("#D29922")
	case "mongo":
		return lipgloss.Color("#3FB950")
	default:
		return lipgloss.Color("#8B949E")
	}
}

// nerd icon (fa-database, tinted per engine). Requires a Nerd Font;
// disable with `"icons": false` in config.
func typeIcon(typ string, icons bool) string {
	if !icons {
		return ""
	}
	return " "
}

func InitialModel() Model {
	cfg, _, _ := config.Load()
	list, _, _ := store.Load()
	ti := make([]textinput.Model, 6)
	placeholders := []string{"name (e.g. myproj-pg)", "image (empty = default)", "user (empty = default)", "password (empty = auto)", "db name (empty = default)", "port (empty = auto)"}
	for i := range ti {
		t := textinput.New()
		t.Placeholder = placeholders[i]
		t.CharLimit = 128
		if i == 0 {
			t.Focus()
		}
		ti[i] = t
	}
	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.CharLimit = 64
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	accent := cfg.Theme.Accent
	if accent == "" {
		accent = "#7C6CFF"
	}
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(accent))
	m := Model{
		cfg:         cfg,
		list:        dedupe(list),
		status:      map[string]docker.Status{},
		dock:        docker.NewCLIClient(),
		inputs:      ti,
		types:       adapters.Types(),
		width:       110,
		height:      30,
		filterInput: fi,
	}
	m.refreshStatus()
	return m
}

func dedupe(list []store.Instance) []store.Instance {
	seen := map[string]bool{}
	out := make([]store.Instance, 0, len(list))
	for _, v := range list {
		if seen[v.Name] {
			continue
		}
		seen[v.Name] = true
		out = append(out, v)
	}
	return out
}

func (m *Model) refreshStatus() {
	for _, inst := range m.list {
		m.status[inst.ContainerName] = m.dock.ContainerStatus(inst.ContainerName)
	}
}

func (m *Model) reload() {
	list, _, _ := store.Load()
	m.list = dedupe(list)
	m.refreshStatus()
	rows := visibleList(*m)
	if m.cursor >= len(rows) {
		m.cursor = max(len(rows)-1, 0)
	}
	if m.scroll > m.cursor {
		m.scroll = m.cursor
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case execDoneMsg:
		m.reload()
		if msg.err != nil {
			m.msg, m.msgIsErr = "command exited: "+msg.err.Error(), true
		} else {
			m.msg, m.msgIsErr = "", false
		}
		return m, nil
	case createDoneMsg:
		m.creating = false
		if msg.err != nil {
			m.setMsg("create failed: "+msg.err.Error(), true)
			m.screen = sCreate
			return m, nil
		}
		m.screen = sList
		m.reload()
		if msg.pulled {
			m.setMsg("image pulled + created", false)
		} else {
			m.setMsg("created", false)
		}
		return m, nil
	case spinner.TickMsg:
		if m.creating {
			var cmd tea.Cmd
			m.createSpin, cmd = m.createSpin.Update(msg)
			return m, cmd
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		if m.showHelp {
			k := msg.String()
			if k == "?" || k == "esc" || k == "q" || k == "enter" {
				m.showHelp = false
			}
			return m, nil
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		return m.handleKey(msg)
	}
	if m.screen == sCreate {
		var cmds []tea.Cmd
		for i := range m.inputs {
			var c tea.Cmd
			m.inputs[i], c = m.inputs[i].Update(msg)
			cmds = append(cmds, c)
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "?" {
		m.showHelp = true
		return m, nil
	}
	switch m.screen {
	case sList:
		return m.updateList(k, msg)
	case sDetail:
		return m.updateDetail(k)
	case sCreate:
		return m.updateCreate(k, msg)
	case sDeleteConfirm:
		return m.updateDeleteConfirm(k)
	}
	return m, nil
}

func (m *Model) setMsg(s string, isErr bool) {
	m.msg, m.msgIsErr = s, isErr
}

// ---------- filter mode (vim `/`) ----------

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	switch k {
	case "enter":
		m.filter = m.filterInput.Value()
		m.filtering = false
		m.cursor, m.scroll = 0, 0
		m.filterInput.Blur()
		return m, nil
	case "esc":
		m.filtering = false
		m.filterInput.Blur()
		return m, nil
	}
	var c tea.Cmd
	m.filterInput, c = m.filterInput.Update(msg)
	m.filter = m.filterInput.Value()
	m.cursor, m.scroll = 0, 0
	return m, c
}

// ---------- list: vim navigation. Tab / enter move to preview ----------

func (m Model) updateList(k string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := visibleList(m)
	last := max(len(rows)-1, 0)
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		m.setMsg("", false)
	case "down", "j":
		if m.cursor < last {
			m.cursor++
		}
		m.setMsg("", false)
	case "g":
		m.cursor = 0
	case "G":
		m.cursor = last
	case "ctrl+d":
		m.cursor += 10
		if m.cursor > last {
			m.cursor = last
		}
	case "ctrl+u":
		m.cursor -= 10
		if m.cursor < 0 {
			m.cursor = 0
		}
	case "r":
		m.reload()
		m.setMsg("refreshed", false)
	case "tab", "enter":
		if len(rows) > 0 {
			m.screen = sDetail
			m.tab = tabInfo
			m.showPw = false
			m.setMsg("", false)
		}
	case "n", "a":
		m.screen = sCreate
		m.focusIdx = 0
		if strings.TrimSpace(m.inputs[1].Value()) == "" {
			m.autofillImage()
		}
		m.focusInputs()
		m.setMsg("", false)
	case "s":
		if len(rows) > 0 {
			inst := rows[m.cursor]
			st := m.status[inst.ContainerName]
			if st.Running {
				_ = m.dock.Stop(inst.ContainerName)
			} else {
				_ = m.dock.Start(inst.ContainerName)
			}
			m.reload()
		}
	case "R":
		if len(rows) > 0 {
			_ = m.dock.Restart(rows[m.cursor].ContainerName)
			m.reload()
		}
	case "D", "d":
		if len(rows) > 0 {
			m.delTarget = rows[m.cursor].Name
			m.keepVol = false
			m.screen = sDeleteConfirm
		}
	case "/":
		m.filtering = true
		m.filterInput.SetValue(m.filter)
		m.filterInput.Focus()
		return m, textinput.Blink
	case "esc":
		m.filter = ""
		m.filterInput.SetValue("")
		m.cursor, m.scroll = 0, 0
	}
	m.ensureVisible()
	return m, nil
}

func (m *Model) ensureVisible() {
	page := m.pageSize()
	if m.cursor < m.scroll {
		m.scroll = m.cursor
	}
	if m.cursor >= m.scroll+page {
		m.scroll = m.cursor - page + 1
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

func (m Model) pageSize() int {
	p := m.height - 16
	if p < 5 {
		p = 10
	}
	return p
}

func visibleList(m Model) []store.Instance {
	if m.filter == "" {
		return m.list
	}
	var out []store.Instance
	for _, v := range m.list {
		if strings.Contains(strings.ToLower(v.Name+" "+v.Type+" "+v.ContainerName), strings.ToLower(m.filter)) {
			out = append(out, v)
		}
	}
	return out
}

// ---------- detail: Tab returns to list; h/l switch tabs ----------

func selectedInstance(m Model) (store.Instance, bool) {
	rows := visibleList(m)
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return store.Instance{}, false
	}
	return rows[m.cursor], true
}

func (m Model) updateDetail(k string) (tea.Model, tea.Cmd) {
	inst, ok := selectedInstance(m)
	if !ok {
		m.screen = sList
		return m, nil
	}
	_, a, _ := adapters.Resolve(inst.Type)
	switch k {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc", "tab":
		m.screen = sList
		m.setMsg("", false)
		return m, nil
	case "1":
		m.tab = tabInfo
	case "2":
		m.tab = tabEnv
	case "3":
		m.tab = tabURLs
	case "h", "left":
		m.tab = (m.tab + 2) % 3
	case "l", "right":
		m.tab = (m.tab + 1) % 3
	case "j", "down":
		m.tab = (m.tab + 1) % 3
	case "k", "up":
		m.tab = (m.tab + 2) % 3
	case "p":
		m.showPw = !m.showPw
		if m.showPw {
			m.setMsg("password revealed — p to hide again", false)
		} else {
			m.setMsg("", false)
		}
	case "y", "c":
		var s string
		if m.tab == tabURLs {
			var b strings.Builder
			for key, v := range a.ConnectionURLs(inst) {
				b.WriteString(key + "=" + v + "\n")
			}
			s = strings.TrimRight(b.String(), "\n")
		} else {
			s = a.EnvBlock(inst)
		}
		_ = util.CopyToClipboard(s)
		m.setMsg("copied to clipboard", false)
	case "Y":
		_ = util.CopyToClipboard(inst.Password)
		m.setMsg("password copied to clipboard", false)
	case "E":
		// copyable connect command — paste into any terminal
		_ = util.CopyToClipboard(connectCmdString(inst, a))
		m.setMsg("connect command copied — paste in any terminal", false)
	case "s":
		st := m.status[inst.ContainerName]
		if st.Running {
			_ = m.dock.Stop(inst.ContainerName)
		} else {
			_ = m.dock.Start(inst.ContainerName)
		}
		m.reload()
	case "R":
		_ = m.dock.Restart(inst.ContainerName)
		m.reload()
	case "r":
		m.reload()
		m.setMsg("refreshed", false)
	case "enter", "o":
		// full interactive DB console in a real terminal window;
		// `exit` returns here. `E` copies this exact command.
		if msg := m.requireRunningTUI(inst); msg != "" {
			m.setMsg(msg, true)
			return m, nil
		}
		cmd := a.ConnectCmd(inst)
		args := append([]string{"exec", "-it", inst.ContainerName}, cmd...)
		c := exec.Command("docker", args...)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err} })
	case "x":
		if msg := m.requireRunningTUI(inst); msg != "" {
			m.setMsg(msg, true)
			return m, nil
		}
		args := append([]string{"exec", "-it", inst.ContainerName}, a.ContainerShellCmd()...)
		c := exec.Command("docker", args...)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err} })
	case "L":
		// full follow-logs view; ctrl+c returns (exited containers refuse with a message)
		if msg := m.requireRunningTUI(inst); msg != "" {
			m.setMsg(msg, true)
			return m, nil
		}
		c := exec.Command("docker", "logs", "--tail", "200", "-f", inst.ContainerName)
		return m, tea.ExecProcess(c, func(err error) tea.Msg { return execDoneMsg{err} })
	}
	return m, nil
}

// requireRunningTUI refreshes status and returns "" when the container is
// live, otherwise a human message (no terminal handover, no flash).
func (m *Model) requireRunningTUI(inst store.Instance) string {
	st := m.dock.ContainerStatus(inst.ContainerName)
	m.status[inst.ContainerName] = st
	if st.Running {
		return ""
	}
	if st.State == "" || st.State == "notfound" {
		return fmt.Sprintf("%s container %s not found — removed outside dbman?", inst.Name, inst.ContainerName)
	}
	return fmt.Sprintf("%s is not running (%s) — press s to start", inst.Name, st.State)
}

// connectCmdString is the exact `docker exec` connect command, for copying.
func connectCmdString(inst store.Instance, a adapters.Adapter) string {
	parts := append([]string{"docker", "exec", "-it", inst.ContainerName}, a.ConnectCmd(inst)...)
	return shellQuoteJoin(parts)
}

func shellQuoteJoin(parts []string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if strings.ContainsAny(p, " \t\"'") {
			out = append(out, "'"+strings.ReplaceAll(p, "'", "'\\''")+"'")
		} else {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func (m *Model) focusInputs() {
	for i := range m.inputs {
		if i == m.focusIdx {
			m.inputs[i].Focus()
		} else {
			m.inputs[i].Blur()
		}
	}
}

// currentCanon maps the selected type row to its canonical adapter key.
func (m Model) currentCanon() string {
	canon, _, err := adapters.Resolve(m.types[m.typeIdx])
	if err != nil {
		return m.types[m.typeIdx]
	}
	return canon
}

// imageOptions are the dropdown choices for the image field (from config).
func (m Model) imageOptions() []string {
	if d, ok := m.cfg.Defaults[m.currentCanon()]; ok {
		return d.Options
	}
	return nil
}

// autofillImage resets the image field to the config default for the type.
func (m *Model) autofillImage() {
	if d, ok := m.cfg.Defaults[m.currentCanon()]; ok && d.Image != "" {
		m.inputs[1].SetValue(d.Image)
	}
}

// cycleImage steps through the config image options; free text stays valid.
func (m *Model) cycleImage(dir int) {
	opts := m.imageOptions()
	if len(opts) == 0 {
		return
	}
	cur := strings.TrimSpace(m.inputs[1].Value())
	idx := -1
	for i, o := range opts {
		if o == cur {
			idx = i
			break
		}
	}
	if idx == -1 {
		if dir > 0 {
			idx = 0
		} else {
			idx = len(opts) - 1
		}
	} else {
		idx = (idx + dir + len(opts)) % len(opts)
	}
	m.inputs[1].SetValue(opts[idx])
}

func (m Model) updateCreate(k string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.creating {
		// pull/run in flight — keep the spinner alive, ignore form keys
		return m, nil
	}
	switch k {
	case "esc":
		m.screen = sList
		m.setMsg("", false)
		return m, nil
	case "tab", "down":
		m.focusIdx = (m.focusIdx + 1) % (len(m.inputs) + 1)
		m.focusInputs()
		return m, nil
	case "shift+tab", "up":
		m.focusIdx = (m.focusIdx - 1 + len(m.inputs) + 1) % (len(m.inputs) + 1)
		m.focusInputs()
		return m, nil
	case "ctrl+j":
		m.focusIdx = (m.focusIdx + 1) % (len(m.inputs) + 1)
		m.focusInputs()
		return m, nil
	case "ctrl+k":
		m.focusIdx = (m.focusIdx - 1 + len(m.inputs) + 1) % (len(m.inputs) + 1)
		m.focusInputs()
		return m, nil
	case "left", "right":
		if m.focusIdx == len(m.inputs) {
			if k == "left" {
				m.typeIdx = (m.typeIdx - 1 + len(m.types)) % len(m.types)
			} else {
				m.typeIdx = (m.typeIdx + 1) % len(m.types)
			}
			m.autofillImage()
			return m, nil
		}
		if m.focusIdx == 1 {
			// image dropdown: cycle config options, free text still allowed
			if k == "left" {
				m.cycleImage(-1)
			} else {
				m.cycleImage(1)
			}
			return m, nil
		}
	case "enter":
		if m.focusIdx == len(m.inputs) {
			m.typeIdx = (m.typeIdx + 1) % len(m.types)
			m.autofillImage()
			return m, nil
		}
		return m.startCreate()
	}
	if m.focusIdx < len(m.inputs) {
		var c tea.Cmd
		m.inputs[m.focusIdx], c = m.inputs[m.focusIdx].Update(msg)
		return m, c
	}
	return m, nil
}

// buildInstance validates the form without touching Docker.
func (m *Model) buildInstance() (store.Instance, docker.Spec, error) {
	fail := func(err error) (store.Instance, docker.Spec, error) {
		return store.Instance{}, docker.Spec{}, err
	}
	name := strings.TrimSpace(m.inputs[0].Value())
	image := strings.TrimSpace(m.inputs[1].Value())
	user := strings.TrimSpace(m.inputs[2].Value())
	password := m.inputs[3].Value()
	db := strings.TrimSpace(m.inputs[4].Value())
	portStr := strings.TrimSpace(m.inputs[5].Value())
	typ := m.types[m.typeIdx]
	if name == "" {
		return fail(fmt.Errorf("name is required"))
	}
	canon, a, err := adapters.Resolve(typ)
	if err != nil {
		return fail(err)
	}
	if store.Find(m.list, name) != nil {
		return fail(fmt.Errorf("instance %q already exists", name))
	}
	if image == "" {
		if d, ok := m.cfg.Defaults[canon]; ok {
			image = d.Image
		}
	}
	if image == "" {
		return fail(fmt.Errorf("image is required (no default for %s)", canon))
	}
	if user == "" {
		user = a.DefaultUser(m.cfg)
	}
	if db == "" {
		db = a.DefaultDB(m.cfg)
	}
	if password == "" && canon != "redis" {
		password = util.GeneratePassword(16)
	}
	port := 0
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return fail(fmt.Errorf("bad port %q", portStr))
		}
		port = p
	}
	if port != 0 {
		if !util.FreePort(port) || m.dock.PortInUseBy(port) != "" {
			return fail(fmt.Errorf("port %d in use; pick another", port))
		}
	} else if m.cfg.PortStrategy == "standard_first" {
		if util.FreePort(a.DefaultPort()) && m.dock.PortInUseBy(a.DefaultPort()) == "" {
			port = a.DefaultPort()
		} else {
			port = util.FindFreePort(m.cfg.PortRangeStart)
		}
	} else {
		port = util.FindFreePort(m.cfg.PortRangeStart)
	}
	inst := store.Instance{
		Name:          name,
		Type:          canon,
		ContainerName: "dbman-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")),
		Image:         image,
		Host:          "127.0.0.1",
		Port:          port,
		User:          user,
		Password:      password,
		Database:      db,
	}
	return inst, a.ContainerSpec(m.cfg, inst), nil
}

// startCreate validates, then pulls (with spinner) + runs without freezing.
func (m Model) startCreate() (tea.Model, tea.Cmd) {
	inst, spec, err := m.buildInstance()
	if err != nil {
		m.setMsg("error: "+err.Error(), true)
		return m, nil
	}
	m.creating = true
	m.setMsg("", false)
	dock := m.dock
	if !dock.ImageExists(inst.Image) {
		m.createStage = "pulling " + inst.Image
	} else {
		m.createStage = "creating " + inst.ContainerName
	}
	createCmd := func() tea.Msg {
		pulled := false
		if !dock.ImageExists(inst.Image) {
			if err := dock.Pull(inst.Image); err != nil {
				return createDoneMsg{err: fmt.Errorf("pull %s: %w", inst.Image, err)}
			}
			pulled = true
		}
		if err := dock.Run(spec); err != nil {
			return createDoneMsg{err: err}
		}
		list, _, lerr := store.Load()
		if lerr != nil {
			return createDoneMsg{err: lerr}
		}
		list = store.Upsert(dedupe(list), inst)
		if serr := store.Save(list); serr != nil {
			return createDoneMsg{err: serr}
		}
		return createDoneMsg{pulled: pulled}
	}
	return m, tea.Batch(m.createSpin.Tick, createCmd)
}

func (m Model) updateDeleteConfirm(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "esc", "n", "q":
		m.screen = sList
	case "y", "enter":
		inst := store.Find(m.list, m.delTarget)
		if inst != nil {
			_ = m.dock.Remove(inst.ContainerName, !m.keepVol)
			m.list = store.Remove(m.list, inst.Name)
			_ = store.Save(m.list)
		}
		m.screen = sList
		m.reload()
	case " ":
		m.keepVol = !m.keepVol
	}
	return m, nil
}

// ---------- view ----------

func (m Model) View() string {
	w, h := m.width, m.height
	if w < 60 {
		w = 110
	}
	if h < 12 {
		h = 30
	}
	t := m.th()

	header := m.renderHeader(w, t)
	footer := m.renderFooterSingleLine(w, t)

	bodyH := h - 5 // top pad(1) + header(1) + msg(1) + footer(1) + gaps
	if bodyH < 8 {
		bodyH = 8
	}

	var body string
	switch m.screen {
	case sCreate:
		base := m.renderMainSplit(w, bodyH, t, false)
		body = m.centerModal(base, m.renderCreateModal(t), w, bodyH)
	case sDeleteConfirm:
		base := m.renderMainSplit(w, bodyH, t, false)
		body = m.centerModal(base, m.renderDeleteModal(t), w, bodyH)
	case sDetail:
		body = m.renderMainSplit(w, bodyH, t, true)
	default:
		body = m.renderMainSplit(w, bodyH, t, false)
	}

	msgLine := " "
	if m.msg != "" {
		st := lipgloss.NewStyle().Foreground(t.warning)
		if m.msgIsErr {
			st = lipgloss.NewStyle().Foreground(t.err).Bold(true)
		}
		msgLine = " " + st.Render("▸ "+truncateSingle(m.msg, w-4))
	}
	if m.filtering {
		msgLine = " " + lipgloss.NewStyle().Foreground(t.accent).Bold(true).Render("/"+m.filterInput.View())
	}

	out := lipgloss.JoinVertical(lipgloss.Left,
		"", // top padding: brand starts below, aligned with panes
		header,
		body,
		msgLine,
		footer,
	)
	if m.showHelp {
		out = m.centerModal(out, m.renderHelpModal(t), w, h)
	}
	if m.creating {
		out = m.centerModal(out, m.renderCreatingModal(t), w, h)
	}
	return out
}

func (m Model) renderCreatingModal(t theme) string {
	var b strings.Builder
	b.WriteString(m.createSpin.View() + "  " +
		lipgloss.NewStyle().Bold(true).Render(m.createStage) + "\n\n")
	if strings.HasPrefix(m.createStage, "pulling") {
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).
			Render("Image is not local — downloading.\nFirst run may take a while.\nThe UI stays responsive.") + "\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("Starting container…") + "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(t.accent).
		Padding(1, 2).
		Width(52).
		Render(b.String())
}

func (m Model) renderHeader(w int, t theme) string {
	// Brand row starts at the same x as the panes below (one space inset).
	brand := lipgloss.NewStyle().Bold(true).Foreground(t.accent).Render(" dbman") +
		lipgloss.NewStyle().Foreground(t.muted).Render("  ·  docker databases  ·  ") +
		lipgloss.NewStyle().Foreground(t.muted).Render(fmt.Sprintf("%d", len(visibleList(m))))
	right := lipgloss.NewStyle().Foreground(t.muted).Render("? all keys ")
	if m.filter != "" {
		right = lipgloss.NewStyle().Foreground(t.warning).Render("/"+m.filter+" ") +
			lipgloss.NewStyle().Foreground(t.muted).Render("? all keys ")
	}
	gap := w - lipgloss.Width(brand) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	line := brand + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(t.muted).
		Width(w).
		Render(line)
}

// One single footer line, fitted — never wraps. Always advertises ? .
func (m Model) renderFooterSingleLine(w int, t theme) string {
	k := func(s string) string { return lipgloss.NewStyle().Bold(true).Foreground(t.accent).Render(s) }
	d := func(s string) string { return lipgloss.NewStyle().Foreground(t.muted).Render(s) }
	type pair struct{ key, desc string }
	var pairs []pair
	switch m.screen {
	case sList:
		pairs = []pair{
			{"j/k", "move"}, {"enter", "open"}, {"tab", "preview"}, {"n", "new"},
			{"s", "start/stop"}, {"D", "del"}, {"/", "filter"}, {"?", "all keys"}, {"q", "quit"},
		}
	case sDetail:
		pairs = []pair{
			{"enter", "connect"}, {"E", "copy cmd"}, {"x", "shell"}, {"p", "reveal"},
			{"y", "copy"}, {"Y", "password"}, {"tab", "list"}, {"?", "all keys"},
		}
	case sCreate:
		pairs = []pair{
			{"tab", "next"}, {"←/→", "type"}, {"enter", "save"},
			{"esc", "cancel"}, {"?", "all keys"},
		}
	default:
		pairs = []pair{{"y", "confirm"}, {"space", "keep vol"}, {"esc", "cancel"}}
	}
	plain := func(ps []pair) int {
		n := 1
		for i, p := range ps {
			n += len(p.key) + 1 + len(p.desc)
			if i < len(ps)-1 {
				n += 5
			}
		}
		return n + 1
	}
	for len(pairs) > 4 && plain(pairs) > w-4 {
		mid := len(pairs) / 2
		pairs = append(pairs[:mid], pairs[mid+1:]...)
	}
	segs := make([]string, 0, len(pairs))
	for _, p := range pairs {
		segs = append(segs, k(p.key)+d(" "+p.desc))
	}
	line := " " + strings.Join(segs, d("  ·  ")) + " "
	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), true, false, false, false).
		BorderForeground(t.muted).
		Width(w).
		MaxWidth(w).
		Render(line)
}

func (m Model) renderMainSplit(w, bodyH int, t theme, detailFocus bool) string {
	// Left pane is wider now (~40%).
	leftW := w * 40 / 100
	if leftW < 34 {
		leftW = 34
	}
	if leftW > 48 {
		leftW = 48
	}
	rightW := w - leftW - 1
	if rightW < 30 {
		rightW = 30
	}
	leftFocused := !detailFocus && m.screen == sList && !m.filtering
	rightFocused := detailFocus || m.screen != sList

	// Active pane gets a THICK border + accent; idle pane thin + muted.
	leftBorder, leftFG := lipgloss.RoundedBorder(), t.muted
	rightBorder, rightFG := lipgloss.RoundedBorder(), t.muted
	if leftFocused {
		leftBorder, leftFG = lipgloss.ThickBorder(), t.accent
	}
	if rightFocused {
		rightBorder, rightFG = lipgloss.ThickBorder(), t.accent
	}

	leftInner := m.renderListPane(leftW-4, bodyH-2, t)
	rightInner := m.renderDetailPane(rightW-4, bodyH-2, t)

	leftBox := lipgloss.NewStyle().
		Border(leftBorder).
		BorderForeground(leftFG).
		Padding(0, 1).Width(leftW).Height(bodyH).
		Render(leftInner)
	rightBox := lipgloss.NewStyle().
		Border(rightBorder).
		BorderForeground(rightFG).
		Padding(0, 1).Width(rightW).Height(bodyH).
		Render(rightInner)

	return lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)
}

func statusDot(running bool, t theme) string {
	if running {
		return lipgloss.NewStyle().Foreground(t.success).Render("●")
	}
	return lipgloss.NewStyle().Foreground(t.muted).Render("○")
}

func typePill(typ string) string {
	return lipgloss.NewStyle().Foreground(typeColor(typ)).Bold(true).Render("[" + typ + "]")
}

// Left pane: full name + type pill visible immediately, no guessing.
func (m Model) renderListPane(innerW, innerH int, t theme) string {
	var b strings.Builder
	title := lipgloss.NewStyle().Bold(true).Foreground(t.accent).Render("❯ databases") +
		" " + lipgloss.NewStyle().Foreground(t.muted).Render(fmt.Sprintf("(%d)", len(visibleList(m))))
	b.WriteString(title + "\n")
	if m.filtering {
		b.WriteString(lipgloss.NewStyle().Foreground(t.accent).Render("/"+m.filterInput.View()) + "\n")
	} else if m.filter != "" {
		b.WriteString(lipgloss.NewStyle().Foreground(t.warning).Render("/"+m.filter) +
			lipgloss.NewStyle().Foreground(t.muted).Render("  ·  esc clears") + "\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("/ filter") + "\n")
	}
	b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render(strings.Repeat("─", max(innerW, 8))) + "\n")

	rows := visibleList(m)
	if len(rows) == 0 {
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("No matches.") + "\n")
		return clipHeight(b.String(), innerH)
	}
	page := max(innerH-4, 5)
	start := m.scroll
	if start < 0 {
		start = 0
	}
	end := min(start+page, len(rows))
	selStyle := lipgloss.NewStyle().Bold(true).Foreground(t.accent)
	for i := start; i < end; i++ {
		inst := rows[i]
		st := m.status[inst.ContainerName]
		dot := statusDot(st.Running, t)
		icon := lipgloss.NewStyle().Foreground(typeColor(inst.Type)).Render(typeIcon(inst.Type, m.cfg.Icons))
		pill := typePill(inst.Type)
		// name gets all remaining space; pill + port are never cut for space reasons
		reserved := 2 + 2 + lipgloss.Width(pill) + 1 + 7 + 1 // cursor + dot + pill + port(~:65535)
		nameW := innerW - reserved
		if nameW < 8 {
			nameW = 8
		}
		name := truncatePlain(inst.Name, nameW)
		port := lipgloss.NewStyle().Foreground(t.muted).Render(fmt.Sprintf(":%d", inst.Port))
		// NOTE: no truncation of the composed line — it fits by construction.
		// Truncating styled text by bytes splits ANSI escapes and garbles rows.
		line := dot + " " + icon + padRight(name, nameW) + " " + pill + " " + port
		if i == m.cursor {
			b.WriteString(selStyle.Render("▸ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}
	if len(rows) > page {
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render(
			fmt.Sprintf("… %d/%d · j/k scroll", m.cursor+1, len(rows))) + "\n")
	}
	return clipHeight(b.String(), innerH)
}

func (m Model) tabBar(t theme) string {
	names := []string{"Info", "Env", "URLs"}
	var parts []string
	for i, n := range names {
		label := fmt.Sprintf(" %d %s ", i+1, n)
		if i == m.tab {
			// active tab: accent + underline + breathing room, never glued to the marker
			parts = append(parts, lipgloss.NewStyle().
				Bold(true).Underline(true).Foreground(m.th().accent).Render(label))
		} else {
			parts = append(parts, lipgloss.NewStyle().Foreground(t.muted).Render(label))
		}
	}
	return strings.Join(parts, lipgloss.NewStyle().Foreground(t.muted).Render("│"))
}

func (m Model) renderDetailPane(innerW, innerH int, t theme) string {
	var b strings.Builder
	inst, ok := selectedInstance(m)
	if !ok {
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("No database selected.") + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("Press n to create one.") + "\n")
		return clipHeight(b.String(), innerH)
	}
	_, a, _ := adapters.Resolve(inst.Type)
	st := m.status[inst.ContainerName]

	icon := lipgloss.NewStyle().Foreground(typeColor(inst.Type)).Render(typeIcon(inst.Type, m.cfg.Icons))
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("❯ " + icon + truncate(inst.Name, 28)))
	if st.Running {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(t.success).Render("● "+st.State) + "\n")
	} else {
		b.WriteString("  " + lipgloss.NewStyle().Foreground(t.muted).Render("○ "+st.State) + "\n")
	}
	b.WriteString("\n")
	b.WriteString(m.tabBar(t) + "  " + lipgloss.NewStyle().Foreground(t.muted).Render("· h/l switch") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render(strings.Repeat("─", max(innerW, 8))) + "\n")

	labStyle := lipgloss.NewStyle().Foreground(t.muted)
	valStyle := lipgloss.NewStyle().Bold(true)
	kv := func(k, v string, vs lipgloss.Style) {
		b.WriteString(labStyle.Render(padRight(k, 10)) + vs.Render(truncateSingle(v, innerW-12)) + "\n")
	}
	pw := maskPw(inst.Password, m.showPw)
	pwStyle := lipgloss.NewStyle().Foreground(t.warning).Bold(true)
	if m.showPw {
		pwStyle = lipgloss.NewStyle().Foreground(t.success).Bold(true)
	}
	if inst.Password != "" && !m.showPw {
		pw += "  " + lipgloss.NewStyle().Foreground(t.muted).Render("· p reveal · Y copy password")
	}
	switch m.tab {
	case tabInfo:
		kv("container", inst.ContainerName, valStyle)
		kv("image", inst.Image, lipgloss.NewStyle().Foreground(t.accent))
		kv("host", inst.Host, valStyle)
		kv("port", fmt.Sprintf("%d", inst.Port), lipgloss.NewStyle().Foreground(t.accent).Bold(true))
		kv("user", orDash(inst.User), lipgloss.NewStyle().Foreground(t.success).Bold(true))
		// password carries its own hint suffix; render untruncated-ish
		b.WriteString(labStyle.Render(padRight("password", 10)) + pwStyle.Render(pw) + "\n")
		kv("db", orDash(inst.Database), lipgloss.NewStyle().Foreground(t.success).Bold(true))
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(t.muted).Render("enter connect · E copy cmd · x shell · L logs") + "\n")
	case tabEnv:
		block := a.EnvBlock(inst)
		if !m.showPw && inst.Password != "" {
			block = strings.ReplaceAll(block, inst.Password, "********")
		}
		code := lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, true, true, true).
			BorderForeground(t.accent).Padding(0, 1).Width(min(innerW-2, 64))
		b.WriteString(code.Render(colorizeEnv(clipLines(block, innerH-9), t)) + "\n")
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("y copy env · p reveal · Y copy password") + "\n")
	case tabURLs:
		for k, v := range a.ConnectionURLs(inst) {
			if !m.showPw && inst.Password != "" {
				v = strings.ReplaceAll(v, inst.Password, "********")
			}
			b.WriteString(lipgloss.NewStyle().Foreground(t.accent).Bold(true).Render(k+" =") + "\n")
			b.WriteString("  " + truncateSingle(v, innerW-4) + "\n\n")
		}
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("y copy urls · p reveal · Y copy password") + "\n")
	}
	return clipHeight(b.String(), innerH)
}

// colorizeEnv renders KEY= lines with the key in accent, value default.
func colorizeEnv(block string, t theme) string {
	lines := strings.Split(block, "\n")
	for i, ln := range lines {
		if idx := strings.Index(ln, "="); idx > 0 {
			lines[i] = lipgloss.NewStyle().Foreground(t.accent).Bold(true).Render(ln[:idx]+"=") +
				ln[idx+1:]
		}
	}
	return strings.Join(lines, "\n")
}

// ---------- modals ----------

func (m Model) renderHelpModal(t theme) string {
	secStyle := lipgloss.NewStyle().Bold(true)
	row := func(key, desc string) string {
		return lipgloss.NewStyle().Bold(true).Foreground(t.accent).Render(padRight(key, 11)) +
			lipgloss.NewStyle().Foreground(t.muted).Render(desc)
	}
	sec := func(s string, rows ...string) string {
		out := secStyle.Render(s) + "\n"
		for _, r := range rows {
			out += r + "\n"
		}
		return out
	}
	left := sec("Panes / vim",
		row("tab", "list ↔ preview"),
		row("enter", "open preview"),
		row("esc", "back to list"),
		row("j/k", "move down/up"),
		row("g/G", "first / last"),
		row("ctrl+d/u", "half page"),
		row("h/l · 1–3", "preview tabs"),
	) + "\n" + sec("List",
		row("n", "new database"),
		row("s", "start / stop"),
		row("R", "restart"),
		row("D", "delete"),
		row("/", "filter"),
		row("r", "refresh"),
	)
	right := sec("Preview (pw masked)",
		row("enter", "full console"),
		row("E", "copy connect cmd"),
		row("x", "container shell"),
		row("L", "follow logs"),
		row("p", "reveal password"),
		row("y", "copy env/URLs"),
		row("Y", "copy password"),
	) + "\n" + sec("Create",
		row("tab", "next field"),
		row("←/→", "type + image pick"),
		row("enter", "save"),
		row("esc", "cancel"),
	) + "\n" + sec("Global",
		row("?", "this help"),
		row("q", "quit"),
	)
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, "   ", right)
	content := lipgloss.NewStyle().Bold(true).Foreground(t.accent).Render("?  keybindings") + "\n" +
		strings.Repeat("─", 52) + "\n" + body + "\n" +
		lipgloss.NewStyle().Foreground(t.muted).Render("? / esc closes")
	w := lipgloss.Width(content) + 6
	if w > m.width-4 {
		w = m.width - 4
	}
	if w < 40 {
		w = 40
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(t.accent).
		Padding(1, 2).
		Width(w).
		Render(content)
}

func (m Model) renderCreateModal(t theme) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(t.accent).Render("＋ New database") + "\n")
	b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("tab move · enter save · esc cancel") + "\n")
	b.WriteString(strings.Repeat("─", 56) + "\n")

	focused := m.focusIdx == len(m.inputs)
	markType := "  "
	labType := lipgloss.NewStyle().Foreground(t.muted)
	if focused {
		markType = lipgloss.NewStyle().Foreground(t.accent).Bold(true).Render("▸ ")
		labType = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
	}
	typeLine := markType + labType.Render("type:     ")
	for i, typ := range m.types {
		if i == m.typeIdx {
			// selected option reads as a chip, not a full-line wash
			typeLine += lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1A1A2E")).
				Background(t.accent).Render(" "+typ+" ") + " "
		} else {
			typeLine += lipgloss.NewStyle().Foreground(t.muted).Render(typ + " ")
		}
	}
	typeLine += lipgloss.NewStyle().Foreground(t.muted).Faint(true).Render(" (←/→)")
	b.WriteString(typeLine + "\n")
	b.WriteString("\n")

	labels := []string{"name*", "image", "user", "password", "db", "port"}
	hints := []string{
		"friendly name, e.g. myproj-pg",
		"←/→ picks from config, or type custom",
		"empty = adapter default",
		"empty = auto-generate (redis: none)",
		"empty = adapter default",
		"empty = auto-assign",
	}
	for i, inp := range m.inputs {
		isFocus := i == m.focusIdx
		mark := "  "
		if isFocus {
			mark = lipgloss.NewStyle().Foreground(t.accent).Bold(true).Render("▸ ")
		}
		lab := lipgloss.NewStyle().Foreground(t.muted)
		if isFocus {
			lab = lipgloss.NewStyle().Foreground(t.accent).Bold(true)
		}
		b.WriteString(mark + lab.Render(padRight(labels[i], 9)) + inp.View() + "\n")
		if i == 1 {
			// image dropdown: one option per line, always indented —
			// wrapped chips used to restart at column 0
			opts := m.imageOptions()
			cur := strings.TrimSpace(inp.Value())
			for _, o := range opts {
				if o == cur {
					b.WriteString("            " + lipgloss.NewStyle().Bold(true).
						Foreground(t.accent).Render("● "+o) + "\n")
				} else {
					b.WriteString("            " + lipgloss.NewStyle().Foreground(t.muted).Render("○ "+o) + "\n")
				}
			}
		}
		b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Faint(true).
			Render("            "+hints[i]) + "\n")
	}
	if m.msg != "" {
		b.WriteString("\n" + lipgloss.NewStyle().Foreground(t.err).Bold(true).Render(truncateSingle(m.msg, 56)) + "\n")
	}
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(t.accent).
		Padding(1, 2).
		Width(70).
		Render(b.String())
}

func (m Model) renderDeleteModal(t theme) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(t.err).Render("Delete "+truncate(m.delTarget, 28)+"?") + "\n\n")
	b.WriteString("Container + record removed.\nVolume removed unless kept.\n\n")
	chk := "[ ]"
	if m.keepVol {
		chk = "[x]"
	}
	b.WriteString(fmt.Sprintf("%s keep volume  (space)\n\n", chk))
	b.WriteString(lipgloss.NewStyle().Foreground(t.muted).Render("y confirm · n cancel"))
	return lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(t.err).
		Padding(1, 2).
		Width(46).
		Render(b.String())
}

func (m Model) centerModal(base, modal string, w, h int) string {
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal,
		lipgloss.WithWhitespaceChars(" "), lipgloss.WithWhitespaceForeground(lipgloss.Color("#333")))
}

// ---------- helpers ----------

func maskPw(s string, show bool) string {
	if show {
		return s
	}
	if s == "" {
		return "(empty)"
	}
	return "********"
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func truncate(s string, n int) string {
	return truncatePlain(s, n)
}

// truncatePlain is rune-aware (byte slicing breaks multibyte names).
func truncatePlain(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}

func truncateSingle(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return truncate(s, n)
}

func clipLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}

func clipHeight(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[:n-1]
		lines = append(lines, "…")
	}
	return strings.Join(lines, "\n")
}

func padRight(s string, n int) string {
	w := lipgloss.Width(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func Run() error {
	p := tea.NewProgram(InitialModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
