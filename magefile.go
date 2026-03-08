//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Build compiles the moss binary with version info.
func Build() error {
	fmt.Println("Building moss...")
	commit := gitCommit()
	date := time.Now().UTC().Format(time.RFC3339)
	ldflags := fmt.Sprintf("-X github.com/devenjarvis/moss/internal/version.Version=dev -X github.com/devenjarvis/moss/internal/version.Commit=%s -X github.com/devenjarvis/moss/internal/version.Date=%s", commit, date)
	return run("go", "build", "-ldflags", ldflags, "-o", "moss", "./cmd/moss/")
}

func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "--short", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// Test runs all tests.
func Test() error {
	fmt.Println("Running tests...")
	return run("go", "test", "./...", "-count=1")
}

// Vet runs go vet on all packages.
func Vet() error {
	fmt.Println("Running go vet...")
	return run("go", "vet", "./...")
}

// Tidy runs go mod tidy.
func Tidy() error {
	fmt.Println("Tidying modules...")
	return run("go", "mod", "tidy")
}

// Clean removes the built binary.
func Clean() error {
	fmt.Println("Cleaning...")
	return os.Remove("moss")
}

// Check runs vet and tests together.
func Check() error {
	if err := Vet(); err != nil {
		return err
	}
	return Test()
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
