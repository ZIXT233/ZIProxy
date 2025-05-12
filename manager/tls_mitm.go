package manager

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/ZIXT233/ziproxy/utils"
	"log"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

var (
	// MITM CA 证书和私钥
	caCert     *x509.Certificate
	caPrivKey  crypto.Signer
	cacheMutex = &sync.RWMutex{}
	certCache  = make(map[string]stdtls.Certificate) // host -> certPath
)

func loadCA(caCertPath, caKeyPath string) error {
	certPEM, err := os.ReadFile(caCertPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(caKeyPath)
	if err != nil {
		return err
	}

	// 解析 CA 证书
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return errors.New("invalid certificate PEM")
	}
	caCert, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	// 解析 CA 私钥
	block, _ = pem.Decode(keyPEM)
	if block == nil {
		return errors.New("failed to decode PEM block for private key")
	}

	var key interface{}
	switch block.Type {
	case "RSA PRIVATE KEY":
		// PKCS#1 RSA 私钥
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
	case "EC PRIVATE KEY":
		// 原始 EC 私钥
		key, err = x509.ParseECPrivateKey(block.Bytes)
		if err != nil {
			return err
		}
	case "PRIVATE KEY":
		// PKCS#8 格式，兼容 RSA 和 ECDSA
		key, err = x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported key type: %s", block.Type)
	}

	// 确保私钥实现了 crypto.Signer 接口（可用于签发证书）
	signer, ok := key.(crypto.Signer)
	if !ok {
		return errors.New("private key does not implement crypto.Signer")
	}

	caPrivKey = signer
	return nil
}

func isTLSClientHello(conn *utils.PeekConn) (bool, error) {
	// Peek 前 5 字节：type(1), version(2), length(2)
	header, err := conn.Peek(5)
	if err != nil {
		return false, err
	}

	if header[0] != 0x16 { // 必须是 handshake 消息
		return false, nil
	}

	// 协议版本 >= TLS 1.0 （0x0301）
	// 可选判断，有些客户端仍用 0x0300（SSLv3）
	version := binary.BigEndian.Uint16(header[1:3])
	if version < 0x0300 {
		return false, nil
	}

	// 消息总长度（注意：这是整个 handshake 消息的长度）
	msgLen := binary.BigEndian.Uint16(header[3:5])

	// 整个 ClientHello 至少需要 39 字节以上（handshake 头 + client hello 固定字段）
	// 所以如果 total len <= 39，不合法
	if msgLen <= 39 {
		return false, nil
	}

	return true, nil
}
func InitTlsMITM(caCertPath, caKeyPath string) error {

	err := loadCA(caCertPath, caKeyPath)
	if err != nil {
		log.Printf("loadCA failed: %v", err)
		return err
	}

	return nil
}

func TLS_MITM_to_client(clientConn net.Conn) (net.Conn, bool, error) {
	var tlsConfig *stdtls.Config
	// 动态证书签发
	if caCert != nil && caPrivKey != nil {
		tlsConfig = &stdtls.Config{
			GetCertificate: func(clientHello *stdtls.ClientHelloInfo) (*stdtls.Certificate, error) {
				host := clientHello.ServerName
				if host == "" {
					return nil, errors.New("no SNI provided")
				}

				// 检查缓存是否已有证书
				cacheMutex.RLock()
				if cachedCert, ok := certCache[host]; ok {
					cacheMutex.RUnlock()
					log.Println("MITM使用缓存证书" + host)
					return &cachedCert, nil
				}
				cacheMutex.RUnlock()
				log.Println("MITM生成证书" + host)
				// 动态生成证书
				certPEM, keyPEM, err := generateCertForHost(host)
				if err != nil {
					return nil, err
				}

				// 将 PEM 转换为 tls.Certificate
				cert, err := stdtls.X509KeyPair(certPEM, keyPEM)
				if err != nil {
					return nil, err
				}

				// 缓存证书
				cacheMutex.Lock()
				certCache[host] = cert
				cacheMutex.Unlock()

				return &cert, nil
			},
		}
	} else {
		return nil, false, errors.New("no CA provided")
	}

	peekConn := utils.NewPeekConn(clientConn)
	isTLS, err := isTLSClientHello(peekConn)
	if err != nil {
		log.Printf("isTLSConnection Judge failed: %v", err)
		return peekConn, false, err
	}
	if !isTLS { //不是tls流量的话直接传输
		log.Print("mitm not tls")
		return peekConn, false, nil
	} else {
		// TLS 握手
		tlsConn := stdtls.Server(peekConn, tlsConfig)
		err = tlsConn.Handshake()
		if err != nil {
			log.Printf("tlsConn.Handshake failed: %v", err)
			return peekConn, false, err
		}
		return tlsConn, true, nil
	}
}
func TLS_MITM_to_server(serverConn net.Conn, sni string, isTLS bool) (net.Conn, error) {
	if !isTLS {
		return serverConn, nil
	}
	tlsConfig := &stdtls.Config{
		ServerName: sni,
	}
	tlsConn := stdtls.Client(serverConn, tlsConfig)
	err := tlsConn.Handshake()
	return tlsConn, err
}

// 根据域名生成证书并返回 PEM 数据
func generateCertForHost(host string) ([]byte, []byte, error) {
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}

	notBefore := time.Now()
	notAfter := notBefore.Add(24 * time.Hour)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"MITM Proxy"},
			CommonName:   host,
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{host},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, caCert, &privKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privKey)})
	return certPEM, keyPEM, nil
}
