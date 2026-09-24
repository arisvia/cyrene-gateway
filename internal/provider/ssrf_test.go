package provider

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestValidateUpstreamURLSSRF(t *testing.T) {
	denyTestDNS(t)
	for _, tt := range []struct {
		url          string
		allowPrivate bool
		wantErr      bool
	}{
		{"https://93.184.216.34/v1", false, false},
		{"https://127.0.0.1:8080", false, true},
		{"http://localhost:3000", false, true},
		{"http://LOCALHOST./", false, true},
		{"http://service.local./", false, true},
		{"http://child.localhost/", false, true},
		{"https://192.168.1.1/v1", false, true},
		{"https://10.0.0.1/v1", false, true},
		{"https://169.254.169.254/latest/meta-data", false, true},
		{"https://100.64.0.1/", false, true},
		{"https://[::ffff:127.0.0.1]/", false, true},
		{"https://[fe80::1%25eth0]/", false, true},
		{"ftp://93.184.216.34", false, true},
		{"http:///missing-host", true, true},
		{"http://unresolved.invalid/", false, true},
		{"http://unresolved.invalid/", true, false},
		{"http://localhost:3000", true, false},
	} {
		t.Run(tt.url, func(t *testing.T) {
			_, err := ValidateUpstreamURL(tt.url, tt.allowPrivate)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateUpstreamURL(%q, %v) error = %v, wantErr %v", tt.url, tt.allowPrivate, err, tt.wantErr)
			}
		})
	}
}

func TestValidateUpstreamURLLocalNamesBlockedWithoutDNS(t *testing.T) {
	denyTestDNS(t)
	for _, target := range []string{"http://LOCALHOST./", "http://child.localhost/", "http://service.internal./", "http://[fe80::1%25eth0]/"} {
		if _, err := ValidateUpstreamURL(target, false); !errors.Is(err, ErrPrivateNetworkBlocked) {
			t.Errorf("expected private-network error for %s, got %v", target, err)
		}
	}
}

// Serve fixed A/AAAA answers over an in-memory DNS-over-TCP connection. This
// exercises the real resolver/validation path without depending on external DNS.
func stubTestDNS(t *testing.T, records map[string][]net.IP) {
	t.Helper()
	stubTestDNSFunc(t, func(host string, _ uint16) []net.IP { return records[host] })
}

func stubTestDNSFunc(t *testing.T, lookup func(string, uint16) []net.IP) {
	t.Helper()
	previous := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(context.Context, string, string) (net.Conn, error) {
			client, server := net.Pipe()
			_ = server.SetDeadline(time.Now().Add(5 * time.Second))
			go func() {
				defer server.Close()
				var size [2]byte
				if _, err := io.ReadFull(server, size[:]); err != nil {
					return
				}
				query := make([]byte, binary.BigEndian.Uint16(size[:]))
				if _, err := io.ReadFull(server, query); err != nil || len(query) < 17 {
					return
				}
				pos, host := 12, ""
				for pos < len(query) && query[pos] != 0 {
					n := int(query[pos])
					pos++
					if pos+n >= len(query) {
						return
					}
					host += string(query[pos:pos+n]) + "."
					pos += n
				}
				if pos+5 > len(query) {
					return
				}
				kind := binary.BigEndian.Uint16(query[pos+1 : pos+3])
				response := append([]byte(nil), query[:pos+5]...)
				binary.BigEndian.PutUint16(response[2:4], 0x8180)
				clear(response[6:12])
				var count uint16
				for _, ip := range lookup(host, kind) {
					var data []byte
					if kind == 1 {
						data = ip.To4()
					} else if kind == 28 && ip.To4() == nil {
						data = ip.To16()
					}
					if data == nil {
						continue
					}
					response = append(response, 0xc0, 0x0c, 0, byte(kind), 0, 1, 0, 0, 0, 0, 0, byte(len(data)))
					response = append(response, data...)
					count++
				}
				binary.BigEndian.PutUint16(response[6:8], count)
				binary.BigEndian.PutUint16(size[:], uint16(len(response)))
				_, _ = server.Write(append(size[:], response...))
			}()
			return client, nil
		},
	}
	t.Cleanup(func() { net.DefaultResolver = previous })
}
