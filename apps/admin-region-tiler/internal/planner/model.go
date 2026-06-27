package planner

import (
	"errors"
	"strings"
	"time"

	"tiler/internal/area"
)

const (
	ScheduleImmediate ScheduleMode = "immediate"
	ScheduleOnce      ScheduleMode = "once"

	OutputFiles   OutputFormat = "files"
	OutputMBTiles OutputFormat = "mbtiles"

	PackageZIP   PackageFormat = "zip"
	PackageTarGZ PackageFormat = "tar.gz"
)

type ScheduleMode string
type OutputFormat string
type PackageFormat string

type Request struct {
	Name     string         `json:"name"`
	Mode     area.Mode      `json:"mode"`
	Area     area.Spec      `json:"area"`
	Sources  []Source       `json:"sources"`
	Zoom     area.ZoomRange `json:"zoom"`
	Schedule Schedule       `json:"schedule"`
	Output   Output         `json:"output"`
}

type Source struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Layer  string `json:"layer,omitempty"`
	Format string `json:"format"`
	Schema string `json:"schema"`
}

type Schedule struct {
	Mode  ScheduleMode `json:"mode"`
	RunAt string       `json:"runAt,omitempty"`
}

type Output struct {
	Format  OutputFormat  `json:"format"`
	Package PackageFormat `json:"package"`
}

type TaskDefinition struct {
	Name     string
	Mode     area.Mode
	Area     area.Spec
	Sources  []Source
	Zoom     area.ZoomRange
	Schedule Schedule
	RunAt    *time.Time
	Output   Output
}

func NormalizeRequest(req Request) (*TaskDefinition, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errors.New("name is required")
	}
	if len(name) > 80 {
		return nil, errors.New("name must be 80 characters or fewer")
	}

	normalizedArea, err := area.Normalize(req.Area, req.Mode)
	if err != nil {
		return nil, err
	}
	if err := req.Zoom.Validate(); err != nil {
		return nil, err
	}

	sources, err := normalizeSources(req.Sources)
	if err != nil {
		return nil, err
	}
	schedule, runAt, err := normalizeSchedule(req.Schedule)
	if err != nil {
		return nil, err
	}
	output, err := normalizeOutput(req.Output)
	if err != nil {
		return nil, err
	}

	return &TaskDefinition{
		Name:     name,
		Mode:     normalizedArea.Mode,
		Area:     normalizedArea,
		Sources:  sources,
		Zoom:     req.Zoom,
		Schedule: schedule,
		RunAt:    runAt,
		Output:   output,
	}, nil
}

func normalizeSources(input []Source) ([]Source, error) {
	if len(input) == 0 {
		return nil, errors.New("at least one source is required")
	}
	sources := make([]Source, 0, len(input))
	for _, source := range input {
		source.Name = strings.TrimSpace(source.Name)
		source.URL = strings.TrimSpace(source.URL)
		source.Layer = strings.TrimSpace(source.Layer)
		source.Format = strings.ToLower(strings.TrimSpace(source.Format))
		source.Schema = strings.ToLower(strings.TrimSpace(source.Schema))
		if source.URL == "" {
			return nil, errors.New("source url is required")
		}
		if source.Name == "" {
			if source.Layer != "" {
				source.Name = source.Layer
			} else {
				source.Name = source.URL
			}
		}
		if !supportedFormat(source.Format) {
			return nil, errors.New("unsupported source format")
		}
		if source.Schema == "" {
			source.Schema = "xyz"
		}
		if source.Schema != "xyz" && source.Schema != "tms" {
			return nil, errors.New("source schema must be xyz or tms")
		}
		sources = append(sources, source)
	}
	return sources, nil
}

func normalizeSchedule(input Schedule) (Schedule, *time.Time, error) {
	input.Mode = ScheduleMode(strings.ToLower(strings.TrimSpace(string(input.Mode))))
	if input.Mode == "" {
		input.Mode = ScheduleImmediate
	}
	switch input.Mode {
	case ScheduleImmediate:
		input.RunAt = ""
		return input, nil, nil
	case ScheduleOnce:
		input.RunAt = strings.TrimSpace(input.RunAt)
		if input.RunAt == "" {
			return Schedule{}, nil, errors.New("runAt is required for scheduled tasks")
		}
		parsed, err := time.Parse(time.RFC3339, input.RunAt)
		if err != nil {
			return Schedule{}, nil, errors.New("runAt must be RFC3339 time")
		}
		return input, &parsed, nil
	default:
		return Schedule{}, nil, errors.New("unsupported schedule mode")
	}
}

func normalizeOutput(input Output) (Output, error) {
	input.Format = OutputFormat(strings.ToLower(strings.TrimSpace(string(input.Format))))
	if input.Format == "" {
		input.Format = OutputMBTiles
	}
	if input.Format != OutputFiles && input.Format != OutputMBTiles {
		return Output{}, errors.New("output format must be files or mbtiles")
	}
	input.Package = PackageFormat(strings.ToLower(strings.TrimSpace(string(input.Package))))
	if input.Package == "" {
		input.Package = PackageZIP
	}
	if input.Package != PackageZIP && input.Package != PackageTarGZ {
		return Output{}, errors.New("output package must be zip or tar.gz")
	}
	return input, nil
}

func supportedFormat(format string) bool {
	switch format {
	case "jpg", "png", "pbf", "webp":
		return true
	default:
		return false
	}
}
