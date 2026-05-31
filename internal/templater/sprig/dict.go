package sprig

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
