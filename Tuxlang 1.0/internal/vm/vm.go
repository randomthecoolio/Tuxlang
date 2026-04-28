package vm

import (
	"bufio"
	cryptorand "crypto/rand"
	"crypto/sha256"
	stdcsv "encoding/csv"
	"encoding/base64"
	"encoding/hex"
	stdjson "encoding/json"
	"fmt"
	"io"
	"math"
	mathrand "math/rand"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"tuxlang/internal/bytecode"
	"tuxlang/internal/gui"
)

type opcodeHandler func(vm *VM, fr *frame) error

var opcodeHandlers [256]opcodeHandler

const (
	stackLimit   = 4096
	globalsLimit = 1024
	frameLimit   = 256
)

type Value = any

type frame struct {
	closure     *closureValue
	ip          int
	basePointer int
	returnThis  *instanceValue
}

func (fr *frame) fn() *bytecode.Function {
	return fr.closure.fn
}

type cell struct {
	value Value
}

type closureValue struct {
	fn   *bytecode.Function
	free []*cell
}

type classValue struct {
	name    string
	super   *classValue
	methods map[string]*closureValue
}

type instanceValue struct {
	class  *classValue
	fields map[hashKey]Value
}

type boundMethodValue struct {
	receiver *instanceValue
	method   *closureValue
}

type builtinFn func(args []Value) (Value, error)
type builtinValue struct {
	name string
	fn   builtinFn
}

var errTaskCanceled = fmt.Errorf("task canceled")

type moduleSpawnValue struct {
	ctx *VM
}
type moduleMapValue struct {
	ctx *VM
}
type taskValue struct {
	once       sync.Once
	cancelOnce sync.Once
	done       chan struct{}
	cancelCh   chan struct{}
	mu         sync.Mutex
	value      Value
	err        error
	canceled   bool
}

func newTaskValue() *taskValue {
	return &taskValue{done: make(chan struct{}), cancelCh: make(chan struct{})}
}

func (t *taskValue) complete(value Value, err error) {
	t.once.Do(func() {
		t.mu.Lock()
		if t.canceled && err == nil {
			err = errTaskCanceled
		}
		t.value = value
		t.err = err
		t.mu.Unlock()
		close(t.done)
	})
}

func (t *taskValue) requestCancel() bool {
	canceled := false
	t.cancelOnce.Do(func() {
		if t.isDone() {
			return
		}
		t.mu.Lock()
		t.canceled = true
		t.mu.Unlock()
		close(t.cancelCh)
		t.complete(nil, errTaskCanceled)
		canceled = true
	})
	return canceled
}

func (t *taskValue) isDone() bool {
	t.mu.Lock()
	canceled := t.canceled
	t.mu.Unlock()
	if canceled {
		return true
	}
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

func (t *taskValue) status() string {
	t.mu.Lock()
	canceled := t.canceled
	t.mu.Unlock()
	if canceled {
		return "canceled"
	}
	select {
	case <-t.done:
		if t.err != nil {
			return "failed"
		}
		return "done"
	default:
		return "pending"
	}
}

type nativeModuleValue struct {
	name    string
	members map[string]builtinValue
}

type VM struct {
	stack        [stackLimit]TaggedValue
	sp           int
	globals      [globalsLimit]Value
	frames       [frameLimit]frame
	fp           int
	profiling    bool
	opcodeCounts [256]int
	lastResult   Value
	cancel       <-chan struct{}
}

func New() *VM {
	return &VM{}
}

func (vm *VM) loadModule(name string) (Value, error) {
	if module, ok := standardModules[name]; ok {
		return module, nil
	}
	return nil, vm.runtimeError("module %q is not available; use a standard library import", name)
}

func init() {
	opcodeHandlers[bytecode.OpConstant] = handleOpConstant
	opcodeHandlers[bytecode.OpNull] = handleOpNull
	opcodeHandlers[bytecode.OpTrue] = handleOpTrue
	opcodeHandlers[bytecode.OpFalse] = handleOpFalse
	opcodeHandlers[bytecode.OpPop] = handleOpPop
	opcodeHandlers[bytecode.OpGetGlobal] = handleOpGetGlobal
	opcodeHandlers[bytecode.OpSetGlobal] = handleOpSetGlobal
	opcodeHandlers[bytecode.OpGetLocal] = handleOpGetLocal
	opcodeHandlers[bytecode.OpSetLocal] = handleOpSetLocal
	opcodeHandlers[bytecode.OpGetFree] = handleOpGetFree
	opcodeHandlers[bytecode.OpSetFree] = handleOpSetFree
	opcodeHandlers[bytecode.OpGetBuiltin] = handleOpGetBuiltin
	opcodeHandlers[bytecode.OpClosure] = handleOpClosure
	opcodeHandlers[bytecode.OpArray] = handleOpArray
	opcodeHandlers[bytecode.OpMap] = handleOpMap
	opcodeHandlers[bytecode.OpIndex] = handleOpIndex
	opcodeHandlers[bytecode.OpSetIndex] = handleOpSetIndex
	opcodeHandlers[bytecode.OpClass] = handleOpClass
	opcodeHandlers[bytecode.OpNew] = handleOpNew
	opcodeHandlers[bytecode.OpAdd] = handleOpAdd
	opcodeHandlers[bytecode.OpSub] = handleOpSub
	opcodeHandlers[bytecode.OpMul] = handleOpMul
	opcodeHandlers[bytecode.OpDiv] = handleOpDiv
	opcodeHandlers[bytecode.OpMod] = handleOpMod
	opcodeHandlers[bytecode.OpNegate] = handleOpNegate
	opcodeHandlers[bytecode.OpNot] = handleOpNot
	opcodeHandlers[bytecode.OpEqual] = handleOpEqual
	opcodeHandlers[bytecode.OpNotEqual] = handleOpNotEqual
	opcodeHandlers[bytecode.OpLess] = handleOpLess
	opcodeHandlers[bytecode.OpLessEqual] = handleOpLessEqual
	opcodeHandlers[bytecode.OpGreater] = handleOpGreater
	opcodeHandlers[bytecode.OpGreaterEqual] = handleOpGreaterEqual
	opcodeHandlers[bytecode.OpJump] = handleOpJump
	opcodeHandlers[bytecode.OpJumpIfFalse] = handleOpJumpIfFalse
	opcodeHandlers[bytecode.OpCall] = handleOpCall
	opcodeHandlers[bytecode.OpReturn] = handleOpReturn
}

func handleOpConstant(vm *VM, fr *frame) error {
	index := vm.readUint16(fr)
	if index < 0 || index >= len(fr.fn().Constants) {
		return vm.runtimeError("invalid constant index %d", index)
	}
	value := fr.fn().Constants[index]
	// Fast path: most constants are direct values, not NativeModule
	if module, ok := value.(*bytecode.NativeModule); ok {
		resolved, err := vm.loadModule(module.Name)
		if err != nil {
			return err
		}
		value = resolved
	}
	return vm.push(value)
}

func handleOpNull(vm *VM, fr *frame) error {
	return vm.push(nil)
}

func handleOpTrue(vm *VM, fr *frame) error {
	return vm.push(true)
}

func handleOpFalse(vm *VM, fr *frame) error {
	return vm.push(false)
}

func handleOpPop(vm *VM, fr *frame) error {
	_, err := vm.pop()
	return err
}

func handleOpGetGlobal(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	return vm.push(vm.globals[slot])
}

func handleOpSetGlobal(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	value, err := vm.pop()
	if err != nil {
		return err
	}
	vm.globals[slot] = value
	return nil
}

func handleOpGetLocal(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	return vm.push(vm.readLocal(fr.basePointer + slot))
}

func handleOpSetLocal(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	value, err := vm.pop()
	if err != nil {
		return err
	}
	vm.writeLocal(fr.basePointer+slot, value)
	return nil
}

func handleOpGetFree(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	if slot < 0 || slot >= len(fr.closure.free) {
		return vm.runtimeError("invalid closure slot %d", slot)
	}
	return vm.push(fr.closure.free[slot].value)
}

func handleOpSetFree(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	if slot < 0 || slot >= len(fr.closure.free) {
		return vm.runtimeError("invalid closure slot %d", slot)
	}
	value, err := vm.pop()
	if err != nil {
		return err
	}
	fr.closure.free[slot].value = value
	return nil
}

func handleOpGetBuiltin(vm *VM, fr *frame) error {
	slot := vm.readUint16(fr)
	if slot < 0 || slot >= len(builtins) {
		return vm.runtimeError("invalid builtin slot %d", slot)
	}
	return vm.push(builtins[slot])
}

func handleOpClosure(vm *VM, fr *frame) error {
	index := vm.readUint16(fr)
	count := int(fr.fn().Code[fr.ip])
	fr.ip++
	if index < 0 || index >= len(fr.fn().Constants) {
		return vm.runtimeError("invalid constant index %d", index)
	}
	rawFn, ok := fr.fn().Constants[index].(*bytecode.Function)
	if !ok {
		return vm.runtimeError("closure constant is not a function")
	}
	free := make([]*cell, count)
	for i := 0; i < count; i++ {
		sourceKind := fr.fn().Code[fr.ip]
		fr.ip++
		sourceIndex := vm.readUint16(fr)
		switch sourceKind {
		case 0:
			free[i] = vm.ensureLocalCell(fr.basePointer + sourceIndex)
		case 1:
			if sourceIndex < 0 || sourceIndex >= len(fr.closure.free) {
				return vm.runtimeError("invalid closure capture %d", sourceIndex)
			}
			free[i] = fr.closure.free[sourceIndex]
		default:
			return vm.runtimeError("invalid closure capture kind %d", sourceKind)
		}
	}
	return vm.push(&closureValue{fn: rawFn, free: free})
}

func handleOpArray(vm *VM, fr *frame) error {
	count := vm.readUint16(fr)
	if vm.sp < count {
		return vm.runtimeError("stack underflow")
	}
	// Fast: avoid allocation, reuse stack space
	base := vm.sp - count
	arr := make([]Value, count)
	for i := 0; i < count; i++ {
		arr[i] = vm.stack[base+i].ToAny()
	}
	vm.sp -= count
	return vm.push(arr)
}

func handleOpMap(vm *VM, fr *frame) error {
	count := vm.readUint16(fr)
	if vm.sp < count*2 {
		return vm.runtimeError("stack underflow")
	}
	// Pre-allocate map with exact capacity
	m := make(map[hashKey]Value, count)
	base := vm.sp - count*2
	for i := 0; i < count*2; i += 2 {
		keyValue := vm.stack[base+i].ToAny()
		value := vm.stack[base+i+1].ToAny()
		key, err := makeHashKey(keyValue)
		if err != nil {
			return vm.runtimeError("%v", err)
		}
		m[key] = value
	}
	vm.sp = base
	return vm.push(m)
}

func handleOpIndex(vm *VM, fr *frame) error {
	pop := vm.pop
	index, err := pop()
	if err != nil {
		return err
	}
	left, err := pop()
	if err != nil {
		return err
	}
	value, err := vm.index(left, index)
	if err != nil {
		return err
	}
	return vm.push(value)
}

func handleOpSetIndex(vm *VM, fr *frame) error {
	value, err := vm.pop()
	if err != nil {
		return err
	}
	index, err := vm.pop()
	if err != nil {
		return err
	}
	left, err := vm.pop()
	if err != nil {
		return err
	}
	if err := vm.setIndex(left, index, value); err != nil {
		return err
	}
	return nil
}

func handleOpAdd(vm *VM, fr *frame) error {
	// Optimize: inline type checking for hot path
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	rtv := vm.stack[vm.sp-1]
	ltv := vm.stack[vm.sp-2]
	if ln, ok := ltv.AsNumber(); ok {
		if rn, ok := rtv.AsNumber(); ok {
			vm.sp--
			vm.stack[vm.sp-1] = NewNumber(ln + rn)
			return nil
		}
	}
	if ls, ok := ltv.AsString(); ok {
		if rs, ok := rtv.AsString(); ok {
			vm.sp--
			vm.stack[vm.sp-1] = NewString(ls + rs)
			return nil
		}
	}
	return vm.runtimeError("type mismatch: %s and %s", typeName(ltv.ToAny()), typeName(rtv.ToAny()))
}

func handleOpSub(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	if ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]; true {
		if ln, ok := ltv.AsNumber(); ok {
			if rn, ok := rtv.AsNumber(); ok {
				vm.sp--
				vm.stack[vm.sp-1] = NewNumber(ln - rn)
				return nil
			}
		}
	}
	return vm.runtimeError("type mismatch in subtraction")
}

