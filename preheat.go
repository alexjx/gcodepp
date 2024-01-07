package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

type Extruder struct {
	Name        string  `yaml:"name"`
	HeatUp      float64 `yaml:"heat_up"`
	ActiveGcode string  `yaml:"active_gcode"`
}

type PreheatConfig struct {
	Extruders []*Extruder `yaml:"extruders"`
}

type PreheatState struct {
	Config    *PreheatConfig
	Extruders map[string]*Extruder
	MaxHeatUp float64

	// state tracking
	Feedrate float64 // mm/min

	GcodesTime   float64 // current queued print time
	Gcodes       []*Gcode
	ExpiredCount int64
}

type Gcode struct {
	Parsed bool
	Line   string  // original line
	Time   float64 // for calculating print time

	Op      string
	X       float64
	Y       float64
	Z       float64
	E       float64
	F       float64
	Comment string
}

func (g *Gcode) String() string {
	return g.Line
}

func (g *Gcode) Distance() float64 {
	return math.Sqrt(math.Pow(g.X, 2) + math.Pow(g.Y, 2) + math.Pow(g.Z, 2) + math.Pow(g.E, 2))
}

func ParseGcode(line string) (g *Gcode) {
	g = &Gcode{Line: line}

	// strip comments
	if i := strings.Index(line, ";"); i != -1 {
		g.Comment = line[i+1:]
		line = line[:i]
	}

	// strip whitespace
	line = strings.TrimSpace(line)

	// split on whitespace
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return
	}

	// parse op
	g.Op = parts[0]

	// parse args
	prefix := ""
	for _, part := range parts[1:] {
		if prefix == "" {
			if len(part) == 0 {
				continue
			}
			if len(part) == 1 {
				prefix = part
				continue
			}
			prefix = part[:1]
			part = part[1:]
		}

		param, err := strconv.ParseFloat(part, 64)
		if err != nil {
			logrus.Debugf("failed to parse float: %s", part)
			return
		}

		switch prefix {
		case "X":
			g.X = param
		case "Y":
			g.Y = param
		case "Z":
			g.Z = param
		case "E":
			g.E = param
		case "F":
			g.F = param / 60.0 // convert to mm/s
		default:
			logrus.Debugf("unknown prefix: %s", prefix)
			return
		}

		prefix = ""
	}

	g.Parsed = true
	return
}

var preheatCmd = &cli.Command{
	Name:  "preheat",
	Usage: "preheat the next extruder in the queue",
	Flags: []cli.Flag{
		&cli.PathFlag{
			Name:     "config",
			Usage:    "config file",
			Required: true,
		},
	},
	Args:      true,
	ArgsUsage: "<gcode file>",
	Action: func(cctx *cli.Context) error {
		gcodePath := cctx.Args().First()
		if gcodePath == "" {
			return fmt.Errorf("missing gcode file")
		}

		var (
			cfg     PreheatConfig
			cfgPath = cctx.Path("config")
		)
		cfgFp, err := os.Open(cfgPath)
		if err != nil {
			return fmt.Errorf("failed to open config file: %w", err)
		}
		defer cfgFp.Close()

		if err := yaml.NewDecoder(cfgFp).Decode(&cfg); err != nil {
			return fmt.Errorf("failed to decode config file: %w", err)
		}

		// setup logging
		logfile := cctx.Path("log")
		if err := setupLogging(logfile); err != nil {
			return err
		}

		if err := Preheat(gcodePath, &cfg); err != nil {
			logrus.Errorf("failed to Preheat: %v", err)
			return err
		}

		return nil
	},
}

func Preheat(gcodePath string, cfg *PreheatConfig) error {
	state := &PreheatState{
		Config:    cfg,
		Extruders: make(map[string]*Extruder),
	}
	for _, extruder := range cfg.Extruders {
		state.Extruders[extruder.Name] = extruder
		if extruder.HeatUp > state.MaxHeatUp {
			state.MaxHeatUp = extruder.HeatUp
		}
	}

	// read gcode file
	gcodeFp, err := os.Open(gcodePath)
	if err != nil {
		return fmt.Errorf("failed to open gcode file: %w", err)
	}
	defer gcodeFp.Close()

	outputFp, err := os.Create(gcodePath + ".preheat")
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outputFp.Close()

	// parse gcode file
	scanner := bufio.NewScanner(gcodeFp)
	for scanner.Scan() {
		line := scanner.Text()

		g := ParseGcode(line)
		if g.Parsed {
			if g.F > 0 {
				g.Time = g.Distance() / g.F
				state.Feedrate = g.F
			} else if state.Feedrate > 0 {
				g.Time = g.Distance() / state.Feedrate
			}
		}
		state.Gcodes = append(state.Gcodes, g)
		state.GcodesTime += g.Time

		// if we don't have a tool change
		if g.Parsed && g.Op[0] != 'T' {
			// see if we to expire an entry
			for len(state.Gcodes) > 1 && (state.GcodesTime-state.Gcodes[0].Time) > state.MaxHeatUp {
				outputFp.WriteString(state.Gcodes[0].Line + "\n")
				state.GcodesTime -= state.Gcodes[0].Time
				state.Gcodes = state.Gcodes[1:]
				state.ExpiredCount++
			}
			continue
		}

		// we have a tool change, we insert an tool active gcode at current queue head
		if extruder, ok := state.Extruders[g.Op]; ok && state.ExpiredCount > 0 { // avoid inserting active gcode at the start of the file
			preheatGcode := fmt.Sprintf("; PREHEAT %s %.1fs earlier\n%s\n", extruder.Name, state.GcodesTime, extruder.ActiveGcode)
			outputFp.WriteString(preheatGcode)
		}
	}

	// write out remaining gcodes
	for _, g := range state.Gcodes {
		outputFp.WriteString(g.Line + "\n")
	}

	// close files and rename output file to input
	gcodeFp.Close()
	outputFp.Close()

	if err := os.Rename(gcodePath+".preheat", gcodePath); err != nil {
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	return nil
}
