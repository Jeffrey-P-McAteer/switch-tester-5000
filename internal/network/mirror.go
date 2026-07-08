package network

import (
	"fmt"
	"net"
	"sync/atomic"
	"time"
)

type MirrorStats struct {
	PacketsReceived uint64
	PacketsEchoed   uint64
	BytesReceived   uint64
	StartTime       time.Time
}

type Mirror struct {
	port    int
	conn    *net.UDPConn
	running atomic.Bool

	received atomic.Uint64
	echoed   atomic.Uint64
	bytes    atomic.Uint64
	start    time.Time
}

func NewMirror(port int) *Mirror {
	return &Mirror{port: port}
}

func (m *Mirror) Start() error {
	addr := &net.UDPAddr{Port: m.port}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("listen udp4 :%d: %w", m.port, err)
	}
	m.conn = conn
	m.start = time.Now()
	m.running.Store(true)
	go m.loop()
	return nil
}

func (m *Mirror) Stop() {
	m.running.Store(false)
	if m.conn != nil {
		m.conn.Close()
	}
}

func (m *Mirror) Stats() MirrorStats {
	return MirrorStats{
		PacketsReceived: m.received.Load(),
		PacketsEchoed:   m.echoed.Load(),
		BytesReceived:   m.bytes.Load(),
		StartTime:       m.start,
	}
}

func (m *Mirror) loop() {
	buf := make([]byte, 65536)
	for m.running.Load() {
		m.conn.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
		n, addr, err := m.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		if _, ok := DecodeHeader(buf[:n]); !ok {
			continue
		}
		m.received.Add(1)
		m.bytes.Add(uint64(n))
		if _, werr := m.conn.WriteToUDP(buf[:n], addr); werr == nil {
			m.echoed.Add(1)
		}
	}
}
