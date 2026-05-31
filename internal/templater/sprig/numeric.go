package sprig

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return 0
}

func toInt64(v interface{}) int64 {
	switch val := v.(type) {
	case int64:
		return val
	case int:
		return int64(val)
	case float64:
		return int64(val)
	case string:
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			return i
		}
	}
	return 0
}

func toInt(v interface{}) int {
	return int(toInt64(v))
}

func max(a interface{}, i ...interface{}) int64 {
	res := toInt64(a)
	for _, b := range i {
		if val := toInt64(b); val > res {
			res = val
		}
	}
	return res
}

func min(a interface{}, i ...interface{}) int64 {
	res := toInt64(a)
	for _, b := range i {
		if val := toInt64(b); val < res {
			res = val
		}
	}
	return res
}

func maxf(a interface{}, i ...interface{}) float64 {
	res := toFloat64(a)
	for _, b := range i {
		if val := toFloat64(b); val > res {
			res = val
		}
	}
	return res
}

func minf(a interface{}, i ...interface{}) float64 {
	res := toFloat64(a)
	for _, b := range i {
		if val := toFloat64(b); val < res {
			res = val
		}
	}
	return res
}

func until(count int) []int {
	if count < 0 {
		return untilStep(0, count, -1)
	}
	return untilStep(0, count, 1)
}

func untilStep(start, stop, step int) []int {
	v := []int{}
	if step == 0 || (start < stop && step < 0) || (start > stop && step > 0) {
		return v
	}
	if start < stop {
		for i := start; i < stop; i += step {
			v = append(v, i)
		}
	} else {
		for i := start; i > stop; i += step {
			v = append(v, i)
		}
	}
	return v
}

func floor(a interface{}) float64 { return math.Floor(toFloat64(a)) }
func ceil(a interface{}) float64  { return math.Ceil(toFloat64(a)) }

func round(a interface{}, p int, rOpt ...float64) float64 {
	roundOn := 0.5
	if len(rOpt) > 0 {
		roundOn = rOpt[0]
	}
	pow := math.Pow(10, float64(p))
	digit := pow * toFloat64(a)
	_, div := math.Modf(digit)

	if div >= roundOn {
		return math.Ceil(digit) / pow
	}
	return math.Floor(digit) / pow
}

func toDecimal(v interface{}) int64 {
	if res, err := strconv.ParseInt(fmt.Sprint(v), 8, 64); err == nil {
		return res
	}
	return 0
}

func seq(params ...int) string {
	var start, end, step int = 1, 0, 1
	switch len(params) {
	case 0:
		return ""
	case 1:
		end = params[0]
	case 2:
		start, end = params[0], params[1]
	case 3:
		start, step, end = params[0], params[1], params[2]
	default:
		return ""
	}

	if end < start && step > 0 {
		step = -1
	}
	res := untilStep(start, end+step, step)

	var fields []string
	for _, n := range res {
		fields = append(fields, strconv.Itoa(n))
	}
	return strings.Join(fields, " ")
}
