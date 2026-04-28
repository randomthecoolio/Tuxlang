package compiler

import (
	"fmt"
	"math"

	"tuxlang/internal/ast"
	"tuxlang/internal/bytecode"
)

type symbolScope int

const (
	scopeGlobal symbolScope = iota
	scopeLocal
	scopeBuiltin
	scopeFree
)

type symbol struct {
	Name  string
	Scope symbolScope
	Index int
}

type symbolTable struct {
	outer          *symbolTable
	store          map[string]symbol
	numDefinitions int
	freeSymbols    []symbol
}

func newSymbolTable(outer *symbolTable) *symbolTable {
	st := &symbolTable{outer: outer, store: map[string]symbol{}}
	if outer == nil {
		for i, name := range builtinNames {
			st.store[name] = symbol{Name: name, Scope: scopeBuiltin, Index: i}
		}
	}
	return st
}

func (s *symbolTable) define(name string) symbol {
	scope := scopeGlobal
	if s.outer != nil {
		scope = scopeLocal
	}
	sym := symbol{Name: name, Scope: scope, Index: s.numDefinitions}
	s.store[name] = sym
	s.numDefinitions++
	return sym
}

func (s *symbolTable) defineFree(original symbol) symbol {
	s.freeSymbols = append(s.freeSymbols, original)
	sym := symbol{Name: original.Name, Scope: scopeFree, Index: len(s.freeSymbols) - 1}
	s.store[original.Name] = sym
	return sym
}

func (s *symbolTable) resolve(name string) (symbol, bool) {
	if sym, ok := s.store[name]; ok {
		return sym, true
	}
	if s.outer == nil {
		return symbol{}, false
	}
	sym, ok := s.outer.resolve(name)
	if !ok {
		return symbol{}, false
	}
	if sym.Scope == scopeGlobal || sym.Scope == scopeBuiltin {
		return sym, true
	}
	return s.defineFree(sym), true
}

type Compiler struct {
	code      []byte
	constants []any
	symbols   *symbolTable
	loopID    int
	isRoot    bool
}

type compiledFunction struct {
	fn          *bytecode.Function
	freeSymbols []symbol
}

func New() *Compiler {
	return &Compiler{symbols: newSymbolTable(nil), isRoot: true}
}

func (c *Compiler) Compile(program *ast.Program) (*bytecode.Function, error) {
	c.code = make([]byte, 0, 4096)
	c.constants = make([]any, 0, 512)
	for _, stmt := range program.Statements {
		if err := c.compileStatement(stmt); err != nil {
			return nil, err
		}
	}
	c.emit(bytecode.OpNull)
	c.emit(bytecode.OpReturn)
	return &bytecode.Function{
		Name:      "<main>",
		Code:      c.code,
		Constants: c.constants,
	}, nil
}

func (c *Compiler) compileStatement(stmt ast.Statement) error {
	switch node := stmt.(type) {
	case ast.LetStatement:
		if err := c.compileExpression(node.Value); err != nil {
			return err
		}
		sym := c.symbols.define(node.Name)
		c.emitStore(sym)
	case ast.AssignStatement:
		if err := c.compileAssignTarget(node.Target, node.Value); err != nil {
			return err
		}
	case ast.ReturnStatement:
		if err := c.compileExpression(node.Value); err != nil {
			return err
		}
		c.emit(bytecode.OpReturn)
	case ast.ImportStatement:
		c.emitConstant(&bytecode.NativeModule{Name: node.Name})
		sym := c.symbols.define(node.Name)
		c.emitStore(sym)
	case ast.ExpressionStatement:
		if err := c.compileExpression(node.Value); err != nil {
			return err
		}
		c.emit(bytecode.OpPop)
	case ast.FunctionStatement:
		sym := c.symbols.define(node.Name)
		compiled, err := c.compileFunction(node.Name, node.Params, node.Body, false)
		if err != nil {
			return err
		}
		c.emitClosure(compiled)
		c.emitStore(sym)
	case ast.ClassStatement:
		sym := c.symbols.define(node.Name)
		c.emitConstant(node.Name)
		if node.SuperName != "" {
			superSym, ok := c.symbols.resolve(node.SuperName)
			if !ok {
				return fmt.Errorf("superclass %q is not defined", node.SuperName)
			}
			c.emitLoad(superSym)
		} else {
			c.emit(bytecode.OpNull)
		}
		for _, method := range node.Methods {
			c.emitConstant(method.Name)
			compiled, err := c.compileFunction(node.Name+"."+method.Name, method.Params, method.Body, true)
			if err != nil {
				return err
			}
			c.emitClosure(compiled)
		}
		c.emit(bytecode.OpClass)
		c.emitUint16(len(node.Methods))
		c.emitStore(sym)
	case ast.ForStatement:
		if err := c.compileForStatement(node); err != nil {
			return err
		}
	case ast.WhileStatement:
		loopStart := len(c.code)
		if err := c.compileExpression(node.Condition); err != nil {
			return err
		}
		jumpIfFalsePos := c.emitJump(bytecode.OpJumpIfFalse)
		if err := c.compileBlock(node.Body); err != nil {
			return err
		}
		c.emit(bytecode.OpJump)
		c.emitUint16(loopStart)
		c.patchJump(jumpIfFalsePos)
	default:
		return fmt.Errorf("unsupported statement %T", node)
	}
	return nil
}

