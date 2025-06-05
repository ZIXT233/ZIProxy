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
	"net/url"
	"os"            // 用于文件系统操作
	"path/filepath" // 用于路径拼接
	"strconv"
	"strings"
	"time"
	"unicode"
)

var bdb *badger.DB
var ctx = context.Background()
var LRU *lru.Cache[string, any]
var cacheFileDir string // 新增：缓存文件存储目录

// 初始化 Badger DB 和文件缓存目录
func initHttpCacheDB(dir string, size int) error {
	var err error
	bdb, err = badger.Open(badger.DefaultOptions(dir))
	if err != nil {
		log.Fatal(err)
	}

	// 初始化文件缓存目录
	cacheFileDir = filepath.Join(dir, "file_cache")
	if err := os.MkdirAll(cacheFileDir, 0755); err != nil {
		log.Fatalf("创建文件缓存目录 %s 失败: %v", cacheFileDir, err)
	}
	log.Printf("HTTP 响应体将存储在: %s", cacheFileDir)

	LRU, _ = lru.NewWithEvict(size, BadgerDelete) // BadgerDelete 会处理文件删除

	err = bdb.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			// 注意：如果重启时需要确保文件存在，这里可能需要更复杂的逻辑
			// 但通常LRU加载key，如果后续get时文件丢失，会当做cache miss处理
			LRU.Add(string(k), nil) // LRU只存key，value通过BadgerDB获取
		}
		return nil
	})
	if err != nil {
		log.Printf("从BadgerDB加载LRU key失败: %v", err)
		// 根据实际需求决定是否 Fatal
	}
	log.Printf("加载了 %d 条HTTP代理缓存元数据索引", LRU.Len())
	return nil
}

// BadgerDelete 在条目被LRU淘汰时调用
func BadgerDelete(key string, value any) { // value 在这里是 nil
	log.Printf("HTTP缓存条目 %s 被淘汰", key)
	err := bdb.Update(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				log.Printf("尝试删除 %s 时，元数据已不在BadgerDB中", key)
				return nil // 可能已被其他方式删除
			}
			return fmt.Errorf("获取被淘汰条目 %s 元数据失败: %w", key, err)
		}

		var entry CacheEntry
		err = item.Value(func(val []byte) error {
			parsedEntry, parseErr := parseCacheEntry(val)
			if parseErr != nil {
				return fmt.Errorf("解析被淘汰条目 %s 元数据失败: %w", key, parseErr)
			}
			entry = *parsedEntry
			return nil
		})
		if err != nil {
			log.Printf("处理被淘汰条目 %s 失败: %v", key, err)
			// 即使无法解析元数据，也尝试删除key
			return txn.Delete([]byte(key))
		}

		// 删除对应的缓存文件
		if entry.BodyFilePath != "" {
			fullPath := filepath.Join(cacheFileDir, entry.BodyFilePath)
			if err := os.Remove(fullPath); err != nil {
				// 如果文件不存在，也认为是可以接受的
				if !os.IsNotExist(err) {
					log.Printf("删除缓存文件 %s 失败: %v", fullPath, err)
				} else {
					log.Printf("缓存文件 %s 已不存在，无需删除", fullPath)
				}
			} else {
				log.Printf("已删除缓存文件: %s", fullPath)
			}
		}
		// 从BadgerDB删除元数据
		return txn.Delete([]byte(key))
	})
	if err != nil {
		log.Printf("从BadgerDB删除元数据 %s 失败: %v", key, err)
	}
}

// ClearHTTPCache 清理所有缓存（BadgerDB 和 文件系统）
func ClearHTTPCache() error {
	LRU.Purge() // 这会触发所有条目的BadgerDelete，进而删除文件和BadgerDB条目

	// 作为保险，额外清理文件目录和BadgerDB
	log.Printf("正在清理文件缓存目录: %s", cacheFileDir)
	if err := os.RemoveAll(cacheFileDir); err != nil {
		log.Printf("清理文件缓存目录 %s 失败: %v", cacheFileDir, err)
		// 尝试重新创建目录，即使失败了也继续
		if mkErr := os.MkdirAll(cacheFileDir, 0755); mkErr != nil {
			log.Printf("重新创建文件缓存目录 %s 失败: %v", cacheFileDir, mkErr)
		}
		return fmt.Errorf("清理文件缓存目录失败: %w", err)
	}
	if err := os.MkdirAll(cacheFileDir, 0755); err != nil { // 清理后重新创建
		log.Fatalf("清理后重新创建文件缓存目录 %s 失败: %v", cacheFileDir, err)
	}

	log.Printf("正在DropAll BadgerDB数据")
	return bdb.DropAll() // DropAll会删除所有BadgerDB中的数据
}

