APP_NAME := kontrol
CMD := ./cmd/kontrol
DIST := dist

GOOS_ARCH := \
linux/amd64 \
linux/arm64 \
darwin/amd64 \
darwin/arm64 \
windows/amd64

.PHONY: build build-cross test clean

build:
@mkdir -p $(DIST)
go build -o $(DIST)/$(APP_NAME) $(CMD)

build-cross:
@mkdir -p $(DIST)
@for target in $(GOOS_ARCH); do \
GOOS=$${target%/*}; \
GOARCH=$${target#*/}; \
out="$(DIST)/$(APP_NAME)-$$GOOS-$$GOARCH"; \
if [ "$$GOOS" = "windows" ]; then out="$$out.exe"; fi; \
echo "building $$out"; \
CGO_ENABLED=0 GOOS=$$GOOS GOARCH=$$GOARCH go build -o $$out $(CMD); \
done

test:
go test ./...

clean:
rm -rf $(DIST)
