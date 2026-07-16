package prompt

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func Choose(r io.Reader, w io.Writer, question string, options []string, def int) (string, error) {
	if def < 0 || def >= len(options) {
		def = 0
	}
	fmt.Fprintf(w, "%s\n", question)
	for i, opt := range options {
		mark := " "
		if i == def {
			mark = "*"
		}
		fmt.Fprintf(w, "  %d)%s %s\n", i+1, mark, opt)
	}
	fmt.Fprintf(w, "Choice [%d]: ", def+1)
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return options[def], nil
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return options[def], nil
	}
	n, err := strconv.Atoi(line)
	if err != nil || n < 1 || n > len(options) {
		return "", fmt.Errorf("invalid choice %q", line)
	}
	return options[n-1], nil
}

// String prompts for a single line; empty input returns def.
func String(r io.Reader, w io.Writer, question, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(w, "%s [%s]: ", question, def)
	} else {
		fmt.Fprintf(w, "%s: ", question)
	}
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return def, nil
	}
	line := strings.TrimSpace(scanner.Text())
	if line == "" {
		return def, nil
	}
	return line, nil
}
