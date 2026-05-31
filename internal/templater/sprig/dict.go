package sprig

import (
	"fmt"
)

func pluck(key string, d ...map[string]interface{}) []interface{} {
	res := []interface{}{}
	for _, dict := range d {
		if val, ok := dict[key]; ok {
			res = append(res, val)
		}
	}
	return res
}

func keys(dicts ...map[string]interface{}) []string {
	k := []string{}
	for _, dict := range dicts {
		for key := range dict {
			k = append(k, key)
		}
	}
	return k
}

func pick(dict map[string]interface{}, keys ...string) map[string]interface{} {
	res := map[string]interface{}{}
	for _, k := range keys {
		if v, ok := dict[k]; ok {
			res[k] = v
		}
	}
	return res
}

func omit(dict map[string]interface{}, keys ...string) map[string]interface{} {
	res := map[string]interface{}{}
	omitMap := make(map[string]bool, len(keys))
	for _, k := range keys {
		omitMap[k] = true
	}
	for k, v := range dict {
		if !omitMap[k] {
			res[k] = v
		}
	}
	return res
}

func dict(v ...interface{}) map[string]interface{} {
	dictMap := map[string]interface{}{}
	lenv := len(v)
	for i := 0; i < lenv; i += 2 {
		key := strval(v[i]) // Assumes your existing package-level strval helper
		if i+1 >= lenv {
			dictMap[key] = ""
			continue
		}
		dictMap[key] = v[i+1]
	}
	return dictMap
}

func values(dict map[string]interface{}) []interface{} {
	vals := make([]interface{}, 0, len(dict))
	for _, value := range dict {
		vals = append(vals, value)
	}
	return vals
}

func dig(ps ...interface{}) interface{} {
	if len(ps) < 3 {
		panic("dig needs at least three arguments")
	}

	// Unpack inputs from the parameter slice
	currentMap, ok := ps[len(ps)-1].(map[string]interface{})
	if !ok {
		panic("the last argument to dig must be a map[string]interface{}")
	}
	def := ps[len(ps)-2]

	// Flattened iterative traversal to avoid multi-file recursion
	for i := 0; i < len(ps)-2; i++ {
		key, ok := ps[i].(string)
		if !ok {
			panic(fmt.Sprintf("dig keys must be strings, got %T", ps[i]))
		}

		step, found := currentMap[key]
		if !found {
			return def
		}

		if i == len(ps)-3 {
			return step
		}

		currentMap, ok = step.(map[string]interface{})
		if !ok {
			return def
		}
	}
	return def
}
