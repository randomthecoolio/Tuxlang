package parser

import (
	"fmt"
	"strconv"

	"tuxlang/internal/ast"
	"tuxlang/internal/lexer"
	"tuxlang/internal/token"
)

const (
	_ int = iota
	lowest
	logical
	equality
	comparison
	sum
	product
	prefix
	call
	index
)

var precedences = map[token.Type]int{
	token.Or:        logical,
	token.And:       logical,
	token.Eq:        equality,
	token.NotEq:     equality,
	token.Less:      comparison,
	token.LessEq:    comparison,
	token.Greater:   comparison,
	token.GreaterEq: comparison,
	token.Plus:      sum,
	token.Minus:     sum,
	token.Star:      product,
	token.Slash:     product,
	token.Percent:   product,
	token.LParen:    call,
	token.LBracket:  index,
	token.Dot:       index,
}

type (
	prefixParseFn func() (ast.Expression, error)
	infixParseFn  func(ast.Expression) (ast.Expression, error)
)

type Parser struct {
	l         *lexer.Lexer
	curToken  token.Token
	peekToken token.Token
	prefixFns map[token.Type]prefixParseFn
	infixFns  map[token.Type]infixParseFn
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{l: l}
	p.prefixFns = map[token.Type]prefixParseFn{
		token.Identifier: p.parseIdentifier,
		token.Number:     p.parseNumberLiteral,
		token.String:     p.parseStringLiteral,
		token.True:       p.parseBoolLiteral,
		token.False:      p.parseBoolLiteral,
		token.Null:       p.parseNullLiteral,
		token.This:       p.parseThisExpression,
		token.Bang:       p.parsePrefixExpression,
		token.Minus:      p.parsePrefixExpression,
		token.LParen:     p.parseGroupedExpression,
		token.If:         p.parseIfExpression,
		token.New:        p.parseNewExpression,
		token.LBracket:   p.parseArrayLiteral,
		token.LBrace:     p.parseMapLiteral,
	}
	p.infixFns = map[token.Type]infixParseFn{
		token.Plus:      p.parseInfixExpression,
		token.Minus:     p.parseInfixExpression,
		token.Star:      p.parseInfixExpression,
		token.Slash:     p.parseInfixExpression,
		token.Percent:   p.parseInfixExpression,
		token.Eq:        p.parseInfixExpression,
		token.NotEq:     p.parseInfixExpression,
		token.Less:      p.parseInfixExpression,
		token.LessEq:    p.parseInfixExpression,
		token.Greater:   p.parseInfixExpression,
		token.GreaterEq: p.parseInfixExpression,
		token.And:       p.parseInfixExpression,
		token.Or:        p.parseInfixExpression,
		token.LParen:    p.parseCallExpression,
		token.LBracket:  p.parseIndexExpression,
		token.Dot:       p.parseMemberExpression,
	}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) ParseProgram() (*ast.Program, error) {
	program := &ast.Program{}
	for p.curToken.Type != token.EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return nil, err
		}
		program.Statements = append(program.Statements, stmt)
		p.nextToken()
	}
	return program, nil
}

