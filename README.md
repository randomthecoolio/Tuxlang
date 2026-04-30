--Tuxlang--

Tuxlang is a lightweight general-purpose language implemented in Go It compiles source into bytecode and executes that bytecode on a compact stack VM

The syntax is closer to Lua for readability, but it keeps a few distinct Tuxlang choices such as 0-based indexing and square-bracket arrays

--Goals--

- Bytecode execution instead of direct AST walking
- Simple syntax with 0-based indexing
- General-purpose control flow, functions, arrays, maps, and strings
- Useful internals: constant pooling, indexed locals, fixed stack/frame limits, and explicit runtime errors

Features

- `local` bindings and reassignment
- `function` declarations with `end`
- optional OOP: `class`, `extends`, `new`, and `this`
- `if ... then ... else ... end`
- `while ... do ... end`
- `for i = start, end[, step] do ... end`
- numbers, strings, booleans, `null`
- arrays, maps, and 0-based indexing
- core built-ins: `print`, `len`, `push`, `type`, `str`, `num`, `bool`, `keys`, `has`, `coalesce`, `assert`, `range`, `sum`
- 25 importable built-in stdlib modules: `math`, `text`, `array`, `maplib`, `rand`, `base64`, `hash`, `url`, `uuid`, `conv`, `safe`, `io`, `time`, `tuxies`, `json`, `path`, `fs`, `os`, `subprocess`, `file`, `gui`, `net`, `csv`, `regex`
- compatibility support for older `let` / `fn` / brace blocks
- null-safe indexing for arrays, strings, and `nil` values
- runtime errors with function stack context


## Example

```tux
import math

function fact(n)
    if n <= 1 then
        return 1
    end

    return n * fact(n - 1)
end

local values = [10, 20, 30]
print(values[0], fact(5), math.sqrt(81))
```

## Language overview

Tuxlang programs are made of statements and expressions. The language is dynamically typed and uses `null` for missing values.

### Comments

Single-line comments:

```tux
// this is a comment
local x = 10
```

Block comments:

```tux
/*
   this is a block comment
*/
local y = 20
```

### Values and types

Tuxlang currently supports:

- `number`
- `string`
- `bool`
- `null`
- `array`
- `map`
- `function`
- imported built-in stdlib values

Examples:

```tux
local n = 42
local s = "hello"
local ok = true
local missing = null
local items = [1, 2, 3]
local user = {"name": "Tux", "age": 3}
```

### Variables

Declare variables with `local`:

```tux
local name = "Tux"
local count = 1
```

Reassign them later:

```tux
count = count + 1
name = "Penguin"
```

### Expressions and operators

Supported arithmetic operators:

- `+`
- `-`
- `*`
- `/`
- `%`

Comparison operators:

- `==`
- `!=`
- `<`
- `<=`
- `>`
- `>=`

Logical operators:

- `&&`
- `||`
- prefix `!`

Examples:

```tux
local total = (4 + 2) * 3
local same = total == 18
local allowed = same && true
local denied = !allowed
```

### Strings

Strings use double quotes and support basic escapes:

- `\n`
- `\t`
- `\"`
- `\\`

Example:

```tux
local text = "line 1\nline 2"
print(text)
```

Strings can be indexed with 0-based indexes:

```tux
local word = "tux"
print(word[0]) // "t"
print(word[10]) // null
```

### Arrays

Arrays use square brackets and 0-based indexing:

```tux
local values = [10, 20, 30]
print(values[0]) // 10
print(values[2]) // 30
print(values[9]) // null
```

### Maps

Maps use braces with `key: value` pairs:

```tux
local user = {
    "name": "Tux",
    "age": 3,
    true: "yes"
}

print(user["name"])
print(user["missing"]) // null
```

Current map key support is limited to:

- strings
- numbers
- booleans

### Conditionals

Tuxlang uses Lua-like block keywords:

```tux
local score = 90

if score >= 90 then
    print("great")
else
    print("keep going")
end
```

Brace-style blocks are also accepted for compatibility:

```tux
if true {
    print("works")
} else {
    print("no")
}
```

### While loops

