package main

import (
	"github.com/shibukawa/configdir"
	"path/filepath"
	"strings"
)

const configFile = "ght.json"

func configDir() *configdir.Config {
	configDirs := configdir.New("ght", "ght")
	return configDirs.QueryCacheFolder()
}

func writeCacheFile(name,txt string) error {
	cache := configDir()
	return cache.WriteFile(name,[]byte(txt))
}

func readCacheFile(name string) (string,error) {
	cache := configDir()
	b,e := cache.ReadFile(name)
	return string(b),e
}

type RequestDataExtra struct {
	RequestData
	Children map[string]*RequestData
}

type RequestCache struct {
	Data RequestDataMap
}

type RequestDataMap = map[string]*RequestDataExtra

func New() RequestCache {
	data := make(RequestDataMap)
	return RequestCache{Data: data}
}

func cacheFile() string {
	cache := configDir()
	return filepath.Join(cache.Path,configFile)
}

func cachePattern(name string) string {
	base,method,ok := splitDot2(name)
	if ok {
		return "Data." + base + ".Children." + method
	} else {
		return "Data." + name
	}
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
	data, ok := in.Data[key]
	if !ok {
		quit("no such saved config " + key)
	}
	res := &data.RequestData
	partial := isStandardMethod(method)
	if ! partial {
		new := data.Children[method]
		new.merge(res,false)
		res = new
	}
	return res, partial
}

func readConfigSingle(name string) (*RequestData, bool) {
	in, e := readCache()
	if e != nil {
		return nil, false
	}
	data, ok := in.Data[name]
	if data == nil {
		return nil, false
	}
	return &data.RequestData, ok
}

func readConfigHelp(name string) *RequestData {
	idx := strings.Index(name,".")
	if idx == -1 {
		res, _ := readConfigSingle(name)
		return res
	} else {
		res, _ := readConfig(name[:idx], name[idx+1:])
		return res
	}
}

func writeConfig(name string, data *RequestData) {
	// no point in persisting payload (could be rather large!)
	data.Payload = ""
	base, group, hasMethod := splitDot2(name)
	cache := configDir()
	// config not existing yet is not an error, unless we're defining a new method
	xdata := RequestDataExtra{
		RequestData: *data,
		Children:    nil,
	}
	out, e := readCache()
	if e != nil {
		m := RequestDataMap{name: &xdata}
		out = RequestCache{m}
	}
	if hasMethod {
		if out.Data[base].Children == nil {
			out.Data[base].Children = map[string]*RequestData{}
		}
		out.Data[base].Children[group] = data
	} else {
		xdata.Children = out.Data[name].Children
		out.Data[name] = &xdata
	}
	res, e := marshal(out,false)
	checke(e)
	e = cache.WriteFile(configFile, []byte(res))
	checke(e)
}
