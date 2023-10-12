package main

import (
	//"context"
	//"flag"
	"fmt"
	"github.com/google/shlex"
	"github.com/stevedonovan/ght/term"
	"io"
	"net/http/httputil"
	"strconv"

	//"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Map = map[string]interface{}
type Array = []interface{}
type Strings = []string
type SMap = map[string]string

type RequestData struct {
	Method       string       `json:"method,omitempty"`
	Payload      string       `json:"payload,omitempty"`
	Base         string       `json:"base,omitempty"`
	Url          string       `json:"url,omitempty"`
	Vars         Map          `json:"vars,omitempty"`
	QueryPairs   Pairs        `json:"query_pairs,omitempty"`
	Headers      [][]string   `json:"headers,omitempty"`
	User         string       `json:"user,omitempty"`
	Password     string       `json:"password,omitempty"`
	OutputFormat OutputFormat `json:"output_format,omitempty"`
	Help         bool         `json:"help,omitempty"`
	Params       [][]string   `json:"args,omitempty"`
	Pairs        Pairs        `json:"pairs,omitempty"`
	Req          bool         `json:"dump,omitempty"`
	Thru         bool         `json:"thru"`
	Last         bool         `json:"last"`
	Resp         bool         `json:"req"`
	Flags        SMap         `json:"flags"`

	// private
	mimeType    string
	getShortcut bool
}

var standardMethods = map[string]bool{
	"GET": true, "POST": true, "HEAD": true, "PUT": true, "PATCH": true, "DELETE": true,
	"TEST": true,
}

func isStandardMethod(m string) bool {
	m = strings.ToUpper(m)
	_, ok := standardMethods[m]
	return ok
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		quit("ght <method> <values> <fragment>")
	}
	if args[0] == "server" {
		if len(args) != 2 {
			quit("ght server <bind-address>")
		}
		runServer(args[1:])
	} else {
		_ = readEnvFile(".env")
		runAndPrint(args)
	}
}

func runAndPrint(args Strings) {
	resp := run(args)
	failed := !(resp.statusCode >= 200 && resp.statusCode < 300)
	if failed && resp.statusCode != 0 {
		term.BrightRed(os.Stderr, "status %s\n", resp.status)
		os.Exit(1)
	}
	e := resp.outputFormat.process(resp.js, resp.body, resp.data)
	checke(e)
}

type RunResponse struct {
	js            interface{} // non-nil if we got JSON
	body          []byte      // body as text otherwise
	contentType   string
	contentLength int
	statusCode    int
	status        string
	outputFormat  OutputFormat
	data          Map
}

func grabEnvVarPairs(envVar string) Pairs {
	if ps := os.Getenv(envVar); ps != "" {
		parts, e := shlex.Split(ps)
		checke(e)
		pairs, rest := grabWhilePairs(parts)
		if len(rest) > 0 {
			quit(envVar + " must only consist of var=value pairs")
		}
		return pairs
	}
	return Pairs{}
}

