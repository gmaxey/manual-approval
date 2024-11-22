FROM golang:1.23-alpine AS build

WORKDIR /work

COPY go.mod* go.sum* ./

RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -tags netgo -ldflags '-w -extldflags "-static"' -o /usr/local/bin/manual-approval main.go

FROM gcr.io/kaniko-project/executor:v1.23.2

COPY --from=build /usr/local/bin/manual-approval /usr/local/bin/manual-approval

ENTRYPOINT ["manual-approval"]
