package network

import "encoding/binary"

const (
	Magic      = uint32(0x53574954) // "SWIT"
	HeaderSize = 20                 // magic(4) + seq(8) + timestamp_ns(8)
)

type PacketHeader struct {
	Magic       uint32
	Seq         uint64
	TimestampNs int64
}

func EncodeHeader(buf []byte, h PacketHeader) {
	binary.BigEndian.PutUint32(buf[0:4], h.Magic)
	binary.BigEndian.PutUint64(buf[4:12], h.Seq)
	binary.BigEndian.PutUint64(buf[12:20], uint64(h.TimestampNs))
}

func DecodeHeader(buf []byte) (PacketHeader, bool) {
	if len(buf) < HeaderSize {
		return PacketHeader{}, false
	}
	m := binary.BigEndian.Uint32(buf[0:4])
	if m != Magic {
		return PacketHeader{}, false
	}
	return PacketHeader{
		Magic:       m,
		Seq:         binary.BigEndian.Uint64(buf[4:12]),
		TimestampNs: int64(binary.BigEndian.Uint64(buf[12:20])),
	}, true
}
