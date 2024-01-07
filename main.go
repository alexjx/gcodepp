package main

import (
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
)

var (
	Version     = "0.1"
	BuiltAt     = "unknown"
	GitHash     = "unknown"
	BuildNumber = "unknown"
)

func main() {
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Fprintf(c.App.Writer,
			"Version:    %s\n"+
				"Git Commit: %s\n"+
				"Build Time: %s\n"+
				"Build:      %s\n",
			c.App.Version, GitHash, BuiltAt, BuildNumber)
	}

	app := cli.App{
		Name:    "gcodeproc",
		Usage:   "postprocess gcode",
		Version: Version,
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
