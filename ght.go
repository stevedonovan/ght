package main

import (
	"fmt"
	"github.com/olivere/elastic/uritemplates"
	"io"
	"io/ioutil"
	"mime"
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
	Vars     SMap       `json:"vars,omitempty"`
	Headers  [][]string `json:"headers,omitempty"`
	User     string     `json:"user,omitempty"`
	Password string     `json:"password,omitempty"`
}

func (rd *RequestData) merge(other *RequestData, partial bool) {
	rd.Base = other.Base
	rd.User = other.User
	rd.Password = other.Password
	rd.Headers = other.Headers
	rd.Payload = other.Payload
	rd.Vars = other.Vars
	rd.Method = other.Method
	rd.Url = other.Url
}

var fullTest = Strings{
	"test",
	"d:", "a=1", "b=hello", "c=1,ok", "d.x=42", "d.y.alpha=answer",
	"v:", "ex=1", "why=a+b", "new=something",
	"h:", "Content-Type=application/json",
	"/anything",
	"u:", "https://httpbin.org",
	//"s:", "t",
}

var runTest = Strings{
	"e.dump", "d:", "test.json",
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
	osArgs := os.Args[1:]
	args := fullTest
	if len(osArgs) > 0 {
		args = osArgs
	}
	if len(args) == 1 && filepath.Ext(args[0]) == ".ght" {
		contents, e := ioutil.ReadFile(args[0])
		checke(e)
		wspace := regexp.MustCompile(`\s+`)
		actualArgs := Strings{}
		for _, line := range strings.Split(string(contents), "\n") {
			if strings.HasPrefix(line, "#") {
				fmt.Println(line)
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
		fmt.Fprintln(os.Stderr, "status", resp.statusCode)
	}
	if resp.js != nil {
		hj, e := marshal(resp.js, true)
		checke(e)
		fmt.Println(hj)
	} else if len(resp.body) > 0 {
		fmt.Println(string(resp.body))
	}
}

type RunResponse struct {
	js            Map    // body as a Map, if possible
	body          []byte // body as text otherwise
	contentType   string
	contentLength int
	statusCode    int
}

func run(args []string) RunResponse {
	var flag string
	var pairs [][]string
	var saveName string
	var data RequestData
	var mimeType string
	var dataPairs [][]string
	ns, method, ok := splitDot2(args[0])
	if !ok {
		method = args[0]
		if !isStandardMethod(method) {
			quit("not a valid HTTP method " + data.Method)
		}
		data.Method, args = strings.ToUpper(method), args[1:]
	} else {
		prevData, partial := readConfig(ns, method)
		data.merge(prevData, partial)
		if partial {
			data.Method = strings.ToUpper(method)
		}
		args = args[1:]
	}
	fetch := data.Method != "TEST"
	// after the method, vars may start immediately
	skipV := len(args) > 0 && strings.Contains(args[0], "=") && !strings.HasPrefix(args[0], "http")
	for len(args) > 0 {
		if skipV {
			flag = "v:"
		} else {
			flag, args = args[0], args[1:]
		}
		skipV = false
		switch flag {
		case "d:", "data:":
			dataPairs, args = grabWhilePairs(args)
			if len(dataPairs) == 0 {
				dataPairs = [][]string{{"@", args[0]}}
				args = args[1:]
			}
		case "v:", "vars:":
			pairs, args = grabWhilePairs(args)
			if len(pairs) == 0 {
				var contents string
				contents, _ = loadFileIfPossible(args[0], false)
				args = args[1:]
				mappa := make(SMap)
				e := unmarshal(contents, &mappa)
				checke(e)
				data.Vars = mappa
			} else {
				// this _overrides_ inherited map entries
				if data.Vars == nil {
					data.Vars = SMap{}
				}
				newVars := pairsToMap(pairs)
				for k := range newVars {
					data.Vars[k] = newVars[k]
				}
			}
		case "h:", "head:":
			pairs, args = grabWhilePairs(args)
			data.Headers = pairs
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
		case "s:", "save:":
			saveName, args = args[0], args[1:]
		default:
			if flag == "" {
				break
			}
			fmt.Println("got",flag,args)
			data.Url = flag
		}
	}

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

	// we save at this point because expansion is now going further to take place
	if saveName != "" {
		fmt.Println("Data", data)
		writeConfig(saveName, ns, &data)
		return RunResponse{}
	}

	if len(dataPairs) == 1 && dataPairs[0][0] == "@" {
		file := dataPairs[0][1]
		if containsVarExpansions(file) {
			file = expandVariables(file, data.Vars)
		}
		data.Payload, mimeType = loadFileIfPossible(file, true)
	} else {
		m := parsePairs(dataPairs)
		if len(m) > 0 {
			payload, e := marshal(m, false)
			checke(e)
			mimeType = "application/json"
			if containsVarExpansions(payload) {
				payload = expandVariables(payload, data.Vars)
			}
			data.Payload = payload
		}
	}
	if !fetch && len(data.Payload) > 0 && mimeType == "application/json" {
		fmt.Print("Payload", data.Payload)
	}

	fullUrl := data.Url
	if data.Base != "" {
		fullUrl = data.Base + fullUrl
	}
	// is this a URI template?
	if strings.Contains(fullUrl, "{") {
		var e error
		fullUrl, e = uritemplates.Expand(fullUrl, data.Vars)
		checke(e)
	} else {
		q := parseQueryPairs(data.Vars)
		query := q.Encode()
		if query != "" {
			fullUrl += "?" + query
		}
	}
	if !fetch {
		fmt.Println("Url", fullUrl)
		return RunResponse{}
	}
	client := &http.Client{}
	//start := time.Now()
	var rdr io.Reader
	if data.Payload != "" {
		data.Headers = append(data.Headers, []string{"Content-Type", mimeType})
		rdr = strings.NewReader(data.Payload)
	}
	req, err := http.NewRequest(data.Method, fullUrl, rdr)
	checke(err)
	if data.Headers != nil {
		req.Header = parseHeaders(data.Headers)
	}
	if data.User != "" {
		req.SetBasicAuth(data.User, data.Password)
	}
	resp, err := client.Do(req)
	checke(err)

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	// a special case with HEAD method - display headers in JSON format
	var js Map
	contentType := resp.Header.Get("Content-Type")
	if data.Method == "HEAD" {
		js = headersToMap(resp.Header)
	} else {
		// strong opinion: should just be able to get files
		path := filepath.Base(fullUrl)
		ext := filepath.Ext(path)
		if data.Method == "GET" && ext != "" && !strings.ContainsAny(ext, "?=#") {
			err = ioutil.WriteFile(path, body, 0644)
			checke(err)
			body = nil
		} else if contentType == "application/json" {
			unmarshal(string(body), &js)
			body = nil
		}
	}
	contentLength, _ := strconv.Atoi(resp.Header.Get("Content-Length"))
	return RunResponse{
		js:            js,
		body:          body,
		contentType:   contentType,
		contentLength: contentLength,
		statusCode:    resp.StatusCode,
	}
}

// a LOT faster the second time 260ms vs 1560ms
//start = time.Now()
//client.Get(Url)
//fmt.Println("took", time.Since(start))

func loadFileIfPossible(maybeFile string, allTypes bool) (string, string) {
	ext := filepath.Ext(maybeFile)
	if ext != ".json" && !allTypes {
		// for now: could just read and see if we can marshal..
		quit("vars can only be set with JSON currently")
	}
	mtype := mime.TypeByExtension(ext)
	if mtype == "" {
		// don'mtype know if we can further make a guess about charset...
		mtype = "text/plain"
	}
	bb, e := ioutil.ReadFile(maybeFile)
	checke(e)
	return string(bb), mtype
}

func parseQueryPairs(m SMap) url.Values {
	q := make(url.Values)
	for key := range m {
		q.Add(key, m[key])
	}
	return q
}

func parseHeaders(pairs [][]string) http.Header {
	q := make(http.Header)
	for _, p := range pairs {
		q.Add(p[0], p[1])
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
