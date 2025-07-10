package usbwallet

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

const (
	ledgerOpSignPersonalMessage  ledgerOpcode = 0x08 // Signs a personal message following the EIP 712 specification
	ledgerOpEip712SendStructDef  ledgerOpcode = 0x1a // Sends EIP-712 struct types to the ledger
	ledgerOpEip712SendStructImpl ledgerOpcode = 0x1c // Sends EIP-712 struct values to the ledger

	ledgerP1CompleteSend       ledgerParam1 = 0x00 // Send the full value in a single message
	ledgerP2StructName         ledgerParam2 = 0x00 // Send EIP-712 struct name
	ledgerP2RootStruct         ledgerParam2 = 0x00 // Send EIP-712 root struct
	ledgerP2Array              ledgerParam2 = 0x0f // Send EIP-712 array
	ledgerP2StructField        ledgerParam2 = 0xff // Send EIP-712 struct field
	ledgerP2FullImplementation ledgerParam2 = 0x01 // EIP-712 full implementation (typed data)
)

// SignText implements usbwallet.driver, sending the message to the Ledger and
// waiting for the user to confirm or deny the signature.
func (w *ledgerDriver) SignText(path accounts.DerivationPath, text []byte) ([]byte, error) {
	// If the Ethereum app doesn't run, abort
	if w.offline() {
		return nil, accounts.ErrWalletClosed
	}
	// Ensure the wallet is capable of signing the given transaction
	if w.version[0] < 1 && w.version[1] < 5 {
		//lint:ignore ST1005 brand name displayed on the console
		return nil, fmt.Errorf("Ledger version >= 1.5.0 required for EIP-712 signing (found version v%d.%d.%d)", w.version[0], w.version[1], w.version[2])
	}
	// All infos gathered and metadata checks out, request signing
	return w.ledgerSignPersonalMessage(path, text)
}

// SignedTypedData implements usbwallet.driver, sending the message to the Ledger and
// waiting for the user to sign or deny signing an EIP-712 typed data struct.
func (w *ledgerDriver) SignedTypedData(path accounts.DerivationPath, data apitypes.TypedData) ([]byte, error) {
	// If the Ethereum app doesn't run, abort
	if w.offline() {
		return nil, accounts.ErrWalletClosed
	}
	// Ensure the wallet is capable of signing the given transaction
	if w.version[0] < 1 && w.version[1] < 5 {
		//lint:ignore ST1005 brand name displayed on the console
		return nil, fmt.Errorf("Ledger version >= 1.5.0 required for EIP-712 signing (found version v%d.%d.%d)", w.version[0], w.version[1], w.version[2])
	}
	// All infos gathered and metadata checks out, request signing
	return w.ledgerSignTypedData(path, data)
}

// ledgerSignPersonalMessage sends the transaction to the Ledger wallet, and waits for the user
// to confirm or deny the transaction.
//
// The signing protocol is defined as follows:
//
//	CLA | INS | P1 | P2                          | Lc  | Le
//	----+-----+----+-----------------------------+-----+---
//	 E0 | 08  | 00 | implementation version : 00 | variable | variable
//
// Where the input is:
//
//	Description                                      | Length
//	-------------------------------------------------+----------
//	Number of BIP 32 derivations to perform (max 10) | 1 byte
//	First derivation index (big endian)              | 4 bytes
//	...                                              | 4 bytes
//	Last derivation index (big endian)               | 4 bytes
//	text                                             | arbitrary
//
// And the output data is:
//
//	Description | Length
//	------------+---------
//	signature V | 1 byte
//	signature R | 32 bytes
//	signature S | 32 bytes
func (w *ledgerDriver) ledgerSignPersonalMessage(derivationPath []uint32, text []byte) ([]byte, error) {
	// Flatten the derivation path into the Ledger request
	path := make([]byte, 5+4*len(derivationPath))
	path[0] = byte(len(derivationPath))
	for i, component := range derivationPath {
		binary.BigEndian.PutUint32(path[1+4*i:], component)
	}
	binary.BigEndian.PutUint32(path[1+4*len(derivationPath):], uint32(len(text)))
	// Create the 712 message
	payload := append(path, text...)

	// Send the request and wait for the response
	var (
		reply []byte
		err   error
	)

	// Send the message over, ensuring it's processed correctly
	reply, err = w.ledgerExchange(ledgerOpSignPersonalMessage, ledgerP1InitTransactionData, 0, payload)

	if err != nil {
		return nil, err
	}

	// Extract the Ethereum signature and do a sanity validation
	if len(reply) != crypto.SignatureLength {
		return nil, errors.New("reply lacks signature")
	}
	signature := append(reply[1:], reply[0])
	return signature, nil
}

