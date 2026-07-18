//go:build !windows

package main

func defaultConfigPath() string {
	return "/etc/adsbpro-feeder/config.env"
}

func defaultDataDir() string {
	return "/var/lib/adsbpro-feeder"
}
