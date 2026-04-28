package gui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type App struct {
	windows []*Window
}

type Result struct {
	Action string
	Target string
	Values map[string]string
}

type Window struct {
	title      string
	x          int
	y          int
	width      int
	height     int
	background string
	children   []Widget
}

type Widget interface {
	kind() string
	base() *baseWidget
}

type baseWidget struct {
	id         string
	text       string
	x          int
	y          int
	width      int
	height     int
	foreground string
	background string
	fontSize   int
}

type Label struct {
	baseWidget
}

type Button struct {
	baseWidget
}

type TextInput struct {
	baseWidget
	placeholder string
}

type Panel struct {
	baseWidget
	children []Widget
}

func NewApp() *App {
	return &App{}
}

func (a *App) NewWindow(title string) *Window {
	w := &Window{
		title:      title,
		width:      720,
		height:     480,
		background: "#f7f3ea",
	}
	a.windows = append(a.windows, w)
	return w
}

func NewLabel(text string) *Label {
	return &Label{baseWidget: defaultBase(text, 24, 24, 220, 32)}
}

func NewButton(text string) *Button {
	return &Button{baseWidget: defaultBase(text, 24, 24, 140, 40)}
}

func NewTextInput(text string) *TextInput {
	return &TextInput{
		baseWidget:  defaultBase(text, 24, 24, 220, 34),
		placeholder: "",
	}
}

func NewPanel() *Panel {
	return &Panel{
		baseWidget: defaultBase("", 0, 0, 320, 200),
		children:   []Widget{},
	}
}

func (w *Window) SetContent(content interface{}) {
	w.children = nil
	w.Add(content)
}

func (w *Window) Add(content interface{}) {
	if widget, ok := content.(Widget); ok {
		w.children = append(w.children, widget)
	}
}

func (w *Window) SetSize(width, height int) {
	if width > 0 {
		w.width = width
	}
	if height > 0 {
		w.height = height
	}
}

func (w *Window) SetPosition(x, y int) {
	w.x = x
	w.y = y
}

func (w *Window) SetBackground(color string) {
	if color != "" {
		w.background = color
	}
}

func (w *Window) Show() (*Result, error) {
	return showWindow(w)
}

func (a *App) Run() (*Result, error) {
	var last *Result
	for _, w := range a.windows {
		result, err := showWindow(w)
		if err != nil {
			return nil, err
		}
		last = result
	}
	if last == nil {
		last = &Result{Action: "none", Values: map[string]string{}}
	}
	return last, nil
}

func (p *Panel) Add(content interface{}) {
	if widget, ok := content.(Widget); ok {
		p.children = append(p.children, widget)
	}
}

func SetID(content interface{}, id string) {
	if widget, ok := content.(Widget); ok {
		widget.base().id = id
	}
}

func SetText(content interface{}, text string) {
	switch value := content.(type) {
	case Widget:
		value.base().text = text
	case *Window:
		value.title = text
	}
}

func SetPosition(content interface{}, x, y int) {
	switch value := content.(type) {
	case Widget:
		value.base().x = x
		value.base().y = y
	case *Window:
		value.SetPosition(x, y)
	}
}

func SetSize(content interface{}, width, height int) {
	switch value := content.(type) {
	case Widget:
		if width > 0 {
			value.base().width = width
		}
		if height > 0 {
			value.base().height = height
		}
	case *Window:
		value.SetSize(width, height)
	}
}

func SetColors(content interface{}, foreground, background string) {
	if widget, ok := content.(Widget); ok {
		if foreground != "" {
			widget.base().foreground = foreground
		}
		if background != "" {
			widget.base().background = background
		}
	}
}

func SetFontSize(content interface{}, size int) {
	if widget, ok := content.(Widget); ok && size > 0 {
		widget.base().fontSize = size
	}
}

func SetPlaceholder(content interface{}, text string) {
	if input, ok := content.(*TextInput); ok {
		input.placeholder = text
	}
}

func SetValue(content interface{}, text string) {
	switch value := content.(type) {
	case *TextInput:
		value.text = text
	case Widget:
		value.base().text = text
	}
}

func GetID(content interface{}) string {
	if widget, ok := content.(Widget); ok {
		return widget.base().id
	}
	return ""
}

func GetValue(content interface{}) string {
	switch value := content.(type) {
	case *TextInput:
		return value.text
	case Widget:
		return value.base().text
	default:
		return ""
	}
}