func handleOpMul(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	if ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]; true {
		if ln, ok := ltv.AsNumber(); ok {
			if rn, ok := rtv.AsNumber(); ok {
				vm.sp--
				vm.stack[vm.sp-1] = NewNumber(ln * rn)
				return nil
			}
		}
	}
	return vm.runtimeError("type mismatch in multiplication")
}

func handleOpDiv(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	if ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]; true {
		if ln, ok := ltv.AsNumber(); ok {
			if rn, ok := rtv.AsNumber(); ok {
				if rn == 0 {
					return vm.runtimeError("division by zero")
				}
				vm.sp--
				vm.stack[vm.sp-1] = NewNumber(ln / rn)
				return nil
			}
		}
	}
	return vm.runtimeError("type mismatch in division")
}

func handleOpMod(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	if ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]; true {
		if ln, ok := ltv.AsNumber(); ok {
			if rn, ok := rtv.AsNumber(); ok {
				if rn == 0 {
					return vm.runtimeError("modulo by zero")
				}
				vm.sp--
				vm.stack[vm.sp-1] = NewNumber(math.Mod(ln, rn))
				return nil
			}
		}
	}
	return vm.runtimeError("type mismatch in modulo")
}

func handleOpNegate(vm *VM, fr *frame) error {
	if vm.sp < 1 {
		return vm.runtimeError("stack underflow")
	}
	if num, ok := vm.stack[vm.sp-1].AsNumber(); ok {
		vm.stack[vm.sp-1] = NewNumber(-num)
		return nil
	}
	return vm.runtimeError("negation requires a number")
}

func handleOpNot(vm *VM, fr *frame) error {
	if vm.sp < 1 {
		return vm.runtimeError("stack underflow")
	}
	tv := vm.stack[vm.sp-1]
	switch tv.Type() {
	case TypeNull:
		vm.stack[vm.sp-1] = NewBool(true)
	case TypeBool:
		if b, _ := tv.AsBool(); b {
			vm.stack[vm.sp-1] = NewBool(false)
		} else {
			vm.stack[vm.sp-1] = NewBool(true)
		}
	case TypeNumber:
		if n, _ := tv.AsNumber(); n == 0 {
			vm.stack[vm.sp-1] = NewBool(true)
		} else {
			vm.stack[vm.sp-1] = NewBool(false)
		}
	case TypeString:
		if s, _ := tv.AsString(); s == "" {
			vm.stack[vm.sp-1] = NewBool(true)
		} else {
			vm.stack[vm.sp-1] = NewBool(false)
		}
	default:
		vm.stack[vm.sp-1] = NewBool(false)
	}
	return nil
}

func handleOpJumpIfFalse(vm *VM, fr *frame) error {
	if vm.sp < 1 {
		return vm.runtimeError("stack underflow")
	}
	target := vm.readUint16(fr)
	tv := vm.stack[vm.sp-1]
	// pop the condition value
	vm.sp--
	should_jump := false
	switch tv.Type() {
	case TypeNull:
		should_jump = true
	case TypeBool:
		if b, _ := tv.AsBool(); !b {
			should_jump = true
		}
	case TypeNumber:
		if n, _ := tv.AsNumber(); n == 0 {
			should_jump = true
		}
	case TypeString:
		if s, _ := tv.AsString(); s == "" {
			should_jump = true
		}
	}
	if should_jump {
		fr.ip = target
	}
	return nil
}

func handleOpEqual(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	ltv := vm.stack[vm.sp-2]
	rtv := vm.stack[vm.sp-1]
	if ln, lok := ltv.AsNumber(); lok {
		if rn, rok := rtv.AsNumber(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ln == rn)
			return nil
		}
	}
	if ls, lok := ltv.AsString(); lok {
		if rs, rok := rtv.AsString(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ls == rs)
			return nil
		}
	}
	// Fallback to deep equality on interface values
	lv := ltv.ToAny()
	rv := rtv.ToAny()
	vm.sp--
	vm.stack[vm.sp-1] = NewBool(lv == rv)
	return nil
}

func handleOpNotEqual(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	ltv := vm.stack[vm.sp-2]
	rtv := vm.stack[vm.sp-1]
	if ln, lok := ltv.AsNumber(); lok {
		if rn, rok := rtv.AsNumber(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ln != rn)
			return nil
		}
	}
	if ls, lok := ltv.AsString(); lok {
		if rs, rok := rtv.AsString(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ls != rs)
			return nil
		}
	}
	lv := ltv.ToAny()
	rv := rtv.ToAny()
	vm.sp--
	vm.stack[vm.sp-1] = NewBool(lv != rv)
	return nil
}

func handleOpLess(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]
	if ln, lok := ltv.AsNumber(); lok {
		if rn, rok := rtv.AsNumber(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ln < rn)
			return nil
		}
	}
	return vm.runtimeError("less-than comparison requires numbers, got %s and %s", typeName(ltv.ToAny()), typeName(rtv.ToAny()))
}

func handleOpLessEqual(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]
	if ln, lok := ltv.AsNumber(); lok {
		if rn, rok := rtv.AsNumber(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ln <= rn)
			return nil
		}
	}
	return vm.runtimeError("less-or-equal comparison requires numbers, got %s and %s", typeName(ltv.ToAny()), typeName(rtv.ToAny()))
}

func handleOpGreater(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]
	if ln, lok := ltv.AsNumber(); lok {
		if rn, rok := rtv.AsNumber(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ln > rn)
			return nil
		}
	}
	return vm.runtimeError("greater-than comparison requires numbers, got %s and %s", typeName(ltv.ToAny()), typeName(rtv.ToAny()))
}

func handleOpGreaterEqual(vm *VM, fr *frame) error {
	if vm.sp < 2 {
		return vm.runtimeError("stack underflow")
	}
	ltv, rtv := vm.stack[vm.sp-2], vm.stack[vm.sp-1]
	if ln, lok := ltv.AsNumber(); lok {
		if rn, rok := rtv.AsNumber(); rok {
			vm.sp--
			vm.stack[vm.sp-1] = NewBool(ln >= rn)
			return nil
		}
	}
	return vm.runtimeError("greater-or-equal comparison requires numbers, got %s and %s", typeName(ltv.ToAny()), typeName(rtv.ToAny()))
}

func handleOpJump(vm *VM, fr *frame) error {
	fr.ip = vm.readUint16(fr)
	return nil
}

func handleOpClass(vm *VM, fr *frame) error {
	count := vm.readUint16(fr)
	methods := make(map[string]*closureValue, count)
	for i := 0; i < count; i++ {
		methodAny, err := vm.pop()
		if err != nil {
			return err
		}
		nameAny, err := vm.pop()
		if err != nil {
			return err
		}
		name, ok := nameAny.(string)
		if !ok {
			return vm.runtimeError("class method name must be a string")
		}
		method, ok := methodAny.(*closureValue)
		if !ok {
			return vm.runtimeError("class method %q is not a function", name)
		}
		methods[name] = method
	}
	superAny, err := vm.pop()
	if err != nil {
		return err
	}
	nameAny, err := vm.pop()
	if err != nil {
		return err
	}
	name, ok := nameAny.(string)
	if !ok {
		return vm.runtimeError("class name must be a string")
	}
	var super *classValue
	if superAny != nil {
		var ok bool
		super, ok = superAny.(*classValue)
		if !ok {
			return vm.runtimeError("extends expects a class")
		}
	}
	return vm.push(&classValue{name: name, super: super, methods: methods})
}

