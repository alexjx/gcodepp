package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/sirupsen/logrus"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

var substituteCmd = &cli.Command{
	Name:    "substitute",
	Aliases: []string{"sub"},
	Usage:   "substitute a string in a gcode file",
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
	},
	Args:      true,
	ArgsUsage: "<gcode file>",
	Action: func(cctx *cli.Context) error {
		gcodePath := cctx.Args().First()
		if gcodePath == "" {
			return fmt.Errorf("missing gcode file")
		}

		var (
			cfg     SubstitutionConfig
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

		if err := substitute(gcodePath, &cfg); err != nil {
			logrus.Errorf("failed to substitute: %v", err)
			return err
		}

		return nil
	},
}

type Substitution struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`

	fromRegex *regexp.Regexp
	template  *template.Template
}

type SubstitutionConfig struct {
	Substitutions []*Substitution `yaml:"substitutions"`
}

type TemplateContext struct {
	Matches [][]string
}

func substitute(gcodePath string, cfg *SubstitutionConfig) error {
	// dump env
	logrus.Debugf("gcodePath: %s", gcodePath)
	for _, e := range os.Environ() {
		logrus.Debugf("env: %s", e)
	}

	// preparations
	for _, s := range cfg.Substitutions {
		s.fromRegex = regexp.MustCompile(s.From)
		tt := template.New(s.From).Funcs(sprig.TxtFuncMap())
		tt, err := tt.Parse(s.To)
		if err != nil {
			return fmt.Errorf("failed to parse template: %w", err)
		}
		s.template = tt
	}

	// open input and output files
	fp, err := os.Open(gcodePath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer fp.Close()

	outFp, err := os.Create(gcodePath + ".procssed")
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFp.Close()

	// scan gcode one line at a time
	scanner := bufio.NewScanner(fp)
	for scanner.Scan() {
		line := scanner.Text()

		var (
			matchesAny bool
			matches    = make([][]string, len(cfg.Substitutions))
		)
		for i, s := range cfg.Substitutions {
			matches[i] = s.fromRegex.FindStringSubmatch(line)
			if len(matches[i]) > 0 {
				matchesAny = true
			}
		}
		if !matchesAny {
			// no match, write line as is
			outFp.WriteString(line + "\n")
			continue
		}

		// provide matches as template data
		data := &TemplateContext{
			Matches: matches,
		}

		// render template into a temporary buffer
		for _, s := range cfg.Substitutions {
			var b bytes.Buffer
			w := bufio.NewWriter(&b)
			if err := s.template.Execute(w, data); err != nil {
				return fmt.Errorf("failed to execute template: %w", err)
			}
			w.Flush()

			ts := strings.ReplaceAll(b.String(), "\\n", "\n")
			line = s.fromRegex.ReplaceAllString(line, ts)
		}
		outFp.WriteString(line + "\n")
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan input file: %w", err)
	}

	fp.Close()
	outFp.Close()

	// rename output file to input file
	if err := os.Rename(outFp.Name(), fp.Name()); err != nil {
		return fmt.Errorf("failed to rename output file: %w", err)
	}

	return nil
}
