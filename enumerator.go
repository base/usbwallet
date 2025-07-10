package usbwallet

import "io"

type enumerator interface {
	// Infos returns the list of USB devices matching the vendor and product IDs.
	Infos() ([]info, error)
	// Close releases any resources held by the enumerator.
	Close()
}

type info interface {
	// Path returns the USB device path, which can be used for identifying the connection.
	Path() string
	// Open opens a connection to the USB device and returns a ReadWriteCloser for communication.
	Open() (io.ReadWriteCloser, error)
}
