package usbwallet

import (
	"errors"
	"io"

	"github.com/karalabe/usb"
)

type usbEnumerator struct {
	vendorID   uint16   // USB vendor identifier used for device discovery
	productIDs []uint16 // USB product identifiers used for device discovery
	usageID    uint16   // USB usage page identifier used for macOS device discovery
	endpointID int      // USB endpoint identifier used for non-macOS device discovery
}

// newUsbEnumerator creates a new USB device enumerator for HID devices.
func newUsbEnumerator(vendorID uint16, productIDs []uint16, usageID uint16, endpointID int) enumerator {
	return &usbEnumerator{
		vendorID:   vendorID,
		productIDs: productIDs,
		usageID:    usageID,
		endpointID: endpointID,
	}
}

func (e *usbEnumerator) Infos() ([]info, error) {
	if !usb.Supported() {
		return nil, errors.New("unsupported platform")
	}

	var infos []info

	i, err := usb.Enumerate(e.vendorID, 0)
	if err != nil {
		return nil, err
	}

	for _, info := range i {
		for _, id := range e.productIDs {
			// We check both the raw ProductID (legacy) and just the upper byte, as Ledger
			// uses `MMII`, encoding a model (MM) and an interface bitfield (II)
			mmOnly := info.ProductID & 0xff00
			// Windows and Macos use UsageID matching, Linux uses Interface matching
			if (info.ProductID == id || mmOnly == id) && (info.UsagePage == e.usageID || info.Interface == e.endpointID) {
				infos = append(infos, &usbInfo{info})
				break
			}
		}
	}

	return infos, nil
}

func (e *usbEnumerator) Close() {
}

type usbInfo struct {
	usb.DeviceInfo
}

func (o *usbInfo) Path() string {
	return o.DeviceInfo.Path
}

func (o *usbInfo) Open() (io.ReadWriteCloser, error) {
	return o.DeviceInfo.Open()
}