func (p *Parser) parseStatement() (ast.Statement, error) {
	switch p.curToken.Type {
	case token.Let:
		return p.parseLetStatement()
	case token.Return:
		return p.parseReturnStatement()
	case token.Fn:
		return p.parseFunctionStatement()
	case token.Import:
		return p.parseImportStatement()
	case token.For:
		return p.parseForStatement()
	case token.While:
		return p.parseWhileStatement()
	case token.Class:
		return p.parseClassStatement()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseLetStatement() (ast.Statement, error) {
	if !p.expectPeek(token.Identifier) {
		return nil, p.peekError(token.Identifier)
	}
	name := p.curToken.Literal
	if !p.expectPeek(token.Assign) {
		return nil, p.peekError(token.Assign)
	}
	p.nextToken()
	value, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type == token.Semicolon {
		p.nextToken()
	}
	return ast.LetStatement{Name: name, Value: value}, nil
}

func (p *Parser) parseReturnStatement() (ast.Statement, error) {
	p.nextToken()
	value, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type == token.Semicolon {
		p.nextToken()
	}
	return ast.ReturnStatement{Value: value}, nil
}

func (p *Parser) parseFunctionStatement() (ast.Statement, error) {
	if !p.expectPeek(token.Identifier) {
		return nil, p.peekError(token.Identifier)
	}
	name := p.curToken.Literal
	if !p.expectPeek(token.LParen) {
		return nil, p.peekError(token.LParen)
	}
	params, err := p.parseFunctionParameters()
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type == token.Do || p.peekToken.Type == token.LBrace {
		p.nextToken()
	}
	body, err := p.parseBlockStatement(token.End, token.RBrace)
	if err != nil {
		return nil, err
	}
	return ast.FunctionStatement{Name: name, Params: params, Body: body}, nil
}

func (p *Parser) parseWhileStatement() (ast.Statement, error) {
	p.nextToken()
	condition, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if err := p.expectBlockStart(token.Do, token.LBrace); err != nil {
		return nil, err
	}
	body, err := p.parseBlockStatement(token.End, token.RBrace)
	if err != nil {
		return nil, err
	}
	return ast.WhileStatement{Condition: condition, Body: body}, nil
}

func (p *Parser) parseForStatement() (ast.Statement, error) {
	if !p.expectPeek(token.Identifier) {
		return nil, p.peekError(token.Identifier)
	}
	name := p.curToken.Literal
	if !p.expectPeek(token.Assign) {
		return nil, p.peekError(token.Assign)
	}
	p.nextToken()
	start, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if !p.expectPeek(token.Comma) {
		return nil, p.peekError(token.Comma)
	}
	p.nextToken()
	end, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	var step ast.Expression
	if p.peekToken.Type == token.Comma {
		p.nextToken()
		p.nextToken()
		step, err = p.parseExpression(lowest)
		if err != nil {
			return nil, err
		}
	}
	if err := p.expectBlockStart(token.Do, token.LBrace); err != nil {
		return nil, err
	}
	body, err := p.parseBlockStatement(token.End, token.RBrace)
	if err != nil {
		return nil, err
	}
	return ast.ForStatement{Name: name, Start: start, End: end, Step: step, Body: body}, nil
}

func (p *Parser) parseImportStatement() (ast.Statement, error) {
	if !p.expectPeek(token.Identifier) {
		return nil, p.peekError(token.Identifier)
	}
	name := p.curToken.Literal
	if p.peekToken.Type == token.Semicolon {
		p.nextToken()
	}
	return ast.ImportStatement{Name: name}, nil
}

func (p *Parser) parseClassStatement() (ast.Statement, error) {
	if !p.expectPeek(token.Identifier) {
		return nil, p.peekError(token.Identifier)
	}
	stmt := ast.ClassStatement{Name: p.curToken.Literal}
	if p.peekToken.Type == token.Extends {
		p.nextToken()
		if !p.expectPeek(token.Identifier) {
			return nil, p.peekError(token.Identifier)
		}
		stmt.SuperName = p.curToken.Literal
	}
	p.nextToken()
	for p.curToken.Type != token.End && p.curToken.Type != token.EOF {
		if p.curToken.Type != token.Fn {
			return nil, fmt.Errorf("line %d:%d: expected function inside class, got %s", p.curToken.Line, p.curToken.Column, p.curToken.Type)
		}
		methodStmt, err := p.parseFunctionStatement()
		if err != nil {
			return nil, err
		}
		method, ok := methodStmt.(ast.FunctionStatement)
		if !ok {
			return nil, fmt.Errorf("line %d:%d: invalid class method", p.curToken.Line, p.curToken.Column)
		}
		stmt.Methods = append(stmt.Methods, method)
		p.nextToken()
	}
	if p.curToken.Type != token.End {
		return nil, fmt.Errorf("line %d:%d: expected end to close class", p.curToken.Line, p.curToken.Column)
	}
	return stmt, nil
}

func (p *Parser) parseExpressionStatement() (ast.Statement, error) {
	expr, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if p.peekToken.Type == token.Assign {
		switch expr.(type) {
		case ast.Identifier, ast.MemberExpression, ast.IndexExpression, ast.ThisExpression:
			p.nextToken()
			p.nextToken()
			value, err := p.parseExpression(lowest)
			if err != nil {
				return nil, err
			}
			if p.peekToken.Type == token.Semicolon {
				p.nextToken()
			}
			return ast.AssignStatement{Target: expr, Value: value}, nil
		}
	}
	if p.peekToken.Type == token.Semicolon {
		p.nextToken()
	}
	return ast.ExpressionStatement{Value: expr}, nil
}

func (p *Parser) parseBlockStatement(endTokens ...token.Type) (ast.BlockStatement, error) {
	block := ast.BlockStatement{}
	p.nextToken()
	for !p.isBlockEnd(p.curToken.Type, endTokens...) && p.curToken.Type != token.EOF {
		stmt, err := p.parseStatement()
		if err != nil {
			return block, err
		}
		block.Statements = append(block.Statements, stmt)
		p.nextToken()
	}
	return block, nil
}

func (p *Parser) parseExpression(precedence int) (ast.Expression, error) {
	prefixFn := p.prefixFns[p.curToken.Type]
	if prefixFn == nil {
		return nil, fmt.Errorf(
			"line %d:%d: I found %q where an expression should start. Try a number, string, variable name, or '(' expression ')'",
			p.curToken.Line, p.curToken.Column, p.curToken.Literal,
		)
	}
	left, err := prefixFn()
	if err != nil {
		return nil, err
	}

	for p.peekToken.Type != token.Semicolon && precedence < p.peekPrecedence() {
		infixFn := p.infixFns[p.peekToken.Type]
		if infixFn == nil {
			return left, nil
		}
		p.nextToken()
		left, err = infixFn(left)
		if err != nil {
			return nil, err
		}
	}
	return left, nil
}

func (p *Parser) parseIdentifier() (ast.Expression, error) {
	return ast.Identifier{Name: p.curToken.Literal}, nil
}

func (p *Parser) parseNumberLiteral() (ast.Expression, error) {
	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		return nil, fmt.Errorf("line %d:%d: %q is not a valid number", p.curToken.Line, p.curToken.Column, p.curToken.Literal)
	}
	return ast.NumberLiteral{Value: value}, nil
}

func (p *Parser) parseStringLiteral() (ast.Expression, error) {
	return ast.StringLiteral{Value: p.curToken.Literal}, nil
}

func (p *Parser) parseBoolLiteral() (ast.Expression, error) {
	return ast.BoolLiteral{Value: p.curToken.Type == token.True}, nil
}

func (p *Parser) parseNullLiteral() (ast.Expression, error) {
	return ast.NullLiteral{}, nil
}

func (p *Parser) parseThisExpression() (ast.Expression, error) {
	return ast.ThisExpression{}, nil
}

func (p *Parser) parsePrefixExpression() (ast.Expression, error) {
	expr := ast.PrefixExpression{Operator: p.curToken.Literal}
	p.nextToken()
	right, err := p.parseExpression(prefix)
	if err != nil {
		return nil, err
	}
	expr.Right = right
	return expr, nil
}

func (p *Parser) parseGroupedExpression() (ast.Expression, error) {
	p.nextToken()
	expr, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if !p.expectPeek(token.RParen) {
		return nil, p.peekError(token.RParen)
	}
	return expr, nil
}

func (p *Parser) parseIfExpression() (ast.Expression, error) {
	p.nextToken()
	condition, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if err := p.expectBlockStart(token.Then, token.LBrace); err != nil {
		return nil, err
	}
	consequence, err := p.parseBlockStatement(token.Else, token.End, token.RBrace)
	if err != nil {
		return nil, err
	}

	var alternative *ast.BlockStatement
	switch p.curToken.Type {
	case token.Else:
		if p.peekToken.Type == token.LBrace {
			p.nextToken()
		}
		block, err := p.parseBlockStatement(token.End, token.RBrace)
		if err != nil {
			return nil, err
		}
		alternative = &block
	case token.End, token.RBrace:
	default:
		return nil, fmt.Errorf("line %d:%d: expected 'else' or 'end' to close this 'if' block, but got %q", p.curToken.Line, p.curToken.Column, p.curToken.Literal)
	}

	return ast.IfExpression{
		Condition:   condition,
		Consequence: consequence,
		Alternative: alternative,
	}, nil
}

func (p *Parser) parseArrayLiteral() (ast.Expression, error) {
	elements, err := p.parseExpressionList(token.RBracket)
	if err != nil {
		return nil, err
	}
	return ast.ArrayLiteral{Elements: elements}, nil
}

func (p *Parser) parseMapLiteral() (ast.Expression, error) {
	pairs := []ast.MapPair{}
	if p.peekToken.Type == token.RBrace {
		p.nextToken()
		return ast.MapLiteral{Pairs: pairs}, nil
	}
	for {
		p.nextToken()
		key, err := p.parseExpression(lowest)
		if err != nil {
			return nil, err
		}
		if !p.expectPeek(token.Colon) {
			return nil, p.peekError(token.Colon)
		}
		p.nextToken()
		value, err := p.parseExpression(lowest)
		if err != nil {
			return nil, err
		}
		pairs = append(pairs, ast.MapPair{Key: key, Value: value})
		if p.peekToken.Type != token.Comma {
			break
		}
		p.nextToken()
	}
	if !p.expectPeek(token.RBrace) {
		return nil, p.peekError(token.RBrace)
	}
	return ast.MapLiteral{Pairs: pairs}, nil
}

func (p *Parser) parseInfixExpression(left ast.Expression) (ast.Expression, error) {
	expr := ast.InfixExpression{Left: left, Operator: p.curToken.Literal}
	precedence := p.curPrecedence()
	p.nextToken()
	right, err := p.parseExpression(precedence)
	if err != nil {
		return nil, err
	}
	expr.Right = right
	return expr, nil
}

func (p *Parser) parseCallExpression(function ast.Expression) (ast.Expression, error) {
	args, err := p.parseExpressionList(token.RParen)
	if err != nil {
		return nil, err
	}
	return ast.CallExpression{Callee: function, Arguments: args}, nil
}

func (p *Parser) parseIndexExpression(left ast.Expression) (ast.Expression, error) {
	p.nextToken()
	indexExpr, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	if !p.expectPeek(token.RBracket) {
		return nil, p.peekError(token.RBracket)
	}
	return ast.IndexExpression{Left: left, Index: indexExpr}, nil
}

func (p *Parser) parseMemberExpression(left ast.Expression) (ast.Expression, error) {
	if !p.expectPeek(token.Identifier) {
		return nil, p.peekError(token.Identifier)
	}
	return ast.MemberExpression{Left: left, Property: p.curToken.Literal}, nil
}

func (p *Parser) parseNewExpression() (ast.Expression, error) {
	p.nextToken()
	classExpr, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	callExpr, ok := classExpr.(ast.CallExpression)
	if !ok {
		return nil, fmt.Errorf("line %d:%d: new expects a constructor call", p.curToken.Line, p.curToken.Column)
	}
	return ast.NewExpression{Class: callExpr.Callee, Arguments: callExpr.Arguments}, nil
}

func (p *Parser) parseExpressionList(end token.Type) ([]ast.Expression, error) {
	items := []ast.Expression{}
	if p.peekToken.Type == end {
		p.nextToken()
		return items, nil
	}
	p.nextToken()
	first, err := p.parseExpression(lowest)
	if err != nil {
		return nil, err
	}
	items = append(items, first)
	for p.peekToken.Type == token.Comma {
		p.nextToken()
		p.nextToken()
		expr, err := p.parseExpression(lowest)
		if err != nil {
			return nil, err
		}
		items = append(items, expr)
	}
	if !p.expectPeek(end) {
		return nil, p.peekError(end)
	}
	return items, nil
}

func (p *Parser) parseFunctionParameters() ([]string, error) {
	params := []string{}
	if p.peekToken.Type == token.RParen {
		p.nextToken()
		return params, nil
	}
	p.nextToken()
	params = append(params, p.curToken.Literal)
	for p.peekToken.Type == token.Comma {
		p.nextToken()
		if !p.expectPeek(token.Identifier) {
			return nil, p.peekError(token.Identifier)
		}
		params = append(params, p.curToken.Literal)
	}
	if !p.expectPeek(token.RParen) {
		return nil, p.peekError(token.RParen)
	}
	return params, nil
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	tok, err := p.l.NextToken()
	if err != nil {
		p.peekToken = token.Token{
			Type:    token.Illegal,
			Literal: err.Error(),
			Line:    p.curToken.Line,
			Column:  p.curToken.Column,
		}
		return
	}
	p.peekToken = tok
}

func (p *Parser) expectPeek(t token.Type) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}
	return false
}

func (p *Parser) peekError(t token.Type) error {
	if p.peekToken.Type == token.Illegal {
		return fmt.Errorf("%s", p.peekToken.Literal)
	}
	return fmt.Errorf(
		"line %d:%d: expected %q next, but found %q",
		p.peekToken.Line, p.peekToken.Column, string(t), p.peekToken.Literal,
	)
}

func (p *Parser) expectBlockStart(keyword token.Type, brace token.Type) error {
	switch p.peekToken.Type {
	case keyword, brace:
		p.nextToken()
		return nil
	default:
		return fmt.Errorf(
			"line %d:%d: expected block start %q or %q, but found %q",
			p.peekToken.Line, p.peekToken.Column, string(keyword), string(brace), p.peekToken.Literal,
		)
	}
}

func (p *Parser) isBlockEnd(current token.Type, endTokens ...token.Type) bool {
	for _, end := range endTokens {
		if current == end {
			return true
		}
	}
	return false
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return lowest
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return lowest
}