```tux
local i = 0

while i < 3 do
    print(i)
    i = i + 1
end
```

Compatibility brace style:

```tux
while false {
    print("never")
}
```

### For loops

Numeric `for` loops count from a start value to an end value, with an optional step:

```tux
local sum = 0

for i = 1, 5 do
    sum = sum + i
end

for i = 5, 1, -1 do
    print(i)
end
```

### Functions

Declare functions with `function` and return values with `return`:

```tux
function add(a, b)
    return a + b
end

print(add(2, 3))
```

Functions are called with normal parentheses:

```tux
function greet(name)
    print("hello " + name)
end

greet("tux")
```

### Optional OOP

Tuxlang also supports optional class-based OOP without changing existing non-OOP code:

```tux
class User
    function init(name)
        this.name = name
    end

    function greet()
        return "hi " + this.name
    end
end

class Admin extends User
    function greet()
        return "admin " + this.name
    end
end

local u = new User("tux")
local a = new Admin("root")
print(u.greet(), a.greet())
```

### Imports and stdlib

Import built-in stdlib modules with `import`:

```tux
import math
import text

print(math.sqrt(81))
print(text.upper("tux"))
```

Stdlib members can be accessed with dot syntax:

```tux
math.sqrt(16)
text.lower("HELLO")
```

This is compiled internally as indexed access, so both of these are conceptually similar:

```tux
math.sqrt(16)
math["sqrt"](16)
```

## Built-in functions

These built-ins are always available:

- `print(...)`: print values separated by spaces
- `len(value)`: length of string, array, map, or `0` for `nil`
- `push(array, value)`: append a value and return a new array
- `type(value)`: return the runtime type name
- `str(value)`: convert a value to its string form
- `num(value)`: convert `null`, bools, numbers, or numeric strings to number
- `bool(value)`: truthiness conversion
- `keys(map)`: return map keys as an array
- `has(container, keyOrIndex)`: check membership / valid index
- `coalesce(a, b, ...)`: return the first non-null value
- `assert(condition, message?)`: fail with a runtime error when condition is falsy
- `range(start, end, step?)`: build a numeric array
- `sum(array)`: sum numeric array values

Examples:

```tux
print(type([1, 2, 3]))
print(len("tux"))
print(push([1, 2], 3))
print(num("12.5"))
print(bool(0))
print(coalesce(null, null, "fallback"))
```

## Built-in stdlib modules

```tux
import math
import text
import io

local answer = math.sqrt(144)
local label = text.upper("tux")
io.print(label, answer)
```

Available built-in modules:

- `math`: `abs`, `sqrt`, `floor`, `ceil`, `round`, `pow`, `min`, `max`
- `text`: `upper`, `lower`, `trim`, `contains`, `replace`, `split`, `startsWith`, `endsWith`
- `array`: `join`, `at`, `slice`, `reverse`
- `maplib`: `get`, `size`, `set`, `delete`
- `rand`: `float`, `int`, `pick`
- `base64`: `encode`, `decode`
- `hash`: `sha256`
- `url`: `encode`, `decode`, `queryGet`
- `uuid`: `v4`
- `conv`: `str`, `num`, `bool`
- `safe`: `coalesce`, `has`
- `io`: `print`, `input`
- `time`: `now`, `sleepMs`
- `tuxies`: `spawn`, `cancel`, `cancelAll`, `wait`, `join`, `group`, `all`, `race`, `map`, `status`, `done`
- `json`: `stringify`, `parse`
- `csv`: `parse`, `stringify`
- `regex`: `match`, `find`, `findAll`, `replace`
- `path`: `join`, `base`, `dir`, `ext`, `clean`, `abs`
- `fs`: `exists`, `isDir`, `isFile`, `mkdir`, `mkdirAll`, `list`, `readText`, `writeText`, `appendText`, `remove`, `removeAll`, `rename`
- `os`: `getenv`, `setenv`, `cwd`, `exit`
- `subprocess`: `run`
- `file`: `read`, `write`
- `net`: `httpGet`
- `gui`: `newApp`, `newWindow`, `newButton`, `newLabel`, `newTextInput`, `newPanel`, `add`, `setContent`, `setID`, `setText`, `setPosition`, `setSize`, `setWindowSize`, `setWindowPosition`, `setBackground`, `setColors`, `setFontSize`, `setPlaceholder`, `setValue`, `getValue`, `showWindow`, `showAndRun`

