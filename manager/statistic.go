package manager

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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

func parseAndDecompressGZIP(buf []byte) ([]byte, error) {
	// Step 1: 创建 reader
	bufReader := bytes.NewReader(buf)
	reader := bufio.NewReader(bufReader)

	// Step 2: 解析 HTTP 响应头
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		log.Println("Failed to parse HTTP response:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// 打印响应头
	fmt.Println("HTTP Response Headers:")
	for k, v := range resp.Header {
		fmt.Printf("%s: %v\n", k, v)
	}

	// Step 3: 判断是否是 gzip 压缩
	contentEncoding := resp.Header.Get("Content-Encoding")
	if contentEncoding == "gzip" {
		// Step 4: 使用 gzip.NewReader 解压
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			log.Println("Failed to create gzip reader:", err)
			return nil, err
		}
		defer gz.Close()

		// Step 5: 读取解压后的内容
		decompressed, err := io.ReadAll(gz)
		if err != nil {
			log.Println("Failed to read decompressed data:", err)
			return nil, err
		}

		// 返回解压后的内容
		return decompressed, nil
	} else {
		// 如果不是 gzip，直接读取 body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Println("Failed to read plain body:", err)
			return nil, err
		}
		return body, nil
	}
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
