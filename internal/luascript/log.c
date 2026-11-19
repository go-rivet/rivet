// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

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

void rivet_lua_interrupt_context(lua_State *state) {
	lua_State *main_state;
	lua_rawgeti(state, LUA_REGISTRYINDEX, LUA_RIDX_MAINTHREAD);
	main_state = lua_tothread(state, -1);
	lua_pop(state, 1);
	lua_sethook(state, rivet_lua_check_context, LUA_MASKCOUNT, 1);
	if (main_state != NULL && main_state != state) {
		lua_sethook(main_state, rivet_lua_check_context, LUA_MASKCOUNT, 1);
	}
}

static int rivet_lua_error_handler(lua_State *state) {
	uint64_t context_id = (uint64_t)lua_tointeger(state, lua_upvalueindex(2));
	if (goLuaContextCancelled(context_id)) {
		return luaL_error(state, "execution cancelled");
	}
	lua_pushvalue(state, lua_upvalueindex(1));
	lua_pushvalue(state, 1);
	lua_call(state, 1, 1);
	return 1;
}

/* Preserve coroutine yields while propagating cancellation through protected calls. */
static int rivet_lua_protected_continue(lua_State *state, int status, lua_KContext context) {
	(void)status;
	if (goLuaContextCancelled((uint64_t)context)) {
		return luaL_error(state, "execution cancelled");
	}
	return lua_gettop(state);
}

static int rivet_lua_protected_call(lua_State *state) {
	uint64_t context_id = (uint64_t)lua_tointeger(state, lua_upvalueindex(2));
	int count = lua_gettop(state);
	if (lua_toboolean(state, lua_upvalueindex(3)) && count >= 2 && lua_isfunction(state, 2)) {
		lua_pushvalue(state, 2);
		lua_pushinteger(state, (lua_Integer)context_id);
		lua_pushcclosure(state, rivet_lua_error_handler, 2);
		lua_replace(state, 2);
	}
	lua_pushvalue(state, lua_upvalueindex(1));
	lua_insert(state, 1);
	lua_callk(state, count, LUA_MULTRET, (lua_KContext)context_id, rivet_lua_protected_continue);
	return rivet_lua_protected_continue(state, LUA_OK, (lua_KContext)context_id);
}

void rivet_lua_protect_calls(lua_State *state, uint64_t context_id) {
	static const char *names[] = {"pcall", "xpcall"};
	int i;
	for (i = 0; i < 2; i++) {
		lua_getglobal(state, names[i]);
		lua_pushinteger(state, (lua_Integer)context_id);
		lua_pushboolean(state, i == 1);
		lua_pushcclosure(state, rivet_lua_protected_call, 3);
		lua_setglobal(state, names[i]);
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
