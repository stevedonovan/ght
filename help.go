package main

import (
	"fmt"
	"os"
)

var helpMap = map[string]string{
	"data": `data (d): <pairs> or <arg>
	<pairs> are interpreted as a JSON document to be part of a HTTP request
	<arg> may be text or a filename beginning with @ a la curl
	The file may contain JSON5 or YAML and will be converted to JSON.
`,
	"body": `body: (b): <arg>
	<arg> may be text or a @filename like with data: but NOT converted to JSON
`,
	"flags": `flags (f):
`,
	"query": `query (q):
`,
	"head": `head (h):
`,
	"out": `out (o):
`,
	"user": `user: <user>=<password>
	Simple Auth
`,
	"url": `url (u): <url>
	The host part of the complete address (env GHT_URL)
`,

	"output": "",
}

func checkHelp(empty bool, args []string, cmd string) {
	if empty && len(args) == 0 {
		fmt.Println(helpMap[cmd])
		os.Exit(0)
	}
}

func mainHelp() {
	for _, msg := range helpMap {
		fmt.Print(msg)
	}
	os.Exit(0)
}
