CGO_ENABLED ?= 0
BINARY := taskg
CMD := ./cmd/taskg

.PHONY: all build clean

all: build

build: $(BINARY)

$(BINARY):
	CGO_ENABLED=$(CGO_ENABLED) go build -o $(BINARY) $(CMD)

clean:
	rm -f $(BINARY)