GUI example:

```tux
import gui

local app = gui.newApp()
local win = gui.newWindow(app, "Tux Designer")
gui.setWindowSize(win, 760, 520)
gui.setBackground(win, "#f4efe6")

local label = gui.newLabel("Project name")
gui.setPosition(label, 24, 24)
gui.setFontSize(label, 16)

local input = gui.newTextInput("Tux Studio")
gui.setID(input, "project")
gui.setPosition(input, 24, 60)
gui.setSize(input, 260, 32)

local button = gui.newButton("Create")
gui.setID(button, "create")
gui.setPosition(button, 24, 110)
gui.setColors(button, "#ffffff", "#0f766e")

gui.add(win, label)
gui.add(win, input)
gui.add(win, button)

local result = gui.showAndRun(app)
print(result["action"], result["target"], result["values"]["project"])
```
### Module reference

#### `math`

- `math.abs(number)`
- `math.sqrt(number)`
- `math.floor(number)`
- `math.ceil(number)`
- `math.round(number)`
- `math.pow(base, exponent)`
- `math.min(a, b)`
- `math.max(a, b)`

#### `text`

- `text.upper(string)`
- `text.lower(string)`
- `text.trim(string)`
- `text.contains(string, substring)`
- `text.replace(string, old, new)`
- `text.split(string, separator)`
- `text.startsWith(string, prefix)`
- `text.endsWith(string, suffix)`

#### `array`

- `array.join(array, separator)`
- `array.at(array, index)`
- `array.slice(array, start, end)`
- `array.reverse(array)`

#### `maplib`

- `maplib.get(map, key)`
- `maplib.size(map)`
- `maplib.set(map, key, value)`
- `maplib.delete(map, key)`

#### `rand`

- `rand.float()`
- `rand.int(min, max)` (max is exclusive)
- `rand.pick(array)`

#### `base64`

- `base64.encode(string)`
- `base64.decode(string)`

#### `hash`

- `hash.sha256(string)`

#### `url`

- `url.encode(string)`
- `url.decode(string)`
- `url.queryGet(urlOrQuery, key)`

#### `uuid`

- `uuid.v4()`

#### `conv`

- `conv.str(value)`
- `conv.num(value)`
- `conv.bool(value)`

#### `safe`

- `safe.coalesce(a, b, ...)`
- `safe.has(container, keyOrIndex)`

#### `io`

- `io.print(...)`
- `io.input()`

#### `time`

- `time.now()` returns a Unix timestamp
- `time.sleepMs(milliseconds)` pauses execution for a short duration

#### `tuxies`

- `tuxies.spawn(callable, ...args)` starts a function or bound method in the background and returns a task
- `tuxies.cancel(task)` requests cancellation of a task
- `tuxies.cancelAll(tasks)` requests cancellation for every task in an array
- `tuxies.wait(task)` waits for a task and returns its result
- `tuxies.group(tasks)` waits for a task group and cancels siblings if one task fails
- `tuxies.all(tasks)` waits for an array of tasks and returns an array of results in order
- `tuxies.race(tasks)` waits for the first task in an array to finish and returns that result
- `tuxies.map(values, callable)` runs a function over an array concurrently and returns the mapped results
- `tuxies.status(task)` returns `pending`, `done`, `failed`, or `canceled`
- `tuxies.done(task)` checks whether a task has finished
- `tuxies.join(task)` waits for the task to finish and returns its result

Example:

```tux
import tuxies

function add(a, b)
    return a + b
end

local task = tuxies.spawn(add, 10, 32)
print(tuxies.join(task))
```

#### `json`

- `json.stringify(value)` returns valid JSON text
- `json.parse(text)` parses JSON text into Tuxlang values

#### `csv`

- `csv.parse(text)` parses CSV text into an array of rows
- `csv.stringify(rows)` turns an array of rows into CSV text

