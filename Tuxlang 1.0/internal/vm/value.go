package vm

// TaggedValue is a compact, typed value representation for the VM.
// It's introduced incrementally to reduce interface{} boxing on hot paths.
// For now it coexists with the existing `Value` alias and provides
// conversion helpers so we can migrate the stack/handlers safely.

import "fmt"

type ValueType byte

const (
	TypeNull ValueType = iota
	TypeNumber
	TypeString
	TypeBool
	TypeOther
)

type TaggedValue struct {
	typ   ValueType
	n     float64
	s     string
	b     bool
	other any
}

func NewNull() TaggedValue            { return TaggedValue{typ: TypeNull} }
func NewNumber(v float64) TaggedValue { return TaggedValue{typ: TypeNumber, n: v} }
func NewString(v string) TaggedValue  { return TaggedValue{typ: TypeString, s: v} }
func NewBool(v bool) TaggedValue      { return TaggedValue{typ: TypeBool, b: v} }
func NewOther(v any) TaggedValue      { return TaggedValue{typ: TypeOther, other: v} }

func (tv TaggedValue) Type() ValueType { return tv.typ }

func (tv TaggedValue) AsNumber() (float64, bool) {
	if tv.typ != TypeNumber {
		return 0, false
	}
	return tv.n, true
}

func (tv TaggedValue) AsString() (string, bool) {
	if tv.typ != TypeString {
		return "", false
	}
	return tv.s, true
}

func (tv TaggedValue) AsBool() (bool, bool) {
	if tv.typ != TypeBool {
		return false, false
	}
	return tv.b, true
}

func (tv TaggedValue) AsOther() (any, bool) {
	if tv.typ != TypeOther {
		return nil, false
	}
	return tv.other, true
}

// FromAny converts a generic value into a TaggedValue. Complex types
// are placed into the Other slot for now.
func FromAny(v any) TaggedValue {
	switch x := v.(type) {
	case nil:
		return NewNull()
	case float64:
		return NewNumber(x)
	case string:
		return NewString(x)
	case bool:
		return NewBool(x)
	default:
		return NewOther(v)
	}
}

// ToAny converts a TaggedValue back to interface{} for compatibility.
func (tv TaggedValue) ToAny() any {
	switch tv.typ {
	case TypeNull:
		return nil
	case TypeNumber:
		return tv.n
	case TypeString:
		return tv.s
	case TypeBool:
		return tv.b
	case TypeOther:
		return tv.other
	default:
		return fmt.Sprintf("<unknown:%d>", tv.typ)
	}
}
