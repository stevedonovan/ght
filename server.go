package main

import (
	"fmt"
	"github.com/NYTimes/gziphandler"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os/exec"
	"runtime"
	"strings"
)

type Mapa = map[string][]string

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	file := strings.TrimPrefix(r.RequestURI, "/upload/")
	contents, e := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if e != nil {
		http.Error(w, "cannot read body", http.StatusInternalServerError)
	} else {
		e = ioutil.WriteFile(file, contents, 0644)
		if e != nil {
			http.Error(w, "cannot write "+file, http.StatusBadRequest)
		}
		log.Printf("upload: wrote %q (%d)", file, len(contents))
	}
}

func requestToJson(r *http.Request) Map {
	b := []byte{}
	if r.Body != nil {
		b, _ = ioutil.ReadAll(r.Body)
		defer r.Body.Close()
	}
	q := r.URL.Query()
	h := squashArray(r.Header)
	m := Map{
		"method": r.Method,
		"header": h,
		"url":    r.URL.Path,
		"host":   r.Host,
	}
	if len(b) > 0 {
		if r.Header.Get("Content-Type") == "application/json" {
			var out interface{}
			unmarshal(string(b), &out)
			m["json"] = out
		} else {
			m["body"] = string(b)
		}
	}
	if len(q) > 0 {
		m["query"] = squashArray(q)
	}
	return m
}

func anyHandler(w http.ResponseWriter, r *http.Request) {
	m := requestToJson(r)
	o, _ := marshal(m, true)
	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, "%s", o)
}

func squashArray(in Mapa) SMap {
	res := make(SMap, len(in))
	for k, v := range in {
		res[k] = v[0]
	}
	return res
}

func execHandler(w http.ResponseWriter, r *http.Request) {
	b, e := ioutil.ReadAll(r.Body)
	defer r.Body.Close()
	if e != nil {
		http.Error(w, "cannot read body", http.StatusInternalServerError)
		return
	}
	json := string(b)
	var cc map[string]string
	e = unmarshal(json, &cc)
	if e != nil || cc["cmd"] == "" {
		http.Error(w, "cannot decode body or no cmd field", http.StatusInternalServerError)
		return
	}
	command := cc["cmd"]
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", command)
	} else {
		cmd = exec.Command("sh", "-c", command)
	}
	if dir, ok := cc["dir"]; ok {
		cmd.Dir = dir
	}
	stdoutf, _ := cmd.StdoutPipe()
	stderrf, _ := cmd.StderrPipe()
	cmd.Start()
	res := map[string]string{}
	out_bytes, _ := ioutil.ReadAll(stdoutf)
	res["stdout"] = string(out_bytes)
	err_bytes, _ := ioutil.ReadAll(stderrf)
	res["stderr"] = string(err_bytes)
	e = cmd.Wait()
	if e != nil {
		res["error"] = e.Error()
	}
	o, _ := marshal(res, true)
	w.Header().Add("Content-Type", "application/json")
	fmt.Fprintf(w, "%s", o)
}

func runServer(args []string) {
	http.HandleFunc("/any/", anyHandler)
	http.Handle("/download/", gziphandler.GzipHandler(
		http.StripPrefix("/download/", http.FileServer(http.Dir(".")))))
	http.HandleFunc("/upload/", uploadHandler)
	http.HandleFunc("/exec", execHandler)
	http.HandleFunc("/echo/", echoHandler)
	log.Fatal(http.ListenAndServe(args[0], nil))
}

func echoHandler(w http.ResponseWriter, r *http.Request) {
	bb, err := httputil.DumpRequest(r, true)
	if err != nil {
		log.Println("cannot dump", err)
	} else {
		log.Println(string(bb))
	}
	fmt.Fprintf(w, "ok")
}
