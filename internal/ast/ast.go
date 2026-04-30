package ast

type Program struct {
	Statements []Statement
}

type Statement interface {
	stmt()
}

type Expression interface {
	expr()
}

type LetStatement struct {
	Name  string
	Value Expression
}

func (LetStatement) stmt() {}

type AssignStatement struct {
	Target Expression
	Value  Expression
}

func (AssignStatement) stmt() {}

type ReturnStatement struct {
	Value Expression
}

func (ReturnStatement) stmt() {}

type ExpressionStatement struct {
	Value Expression
}

func (ExpressionStatement) stmt() {}

type BlockStatement struct {
	Statements []Statement
}

func (BlockStatement) stmt() {}

type FunctionStatement struct {
	Name   string
	Params []string
	Body   BlockStatement
}

func (FunctionStatement) stmt() {}

type WhileStatement struct {
	Condition Expression
	Body      BlockStatement
}

func (WhileStatement) stmt() {}

type ForStatement struct {
	Name  string
	Start Expression
	End   Expression
	Step  Expression
	Body  BlockStatement
}

func (ForStatement) stmt() {}

type ImportStatement struct {
	Name string
}

func (ImportStatement) stmt() {}

type ClassStatement struct {
	Name      string
	SuperName string
	Methods   []FunctionStatement
}

func (ClassStatement) stmt() {}

type Identifier struct {
	Name string
}

func (Identifier) expr() {}

type NumberLiteral struct {
	Value float64
}

func (NumberLiteral) expr() {}

type StringLiteral struct {
	Value string
}

func (StringLiteral) expr() {}

type BoolLiteral struct {
	Value bool
}

func (BoolLiteral) expr() {}

type NullLiteral struct{}

func (NullLiteral) expr() {}

type ThisExpression struct{}

func (ThisExpression) expr() {}

type PrefixExpression struct {
	Operator string
	Right    Expression
}

func (PrefixExpression) expr() {}

type InfixExpression struct {
	Left     Expression
	Operator string
	Right    Expression
}

func (InfixExpression) expr() {}

type IfExpression struct {
	Condition   Expression
	Consequence BlockStatement
	Alternative *BlockStatement
}

func (IfExpression) expr() {}

type CallExpression struct {
	Callee    Expression
	Arguments []Expression
}

func (CallExpression) expr() {}

type ArrayLiteral struct {
	Elements []Expression
}

func (ArrayLiteral) expr() {}

type MapPair struct {
	Key   Expression
	Value Expression
}

type MapLiteral struct {
	Pairs []MapPair
}

func (MapLiteral) expr() {}

type IndexExpression struct {
	Left  Expression
	Index Expression
}

func (IndexExpression) expr() {}

type MemberExpression struct {
	Left     Expression
	Property string
}

func (MemberExpression) expr() {}

type NewExpression struct {
	Class     Expression
	Arguments []Expression
}

func (NewExpression) expr() {}
