CGO_ENABLED ?= 0
BINARY := xuanchu
CMD := ./cmd/xuanchu
VERSION ?= $(shell git describe --tags --exact-match 2>/dev/null || echo "")

.PHONY: all build clean

all: build

build: $(BINARY)

$(BINARY):
	$(if $(VERSION),\
		CGO_ENABLED=$(CGO_ENABLED) go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY) $(CMD),\
		CGO_ENABLED=$(CGO_ENABLED) go build -o $(BINARY) $(CMD))

clean:
	rm -f $(BINARY)
