//go:build unit

package service

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newSMTPTestCert 生成 127.0.0.1/localhost 的自签证书及其信任池。
func newSMTPTestCert(t *testing.T) (tls.Certificate, *x509.CertPool) {
REDACTED
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
REDACTED
	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "127.0.0.1"REDACTED,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuthREDACTED,
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")REDACTED,
		DNSNames:              []string{"localhost"REDACTED,
REDACTED
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
REDACTED
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
REDACTED
	pool := x509.NewCertPool()
	pool.AddCert(leaf)
	return tls.Certificate{Certificate: [][]byte{derREDACTED, PrivateKey: privREDACTED, pool
REDACTED

// fakeSMTPServer 是覆盖三种连接形态的最小 SMTP 服务器：
// 隐式 TLS（465 语义）、明文+STARTTLS（587 语义）、纯明文。
type fakeSMTPServer struct {
	listener          net.Listener
	tlsConfig         *tls.Config
	advertiseStartTLS bool

	mu       sync.Mutex
	commands []string
	conns    atomic.Int64
	wg       sync.WaitGroup
REDACTED

func startFakeSMTPServer(t *testing.T, implicitTLS, advertiseStartTLS bool) (*fakeSMTPServer, int) {
REDACTED
	cert, pool := newSMTPTestCert(t)
	prevPool := smtpTestRootCAs
	smtpTestRootCAs = pool
	t.Cleanup(func() { smtpTestRootCAs = prevPool REDACTED)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
REDACTED
	srv := &fakeSMTPServer{
		listener:          listener,
		tlsConfig:         &tls.Config{Certificates: []tls.Certificate{certREDACTED, MinVersion: tls.VersionTLS12REDACTED,
		advertiseStartTLS: advertiseStartTLS,
REDACTED
	if implicitTLS {
		srv.listener = tls.NewListener(listener, srv.tlsConfig)
REDACTED
	t.Cleanup(func() {
		_ = srv.listener.Close()
		srv.wg.Wait()
REDACTED)

	srv.wg.Add(1)
	go func() {
		defer srv.wg.Done()
		for {
			conn, err := srv.listener.Accept()
			if err != nil {
				return
		REDACTED
			srv.conns.Add(1)
			srv.wg.Add(1)
			go func() {
				defer srv.wg.Done()
				defer func() { _ = conn.Close() REDACTED()
				_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
				srv.serve(conn, srv.advertiseStartTLS)
		REDACTED()
	REDACTED
REDACTED()

	port := listener.Addr().(*net.TCPAddr).Port
	return srv, port
REDACTED

func (srv *fakeSMTPServer) record(cmd string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	srv.commands = append(srv.commands, cmd)
REDACTED

func (srv *fakeSMTPServer) sawCommand(prefix string) bool {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for _, cmd := range srv.commands {
		if strings.HasPrefix(strings.ToUpper(cmd), prefix) {
			return true
	REDACTED
REDACTED
	return false
REDACTED

func (srv *fakeSMTPServer) serve(conn net.Conn, allowStartTLS bool) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	writeLine := func(line string) bool {
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return false
	REDACTED
		return writer.Flush() == nil
REDACTED
	if !writeLine("220 fake.test ESMTP ready") {
		return
REDACTED
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
	REDACTED
		cmd := strings.TrimSpace(line)
		srv.record(cmd)
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			ok := writeLine("250-fake.test")
			if allowStartTLS {
				ok = ok && writeLine("250-STARTTLS")
		REDACTED
			if !(ok && writeLine("250-AUTH PLAIN LOGIN") && writeLine("250 8BITMIME")) {
				return
		REDACTED
		case upper == "STARTTLS" && allowStartTLS:
			if !writeLine("220 2.0.0 ready to start TLS") {
				return
		REDACTED
			tlsConn := tls.Server(conn, srv.tlsConfig)
			if err := tlsConn.Handshake(); err != nil {
				return
		REDACTED
			srv.serveUpgraded(tlsConn)
			return
		case strings.HasPrefix(upper, "AUTH"):
			if !writeLine("235 2.7.0 authentication successful") {
				return
		REDACTED
		case strings.HasPrefix(upper, "MAIL"), strings.HasPrefix(upper, "RCPT"):
			if !writeLine("250 ok") {
				return
		REDACTED
		case upper == "DATA":
			if !writeLine("354 go ahead") {
				return
		REDACTED
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
			REDACTED
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
			REDACTED
		REDACTED
			if !writeLine("250 message accepted") {
				return
		REDACTED
		case upper == "QUIT":
			_ = writeLine("221 bye")
			return
		default:
			if !writeLine("250 ok") {
				return
		REDACTED
	REDACTED
REDACTED
REDACTED

// serveUpgraded 复用命令循环处理 STARTTLS 升级后的会话（升级后不再提供 STARTTLS）。
func (srv *fakeSMTPServer) serveUpgraded(conn net.Conn) {
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)
	// net/smtp 在 StartTLS 成功后会重新发送 EHLO，直接进入命令循环即可。
	srv.serveCommands(reader, writer)
REDACTED

func (srv *fakeSMTPServer) serveCommands(reader *bufio.Reader, writer *bufio.Writer) {
	writeLine := func(line string) bool {
		if _, err := writer.WriteString(line + "\r\n"); err != nil {
			return false
	REDACTED
		return writer.Flush() == nil
REDACTED
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
	REDACTED
		cmd := strings.TrimSpace(line)
		srv.record(cmd)
		upper := strings.ToUpper(cmd)
		switch {
		case strings.HasPrefix(upper, "EHLO"), strings.HasPrefix(upper, "HELO"):
			if !(writeLine("250-fake.test") && writeLine("250-AUTH PLAIN LOGIN") && writeLine("250 8BITMIME")) {
				return
		REDACTED
		case strings.HasPrefix(upper, "AUTH"):
			if !writeLine("235 2.7.0 authentication successful") {
				return
		REDACTED
		case strings.HasPrefix(upper, "MAIL"), strings.HasPrefix(upper, "RCPT"):
			if !writeLine("250 ok") {
				return
		REDACTED
		case upper == "DATA":
			if !writeLine("354 go ahead") {
				return
		REDACTED
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
			REDACTED
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
			REDACTED
		REDACTED
			if !writeLine("250 message accepted") {
				return
		REDACTED
		case upper == "QUIT":
			_ = writeLine("221 bye")
			return
		default:
			if !writeLine("250 ok") {
				return
		REDACTED
	REDACTED
REDACTED
REDACTED

func smtpTestConfig(port int, useTLS bool) *SMTPConfig {
	return &SMTPConfig{
		Host:     "127.0.0.1",
		Port:     port,
		Username: "user",
		Password: "pass",
		From:     "noreply@example.com",
		FromName: "Test",
		UseTLS:   useTLS,
REDACTED
REDACTED

// 465 语义：UseTLS=true + 隐式 TLS 服务器，原有路径保持可用。
func TestSMTPConnectionImplicitTLS(t *testing.T) {
	srv, port := startFakeSMTPServer(t, true, false)
	svc := &EmailService{REDACTED

	if err := svc.TestSMTPConnectionWithConfig(smtpTestConfig(port, true)); err != nil {
		t.Fatalf("expected implicit TLS connection to succeed, got: %v", err)
REDACTED
	if !srv.sawCommand("EHLO") {
		t.Fatal("expected server to receive EHLO")
REDACTED
REDACTED

// 587 语义（#1470/#1488 核心场景）：UseTLS=true + 明文问候的 STARTTLS 服务器，
// 隐式 TLS 失败后必须自动降级为强制 STARTTLS 并成功。
func TestSMTPConnectionStartTLSFallbackWhenTLSEnabled(t *testing.T) {
	srv, port := startFakeSMTPServer(t, false, true)
	svc := &EmailService{REDACTED

	if err := svc.TestSMTPConnectionWithConfig(smtpTestConfig(port, true)); err != nil {
		t.Fatalf("expected STARTTLS fallback to succeed, got: %v", err)
REDACTED
	if !srv.sawCommand("STARTTLS") {
		t.Fatal("expected server to receive STARTTLS command")
REDACTED
	if got := srv.conns.Load(); got < 2 {
		t.Fatalf("expected implicit TLS attempt before STARTTLS fallback (>=2 connections), got %d", got)
REDACTED
REDACTED

// UseTLS=true 但服务器不支持 STARTTLS：必须报错，且绝不能把凭据发到明文连接上。
func TestSMTPConnectionMandatoryStartTLSRefusesPlaintext(t *testing.T) {
	srv, port := startFakeSMTPServer(t, false, false)
	svc := &EmailService{REDACTED

	err := svc.TestSMTPConnectionWithConfig(smtpTestConfig(port, true))
	if err == nil {
		t.Fatal("expected error when server does not support STARTTLS")
REDACTED
	if !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS-related error, got: %v", err)
REDACTED
	if srv.sawCommand("AUTH") {
		t.Fatal("credentials must not be sent over plaintext when TLS is required")
REDACTED
REDACTED

// UseTLS=false + 服务器支持 STARTTLS：测试连接与发送路径一致，机会式升级后认证成功。
// 这是 #1488 评论"测试连接不成功，发送测试邮件实际上能发"的回归用例。
func TestSMTPConnectionOpportunisticStartTLSWhenTLSDisabled(t *testing.T) {
	srv, port := startFakeSMTPServer(t, false, true)
	svc := &EmailService{REDACTED

	if err := svc.TestSMTPConnectionWithConfig(smtpTestConfig(port, false)); err != nil {
		t.Fatalf("expected opportunistic STARTTLS test connection to succeed, got: %v", err)
REDACTED
	if !srv.sawCommand("STARTTLS") {
		t.Fatal("expected test connection to upgrade via STARTTLS like the send path")
REDACTED
REDACTED

// UseTLS=false + 服务器不支持 STARTTLS：保持明文直连（既有行为不回归）。
func TestSMTPConnectionPlainWhenNoStartTLS(t *testing.T) {
	srv, port := startFakeSMTPServer(t, false, false)
	svc := &EmailService{REDACTED

	if err := svc.TestSMTPConnectionWithConfig(smtpTestConfig(port, false)); err != nil {
		t.Fatalf("expected plain connection to succeed, got: %v", err)
REDACTED
	if srv.sawCommand("STARTTLS") {
		t.Fatal("did not expect STARTTLS command when server does not advertise it")
REDACTED
REDACTED

// 发送路径全流程：UseTLS=true + STARTTLS 服务器（587 语义）完整走完 MAIL/RCPT/DATA。
func TestSendEmailWithConfigStartTLSFallback(t *testing.T) {
	srv, port := startFakeSMTPServer(t, false, true)
	svc := &EmailService{REDACTED

	err := svc.SendEmailWithConfig(smtpTestConfig(port, true), "rcpt@example.com", "subject", "<p>body</p>")
	if err != nil {
		t.Fatalf("expected send via STARTTLS fallback to succeed, got: %v", err)
REDACTED
	if !srv.sawCommand("STARTTLS") {
		t.Fatal("expected send path to upgrade via STARTTLS")
REDACTED
	if !srv.sawCommand("DATA") {
		t.Fatal("expected send path to reach DATA")
REDACTED
REDACTED

// 发送路径全流程：UseTLS=true + 隐式 TLS 服务器（465 语义）保持既有行为。
func TestSendEmailWithConfigImplicitTLS(t *testing.T) {
	srv, port := startFakeSMTPServer(t, true, false)
	svc := &EmailService{REDACTED

	err := svc.SendEmailWithConfig(smtpTestConfig(port, true), "rcpt@example.com", "subject", "<p>body</p>")
	if err != nil {
		t.Fatalf("expected send via implicit TLS to succeed, got: %v", err)
REDACTED
	if !srv.sawCommand("DATA") {
		t.Fatal("expected send path to reach DATA")
REDACTED
REDACTED
