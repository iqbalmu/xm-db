package main

import (
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/iqbalmu/dbm/internal/commands"
)

func main() {
	// Seed random for password generation
	rand.Seed(time.Now().UnixNano())

	if err := commands.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
