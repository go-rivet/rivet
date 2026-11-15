//go:build cgo

package luascript

/*
#cgo CFLAGS: -std=c99 -I${SRCDIR}/../lua/src
#include <stdlib.h>
#include "lua.h"
#include "funcs.h"

static void rivet_lua_pop(lua_State *state, int count) {
	lua_pop(state, count);
}
*/
import "C"

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"unsafe"

	"github.com/go-rivet/rivet/internal/templater/task"
)

const maxFuncDepth = 100

//export goLuaFn
func goLuaFn(L *C.lua_State, cName *C.char) (result *C.rivet_lua_value) {
	name := C.GoString(cName)
	defer func() {
		if recovered := recover(); recovered != nil {
			result = errorValue(fmt.Errorf("rivet.fn.%s: Go callback panic (%T)", name, recovered))
		}
	}()
	args := make([]any, int(C.lua_gettop(L)))
	for i := range args {
		value, err := readFuncValue(L, C.int(i+1), 0, make(map[unsafe.Pointer]bool))
		if err != nil {
			return errorValue(fmt.Errorf("rivet.fn.%s: argument %d: %w", name, i+1, err))
		}
		args[i] = value
	}
	value, err := callTaskFunc(name, args)
	if err != nil {
		return errorValue(fmt.Errorf("rivet.fn.%s: %w", name, err))
	}
	result, err = makeFuncValue(reflect.ValueOf(value), 0)
	if err != nil {
		return errorValue(fmt.Errorf("rivet.fn.%s: %w", name, err))
	}
	return result
}

func errorValue(err error) *C.rivet_lua_value {
	value := newFuncValue(C.RIVET_ERROR)
	if value != nil {
		text := err.Error()
		value.text = C.CString(text)
		value.length = C.size_t(len(text))
	}
	return value
}

func readFuncValue(L *C.lua_State, index C.int, depth int, active map[unsafe.Pointer]bool) (any, error) {
	if depth > maxFuncDepth {
		return nil, fmt.Errorf("table nesting exceeds %d levels", maxFuncDepth)
	}
	switch C.lua_type(L, index) {
	case C.LUA_TNIL:
		return nil, nil
	case C.LUA_TBOOLEAN:
		return C.lua_toboolean(L, index) != 0, nil
	case C.LUA_TSTRING:
		var length C.size_t
		text := C.lua_tolstring(L, index, &length)
		if uint64(length) > math.MaxInt32 {
			return nil, fmt.Errorf("string is too long")
		}
		return C.GoStringN(text, C.int(length)), nil
	case C.LUA_TNUMBER:
		if C.lua_isinteger(L, index) != 0 {
			return int64(C.lua_tointegerx(L, index, nil)), nil
		}
		return float64(C.lua_tonumberx(L, index, nil)), nil
	case C.LUA_TTABLE:
		index = C.lua_absindex(L, index)
		ptr := unsafe.Pointer(C.lua_topointer(L, index))
		if active[ptr] {
			return nil, fmt.Errorf("cyclic table")
		}
		active[ptr] = true
		defer delete(active, ptr)
		strings := make(map[string]any)
		indices := make(map[int64]any)
		C.lua_pushnil(L)
		for C.lua_next(L, index) != 0 {
			value, err := readFuncValue(L, -1, depth+1, active)
			if err != nil {
				C.rivet_lua_pop(L, 2)
				return nil, err
			}
			switch C.lua_type(L, -2) {
			case C.LUA_TSTRING:
				key, err := readFuncValue(L, -2, depth+1, active)
				if err != nil {
					C.rivet_lua_pop(L, 2)
					return nil, err
				}
				strings[key.(string)] = value
			case C.LUA_TNUMBER:
				if C.lua_isinteger(L, -2) == 0 {
					C.rivet_lua_pop(L, 2)
					return nil, fmt.Errorf("table indices must be positive integers")
				}
				key := int64(C.lua_tointegerx(L, -2, nil))
				if key < 1 {
					C.rivet_lua_pop(L, 2)
					return nil, fmt.Errorf("table indices must be positive integers")
				}
				indices[key] = value
			default:
				C.rivet_lua_pop(L, 2)
				return nil, fmt.Errorf("table keys must be strings or positive integers")
			}
			C.rivet_lua_pop(L, 1)
		}
		if len(strings) != 0 && len(indices) != 0 {
			return nil, fmt.Errorf("table cannot mix string and integer keys")
		}
		if len(indices) == 0 {
			return strings, nil
		}
		list := make([]any, len(indices))
		for index, value := range indices {
			if index > int64(len(list)) {
				return nil, fmt.Errorf("table indices must form a contiguous sequence")
			}
			list[index-1] = value
		}
		return list, nil
	default:
		return nil, fmt.Errorf("unsupported Lua type %s", C.GoString(C.lua_typename(L, C.lua_type(L, index))))
	}
}

