package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/knightmare2600/vogter/internal/model"
	"github.com/knightmare2600/vogter/internal/poller"
	"github.com/knightmare2600/vogter/internal/zabbix"
)

func testBoards() []model.Board {
	return []model.Board{
		{ID: "1", Name: "Global view", Pages: []model.BoardPage{
			{Widgets: []model.Widget{
				{Type: "problems", Name: "Current problems", X: 0, Y: 0, W: 48, H: 5},
				{Type: "clock", Name: "Local time", X: 48, Y: 0, W: 24, H: 5},
				{Type: "problemsbysv", X: 0, Y: 5, W: 24, H: 3, Fields: []zabbix.WidgetField{{Name: "hostids.0", Value: "2"}}},
				{Type: "geomap", X: 24, Y: 5, W: 24, H: 3},
				{Type: "hostavail", X: 48, Y: 5, W: 24, H: 3},
				{Type: "item", X: 0, Y: 8, W: 12, H: 4, Fields: []zabbix.WidgetField{{Name: "description", Value: "{HOST.NAME}\nrate"}}},
				{Type: "tophosts", X: 12, Y: 8, W: 30, H: 4},
				{Type: "svggraph", Name: "Load", X: 42, Y: 8, W: 30, H: 4},
				{Type: "systeminfo", X: 0, Y: 12, W: 36, H: 4},
				{Type: "newwidget", X: 36, Y: 12, W: 36, H: 4},
			}},
			{Name: "Second", Widgets: []model.Widget{{Type: "clock", W: 72, H: 4}}},
		}},
		{ID: "2", Name: "Empty", Pages: []model.BoardPage{{}}},
	}
}

func testBoardData(page int) poller.BoardData {
	now := float64(testNow.Unix())
	item := zabbix.Item{ItemID: "1", HostID: "1", Name: "NVPS", LastValue: "5.5", LastClock: "1", Units: "!vps", ValueType: "0"}
	return poller.BoardData{BoardData: model.BoardData{BoardID: "1", Page: page, Widgets: []model.WidgetData{
		{}, {}, {},
		{},
		{Avail: &model.AvailCount{Up: 3, Down: 1}},
		{Items: []zabbix.Item{item}},
		{Top: &model.Top{
			Columns: []model.TopColumn{{Name: "Host", Host: true}, {Name: "NVPS", Item: "NVPS"}, {Name: "Gone", Item: "x"}},
			Rows:    []model.TopRow{{Host: "web-01", Cells: []*zabbix.Item{nil, &item, nil}}},
		}},
		{Range: 3600, Dropped: 2, Series: []model.NamedSeries{{Name: "load", Points: []model.Point{{T: now - 600, V: 1}, {T: now - 60, V: 2}}}}},
		{Info: [][2]string{{"Zabbix version", "7.0.32"}, {"Hosts", "5"}}},
		{},
	}}}
}

type boardCall struct {
	on   bool
	id   string
	page int
}

func boardModel(t *testing.T, w, h int) (Model, *[]boardCall) {
	t.Helper()
	m, _ := newModel(nil)
	var calls []boardCall
	m.opts.WatchBoard = func(on bool, id string, page int) { calls = append(calls, boardCall{on, id, page}) }
	mm, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	m = live(mm.(Model))
	m, _ = press(m, "4")
	mm, _ = m.Update(eventMsg{poller.Boards{List: testBoards()}})
	return mm.(Model), &calls
}

