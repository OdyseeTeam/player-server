FROM alpine
EXPOSE 8080

# This is for /etc/mime.types
RUN apk add mailcap

WORKDIR /app
COPY dist/lbrytv_player /app/

CMD ["./lbrytv_player"]
