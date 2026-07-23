package main

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/lmilojevicc/seshagy/internal/cli"
	appconfig "github.com/lmilojevicc/seshagy/internal/config"
	"github.com/lmilojevicc/seshagy/internal/logging"
	"github.com/lmilojevicc/seshagy/internal/sessionmgr"
)

type diagnosticsLogging struct {
	Level           string `json:"level"`
	Enabled         bool   `json:"enabled"`
	Destination     string `json:"destination"`
	PathRedacted    bool   `json:"path_redacted"`
	DirectoryExists bool   `json:"directory_exists"`
	FileExists      bool   `json:"file_exists"`
	FileType        string `json:"file_type"`
	SizeBytes       int64  `json:"size_bytes"`
	LatestPresent   bool   `json:"latest_present"`
}

type diagnosticsRuntime struct {
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	Backend   string `json:"backend"`
}

type diagnosticsGuidance struct {
	Upload              bool     `json:"upload"`
	ReviewBeforeSharing bool     `json:"review_before_sharing"`
	Steps               []string `json:"steps"`
}

type diagnosticsReport struct {
	Logging  diagnosticsLogging  `json:"logging"`
	Runtime  diagnosticsRuntime  `json:"runtime"`
	Guidance diagnosticsGuidance `json:"guidance"`
}

func runDiagnostics(args []string) error {
	rest, jsonOutput := stripJSONFlag(args)
	if len(rest) > 0 {
		return errors.New(joinUsage("diagnostics", "[--json]"))
	}
	cfg, err := appconfig.Load()
	if err != nil {
		return diagnosticsFailure(jsonOutput, err, logging.ClassifyError(err))
	}
	resolved, err := logging.Resolve(
		logging.Config{Level: cfg.Log.Level, File: cfg.Log.File},
		os.LookupEnv,
	)
	if err != nil {
		return diagnosticsFailure(jsonOutput, err, "invalid")
	}
	status, err := logging.Inspect(resolved)
	if err != nil {
		return diagnosticsFailure(jsonOutput, err, logging.ClassifyError(err))
	}
	destination := "default"
	if resolved.Explicit {
		destination = "explicit"
	}
	report := diagnosticsReport{
		Logging: diagnosticsLogging{
			Level: resolved.LevelName, Enabled: resolved.Enabled, Destination: destination,
			PathRedacted: true, DirectoryExists: status.DirectoryExists,
			FileExists: status.FileExists, FileType: status.FileType,
			SizeBytes: status.SizeBytes, LatestPresent: status.LatestPresent,
		},
		Runtime: diagnosticsRuntime{
			Version: version, GoVersion: runtime.Version(), OS: runtime.GOOS,
			Arch: runtime.GOARCH, Backend: string(sessionmgr.Detect().Kind()),
		},
		Guidance: diagnosticsGuidance{
			Upload: false, ReviewBeforeSharing: true,
			Steps: []string{"enable", "reproduce", "exit", "inspect", "attach", "disable"},
		},
	}
	if jsonOutput {
		return encodeSuccess(report)
	}
	cli.Infof("logging: %s (%s destination)", resolved.LevelName, destination)
	if resolved.Explicit {
		cli.Infof("log file: %s", resolved.File)
	} else {
		cli.Infof("log directory: %s", resolved.Directory)
		if status.LatestPresent {
			cli.Infof("latest log: %s", status.Latest)
		}
	}
	if status.FileExists {
		cli.Infof("log status: %s, %d bytes", status.FileType, status.SizeBytes)
	} else {
		cli.Info("log status: no log file found")
	}
	cli.Info(
		"workflow: set SESHAGY_LOG_LEVEL=debug, reproduce, exit, inspect the JSONL, attach only if comfortable, then disable and delete it",
	)
	cli.Info(
		"logs stay local and are never uploaded automatically; paths and log contents may be sensitive",
	)
	return nil
}

func diagnosticsFailure(jsonOutput bool, err error, class string) error {
	if !jsonOutput {
		return err
	}
	return fmt.Errorf("diagnostics failed (error class: %s)", class)
}