func handleOpNew(vm *VM, fr *frame) error {
	argc := int(fr.fn().Code[fr.ip])
	fr.ip++
	sp := vm.sp
	if sp-1-argc < 0 {
		return vm.runtimeError("constructor stack is corrupted")
	}
	classAny := vm.stack[sp-1-argc].ToAny()
	class, ok := classAny.(*classValue)
	if !ok {
		return vm.runtimeError("new expects a class, got %s", typeName(classAny))
	}
	instance := &instanceValue{class: class, fields: map[hashKey]Value{}}
	initMethod := class.lookupMethod("init")
	if initMethod == nil {
		if argc != 0 {
			return vm.runtimeError("class %s has no init method", class.name)
		}
		vm.sp = sp - argc - 1
		return vm.push(instance)
	}
	return vm.invokeClosure(initMethod, argc, instance, instance)
}

func handleOpCall(vm *VM, fr *frame) error {
	argc := int(fr.fn().Code[fr.ip])
	fr.ip++
	return vm.call(argc)
}

func handleOpReturn(vm *VM, fr *frame) error {
	result, err := vm.pop()
	if err != nil {
		return err
	}
	// Clear frame slots so future calls don't accidentally reuse captured cells.
	for i := fr.basePointer - 1; i < vm.sp; i++ {
		if i >= 0 && i < stackLimit {
			vm.stack[i] = TaggedValue{}
		}
	}
	vm.sp = fr.basePointer - 1
	vm.fp--
	if fr.returnThis != nil {
		result = fr.returnThis
	}
	if vm.fp == 0 {
		vm.lastResult = result
		return nil // end execution
	}
	return vm.push(result)
}

func (vm *VM) Run(fn *bytecode.Function) (runErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = vm.runtimeError("internal runtime panic: %v", recovered)
		}
	}()
	vm.fp = 1
	vm.lastResult = nil
	vm.frames[0] = frame{closure: &closureValue{fn: fn}}
	return vm.runLoop()
}

func (vm *VM) runLoop() error {
	for vm.fp > 0 {
		if vm.cancel != nil {
			select {
			case <-vm.cancel:
				return vm.runtimeError("task canceled")
			default:
			}
		}
		fr := &vm.frames[vm.fp-1]
		if fr.ip >= len(fr.fn().Code) {
			vm.fp--
			continue
		}
		op := bytecode.Opcode(fr.fn().Code[fr.ip])
		if vm.profiling {
			vm.opcodeCounts[op]++
		}
		fr.ip++
		handler := opcodeHandlers[op]
		if handler == nil {
			return vm.runtimeError("unknown opcode %d", op)
		}
		if err := handler(vm, fr); err != nil {
			return err
		}
	}
	return nil
}

func (vm *VM) call(argc int) error {
	if vm.sp-1-argc < 0 {
		return vm.runtimeError("call stack is corrupted")
	}
	calleeAny := vm.stack[vm.sp-1-argc].ToAny()
	switch fn := calleeAny.(type) {
	case *closureValue:
		return vm.invokeClosure(fn, argc, nil, nil)
	case *bytecode.Function:
		return vm.invokeClosure(&closureValue{fn: fn}, argc, nil, nil)
	case *boundMethodValue:
		return vm.invokeClosure(fn.method, argc, fn.receiver, nil)
	case builtinValue:
		args := make([]Value, argc)
		for i := 0; i < argc; i++ {
			args[i] = vm.stack[vm.sp-argc+i].ToAny()
		}
		vm.sp = vm.sp - argc - 1
		result, err := fn.fn(args)
		if err != nil {
			return vm.runtimeError("%s failed: %v", fn.name, err)
		}
		return vm.push(result)
	case moduleSpawnValue:
		if argc < 1 {
			return vm.runtimeError("tuxies.spawn expects at least 1 argument")
		}
		callable := vm.stack[vm.sp-argc].ToAny()
		args := make([]Value, argc-1)
		for i := 0; i < argc-1; i++ {
			args[i] = vm.stack[vm.sp-argc+1+i].ToAny()
		}
		vm.sp = vm.sp - argc - 1
		task := spawnTask(fn.ctx, callable, args)
		return vm.push(task)
	case moduleMapValue:
		if argc != 2 {
			return vm.runtimeError("tuxies.map expects 2 arguments")
		}
		items, ok := vm.stack[vm.sp-argc].ToAny().([]Value)
		if !ok {
			return vm.runtimeError("tuxies.map expects an array")
		}
		callable := vm.stack[vm.sp-argc+1].ToAny()
		vm.sp = vm.sp - argc - 1
		results := make([]Value, len(items))
		tasks := make([]*taskValue, len(items))
		for i, item := range items {
			tasks[i] = spawnTask(fn.ctx, callable, []Value{item})
		}
		for i, task := range tasks {
			<-task.done
			if task.err != nil {
				return vm.runtimeError("tuxies.map task failed: %v", task.err)
			}
			results[i] = task.value
		}
		return vm.push(results)
	default:
		return vm.runtimeError("value is not callable: %s", typeName(calleeAny))
	}
}

func runCallableValue(ctx *VM, callable Value, args []Value, cancel <-chan struct{}) (Value, error) {
	switch fn := callable.(type) {
	case builtinValue:
		if cancel != nil {
			select {
			case <-cancel:
				return nil, errTaskCanceled
			default:
			}
		}
		return fn.fn(args)
	case *bytecode.Function:
		return runCallableOnFreshVM(ctx, &closureValue{fn: fn}, args, cancel)
	case *closureValue:
		return runCallableOnFreshVM(ctx, fn, args, cancel)
	case *boundMethodValue:
		return runCallableOnFreshVM(ctx, fn, args, cancel)
	default:
		return nil, fmt.Errorf("value is not callable: %s", typeName(callable))
	}
}

func runCallableOnFreshVM(ctx *VM, callable Value, args []Value, cancel <-chan struct{}) (Value, error) {
	worker := New()
	if ctx != nil {
		worker.globals = ctx.globals
	}
	worker.cancel = cancel
	if err := worker.push(callable); err != nil {
		return nil, err
	}
	for _, arg := range args {
		if err := worker.push(arg); err != nil {
			return nil, err
		}
	}
	if err := worker.call(len(args)); err != nil {
		return nil, err
	}
	if err := worker.runLoop(); err != nil {
		return nil, err
	}
	return worker.lastResult, nil
}

func spawnTask(ctx *VM, callable Value, args []Value) *taskValue {
	task := newTaskValue()
	copied := append([]Value(nil), args...)
	go func() {
		select {
		case <-task.cancelCh:
			task.complete(nil, errTaskCanceled)
			return
		default:
		}
		defer func() {
			if recovered := recover(); recovered != nil {
				task.complete(nil, fmt.Errorf("task panic: %v", recovered))
			}
		}()
		value, err := runCallableValue(ctx, callable, copied, task.cancelCh)
		task.complete(value, err)
	}()
	return task
}

func (vm *VM) invokeClosure(closure *closureValue, argc int, receiver *instanceValue, returnThis *instanceValue) error {
	fn := closure.fn
	if argc != fn.NumParams {
		return vm.runtimeError("%s expects %d arguments, got %d", fn.Name, fn.NumParams, argc)
	}
	if vm.fp >= frameLimit {
		return vm.runtimeError("too many nested function calls")
	}
	basePointer := vm.sp - argc
	if receiver != nil {
		thisSlot := basePointer + fn.NumParams
		vm.stack[thisSlot] = FromAny(receiver)
	}
	vm.frames[vm.fp] = frame{
		closure:     closure,
		basePointer: basePointer,
		returnThis:  returnThis,
	}
	vm.fp++
	vm.sp = basePointer + fn.NumLocals
	if vm.sp >= stackLimit {
		return vm.runtimeError("stack overflow")
	}
	return nil
}

func (vm *VM) index(left, index Value) (Value, error) {
	switch container := left.(type) {
	case nil:
		return nil, nil
	case []Value:
		idx, ok := index.(float64)
		if !ok {
			return nil, vm.runtimeError("array index must be a number, got %s", typeName(index))
		}
		i := int(idx)
		if float64(i) != idx {
			return nil, vm.runtimeError("array index must be an integer, got %v", idx)
		}
		if i < 0 || i >= len(container) {
			return nil, nil
		}
		return container[i], nil
	case map[hashKey]Value:
		key, err := makeHashKey(index)
		if err != nil {
			return nil, vm.runtimeError("%v", err)
		}
		return container[key], nil
	case nativeModuleValue:
		name, ok := index.(string)
		if !ok {
			return nil, vm.runtimeError("module member name must be a string, got %s", typeName(index))
		}
		if container.name == "tuxies" && name == "spawn" {
			return moduleSpawnValue{ctx: vm}, nil
		}
		if container.name == "tuxies" && name == "map" {
			return moduleMapValue{ctx: vm}, nil
		}
		member, ok := container.members[name]
		if !ok {
			return nil, vm.runtimeError("module %s has no member %q", container.name, name)
		}
		return member, nil
	case *instanceValue:
		name, ok := index.(string)
		if !ok {
			return nil, vm.runtimeError("object member name must be a string, got %s", typeName(index))
		}
		key := hashKey{kind: "string", value: name}
		if value, ok := container.fields[key]; ok {
			return value, nil
		}
		if method := container.class.lookupMethod(name); method != nil {
			return &boundMethodValue{receiver: container, method: method}, nil
		}
		return nil, nil
	case *classValue:
		name, ok := index.(string)
		if !ok {
			return nil, vm.runtimeError("class member name must be a string, got %s", typeName(index))
		}
		if method := container.lookupMethod(name); method != nil {
			return method, nil
		}
		return nil, nil
	case string:
		idx, ok := index.(float64)
		if !ok {
			return nil, vm.runtimeError("string index must be a number, got %s", typeName(index))
		}
		i := int(idx)
		if float64(i) != idx {
			return nil, vm.runtimeError("string index must be an integer, got %v", idx)
		}
		runes := []rune(container)
		if i < 0 || i >= len(runes) {
			return nil, nil
		}
		return string(runes[i]), nil
	default:
		return nil, vm.runtimeError("value is not indexable: %s", typeName(left))
	}
}

