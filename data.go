package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/flynn/json5"
	"github.com/stevedonovan/ght/pointer"
	"github.com/stevedonovan/ght/term"
	"gopkg.in/yaml.v2"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

func marshal(data interface{}, indent bool) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if indent {
		enc.SetIndent("", "  ")
	}
	if e := enc.Encode(data); e != nil {
		return "", e
	}
	return b.String(), nil
}

func unmarshal(text string, obj interface{}) error {
	dec := json.NewDecoder(strings.NewReader(text))
	return dec.Decode(obj)
}

func unmarshal5(text string, obj interface{}) error {
	dec := json5.NewDecoder(strings.NewReader(text))
	return dec.Decode(obj)
}

func marshalNdJson(data interface{}) (string, error) {
	arr, err := dataAsArray(data, nil)
	if err != nil {
		return "", err
	}
	outs := bytes.NewBufferString("")
	for _, rec := range arr {
		obj, ok := rec.(pointer.Object)
		if !ok {
			return "", fmt.Errorf("NdJson rows must be objects")
		}
		delete(obj, "_keys_")
		line, e := marshal(obj, false)
		if e != nil {
			return "", e
		}
		outs.Write([]byte(line))
	}
	return outs.String(), nil
}

func marshalYAML(data interface{}) (string, error) {
	out, e := yaml.Marshal(data)
	return string(out), e
}

func unmarshallYAML(text string, obj interface{}) error {
	return yaml.Unmarshal([]byte(text), obj)
}

var defaultKey = "key"
var defaultValue = "value"

func dataAsArray(data interface{}, keys *[]string) ([]interface{}, error) {
	arr, ok := data.(Array)
	if !ok {
		obj, ok := data.(Map)
		if ok {
			if keys != nil {
				*keys = append([]string{defaultKey}, *keys...)
			}
			for k, v := range obj {
				mem, ok := v.(Map)
				if !ok {
					mem = Map{defaultValue: v}
				}
				mem[defaultKey] = k
				arr = append(arr, mem)
			}
		} else {
			return arr, fmt.Errorf("CSV output requires array or object")
		}
	}
	return arr, nil
}

func marshalCSV(data interface{}, delim rune, keys []string) (string, error) {
	arr, err := dataAsArray(data, &keys)
	if err != nil {
		return "", err
	}
	if len(arr) == 0 {
		return "", nil
	}
	firstElem := arr[0]
	obj, ok := firstElem.(Map)
	if !ok {
		return "", fmt.Errorf("CSV output requires array elements to be objects, not %T", firstElem)
	}
	var cols []string
	if keys != nil {
		cols = keys
	} else {
		for k := range obj {
			cols = append(cols, k)
		}
	}
	outs := bytes.NewBufferString("")
	wtr := csv.NewWriter(outs)
	wtr.Comma = delim
	wtr.Write(cols)
	wtr.Flush()

	for _, row := range arr {
		obj, ok := row.(Map)
		if !ok {
			return "", fmt.Errorf("CSV rows must be objects")
		}
		rec := []string{}
		for _, k := range cols {
			v := obj[k]
			rec = append(rec, fmt.Sprintf("%v", v))
		}
		wtr.Write(rec)
		wtr.Flush()
	}
	return outs.String(), nil
}

type OutputFormat struct {
	JPointer string `json:"j_pointer,omitempty"`
	OType    string `json:"o_type,omitempty"`
	File     string `json:"file,omitempty"`
	Last     bool   `json:"last,omitempty"`
	Merge    string `json:"merge,omitempty"`
}

