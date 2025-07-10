package usbwallet

import (
	"io"
	"time"

	"github.com/google/gousb"
)

type usbEnumerator struct {
	ctx        *gousb.Context
	vendorID   uint16   // USB vendor identifier used for device discovery
	productIDs []uint16 // USB product identifiers used for device discovery
}

// NewUsbEnumerator creates a new USB device enumerator for generic USB devices.
func newUsbEnumerator(vendorID uint16, productIDs []uint16) enumerator {
	ctx := gousb.NewContext()
	return &usbEnumerator{
		ctx:        ctx,
		vendorID:   vendorID,
		productIDs: productIDs,
	}
}

func (e *usbEnumerator) Infos() ([]info, error) {
	var infos []info

	d, err := e.ctx.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if uint16(desc.Vendor) == e.vendorID {
			for _, id := range e.productIDs {
				if uint16(desc.Product) == id {
					return true
				}
			}
		}
		return false
	})
	if err != nil {
		return nil, err
	}

	for _, device := range d {
		infos = append(infos, &usbInfo{device})
	}

	return infos, nil
}

func (e *usbEnumerator) Close() {
	if e.ctx != nil {
		_ = e.ctx.Close()
		e.ctx = nil
	}
}

type usbInfo struct {
	*gousb.Device
}

func (o *usbInfo) Path() string {
	return o.Device.String()
}

func (o *usbInfo) Open() (io.ReadWriteCloser, error) {
	rwc, err := o.open()
	if err != nil {
		return nil, err
	}

	// hacky: flush the device input buffer to avoid reading stale responses from Trezor
	c := make(chan interface{})
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := rwc.Read(buf)
			if err != nil {
				return
			}
			c <- struct{}{}
		}
	}()
	for {
		select {
		case <-c:
			continue
		case <-time.After(100 * time.Millisecond):
		}
		break
	}
	_ = rwc.Close()

	return o.open()
}

func (o *usbInfo) open() (io.ReadWriteCloser, error) {
	intf, done, err := o.Device.DefaultInterface()
	if err != nil {
		return nil, err
	}

	in, err := intf.InEndpoint(0x01)
	if err != nil {
		return nil, err
	}

	out, err := intf.OutEndpoint(0x01)
	if err != nil {
		return nil, err
	}

	return &usbReadWriteCloser{
		done: done,
		in:   in,
		out:  out,
	}, nil
}

type usbReadWriteCloser struct {
	done func()
	in   *gousb.InEndpoint
	out  *gousb.OutEndpoint
}

func (r usbReadWriteCloser) Read(p []byte) (n int, err error) {
	return r.in.Read(p)
}

func (r usbReadWriteCloser) Write(p []byte) (n int, err error) {
	return r.out.Write(p)
}

func (r usbReadWriteCloser) Close() error {
	if r.done != nil {
		r.done()
		r.done = nil
	}
	return nil
}
