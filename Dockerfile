# 1. Pin the builder stages to the runner's architecture using $BUILDPLATFORM
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build_deps

RUN apk add --no-cache git

WORKDIR /workspace

COPY go.mod .
COPY go.sum .

RUN go mod download

# This stage also inherits the --platform=$BUILDPLATFORM from build_deps
FROM build_deps AS build

# 2. Bring in the target OS and Architecture variables automatically set by Docker Buildx
ARG TARGETOS
ARG TARGETARCH

COPY . .

# 3. Pass the target OS and Arch to the Go compiler
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o webhook -ldflags '-w -extldflags "-static"' .

# 4. The final stage does NOT use $BUILDPLATFORM. 
# Docker Buildx will automatically pull the correct Alpine architecture based on the target platform.
FROM alpine:3.23@sha256:85fe1e81d6758c208f3e1eed4338a1997e19d4be002d4dd32d3100c9a8c010a0

RUN apk add --no-cache ca-certificates

COPY --from=build /workspace/webhook /usr/local/bin/webhook

ENTRYPOINT ["webhook"]