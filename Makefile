POOLNAME=k_pool

.PHONY: build
build:
	GOOS=linux GOARCH=amd64 go build -o ${POOLNAME} ./cmd/kaspabridge
	
# scp ./k_pool demo@192.168.11.115:/home/demo/kaspa/${POOLNAME}
# scp ./cmd/kaspabridge/config.yaml demo@192.168.11.115:/home/demo/kaspa/
.PHONY: dev
dev:
	scp ./k_pool demo@192.168.11.75:/home/demo/kaspa/${POOLNAME}
	scp ./cmd/kaspabridge/config.yaml demo@192.168.11.75:/home/demo/kaspa/