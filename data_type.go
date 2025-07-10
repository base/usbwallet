package usbwallet

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

type dataType byte

const (
	CustomType dataType = iota
	IntType
	UintType
	AddressType
	BoolType
	StringType
	FixedBytesType
	BytesType
)

var nameToType = map[string]dataType{
	"int":     IntType,
	"uint":    UintType,
	"address": AddressType,
	"bool":    BoolType,
	"string":  StringType,
	"bytes":   BytesType,
}

func parseType(data apitypes.TypedData, field apitypes.Type) (dt dataType, name string, byteLength int, arrayLevels []*int, err error) {
	name = strings.TrimSpace(field.Type)
	arrayLengths := regexp.MustCompile(`\[(\d*)]`).FindAllStringSubmatch(name, -1)
	if len(arrayLengths) > 0 {
		arrayLevels = make([]*int, len(arrayLengths))
		for i, arrayLength := range arrayLengths {
			if len(arrayLength[1]) == 0 {
				arrayLevels[i] = nil // nil means dynamic length
			} else {
				length, _ := strconv.Atoi(arrayLength[1]) // guaranteed to be a digit, ignore error
				arrayLevels[i] = &length
			}
		}
		name = name[0:strings.Index(name, "[")]
	}
	if data.Types[name] != nil {
		dt = CustomType
		return
	}

	matches := regexp.MustCompile(`^(.+?)(\d*)$`).FindStringSubmatch(name)
	name = matches[1]
	lengthStr := matches[2]

	var ok bool
	if dt, ok = nameToType[name]; !ok {
		err = fmt.Errorf("unknown type: %s", field.Type)
		return
	}

	byteLength, _ = strconv.Atoi(lengthStr)
	if dt == UintType || dt == IntType {
		if lengthStr == "" {
			byteLength = 32
		} else if byteLength%8 != 0 {
			err = fmt.Errorf("invalid length for %s: %s", field.Type, lengthStr)
			return
		} else {
			byteLength /= 8
		}
	} else if lengthStr != "" {
		if dt == BytesType {
			dt = FixedBytesType
		} else {
			err = fmt.Errorf("invalid type: %s", field.Type)
			return
		}
	} else if dt == AddressType {
		byteLength = 20 // address is always 20 bytes
	}
	if lengthStr != "" && (byteLength < 1 || byteLength > 32) {
		err = fmt.Errorf("invalid length for %s: %s", field.Type, lengthStr)
		return
	}

	return
}
