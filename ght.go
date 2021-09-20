package main

import (
	//"context"
	//"flag"
	"fmt"
	"github.com/stevedonovan/ght/term"
	"io"
	"io/ioutil"
	//"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Map = map[string]interface{}
type Array = []interface{}
type Strings = []string
type SMap = map[string]string

type RequestData struct {
	Method   string     `json:"method,omitempty"`
	Payload  string     `json:"payload,omitempty"`
	Base     string     `json:"base,omitempty"`
	Url      string     `json:"url,omitempty"`
	Vars     Map       `json:"vars,omitempty"`
	Query    Map  `json:"query,omitempty"`
	Headers  [][]string `json:"headers,omitempty"`
	User     string     `json:"user,omitempty"`
	Password string     `json:"password,omitempty"`
	OutputFormat OutputFormat `json:"output_format,omitempty"`
	Help string `json:"help,omitempty"`
	Args []string `json:"args,omitempty"`
	Pairs       Pairs `json:"pairs,omitempty"`

	// private
	mimeType    string
	getShortcut bool
	saveName    string
	namespace   string
	fromFile    string
	help bool
}

func (rd *RequestData) merge(other *RequestData, partial bool) {
	rd.Base = other.Base
	rd.User = other.User
	rd.Password = other.Password
	rd.Headers = other.Headers
	if rd.OutputFormat.empty() {
		rd.OutputFormat = other.OutputFormat
	}
	if rd.Help == "" {
		rd.Help = other.Help
	}
	if len(rd.Args) == 0 {
		rd.Args = other.Args
	}
	if len(rd.Pairs) == 0 {
		rd.Pairs = other.Pairs
	}
	if rd.Vars == nil {
		rd.Vars = make(Map)
	}
	if other.Vars != nil {
		for k, v := range other.Vars {
			if rd.Vars[k] == nil {
				rd.Vars[k] = v
			}
		}
	}
	if rd.Method == "" {
		rd.Method = other.Method
	}
	if rd.Url == "" {
		rd.Url = other.Url
	}
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
		runServer(args[1:])
		return
	}
	if len(args) == 1 && filepath.Ext(args[0]) == ".ght" {
		contents, e := ioutil.ReadFile(args[0])
		checke(e)
		wspace := regexp.MustCompile(`\s+`)
		actualArgs := Strings{}
		for _, line := range strings.Split(string(contents), "\n") {
			if strings.HasPrefix(line, "#") {
				if len(actualArgs) > 0 {
					runAndPrint(actualArgs)
					actualArgs = Strings{}
				}
			} else {
				actualArgs = append(actualArgs, wspace.Split(line, -1)...)
			}
		}
		if len(actualArgs) > 0 {
			runAndPrint(actualArgs)
		}
	} else {
		runAndPrint(args)
	}
}

func runAndPrint(args Strings) {
	resp := run(args)
	failed := !(resp.statusCode >= 200 && resp.statusCode < 300)
	if failed && resp.statusCode != 0 {
		term.BrightRed(os.Stderr, "status %d\n",resp.statusCode)
		os.Exit(1)
	}
	e := resp.outputFormat.process(resp.js,resp.body,resp.data)
	checke(e)
}

type RunResponse struct {
	js            interface{} // non-nil if we got JSON
	body          []byte // body as text otherwise
	contentType   string
	contentLength int
	statusCode    int
	outputFormat  OutputFormat
	data          Map
}

