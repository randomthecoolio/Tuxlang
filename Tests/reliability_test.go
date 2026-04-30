package main

import (
	"strings"
	"testing"

	"tuxlang/internal/bytecode"
	"tuxlang/internal/vm"
)

func TestRuntimeHandlesTruncatedBytecodeGracefully(t *testing.T) {
	machine := vm.New()
	fn := &bytecode.Function{
		Name: "<bad>",
		Code: []byte{byte(bytecode.OpGetGlobal)}, // missing 2-byte operand
	}

	err := machine.Run(fn)
	if err == nil {
		t.Fatal("expected runtime error for truncated bytecode")
	}
	if !strings.Contains(err.Error(), "truncated operand") {
		t.Fatalf("unexpected error: %v", err)
	}
}