func (l *Label) kind() string     { return "label" }
func (b *Button) kind() string    { return "button" }
func (i *TextInput) kind() string { return "input" }
func (p *Panel) kind() string     { return "panel" }

func (l *Label) base() *baseWidget     { return &l.baseWidget }
func (b *Button) base() *baseWidget    { return &b.baseWidget }
func (i *TextInput) base() *baseWidget { return &i.baseWidget }
func (p *Panel) base() *baseWidget     { return &p.baseWidget }

func defaultBase(text string, x, y, width, height int) baseWidget {
	return baseWidget{
		text:       text,
		x:          x,
		y:          y,
		width:      width,
		height:     height,
		foreground: "#18222f",
		background: "#ffffff",
		fontSize:   12,
	}
}

func showWindow(w *Window) (*Result, error) {
	return showWindowWithFallback(w)
}

func showWindowWithFallback(w *Window) (*Result, error) {
	if len(w.children) == 0 {
		w.children = []Widget{NewLabel("Empty window")}
	}
	switch runtime.GOOS {
	case "windows":
		return showWindowsNative(w)
	case "darwin":
		result, err := showNativeDesktop(w)
		if err == nil {
			return result, nil
		}
		return showBrowserBackend(w)
	case "linux":
		result, err := showNativeDesktop(w)
		if err == nil {
			return result, nil
		}
		return showBrowserBackend(w)
	default:
		return showBrowserBackend(w)
	}
}

func showWindowsNative(w *Window) (*Result, error) {
	script := windowsFormScript(w)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-EncodedCommand", encodePowerShell(script))
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return decodeResult(output)
}

func showMacNative(w *Window) (*Result, error) {
	return showNativeDesktop(w)
}

func showLinuxNative(w *Window) (*Result, error) {
	return showNativeDesktop(w)
}

func windowsFormScript(w *Window) string {
	lines := []string{
		"Add-Type -AssemblyName System.Windows.Forms",
		"Add-Type -AssemblyName System.Drawing",
		"[System.Windows.Forms.Application]::EnableVisualStyles()",
		"$result = @{ action = ''; target = ''; values = @{} }",
		"$script:keepRunning = $true",
		"$expression = ''",
		"$display = $null",
		"//calc:globalcHandler = $true",
		"function Collect-Inputs($control) {",
		"  if ($control -is [System.Windows.Forms.TextBox]) {",
		"    $tag = [string]$control.Tag",
		"    if ($tag.StartsWith('input:')) {",
		"      $result.values[$tag.Substring(6)] = $control.Text",
		"    }",
		"  }",
		"  foreach ($child in $control.Controls) {",
		"    Collect-Inputs $child",
		"  }",
		"}",
		"$form = New-Object System.Windows.Forms.Form",
		fmt.Sprintf("$form.Text = '%s'", psSingleQuoted(w.title)),
		"$form.StartPosition = [System.Windows.Forms.FormStartPosition]::Manual",
		fmt.Sprintf("$form.Location = New-Object System.Drawing.Point(%d, %d)", w.x, w.y),
		fmt.Sprintf("$form.Size = New-Object System.Drawing.Size(%d, %d)", w.width, w.height),
		fmt.Sprintf("$form.BackColor = %s", colorExpr(w.background, "$null")),
		"$form.AutoScaleMode = [System.Windows.Forms.AutoScaleMode]::None",
		"$form.Add_FormClosing({ $script:keepRunning = $false })",
	}
	counter := 0
	lines = append(lines, renderWindowsWidgets("$form", w.children, &counter)...)
	lines = append(lines,
		"$form.Show()",
		"while ($form.IsDisposed -eq $false) {",
		"  [System.Windows.Forms.Application]::DoEvents()",
		"  Start-Sleep -Milliseconds 50",
		"  if ($result.action -eq 'click') {",
		"    $clicked = $result.target",
		"    if ($clicked -eq '=') {",
		"      try { $res = Invoke-Expression $expression | Out-String } catch { $res = 'Error' }",
		"      $display.Text = $res",
		"      $expression = ''",
		"    } elseif ($clicked -eq 'C') {",
		"      $expression = ''",
		"      $display.Text = ''",
		"    } else {",
		"      $expression = $expression + $clicked",
		"      $display.Text = $expression",
		"    }",
		"    $result.action = ''",
		"    $result.target = ''",
		"  }",
		"  if ($result.action -eq 'close') {",
		"    try { $res = Invoke-Expression $expression | Out-String } catch { $res = $expression }",
		"    Write-Output ($res)",
		"    break",
		"  }",
		"}",
	)
	return strings.Join(lines, "\n")
}

