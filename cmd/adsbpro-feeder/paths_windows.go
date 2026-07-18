//go:build windows

package main

import (
	"os"
	"path/filepath"
)

func windowsProgramData() string {
	if value := os.Getenv("ProgramData"); value != "" {
		return value
	}
	return `C:\ProgramData`
}

func defaultConfigPath() string {
	return filepath.Join(windowsProgramData(), "ADSBPro", "Feeder", "config.env")
}

func defaultDataDir() string {
	return filepath.Join(windowsProgramData(), "ADSBPro", "Feeder", "data")
}