func (vm *VM) setIndex(left, index, value Value) error {
	switch container := left.(type) {
	case []Value:
		i, err := expectIndex(index)
		if err != nil {
			return vm.runtimeError("%v", err)
		}
		if i < 0 || i >= len(container) {
			return vm.runtimeError("array index out of range: %d", i)
		}
		container[i] = value
		return nil
	case map[hashKey]Value:
		key, err := makeHashKey(index)
		if err != nil {
			return vm.runtimeError("%v", err)
		}
		container[key] = value
		return nil
	case *instanceValue:
		name, ok := index.(string)
		if !ok {
			return vm.runtimeError("object member name must be a string, got %s", typeName(index))
		}
		container.fields[hashKey{kind: "string", value: name}] = value
		return nil
	default:
		return vm.runtimeError("value is not assignable by index: %s", typeName(left))
	}
}

func (vm *VM) ensureLocalCell(slot int) *cell {
	value := vm.stack[slot].ToAny()
	if existing, ok := value.(*cell); ok {
		return existing
	}
	c := &cell{value: value}
	vm.stack[slot] = FromAny(c)
	return c
}

func (vm *VM) readLocal(slot int) Value {
	value := vm.stack[slot].ToAny()
	if c, ok := value.(*cell); ok {
		return c.value
	}
	return value
}

func (vm *VM) writeLocal(slot int, value Value) {
	current := vm.stack[slot].ToAny()
	if c, ok := current.(*cell); ok {
		c.value = value
		return
	}
	vm.stack[slot] = FromAny(value)
}

func (c *classValue) lookupMethod(name string) *closureValue {
	for current := c; current != nil; current = current.super {
		if method, ok := current.methods[name]; ok {
			return method
		}
	}
	return nil
}

func (vm *VM) push(v Value) error {
	sp := vm.sp
	if sp >= stackLimit {
		return vm.runtimeError("stack overflow")
	}
	vm.stack[sp] = FromAny(v)
	vm.sp = sp + 1
	return nil
}

func (vm *VM) pop() (Value, error) {
	sp := vm.sp
	if sp == 0 {
		return nil, vm.runtimeError("stack underflow")
	}
	sp--
	tv := vm.stack[sp]
	// clear slot
	vm.stack[sp] = TaggedValue{}
	vm.sp = sp
	return tv.ToAny(), nil
}

func (vm *VM) peek() (Value, error) {
	sp := vm.sp
	if sp == 0 {
		return nil, vm.runtimeError("stack is empty")
	}
	return vm.stack[sp-1].ToAny(), nil
}

func (vm *VM) readUint16(fr *frame) int {
	ip := fr.ip
	code := fr.fn().Code
	value := int(code[ip])<<8 | int(code[ip+1])
	fr.ip = ip + 2
	return value
}

type hashKey struct {
	kind  string
	value string
}

func makeHashKey(v Value) (hashKey, error) {
	switch x := v.(type) {
	case string:
		return hashKey{kind: "string", value: x}, nil
	case float64:
		return hashKey{kind: "number", value: strconv.FormatFloat(x, 'g', -1, 64)}, nil
	case bool:
		return hashKey{kind: "bool", value: strconv.FormatBool(x)}, nil
	default:
		return hashKey{}, fmt.Errorf("invalid map key type %T", v)
	}
}

func isTruthy(v Value) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	default:
		return true
	}
}

func equals(left, right Value) bool {
	switch l := left.(type) {
	case nil:
		return right == nil
	case bool:
		r, ok := right.(bool)
		return ok && l == r
	}
	return left == right
}

var builtins = []builtinValue{
	{name: "print", fn: func(args []Value) (Value, error) {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = formatValue(arg)
		}
		fmt.Println(strings.Join(parts, " "))
		return nil, nil
	}},
	{name: "len", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("len expects 1 argument")
		}
		switch v := args[0].(type) {
		case nil:
			return float64(0), nil
		case string:
			return float64(len([]rune(v))), nil
		case []Value:
			return float64(len(v)), nil
		case map[hashKey]Value:
			return float64(len(v)), nil
		default:
			return nil, fmt.Errorf("len does not support %s", typeName(args[0]))
		}
	}},
	{name: "push", fn: func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("push expects 2 arguments")
		}
		if args[0] == nil {
			return []Value{args[1]}, nil
		}
		arr, ok := args[0].([]Value)
		if !ok {
			return nil, fmt.Errorf("push expects an array as the first argument")
		}
		out := make([]Value, len(arr)+1)
		copy(out, arr)
		out[len(arr)] = args[1]
		return out, nil
	}},
	{name: "type", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("type expects 1 argument")
		}
		return typeName(args[0]), nil
	}},
	{name: "str", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("str expects 1 argument")
		}
		return formatValue(args[0]), nil
	}},
	{name: "num", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("num expects 1 argument")
		}
		return toNumber(args[0])
	}},
	{name: "bool", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("bool expects 1 argument")
		}
		return isTruthy(args[0]), nil
	}},
	{name: "keys", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("keys expects 1 argument")
		}
		if args[0] == nil {
			return []Value{}, nil
		}
		m, ok := args[0].(map[hashKey]Value)
		if !ok {
			return nil, fmt.Errorf("keys expects a map")
		}
		out := make([]Value, 0, len(m))
		for key := range m {
			out = append(out, key.value)
		}
		return out, nil
	}},
	{name: "has", fn: func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nil, fmt.Errorf("has expects 2 arguments")
		}
		if args[0] == nil {
			return false, nil
		}
		switch container := args[0].(type) {
		case map[hashKey]Value:
			key, err := makeHashKey(args[1])
			if err != nil {
				return nil, err
			}
			_, ok := container[key]
			return ok, nil
		case []Value:
			idx, err := expectIndex(args[1])
			if err != nil {
				return nil, err
			}
			return idx >= 0 && idx < len(container), nil
		case string:
			idx, err := expectIndex(args[1])
			if err != nil {
				return nil, err
			}
			return idx >= 0 && idx < len([]rune(container)), nil
		default:
			return nil, fmt.Errorf("has expects a map, array, or string")
		}
	}},
	{name: "coalesce", fn: func(args []Value) (Value, error) {
		if len(args) == 0 {
			return nil, fmt.Errorf("coalesce expects at least 1 argument")
		}
		for _, arg := range args {
			if arg != nil {
				return arg, nil
			}
		}
		return nil, nil
	}},
	{name: "assert", fn: func(args []Value) (Value, error) {
		if len(args) != 1 && len(args) != 2 {
			return nil, fmt.Errorf("assert expects 1 or 2 arguments")
		}
		if isTruthy(args[0]) {
			return nil, nil
		}
		if len(args) == 2 {
			msg, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("assert message must be a string")
			}
			return nil, fmt.Errorf("assert failed: %s", msg)
		}
		return nil, fmt.Errorf("assert failed")
	}},
	{name: "range", fn: func(args []Value) (Value, error) {
		if len(args) != 2 && len(args) != 3 {
			return nil, fmt.Errorf("range expects 2 or 3 arguments")
		}
		start, ok := args[0].(float64)
		if !ok {
			return nil, fmt.Errorf("range expects numeric start")
		}
		end, ok := args[1].(float64)
		if !ok {
			return nil, fmt.Errorf("range expects numeric end")
		}
		step := float64(1)
		if len(args) == 3 {
			stepArg, ok := args[2].(float64)
			if !ok {
				return nil, fmt.Errorf("range expects numeric step")
			}
			if stepArg == 0 {
				return nil, fmt.Errorf("range step cannot be zero")
			}
			step = stepArg
		} else if start > end {
			step = -1
		}
		out := []Value{}
		if step > 0 {
			for n := start; n < end; n += step {
				out = append(out, n)
			}
		} else {
			for n := start; n > end; n += step {
				out = append(out, n)
			}
		}
		return out, nil
	}},
	{name: "sum", fn: func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nil, fmt.Errorf("sum expects 1 argument")
		}
		arr, ok := args[0].([]Value)
		if !ok {
			return nil, fmt.Errorf("sum expects an array")
		}
		total := float64(0)
		for _, item := range arr {
			n, ok := item.(float64)
			if !ok {
				return nil, fmt.Errorf("sum expects only numbers")
			}
			total += n
		}
		return total, nil
	}},
}

func toNumber(v Value) (Value, error) {
	switch x := v.(type) {
	case nil:
		return float64(0), nil
	case float64:
		return x, nil
	case bool:
		if x {
			return float64(1), nil
		}
		return float64(0), nil
	case string:
		value, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return nil, fmt.Errorf("num could not parse %q", x)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("num does not support %s", typeName(v))
	}
}

func expectIndex(v Value) (int, error) {
	number, ok := v.(float64)
	if !ok {
		return 0, fmt.Errorf("index must be a number")
	}
	index := int(number)
	if float64(index) != number {
		return 0, fmt.Errorf("index must be an integer")
	}
	return index, nil
}

func typeName(v Value) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "bool"
	case float64:
		return "number"
	case string:
		return "string"
	case []Value:
		return "array"
	case map[hashKey]Value:
		return "map"
	case *closureValue:
		return "function"
	case *bytecode.Function:
		return "function"
	case *boundMethodValue:
		return "method"
	case *classValue:
		return "class"
	case *instanceValue:
		return "object"
	case *taskValue:
		return "task"
	case moduleSpawnValue:
		return "builtin"
	case moduleMapValue:
		return "builtin"
	case builtinValue:
		return "builtin"
	case nativeModuleValue:
		return "module"
	default:
		return "unknown"
	}
}

func (vm *VM) runtimeError(format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	frames := make([]string, 0, vm.fp)
	for i := vm.fp - 1; i >= 0; i-- {
		name := vm.frames[i].fn().Name
		if name == "" {
			name = "<anonymous>"
		}
		frames = append(frames, name)
	}
	if len(frames) == 0 {
		return fmt.Errorf("%s", message)
	}
	return fmt.Errorf("%s [stack: %s]", message, strings.Join(frames, " -> "))
}

