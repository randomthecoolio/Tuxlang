package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tuxlang/internal/compiler"
	"tuxlang/internal/lexer"
	"tuxlang/internal/parser"
	"tuxlang/internal/vm"
)

func TestPipelineParsesCompilesAndRuns(t *testing.T) {
	source := `
		function fib(n)
			if n <= 1 then
				return n
			end
			return fib(n - 1) + fib(n - 2)
		end

		local nums = [2, 4, 6]
		local idx = 0
		local total = 0
		while idx < len(nums) do
			total = total + nums[idx]
			idx = idx + 1
		end

		local data = {"name": "tux", "count": total}
		print(fib(8), data["name"], data["count"])
	`

	l := lexer.New(source)
	p := parser.New(l)
	program, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	comp := compiler.New()
	fn, err := comp.Compile(program)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	machine := vm.New()
	if err := machine.Run(fn); err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
}

func TestForLoopsCompileAndRun(t *testing.T) {
	source := `
		local sum = 0
		for i = 1, 5 do
			sum = sum + i
		end

		local reverse = ""
		for j = 3, 1, -1 do
			reverse = reverse + str(j)
		end

		assert(sum == 15, "forward for loop mismatch")
		assert(reverse == "321", "reverse for loop mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("for loop runtime failed: %v", err)
	}
}

func TestStandardLibraryAndNullSafety(t *testing.T) {
	source := `
		import math
		import text
		import safe

		local items = push(nil, 42)
		local record = {"name": "tux", "version": "1"}
		local missing = items[9]
		local fromNil = nil[0]
		local fallback = safe.coalesce(missing, fromNil, "safe")
		local total = num(record["version"]) + len(keys(record))
		local root = math.sqrt(81)
		local loud = text.upper("tux")
		print(type(items), bool(missing), has(record, "name"), str(fallback), total, root, loud)
	`

	if err := runSource(source); err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
}

func TestImportModulesCompileAndRun(t *testing.T) {
	source := `
		import math
		import io
		import array
		import path

		local values = [1, 2, 3]
		local joined = array.join(values, "-")
		local location = path.join("usr", "local", "bin")
		io.print(math.abs(-12), joined, location)
	`

	if err := runSource(source); err != nil {
		t.Fatalf("module runtime failed: %v", err)
	}
}

func TestOSAndSubprocessModules(t *testing.T) {
	source := `
		import os
		import io

		local env = os.getenv("PATH")
		os.setenv("TEST_VAR", "tuxlang")
		local cwd = os.cwd()
		io.print("PATH exists:", bool(env), "CWD:", cwd)
	`

	if err := runSource(source); err != nil {
		t.Fatalf("os module runtime failed: %v", err)
	}
}

func TestRuntimeErrorHasContext(t *testing.T) {
	source := `
		function explode()
			return 10 / 0
		end

		explode()
	`

	err := runSource(source)
	if err == nil {
		t.Fatal("expected runtime error")
	}
	message := err.Error()
	if !strings.Contains(message, "division by zero") {
		t.Fatalf("missing error detail: %v", err)
	}
	if !strings.Contains(message, "explode") {
		t.Fatalf("missing stack context: %v", err)
	}
}

func TestClosuresCaptureOuterLocals(t *testing.T) {
	source := `
		function makeAdder(base)
			function add(x)
				return base + x
			end
			return add
		end

		local add10 = makeAdder(10)
		print(add10(5))
	`

	if err := runSource(source); err != nil {
		t.Fatalf("closure capture failed: %v", err)
	}
}

func TestClosuresCanMutateCapturedLocals(t *testing.T) {
	source := `
		function makeCounter()
			local n = 0
			function next()
				n = n + 1
				return n
			end
			return next
		end

		local c = makeCounter()
		print(c(), c(), c())
	`

	if err := runSource(source); err != nil {
		t.Fatalf("closure mutation failed: %v", err)
	}
}

func TestExpandedBuiltinsAndModules(t *testing.T) {
	source := `
		import math
		import text
		import array
		import maplib
		import time

		local nums = range(0, 6)
		assert(len(nums) == 6, "range length mismatch")
		assert(sum(nums) == 15, "sum mismatch")
		assert(math.max(3, 9) == 9, "math.max mismatch")
		assert(math.pow(2, 3) == 8, "math.pow mismatch")

		local phrase = "tux,lang,rocks"
		local parts = text.split(phrase, ",")
		assert(len(parts) == 3, "split mismatch")
		assert(text.contains(phrase, "lang"), "contains mismatch")
		assert(text.startsWith(phrase, "tux"), "startsWith mismatch")
		assert(text.endsWith(phrase, "rocks"), "endsWith mismatch")

		local rev = array.reverse([1, 2, 3])
		local cut = array.slice([10, 20, 30, 40], 1, 3)
		assert(rev[0] == 3, "reverse mismatch")
		assert(cut[0] == 20, "slice mismatch")

		local bag = {"a": 1}
		bag = maplib.set(bag, "b", 2)
		assert(maplib.size(bag) == 2, "map set mismatch")
		bag = maplib.delete(bag, "a")
		assert(maplib.size(bag) == 1, "map delete mismatch")

		time.sleepMs(1)
	`

	if err := runSource(source); err != nil {
		t.Fatalf("expanded builtin/module runtime failed: %v", err)
	}
}

func TestNewAppFocusedModules(t *testing.T) {
	source := `
		import rand
		import base64
		import hash
		import url
		import uuid

		local token = uuid.v4()
		assert(type(token) == "string", "uuid should be string")

		local enc = base64.encode("tuxlang")
		local dec = base64.decode(enc)
		assert(dec == "tuxlang", "base64 decode mismatch")

		local digest = hash.sha256("abc")
		assert(len(digest) == 64, "sha256 length mismatch")

		local q = "name=tux&mode=dev"
		assert(url.queryGet(q, "name") == "tux", "queryGet mismatch")
		assert(url.decode(url.encode("a b")) == "a b", "url encode/decode mismatch")

		local n = rand.int(1, 3)
		assert(n >= 1 && n < 3, "rand.int out of range")
		local pick = rand.pick(["x", "y", "z"])
		assert(type(pick) == "string", "rand.pick mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("new module runtime failed: %v", err)
	}
}

