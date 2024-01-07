package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/phf/go-queue/queue"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

type Extruder struct {
	Name            string  `yaml:"name"`
	HeatUp          float64 `yaml:"heat_up"`
	ActiveGcode     string  `yaml:"active_gcode"`
	DeactivateGcode string  `yaml:"deactivate_gcode"`

	// internal state
	preheatedTime    float64 // time when this extruder is preheated
	preheatedForTime float64 // time when this extruder is preheated for
	deactivatedTime  float64 // time when this extruder is deactivated
}

type GcodeCost struct {
	Toolchange float64 `yaml:"toolchange"`
	Retraction float64 `yaml:"retraction"`
}

type PreheatConfig struct {
	Extruders []*Extruder `yaml:"extruders"`
	Costs     *GcodeCost  `yaml:"costs"`

	speedChangeRatio float64
	noRename         bool
	debug            bool
}

type ExtruderState struct {
	X float64
	Y float64
	Z float64
	E float64

	Feedrate float64
	RelExtr  bool
	RelPos   bool
}

func (s *ExtruderState) Update(g *Gcode) {
	if s.RelPos {
		s.X += g.X.Value
		s.Y += g.Y.Value
		s.Z += g.Z.Value
	} else {
		if g.X.Valid {
			s.X = g.X.Value
		}
		if g.Y.Valid {
			s.Y = g.Y.Value
		}
		if g.Z.Valid {
			s.Z = g.Z.Value
		}
	}

	if !s.RelExtr {
		if g.E.Valid {
			s.E = g.E.Value
		}
	} else {
		s.E += g.E.Value
	}
}

type PreheatState struct {
	Config    *PreheatConfig
	Extruders map[string]*Extruder
	MaxHeatUp float64

	// state tracking
	State   *ExtruderState
	Current *Extruder

	// gcode tracking
	PrintTime float64 // print time of all processed gcodes

	GcodesTime float64 // current queued print time
	Gcodes     *GcodeQueue

	ToolchangeCount int64
}

type NullableFloat64 struct {
	Value float64
	Valid bool
}

type Gcode struct {
	Parsed bool

	PrintTime float64 // cumulative time offset

	Line   string  // original line
	LineNo int64   // line number
	Time   float64 // for calculating print time

	ToolchangeCode bool // is this a toolchange code
	DeactivateCode bool // is this a deactivate code

	Extruder *Extruder
	PrevExtr *Extruder

	Op string

	X NullableFloat64
	Y NullableFloat64
	Z NullableFloat64

	E NullableFloat64

	I NullableFloat64
	J NullableFloat64
	K NullableFloat64

	S NullableFloat64
	F NullableFloat64
	P NullableFloat64
	R NullableFloat64

	Comment string
}

func (g *Gcode) String() string {
	return g.Line
}

func (g *Gcode) IsMove() bool {
	switch g.Op {
	case "G0", "G1", "G2", "G3":
		return true
	}
	return false
}

func (g *Gcode) Distance(cur *ExtruderState) float64 {
	switch g.Op {
	case "G0", "G1":
		var (
			E float64
			X float64
			Y float64
			Z float64
		)

		if g.E.Valid {
			E = g.E.Value
		}
		if !cur.RelExtr {
			E -= cur.E
		}

		if g.X.Valid {
			X = g.X.Value
			if !cur.RelPos {
				X -= cur.X
			}
		}
		if g.Y.Valid {
			Y = g.Y.Value
			if !cur.RelPos {
				Y -= cur.Y
			}
		}
		if g.Z.Valid {
			Z = g.Z.Value
			if !cur.RelPos {
				Z -= cur.Z
			}
		}
		return math.Sqrt(math.Pow(X, 2) + math.Pow(Y, 2) + math.Pow(Z, 2) + math.Pow(E, 2))
	case "G2", "G3":
		// arc fitting gcodes
	}

	return 0.0
}

func ParseGcode(line string, lineNo int64) (g *Gcode) {
	g = &Gcode{Line: line, LineNo: lineNo}

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
	g.Op = strings.ToUpper(parts[0])

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
		case "X", "x":
			g.X.Value = param
			g.X.Valid = true
		case "Y", "y":
			g.Y.Value = param
			g.Y.Valid = true
		case "Z", "z":
			g.Z.Value = param
			g.Z.Valid = true
		case "E", "e":
			g.E.Value = param
			g.E.Valid = true
		case "I", "i":
			g.I.Value = param
			g.I.Valid = true
		case "J", "j":
			g.J.Value = param
			g.J.Valid = true
		case "K", "k":
			g.K.Value = param
			g.K.Valid = true
		case "F", "f":
			g.F.Value = param / 60.0 // convert to mm/s
			g.F.Valid = true
		case "S", "s":
			g.S.Value = param
			g.S.Valid = true
		case "P", "p":
			g.P.Value = param
			g.P.Valid = true
		case "R", "r":
			g.R.Value = param
			g.R.Valid = true
		default:
			logrus.Debugf("line: %s", line)
			logrus.Debugf("unknown prefix: %s", prefix)
			return
		}

		prefix = ""
	}

	g.Parsed = true
	return
}

