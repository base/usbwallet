.PHONY: gen
gen:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go generate ./trezor
