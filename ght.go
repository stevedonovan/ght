package main

import (
	//"context"
	//"flag"
	"fmt"
	"github.com/stevedonovan/ght/term"
	"io"
	"io/ioutil"
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
	Query        Map          `json:"query,omitempty"`
	Headers      [][]string   `json:"headers,omitempty"`
	User         string       `json:"user,omitempty"`
	Password     string       `json:"password,omitempty"`
	OutputFormat OutputFormat `json:"output_format,omitempty"`
	Help         string       `json:"help,omitempty"`
	Args         []string     `json:"args,omitempty"`
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
		// any .env file?
		bb, err := os.ReadFile(".env")
		if err == nil {
			pairs, err := readKeyValuePairs(string(bb))
			if err != nil {
				quit(err.Error())
			}
			for _, pair := range pairs {
				os.Setenv(pair[0], pair[1])
			}
		}
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

func run(args []string) RunResponse {
	var data RequestData

	if !data.parse(args) {
		quit("cannot parse")
	}
	if os.Getenv("DUMP_GHT_DATA") != "" {
		fmt.Printf("we got %#v\n", data)
	}
	// so data may come in as key-value Pairs
	if data.Payload == "" {
		m := parsePairs(data.Pairs)
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
	if base == "" && strings.HasPrefix(fullUrl, "/") {
		base = os.Getenv("GHT_URL")
	}
	if base != "" {
		fullUrl = base + fullUrl
	}
	// is this a URI template?
	if containsVarExpansions(fullUrl) {
		var e error
		fullUrl = expandVariables(fullUrl, data.Vars)
		checke(e)
	}
	if len(data.Query) > 0 {
		q := parseQueryPairs(data.Query)
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
		// the mineType has been already deduced, but let it be overridable
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

	envVars := os.Environ()
	for _, v := range envVars {
		pair, ok := strings.CutPrefix(v, "GHT_HDR_")
		if ok {
			idx := strings.Index(pair, "=")
			key, value := pair[:idx], pair[idx+1:]
			data.Headers = append(data.Headers, []string{key, value})
		}
	}
	if data.Headers != nil {
		req.Header = parseHeaders(data.Headers, data.Vars)
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
			err = ioutil.WriteFile(path, body, 0644)
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
			} else {
				quit("not a valid HTTP method " + data.Method)
			}
		}
		data.Method = strings.ToUpper(data.Method)
		args = args[1:]
	}
	fetch := data.Method != "TEST"
	for len(args) > 0 {
		flag, args = args[0], args[1:]
		switch flag {
		case "d:", "data:":
			data.Pairs, args = grabWhilePairs(args)
			if len(data.Pairs) == 0 {
				first := args[0]
				args = args[1:]
				if file, ok := strings.CutPrefix(first, "@"); ok || first == "-" {
					if first == "-" {
						file = "IN"
					}
					// as a file, always interpreted as JSON data (use body: otherwise)
					data.Payload, data.mimeType = loadFileIfPossible(file, true)
				} else {
					data.Payload, data.mimeType = file, "text/plain"
				}
			}
		case "b:", "body:":
			var fname string
			fname, args = args[0], args[1:]
			if fname == "-" {
				fname = "IN"
			}
			data.Payload, data.mimeType = loadFileIfPossible(fname, false)
		case "req:":
			if len(args) > 1 {
				args = args[1:]
			}
			data.Req = true
		case "resp:":
			if len(args) > 1 {
				args = args[1:]
			}
			data.Resp = true
		case "f:", "flags:":
			pairs, args = grabWhilePairs(args)
			data.Flags = pairsToMap(pairs)
		case "v:", "vars:":
			pairs, args = grabWhilePairs(args)
			if len(pairs) == 0 {
				contents, mtype := loadFileIfPossible(args[0], false)
				if mtype != "application/json" {
					quit("vars can only be set with JSON currently")
				}
				args = args[1:]
				mappa := make(Map)
				e := unmarshal(contents, &mappa)
				checke(e)
				data.Vars = mappa
			} else {
				// this _overrides_ inherited map entries
				if data.Vars == nil {
					data.Vars = Map{}
				}
				newVars := parsePairs(pairs)
				for k := range newVars {
					data.Vars[k] = newVars[k]
				}
			}
		case "q:", "query:":
			pairs, args = grabWhilePairs(args)
			pmap := pairsToMap(pairs)
			data.Query = make(Map, len(pmap))
			for k, v := range pmap {
				data.Query[k] = v
			}
		case "h:", "head:":
			pairs, args = grabWhilePairs(args)
			data.Headers = pairs
		case "o:", "out:":
			var e error
			args, e = data.OutputFormat.parse(args)
			data.OutputFormat.Last = wasLast
			checke(e)
		case "user:":
			maybePair := args[0]
			args = args[1:]
			parts := strings.Split(maybePair, "=")
			data.User = parts[0]
			if len(parts) > 1 {
				data.Password = parts[1]
			}
		case "u:", "url:":
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
	return fetch
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
