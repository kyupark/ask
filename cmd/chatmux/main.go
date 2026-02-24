package main

import (
	"os"

	"github.com/qm4/webai-cli/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
