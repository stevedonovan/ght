package term

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/scanner"
)

func cOut(f io.Writer, s *scanner.Scanner, clr string) {
	Out(f,s.TokenText(),clr)
}

func Out(f io.Writer, txt string, clr string) {
	fmt.Fprintf(f,"%s%s%s", clr,txt,cReset)
}

func BrightRed(f io.Writer, format string,  args ...interface{}) {
	Out(f,fmt.Sprintf(format,args...),cBrightRed)
}

func Pretty(src string) string {
	var	s        scanner.Scanner
	stack := []bool{}
	push := func (obj bool) {
		stack = append(stack,obj)
	}
	pop := func () bool {
		l := len(stack) - 1
		res := stack[l]
		stack = stack[0:l]
		return res
	}

	s.Init(strings.NewReader(src))
	s.Whitespace  = 0 //^= 1<<'\t' | 1<<'\n' | 1<<' ' // don't skip tabs and new lines
	ff := &bytes.Buffer{}
	push(false)
	inKey := true
	inObj := false
	for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
		switch tok {
		case '{','}','[',']',':',',':
			switch tok {
			case '{':
				push(inObj)
				inObj = true
				inKey = inObj
			case '[':
				push(inObj)
				inObj = false
				inKey = inObj
			case '}', ']':
				inObj = pop()
			case ':':
				inKey = false
			case ',':
				if inObj {
					inKey = true
				}
			}
			cOut(ff,&s,cBrightGreen)
		case scanner.Ident:
			cOut(ff,&s,cBrightGreen)
		case scanner.Int, scanner.Float:
			cOut(ff,&s,cBrightCyan)
		case scanner.String:
			if inKey {
				cOut(ff, &s, cBrightMagenta)
			} else {
				cOut(ff, &s, cBrightBlue)
			}
		default:
			fmt.Fprintf(ff,"%s",s.TokenText())
		}
	}
	return ff.String()
}

