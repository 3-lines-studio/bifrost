FROM debian:trixie-slim AS builder
WORKDIR /app

RUN apt update && apt install -y curl unzip golang-go

RUN curl -fsSL https://bun.sh/install | bash
ENV PATH="/root/.bun/bin:${PATH}"

COPY package.json bun.lock ./
RUN bun install --frozen-lockfile

COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go run github.com/3-lines-studio/bifrost/cmd/build@latest main.go
RUN CGO_ENABLED=0 GOOS=linux go build -o app main.go

FROM debian:trixie-slim
WORKDIR /app
RUN apt update && apt install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY --from=builder /app/app ./app
EXPOSE 8080
CMD ["./app"]
