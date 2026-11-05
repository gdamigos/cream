# only built by goreleaser, which puts the binary under $TARGETPLATFORM
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
ARG TARGETPLATFORM
COPY $TARGETPLATFORM/cream /usr/local/bin/cream
ENV CREAM_NO_UPDATE_CHECK=1
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/cream"]