// HttpCacheSet 设置键值对 (现在value是序列化后的CacheEntry元数据)
func HttpCacheSet(key string, value []byte) error {
	LRU.Add(key, nil) // LRU中value为nil，实际数据在BadgerDB
	log.Printf("添加HTTP缓存元数据 %s, 现在缓存数目为:%d", key, LRU.Len())
	return bdb.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	})
}

// HttpCacheGet 获取键对应的值 (返回序列化后的CacheEntry元数据)
func HttpCacheGet(key string) ([]byte, bool) {
	_, ok := LRU.Get(key) // 检查LRU中是否存在，并刷新其近用记录
	if !ok {
		return nil, false
	}
	var valCopy []byte
	err := bdb.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
		if err != nil {
			return err // 如果在BadgerDB找不到(可能刚被淘汰)，则返回错误
		}
		valCopy, err = item.ValueCopy(nil)
		return err
	})
	if err != nil {
		if err == badger.ErrKeyNotFound {
			log.Printf("LRU命中但BadgerDB未找到key %s, 可能已被淘汰", key)
			LRU.Remove(key) // 保持LRU和BadgerDB的一致性
			return nil, false
		}
		log.Printf("从BadgerDB获取key %s 失败: %v", key, err)
		return nil, false
	}
	return valCopy, true
}

func init() {
	gob.Register(CacheEntry{})
}

func generateCacheFilename(cacheKey string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, cacheKey)
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
	// Method, Host, URL, and Vary header values make up the key
	return fmt.Sprintf("%s:%s:%s:%s", req.Method, req.Host, req.URL.String(), varyVal)
}

func buildCacheKeyFromURL(url *url.URL) string {
	return fmt.Sprintf("GET:%s:%s:", url.Host, url.Path+"?"+url.RawQuery)
}

type CacheControl struct {
	Raw     string
	MaxAge  int
	NoStore bool
	NoCache bool
	Public  bool
	Private bool
}

// CacheEntry 结构更新
type CacheEntry struct {
	BodyFilePath string // 响应体存储在文件系统中的【文件名】(非完整路径)
	ETag         string
	LastMod      string
	ExpireTime   time.Time
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
			// 使用 Sscanf 解析 max-age
			var maxAgeVal int
			if n, err := fmt.Sscanf(part, "max-age=%d", &maxAgeVal); n == 1 && err == nil {
				cc.MaxAge = maxAgeVal
			}
		}
	}
	return cc
}

