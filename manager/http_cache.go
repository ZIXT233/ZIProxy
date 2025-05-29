package manager

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"github.com/dgraph-io/badger/v4"
	"github.com/hashicorp/golang-lru/v2"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

var bdb *badger.DB
var ctx = context.Background()
var LRU *lru.Cache[string, any]

// 初始化 Badger DB
func initHttpCacheDB(dir string, size int) error {
	var err error
	bdb, err = badger.Open(badger.DefaultOptions(dir)) // 可选：替换 nil 为自定义日志
	if err != nil {
		log.Fatal(err)
	}
	LRU, _ = lru.NewWithEvict(size, BadgerDelete)

	err = bdb.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			LRU.Add(string(k), nil)
		}

		return nil
	})
	log.Printf("加载了%d条HTTP代理缓存", LRU.Len())
	return nil
}

func BadgerDelete(key string, nothing any) {
	log.Printf("HTTP缓存条目%s被淘汰", key)
	bdb.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

func ClearHTTPCache() error {
	LRU.Purge()
	return bdb.DropAll()
}

// 设置键值对
func HttpCacheSet(key string, value []byte) error {
	LRU.Add(key, nil)
	log.Printf("添加HTTP缓存条目%s, 现在缓存数目为:%d", key, LRU.Len())
	return bdb.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

func HttpCacheGet(key string) ([]byte, bool) {
	_, ok := LRU.Get(key)
	if !ok {
		return nil, false
	}
	var valCopy []byte
	err := bdb.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err
		}
		valCopy, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		return nil, false
	}
	return valCopy, true
}
func init() {
	gob.Register(CacheEntry{})
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
	return fmt.Sprintf("%s:%s:%s:%s", req.Method, req.Host, req.URL.String(), varyVal)
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
	var reqBuf bytes.Buffer
	twinWriter := io.MultiWriter(serverConn, &reqBuf)
	err := req.Write(twinWriter)
	//log.Println(string(reqBuf.Bytes()))
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
	go HttpCacheSet(cacheKey, newEntryBytes)
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
		//由请求头信息生成cacheKey
		cacheKey = buildCacheKey(req)
		log.Printf("Cache Search " + req.Method + " " + req.URL.String())
		//从Badger数据库中查询缓存
		cacheValue, cacheHit := HttpCacheGet(cacheKey)
		responseFinished := false

		if req.Method == "GET" && cacheHit {
			//如果数据库中有响应缓存内容
			log.Println("Serving from cache:", req.URL)
			entry, err := parseCacheEntry(cacheValue)
			if err != nil {
				log.Println("Error parsing cache entry:", err)
			} else {
				//根据请求头中的Cache-Contronl字段控制缓存下一步行为
				reqcc := parseCacheControl(req.Header.Get("Cache-Control"))
				if isFresh(entry) && !reqcc.NoCache {
					//如果缓存足够新鲜且请求头不要求强制验证缓存内容
					_, err := clientConn.Write(entry.RespBytes)
					if err != nil {
						log.Println("Error writing to cache:", err)
					} else {
						log.Println("*** Use cache " + req.URL.String() + " ***") // + string(entry.RespBytes))
						responseFinished = true
					}
				} else {
					//如果需要验证缓存内容，构造缓存验证请求头
					modreq := req.Clone(context.Background())
					if entry.ETag != "" {
						modreq.Header.Set("If-None-Match", entry.ETag)
					}
					if entry.LastMod != "" {
						modreq.Header.Set("Last-Modified", entry.LastMod)
					}
					modreq.Body = http.NoBody
					//发送验证请求头到目标网站
					resp, err = forwardRequest(modreq, serverConn)
					if err != nil {
						log.Println("Error forwarding cache verify request:", err)
					} else {
						defer resp.Body.Close()
						if resp.StatusCode == http.StatusNotModified {
							// 如果响应报文为304状态，无需更新缓存内容，延长缓存内容生命周期
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
								go HttpCacheSet(cacheKey, newEntryBytes)
							}

						} else if resp.StatusCode == http.StatusOK {
							// 如果响应报文为200状态，更新缓存内容
							log.Println("200 OK, replacing cache")

							err = forwardResponseWithCacheUpdate(cacheKey, resp, clientConn)
							if err != nil {
								log.Println("Error writing to cache:", err)
							} else {
								responseFinished = true
							}

						} else {
							//未预期状态，不使用缓存
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

		if resp != nil && resp.Header.Get("Connection") == "close" {
			break
		}
	}
}
