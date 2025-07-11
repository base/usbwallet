# usbwallet

Fork of go-ethereum's [usbwallet package](https://github.com/ethereum/go-ethereum/tree/master/accounts/usbwallet)
with support for:
 - non-HID devices (recent Trezor firmware)
 - EIP-712 typed-data signatures
 - Personal message signatures

Should be a drop-in replacement.

### Usage

```go
package main

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/signer/core/apitypes"
	"github.com/base/usbwallet"
)

func main() {
	hub, _ := usbwallet.NewTrezorHubWithWebUSB()
	wallet := hub.Wallets()[0]
	_ = wallet.Open("")
	path, _ := accounts.ParseDerivationPath("m/44'/60'/0'/0/0")
	account, _ := wallet.Derive(path, true)
	data := apitypes.TypedData{
		Types: map[string][]apitypes.Type{
			"EIP712Domain": {{Name: "name", Type: "string"}},
			"Mail":         {{Name: "from", Type: "string"}, {Name: "to", Type: "string"}, {Name: "contents", Type: "string"}},
		},
		PrimaryType: "Mail",
		Domain:      apitypes.TypedDataDomain{Name: "example mail"},
		Message:     map[string]interface{}{"from": "from@example.com", "to": "to@example.com", "contents": "hello world"},
	}
	sig, _ := wallet.SignTypedData(account, data)
	fmt.Printf("Sig: 0x%x\n", sig)
}
```
