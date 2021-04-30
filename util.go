package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
)

func splitDot2(s string) (string, string, bool) {
	parts := strings.SplitN(s, ".", 2)
	if len(parts) > 1 {
		return parts[0], parts[1], true
	} else {
		return "", "", false
	}
}

func pairsToMap(pairs [][]string) map[string]string {
	m := make(map[string]string)
	for _,p := range pairs {
		m[p[0]] = p[1]
	}
	return m
}

func marshal(data interface{}) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if e := enc.Encode(data); e != nil {
		return "", e
	}
	return b.String(), nil
}

func unmarshal(text string, obj interface{}) error {
	dec := json.NewDecoder(strings.NewReader(text))
	return dec.Decode(obj)
}

func quit(msg string) {
	//fmt.Fprintln(os.Stderr, "ght:", msg)
	panic(msg)
	os.Exit(1)
}

func checke(e error) {
	if e != nil {
		quit(e.Error())
	}
}

func grabWhilePairs(args []string) ([][]string, []string) {
	out := [][]string{}
	i := 0
	for ; i < len(args); i++ {
		a := args[i]
		idx := strings.Index(a, "=")
		if idx == -1 {
			break
		}
		key, value := a[0:idx], a[idx+1:]
		out = append(out, []string{key, value})
	}
	return out, args[i:]
}

// useful stuff for turning pairs like a=1 name=hello into JSON
func parsePairs(args [][]string) Map {
	m := make(Map)
	for _, a := range args {
		setKey(m, a[0], valueToInterface(a[1]))
	}
	return m
}

func setKey(m Map, key string, value interface{}) {
	subkey, rest, ok := splitDot2(key)
	if ok {
		subm, ok := m[subkey]
		if !ok {
			subm = make(Map)
			m[subkey] = subm
		}
		mm, ok := subm.(Map)
		if !ok {
			quit("key" + subkey + "is not an object")
		}
		setKey(mm, rest, value)
	} else {
		m[key] = value
	}
}

func valueToInterface(value string) interface{} {
	parts := strings.Split(value, ",")
	if len(parts) > 1 {
		arr := make(Array, len(parts))
		for i, p := range parts {
			arr[i] = valueToInterface(p)
		}
		return arr
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		return v
	} else {
		return value
	}
}

