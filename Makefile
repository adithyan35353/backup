SHELL:=/bin/bash

APP_VERSION?=1.1.5

# build vars
BUILD_DATE:=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
REPOSITORY:=us.gcr.io/mihin-191723/common

#run vars
CONFIG:=$$(pwd)/test/config
TRAVIS:=$$(pwd)/test/travis

# go tools
PACKAGES:=$(shell go list ./... | grep -v '/vendor/')
VETARGS:=-asmdecl -atomic -bool -buildtags -copylocks -methods -nilfunc -rangeloops -shift -structtags -unsafeptr

travis:
	@echo ">>> Building kube-backup:build image"
	@docker build --build-arg APP_VERSION=$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) \
	    -t $(REPOSITORY)/kube-backup:build -f Dockerfile.build .
	@docker create --name kube-backup_extract $(REPOSITORY)/kube-backup:build
	@docker cp kube-backup_extract:/dist/kube-backup ./kube-backup
	@docker rm -f kube-backup_extract
	@echo ">>> Building kube-backup:$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) image"
	@docker build \
	    --build-arg BUILD_DATE=$(BUILD_DATE) \
	    --build-arg VCS_REF=$(TRAVIS_COMMIT) \
	    --build-arg VERSION=$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) \
	    -t $(REPOSITORY)/kube-backup:$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) .
	@rm ./kube-backup
	@echo ">>> Starting kube-backup container"
	@docker run -d --net=host --name kube-backup \
	    --restart unless-stopped \
	    -v "$(TRAVIS):/config" \
        $(REPOSITORY)/kube-backup:$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) \
		-ConfigPath=/config \
		-StoragePath=/storage \
		-TmpPath=/tmp \
		-LogLevel=info

publish:
	@echo $(DOCKER_PASS) | docker login -u "$(DOCKER_USER)" --password-stdin
	@docker tag $(REPOSITORY)/kube-backup:$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) $(REPOSITORY)/kube-backup:edge
	@docker push $(REPOSITORY)/kube-backup:edge

release:
	@echo $(DOCKER_PASS) | docker login -u "$(DOCKER_USER)" --password-stdin
	@docker tag $(REPOSITORY)/kube-backup:$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) $(REPOSITORY)/kube-backup:$(APP_VERSION)
	@docker tag $(REPOSITORY)/kube-backup:$(APP_VERSION).$(TRAVIS_BUILD_NUMBER) $(REPOSITORY)/kube-backup:latest
	@docker push $(REPOSITORY)/kube-backup:$(APP_VERSION)
	@docker push $(REPOSITORY)/kube-backup:latest

run: build
	@echo ">>> Starting kube-backup container"
	@docker run -dp 8090:8090 --name kube-backup-$(APP_VERSION) \
	    --restart unless-stopped \
	    -v "$(CONFIG):/config" \
        $(REPOSITORY)/kube-backup:$(APP_VERSION) \
		-ConfigPath=/config \
		-StoragePath=/storage \
		-TmpPath=/tmp \
		-LogLevel=info

build: clean
	@echo ">>> Building kube-backup:build image"
	@docker build --build-arg APP_VERSION=$(APP_VERSION) -t $(REPOSITORY)/kube-backup:build -f Dockerfile.build .
	@docker create --name kube-backup_extract $(REPOSITORY)/kube-backup:build
	@docker cp kube-backup_extract:/dist/kube-backup ./kube-backup
	@docker rm -f kube-backup_extract
	@echo ">>> Building kube-backup:$(APP_VERSION) image"
	@docker build -t $(REPOSITORY)/kube-backup:$(APP_VERSION) .
	@rm ./kube-backup

clean:
	@docker rm -f kube-backup-$(APP_VERSION) || true
	#@docker rmi $$(docker images | awk '$$1 ~ /kube-backup/ { print $$3 }') || true
	@docker volume rm $$(docker volume ls -f dangling=true -q) || true

backend:
	@docker run -dp 20022:22 --name kube-backup-sftp \
	    atmoz/sftp:alpine test:test:::backup
	@docker run -dp 20099:9000 --name kube-backup-s3 \
	    -e "MINIO_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE" \
	    -e "MINIO_SECRET_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" \
	    minio/minio server /export
	@mc config host add local http://localhost:20099 \
	    AKIAIOSFODNN7EXAMPLE wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY S3v4
	@sleep 5
	@mc mb local/backup

fmt:
	@echo ">>> Running go fmt $(PACKAGES)"
	@go fmt $(PACKAGES)

vet:
	@echo ">>> Running go vet $(VETARGS)"
	@go list ./... \
		| grep -v /vendor/ \
		| cut -d '/' -f 4- \
		| xargs -n1 \
			go tool vet $(VETARGS) ;\
	if [ $$? -ne 0 ]; then \
		echo ""; \
		echo "go vet failed"; \
	fi

.PHONY: build
