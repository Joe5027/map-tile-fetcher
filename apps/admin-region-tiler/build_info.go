package main

import (
	"encoding/json"
	"runtime/debug"
	"strconv"
)

// Set together by the verified build entrypoint; ordinary go build uses VCS settings.
var buildVersion, buildCommit, buildTime, buildDirty string

type BuildInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	BuiltAt string `json:"builtAt"`
	Dirty   *bool  `json:"dirty"`
}

func currentBuildInfo() BuildInfo {
	result := BuildInfo{Version: buildVersion, Commit: buildCommit, BuiltAt: buildTime}
	dirty := buildDirty
	if result.Commit == "" {
		if metadata, ok := debug.ReadBuildInfo(); ok {
			for _, setting := range metadata.Settings {
				switch setting.Key {
				case "vcs.revision":
					result.Commit = setting.Value
				case "vcs.time":
					result.BuiltAt = setting.Value
				case "vcs.modified":
					dirty = setting.Value
				}
			}
		}
	}
	if value, err := strconv.ParseBool(dirty); err == nil {
		result.Dirty = &value
	}
	if result.Commit == "" {
		result.Commit = "unknown"
	}
	if result.BuiltAt == "" {
		result.BuiltAt = "unknown"
	}
	if result.Version == "" {
		result.Version = "unknown"
		if result.Commit != "unknown" {
			revision := result.Commit
			if len(revision) > 12 {
				revision = revision[:12]
			}
			result.Version = "dev+" + revision
			if result.Dirty != nil && *result.Dirty {
				result.Version += ".dirty"
			}
		}
	}
	return result
}

func buildInfoJSON() string {
	raw, _ := json.Marshal(currentBuildInfo())
	return string(raw)
}
