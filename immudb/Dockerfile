# Extend the official distroless immudb image with wget for healthchecks
FROM busybox:stable-musl AS tools

FROM codenotary/immudb:latest
COPY --from=tools /bin/wget /usr/local/bin/wget
