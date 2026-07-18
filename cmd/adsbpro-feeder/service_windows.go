//go:build windows

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows/svc"
)

const windowsServiceName = "ADSBProFeeder"

var serviceLog *os.File

func configureServiceLogging(dataDir string) error {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return err
	}
	path := filepath.Join(dataDir, "feeder.log")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	serviceLog = file
	log.SetOutput(file)
	return nil
}

func runPlatformService(run func(context.Context) error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		return fmt.Errorf("service must be started by the Windows Service Control Manager")
	}
	defer func() {
		if serviceLog != nil {
			_ = serviceLog.Close()
		}
	}()
	return svc.Run(windowsServiceName, &windowsServiceHandler{run: run})
}

type windowsServiceHandler struct {
	run func(context.Context) error
}

func (handler *windowsServiceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- handler.run(ctx) }()
	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for {
		select {
		case request, ok := <-requests:
			if !ok {
				cancel()
				return false, 0
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- request.CurrentStatus
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				cancel()
				select {
				case err := <-done:
					if err != nil && err != context.Canceled {
						log.Printf("service stopped after error: %v", err)
					}
				case <-time.After(15 * time.Second):
					log.Printf("service shutdown timed out")
				}
				return false, 0
			}
		case err := <-done:
			if err != nil && err != context.Canceled {
				log.Printf("service failed: %v", err)
				return true, 1
			}
			return false, 0
		}
	}
}
