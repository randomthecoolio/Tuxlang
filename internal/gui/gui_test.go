package gui

import (
	"strings"
	"testing"
)

func TestRenderHTMLButtonIncludesRightClick(t *testing.T) {
	button := NewButton("Open")
	html := renderHTMLWidget(button, 0)

	if !strings.Contains(html, "oncontextmenu") {
		t.Fatalf("expected right-click handler in html: %s", html)
	}
	if !strings.Contains(html, "rightclick") {
		t.Fatalf("expected rightclick action in html: %s", html)
	}
}

func TestWindowsFormScriptIncludesRightClick(t *testing.T) {
	win := &Window{title: "Demo", width: 320, height: 200, background: "#ffffff"}
	win.Add(NewButton("Open"))

	script := windowsFormScript(win)
	if !strings.Contains(script, "MouseDown") {
		t.Fatalf("expected mouse down handler in powershell script: %s", script)
	}
	if !strings.Contains(script, "rightclick") {
		t.Fatalf("expected rightclick action in powershell script: %s", script)
	}
}
