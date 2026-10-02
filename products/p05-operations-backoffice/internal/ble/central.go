package ble

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"tinygo.org/x/bluetooth"

	"github.com/0xmhha/nu-54v-dk-toy/packages/protocol/go/protocol"
)

// Found is a device advertising the payment service.
type Found struct {
	Address bluetooth.Address
	Name    string
	RSSI    int16
}

// Scan finds the nearest device advertising the payment service (or named NU54..., for a
// peripheral that put the service UUID in its scan response). Pairing for setup sessions is
// done by the operating system when the device asks for it (payment-protocol.md 3).
func Scan(ctx context.Context, wait time.Duration) (Found, error) {
	adapter := bluetooth.DefaultAdapter
	if err := adapter.Enable(); err != nil {
		return Found{}, fmt.Errorf("bluetooth: %w", err)
	}
	service, _ := bluetooth.ParseUUID(protocol.GATTService)
	var best *Found
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
		_ = adapter.StopScan()
		close(done)
	}()
	err := adapter.Scan(func(_ *bluetooth.Adapter, r bluetooth.ScanResult) {
		if r.HasServiceUUID(service) || strings.HasPrefix(r.LocalName(), "NU54") {
			if best == nil || r.RSSI > best.RSSI {
				best = &Found{r.Address, r.LocalName(), r.RSSI}
			}
		}
	})
	<-done
	if err != nil {
		return Found{}, fmt.Errorf("scan: %w", err)
	}
	if best == nil {
		return Found{}, errors.New("no device in setup reach (is it UNPROVISIONED or in pairing mode?)")
	}
	return *best, nil
}

// Central is a Transport over a GATT connection: writes to RX with response, TX notifications.
type Central struct {
	dev   bluetooth.Device
	rx    bluetooth.DeviceCharacteristic
	mtu   int
	notes chan []byte
}

// Connect connects to a found device and subscribes to TX.
func Connect(f Found) (*Central, error) {
	dev, err := bluetooth.DefaultAdapter.Connect(f.Address, bluetooth.ConnectionParams{})
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	fail := func(err error) (*Central, error) {
		_ = dev.Disconnect()
		return nil, err
	}
	sid, _ := bluetooth.ParseUUID(protocol.GATTService)
	rid, _ := bluetooth.ParseUUID(protocol.GATTRx)
	tid, _ := bluetooth.ParseUUID(protocol.GATTTx)
	svcs, err := dev.DiscoverServices([]bluetooth.UUID{sid})
	if err != nil || len(svcs) == 0 {
		return fail(fmt.Errorf("payment service not found: %v", err))
	}
	chars, err := svcs[0].DiscoverCharacteristics([]bluetooth.UUID{rid, tid})
	if err != nil {
		return fail(fmt.Errorf("characteristics: %w", err))
	}
	c := &Central{dev: dev, notes: make(chan []byte, 256)}
	var tx *bluetooth.DeviceCharacteristic
	for i := range chars {
		switch chars[i].UUID() {
		case rid:
			c.rx = chars[i]
		case tid:
			tx = &chars[i]
		}
	}
	if tx == nil || c.rx.UUID() != rid {
		return fail(errors.New("RX or TX characteristic missing"))
	}
	m, err := c.rx.GetMTU()
	if err != nil {
		return fail(err)
	}
	c.mtu = attMTU(m)
	if err := tx.EnableNotifications(func(buf []byte) {
		c.notes <- append([]byte(nil), buf...) // the library reuses buf
	}); err != nil {
		return fail(fmt.Errorf("subscribe to TX: %w", err))
	}
	return c, nil
}

func (c *Central) MTU() int                     { return c.mtu }
func (c *Central) Notifications() <-chan []byte { return c.notes }
func (c *Central) Close() error                 { return c.dev.Disconnect() }

func (c *Central) Write(fragment []byte) error {
	_, err := c.rx.Write(fragment) // with response: in order, one at a time
	return err
}

var _ Transport = (*Central)(nil)