func TestCsvAndRegexModules(t *testing.T) {
	source := `
		import csv
		import regex

		local rows = [
			["name", "role"],
			["tux", "dev"]
		]

		local text = csv.stringify(rows)
		local parsed = csv.parse(text)

		assert(len(parsed) == 2, "csv row count mismatch")
		assert(parsed[0][0] == "name", "csv header mismatch")
		assert(parsed[1][1] == "dev", "csv value mismatch")

		assert(regex.match("^tux", "tuxlang"), "regex.match mismatch")
		assert(regex.find("[0-9]+", "abc123def") == "123", "regex.find mismatch")

		local hits = regex.findAll("[a-z]+", "tux 123 lang")
		assert(len(hits) == 2, "regex.findAll length mismatch")
		assert(hits[0] == "tux", "regex.findAll first mismatch")
		assert(hits[1] == "lang", "regex.findAll second mismatch")

		assert(regex.replace("lang", "tuxlang", "language") == "tuxlanguage", "regex.replace mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("csv/regex runtime failed: %v", err)
	}
}

func TestRegexEscapedBackslashPattern(t *testing.T) {
	source := `
		import regex

		local pattern = "\\s+"
		local input = "tux   lang"
		local out = regex.replace(pattern, input, "-")
		assert(out == "tux-lang", "regex escaped backslash pattern mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("regex escaped backslash runtime failed: %v", err)
	}
}

func TestDirectIndexAssignmentForMapAndArray(t *testing.T) {
	source := `
		local arr = [10, 20, 30]
		arr[1] = 99
		assert(arr[1] == 99, "array index assignment mismatch")

		local m = {"name": "tux"}
		m["name"] = "penguin"
		m["age"] = 3
		assert(m["name"] == "penguin", "map existing key assignment mismatch")
		assert(m["age"] == 3, "map new key assignment mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("direct index assignment runtime failed: %v", err)
	}
}

func TestArrayIndexAssignmentOutOfRangeFails(t *testing.T) {
	source := `
		local arr = [1, 2]
		arr[5] = 9
	`

	err := runSource(source)
	if err == nil {
		t.Fatal("expected out-of-range assignment failure")
	}
	if !strings.Contains(err.Error(), "array index out of range") {
		t.Fatalf("expected out-of-range error, got: %v", err)
	}
}

func TestJsonAndFsModules(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "config.json")
	notePath := filepath.Join(root, "note.txt")
	filePathLit := filepath.ToSlash(filePath)
	notePathLit := filepath.ToSlash(notePath)

	source := fmt.Sprintf(`
		import json
		import fs
		import path

		local blob = json.stringify({"name": "tux", "count": 3, "tags": [1, 2, 3]})
		local parsed = json.parse(blob)
		assert(parsed["name"] == "tux", "json parse string mismatch")
		assert(parsed["count"] == 3, "json parse number mismatch")
		assert(parsed["tags"][2] == 3, "json parse array mismatch")

		fs.writeText(%q, blob)
		fs.writeText(%q, "hello")
		fs.appendText(%q, " world")
		assert(fs.exists(%q), "config file should exist")
		assert(fs.exists(%q), "fs.exists mismatch")
		assert(fs.isFile(%q), "fs.isFile mismatch")
		assert(fs.readText(%q) == "hello world", "fs read/write mismatch")
		assert(json.parse(fs.readText(%q))["name"] == "tux", "file json mismatch")
		assert(path.base(%q) == "note.txt", "path.base mismatch")
		assert(path.ext(%q) == ".txt", "path.ext mismatch")
		assert(path.clean(path.join("a", "..", "b")) == "b", "path.clean mismatch")
	`, filePathLit, notePathLit, notePathLit, filePathLit, notePathLit, notePathLit, notePathLit, filePathLit, notePathLit, notePathLit)

	if err := runSource(source); err != nil {
		t.Fatalf("json/fs runtime failed: %v", err)
	}

	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected config file on disk: %v", err)
	}
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("expected note file on disk: %v", err)
	}
}

func TestOptionalOOPClassAndInheritance(t *testing.T) {
	source := `
		class Animal
			function init(name)
				this.name = name
			end

			function speak()
				return "?"
			end
		end

		class Dog extends Animal
			function speak()
				return "woof " + this.name
			end
		end

		local a = new Animal("mystery")
		local d = new Dog("tux")

		assert(a.speak() == "?", "animal speak mismatch")
		assert(d.speak() == "woof tux", "dog speak mismatch")
		d.name = "max"
		assert(d.speak() == "woof max", "field assignment mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("oop runtime failed: %v", err)
	}
}

