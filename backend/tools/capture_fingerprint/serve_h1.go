package main

import (
	"bufio"
	ctls "crypto/tls"
)

func serveH1(tlsConn *ctls.Conn, c *Capture) error {
	c.HTTP1 = &H1Capture{}
	r := bufio.NewReader(tlsConn)

	line, err := r.ReadString('\n')
	if err != nil {
		return err
	}
	c.HTTP1.RequestLine = trimCRLF(line)
	for {
		h, err := r.ReadString('\n')
		if err != nil {
			return err
		}
		h = trimCRLF(h)
		if h == "" {
			break
		}
		c.HTTP1.Headers = append(c.HTTP1.Headers, h)
	}

	resp := "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nCache-Control: no-cache\r\nConnection: close\r\n\r\n" +
		"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
	_, _ = tlsConn.Write([]byte(resp))
	return nil
}
