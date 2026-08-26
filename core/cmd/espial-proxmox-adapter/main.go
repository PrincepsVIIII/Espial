package main

import (
	"fmt"
	"os"

	"github.com/PrincepsVIIII/Espial/core/internal/proxmox"
)

func main() {
	if err := proxmox.Run(os.Stdin, os.Stdout, proxmox.PolicyFromEnvironment()); err != nil {
		fmt.Fprintln(os.Stderr, "proxmox adapter stopped")
		os.Exit(1)
	}
}