func formatValue(v Value) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	case []Value:
		parts := make([]string, len(x))
		for i, item := range x {
			parts[i] = formatValue(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[hashKey]Value:
		parts := make([]string, 0, len(x))
		for key, value := range x {
			parts = append(parts, key.value+": "+formatValue(value))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *closureValue:
		return "<fn " + x.fn.Name + ">"
	case *bytecode.Function:
		return "<fn " + x.Name + ">"
	case *boundMethodValue:
		return "<bound-method>"
	case *classValue:
		return "<class " + x.name + ">"
	case *instanceValue:
		return "<object " + x.class.name + ">"
	case *taskValue:
		select {
		case <-x.done:
			if x.err != nil {
				return "<task failed>"
			}
			return "<task done>"
		default:
			return "<task pending>"
		}
	case moduleSpawnValue:
		return "<builtin tuxies.spawn>"
	case moduleMapValue:
		return "<builtin tuxies.map>"
	case nativeModuleValue:
		return "<module " + x.name + ">"
	default:
		return fmt.Sprintf("%v", x)
	}
}

func namedBuiltin(name string, fn builtinFn) builtinValue {
	return builtinValue{name: name, fn: fn}
}

func jsonMarshalValue(v Value) (any, error) {
	switch x := v.(type) {
	case nil, bool, string, float64:
		return x, nil
	case []Value:
		out := make([]any, len(x))
		for i, item := range x {
			converted, err := jsonMarshalValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = converted
		}
		return out, nil
	case map[hashKey]Value:
		out := make(map[string]any, len(x))
		for key, item := range x {
			converted, err := jsonMarshalValue(item)
			if err != nil {
				return nil, err
			}
			out[key.value] = converted
		}
		return out, nil
	default:
		return nil, fmt.Errorf("json does not support %s", typeName(v))
	}
}

func jsonUnmarshalValue(v any) Value {
	switch x := v.(type) {
	case nil:
		return nil
	case bool, string:
		return x
	case float64:
		return x
	case []any:
		out := make([]Value, len(x))
		for i, item := range x {
			out[i] = jsonUnmarshalValue(item)
		}
		return out
	case map[string]any:
		out := make(map[hashKey]Value, len(x))
		for key, item := range x {
			out[hashKey{kind: "string", value: key}] = jsonUnmarshalValue(item)
		}
		return out
	default:
		return x
	}
}

func requireStringPathArg(name string, value Value) (string, error) {
	s, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s expects a string path", name)
	}
	return s, nil
}