func isCachable(resp *http.Response, req *http.Request) bool {
	if req.Method != "GET" { // 通常只缓存GET请求
		return false
	}
	rangeByte := req.Header.Get("Range")
	if rangeByte != "" {
		rangeByte = rangeByte[strings.Index(rangeByte, "=")+1:]
		ranges := strings.Split(rangeByte, "-")
		if len(ranges) > 0 {
			begin, err := strconv.Atoi(ranges[0])
			if err != nil {
				log.Printf("解析Range头失败: %v, Range: %s", err, rangeByte)
				return false // 如果Range头格式不正确，认为不可缓存
			}
			if begin != 0 {
				return false
			}
		}
	}
	cc := parseCacheControl(resp.Header.Get("Cache-Control"))
	// 不缓存 no-store 和 private 的响应
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

// forwardRequest 保持不变
func forwardRequest(req *http.Request, serverConn io.ReadWriter) (*http.Response, error) {
	// Ensure the request body is correctly handled if it exists
	// For GET, HEAD, etc., req.Body is typically nil or http.NoBody.
	// For POST, PUT, etc., req.Write handles the body.
	err := req.Write(serverConn)
	if err != nil {
		log.Println("Error writing request to server:", err)
		return nil, err
	}
	return http.ReadResponse(bufio.NewReader(serverConn), req)
}

func calculateExpireTime(resp *http.Response) time.Time {
	cc := parseCacheControl(resp.Header.Get("Cache-Control"))
	if cc.MaxAge > 0 {
		return time.Now().Add(time.Duration(cc.MaxAge) * time.Second)
	}
	// 如果没有max-age，可以根据Expires头或者启发式缓存策略设置默认过期时间
	// 这里简化处理，如果没有max-age，则认为不久后过期或依赖其他验证机制
	// 对于实际应用，可能需要更完善的启发式缓存策略
	if expiresHeader := resp.Header.Get("Expires"); expiresHeader != "" {
		if expiresTime, err := http.ParseTime(expiresHeader); err == nil {
			return expiresTime
		}
	}
	// 默认给一个较短的过期时间，比如1分钟，如果没有其他指示
	// 或者依赖 ETag/Last-Modified 进行验证
	return time.Now().Add(60 * time.Second) // 示例：默认1分钟
}

// updateExpireTime 更新元数据中的过期时间，但不修改文件路径和头信息
func updateExpireTime(entry *CacheEntry, resp *http.Response) *CacheEntry {
	return &CacheEntry{
		BodyFilePath: entry.BodyFilePath,        // 保持不变
		ETag:         entry.ETag,                // ETag可能不变，或者由304响应确认
		LastMod:      entry.LastMod,             // LastMod可能不变
		ExpireTime:   calculateExpireTime(resp), // 更新过期时间
	}
}

// forwardResponseWithCacheUpdate: 将响应转发给客户端，同时流式写入缓存文件
func forwardResponseWithCacheUpdate(cacheKey string, resp *http.Response, clientConn io.Writer, serverConn io.ReadWriter) error {
	newEntry := CacheEntry{
		ETag:       resp.Header.Get("ETag"),
		LastMod:    resp.Header.Get("Last-Modified"),
		ExpireTime: calculateExpireTime(resp),
	}
	bodyFilename := generateCacheFilename(cacheKey) // 使用哈希作为文件名
	newEntry.BodyFilePath = bodyFilename
	fullBodyPath := filepath.Join(cacheFileDir, bodyFilename)

	file, err := os.Create(fullBodyPath)
	if err != nil {
		log.Printf("创建缓存文件 %s 失败: %v. 响应将只发给客户端，不缓存.", fullBodyPath, err)
		err = resp.Write(clientConn)
		if err != nil {
			log.Printf("写入响应到客户端失败 for %s: %v", cacheKey, err)
			return err
		}
		return nil // 已经尝试发送给客户端，错误已记录
	}
	defer file.Close() // 确保文件被关闭

	bodyAndFileSaver := io.MultiWriter(clientConn, file)
	twinWriter := bodyAndFileSaver

	err = resp.Write(twinWriter)
	if err != nil {
		log.Printf("流式写入响应体失败 for %s : %v", cacheKey, err)
		// 尝试关闭并删除部分写入的文件
		file.Close() // 关闭文件以便删除
		os.Remove(fullBodyPath)
		return fmt.Errorf("流式写入响应体时发生错误: %w", err)
	}
	log.Printf("成功流式写入字节到客户端和文件 %s", fullBodyPath)

	// 6. 序列化CacheEntry元数据并存入BadgerDB
	newEntryBytes, err := serializeCacheEntry(&newEntry)
	if err != nil {
		log.Printf("序列化CacheEntry失败 for %s: %v. 文件已保存但元数据未存.", cacheKey, err)
		// 虽然元数据保存失败，但文件已写。可以考虑删除文件保持一致性，或者保留文件待下次修复。
		// 这里选择删除文件，避免孤立的缓存文件。
		os.Remove(fullBodyPath)
		return err
	}
	go HttpCacheSet(cacheKey, newEntryBytes) // 异步存入BadgerDB

	return nil
}
func useResponseFromCacheFile(entry *CacheEntry, cacheKey string, clientConn io.Writer) (bool, error) {
	fullBodyPath := filepath.Join(cacheFileDir, entry.BodyFilePath)
	file, openErr := os.Open(fullBodyPath)
	if openErr != nil {
		log.Printf("打开缓存文件 %s 失败: %v. 尝试重新获取.", fullBodyPath, openErr)
		LRU.Remove(cacheKey) // 文件丢失，移除缓存记录
		return false, openErr
	} else {
		defer file.Close()
		if _, copyErr := io.Copy(clientConn, file); copyErr != nil {
			log.Printf("从缓存文件 %s 写入响应体失败: %v", fullBodyPath, copyErr)
			return false, copyErr
		} else {
			log.Printf("成功从文件 %s 提供缓存响应体", fullBodyPath)
			return true, nil
		}
	}
}

// httpCache 主处理函数
func httpCache(clientConn net.Conn, serverConn net.Conn) {
	defer clientConn.Close() // 确保客户端连接最终被关闭
	defer serverConn.Close() // 确保服务端连接最终被关闭

	reader := bufio.NewReader(clientConn)
	var cacheKey string

	for {
		req, err := http.ReadRequest(reader)
		if err == io.EOF {
			log.Println("客户端关闭连接 (EOF)")
			break
		}
		if err != nil {
			// 处理 net.ErrClosed 等错误，这些可能是keep-alive超时或客户端主动关闭
			if netErr, ok := err.(net.Error); ok && (netErr.Timeout() || strings.Contains(err.Error(), "use of closed network connection")) {
				log.Printf("读取请求时连接已关闭或超时: %v", err)
			} else {
				log.Printf("读取请求失败: %v", err)
			}
			break
		}

		// 设置一个合理的请求处理超时上下文
		reqCtx, cancelReq := context.WithTimeout(ctx, 60*time.Second) // 例如60秒超时
		defer cancelReq()
		req = req.WithContext(reqCtx)

		var resp *http.Response
		cacheKey = buildCacheKey(req)
		log.Printf("缓存搜索: %s %s (Key: %s)", req.Method, req.URL.String(), cacheKey)

		cacheValue, cacheHit := HttpCacheGet(cacheKey)
		responseFinished := false

		if req.Method == "GET" && cacheHit {
			log.Printf("元数据缓存命中: %s", req.URL)
			entry, parseErr := parseCacheEntry(cacheValue)
			if parseErr != nil {
				log.Printf("解析缓存元数据失败 for %s: %v", req.URL, parseErr)
				// 从LRU和BadgerDB中移除损坏的条目
				LRU.Remove(cacheKey) // 会触发BadgerDelete
			} else {
				reqcc := parseCacheControl(req.Header.Get("Cache-Control"))
				if isFresh(entry) && !reqcc.NoCache {
					log.Printf("*** 使用新鲜缓存: %s ***", req.URL)

					responseFinished, err = useResponseFromCacheFile(entry, cacheKey, clientConn)

				} else { // 缓存不新鲜或客户端要求验证 (no-cache)
					log.Printf("缓存存在但需要验证 (stale or no-cache): %s", req.URL)
					modreq := req.Clone(reqCtx) // 使用带超时的上下文
					if entry.ETag != "" {
						modreq.Header.Set("If-None-Match", entry.ETag)
					}
					if entry.LastMod != "" {
						// 注意：If-Modified-Since 通常用于 Last-Modified
						modreq.Header.Set("If-Modified-Since", entry.LastMod)
					}
					modreq.Body = http.NoBody // 验证请求不需要body

					// 确保 serverConn 是可用的，如果之前的请求是keep-alive且serverConn已关闭，这里会失败
					resp, err = forwardRequest(modreq, serverConn)
					if err != nil {
						log.Printf("转发缓存验证请求失败 for %s: %v", req.URL, err)
						// 如果验证失败，可以考虑服务旧缓存（stale-if-error），或报错
						// 这里简单处理，认为无法提供服务
						responseFinished = false // 让它走下面的非缓存逻辑（可能会失败）
					} else {
						defer resp.Body.Close()
						if resp.StatusCode == http.StatusNotModified {
							log.Printf("304 Not Modified，使用旧缓存 for %s", req.URL)
							responseFinished, err = useResponseFromCacheFile(entry, cacheKey, clientConn)
							if responseFinished { // 只有在成功服务后才更新TTL
								// 304 响应的 Cache-Control, Expires 等头可以用来更新原缓存条目的新鲜度
								newMetaEntry := updateExpireTime(entry, resp)
								newMetaBytes, serErr := serializeCacheEntry(newMetaEntry)
								if serErr != nil {
									log.Printf("序列化更新后的元数据失败 (304) for %s: %v", req.URL, serErr)
								} else {
									go HttpCacheSet(cacheKey, newMetaBytes)
								}
							}

						} else if resp.StatusCode == http.StatusOK {
							log.Printf("200 OK (验证后)，替换缓存内容 for %s", req.URL)
							if entry.BodyFilePath != "" {
								oldFullPath := filepath.Join(cacheFileDir, entry.BodyFilePath)
								if rmErr := os.Remove(oldFullPath); rmErr != nil && !os.IsNotExist(rmErr) {
									log.Printf("替换缓存前删除旧文件 %s 失败: %v", oldFullPath, rmErr)
								} else {
									log.Printf("已删除旧缓存文件 %s 以便更新", oldFullPath)
								}
							}

							err = forwardResponseWithCacheUpdate(cacheKey, resp, clientConn, serverConn)
							if err != nil {
								log.Printf("写入新缓存(200 OK after validation)失败 for %s: %v", req.URL, err)
								// 响应可能已部分发送，这里break
								break
							}
							responseFinished = true
						} else if resp.StatusCode == http.StatusFound { // 302 Found
							fullBodyPath := filepath.Join(cacheFileDir, entry.BodyFilePath)
							file, openErr := os.Open(fullBodyPath)
							if openErr != nil {
								log.Printf("打开缓存文件 %s 失败: %v. 尝试重新获取.", fullBodyPath, openErr)
								LRU.Remove(cacheKey) // 文件丢失，移除缓存记录
								responseFinished = false
							} else {
								defer file.Close()
								resp302, _ := http.ReadResponse(bufio.NewReader(file), nil)
								locUrl, _ := resp302.Location()
								cacheKey302 := buildCacheKeyFromURL(locUrl)
								_, hit302 := HttpCacheGet(cacheKey302)
								if hit302 {
									responseFinished, err = useResponseFromCacheFile(entry, cacheKey, clientConn)
								} else {
									responseFinished = false
								}
								if responseFinished {
									log.Printf("302 Found响应Location已缓存，使用缓存 for %s", cacheKey)
								} else {
									log.Printf("302 Found响应Location未缓存，尝试重新获取 for %s", cacheKey)
								}
							}
						} else {
							log.Printf("验证后状态码 %s, 不使用缓存 for %s", resp.Status, req.URL)
							responseFinished = false
						}
					}
				}
			}
		}

		if !responseFinished {
			log.Printf("缓存未使用或验证失败/未命中，从源获取: %s", req.URL)
			resp, err = forwardRequest(req, serverConn)
			if err != nil {
				log.Printf("转发原始请求失败 for %s: %v", req.URL, err)
				break
			}
			defer resp.Body.Close() // 确保在循环的这个迭代中resp.Body被关闭

			if isCachable(resp, req) {
				log.Printf("响应可缓存，进行缓存并转发: %s", req.URL)
				err = forwardResponseWithCacheUpdate(cacheKey, resp, clientConn, serverConn)
				if err != nil {
					log.Printf("缓存并转发响应失败 for %s: %v", req.URL, err)
					// 响应可能已部分发送或未发送，这里直接break，因为clientConn状态未知
					break
				}
			} else {
				log.Printf("响应不可缓存，直接转发: %s", req.URL)
				err = resp.Write(clientConn) // 直接将响应写给客户端
				if err != nil {
					log.Printf("直接转发响应失败 for %s: %v", req.URL, err)
					break
				}
			}
		}

		// 处理 Keep-Alive
		if req.Header.Get("Connection") == "close" {
			log.Printf("请求Connection: close，关闭此轮代理 for %s", req.Host)
			break
		}
		if resp != nil && resp.Header.Get("Connection") == "close" {
			log.Printf("响应Connection: close，关闭此轮代理 for %s", req.Host)
			break
		}
		// 如果是 HTTP/1.0 或 Connection: keep-alive 未明确，则默认短连接 (取决于server行为)
		// 为了简化，这里主要依赖明确的 "close" 指令。
		// 在一个更健壮的代理中，需要更仔细地处理连接持久性。
		log.Printf("请求 %s %s 处理完毕, 等待下一个请求 (Keep-Alive)", req.Method, req.URL.String())

	}
	log.Printf("结束与 %s 的HTTP代理会话", clientConn.RemoteAddr())
}