func (of OutputFormat) process(js interface{}, body []byte, data Map) error {
	var e error
	if js == nil {
		if len(body) > 0 {
			if of.File != "" {
				e := os.WriteFile(of.File, body, 0666)
				checke(e)
			} else {
				of.pageOut(string(body), false)
			}
		}
		return nil
	}
	var res = js
	ot := "json"
	if of.OType != "" {
		ot = of.OType
	} else if of.File != "" {
		// ah but what about .jpg etc? They would not look like json...
		ext := filepath.Ext(of.File)
		if ext != "" {
			ot = ext[1:]
		}
		if nameOf(of.File) == "OUT" {
			of.File = ""
		}
	}
	var keys []string
	if of.JPointer != "" {
		res, e = pointer.Filter(js, of.JPointer, &keys)
		if e != nil {
			return e
		}
	}
	if of.Merge != "" {
		if m, ok := res.(Map); ok {
			for k, v := range data {
				if _, ok := m[k]; !ok {
					m[k] = v
				}
			}
		} else {
			name := of.Merge
			if name == "*" {
				name = "FIELD"
			}
			data[name] = res
			res = data
		}
	}
	var out string
	var ok bool
	switch ot {
	case "tsv":
		out, e = marshalCSV(res, '\t', keys)
	case "csv":
		out, e = marshalCSV(res, ',', keys)
	case "yaml", "yml":
		out, e = marshalYAML(res)
	case "json", "js":
		out, e = marshal(res, true)
	case "ndjson":
		out, e = marshalNdJson(res)
	case "txt", "text":
		out, ok = res.(string)
		if !ok {
			return fmt.Errorf("value was %T, not string", res)
		}
		out += "\n"
	default:
		out = expandVariables(ot, js.(Map)) + "\n"
		//return fmt.Errorf("unknown output format %q", ot)
	}
	if e != nil {
		return e
	}
	f := os.Stdout
	if of.File != "" {
		f, e = os.Create(of.File)
		defer f.Close()
		if e != nil {
			return e
		}
		_, e = f.Write([]byte(out))
	} else {
		of.pageOut(out, ot == "json" || ot == "ndjson")
	}
	return e

}

func lastFile() string {
	return filepath.Join(os.TempDir(), "ght_last")
}

func (of *OutputFormat) pageOut(txt string, pretty bool) {
	term.Page(txt, pretty)
	if !of.Last {
		os.WriteFile(lastFile(), []byte(txt), 0644)
	}
}

func (of *OutputFormat) empty() bool {
	return of.JPointer == "" && of.OType == "" && of.File == ""
}

func lookupAlt(key1, key2 string, m map[string]string) string {
	val, ok := m[key1]
	if ok {
		delete(m, key1)
		return val
	}
	val, ok = m[key2]
	if ok {
		delete(m, key2)
		return val
	}
	return ""
}

func (ofp *OutputFormat) parse(args []string) ([]string, error) {
	var pairs [][]string
	pairs, args = grabWhilePairs(args)
	if len(pairs) > 0 {
		m := pairsToMap(pairs)
		ofp.JPointer = lookupAlt("F", "field", m)
		ofp.OType = lookupAlt("t", "type", m)
		ofp.File = lookupAlt("f", "file", m)
		ofp.Merge = lookupAlt("m", "merge", m)
		if len(m) > 0 {
			quit("unrecognized output field " + strings.Join(keys(m), ","))
		}
	} else {
		ofp.File, args = args[0], args[1:]
	}
	return args, nil
}

func nameOf(f string) string {
	idx := strings.LastIndex(f, ".")
	if idx == -1 {
		return f
	} else {
		return f[:idx]
	}
}

func loadFileIfPossible(maybeFile string, forceJson bool) (string, string) {
	var bb []byte
	var e error
	ext := filepath.Ext(maybeFile)
	name := nameOf(maybeFile)
	if name == "IN" {
		var buf bytes.Buffer
		_, e = io.Copy(&buf, os.Stdin)
		bb = buf.Bytes()
	} else {
		bb, e = os.ReadFile(maybeFile)
	}
	checke(e)
	mtype := mime.TypeByExtension(ext)
	if mtype == "" { // this is quite the assumption...
		mtype = "text/plain"
	}
	contents := string(bb)
	if forceJson {
		if ext == ".yml" || ext == ".yaml" {
			var data interface{}
			e = unmarshallYAML(contents, &data)
			checke(e)
			data = pointer.MassageYaml(data)
			contents, e = marshal(data, false)
			checke(e)
		} else if ext == ".kvj" {
			data, e := readKeyValueFile(contents)
			checke(e)
			contents, e = marshal(data, false)
			checke(e)
		} else if ext == ".json" {
			var v any
			err := unmarshal5(contents, &v)
			if err != nil {
				quit("bad json5 file: " + err.Error())
			}
			contents, err = marshal(v, false)
			checke(e)
		} else {
			return strings.TrimSpace(contents), "text/plain"
		}
		mtype = "application/json"
	}
	return contents, mtype
}
