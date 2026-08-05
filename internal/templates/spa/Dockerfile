FROM debian:trixie-slim AS builder
WORKDIR /app

RUN apt update && apt install -y ca-certificates golang-go nodejs npm

COPY package.json ./
RUN npm install --legacy-peer-deps

COPY go.mod go.sum ./
RUN go mod download
RUN go install github.com/3-lines-studio/bifrost/cmd/bifrost@latest
COPY . .
RUN bifrost build ./main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o app main.go

FROM debian:trixie-slim
WORKDIR /app
RUN apt update && apt install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/app ./app
EXPOSE 8080
CMD ["./app"]
