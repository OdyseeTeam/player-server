FROM alpine
EXPOSE 8080

WORKDIR /app
COPY lbrytv_player /app/

CMD ["./lbrytv_player"]
