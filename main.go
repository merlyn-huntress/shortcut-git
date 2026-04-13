package main

import (
	"fmt"
	"os"
)

var commands = map[string]func([]string) error{
	"clone":    cmdClone,
	"fetch":    cmdFetch,
	"pull":     cmdPull,
	"push":     cmdPush,
	"add":      cmdAdd,
	"commit":   cmdCommit,
	"status":   cmdStatus,
	"diff":     cmdDiff,
	"log":      cmdLog,
	"reset":    cmdReset,
	"checkout": cmdCheckout,
	"version":  cmdVersion,
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: shortcut-git <command>")
		fmt.Fprintln(os.Stderr, "\nCommands:")
		fmt.Fprintln(os.Stderr, "  clone       Clone a Shortcut objective")
		fmt.Fprintln(os.Stderr, "  add         Stage changes")
		fmt.Fprintln(os.Stderr, "  commit      Record staged changes")
		fmt.Fprintln(os.Stderr, "  status      Show working tree status")
		fmt.Fprintln(os.Stderr, "  diff        Show changes")
		fmt.Fprintln(os.Stderr, "  log         Show commit history")
		fmt.Fprintln(os.Stderr, "  reset       Reset changes")
		fmt.Fprintln(os.Stderr, "  checkout    Restore files")
		fmt.Fprintln(os.Stderr, "  fetch       Download remote changes")
		fmt.Fprintln(os.Stderr, "  pull        Fetch and merge remote changes")
		fmt.Fprintln(os.Stderr, "  push        Send local changes to Shortcut")
		fmt.Fprintln(os.Stderr, "  version     Print version")
		os.Exit(1)
	}

	cmd, ok := commands[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
	if err := cmd(os.Args[2:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func cmdVersion(args []string) error {
	fmt.Println("shortcut-git v0.1.0")
	return nil
}
