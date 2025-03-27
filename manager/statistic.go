package manager

import (
	"io"
	"time"

	"github.com/ZIXT233/ziproxy/db"
)

type StatisticIO struct {
	BytesIn  uint64
	BytesOut uint64
	stream   io.ReadWriter
}

func (s *StatisticIO) Read(p []byte) (n int, err error) {
	n, err = s.stream.Read(p)
	if err == nil {
		s.BytesIn += uint64(n)
	}
	return n, err
}

func (s *StatisticIO) Write(p []byte) (n int, err error) {
	n, err = s.stream.Write(p)
	if err == nil {
		s.BytesOut += uint64(n)
	}
	return n, err
}

func StatisticWrap(stream io.ReadWriter) *StatisticIO {
	return &StatisticIO{
		stream: stream,
	}
}

func (s *StatisticIO) AddToDB(inboundID, outboundID, userID, destAddr string) {
	StatisticDBM.Traffic.Create(&db.Traffic{
		InboundID:  inboundID,
		OutboundID: outboundID,
		UserID:     userID,
		BytesIn:    s.BytesIn,
		BytesOut:   s.BytesOut,
		Time:       time.Now().Truncate(time.Second),
		DestAddr:   destAddr,
	})
}
