package sprig

import (
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/adler32"
	"math"
	"net"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	mrand "math/rand/v2"

	"github.com/go-task/template"
)

var SprigFuncs = template.FuncMap{

	// --- Date ---
	"date":             func(fmt string, d interface{}) string { return dateInZone(fmt, d, "Local") },
	"htmlDate":         func(d interface{}) string { return dateInZone("2006-01-02", d, "Local") },
	"htmlDateInZone":   func(d interface{}, zone string) string { return dateInZone("2006-01-02", d, zone) },
	"toDate":           func(fmt, str string) time.Time { t, _ := time.ParseInLocation(fmt, str, time.Local); return t },
	"mustToDate":       func(fmt, str string) (time.Time, error) { return time.ParseInLocation(fmt, str, time.Local) },
	"unixEpoch":        func(date time.Time) string { return strconv.FormatInt(date.Unix(), 10) },
	"dateInZone":       dateInZone,
	"dateModify":       dateModify,
	"mustDateModify":   mustDateModify,
	"dateAgo":          dateAgo,
	"duration":         duration,
	"durationRound":    durationRound,
	"ago":              dateAgo,
	"date_in_zone":     dateInZone,
	"date_modify":      dateModify,
	"must_date_modify": mustDateModify,
	"now":              time.Now,

	// --- String ---
	"base64encode": func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) },
	"base32encode": func(v string) string { return base32.StdEncoding.EncodeToString([]byte(v)) },
	"indent": func(sp int, v string) string {
		return strings.Repeat(" ", sp) + strings.Replace(v, "\n", "\n"+strings.Repeat(" ", sp), -1)
	},
	"nindent": func(sp int, v string) string {
		return "\n" + strings.Repeat(" ", sp) + strings.Replace(v, "\n", "\n"+strings.Repeat(" ", sp), -1)
	},
	"replace": func(old, new, src string) string { return strings.Replace(src, old, new, -1) },
	"plural": func(one, many string, count int) string {
		if count == 1 {
			return one
		}
		return many
	},
	"join":       func(sep string, v interface{}) string { return strings.Join(strslice(v), sep) },
	"trunc":      trunc,
	"trim":       strings.TrimSpace,
	"upper":      strings.ToUpper,
	"lower":      strings.ToLower,
	"title":      strings.Title,
	"substr":     substring,
	"repeat":     func(count int, str string) string { return strings.Repeat(str, count) },
	"trimAll":    func(a, b string) string { return strings.Trim(b, a) },
	"trimSuffix": func(a, b string) string { return strings.TrimSuffix(b, a) },
	"trimPrefix": func(a, b string) string { return strings.TrimPrefix(b, a) },
	"contains":   func(substr string, str string) bool { return strings.Contains(str, substr) },
	"hasPrefix":  func(substr string, str string) bool { return strings.HasPrefix(str, substr) },
	"hasSuffix":  func(substr string, str string) bool { return strings.HasSuffix(str, substr) },
	"quote":      quote,
	"squote":     squote,
	"cat":        cat,
	"toString":   strval,

	// --- Checksum ---
	"sha256sum":  func(input string) string { hash := sha256.Sum256([]byte(input)); return hex.EncodeToString(hash[:]) },
	"sha1sum":    func(input string) string { hash := sha1.Sum([]byte(input)); return hex.EncodeToString(hash[:]) },
	"adler32sum": func(input string) string { return fmt.Sprintf("%d", adler32.Checksum([]byte(input))) },

	// --- Numerical ---
	"toInt":     func(v interface{}) int { return toInt(v) },
	"toInt64":   func(v interface{}) int64 { return toInt64(v) },
	"toFloat64": func(v interface{}) float64 { return toFloat64(v) },
	"floor":     func(v interface{}) float64 { return math.Floor(toFloat64(v)) },
	"ceil":      func(v interface{}) float64 { return math.Ceil(toFloat64(v)) },
	"toDecimal": func(v interface{}) int64 { return toDecimal(v) },
	"max":       max,
	"maxf":      maxf,
	"min":       min,
	"minf":      minf,
	"until":     until,
	"untilStep": untilStep,
	"round":     round,
	"seq":       seq,
	"atoi":      func(a string) int { i, _ := strconv.Atoi(a); return i },
	"int64":     toInt64,
	"int":       toInt,
	"float64":   toFloat64,

	// --- List ---
	"list":      func(v ...interface{}) []interface{} { return v },
	"push":      push,
	"append":    push,
	"prepend":   prepend,
	"chunk":     chunk,
	"last":      last,
	"first":     first,
	"rest":      rest,
	"initial":   initial,
	"sortAlpha": sortAlpha,
	"reverse":   reverse,
	"compact":   compact,
	"uniq":      uniq,
	"without":   without,
	"has":       has,
	"slice":     slice,
	"concat":    concat,

	"split":     split,
	"splitList": func(sep, orig string) []string { return strings.Split(orig, sep) },
	"splitn":    splitn,
	"toStrings": strslice,

	// --- Basic Arithmetic ---
	"add1": func(i interface{}) int64 { return toInt64(i) + 1 },
	"add": func(i ...interface{}) int64 {
		var a int64 = 0
		for _, b := range i {
			a += toInt64(b)
		}
		return a
	},
	"sub": func(a, b interface{}) int64 { return toInt64(a) - toInt64(b) },
	"div": func(a, b interface{}) int64 {
		den := toInt64(b)
		if den == 0 {
			return 0
		}
		return toInt64(a) / den
	},
	"mod": func(a, b interface{}) int64 { return toInt64(a) % toInt64(b) },
	"mul": func(a interface{}, v ...interface{}) int64 {
		val := toInt64(a)
		for _, b := range v {
			val = val * toInt64(b)
		}
		return val
	},
	"randInt": func(min, max int) int {
		diff := max - min
		if diff <= 0 {
			return min
		}
		return mrand.IntN(diff) + min
	},
	"biggest": max,

	// --- Utility ---
	"hello": func() string { return "Hello!" },
	"default": func(d interface{}, given ...interface{}) interface{} {
		if empty(given) || empty(given[0]) {
			return d
		}
		return given[0]
	},
	"fromJson":         func(v string) interface{} { out, _ := mustFromJson(v); return out },
	"mustFromJson":     mustFromJson,
	"toJson":           func(v interface{}) string { out, _ := json.Marshal(v); return string(out) },
	"mustToJson":       mustToJson,
	"toPrettyJson":     func(v interface{}) string { out, _ := json.MarshalIndent(v, "", "  "); return string(out) },
	"mustToPrettyJson": mustToPrettyJson,
	"ternary": func(vt interface{}, vf interface{}, v bool) interface{} {
		if v {
			return vt
		}
		return vf
	},
	"empty":         empty,
	"coalesce":      coalesce,
	"all":           all,
	"any":           anyFunc,
	"toRawJson":     toRawJson,
	"mustToRawJson": mustToRawJson,

	// --- Reflection ---
	"typeOf": func(src interface{}) string { return fmt.Sprintf("%T", src) },
	"typeIs": func(target string, src interface{}) bool { return target == fmt.Sprintf("%T", src) },
	"typeIsLike": func(target string, src interface{}) bool {
		t := fmt.Sprintf("%T", src)
		return target == t || "*"+target == t
	},
	"kindOf":    func(src interface{}) string { return reflect.ValueOf(src).Kind().String() },
	"kindIs":    func(target string, src interface{}) bool { return target == reflect.ValueOf(src).Kind().String() },
	"deepEqual": reflect.DeepEqual,

	// --- OS ---
	"env":       os.Getenv,
	"expandenv": os.ExpandEnv,

	// --- Network ---
	"getHostByName": func(name string) string {
		addrs, _ := net.LookupHost(name)
		if len(addrs) == 0 {
			return ""
		}
		return addrs[mrand.IntN(len(addrs))]
	},

	// --- Path ---
	"base":  path.Base,
	"dir":   path.Dir,
	"clean": path.Clean,
	"ext":   path.Ext,
	"isAbs": path.IsAbs,

	// --- Filepaths ---
	"osBase":  filepath.Base,
	"osClean": filepath.Clean,
	"osDir":   filepath.Dir,
	"osExt":   filepath.Ext,
	"osIsAbs": filepath.IsAbs,

	// --- Encoding ---
	"b64enc": func(v string) string { return base64.StdEncoding.EncodeToString([]byte(v)) },
	"b64dec": base64decode,
	"b32enc": func(v string) string { return base32.StdEncoding.EncodeToString([]byte(v)) },
	"b32dec": base32decode,

	// --- Data Structure ---
	"get": func(d map[string]interface{}, k string) interface{} {
		if val, ok := d[k]; ok {
			return val
		}
		return ""
	},
	"set":    func(d map[string]interface{}, k string, v interface{}) map[string]interface{} { d[k] = v; return d },
	"unset":  func(d map[string]interface{}, k string) map[string]interface{} { delete(d, k); return d },
	"hasKey": func(d map[string]interface{}, k string) bool { _, ok := d[k]; return ok },
	"tuple":  func(v ...interface{}) []interface{} { return v },
	"dict":   dict,
	"pluck":  pluck,
	"keys":   keys,
	"pick":   pick,
	"omit":   omit,
	"values": values,

	// --- Flow Control ---
	"fail": func(msg string) (string, error) { return "", errors.New(msg) },

	// --- Regex ---
	"regexMatch":             func(rx, s string) bool { m, _ := regexp.MatchString(rx, s); return m },
	"mustRegexMatch":         func(rx, s string) (bool, error) { return regexp.MatchString(rx, s) },
	"regexFindAll":           func(rx, s string, n int) []string { return regexp.MustCompile(rx).FindAllString(s, n) },
	"regexFind":              func(rx, s string) string { return regexp.MustCompile(rx).FindString(s) },
	"regexReplaceAll":        func(rx, s, repl string) string { return regexp.MustCompile(rx).ReplaceAllString(s, repl) },
	"regexReplaceAllLiteral": func(rx, s, repl string) string { return regexp.MustCompile(rx).ReplaceAllLiteralString(s, repl) },
	"regexSplit":             func(rx, s string, n int) []string { return regexp.MustCompile(rx).Split(s, n) },
	"regexQuoteMeta":         func(s string) string { return regexp.QuoteMeta(s) },

	// --- URL ---
	"urlParse": urlParse,
	"urlJoin":  urlJoin,
}
