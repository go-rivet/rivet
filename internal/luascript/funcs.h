#ifndef RIVET_LUA_FUNCS_H
#define RIVET_LUA_FUNCS_H

#include <stddef.h>
#include <stdint.h>
#include "lua.h"

typedef struct rivet_lua_value {
	int type;
	char *text;
	size_t length;
	lua_Integer integer;
	lua_Number number;
	struct rivet_lua_value *child;
	struct rivet_lua_value *next;
	char *key;
	size_t key_length;
} rivet_lua_value;

enum {
	RIVET_NIL, RIVET_BOOL, RIVET_INTEGER, RIVET_NUMBER, RIVET_STRING,
	RIVET_ARRAY, RIVET_MAP, RIVET_ERROR
};

void rivet_lua_free_value(rivet_lua_value *value);
void rivet_lua_register_funcs(lua_State *state);
void rivet_lua_register_task(lua_State *state, uint64_t context_id);
void rivet_lua_register_cmd(lua_State *state, uint64_t context_id);

#endif
