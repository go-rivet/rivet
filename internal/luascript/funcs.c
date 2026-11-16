// SPDX-FileCopyrightText: 2026 Timothy Rule
// SPDX-License-Identifier: MIT

//go:build cgo

#include <stdlib.h>
#include "lua.h"
#include "lauxlib.h"
#include "funcs.h"
#include "_cgo_export.h"

void rivet_lua_free_value(rivet_lua_value *value) {
	while (value != NULL) {
		rivet_lua_value *next = value->next;
		rivet_lua_free_value(value->child);
		free(value->text);
		free(value->key);
		free(value);
		value = next;
	}
}

static void rivet_lua_push_value(lua_State *state, const rivet_lua_value *value) {
	const rivet_lua_value *child;
	lua_Integer index;
	switch (value->type) {
	case RIVET_NIL: lua_pushnil(state); break;
	case RIVET_BOOL: lua_pushboolean(state, (int)value->integer); break;
	case RIVET_INTEGER: lua_pushinteger(state, value->integer); break;
	case RIVET_NUMBER: lua_pushnumber(state, value->number); break;
	case RIVET_STRING: lua_pushlstring(state, value->text, value->length); break;
	case RIVET_ARRAY:
		lua_newtable(state);
		index = 1;
		for (child = value->child; child != NULL; child = child->next) {
			rivet_lua_push_value(state, child);
			lua_rawseti(state, -2, index++);
		}
		break;
	case RIVET_MAP:
		lua_newtable(state);
		for (child = value->child; child != NULL; child = child->next) {
			lua_pushlstring(state, child->key, child->key_length);
			rivet_lua_push_value(state, child);
			lua_rawset(state, -3);
		}
		break;
	}
}

static int rivet_lua_push_result(lua_State *state) {
	rivet_lua_value *value = lua_touserdata(state, 1);
	if (!lua_checkstack(state, 220)) {
		return luaL_error(state, "rivet.fn: insufficient Lua stack space");
	}
	rivet_lua_push_value(state, value);
	return 1;
}

static int rivet_lua_fn(lua_State *state) {
	const char *name = lua_tostring(state, lua_upvalueindex(1));
	rivet_lua_value *value;
	int status;
	int failed;
	if (!lua_checkstack(state, 220)) {
		return luaL_error(state, "rivet.fn: insufficient Lua stack space");
	}
	value = goLuaFn(state, (char *)name);
	if (value == NULL) {
		return luaL_error(state, "rivet.fn: unable to allocate result");
	}
	failed = value->type == RIVET_ERROR;
	if (failed) {
		value->type = RIVET_STRING;
	}
	/* Keep Lua allocation errors inside C and free the result before rethrowing. */
	lua_pushcfunction(state, rivet_lua_push_result);
	lua_pushlightuserdata(state, value);
	status = lua_pcall(state, 1, 1, 0);
	rivet_lua_free_value(value);
	if (status != LUA_OK || failed) return lua_error(state);
	return 1;
}

static int rivet_lua_task_impl(lua_State *state) {
	uint64_t context_id = lua_tointeger(state, lua_upvalueindex(1));
	const char *task_name;
	rivet_lua_value *value;
	int status;
	int failed;

	if (!lua_checkstack(state, 220)) {
		return luaL_error(state, "rivet.task: insufficient Lua stack space");
	}

	if (lua_gettop(state) != 2) {
		return luaL_error(state, "rivet.task: expected 2 arguments (taskName, vars)");
	}

	task_name = lua_tostring(state, 1);
	if (task_name == NULL) {
		return luaL_error(state, "rivet.task: first argument must be a string task name");
	}

	if (!lua_istable(state, 2)) {
		return luaL_error(state, "rivet.task: second argument must be a table");
	}

	value = goLuaTask(context_id, (char *)task_name, state, 2);
	if (value == NULL) {
		return luaL_error(state, "rivet.task: unable to allocate result");
	}
	failed = value->type == RIVET_ERROR;
	if (failed) {
		value->type = RIVET_STRING;
	}
	/* Keep Lua allocation errors inside C and free the result before rethrowing. */
	lua_pushcfunction(state, rivet_lua_push_result);
	lua_pushlightuserdata(state, value);
	status = lua_pcall(state, 1, 1, 0);
	rivet_lua_free_value(value);
	if (status != LUA_OK || failed) return lua_error(state);
	return 1;
}

void rivet_lua_register_funcs(lua_State *state) {
	static const char *names[] = {
		"OS", "ARCH", "numCPU", "exeExt", "uuid",
		"catLines", "splitLines", "split", "fromSlash", "toSlash", "joinPath",
		"relPath", "absPath", "patsubst", "shellQuote", "splitArgs",
		"joinEnv", "joinUrl", "merge", "fromYaml", "toYaml"
	};
	size_t i;
	lua_getglobal(state, "rivet");
	lua_createtable(state, 0, (int)(sizeof(names) / sizeof(names[0])));
	for (i = 0; i < sizeof(names) / sizeof(names[0]); i++) {
		lua_pushstring(state, names[i]);
		lua_pushcclosure(state, rivet_lua_fn, 1);
		lua_setfield(state, -2, names[i]);
	}
	lua_setfield(state, -2, "fn");
	lua_pop(state, 1);
}

void rivet_lua_register_task(lua_State *state, uint64_t context_id) {
	lua_getglobal(state, "rivet");
	lua_pushinteger(state, (lua_Integer)context_id);
	lua_pushcclosure(state, rivet_lua_task_impl, 1);
	lua_setfield(state, -2, "task");
	lua_pop(state, 1);
}

static int rivet_lua_cmd_impl(lua_State *state) {
	uint64_t context_id = lua_tointeger(state, lua_upvalueindex(1));
	const char *cmd;
	rivet_lua_value *value;
	int status;
	int failed;

	if (!lua_checkstack(state, 220)) {
		return luaL_error(state, "rivet.cmd: insufficient Lua stack space");
	}

	if (lua_gettop(state) != 1) {
		return luaL_error(state, "rivet.cmd: expected 1 argument (cmd string)");
	}

	cmd = lua_tostring(state, 1);
	if (cmd == NULL) {
		return luaL_error(state, "rivet.cmd: argument must be a string");
	}

	value = goLuaCmd(context_id, (char *)cmd);
	if (value == NULL) {
		return luaL_error(state, "rivet.cmd: unable to allocate result");
	}
	failed = value->type == RIVET_ERROR;
	if (failed) {
		value->type = RIVET_STRING;
	}
	/* Keep Lua allocation errors inside C and free the result before rethrowing. */
	lua_pushcfunction(state, rivet_lua_push_result);
	lua_pushlightuserdata(state, value);
	status = lua_pcall(state, 1, 1, 0);
	rivet_lua_free_value(value);
	if (status != LUA_OK || failed) return lua_error(state);
	return 1;
}

void rivet_lua_register_cmd(lua_State *state, uint64_t context_id) {
	lua_getglobal(state, "rivet");
	lua_pushinteger(state, (lua_Integer)context_id);
	lua_pushcclosure(state, rivet_lua_cmd_impl, 1);
	lua_setfield(state, -2, "cmd");
	lua_pop(state, 1);
}

