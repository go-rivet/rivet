package main

import (
	"os"
)

func main() {
	if len(os.Args) < 2 {
		os.Exit(1)
	}

	switch os.Args[1] {
	case "templater":
		genTemplaterDoc()
	case "cli":
		genCliDoc()
	}

	os.Exit(0)
}
