package tui

import (
	"strings"
	"testing"

	"github.com/muizzyranking/dbman/internal/adapters"
	"github.com/muizzyranking/dbman/internal/docker"
	"github.com/muizzyranking/dbman/internal/store"
)

func demoModel() Model {
	m := InitialModel()
	m.width, m.height = 110, 30
	m.list = []store.Instance{
		{Name: "demo-pg", Type: "postgres", ContainerName: "dbman-demo-pg", Image: "postgres:16-alpine", Host: "127.0.0.1", Port: 5432, User: "postgres", Password: "secret", Database: "postgres"},
	}
	m.status = map[string]docker.Status{
		"dbman-demo-pg": {Running: true, State: "running"},
	}
	m.cursor, m.scroll = 0, 0
	return m
}

func TestWindowedRender(t *testing.T) {
	m := demoModel()
	out := m.View()
	// two panes + header + single-line footer
	for _, want := range []string{"╭", "╮", "╰", "╯", "dbman", "databases", "preview", "?"} {
		if !strings.Contains(out, want) {
			t.Fatalf("list View() missing %q\n%s", want, out)
		}
	}
	// list row shows full name + type pill
	if !strings.Contains(out, "demo-pg") || !strings.Contains(out, "[postgres]") {
		t.Fatalf("list row must show name + type pill\n%s", out)
	}
	// footer must be a single line (never wraps)
	lines := strings.Split(out, "\n")
	footerHits := 0
	for _, ln := range lines {
		if strings.Contains(ln, "quit") {
			footerHits++
		}
	}
	if footerHits != 1 {
		t.Fatalf("footer should be exactly one line containing quit, got %d\n%s", footerHits, out)
	}

	m.screen = sDetail
	out = m.View()
	for _, want := range []string{"╭", "Info", "Env", "URLs", "connect", "x shell", "********"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail View() missing %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret") {
		t.Fatalf("password must be masked by default\n%s", out)
	}

	m.showPw = true
	if out := m.View(); !strings.Contains(out, "secret") {
		t.Fatalf("revealed view should contain password\n%s", out)
	}
	m.showPw = false
	m.showHelp = true
	out = m.View()
	for _, want := range []string{"keybindings", "tab", "copy connect", "reveal", "follow logs"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help modal missing %q\n%s", want, out)
		}
	}
	m.showHelp = false
}

func TestConnectGuardStopped(t *testing.T) {
	m := demoModel()
	m.screen = sDetail
	// demo container does not exist in docker -> guard must refuse, not exec
	got, _ := m.updateDetail("enter")
	mm := got.(Model)
	if mm.msg == "" || (!strings.Contains(mm.msg, "not running") && !strings.Contains(mm.msg, "not found")) {
		t.Fatalf("expected stopped-container message, got %q", mm.msg)
	}
	if mm.screen != sDetail {
		t.Fatalf("should stay on detail screen")
	}
}

func TestConnectCmdString(t *testing.T) {
	m := demoModel()
	inst := m.list[0]
	_, a, err := adapters.Resolve(inst.Type)
	if err != nil {
		t.Fatal(err)
	}
	got := connectCmdString(inst, a)
	want := "docker exec -it dbman-demo-pg psql -U postgres -d postgres"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDedupe(t *testing.T) {
	in := []store.Instance{{Name: "a"}, {Name: "a"}, {Name: "b"}}
	out := dedupe(in)
	if len(out) != 2 || out[0].Name != "a" || out[1].Name != "b" {
		t.Fatalf("dedupe failed: %+v", out)
	}
}

func TestImageCycle(t *testing.T) {
	m := demoModel()
	// find postgres type index
	for i, typ := range m.types {
		if typ == "postgres" {
			m.typeIdx = i
		}
	}
	m.inputs[1].SetValue("custom:1")
	m.cycleImage(1)
	if got := m.inputs[1].Value(); got != "postgres:16-alpine" {
		t.Fatalf("cycle from custom should go to first option, got %q", got)
	}
	m.cycleImage(1)
	if got := m.inputs[1].Value(); got != "pgvector/pgvector:pg16" {
		t.Fatalf("cycle should advance, got %q", got)
	}
	m.cycleImage(-1)
	if got := m.inputs[1].Value(); got != "postgres:16-alpine" {
		t.Fatalf("cycle back should return, got %q", got)
	}
}

func TestListRowsShowNames(t *testing.T) {
	m := demoModel()
	m.list = append(m.list, store.Instance{
		Name: "averylongdatabasenamethatshouldnotvanish", Type: "redis",
		ContainerName: "dbman-x", Image: "redis:7-alpine", Host: "127.0.0.1", Port: 6379,
	})
	m.screen = sList
	out := m.View()
	// every instance name must appear exactly as the row subject (no ANSI garbage)
	if !strings.Contains(out, "demo-pg") {
		t.Fatalf("list missing demo-pg\n%s", out)
	}
	if !strings.Contains(out, "averylongdataba") {
		t.Fatalf("long name prefix vanished from list\n%s", out)
	}
	if !strings.Contains(out, "[redis]") || !strings.Contains(out, "[postgres]") {
		t.Fatalf("type pills missing\n%s", out)
	}
	// raw ANSI fragments must never leak (broken escape = garbled rows)
	if strings.Contains(out, "[38;5;") && strings.Contains(out, "…m") {
		t.Fatalf("broken ANSI escape in output\n%s", out)
	}
}
