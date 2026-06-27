FROM alpine:latest
WORKDIR /app
ENV ASPNETCORE_URLS=http://+:8082
ENV DOTNET_SYSTEM_GLOBALIZATION_INVARIANT=1
ENV DOWNLOAD_PATH=/data/tiles
EXPOSE 8082
VOLUME ["/data/tiles"]
RUN printf '%s\n' \
    'http://mirrors.aliyun.com/alpine/v3.23/main' \
    'http://mirrors.aliyun.com/alpine/v3.23/community' \
    > /etc/apk/repositories \
    && apk add --no-cache libstdc++ libgcc ca-certificates
COPY publish-linux-musl/ ./
RUN chmod +x /app/TianDiTuDownLoader.Web
ENTRYPOINT ["/app/TianDiTuDownLoader.Web"]