func TestBoardListAndNavigation(t *testing.T) {
	m, calls := boardModel(t, 140, 40)
	wants(t, plain(m), "4 Boards", "Zabbix dashboards", "Global view", "Empty")
	if len(*calls) != 1 || (*calls)[0] != (boardCall{true, "", 0}) {
		t.Fatalf("%+v", *calls)
	}
	m, _ = press(m, "enter")
	if m.boardID != "1" || (*calls)[1] != (boardCall{true, "1", 0}) {
		t.Fatalf("%+v", *calls)
	}
	wants(t, plain(m), "› Global view (page 1/2)")

	m, _ = press(m, "]")
	if m.boardPage != 1 || (*calls)[2] != (boardCall{true, "1", 1}) {
		t.Fatalf("%+v", *calls)
	}
	m, _ = press(m, "]")
	if m.boardPage != 1 {
		t.Error("page ran past the last")
	}
	m, _ = press(m, "esc")
	if m.boardID != "" || m.view != Boards {
		t.Error("esc must return to the list")
	}
	m, _ = press(m, "1")
	if last := (*calls)[len(*calls)-1]; last.on {
		t.Errorf("leaving must stop the watch: %+v", last)
	}
	m, _ = press(m, "tab", "tab", "tab")
	if m.view != Boards {
		t.Errorf("tab reaches Boards: %v", m.view)
	}
	if last := (*calls)[len(*calls)-1]; !last.on {
		t.Errorf("tab must start the watch: %+v", last)
	}
	if m, _ = press(m, "tab"); m.view != Dashboard {
		t.Errorf("tab wraps: %v", m.view)
	}
}

func TestBoardWebURL(t *testing.T) {
	m, _ := boardModel(t, 140, 40)
	if u := m.webURL(); !strings.HasSuffix(u, "action=dashboard.list") {
		t.Error(u)
	}
	m, _ = press(m, "enter")
	if u := m.webURL(); !strings.HasSuffix(u, "action=dashboard.view&dashboardid=1") {
		t.Error(u)
	}
}

func TestBoardRendersEveryWidget(t *testing.T) {
	m, _ := boardModel(t, 150, 45)
	m, _ = press(m, "enter")
	mm, _ := m.Update(eventMsg{testBoardData(0)})
	m = mm.(Model)
	out := plain(m)
	wants(t, out,
		"Current problems", "Zabbix agent is not available", "Local time",
		"Problems by severity", "High 1", "Average 0", // filtered to host 2
		"geomap widget", "newwidget widget", "not drawn in a terminal",
		"3 available", "1 not available",
		"5.50 vps", "web-01 · rate",
		"Top hosts", "NVPS", "+2 more series",
		"System information", "Zabbix version", "7.0.32")
	if strings.Contains(out, "!vps") {
		t.Error("the unit marker leaked")
	}
	for i, l := range strings.Split(out, "\n") {
		if lipgloss.Width(l) > 150 {
			t.Fatalf("line %d is %d wide", i, lipgloss.Width(l))
		}
	}
}

func TestBoardIgnoresDataForAnotherPage(t *testing.T) {
	m, _ := boardModel(t, 150, 45)
	m, _ = press(m, "enter", "]")
	mm, _ := m.Update(eventMsg{testBoardData(0)})
	if strings.Contains(plain(mm.(Model)), "3 available") {
		t.Error("stale data drawn")
	}
}

func TestBoardScrollsBothWaysOnSmallTerminal(t *testing.T) {
	m, _ := boardModel(t, 80, 24)
	m, _ = press(m, "enter")
	mm, _ := m.Update(eventMsg{testBoardData(0)})
	m = mm.(Model)
	g, _ := m.boardGeom()
	if !g.vbar || !g.hbar || g.canvasW != boardMinWidth || g.viewW != 79 || g.viewH != 20 {
		t.Fatalf("%+v", g)
	}
	out := plain(m)
	wants(t, out, "Current problems", "█")
	if strings.Contains(out, "System information") {
		t.Fatal("expected a later widget off-screen")
	}

	m, _ = press(m, "G")
	wants(t, plain(m), "System information")
	if m.boardScroll != g.maxY() {
		t.Errorf("scroll %d of %d", m.boardScroll, g.maxY())
	}
	m, _ = press(m, "ctrl+u")
	if m.boardScroll != g.maxY()-g.viewH/2 {
		t.Errorf("half screen: %d", m.boardScroll)
	}
	m, _ = press(m, "g", "pgdown")
	if m.boardScroll != min(g.viewH-1, g.maxY()) {
		t.Errorf("full screen: %d", m.boardScroll)
	}
	m, _ = press(m, "g", "ctrl+d")
	if m.boardScroll != g.viewH/2 {
		t.Errorf("half screen: %d", m.boardScroll)
	}

	m, _ = press(m, "l")
	if m.boardX != boardPanStep {
		t.Errorf("pan %d", m.boardX)
	}
	for range 20 {
		m, _ = press(m, "l")
	}
	if m.boardX != g.maxX() {
		t.Errorf("pan ran past the edge: %d of %d", m.boardX, g.maxX())
	}
	m, _ = press(m, "h")
	if m.boardX != g.maxX()-boardPanStep {
		t.Errorf("pan back: %d", m.boardX)
	}
	for i, l := range strings.Split(plain(m), "\n") {
		if lipgloss.Width(l) > 80 {
			t.Fatalf("line %d is %d wide", i, lipgloss.Width(l))
		}
	}
}