func renderWindowsWidgets(parent string, widgets []Widget, counter *int) []string {
	lines := []string{}
	for i, widget := range widgets {
		*counter = *counter + 1
		name := fmt.Sprintf("$ctrl%d", *counter)
		base := widget.base()
		id := base.id
		if id == "" {
			id = fmt.Sprintf("%s_%d", widget.kind(), i)
		}
		switch value := widget.(type) {
		case *Label:
			lines = append(lines,
				fmt.Sprintf("%s = New-Object System.Windows.Forms.Label", name),
				fmt.Sprintf("%s.Text = '%s'", name, psSingleQuoted(value.text)),
				fmt.Sprintf("%s.AutoSize = $false", name),
				fmt.Sprintf("%s.Location = New-Object System.Drawing.Point(%d, %d)", name, base.x, base.y),
				fmt.Sprintf("%s.Size = New-Object System.Drawing.Size(%d, %d)", name, base.width, base.height),
				fmt.Sprintf("%s.Font = New-Object System.Drawing.Font('Segoe UI', %d)", name, base.fontSize),
				fmt.Sprintf("%s.ForeColor = %s", name, colorExpr(base.foreground, "[System.Drawing.Color]::Black")),
				fmt.Sprintf("%s.BackColor = %s", name, colorExpr(base.background, "[System.Drawing.Color]::Transparent")),
				fmt.Sprintf("%s.Controls.Add(%s)", parent, name),
			)
			lines = append(lines, "if ($display -eq $null) { $display = "+name+" }")
		case *Button:
			lines = append(lines,
				fmt.Sprintf("%s = New-Object System.Windows.Forms.Button", name),
				fmt.Sprintf("%s.Text = '%s'", name, psSingleQuoted(value.text)),
				fmt.Sprintf("%s.Location = New-Object System.Drawing.Point(%d, %d)", name, base.x, base.y),
				fmt.Sprintf("%s.Size = New-Object System.Drawing.Size(%d, %d)", name, base.width, base.height),
				fmt.Sprintf("%s.Font = New-Object System.Drawing.Font('Segoe UI', %d)", name, base.fontSize),
				fmt.Sprintf("%s.ForeColor = %s", name, colorExpr(base.foreground, "[System.Drawing.Color]::Black")),
				fmt.Sprintf("%s.BackColor = %s", name, colorExpr(base.background, "[System.Drawing.SystemColors]::Control")),
				fmt.Sprintf("%s.Tag = '%s'", name, psSingleQuoted(id)),
				fmt.Sprintf("%s.Add_Click({", name),
				"  $result.action = 'click'",
				fmt.Sprintf("  $result.target = '%s'", psSingleQuoted(id)),
				"  Collect-Inputs $form",
				"})",
				fmt.Sprintf("%s.Add_MouseDown({", name),
				"  param($sender, $e)",
				"  if ($e.Button -eq [System.Windows.Forms.MouseButtons]::Right) {",
				"    $result.action = 'rightclick'",
				fmt.Sprintf("    $result.target = '%s'", psSingleQuoted(id)),
				"    Collect-Inputs $form",
				"    Write-Output ((ConvertTo-Json $result -Compress))",
				"    $form.Close()",
				"  }",
				"})",
				fmt.Sprintf("%s.Controls.Add(%s)", parent, name),
			)
		case *TextInput:
			lines = append(lines,
				fmt.Sprintf("%s = New-Object System.Windows.Forms.TextBox", name),
				fmt.Sprintf("%s.Text = '%s'", name, psSingleQuoted(value.text)),
				fmt.Sprintf("%s.Location = New-Object System.Drawing.Point(%d, %d)", name, base.x, base.y),
				fmt.Sprintf("%s.Size = New-Object System.Drawing.Size(%d, %d)", name, base.width, base.height),
				fmt.Sprintf("%s.Font = New-Object System.Drawing.Font('Segoe UI', %d)", name, base.fontSize),
				fmt.Sprintf("%s.ForeColor = %s", name, colorExpr(base.foreground, "[System.Drawing.Color]::Black")),
				fmt.Sprintf("%s.BackColor = %s", name, colorExpr(base.background, "[System.Drawing.Color]::White")),
				fmt.Sprintf("%s.Tag = 'input:%s'", name, psSingleQuoted(id)),
				fmt.Sprintf("%s.Controls.Add(%s)", parent, name),
			)
			lines = append(lines, "if ($display -eq $null) { $display = "+name+" }")
		case *Panel:
			lines = append(lines,
				fmt.Sprintf("%s = New-Object System.Windows.Forms.Panel", name),
				fmt.Sprintf("%s.Location = New-Object System.Drawing.Point(%d, %d)", name, base.x, base.y),
				fmt.Sprintf("%s.Size = New-Object System.Drawing.Size(%d, %d)", name, base.width, base.height),
				fmt.Sprintf("%s.BorderStyle = [System.Windows.Forms.BorderStyle]::FixedSingle", name),
				fmt.Sprintf("%s.BackColor = %s", name, colorExpr(base.background, "[System.Drawing.Color]::Transparent")),
				fmt.Sprintf("%s.Controls.Add(%s)", parent, name),
			)
			lines = append(lines, renderWindowsWidgets(name, value.children, counter)...)
		}
	}
	return lines
}

