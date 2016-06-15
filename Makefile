install_dev:
	@echo Installing development-branch of crypto and cosi
	@for r in crypto cosi; do \
		repo=github.com/dedis/$$r; \
		go get $$repo; \
		cd $$GOPATH/src/$$repo; \
		git checkout development; \
	done
	@cd $$GOPATH/src/github.com/dedis/cothority
	go get -t ./...

install:
	go get -t ./...

test_fmt:
	@echo Checking correct formatting of files
	@{ \
	files=$$( go fmt ./... ); \
	if [ -n "$$files" ]; then \
		echo "Files not properly formatted: $$files"; \
		exit 1; \
	fi; \
	if ! go vet ./...; then \
		exit 1; \
	fi \
	}

test_lint:
	@echo Checking linting of files
	@{ \
	go get -u github.com/golang/lint/golint; \
	failOn="should have comment or be unexported\| by other packages, and that stutters; consider calling this"; \
	failOn="$$failOn\|should be of the form"; \
	lintfiles=$$( golint ./... | grep "$$failOn" ); \
	if [ -n "$$lintfiles" ]; then \
		echo "Lint errors:"; \
		echo "$$lintfiles"; \
		exit 1; \
	fi \
	}

test: test_fmt test_lint
	go test -race -p=1 ./...

all: install test