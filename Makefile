all: vet test testrace

build:
	go build github.com/qiaohao9/grpc/...

clean:
	go clean -i github.com/qiaohao9/grpc/...

deps:
	GO111MODULE=on go get -d -v github.com/qiaohao9/grpc/...

proto:
	@ if ! which protoc > /dev/null; then \
		echo "error: protoc not installed" >&2; \
		exit 1; \
	fi
	go generate github.com/qiaohao9/grpc/...

test:
	go test -cpu 1,4 -timeout 7m github.com/qiaohao9/grpc/...

testsubmodule:
	cd security/advancedtls && go test -cpu 1,4 -timeout 7m github.com/qiaohao9/grpc/security/advancedtls/...
	cd security/authorization && go test -cpu 1,4 -timeout 7m github.com/qiaohao9/grpc/security/authorization/...

testrace:
	go test -race -cpu 1,4 -timeout 7m github.com/qiaohao9/grpc/...

testdeps:
	GO111MODULE=on go get -d -v -t github.com/qiaohao9/grpc/...

vet: vetdeps
	./vet.sh

vetdeps:
	./vet.sh -install

.PHONY: \
	all \
	build \
	clean \
	proto \
	test \
	testrace \
	vet \
	vetdeps
