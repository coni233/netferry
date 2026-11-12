package main

import (
	"fmt"
	"os"
	"strings"
)

const (
	colorReset = "\x1b[0m"
	colorGreen = "\x1b[32m"
	colorRed   = "\x1b[31m"
)

func printSuccess(format string, args ...any) {
	printColored(os.Stdout, colorGreen, "SUCCESS: "+format, args...)
}

func printError(format string, args ...any) {
	printColored(os.Stderr, colorRed, "ERROR: "+format, args...)
}

func printColored(file *os.File, color, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" || !enableColor(file) {
		fmt.Fprintln(file, message)
		return
	}
	fmt.Fprintf(file, "%s%s%s\n", color, message, colorReset)
}