func (c *Compiler) compileForStatement(node ast.ForStatement) error {
	loopCompiler := &Compiler{
		code:      c.code,
		constants: c.constants,
		symbols:   newSymbolTable(c.symbols),
		loopID:    c.loopID,
		isRoot:    c.isRoot,
	}

	loopName := node.Name
	var startSym, endSym, stepSym symbol
	if c.isRoot {
		startSym = c.symbols.define(c.nextLoopTemp("start"))
		endSym = c.symbols.define(c.nextLoopTemp("end"))
		stepSym = c.symbols.define(c.nextLoopTemp("step"))
		loopCompiler.loopID = c.loopID
	} else {
		startSym = loopCompiler.symbols.define(loopCompiler.nextLoopTemp("start"))
		endSym = loopCompiler.symbols.define(loopCompiler.nextLoopTemp("end"))
		stepSym = loopCompiler.symbols.define(loopCompiler.nextLoopTemp("step"))
	}
	loopCompiler.symbols.store[loopName] = startSym
	loopCompiler.symbols.store[startSym.Name] = startSym
	if err := loopCompiler.compileExpression(node.Start); err != nil {
		return err
	}
	loopCompiler.emitStore(startSym)

	loopCompiler.symbols.store[endSym.Name] = endSym
	if err := loopCompiler.compileExpression(node.End); err != nil {
		return err
	}
	loopCompiler.emitStore(endSym)

	loopCompiler.symbols.store[stepSym.Name] = stepSym
	if node.Step != nil {
		if err := loopCompiler.compileExpression(node.Step); err != nil {
			return err
		}
	} else {
		loopCompiler.emitConstant(float64(1))
	}
	loopCompiler.emitStore(stepSym)

	// Guard against zero-step loops before entering the loop body.
	if err := loopCompiler.compileStatement(ast.ExpressionStatement{Value: ast.CallExpression{
		Callee: ast.Identifier{Name: "assert"},
		Arguments: []ast.Expression{
			ast.InfixExpression{
				Left:     ast.Identifier{Name: stepSym.Name},
				Operator: "!=",
				Right:    ast.NumberLiteral{Value: 0},
			},
			ast.StringLiteral{Value: "for loop step cannot be 0"},
		},
	}}); err != nil {
		return err
	}

	loopStart := len(loopCompiler.code)
	if err := loopCompiler.compileExpression(ast.InfixExpression{
		Left:     ast.Identifier{Name: stepSym.Name},
		Operator: ">=",
		Right:    ast.NumberLiteral{Value: 0},
	}); err != nil {
		return err
	}
	jumpToNegative := loopCompiler.emitJump(bytecode.OpJumpIfFalse)
	if err := loopCompiler.compileExpression(ast.InfixExpression{
		Left:     ast.Identifier{Name: loopName},
		Operator: "<=",
		Right:    ast.Identifier{Name: endSym.Name},
	}); err != nil {
		return err
	}
	jumpExitForward := loopCompiler.emitJump(bytecode.OpJumpIfFalse)
	jumpBody := loopCompiler.emitJump(bytecode.OpJump)
	loopCompiler.patchJump(jumpToNegative)
	if err := loopCompiler.compileExpression(ast.InfixExpression{
		Left:     ast.Identifier{Name: loopName},
		Operator: ">=",
		Right:    ast.Identifier{Name: endSym.Name},
	}); err != nil {
		return err
	}
	jumpExitNegative := loopCompiler.emitJump(bytecode.OpJumpIfFalse)
	loopCompiler.patchJump(jumpBody)

	if err := loopCompiler.compileBlock(node.Body); err != nil {
		return err
	}
	update := ast.AssignStatement{
		Target: ast.Identifier{Name: loopName},
		Value: ast.InfixExpression{
			Left:     ast.Identifier{Name: loopName},
			Operator: "+",
			Right:    ast.Identifier{Name: stepSym.Name},
		},
	}
	if err := loopCompiler.compileStatement(update); err != nil {
		return err
	}
	loopCompiler.emit(bytecode.OpJump)
	loopCompiler.emitUint16(loopStart)
	loopCompiler.patchJump(jumpExitForward)
	loopCompiler.patchJump(jumpExitNegative)

	c.code = loopCompiler.code
	c.constants = loopCompiler.constants
	c.loopID = loopCompiler.loopID
	return nil
}

