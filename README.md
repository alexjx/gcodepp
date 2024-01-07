# GCODE Post Processor

This is a post processor tool for 3D print slicers.

## Features

- Substitutes
- Preheat extruder in tool changer

## TODO

- [ ] Preheat support for G2/G3
- [ ] Preheat support for G4

## Usage

### Substitutes

It finds all lines matching with the regular expression and replaces them with the template.
The template is a go template with the following variables:

- `.Matches`: the list of matches of the regular expression

```bash
gcodepp.exe sub --config config.yaml <input file>
```

Config file example:

```yaml
substitutions:
- from: EXCLUDE_OBJECT_(START|END) NAME=(.*)
  to: |-
    {{- $op := index .Matches 0 1 }}
    {{- $name := index .Matches 0 2 }}
    {{- $offset := mulf (sub ( $name | int ) 5 | float64) 0.02 -}}
    EXCLUDE_OBJECT_{{ $op }} NAME={{ $name }}
    {{- if eq $op "START" }}
    RESPOND MSG="offsetting {{ $name }} {{ $offset }}"
    SET_GCODE_OFFSET Z_ADJUST={{ addf 0.0 $offset }} MOVE=1
    {{- else }}
    RESPOND MSG="restore offset {{ $name }} {{ $offset }}"
    SET_GCODE_OFFSET Z_ADJUST={{ subf 0.0 $offset }} MOVE=1
    {{- end }}

```

### Preheat extruder in tool changer

```bash
gcodepp.exe preheat --config config.yaml <input file>
```

Config file example:

```yaml
costs:
  toolchange: 10 # seconds, time to change tool
  retraction: 0.02 # seconds, time to retract/unretract filament
extruders:
- name: T0
  heat_up: 90
  active_gcode: |-
    RESPOND PREFIX="preheat" MSG="preheating tool 0"
    SET_TOOL_TEMPERATURE TOOL=0 CHNG_STATE=2
- name: T1
  heat_up: 90
  active_gcode: |-
    RESPOND PREFIX="preheat" MSG="preheating tool 1"
    SET_TOOL_TEMPERATURE TOOL=1 CHNG_STATE=2
- name: T2
  heat_up: 90
  active_gcode: |-
    RESPOND PREFIX="preheat" MSG="preheating tool 2"
    SET_TOOL_TEMPERATURE TOOL=2 CHNG_STATE=2
- name: T3
  heat_up: 90
  active_gcode: |-
    RESPOND PREFIX="preheat" MSG="preheating tool 3"
    SET_TOOL_TEMPERATURE TOOL=3 CHNG_STATE=2
```

Each extruder has the following properties:

- `name`: the name of the extruder (This matches the tool change gcode)
- `heat_up`: the time (in seconds) needs to heat up the extruder
- `active_gcode`: the gcode to activate the extruder
- `deactive_gcode`: the gcode to deactivate the extruder (optional)

There is also a `costs` section with the following properties:

- `toolchange`: the time (in seconds) to change the tool
- `retraction`: the time (in seconds) to retract/unretract the filament
