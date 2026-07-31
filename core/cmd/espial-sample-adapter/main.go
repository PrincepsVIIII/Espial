package main

import (
	"errors"
	"flag"
	"os"

	"github.com/PrincepsVIIII/Espial/core/internal/sampleadapter"
)

func main() {
	startupFault := flag.String("startup-fault", "none", "test-only startup fault")
	flag.Parse()
	if err := sampleadapter.Run(os.Stdin, os.Stdout, os.Stderr, *startupFault); err != nil {
		var exit *sampleadapter.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.Code)
		}
		os.Exit(1)
	}
}
