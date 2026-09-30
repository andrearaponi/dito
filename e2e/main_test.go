package e2e

import (
	"fmt"
	"os"
	"testing"
)

// TestMain runs the scenarios, then prints the summary of their outcomes.
func TestMain(m *testing.M) {
	code := m.Run()
	for _, check := range afterSuite {
		if !check() && code == 0 {
			code = 1
		}
	}
	fmt.Println(defaultRegistry.summary())
	os.Exit(code)
}

// afterSuite holds checks run after all scenarios; a false result fails the
// suite (for example, a child process still alive).
var afterSuite []func() bool
