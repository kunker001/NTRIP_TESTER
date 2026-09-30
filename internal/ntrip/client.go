package ntrip

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http/httputil"
	"strconv"
	"strings"
	"sync"
	"time"
)

const userAgent = "NTRIP NtripTester/1.0"

var ErrUnauthorized = errors.New("认证失败 (401)：请检查用户名和密码")

type Config struct {
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	TLS           bool    `json:"tls"`
	SelfSign      bool    `json:"selfSign"`
	Version       string  `json:"version"`
	Username      string  `json:"username"`
	Password      string  `json:"password"`
	Mountpoint    string  `json:"mountpoint"`
	GGAInterval   int     `json:"ggaInterval"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
	Alt           float64 `json:"alt"`
	Sep           float64 `json:"sep"`
	AutoReconnect bool    `json:"autoReconnect"`
	Timeout       int     `json:"timeout"`
}

func (c Config) HasPosition() bool { return c.Lat != 0 || c.Lon != 0 }

func (c *Config) Normalize() {
	h := strings.TrimSpace(c.Host)
	lower := strings.ToLower(h)
	if strings.HasPrefix(lower, "https://") {
		c.TLS = true
		h = h[len("https://"):]
	} else if strings.HasPrefix(lower, "http://") {
		h = h[len("http://"):]
	}
	c.Host = strings.TrimRight(h, "/")
	if c.Port <= 0 {
		c.Port = 2101
	}
	if c.Version != "1.0" {
		c.Version = "2.0"
	}
	c.Mountpoint = strings.TrimPrefix(strings.TrimSpace(c.Mountpoint), "/")
	if c.Timeout <= 0 {
		c.Timeout = 30
	}
}

func dial(ctx context.Context, cfg Config) (net.Conn, error) {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if !cfg.TLS {
		return d.DialContext(ctx, "tcp", addr)
	}
	td := &tls.Dialer{NetDialer: d, Config: &tls.Config{ServerName: cfg.Host, InsecureSkipVerify: cfg.SelfSign}}
	return td.DialContext(ctx, "tcp", addr)
}

func buildRequest(cfg Config, mount string) string {
	var b strings.Builder
	host := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	if cfg.Version == "1.0" {
		fmt.Fprintf(&b, "GET /%s HTTP/1.0\r\n", mount)
		fmt.Fprintf(&b, "Host: %s\r\n", host)
	} else {
		fmt.Fprintf(&b, "GET /%s HTTP/1.1\r\n", mount)
		fmt.Fprintf(&b, "Host: %s\r\n", host)
		b.WriteString("Ntrip-Version: Ntrip/2.0\r\n")
	}
	fmt.Fprintf(&b, "User-Agent: %s\r\n", userAgent)
	if cfg.Username != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(cfg.Username + ":" + cfg.Password))
		fmt.Fprintf(&b, "Authorization: Basic %s\r\n", auth)
	}
	b.WriteString("Accept: */*\r\n")
	b.WriteString("Connection: close\r\n\r\n")
	return b.String()
}

type respKind int

const (
	kindStream respKind = iota
	kindSourceTable
)

type response struct {
	kind   respKind
	status string
	body   io.Reader
}

func readResponse(br *bufio.Reader) (*response, error) {
	status, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	status = strings.TrimSpace(status)
	switch {
	case strings.HasPrefix(status, "ICY 200"):
		skipICYHeaders(br)
		return &response{kind: kindStream, status: status, body: br}, nil
	case strings.HasPrefix(status, "SOURCETABLE 200"):
		if _, err := readHeaders(br); err != nil {
			return nil, err
		}
		return &response{kind: kindSourceTable, status: status, body: br}, nil
	case strings.HasPrefix(status, "HTTP/"):
		parts := strings.SplitN(status, " ", 3)
		code := 0
		if len(parts) >= 2 {
			code, _ = strconv.Atoi(parts[1])
		}
		hdr, err := readHeaders(br)
		if err != nil {
			return nil, err
		}
		var body io.Reader = br
		if strings.Contains(strings.ToLower(hdr["transfer-encoding"]), "chunked") {
			body = httputil.NewChunkedReader(br)
		}
		switch code {
		case 200:
			kind := kindStream
			if strings.Contains(strings.ToLower(hdr["content-type"]), "sourcetable") {
				kind = kindSourceTable
			}
			return &response{kind: kind, status: status, body: body}, nil
		case 401:
			return nil, ErrUnauthorized
		default:
			snippet, _ := io.ReadAll(io.LimitReader(body, 512))
			if msg := strings.TrimSpace(string(snippet)); msg != "" {
				return nil, fmt.Errorf("服务器返回 %s: %s", status, msg)
			}
			return nil, fmt.Errorf("服务器返回 %s", status)
		}
	case strings.HasPrefix(status, "ERROR"):
		return nil, fmt.Errorf("服务器返回错误: %s", status)
	}
	if len(status) > 120 {
		status = status[:120]
	}
	return nil, fmt.Errorf("无法识别的响应: %q", status)
}

func readHeaders(br *bufio.Reader) (map[string]string, error) {
	h := map[string]string{}
	for i := 0; i < 100; i++ {
		line, err := br.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
				return h, nil
			}
			return h, fmt.Errorf("读取响应头失败: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			return h, nil
		}
		if k, v, ok := strings.Cut(line, ":"); ok {
			h[strings.ToLower(strings.TrimSpace(k))] = strings.TrimSpace(v)
		}
	}
	return h, nil
}

// skipICYHeaders consumes optional header lines after "ICY 200 OK" without
// swallowing binary stream data.
func skipICYHeaders(br *bufio.Reader) {
	for {
		b, err := br.Peek(1)
		if err != nil {
			return
		}
		switch {
		case b[0] == '\n':
			br.Discard(1)
			return
		case b[0] == '\r':
			if bb, err := br.Peek(2); err == nil && bb[1] == '\n' {
				br.Discard(2)
			} else {
				br.Discard(1)
			}
			return
		case !((b[0] >= 'A' && b[0] <= 'Z') || (b[0] >= 'a' && b[0] <= 'z')):
			return
		}
		win, _ := br.Peek(br.Buffered())
		idx := bytes.IndexByte(win, '\n')
		if idx < 0 {
			return
		}
		line := win[:idx]
		if bytes.IndexByte(line, ':') < 0 {
			return
		}
		for _, c := range line {
			if (c < 0x20 || c > 0x7e) && c != '\r' {
				return
			}
		}
		br.Discard(idx + 1)
	}
}

func watchCtx(ctx context.Context, c net.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// FetchSourceTable requests the caster sourcetable (GET /).
func FetchSourceTable(ctx context.Context, cfg Config) (*SourceTable, error) {
	cfg.Normalize()
	if cfg.Host == "" {
		return nil, errors.New("请填写 Caster 地址")
	}
	conn, err := dial(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("连接 %s:%d 失败: %w", cfg.Host, cfg.Port, err)
	}
	defer conn.Close()
	stop := watchCtx(ctx, conn)
	defer stop()

	conn.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := io.WriteString(conn, buildRequest(cfg, "")); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}
	br := bufio.NewReaderSize(conn, 64*1024)
	resp, err := readResponse(br)
	if err != nil {
		return nil, err
	}
	raw, err := readSourceTableBody(resp.body)
	if err != nil {
		return nil, fmt.Errorf("读取源列表失败: %w", err)
	}
	st := ParseSourceTable(raw)
	if len(st.Mounts) == 0 && !strings.Contains(raw, "ENDSOURCETABLE") {
		snippet := strings.TrimSpace(raw)
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}
		return nil, fmt.Errorf("未能解析源列表（响应: %s）: %q", resp.status, snippet)
	}
	return st, nil
}

func readSourceTableBody(r io.Reader) (string, error) {
	var sb strings.Builder
	br := bufio.NewReader(r)
	for sb.Len() < 8<<20 {
		line, err := br.ReadString('\n')
		sb.WriteString(line)
		if strings.HasPrefix(strings.TrimSpace(line), "ENDSOURCETABLE") {
			break
		}
		if err != nil {
			if errors.Is(err, io.EOF) || sb.Len() > 0 {
				break
			}
			return "", err
		}
	}
	return sb.String(), nil
}

type Handler struct {
	OnData  func([]byte)
	OnState func(state, msg string)
	OnGGA   func(sentence string)
}

// Client streams correction data from one mountpoint, optionally reconnecting.
type Client struct {
	cfg Config
	h   Handler
}

func NewClient(cfg Config, h Handler) *Client {
	cfg.Normalize()
	return &Client{cfg: cfg, h: h}
}

const reconnectDelay = 5 * time.Second

func (c *Client) Run(ctx context.Context) {
	attempt := 0
	for {
		err := c.session(ctx)
		if ctx.Err() != nil {
			c.h.OnState("idle", "已断开")
			return
		}
		msg := "连接已结束"
		if err != nil {
			msg = err.Error()
		}
		if !c.cfg.AutoReconnect || errors.Is(err, ErrUnauthorized) {
			c.h.OnState("error", msg)
			return
		}
		attempt++
		c.h.OnState("reconnecting", fmt.Sprintf("%s；%d 秒后第 %d 次重连", msg, int(reconnectDelay.Seconds()), attempt))
		select {
		case <-ctx.Done():
			c.h.OnState("idle", "已断开")
			return
		case <-time.After(reconnectDelay):
		}
	}
}

func (c *Client) session(ctx context.Context) error {
	cfg := c.cfg
	if cfg.Mountpoint == "" {
		return errors.New("未指定挂载点")
	}
	c.h.OnState("connecting", fmt.Sprintf("正在连接 %s:%d/%s", cfg.Host, cfg.Port, cfg.Mountpoint))
	conn, err := dial(ctx, cfg)
	if err != nil {
		return fmt.Errorf("连接失败: %w", err)
	}
	defer conn.Close()
	stop := watchCtx(ctx, conn)
	defer stop()

	conn.SetDeadline(time.Now().Add(15 * time.Second))
	if _, err := io.WriteString(conn, buildRequest(cfg, cfg.Mountpoint)); err != nil {
		return fmt.Errorf("发送请求失败: %w", err)
	}
	br := bufio.NewReaderSize(conn, 64*1024)
	resp, err := readResponse(br)
	if err != nil {
		return err
	}
	if resp.kind == kindSourceTable {
		return fmt.Errorf("挂载点 %q 不存在（服务器返回了源列表）", cfg.Mountpoint)
	}
	conn.SetDeadline(time.Time{})
	c.h.OnState("connected", fmt.Sprintf("%s/%s · %s", cfg.Host, cfg.Mountpoint, resp.status))

	if cfg.GGAInterval > 0 && cfg.HasPosition() {
		var wmu sync.Mutex
		sendGGA := func() error {
			s := BuildGGA(time.Now(), cfg.Lat, cfg.Lon, cfg.Alt, cfg.Sep)
			wmu.Lock()
			defer wmu.Unlock()
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			_, err := io.WriteString(conn, s)
			if err == nil && c.h.OnGGA != nil {
				c.h.OnGGA(strings.TrimSpace(s))
			}
			return err
		}
		if err := sendGGA(); err != nil {
			return fmt.Errorf("发送 GGA 失败: %w", err)
		}
		ggaDone := make(chan struct{})
		defer close(ggaDone)
		go func() {
			t := time.NewTicker(time.Duration(cfg.GGAInterval) * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ggaDone:
					return
				case <-t.C:
					if err := sendGGA(); err != nil {
						conn.Close()
						return
					}
				}
			}
		}()
	}

	timeout := time.Duration(cfg.Timeout) * time.Second
	buf := make([]byte, 32*1024)
	for {
		conn.SetReadDeadline(time.Now().Add(timeout))
		n, err := resp.body.Read(buf)
		if n > 0 {
			c.h.OnData(buf[:n])
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return errors.New("服务器关闭了连接")
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				return fmt.Errorf("%d 秒内未收到数据", cfg.Timeout)
			}
			return fmt.Errorf("读取数据失败: %w", err)
		}
	}
}
