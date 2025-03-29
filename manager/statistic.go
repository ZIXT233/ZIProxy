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
		downloadCollect <- uint64(n)
	}
	return n, err
}

func (s *StatisticIO) Write(p []byte) (n int, err error) {
	n, err = s.stream.Write(p)
	if err == nil {
		s.BytesOut += uint64(n)
		uploadCollect <- uint64(n)
	}
	return n, err
}

func StatisticWrap(stream io.ReadWriter) *StatisticIO {
	return &StatisticIO{
		stream: stream,
	}
}

var (
	AddingDownload  uint64
	AddingUpload    uint64
	SumDownload     uint64
	SumUpload       uint64
	downloadCollect = make(chan uint64, 1024)
	uploadCollect   = make(chan uint64, 1024)
)

func LaunchRealTimeStatistic() {
	go func() {
		for {
			SumDownload = AddingDownload
			SumUpload = AddingUpload
			AddingDownload = 0
			AddingUpload = 0
			time.Sleep(1 * time.Second)
		}
	}()
	go func() {
		for {
			dybte := <-downloadCollect
			AddingDownload += dybte
		}
	}()
	go func() {
		for {
			ubyte := <-uploadCollect
			AddingUpload += ubyte
		}
	}()
}
func GetRealTimeTraffic() (uint64, uint64, uint64) {
	return SumDownload, SumUpload, SumDownload + SumDownload
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