func (c *Compiler) nextLoopTemp(kind string) string {
	name := fmt.Sprintf("__for_%s_%d", kind, c.loopID)
	c.loopID++
	return name
}

func (c *Compiler) compileAssignTarget(target ast.Expression, value ast.Expression) error {
	switch node := target.(type) {
	case ast.Identifier:
		sym, ok := c.symbols.resolve(node.Name)
		if !ok || sym.Scope == scopeBuiltin {
			return fmt.Errorf("variable %q is not defined. Declare it first with 'local %s = ...'", node.Name, node.Name)
		}
		if err := c.compileExpression(value); err != nil {
			return err
		}
		c.emitStore(sym)
	case ast.MemberExpression:
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		c.emitConstant(node.Property)
		if err := c.compileExpression(value); err != nil {
			return err
		}
		c.emit(bytecode.OpSetIndex)
	case ast.IndexExpression:
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		if err := c.compileExpression(node.Index); err != nil {
			return err
		}
		if err := c.compileExpression(value); err != nil {
			return err
		}
		c.emit(bytecode.OpSetIndex)
	case ast.ThisExpression:
		sym, ok := c.symbols.resolve("this")
		if !ok {
			return fmt.Errorf("'this' can only be assigned inside class methods")
		}
		if err := c.compileExpression(value); err != nil {
			return err
		}
		c.emitStore(sym)
	default:
		return fmt.Errorf("invalid assignment target %T", target)
	}
	return nil
}

