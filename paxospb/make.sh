#~/bin/bash

# REQUIREMENTS:
# 1. Install google protobuf 3.0.0 or higher
# 2. go get github.com/gogo/protobuf/protoc-gen-gofast
# 3. go get google.golang.org/grpc
# 4. go get -u github.com/golang/protobuf/{proto,protoc-gen-go}
# protoc --go_out=plugins=grpc:. paxos.proto 
protoc --gofast_out=plugins=grpc:. paxos.proto 
