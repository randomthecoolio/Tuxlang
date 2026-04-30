package token

type Type string

const (
	EOF     Type = "EOF"
	Illegal Type = "ILLEGAL"

	Identifier Type = "IDENT"
	Number     Type = "NUMBER"
	String     Type = "STRING"

	Let     Type = "LET"
	Fn      Type = "FN"
	If      Type = "IF"
	Else    Type = "ELSE"
	Then    Type = "THEN"
	Do      Type = "DO"
	End     Type = "END"
	Import  Type = "IMPORT"
	For     Type = "FOR"
	While   Type = "WHILE"
	Return  Type = "RETURN"
	Class   Type = "CLASS"
	Extends Type = "EXTENDS"
	New     Type = "NEW"
	This    Type = "THIS"
	True    Type = "TRUE"
	False   Type = "FALSE"
	Null    Type = "NULL"

	Assign    Type = "="
	Plus      Type = "+"
	Minus     Type = "-"
	Star      Type = "*"
	Slash     Type = "/"
	Percent   Type = "%"
	Bang      Type = "!"
	Eq        Type = "=="
	NotEq     Type = "!="
	Less      Type = "<"
	LessEq    Type = "<="
	Greater   Type = ">"
	GreaterEq Type = ">="
	And       Type = "&&"
	Or        Type = "||"

	Comma     Type = ","
	Semicolon Type = ";"
	Colon     Type = ":"
	Dot       Type = "."

	LParen   Type = "("
	RParen   Type = ")"
	LBrace   Type = "{"
	RBrace   Type = "}"
	LBracket Type = "["
	RBracket Type = "]"
)

type Token struct {
	Type    Type
	Literal string
	Line    int
	Column  int
}

var keywords = map[string]Type{
	"let":      Let,
	"local":    Let,
	"fn":       Fn,
	"function": Fn,
	"if":       If,
	"else":     Else,
	"then":     Then,
	"do":       Do,
	"end":      End,
	"import":   Import,
	"for":      For,
	"while":    While,
	"return":   Return,
	"class":    Class,
	"extends":  Extends,
	"new":      New,
	"this":     This,
	"true":     True,
	"false":    False,
	"null":     Null,
	"nil":      Null,
	"and":      And,
	"or":       Or,
	"not":      Bang,
}

func LookupIdent(ident string) Type {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return Identifier
}