func (c *Compiler) compileBlock(block ast.BlockStatement) error {
	for _, stmt := range block.Statements {
		if err := c.compileStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) compileExpression(expr ast.Expression) error {
	if folded, ok := c.foldConstants(expr); ok {
		return c.compileExpression(folded)
	}

	switch node := expr.(type) {
	case ast.NumberLiteral:
		c.emitConstant(node.Value)
	case ast.StringLiteral:
		c.emitConstant(node.Value)
	case ast.BoolLiteral:
		if node.Value {
			c.emit(bytecode.OpTrue)
		} else {
			c.emit(bytecode.OpFalse)
		}
	case ast.NullLiteral:
		c.emit(bytecode.OpNull)
	case ast.ThisExpression:
		sym, ok := c.symbols.resolve("this")
		if !ok {
			return fmt.Errorf("'this' can only be used inside class methods")
		}
		c.emitLoad(sym)
	case ast.Identifier:
		sym, ok := c.symbols.resolve(node.Name)
		if !ok {
			return fmt.Errorf("variable %q is not defined. Declare it first with 'local %s = ...'", node.Name, node.Name)
		}
		c.emitLoad(sym)
	case ast.ArrayLiteral:
		for _, elem := range node.Elements {
			if err := c.compileExpression(elem); err != nil {
				return err
			}
		}
		c.emit(bytecode.OpArray)
		c.emitUint16(len(node.Elements))
	case ast.MapLiteral:
		for _, pair := range node.Pairs {
			if err := c.compileExpression(pair.Key); err != nil {
				return err
			}
			if err := c.compileExpression(pair.Value); err != nil {
				return err
			}
		}
		c.emit(bytecode.OpMap)
		c.emitUint16(len(node.Pairs))
	case ast.IndexExpression:
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		if err := c.compileExpression(node.Index); err != nil {
			return err
		}
		c.emit(bytecode.OpIndex)
	case ast.MemberExpression:
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		c.emitConstant(node.Property)
		c.emit(bytecode.OpIndex)
	case ast.PrefixExpression:
		if err := c.compileExpression(node.Right); err != nil {
			return err
		}
		switch node.Operator {
		case "-":
			c.emit(bytecode.OpNegate)
		case "!":
			c.emit(bytecode.OpNot)
		default:
			return fmt.Errorf("unsupported prefix operator %q", node.Operator)
		}
	case ast.InfixExpression:
		if node.Operator == "&&" {
			return c.compileLogicalAnd(node.Left, node.Right)
		}
		if node.Operator == "||" {
			return c.compileLogicalOr(node.Left, node.Right)
		}
		if err := c.compileExpression(node.Left); err != nil {
			return err
		}
		if err := c.compileExpression(node.Right); err != nil {
			return err
		}
		switch node.Operator {
		case "+":
			c.emit(bytecode.OpAdd)
		case "-":
			c.emit(bytecode.OpSub)
		case "*":
			c.emit(bytecode.OpMul)
		case "/":
			c.emit(bytecode.OpDiv)
		case "%":
			c.emit(bytecode.OpMod)
		case "==":
			c.emit(bytecode.OpEqual)
		case "!=":
			c.emit(bytecode.OpNotEqual)
		case "<":
			c.emit(bytecode.OpLess)
		case "<=":
			c.emit(bytecode.OpLessEqual)
		case ">":
			c.emit(bytecode.OpGreater)
		case ">=":
			c.emit(bytecode.OpGreaterEqual)
		default:
			return fmt.Errorf("unsupported infix operator %q", node.Operator)
		}
	case ast.IfExpression:
		if err := c.compileExpression(node.Condition); err != nil {
			return err
		}
		jumpIfFalsePos := c.emitJump(bytecode.OpJumpIfFalse)
		if err := c.compileBlock(node.Consequence); err != nil {
			return err
		}
		c.emit(bytecode.OpNull)
		jumpPos := c.emitJump(bytecode.OpJump)
		c.patchJump(jumpIfFalsePos)
		if node.Alternative != nil {
			if err := c.compileBlock(*node.Alternative); err != nil {
				return err
			}
			c.emit(bytecode.OpNull)
		} else {
			c.emit(bytecode.OpNull)
		}
		c.patchJump(jumpPos)
	case ast.CallExpression:
		if err := c.compileExpression(node.Callee); err != nil {
			return err
		}
		for _, arg := range node.Arguments {
			if err := c.compileExpression(arg); err != nil {
				return err
			}
		}
		c.emit(bytecode.OpCall)
		c.emitByte(byte(len(node.Arguments)))
	case ast.NewExpression:
		if err := c.compileExpression(node.Class); err != nil {
			return err
		}
		for _, arg := range node.Arguments {
			if err := c.compileExpression(arg); err != nil {
				return err
			}
		}
		c.emit(bytecode.OpNew)
		c.emitByte(byte(len(node.Arguments)))
	default:
		return fmt.Errorf("unsupported expression %T", node)
	}
	return nil
}

func (c *Compiler) compileFunction(name string, params []string, body ast.BlockStatement, hasThis bool) (*compiledFunction, error) {
	child := &Compiler{
		constants: c.constants,
		symbols:   newSymbolTable(c.symbols),
		isRoot:    false,
	}
	for _, param := range params {
		child.symbols.define(param)
	}
	if hasThis {
		// Reserve `this` as an extra local slot after explicit parameters.
		child.symbols.define("this")
	}
	if err := child.compileBlock(body); err != nil {
		return nil, err
	}
	child.emit(bytecode.OpNull)
	child.emit(bytecode.OpReturn)
	c.constants = child.constants
	return &compiledFunction{
		fn: &bytecode.Function{
			Name:      name,
			Code:      child.code,
			Constants: child.constants,
			NumLocals: child.symbols.numDefinitions,
			NumParams: len(params),
		},
		freeSymbols: child.symbols.freeSymbols,
	}, nil
}

func (c *Compiler) emitClosure(compiled *compiledFunction) {
	index := len(c.constants)
	c.constants = append(c.constants, compiled.fn)
	c.emit(bytecode.OpClosure)
	c.emitUint16(index)
	c.emitByte(byte(len(compiled.freeSymbols)))
	for _, free := range compiled.freeSymbols {
		scopeByte := byte(0)
		switch free.Scope {
		case scopeLocal:
			scopeByte = 0
		case scopeFree:
			scopeByte = 1
		default:
			panic("invalid free symbol scope")
		}
		c.emitByte(scopeByte)
		c.emitUint16(free.Index)
	}
}

func (c *Compiler) compileLogicalAnd(left, right ast.Expression) error {
	if err := c.compileExpression(left); err != nil {
		return err
	}
	jumpIfFalse := c.emitJump(bytecode.OpJumpIfFalse)
	if err := c.compileExpression(right); err != nil {
		return err
	}
	jumpEnd := c.emitJump(bytecode.OpJump)
	c.patchJump(jumpIfFalse)
	c.emit(bytecode.OpFalse)
	c.patchJump(jumpEnd)
	return nil
}

func (c *Compiler) compileLogicalOr(left, right ast.Expression) error {
	if err := c.compileExpression(left); err != nil {
		return err
	}
	jumpIfFalse := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpTrue)
	jumpEnd := c.emitJump(bytecode.OpJump)
	c.patchJump(jumpIfFalse)
	if err := c.compileExpression(right); err != nil {
		return err
	}
	jumpIfFalseRight := c.emitJump(bytecode.OpJumpIfFalse)
	c.emit(bytecode.OpTrue)
	jumpEndRight := c.emitJump(bytecode.OpJump)
	c.patchJump(jumpIfFalseRight)
	c.emit(bytecode.OpFalse)
	c.patchJump(jumpEndRight)
	c.patchJump(jumpEnd)
	return nil
}

