package controller

import (
	"bytes"
	"context"
	"fmt"
	"net"
)

func MagicPacket(address string) ([]byte, error) {
	mac, err := net.ParseMAC(address)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 || bytes.Equal(mac, make([]byte, 6)) {
		return nil, fmt.Errorf("invalid six-byte unicast wake MAC")
	}
	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xff
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}
	return packet, nil
}

func SendWake(ctx context.Context, mac, broadcast string) error {
	packet, err := MagicPacket(mac)
	if err != nil {
		return err
	}
	ip := net.ParseIP(broadcast)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("invalid wake broadcast IPv4 address")
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp4", net.JoinHostPort(broadcast, "9"))
	if err != nil {
		return fmt.Errorf("open wake socket: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetWriteDeadline(deadline); err != nil {
			return err
		}
	}
	n, err := conn.Write(packet)
	if err != nil {
		return fmt.Errorf("send wake packet: %w", err)
	}
	if n != len(packet) {
		return fmt.Errorf("incomplete wake packet")
	}
	return nil
}
