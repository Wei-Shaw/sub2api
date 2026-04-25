module github.com/Wei-Shaw/sub2api/plugins/hello-world

go 1.26.2

require github.com/Wei-Shaw/sub2api/plugin-sdk v0.0.0

require (
	golang.org/x/net v0.41.0 // indirect
	golang.org/x/sys v0.33.0 // indirect
	golang.org/x/text v0.26.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20250707201910-8d1bb00bc6a7 // indirect
	google.golang.org/grpc v1.75.1 // indirect
	google.golang.org/protobuf v1.36.10 // indirect
)

replace github.com/Wei-Shaw/sub2api/plugin-sdk => ../../plugin-sdk
