package main

import (
	"fmt"
	"github.com/stevedonovan/ght/pointer"
	"github.com/stevedonovan/ght/term"
	"log"
	"net/url"
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
	for _,p := range pairs {
		m[p[0]] = p[1]
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
	res := make([]string,len(m))
	i := 0
	for k := range m {
		res[i] = k
		i++
	}
	return res
}

type Pairs [][]string

func NewFilePair(file string) Pairs {
	return Pairs{{"@",file}}
}

func (p Pairs) KeyValue(idx int) (string,string) {
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
	if value == "true" || value == "false" {
		return value == "true"
	}
	if v, err := strconv.ParseFloat(value, 64); err == nil {
		return v
	} else {
		return value
	}
}

func readKeyValueFile(contents string) (Map,error) {
	res := make(Map)
	pairs := [][]string{}
	// Windows?
	for _,line := range strings.Split(contents,"\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line,"#") {
			continue
		}
		idx := strings.Index(line,"=")
		if idx == -1 {
			return res,fmt.Errorf("cannot split %q in key/value Pairs with =", line)
		}
		pairs = append(pairs,[]string{line[:idx],line[idx+1:]})
	}
	res = parsePairs(pairs)
	return res,nil
}

func containsVarExpansions(s string) bool {
	return varExpansion.MatchString(s)
}
var (
	//regularVar = regexp.MustCompile(`[a-z][\w_-]*`)
	varExpansion = regexp.MustCompile(`{[/?]*[a-zA-Z][\w_\-,.]*}`)
)

type Pair struct {
	name string
	value string
}

func expandVariables(value string, vars Map) string {
	return varExpansion.ReplaceAllStringFunc(value,func(s string) string {
		s = s[1:len(s)-1] // trim the {}
		if s[0] == '/' || s[0] == '?' {
			op := s[0]
			s = s[1:]
			pairs := []Pair{}
			for _,p := range strings.Split(s,",") {
				r := lookup(p, vars, false)
				if r != "" {
					pairs = append(pairs,Pair{p,r})
				}
			}
			subst := strings.Builder{}
			if op == '/' {
				for _,p := range pairs {
					value := url.PathEscape(p.value)
					subst.WriteString("/" + value)
				}
			} else {
				sep := "?"
				for _,p := range pairs {
					value := url.QueryEscape(p.value)
					subst.WriteString(sep + p.name + "=" + value)
					sep = "&"
				}
			}
			return subst.String()
		} else {
			return lookup(s, vars, true)
		}
	})
}

func lookup(s string, vars Map, full bool) string {
	if full {
		r,e := pointer.Filter(vars,s,nil)
		if e != nil {
			if v := os.Getenv(s); v != "" {
				return v
			} else {
				log.Printf("warning: with %q error %v", s, e)
				return ""
			}
		}
		return fmt.Sprintf("%v",r)
	} else if r, ok := vars[s]; ok {
		return fmt.Sprintf("%v",r)
	} else {
		log.Printf("%q is not defined", s)
		return ""
	}
}
