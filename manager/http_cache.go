package manager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var rdb *redis.Client
var ctx = context.Background()
var rdbMu sync.Mutex

func init() {
	gob.Register(CacheEntry{})
}
func initRedis(addr, passwd string, db int) {
	rdb = redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: passwd, // no password
		DB:       db,     // default DB
	})
}
func testRedis() error {
	_, err := rdb.Ping(ctx).Result()
	return err
}
func buildCacheKey(req *http.Request) string {
	vary := req.Header.Get("Vary")
	var varyVal string
	if vary == "*" {
		varyVal = "vary-*"
	} else {
		var parts []string
		for _, h := range strings.Split(vary, ",") {
			h = strings.TrimSpace(h)
			parts = append(parts, req.Header.Get(h))
		}
		varyVal = strings.Join(parts, "|")
	}
	return fmt.Sprintf("cache:%s:%s:%s:%s", req.Method, req.Host, req.URL.String(), varyVal)
}

type CacheControl struct {
	Raw     string
	MaxAge  int
	NoStore bool
	NoCache bool
	Public  bool
	Private bool
}

type CacheEntry struct {
	RespBytes  []byte    // 完整的 HTTP 响应字节流（header + body）
	ETag       string    // 资源标识符
	LastMod    string    // 最后修改时间（备用）
	ExpireTime time.Time // 缓存过期时间
}

func parseCacheControl(s string) CacheControl {
	cc := CacheControl{Raw: s}
	if s == "" {
		return cc
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "no-store" {
			cc.NoStore = true
		} else if part == "no-cache" {
			cc.NoCache = true
		} else if part == "public" {
			cc.Public = true
		} else if part == "private" {
			cc.Private = true
		} else if strings.HasPrefix(part, "max-age=") {
			fmt.Sscanf(part, "max-age=%d", &cc.MaxAge)
		}
	}
	return cc
}
func setToRedis(key string, respBytes []byte, maxAge int) {
	if maxAge > 0 {
		rdb.Set(ctx, key, respBytes, time.Duration(maxAge)*time.Second)
	} else {
		rdb.Set(ctx, key, respBytes, 0)
	}
}

func getFromRedis(key string) ([]byte, bool) {
	val, err := rdb.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return val, true
}
func isCachable(resp *http.Response, req *http.Request) bool {
	if req.Method != "GET" {
		return false
	}
	cc := parseCacheControl(resp.Header.Get("Cache-Control"))
	return !cc.NoStore && !strings.Contains(cc.Raw, "private")
}
func serializeCacheEntry(entry *CacheEntry) ([]byte, error) {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)

	err := enc.Encode(entry)
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
func parseCacheEntry(data []byte) (*CacheEntry, error) {
	buf := bytes.NewBuffer(data)
	dec := gob.NewDecoder(buf)

	var entry CacheEntry
	err := dec.Decode(&entry)
	if err != nil {
		return nil, err
	}

	return &entry, nil
}
func isFresh(entry *CacheEntry) bool {
	return time.Now().Before(entry.ExpireTime)
}

func forwardRequest(req *http.Request, serverConn io.ReadWriter) (*http.Response, error) {
	err := req.Write(serverConn)
	if err != nil {
		log.Println("Error writing to server:", err)
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(serverConn), req)
}
func calculateExpireTime(resp *http.Response) time.Time {
	cc := parseCacheControl(resp.Header.Get("Cache-Control"))
	return time.Now().Add(time.Duration(cc.MaxAge) * time.Second)
}
func updateExpireTime(entry *CacheEntry, resp *http.Response) *CacheEntry {
	return &CacheEntry{
		RespBytes:  entry.RespBytes,
		ETag:       entry.ETag,
		LastMod:    entry.LastMod,
		ExpireTime: calculateExpireTime(resp),
	}
}

func calculateCacheTTL(expire time.Time) int {
	ttl := expire.Sub(time.Now())
	if ttl < 0 {
		return 0
	}
	return int(ttl.Seconds()) + 3600 //比expiretime多
}

