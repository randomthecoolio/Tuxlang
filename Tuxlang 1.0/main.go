package main

import (
	"fmt"
	"os"
	"strings"

	"tuxlang/internal/compiler"
	"tuxlang/internal/lexer"
	"tuxlang/internal/parser"
	"tuxlang/internal/vm"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: tuxlang <file.tux>")
		os.Exit(2)
	}

	src, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read error: %v\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(src))
	p := parser.New(l)
	program, err := p.ParseProgram()
	if err != nil {
		printUserError("Parse", err)
		os.Exit(1)
	}

	comp := compiler.New()
	fn, err := comp.Compile(program)
	if err != nil {
		printUserError("Compile", err)
		os.Exit(1)
	}

	machine := vm.New()
	if err := machine.Run(fn); err != nil {
		printUserError("Runtime", err)
		os.Exit(1)
	}
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
