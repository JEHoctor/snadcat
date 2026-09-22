// Package prompt ports cli/lib/select.bash.
//
// The bash originals write the prompt and option list to stderr and the result
// to stdout, because callers capture them with command substitution. Go has no
// such constraint, but the split is kept: it means piping `sandcat init`'s
// stdout somewhere never swallows the questions.
package prompt

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// In and Out are indirected for tests.
var (
	In  io.Reader = os.Stdin
	Out io.Writer = os.Stderr
)

// Line reads a single line of input after showing prompt. Ports read_line.
func Line(prompt string) (string, error) {
	fmt.Fprintf(Out, "%s ", prompt)
	r := bufio.NewReader(In)
	s, err := r.ReadString('\n')
	if err != nil && s == "" {
		return "", err
	}
	return strings.TrimRight(s, "\r\n"), nil
}

// Option asks the user to pick one of options, defaulting to the first on empty
// input. Ports select_option, including its re-prompt-until-valid loop.
func Option(prompt string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no options provided")
	}
	def := options[0]
	fmt.Fprintf(Out, "%s [%s]\n", prompt, def)
	for i, o := range options {
		fmt.Fprintf(Out, "  %d) %s\n", i+1, o)
	}

	r := bufio.NewReader(In)
	for {
		fmt.Fprint(Out, "> ")
		reply, err := r.ReadString('\n')
		reply = strings.TrimSpace(reply)
		if reply == "" {
			// EOF with no input also lands here, matching bash's behavior of
			// taking the default rather than erroring.
			return def, nil
		}
		if n, cerr := strconv.Atoi(reply); cerr == nil && n >= 1 && n <= len(options) {
			return options[n-1], nil
		}
		if err != nil {
			return "", err
		}
		fmt.Fprintln(Out, "Invalid selection, try again.")
	}
}

// Multiple asks the user to pick zero or more options by comma-separated index,
// returning defaults on empty input. Ports select_multiple.
func Multiple(prompt string, options, defaults []string) ([]string, error) {
	fmt.Fprintf(Out, "%s\n", prompt)
	for i, o := range options {
		marker := ""
		for _, d := range defaults {
			if d == o {
				marker = " (default)"
				break
			}
		}
		fmt.Fprintf(Out, "  %d) %s%s\n", i+1, o, marker)
	}

	r := bufio.NewReader(In)
	for {
		fmt.Fprint(Out, "> ")
		reply, err := r.ReadString('\n')
		reply = strings.TrimSpace(reply)
		if reply == "" {
			return defaults, nil
		}

		var result []string
		valid := true
		for _, part := range strings.Split(reply, ",") {
			part = strings.TrimSpace(part)
			n, cerr := strconv.Atoi(part)
			if cerr != nil || n < 1 || n > len(options) {
				fmt.Fprintf(Out, "Invalid selection: %s\n", part)
				valid = false
				break
			}
			result = append(result, options[n-1])
		}
		if valid {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// YesNo asks a [y/N] question; only "y" or "Y" is a yes. Ports select_yes_no.
func YesNo(prompt string) (bool, error) {
	fmt.Fprintf(Out, "%s [y/N]: ", prompt)
	r := bufio.NewReader(In)
	s, err := r.ReadString('\n')
	if err != nil && s == "" {
		return false, nil // EOF is a no, as an empty read is in bash
	}
	s = strings.TrimSpace(s)
	return s == "y" || s == "Y", nil
}
