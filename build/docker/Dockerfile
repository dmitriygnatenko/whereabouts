FROM alpine:3.20

RUN apk add --no-cache ca-certificates

WORKDIR /app

COPY build/app/whereabouts /app/whereabouts

RUN chmod +x /app/whereabouts

EXPOSE 8080

ENTRYPOINT ["/app/whereabouts"]