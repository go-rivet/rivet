package sprig

import (
	"fmt"
	"net/url"
)

func urlParse(v string) map[string]interface{} {
	parsedURL, err := url.Parse(v)
	if err != nil {
		panic(fmt.Sprintf("unable to parse url: %s", err))
	}

	userinfo := ""
	if parsedURL.User != nil {
		userinfo = parsedURL.User.String()
	}

	return map[string]interface{}{
		"scheme":   parsedURL.Scheme,
		"host":     parsedURL.Host,
		"hostname": parsedURL.Hostname(),
		"path":     parsedURL.Path,
		"query":    parsedURL.RawQuery,
		"opaque":   parsedURL.Opaque,
		"fragment": parsedURL.Fragment,
		"userinfo": userinfo,
	}
}

func urlJoin(d map[string]interface{}) string {
	getStr := func(key string) string {
		val, ok := d[key]
		if !ok || val == nil {
			return ""
		}
		str, ok := val.(string)
		if !ok {
			panic(fmt.Sprintf("unable to parse %s key, must be of type string, but %T found", key, val))
		}
		return str
	}

	resURL := url.URL{
		Scheme:   getStr("scheme"),
		Host:     getStr("host"),
		Path:     getStr("path"),
		RawQuery: getStr("query"),
		Opaque:   getStr("opaque"),
		Fragment: getStr("fragment"),
	}

	if userinfo := getStr("userinfo"); userinfo != "" {
		tempURL, err := url.Parse(fmt.Sprintf("proto://%s@host", userinfo))
		if err != nil {
			panic(fmt.Sprintf("unable to parse userinfo in dict: %s", err))
		}
		resURL.User = tempURL.User
	}

	return resURL.String()
}
