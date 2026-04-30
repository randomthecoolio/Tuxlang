package main

import (
	"fmt"
	"strings"
	"testing"

	"tuxlang/internal/compiler"
	"tuxlang/internal/lexer"
	"tuxlang/internal/parser"
	"tuxlang/internal/vm"
)

func benchmarkProgramSource() string {
	lines := []string{
		"function fib(n)",
		"\tif n <= 1 then",
		"\t\treturn n",
		"\tend",
		"\treturn fib(n - 1) + fib(n - 2)",
		"end",
		"",
		"local total = 0",
		"for i = 1, 250 do",
		"\ttotal = total + i",
		"end",
		"",
		"local data = {\"a\": 1, \"b\": 2, \"c\": 3}",
		"assert(data[\"b\"] == 2, \"map read failed\")",
		"assert(total == 31375, \"sum mismatch\")",
		"assert(fib(12) == 144, \"fib mismatch\")",
	}
	return strings.Join(lines, "\n")
}

func mustParseForBench(b *testing.B, source string) *parser.Parser {
	b.Helper()
	l := lexer.New(source)
	return parser.New(l)
}

func BenchmarkTuxlangParseCompileRun(b *testing.B) {
	source := benchmarkProgramSource()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		p := mustParseForBench(b, source)
		program, err := p.ParseProgram()
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}

		comp := compiler.New()
		fn, err := comp.Compile(program)
		if err != nil {
			b.Fatalf("compile failed: %v", err)
		}

		machine := vm.New()
		if err := machine.Run(fn); err != nil {
			b.Fatalf("run failed: %v", err)
		}
	}
}

func BenchmarkTuxlangCompileOnly(b *testing.B) {
	source := benchmarkProgramSource()
	p := mustParseForBench(b, source)
	program, err := p.ParseProgram()
	if err != nil {
		b.Fatalf("parse setup failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		comp := compiler.New()
		if _, err := comp.Compile(program); err != nil {
			b.Fatalf("compile failed: %v", err)
		}
	}
}

func BenchmarkTuxlangRunOnly(b *testing.B) {
	source := benchmarkProgramSource()
	p := mustParseForBench(b, source)
	program, err := p.ParseProgram()
	if err != nil {
		b.Fatalf("parse setup failed: %v", err)
	}
	comp := compiler.New()
	fn, err := comp.Compile(program)
	if err != nil {
		b.Fatalf("compile setup failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		machine := vm.New()
		if err := machine.Run(fn); err != nil {
			b.Fatalf("run failed at iter %d: %v", i, err)
		}
	}
}

func BenchmarkTuxlangLargeLoopRunOnly(b *testing.B) {
	source := fmt.Sprintf("local sum = 0\nfor i = 1, %d do\n\tsum = sum + i\nend\nassert(sum > 0, \"sum should be > 0\")", 10000)
	p := mustParseForBench(b, source)
	program, err := p.ParseProgram()
	if err != nil {
		b.Fatalf("parse setup failed: %v", err)
	}
	comp := compiler.New()
	fn, err := comp.Compile(program)
	if err != nil {
		b.Fatalf("compile setup failed: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		machine := vm.New()
		if err := machine.Run(fn); err != nil {
			b.Fatalf("run failed: %v", err)
		}
	}
}
