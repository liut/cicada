FROM alpine:3.17

ENV APP_HOME=/opt/app CICADA_REDIS_DSN=redis://redis:6379/0
RUN mkdir -p "${APP_HOME}"
WORKDIR "$APP_HOME"

COPY linux_amd64/cicada ${APP_HOME}

EXPOSE 1353/udp

ENTRYPOINT ["./cicada"]
CMD ["-serv"]
