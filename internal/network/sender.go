package network

import (
	"fmt"
	"net"
	"sync"
	"time"
)

type BandwidthResult struct {
	BandwidthKbps int
	Sent          int
	Received      int
	LossPercent   float64
	Passed        bool
}

type TestResult struct {
	SwitchPort int
	Results    []BandwidthResult
	MaxKbps    int
}

type ProgressUpdate struct {
	BandwidthKbps int
	Sent          int
	Received      int
}

type Sender struct {
	mirrorAddr string
	mirrorPort int
	listenPort int
	packetSize int
}

func NewSender(mirrorAddr string, mirrorPort, listenPort, packetSize int) *Sender {
	return &Sender{
		mirrorAddr: mirrorAddr,
		mirrorPort: mirrorPort,
		listenPort: listenPort,
		packetSize: packetSize,
	}
}

func (s *Sender) Ping() error {
	remoteAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", s.mirrorAddr, s.mirrorPort))
	if err != nil {
		return err
	}
	localAddr := &net.UDPAddr{Port: s.listenPort}
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return fmt.Errorf("listen :%d: %w", s.listenPort, err)
	}
	defer conn.Close()

	pkt := make([]byte, HeaderSize)
	EncodeHeader(pkt, PacketHeader{Magic: Magic, Seq: 1, TimestampNs: time.Now().UnixNano()})
	if _, err := conn.WriteToUDP(pkt, remoteAddr); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	buf := make([]byte, 65536)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return fmt.Errorf("no response from mirror (timeout): %w", err)
	}
	if _, ok := DecodeHeader(buf[:n]); !ok {
		return fmt.Errorf("invalid response from mirror")
	}
	return nil
}

// RunBandwidthLevel sends packets at the given kbps for durationSec seconds.
// It calls onProgress periodically with current stats.
// Returns when done. Not safe to call concurrently.
func (s *Sender) RunBandwidthLevel(
	kbps, durationSec int,
	lossThresholdPct float64,
	onProgress func(ProgressUpdate),
) (BandwidthResult, error) {
	remoteAddr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", s.mirrorAddr, s.mirrorPort))
	if err != nil {
		return BandwidthResult{}, fmt.Errorf("resolve: %w", err)
	}

	localAddr := &net.UDPAddr{Port: s.listenPort}
	conn, err := net.ListenUDP("udp4", localAddr)
	if err != nil {
		return BandwidthResult{}, fmt.Errorf("listen :%d: %w", s.listenPort, err)
	}

	pktSize := s.packetSize
	if pktSize < HeaderSize {
		pktSize = HeaderSize
	}

	bytesPerSec := float64(kbps) * 1000.0 / 8.0
	pktsPerSec := bytesPerSec / float64(pktSize)

	var (
		mu       sync.Mutex
		sentSeqs = make(map[uint64]struct{}, 4096)
		received int
	)

	stopRecv := make(chan struct{})
	recvDone := make(chan struct{})

	go func() {
		defer close(recvDone)
		buf := make([]byte, 65536)
		for {
			conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
			n, _, rerr := conn.ReadFromUDP(buf)
			if rerr != nil {
				select {
				case <-stopRecv:
					return
				default:
					continue
				}
			}
			h, ok := DecodeHeader(buf[:n])
			if !ok {
				continue
			}
			mu.Lock()
			if _, exists := sentSeqs[h.Seq]; exists {
				delete(sentSeqs, h.Seq)
				received++
			}
			mu.Unlock()
		}
	}()

	pkt := make([]byte, pktSize)
	var seq uint64
	startTime := time.Now()
	deadline := startTime.Add(time.Duration(durationSec) * time.Second)

	for time.Now().Before(deadline) {
		elapsed := time.Since(startTime).Seconds()
		targetPkts := int64(elapsed * pktsPerSec)

		for int64(seq) < targetPkts && time.Now().Before(deadline) {
			seq++
			EncodeHeader(pkt, PacketHeader{Magic: Magic, Seq: seq, TimestampNs: time.Now().UnixNano()})
			mu.Lock()
			sentSeqs[seq] = struct{}{}
			mu.Unlock()
			conn.WriteToUDP(pkt, remoteAddr) //nolint:errcheck
		}

		if onProgress != nil && seq%100 == 0 {
			mu.Lock()
			r := received
			mu.Unlock()
			onProgress(ProgressUpdate{BandwidthKbps: kbps, Sent: int(seq), Received: r})
		}

		// brief sleep to avoid 100% CPU when ahead of schedule
		sleepDur := time.Duration(float64(time.Second)/pktsPerSec*0.3)
		if sleepDur < 50*time.Microsecond {
			sleepDur = 50 * time.Microsecond
		}
		if sleepDur > time.Millisecond {
			sleepDur = time.Millisecond
		}
		time.Sleep(sleepDur)
	}

	// let echoes drain
	time.Sleep(600 * time.Millisecond)
	close(stopRecv)
	conn.Close()
	<-recvDone

	totalSent := int(seq)
	mu.Lock()
	totalReceived := received
	mu.Unlock()

	loss := 0.0
	if totalSent > 0 {
		loss = float64(totalSent-totalReceived) / float64(totalSent) * 100.0
	}

	return BandwidthResult{
		BandwidthKbps: kbps,
		Sent:          totalSent,
		Received:      totalReceived,
		LossPercent:   loss,
		Passed:        loss <= lossThresholdPct,
	}, nil
}
