package main

import (
	"os"
	"os/exec"
)

// execServer runs the real MCP server as a child process, wiring stdio
// through directly (MCP over stdio needs an unbuffered passthrough).
func execServer(command string, args []string, env []string) error {
	cmd := exec.Command(command, args...)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