func callTaskFunc(name string, args []any) (any, error) {
	stringsArg := func(min, max int) ([]string, error) {
		if len(args) < min || (max >= 0 && len(args) > max) {
			return nil, fmt.Errorf("wrong number of arguments: got %d", len(args))
		}
		out := make([]string, len(args))
		for i, arg := range args {
			value, ok := arg.(string)
			if !ok {
				return nil, fmt.Errorf("argument %d must be a string", i+1)
			}
			out[i] = value
		}
		return out, nil
	}
	switch name {
	case "OS", "ARCH", "exeExt", "uuid":
		if _, err := stringsArg(0, 0); err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func() string)(), nil
	case "numCPU":
		if _, err := stringsArg(0, 0); err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func() int)(), nil
	case "catLines", "fromSlash", "toSlash":
		values, err := stringsArg(1, 1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(string) string)(values[0]), nil
	case "splitLines":
		values, err := stringsArg(1, 1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(string) []string)(values[0]), nil
	case "split":
		values, err := stringsArg(2, 2)
		if err != nil {
			return nil, err
		}
		parts := []string{}
		for _, part := range strings.Split(values[0], values[1]) {
			if part = strings.TrimSpace(part); part != "" {
				parts = append(parts, part)
			}
		}
		return parts, nil
	case "absPath", "shellQuote":
		values, err := stringsArg(1, 1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(string) (string, error))(values[0])
	case "relPath":
		values, err := stringsArg(2, 2)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(string, string) (string, error))(values[0], values[1])
	case "splitArgs":
		values, err := stringsArg(1, 1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(string) ([]string, error))(values[0])
	case "joinPath", "joinEnv":
		values, err := stringsArg(0, -1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(...string) string)(values...), nil
	case "joinUrl":
		values, err := stringsArg(0, -1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs[name].(func(...string) (string, error))(values...)
	case "patsubst":
		if len(args) != 3 {
			return nil, fmt.Errorf("expected 3 arguments")
		}
		old, okOld := args[0].(string)
		new, okNew := args[1].(string)
		list, okList := args[2].([]any)
		if empty, ok := args[2].(map[string]any); ok && len(empty) == 0 {
			list, okList = []any{}, true
		}
		if !okOld || !okNew || !okList {
			return nil, fmt.Errorf("expected two strings and a sequence of strings")
		}
		paths := make([]string, len(list))
		for i, v := range list {
			var ok bool
			paths[i], ok = v.(string)
			if !ok {
				return nil, fmt.Errorf("path %d must be a string", i+1)
			}
		}
		return task.TaskFuncs[name].(func(string, string, []string) []string)(old, new, paths), nil
	case "merge":
		if len(args) == 0 {
			return nil, fmt.Errorf("expected at least one map")
		}
		maps := make([]map[string]any, len(args))
		for i, v := range args {
			var ok bool
			maps[i], ok = v.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("argument %d must be a string-keyed map", i+1)
			}
		}
		return task.TaskFuncs[name].(func(map[string]any, ...map[string]any) map[string]any)(maps[0], maps[1:]...), nil
	case "fromYaml":
		values, err := stringsArg(1, 1)
		if err != nil {
			return nil, err
		}
		return task.TaskFuncs["mustFromYaml"].(func(string) (any, error))(values[0])
	case "toYaml":
		if len(args) != 1 {
			return nil, fmt.Errorf("expected one argument")
		}
		return task.TaskFuncs["mustToYaml"].(func(any) (string, error))(args[0])
	default:
		return nil, fmt.Errorf("unknown function")
	}
}

func newFuncValue(kind C.int) *C.rivet_lua_value {
	value := (*C.rivet_lua_value)(C.calloc(1, C.size_t(unsafe.Sizeof(C.rivet_lua_value{}))))
	if value != nil {
		value._type = kind
	}
	return value
}

