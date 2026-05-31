package sprig

import (
	"fmt"
	"math"
	"reflect"
	"sort"
)

func push(list interface{}, v interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot push on type %s", val.Kind()))
	}
	nl := make([]interface{}, val.Len())
	for i := 0; i < val.Len(); i++ {
		nl[i] = val.Index(i).Interface()
	}
	return append(nl, v)
}

func prepend(list interface{}, v interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot prepend on type %s", val.Kind()))
	}
	nl := make([]interface{}, val.Len())
	for i := 0; i < val.Len(); i++ {
		nl[i] = val.Index(i).Interface()
	}
	return append([]interface{}{v}, nl...)
}

func chunk(size int, list interface{}) [][]interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot chunk type %s", val.Kind()))
	}
	l := val.Len()
	if l == 0 || size <= 0 {
		return [][]interface{}{}
	}
	cs := int(math.Floor(float64(l-1)/float64(size)) + 1)
	nl := make([][]interface{}, cs)
	for i := 0; i < cs; i++ {
		clen := size
		if i == cs-1 {
			clen = l % size
			if clen == 0 {
				clen = size
			}
		}
		nl[i] = make([]interface{}, clen)
		for j := 0; j < clen; j++ {
			nl[i][j] = val.Index(i*size + j).Interface()
		}
	}
	return nl
}

func last(list interface{}) interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find last on type %s", val.Kind()))
	}
	if val.Len() == 0 {
		return nil
	}
	return val.Index(val.Len() - 1).Interface()
}

func first(list interface{}) interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find first on type %s", val.Kind()))
	}
	if val.Len() == 0 {
		return nil
	}
	return val.Index(0).Interface()
}

func rest(list interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find rest on type %s", val.Kind()))
	}
	if val.Len() == 0 {
		return nil
	}
	nl := make([]interface{}, val.Len()-1)
	for i := 1; i < val.Len(); i++ {
		nl[i-1] = val.Index(i).Interface()
	}
	return nl
}

func initial(list interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find initial on type %s", val.Kind()))
	}
	if val.Len() == 0 {
		return nil
	}
	nl := make([]interface{}, val.Len()-1)
	for i := 0; i < val.Len()-1; i++ {
		nl[i] = val.Index(i).Interface()
	}
	return nl
}

func sortAlpha(list interface{}) []string {
	val := reflect.Indirect(reflect.ValueOf(list))
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		return []string{fmt.Sprintf("%v", list)}
	}
	// Note: strslice parsing helper must be imported/declared in package
	a := strslice(list)
	sort.Strings(a)
	return a
}

func reverse(v interface{}) []interface{} {
	val := reflect.ValueOf(v)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find reverse on type %s", val.Kind()))
	}
	nl := make([]interface{}, val.Len())
	for i := 0; i < val.Len(); i++ {
		nl[val.Len()-i-1] = val.Index(i).Interface()
	}
	return nl
}

func compact(list interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot compact on type %s", val.Kind()))
	}
	nl := []interface{}{}
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		if !empty(item) { // Note: empty helper must be present in package
			nl = append(nl, item)
		}
	}
	return nl
}

func uniq(list interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find uniq on type %s", val.Kind()))
	}
	dest := []interface{}{}
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		if !inList(dest, item) {
			dest = append(dest, item)
		}
	}
	return dest
}

func inList(haystack []interface{}, needle interface{}) bool {
	for _, h := range haystack {
		if reflect.DeepEqual(needle, h) {
			return true
		}
	}
	return false
}

func without(list interface{}, omit ...interface{}) []interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find without on type %s", val.Kind()))
	}
	res := []interface{}{}
	for i := 0; i < val.Len(); i++ {
		item := val.Index(i).Interface()
		if !inList(omit, item) {
			res = append(res, item)
		}
	}
	return res
}

func has(needle interface{}, haystack interface{}) bool {
	if haystack == nil {
		return false
	}
	val := reflect.ValueOf(haystack)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("Cannot find has on type %s", val.Kind()))
	}
	for i := 0; i < val.Len(); i++ {
		if reflect.DeepEqual(needle, val.Index(i).Interface()) {
			return true
		}
	}
	return false
}

func slice(list interface{}, indices ...interface{}) interface{} {
	val := reflect.ValueOf(list)
	if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
		panic(fmt.Sprintf("list should be type of slice or array but %s", val.Kind()))
	}
	l := val.Len()
	if l == 0 {
		return nil
	}

	var start, end int
	if len(indices) > 0 {
		start = toInt(indices[0]) // Assumes your existing package-level toInt helper
	}
	if len(indices) < 2 {
		end = l
	} else {
		end = toInt(indices[1])
	}

	return val.Slice(start, end).Interface()
}

func concat(lists ...interface{}) interface{} {
	var res []interface{}
	for _, list := range lists {
		if list == nil {
			continue
		}
		val := reflect.ValueOf(list)
		if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
			panic(fmt.Sprintf("Cannot concat type %s as list", val.Kind()))
		}
		for i := 0; i < val.Len(); i++ {
			res = append(res, val.Index(i).Interface())
		}
	}
	return res
}