// ledgerSignTypedData sends the transaction to the Ledger wallet, and waits for the user
// to confirm or deny the transaction.
//
// The typed data struct fields and values need to be sent to the Ledger first.
// See https://github.com/LedgerHQ/app-ethereum/blob/develop/doc/eip712.md.
//
// After the data is sent, the signing protocol is defined as follows:
//
//	CLA | INS | P1 | P2                          | Lc  | Le
//	----+-----+----+-----------------------------+-----+---
//	 E0 | 0C  | 00 | implementation version : 00 | variable | variable
//
// Where the input is:
//
//	Description                                      | Length
//	-------------------------------------------------+----------
//	Number of BIP 32 derivations to perform (max 10) | 1 byte
//	First derivation index (big endian)              | 4 bytes
//	...                                              | 4 bytes
//	Last derivation index (big endian)               | 4 bytes
//
// And the output data is:
//
//	Description | Length
//	------------+---------
//	signature V | 1 byte
//	signature R | 32 bytes
//	signature S | 32 bytes
func (w *ledgerDriver) ledgerSignTypedData(derivationPath []uint32, data apitypes.TypedData) ([]byte, error) {
	// Check if the EIP712Domain and primary type are present in the data
	domainStruct := data.Types["EIP712Domain"]
	if domainStruct == nil {
		return nil, fmt.Errorf("EIP712Domain type is required")
	}
	primaryType := data.Types[data.PrimaryType]
	if primaryType == nil {
		return nil, fmt.Errorf("primary type %s not found in types", data.PrimaryType)
	}

	// sendField is a function for sending an EIP-712 struct field name + type
	sendField := func(field apitypes.Type) error {
		dt, name, byteLength, arrays, err := parseType(data, field)
		if err != nil {
			return err
		}

		typeDesc := byte(dt)

		var typeName []byte
		var typeSize []byte
		switch dt {
		case CustomType:
			typeName = append([]byte{byte(len(name))}, []byte(name)...)
		case IntType, UintType, FixedBytesType:
			typeSize = []byte{byte(byteLength)}
			typeDesc |= 0x40
		default:
		}

		var arrayLevels []byte
		if len(arrays) > 0 {
			typeDesc |= 0x80
			arrayLevels = []byte{byte(len(arrays))}
			for _, length := range arrays {
				if length == nil {
					arrayLevels = append(arrayLevels, 0)
				} else {
					arrayLevels = append(arrayLevels, 1, byte(*length))
				}
			}
		}

		payload := []byte{typeDesc}
		payload = append(payload, typeName...)
		payload = append(payload, typeSize...)
		payload = append(payload, arrayLevels...)
		payload = append(payload, byte(len(field.Name)))
		payload = append(payload, []byte(field.Name)...)
		_, err = w.ledgerExchange(ledgerOpEip712SendStructDef, 0, ledgerP2StructField, payload)
		return err
	}

	// sendValue is a recursive function that sends the value of a field
	var sendValue func(t, name string, value interface{}) error
	sendValue = func(t, name string, value interface{}) error {
		if value == nil {
			return fmt.Errorf("nil value for field %s", name)
		}
		if strings.HasSuffix(t, "]") {
			a, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("expected array for field %s, got %T", name, value)
			}
			if _, err := w.ledgerExchange(ledgerOpEip712SendStructImpl, ledgerP1CompleteSend, ledgerP2Array, []byte{byte(len(a))}); err != nil {
				return fmt.Errorf("failed to send array length: %w", err)
			}
			t = t[:strings.LastIndex(t, "[")]
			for _, item := range a {
				if err := sendValue(t, name, item); err != nil {
					return fmt.Errorf("failed to send array item: %w", err)
				}
			}
			return nil
		}
		s := data.Types[t]
		if s != nil {
			m, ok := value.(map[string]interface{})
			if !ok {
				return fmt.Errorf("expected struct for field %s, got %T", name, value)
			}
			for _, field := range s {
				if err := sendValue(field.Type, field.Name, m[field.Name]); err != nil {
					return fmt.Errorf("failed to send struct field %s: %w", field.Name, err)
				}
			}
			return nil
		}
		var enc []byte
		var err error
		switch v := value.(type) {
		case string:
			if t == "string" {
				enc = []byte(v)
			} else if strings.HasPrefix(v, "0x") {
				enc, err = hex.DecodeString(v[2:])
				if err != nil {
					return fmt.Errorf("failed to decode hex string for field %s: %w", name, err)
				}
			} else {
				return fmt.Errorf("invalid string value for field %s: %s", name, v)
			}
		case bool:
			if v {
				enc = []byte{1}
			} else {
				enc = []byte{0}
			}
		case float64:
			enc = new(big.Int).SetInt64(int64(v)).Bytes()
		case *math.HexOrDecimal256:
			if v == nil {
				return fmt.Errorf("nil value for field %s", name)
			}
			h := big.Int(*v)
			enc = (&h).Bytes()

		default:
			return fmt.Errorf("unsupported type for field %s: %T", name, value)
		}

		payload := binary.BigEndian.AppendUint16([]byte{}, uint16(len(enc)))
		payload = append(payload, enc...)
		if _, err = w.ledgerExchange(ledgerOpEip712SendStructImpl, ledgerP1CompleteSend, ledgerP2StructField, payload); err != nil {
			return fmt.Errorf("failed to send domain field %s: %w", name, err)
		}
		return nil
	}

	// first send all the EIP-712 struct definitions
	for name, fields := range data.Types {
		_, err := w.ledgerExchange(ledgerOpEip712SendStructDef, 0, ledgerP2StructName, []byte(name))
		if err != nil {
			return nil, fmt.Errorf("failed to send type name %s: %w", name, err)
		}
		for _, field := range fields {
			if err := sendField(field); err != nil {
				return nil, fmt.Errorf("failed to send field %s: %w", field.Name, err)
			}
		}
	}

	// send the EIP-712 domain field values
	if _, err := w.ledgerExchange(ledgerOpEip712SendStructImpl, ledgerP1CompleteSend, ledgerP2RootStruct, []byte("EIP712Domain")); err != nil {
		return nil, fmt.Errorf("failed to send domain type name: %w", err)
	}
	if err := sendValue("EIP712Domain", "domain", data.Domain.Map()); err != nil {
		return nil, fmt.Errorf("failed to send domain fields: %w", err)
	}

	// send the message field values
	if _, err := w.ledgerExchange(ledgerOpEip712SendStructImpl, ledgerP1CompleteSend, ledgerP2RootStruct, []byte(data.PrimaryType)); err != nil {
		return nil, fmt.Errorf("failed to send primary type name: %w", err)
	}
	if err := sendValue(data.PrimaryType, "message", data.Message); err != nil {
		return nil, fmt.Errorf("failed to send primary type fields: %w", err)
	}

	// Flatten the derivation path into the Ledger request
	path := make([]byte, 1+4*len(derivationPath))
	path[0] = byte(len(derivationPath))
	for i, component := range derivationPath {
		binary.BigEndian.PutUint32(path[1+4*i:], component)
	}

	// Send the message over, ensuring it's processed correctly
	reply, err := w.ledgerExchange(ledgerOpSignTypedMessage, 0, ledgerP2FullImplementation, path)
	if err != nil {
		return nil, err
	}

	// Extract the Ethereum signature and do a sanity validation
	if len(reply) != crypto.SignatureLength {
		return nil, fmt.Errorf("invalid signature length: %d", len(reply))
	}
	signature := append(reply[1:], reply[0])
	return signature, nil
}