func requireOneArg(name string, args []Value) Value {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathIsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

var standardModules = map[string]nativeModuleValue{
	"math": {
		name: "math",
		members: map[string]builtinValue{
			"abs": namedBuiltin("math.abs", func(args []Value) (Value, error) {
				n, err := requireNumberArg("math.abs", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return math.Abs(n), nil
			}),
			"sqrt": namedBuiltin("math.sqrt", func(args []Value) (Value, error) {
				n, err := requireNumberArg("math.sqrt", args, 1, 0)
				if err != nil {
					return nil, err
				}
				if n < 0 {
					return nil, fmt.Errorf("sqrt expects a non-negative number")
				}
				return math.Sqrt(n), nil
			}),
			"floor": namedBuiltin("math.floor", func(args []Value) (Value, error) {
				n, err := requireNumberArg("math.floor", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return math.Floor(n), nil
			}),
			"ceil": namedBuiltin("math.ceil", func(args []Value) (Value, error) {
				n, err := requireNumberArg("math.ceil", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return math.Ceil(n), nil
			}),
			"round": namedBuiltin("math.round", func(args []Value) (Value, error) {
				n, err := requireNumberArg("math.round", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return math.Round(n), nil
			}),
			"pow": namedBuiltin("math.pow", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("math.pow expects 2 arguments")
				}
				base, ok := args[0].(float64)
				if !ok {
					return nil, fmt.Errorf("math.pow expects numbers")
				}
				exp, ok := args[1].(float64)
				if !ok {
					return nil, fmt.Errorf("math.pow expects numbers")
				}
				return math.Pow(base, exp), nil
			}),
			"min": namedBuiltin("math.min", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("math.min expects 2 arguments")
				}
				a, ok := args[0].(float64)
				if !ok {
					return nil, fmt.Errorf("math.min expects numbers")
				}
				b, ok := args[1].(float64)
				if !ok {
					return nil, fmt.Errorf("math.min expects numbers")
				}
				return math.Min(a, b), nil
			}),
			"max": namedBuiltin("math.max", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("math.max expects 2 arguments")
				}
				a, ok := args[0].(float64)
				if !ok {
					return nil, fmt.Errorf("math.max expects numbers")
				}
				b, ok := args[1].(float64)
				if !ok {
					return nil, fmt.Errorf("math.max expects numbers")
				}
				return math.Max(a, b), nil
			}),
		},
	},
	"text": {
		name: "text",
		members: map[string]builtinValue{
			"upper": namedBuiltin("text.upper", func(args []Value) (Value, error) {
				s, err := requireStringArg("text.upper", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return strings.ToUpper(s), nil
			}),
			"lower": namedBuiltin("text.lower", func(args []Value) (Value, error) {
				s, err := requireStringArg("text.lower", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return strings.ToLower(s), nil
			}),
			"trim": namedBuiltin("text.trim", func(args []Value) (Value, error) {
				s, err := requireStringArg("text.trim", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return strings.TrimSpace(s), nil
			}),
			"contains": namedBuiltin("text.contains", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("text.contains expects 2 arguments")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("text.contains expects strings")
				}
				sub, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("text.contains expects strings")
				}
				return strings.Contains(s, sub), nil
			}),
			"replace": namedBuiltin("text.replace", func(args []Value) (Value, error) {
				if len(args) != 3 {
					return nil, fmt.Errorf("text.replace expects 3 arguments")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("text.replace expects strings")
				}
				oldValue, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("text.replace expects strings")
				}
				newValue, ok := args[2].(string)
				if !ok {
					return nil, fmt.Errorf("text.replace expects strings")
				}
				return strings.ReplaceAll(s, oldValue, newValue), nil
			}),
			"split": namedBuiltin("text.split", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("text.split expects 2 arguments")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("text.split expects strings")
				}
				sep, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("text.split expects strings")
				}
				parts := strings.Split(s, sep)
				out := make([]Value, len(parts))
				for i, part := range parts {
					out[i] = part
				}
				return out, nil
			}),
			"startsWith": namedBuiltin("text.startsWith", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("text.startsWith expects 2 arguments")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("text.startsWith expects strings")
				}
				prefix, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("text.startsWith expects strings")
				}
				return strings.HasPrefix(s, prefix), nil
			}),
			"endsWith": namedBuiltin("text.endsWith", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("text.endsWith expects 2 arguments")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("text.endsWith expects strings")
				}
				suffix, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("text.endsWith expects strings")
				}
				return strings.HasSuffix(s, suffix), nil
			}),
		},
	},
	"array": {
		name: "array",
		members: map[string]builtinValue{
			"join": namedBuiltin("array.join", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("array.join expects 2 arguments")
				}
				arr, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("array.join expects an array")
				}
				sep, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("array.join expects a string separator")
				}
				parts := make([]string, len(arr))
				for i, item := range arr {
					parts[i] = formatValue(item)
				}
				return strings.Join(parts, sep), nil
			}),
			"at": namedBuiltin("array.at", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("array.at expects 2 arguments")
				}
				arr, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("array.at expects an array")
				}
				idx, err := expectIndex(args[1])
				if err != nil {
					return nil, err
				}
				if idx < 0 || idx >= len(arr) {
					return nil, nil
				}
				return arr[idx], nil
			}),
			"slice": namedBuiltin("array.slice", func(args []Value) (Value, error) {
				if len(args) != 3 {
					return nil, fmt.Errorf("array.slice expects 3 arguments")
				}
				arr, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("array.slice expects an array")
				}
				start, err := expectIndex(args[1])
				if err != nil {
					return nil, err
				}
				end, err := expectIndex(args[2])
				if err != nil {
					return nil, err
				}
				if start < 0 {
					start = 0
				}
				if end > len(arr) {
					end = len(arr)
				}
				if start > end {
					start = end
				}
				out := make([]Value, end-start)
				copy(out, arr[start:end])
				return out, nil
			}),
			"reverse": namedBuiltin("array.reverse", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("array.reverse expects 1 argument")
				}
				arr, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("array.reverse expects an array")
				}
				out := make([]Value, len(arr))
				copy(out, arr)
				for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
					out[i], out[j] = out[j], out[i]
				}
				return out, nil
			}),
		},
	},
	"maplib": {
		name: "maplib",
		members: map[string]builtinValue{
			"get": namedBuiltin("maplib.get", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("maplib.get expects 2 arguments")
				}
				if args[0] == nil {
					return nil, nil
				}
				m, ok := args[0].(map[hashKey]Value)
				if !ok {
					return nil, fmt.Errorf("maplib.get expects a map")
				}
				key, err := makeHashKey(args[1])
				if err != nil {
					return nil, err
				}
				return m[key], nil
			}),
			"size": namedBuiltin("maplib.size", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("maplib.size expects 1 argument")
				}
				if args[0] == nil {
					return float64(0), nil
				}
				m, ok := args[0].(map[hashKey]Value)
				if !ok {
					return nil, fmt.Errorf("maplib.size expects a map")
				}
				return float64(len(m)), nil
			}),
			"set": namedBuiltin("maplib.set", func(args []Value) (Value, error) {
				if len(args) != 3 {
					return nil, fmt.Errorf("maplib.set expects 3 arguments")
				}
				if args[0] == nil {
					args[0] = map[hashKey]Value{}
				}
				m, ok := args[0].(map[hashKey]Value)
				if !ok {
					return nil, fmt.Errorf("maplib.set expects a map")
				}
				key, err := makeHashKey(args[1])
				if err != nil {
					return nil, err
				}
				m[key] = args[2]
				return m, nil
			}),
			"delete": namedBuiltin("maplib.delete", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("maplib.delete expects 2 arguments")
				}
				if args[0] == nil {
					return nil, nil
				}
				m, ok := args[0].(map[hashKey]Value)
				if !ok {
					return nil, fmt.Errorf("maplib.delete expects a map")
				}
				key, err := makeHashKey(args[1])
				if err != nil {
					return nil, err
				}
				delete(m, key)
				return m, nil
			}),
		},
	},
	"rand": {
		name: "rand",
		members: map[string]builtinValue{
			"float": namedBuiltin("rand.float", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("rand.float expects 0 arguments")
				}
				return mathrand.Float64(), nil
			}),
			"int": namedBuiltin("rand.int", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("rand.int expects 2 arguments")
				}
				minValue, err := requireIntArg("rand.int", args[0])
				if err != nil {
					return nil, err
				}
				maxValue, err := requireIntArg("rand.int", args[1])
				if err != nil {
					return nil, err
				}
				if maxValue <= minValue {
					return nil, fmt.Errorf("rand.int expects max > min")
				}
				return float64(mathrand.Intn(maxValue-minValue) + minValue), nil
			}),
			"pick": namedBuiltin("rand.pick", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("rand.pick expects 1 argument")
				}
				arr, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("rand.pick expects an array")
				}
				if len(arr) == 0 {
					return nil, nil
				}
				return arr[mathrand.Intn(len(arr))], nil
			}),
		},
	},
	"base64": {
		name: "base64",
		members: map[string]builtinValue{
			"encode": namedBuiltin("base64.encode", func(args []Value) (Value, error) {
				s, err := requireStringArg("base64.encode", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return base64.StdEncoding.EncodeToString([]byte(s)), nil
			}),
			"decode": namedBuiltin("base64.decode", func(args []Value) (Value, error) {
				s, err := requireStringArg("base64.decode", args, 1, 0)
				if err != nil {
					return nil, err
				}
				out, err := base64.StdEncoding.DecodeString(s)
				if err != nil {
					return nil, err
				}
				return string(out), nil
			}),
		},
	},
	"hash": {
		name: "hash",
		members: map[string]builtinValue{
			"sha256": namedBuiltin("hash.sha256", func(args []Value) (Value, error) {
				s, err := requireStringArg("hash.sha256", args, 1, 0)
				if err != nil {
					return nil, err
				}
				sum := sha256.Sum256([]byte(s))
				return hex.EncodeToString(sum[:]), nil
			}),
		},
	},
	"url": {
		name: "url",
		members: map[string]builtinValue{
			"encode": namedBuiltin("url.encode", func(args []Value) (Value, error) {
				s, err := requireStringArg("url.encode", args, 1, 0)
				if err != nil {
					return nil, err
				}
				return neturl.QueryEscape(s), nil
			}),
			"decode": namedBuiltin("url.decode", func(args []Value) (Value, error) {
				s, err := requireStringArg("url.decode", args, 1, 0)
				if err != nil {
					return nil, err
				}
				value, err := neturl.QueryUnescape(s)
				if err != nil {
					return nil, err
				}
				return value, nil
			}),
			"queryGet": namedBuiltin("url.queryGet", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("url.queryGet expects 2 arguments")
				}
				raw, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("url.queryGet expects string input")
				}
				key, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("url.queryGet expects string key")
				}
				if strings.Contains(raw, "://") {
					parsed, err := neturl.Parse(raw)
					if err != nil {
						return nil, err
					}
					raw = parsed.RawQuery
				}
				values, err := neturl.ParseQuery(raw)
				if err != nil {
					return nil, err
				}
				value := values.Get(key)
				if value == "" {
					return nil, nil
				}
				return value, nil
			}),
		},
	},
	"uuid": {
		name: "uuid",
		members: map[string]builtinValue{
			"v4": namedBuiltin("uuid.v4", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("uuid.v4 expects 0 arguments")
				}
				b := make([]byte, 16)
				if _, err := cryptorand.Read(b); err != nil {
					return nil, err
				}
				b[6] = (b[6] & 0x0f) | 0x40
				b[8] = (b[8] & 0x3f) | 0x80
				hexValue := hex.EncodeToString(b)
				return fmt.Sprintf(
					"%s-%s-%s-%s-%s",
					hexValue[0:8],
					hexValue[8:12],
					hexValue[12:16],
					hexValue[16:20],
					hexValue[20:32],
				), nil
			}),
		},
	},
	"conv": {
		name: "conv",
		members: map[string]builtinValue{
			"str":  namedBuiltin("conv.str", func(args []Value) (Value, error) { return builtinByName("str").fn(args) }),
			"num":  namedBuiltin("conv.num", func(args []Value) (Value, error) { return builtinByName("num").fn(args) }),
			"bool": namedBuiltin("conv.bool", func(args []Value) (Value, error) { return builtinByName("bool").fn(args) }),
		},
	},
	"safe": {
		name: "safe",
		members: map[string]builtinValue{
			"coalesce": namedBuiltin("safe.coalesce", func(args []Value) (Value, error) { return builtinByName("coalesce").fn(args) }),
			"has":      namedBuiltin("safe.has", func(args []Value) (Value, error) { return builtinByName("has").fn(args) }),
		},
	},
	"io": {
		name: "io",
		members: map[string]builtinValue{
			"print": namedBuiltin("io.print", func(args []Value) (Value, error) { return builtinByName("print").fn(args) }),
			"input": namedBuiltin("io.input", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("io.input expects 0 arguments")
				}
				scanner := bufio.NewScanner(os.Stdin)
				if scanner.Scan() {
					return scanner.Text(), nil
				}
				if err := scanner.Err(); err != nil {
					return nil, err
				}
				return "", nil
			}),
		},
	},
	"time": {
		name: "time",
		members: map[string]builtinValue{
			"now": namedBuiltin("time.now", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("time.now expects 0 arguments")
				}
				return float64(time.Now().Unix()), nil
			}),
			"sleepMs": namedBuiltin("time.sleepMs", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("time.sleepMs expects 1 argument")
				}
				ms, ok := args[0].(float64)
				if !ok {
					return nil, fmt.Errorf("time.sleepMs expects a number")
				}
				if ms < 0 {
					return nil, fmt.Errorf("time.sleepMs expects a non-negative number")
				}
				time.Sleep(time.Duration(ms) * time.Millisecond)
				return nil, nil
			}),
		},
	},
	"tuxies": {
		name: "tuxies",
		members: map[string]builtinValue{
			"spawn": namedBuiltin("tuxies.spawn", func(args []Value) (Value, error) {
				if len(args) < 1 {
					return nil, fmt.Errorf("tuxies.spawn expects at least 1 argument")
				}
				task := spawnTask(nil, args[0], args[1:])
				return task, nil
			}),
			"cancel": namedBuiltin("tuxies.cancel", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.cancel expects 1 argument")
				}
				task, ok := args[0].(*taskValue)
				if !ok {
					return nil, fmt.Errorf("tuxies.cancel expects a task")
				}
				return task.requestCancel(), nil
			}),
			"cancelAll": namedBuiltin("tuxies.cancelAll", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.cancelAll expects 1 argument")
				}
				tasks, err := requireTaskArray("tuxies.cancelAll", args[0])
				if err != nil {
					return nil, err
				}
				var canceled float64
				for _, task := range tasks {
					if task.requestCancel() {
						canceled++
					}
				}
				return canceled, nil
			}),
			"wait": namedBuiltin("tuxies.wait", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.wait expects 1 argument")
				}
				task, ok := args[0].(*taskValue)
				if !ok {
					return nil, fmt.Errorf("tuxies.wait expects a task")
				}
				<-task.done
				if task.err != nil {
					return nil, task.err
				}
				return task.value, nil
			}),
			"group": namedBuiltin("tuxies.group", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.group expects 1 argument")
				}
				tasks, err := requireTaskArray("tuxies.group", args[0])
				if err != nil {
					return nil, err
				}
				type groupResult struct {
					index int
					task  *taskValue
				}
				results := make([]Value, len(tasks))
				ch := make(chan groupResult, len(tasks))
				for i, task := range tasks {
					go func(i int, task *taskValue) {
						<-task.done
						ch <- groupResult{index: i, task: task}
					}(i, task)
				}
				for remaining := len(tasks); remaining > 0; remaining-- {
					result := <-ch
					if result.task.err != nil {
						for _, task := range tasks {
							task.requestCancel()
						}
						return nil, result.task.err
					}
					results[result.index] = result.task.value
				}
				return results, nil
			}),
			"all": namedBuiltin("tuxies.all", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.all expects 1 argument")
				}
				tasks, err := requireTaskArray("tuxies.all", args[0])
				if err != nil {
					return nil, err
				}
				results := make([]Value, len(tasks))
				for i, task := range tasks {
					<-task.done
					if task.err != nil {
						return nil, task.err
					}
					results[i] = task.value
				}
				return results, nil
			}),
			"race": namedBuiltin("tuxies.race", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.race expects 1 argument")
				}
				tasks, err := requireTaskArray("tuxies.race", args[0])
				if err != nil {
					return nil, err
				}
				if len(tasks) == 0 {
					return nil, fmt.Errorf("tuxies.race expects a non-empty array")
				}
				result := make(chan *taskValue, len(tasks))
				for _, task := range tasks {
					go func(task *taskValue) {
						<-task.done
						result <- task
					}(task)
				}
				task := <-result
				if task.err != nil {
					return nil, task.err
				}
				return task.value, nil
			}),
			"map": namedBuiltin("tuxies.map", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("tuxies.map expects 2 arguments")
				}
				items, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("tuxies.map expects an array")
				}
				callable := args[1]
				tasks := make([]*taskValue, len(items))
				for i, item := range items {
					tasks[i] = spawnTask(nil, callable, []Value{item})
				}
				results := make([]Value, len(tasks))
				for i, task := range tasks {
					<-task.done
					if task.err != nil {
						return nil, task.err
					}
					results[i] = task.value
				}
				return results, nil
			}),
			"status": namedBuiltin("tuxies.status", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.status expects 1 argument")
				}
				task, ok := args[0].(*taskValue)
				if !ok {
					return nil, fmt.Errorf("tuxies.status expects a task")
				}
				return task.status(), nil
			}),
			"done": namedBuiltin("tuxies.done", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.done expects 1 argument")
				}
				task, ok := args[0].(*taskValue)
				if !ok {
					return nil, fmt.Errorf("tuxies.done expects a task")
				}
				return task.isDone(), nil
			}),
			"join": namedBuiltin("tuxies.join", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("tuxies.join expects 1 argument")
				}
				task, ok := args[0].(*taskValue)
				if !ok {
					return nil, fmt.Errorf("tuxies.join expects a task")
				}
				<-task.done
				if task.err != nil {
					return nil, task.err
				}
				return task.value, nil
			}),
		},
	},
	"json": {
		name: "json",
		members: map[string]builtinValue{
			"stringify": namedBuiltin("json.stringify", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("json.stringify expects 1 argument")
				}
				encoded, err := jsonMarshalValue(args[0])
				if err != nil {
					return nil, err
				}
				data, err := stdjson.Marshal(encoded)
				if err != nil {
					return nil, err
				}
				return string(data), nil
			}),
			"parse": namedBuiltin("json.parse", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("json.parse expects 1 argument")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("json.parse expects a string")
				}
				var decoded any
				if err := stdjson.Unmarshal([]byte(s), &decoded); err != nil {
					return nil, err
				}
				return jsonUnmarshalValue(decoded), nil
			}),
		},
	},
	"csv": {
		name: "csv",
		members: map[string]builtinValue{
			"parse": namedBuiltin("csv.parse", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("csv.parse expects 1 argument")
				}
				s, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("csv.parse expects a string")
				}
				reader := stdcsv.NewReader(strings.NewReader(s))
				records, err := reader.ReadAll()
				if err != nil {
					return nil, err
				}
				rows := make([]Value, len(records))
				for i, record := range records {
					row := make([]Value, len(record))
					for j, cell := range record {
						row[j] = cell
					}
					rows[i] = row
				}
				return rows, nil
			}),
			"stringify": namedBuiltin("csv.stringify", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("csv.stringify expects 1 argument")
				}
				rows, ok := args[0].([]Value)
				if !ok {
					return nil, fmt.Errorf("csv.stringify expects an array of rows")
				}
				var b strings.Builder
				writer := stdcsv.NewWriter(&b)
				for _, rowValue := range rows {
					row, ok := rowValue.([]Value)
					if !ok {
						return nil, fmt.Errorf("csv.stringify expects an array of rows")
					}
					record := make([]string, len(row))
					for i, cell := range row {
						record[i] = formatValue(cell)
					}
					if err := writer.Write(record); err != nil {
						return nil, err
					}
				}
				writer.Flush()
				if err := writer.Error(); err != nil {
					return nil, err
				}
				return b.String(), nil
			}),
		},
	},
	"regex": {
		name: "regex",
		members: map[string]builtinValue{
			"match": namedBuiltin("regex.match", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("regex.match expects 2 arguments")
				}
				pattern, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("regex.match expects a string pattern")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("regex.match expects a string input")
				}
				return regexp.MatchString(pattern, text)
			}),
			"find": namedBuiltin("regex.find", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("regex.find expects 2 arguments")
				}
				pattern, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("regex.find expects a string pattern")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("regex.find expects a string input")
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, err
				}
				match := re.FindString(text)
				if match == "" {
					return nil, nil
				}
				return match, nil
			}),
			"findAll": namedBuiltin("regex.findAll", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("regex.findAll expects 2 arguments")
				}
				pattern, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("regex.findAll expects a string pattern")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("regex.findAll expects a string input")
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, err
				}
				matches := re.FindAllString(text, -1)
				out := make([]Value, len(matches))
				for i, match := range matches {
					out[i] = match
				}
				return out, nil
			}),
			"replace": namedBuiltin("regex.replace", func(args []Value) (Value, error) {
				if len(args) != 3 {
					return nil, fmt.Errorf("regex.replace expects 3 arguments")
				}
				pattern, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("regex.replace expects a string pattern")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("regex.replace expects a string input")
				}
				repl, ok := args[2].(string)
				if !ok {
					return nil, fmt.Errorf("regex.replace expects a string replacement")
				}
				re, err := regexp.Compile(pattern)
				if err != nil {
					return nil, err
				}
				return re.ReplaceAllString(text, repl), nil
			}),
		},
	},
	"path": {
		name: "path",
		members: map[string]builtinValue{
			"join": namedBuiltin("path.join", func(args []Value) (Value, error) {
				if len(args) == 0 {
					return "", nil
				}
				parts := make([]string, len(args))
				for i, arg := range args {
					s, ok := arg.(string)
					if !ok {
						return nil, fmt.Errorf("path.join expects string arguments")
					}
					parts[i] = strings.Trim(s, "/")
				}
				return strings.Join(parts, "/"), nil
			}),
			"base": namedBuiltin("path.base", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("path.base", requireOneArg("path.base", args))
				if err != nil {
					return nil, err
				}
				return filepath.Base(s), nil
			}),
			"dir": namedBuiltin("path.dir", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("path.dir", requireOneArg("path.dir", args))
				if err != nil {
					return nil, err
				}
				return filepath.Dir(s), nil
			}),
			"ext": namedBuiltin("path.ext", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("path.ext", requireOneArg("path.ext", args))
				if err != nil {
					return nil, err
				}
				return filepath.Ext(s), nil
			}),
			"clean": namedBuiltin("path.clean", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("path.clean", requireOneArg("path.clean", args))
				if err != nil {
					return nil, err
				}
				return filepath.Clean(s), nil
			}),
			"abs": namedBuiltin("path.abs", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("path.abs", requireOneArg("path.abs", args))
				if err != nil {
					return nil, err
				}
				return filepath.Abs(s)
			}),
		},
	},
	"fs": {
		name: "fs",
		members: map[string]builtinValue{
			"exists": namedBuiltin("fs.exists", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.exists", requireOneArg("fs.exists", args))
				if err != nil {
					return nil, err
				}
				return pathExists(s), nil
			}),
			"isDir": namedBuiltin("fs.isDir", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.isDir", requireOneArg("fs.isDir", args))
				if err != nil {
					return nil, err
				}
				return pathIsDir(s), nil
			}),
			"isFile": namedBuiltin("fs.isFile", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.isFile", requireOneArg("fs.isFile", args))
				if err != nil {
					return nil, err
				}
				return pathIsFile(s), nil
			}),
			"mkdir": namedBuiltin("fs.mkdir", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.mkdir", requireOneArg("fs.mkdir", args))
				if err != nil {
					return nil, err
				}
				return nil, os.Mkdir(s, 0755)
			}),
			"mkdirAll": namedBuiltin("fs.mkdirAll", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.mkdirAll", requireOneArg("fs.mkdirAll", args))
				if err != nil {
					return nil, err
				}
				return nil, os.MkdirAll(s, 0755)
			}),
			"list": namedBuiltin("fs.list", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.list", requireOneArg("fs.list", args))
				if err != nil {
					return nil, err
				}
				entries, err := os.ReadDir(s)
				if err != nil {
					return nil, err
				}
				out := make([]Value, len(entries))
				for i, entry := range entries {
					out[i] = entry.Name()
				}
				return out, nil
			}),
			"readText": namedBuiltin("fs.readText", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.readText", requireOneArg("fs.readText", args))
				if err != nil {
					return nil, err
				}
				data, err := os.ReadFile(s)
				if err != nil {
					return nil, err
				}
				return string(data), nil
			}),
			"writeText": namedBuiltin("fs.writeText", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("fs.writeText expects 2 arguments")
				}
				path, err := requireStringPathArg("fs.writeText", args[0])
				if err != nil {
					return nil, err
				}
				content, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("fs.writeText expects string content")
				}
				return nil, os.WriteFile(path, []byte(content), 0644)
			}),
			"appendText": namedBuiltin("fs.appendText", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("fs.appendText expects 2 arguments")
				}
				path, err := requireStringPathArg("fs.appendText", args[0])
				if err != nil {
					return nil, err
				}
				content, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("fs.appendText expects string content")
				}
				f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					return nil, err
				}
				defer f.Close()
				if _, err := f.WriteString(content); err != nil {
					return nil, err
				}
				return nil, nil
			}),
			"remove": namedBuiltin("fs.remove", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.remove", requireOneArg("fs.remove", args))
				if err != nil {
					return nil, err
				}
				return nil, os.Remove(s)
			}),
			"removeAll": namedBuiltin("fs.removeAll", func(args []Value) (Value, error) {
				s, err := requireStringPathArg("fs.removeAll", requireOneArg("fs.removeAll", args))
				if err != nil {
					return nil, err
				}
				return nil, os.RemoveAll(s)
			}),
			"rename": namedBuiltin("fs.rename", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("fs.rename expects 2 arguments")
				}
				oldPath, err := requireStringPathArg("fs.rename", args[0])
				if err != nil {
					return nil, err
				}
				newPath, err := requireStringPathArg("fs.rename", args[1])
				if err != nil {
					return nil, err
				}
				return nil, os.Rename(oldPath, newPath)
			}),
		},
	},
	"os": {
		name: "os",
		members: map[string]builtinValue{
			"getenv": namedBuiltin("os.getenv", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("os.getenv expects 1 argument")
				}
				key, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("os.getenv expects a string key")
				}
				return os.Getenv(key), nil
			}),
			"setenv": namedBuiltin("os.setenv", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("os.setenv expects 2 arguments")
				}
				key, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("os.setenv expects string arguments")
				}
				value, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("os.setenv expects string arguments")
				}
				err := os.Setenv(key, value)
				if err != nil {
					return nil, err
				}
				return nil, nil
			}),
			"cwd": namedBuiltin("os.cwd", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("os.cwd expects 0 arguments")
				}
				cwd, err := os.Getwd()
				if err != nil {
					return nil, err
				}
				return cwd, nil
			}),
			"exit": namedBuiltin("os.exit", func(args []Value) (Value, error) {
				code := 0
				if len(args) > 1 {
					return nil, fmt.Errorf("os.exit expects 0 or 1 argument")
				}
				if len(args) == 1 {
					c, ok := args[0].(float64)
					if !ok {
						return nil, fmt.Errorf("os.exit expects a number code")
					}
					code = int(c)
				}
				os.Exit(code)
				return nil, nil
			}),
		},
	},
	"subprocess": {
		name: "subprocess",
		members: map[string]builtinValue{
			"run": namedBuiltin("subprocess.run", func(args []Value) (Value, error) {
				if len(args) < 1 {
					return nil, fmt.Errorf("subprocess.run expects at least 1 argument")
				}
				cmdArgs := make([]string, len(args))
				for i, arg := range args {
					s, ok := arg.(string)
					if !ok {
						return nil, fmt.Errorf("subprocess.run expects string arguments")
					}
					cmdArgs[i] = s
				}
				cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
				err := cmd.Run()
				if err != nil {
					return nil, err
				}
				return nil, nil
			}),
		},
	},
	"file": {
		name: "file",
		members: map[string]builtinValue{
			"read": namedBuiltin("file.read", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("file.read expects 1 argument")
				}
				path, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("file.read expects string path")
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				return string(data), nil
			}),
			"write": namedBuiltin("file.write", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("file.write expects 2 arguments")
				}
				path, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("file.write expects string path")
				}
				content, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("file.write expects string content")
				}
				err := os.WriteFile(path, []byte(content), 0644)
				if err != nil {
					return nil, err
				}
				return nil, nil
			}),
		},
	},
	"gui": {
		name: "gui",
		members: map[string]builtinValue{
			"newApp": namedBuiltin("gui.newApp", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("gui.newApp expects 0 arguments")
				}
				return gui.NewApp(), nil
			}),
			"newWindow": namedBuiltin("gui.newWindow", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.newWindow expects 2 arguments")
				}
				a, ok := args[0].(*gui.App)
				if !ok {
					return nil, fmt.Errorf("gui.newWindow expects app as first argument")
				}
				title, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.newWindow expects string title")
				}
				return a.NewWindow(title), nil
			}),
			"newButton": namedBuiltin("gui.newButton", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("gui.newButton expects 1 argument")
				}
				text, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("gui.newButton expects string text")
				}
				return gui.NewButton(text), nil
			}),
			"newTextInput": namedBuiltin("gui.newTextInput", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("gui.newTextInput expects 1 argument")
				}
				text, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("gui.newTextInput expects string text")
				}
				return gui.NewTextInput(text), nil
			}),
			"newLabel": namedBuiltin("gui.newLabel", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("gui.newLabel expects 1 argument")
				}
				text, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("gui.newLabel expects string text")
				}
				return gui.NewLabel(text), nil
			}),
			"newPanel": namedBuiltin("gui.newPanel", func(args []Value) (Value, error) {
				if len(args) != 0 {
					return nil, fmt.Errorf("gui.newPanel expects 0 arguments")
				}
				return gui.NewPanel(), nil
			}),
			"add": namedBuiltin("gui.add", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.add expects 2 arguments")
				}
				switch parent := args[0].(type) {
				case *gui.Window:
					parent.Add(args[1])
				case *gui.Panel:
					parent.Add(args[1])
				default:
					return nil, fmt.Errorf("gui.add expects window or panel as first argument")
				}
				return nil, nil
			}),
			"setContent": namedBuiltin("gui.setContent", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setContent expects 2 arguments")
				}
				w, ok := args[0].(*gui.Window)
				if !ok {
					return nil, fmt.Errorf("gui.setContent expects window as first argument")
				}
				content := args[1] // Assume Label or Button
				w.SetContent(content)
				return nil, nil
			}),
			"setID": namedBuiltin("gui.setID", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setID expects 2 arguments")
				}
				id, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setID expects string id")
				}
				gui.SetID(args[0], id)
				return nil, nil
			}),
			"setText": namedBuiltin("gui.setText", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setText expects 2 arguments")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setText expects string text")
				}
				gui.SetText(args[0], text)
				return nil, nil
			}),
			"setPosition": namedBuiltin("gui.setPosition", func(args []Value) (Value, error) {
				x, y, err := requireTwoInts("gui.setPosition", args)
				if err != nil {
					return nil, err
				}
				gui.SetPosition(args[0], x, y)
				return nil, nil
			}),
			"setSize": namedBuiltin("gui.setSize", func(args []Value) (Value, error) {
				width, height, err := requireTwoInts("gui.setSize", args)
				if err != nil {
					return nil, err
				}
				gui.SetSize(args[0], width, height)
				return nil, nil
			}),
			"setWindowSize": namedBuiltin("gui.setWindowSize", func(args []Value) (Value, error) {
				width, height, err := requireTwoInts("gui.setWindowSize", args)
				if err != nil {
					return nil, err
				}
				window, ok := args[0].(*gui.Window)
				if !ok {
					return nil, fmt.Errorf("gui.setWindowSize expects window")
				}
				window.SetSize(width, height)
				return nil, nil
			}),
			"setWindowPosition": namedBuiltin("gui.setWindowPosition", func(args []Value) (Value, error) {
				x, y, err := requireTwoInts("gui.setWindowPosition", args)
				if err != nil {
					return nil, err
				}
				window, ok := args[0].(*gui.Window)
				if !ok {
					return nil, fmt.Errorf("gui.setWindowPosition expects window")
				}
				window.SetPosition(x, y)
				return nil, nil
			}),
			"setBackground": namedBuiltin("gui.setBackground", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setBackground expects 2 arguments")
				}
				color, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setBackground expects string color")
				}
				window, ok := args[0].(*gui.Window)
				if ok {
					window.SetBackground(color)
					return nil, nil
				}
				gui.SetColors(args[0], "", color)
				return nil, nil
			}),
			"setColors": namedBuiltin("gui.setColors", func(args []Value) (Value, error) {
				if len(args) != 3 {
					return nil, fmt.Errorf("gui.setColors expects 3 arguments")
				}
				foreground, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setColors expects string foreground")
				}
				background, ok := args[2].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setColors expects string background")
				}
				gui.SetColors(args[0], foreground, background)
				return nil, nil
			}),
			"setFontSize": namedBuiltin("gui.setFontSize", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setFontSize expects 2 arguments")
				}
				size, err := requireIntArg("gui.setFontSize", args[1])
				if err != nil {
					return nil, err
				}
				gui.SetFontSize(args[0], size)
				return nil, nil
			}),
			"setPlaceholder": namedBuiltin("gui.setPlaceholder", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setPlaceholder expects 2 arguments")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setPlaceholder expects string text")
				}
				gui.SetPlaceholder(args[0], text)
				return nil, nil
			}),
			"setValue": namedBuiltin("gui.setValue", func(args []Value) (Value, error) {
				if len(args) != 2 {
					return nil, fmt.Errorf("gui.setValue expects 2 arguments")
				}
				text, ok := args[1].(string)
				if !ok {
					return nil, fmt.Errorf("gui.setValue expects string text")
				}
				gui.SetValue(args[0], text)
				return nil, nil
			}),
			"getValue": namedBuiltin("gui.getValue", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("gui.getValue expects 1 argument")
				}
				return gui.GetValue(args[0]), nil
			}),
			"showWindow": namedBuiltin("gui.showWindow", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("gui.showWindow expects 1 argument")
				}
				w, ok := args[0].(*gui.Window)
				if !ok {
					return nil, fmt.Errorf("gui.showWindow expects window")
				}
				result, err := w.Show()
				if err != nil {
					return nil, err
				}
				return guiResultToValue(result), nil
			}),
			"showAndRun": namedBuiltin("gui.showAndRun", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("gui.showAndRun expects 1 argument")
				}
				a, ok := args[0].(*gui.App)
				if !ok {
					return nil, fmt.Errorf("gui.showAndRun expects app")
				}
				result, err := a.Run()
				if err != nil {
					return nil, err
				}
				return guiResultToValue(result), nil
			}),
		},
	},
	"net": {
		name: "net",
		members: map[string]builtinValue{
			"httpGet": namedBuiltin("net.httpGet", func(args []Value) (Value, error) {
				if len(args) != 1 {
					return nil, fmt.Errorf("net.httpGet expects 1 argument")
				}
				url, ok := args[0].(string)
				if !ok {
					return nil, fmt.Errorf("net.httpGet expects string url")
				}
				resp, err := http.Get(url)
				if err != nil {
					return nil, err
				}
				defer resp.Body.Close()
				body, err := io.ReadAll(resp.Body)
				if err != nil {
					return nil, err
				}
				return string(body), nil
			}),
		},
	},
}

