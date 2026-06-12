package kiss

import (
	"fmt"
	"io"
)

// errWriter 让多行 CLI 输出保留第一个 writer error，调用点可以保持顺序可读。
type errWriter struct {
	out io.Writer
	err error
}

func (w *errWriter) printf(format string, args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintf(w.out, format, args...)
}

func (w *errWriter) println(args ...any) {
	if w.err != nil {
		return
	}
	_, w.err = fmt.Fprintln(w.out, args...)
}