func run(args []string) RunResponse {
	var data RequestData

	if !data.parse(args) {
		quit("cannot parse")
	}
	if data.Help {
		mainHelp()
	}

	envVars := os.Environ()

	if os.Getenv("GHT_DUMP_DATA") != "" {
		out, _ := marshal(data, false)
		fmt.Printf("data: %s\n", out)
		os.Exit(0)
	}

	// output format from the environment
	if of := os.Getenv("GHT_OUT"); of != "" && !data.OutputFormat.Init {
		parts, e := shlex.Split(of)
		checke(e)
		_, e = data.OutputFormat.parse(parts, data.Vars)
		checke(e)
	}
	// request data may come from the environment
	if data.Payload == "" {
		if len(data.Pairs) == 0 {
			denv := os.Getenv("GHT_DATA")
			if denv != "" {
				// same key-value format OR @file as in data:
				parts, e := shlex.Split(denv)
				checke(e)
				data.Pairs, parts = grabWhilePairs(parts)
				if len(parts) > 0 {
					data.processDataFileOrText(parts[0], true)
				}
			}
		}
		m := parsePairs(data.Pairs, data.Vars)
		if len(m) > 0 {
			payload, e := marshal(m, false)
			checke(e)
			data.mimeType = "application/json"
			data.Payload = payload
		}
	}
	if containsVarExpansions(data.OutputFormat.File) {
		data.OutputFormat.File = expandVariables(data.OutputFormat.File, data.Vars)
	}
	fullUrl := data.Url
	base := data.Base
	logf("url %q base %q", data.Url, data.Base)
	if base == "" && strings.HasPrefix(fullUrl, "/") {
		base = os.Getenv("GHT_HOST")
	}
	if port := os.Getenv("GHT_PORT"); port != "" {
		base += ":" + port
	}
	if base != "" {
		fullUrl = base + fullUrl
	}

	// path parameters, if defined...note that var expansion can take place as well
	if len(data.Params) == 0 {
		data.Params = grabEnvVarPairs("GHT_PARAMS")
	}
	if len(data.Params) != 0 {
		for _, a := range data.Params {
			fullUrl += fmt.Sprintf("/%v", valueToInterface(a[1], data.Vars))
		}
	}
	// query parameters
	// question here is: can there be partial overloads from environment?
	if len(data.QueryPairs) == 0 {
		data.QueryPairs = grabEnvVarPairs("GHT_QUERY")
	}
	if len(data.QueryPairs) > 0 {
		qmap := pairsToMapInterface(data.QueryPairs, data.Vars)
		q := parseQueryPairs(qmap)
		query := q.Encode()
		if query != "" {
			fullUrl += "?" + query
		}
	}
	client := &http.Client{
		//		Transport: transport,
	}

	var rdr io.Reader
	if data.Payload != "" {
		// specify the content type, if not already set
		doAppend := false
		for _, pair := range data.Headers {
			if strings.EqualFold("Content-Type", pair[0]) {
				doAppend = false
				break
			}
		}
		if doAppend {
			data.Headers = append(data.Headers, []string{"Content-Type", data.mimeType})
		}
		rdr = strings.NewReader(data.Payload)
	}

	req, err := http.NewRequest(data.Method, fullUrl, rdr)
	checke(err)

	// headers
	for _, v := range envVars {
		pair, ok := strings.CutPrefix(v, "GHT_HDR_")
		if ok {
			data.Headers = append(data.Headers, strings.Split(pair, "="))
		}
	}
	if data.Headers != nil {
		req.Header = parseHeaders(data.Headers, data.Vars)
	}

	// basic auth
	if data.User == "" {
		if pair := os.Getenv("GHT_USER"); pair != "" {
			var ok bool
			data.User, data.Password, ok = split2(pair, "=")
			if !ok {
				quit("GHT_USER value must be of form user=password")
			}
		}
	}
	if data.User != "" {
		req.SetBasicAuth(data.User, data.Password)
	}

	if data.Req {
		bb, err2 := httputil.DumpRequest(req, true)
		if err2 != nil {
			return RunResponse{}
		}
		fmt.Println(string(bb))
		os.Exit(0)
	}

	var body []byte
	var resp *http.Response
	var status string
	contentType := data.mimeType
	contentLength := 0
	statusCode := 0
	if !data.Thru {
		resp, err = client.Do(req)
		checke(err)
		if data.Resp {
			bb, err := httputil.DumpResponse(resp, true)
			if err != nil {
				quit("cannot dump response " + err.Error())
			}
			fmt.Println(string(bb))
			os.Exit(0)
		} else {
			body, err = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		contentType = resp.Header.Get("Content-Type")
		contentLength, _ = strconv.Atoi(resp.Header.Get("Content-Length"))
		statusCode = resp.StatusCode
		status = resp.Status
	} else {
		body = []byte(data.Payload)
	}

	// a special case with HEAD method - display headers in JSON format
	var js interface{}
	if data.Method == "HEAD" {
		js = headersToMap(resp.Header)
	} else {
		sbody := string(body)
		wasJson := strings.HasPrefix(contentType, "application/json")
		// strong opinion: should just be able to get files
		if data.getShortcut && resp.StatusCode == 200 {
			path := filepath.Base(fullUrl)
			path = strings.ReplaceAll(path, "?", "@")
			path = strings.ReplaceAll(path, "&", "@")
			err = os.WriteFile(path, body, 0644)
			checke(err)
			body = nil
		} else if wasJson || (len(sbody) > 0 && sbody[0] == '{') {
			unmarshal(sbody, &js)
			if js != nil {
				if !wasJson {
					// because it actually was!
					contentType = "application/json"
				}
				body = nil
			}
		}
	}
	return RunResponse{
		js:            js,
		body:          body,
		contentType:   contentType,
		contentLength: contentLength,
		statusCode:    statusCode,
		outputFormat:  data.OutputFormat,
		data:          data.Vars,
		status:        status,
	}
}

func (data *RequestData) parse(args []string) bool {
	var flag string
	var pairs [][]string
	var wasLast bool
	verb := args[0]
	if isUrl(args[0]) {
		data.getShortcut = true
		data.Method = "GET"
		data.Url, args = args[0], args[1:]
	} else {
		data.Method = verb
		if !isStandardMethod(data.Method) {
			if data.Method == "print" {
				data.Thru = true
			} else if data.Method == "last" {
				data.Thru = true
				data.Last = true
				wasLast = true
				contents, _ := os.ReadFile(lastFile())
				data.Payload = string(contents)
			} else if data.Method == "help" {
				data.Help = true
			} else {
				e := readEnvFile(data.Method + ".env")
				if e != nil {
					quit("not a valid HTTP method or a local .env file " + data.Method)
				}
				data.Method = os.Getenv("GHT_METHOD")
				if !isStandardMethod(data.Method) {
					quit("GHT_METHOD not defined " + data.Method)
				}
			}
		}
		data.Method = strings.ToUpper(data.Method)
		args = args[1:]
	}
	if len(args) == 0 {
		quit("provide arguments")
	}
	if !data.getShortcut {
		var nargs []string
		Append := func(key string) {
			nargs = append(nargs, key+"="+args[0])
			args = args[1:]
		}
		if arg1 := os.Getenv("GHT_ARG_1"); arg1 != "" {
			Append(arg1)
			if arg2 := os.Getenv("GHT_ARG_2"); arg2 != "" {
				Append(arg2)
				if arg3 := os.Getenv("GHT_ARG_3"); arg3 != "" {
					Append(arg3)
				}
			}
		}
		if len(nargs) > 0 {
			args = append(nargs, args...)
			fmt.Println("new args", args)
		}
	}
	var implicitVars bool
	if len(args) > 0 && strings.Contains(args[0], "=") {
		implicitVars = true
	}
	for len(args) > 0 {
		if !implicitVars {
			flag, args = args[0], args[1:]
		} else {
			implicitVars = false
			flag = "vars:"
		}
		switch flag {
		case "v:", "vars:":
			pairs, args = grabWhilePairs(args)
			// var data can either be a file convertible to JSON (like YAML or JSON5) or key-value pairs
			checkHelp(len(pairs) == 0, args, "vars")
			if len(pairs) == 0 {
				contents, mtype := loadFileIfPossible(args[0], true)
				if mtype != "application/json" {
					quit("vars can only be set with JSON or YAML currently")
				}
				args = args[1:]
				mappa := make(Map)
				checke(unmarshal(contents, &mappa))
				data.Vars = mappa
			} else {
				data.Vars = parsePairs(pairs, nil)
			}
		case "d:", "data:":
			data.Pairs, args = grabWhilePairs(args)
			checkHelp(len(pairs) == 0, args, "data")
			if len(data.Pairs) == 0 {
				first := args[0]
				args = args[1:]
				data.processDataFileOrText(first, true)
			}
		case "b:", "body:":
			var fname string
			checkHelp(len(args) == 0, args, "body")
			fname, args = args[0], args[1:]
			data.processDataFileOrText(fname, false)
		case "req:", "request:":
			if len(args) > 1 {
				args = args[1:]
			}
			data.Req = true
		case "resp:", "response:":
			if len(args) > 1 {
				args = args[1:]
			}
			data.Resp = true
		case "f:", "flags:":
			pairs, args = grabWhilePairs(args)
			checkHelp(len(pairs) == 0, args, "flags")
			data.Flags = pairsToMap(pairs)
		case "q:", "query:":
			pairs, args = grabWhilePairs(args)
			checkHelp(len(pairs) == 0, args, "query")
			data.QueryPairs = pairs
		case "h:", "head:":
			pairs, args = grabWhilePairs(args)
			checkHelp(len(pairs) == 0, args, "head")
			data.Headers = pairs
		case "p:", "params:":
			pairs, args = grabWhilePairs(args)
			checkHelp(len(pairs) == 0, args, "params")
			data.Params = pairs
		case "o:", "out:":
			var e error
			checkHelp(len(args) == 0, args, "out")
			args, e = data.OutputFormat.parse(args, data.Vars)
			data.OutputFormat.Last = wasLast
			checke(e)
		case "user:":
			checkHelp(len(args) == 0, args, "user")
			maybePair := args[0]
			args = args[1:]
			parts := strings.Split(maybePair, "=")
			data.User = parts[0]
			if len(parts) > 1 {
				data.Password = parts[1]
			}
		case "host:":
			checkHelp(len(args) == 0, args, "host")
			data.Base, args = args[0], args[1:]
		default:
			if flag == "" {
				break
			}
			if data.Url == "" {
				data.Url = flag
			}
		}
	}
	if data.Url == "" {
		data.Url = os.Getenv("GHT_PATH")
	}
	return true
}

func (data *RequestData) processDataFileOrText(first string, forceJson bool) {
	if file, ok := strings.CutPrefix(first, "@"); ok || first == "-" {
		if first == "-" {
			file = "IN"
		}
		// as a file, always interpreted as JSON data (use body: otherwise)
		data.Payload, data.mimeType = loadFileIfPossible(file, forceJson)
	} else {
		data.Payload, data.mimeType = file, "text/plain"
	}
}

func isUrl(s string) bool {
	prefix := strings.HasPrefix
	return prefix(s, "http:") || prefix(s, "https:") || prefix(s, "/")
}

func parseQueryPairs(m Map) url.Values {
	q := make(url.Values)
	for key := range m {
		q.Add(key, fmt.Sprintf("%v", m[key]))
	}
	return q
}

func parseHeaders(pairs [][]string, vars Map) http.Header {
	q := make(http.Header)
	for _, p := range pairs {
		val := expandVariables(p[1], vars)
		q.Add(p[0], val)
	}
	return q
}

func headersToMap(h http.Header) Map {
	m := make(Map)
	for key := range h {
		m[key] = h.Get(key)
	}
	return m
}
