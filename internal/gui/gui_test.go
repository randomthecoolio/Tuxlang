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

func TestWindowsFormScriptEmitsJSONResultOnClose(t *testing.T) {
	win := &Window{title: "Demo", width: 320, height: 200, background: "#ffffff"}
	win.Add(NewButton("Open"))

	script := windowsFormScript(win)
	if !strings.Contains(script, "ConvertTo-Json $result -Compress") {
		t.Fatalf("expected json output in windows script: %s", script)
	}
	if strings.Contains(script, "Invoke-Expression") {
		t.Fatalf("expected calculator legacy logic to be removed: %s", script)
	}
}

func TestDecodeResultSkipsNonJSONLines(t *testing.T) {
	out := []byte("warning line\n{\"action\":\"click\",\"target\":\"ok\",\"values\":{\"name\":\"tux\"}}")
	result, err := decodeResult(out)
	if err != nil {
		t.Fatalf("decodeResult failed: %v", err)
	}
	if result.Action != "click" || result.Target != "ok" || result.Values["name"] != "tux" {
		t.Fatalf("unexpected result: %#v", result)
	}
}
