package manager

import (
	"net"
	"time"

	"github.com/ZIXT233/ziproxy/db"
)

type StatisticIO struct {
	BytesIn  uint64
	BytesOut uint64
	net.Conn
}

func (s *StatisticIO) Read(p []byte) (n int, err error) {
	n, err = s.Conn.Read(p)
	if err == nil {
		s.BytesIn += uint64(n)
		downloadCollect <- uint64(n)
	}

	return n, err
}

func (s *StatisticIO) Write(p []byte) (n int, err error) {
	n, err = s.Conn.Write(p)
	if err == nil {
		s.BytesOut += uint64(n)
		uploadCollect <- uint64(n)
	}
	return n, err
}

func StatisticWrap(stream net.Conn) *StatisticIO {
	return &StatisticIO{
		Conn: stream,
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
func (s *StatisticIO) AddToDB(inboundID, outboundID, userID, destAddr string, tm time.Time) {
	StatisticDBM.Traffic.Create(&db.Traffic{
		InboundID:  inboundID,
		OutboundID: outboundID,
		UserID:     userID,
		BytesIn:    s.BytesIn,
		BytesOut:   s.BytesOut,
		Time:       tm,
		DestAddr:   destAddr,
	})
}
