FROM golang:1.25 AS builder
WORKDIR /workspace

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/   cmd/
COPY pkg/   pkg/

ARG GIT_VERSION=dev
ARG GIT_COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-s -w \
      -X github.com/aws-controllers-k8s/ack-event-publisher/pkg/version.GitVersion=${GIT_VERSION} \
      -X github.com/aws-controllers-k8s/ack-event-publisher/pkg/version.GitCommit=${GIT_COMMIT} \
      -X github.com/aws-controllers-k8s/ack-event-publisher/pkg/version.BuildDate=${BUILD_DATE}" \
    -o ack-event-publisher \
    ./cmd/...

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /workspace/ack-event-publisher /ack-event-publisher
USER 65532:65532
ENTRYPOINT ["/ack-event-publisher"]
