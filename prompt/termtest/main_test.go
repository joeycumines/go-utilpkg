package termtest

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

// TestMain acts as a gatekeeper for our test binary. When tests are run,
// this function is executed. If the special environment variable "GO_TEST_MODE"
// is set to "helper", it runs the helper process logic instead of the tests.
// This allows the main test binary to be re-executed as a subprocess for testing
// PTY interactions.
func TestMain(m *testing.M) {
	if os.Getenv("GO_TEST_MODE") == "helper" {
		runHelperProcess()
		return
	}
	os.Exit(m.Run())
}

// runHelperProcess contains the logic for the command-line tool we are testing against.
// It reads commands from os.Args and performs actions like echoing input,
// printing env vars, or exiting with a specific code.
func runHelperProcess() {
	args := os.Args[1:]

	// Skip past the "--" separator if present
	for i, arg := range args {
		if arg == "--" {
			args = args[i+1:]
			break
		}
	}

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Helper process requires a command.")
		os.Exit(1)
	}

	command := args[0]
	switch command {
	case "echo":
		// Echoes arguments back to stdout, one per line.
		for _, arg := range args[1:] {
			fmt.Println(arg)
		}
	case "interactive":
		// A simple interactive echo server.
		fmt.Println("Interactive mode ready")
		os.Stdout.Sync() // Ensure initial message is flushed
		scanner := &lineScanner{r: os.Stdin}
		for {
			input, err := scanner.ReadLine()
			if err != nil {
				os.Exit(0) // Exit cleanly on EOF (e.g., Ctrl-D)
			}
			if input == "exit" {
				fmt.Println("Exiting.")
				os.Exit(0)
			}
			fmt.Printf("ECHO: %s\n", input)
			os.Stdout.Sync() // Flush after each echo
		}
	case "env":
		// Prints the value of a specific environment variable.
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "env command requires an environment variable name.")
			os.Exit(1)
		}
		fmt.Println(os.Getenv(args[1]))
	case "pwd":
		// Prints the current working directory.
		wd, err := os.Getwd()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Failed to get working directory: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(wd)
	case "exit":
		// Exits with a specific code.
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "exit command requires an exit code.")
			os.Exit(1)
		}
		var code int
		fmt.Sscan(args[1], &code)
		os.Exit(code)
	case "ansi":
		// Prints text with ANSI escape codes.
		fmt.Println("\x1b[31mHello Red\x1b[0m")
	case "wait":
		// Waits for a specified duration then prints a message.
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "wait command requires a duration (e.g., 100ms).")
			os.Exit(1)
		}
		d, err := time.ParseDuration(args[1])
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Invalid duration: %v\n", err)
			os.Exit(1)
		}
		time.Sleep(d)
		fmt.Println("Waited.")
	default:
		_, _ = fmt.Fprintf(os.Stderr, "Unknown helper command: %s\n", command)
		os.Exit(1)
	}
}

// This is a dummy test that is never run, but its presence allows the `TestMain`
// logic to be part of the test binary.
func TestHelperProcess(t *testing.T) {
	// This test validates the test binary's helper behavior is not executed
	// when running the normal test harness. If this file is executed in
	// helper mode TestMain will call runHelperProcess and not run tests.
	if os.Getenv("GO_TEST_MODE") == "helper" {
		// When running in helper mode TestMain will not run tests; ensure we
		// never get here under normal circumstances.
		t.Fatalf("TestHelperProcess should not run in helper mode")
	}
}

// lineScanner provides simple line-by-line reading that works with PTYs
type lineScanner struct {
	r      io.Reader
	buffer *bufio.Reader
}

func (s *lineScanner) ReadLine() (string, error) {
	if s.buffer == nil {
		s.buffer = bufio.NewReader(s.r)
	}
	line, err := s.buffer.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	// Trim newline and carriage return
	line = strings.TrimRight(line, "\r\n")
	return line, err
}
