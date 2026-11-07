package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// readHidden reads a line from the console without showing it as it is typed or pasted.
func readHidden(prompt string) (string, error) {
	fmt.Fprint(os.Stderr, prompt)
	console := windows.Handle(os.Stdin.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(console, &mode); err == nil {
		if err := windows.SetConsoleMode(console, mode&^windows.ENABLE_ECHO_INPUT); err != nil {
			return "", fmt.Errorf("the key cannot be read without showing it: %w", err)
		}
		defer windows.SetConsoleMode(console, mode)
	}
	line, err := stdin.ReadString('\n')
	fmt.Fprintln(os.Stderr)
	if err != nil && line == "" {
		return "", fmt.Errorf("no key was given: %w", err)
	}
	return strings.TrimSpace(line), nil
}
