package main

import (
	"fmt"
	"github.com/shibukawa/configdir"
)

const configFile = "ght.json"

func configDir() *configdir.Config {
	configDirs := configdir.New("ght", "ght")
	return configDirs.QueryCacheFolder()
}

type RequestCache struct {
	Data RequestDataMap
}

type RequestDataMap = map[string]*RequestData

func New() RequestCache {
	data := make(RequestDataMap)
	return RequestCache{Data: data}
}

func readCache() (RequestCache, error) {
	cache := configDir()
	in := New()
	data, e := cache.ReadFile(configFile)
	if e != nil {
		return in, e
	}
	e = unmarshal(string(data), &in)
	if e != nil {
		return in, e
	}
	return in, nil
}

func readConfig(name, method string) (*RequestData, bool) {
	in, e := readCache()
	checke(e)
	key := name
	partial := isStandardMethod(method)
	if !partial {
		key = name + "." + method
	}
	res, ok := in.Data[key]
	if !ok {
		quit("no such saved config " + key)
	}
	//if !partial {
	//	base := in.Data[name]
	//	res.merge(base, true)
	//}
	return res, partial
}

func writeConfig(name, group string, data *RequestData) {
	fmt.Println("saving", name)
	// no point in persisting payload (could be rather large!)
	data.Payload = ""
	base, _, ok := splitDot2(name)
	cache := configDir()
	// config not existing yet is not an error, unless we're defining a new method
	out, e := readCache()
	if ok { // there is an existing namespace/group/whatever
		if base != group {
			quit(fmt.Sprintf("%s is not the same as %s", base, group))
		}
	}
	if e != nil {
		m := RequestDataMap{name: data}
		out = RequestCache{m}
	} else {
		out.Data[name] = data
	}
	res, e := marshal(out,false)
	checke(e)
	e = cache.WriteFile(configFile, []byte(res))
	checke(e)
}