#### `regex`

- `regex.match(pattern, text)` returns whether the pattern matches
- `regex.find(pattern, text)` returns the first match or `null`
- `regex.findAll(pattern, text)` returns an array of all matches
- `regex.replace(pattern, text, replacement)` returns the replaced text

#### `path`

- `path.join(part1, part2, ...)`
- `path.base(path)`
- `path.dir(path)`
- `path.ext(path)`
- `path.clean(path)`
- `path.abs(path)`

Note: this is a simple slash-joiner, not a full OS-aware path normalizer.

#### `fs`

- `fs.exists(path)`
- `fs.isDir(path)`
- `fs.isFile(path)`
- `fs.mkdir(path)`
- `fs.mkdirAll(path)`
- `fs.list(path)`
- `fs.readText(path)`
- `fs.writeText(path, content)`
- `fs.appendText(path, content)`
- `fs.remove(path)`
- `fs.removeAll(path)`
- `fs.rename(oldPath, newPath)`

#### `os`

- `os.getenv(name)`
- `os.setenv(name, value)`
- `os.cwd()`
- `os.exit()`
- `os.exit(code)`

#### `subprocess`

- `subprocess.run(command, arg1, arg2, ...)`

#### `file`

- `file.read(path)`
- `file.write(path, content)`

#### `net`

- `net.httpGet(url)`

#### `gui`

The `gui` module creates simple desktop UIs.

- `gui.newApp()`
- `gui.newWindow(app, title)`
- `gui.newButton(text)`
- `gui.newLabel(text)`
- `gui.newTextInput(text)`
- `gui.newPanel()`
- `gui.add(parent, child)`
- `gui.setContent(window, child)`
- `gui.setID(widget, id)`
- `gui.setText(widgetOrWindow, text)`
- `gui.setPosition(widgetOrWindow, x, y)`
- `gui.setSize(widgetOrWindow, width, height)`
- `gui.setWindowSize(window, width, height)`
- `gui.setWindowPosition(window, x, y)`
- `gui.setBackground(target, color)`
- `gui.setColors(target, foreground, background)`
- `gui.setFontSize(widget, size)`
- `gui.setPlaceholder(input, text)`
- `gui.setValue(target, text)`
- `gui.getValue(target)`
- `gui.showWindow(window)`
- `gui.showAndRun(app)`

Buttons can also emit a `rightclick` action in the returned result when they are right-clicked.

## How to write Tuxlang code

### Small script example

```tux
import text

local name = " tux "
local clean = text.trim(name)

if clean == "" then
    print("missing name")
else
    print("hello " + text.upper(clean))
end
```

### Array processing example

```tux
local numbers = [1, 2, 3]
local numbers = push(numbers, 4)

local i = 0
while i < len(numbers) do
    print(numbers[i])
    i = i + 1
end
```

### Map usage example

```tux
local user = {"name": "Tux", "role": "admin"}

if has(user, "role") then
    print(user["role"])
end
```

### Function example

```tux
function fib(n)
    if n <= 1 then
        return n
    end

    return fib(n - 1) + fib(n - 2)
end

print(fib(6))
```

## Run

```bash
go run . ./examples/demo.tux
```

Other useful examples:

```bash
go run . ./examples/input.tux
go run . ./examples/gui_hello.tux
go run . ./examples/gui_designer.tux
```


## Safety notes

- `array[index]`, `string[index]`, and `nil[index]` return `null` when the target is missing instead of crashing the runtime.
- `len(nil)` returns `0`, and `push(nil, value)` creates a new array.
- `coalesce(a, b, c)` returns the first non-null value.
- Division by zero, bad calls, and unsupported operations return contextual runtime errors with the active function stack.
- `&&` and `||` short-circuit.

## Compatibility notes

Tuxlang still accepts some older syntax:

- `let` behaves like `local`
- `fn` behaves like `function`
- brace blocks are accepted in places where `do` / `then` / `end` are preferred

The preferred modern style is:

- `local`
- `function`
- `if ... then ... end`
- `while ... do ... end`



released under MIT license
