VERSION=0.1.1
NAME=getfildata
REPO=harbor.filcoin.xyz:8080/filecoin

# .PHONY: docker
# docker:
# 	GOOS=linux GOARCH=amd64 go build
# 	@docker build -t $(REPO)/$(NAME):$(VERSION) -f ./dockerfiles/Dockerfile .

# .PHONY: push
# push:
# 	@docker push $(REPO)/$(NAME):$(VERSION)

# .PHONY: run
# run: 
# 	@docker run -it -p 1235:1235 -v /Users/huweixiong/code/filecoin/filecoin-get-blockdata/config.json:/config.json --rm $(REPO)/$(NAME):$(VERSION)

.PHONY: build
build:
	GOOS=linux GOARCH=amd64 go build ./cmd/kaspabridge
	
.PHONY: dev
dev:
	scp ./kaspabridge demo@192.168.11.79:/home/demo/app/
	scp ./cmd/kaspabridge/config.yaml demo@192.168.11.79:/home/demo/app/