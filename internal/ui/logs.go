package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"time"
)

// LogPath returns the Lab UI log file used by the background server.
func LogPath() string {
	return uiLogPath()
}

// PrintLogs writes recent UI log lines to stdout. When follow is true, new lines are streamed.
func PrintLogs(lines int, follow bool) error {
	path := LogPath()
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no UI log at %s yet — start background UI with: camunda ui", path)
		}
		return err
	}
	defer f.Close()

	if lines > 0 {
		if err := printLastLines(f, lines); err != nil {
			return err
		}
	} else if _, err := io.Copy(os.Stdout, f); err != nil {
		return err
	}

	if !follow {
		return nil
	}

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			_, _ = os.Stdout.WriteString(line)
		}
		if err == io.EOF {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
	}
}

func printLastLines(f *os.File, n int) error {
	sc := bufio.NewScanner(f)
	buf := make([]string, 0, n)
	for sc.Scan() {
		buf = append(buf, sc.Text())
		if len(buf) > n {
			buf = buf[1:]
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	for _, line := range buf {
		fmt.Println(line)
	}
	return nil
}
