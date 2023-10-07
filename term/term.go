package term

import (
	"github.com/edgexfoundry/edgex-cli/pkg/pager"
	T "github.com/moby/term"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

const (
	cBrightRed     = "\x1b[31;1m"
	cBrightGreen   = "\x1b[32;1m"
	cGreen         = "\x1b[32m"
	cBrightYellow  = "\x1b[33;1m"
	cBrightBlue    = "\x1b[34;1m"
	cBrightMagenta = "\x1b[35;1m"
	cBrightCyan    = "\x1b[36;1m"
	cBlue          = "\x1b[34m"
	cBrightWhite   = "\x1b[37;1m"
	cReset         = "\x1b[0m"
)

func which(cmd string) string {
	path := os.Getenv("PATH")
	parts := strings.Split(path, ":")
	for _, p := range parts {
		exe := filepath.Join(p, cmd)
		_, e := os.Stat(exe)
		if e == nil {
			return exe
		}
	}
	return ""
}

func Page(txt string, pretty bool) {
	n := strings.Count(txt, "\n")
	var oww io.WriteCloser = os.Stdout
	fd := os.Stdout.Fd()
	if T.IsTerminal(fd) {
		if pretty {
			txt = Pretty(txt)
		}
		ws, err := T.GetWinsize(fd)
		if err != nil {
			log.Fatalf("term.GetWinsize: %s", err)
		}
		// does not account for lines which are longer than the page width and wrap around....
		if n > int(ws.Height) {
			if os.Getenv("PAGER") == "" {
				if which("less") != "" {
					os.Setenv("PAGER", "less -r")
				}
			}
			w, err := pager.NewWriter()
			if err != nil {
				log.Fatal(err)
			} else {
				oww = w
			}
		}
	}
	oww.Write([]byte(txt))
	oww.Close()
}
