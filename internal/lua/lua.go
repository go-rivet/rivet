//go:build cgo

package lua

/*
#cgo CFLAGS: -std=c99 -I${SRCDIR}/src
#cgo !windows CFLAGS: -DLUA_USE_POSIX
#cgo !windows LDFLAGS: -lm

#include "lua.h"
#include "lualib.h"
#include "lauxlib.h"

static int rivet_lua_disabled_os_exit(lua_State *state) {
	return luaL_error(state, "os.exit is disabled; use error() to fail the script");
}

static void rivet_lua_disable_os_exit(lua_State *state) {
	lua_getglobal(state, "os");
	lua_pushcfunction(state, rivet_lua_disabled_os_exit);
	lua_setfield(state, -2, "exit");
	lua_pop(state, 1);
}
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

// Runtime owns a Lua state. It must not be copied after first use.
type Runtime struct {
	master *C.lua_State
	mu     sync.Mutex
}

// NewRuntime creates a Lua 5.4 state and opens its standard libraries.
func NewRuntime() (*Runtime, error) {
	master := C.luaL_newstate()
	if master == nil {
		return nil, errors.New("lua: could not create state")
	}
	C.luaL_openlibs(master)
	C.rivet_lua_disable_os_exit(master)
	return &Runtime{master: master}, nil
}

// Close destroys the Lua state and is safe to call more than once.
func (r *Runtime) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.master != nil {
		C.lua_close(r.master)
		r.master = nil
	}
}

// WithState calls fn while holding the runtime lock. The state pointer is only
// valid for the duration of fn and must not be retained.
func (r *Runtime) WithState(fn func(unsafe.Pointer) error) error {
	if fn == nil {
		return errors.New("lua: nil state callback")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.master == nil {
		return errors.New("lua: runtime is closed")
	}
	return fn(unsafe.Pointer(r.master))
}
