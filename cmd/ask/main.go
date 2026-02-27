package main

import (
	"os"

	"github.com/kyupark/ask/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
