package forward

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

// SOCKS5 constants (RFC 1928).
const (
	socksVersion   = 5
	methodNoAuth   = 0
	methodNoAccept = 0xff
	cmdConnect     = 1
	atypIPv4       = 1
	atypDomain     = 3
	atypIPv6       = 4

	repSucceeded      = 0
	repHostUnreach    = 4
	repCmdUnsupported = 7
	repAtypUnsupport  = 8
)

// socksHandshake performs a minimal no-auth SOCKS5 CONNECT handshake on c,
// dials the requested target with dial and returns the outbound connection.
func socksHandshake(c net.Conn, dial func(addr string) (net.Conn, error)) (net.Conn, error) {
	_ = c.SetDeadline(time.Now().Add(15 * time.Second))
	defer c.SetDeadline(time.Time{})

	hdr := make([]byte, 2)
	if _, err := io.ReadFull(c, hdr); err != nil {
		return nil, err
	}
	if hdr[0] != socksVersion {
		return nil, errors.New("not a SOCKS5 client")
	}
	methods := make([]byte, hdr[1])
	if _, err := io.ReadFull(c, methods); err != nil {
		return nil, err
	}
	ok := false
	for _, m := range methods {
		if m == methodNoAuth {
			ok = true
		}
	}
	if !ok {
		_, _ = c.Write([]byte{socksVersion, methodNoAccept})
		return nil, errors.New("SOCKS client requires authentication")
	}
	if _, err := c.Write([]byte{socksVersion, methodNoAuth}); err != nil {
		return nil, err
	}

	req := make([]byte, 4)
	if _, err := io.ReadFull(c, req); err != nil {
		return nil, err
	}
	if req[0] != socksVersion {
		return nil, errors.New("bad SOCKS request")
	}
	if req[1] != cmdConnect {
		reply(c, repCmdUnsupported)
		return nil, errors.New("only SOCKS CONNECT is supported")
	}
	var host string
	switch req[3] {
	case atypIPv4:
		b := make([]byte, 4)
		if _, err := io.ReadFull(c, b); err != nil {
			return nil, err
		}
		host = net.IP(b).String()
	case atypIPv6:
		b := make([]byte, 16)
		if _, err := io.ReadFull(c, b); err != nil {
			return nil, err
		}
		host = net.IP(b).String()
	case atypDomain:
		l := make([]byte, 1)
		if _, err := io.ReadFull(c, l); err != nil {
			return nil, err
		}
		b := make([]byte, l[0])
		if _, err := io.ReadFull(c, b); err != nil {
			return nil, err
		}
		host = string(b)
	default:
		reply(c, repAtypUnsupport)
		return nil, errors.New("unsupported SOCKS address type")
	}
	pb := make([]byte, 2)
	if _, err := io.ReadFull(c, pb); err != nil {
		return nil, err
	}
	port := binary.BigEndian.Uint16(pb)

	out, err := dial(net.JoinHostPort(host, strconv.Itoa(int(port))))
	if err != nil {
		reply(c, repHostUnreach)
		return nil, err
	}
	if err := reply(c, repSucceeded); err != nil {
		out.Close()
		return nil, err
	}
	return out, nil
}

func reply(c net.Conn, code byte) error {
	// Bound address is reported as 0.0.0.0:0; clients ignore it for CONNECT.
	_, err := c.Write([]byte{socksVersion, code, 0, atypIPv4, 0, 0, 0, 0, 0, 0})
	return err
}