func forwardResponseWithCacheUpdate(cacheKey string, resp *http.Response, clientConn io.ReadWriter) error {
	newEntry := CacheEntry{
		ETag:       resp.Header.Get("ETag"),
		LastMod:    resp.Header.Get("Last-Modified"),
		ExpireTime: calculateExpireTime(resp),
	}

	var respBuf bytes.Buffer
	twinWriter := io.MultiWriter(clientConn, &respBuf)
	resp.Write(twinWriter)
	newEntry.RespBytes = respBuf.Bytes()

	newEntryBytes, err := serializeCacheEntry(&newEntry)
	if err != nil {
		log.Println("Error writing to server:", err)
		return err
	}
	go setToRedis(cacheKey, newEntryBytes, calculateCacheTTL(newEntry.ExpireTime))
	return nil
}
func httpCache(clientConn, serverConn net.Conn) {

	reader := bufio.NewReader(clientConn)

	var cacheKey string

	//应对keepalive，采用循环停等
	for {
		req, err := http.ReadRequest(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Println("Error reading request:", err)
			break
		}
		var resp *http.Response
		cacheKey = buildCacheKey(req)
		log.Printf("Cache Search " + req.Method + " " + req.URL.String())
		cacheValue, cacheHit := getFromRedis(cacheKey)
		responseFinished := false

		if req.Method == "GET" && cacheHit {
			log.Println("Serving from cache:", req.URL)
			entry, err := parseCacheEntry(cacheValue)
			if err != nil {
				log.Println("Error parsing cache entry:", err)
			} else {
				reqcc := parseCacheControl(req.Header.Get("Cache-Control"))
				if isFresh(entry) && !reqcc.NoCache {

					_, err := clientConn.Write(entry.RespBytes)
					if err != nil {
						log.Println("Error writing to cache:", err)
					} else {
						log.Println("***Use cache " + req.URL.String() + "***") // + string(entry.RespBytes))
						responseFinished = true
					}
				} else {
					modreq := req.Clone(context.Background())
					if entry.ETag != "" {
						modreq.Header.Set("If-None-Match", entry.ETag)
					}
					if entry.LastMod != "" {
						modreq.Header.Set("Last-Modified", entry.LastMod)
					}
					modreq.Body = http.NoBody
					resp, err = forwardRequest(modreq, serverConn)
					if err != nil {
						log.Println("Error forwarding cache verify request:", err)
					} else {
						defer resp.Body.Close()
						if resp.StatusCode == http.StatusNotModified {
							_, err = clientConn.Write(entry.RespBytes)
							if err != nil {
								log.Println("Error writing to cache:", err)
							} else {
								responseFinished = true
							}
							log.Println("304 Not Modified, refreshing cache TTL " + req.URL.String()) // + string(entry.RespBytes))
							newEntry := updateExpireTime(entry, resp)
							newEntryBytes, err := serializeCacheEntry(newEntry)
							if err != nil {
								log.Println("Error serializing cache entry:", err)
							} else {
								go setToRedis(cacheKey, newEntryBytes, calculateCacheTTL(newEntry.ExpireTime))
							}

						} else if resp.StatusCode == http.StatusOK {
							log.Println("200 OK, replacing cache")

							err = forwardResponseWithCacheUpdate(cacheKey, resp, clientConn)
							if err != nil {
								log.Println("Error writing to cache:", err)
							} else {
								responseFinished = true
							}

						} else {
							log.Printf("No use cache, because %s at %s", resp.Status, req.URL.String())

						}

					}

				}
			}
		}
		if !responseFinished {
			log.Printf("Cache no use, %s", req.URL.String())
			resp, err = forwardRequest(req, serverConn)
			if err != nil {
				log.Println("Error reading response:", err)
				break
			}
			defer resp.Body.Close()
			if !isCachable(resp, req) {
				err = resp.Write(clientConn)
				if err != nil {
					log.Println("Error writing to client:", err)
					break
				}
			} else {
				err = forwardResponseWithCacheUpdate(cacheKey, resp, clientConn)
				if err != nil {
					log.Println("Error writing to client:", err)
					break
				}
			}
		}

		if resp == nil || resp.Header.Get("Connection") == "close" {
			break
		}
	}
}
