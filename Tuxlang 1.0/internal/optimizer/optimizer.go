package optimizer

import (
	"math"

	"tuxlang/internal/ast"
	"tuxlang/internal/bytecode"
)

// Optimizer performs AOT (Ahead-Of-Time) optimizations on bytecode
type Optimizer struct {
	code      []byte
	constants []any
}

// New creates a new bytecode optimizer
func New() *Optimizer {
	return &Optimizer{
		code:      make([]byte, 0, 8192),
		constants: make([]any, 0, 1024),
	}
}

// Optimize performs optimization passes on bytecode
func (o *Optimizer) Optimize(fn *bytecode.Function) *bytecode.Function {
	// Optimize: peephole optimization for common patterns
	optimized := o.peepholeOptimize(fn.Code)

	return &bytecode.Function{
		Name:      fn.Name,
		Code:      optimized,
		Constants: fn.Constants,
		NumLocals: fn.NumLocals,
		NumParams: fn.NumParams,
	}
}

// peepholeOptimize removes redundant operations
func (o *Optimizer) peepholeOptimize(code []byte) []byte {
	result := make([]byte, 0, len(code))

	for i := 0; i < len(code); i++ {
		op := bytecode.Opcode(code[i])

		// Pattern: OpPop followed by OpReturn -> just return
		if op == bytecode.OpPop && i+1 < len(code) && bytecode.Opcode(code[i+1]) == bytecode.OpReturn {
			result = append(result, byte(bytecode.OpReturn))
			i++ // Skip next opcode
			continue
		}

		// Pattern: OpNull followed by OpReturn -> same pattern
		if op == bytecode.OpNull && i+1 < len(code) && bytecode.Opcode(code[i+1]) == bytecode.OpReturn {
			result = append(result, byte(bytecode.OpNull), byte(bytecode.OpReturn))
			i++ // Skip next opcode
			continue
		}

		// Pattern: Duplicate boolean push -> eliminate redundancy
		if (op == bytecode.OpTrue || op == bytecode.OpFalse) && i+1 < len(code) {
			if bytecode.Opcode(code[i+1]) == op {
				// Keep first, skip second
				result = append(result, byte(op))
				i++ // Skip the duplicate
				continue
			}
		}

		result = append(result, byte(op))

		// Preserve argument bytes for instructions with arguments
		switch op {
		case bytecode.OpConstant, bytecode.OpGetGlobal, bytecode.OpSetGlobal,
			bytecode.OpGetLocal, bytecode.OpSetLocal, bytecode.OpArray, bytecode.OpMap:
			// 2-byte argument (uint16)
			if i+2 < len(code) {
				result = append(result, code[i+1], code[i+2])
				i += 2
			}
		case bytecode.OpCall:
			// 1-byte argument
			if i+1 < len(code) {
				result = append(result, code[i+1])
				i++
			}
		}
	}

	return result
}

// DeadCodeEliminate removes unreachable code
func (o *Optimizer) DeadCodeEliminate(code []byte) []byte {
	// Track jump targets and reachable code
	reachable := make(map[int]bool)
	reachable[0] = true // Entry point is always reachable

	for i := 0; i < len(code); i++ {
		if !reachable[i] {
			continue
		}

		op := bytecode.Opcode(code[i])

		switch op {
		case bytecode.OpJump:
			// Jump to new location, mark target as reachable
			if i+3 <= len(code) {
				target := int(code[i+1])<<8 | int(code[i+2])
				reachable[target] = true
				i += 2
				// Code after unconditional jump is unreachable
			}
		case bytecode.OpJumpIfFalse:
			// Conditional jump, mark target as reachable
			if i+3 <= len(code) {
				target := int(code[i+1])<<8 | int(code[i+2])
				reachable[target] = true
				reachable[i+3] = true // Also mark next instruction
				i += 2
			}
		case bytecode.OpReturn:
			// Nothing after return is reachable
		default:
			// Regular instruction, next one is reachable
			if i+1 < len(code) {
				reachable[i+1] = true
			}
		}
	}

	// Build optimized code with only reachable instructions
	result := make([]byte, 0, len(code))
	for i := 0; i < len(code); i++ {
		if reachable[i] {
			result = append(result, code[i])
		}
	}
	return result
}

// ConstantProp performs constant propagation
func (o *Optimizer) ConstantProp(expr ast.Expression) (ast.Expression, bool) {
	// Optimize: propagate known constant values
	switch e := expr.(type) {
	case ast.InfixExpression:
		left, leftOK := o.ConstantProp(e.Left)
		right, rightOK := o.ConstantProp(e.Right)

		if !leftOK || !rightOK {
			return expr, false
		}

		// Try to fold
		lNum, lOK := left.(ast.NumberLiteral)
		rNum, rOK := right.(ast.NumberLiteral)
		if !lOK || !rOK {
			return expr, false
		}

		var result float64
		switch e.Operator {
		case "+":
			result = lNum.Value + rNum.Value
		case "-":
			result = lNum.Value - rNum.Value
		case "*":
			result = lNum.Value * rNum.Value
		case "/":
			if rNum.Value == 0 {
				return expr, false
			}
			result = lNum.Value / rNum.Value
		case "%":
			if rNum.Value == 0 {
				return expr, false
			}
			result = math.Mod(lNum.Value, rNum.Value)
		default:
			return expr, false
		}

		return ast.NumberLiteral{Value: result}, true

	case ast.PrefixExpression:
		right, ok := o.ConstantProp(e.Right)
		if !ok {
			return expr, false
		}

		num, ok := right.(ast.NumberLiteral)
		if !ok {
			return expr, false
		}

		switch e.Operator {
		case "-":
			return ast.NumberLiteral{Value: -num.Value}, true
		case "!":
			// This would need to handle bool, so skip for now
		}
	}

	return expr, false
}

// LoopUnroll unrolls small loops (future optimization)
func (o *Optimizer) LoopUnroll(code []byte, maxIterations int) []byte {

	// This would require analyzing loop structure and duplicating body
	return code
}

// InlineSmallFunctions inlines small function calls
func (o *Optimizer) InlineSmallFunctions(fn *bytecode.Function, maxSize int) *bytecode.Function {

	// This requires tracking call sites and function definitions
	return fn
}
