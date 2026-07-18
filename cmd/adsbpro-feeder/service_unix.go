//go:build !windows

package main

import (
	"context"
	"errors"
)

func configureServiceLogging(string) error {
	return errors.New("the service command is available only on Windows")
}

func runPlatformService(func(context.Context) error) error {
	return errors.New("the service command is available only on Windows")
}
