POOLNAME=k_pool

.PHONY: build
build:
	GOOS=linux GOARCH=amd64 go build -o ${POOLNAME} ./cmd/kaspabridge
	
.PHONY: dev
dev:
	scp ./k_pool demo@192.168.11.75:/home/demo/kaspa/${POOLNAME}
	scp ./cmd/kaspabridge/config.yaml demo@192.168.11.75:/home/demo/kaspa/