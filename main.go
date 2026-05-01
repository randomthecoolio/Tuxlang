package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"tuxlang/internal/compiler"
	"tuxlang/internal/lexer"
	"tuxlang/internal/parser"
	"tuxlang/internal/vm"
)

var embeddedMarker = []byte("TUXLANG_EMBEDDED_V1")

func main() {
	sourceText, sourcePath, fromEmbeddedBinary, err := resolveSourceFromArgs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}
	opts, cleanSource := parseScriptRuntimeOptions(sourceText)
	applyRuntimeOptions(opts)

	l := lexer.New(cleanSource)
	p := parser.New(l)
	program, err := p.ParseProgram()
	if err != nil {
		printUserError("Parse", err)
		maybePauseOnExit(opts, fromEmbeddedBinary)
		os.Exit(1)
	}

	comp := compiler.New()
	fn, err := comp.Compile(program)
	if err != nil {
		printUserError("Compile", err)
		maybePauseOnExit(opts, fromEmbeddedBinary)
		os.Exit(1)
	}

	machine := vm.New()
	machine.SetProgramContext(sourcePath, sourceText)
	if err := machine.Run(fn); err != nil {
		printUserError("Runtime", err)
		maybePauseOnExit(opts, fromEmbeddedBinary)
		os.Exit(1)
	}
	maybePauseOnExit(opts, fromEmbeddedBinary)
}

type scriptRuntimeOptions struct {
	exitWindow  bool
	showConsole bool
}

func parseScriptRuntimeOptions(source string) (scriptRuntimeOptions, string) {
	opts := scriptRuntimeOptions{
		exitWindow:  false,
		showConsole: true,
	}
	lines := strings.Split(source, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		line = strings.TrimPrefix(line, "//")
		line = strings.TrimSpace(line)
		if parsed, key, ok := parseBoolDirectiveLine(line); ok {
			switch key {
			case "exit_window":
				opts.exitWindow = parsed
			case "show_console":
				opts.showConsole = parsed
			}
			continue
		}
		cleaned = append(cleaned, rawLine)
	}
	return opts, strings.Join(cleaned, "\n")
}

func parseBoolDirectiveLine(line string) (bool, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return false, "", false
	}
	key := strings.TrimSpace(parts[0])
	if key != "exit_window" && key != "show_console" {
		return false, "", false
	}
	valueRaw := strings.TrimSpace(parts[1])
	value, err := strconv.ParseBool(valueRaw)
	if err != nil {
		return false, "", false
	}
	return value, key, true
}

func maybePauseOnExit(opts scriptRuntimeOptions, fromEmbeddedBinary bool) {
	if !fromEmbeddedBinary {
		return
	}
	if opts.exitWindow || !opts.showConsole {
		return
	}
	stat, err := os.Stdin.Stat()
	if err != nil || (stat.Mode()&os.ModeCharDevice) == 0 {
		return
	}
	fmt.Println()
	fmt.Print("Press Enter to exit...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
}

func applyRuntimeOptions(opts scriptRuntimeOptions) {
	if opts.showConsole {
		return
	}
	nullFile, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return
	}
	os.Stdout = nullFile
	os.Stderr = nullFile
}

func resolveSourceFromArgs() (string, string, bool, error) {
	if len(os.Args) == 2 {
		path, err := filepath.Abs(os.Args[1])
		if err != nil {
			return "", "", false, err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return "", "", false, err
		}
		return string(src), path, false, nil
	}
	if len(os.Args) == 1 {
		src, err := readEmbeddedProgramFromSelf()
		if err == nil {
			return src, "__embedded__", true, nil
		}
		if !errors.Is(err, errNoEmbeddedProgram) {
			return "", "", false, err
		}
	}
	return "", "", false, fmt.Errorf("usage: tuxlang <file.tux> (or run an embedded self-built binary)")
}

var errNoEmbeddedProgram = errors.New("no embedded tux program")

func readEmbeddedProgramFromSelf() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(exePath)
	if err != nil {
		return "", err
	}
	source, ok := extractEmbeddedProgram(data)
	if !ok {
		return "", errNoEmbeddedProgram
	}
	return string(source), nil
}

func extractEmbeddedProgram(data []byte) ([]byte, bool) {
	const footerLen = 8
	need := len(embeddedMarker) + footerLen
	if len(data) < need {
		return nil, false
	}
	lengthOffset := len(data) - footerLen
	sourceLen := binary.LittleEndian.Uint64(data[lengthOffset:])
	if sourceLen == 0 {
		return nil, false
	}
	if sourceLen > uint64(len(data)-need) {
		return nil, false
	}
	markerStart := len(data) - footerLen - len(embeddedMarker)
	if markerStart < 0 {
		return nil, false
	}
	if string(data[markerStart:markerStart+len(embeddedMarker)]) != string(embeddedMarker) {
		return nil, false
	}
	sourceStart := markerStart - int(sourceLen)
	if sourceStart < 0 {
		return nil, false
	}
	return data[sourceStart:markerStart], true
}

func printUserError(stage string, err error) {
	message := err.Error()
	fmt.Fprintf(os.Stderr, "%s error:\n  %s\n", stage, message)

	hint := ""
	switch {
	case strings.Contains(message, "not defined"):
		hint = "Hint: declare variables before use, for example: local name = value"
	case strings.Contains(message, "expected 'else' or 'end'"):
		hint = "Hint: every if block should end with 'end' (and optional 'else' before it)."
	case strings.Contains(message, "expected block start"):
		hint = "Hint: use 'then' / 'do' / '{' to start a block."
	case strings.Contains(message, "where an expression should start"):
		hint = "Hint: expressions can start with a value, variable name, or parentheses."
	case strings.Contains(message, "expects"):
		hint = "Hint: check function argument count and types."
	}
	if hint != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", hint)
	}
}
