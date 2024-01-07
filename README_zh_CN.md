# GCODE 后处理器

本工具用来处理切片后的GCODE文件, 为了满足一些特殊需求, 例如删除替换某些指令, 或者用于换头系统的温度预热.

## 功能

- 替换指令 (sub)
- 预热 (preheat)

## TODO

- [ ] Preheat support for G2/G3
- [ ] Preheat support for G4

## 使用方法

配置切片工具, 调用本工具.

### 预热 (preheat)

预热指令用于在换头时, 预热挤出机. 本工具会在切片后的GCODE文件中, 查找换头指令, 并在换头前, 预热挤出机.
假如在多次换头前后的预热时间内, 有换头指令, 则会跳过关闭挤出机的指令, 同时跳过预热指令.

```bash
gcodepp.exe preheat --config config.yaml <input file>
```

配置样例, 以下配置用于在prushaslicer中, 预热温度:

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

配置文件包含两个部分, `extruders` 和 `costs`.

`extruders` 部分包含每个挤出机的配置, 每个包含以下属性:

- `name`: 挤出机的名称 (用于匹配换头指令)
- `heat_up`: 预热时间 (秒)
- `active_gcode`: 激活挤出机的指令
- `deactive_gcode`: 关闭挤出机的指令 (可选)

因为预热依赖于对于gcode的解析, 所以需要对指令时间进行追踪. `costs` 部分用于提供指令时间的估算.

- `toolchange`: 换头时间 (秒)
- `retraction`: 挤出机回抽时间 (秒)

### 替换指令 (sub)

替换指令, 用于查找符合正则表达式的指令, 并用模板替换. (模板使用go template语法)

模板包含以下变量:

- `.Matches`: 正则表达式的匹配结果

```bash
gcodepp.exe sub --config config.yaml <input file>
```

配置文件样例, 以下配置用于在prushaslicer中, 开启klipper的对象标记下, 微调z偏移:

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