func TestListsKeepTheirPageStep(t *testing.T) {
	m, _ := boardModel(t, 140, 40)
	m.boards = make([]model.Board, 30)
	if m, _ = press(m, "pgdown"); m.boardSel != pageStep {
		t.Errorf("sel %d", m.boardSel)
	}
}

func TestTablesShowAScrollbarOnlyWhenTheyOverflow(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "2")
	if strings.Contains(plain(m), "█") {
		t.Fatal("a list that fits must look as it did")
	}
	var hosts []model.HostRow
	for i := range 80 {
		hosts = append(hosts, model.HostRow{ID: fmt.Sprint(i), Name: fmt.Sprintf("h%02d", i), Enabled: true})
	}
	var problems []model.Problem
	for i := range 80 {
		problems = append(problems, model.Problem{EventID: fmt.Sprint(i), Name: "p", Clock: testNow.Unix(), Host: "h"})
	}
	m.snapshot.Hosts, m.snapshot.Problems = hosts, problems
	for _, key := range []string{"1", "2", "3"} {
		m, _ = press(m, key)
		out := plain(m)
		wants(t, out, "█")
		for i, l := range strings.Split(out, "\n") {
			if lipgloss.Width(l) > 140 {
				t.Fatalf("view %s line %d is %d wide", key, i, lipgloss.Width(l))
			}
		}
	}
}

func TestItemsTableScrollbar(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "2", "enter", "tab")
	var items []zabbix.Item
	for i := range 80 {
		items = append(items, zabbix.Item{ItemID: fmt.Sprint(i), Name: fmt.Sprintf("item %d", i), LastClock: "0"})
	}
	mm, _ := m.Update(eventMsg{poller.Detail{HostDetail: model.HostDetail{HostID: m.detailHost, Range: ranges[m.rangeIdx].secs, Items: items}}})
	wants(t, plain(mm.(Model)), "item 0", "█")
}

func wheel(m Model, b tea.MouseButton, mod tea.KeyMod) Model {
	mm, _ := m.Update(tea.MouseWheelMsg{Button: b, Mod: mod})
	return mm.(Model)
}

func TestMouseWheel(t *testing.T) {
	m, _ := boardModel(t, 80, 24)
	m, _ = press(m, "enter")
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("the mouse must default to off")
	}
	if m = wheel(m, tea.MouseWheelDown, 0); m.boardScroll != 0 {
		t.Fatal("the wheel moved while off")
	}
	m, _ = press(m, "M")
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("M must turn it on")
	}
	wants(t, plain(m), "Mouse wheel on")
	m = wheel(m, tea.MouseWheelDown, 0)
	if m.boardScroll != wheelStep {
		t.Errorf("scroll %d", m.boardScroll)
	}
	m = wheel(m, tea.MouseWheelUp, 0)
	if m.boardScroll != 0 {
		t.Errorf("scroll %d", m.boardScroll)
	}
	m = wheel(m, tea.MouseWheelRight, 0)
	m = wheel(m, tea.MouseWheelDown, tea.ModShift)
	if m.boardX != 2*boardPanStep || m.boardScroll != 0 {
		t.Errorf("pan %d scroll %d", m.boardX, m.boardScroll)
	}
	m = wheel(m, tea.MouseWheelUp, tea.ModShift)
	m = wheel(m, tea.MouseWheelLeft, 0)
	if m.boardX != 0 {
		t.Errorf("pan %d", m.boardX)
	}
	m, _ = press(m, "M")
	if m.View().MouseMode != tea.MouseModeNone {
		t.Fatal("M must turn it off")
	}
}

