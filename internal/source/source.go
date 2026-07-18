package source

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"time"
)

type Config struct {
	Host      string
	Mode      string
	BeastPort int
	SBSPort   int
}

type Connection struct {
	net.Conn
	Format     string
	Prefetched []byte
}

func Connect(config Config) (*Connection, error) {
	if config.Mode == "auto" || config.Mode == "beast" {
		connection, err := probe(config.Host, config.BeastPort, "beast")
		if err == nil {
			return connection, nil
		}
		if config.Mode == "beast" {
			return nil, err
		}
	}
	if config.Mode == "auto" || config.Mode == "sbs" {
		return probe(config.Host, config.SBSPort, "sbs")
	}
	return nil, errors.New("source mode must be auto, beast or sbs")
}

func probe(host string, port int, format string) (*Connection, error) {
	connection, err := net.DialTimeout("tcp", net.JoinHostPort(host, fmt.Sprint(port)), 5*time.Second)
	if err != nil {
		return nil, err
	}
	_ = connection.SetReadDeadline(time.Now().Add(5 * time.Second))
	buffer := make([]byte, 8192)
	used := 0
	for used < len(buffer) {
		read, readErr := connection.Read(buffer[used:])
		used += read
		if start := validStart(buffer[:used], format); start >= 0 {
			_ = connection.SetReadDeadline(time.Time{})
			return &Connection{Conn: connection, Format: format, Prefetched: append([]byte(nil), buffer[start:used]...)}, nil
		}
		if readErr != nil {
			connection.Close()
			return nil, readErr
		}
	}
	connection.Close()
	return nil, errors.New("source did not produce a valid ADS-B frame")
}

func validStart(value []byte, format string) int {
	if format == "sbs" {
		return bytes.Index(value, []byte("MSG,"))
	}
	for index := 0; index+1 < len(value); index++ {
		if value[index] == 0x1a && value[index+1] >= 0x31 && value[index+1] <= 0x34 {
			return index
		}
	}
	return -1
}
