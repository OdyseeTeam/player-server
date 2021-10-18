FROM alpine
EXPOSE 8080

# For /etc/mime.types
RUN apk add mailcap

WORKDIR /app
COPY dist/linux_amd64/lbrytv_player /app/

CMD ["./lbrytv_player"]
