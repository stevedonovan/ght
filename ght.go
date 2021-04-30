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
	"strings"
)

type Map = map[string]interface{}
type Array = []interface{}
type Strings = []string
type SMap = map[string]string

type RequestData struct {
	Method  string
	Payload string
	Base    string
	Url     string
	Vars    SMap
	Headers [][]string
	User string
	Password string
}

func (rd *RequestData) merge(other *RequestData, partial bool) {
	rd.Base = other.Base
	rd.User = other.User
	rd.Password = other.Password
	//if rd.Headers != nil {
	//	for _,p := range other.Headers {
	//
	//	}
	//} else {
	//	rd.Headers = other.Headers
	//}
	rd.Headers = other.Headers
	if partial {
		return
	}
	rd.Payload = other.Payload
	rd.Vars = other.Vars
	rd.Method = other.Method
	rd.Url = other.Url
}

var fullTest = Strings{
	"dump",
	"d:", "a=1", "b=hello", "c=1,ok", "d.x=42", "d.y.alpha=answer",
	"v:", "ex=1", "why=a+b", "new=something",
	"h:", "Content-Type=application/json",
	"/anything",
	"u:", "https://httpbin.org",
	"s:", "t",
}

var runTest = Strings{
	"e.dump","d:","test.json",
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
	args := runTest
	if len(osArgs) > 0 {
		args = osArgs
	}
	var flag string
	var pairs [][]string
	var saveName string
	var data RequestData
	var mimeType string
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
	skipV := strings.Contains(args[0],"=") && ! strings.HasPrefix(args[0],"http")
	for len(args) > 0 {
		if skipV {
			flag = "v:"
		} else {
			flag, args = args[0], args[1:]
		}
		skipV = false
		switch flag {
		case "d:","data:":
			var m Map
			var e error
			pairs, args = grabWhilePairs(args)
			if len(pairs) == 0 {
				data.Payload, mimeType, args = loadFileIfPossible(args,true)
			} else {
				m = parsePairs(pairs)
				data.Payload, e = marshal(m)
				mimeType = "application/json"
				checke(e)
			}
			if !fetch {
				fmt.Print("Payload", data.Payload)
			}
		case "v:", "vars:":
			pairs, args = grabWhilePairs(args)
			if len(pairs) == 0 {
				var contents string
				contents,_, args = loadFileIfPossible(args,false)
				mappa := make(SMap)
				e := unmarshal(contents, &mappa)
				checke(e)
				data.Vars = mappa
			} else {
				data.Vars = pairsToMap(pairs)
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
			data.Url = flag
		}
	}
	if saveName != "" {
		writeConfig(saveName, ns, &data)
		return
	}

	if !fetch {
		fmt.Println("Vars", data.Vars)
		fmt.Println("Headers", data.Headers)
		if data.User != "" {
			fmt.Println("user", data.User, "password", data.Password)
		}
	}

	fullUrl := data.Url
	if data.Base != "" {
		fullUrl = data.Base + fullUrl
	}
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
		return
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
	failed := !(resp.StatusCode >= 200 && resp.StatusCode < 300)
	if failed {
		fmt.Fprintln(os.Stderr, "status", resp.Status)
	}
	// a special case with HEAD method - display headers in JSON format
	if data.Method == "HEAD" {
		m := make(SMap)
		for key := range resp.Header {
			m[key] = resp.Header.Get(key)
		}
		hj, e := marshal(m)
		checke(e)
		fmt.Println(hj)
	}
	// strong opinion: should just be able to get files
	path := filepath.Base(fullUrl)
	ext := filepath.Ext(path)
	if data.Method == "GET" && ext != "" && ! strings.ContainsAny(ext,"?=#") {
		err = ioutil.WriteFile(path, body, 0644)
		checke(err)
	} else {
		fmt.Println(string(body))
	}
	if failed {
		os.Exit(1)
	}

	// a LOT faster the second time 260ms vs 1560ms
	//start = time.Now()
	//client.Get(Url)
	//fmt.Println("took", time.Since(start))

}

func loadFileIfPossible(args Strings, allTypes bool) (string,string,Strings) {
	maybeFile := args[0]
	ext := filepath.Ext(maybeFile)
	if ext != ".json" && ! allTypes {
		// for now: could just read and see if we can marshel...
		quit("vars can only be set with JSON currently")
	}
	mtype := mime.TypeByExtension(ext)
	fmt.Println("type", mtype)
	if mtype == "" {
		// don'mtype know if we can further make a guess about charset...
		mtype = "text/plain"
	}
	if ext == ".json" {
		bb, e := ioutil.ReadFile(maybeFile)
		checke(e)
		return string(bb), mtype,args[1:]
	} else {
		quit("file extension not supported " + ext)
	}
	return "","",args
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

