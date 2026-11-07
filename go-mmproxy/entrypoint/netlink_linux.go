package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"syscall"
)

// Netlink constants missing from package syscall.
const (
	nlmFRequest = 0x1
	nlmFReplace = 0x100
	nlmFExcl    = 0x200
	nlmFCreate  = 0x400
	nlmFAck     = 0x4

	rtaOIF   = 4
	rtaTable = 15

	fraSrc      = 2
	fraIIFName  = 3
	fraPriority = 6
	fraTable    = 15

	frActToTbl = 1

	rtnLocal    = 2
	rtprotBoot  = 3
	rtScopeHost = 254
)

var native = binary.NativeEndian

func align4(n int) int { return (n + 3) &^ 3 }

func attr(typ uint16, data []byte) []byte {
	b := make([]byte, align4(4+len(data)))
	native.PutUint16(b[0:], uint16(4+len(data)))
	native.PutUint16(b[2:], typ)
	copy(b[4:], data)
	return b
}

func attrU32(typ uint16, v uint32) []byte {
	var d [4]byte
	native.PutUint32(d[:], v)
	return attr(typ, d[:])
}

func message(typ, flags uint16, seq uint32, payload []byte) []byte {
	b := make([]byte, syscall.NLMSG_HDRLEN+len(payload))
	native.PutUint32(b[0:], uint32(len(b)))
	native.PutUint16(b[4:], typ)
	native.PutUint16(b[6:], flags)
	native.PutUint32(b[8:], seq)
	copy(b[syscall.NLMSG_HDRLEN:], payload)
	return b
}

func tableByte(table uint32) byte {
	if table < 256 {
		return byte(table)
	}
	return 0
}

// localRouteMessage builds `ip route replace local <default> dev <ifindex> table <table>`.
// NLM_F_REPLACE makes it safe to apply repeatedly.
func localRouteMessage(family int, table uint32, ifindex int, seq uint32) []byte {
	scope := byte(0)
	if family == syscall.AF_INET {
		scope = rtScopeHost
	}
	body := []byte{byte(family), 0, 0, 0, tableByte(table), rtprotBoot, scope, rtnLocal, 0, 0, 0, 0}
	body = append(body, attrU32(rtaTable, table)...)
	body = append(body, attrU32(rtaOIF, uint32(ifindex))...)
	return message(syscall.RTM_NEWROUTE, nlmFRequest|nlmFAck|nlmFCreate|nlmFReplace, seq, body)
}

// iifRuleMessage builds `ip rule add from <src>/<srcLen> iif <iif> table <table> priority <prio>`.
// NLM_F_EXCL turns an identical existing rule into EEXIST. An explicit priority is
// required for the kernel's duplicate detection.
func iifRuleMessage(family int, src net.IP, srcLen int, iif string, table, priority uint32, seq uint32) []byte {
	body := []byte{byte(family), 0, byte(srcLen), 0, tableByte(table), 0, 0, frActToTbl, 0, 0, 0, 0}
	body = append(body, attr(fraSrc, src)...)
	body = append(body, attr(fraIIFName, append([]byte(iif), 0))...)
	body = append(body, attrU32(fraPriority, priority)...)
	body = append(body, attrU32(fraTable, table)...)
	return message(syscall.RTM_NEWRULE, nlmFRequest|nlmFAck|nlmFCreate|nlmFExcl, seq, body)
}

// ackError extracts the errno from a netlink acknowledgement; nil means success.
func ackError(resp []byte, seq uint32) error {
	for len(resp) >= syscall.NLMSG_HDRLEN {
		length := int(native.Uint32(resp[0:]))
		if length < syscall.NLMSG_HDRLEN || length > len(resp) {
			return errors.New("malformed netlink response")
		}
		typ := native.Uint16(resp[4:])
		if native.Uint32(resp[8:]) == seq && typ == syscall.NLMSG_ERROR {
			if length < syscall.NLMSG_HDRLEN+4 {
				return errors.New("short netlink error")
			}
			if code := int32(native.Uint32(resp[syscall.NLMSG_HDRLEN:])); code != 0 {
				return syscall.Errno(-code)
			}
			return nil
		}
		resp = resp[align4(length):]
	}
	return errors.New("no netlink acknowledgement")
}

type conn struct {
	fd  int
	seq uint32
}

func dial() (*conn, error) {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	return &conn{fd: fd}, nil
}

func (c *conn) close() { syscall.Close(c.fd) }

func (c *conn) do(build func(seq uint32) []byte) error {
	c.seq++
	if err := syscall.Sendto(c.fd, build(c.seq), 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return err
	}
	buf := make([]byte, 8192)
	n, _, err := syscall.Recvfrom(c.fd, buf, 0)
	if err != nil {
		return err
	}
	return ackError(buf[:n], c.seq)
}

// applyRouting installs the policy routing go-mmproxy needs in the current network
// namespace: a local route for all addresses in the table, and a rule sending
// loopback-sourced traffic arriving on lo to that table. It is idempotent.
func applyRouting(table, priority uint32) error {
	lo, err := net.InterfaceByName("lo")
	if err != nil {
		return fmt.Errorf("lookup lo: %w", err)
	}
	c, err := dial()
	if err != nil {
		return fmt.Errorf("open netlink: %w", err)
	}
	defer c.close()

	steps := []struct {
		name   string
		family int
		src    net.IP
		srcLen int
		route  bool
	}{
		{"ip route local 0.0.0.0/0", syscall.AF_INET, nil, 0, true},
		{"ip rule from 127.0.0.1/8", syscall.AF_INET, net.IPv4(127, 0, 0, 1).To4(), 8, false},
		{"ip -6 route local ::/0", syscall.AF_INET6, nil, 0, true},
		{"ip -6 rule from ::1/128", syscall.AF_INET6, net.IPv6loopback, 128, false},
	}
	for _, s := range steps {
		err := c.do(func(seq uint32) []byte {
			if s.route {
				return localRouteMessage(s.family, table, lo.Index, seq)
			}
			return iifRuleMessage(s.family, s.src, s.srcLen, "lo", table, priority, seq)
		})
		if err != nil && !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("%s table %d: %w", s.name, table, err)
		}
	}
	return nil
}
