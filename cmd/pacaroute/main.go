package main

import (
	"github.com/LandoWang/pacaroute/internal/pacaroute"
	"os"
)

func main() { os.Exit(pacaroute.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
