//go:build !windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// readHidden reads a line from the terminal without showing it as it is typed or pasted.
func readHidden(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	if info, err := os.Stdin.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
		hide := exec.Command("stty", "-echo")
		hide.Stdin = os.Stdin
		if err := hide.Run(); err != nil {
			return "", fmt.Errorf("the key cannot be read without showing it: %w", err)
		}
		defer func() {
			show := exec.Command("stty", "echo")
			show.Stdin = os.Stdin
			show.Run()
		}()
	}
	line, err := stdin.ReadString('\n')
	fmt.Fprintln(os.Stderr)
	if err != nil && line == "" {
		return "", fmt.Errorf("no key was given: %w", err)
	}
	return strings.TrimSpace(line), nil
}
