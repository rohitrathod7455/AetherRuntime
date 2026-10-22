.PHONY: build run-daemon clean clean-all dashboard setup-rootfs test benchmark demo stress

build:
	go build -o bin/aetherd ./cmd/aetherd

setup-rootfs:
	bash scripts/setup-rootfs.sh

demo:
	bash scripts/demo.sh

stress:
	bash scripts/stress_test.sh

test:
	go test -v ./runtime/...

benchmark:
	bash scripts/benchmark.sh

run-daemon: build
	./bin/aetherd

dashboard:
	cd dashboard && npm run dev

clean:
	rm -rf bin/ logs/

clean-all:
	rm -rf bin/ rootfs/ logs/
