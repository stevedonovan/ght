package main

import (
	"fmt"
	"github.com/stevedonovan/ght/pointer"
	"github.com/stevedonovan/ght/term"
	"log"
	"os"
	"regexp"
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
	for _, p := range pairs {
		m[p[0]] = p[1]
	}
	return m
}

func pairsToMapInterface(pairs [][]string, vars Map) Map {
	m := make(Map)
	for _, p := range pairs {
		m[p[0]] = valueToInterface(p[1], vars)
	}
	return m
}

func quit(msg string) {
	if os.Getenv("GHT_PANIC") != "" {
		panic(msg)
	} else {
		term.BrightRed(os.Stderr, "ght: %s\n", msg)
		os.Exit(1)
	}
}

func checke(e error) {
	if e != nil {
		quit(e.Error())
	}
}

func keys(m map[string]string) []string {
	res := make([]string, len(m))
	i := 0
	for k := range m {
		res[i] = k
		i++
	}
	return res
}

type Pairs [][]string

func (p Pairs) KeyValue(idx int) (string, string) {
	return p[idx][0], p[idx][1]
}

func grabWhilePairs(args []string) (Pairs, []string) {
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

// useful stuff for turning Pairs like a=1 name=hello into JSON
func parsePairs(args [][]string, vars Map) Map {
	m := make(Map)
	for _, a := range args {
		setKey(m, a[0], valueToInterface(a[1], vars))
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

func pruneEnds(value string, start, end string) (string, bool) {
	if strings.HasPrefix(value, start) && strings.HasSuffix(value, end) {
		return value[1 : len(value)-1], true
	} else {
		return value, false
	}
}

func valueToInterface(value string, vars Map) interface{} {
	// arrays are simple expressions like [1,2] or [hello,barbie,doll]
	if avalue, ok := pruneEnds(value, "[", "]"); ok {
		parts := strings.Split(avalue, ",")
		arr := make(Array, len(parts))
		for i, p := range parts {
			arr[i] = valueToInterface(p, vars)
		}
		return arr
	}
	if lvalue, ok := pruneEnds(value, "{", "}"); ok && vars != nil {
		r, e := pointer.Filter(vars, lvalue, nil)
		if e != nil {
			log.Printf("cannot lookup %q: %v", lvalue, e)
			return value
		} else {
			return r
		}
	}
	if value == "true" || value == "false" {
		return value == "true"
	}
	// we can read @-files at the key-value level; @@ escapes initial @
	if strings.HasPrefix(value, "@") {
		value = value[1:]
		if strings.HasPrefix(value, "@") { // then just escape @
			return value
		}
		b, mtype := loadFileIfPossible(value, true)
		if mtype != "application/json" {
			return b
		} else {
			// we *could* convert to JSON, so slice the resulting object in...
			var res interface{}
			checke(unmarshal(b, &res))
			return res
		}
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		return v
	} else {
		return value
	}
}

func readKeyValuePairs(contents string) ([][]string, error) {
	var pairs [][]string
	// Windows?
	for _, line := range strings.Split(contents, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx == -1 {
			return pairs, fmt.Errorf("cannot split %q in key/value Pairs with =", line)
		}
		pairs = append(pairs, []string{line[:idx], line[idx+1:]})
	}
	return pairs, nil
}

func readKeyValueFile(contents string) (Map, error) {
	res := make(Map)
	pairs, err := readKeyValuePairs(contents)
	if err != nil {
		return res, err
	}
	res = parsePairs(pairs, nil)
	return res, nil
}

func containsVarExpansions(s string) bool {
	return varExpansion.MatchString(s)
}

var (
	// identifiers, but also with hyphens and periods (for indexing)
	varExpansion = regexp.MustCompile(`{[a-zA-Z][\w_\-.]*}`)
)

func expandVariables(value string, vars Map) string {
	return varExpansion.ReplaceAllStringFunc(value, func(s string) string {
		return lookup(s, vars, true)
	})
}

func lookup(s string, vars Map, full bool) string {
	if full {
		r, e := pointer.Filter(vars, s, nil)
		if e != nil {
			if v := os.Getenv(s); v != "" {
				return v
			} else {
				log.Printf("warning: with %q error %v", s, e)
				return ""
			}
		}
		return fmt.Sprintf("%v", r)
	} else if r, ok := vars[s]; ok {
		return fmt.Sprintf("%v", r)
	} else {
		log.Printf("%q is not defined", s)
		return ""
	}
}

func readEnvFile(f string) error {
	bb, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	//fmt.Println("len", len(bb))
	pairs, err := readKeyValuePairs(string(bb))
	if err != nil {
		return err
	}
	//fmt.Println("pairs", len(pairs))
	for _, pair := range pairs {
		//log.Printf("Setting %s to %q", pair[0], pair[1])
		os.Setenv(pair[0], pair[1])
	}
	return nil
}