func decodeResult(output []byte) (*Result, error) {
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[len(lines)-1]) == "" {
		return &Result{Action: "close", Values: map[string]string{}}, nil
	}
	raw := strings.TrimSpace(lines[len(lines)-1])
	var decoded struct {
		Action string            `json:"action"`
		Target string            `json:"target"`
		Values map[string]string `json:"values"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return nil, err
	}
	if decoded.Values == nil {
		decoded.Values = map[string]string{}
	}
	return &Result{Action: decoded.Action, Target: decoded.Target, Values: decoded.Values}, nil
}

func flattenText(widgets []Widget) string {
	parts := make([]string, 0, len(widgets))
	for _, widget := range widgets {
		switch value := widget.(type) {
		case *Panel:
			parts = append(parts, flattenText(value.children))
		default:
			if text := widget.base().text; text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func showBrowserBackend(w *Window) (*Result, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()

	resultCh := make(chan *Result, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = writer.Write([]byte(renderHTMLWindow(w)))
	})
	mux.HandleFunc("/event", func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		defer request.Body.Close()
		var payload struct {
			Action string            `json:"action"`
			Target string            `json:"target"`
			Values map[string]string `json:"values"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		if payload.Values == nil {
			payload.Values = map[string]string{}
		}
		select {
		case resultCh <- &Result{Action: payload.Action, Target: payload.Target, Values: payload.Values}:
		default:
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true}`))
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			errCh <- serveErr
		}
	}()
	defer server.Close()

	url := "http://" + listener.Addr().String()
	if err := openBrowser(url); err != nil {
		return nil, err
	}

	select {
	case result := <-resultCh:
		_ = server.Close()
		return result, nil
	case serveErr := <-errCh:
		return nil, serveErr
	case <-time.After(10 * time.Minute):
		_ = server.Close()
		return &Result{Action: "timeout", Target: "", Values: map[string]string{}}, nil
	}
}

func renderHTMLWindow(w *Window) string {
	return fmt.Sprintf(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%s</title>
<style>
:root {
	--shadow: 0 28px 80px rgba(24, 34, 47, .16);
}
* { box-sizing: border-box; }
body {
	margin: 0;
	background: linear-gradient(135deg, #ebe5d8, #f8f4ec);
	font-family: "Segoe UI", "Helvetica Neue", Arial, sans-serif;
	color: #18222f;
}
.window {
	position: relative;
	width: %dpx;
	height: %dpx;
	margin: 24px auto;
	background: %s;
	border: 1px solid rgba(24, 34, 47, .12);
	border-radius: 18px;
	box-shadow: var(--shadow);
	overflow: hidden;
}
.titlebar {
	height: 48px;
	display: flex;
	align-items: center;
	padding: 0 18px;
	background: linear-gradient(90deg, rgba(15, 118, 110, .95), rgba(21, 94, 117, .95));
	color: #fff;
	font-size: 14px;
	font-weight: 700;
	letter-spacing: .04em;
	text-transform: uppercase;
}
.surface {
	position: absolute;
	left: 0;
	top: 48px;
	right: 0;
	bottom: 0;
}
.widget {
	position: absolute;
}
.label {
	white-space: pre-wrap;
}
.button {
	border: 0;
	border-radius: 12px;
	cursor: pointer;
	box-shadow: 0 12px 24px rgba(0, 0, 0, .12);
}
.input {
	border: 1px solid rgba(24, 34, 47, .18);
	border-radius: 10px;
	padding: 6px 10px;
	outline: none;
}
.panel {
	border: 1px solid rgba(24, 34, 47, .12);
	border-radius: 14px;
}
</style>
</head>
<body>
	<div class="window">
		<div class="titlebar">%s</div>
		<div class="surface">%s</div>
	</div>
<script>
function collectValues() {
	const values = {};
	document.querySelectorAll('[data-input-id]').forEach((node) => {
		values[node.dataset.inputId] = node.value;
	});
	return values;
}
async function sendResult(action, target) {
	await fetch('/event', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ action, target, values: collectValues() })
	});
}
window.addEventListener('beforeunload', function() {
	navigator.sendBeacon('/event', JSON.stringify({ action: 'close', target: '', values: collectValues() }));
});
</script>
</body>
</html>`,
		html.EscapeString(w.title),
		w.width,
		w.height,
		htmlColor(w.background, "#f7f3ea"),
		html.EscapeString(w.title),
		renderHTMLWidgets(w.children),
	)
}

