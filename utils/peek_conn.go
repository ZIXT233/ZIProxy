package utils

import (
	"bytes"
	"net"
	"time"
)

// PeekConn 是一个 net.Conn 的封装，支持在不破坏流的情况下 Peek 数据
type PeekConn struct {
	conn   net.Conn
	buf    *bytes.Buffer
	peeked bool
}

func NewPeekConn(conn net.Conn) *PeekConn {
	return &PeekConn{
		conn: conn,
		buf:  bytes.NewBuffer(nil),
	}
}

func (c *PeekConn) SetPeekedBuf(buf *bytes.Buffer) {
	c.buf = buf
}

// Peek 窥探前面的数据（不会移动读指针）
func (c *PeekConn) Peek(n int) ([]byte, error) {
	if c.peeked {
		return c.buf.Bytes(), nil
	}

	// 从底层连接中读取最多 n 字节
	buf := make([]byte, n)
	numRead, err := c.conn.Read(buf)
	if numRead > 0 {
		c.buf.Write(buf[:numRead])
		// 把没读的部分放回去
		if numRead < n {
			buf = buf[:numRead]
		}
	}

	c.peeked = true
	return c.buf.Bytes(), err
}

// Read 实现 net.Conn 接口的 Read 方法
func (c *PeekConn) Read(b []byte) (n int, err error) {
	// 先读取缓冲中的数据
	if c.buf.Len() > 0 {
		return c.buf.Read(b)
	}
	return c.conn.Read(b)
}

// Write 实现 net.Conn 接口的 Write 方法
func (c *PeekConn) Write(b []byte) (n int, err error) {
	return c.conn.Write(b)
}

// Close 关闭连接
func (c *PeekConn) Close() error {
	return c.conn.Close()
}

// LocalAddr 返回本地网络地址
func (c *PeekConn) LocalAddr() net.Addr {
	return c.conn.LocalAddr()
}

// RemoteAddr 返回远程网络地址
func (c *PeekConn) RemoteAddr() net.Addr {
	return c.conn.RemoteAddr()
}

// SetDeadline 设置截止时间
func (c *PeekConn) SetDeadline(t time.Time) error {
	return c.conn.SetDeadline(t)
}

// SetReadDeadline 设置读操作的截止时间
func (c *PeekConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写操作的截止时间
func (c *PeekConn) SetWriteDeadline(t time.Time) error {
	return c.conn.SetWriteDeadline(t)
}
