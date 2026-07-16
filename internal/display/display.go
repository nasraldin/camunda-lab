package display

import (
	"fmt"
	"io"
	"strings"
)

// Report builds a readable CLI card: title, key/value fields, named sections.
type Report struct {
	Title    string
	Fields   []Field
	Sections []Section
	Footer   []string
}

type Field struct {
	Label string
	Value string
}

type Section struct {
	Title string
	Items []string
	// Raw prints items as-is (no bullet prefix). Useful for nested blocks.
	Raw bool
}

func (r Report) Write(w io.Writer) {
	if r.Title != "" {
		fmt.Fprintln(w, r.Title)
		fmt.Fprintln(w, strings.Repeat("=", len(r.Title)))
		fmt.Fprintln(w)
	}
	if len(r.Fields) > 0 {
		width := 0
		for _, f := range r.Fields {
			if len(f.Label) > width {
				width = len(f.Label)
			}
		}
		for _, f := range r.Fields {
			fmt.Fprintf(w, "%-*s  %s\n", width, f.Label, f.Value)
		}
	}
	for _, sec := range r.Sections {
		fmt.Fprintln(w)
		fmt.Fprintln(w, sec.Title)
		for _, item := range sec.Items {
			if sec.Raw {
				fmt.Fprintln(w, item)
			} else {
				fmt.Fprintf(w, "  - %s\n", item)
			}
		}
	}
	if len(r.Footer) > 0 {
		fmt.Fprintln(w)
		for _, line := range r.Footer {
			fmt.Fprintln(w, line)
		}
	}
}

func KV(label, value string) Field {
	return Field{Label: label, Value: value}
}

func Bullet(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

func Success(msg string) string { return "pass  " + msg }
func Fail(msg string) string    { return "fail  " + msg }
func Info(msg string) string    { return "info  " + msg }
func Warn(msg string) string    { return "warn  " + msg }

func Note(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "note: "+format+"\n", args...)
}

func Step(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "==> "+format+"\n", args...)
}

func Done(w io.Writer, format string, args ...any) {
	fmt.Fprintf(w, "ok: "+format+"\n", args...)
}