func requireTaskArray(name string, value Value) ([]*taskValue, error) {
	tasks, ok := value.([]Value)
	if !ok {
		return nil, fmt.Errorf("%s expects an array of tasks", name)
	}
	out := make([]*taskValue, len(tasks))
	for i, item := range tasks {
		task, ok := item.(*taskValue)
		if !ok {
			return nil, fmt.Errorf("%s expects an array of tasks", name)
		}
		out[i] = task
	}
	return out, nil
}

func builtinByName(name string) builtinValue {
	for _, builtin := range builtins {
		if builtin.name == name {
			return builtin
		}
	}
	panic("missing builtin: " + name)
}

func requireNumberArg(name string, args []Value, count int, index int) (float64, error) {
	if len(args) != count {
		return 0, fmt.Errorf("%s expects %d arguments", name, count)
	}
	value, ok := args[index].(float64)
	if !ok {
		return 0, fmt.Errorf("%s expects a number", name)
	}
	return value, nil
}

func requireStringArg(name string, args []Value, count int, index int) (string, error) {
	if len(args) != count {
		return "", fmt.Errorf("%s expects %d arguments", name, count)
	}
	value, ok := args[index].(string)
	if !ok {
		return "", fmt.Errorf("%s expects a string", name)
	}
	return value, nil
}

func requireIntArg(name string, value Value) (int, error) {
	number, ok := value.(float64)
	if !ok {
		return 0, fmt.Errorf("%s expects a number", name)
	}
	out := int(number)
	if float64(out) != number {
		return 0, fmt.Errorf("%s expects an integer", name)
	}
	return out, nil
}

func requireTwoInts(name string, args []Value) (int, int, error) {
	if len(args) != 3 {
		return 0, 0, fmt.Errorf("%s expects 3 arguments", name)
	}
	first, err := requireIntArg(name, args[1])
	if err != nil {
		return 0, 0, err
	}
	second, err := requireIntArg(name, args[2])
	if err != nil {
		return 0, 0, err
	}
	return first, second, nil
}

func guiResultToValue(result *gui.Result) Value {
	if result == nil {
		return nil
	}
	values := make(map[hashKey]Value, len(result.Values))
	for key, value := range result.Values {
		values[hashKey{kind: "string", value: key}] = value
	}
	return map[hashKey]Value{
		{kind: "string", value: "action"}: result.Action,
		{kind: "string", value: "target"}: result.Target,
		{kind: "string", value: "values"}: values,
	}
}
