package lexer

import (
	"fmt"
	"strings"
	"unicode"

	"tuxlang/internal/token"
)

type Lexer struct {
	input        []rune
	position     int
	readPosition int
	ch           rune
	line         int
	column       int
}

func New(input string) *Lexer {
	l := &Lexer{
		input: []rune(input),
		line:  1,
	}
	l.readChar()
	return l
}

func (l *Lexer) NextToken() (token.Token, error) {
	if err := l.skipWhitespaceAndComments(); err != nil {
		return token.Token{}, err
	}

	tok := token.Token{Literal: string(l.ch), Line: l.line, Column: l.column}
	switch l.ch {
	case 0:
		tok.Type = token.EOF
		tok.Literal = ""
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = token.Eq
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = token.Assign
		}
	case '+':
		tok.Type = token.Plus
	case '-':
		tok.Type = token.Minus
	case '*':
		tok.Type = token.Star
	case '/':
		tok.Type = token.Slash
	case '%':
		tok.Type = token.Percent
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = token.NotEq
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = token.Bang
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = token.LessEq
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = token.Less
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok.Type = token.GreaterEq
			tok.Literal = string(ch) + string(l.ch)
		} else {
			tok.Type = token.Greater
		}
	case '&':
		if l.peekChar() != '&' {
			return tok, fmt.Errorf("line %d:%d: unexpected '&'", l.line, l.column)
		}
		l.readChar()
		tok.Type = token.And
		tok.Literal = "&&"
	case '|':
		if l.peekChar() != '|' {
			return tok, fmt.Errorf("line %d:%d: unexpected '|'", l.line, l.column)
		}
		l.readChar()
		tok.Type = token.Or
		tok.Literal = "||"
	case ',':
		tok.Type = token.Comma
	case ';':
		tok.Type = token.Semicolon
	case ':':
		tok.Type = token.Colon
	case '.':
		tok.Type = token.Dot
	case '(':
		tok.Type = token.LParen
	case ')':
		tok.Type = token.RParen
	case '{':
		tok.Type = token.LBrace
	case '}':
		tok.Type = token.RBrace
	case '[':
		tok.Type = token.LBracket
	case ']':
		tok.Type = token.RBracket
	case '"':
		value, err := l.readString()
		if err != nil {
			return tok, err
		}
		tok.Type = token.String
		tok.Literal = value
		return tok, nil
	default:
		if isLetter(l.ch) {
			lit := l.readIdentifier()
			return token.Token{
				Type:    token.LookupIdent(lit),
				Literal: lit,
				Line:    tok.Line,
				Column:  tok.Column,
			}, nil
		}
		if unicode.IsDigit(l.ch) {
			return token.Token{
				Type:    token.Number,
				Literal: l.readNumber(),
				Line:    tok.Line,
				Column:  tok.Column,
			}, nil
		}
		return tok, fmt.Errorf("line %d:%d: unexpected character %q", l.line, l.column, l.ch)
	}

	l.readChar()
	return tok, nil
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
		l.position = l.readPosition
		return
	}

	l.position = l.readPosition
	l.ch = l.input[l.readPosition]
	l.readPosition++
	if l.ch == '\n' {
		l.line++
		l.column = 0
	} else {
		l.column++
	}
}

func (l *Lexer) peekChar() rune {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) skipWhitespaceAndComments() error {
	for {
		for unicode.IsSpace(l.ch) {
			l.readChar()
		}

		if l.ch == '/' && l.peekChar() == '/' {
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
			continue
		}

		if l.ch == '/' && l.peekChar() == '*' {
			l.readChar()
			l.readChar()
			for !(l.ch == '*' && l.peekChar() == '/') {
				if l.ch == 0 {
					return fmt.Errorf("line %d:%d: unterminated block comment", l.line, l.column)
				}
				l.readChar()
			}
			l.readChar()
			l.readChar()
			continue
		}

		return nil
	}
}

func (l *Lexer) readIdentifier() string {
	start := l.position
	// Optimize: inline character checks, avoid function calls in hot loop
	for {
		if !(l.ch == '_' || (l.ch >= 'a' && l.ch <= 'z') || (l.ch >= 'A' && l.ch <= 'Z') || (l.ch >= '0' && l.ch <= '9')) {
			break
		}
		l.readChar()
	}
	return string(l.input[start:l.position])
}

func (l *Lexer) readNumber() string {
	start := l.position
	dotSeen := false
	// Optimize: inline digit check, avoid function calls
	for {
		isDigit := l.ch >= '0' && l.ch <= '9'
		if !isDigit && (!(!dotSeen && l.ch == '.')) {
			break
		}
		if l.ch == '.' {
			dotSeen = true
		}
		l.readChar()
	}
	return string(l.input[start:l.position])
}

func (l *Lexer) readString() (string, error) {
	// Optimize: use direct byte buffer instead of strings.Builder for faster concatenation
	var b strings.Builder
	b.Grow(32) // Pre-allocate common string size
	for {
		l.readChar()
		switch l.ch {
		case 0:
			return "", fmt.Errorf("line %d:%d: unterminated string", l.line, l.column)
		case '"':
			l.readChar()
			return b.String(), nil
		case '\\':
			l.readChar()
			switch l.ch {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			}
		default:
			b.WriteRune(l.ch)
		}
	}
}

func isLetter(ch rune) bool {
	return ch == '_' || unicode.IsLetter(ch)
}
