package qmdx

import (
	"os/exec"
	"sync"
)

var (
	availableOnce sync.Once
	availableVal  bool
)

// Available reports whether the qmd CLI is installed and reachable on $PATH.
// The result is cached after the first call.
func Available() bool {
	availableOnce.Do(func() {
		_, err := exec.LookPath("qmd")
		availableVal = err == nil
	})
	return availableVal
}
