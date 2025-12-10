module github.com/ekodb/ekodb-client-go/examples

go 1.24.0

toolchain go1.24.2

replace github.com/ekodb/ekodb-client-go => ../

require github.com/ekodb/ekodb-client-go v0.0.0-00010101000000-000000000000

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
)
