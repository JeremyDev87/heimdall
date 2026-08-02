package main

import (
	"os"

	"github.com/JeremyDev87/heimdall/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr)) }
