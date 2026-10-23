package usbwallet

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/signer/core/apitypes"
)

func TestEncodeInteger(t *testing.T) {
	tests := []struct {
		name   string
		value  interface{}
		signed bool
		size   int
		want   string
	}{
		{"decimal string", "560048", false, 32, "0000000000000000000000000000000000000000000000000000000000088bb0"},
		{"hex string", "0x88bb0", false, 32, "0000000000000000000000000000000000000000000000000000000000088bb0"},
		{"json number", float64(36), false, 32, "0000000000000000000000000000000000000000000000000000000000000024"},
		{"big.Int", big.NewInt(1), false, 1, "01"},
		{"zero", "0", false, 32, "0000000000000000000000000000000000000000000000000000000000000000"},
		{"negative int", float64(-1), true, 2, "ffff"},
		{"min int8", "-128", true, 1, "80"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := encodeInteger(tt.value, tt.signed, tt.size)
			if err != nil {
				t.Fatal(err)
			}
			if hex.EncodeToString(got) != tt.want {
				t.Fatalf("got %x, want %s", got, tt.want)
			}
		})
	}
}

func TestEncodeIntegerErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		value  interface{}
		signed bool
		size   int
	}{
		"unsupported type":  {true, false, 32},
		"nil":               {nil, false, 32},
		"fractional float":  {1.5, false, 32},
		"not a number":      {"abc", false, 32},
		"uint overflow":     {"256", false, 1},
		"negative for uint": {"-1", false, 32},
		"int overflow":      {"128", true, 1},
	} {
		if _, err := encodeInteger(tc.value, tc.signed, tc.size); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

// TypedDataDomain.Map() returns chainId as *math.HexOrDecimal256. It used to be encoded as an
// empty value, so the device signed a domain with chainId 0 and the signature did not recover.
func TestEncodeIntegerDomainChainId(t *testing.T) {
	var td apitypes.TypedData
	if err := json.Unmarshal([]byte(`{
		"types": {"EIP712Domain": [{"name": "chainId", "type": "uint256"}, {"name": "verifyingContract", "type": "address"}]},
		"primaryType": "EIP712Domain",
		"domain": {"chainId": "560048", "verifyingContract": "0x856611eD7E07D83243b15E93f6321f2df6865852"},
		"message": {}
	}`), &td); err != nil {
		t.Fatal(err)
	}
	got, err := encodeInteger(td.Domain.Map()["chainId"], false, 32)
	if err != nil {
		t.Fatal(err)
	}
	if want := big.NewInt(560048).FillBytes(make([]byte, 32)); !bytes.Equal(got, want) {
		t.Fatalf("got %x, want %x", got, want)
	}
}