type GcodeQueue struct {
	q *queue.Queue
}

func (q *GcodeQueue) Push(g *Gcode) {
	q.q.PushBack(g)
}

func (q *GcodeQueue) Pop() *Gcode {
	v := q.q.PopFront()
	if v == nil {
		return nil
	}
	return v.(*Gcode)
}

func (q *GcodeQueue) Len() int {
	return q.q.Len()
}

func (q *GcodeQueue) Front() *Gcode {
	v := q.q.Front()
	if v == nil {
		return nil
	}
	return v.(*Gcode)
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
		&cli.PathFlag{
			Name:  "log",
			Usage: "log file",
		},
		&cli.Float64Flag{
			Name:  "speed-change-ratio",
			Usage: "ratio of time in speed change phase of each move",
			Value: 0.4,
		},

		// debug flags
		&cli.BoolFlag{
			Name:   "no-rename",
			Value:  false,
			Hidden: true,
		},
		&cli.BoolFlag{
			Name:   "debug",
			Value:  false,
			Hidden: true,
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

		// check configs
		if len(cfg.Extruders) == 0 {
			return fmt.Errorf("no extruders defined")
		}
		for _, extruder := range cfg.Extruders {
			if extruder.Name == "" {
				return fmt.Errorf("extruder name cannot be empty")
			}
			if extruder.ActiveGcode == "" {
				return fmt.Errorf("extruder active gcode cannot be empty")
			}
			if extruder.HeatUp <= 0 {
				return fmt.Errorf("extruder heat up time must be positive")
			}
		}

		// setup logging
		logfile := cctx.Path("log")
		if err := setupLogging(logfile); err != nil {
			return err
		}

		cfg.speedChangeRatio = cctx.Float64("speed-change-ratio")
		cfg.noRename = cctx.Bool("no-rename")
		cfg.debug = cctx.Bool("debug")

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
		State:     &ExtruderState{},
		Gcodes:    &GcodeQueue{q: queue.New()},
	}
	for _, extruder := range cfg.Extruders {
		normlizedName := strings.ToUpper(extruder.Name)
		state.Extruders[normlizedName] = extruder
		if extruder.HeatUp > state.MaxHeatUp {
			state.MaxHeatUp = extruder.HeatUp
		}

		// reset internal state
		extruder.preheatedForTime = -1.0
		extruder.preheatedTime = -1.0
		extruder.deactivatedTime = -1.0
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
	lineNo := int64(0)
	for scanner.Scan() {
		// output processed gcodes
		// see if we to expire an entry
		shouldFlush := func() bool {
			// if we have long enough gcodes in the queue
			frontCode := state.Gcodes.Front()
			if (state.GcodesTime - frontCode.Time) > state.MaxHeatUp {
				return true
			}
			// always flush for the prolog of the file
			if state.ToolchangeCount == 0 {
				return true
			}
			return false
		}

		for state.Gcodes.Len() > 1 && shouldFlush() {
			var (
				frontCode    = state.Gcodes.Front()
				debugComment string
			)

			// see if we should cancel the deactivate code
			if frontCode.DeactivateCode {
				extr := frontCode.Extruder
				// if there is a preheat for this extruder in the last
				// and it's for a time after the current gcode
				if extr.preheatedTime < frontCode.PrintTime && extr.preheatedForTime > frontCode.PrintTime {
					logrus.Debugf("cancel deactivate %s @ %.1f: preheatedTime=%.1f preheatedFor=%.1f",
						extr.Name, frontCode.PrintTime, extr.preheatedTime, extr.preheatedForTime)
					// this deactivate code should be cancelled
					state.Gcodes.Pop()
					continue
				}
				// this deactivate code should be executed
				extr.deactivatedTime = frontCode.PrintTime
			}

			if cfg.debug {
				if frontCode.IsMove() || frontCode.ToolchangeCode {
					debugComment = fmt.Sprintf("  ; printTime=%.1f", frontCode.PrintTime)
				}
				if frontCode.ToolchangeCode {
					// include if there is a pending preheat
					// - previous extruder has preheat time before current
					// - previous extruder has preheat for time after current
					if prevExtruder := frontCode.PrevExtr; prevExtruder != nil {
						debugComment += " prev=" + prevExtruder.Name
						if prevExtruder.preheatedTime < frontCode.PrintTime && prevExtruder.preheatedForTime > frontCode.PrintTime {
							debugComment += fmt.Sprintf(" preheating [%1.f -> %.1f]", prevExtruder.preheatedTime, prevExtruder.preheatedForTime)
						}
					}
				}
			}
			outputFp.WriteString(frontCode.Line + debugComment + "\n")

			state.GcodesTime -= frontCode.Time
			state.Gcodes.Pop()
		}

		// parse gcode
		line := scanner.Text()
		lineNo++

		g := ParseGcode(line, lineNo)
		if g.Parsed {
			if g.Op == "M82" {
				state.State.RelExtr = false
			} else if g.Op == "M83" {
				state.State.RelExtr = true
			} else if g.Op == "G90" {
				state.State.RelPos = false
			} else if g.Op == "G91" {
				state.State.RelPos = true
			} else if g.Op == "G10" || g.Op == "G11" {
				if cfg.Costs != nil {
					g.Time = cfg.Costs.Retraction
				}
			} else if g.IsMove() {
				// calculate time for move gcodes
				d := g.Distance(state.State)

				// FIXME: this is not accurate
				// we should consider the acceleration and deceleration time
				// let's first be rough: 30% of the time is acceleration and deceleration
				if g.F.Valid {
					g.Time = d / g.F.Value
					state.State.Feedrate = g.F.Value
				} else if state.State.Feedrate > 0 {
					g.Time = d / state.State.Feedrate
				}
				g.Time += g.Time * cfg.speedChangeRatio

				state.State.Update(g)
			}

			// toolchange code has fixed time
			_, tcCode := state.Extruders[g.Op]
			if tcCode {
				g.ToolchangeCode = tcCode
				if cfg.Costs != nil {
					g.Time = cfg.Costs.Toolchange
				}
			}
		}

		// this is essential:
		// by trying to encode each gcode with the print time
		// we establish an order of gcodes which we could compare
		// if a gcode is within a certain time of the head of the queue
		g.PrintTime = state.PrintTime
		state.Gcodes.Push(g)
		if g.Time > 0 {
			state.GcodesTime += g.Time
			state.PrintTime += g.Time
		}

		// update tracking state
		if g.Parsed {
			// for none toolchange codes, it triggers a queue maintenance phase
			if !g.ToolchangeCode {
				g.Extruder = state.Current
				continue
			}

			// this is a toolchange code
			// we need to do two things:
			// 1. insert a preheat gcode at the head of the queue for this tool
			// 2. consider if we need to deactivate the previous tool, at the tail of the queue
			//    but this is not optimal. Since we might problem to deactivate a tool that is preheated earlier
			//    ----pA-pB---pA--a-b-a
			//                      ^ if we deactivated a here. the previous pA is useless
			//    so we can deactivate a tool only if it is not preheated in the future queue...
			extruder := state.Extruders[g.Op]
			curExtr := state.Current
			if state.ToolchangeCount > 0 { // avoid inserting active gcode at the start of the file
				// we have a tool change, we insert an tool active gcode at current queue head
				qHeadCode := state.Gcodes.Front()

				// do not preheat the tool if it has not yet been deactivated
				if extruder.preheatedTime < 0.0 || (extruder.deactivatedTime > 0.0 && extruder.preheatedTime < extruder.deactivatedTime) {
					preheatGcode := fmt.Sprintf("; PREHEAT %s [%.1f -> %.1f] (last %.1f / deactive %.1f) \n%s\n",
						extruder.Name, qHeadCode.PrintTime, g.PrintTime,
						extruder.preheatedTime, extruder.deactivatedTime,
						extruder.ActiveGcode,
					)
					outputFp.WriteString(preheatGcode)
					extruder.preheatedTime = qHeadCode.PrintTime // this is the time when this extruder is preheated
				} else {
					logrus.Debugf("skip preheat for %s @ %.1f: [%.1f -> %.1f] / %.1f",
						extruder.Name, g.PrintTime, extruder.preheatedTime, extruder.preheatedForTime, extruder.deactivatedTime)
				}
				extruder.preheatedForTime = g.PrintTime

				// check if we should deactivate the current tool
				// we should only deactivate if the current tool is not preheated
				// NOTE: this is only queued, it might be cancelled when we flush the queue
				if curExtr != nil && curExtr.DeactivateGcode != "" {
					logrus.Debugf("queue deactivate %s @ %.1f", curExtr.Name, g.PrintTime)
					deactivateGcode := fmt.Sprintf("; DEACTIVATE %s @ %.1f\n%s\n",
						curExtr.Name, g.PrintTime, curExtr.DeactivateGcode)
					// we need to enqueue this gcode
					code := Gcode{
						Line:           deactivateGcode,
						LineNo:         g.LineNo, // duplicate line number
						DeactivateCode: true,
						Extruder:       curExtr,
						PrintTime:      g.PrintTime,
					}
					state.Gcodes.Push(&code)
				}
			}

			state.ToolchangeCount++
			state.Current = extruder

			g.Extruder = extruder
			g.PrevExtr = curExtr
		}
	}

	// write out remaining gcodes
	for state.Gcodes.Len() > 0 {
		g := state.Gcodes.Pop()
		outputFp.WriteString(g.Line + "\n")
	}

	// close files and rename output file to input
	gcodeFp.Close()
	outputFp.Close()

	if !cfg.noRename {
		if err := os.Rename(gcodePath+".preheat", gcodePath); err != nil {
			return fmt.Errorf("failed to rename output file: %w", err)
		}
	}

	return nil
}
