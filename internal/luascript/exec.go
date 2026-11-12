//go:build cgo

package luascript

/*
#cgo CFLAGS: -std=c99 -I${SRCDIR}/../lua/src
#include <stdint.h>
#include <stdlib.h>

#include "lua.h"
#include "lauxlib.h"

void rivet_lua_register_loggers(lua_State *state, uint64_t context_id);
void rivet_lua_register_funcs(lua_State *state);
void rivet_lua_set_context(lua_State *state, uint64_t context_id);
int goLuaPrint(uint64_t context_id, char *message, size_t length);
int goLuaLog(uint64_t context_id, int level, char *message, size_t length);

static void rivet_lua_set_vars(lua_State *state) {
	lua_setfield(state, -2, "vars");
	lua_setglobal(state, "ctx");
}
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"reflect"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/go-rivet/rivet/internal/lua"
	"github.com/go-rivet/rivet/pkg/rlog"
)

const (
	logTrace = iota
	logDebug
	logInfo
	logWarn
	logError
)

var (
	logContextID atomic.Uint64
	logContexts  sync.Map
)

type runContext struct {
	ctx    context.Context
	stdout io.Writer
}

// Run starts a fresh Lua state and makes a snapshot of vars available as ctx.vars.
// Scripts can use rivet.log.trace, debug, info, warn, and error.
// Lua's built-in error() function reports failures; returned values are ignored.
func Run(ctx context.Context, opts *Options) (runErr error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = fmt.Errorf("lua: recovered Go panic (%T)", recovered)
		}
	}()

	if opts == nil {
		return errors.New("lua: nil options")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	state, err := lua.NewRuntime()
	if err != nil {
		return err
	}
	defer state.Close()

	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}
	id := logContextID.Add(1)
	logContexts.Store(id, runContext{ctx: ctx, stdout: stdout})
	defer logContexts.Delete(id)

	return state.WithState(func(statePtr unsafe.Pointer) error {
		L := (*C.lua_State)(statePtr)
		C.rivet_lua_register_loggers(L, C.uint64_t(id))
		C.rivet_lua_register_funcs(L)
		C.rivet_lua_set_context(L, C.uint64_t(id))

		names := make([]string, 0, len(opts.Vars))
		for name := range opts.Vars {
			names = append(names, name)
		}
		sort.Strings(names)
		C.lua_createtable(L, 0, 1)
		C.lua_createtable(L, 0, C.int(len(names)))
		for _, name := range names {
			cName := C.CString(name)
			C.lua_pushlstring(L, cName, C.size_t(len(name)))
			C.free(unsafe.Pointer(cName))
			if err := pushValue(L, reflect.ValueOf(opts.Vars[name]), 0); err != nil {
				return fmt.Errorf("lua: setting variable %q: %w", name, err)
			}
			C.lua_settable(L, -3)
		}
		C.rivet_lua_set_vars(L)

		source := C.CBytes([]byte(opts.Script))
		defer C.free(source)
		chunkName := C.CString("task")
		defer C.free(unsafe.Pointer(chunkName))
		if status := C.luaL_loadbufferx(L, (*C.char)(source), C.size_t(len(opts.Script)), chunkName, nil); status != C.LUA_OK {
			return luaError(L)
		}
		if status := C.lua_pcallk(L, 0, 0, 0, 0, nil); status != C.LUA_OK {
			if err := ctx.Err(); err != nil {
				return err
			}
			return luaError(L)
		}
		return ctx.Err()
	})
}

func luaError(L *C.lua_State) error {
	var length C.size_t
	message := C.lua_tolstring(L, -1, &length)
	if message == nil {
		return &ScriptError{Message: "script failed with a non-string error"}
	}
	return &ScriptError{Message: C.GoStringN(message, C.int(length))}
}

func pushValue(L *C.lua_State, value reflect.Value, depth int) error {
	if depth > 100 {
		return errors.New("variable nesting exceeds 100 levels")
	}
	if !value.IsValid() {
		C.lua_pushnil(L)
		return nil
	}
	if value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			C.lua_pushnil(L)
			return nil
		}
		return pushValue(L, value.Elem(), depth+1)
	}

	switch value.Kind() {
	case reflect.Bool:
		C.lua_pushboolean(L, cbool(value.Bool()))
	case reflect.String:
		text := value.String()
		cText := C.CString(text)
		C.lua_pushlstring(L, cText, C.size_t(len(text)))
		C.free(unsafe.Pointer(cText))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		C.lua_pushinteger(L, C.lua_Integer(value.Int()))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		number := value.Uint()
		if number <= math.MaxInt64 {
			C.lua_pushinteger(L, C.lua_Integer(number))
		} else {
			C.lua_pushnumber(L, C.lua_Number(number))
		}
	case reflect.Float32, reflect.Float64:
		C.lua_pushnumber(L, C.lua_Number(value.Float()))
	case reflect.Slice, reflect.Array:
		C.lua_createtable(L, C.int(value.Len()), 0)
		for i := 0; i < value.Len(); i++ {
			if err := pushValue(L, value.Index(i), depth+1); err != nil {
				return err
			}
			C.lua_rawseti(L, -2, C.lua_Integer(i+1))
		}
	case reflect.Map:
		if value.IsNil() {
			C.lua_pushnil(L)
			return nil
		}
		if value.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("unsupported map key type %s", value.Type().Key())
		}
		C.lua_createtable(L, 0, C.int(value.Len()))
		iter := value.MapRange()
		for iter.Next() {
			if err := pushValue(L, iter.Value(), depth+1); err != nil {
				return err
			}
			key := iter.Key().String()
			cKey := C.CString(key)
			C.lua_setfield(L, -2, cKey)
			C.free(unsafe.Pointer(cKey))
		}
	default:
		return fmt.Errorf("unsupported value type %s", value.Type())
	}
	return nil
}

func cbool(value bool) C.int {
	if value {
		return 1
	}
	return 0
}

//export goLuaLog
func goLuaLog(contextID C.uint64_t, level C.int, message *C.char, length C.size_t) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 1
		}
	}()

	ctxValue, ok := logContexts.Load(uint64(contextID))
	if !ok {
		return 1
	}
	ctx := ctxValue.(runContext).ctx
	text := C.GoStringN(message, C.int(length))
	switch int(level) {
	case logTrace:
		rlog.Trace(ctx, text)
	case logDebug:
		rlog.Debug(ctx, text)
	case logInfo:
		rlog.Info(ctx, text)
	case logWarn:
		rlog.Warn(ctx, text)
	case logError:
		rlog.Error(ctx, text)
	}
	return 0
}

//export goLuaPrint
func goLuaPrint(contextID C.uint64_t, message *C.char, length C.size_t) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 1
		}
	}()

	ctxValue, ok := logContexts.Load(uint64(contextID))
	if !ok {
		return 1
	}
	run := ctxValue.(runContext)
	if _, err := fmt.Fprintln(run.stdout, C.GoStringN(message, C.int(length))); err != nil {
		return 1
	}
	return 0
}

//export goLuaContextCancelled
func goLuaContextCancelled(contextID C.uint64_t) (result C.int) {
	defer func() {
		if recover() != nil {
			result = 1
		}
	}()

	ctxValue, ok := logContexts.Load(uint64(contextID))
	if !ok {
		return 1
	}
	if ctxValue.(runContext).ctx.Err() != nil {
		return 1
	}
	return 0
}
