package bytecode

type Opcode byte

const (
	// Basic opcodes
	OpConstant Opcode = iota
	OpNull
	OpTrue
	OpFalse
	OpPop

	// Optimized variable access (faster paths)
	OpGetGlobal
	OpSetGlobal
	OpGetLocal
	OpSetLocal
	OpGetFree
	OpSetFree
	OpGetBuiltin
	OpClosure

	// Collection operations
	OpArray
	OpMap
	OpIndex
	OpSetIndex
	OpClass
	OpNew

	// Arithmetic
	OpAdd
	OpSub
	OpMul
	OpDiv
	OpMod

	// Unary operations
	OpNegate
	OpNot

	// Comparison
	OpEqual
	OpNotEqual
	OpLess
	OpLessEqual
	OpGreater
	OpGreaterEqual

	// Control flow
	OpJump
	OpJumpIfFalse
	OpCall
	OpReturn

	// Combined operations for optimization
	OpConstantAdd  // Optimize: const + pop = single op
	OpLocalAddPush // Optimize: local + add + result handling
	OpAddImmediate // Optimize: add with small immediate
	OpSubImmediate // Optimize: subtract with small immediate
)

type Function struct {
	Name      string
	Code      []byte
	Constants []any
	NumLocals int
	NumParams int
}

type NativeModule struct {
	Name string
}