func TestWheelMovesListsButNotBehindADialog(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "3", "M")
	m = wheel(m, tea.MouseWheelDown, 0)
	if m.problemSel != 2 {
		t.Errorf("sel %d", m.problemSel)
	}
	m, _ = press(m, "?")
	m = wheel(m, tea.MouseWheelUp, 0)
	if m.problemSel != 2 {
		t.Error("the wheel moved the list behind help")
	}
}

func TestMouseOptionStartsItOn(t *testing.T) {
	if New(Options{Theme: "nord", Mouse: true}).View().MouseMode != tea.MouseModeCellMotion {
		t.Error("config mouse = true ignored")
	}
}

func TestBoardListScrollbar(t *testing.T) {
	m, _ := boardModel(t, 100, 24)
	m.boards = nil
	for i := range 40 {
		m.boards = append(m.boards, model.Board{ID: fmt.Sprint(i), Name: fmt.Sprintf("board %d", i)})
	}
	out := plain(m)
	wants(t, out, "board 0", "█")
	if strings.Contains(out, "board 39") {
		t.Fatal("expected the list to overflow")
	}
	m, _ = press(m, "G")
	wants(t, plain(m), "board 39")
}

func TestProblemWidgetCountsWhatDoesNotFit(t *testing.T) {
	m, _ := boardModel(t, 150, 45)
	m, _ = press(m, "enter")
	m.snapshot.Problems = nil
	for i := range 40 {
		m.snapshot.Problems = append(m.snapshot.Problems, model.Problem{EventID: fmt.Sprint(i), Name: "p", Clock: testNow.Unix()})
	}
	wants(t, plain(m), "more")
}

func TestScrollThumb(t *testing.T) {
	for _, c := range []struct{ track, total, offset, start, size int }{
		{10, 20, 0, 0, 5},
		{10, 20, 10, 5, 5},
		{10, 1000, 990, 9, 1},
		{10, 10, 0, 0, 10},
	} {
		if s, n := scrollThumb(c.track, c.total, c.offset); s != c.start || n != c.size {
			t.Errorf("%+v: %d %d", c, s, n)
		}
	}
}

func TestBoardLayoutFitsTheGrid(t *testing.T) {
	p := testBoards()[0].Pages[0]
	g := boardLayout(&p, 150, 42)
	if g.canvasH != 48 || !g.vbar || g.hbar || g.canvasW != 149 {
		t.Fatalf("16 grid rows at 3 cells: %+v", g)
	}
	for _, c := range g.cells {
		if c.x+c.w > g.canvasW || c.y+c.h > g.canvasH || c.w <= 0 || c.h <= 0 {
			t.Errorf("%+v", c)
		}
	}
	if g = boardLayout(&p, 150, 60); g.canvasH != 60 || g.vbar || g.cells[0].h != 18 {
		t.Errorf("a roomy screen stretches the grid: %+v", g)
	}
}

func TestBoardEmptyAndFailedStates(t *testing.T) {
	m, _ := newModel(nil)
	m = live(m)
	m, _ = press(m, "4")
	wants(t, plain(m), "loading dashboards")
	mm, _ := m.Update(eventMsg{poller.Boards{}})
	wants(t, plain(mm.(Model)), "can see no dashboards")

	w, _ := boardModel(t, 150, 45)
	w, _ = press(w, "enter")
	d := testBoardData(0)
	d.Widgets[7] = model.WidgetData{Err: "Application error."}
	mm, _ = w.Update(eventMsg{d})
	wants(t, plain(mm.(Model)), "Application error.", "3 available")
}

func TestBoardASCII(t *testing.T) {
	m, _ := boardModel(t, 150, 45)
	m, _ = press(m, "enter")
	mm, _ := m.Update(eventMsg{testBoardData(0)})
	for _, r := range asciiOnly(plain(mm.(Model))) {
		if r > 0x7e && r != '\n' {
			t.Fatalf("non-ASCII %q", r)
		}
	}
}
