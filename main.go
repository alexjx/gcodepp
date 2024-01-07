package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.App{
		Name:  "gcodeproc",
		Usage: "postprocess gcode",
		Commands: []*cli.Command{
			substituteCmd,
			preheatCmd,
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}