func run(args []string) RunResponse {
	var data RequestData

	fetch := data.parse(args)

	if !fetch {
		if len(data.Vars) > 0 {
			fmt.Println("Vars", data.Vars)
		}
		if len(data.Headers) > 0 {
			fmt.Println("Headers", data.Headers)
		}
		if data.User != "" {
			fmt.Println("user", data.User, "password", data.Password)
		}
	}
	if data.isFromFile() {
		return data.responseFromFile()
	}

	// we save at this point because further expansion is going to take place
	if data.saveName != "" {
		writeConfig(data.saveName, &data)
		return RunResponse{}
	}

	// so data may come in as a file, or by key-value Pairs
	// The filename or the values may contain variable expansions
	if len(data.Pairs) == 1 {
		key, value := data.Pairs.KeyValue(0)
		if key == "@" {
			file := value
			if containsVarExpansions(file) {
				file = expandVariables(file, data.Vars)
			}
			data.Payload, data.mimeType = loadFileIfPossible(file)
		} else
		if key == "" {
			data.Payload, data.mimeType = value, "text/plain"
		}
	}
	if data.Payload == "" {
		m := parsePairs(data.Pairs)
		if len(m) > 0 {
			payload, e := marshal(m, false)
			checke(e)
			data.mimeType = "application/json"
			if containsVarExpansions(payload) {
				payload = expandVariables(payload, data.Vars)
			}
			data.Payload = payload
		}
	}
	if !fetch && len(data.Payload) > 0 && data.mimeType == "application/json" {
		fmt.Print("Payload", data.Payload)
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
	if !fetch {
		fmt.Println("Url", fullUrl)
		return RunResponse{}
	}
	//~ var unixHttp = regexp.MustCompile(`https?://\[([^]]+)\](.+)`)
	//~ var transport *http.Transport = nil
	//~ if matches := unixHttp.FindStringSubmatch(fullUrl); matches != nil {
		//~ transport = &http.Transport{
			//~ DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				//~ return net.Dial("unix", matches[1])
			//~ },
		//~ }
		//~ fullUrl = "http://unix" + matches[2]
	//~ }
	client := &http.Client{
//		Transport: transport,
	}
	var rdr io.Reader
	if data.Payload != "" {
		data.Headers = append(data.Headers, []string{"Content-Type", data.mimeType})
		rdr = strings.NewReader(data.Payload)
	}

	req, err := http.NewRequest(data.Method, fullUrl, rdr)
	checke(err)
	if data.Headers != nil {
		req.Header = parseHeaders(data.Headers, data.Vars)
	}
	if data.User != "" {
		req.SetBasicAuth(data.User, data.Password)
	}
	if data.help {
		js := requestToJson(req)
		js["output_format"] = data.OutputFormat
		return RunResponse{
			js: js,
			contentType: "application/json",
		}
	}
	resp, err := client.Do(req)
	checke(err)
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	// a special case with HEAD method - display headers in JSON format
	var js interface{}
	contentType := resp.Header.Get("Content-Type")
	if data.Method == "HEAD" {
		js = headersToMap(resp.Header)
	} else {
		sbody := string(body)
		wasJson := strings.HasPrefix(contentType, "application/json")
		// strong opinion: should just be able to get files
		if data.getShortcut && resp.StatusCode == 200 {
			path := filepath.Base(fullUrl)
			path = strings.ReplaceAll(path,"?","@")
			path = strings.ReplaceAll(path,"&","@")
			err = ioutil.WriteFile(path, body, 0644)
			checke(err)
			body = nil
		} else if wasJson || (len(sbody) > 0 && sbody[0]=='{') {
			unmarshal(sbody, &js)
			if js != nil {
				if ! wasJson {
					// because it actually was!
					contentType = "application/json"
				}
				body = nil
			}
		}
	}
	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	return RunResponse{
		js:            js,
		body:          body,
		contentType:   contentType,
		contentLength: contentLength,
		statusCode:    resp.StatusCode,
		outputFormat:  data.OutputFormat,
		data:          data.Vars,
	}
}

func (data *RequestData) responseFromFile() RunResponse {
	var js interface{}
	var body string
	if data.fromFile == "application/json" {
		e := unmarshal(data.Payload, &js)
		checke(e)
	} else {
		body = data.Payload
	}
	return RunResponse{
		js:           js,
		body:         []byte(body),
		contentType:  data.fromFile,
		outputFormat: data.OutputFormat,
	}
}

func (data *RequestData) parse(args []string) bool {
	var flag string
	var pairs [][]string
	var wasLast bool
	verb := args[0]
	ns, method, ok := splitDot2(verb)
	data.namespace = ns
	if args[0] == "last" {
		wasLast = true
		data.Method = "GET"
		args = append([]string{"file:", "LAST"}, args[1:]...)
	} else
	if isUrl(args[0]) {
		data.getShortcut = true
		data.Method = "GET"
		data.Url, args = args[0], args[1:]
	} else
	if !ok {
		method = args[0]
		pdata, ok := readConfigSingle(method)
		if !ok {
			if !isStandardMethod(method) {
				quit("not a valid HTTP method " + data.Method)
			}
			data.Method = strings.ToUpper(method)
		} else {
			*data = *pdata
		}
		args = args[1:]
	} else {
		pdata, partial := readConfig(ns, method)
		*data = *pdata
		if partial {
			data.Method = strings.ToUpper(method)
		}
		args = args[1:]
	}
	if len(args) > 0 && args[0] == "help:" {
		args = []string{"file:", cacheFile()}
		data.OutputFormat = OutputFormat{
			JPointer: cachePattern(verb),
			OType:    "json",
		}
	}
	fetch := data.Method != "TEST"
	data.fromFile = "NADA"
	if len(data.Args) > 0 {
		i := 0
		if data.Vars == nil {
			data.Vars = Map{}
		}
		for len(args) > 0 {
			flag = args[0]
			if isVar(flag) || strings.HasSuffix(flag,":") {
				break
			}
			if i >= len(data.Args) {
				quit(fmt.Sprintf("needed %d args, got %d\n",len(data.Args),i))
			}
			data.Vars[data.Args[i]] = valueToInterface(flag)
			args = args[1:]
			i++
		}
	}
	// after the method, vars may start immediately
	skipV := len(args) > 0 && isVar(args[0]) && !isUrl(args[0])
	for len(args) > 0 {
		if skipV {
			flag = "v:"
		} else {
			flag, args = args[0], args[1:]
		}
		skipV = false
		switch flag {
		case "d:", "data:":
			data.Pairs, args = grabWhilePairs(args)
			if len(data.Pairs) == 0 {
				data.Pairs, args = NewFilePair(args[0]), args[1:]
			}
		case "v:", "vars:":
			pairs, args = grabWhilePairs(args)
			if len(pairs) == 0 {
				contents, mtype := loadFileIfPossible(args[0])
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
			data.Query = make(Map,len(pmap))
			for k,v := range pmap {
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
		case "file:":
			var fname string
			fname, args = args[0], args[1:]
			data.Payload, data.fromFile = loadFileIfPossible(fname)
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
		case "help:":
			if len(args) > 0 {
				args = args[1:]
			}
			data.help = true
		case "s:", "save:", "update:":
			if flag == "update:" {
				data.saveName = verb
			} else {
				data.saveName, args = args[0], args[1:]
			}
			if len(args) == 0 {
				continue
			}
			parts := strings.Split(args[0], "=")
			if len(parts) > 1 {
				switch parts[0] {
				case "m","help":
					data.Help = parts[1]
				case "args":
					data.Args = strings.Split(parts[1],",")
				default:
					quit("only m,help or args allowed here")
				}
				args = args[1:]
			}
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

func (rd *RequestData) isFromFile() bool {
	return rd.fromFile != "NADA"
}

func isVar(s string) bool {
	return strings.Contains(s, "=")
}

func isUrl(s string) bool {
	prefix := strings.HasPrefix
	return prefix(s, "http:") || prefix(s, "https:") || prefix(s, "/")
}

func parseQueryPairs(m Map) url.Values {
	q := make(url.Values)
	for key := range m {
		q.Add(key, fmt.Sprintf("%v",m[key]))
	}
	return q
}

func parseHeaders(pairs [][]string, vars Map) http.Header {
	q := make(http.Header)
	for _, p := range pairs {
		val := expandVariables(p[1],vars)
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