func (c *Compiler) foldConstants(expr ast.Expression) (ast.Expression, bool) {
	infix, ok := expr.(ast.InfixExpression)
	if !ok {
		return nil, false
	}
	switch infix.Operator {
	case "+":
		if l, ok := infix.Left.(ast.NumberLiteral); ok {
			if r, ok := infix.Right.(ast.NumberLiteral); ok {
				return ast.NumberLiteral{Value: l.Value + r.Value}, true
			}
		}
		if l, ok := infix.Left.(ast.StringLiteral); ok {
			if r, ok := infix.Right.(ast.StringLiteral); ok {
				return ast.StringLiteral{Value: l.Value + r.Value}, true
			}
		}
	case "-":
		if l, ok := infix.Left.(ast.NumberLiteral); ok {
			if r, ok := infix.Right.(ast.NumberLiteral); ok {
				return ast.NumberLiteral{Value: l.Value - r.Value}, true
			}
		}
	case "*":
		if l, ok := infix.Left.(ast.NumberLiteral); ok {
			if r, ok := infix.Right.(ast.NumberLiteral); ok {
				return ast.NumberLiteral{Value: l.Value * r.Value}, true
			}
		}
	case "/":
		if l, ok := infix.Left.(ast.NumberLiteral); ok {
			if r, ok := infix.Right.(ast.NumberLiteral); ok && r.Value != 0 {
				return ast.NumberLiteral{Value: l.Value / r.Value}, true
			}
		}
	case "%":
		if l, ok := infix.Left.(ast.NumberLiteral); ok {
			if r, ok := infix.Right.(ast.NumberLiteral); ok && r.Value != 0 {
				return ast.NumberLiteral{Value: math.Mod(l.Value, r.Value)}, true
			}
		}
	}
	return nil, false
}

func (c *Compiler) emit(op bytecode.Opcode) { c.code = append(c.code, byte(op)) }
func (c *Compiler) emitByte(value byte)     { c.code = append(c.code, value) }
func (c *Compiler) emitUint16(value int)    { c.code = append(c.code, byte(value>>8), byte(value)) }

func (c *Compiler) emitConstant(value any) {
	index := len(c.constants)
	c.constants = append(c.constants, value)
	c.emit(bytecode.OpConstant)
	c.emitUint16(index)
}

func (c *Compiler) emitLoad(sym symbol) {
	switch sym.Scope {
	case scopeGlobal:
		c.emit(bytecode.OpGetGlobal)
	case scopeLocal:
		c.emit(bytecode.OpGetLocal)
	case scopeBuiltin:
		c.emit(bytecode.OpGetBuiltin)
	case scopeFree:
		c.emit(bytecode.OpGetFree)
	}
	c.emitUint16(sym.Index)
}

func (c *Compiler) emitStore(sym symbol) {
	switch sym.Scope {
	case scopeGlobal:
		c.emit(bytecode.OpSetGlobal)
	case scopeLocal:
		c.emit(bytecode.OpSetLocal)
	case scopeFree:
		c.emit(bytecode.OpSetFree)
	default:
		panic("invalid store scope")
	}
	c.emitUint16(sym.Index)
}

func (c *Compiler) emitJump(op bytecode.Opcode) int {
	c.emit(op)
	pos := len(c.code)
	c.emitUint16(0)
	return pos
}

func (c *Compiler) patchJump(position int) {
	target := len(c.code)
	c.code[position] = byte(target >> 8)
	c.code[position+1] = byte(target)
}

var builtinNames = []string{"print", "len", "push", "type", "str", "num", "bool", "keys", "has", "coalesce", "assert", "range", "sum"}