func renderHTMLWidgets(widgets []Widget) string {
	var builder strings.Builder
	for i, widget := range widgets {
		builder.WriteString(renderHTMLWidget(widget, i))
	}
	return builder.String()
}

func renderHTMLWidget(widget Widget, index int) string {
	base := widget.base()
	id := base.id
	if id == "" {
		id = fmt.Sprintf("%s_%d", widget.kind(), index)
	}
	style := inlineStyle(base)
	switch value := widget.(type) {
	case *Label:
		return fmt.Sprintf(`<div class="widget label" data-widget-id="%s" style="%s">%s</div>`,
			html.EscapeString(id),
			style,
			html.EscapeString(value.text),
		)
	case *Button:
		return fmt.Sprintf(`<button class="widget button" type="button" data-widget-id="%s" style="%s" onclick="sendResult('click', '%s')" oncontextmenu="event.preventDefault(); sendResult('rightclick', '%s'); return false;">%s</button>`,
			html.EscapeString(id),
			style,
			jsQuoted(id),
			jsQuoted(id),
			html.EscapeString(value.text),
		)
	case *TextInput:
		return fmt.Sprintf(`<input class="widget input" data-widget-id="%s" data-input-id="%s" style="%s" value="%s" placeholder="%s">`,
			html.EscapeString(id),
			html.EscapeString(id),
			style,
			html.EscapeString(value.text),
			html.EscapeString(value.placeholder),
		)
	case *Panel:
		return fmt.Sprintf(`<div class="widget panel" data-widget-id="%s" style="%s">%s</div>`,
			html.EscapeString(id),
			style,
			renderHTMLWidgets(value.children),
		)
	default:
		return ""
	}
}

func inlineStyle(base *baseWidget) string {
	parts := []string{
		fmt.Sprintf("left:%dpx", base.x),
		fmt.Sprintf("top:%dpx", base.y),
		fmt.Sprintf("width:%dpx", base.width),
		fmt.Sprintf("height:%dpx", base.height),
		fmt.Sprintf("font-size:%dpx", maxInt(base.fontSize, 12)),
		fmt.Sprintf("color:%s", htmlColor(base.foreground, "#18222f")),
	}
	if base.background != "" {
		parts = append(parts, fmt.Sprintf("background:%s", htmlColor(base.background, "transparent")))
	}
	return strings.Join(parts, ";")
}

func htmlColor(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func jsQuoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return value
}

func maxInt(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func encodePowerShell(script string) string {
	var b strings.Builder
	b.Grow(len(script) * 2)
	for _, r := range script {
		b.WriteByte(byte(r))
		b.WriteByte(byte(r >> 8))
	}
	return base64.StdEncoding.EncodeToString([]byte(b.String()))
}

func colorExpr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return fmt.Sprintf("[System.Drawing.ColorTranslator]::FromHtml('%s')", psSingleQuoted(value))
}

func psSingleQuoted(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func applescriptQuoted(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func openBrowser(target string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", target).Start()
	case "darwin":
		return exec.Command("open", target).Start()
	default:
		return exec.Command("xdg-open", target).Start()
	}
}
