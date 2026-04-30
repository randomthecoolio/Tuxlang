package main

import (
	"strings"
	"testing"
	"time"

	"tuxlang/internal/compiler"
	"tuxlang/internal/lexer"
	"tuxlang/internal/parser"
	"tuxlang/internal/vm"
)

func FuzzParserDoesNotPanic(f *testing.F) {
	seeds := []string{
		"",
		"local x = 1",
		"function add(a, b) return a + b end",
		"if true then print(1) end",
		"for i = 1, 3 do print(i) end",
		"class A function init() this.x = 1 end end",
		"import math\nprint(math.sqrt(9))",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, source string) {
		// Keep fuzz iterations bounded to avoid pathological slow inputs.
		if len(source) > 20000 {
			t.Skip()
		}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("parser panicked for input %q: %v", source, r)
			}
		}()

		l := lexer.New(source)
		p := parser.New(l)
		_, _ = p.ParseProgram()
	})
}

func FuzzPipelineDoesNotPanic(f *testing.F) {
	seeds := []string{
		"local a = 1 + 2\nprint(a)",
		"function f(x) return x * 2 end\nprint(f(21))",
		"local m = {\"x\": 1}\nprint(m[\"x\"])",
		"for i = 1, 5 do end",
		"import text\nprint(text.upper(\"tux\"))",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 12000 {
			t.Skip()
		}
		if strings.Count(source, "\n") > 2000 {
			t.Skip()
		}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("pipeline panicked for input %q: %v", source, r)
			}
		}()

		l := lexer.New(source)
		p := parser.New(l)
		program, err := p.ParseProgram()
		if err != nil {
			return
		}

		comp := compiler.New()
		fn, err := comp.Compile(program)
		if err != nil {
			return
		}

		machine := vm.New()
		machine.SetExecutionLimits(20000, 50*time.Millisecond)
		machine.SetBlockDangerousSideEffects(true)
		_ = machine.Run(fn)
	})
}
