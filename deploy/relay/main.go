// AstroGate's public relay forwards raw HTTP and TLS streams to the private origin.
// It holds no application secrets, database credentials, or TLS private keys.
package main

import (
	"io"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

type route struct {
	listen   string
	upstream string
}

func setting(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func serve(route route, ready chan<- error) {
	listener, err := net.Listen("tcp", route.listen)
	if err != nil {
		ready <- err
		return
	}
	ready <- nil
	log.Printf("listening on %s, forwarding to %s", route.listen, route.upstream)
	limit := make(chan struct{}, 2048)
	for {
		client, err := listener.Accept()
		if err != nil {
			log.Printf("accept %s: %v", route.listen, err)
			continue
		}
		select {
		case limit <- struct{}{}:
			go func() {
				defer func() { <-limit }()
				forward(client, route.upstream)
			}()
		default:
			client.Close()
		}
	}
}

func forward(client net.Conn, upstream string) {
	defer client.Close()
	origin, err := net.DialTimeout("tcp", upstream, 5*time.Second)
	if err != nil {
		return
	}
	defer origin.Close()

	var send sync.WaitGroup
	send.Add(1)
	go func() {
		defer send.Done()
		io.Copy(origin, client)
		if tcp, ok := origin.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()
	io.Copy(client, origin)
	if tcp, ok := client.(*net.TCPConn); ok {
		tcp.CloseWrite()
	}
	send.Wait()
}

func main() {
	routes := []route{
		{setting("ASTROGATE_HTTP_LISTEN", ":80"), setting("ASTROGATE_HTTP_UPSTREAM", "127.0.0.1:18080")},
		{setting("ASTROGATE_HTTPS_LISTEN", ":443"), setting("ASTROGATE_HTTPS_UPSTREAM", "127.0.0.1:18443")},
	}
	ready := make(chan error, len(routes))
	for _, item := range routes {
		go serve(item, ready)
	}
	for range routes {
		if err := <-ready; err != nil {
			log.Fatal(err)
		}
	}
	select {}
}
