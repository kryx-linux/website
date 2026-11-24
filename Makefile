OUT      ?= public
PORT     ?= 8080
SITE_URL ?= https://kryx.anrix.org
BIN      := bin/sitegen
FLAGS    ?=

.PHONY: all build build-offline serve test vet verify fonts clean

all: build

$(BIN): $(shell find cmd -type f -name '*.go') go.mod
	@mkdir -p bin
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/sitegen

build: $(BIN)
	./$(BIN) -out $(OUT) -site-url $(SITE_URL) $(FLAGS)

build-offline: $(BIN)
	./$(BIN) -out $(OUT) -site-url $(SITE_URL) -offline $(FLAGS)

serve: build-offline
	@echo "Serving $(OUT)/ at http://127.0.0.1:$(PORT)/"
	./$(BIN) -serve :$(PORT) -out $(OUT)

fonts:
	./scripts/fetch-fonts.sh

vet:
	CGO_ENABLED=0 go vet ./...

test:
	CGO_ENABLED=0 go test -v ./...

verify: vet test build-offline
	@JS_COUNT="$$(find $(OUT) -type f -name '*.js' | wc -l)"; \
	if [ "$$JS_COUNT" -ne 0 ]; then \
		echo "ERROR: found $$JS_COUNT JavaScript file(s) in $(OUT)/" >&2; \
		exit 1; \
	fi; \
	if grep -rni '<script' $(OUT)/ >/dev/null 2>&1; then \
		echo "ERROR: found <script> tag in $(OUT)/" >&2; \
		exit 1; \
	fi; \
	echo "OK: 0 bytes of JavaScript in $(OUT)/"

clean:
	rm -rf $(OUT) bin
