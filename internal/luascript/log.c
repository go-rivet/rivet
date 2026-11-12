//go:build cgo

#include <stdint.h>

#include "lua.h"
#include "_cgo_export.h"

static char rivet_lua_context_key;

static void rivet_lua_push_arguments(lua_State *state, const char *separator) {
	int count = lua_gettop(state);
	int i;
	luaL_Buffer buffer;
	luaL_buffinit(state, &buffer);
	for (i = 1; i <= count; i++) {
		size_t length;
		const char *value;
		if (i > 1) {
			luaL_addstring(&buffer, separator);
		}
		value = luaL_tolstring(state, i, &length);
		luaL_addlstring(&buffer, value, length);
		lua_pop(state, 1);
	}
	luaL_pushresult(&buffer);
}

static void rivet_lua_check_context(lua_State *state, lua_Debug *debug) {
	uint64_t context_id;
	(void)debug;
	lua_rawgetp(state, LUA_REGISTRYINDEX, &rivet_lua_context_key);
	context_id = (uint64_t)lua_tointeger(state, -1);
	lua_pop(state, 1);
	if (goLuaContextCancelled(context_id)) {
		luaL_error(state, "execution cancelled");
	}
}

static int rivet_lua_log(lua_State *state) {
	int level = (int)lua_tointeger(state, lua_upvalueindex(1));
	uint64_t context_id = (uint64_t)lua_tointeger(state, lua_upvalueindex(2));
	size_t length;
	const char *message;
	rivet_lua_push_arguments(state, " ");
	message = lua_tolstring(state, -1, &length);
	if (goLuaLog(context_id, level, (char *)message, length) != 0) {
		lua_pop(state, 1);
		return luaL_error(state, "Lua logging callback failed");
	}
	lua_pop(state, 1);
	return 0;
}

static int rivet_lua_print(lua_State *state) {
	uint64_t context_id = (uint64_t)lua_tointeger(state, lua_upvalueindex(1));
	size_t length;
	const char *message;
	int result;
	rivet_lua_push_arguments(state, "\t");
	message = lua_tolstring(state, -1, &length);
	result = goLuaPrint(context_id, (char *)message, length);
	lua_pop(state, 1);
	if (result != 0) {
		return luaL_error(state, "unable to write Lua output");
	}
	return 0;
}

void rivet_lua_register_loggers(lua_State *state, uint64_t context_id) {
	static const char *names[] = {"trace", "debug", "info", "warn", "error"};
	int i;

	lua_createtable(state, 0, 1);
	lua_createtable(state, 0, 5);
	for (i = 0; i < 5; i++) {
		lua_pushinteger(state, i);
		lua_pushinteger(state, (lua_Integer)context_id);
		lua_pushcclosure(state, rivet_lua_log, 2);
		lua_setfield(state, -2, names[i]);
	}
	lua_setfield(state, -2, "log");
	lua_setglobal(state, "rivet");
	lua_pushinteger(state, (lua_Integer)context_id);
	lua_pushcclosure(state, rivet_lua_print, 1);
	lua_setglobal(state, "print");
}

void rivet_lua_set_context(lua_State *state, uint64_t context_id) {
	lua_pushinteger(state, (lua_Integer)context_id);
	lua_rawsetp(state, LUA_REGISTRYINDEX, &rivet_lua_context_key);
	lua_sethook(state, rivet_lua_check_context, LUA_MASKCOUNT, 10000);
}
