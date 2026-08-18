# -------------- build stage ----------------

FROM golang:1.26-alpine AS build

WORKDIR /src

# Copy the dependency manifests first.  This layer is cached and only
# invalidated when go.mod or go.sum changes, so source edits don't
# re-download the module graph.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a statically linked binary with no libc dependency,
# which is what lets the final image contain nothing but the binary.
#   -trimpath  strips local filesystem paths from the build
#   -s -w      omit the symbol table and DWARF debug info (smaller binary)
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/cairn ./cmd/cairn

# -------------- runtime stage ------------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/cairn /cairn

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/cairn"]