func makeFuncValue(value reflect.Value, depth int) (*C.rivet_lua_value, error) {
	if depth > maxFuncDepth {
		return nil, fmt.Errorf("result nesting exceeds %d levels", maxFuncDepth)
	}
	for value.IsValid() && (value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer) {
		if value.IsNil() {
			value = reflect.Value{}
			break
		}
		value = value.Elem()
	}
	kind := C.int(C.RIVET_NIL)
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Bool:
			kind = C.RIVET_BOOL
		case reflect.String:
			kind = C.RIVET_STRING
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			kind = C.RIVET_INTEGER
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if value.Uint() > math.MaxInt64 {
				return nil, fmt.Errorf("result integer is out of Lua integer range")
			}
			kind = C.RIVET_INTEGER
		case reflect.Float32, reflect.Float64:
			kind = C.RIVET_NUMBER
		case reflect.Slice, reflect.Array:
			kind = C.RIVET_ARRAY
		case reflect.Map:
			kind = C.RIVET_MAP
		default:
			return nil, fmt.Errorf("unsupported result type %s", value.Type())
		}
	}
	if depth > 0 && kind == C.RIVET_NIL {
		return nil, fmt.Errorf("null values inside tables cannot be represented in Lua")
	}
	out := newFuncValue(kind)
	if out == nil {
		return nil, fmt.Errorf("unable to allocate result")
	}
	fail := func(err error) (*C.rivet_lua_value, error) {
		C.rivet_lua_free_value(out)
		return nil, err
	}
	switch kind {
	case C.RIVET_BOOL:
		if value.Bool() {
			out.integer = 1
		}
	case C.RIVET_INTEGER:
		if value.Kind() >= reflect.Uint && value.Kind() <= reflect.Uint64 {
			out.integer = C.lua_Integer(value.Uint())
		} else {
			out.integer = C.lua_Integer(value.Int())
		}
	case C.RIVET_NUMBER:
		out.number = C.lua_Number(value.Float())
	case C.RIVET_STRING:
		text := value.String()
		out.text = C.CString(text)
		out.length = C.size_t(len(text))
	case C.RIVET_ARRAY, C.RIVET_MAP:
		var tail *C.rivet_lua_value
		appendChild := func(child *C.rivet_lua_value) {
			if tail == nil {
				out.child = child
			} else {
				tail.next = child
			}
			tail = child
		}
		if kind == C.RIVET_ARRAY {
			for i := 0; i < value.Len(); i++ {
				child, err := makeFuncValue(value.Index(i), depth+1)
				if err != nil {
					return fail(err)
				}
				appendChild(child)
			}
		} else {
			keys := value.MapKeys()
			for _, key := range keys {
				if key.Kind() == reflect.Interface {
					key = key.Elem()
				}
				if !key.IsValid() || key.Kind() != reflect.String {
					return fail(fmt.Errorf("result map keys must be strings"))
				}
			}
			sort.Slice(keys, func(i, j int) bool { return fmt.Sprint(keys[i].Interface()) < fmt.Sprint(keys[j].Interface()) })
			for _, key := range keys {
				child, err := makeFuncValue(value.MapIndex(key), depth+1)
				if err != nil {
					return fail(err)
				}
				text := fmt.Sprint(key.Interface())
				child.key = C.CString(text)
				child.key_length = C.size_t(len(text))
				appendChild(child)
			}
		}
	}
	return out, nil
}

//export goLuaTask
func goLuaTask(contextID C.uint64_t, cTaskName *C.char, L *C.lua_State, varIndex C.int) (result *C.rivet_lua_value) {
	taskName := C.GoString(cTaskName)
	defer func() {
		if recovered := recover(); recovered != nil {
			result = errorValue(fmt.Errorf("rivet.task: Go callback panic (%T)", recovered))
		}
	}()

	ctxValue, ok := logContexts.Load(uint64(contextID))
	if !ok {
		return errorValue(fmt.Errorf("rivet.task: context not found"))
	}
	runCtx := ctxValue.(runContext)
	if runCtx.taskRunner == nil {
		return errorValue(fmt.Errorf("rivet.task: task runner not available"))
	}

	vars, err := readFuncValue(L, varIndex, 0, make(map[unsafe.Pointer]bool))
	if err != nil {
		return errorValue(fmt.Errorf("rivet.task: failed to read variables: %w", err))
	}

	varsMap, ok := vars.(map[string]any)
	if !ok {
		return errorValue(fmt.Errorf("rivet.task: variables must be a map"))
	}

	if err := runCtx.taskRunner(runCtx.ctx, taskName, varsMap); err != nil {
		return errorValue(fmt.Errorf("rivet.task: %w", err))
	}

	result = newFuncValue(C.RIVET_NIL)
	return result
}

//export goLuaCmd
func goLuaCmd(contextID C.uint64_t, cCmd *C.char) (result *C.rivet_lua_value) {
	cmd := C.GoString(cCmd)
	defer func() {
		if recovered := recover(); recovered != nil {
			result = errorValue(fmt.Errorf("rivet.cmd: Go callback panic (%T)", recovered))
		}
	}()

	ctxValue, ok := logContexts.Load(uint64(contextID))
	if !ok {
		return errorValue(fmt.Errorf("rivet.cmd: context not found"))
	}
	runCtx := ctxValue.(runContext)
	if runCtx.cmdRunner == nil {
		return errorValue(fmt.Errorf("rivet.cmd: cmd runner not available"))
	}

	if err := runCtx.cmdRunner(runCtx.ctx, cmd); err != nil {
		return errorValue(fmt.Errorf("rivet.cmd: %w", err))
	}

	result = newFuncValue(C.RIVET_NIL)
	return result
}
