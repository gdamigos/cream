# only built by goreleaser, which puts the binary under $TARGETPLATFORM
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/cream /usr/local/bin/cream
ENV CREAM_NO_UPDATE_CHECK=1
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/cream"]
