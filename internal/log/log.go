// Package log reproduces the output format of cli/lib/logging.bash:
//
//	HH:MM:SS [INFO] message
//
// with the label colored and bold when stderr is a terminal. Everything goes to
// stderr, including Info — the bash helpers redirect the whole _log function
// there, and `sandcat init`'s summary output relies on it (stdout stays clean
// for command substitution).
package log

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// ANSI equivalents of the tput calls in logging.bash: setaf <n>, bold, sgr0.
const (
	ansiReset  = "\033[0m"
	ansiBold   = "\033[1m"
	ansiRed    = "\033[31m" // setaf 1
	ansiYellow = "\033[33m" // setaf 3
	ansiBlue   = "\033[34m" // setaf 4
)

// Out is where log lines are written. Overridden in tests.
var Out io.Writer = os.Stderr

// color reports whether to emit escape sequences. logging.bash degrades to
// plain text when tput fails (TERM unset or not a terminal); the equivalent
// check here is "stderr is a character device and TERM is usable".
var color = func() bool {
	if os.Getenv("TERM") == "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}()

// now is indirected so tests can pin the timestamp.
var now = time.Now

func emit(colorCode, label, msg string) {
	ts := now().Format("15:04:05")
	// Matches logging.bash's `while IFS= read -r line`: each line of a
	// multi-line message gets its own timestamp and label.
	for _, line := range strings.Split(strings.TrimSuffix(msg, "\n"), "\n") {
		if color {
			fmt.Fprintf(Out, "%s %s%s%s %s%s\n", ts, colorCode, ansiBold, label, ansiReset, line)
		} else {
			fmt.Fprintf(Out, "%s %s %s\n", ts, label, line)
		}
	}
}

func Info(format string, a ...any)  { emit(ansiBlue, "[INFO]", fmt.Sprintf(format, a...)) }
func Warn(format string, a ...any)  { emit(ansiYellow, "[WARN]", fmt.Sprintf(format, a...)) }
func Error(format string, a ...any) { emit(ansiRed, "[ERROR]", fmt.Sprintf(format, a...)) }