func TestTuxiesConcurrencyModule(t *testing.T) {
	source := `
		import tuxies
		import time

		function add(a, b)
			return a + b
		end

		function tag(label, ms)
			time.sleepMs(ms)
			return label
		end

		local one = tuxies.spawn(add, 7, 5)
		local two = tuxies.spawn(tag, "done", 1)

		assert(tuxies.join(one) == 12, "spawn/join mismatch")
		assert(tuxies.join(two) == "done", "task join mismatch")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("tuxies concurrency runtime failed: %v", err)
	}
}

func TestTuxiesHelpersAllRaceDone(t *testing.T) {
	source := `
		import tuxies
		import time
		import array

		function work(name, ms)
			time.sleepMs(ms)
			return name
		end

		local first = tuxies.spawn(work, "alpha", 25)
		local second = tuxies.spawn(work, "beta", 5)
		local third = tuxies.spawn(work, "gamma", 15)

		time.sleepMs(30)
		assert(tuxies.done(first), "done should report a finished task")

		local results = tuxies.all([first, second, third])
		assert(array.join(results, ",") == "alpha,beta,gamma", "all should preserve order")

		local winner = tuxies.race([
			tuxies.spawn(work, "slow", 20),
			tuxies.spawn(work, "fast", 1)
		])
		assert(winner == "fast", "race should return the first completed task")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("tuxies helper runtime failed: %v", err)
	}
}

func TestTuxiesMapWaitAndStatus(t *testing.T) {
	source := `
		import tuxies
		import time
		import array

		function double(n)
			time.sleepMs(2)
			return n * 2
		end

		local task = tuxies.spawn(double, 21)
		local before = tuxies.status(task)
		local value = tuxies.wait(task)
		local after = tuxies.status(task)
		local mapped = tuxies.map([1, 2, 3], double)

		assert(before == "pending", "status should start pending")
		assert(value == 42, "wait should return task result")
		assert(after == "done", "status should end done")
		assert(array.join(mapped, ",") == "2,4,6", "map should preserve order")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("tuxies map/status runtime failed: %v", err)
	}
}

func TestTuxiesCancelAndGroup(t *testing.T) {
	source := `
		import tuxies
		import time
		import array

		function slow(name, ms)
			time.sleepMs(ms)
			return name
		end

		function explode(name)
			return 10 / 0
		end

		local task = tuxies.spawn(slow, "later", 50)
		time.sleepMs(5)
		assert(tuxies.cancel(task), "cancel should request cancellation")
		assert(tuxies.status(task) == "canceled", "status should show canceled")
		assert(tuxies.done(task), "canceled task should be done")

		local canceled = tuxies.cancelAll([
			tuxies.spawn(slow, "a", 10),
			tuxies.spawn(slow, "b", 10)
		])
		assert(canceled == 2, "cancelAll should cancel every task")

		local grouped = tuxies.group([
			tuxies.spawn(slow, "ok", 5),
			tuxies.spawn(slow, "nice", 1)
		])
		assert(array.join(grouped, ",") == "ok,nice", "group should preserve order")
	`

	if err := runSource(source); err != nil {
		t.Fatalf("tuxies cancel/group runtime failed: %v", err)
	}
}

func TestTuxiesGroupCancelsOnError(t *testing.T) {
	source := `
		import tuxies
		import time

		function slow(name, ms)
			time.sleepMs(ms)
			return name
		end

		function explode(name)
			return 10 / 0
		end

		tuxies.group([
			tuxies.spawn(slow, "ok", 5),
			tuxies.spawn(explode, "boom"),
			tuxies.spawn(slow, "skip", 25)
		])
	`

	err := runSource(source)
	if err == nil {
		t.Fatal("expected group failure")
	}
	if !strings.Contains(err.Error(), "division by zero") {
		t.Fatalf("expected group error to mention division by zero, got: %v", err)
	}
}

func runSource(source string) error {
	l := lexer.New(source)
	p := parser.New(l)
	program, err := p.ParseProgram()
	if err != nil {
		return err
	}

	comp := compiler.New()
	fn, err := comp.Compile(program)
	if err != nil {
		return err
	}

	machine := vm.New()
	return machine.Run(fn)
}
